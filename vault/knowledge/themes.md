# Built-in themes: the candidate schemes and their licences

Survey done at **THEME-01** (2026-08-06) for feedback `2026-08-06-more-themes`,
which asked for ~10 built-ins including Catppuccin and said to **check licensing
per theme rather than assuming**. This file is the answer, so a later slice ports
a palette without re-running the research.

## What we are actually copying

A theme in kubecom is thirteen hex values in `internal/tui/styles/themes.go`
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
  own thirteen semantic roles; a port is kubecom's reading of the scheme, and the
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

Queued for THEME-02 (→ 11): `dracula`, `gruvbox-dark`, `nord`, `rose-pine`,
`tokyo-night`.

## The light-theme wall (why Latte and solarized-light are not here)

**kubecom never paints an app background.** `styles.New` sets a background on
exactly three things — the selected row, the status bar, and a search match —
and everything else renders as foreground text over whatever the terminal
already is. Every built-in so far is dark because that is the only thing that
works: a light palette's dark text (Latte's `#4c4f69`, Solarized Light's
`base00`) drawn on a dark terminal is unreadable, and it would look like a
kubecom rendering bug rather than a mismatched terminal.

Shipping a light theme therefore means adding a `Background` role to `Theme` and
having the panes actually paint it — which by D169 pt 3 must be filled in *every*
built-in in the same leg, and which changes how every component renders. That is
its own slice (THEME-03), not a footnote on a palette port.

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
