# Keybindings

Per [decision D10](decisions.md#d10--vim-style-navigation-first-class-arrowsclassic-as-fallback):
**vim-style navigation is first-class; arrows/classic keys are an equivalent
fallback.** This file is the reference map. Implement with `bubbles/key` so every
action carries both its vim binding and its fallback, and both show up in the
help overlay.

## Reserved navigation keys (do not rebind to actions)

`h` `j` `k` `l` · `g` `G` · `n` `N` · `/` · `Ctrl+u` `Ctrl+d` `Ctrl+f` `Ctrl+b`

Single-letter **action** bindings must avoid these.

## Navigation

| Intent | Vim (first-class) | Fallback |
|--------|-------------------|----------|
| Move down / up | `j` / `k` | `↓` / `↑` |
| Move left pane / right pane | `h` / `l` | `←` / `→` |
| Drill in / open selection | `l` or `Enter` | `→`, `Enter` |
| Back / up a level | `h` or `Esc` | `←`, `Esc` |
| Top / bottom | `gg` / `G` | `Home` / `End` |
| Half page down / up | `Ctrl+d` / `Ctrl+u` | `PgDn` / `PgUp` |
| Full page down / up | `Ctrl+f` / `Ctrl+b` | `PgDn` / `PgUp` |
| Filter / search | `/` | `/` |
| Next / prev match | `n` / `N` | `n` / `N` |
| Quit | `q` (context-aware), `Ctrl+c` | `Ctrl+c` |

Left/right (`h`/`l`) is dual-purpose: pane focus in the two-pane browse view, and
in-list drill in/out — matching how the original mapped `Enter`/`Right`/`Left`.

## Actions (must not use reserved keys)

Legacy pod actions collided with navigation (`l` logs, `s` shell, `f`
port-forward). Resolve by moving actions **behind a leader / actions menu** so the
nav keys stay clean and the action set is discoverable. Proposed (finalize in M3):

| Action | Proposed binding |
|--------|------------------|
| Open actions menu for selection | `a` (or leader `<space>`) |
| Describe | `d` |
| View YAML | `y` |
| Logs | via actions menu / `L` |
| Previous logs | via actions menu |
| Exec shell | via actions menu |
| Port-forward | via actions menu |
| Edit (`$EDITOR`) | `e` |
| Delete | `x` or `Del` (confirm) |
| Help overlay | `?` |
| Namespace picker | `:` ns, or keep `Ctrl+n` |
| Context switcher | `:` ctx (M4) |

`d y e x a` don't collide with reserved nav keys. Exact final map is an M3
deliverable; keep it in this file and generate the help/keybindings doc from it.

## Rules for implementers
- Every binding registered via `bubbles/key.Binding` with **both** the vim key and
  its fallback, plus help text — so the help overlay and generated docs stay honest.
- Never assign an action to `h j k l n g G /` or the `Ctrl+u/d/f/b` set.
- Modal/picker views inherit `j/k` + arrows and `Enter`/`Esc` consistently.
