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
- **Restore remote `go install`**: tag a real `v1.x.x` release so
  `go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@latest` works
  again (today `@v1` is semver-parsed as a version query, not the branch — see
  FB-go-install / journal 2026-07-20). Renaming `v1`→`main` also dissolves the
  branch-vs-semver collision. Update the README install section back to the remote
  one-liner once tagged.
- Merge/prepare `v1` toward becoming the default branch when ready.

## Exit criteria

- [ ] README + keybindings docs current and accurate.
- [ ] `goreleaser release` produces Linux+macOS artifacts from a tag via CI.
- [ ] Homebrew/AUR/Docker install paths verified.
- [ ] Migration verified from a real legacy config file.
- [ ] Definition of Done in [`../goals.md`](../goals.md) fully checked.

## Depends on
M0 CI/goreleaser scaffolding; feature milestones M1–M4 complete.
