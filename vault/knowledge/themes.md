# Built-in themes: the candidate schemes and their licences

Survey done at **THEME-01** (2026-08-06) for feedback `2026-08-06-more-themes`,
which asked for ~10 built-ins including Catppuccin and said to **check licensing
per theme rather than assuming**. This file is the answer, so a later slice ports
a palette without re-running the research.

## What we are actually copying

A theme in kubecom is fourteen hex values in `internal/tui/styles/themes.go`
(`Theme`, D169 pt 3) — no code, no config format, no artwork. The upstream
projects licence far more than that, and the licences below all permit
redistribution with attribution, so the honest thing is to **attribute in the
constructor's doc comment** (project, licence, copyright line) and move on. No
upstream file is vendored, so there is no `LICENSE` to carry in-tree; the
attribution comment is the notice.

Two caveats worth keeping straight:

- **A name is API** (D169 pt 1). `catppuccin-mocha`, not `mocha` — the family
  prefix is also what makes the flavors filter as one group in the theme picker.
- **Do not claim to *be* the upstream theme.** kubecom maps a palette onto its
  own fourteen semantic roles; a port is kubecom's reading of the scheme, and the
  doc comment says which upstream role each kubecom role took.

## Licence findings (verified 2026-08-06, not assumed)

| Scheme | Source of truth | Licence | Copyright line |
|---|---|---|---|
| **Catppuccin** | `github.com/catppuccin/catppuccin` `LICENSE`; values from `catppuccin/palette` `palette.json` | MIT | © 2021 Catppuccin |
| **Dracula** | `github.com/dracula/dracula-theme` `LICENSE` | MIT | © 2023 Dracula Theme |
| **Nord** | `github.com/nordtheme/nord` `license` (branch `develop` — `main` 404s) | MIT | © 2016-present Sven Greb |
| **Rosé Pine** | `github.com/rose-pine/rose-pine-theme` `LICENSE` | MIT | © 2023 Rosé Pine |
| **Solarized** | `github.com/altercation/solarized` `LICENSE` | MIT | © 2011 Ethan Schoonover |
| **Tokyo Night** | `github.com/folke/tokyonight.nvim` `LICENSE` | **Apache-2.0** | folke |
| **Gruvbox** | upstream `github.com/morhetz/gruvbox` has **no licence file**; the author's community fork `gruvbox-community/gruvbox` `LICENSE.md` is MIT | MIT (fork only) | © 2018 Pavel Pertsev (= morhetz) |

Notes on the two that are not plain MIT:

- **Tokyo Night is Apache-2.0.** Permissive, but a *different* licence with a
  notice requirement of its own — attribute it as Apache-2.0, not MIT, and do not
  fold it into an "all MIT" sentence.
- **Gruvbox upstream ships no licence at all.** The palette values are facts
  (a list of hex numbers is not a creative work), and the same author's community
  fork states MIT, so porting is fine — but the attribution should point at the
  fork, and nobody should later "correct" it to morhetz/gruvbox as the licence
  source, because there isn't one there.

## The ten, and why these

Popularity checked 2026-08-06 rather than taken from the feedback's own list:
Catppuccin and Tokyo Night lead on ports/mindshare, Gruvbox is the perennial
daily-driver recommendation, Nord and Dracula have the widest cross-app port
libraries, Rosé Pine is the newer entrant that keeps appearing.

Shipped after THEME-01 (6): `default`, `catppuccin-frappe`,
`catppuccin-macchiato`, `catppuccin-mocha`, `monokai`, `solarized-dark`.
`default` **is** the Frappé palette under its own name (D236 pt 2).

**Shipped after THEME-02 (11)**: the five queued ones landed —
`dracula`, `gruvbox-dark`, `nord`, `rose-pine`, `tokyo-night` — each transcribed
from the upstream data file named under "Where the values came from" below, each
attributed in its constructor's doc comment, and each held to the registry by
`TestPortedPalettesCarryTheirAttribution`. The feedback's "~10" is met.

Two of the five are named without a variant suffix, which is deliberate and is
D248 pt 2: `rose-pine` is Rosé Pine's `main` and `tokyo-night` is Tokyo Night's
`night`, because in both schemes the bare name already means that variant.
`gruvbox-dark` keeps its suffix because the dark/light split has no default
(same reason as `solarized-dark`, D169 pt 1). Adding `rose-pine-moon` later is a
*new* name beside `rose-pine`, never a rename of it.

## The app background: how it is painted, and what that leaves for a light theme

Before THEME-03 kubecom painted a background on exactly three things — the
selected row, the status bar and a search match — and everything else was
foreground text over whatever the terminal already was. `Theme.Background`
(THEME-03, **D249**) is the canvas, and it is painted **once, by the root
`View`**, as `tea.View.BackgroundColor` — the terminal's own default background
for as long as kubecom holds the screen.

**Do not try to paint it with lipgloss styles instead.** That is the obvious
design and it does not work, which is worth knowing before someone re-derives it:
lipgloss does not re-open an outer background after a nested style's reset. Wrap
an already-composed frame in `NewStyle().Background(c).Width(w).Height(h)` and
what comes back is painted at the margins and bare everywhere a colored span
already ran — the sequence is `48;2;…m` … `[m` … and the rest of the line is the
terminal's default again. Measured on lipgloss v2.0.0 at THEME-03; the border
glyphs and the pane interior both come back unpainted. Per-component painting
would be the same failure spread over eleven components plus the gaps between
them, which nothing owns.

What `tea.View.BackgroundColor` buys, in exchange for being a terminal-level
setting rather than in-band text:

- Every cell, including the ones no component draws: the gap between the panes,
  a pane's border, the unfilled tail of a short line, the erased rows below a
  short overlay.
- Bubble Tea's own lifecycle. `cursedRenderer` emits the OSC on the first frame,
  re-emits it when the color changes (so a runtime theme switch repaints), and
  writes `ResetBackgroundColor` from `close()` — which runs on quit **and** on
  `ReleaseTerminal`, i.e. every `tea.ExecProcess` suspend. `$EDITOR` and `exec`
  therefore get the user's own terminal back, not kubecom's.
- Nothing for a terminal that filters the escape (tmux without passthrough is the
  case to expect). There the app renders exactly as it did before THEME-03.

That last point **is** the remaining light-theme question. A light palette's dark
text (Latte's `#4c4f69`, Solarized Light's `base00`) is readable only if the
background request lands; where it is filtered, a light theme is dark-on-dark and
looks like a kubecom bug. So the light slice's real work is not the values — it is
deciding what kubecom does when it cannot paint: refuse the palette, warn, or
detect. Until then `TestBuiltinThemesAreDarkAndLegible` measures every built-in as
dark, and D248 pt 1 says that test is retuned deliberately in that slice, never
deleted.

## Where the values came from

`catppuccin/palette`'s `palette.json` is the machine-readable source and is what
THEME-01 transcribed. Prefer it (or the equivalent upstream data file) over a
screenshot, a blog post or another project's port: ports disagree with each other
about the accents constantly, and a wrong value is invisible until someone who
knows the scheme looks at it.

The one transcription worth double-checking on any future port is the **status
bar background**: kubecom uses the flavor's *mantle* (one step darker than
`base`), which reads as a bar against the terminal. Using `base` makes the bar
vanish on a terminal already set to the scheme's background.

THEME-02's five ports go the *other* way from Catppuccin — the first background
shade **above** base (`nord1`, gruvbox `dark1`, rose-pine `surface`, tokyo-night
`bg_highlight`, dracula "Current Line") — which is also what `monokai` and
`solarized-dark` already did. Either direction is fine; the constraint is only
"not the base" (D248 pt 3). Nord is the useful precedent to cite, because
upstream documents `nord1` as exactly this: "a lighter background color for UI
elements like status bars".

Dracula is the one scheme whose published palette has a **single** shade above
the background, so `Selection` and `StatusBarBg` share it. That is not an
oversight — solarized-dark already does the same — and `TestBuiltinThemesRenderDistinctly`
still passes because distinctness is between *themes*, not between roles.

### What "dark" means now

D236 pt 3's admission criterion stopped being prose at THEME-02:
`TestBuiltinThemesAreDarkAndLegible` requires every built-in's `Selection` and
`StatusBarBg` to sit at relative luminance ≤ 0.2 and the text painted on each to
contrast ≥ 4.5:1. The registry's measured spread, so a new port knows where it
would land: selected-row contrast runs 4.86 (solarized-dark) to 8.69
(catppuccin-mocha), status-bar contrast 4.86 to 12.5 (rose-pine). A port that
comes out under 4.5 has almost always mapped the *wrong shade* to a background
role, not found a genuinely low-contrast scheme.
