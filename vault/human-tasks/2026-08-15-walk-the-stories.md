# Walk the five stories against a fresh fixture and hand back the traces + reactions

- Created: 2026-08-15
- By: STORY-03 (the analyzer completed the instrument)
- Priority: high
- Blocks: STORY-06
- Status: done

## What's needed

Everything in the instrument now exists: the fixture (`stories/cluster/up.sh`,
STORY-01), the keystroke recorder (`--keylog` / `KUBECOM_KEYLOG`, STORY-02), the
stories themselves (`stories/s01…s05`, STORY-04) and the analyzer
(`kubecom keys analyze`, STORY-03). The remaining act is yours, because judging a
UX needs a person with intent, and you are the only one who can run the binary
against a real terminal.

For each of the five stories in `stories/` (S02 is the main story):

```bash
./stories/cluster/up.sh
./stories/cluster/run.sh s02    # one run per story id; trace -> ~/traces/s02.jsonl
```

`run.sh` starts kubecom with a throwaway user dir, so every walk begins with no
config, menus/state, cache or log history (up.sh's identical-start guarantee,
extended to kubecom) — see `stories/README.md`.

Walk it, and answer the "When you're done" questions in your own words. Then read
the trace back with `kubecom keys analyze ~/traces/s02.jsonl`. The two things
worth knowing about a trace: a keypress that resolved to **no action** is a reach
for something kubecom does not have (the dead ends), and the gaps between
timestamps are where you stopped to think.

Hand back: the five trace files, the five sets of answers, and any freeform
reactions. Bugs / wrong output go in `vault/feedback/`; nothing needs to flow back
here unless you want it.

## Why the agent can't do it

D268 pt 1: the whole line exists because no user path has ever been walked
deliberately, start to finish, with an intent held in mind. The sandbox has no
cluster and no terminal a human would trust; a machine replaying its own stories
measures the machine. Only a human can produce the traces the fold-in (STORY-06)
is supposed to read.

## How to resolve

Walk the stories as above. When done, set `Status: done` and add a `## Result`
listing the trace paths and a one-line verdict per story — or just delete this
file if you want nothing acted on. Findings that should change the UX go in
`vault/feedback/`, which the agent folds in as STORY-06.

## Result

Walked 2026-08-15 by the maintainer; traces at `~/traces/s01…s05.jsonl` (385,
251, 100, 69, 79 presses). The fold-in's raw material is 23 feedback files in
`vault/feedback/`; the verdicts:

- **S01 — first contact** (s01.jsonl): got oriented, but every major friction
  showed up in one walk — namespace switching was the fiddliest action (7 opens,
  5s pause before the first), the picker's `j`/`k` were dead ends it fell back
  to arrows for, `V` was reached for log selection, `s` was hammered 26× in a
  burst, and the left panel was too dense to scan.
- **S02 — something is broken** (s02.jsonl, the main story): found four of the
  five causes (bad-image URL, missing DB URL in crashloop, bad node selector,
  readiness probe) and answered "is `shop` affected — no"; **missed the fifth
  (the unbindable PVC)** — a pod-first lens blinds non-pod failures; diagnosed
  via logs + describe, with events only reachable through describe.
- **S03 — find it in the noise** (s03.jsonl): found "error gateway timeout 2–3×
  a minute, warn slow upstream every 10 s"; live stream helped, but live-vs-
  frozen was indistinguishable at a glance.
- **S04 — change something** (s04.jsonl): a clean bill — never unsure an action
  happened, confirmations told enough, the nvim hand-edit round-trip was clean,
  nothing was scary to press.
- **S05 — half-remember the name** (s05.jsonl): went straight to cluster search
  (the fastest move), the middle-fragment search worked well, no list-eyeballing;
  friction was reaching a result (double-enter) and trusting the pick (wanted a
  preview).
