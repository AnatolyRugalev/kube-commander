# Theme picker: preview the palette live as the selection moves

- Submitted: 2026-08-09
- Priority: normal
- Area: themes / theme picker (D249-era)

From the maintainer, verbatim: "theme switching should happen as I change
selection in the pallette, so I can test the look without pressing enter".

Today `T` opens the palette list and the theme only applies on `enter`, so
comparing themes means open → enter → look → reopen, once per theme. Wanted:
moving the selection in the picker re-themes the whole UI immediately
(including the painted background), `enter` commits, and `esc` closes the
picker restoring the theme that was active when it opened. This is the natural
pair of THEME-03's painted background — the preview should repaint the canvas
too, not just the panes.
