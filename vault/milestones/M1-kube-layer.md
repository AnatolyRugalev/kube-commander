# M1 — Kube Layer (in-process)

**Status:** `in-progress` (2026-07-19 — active milestone; M0 complete; envtest harness M1-00, client bootstrap M1-01, static seed RESTMapper M1-02, async full discovery M1-03, on-disk discovery cache M1-04, server-side Table List M1-05a + Watch M1-05b landed — List+watch exit criterion met; action set — M1-06a generic **delete** (D35) + M1-06b **scale**+**rollout-restart** (D36) + M1-06c **cordon/uncordon** (D37) + M1-06d **cronjob suspend/resume** (D38) landed; only M1-06e drain remains, then M1-07 logs/describe/YAML, M1-08 port-forward, M1-09 typed errors)
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
- [ ] Logs stream, describe, and YAML-get return correct output in-process.
- [ ] Port-forward runs in a background goroutine and can be stopped.
- [ ] Tests cover discovery, watch reconnect, and the action set — fakes by
      default, envtest opt-in (D18).
- [ ] Zero TUI imports in `internal/kube`.

## Depends on
M0 skeleton + envtest harness.
