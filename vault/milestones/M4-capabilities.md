# M4 — New Capabilities

**Status:** `todo`
**Phase:** REWRITE_PLAN Phase 4

## Goal

The capabilities the original lacked, now natural on the new architecture.

## Scope

- **Context/cluster switcher** in-UI + current context in the top bar (**#80**, old #79).
- **Sort by column** (**#85**); column-aware coloring (pod phase, restarts, readiness).
- **Owner → children drill-down** (Deployment → Pods, Node → Pods, etc.).
- **Metrics** (CPU/mem) via `metrics.k8s.io` when the API is available; hidden otherwise.
- **Theme selection**; ship a couple of solid built-ins (port monokai/solarized).

## Exit criteria

- [ ] Switch context without restarting; watches and menu rebind to the new cluster.
- [ ] Any column sortable; sort indicator visible; stable under live updates.
- [ ] Drill-down navigates from an owner to its pods and back.
- [ ] Metrics columns appear only when metrics-server is present; absence is silent.
- [ ] At least two themes selectable and persisted.

## Depends on
M2 browse shell + M1 discovery (context switch = rebuild client + discovery).
