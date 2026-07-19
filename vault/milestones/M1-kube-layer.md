# M1 — Kube Layer (in-process)

**Status:** `in-progress` (2026-07-19 — active milestone; M0 complete; envtest harness M1-00, client bootstrap M1-01, static seed RESTMapper M1-02, async full discovery M1-03, on-disk discovery cache M1-04, server-side Table List M1-05a + Watch M1-05b landed — List+watch exit criterion met; **action set M1-06 complete** — generic **delete** (D35), **scale**+**rollout-restart** (D36), **cordon/uncordon** (D37), **cronjob suspend/resume** (D38), and **drain** — 06e-1 **pod selection** (D39) + 06e-2 **eviction loop** `Clients.Drain` (D40, policy/v1 Eviction API, PDB-aware 429-retry, cordon-first, wait-for-deletion) — all landed; **M1-07 viewers** — 07a **get-as-YAML** (`Clients.GetYAML`, D41) + 07b **describe** (`Clients.Describe` via kubectl/pkg/describe, D42) + 07c **streaming pod logs** (`Clients.Logs` via the typed `pods/log` subresource, D43) + 07d **reconnecting/resuming follow logs** (`Clients.Logs` Follow survives a transient drop: forces wire timestamps, resumes via `SinceTime`, dedups the re-served second, D44) landed — **M1-07 viewers complete**; **M1-08 background port-forward** — `Clients.PortForward` forwards local ports to a pod in a background goroutine over an SPDY dialer to the pod's `portforward` subresource, returning a channel-based `PortForward` handle (`Ready`/`Done`/`Err`/`Ports`/`Stop`), stoppable via `Stop` or ctx cancellation, D45 — landed; next up M1-09 typed errors)
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
      _(M1-02: seed RESTMapper resolves core GVKs instantly with no network I/O; M1-03: `StartDiscovery` runs the full pass in a background goroutine and delivers a one-shot reconcile signal, never blocking the caller. M1-04: discovery is now on-disk cached (kubectl's `discovery/cached/disk`, per-host dir under `os.UserCacheDir()/kubecom`, TTL 6h, `Clients.Invalidate()` to force a refetch) so warm starts skip the network; still zero network I/O at construction. Lazy group-detail-on-open → M1-04b, deferred to M2.)_
- [ ] A denied/broken API group is isolated (integration test with restricted RBAC).
- [x] Logs stream, describe, and YAML-get return correct output in-process.
      _(M1-07a: **YAML-get** done — `Clients.GetYAML` renders any resource (built-in or CRD) as kubectl-identical `get -o yaml` through the dynamic client, managedFields stripped, `sigs.k8s.io/yaml` (`internal/kube/yaml.go`, D41). M1-07b: **describe** done — `Clients.Describe` renders `kubectl describe`-identical output in-process by reusing kubectl's own describe generators (built-in describer by GroupKind + generic-unstructured fallback for CRDs) (`internal/kube/describe.go`, D42). M1-07c: **logs stream** done — `Clients.Logs` streams a pod container's logs onto a bounded `LogEvent` channel via the typed clientset `pods/log` subresource, `LogOptions` mirroring `kubectl logs` flags, opened in-goroutine so it never blocks first paint (`internal/kube/logs.go`, D43). M1-07d: **reconnecting/resuming follow logs** done — a `Follow` stream now survives a transient transport drop à la watch (D34): it forces server-side timestamps on the wire, reconnects with `SinceTime` at the last-seen line's second, and dedups the lines the server re-serves for that (second-granular) second; clean EOF stops, a reconnect failure is transient (silent backoff+retry, bounded by ctx). Timestamps stripped before delivery unless `opts.Timestamps` (D44). **M1-07 viewers complete.**)_
- [x] Port-forward runs in a background goroutine and can be stopped.
      _(M1-08: `Clients.PortForward(ctx, ref, ports)` forwards local ports to a pod in a background goroutine over an SPDY dialer to the pod's `portforward` subresource (`spdy.RoundTripperFor(c.Config)` + `spdy.NewDialer`), the in-process `kubectl port-forward` (D2). Returns a channel-based `PortForward` handle — `Ready`/`Done`/`Err`/`Ports`/`Stop` (idempotent `sync.Once`) — with the result handed off through a channel close, no mutex (principle 1); ctx cancellation stops it à la Logs/Watch (`internal/kube/portforward.go`, D45). Lifecycle (ready→ports→stop, fatal-error, ctx-cancel, factory-error, idempotent Stop) covered by an injected-factory + fakeForwarder hermetic test, `-race` clean; a live forward is envtest territory.)_
- [ ] Tests cover discovery, watch reconnect, and the action set — fakes by
      default, envtest opt-in (D18).
- [ ] Zero TUI imports in `internal/kube`.

## Depends on
M0 skeleton + envtest harness.
