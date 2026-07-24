# Logs should open in a dedicated logs view with real-time grep

- Submitted: 2026-07-24
- Priority: normal
- Area: logs viewer (M3-05/06) — new feature, likely several legs

Logs currently open in the shared read-only viewer overlay. Instead, give logs a
**dedicated full-screen logs view** ("mini-app") built for streaming, with
**real-time grep/filter**:

- A `/`-style filter that narrows the streamed lines **live while following** —
  type a pattern and only matching lines show, updating as new lines arrive (à la
  `stern` / k9s logs filter). Ideally substring + regex; case-insensitive default.
- Keep the M3-06 follow/pause behavior, container picker (M3-07a), and pod-owning
  resolution (M3-07b) inside this view.
- Full-screen (not a small centered overlay) so long lines and high throughput are
  usable; horizontal scroll or wrap toggle for long lines; clear "[following]/
  [paused]" + active-filter indicator.
- Nice-to-haves (later, don't block): highlight matches, timestamps toggle,
  wrap toggle, jump-to-latest.

This is a bigger item — **triage into board tasks** (e.g. dedicated-view shell →
live filter → regex/highlighting) and do the first slice; record a decision for the
logs-view architecture. Keep it keymap-driven (D11) and message-only (principle 1).
