# `C` (ctx.switch) is unreachable while an overlay or the logs view is open

- Submitted: 2026-08-09
- Priority: normal
- Area: keymap / context switch

From the maintainer, verbatim: "Impossible - C doesn't work when popup or logs
are open."

Found while attempting the pt-3 leak check of the context-switch dogfood
(open a log stream / port-forward panel, *then* press `C`): the picker's
binding never fires because the logs view / overlay captures keys first. Two
things to settle:

1. **Decide whether global actions should be reachable from anywhere.** If
   `ctx.switch` is meant to be global, this is a keymap bug — `C` should work
   over the logs view and overlays (or the palette, once it opens from there,
   should offer it). If it is deliberately browse-view-only, say so in the
   keybindings doc and close this as by-design.
2. Either way, the pt-3 teardown claim (overlays close, forwards stop on
   switch) was never exercisable as scripted — worth one hermetic look at
   whether `resetCluster` really is unreachable-but-unnecessary for open
   overlays, or whether the test gap is real.
