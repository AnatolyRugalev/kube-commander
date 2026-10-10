# Keybindings

Two decisions govern this file:
- [D10](decisions.md#d10--vim-style-navigation-first-class-arrowsclassic-as-fallback):
  **vim-first navigation, arrows/classic as fallback.**
- [D11](decisions.md#d11--fully-configurable-keybindings-zero-hard-coded-keys):
  **fully configurable, zero hard-coded keys.**

Everything below (the vim scheme included) is the **default keymap**. Users can
rebind any action, in any context, in config.kdl's `keymap { }` (D303). Help and
hints are built *from the keymap in the context path*, never from literals.

> The authoritative default map is `internal/tui/keymap/default.kdl`
> (`kubecom keys default` prints it), generated into
> [`docs/keybindings.md`](../../docs/keybindings.md) (`make keys-doc`;
> drift-guarded by `TestKeybindingsDoc`, M2-01e/D51). The tables in this file are
> the human design intent; default.kdl is what actually ships.

## Architecture: actions, contexts, layers (D11, D303)

- Every user-triggerable behavior is a named **`Action`** (a stable string id,
  e.g. `nav.down`, `res.describe`), registered in `internal/tui/keymap`.
- Keys live in KDL keymaps, the context engine in `internal/keybind` (ported
  from plexos, not imported): `bind "<keys>" "<action>" [value]`, `unbind`,
  `group`, `leader`, `base`, and `context "<CEL>" { … }` blocks.
- Layers load in order: default.kdl, config.kdl's `keymap`, the overlay's.
  Precedence: the deepest context wins, then the later layer, then the later line.
- The TUI builds a **context path** for every key (`Model.keyPath`): cluster,
  menu or table (resource, kind…), sort, one view (logs, viewer, search,
  unhealthy), one overlay (forwards, help, picker, prompt, confirm), field, and
  `focus`, the innermost node. Every key resolves in it: `Keymap.Action(key,
  path)` or the Sequencer for `g g`. **No view ever matches a raw key.**
- A **field** on the path keeps its text: a key that types goes to the field
  unless a binding as deep as the field takes it (D303 pt 5) — the old
  "mapped and no text" split, now in one place.
- The help overlay, the hint line (`HelpContextOf(path)` picks the curated set)
  and the generated doc read the same keymap, so they can't drift from it.

### Key syntax (config + defaults)
Human-readable tokens: `j`, `G`, `up`, `down`, `enter`, `esc`, `pgup`, `pgdn`,
`home`, `end`, `ctrl+d`, `alt+b`, `shift+tab`, `space`, `/`. A sequence is
space-separated (`g g`, `<leader> p`). An action may have several bindings (vim
+ fallback).

### Validation at load
- Unknown action id, a value for an action that takes none, an unreadable key or
  context, `<leader>` with no leader → the launch fails at file:line
  (`kubecom keys check` runs the same check).
- Two bindings of the same keys are not an error: precedence picks one.
- The old nav-shadow warning (binding an action over a navigation key) is
  retired with the flat map: a context binding over `j` is the point.
- `unbind` (or `base "none"`) makes an action unavailable; the palette still runs it.

## Config format

```kdl
version 1
keymap {
    bind "L" "res.logs"
    unbind "q"
    context "table.kind == 'Pod'" {
        bind "g p" "resources.switch" "pods"
    }
}
```

The full reference, with the contexts and their attributes, is
[`docs/configuration.md#key-bindings`](../../docs/configuration.md#key-bindings).
`kubecom keys` lists what is in force and where each binding comes from.

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
| Top / bottom | `g g` / `G` | `Home` / `End` |
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
(D107): `a` lists the actions applicable to the selected row's kind; the most-used
actions also have a direct key; the rest are reachable only through that list. Since
**PAL-05d** (D210) the list is not a menu of its own — `a` opens the command palette's
`:action ` stage over the same set, so "via the actions menu" below means "via `a` /
the palette's row verbs".

| Action | Binding | Notes |
|--------|---------|-------|
| Act on the selected row | `a` (`actions.menu`) | opens the palette's `:action ` stage over the actions applicable to the selected row (PAL-05d/D210) |
| Describe | `D` (`res.describe`) | any kind; read-only. Relocated off `d` (D133) so delete can take `d` |
| Logs | `L` (`res.logs`) | Pod + pod-owning kinds (#84) |
| Toggle log follow | `f` (`logs.follow`) | logs viewer only; auto-scroll on/off, manual up-scroll pauses (M3-06) |
| Toggle logs grep regex | `Ctrl+r` (`logs.regex`) | logs view only; re-reads the live grep as a case-insensitive regex. A no-text chord by necessity — it must fire while the grep field is open, and that field swallows every text key (LOGS-03/D145) |
| Jump to latest log line | `G` (`nav.bottom`) | logs view only; the ordinary jump-to-bottom *also re-arms* following, so it is the one "catch up and keep tailing" gesture — the inverse of "any upward scroll pauses". Incremental downward scrolling does not re-arm (LOGS-04c/D147) |
| Toggle log line wrap | `w` (`logs.wrap`) | logs view only; soft-wrap long lines vs clip them. While clipping, `h`/`l` (`nav.left`/`nav.right`) scroll the view horizontally — the two are mutually exclusive, so one toggle covers both (LOGS-04a) |
| Toggle log timestamps | `t` (`logs.timestamps`) | logs view only; shows each line's server timestamp ahead of its message (`kubectl logs --timestamps`). **Display only** — the stream always requests timestamps, so the toggle redraws the buffer instead of re-fetching, and the grep still matches only the message. Off by default (an RFC3339 stamp is 30 columns). Hinted nowhere — the closed-grep hint line is full, so it lives in `?` and `docs/keybindings.md` (LOGS-04b/D148) |
| Toggle previous-instance logs | `o` / `Ctrl+p` (`logs.previous`) | logs view only; re-opens the stream with `kube.LogOptions.Previous` — the container's previous *terminated* instance (`kubectl logs -p`), which is where a `CrashLoopBackOff` explains itself. A **request** flag, not a display toggle: the buffer is replaced (two instances are two logs), but the grep, wrap and timestamps survive the flip, and the header shows `[previous]`. `o` (the **o**ld/**o**ther instance) since LOGS-09/D258 — the maintainer found `Ctrl+p` weird, and the mnemonic `p`/`P` are both spent (D139/D165); the chord stays as the second binding because, carrying no text, it is the one form that fires mid-grep (D140 pt 1). Also listed by the palette over the logs view, so the key is no longer the only way to find it (M5-01a/D177) |
| View / Edit YAML | `e` (`res.edit`) | any gettable kind; opens the object's YAML in `$EDITOR` — the single view+edit surface. Save applies (needs `update`/`patch`; a read-only save degrades to a toast). The standalone read-only YAML viewer + its `y` key were retired here (D135/M3-15c); `y` is now unbound |
| Delete | `d` (`res.delete`) | any kind with `delete` (confirm); vim `dd` muscle memory (D133) |
| Show the owner's pods | `P` (`res.children`) | the owner kinds `kube.HasChildren` names (Deployment/RS/StatefulSet/DaemonSet/Job/RC, Service, Node); switches the table to that owner's pods under a server-side selector, `esc` returns to the owner. `P` because lowercase `p` is the port picker's local-port prompt (M4-08/D166) |
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
| Widen search to all namespaces | `Ctrl+w` (`search.allNamespaces`) | search view only; searches every namespace and re-runs the current query, header *replaces* the namespace it names with `all namespaces` — a header carries one namespace scope, never two (SEARCH-04b/D150). Independent of the kind widen, so all four scope combinations are reachable; never changes the app's own namespace. `Ctrl+w` because `Ctrl+a` is spent and `Ctrl+n` is `ns.switch`: it reads as cluster-**w**ide. Off again on every fresh open |
| Help overlay | `?` | |
| Namespace picker | `Ctrl+n` (`ns.switch`) | |
| Command palette | `:` (`app.palette`) | app-global verb list; type to narrow, `enter` runs the verb through the same dispatch its key uses (PAL-02/D197). Opens over the logs view too — then listing the logs view's own verbs beside the globals, the row verbs withheld since their target is invisible there (LOGS-09/D258) |
| Resource picker | `R` (`resources.switch`) | took `R` when the palette took `:`; also the palette's "Switch resource" verb (D197 pt 3) |
| Context switcher | `C` (M4) | opens the palette's `:context ` stage; reachable from the logs view like the rest of the palette family (LOGS-09/D258) |

`a D y e d` and `L` don't collide with reserved nav keys (`d`/`D` are not in the
reserved nav set; delete uses `d`, describe `D`, D133). The direct keys and the
menu both dispatch one typed `rowActionMsg` intent (D107); each later M3 leg
(M3-03…) wires the real viewer/action. The generated
[`docs/keybindings.md`](../../docs/keybindings.md) is the shipping map.

## Rules for implementers
- Every default binding goes in default.kdl with **both** the vim key and its
  fallback; the registry carries the help text — so the help overlay and generated
  docs stay honest.
- A key that should act only on one surface goes in that surface's `context`
  block, not in a router.
- Never assign an action to `h j k l n g G /` or the `Ctrl+u/d/f/b` set.
- Modal/picker views inherit `j/k` + arrows and `Enter`/`Esc` consistently.

## Which surface owns a key (D258)

Input is captured top-down, and a surface that owns input owns it fully: the search
view and any open text field (the logs grep, a picker's filter, the table filter)
take every text-carrying key as *text*; transient modals (a picker, the confirm
modal, the help overlay, the viewer, the port-forward panel) swallow the keys they
do not use — the way out of any of them is `esc`, and that is by design, since a
modal is one gesture deep. The **logs view** is the one long-lived capture surface,
and it is the exception: the palette family (`:`, `T`, `R`, `Ctrl+n`, `C`) passes
through it, so the switchers and every logs verb stay reachable from a log stream
(LOGS-09). Row-scoped gestures (`a`, `d`, …) stay swallowed there — their target is
the browse table's selection, which the full-screen view hides.
