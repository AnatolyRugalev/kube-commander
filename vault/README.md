# kubecom Rewrite Vault

This directory is the **single source of truth and working memory** for the
kube-commander → **kubecom** rewrite. The rewrite is driven by Claude Code
agents running in a cloud environment, so everything an agent needs — goals,
milestones, tasks, accumulated knowledge, and process — lives here in the repo,
under version control, and is discoverable without external context.

> **If you are an agent picking up this project: read this file, then
> [`goals.md`](goals.md), then the current milestone in [`milestones/`](milestones/),
> then the open items in [`tasks/board.md`](tasks/board.md). Record anything you
> learn in [`knowledge/`](knowledge/).**

## Branch model

- **`master`** — the original 2020 codebase. Left untouched for now.
- **`v1`** — the rewrite branch. **All rewrite work happens here.** This vault
  lives on `v1`.
- Final destination for the finished rewrite is a `main` branch (to be created
  when the rewrite is ready to become the default). Until then, `v1` is the
  active line of development.

## Layout

| Path | Purpose |
|------|---------|
| [`../CLAUDE.md`](../CLAUDE.md) | Agent operating guide: autonomous model, the leg loop, hard rules |
| [`goals.md`](goals.md) | Top-level goal, definition of done, non-goals, principles |
| [`milestones/`](milestones/) | Large milestone definitions (M0–M5) with exit criteria |
| [`tasks/`](tasks/) | Task board + workflow conventions |
| [`feedback/`](feedback/) | Human → agent inbox; checked before every leg, preempts the board, deleted once addressed (D69) |
| [`human-tasks/`](human-tasks/) | Agent → human tasks (only-a-human work); an open task can **block** board items or a milestone (D79) |
| [`knowledge/`](knowledge/) | Durable knowledge: decisions, target stack, legacy findings |
| [`journal/`](journal/) | Execution journal: one file per leg, `YYYY-MM-DD.N.md` |
| [`REWRITE_PLAN.md`](REWRITE_PLAN.md) | The strategic plan narrative (architecture, phases, risks) |

The rewrite runs **autonomously**: agents self-assign work and progress one small
**leg** at a time via the [`/do-rewrite-leg`](../.claude/skills/do-rewrite-leg/SKILL.md)
skill, pushing directly to `v1`. A claim on the board is committed and pushed
**before** implementation so it acts as a lock (D16). A human reviews
periodically via this vault and the journal. See [`../CLAUDE.md`](../CLAUDE.md) for the full operating model.

## How agents use the vault

1. **Orient** — read `goals.md` and the active milestone before starting work.
2. **Check `feedback/` and `human-tasks/`** — an open `human-tasks/` file's
   `Blocks:` gates what you may pick (a milestone or specific board items); if it
   blocks everything, stop and report rather than inventing busywork (D79). Then
   drain `feedback/`: any file but its `README.md` is unaddressed feedback that
   preempts the board; address it and delete the file (D69).
3. **Pick work** — take the next item from `tasks/board.md` (or the milestone's
   checklist). Move it to In Progress with your agent id and a timestamp.
4. **Do the work** on `v1`, in small, reviewable commits.
5. **Capture knowledge** — anything non-obvious you discover (about the old code,
   the K8s API, a library quirk, a decision) goes into `knowledge/` immediately,
   so the next agent (or a cold-started you) doesn't re-derive it.
6. **Update state** — check off milestone items, move tasks to Done, note blockers.
7. **Leave a trail** — commit messages and the task board should let a fresh agent
   reconstruct where things stand.

## Conventions

- Markdown only; keep files small and single-purpose so they're cheap to load.
- Cross-link with relative links. Prefer linking over duplicating.
- Status vocabulary (used everywhere): `todo` · `in-progress` · `blocked` · `done`.
- Dates are absolute (`YYYY-MM-DD`), never "yesterday"/"last week".
- Decisions are append-only in [`knowledge/decisions.md`](knowledge/decisions.md);
  supersede rather than delete.
