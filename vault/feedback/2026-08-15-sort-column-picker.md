# Sort: `s` cycles direction only, `shift+S` picks the column

- Submitted: 2026-08-15
- Priority: normal
- Area: table sorting (walked in S01)

Current `s` ("sort table, cycle column / direction") made me press it **26 times
in one 7-second burst** (13:55:27–34 in the S01 trace) — cycling through states I
couldn't get to behave.

I want:

- `s` to cycle the sort **direction only** on the current column,
- `shift+S` to open a popup that picks the **column and direction**.

Collision to resolve: `S` is currently `sort.clear`. The clear-sort verb should
live inside the new shift-S popup (a "clear sort" entry), so nothing is lost.
