# Allow terminal text selection (select-to-copy) — don't capture the mouse by default

- Submitted: 2026-07-22
- Priority: normal
- Area: mouse / clipboard / input

I want to select text in the terminal to copy it (names, values, log lines).
"Select to copy" is a reasonable default. Right now I can't, because the app
**captures the mouse**: `Model.View()` sets `v.MouseMode =
tea.MouseModeCellMotion` (`internal/tui/app.go` ~L1149/L1176) to get click + wheel,
and any mouse-reporting mode makes the terminal send events to the app instead of
doing its own click-drag selection.

This is a genuine tradeoff — please pick a sensible default and **record a
decision**:

- **Disable mouse capture by default** → native select-to-copy works everywhere,
  but we lose in-app mouse click-to-select and mouse-wheel scroll (keyboard nav is
  unaffected). Mouse could be an opt-in (config flag) or a runtime toggle keybind.
- **Keep mouse capture** and rely on the terminal's Shift+drag bypass for
  selection — then just document it (weaker; not a true default).
- **In-app copy** (e.g. yank the selected row/field to the clipboard via OSC 52) as
  a complement.

My preference leans toward native select-to-copy as the default; losing mouse-wheel
scroll is acceptable if keyboard scrolling is solid. Your call on the exact default
+ whether mouse becomes opt-in — record it.
