# Configuring kubecom

kubecom runs on sensible defaults — none of this is required. Everything here is
opt-in, and kubecom never writes the file you edit: what it records for you goes to
files of its own.

- [The config file](#the-config-file) — `config.kdl`, and the overlay kubecom writes
- [Key bindings](#key-bindings) — `keymap { }`, contexts, and what wins
- [Themes](#themes) — the built-in palettes
- [Per-context menu](#per-context-menu) — adding CRDs to the resource menu
- [Pinned kinds](#pinned-kinds) — what `*` writes, and where
- [What kubecom remembers](#what-kubecom-remembers) — the namespace and the kind you left open
- [Migrating from the 2020 kube-commander](#migrating-from-the-2020-kube-commander)

## The config file

kubecom reads an optional [KDL](https://kdl.dev) config from
`os.UserConfigDir()/kubecom/config.kdl` (`~/.config/kubecom/config.kdl` on Linux,
`~/Library/Application Support/kubecom/config.kdl` on macOS; `--config-dir` points
kubecom at another directory). It starts with `version 1`:

```kdl
// ~/.config/kubecom/config.kdl
version 1

theme "monokai"

keymap {
    bind "g p" "resources.switch" "pods"
    context "cluster.context.startsWith('prod-')" {
        unbind "D"
    }
}

menu "my-cluster" {
    resource "certificates" group="cert-manager.io" version="v1" kind="Certificate" namespaced=#true
}
```

**kubecom never writes `config.kdl`**, so your comments and layout stay as you
left them. What kubecom does write — the theme you pick with `T` — goes to
`config.overlay.kdl` beside it, which is read on top of `config.kdl` setting by
setting: a theme picked in kubecom wins over the one in your file until you delete
that line from the overlay.

A mistake is reported with its place — `config.kdl:4:1: colour is not a setting
(kubecom reads `theme`; …)` — and kubecom does not start on a config it cannot
read, so a typo never silently does nothing.

kubecom 26.10.10 and older read `config.yaml` and `menus/<context>.yaml`; this
kubecom does not, and says so at start while those files are there and
`config.kdl` is not.

## Key bindings

Every key is a default, and the defaults are a `keymap` like yours:
`kubecom keys default` prints them, and [`keybindings.md`](keybindings.md) lists
them by action. Your `keymap { }` is read on top:

```kdl
keymap {
    bind "ctrl+n" "ns.switch"           // a second key for the namespace switcher
    unbind "q"                          // ctrl+c still quits
    bind "g p" "resources.switch" "pods" // a value runs the verb with it
    context "logs" {
        bind "W" "logs.wrap"
    }
}
```

- `bind "<keys>" "<action>" ["<value>"]` binds keys to an action; the action ids
  are the ones `keybindings.md` and the `:` palette show. A value is for the
  verbs that take one: without one the key opens the palette on that verb, with
  one it runs it, wherever the key without a value would open the palette.

  | Verb | Value |
  |------|-------|
  | `resources.switch`, `menu.pin` | a kind as `:resource` finds it: `Pod`, `pods`, `po`, or `certificates.cert-manager.io` |
  | `ns.switch` | a namespace; `""` is all namespaces |
  | `ctx.switch` | a kubeconfig context |
  | `theme.switch` | a theme name (checked when the keymap loads) |
  | `actions.menu` | a row action, as `:action` lists it (`Describe`) or by id (`describe`) |

  A value that names nothing there says so on the status line.
- Keys are written `j`, `G`, `ctrl+d`, `alt+x`, `shift+tab`, `enter`, `esc`,
  `space`, `pgdn`, `up`, `f5`; a sequence separates its keys with spaces: `g g`.
  They are read as [Ghostty](https://ghostty.org) reads them, so modifiers come in
  any order and case and a key may take its full name (`escape`, `page_down`,
  `arrow_up`). A capital is Shift with its letter, as in vim: `G` is `shift+g`,
  and `ctrl+D` is `ctrl+shift+d`, not `ctrl+d`.
- `unbind "<keys>"` takes keys away; unbinding `g` takes every `g …` with it.
  An unbind is a binding to nothing, so it wins the way a binding does: a key the
  defaults bind in a context (`kubecom keys` shows which) is unbound in that
  context — `context "logs" { unbind "f" }` — and `kubecom keys check` warns of a
  top-level unbind that does not reach one.
- `context "<expression>" { … }` binds only where the expression holds.
- `base "none"` drops the defaults, so every key is yours to bind.
- `leader "<key>"` names a key `<leader>` stands for in your bindings.

**Contexts** describe what holds the keyboard, from the outside in. Each is true
while it is there, and has attributes you can test:

| Name | When | Attributes |
|------|------|------------|
| `cluster` | connected to a cluster | `context`, `namespace` (`""` = all) |
| `menu` | the resource menu has focus | |
| `table` | a resource table has focus | `resource`, `group`, `kind`, `namespaced`, `drilled`, `filtered`, `unhealthy_only` |
| `sort` | the table's column-header sort mode (`S`) | |
| `logs` | the logs view | `follow`, `wrap`, `timestamps`, `previous`, `selecting` |
| `viewer` | describe, events or a secret | `content` (`describe`, `events`, `secret`) |
| `search` | cluster search (`ctrl+f`) | `all_kinds`, `all_namespaces` |
| `unhealthy` | the cross-kind unhealthy list (`U`) | |
| `forwards` | the port-forward panel (`F`) | |
| `help` | the `?` overlay | |
| `picker` | the palette or a picker | `id` (`palette`, `containers`, `ports`, `relations`), `stage` (the palette's verb) |
| `prompt`, `confirm` | a prompt, a yes/no dialog | |
| `field` | a text field: a filter, the logs grep, a query, a prompt | |

`focus` is the name of the innermost one: `context "table"` holds even with a
dialog open over the table, `context "focus == 'table'"` only while the table
itself has the keys. Expressions are [CEL](https://cel.dev); write strings in single
quotes (`table.kind == 'Pod'`).

**What wins:** the binding whose context reaches deepest; at the same depth yours
over the default; then the later line. And **a text field keeps its text**: while
a field is open, a key that types — a letter, a digit, space — goes into it unless
a binding in `context "field"` takes it, so `bind "x" …` never stops you typing an
`x` into a filter.

`kubecom keys` prints what is in force and where each binding comes from;
`kubecom keys check` reads your keymap and reports what is wrong with it.

kubecom's keymap runs on [myelin](https://github.com/neuroplastio/myelin), a
key-binding library; its [keymap reference](https://github.com/neuroplastio/myelin/blob/main/docs/keymaps.md)
has the whole key spelling and the rules for contexts and what wins.

## Themes

`theme` picks the palette kubecom renders with. Fourteen are built in — eleven
dark and three light:

| Name | |
|------|--|
| `default` | dark-friendly, blue accent (used when `theme` is absent) — this is Catppuccin Frappé |
| `catppuccin-frappe` | Catppuccin's mid-dark flavor — the same palette as `default` |
| `catppuccin-latte` | **light** — Catppuccin's light flavor |
| `catppuccin-macchiato` | Catppuccin, darker and cooler than Frappé |
| `catppuccin-mocha` | Catppuccin's darkest flavor |
| `dracula` | Dracula: high-contrast, purple accent and a pink header |
| `gruvbox-dark` | gruvbox's dark variant at medium contrast — warm, retro, low-glare |
| `gruvbox-light` | **light** — gruvbox's light variant |
| `monokai` | the classic warm dark palette, cyan accent |
| `nord` | Nord's arctic blue-greys, muted throughout |
| `rose-pine` | Rosé Pine's `main` variant: muted plum with a rose header |
| `solarized-dark` | Solarized's dark variant |
| `solarized-light` | **light** — Solarized's light variant |
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

**The three light palettes are worth one extra thought.** `catppuccin-latte`,
`solarized-light` and `gruvbox-light` paint a light canvas, so on a dark terminal
they depend on the background actually reaching the screen — and that is precisely
the case the check above exists for. If you run inside tmux without passthrough,
expect the line `catppuccin-latte is light but the terminal stayed dark`; turn
passthrough on, or set your terminal's background to the palette's (`#eff1f5` for
Latte, `#fdf6e3` for Solarized Light, `#fbf1c7` for Gruvbox Light), and it goes
quiet. On a terminal that honours the request they need nothing.

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
`:theme ` line). The palette **previews live**: moving the selection re-themes the
whole screen — painted background included — before you commit, so comparing
palettes is move-and-look, not open-and-reopen. `enter` commits the highlighted
theme: the name is written to `config.overlay.kdl` so the next launch opens on it,
and `esc` closes the palette returning you to the theme you had when it opened.
Your `config.kdl` is never touched.

## Per-context menu

kubecom can add extra resource types (chiefly CRDs the built-in menu doesn't know)
to the browse menu. A `menu` in `config.kdl` names the kubeconfig context it is for,
or none to apply to every context:

```kdl
menu "my-cluster" {
    // plural, as the API addresses it; omit group for the core ("") group
    resource "certificates" group="cert-manager.io" version="v1" kind="Certificate" namespaced=#true
}
menu {
    resource "rollouts" group="argoproj.io" version="v1alpha1" kind="Rollout" namespaced=#true section="Argo"
}
```

`version` is required; `kind` (default: from the resource), `namespaced` (default
`#false`, cluster-scoped), `section` (default: the Custom Resources group) and
`title` are optional. The entries for every context come first, then the
context's own. An entry whose resource is already in the menu (a seed row or one
discovery finds) is merged, never listed twice.

## Pinned kinds

Pressing `*` on a kind (or `:pin ` in the command palette) keeps that kind in this
context's menu whether or not discovery lists it. Pins are recorded *for* you, so
they live in the kubecom-managed state file
(`~/.config/kubecom/state/<context>.kdl`) rather than in your `config.kdl` —
pressing `*` never touches it.

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
`os.UserConfigDir()/kubecom/state/<context>.kdl` (`~/.config/kubecom/state/` on
Linux) — a kubecom-managed file, separate from your config, so
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

A remembered **drill-in** comes back as the drill-in, not as its plain list: if the
pane you left was scoped to an owner's pods (`P`, see the main README), kubecom
re-resolves that owner and re-enters the scope, so the children table — and the
`↳ Deployment/web` scope in the status bar — return with the pane. If the owner is
gone by the time you come back, kubecom lands on the plain list instead and says so
in the status bar; it never silently shows you a different scope than the one you
left.

If the kind isn't there — you pinned a CRD on the cluster that runs the operator and
opened one that doesn't, or the API group it lives in is unavailable — nothing is
said and nothing fails: you land on the resource menu and the welcome pane, exactly
as you would have before. And if you drill into something yourself before the restore
gets there, you keep what you chose.

## Migrating from the 2020 kube-commander

If you have an old `~/.kubecom.yaml` from the original kube-commander, kubecom
migrates it once on first start — when neither `config.kdl` nor
`config.overlay.kdl` exists yet. The old file
held two things, and they migrate differently:

- **Your theme choice is carried over.** If `currentTheme` named a palette kubecom
  still ships, migration writes it to `theme` in `config.overlay.kdl` — `monokai`
  stays `monokai`, and the old `solarized` becomes `solarized-dark` (the same
  palette, renamed). The 2020 built-ins with no port yet (`base16`, `paraiso`,
  `twilight`) fall back to the default theme, and the startup notice names the
  themes you *can* pick. Hand-written palettes under `themes:` are not migrated —
  kubecom's themes are built-in, so a custom color set has nowhere to go.
- **The custom resource menu is not.** The old format stored no API
  version/resource, which the per-context menu needs, so migration lists the
  entries it found and you re-add them as [`menu`
  entries](#per-context-menu).

Migration writes `config.overlay.kdl` once and shows a brief startup notice with
whatever it could not carry over. Your old `~/.kubecom.yaml` is left untouched. A
malformed legacy file is ignored and never blocks launch.
