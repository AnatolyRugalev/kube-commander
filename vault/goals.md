# Top-Level Goals

## Vision

Rewrite the 2020 kube-commander into **kubecom**: a fast, approachable,
keyboard-driven terminal UI for observing and operating Kubernetes clusters.
Keep the original idea — *"kubernetes-dashboard in your terminal"* — while
replacing the aging foundation with a modern Go TUI stack and closing the
long-standing issues.

## Primary goal

Ship a Kubernetes TUI on **Bubble Tea + client-go** that reaches feature parity
with the original, eliminates its data-race / focus / redraw bug class by
construction, removes the hard `kubectl` binary dependency, and adds the
high-value capabilities the original lacked.

## Definition of Done (v1)

- [ ] Two-pane browse UX (resource menu + live-watched table) at parity with the original.
- [ ] Resource listing works generically for **any** resource incl. CRDs (discovery-driven, kubectl-identical columns).
- [ ] In-TUI logs, describe, and YAML viewers (no external pager required).
- [ ] Core actions in-process: delete, scale, rollout restart, cordon/drain, port-forward (background), view secrets.
- [ ] Exec shell + `$EDITOR` edit (the only sanctioned TUI-suspending actions).
- [ ] Context/cluster switcher; namespace switcher; filter; sort by column.
- [ ] **Vim-first navigation** (`hjkl`, `gg`/`G`, `/`, `n`/`N`) with arrows/classic keys as an equivalent fallback.
- [ ] **Fully configurable keybindings** — every action rebindable via config; zero hard-coded keys in view code.
- [ ] **Cold start is responsive** — UI renders immediately; discovery is async and cached.
- [ ] No `kubectl` binary required for anything except the exec fallback.
- [ ] Plain-YAML config with one-shot migration from the old `~/.kubecom.yaml`.
- [ ] Linux + macOS release artifacts via goreleaser + GitHub Actions; tests green.
- [ ] Every open GH issue in scope is resolved or explicitly deferred with a reason.

## Non-goals

- **Native Windows support** — dropped. Windows users run kubecom under **WSL2**.
- **Cloning k9s.** k9s is feature-dense but its UX is deliberately *not* our
  model; kubecom stays simpler and more approachable.
- Cluster mutation beyond the curated action set (no arbitrary `apply`, no manifests authoring).
- Multi-cluster dashboards / server mode. Single-context, local, zero-deploy.

## Principles

1. **No shared mutable UI state.** Concurrency flows through Bubble Tea messages,
   not mutexes. This is the whole reason for the framework choice — protect it.
2. **In-process first.** Reach for client-go before shelling out. Shell out only
   where genuine interactivity requires it (exec, editor).
3. **Degrade, don't crash.** A missing API group, RBAC denial, or bad namespace
   degrades one feature; it never panics or blocks the UI.
4. **Fast cold start.** Never block first paint on discovery or network round-trips.
5. **Discoverable process.** Knowledge and state live in the vault, not in an
   agent's head or a chat log.
6. **Vim-first, never vim-only.** `hjkl` and friends are the primary path; arrows
   and classic keys are always an equivalent fallback. See
   [`knowledge/keybindings.md`](knowledge/keybindings.md).
7. **Zero hard-coded keys.** All input flows through a configurable action
   registry; no view matches a raw key. Every binding is rebindable via config.
8. **Keep it runnable; dogfood it.** Once the binary launches (M2-RUN), it stays
   launchable every leg. A human periodically installs `kubecom` and runs it
   against a real cluster; each leg must incrementally improve — never regress —
   that real-cluster experience, and keep the README's install/usage current
   (D68). Fake-tested parts are not "done" until they work in the running binary.

See [`REWRITE_PLAN.md`](REWRITE_PLAN.md) for the full architecture and
rationale, and [`knowledge/decisions.md`](knowledge/decisions.md) for the locked
decisions.
