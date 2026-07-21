# Esc should return focus to the left pane when the right pane is focused

- Submitted: 2026-07-21
- Priority: normal
- Area: focus / navigation

When the right pane (the resource table) is focused, pressing **Esc** should move
focus back to the **left menu pane**. Right now there's no easy "back to the menu"
gesture. Esc is the natural one-level-back key: table focused → Esc → menu focused.
(If a filter/search is active in the table, Esc should still clear that first, then
the next Esc pops focus back to the menu — one level per press.)
