# Human Tasks (agent → human)

The inverse of [`../feedback/`](../feedback/). This is where the **agent** parks
work that **only a human can do**, and — unlike feedback — a human task can
**block** board items or a whole milestone until it's resolved (D79).

Use it for things the autonomous loop genuinely cannot do itself:

- **Dogfood / visual UX confirmation** against a real cluster in a real terminal
  (the loop runs in a cluster-less sandbox; teatest ≠ a human's eyes).
- **Credentials / infra** the agent must not fabricate: a real kubeconfig, a test
  cluster, envtest control-plane binaries run locally, registry/release secrets.
- **Irreversible or outward-facing actions**: cutting a release tag, changing the
  default branch, publishing packages.
- **A genuine human judgment call** the agent shouldn't decide alone.

If this directory contains only this README, there are no open human tasks.

## File format

One markdown file per task, `YYYY-MM-DD-slug.md` (`README.md` is the only
non-task file). Mirrors the other vault files:

```
# <what the human must do, in one line>

- Created: YYYY-MM-DD
- By: <leg id / agent that raised it>
- Priority: normal | high | blocker
- Blocks: <comma-separated board task ids (e.g. M2-10, M2-11)  |  milestone:M2  |  none (advisory)>
- Status: open            # the human flips this to `done` when finished

## What's needed
<specific, actionable steps a human can follow>

## Why the agent can't do it
<real cluster / credentials / irreversible / human judgment>

## How to resolve
Do the task, then EITHER set `Status: done` and add a `## Result` section (the
agent folds the result in and deletes this file next leg), OR just delete this
file if nothing needs to flow back. Findings/bugs you want acted on go in
`../feedback/`.
```

## How the agent treats these (D79)

At the **Orient** step of every leg the agent lists this directory, then:

1. **An open task with `Blocks:` constrains what the agent may pick.**
   - `Blocks: M2-10, M2-11` → the agent will not start those legs; it picks other
     unblocked work.
   - `Blocks: milestone:M2` → the agent does not advance M2 at all.
   - `Blocks: none` → advisory only (a reminder), blocks nothing.
2. **Blocking never means spinning or busywork.** If open human tasks block *all*
   otherwise-available work, the agent does **not** invent low-value legs — it
   **stops cleanly and reports** which human task(s) it's blocked on (the scheduled
   run ends and notifies). Feedback-inbox items and bug fixes are never blocked by
   default — only what a task's `Blocks:` names.
3. **When a task is `Status: done`**, the agent folds any `## Result` into the right
   place (board/journal/decision/feedback) and **deletes the file** in that leg.
4. **The agent raises tasks here** instead of faking verification it can't do: if a
   leg needs a real cluster, credentials, or a human decision, it writes a task here
   (with a conservative `Blocks:`) rather than claiming a green it didn't earn (D68).
