# Selection bar fill dies at the last glyph under plx — plx's bug, fixed there; no kubecom change requested

- Submitted: 2026-08-21
- Priority: normal
- Area: rendering (table/menu selection background)

Dogfooding kubecom inside plexos (plx), the selected row's background — the
resource table's selection bar and the sidebar's — stopped at the last glyph
instead of filling to the pane edge. Bare ghostty and plx(herdr(kubecom)) both
rendered it correctly.

Diagnosis, so it's on record here: kubecom's own output is correct — lipgloss
pads the row with styled literal spaces. But Bubble Tea v2's renderer
(ultraviolet) compresses the trailing blank run into `SGR bg` + `EL`/`ECH`,
relying on the `bce` terminfo capability (`canClearWith()` in
`terminal_renderer.go` says so explicitly). That reliance is legitimate — the
advertised `xterm-256color` declares `bce` — and plx's terminal layer was
dropping the erased cells' background when re-encoding the grid. Fixed in
plexos on 2026-08-21 (its journal 2026-08-21.1, finding F74).

What I want from kubecom: **nothing to change.** This is awareness, in case a
similar report arrives from tmux/screen users — their terminfos deliberately
do not declare `bce`, but ultraviolet keys its erase optimization off terminal
detection, so if such a report ever comes in, the first suspect is the hosting
multiplexer's BCE handling, not kubecom's row painting. Delete this file once
read; no board task needed unless such a report actually exists.
