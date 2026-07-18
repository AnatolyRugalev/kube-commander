# Execution Journal

Append-only log of the kubecom rewrite. One entry per **leg** (see
[`/CLAUDE.md`](../CLAUDE.md) and the [`/do-rewrite-leg`](../.claude/skills/do-rewrite-leg/SKILL.md)
skill). Newest at the bottom. This is how a human reviews progress and how a
cold-started agent learns what just happened.

Entry template:

```
## YYYY-MM-DD — <leg-id>: <short title>
- Agent: <model/id>
- Milestone: M<x>
- Did: <what changed and why>
- Decisions: <Dnn one-liner, or "none">
- Files: <key paths touched>
- Verify: build ✓ · test ✓ · vet ✓ · lint ✓
- Commit: <short hash>
- Next: <suggested next leg>
```

---

## 2026-07-18 — BOOT: project bootstrap (pre-leg setup)
- Agent: claude (Opus 4.8), interactive setup with the maintainer
- Milestone: pre-M0
- Did: Inspected the 2020 codebase; produced the rewrite plan; created the vault
  (goals, milestones M0–M5, task board, knowledge base); locked decisions D1–D11;
  added `CLAUDE.md` and the `/do-rewrite-leg` skill for autonomous operation.
- Decisions: D1 Bubble Tea · D2 in-process client-go · D3 YAML config ·
  D4 name kubecom · D5 client-go v0.31 · D6 clean break + migration ·
  D7 Linux/macOS only (WSL2) · D8 async cached discovery · D9 branch model ·
  D10 vim-first nav · D11 zero hard-coded keys.
- Files: `CLAUDE.md`, `.claude/skills/do-rewrite-leg/SKILL.md`, `vault/**`.
- Verify: n/a (docs only; no code yet).
- Commit: (this bootstrap; see `v1` history)
- Next: **M0-01** — scaffold the new module layout
  (`cmd/kubecom`, `internal/{kube,tui,config,version}`) with a compiling skeleton
  and one passing test, so `go build ./...` / `go test ./...` are green.

## 2026-07-18 — M0-01: new module skeleton + lint scoped to new code
- Agent: claude (Opus 4.8)
- Milestone: M0
- Did: Scaffolded the rewrite layout — `internal/{version,kube,tui,config}` and a
  new `cmd/kubecom` skeleton with a stdlib `version`/`help` dispatch (repointed
  from the legacy `cli.Run()`; the old app stays reachable via
  `cmd/kube-commander`). Added `.golangci.yml` that lints only the new code and
  excludes the legacy 2020 trees. `internal/version` + `cmd/kubecom` have passing
  tests; `kubecom version` prints build info.
- Decisions: D12 golangci-lint scoped to new code (relative-path-mode gomod +
  anchored legacy excludes, standard ruleset) · D13 kubecom = new skeleton, legacy
  via kube-commander, cobra deferred to M0-02.
- Files: `cmd/kubecom/main.go`, `cmd/kubecom/main_test.go`,
  `internal/version/{version.go,version_test.go}`,
  `internal/{kube,tui,config}/doc.go`, `.golangci.yml`, vault board/journal/decisions.
- Verify: build ✓ · test ✓ · vet ✓ · lint ✓ (0 issues) · gofmt ✓ · `kubecom version` runs ✓
- Commit: (this leg on v1)
- Next: **M0-02** — bump go.mod to Go 1.23+, add latest cobra and wire
  `cmd/kubecom` onto it, drop `ioutil`.
