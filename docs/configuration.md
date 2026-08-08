# Configuring kubecom

kubecom runs on sensible defaults — none of this is required. Everything here is
opt-in, and everything kubecom writes for you lives in its own state file, never
in a file you hand-edit.

- [The config file](#the-config-file) — `keys:`, `theme:`
- [Themes](#themes) — the built-in palettes
- [Per-context menu](#per-context-menu) — adding CRDs to the resource menu
- [Pinned kinds](#pinned-kinds) — what `*` writes, and where
- [What kubecom remembers](#what-kubecom-remembers) — the namespace and the kind you left open
- [Migrating from the 2020 kube-commander](#migrating-from-the-2020-kube-commander)

## The config file

kubecom reads an optional YAML config from
`os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on
Linux). The `keys:` section rebinds any action; see
[`keybindings.md`](keybindings.md) for the action list and defaults, and inspect
the effective map any time with `kubecom keys`.

```yaml
# ~/.config/kubecom/config.yaml
theme: monokai
keys:
  nav.down: ["j", "down"]
  nav.up:   ["k", "up"]
```

## Themes

`theme:` picks the palette kubecom renders with. Eleven are built in:

| Name | |
|------|--|
| `default` | dark-friendly, blue accent (used when `theme:` is absent) — this is Catppuccin Frappé |
| `catppuccin-frappe` | Catppuccin's mid-dark flavor — the same palette as `default` |
| `catppuccin-macchiato` | Catppuccin, darker and cooler than Frappé |
| `catppuccin-mocha` | Catppuccin's darkest flavor |
| `dracula` | Dracula: high-contrast, purple accent and a pink header |
| `gruvbox-dark` | gruvbox's dark variant at medium contrast — warm, retro, low-glare |
| `monokai` | the classic warm dark palette, cyan accent |
| `nord` | Nord's arctic blue-greys, muted throughout |
| `rose-pine` | Rosé Pine's `main` variant: muted plum with a rose header |
| `solarized-dark` | Solarized's dark variant |
| `tokyo-night` | Tokyo Night's `night` style — the darkest of its three |

A theme now paints the **whole screen**, not just the text on it: while kubecom is
running it sets your terminal's background to the palette's own, and restores it on
exit — including while an editor or a shell is suspended in front of it. So the
panes, their borders and the space between them all sit on the palette rather than
on whatever your terminal happened to be. If your terminal ignores the request
(some multiplexers filter it), kubecom looks exactly as it did before: text on your
own background.

**kubecom checks, and tells you when it matters.** Shortly after launch — and again
after you switch theme with `T` — it asks the terminal what its background actually
is. If the answer is the palette's own, the request landed and nothing is said. If
it is a different colour of the same polarity (another dark background under a dark
palette), nothing is said either: that is how kubecom has always rendered. Only when
the two are *opposite* does it show one line, because that is when the text is about
to be hard to read:

```
gruvbox-dark is dark but the terminal stayed light — background not applied
```

Nothing is disabled and the theme still applies — it is a heads-up, not a refusal.
`~/.cache/kubecom/kubecom.log` gets the same event with both colours and what to do
about it: in tmux, `set-option -g allow-passthrough on` lets the request through;
otherwise pick a palette that matches your terminal, or set your terminal's
background to the palette's. A terminal that answers no such question at all is left
alone — silence is not evidence.

All built-ins are still **dark** palettes. A light one (Catppuccin Latte, Solarized
Light) is coming next, into a kubecom that now reports the mismatch above rather
than silently rendering dark text on your dark background.

Ported palettes keep their upstream names and attribution: Catppuccin
(MIT, © 2021 Catppuccin), Dracula (MIT, © 2023 Dracula Theme), gruvbox
(MIT, © 2018 Pavel Pertsev — from the author's community fork, which is where
the licence lives), Monokai, Nord (MIT, © 2016-present Sven Greb), Rosé Pine
(MIT, © 2023 Rosé Pine), Solarized (MIT, © 2011 Ethan Schoonover) and Tokyo
Night (**Apache-2.0**, folke). Each palette is kubecom's reading of the scheme
onto its own fourteen roles, not a claim to be the upstream theme.

The name is matched ignoring case and surrounding space, but it is never guessed
at: an unknown name launches on the default theme and shows a brief startup notice
listing the ones that exist.

`T` switches theme from inside kubecom (it opens the command palette on its
`:theme ` line). The pick repaints immediately, and the name is written back to
`config.yaml` so the next launch opens on it. The write-back keeps the rest of the
file's settings, but it rewrites the file — **YAML comments and hand-crafted
formatting are lost** — so if you keep comments in your config, set `theme:` by
hand instead.

## Per-context menu

kubecom can add extra resource types (chiefly CRDs the built-in menu doesn't know)
to the browse menu, per kubeconfig context. Each context reads its own file from
`os.UserConfigDir()/kubecom/menus/<context>.yaml` (`~/.config/kubecom/menus/` on
Linux; the context name is sanitized into a safe filename). The file is optional —
a context with no file uses the default menu; a malformed file falls back to the
default menu and shows a brief startup notice rather than failing to launch.

```yaml
# ~/.config/kubecom/menus/my-cluster.yaml
resources:
  - group: cert-manager.io   # omit for the core ("") group
    version: v1
    resource: certificates   # plural, as the API addresses it
    kind: Certificate        # optional; defaults from resource
    namespaced: true         # optional; default false (cluster-scoped)
    section: Custom Resources # optional; default the Custom Resources group
```

An entry whose resource is already in the menu (a seed row or one discovery finds)
is merged, never listed twice.

## Pinned kinds

Pressing `*` on a kind (or `:pin ` in the command palette) keeps that kind in this
context's menu whether or not discovery lists it. Pins are recorded *for* you, so
they live in the kubecom-managed state file
(`~/.config/kubecom/state/<context>.yaml`) rather than in the menu file you
hand-write — pressing `*` never rewrites `menus/<context>.yaml`.

Where both name the same resource, your hand-written entry wins and keeps its title
and section; `*` on such a row says so and changes nothing, since that entry is
yours to edit. Only a kind you pinned with `*` can be unpinned with it — seed rows
and discovered rows are not removable this way. A pinned kind is otherwise an
ordinary menu row, listed under **Custom Resources** unless the menu already places
it elsewhere.

## What kubecom remembers

kubecom remembers where you were, per kubeconfig context, and reopens there next
time: the **namespace** you last selected and the **resource kind** you last had
open. Both are stored in
`os.UserConfigDir()/kubecom/state/<context>.yaml` (`~/.config/kubecom/state/` on
Linux) — a kubecom-managed file, separate from your config and menu files, so
kubecom rewrites it freely without touching anything you hand-edit (it also holds
the kinds you pin, above). Passing `-n`/`--namespace` overrides the remembered
scope for that run (use `-n ""` to force all namespaces); switching namespace in
the UI updates what's remembered. Switching context (`C`) lands you in *that*
context's remembered namespace, and what you pick afterwards is remembered against
it — `-n` names the scope for the context you launched on, not for every context
you visit.

The remembered kind is restored a moment after launch — once discovery has listed
the cluster's API surface, so a CRD comes back as readily as `pods` does. It is an
address, not a snapshot: kubecom starts a fresh watch on it, so what you see is the
cluster as it is now, never rows left over from last time. Switching context (`C`)
restores that context's kind the same way.

If the kind isn't there — you pinned a CRD on the cluster that runs the operator and
opened one that doesn't, or the API group it lives in is unavailable — nothing is
said and nothing fails: you land on the resource menu and the welcome pane, exactly
as you would have before. And if you drill into something yourself before the restore
gets there, you keep what you chose.

## Migrating from the 2020 kube-commander

If you have an old `~/.kubecom.yaml` from the original kube-commander, kubecom
migrates it once on first start — when no `config.yaml` exists yet. The old file
held two things, and they migrate differently:

- **Your theme choice is carried over.** If `currentTheme` named a palette kubecom
  still ships, migration writes it to `theme:` in the new `config.yaml` — `monokai`
  stays `monokai`, and the old `solarized` becomes `solarized-dark` (the same
  palette, renamed). The 2020 built-ins with no port yet (`base16`, `paraiso`,
  `twilight`) fall back to the default theme, and the startup notice names the
  themes you *can* pick. Hand-written palettes under `themes:` are not migrated —
  kubecom's themes are built-in, so a custom color set has nowhere to go.
- **The custom resource menu is not.** The old format stored no API
  version/resource, which the per-context menu needs, so migration lists the
  entries it found and you re-add them in a [per-context menu
  file](#per-context-menu).

Migration writes the new `config.yaml` once and shows a brief startup notice with
whatever it could not carry over. Your old `~/.kubecom.yaml` is left untouched. A
malformed legacy file is ignored and never blocks launch.
