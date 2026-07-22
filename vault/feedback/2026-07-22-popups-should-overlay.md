# Namespace & help popups hide the base TUI — overlay them instead

- Submitted: 2026-07-22
- Priority: high
- Area: modals / help / namespace picker

The namespace picker and the help overlay currently **replace** the browse body:
in `Model.View()` the open overlay takes the whole body area, so the menu + table
disappear behind it. They should render as **floating popups composited on top of
the base browse view** — the two-pane browse stays visible (optionally dimmed)
underneath, with the modal centered over it.

Implementation note: `internal/tui/app.go` `View()` switches `body = help.View()` /
`nsPicker.View()` instead of drawing the base and layering the modal. Compose the
base browse string first, then place the modal box over it (lipgloss can overlay a
box onto a background, e.g. via a Place/overlay/canvas helper) so the popup floats.
Applies to every modal (help, namespace picker, and the M2-10 confirm modal once
it's wired). Keep it within the fixed layout — no scrolling/resizing the panes.
