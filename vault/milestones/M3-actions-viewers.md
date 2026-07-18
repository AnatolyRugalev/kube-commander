# M3 — Actions & Viewers

**Status:** `todo`
**Phase:** REWRITE_PLAN Phase 3

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

## Exit criteria

- [ ] Logs/describe/YAML render in-TUI for the relevant kinds; no external pager needed.
- [ ] Secret contents viewable with explicit reveal + copy.
- [ ] Each workload action works with a confirmation step and reports success/failure to the status bar.
- [ ] Port-forwards run in background, are listed, and stop cleanly on exit.
- [ ] Exec drops into a working shell and restores the TUI afterward.
- [ ] Edit round-trips through `$EDITOR`.

## Depends on
M1 actions/streaming + M2 viewport/modal components.
