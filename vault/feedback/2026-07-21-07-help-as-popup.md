# Help/keys dialog should be a popup overlay, not a full-screen replacement

- Submitted: 2026-07-21
- Area: help overlay

The `?` help/keys view currently **replaces the whole TUI**. It should instead be
a **popup / overlay** rendered on top of the current browse view (centered modal,
dims or overlays the background, dismiss with Esc/`?`/`q`), so you keep the context
of what you were looking at. We already have a modal picker component (M2-08a) —
reuse that overlay approach for help.
