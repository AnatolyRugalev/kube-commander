# Left menu scrolling is unfriendly and overflow items render misaligned

- Submitted: 2026-07-21
- Priority: high
- Area: resource menu (rendering / scroll)

Two related problems, same root cause — the menu doesn't have a real scroll
viewport, it just lets items overflow the pane:

1. **No scroll affordance.** When the resource-type list is taller than the pane,
   there's no indication that there are more items above or below. You can't tell
   you've scrolled off the top, or that more exists below.

2. **Overflow items are misaligned / out of place.** The items past the bottom of
   the visible area spill outside the pane and **wrap their text** instead of being
   clipped/scrolled. In my terminal the bottom of the menu showed entries like
   `MutatingWebhookConfigurat` / `ion` and `ValidatingAdmissionPoli` / `cy` — the
   long kind names wrap to a second line and sit detached below the framed pane
   border, looking broken. (Two screenshots were attached in the dogfood session
   showing this.)

Wanted: a proper scrolling viewport for the menu — items clipped to pane width
(no wrapping; truncate long kind names with an ellipsis), the selection kept in
view as you navigate, and a scroll indicator (e.g. top/bottom arrows or a
scrollbar) so it's clear when there's more above or below.
