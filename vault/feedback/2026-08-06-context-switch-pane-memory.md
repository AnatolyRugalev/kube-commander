# Context switch should remember each context's pane, not just reset it

- Submitted: 2026-08-06
- Priority: normal
- Area: context switching (see CTX-WARM, D196)

Live context switching (`C`) is fast, but every switch resets the pane — I
lose which resource type I had open, where I'd drilled in, scroll position,
etc. I want switching contexts to feel like window/tab switching in an OS:
flip to context B, look at its `pods` pane, flip back to context A, and be
looking at what I was looking at before — not starting over each time.

It's fine for this to be bounded rather than unconditional: evict/clean up a
context's retained watchers, caches and pane state after some idle time, or
once more than N contexts are held in memory. I don't need infinite history,
just "the last few contexts I was actually using" to behave like tabs.

## Note for whoever picks this up

This is a bigger ask than CTX-WARM-02/03 on the board — those retain only the
*connector's* client and discovery result (for reconnect speed), capped at a
single previous-context entry, and D196 pt 1/2 explicitly keeps the **shell's
teardown unconditional** — no view/pane state survives a switch today, by
design. Satisfying this feedback means retaining (or reconstructing) UI-level
state per context and bounding it by count/idle-time rather than a 1-entry
cap, which pushes on that boundary. Treat this as a case for a new decision
that amends or supersedes the relevant part of D196 (or draws the shell/
connector line differently), not a silent extension of CTX-WARM-02/03 — record
whichever way you resolve it.
