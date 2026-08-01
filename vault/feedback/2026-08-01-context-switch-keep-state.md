# Keep the previous cluster's state on a context switch, so switching back is instant

- Submitted: 2026-08-01
- Priority: normal
- Area: context switch, watches, discovery

Context switch — can we keep the state of the previous cluster, so we can switch between
contexts instantly?

## Why

Switching away and back currently pays the full cost again: teardown, reconnect,
rediscovery, re-watch. When you are working across two clusters (prod/staging, or a
cluster pair you are comparing) that round trip is the whole interaction, and it is slow
enough to discourage the switch. I want `C` → other cluster → `C` → back to feel like
flipping a tab, not like a restart.

## Grounding, and the tension to resolve

This one **pushes against the current design**, so please record the resolution rather
than quietly changing it:

- **M4-04a deliberately tears the old cluster down** on a switch, and D157 makes switching
  to the context you are already on a no-op precisely so a working cluster is never
  rebuilt. The teardown is also what guarantees the property the dogfood task checks —
  that *nothing from the departed cluster leaks*: overlays close, forwards stop, no stale
  rows flash into the new table.
- Keeping the old cluster warm means keeping its client, its discovery cache and possibly
  its watches alive. That is in direct tension with "nothing leaks", and it costs memory
  and open connections per retained context. A retained watch that keeps firing into a
  dead view is exactly the bug the teardown prevents.
- So the interesting design is probably **warm but quiesced**: retain the connected client
  and the discovery result (the expensive parts), but stop watches and close overlays on
  the way out, then re-arm watches on the way back. Discovery is the dominant cost and is
  also the safest thing to cache, so a discovery-only cache may capture most of the win
  for a fraction of the risk — worth measuring before building the full thing.
- Bound it: retain the last N contexts (or just the previous one), not every context ever
  visited, or a long session accumulates clients indefinitely.
- Per-context namespace/menu memory already exists (D163, `internal/config/state.go`), so
  the *scope* restoration is done — this is about the *connection and discovery* cost, not
  about where you land.

Note the M4 context-switch exit criterion is not yet fully verified by hand (see the open
human task `2026-07-29-context-switch-live-dogfood` — the leak checks are unrun). It would
be worth finishing that dogfood **before** changing the teardown, so there is a known-good
baseline to compare against rather than two moving parts at once.
