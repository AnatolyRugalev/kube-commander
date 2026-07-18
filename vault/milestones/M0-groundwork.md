# M0 — Groundwork

**Status:** `in-progress` (started 2026-07-18)
**Phase:** REWRITE_PLAN Phase 0

## Goal

A clean, modern module skeleton that builds, tests, and lints in CI, with the
old code isolated for staged removal and a single `kubecom` binary.

## Scope

- New module layout under `internal/` (`kube/`, `tui/`, `config/`, `version/`)
  and `cmd/kubecom/`.
- Go 1.23+, latest cobra; drop `ioutil`, klog-as-primary-logging.
- Single `kubecom` binary; retire the duplicate `kube-commander` entrypoint.
- **Linux + macOS only** build matrix; remove Windows sources; README notes WSL2.
- GitHub Actions: build, `go test`, `go vet`, `golangci-lint`. Drop Travis.
- goreleaser config for Linux+macOS artifacts (wire fully in M5).
- Test harness: teatest smoke test for TUI models. envtest is **opt-in and moved
  to M1** (D18) — fake clients are the default test strategy.
- **Delete the legacy trees up-front** (`app/`, `cli/`, `commander/`, `config/`,
  `pb/`, `cmd/kube-commander/`, Windows sources, Travis/snap CI) and prune
  `go.mod` (D14). `master` is the permanent reference; the old "keep compiling
  in parallel" plan is superseded — one module cannot hold k8s.io v0.18 and
  client-go v0.31 at once.
- `Makefile` with `check` = build + test + vet + lint as the canonical gate (D17).

## Exit criteria

- [x] `go build ./...` and `go test ./...` pass on a bare skeleton. *(M0-01)*
- [x] Legacy trees deleted; `go.mod` pruned; lint excludes dropped (D14). *(M0-07, D22)*
- [x] CI is green on `v1` for `make check` (build/test/vet/lint). *(M0-04, D24 — `.github/workflows/ci.yml`; first live run triggers on this push)*
- [x] `kubecom version` runs *(✓ M0-01)*; second binary removed *(✓ M0-07)*.
- [ ] teatest has one passing smoke test (envtest moved to M1, D18).
- [ ] Decision log + vault referenced from the repo README.

## Notes / open questions

- Confirm golangci-lint ruleset (start lenient, tighten later).
- Decide logging library (slog stdlib is the default choice).
