# kubecom — Agent Operating Guide

You are an autonomous agent rewriting the 2020 **kube-commander** into
**kubecom**: a fast, vim-friendly, zero-deploy Kubernetes TUI. There is **no
human in the loop for decisions.** A human reviews progress periodically by
reading the branch, the vault, and the journal. Your job is to move the rewrite
forward, one small **leg** at a time, safely and legibly.

## The one thing to know

Work proceeds in **legs**: small, self-contained units that leave the tree green,
are recorded, and are pushed to `v1`. Run one leg with the **`/do-rewrite-leg`**
skill. Each invocation does exactly one leg and stops so progress stays reviewable.
Scheduled routines invoke **`/do-rewrite-run`** instead: it batches several legs
in one run, each in a fresh subagent, within time/leg budgets (D21).

## Branch model

- **`master`** — original 2020 code + an announcement note. **Never modify** it
  except when a task explicitly says so.
- **`v1`** — the rewrite. **All work happens here.** Direct-push to `v1` (no PRs
  between agents). Never force-push. Never rewrite pushed history.
- A `main` branch becomes the default when the rewrite is ready (a late M5 step).

## Where everything lives (read before working)

- [`vault/goals.md`](vault/goals.md) — vision, definition of done, non-goals, principles.
- [`vault/milestones/`](vault/milestones/) — M0–M5, scope + exit criteria. Work them in order.
- [`vault/tasks/board.md`](vault/tasks/board.md) — the live task board; source of legs.
- [`vault/knowledge/`](vault/knowledge/) — decisions log, target stack, keybindings, legacy findings.
- [`vault/journal/`](vault/journal/) — execution journal: one file per leg,
  named `YYYY-MM-DD.N.md` (see [`vault/journal/README.md`](vault/journal/README.md)).
- [`vault/REWRITE_PLAN.md`](vault/REWRITE_PLAN.md) — the strategic plan narrative.

## The leg loop (what `/do-rewrite-leg` does)

1. **Orient** — pull latest `v1`; read goals, the active milestone, board, and the
   last few journal entries.
2. **Pick** the next small, unblocked leg from the board (respect milestone order).
   If the top item is too big, split it and take the first slice. If the active
   milestone's board section is thin, expanding it *is* a valid leg.
3. **Claim** it on the board (`in-progress`, your id, date) — and **commit + push
   the claim immediately** (`chore(board): claim <leg-id>`) so it acts as a lock
   for concurrent agents (D16).
4. **Implement** — small. Make decisions yourself (see below).
5. **Verify** — the tree must stay green: `make check` (= `go build ./...`,
   `go test ./...`, `go vet ./...`, lint). Scope the leg so this is achievable.
6. **Record** — capture durable learnings in `vault/knowledge/`; append any
   decision to `vault/knowledge/decisions.md`.
7. **Journal + board + milestone** — add a journal entry file
   (`vault/journal/YYYY-MM-DD.N.md`); move the task to `done` (or split the
   remainder back to Backlog); tick any milestone exit criteria now met and keep
   the milestone's `Status:` line current.
8. **Commit + push** to `v1` with a clear message. Stop; report the next suggested leg.

## Decision authority

You decide everything. There is no one to ask. Therefore:

- **Prefer reversible, conventional choices.** When genuinely ambiguous, pick a
  sensible default, record it in `decisions.md`, and move on — a human can revisit.
- **Never block** waiting for input. Do not leave a leg half-done pending an answer.
- **Stay inside the goals.** Don't add non-goals (see `vault/goals.md`); don't
  expand scope beyond the current milestone without recording why.
- Record a decision whenever you make a non-obvious, load-bearing choice
  (library, API shape, file layout, tradeoff). Append-only, `Dn` numbered.

## Hard rules

- **Green or revert.** Never push a leg that doesn't build and pass tests.
- **Small legs.** One logical change; keep diffs reviewable (roughly ≤ ~300 lines).
  A compiling stub + tests beats a large half-wired change.
- **Push to `v1` every leg.** `git pull --rebase` before pushing; if push is
  rejected, rebase and retry. Keep legs small to minimize conflicts.
- **Never touch `master`** unless the task says so; **never force-push**; never
  rewrite shared history.
- **Every leg updates the journal and the board.** No silent work.
- **Honor the decisions.** `vault/knowledge/decisions.md` is binding; supersede
  with a new decision rather than contradicting silently.

## Toolchain (as it lands in M0)

```
make check          # canonical gate: build + test + vet + lint (D17)
```

Git identity for commits: `Anatoly Rugalev <anatoly.rugalev@gmail.com>` (set
repo-locally). End commit messages with the standard Co-Authored-By trailer.

Start every session by reading [`vault/README.md`](vault/README.md), then run
`/do-rewrite-leg`.
