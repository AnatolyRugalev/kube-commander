# M1 — Kube Layer (in-process)

**Status:** `todo`
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

- [ ] List+watch any discovered resource, columns matching `kubectl get`.
- [ ] Discovery never blocks a caller; seed resources usable before full discovery finishes.
- [ ] A denied/broken API group is isolated (integration test with restricted RBAC).
- [ ] Logs stream, describe, and YAML-get return correct output in-process.
- [ ] Port-forward runs in a background goroutine and can be stopped.
- [ ] Tests cover discovery, watch reconnect, and the action set — fakes by
      default, envtest opt-in (D18).
- [ ] Zero TUI imports in `internal/kube`.

## Depends on
M0 skeleton + envtest harness.
