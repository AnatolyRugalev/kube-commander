# `/` in the resources pane should filter the resources, not the table

- Submitted: 2026-08-15
- Priority: high
- Area: resource menu / left pane (walked during S02)

Pressing `/` while the **resources pane** (the left panel listing resource kinds)
has focus launches the filter in the **main table** instead of filtering the
resource list in that pane. The menu component today handles only navigation and
drill-in (`internal/tui/components/menu/menu.go`), and `app.filter` targets the
table wherever the focus is.

I want `/` to filter whatever pane is focused: with the resources pane focused, it
should narrow the resource kinds in that pane (in the same picker-mode spirit as
`2026-08-15-picker-navigation-mode.md`); with the table focused it keeps filtering
the table.
