# M5 — Release & Docs

**Status:** `todo`
**Phase:** REWRITE_PLAN Phase 5

## Goal

Ship v1: documented, packaged, and installable, with a clean migration story
from the old kube-commander.

## Scope

- Rewrite README for kubecom: what/why, install, usage, WSL2 note for Windows.
- Screencast via **vhs** (replaces the old terminalizer GIF pipeline).
- Keybindings reference (generated from the `keys/` bindings where possible).
- Migration note: old `~/.kubecom.yaml` auto-migration + any behavior changes.
- **Distribution** (**#28**): goreleaser release, Homebrew tap, AUR refresh,
  Docker image (Linux/macOS binaries only).
- Merge/prepare `v1` toward becoming the default branch when ready.

## Exit criteria

- [ ] README + keybindings docs current and accurate.
- [ ] `goreleaser release` produces Linux+macOS artifacts from a tag via CI.
- [ ] Homebrew/AUR/Docker install paths verified.
- [ ] Migration verified from a real legacy config file.
- [ ] Definition of Done in [`../goals.md`](../goals.md) fully checked.

## Depends on
M0 CI/goreleaser scaffolding; feature milestones M1–M4 complete.
