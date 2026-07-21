# Left menu needs two distinct item states: nav cursor vs. opened/active

- Submitted: 2026-07-21
- Priority: normal
- Area: resource menu (rendering / state)

The menu currently conflates two things that should look different:

1. **Navigation cursor** — the item the selection cursor is currently on as you
   move up/down in the menu.
2. **Opened / active** — the item whose resource is actually loaded and shown in
   the right pane (the "focused/active" resource).

These need to be **two separate visual states**. When focus is in the right pane
and you've opened e.g. Pods, the menu should still show *Pods* as the active/opened
item, while the navigation cursor can independently sit elsewhere in the menu. Two
styles (e.g. active = highlighted/marked, cursor = a distinct highlight or caret)
so it's always clear both which item is open and where the cursor is.
