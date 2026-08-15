# S02 — Something is broken

- Main story: yes
- Fixture: `stories/cluster` — run `up.sh` first
- Trace: `~/traces/s02.jsonl`

## The situation

You are on call. An alert says workloads in the `broken` namespace are not coming
up. Nobody has told you which ones or why, and the person who deployed them is
asleep.

There are five separate things wrong in there, and they have failed in five
different ways. Each one's explanation lives somewhere different — one is in the
container's own output, one has produced no output at all, one never got as far as
a machine, one is running and still refusing work, and one is not a workload.

## Your goal

Work out what is wrong with each of the five, well enough that you could write the
incident note. You do not have to fix anything.

Then answer the question you would actually be asked next: **is the `shop`
namespace affected?**

## When you're done

Tell me:

1. The five causes, in whatever words you'd use in the note.
2. Which one took the longest, and what you were doing during that time.
3. Any point where you knew what you wanted to see but not how to get to it.
4. Whether you ever had to leave kubecom to answer something.

## Why this one is the main story

It is the job. Everything else in kubecom serves the moment somebody is looking at
a red thing and does not yet know why — so this is the path the screencast and the
usage guide are cut against (D268 pt 1).
