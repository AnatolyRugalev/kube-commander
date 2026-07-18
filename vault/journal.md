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
