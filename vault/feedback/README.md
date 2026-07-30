# Feedback Inbox

A human → agent inbox. Drop a markdown file here to steer the rewrite — a bug you
hit dogfooding, a UX complaint, a priority change, a course correction, a question.
The agent **checks this directory before every leg** and drains it **before**
picking normal board work (D69).

This is a **to-do list, not an archive**: an addressed feedback file is **deleted**
in the same leg that addresses it. The permanent record of what happened lives in
the journal (`vault/journal/`), which the addressing leg links back to. If this
directory contains only this README, the inbox is empty.

**Deleted is not gone** (D178 pt 2). Git keeps every item, and it is the only place your
*exact words* survive — a decision written from your feedback can read like the agent's own
idea once the file is gone. So before any leg concludes "this needs the maintainer", it
should check whether you already said:

```bash
git log --diff-filter=D --all -- 'vault/feedback/*'   # what was addressed, and when
git show <commit> -- vault/feedback/<file>.md         # your words, verbatim
```

## How to submit (human)

Add a markdown file named **`YYYY-MM-DD-short-slug.md`** (any name but `README.md`;
the date prefix keeps them sortable/oldest-first). Commit and push it to `v1` — or
just drop it and let the next leg pick it up. Format mirrors the journal: a title
plus a few optional fields, then free-form prose. Nothing is mandatory except that
it says what you want.

```
# <short title of the feedback>

- Submitted: YYYY-MM-DD
- Priority: normal | high | blocker      (optional; default normal)
- Area: <e.g. namespace picker, logs view, install>   (optional)

<What you observed and what you want changed, in plain words. Reproduction steps,
the cluster/context if relevant, and the outcome you'd prefer all help. It's fine
to ask a question or set a direction rather than file a precise task.>
```

## How the agent processes it (per D69)

At the **Orient** step of every leg, before picking a board item:

1. **List this directory.** Any file other than `README.md` is unaddressed feedback.
2. **If there is feedback, it preempts the board.** Take the oldest (or highest
   `Priority`) item and make addressing it this leg's work.
3. **Address it** — one of:
   - Small enough for one leg → implement it now.
   - Larger than a leg → convert it into concrete board task(s), do the first
     slice, and record the triage.
   - A question / direction / decision → decide, and record it in the journal (and
     `decisions.md` as a new `Dn` if it's load-bearing).
4. **Delete the feedback file** in the same leg's commit, and write a journal entry
   that says what the feedback was and how it was addressed (implemented / triaged
   onto tasks X, Y / answered). Deletion is what keeps the inbox a live to-do list —
   never leave an already-handled item here to be re-read next leg.

Keep the honesty rules: if feedback asks for a non-goal (see
[`../goals.md`](../goals.md)) or contradicts a binding decision, don't silently
comply — record the tension, propose the smallest aligned change, and note it for
the human in the journal.
