# Left menu pane is too wide — smaller default, truncate long names

- Submitted: 2026-07-22
- Priority: normal
- Area: browse layout / resource menu

The left menu pane's default width is too large, eating room the table needs.
It's currently `menuPaneWidth = total/4` (floored at 20) in
`internal/tui/app.go` (M2-07b). Make the default **narrower** — size it closer to
the menu's actual content with a smaller cap, and it's fine to **truncate long
item names** (ellipsis) rather than widening the pane to fit them. Keep a sane
minimum so short terminals still render. A config option for the width is welcome
but not required.

(Related direction: see the feedback about making the left panel optional /
toggleable — a narrower default is a good step toward that.)
