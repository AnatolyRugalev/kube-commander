# M1 — Kube Layer (in-process)

**Status:** `done` (2026-07-31) — every exit criterion is ticked: M1-INT-a proved the last one, group isolation, against a live envtest apiserver (D186), and the remaining envtest slices (M1-INT-b/c/d) deepen coverage rather than gate the milestone. See the journal for the per-leg history.
**Phase:** REWRITE_PLAN Phase 1

## Goal

A self-contained `internal/kube` package that exposes clusters to the TUI with
no TUI dependencies: discovery, live resource tables, and the action set —
all in-process via client-go, fault-tolerant, and fast to start.

## Scope

- clientset + dynamic client + discovery + RESTMapper from kubeconfig/context/flags.
- **Async, cached discovery** (the cold-start fix):
  - Seed set of core GVKs with static REST mapping for instant availability.
  - Background full discovery → emits a "discovery ready" signal to reconcile the menu.
  - Per-group fault isolation: one failing/denied API group degrades only itself (**#87**, **#76**).
  - On-disk discovery cache (kubectl-style) with invalidation; lazy group detail on first open.
- **Server-side Table List+Watch** via dynamic client (`Accept: as=Table`) → a
  Go channel of add/modify/delete/reset events. Generic over all resources incl. CRDs.
- Watch reconnect/resync on expiry; bounded buffering.
- Actions (in-process): delete, scale, rollout restart, cordon/drain, cronjob suspend, get-logs stream, port-forward, describe (kubectl/pkg/describe), get object as YAML.
- Graceful, typed errors — never panic on bad namespace/context (**#86**, old #55).
- Test strategy (D18): client-go **fake clients** (incl. fake discovery) by
  default — hermetic, runs anywhere. **envtest** integration tests opt-in behind
  `KUBECOM_TEST_ENVTEST=1`; harness lands here (M1-00), not M0.

## Exit criteria

- [x] List+watch any discovered resource, columns matching `kubectl get`.
      _(M1-05a: **List** — `Clients.List` server-prints any resource as a Table (`Accept: as=Table`), columns kubectl-identical for built-ins + CRDs (`internal/kube/table.go`, D33). M1-05b: **Watch** — `Clients.Watch` streams `ADDED/MODIFIED/DELETED/RESET/ERROR` deltas on a bounded channel via a reconnecting List→Watch goroutine (`internal/kube/watch.go`, D34); resumes off resourceVersion, re-Lists on 410 Gone, caches columns per connection. Reconnect/resync + fault paths covered by hermetic tests; live-server exercise is envtest territory.)_
- [x] Discovery never blocks a caller; seed resources usable before full discovery finishes.
      _(M1-02: seed RESTMapper resolves core GVKs instantly with no network I/O; M1-03: `StartDiscovery` runs the full pass in a background goroutine and delivers a one-shot reconcile signal, never blocking the caller. M1-04: discovery is now on-disk cached (kubectl's `discovery/cached/disk`, per-host dir under `os.UserCacheDir()/kubecom`, TTL 6h, `Clients.Invalidate()` to force a refetch) so warm starts skip the network; still zero network I/O at construction. Lazy group-detail-on-open (M1-04b) was
      later **retired as obsolete** (D81): the realized M2 menu is a flat
      Dashboard-sectioned list with no group-open interaction (D77), and the
      non-blocking cold-start intent is already met by seed + async discovery + the
      on-disk cache — so there is nothing left to defer without an unwanted UI change.)_
- [x] A denied/broken API group is isolated. _Unit-covered since M1-03 and, as of
      **M1-INT-a** (2026-07-31, D186), proven against a live apiserver — the deferral
      to a human/CI (D66) turned out to rest on a false premise, envtest runs in the
      agent sandbox in 12 s. Both halves are now live-tested in
      `internal/kube/envtest_isolation_test.go`: a **broken** group (an aggregated
      `v1beta1.metrics.k8s.io` APIService whose backing service is missing — the
      metrics-server outage of #87) leaves `DiscoveryResult.Err` nil and every healthy
      kind in place, and a **denied** resource (a real pods-only RBAC user) fails only
      its own call, as `KindForbidden`, while discovery still returns the full menu.
      The criterion is isolation and isolation holds; what the same test found broken
      was the *reporting* — `DiscoveryResult.Failed` never named the culprit on an
      aggregated-discovery cluster — tracked as **DISC-01** and **fixed 2026-07-31**
      (D187): the pass now reports every group the server serves no version of, and
      the test's tripwire is the positive assertion that the broken group is named._
- [x] Logs stream, describe, and YAML-get return correct output in-process.
      _(M1-07a: **YAML-get** done — `Clients.GetYAML` renders any resource (built-in or CRD) as kubectl-identical `get -o yaml` through the dynamic client, managedFields stripped, `sigs.k8s.io/yaml` (`internal/kube/yaml.go`, D41). M1-07b: **describe** done — `Clients.Describe` renders `kubectl describe`-identical output in-process by reusing kubectl's own describe generators (built-in describer by GroupKind + generic-unstructured fallback for CRDs) (`internal/kube/describe.go`, D42). M1-07c: **logs stream** done — `Clients.Logs` streams a pod container's logs onto a bounded `LogEvent` channel via the typed clientset `pods/log` subresource, `LogOptions` mirroring `kubectl logs` flags, opened in-goroutine so it never blocks first paint (`internal/kube/logs.go`, D43). M1-07d: **reconnecting/resuming follow logs** done — a `Follow` stream now survives a transient transport drop à la watch (D34): it forces server-side timestamps on the wire, reconnects with `SinceTime` at the last-seen line's second, and dedups the lines the server re-serves for that (second-granular) second; clean EOF stops, a reconnect failure is transient (silent backoff+retry, bounded by ctx). Timestamps stripped before delivery unless `opts.Timestamps` (D44). **M1-07 viewers complete.**)_
- [x] Port-forward runs in a background goroutine and can be stopped.
      _(M1-08: `Clients.PortForward(ctx, ref, ports)` forwards local ports to a pod in a background goroutine over an SPDY dialer to the pod's `portforward` subresource (`spdy.RoundTripperFor(c.Config)` + `spdy.NewDialer`), the in-process `kubectl port-forward` (D2). Returns a channel-based `PortForward` handle — `Ready`/`Done`/`Err`/`Ports`/`Stop` (idempotent `sync.Once`) — with the result handed off through a channel close, no mutex (principle 1); ctx cancellation stops it à la Logs/Watch (`internal/kube/portforward.go`, D45). Lifecycle (ready→ports→stop, fatal-error, ctx-cancel, factory-error, idempotent Stop) covered by an injected-factory + fakeForwarder hermetic test, `-race` clean; a live forward is envtest territory.)_
- [x] Tests cover discovery, watch reconnect, and the action set — fakes by
      default, envtest opt-in (D18). _Hermetic fake-client coverage is in place
      for discovery, watch reconnect (410/Gone → re-List), and every action. The
      envtest layer is **no longer deferred** (D186 pt 1): it now covers group
      isolation (M1-INT-a) and **both** watch-reconnect branches against a live
      apiserver — resume after a real transport drop (M1-INT-b-1) and re-List
      after a real 410/Expired (M1-INT-b-2). The action set (M1-INT-c) and a CI
      job (M1-INT-d) remain as coverage-deepening backlog items, not milestone
      gates._
- [x] Zero TUI imports in `internal/kube`. _Verified 2026-07-20: no
      bubbletea/lipgloss/bubbles/`internal/tui` import anywhere under
      `internal/kube`._

## Depends on
M0 skeleton + envtest harness.
