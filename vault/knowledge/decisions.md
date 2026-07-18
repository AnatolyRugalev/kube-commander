# Decision Log

Append-only. Supersede rather than delete; note the date. Newest at the bottom.

---

### D1 — TUI framework: Bubble Tea
**2026-07-18.** Use **Bubble Tea + Bubbles + Lipgloss** (Elm architecture).
**Why:** the original's top defect class — data races, focus bugs, popup/redraw
issues — comes from driving `tcell/views`' imperative widget tree from
goroutines. Bubble Tea's single-threaded update loop removes that class by
construction. tview (what k9s uses) was considered but keeps the imperative
model. **Consequence:** no shared mutable UI state; concurrency via messages.

### D2 — In-process client-go, shell out only for interactivity
**2026-07-18.** Reach for client-go first; shell out **only** for exec shell and
`$EDITOR`. **Why:** removes the hard `kubectl` binary dependency (**#68**),
enables background port-forward, and in-TUI logs/describe/YAML. Modern client-go
covers logs/describe/portforward/remotecommand/actions. Push in-process "until we
hit a wall."

### D3 — Config: plain YAML
**2026-07-18.** Drop the protobuf config (`pb/config.proto` + generated code) for
a plain typed Go struct marshalled to YAML. **Why:** the protobuf machinery and
its abandoned theme engine were over-engineered for a small local file.

### D4 — Name: `kubecom`, single binary
**2026-07-18.** Standardize on `kubecom`; retire the duplicate `kube-commander`
binary. **Why:** the original shipped two identical entrypoints and inconsistent
naming.

### D5 — Kubernetes/client-go version
**2026-07-18.** Build against a recent client-go (**target v0.31 / K8s 1.31**);
support servers **~1.27+** via discovery so it degrades gracefully. "Recent, not
too aggressive."

### D6 — Backwards compatibility: clean break + migration
**2026-07-18.** No runtime BC with the old app; instead a one-shot **migration**
of the old `~/.kubecom.yaml` on first start. **Why:** clean architecture beats
carrying legacy config semantics.

### D7 — Platforms: Linux + macOS only (WSL2 for Windows)
**2026-07-18.** Drop native Windows support; recommend **WSL2**. **Why:** removes
Windows PTY/exec complexity (a flagged risk) for a niche of the user base.

### D8 — Async, cached resource discovery
**2026-07-18.** First-paint from a seed set of core GVKs; run full discovery in
the background and reconcile the menu via a "discovery ready" message; isolate
per-group failures; cache discovery on disk. **Why:** the original blocked the
whole UI on `ServerPreferredResources()`, hurting cold starts and turning one bad
API group into a total failure (**#87**, **#76**, **#86**).

### D9 — Branch model
**2026-07-18.** `master` = original code, untouched for now. `v1` = rewrite
branch, holds this vault and all rewrite work. A `main` branch becomes the final
destination when the rewrite is ready to be default. Push `v1` to origin.

### D10 — Vim-style navigation first-class; arrows/classic as fallback
**2026-07-18.** Navigation is **vim-first**: `hjkl`, `gg`/`G`, `Ctrl+u`/`Ctrl+d`,
`/` search + `n`/`N`, `l`/`Enter` to drill in, `h`/`Esc` to go back. Arrow keys,
`PgUp`/`PgDn`, `Home`/`End`, `Enter`/`Esc` work as an equivalent **fallback** so
non-vim users are never stranded. **Why:** the target user is a terminal-native
hacker; muscle-memory navigation is a core UX value, not an add-on.
**Consequence / rule:** `h j k l n` (and `g`, `G`) are the **default**
navigation keys — single-letter *action* defaults must not collide with them.
This supersedes the legacy pod bindings where they clash (legacy used `l` for
logs, `s` shell, `f` port-forward): default actions off the nav keys (e.g. behind
a leader or an actions menu). All of this is *default*, not fixed — see D11. See
[`keybindings.md`](keybindings.md).

### D11 — Fully configurable keybindings; zero hard-coded keys
**2026-07-18.** **No key literal is ever matched in view/update code.** Every
user-triggerable behavior is a named **Action**; an **action registry** maps
`Action → []key` and is the *only* place keys exist. The registry is built from a
**default keymap** (data in one place, expressing the vim-first scheme of D10)
overlaid by the user's config, then consulted by every view to resolve
`tea.KeyMsg → Action`. **Why:** users must be able to rebind anything, and
scattered `case 'l':`-style handling is exactly what made the old code rigid.
**Consequences:**
- Config carries a `keys:` section: `action -> [keys]` overrides merged onto defaults.
- `bubbles/key.Binding`s are constructed *from* the resolved keymap, not literals.
- Loading **validates**: unknown action names error; within-context collisions
  error with a clear message; the vim/nav defaults are just defaults a user may
  override (with a warning if they shadow navigation).
- The help overlay and the generated keybindings doc are **derived from the
  registry**, so they can never drift from actual bindings.
See [`keybindings.md`](keybindings.md).
