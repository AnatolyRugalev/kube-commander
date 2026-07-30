# M5 — Release & Docs

**Status:** `in-progress` (2026-07-30) — the release pipeline exists end-to-end (M5-02/03) and M5-01a closed the last 2020 parity gap; the remaining slices are migration, distribution, the screencast and the two human-performed ends.
**Phase:** REWRITE_PLAN Phase 5

_Scope expanded into ordered, leg-sized Backlog slices **M5-01 … M5-11** on the
[board](../tasks/board.md) (M5-PLAN, D173). M5 is unlike its predecessors in one way that
shapes the plan: **its output leaves the repo and cannot be recalled** — a pushed tag is
cached immutably by the Go module proxy — so "green or revert" does not apply and D173
splits every publishing act off to a human. The slices are: the DoD audit (M5-01), the
artifact's correctness (M5-02 build metadata, M5-03 release workflow + CI dry run),
migration (M5-04/05), distribution (M5-06 Homebrew, M5-07 AUR, M5-08 Docker), the vhs
screencast (M5-09), and the irreversible end (M5-10 tag, M5-11 `v1`→`main`). Per-leg
history: `vault/journal/`._

## Goal

Ship v1: documented, packaged, and installable, with a clean migration story
from the old kube-commander.

## Scope

- Rewrite README for kubecom: what/why, install, usage, WSL2 note for Windows —
  **largely already done**, and continuously, because D68 makes every leg that moves the
  install/launch/config/usage surface update the README in the same leg. What is left is
  not a rewrite but the install section's *release* half: each distribution slice
  (M5-06/07/08) documents its own path as it lands, and M5-10 drops the "why not `@v1`"
  explainer once a real tag exists. There is deliberately **no standalone README slice**
  (D173 pt 3).
- Screencast via **vhs** (replaces the old terminalizer GIF pipeline) — M5-09 writes the
  tape and the `make` target; the recording itself needs `vhs`, a real cluster and a real
  terminal, so it is a human task (D79).
- Keybindings reference (generated from the `keys/` bindings where possible) — **already
  met**: `docs/keybindings.md` is generated from the keymap registry by `make keys-doc`
  and `make check` fails on drift (M2-01e/D51). Nothing to build; M5-01 ticks it.
- Migration note: old `~/.kubecom.yaml` auto-migration + any behavior changes — the
  mechanism landed in M2-12a/12b, but M5-PLAN found its report **stale**: it still tells
  the user themes were dropped, which stopped being true at M4-11/12. M5-04 fixes that and
  M5-05 verifies the whole path against the legacy protobuf schema on `master`.
- **Distribution** (**#28**): goreleaser release, Homebrew tap, AUR refresh,
  Docker image (Linux/macOS binaries only). The goreleaser skeleton exists (M0-06/D27) but
  carries **no publishers** and has **no workflow to run it** — `.github/workflows/` holds
  only `ci.yml`. The `Commit`/`Date` ldflags are done (M5-02/D175, drift-guarded) and
  M5-03 added `release.yml`, the workflow that runs them (D176). Each publisher is
  then its own slice because each needs a human-owned external resource (a tap repo, an AUR
  key), except Docker, which authenticates to `ghcr.io` with the workflow's own token.
- **Restore remote `go install`**: tag a real `v1.x.x` release so
  `go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@latest` works
  again (today `@v1` is semver-parsed as a version query, not the branch — see
  FB-go-install / journal 2026-07-20). Renaming `v1`→`main` also dissolves the
  branch-vs-semver collision. Update the README install section back to the remote
  one-liner once tagged.
- Merge/prepare `v1` toward becoming the default branch when ready.

## Exit criteria

Each criterion names the slice that closes it (M5-PLAN/D173). None can be ticked on
hermetic evidence alone: four of the five assert something about an artifact that has left
the repo, which is the D79 line — so they are ticked when the human tasks their slices
raise come back done, not when the config that would produce them compiles.

- [ ] README + keybindings docs current and accurate.
      (Keybindings half is **already met** — generated + drift-gated by `make check`,
      M2-01e/D51. The README half is continuously maintained under D68 but cannot be called
      accurate until the install paths it will describe exist: M5-06/07/08 add them, M5-10
      does the final pass.)
- [ ] `goreleaser release` produces Linux+macOS artifacts from a tag via CI.
      (M5-02 ✅ 2026-07-30: the artifact reports its own commit and build date, guarded by
      `TestGoreleaserSetsAllVersionVars` (D175). M5-03 ✅ 2026-07-30: `release.yml` exists —
      `goreleaser release` on a `v*` tag, gated on `make check` via ci.yml, plus a
      `--snapshot --clean` dry run on every push that keeps the config from drifting (D176).
      The pipeline is complete; it has just never been fired by a tag. Ticked when a real
      tag has produced real artifacts, i.e. after M5-10's human tag push.)
- [ ] Homebrew/AUR/Docker install paths verified.
      (M5-06/07/08, one each. "Verified" means installed from, so each needs its human task
      back — except possibly Docker, which the agent can build and run locally.)
- [ ] Migration verified from a real legacy config file.
      (M5-04 fixes the stale theme report first, M5-05 verifies the path against the legacy
      protobuf schema on `master` and asks the maintainer for a genuinely real file.)
- [ ] Definition of Done in [`../goals.md`](../goals.md) fully checked.
      (Audited by M5-01 (2026-07-30, D174): **6 of 13 ticked**, each against named tests /
      decisions / milestone criteria, and each unticked box now names the one thing that
      closes it. The remaining seven are: two waiting on open dogfood human-tasks (Edit,
      context switch), one on the CRD-01 bug, one on the migration note (M5-04/05), two on
      release acts that have not happened (M5-02/03/10, plus #28 → M5-06/07/08), and one on
      a maintainer decision the audit refused to make for them (M5-01b — the DoD's in-TUI
      YAML viewer vs D135). Also found M5-01a: no surface reached previous-container logs,
      a 2020 parity gap — **closed 2026-07-30** by `logs.previous`/`ctrl+p` (D177), which
      does not tick a box of its own (the logs box waits on M5-01b's YAML question) but
      removes the parity gap the audit found. Ticked when the last box is.)

## Depends on
M0 CI/goreleaser scaffolding; feature milestones M1–M4 complete.
