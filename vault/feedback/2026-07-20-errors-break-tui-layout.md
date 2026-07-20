# Errors break the TUI layout (scroll the viewport down)

- Submitted: 2026-07-20
- Priority: high
- Area: TUI layout / error surfacing

When an error occurs, it disrupts the TUI layout — the viewport gets scrolled
down and the panes no longer line up. An error should be surfaced **within** the
fixed layout (e.g. a status-bar message, a transient toast, or a bordered modal)
without pushing content around or resizing/scrolling the panes.

Likely cause to check: something is writing the error to stdout/stderr while
Bubble Tea owns the terminal, or an error string with newlines is being rendered
into a pane without being clipped/framed to its box. See the no-stdout-while-TUI
logging rule in `vault/knowledge/stack.md` (principle: degrade, don't crash — and
don't wreck the layout either).
