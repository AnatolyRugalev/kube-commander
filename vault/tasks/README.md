# Task Tracking

Lightweight, in-repo task tracking so any agent (cloud or local, cold or warm)
can see what's in flight and pick up work without external context.

## Where tasks live

- **[`board.md`](board.md)** — the live board: Backlog / In Progress / Blocked / Done.
- Milestone files in [`../milestones/`](../milestones/) hold the coarse checklists;
  the board holds the fine-grained, actively-worked items.

## Workflow

1. **Pick** the top unblocked item from Backlog (respect milestone order).
2. **Claim** it: move to In Progress, add `@agent-id` and `YYYY-MM-DD`.
3. **Work** on `v1` in small commits; reference the task id in commit messages.
4. **Capture** any durable learning in [`../knowledge/`](../knowledge/).
5. **Close**: move to Done with the date and the commit/PR ref. If blocked, move
   to Blocked with the reason and what would unblock it.

## Task id format

`M<milestone>-<seq>` — e.g. `M1-03`. Keep ids stable once assigned.

## Item template

```
- [ ] **M1-03** Async discovery: background full discovery + reconcile signal
      status: todo | owner: — | added: 2026-07-18
      notes: seed set first-paints; emits DiscoveryReady to menu
      → milestone: M1 · knowledge: knowledge/stack.md
```

Status vocabulary: `todo` · `in-progress` · `blocked` · `done`.
