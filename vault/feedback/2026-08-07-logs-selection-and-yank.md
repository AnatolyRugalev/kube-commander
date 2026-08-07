# Logs view: line selection and a visual mode, so log lines can be copied

Priority: normal
Kind: feature

The logs viewer scrolls but has no cursor. There is no way to say "this line" or
"these twelve lines", and therefore no way to copy them.

Today the only route is `M` to drop mouse capture and use the terminal's own
select-to-copy. That works, but it costs you the mouse for as long as you are
doing it, it cannot reach past what is on screen, and on a wrapped line it copies
the visual rows rather than the log line — so a long entry comes back with breaks
in it that were never in the log.

## What I want

**A line cursor.** `j`/`k` move a highlighted line, the same way they move a row
in a table. That alone makes the viewer feel like the rest of kubecom, and it is
the thing selection has to be built on.

**A visual mode.** `v` starts a selection at the cursor, `j`/`k` extend it, `Esc`
cancels. Vim semantics, because that is what the rest of the app assumes.

**Yank.** `y` copies the selection — or the cursor's line when there is no
selection — to the system clipboard, through the same path `secret.copy` already
uses. A brief confirmation in the status bar, as the secret copy gives.

Note `y` is currently bound to `confirm.accept`. That is modal and cannot be live
at the same time as the logs viewer, so I do not think it is a real conflict —
but it is worth being deliberate about rather than discovering later.

## The details that decide whether it feels right

These are where this feature will be won or lost, and they are all cases the
current viewer already has state for:

- **Follow.** Entering visual mode while `f` is on should pause following.
  A selection that slides upward as new lines arrive is unusable. Whether
  leaving visual mode resumes it is a judgement call — I lean yes, since the
  user did not turn it off themselves.
- **Wrapping.** With `w` on, a long line occupies several rows. The cursor and
  the selection are over **log lines**, not screen rows, and a yank returns the
  line as it arrived — unwrapped, no inserted breaks.
- **Filtering.** With a `/` query active, selection covers what is displayed.
  Yank returns the matching lines only, not the lines hidden between them.
- **Timestamps.** `t` toggles them. Copy should match what is on screen — if you
  can see timestamps you get timestamps, if you turned them off you do not.
  Anything else means the copy does not match what you were looking at.
- **No styling in the clipboard.** Whatever highlighting the viewer applies —
  filter matches, the selection itself — must not reach the clipboard. Worth an
  explicit test; it is the kind of thing that looks right on screen and only
  shows up once the text is pasted somewhere.

## Not settled

- Whether the selection highlight and the `/` filter-match highlight can coexist
  legibly, or whether one has to yield while visual mode is active.
- Whether there should be a "yank everything visible" shortcut, or whether
  `gg` `v` `G` `y` is enough.
- What happens at the top of the buffer when lines are still streaming in above.

## Notes

`secret.copy` (M3-08b) already solved the clipboard half, including the OSC-52
path — so this is a cursor, a range, and a join, not a new mechanism.

Sensible to split: a leg that adds the line cursor and proves `j`/`k` against the
existing viewport, then a leg for visual mode and yank on top of it. The cursor
alone is worth having even if visual mode slips.
