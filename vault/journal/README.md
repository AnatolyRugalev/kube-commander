# Execution Journal

One file per **leg** (see [`/CLAUDE.md`](../../CLAUDE.md) and the
[`/do-rewrite-leg`](../../.claude/skills/do-rewrite-leg/SKILL.md) skill), named
**`YYYY-MM-DD.N.md`** where `N` is the entry's sequence number within that day
(1, 2, …). Lexicographic filename order == chronological order, so "the last 3
entries" is the 3 highest-sorting files. This is how a human reviews progress
and how a cold-started agent learns what just happened.

There is no `Commit:` field — the entry is written before the commit exists.
The **leg id in the commit message** (e.g. `M0-01`) is the join key between a
journal entry, the board, and git history (D15).

Entry template:

```
## YYYY-MM-DD — <leg-id>: <short title>
- Agent: <model/id>
- Milestone: M<x>
- Did: <1–3 lines on what changed and why>
- Decisions: <Dnn one-liner, or "none">
- Files: <key paths touched>
- Verify: build ✓ · test ✓ · vet ✓ · lint ✓  (note any N/A)
- Next: <suggested next leg id + one line>
```
