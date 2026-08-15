# Namespace and context switch keys are inconsistent — make both Shift

- Submitted: 2026-08-15
- Priority: high
- Area: keybindings (walked in S01)

`ctrl+n` switches namespace but `shift+c` switches context — two parallel verbs on
two different modifier conventions. Inconsistent.

I want `shift+n` (not `ctrl+n`) for namespace switch, matching `shift+c` for
context switch: same action family, same modifier. (The S01 trace's 5s pause
before the first `ctrl+n` and the 7 namespace opens suggest the ctrl chord was
never comfortable.)

**Updated during S02 (2026-08-15):** the confusion is confirmed and worse —
I mixed up `shift+n` (today `app.searchPrev`) with `ctrl+n` (namespace switch),
and had to recall that `shift+r` jumps the resource type. Note the collision for
the fold-in: `shift+n` is currently `app.searchPrev` (`n`/`N` = next/prev
search match), so moving namespace switch to `shift+n` displaces it. Decide
searchPrev's new home (or a different namespace gesture) in the same change;
the point stands that the n-family is the single most confusing corner of the
keymap.
