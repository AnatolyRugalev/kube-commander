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

**Shipped after THEME-04b (13)**: `catppuccin-latte` and `solarized-light`, the
registry's first light palettes. `solarized-light` is the name D169 pt 1 reserved
when `solarized-dark` was named for its variant, and it landed beside it exactly as
planned — no rename.

**Shipped after THEME-06 (14)**: `gruvbox-light`, the values slice the line above
said was all that was missing. It is the dark port's role mapping mirrored, and the
values are the fork's own light-mode reading of the scheme (see the constructor's
doc comment): bg0=light0, fg1=dark1, chrome on light2/light1, and — the thing a
mirror wouldn't tell you — the **`faded_*`** accent set (`#076678` blue,
`#9d0006` red, …), because in `colors/gruvbox.vim` the dark branch assigns
`bright_*` accents and the light branch assigns `faded_*`. A future leg must not
"fix" the light accents to `bright_*`; that would read as pale text on a light
canvas. It measures body 10.22, selected row 6.76, status bar 8.45.

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

That last point was the remaining light-theme question, and **THEME-04a answered it:
kubecom asks** (D250). `tea.RequestBackgroundColor` sends OSC 11 as a *query* and the
terminal's reply comes back as `tea.BackgroundColorMsg`, so what the screen actually
is stops being a guess. The comparison against `Theme.Background` has three silences
and one line — D250 pt 3 says why each silence is deliberate — and the line is
`<theme> is dark but the terminal stayed light — background not applied`, with the
remedies (tmux passthrough, a matching palette) in the log rather than in the toast.

Two things worth knowing before touching that code:

- **The probe must be late.** The renderer emits the background *set* on the first
  paint and the query is a Cmd, so an immediate probe reads back the terminal's
  *previous* background and reports a false negative on a terminal that honoured the
  request. Measured under a pty at THEME-04a: with the 750 ms delay the bytes leave
  in the right order — `ESC ] 11 ; #282828 BEL` at offset 342, `ESC ] 11 ; ? BEL` at
  362.
- **A pty answers nothing**, which exercises the "no reply" silence and is why
  `script -qec` alone cannot check this end to end. A harness that *does* answer
  proves both halves: replying `rgb:fbfb/f1f1/c7c7` (gruvbox-light's bg0) under
  `theme: gruvbox-dark` puts the line on screen and a WARN with both colours in
  `~/.cache/kubecom/kubecom.log`; replying `rgb:2828/2828/2828` says nothing. Fork a
  pty, watch for `\x1b]11;?`, write the OSC 11 reply back into the master fd.

**THEME-04b then landed the values** (`catppuccin-latte`, `solarized-light`),
taking the registry to thirteen — eleven dark, two light — and retuned the guard as
D248 pt 1 required. See "What 'dark' means now" (renamed below) for what replaced
it. THEME-06 then landed `gruvbox-light` (`#fbf1c7` canvas), taking it to fourteen
— eleven dark, three light — with no change to the guard.

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

### The admission criterion: it is coherence, not darkness

D236 pt 3's criterion stopped being prose at THEME-02 ("every background is dark,
text on it contrasts ≥ 4.5:1") and stopped being *darkness* at THEME-04b, when the
first light palettes joined. The test kept its job and changed its rule
(`TestBuiltinThemeChromeAndTextAreOppositePolarities`, **D251 pt 1**):

1. `Selection` and `StatusBarBg` sit on the **same** side of luminance 0.2 as
   `Background` — chrome that agrees with the canvas.
2. `Foreground`, `SelectionFg` and `StatusBarFg` sit on the **other** side.
3. Every text/background pair still clears **4.5:1** — unchanged, and the floor a
   port may not lower.

The single threshold is `darkLuminance` in `luminance.go`, shared with the runtime
polarity check (D250), so a palette cannot pass admission and then be described to
the user as the other polarity.

The registry's measured spread at fourteen, so a new port knows where it lands
(`gruvbox-light` sits at 10.22 / 6.76 / 8.45 — the mid-range, not a new edge):

| | lowest | highest |
|---|---|---|
| body text on the canvas | 4.75 (solarized-dark) | 13.94 (monokai) |
| the selected row | 4.86 (solarized-dark) | 8.69 (catppuccin-mocha) |
| the status bar | 4.86 (solarized-dark) | 12.50 (rose-pine) |

A port that comes out under 4.5 has almost always mapped the *wrong shade* to a
background role rather than found a genuinely low-contrast scheme — **except at
Solarized's light end**, which is the one case where the scheme really is that
low: see below.

### The two light palettes, and the one deliberate departure

`catppuccin-latte` needed nothing special. The Catppuccin family maps through one
function (`catppuccinTheme`) whose roles name *rungs* of a flavor's ladder, and
Catppuccin builds Latte on the same rungs, so the palette inverts with no change to
the mapping at all. It measures body 7.06, selected row 5.17, status bar 6.57.

`solarized-light` needed one. Solarized is designed as a single palette read from
either end (light swaps base03↔base3, base02↔base2, base01↔base1, base00↔base0,
accents unchanged), but its canonical light body pair — **base00 on base3 —
measures 4.13:1**, below the 4.5 floor. Solarized is low-contrast by design and its
light end is the lower of the two. The port therefore takes the *next rung of
Solarized's own ladder* for each text role: body is **base01** (the scheme's
"optional emphasized content" for a light background, 4.99:1) and chrome text is
**base02** (10.61:1). The whole ladder shifts, which preserves the dark port's own
relationship — chrome text one step more emphasized than body — rather than a
single value nudged to clear a threshold. **D251 pt 2** states the rule this is an
instance of: a port may move to another rung of the upstream ladder, and may not
invent a value or lower the floor.

### What the guard does not measure, and what that costs a light palette

Only `Foreground`/`SelectionFg`/`StatusBarFg` are held to 4.5. `Header`, `Subtle`
and `Primary` are not, and on a light canvas they land lower than their dark
siblings — Latte's rosewater header is 2.34:1 and Solarized Light's cyan header
2.93:1, against 5.37–12.95 across the dark ports. That is upstream's own palette
rather than a mapping error (Latte's rosewater is a pale peach; it is what Latte
*is*), and it is inside the range the dark registry already tolerates for these
roles — nord's subtle is 1.69:1. Nobody should "fix" it by substituting a value
Catppuccin did not publish. If it reads badly in practice the honest fix is a
different *role* mapping for the family, applied to all four flavors.

## The `Match` highlight is mapped for a dark canvas (measured 2026-08-08, LOGS-SEL-03)

`styles.New` builds `Match` as **`StatusBarBg` on `Warn`** — a dark chrome shade on
the palette's yellow. That reads as "dark text on a highlighter pen" only while
`StatusBarBg` is dark, which was true of every built-in until THEME-04b. Measured
across the registry (contrast ratios; the bar is `Selection`, the canvas is
`Background`):

| theme | matched text on its highlight | highlight vs the cursor bar | highlight vs the canvas |
|---|---|---|---|
| catppuccin-mocha | 13.81 | 9.89 | 12.91 |
| catppuccin-macchiato | 11.16 | 7.76 | 10.20 |
| rose-pine | 10.06 | 6.38 | 10.77 |
| default / catppuccin-frappe | 8.55 | 5.85 | 7.62 |
| dracula | 8.19 | 8.19 | 12.74 |
| monokai | 7.69 | 6.47 | 10.44 |
| gruvbox-dark | 6.84 | 5.20 | 8.69 |
| tokyo-night | 6.72 | 4.47 | 8.55 |
| nord | 6.44 | 5.52 | 8.00 |
| solarized-dark | 4.05 | 4.05 | 4.68 |
| **solarized-light** | **2.62** | **2.62** | **2.98** |
| **catppuccin-latte** | **2.15** | **1.70** | **2.31** |

The middle column is the one LOGS-SEL-03 asked about and it is comfortable on every
dark palette: a highlight inside the cursor bar stays a distinguishable second thing,
which is why D252 pt 1 keeps both. The first column is the defect the measurement
found — on the two light palettes the matched span is near-white text on mid yellow,
and on Latte the highlight is also within 1.70:1 of the bar, so under the cursor it
nearly disappears into it. `solarized-dark` at 4.05 is the same low-contrast-by-design
scheme that forced D251 pt 2, and is the registry's edge case rather than its norm.

**No shade in either light palette fixes it in place.** Measured against that palette's
own `Warn`, the best available candidates are Latte `Foreground` **3.05** and
Solarized Light `StatusBarFg` **4.05** — both under the 4.5 the body pair clears, because
these schemes' yellows sit mid-luminance and a light palette has nothing dark enough
above them. So **THEME-05** has to change the mapping itself (a derived shade, or marking
a match by weight rather than paint where paint cannot carry it), not pick a different
role — and per D252 pt 3 it may not lower the floor instead. Whatever it does lands in
one place and fixes three surfaces: the logs grep, the table filter (FILT-02) and
cluster-search hits (SEARCH-06) all render through `styles.Match`.
