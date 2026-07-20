---
name: do-rewrite-leg
description: Execute exactly one small "leg" of the kubecom rewrite autonomously — orient from the vault, pick and claim the next task, implement it, keep the tree green, record decisions/knowledge, update the journal and board, then commit and push to v1. Use when driving the rewrite forward with no human supervision (directly, via /loop, or a scheduled agent).
---

# Do one rewrite leg

You are operating **autonomously** on the kubecom rewrite. There is no human to
ask. Make all decisions yourself, record them, and leave the project in a clean,
reviewable state. Do **exactly one leg**, then stop.

Read [`/CLAUDE.md`](../../../CLAUDE.md) and [`vault/README.md`](../../../vault/README.md)
if not already in context. Follow these steps in order.

## 1. Orient

- `git rev-parse --abbrev-ref HEAD` → must be `v1`. If not, `git checkout v1`.
- `git pull --rebase origin v1` to get others' work. Resolve any trivial rebase
  conflicts; if non-trivial, that becomes your leg (fix the conflict, nothing else).
- Ensure a clean tree (`git status`). If dirty from an interrupted leg, assess:
  finish or revert it — never build on an unknown dirty state.
- Read: `vault/goals.md`, the **active milestone** in `vault/milestones/`, the top
  of `vault/tasks/board.md`, and the **3 newest entry files** in `vault/journal/`
  (named `YYYY-MM-DD.N.md`; filename sort == chronological order).
  Skim `vault/knowledge/decisions.md` for anything relevant.
- **Check the feedback inbox**: list `vault/feedback/`. Any file other than
  `README.md` is unaddressed human feedback and **preempts the board** (see Pick).

## 2. Pick a leg

- **Feedback first (D69).** If `vault/feedback/` holds anything but its `README.md`,
  the oldest (or highest-`Priority`) item **is this leg**. Address it — implement it
  if it fits one leg; if larger, convert it into concrete board task(s) and do the
  first slice; if it's a question/direction, decide and record it. **Delete the
  feedback file in this leg's commit** and link it from the journal entry. Only with
  an empty inbox do you pick from the board.
- Otherwise take the **top unblocked** Backlog item for the active milestone (respect
  milestone order M0→M5).
- **Size it.** A leg must be completable now and leave the tree green. If the item
  is too big, **split it**: write the smaller slices back into the Backlog and take
  the first slice.
- If the active milestone's board section is thin or vague, **expanding it into
  concrete small tasks is itself a valid leg** — do that and stop.
- A leg is one logical change, diff roughly ≤ ~300 lines.

## 3. Claim it (and push the claim)

- Move the chosen item to **In Progress** in `board.md` with `owner: <your id>` and
  today's date (`YYYY-MM-DD`).
- **Commit and push the claim immediately** — a board-only commit
  (`chore(board): claim <leg-id>`), then `git pull --rebase` + `git push origin v1`.
  The pushed claim is the lock that stops a concurrent agent taking the same leg
  (D16). If the rebase reveals someone else claimed it first, pick the next item.

## 4. Implement

- Do the work. Keep it minimal and focused on the leg.
- **Decide autonomously.** When a choice is ambiguous, pick a sensible, reversible
  default aligned with `vault/goals.md` and existing decisions, and note it (step 6).
- Prefer landing a compiling, tested stub over a large half-wired change.
- Match the style/decisions already established; don't reintroduce patterns the
  decisions log rejected (e.g. mutex-guarded UI state, raw-key matching in views).

## 5. Verify (green or revert)

- Run `make check` (= `go build ./...`, `go test ./...`, `go vet ./...`,
  `golangci-lint run`) — the canonical gate (D17). The tree **must** be green.
- If you cannot get green within this leg, **reduce the leg's scope** until you can,
  or revert and pick a smaller leg. Never push red.
- For UI/behavior changes with a runtime surface, sanity-check the behavior, not
  just compilation.
- Once `kubecom` launches (M2-RUN onward): keep it **launchable** and don't
  regress the running binary — the human dogfoods it against a real cluster
  between reviews, so each leg should improve that experience (D68). If a leg
  changes install/launch/config/usage, update `README.md` in the same leg.

## 6. Record decisions & knowledge

- Append any non-obvious, load-bearing decision to `vault/knowledge/decisions.md`
  as the next `Dn` (date it; supersede rather than contradict). A `Dn` is a
  **constraint a future leg must not silently contradict** — not a description of
  how this leg was built. Implementation detail goes in the journal; the
  decisions log is skimmed every leg, so keep it load-bearing (D67).
- Add durable learnings (API quirks, legacy behavior, gotchas) to the relevant
  `vault/knowledge/` file so the next agent doesn't re-derive them.

## 7. Journal, board & milestone

- Move the task to **Done** in `board.md` (with the date). If part remains,
  split the remainder back into Backlog as new small items.
- Write one new journal file `vault/journal/YYYY-MM-DD.N.md` (`N` = next unused
  sequence number for today) using the template below.
- Update the active **milestone file**: tick exit criteria now met; keep its
  `Status:` line current (`todo`/`in-progress`/`done`) (D15). The `Status:` line
  and the board's `Last updated:` line are **one sentence each** — the journal is
  the changelog, not these fields (D67).

## 8. Commit & push

- Stage everything for this leg. Commit with a clear, conventional message
  referencing the leg id (e.g. `feat(kube): M1-05 table watch → event channel`).
- End the message with:
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`
- `git pull --rebase origin v1` then `git push origin v1`. If push is rejected,
  rebase and retry. **Never force-push.**

## 9. Report & stop

Print a short summary: what the leg did, verification result, commit hash, and the
**suggested next leg**. Then stop — one leg per invocation.

---

## Journal entry template

Write to `vault/journal/YYYY-MM-DD.N.md`. No `Commit:` field — the leg id in the
commit message is the join key (D15):

```
# YYYY-MM-DD — <leg-id>: <short title>

- Agent: <model/id>
- Milestone: M<x>
- Did: <1–3 lines on what changed and why>
- Decisions: <Dnn one-liner, or "none">
- Files: <key paths touched>
- Verify: build ✓ · test ✓ · vet ✓ · lint ✓  (note any N/A)
- Next: <suggested next leg id + one line>
```

## Guardrails (do not violate)

- One leg, then stop. Do not chain legs in a single invocation.
- Green or revert; small diffs; push to `v1` every leg.
- Never modify `master` (unless the task explicitly says so); never force-push;
  never rewrite shared history.
- Never block waiting for human input — decide, record, proceed.
- Every leg updates both the **journal** and the **board**.
