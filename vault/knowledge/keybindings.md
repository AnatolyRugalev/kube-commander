# Keybindings

Two decisions govern this file:
- [D10](decisions.md#d10--vim-style-navigation-first-class-arrowsclassic-as-fallback):
  **vim-first navigation, arrows/classic as fallback.**
- [D11](decisions.md#d11--fully-configurable-keybindings-zero-hard-coded-keys):
  **fully configurable, zero hard-coded keys.**

Everything below (the vim scheme included) is the **default keymap**. Users can
rebind any action via config. Implement with `bubbles/key`, but build the
`Binding`s *from the resolved keymap*, never from literals.

> The authoritative, always-current default map is generated from the registry
> into [`docs/keybindings.md`](../../docs/keybindings.md) (`make keys-doc`;
> drift-guarded by `TestKeybindingsDoc`, M2-01e/D51). The tables in this file are
> the human design intent; the generated doc is what actually ships.

## Architecture: action registry (the "zero hard-coded keys" rule)

- Every user-triggerable behavior is a named **`Action`** (a stable string id,
  e.g. `nav.down`, `list.drillIn`, `res.describe`, `pod.logs`).
- A **default keymap** — one data table in code — maps `Action → []key`,
  expressing the vim-first defaults + fallbacks.
- At startup: `resolved = merge(defaultKeymap, config.Keys)` then **validate**.
- Every view resolves input as `keymap.Action(keyMsg)` and switches on the
  `Action`. **No view ever matches a raw key.** (`case KeyRune 'l'` is banned.)
- The help overlay and the generated keybindings doc are produced **from the
  registry**, so they can't drift from reality.

### Key syntax (config + defaults)
Human-readable tokens: `j`, `k`, `up`, `down`, `enter`, `esc`, `pgup`, `pgdn`,
`home`, `end`, `ctrl+d`, `ctrl+u`, `space`, `/`. Multi-key sequences allowed
(`gg`). An action may bind multiple keys (vim + fallback).

### Validation at load
- Unknown action id → error (typo protection).
- Two actions bound to the same key **in the same context** → error with both names.
- Binding an action over a default navigation key → allowed, but **warn** (D10).
- Empty binding → action becomes unavailable (allowed; user opt-out).

## Config format

Lives in the plain-YAML config (see [`stack.md`](stack.md)) under `keys:`.
Overrides are merged onto defaults — you only list what you change:

```yaml
keys:
  # action id: list of keys (replaces that action's default binding)
  res.describe: [d, f3]
  pod.logs:     [L]
  nav.down:     [j, down]     # redundant with default; shown for shape
  res.delete:   [x, delete]
```

Omitted actions keep their defaults. `kubecom keys` (planned) prints the fully
resolved map and any warnings.

## Default navigation keys

These are defaults (overridable), but action **defaults** must not collide with them:

`h` `j` `k` `l` · `g` `G` · `n` `N` · `/` · `Ctrl+u` `Ctrl+d` `Ctrl+f` `Ctrl+b`

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
port-forward). Resolved by moving actions **behind a leader / actions menu** so the
nav keys stay clean and the action set is discoverable. **Finalized in M3-02**
(D107): the actions menu (`a`) lists the actions applicable to the selected row's
kind; the most-used actions also have a direct key; the rest are menu-only.

| Action | Binding | Notes |
|--------|---------|-------|
| Open actions menu for selection | `a` (`actions.menu`) | lists the applicable actions for the selected row |
| Describe | `D` (`res.describe`) | any kind; read-only. Relocated off `d` (D133) so delete can take `d` |
| Logs | `L` (`res.logs`) | Pod + pod-owning kinds (#84) |
| Toggle log follow | `f` (`logs.follow`) | logs viewer only; auto-scroll on/off, manual up-scroll pauses (M3-06) |
| Toggle logs grep regex | `Ctrl+r` (`logs.regex`) | logs view only; re-reads the live grep as a case-insensitive regex. A no-text chord by necessity — it must fire while the grep field is open, and that field swallows every text key (LOGS-03/D145) |
| Jump to latest log line | `G` (`nav.bottom`) | logs view only; the ordinary jump-to-bottom *also re-arms* following, so it is the one "catch up and keep tailing" gesture — the inverse of "any upward scroll pauses". Incremental downward scrolling does not re-arm (LOGS-04c/D147) |
| Toggle log line wrap | `w` (`logs.wrap`) | logs view only; soft-wrap long lines vs clip them. While clipping, `h`/`l` (`nav.left`/`nav.right`) scroll the view horizontally — the two are mutually exclusive, so one toggle covers both (LOGS-04a) |
| Toggle log timestamps | `t` (`logs.timestamps`) | logs view only; shows each line's server timestamp ahead of its message (`kubectl logs --timestamps`). **Display only** — the stream always requests timestamps, so the toggle redraws the buffer instead of re-fetching, and the grep still matches only the message. Off by default (an RFC3339 stamp is 30 columns). Hinted nowhere — the closed-grep hint line is full, so it lives in `?` and `docs/keybindings.md` (LOGS-04b/D148) |
| View / Edit YAML | `e` (`res.edit`) | any gettable kind; opens the object's YAML in `$EDITOR` — the single view+edit surface. Save applies (needs `update`/`patch`; a read-only save degrades to a toast). The standalone read-only YAML viewer + its `y` key were retired here (D135/M3-15c); `y` is now unbound |
| Delete | `d` (`res.delete`) | any kind with `delete` (confirm); vim `dd` muscle memory (D133) |
| Scale · Rollout restart | via actions menu | Deployment/RS/StatefulSet/… |
| Cordon · Uncordon · Drain | via actions menu | Node |
| Suspend · Resume | via actions menu | CronJob (#83) |
| Port-forward · Exec shell | via actions menu | Pod (+Service for forward) |
| Reveal secret | via actions menu | Secret (#89) — opens the secret viewer masked |
| Reveal / hide secret values | `r` (`secret.reveal`) | secret viewer only; values start masked, `r` toggles reveal (M3-08a) |
| Copy selected secret value | `c` (`secret.copy`) | secret viewer only; `j`/`k` move the entry cursor, `c` yanks the decoded value to the clipboard (OSC-52), masked or revealed (M3-08b) |
| Toggle port-forward panel | `F` (`forwards.panel`) | app-global overlay listing active forwards; `j`/`k` move the cursor, `enter` stops the selected one (M3-13b) |
| Stop all port-forwards | `X` (`forwards.stopAll`) | port-forward panel only; cancels every active forward at once (M3-13b) |
| Set the local port | `p` (`forwards.localPort`) | port picker only; prompt seeded with the remote port, blank = free port (FB-pf-local-port/D139) |
| Free local port | `0` (`forwards.freeLocal`) | port picker only; forwards `:<remote>` so the OS assigns the local port — `enter` still forwards local = remote (D139) |
| Cluster search | `Ctrl+s` (`search.cluster`) | app-global full-screen search across the curated kinds in the current namespace; `enter` jumps to the hit, `esc` clears then closes (SEARCH-02b/D141). Distinct from `/` (`app.filter`), which narrows the open table. A ctrl chord by necessity: the view's query field is always open, so a text key would type instead of firing (D140 pt 1) |
| Widen search to all kinds | `Ctrl+a` (`search.allKinds`) | search view only; swaps the curated kind set for every discovered kind and re-runs the current query, header gains `all kinds` (SEARCH-04a/D149). A ctrl chord for the same reason as `Ctrl+s` — the query field is always open. Off again on every fresh open: the widen is per-search, not a mode |
| Help overlay | `?` | |
| Namespace picker | `Ctrl+n` (`ns.switch`) | |
| Resource palette | `:` (`resources.switch`) | |
| Context switcher | `:` ctx (M4) | |

`a D y e d` and `L` don't collide with reserved nav keys (`d`/`D` are not in the
reserved nav set; delete uses `d`, describe `D`, D133). The direct keys and the
menu both dispatch one typed `rowActionMsg` intent (D107); each later M3 leg
(M3-03…) wires the real viewer/action. The generated
[`docs/keybindings.md`](../../docs/keybindings.md) is the shipping map.

## Rules for implementers
- Every binding registered via `bubbles/key.Binding` with **both** the vim key and
  its fallback, plus help text — so the help overlay and generated docs stay honest.
- Never assign an action to `h j k l n g G /` or the `Ctrl+u/d/f/b` set.
- Modal/picker views inherit `j/k` + arrows and `Enter`/`Esc` consistently.
