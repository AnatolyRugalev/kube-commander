# Configuring kubecom

kubecom runs on sensible defaults — none of this is required. Everything here is
opt-in, and everything kubecom writes for you lives in its own state file, never
in a file you hand-edit.

- [The config file](#the-config-file) — `keys:`, `theme:`
- [Themes](#themes) — the built-in palettes
- [Per-context menu](#per-context-menu) — adding CRDs to the resource menu
- [Pinned kinds](#pinned-kinds) — what `*` writes, and where
- [Remembered namespace](#remembered-namespace)
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

`theme:` picks the palette kubecom renders with. Six are built in:

| Name | |
|------|--|
| `default` | dark-friendly, blue accent (used when `theme:` is absent) — this is Catppuccin Frappé |
| `catppuccin-frappe` | Catppuccin's mid-dark flavor — the same palette as `default` |
| `catppuccin-macchiato` | Catppuccin, darker and cooler than Frappé |
| `catppuccin-mocha` | Catppuccin's darkest flavor |
| `monokai` | the classic warm dark palette, cyan accent |
| `solarized-dark` | Solarized's dark variant |

All built-ins are **dark** palettes: kubecom draws text over your terminal's own
background rather than painting one, so a light palette (Catppuccin Latte,
Solarized Light) would be unreadable on a dark terminal. Light themes wait on
kubecom painting its own background.

Ported palettes keep their upstream names and attribution: Catppuccin
(MIT, © 2021 Catppuccin), Monokai, Solarized (MIT, © 2011 Ethan Schoonover).

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

## Remembered namespace

kubecom remembers the last namespace you selected, per kubeconfig context, and
reopens on it next time. The choice is stored in
`os.UserConfigDir()/kubecom/state/<context>.yaml` (`~/.config/kubecom/state/` on
Linux) — a kubecom-managed file, separate from your config and menu files, so
kubecom rewrites it freely without touching anything you hand-edit (it also holds
the kinds you pin, above). Passing `-n`/`--namespace` overrides the remembered
scope for that run (use `-n ""` to force all namespaces); switching namespace in
the UI updates what's remembered. Switching context (`C`) lands you in *that*
context's remembered namespace, and what you pick afterwards is remembered against
it — `-n` names the scope for the context you launched on, not for every context
you visit.

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
