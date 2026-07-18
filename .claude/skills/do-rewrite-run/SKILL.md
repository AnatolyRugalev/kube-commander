---
name: do-rewrite-run
description: Orchestrate a bounded batch of rewrite legs in one run — sequentially spawn a fresh subagent per leg, each executing exactly one /do-rewrite-leg, until the leg or time budget is hit. Keeps the orchestrator context small; meant for scheduled routines that get a ~2-hour usage window.
---

# Do a bounded run of rewrite legs

You are the **orchestrator** for one scheduled run of the kubecom rewrite. You
do **no leg work yourself** — no reading the vault, no editing, no git. You only
spawn subagents, read their reports, and decide whether to continue. This keeps
your context small; each leg gets a fresh subagent context instead (D21).

## Budgets (stop conditions)

- **Time:** note the wall-clock start time now. Do **not** start a new leg once
  **90 minutes** have elapsed — the run lives in the last ~2 hours of the usage
  window, and the final leg needs room to finish and push.
- **Legs:** at most **4** legs per run.
- **Failure:** stop immediately if a subagent reports a blocker, a red tree, a
  failed push, or that it could not claim work. Do not retry a failed leg —
  report it for the human instead.

## Loop

1. Spawn **one** subagent (`general-purpose`, **synchronously** — never in
   parallel: legs claim tasks and push to `v1`, and concurrent legs would
   collide) with this prompt:

   > Invoke the `/do-rewrite-leg` skill and follow it exactly: one leg, then
   > stop. End your final message with a report: leg id · what was done (1–2
   > lines) · verify result (`make check`) · pushed to v1 (y/n + commit subject)
   > · suggested next leg · any blocker.

2. When it returns, check the report against the budgets above.
3. If all budgets hold and the leg succeeded, go to 1 (fresh subagent).
4. Otherwise stop.

## Final report

End the run with: legs completed (ids + one line each), total count, why the
loop stopped (budget/failure), and the suggested next leg for the next run.
