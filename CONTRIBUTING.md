# Contributing to kubecom

kubecom is a rewrite of the 2020 kube-commander, driven against the plan in
[`vault/`](vault/). If you want to work on something, open an issue describing your
intent first, so we can line it up with the milestone plan before you spend time on
it.

## Arguing for a UX change: write a story

**The way to argue for a change in how kubecom feels is to write a story.** Not a
description of the change — a description of the situation that made you want it.

A UX argument written as an opinion ("the logs view should have X") can only be
agreed or disagreed with. The same argument written as a story — *here is what I
was trying to do, here is where I stalled* — can be **walked by somebody else**, and
either it stalls them too or it does not. That is the difference between a
preference and a finding, and it is the only way this project has of telling them
apart, because there is no user research team here and no analytics.

### What a story is

A goal somebody has, written so a stranger can walk it against a known cluster. The
existing ones are in [`stories/`](stories/) — read one before writing yours; they
share a shape:

| section | what goes in it |
|---|---|
| the metadata block | whether it is the main story, which fixture, where to write the trace |
| **The situation** | the circumstances, in the second person. Why is this person at their terminal? |
| **Your goal** | what they are trying to achieve — never how |
| **When you're done** | what to report back |

### The one hard rule: a story never names a key

A story that says "press `L` to open the logs" measures whether you can follow
instructions. A story that says "find out what that pod is printing" measures
whether kubecom is discoverable — which is the thing actually in question. Which
keys the walker reaches for **is the data**, so a story that supplies them has
destroyed its own measurement.

The same rule applies to naming the feature: "use the resource palette to…" is a
key instruction wearing a different hat. Describe the goal and stop.

A test enforces this (`internal/stories`), so a story that names keys fails
`make check` rather than quietly producing a useless walk.

### Walking one

```bash
./stories/cluster/up.sh                  # the fixture — same starting state every time
export KUBECOM_KEYLOG=~/traces/s02.jsonl # record what you actually press
kubecom
```

Then walk it, and answer the questions at the end in your own words. Attach the
trace to the issue or PR along with what you wrote.

The trace is what makes the story evidence rather than an anecdote. The interesting
record in it is a keypress that resolved to **no action** — you reached for
something that does not exist — because that is invisible in every other kind of
report, including a screen recording, where it looks like nothing happening. The
gaps between timestamps are the second signal: they are where you stopped to think.

A trace records every keypress, which means everything you typed — filter queries,
namespace and object names. Read it before you attach it. (Keys pressed inside an
exec shell or your `$EDITOR` never reach it; kubecom is suspended while those run.)

### When a story is not the right tool

Bugs, crashes and wrong output do not need one — open an issue with the steps.
Stories are for the class of problem where nothing is broken and the thing is still
unpleasant to use, which is the class that otherwise gets argued about forever.

## Code

- `make check` (build + test + vet + lint) is the gate; it must be green.
- Keys are never matched raw in view code — every action goes through the keymap
  registry, so all of it stays rebindable. `docs/keybindings.md` is generated from
  that registry and `make check` fails if it drifts.
- Decisions that constrain future work live in
  [`vault/knowledge/decisions.md`](vault/knowledge/decisions.md). If you contradict
  one, supersede it explicitly rather than quietly.
