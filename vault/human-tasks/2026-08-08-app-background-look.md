# Look at the painted app background — in your terminal, and inside tmux

- Created: 2026-08-08
- By: THEME-03
- Priority: normal
- Blocks: none (advisory — the mechanism is verified end-to-end in a sandbox pty: the
  escape is emitted with the palette's own value, re-emitted on a runtime switch, and
  reset on quit. What a sandbox cannot supply is a *terminal* — whether the result looks
  right, and whether the multiplexer you actually use passes the escape through. Nothing
  on the board waits for this; the light-palette slice should carry it forward rather
  than wait on it.)
- Status: open

## What's needed

`kubecom` now sets your terminal's background to the theme's own while it runs
(`Theme.Background`, D249), instead of drawing text over whatever your terminal already
is. Five things, and the last two are the ones that decide the light-palette slice.

1. **The basic look.** Launch on a terminal whose background is *not* the palette's — set
   your terminal to something obviously different (white, or a different scheme) and run
   `kubecom` with the default theme. The whole screen should become Frappé's `#303446`:
   the two panes, their borders, the gap between them, the short tail of every row. If
   any of it stays your terminal's color, say which part — that is the failure mode this
   design is chosen to avoid, and it would mean something is drawing an explicit
   background it should not.

2. **A terminal that already matches.** Set your terminal background to `#303446` and run
   again. Nothing should change visually. This is the case where the whole feature is
   invisible, and it is worth one look because "invisible" and "broken" are hard to tell
   apart from a screenshot.

3. **The overlays.** Open the help overlay (`?`), the command palette (`:` or `T`), a
   confirm modal and the container picker. The box should sit on the canvas cleanly —
   what to look for is a *seam*: a one-cell halo of a different shade around a box, or
   rows below a short overlay that are a different color from rows beside it.

4. **The switch, live.** Press `T` and pick another palette (`rose-pine` and
   `solarized-dark` are the two biggest jumps). The background should change with
   everything else, in the same frame — not one frame later, and not only after a resize.

5. **Suspend and quit — the ones that would annoy you daily.** With a theme whose
   background is nothing like your terminal's:
   - press `e` on an object to open `$EDITOR`. Your editor must come up on **your**
     terminal's background, not kubecom's. Same for `x` (exec into a container) if you
     have a pod handy.
   - quit with `q`. Your shell must come back on your own background.
   - then kill it the rude way — `kill -9` from another pane — and say what the terminal
     is left looking like. That one is expected to leave the color set (the same way it
     leaves the alt screen); the question is only whether it is bad enough to want a
     belt-and-braces reset somewhere.

6. **tmux, screen, or whatever you actually run it inside.** Repeat step 1 inside your
   multiplexer. This is the one the next slice needs an answer to: a multiplexer that
   filters the escape leaves kubecom rendering exactly as it did before this change,
   which is fine for the ten dark palettes and is **not** fine for a light one — dark
   text on your dark background. Please say which multiplexer, which version, and
   whether the background lands.

## Why the agent can't do it

The pty check that was run proves the bytes: `theme: gruvbox-dark` emits
`ESC ] 11 ; #282828 BEL` on the first frame, `T` → `rose-pine` re-emits `#191724`, and
quitting writes `ESC ] 111 BEL` before leaving the alt screen. A pty is not a terminal —
it records the request and never renders it, so it cannot say whether the result reads
well, whether an overlay leaves a seam, or what a given tmux does with the escape.

## Result

<!-- Fill this in, set Status: done, and the next leg will fold it into the vault and
delete this file. If step 6 says the background does not land in your multiplexer, that
answer is the light-palette slice's starting point. -->
