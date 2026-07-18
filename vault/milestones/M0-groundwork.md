# M0 — Groundwork

**Status:** `todo`
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
- Test harness: `envtest` (kube-apiserver) for `kube`; teatest for TUI models.
- Keep old `commander/`, `app/`, `pb/` compiling in parallel until ported; delete per-tree as M1–M3 land.

## Exit criteria

- [ ] `go build ./...` and `go test ./...` pass on a bare skeleton.
- [ ] CI is green on `v1` for build/test/vet/lint.
- [ ] `kubecom version` runs; second binary removed.
- [ ] envtest and teatest each have one passing smoke test.
- [ ] Decision log + vault referenced from the repo README.

## Notes / open questions

- Confirm golangci-lint ruleset (start lenient, tighten later).
- Decide logging library (slog stdlib is the default choice).
