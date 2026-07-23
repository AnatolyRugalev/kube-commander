# M3 — Actions & Viewers

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 3

_Scope expanded into ordered, leg-sized Backlog slices **M3-01 … M3-15** on the
[board](../tasks/board.md) (M3-PLAN, D105). M3 is almost entirely the TUI surface —
the kube layer already has every verb (logs/describe/YAML/actions/port-forward from
M1) — so the slices wire those into a shared read-only viewer (M3-01), an actions
surface off the reserved nav keys (M3-02), the viewers (M3-03…08), the confirm-modal
action wiring (M3-09…12, M3-09 unblocks M2-14b), the port-forward panel (M3-13), and
the exec/edit suspend flows (M3-14/15). Per-leg history: `vault/journal/`._

## Goal

Make kubecom *operate*, not just observe — in-TUI viewers and the curated action
set, killing nearly all kubectl shell-outs.

## Scope

- **Logs viewer** (in-TUI viewport): client-go stream, follow/previous, container
  picker. Enable logs for pod-owning resources — Deployment/RS/StatefulSet/
  DaemonSet/Job (**#84**).
- **Describe viewer**: `kubectl/pkg/describe` in-process → viewport.
- **YAML viewer**: get object as YAML → viewport (no external pager).
- **Secret viewer** with reveal/decode + copy (**#89**).
- **Workload actions** (generic): delete, scale, rollout restart, cordon/drain,
  cronjob suspend/resume (**#83**), with confirm modal.
- **Port-forward manager**: background goroutines; a panel listing active
  forwards; start/stop without suspending the UI.
- **Exec shell**: `tea.ExecProcess` suspend → raw PTY via `remotecommand`
  (fallback `kubectl exec`). Linux/macOS only.
- **Edit**: suspend to `$EDITOR`, apply on save.
- **Finalize action keymap** off the reserved nav keys (no `h j k l n g G /`) —
  actions behind a leader / actions menu; update [`../knowledge/keybindings.md`](../knowledge/keybindings.md)
  and generate the help/keybindings doc from it.

## Exit criteria

- [x] Logs/describe/YAML render in-TUI for the relevant kinds; no external pager needed. (logs cover pods + pod-owning kinds via a resolved backing pod, M3-05…07b/D112; describe M3-04, YAML M3-03)
- [ ] Secret contents viewable with explicit reveal + copy.
- [ ] Each workload action works with a confirmation step and reports success/failure to the status bar.
- [ ] Port-forwards run in background, are listed, and stop cleanly on exit.
- [ ] Exec drops into a working shell and restores the TUI afterward.
- [ ] Edit round-trips through `$EDITOR`.

## Depends on
M1 actions/streaming + M2 viewport/modal components.
