// Package keymap is kubecom's action registry: the single place an action exists
// (D11). Every user-triggerable behaviour is a named Action, and views resolve a
// live keypress to an Action instead of matching raw keys. Which keys run which
// action, and where, is a context keymap (D303): default.kdl as the default layer,
// the user's `keymap` documents on top, resolved by internal/keybind over the
// focus path the shell builds (Keymap). Help overlays and the generated
// keybindings doc are built from the same keymap, so they cannot drift.
package keymap

// Action is a stable string id for a user-triggerable behaviour (e.g. nav.down).
// It is the only thing views switch on; they never match a raw key (D11).
type Action string

// Registered actions. This slice covers the navigation scheme of D10 plus the
// core app actions; the action-menu set (describe/yaml/delete/…) is finalised in
// M3 (see vault/knowledge/keybindings.md) and registered there.
const (
	ActionUp           Action = "nav.up"
	ActionDown         Action = "nav.down"
	ActionLeft         Action = "nav.left"
	ActionRight        Action = "nav.right"
	ActionDrillIn      Action = "nav.drillIn"
	ActionBack         Action = "nav.back"
	ActionTop          Action = "nav.top"
	ActionBottom       Action = "nav.bottom"
	ActionHalfPageDown Action = "nav.halfPageDown"
	ActionHalfPageUp   Action = "nav.halfPageUp"
	ActionPageDown     Action = "nav.pageDown"
	ActionPageUp       Action = "nav.pageUp"
	ActionFilter       Action = "app.filter"
	ActionSearchNext   Action = "app.searchNext"
	ActionSearchPrev   Action = "app.searchPrev"
	ActionHelp         Action = "app.help"
	ActionQuit         Action = "app.quit"
	// ActionUnhealthy narrows the current resource table to the rows the M4-06
	// classifier reads as unhealthy (STORY-06g-1) — CrashLoopBackOff,
	// ImagePullBackOff, Pending/unschedulable, not-ready, a stuck claim — the
	// "what's broken" quick-access a keypress from anywhere, for the kind you are
	// on. It is the unhealthy-only twin of the `/` substring filter: it composes
	// with it, and the table shows the rows that survive both. It is a browse-view
	// action (not row-scoped), app-global like app.filter, and inert with no
	// resource table showing.
	ActionUnhealthy Action = "app.unhealthy"
	// ActionUnhealthyScan opens the cross-kind "what's broken" list (STORY-06g-2):
	// a kube.Scan sweep over every kind the menu offers, keeping the rows the M4-06
	// classifier reads as unhealthy, so a failure that is not a pod — a stuck PVC,
	// a broken claim — finds the operator instead of the operator guessing which
	// kind to visit (D276). It is the cross-kind twin of ActionUnhealthy (H narrows
	// the kind you are on; this sweeps them all), app-global, and inert with no
	// scanner wired.
	ActionUnhealthyScan Action = "app.unhealthyScan"
	// ActionPalette opens the command palette (PAL-02): one modal list of the
	// app-global *verbs*, fuzzy-ranked like every other picker (D194 pt 1), whose
	// pick runs the chosen verb through the same action dispatch a key press takes.
	// It is the one gesture that needs no keymap knowledge — you type what you want
	// to do — and it takes `:`, the key resources.switch held until this slice, since
	// the resource switch is now one verb inside it rather than the only thing `:`
	// could do. It is app-global and never inert: the verbs it lists are registered
	// actions, so it opens with no cluster (the verbs that need one stay inert
	// exactly as their keys are).
	ActionPalette   Action = "app.palette"
	ActionNamespace Action = "ns.switch"
	ActionResources Action = "resources.switch"
	// ActionContext opens the kubeconfig context switcher (M4-04b): a modal picker
	// over the contexts the kubeconfig declares, whose pick tears the current
	// cluster down and reconnects to the chosen one (M4-04a). It is app-global like
	// ns.switch and, like it, is inert without the seam that feeds it.
	ActionContext Action = "ctx.switch"
	// ActionTheme opens the color-theme switcher (M4-12b-2): a modal picker over the
	// built-in palettes whose pick repaints the running shell (Model.applyStyles,
	// D171) and is written to config.overlay.kdl so the next launch opens on it. It is
	// app-global like ctx.switch and ns.switch, and — unlike them — never inert: the
	// themes it offers are compiled in, so it needs no cluster and no seam. Only the
	// write-back needs one, and a missing persister costs the choice its persistence,
	// not the gesture.
	ActionTheme       Action = "theme.switch"
	ActionToggleMouse Action = "mouse.toggle"
	// ActionUpdate replaces the running kubecom with the newest build from its
	// release channel (see internal/update): the palette verb for `kubecom
	// update`, app-global and with no default key — updating is rare and not
	// worth spending a key. It is the one action that suspends the TUI to fetch
	// and swap the binary.
	ActionUpdate Action = "app.update"
	// ActionSort is the column-header sort gesture (STORY-06b): `S` focuses the
	// table's column-header row — a small mode where `h`/`l`/`left`/`right` move a
	// cursor across the columns, `enter` toggles the sort direction on the cursor
	// column, and `esc` returns focus to the rows. It replaced the old `s` cycle
	// (sort.column was the cycle-column key) after the walk read the s-family as
	// confusing: `s` is now freed entirely, `S` is the one sort gesture, and
	// sort.clear lives inside the mode as a "clear" pick.
	ActionSort       Action = "sort.column"
	ActionClearSort  Action = "sort.clear"
	ActionToggleMenu Action = "menu.toggle"
	// M3 row actions (operate on the selected resource row). ActionActions is the
	// way to *all* of them (D107); the rest are direct-key shortcuts for the
	// most-used ones, all off the reserved nav keys (D10). The full curated action
	// set (scale, cordon, drain, suspend, exec, …) is reachable through it rather
	// than through a key of its own (see vault/knowledge/keybindings.md). Since
	// PAL-05d it is an *argument* verb like ns.switch and resources.switch: it takes
	// the action as its argument and opens the palette's `:action ` stage rather than
	// a menu of its own (D210), so its id keeps the `.menu` suffix for compatibility
	// with existing keymap config while the surface it names is the palette's. Since
	// STORY-06c its default key is `enter`, bound in `context "focus == 'table'"`
	// (D303): pressing enter on the selected row opens the menu.
	ActionActions  Action = "actions.menu"
	ActionDescribe Action = "res.describe"
	// ActionEvents lists the selected row's events (STORY-06f): the object's core
	// Events as their own list in the shared viewer — the surface for "why is this
	// red" instead of hunting for them inside describe. It is a direct-key M3 row
	// action like describe, gated on the kind's get verb; elsewhere it is inert.
	ActionEvents Action = "res.events"
	ActionLogs   Action = "res.logs"
	// ActionEdit is the unified object-YAML action (D135/M3-15c): it opens the
	// selected object's YAML in $EDITOR — the single way to both view and edit an
	// object's YAML, replacing the retired standalone read-only YAML viewer. Viewing
	// is `:q` (no-change is a neutral no-op); a save applies via the Editor seam.
	ActionEdit   Action = "res.edit"
	ActionDelete Action = "res.delete"
	// ActionChildren drills from the selected owner row to its pods (M4-08): the
	// browse table switches to the child kind, scoped by the selector kube.Children
	// resolved for that object, and nav.back returns to the owner. It is meaningful
	// only on a kind that has children (kube.HasChildren — the workload kinds,
	// Service and Node); on any other kind it is inert, exactly as the entry is
	// absent from that kind's actions menu.
	ActionChildren Action = "res.children"
	// ActionRelations opens the relations popup over the selected row (STORY-06i-2):
	// the object's navigable neighbours — the owner that created it, the pods it
	// owns, the node it runs on, the claims/config maps/secrets it mounts — as one
	// list, each row opening the thing it names. It is the reverse of res.children
	// generalized to both directions, gated on the kind's get verb like describe;
	// elsewhere it is inert.
	ActionRelations Action = "res.relations"
	// ActionLogsFollow toggles follow (auto-scroll + live streaming) inside the
	// open logs viewer (M3-06). It is meaningful only while the logs viewer is up;
	// elsewhere it is inert.
	ActionLogsFollow Action = "logs.follow"
	// ActionLogsRegex toggles the logs view's live grep between case-insensitive
	// substring matching (the default) and case-insensitive **regex** matching
	// (LOGS-03). It is bound to a no-text chord on purpose: the grep field swallows
	// every text-producing key while it is open (D140 pt 1), so a plain letter could
	// only toggle the mode *before* typing — where the mode matters least. It is
	// meaningful only while the logs view is up; elsewhere it is inert.
	ActionLogsRegex Action = "logs.regex"
	// ActionLogsWrap toggles how the logs view treats a line wider than the screen
	// (LOGS-04a): soft-wrapped onto continuation rows, or clipped at the right edge
	// with nav.left/nav.right scrolling horizontally to reach the tail. The two are
	// mutually exclusive by construction — a wrapped view has no horizontal offset —
	// so one toggle covers both. It is meaningful only while the logs view is up;
	// elsewhere it is inert.
	ActionLogsWrap Action = "logs.wrap"
	// ActionLogsTimestamps shows or hides each log line's server timestamp in the
	// logs view (LOGS-04b), the in-TUI equivalent of `kubectl logs --timestamps`.
	// It is a *display* toggle, not a request flag: the stream always carries the
	// timestamps, so flipping it re-renders the buffer already on screen instead of
	// re-fetching the log. It is meaningful only while the logs view is up;
	// elsewhere it is inert.
	ActionLogsTimestamps Action = "logs.timestamps"
	// ActionLogsPrevious switches the open logs view between the container's running
	// instance and its previous *terminated* one (`kubectl logs -p`, M5-01a) — the
	// 2020 build's second log gesture, and what you want after a CrashLoopBackOff,
	// since the log that explains the crash belongs to the instance that already
	// died. Unlike the display toggles beside it this one is a **request** flag: the
	// two instances are two different logs, so flipping it re-opens the stream and
	// the view starts over on the other instance's output. It is meaningful only
	// while the logs view is up; elsewhere it is inert.
	ActionLogsPrevious Action = "logs.previous"
	// ActionLogsSelect starts a **visual selection** in the logs view at the line
	// cursor and, pressed again, cancels it (LOGS-SEL-02): vim's `v`, over log lines
	// rather than screen rows (D242 pt 1). While it is on, the nav keys extend the
	// selection instead of merely moving, and following is suspended for its duration
	// — a selection anchored in a stream running at ~1,900 lines/sec is not a
	// selection. It is meaningful only while the logs view is up; elsewhere it is
	// inert.
	ActionLogsSelect Action = "logs.select"
	// ActionLogsYank copies the selected log lines — or, with no selection, the
	// cursor's line — to the system clipboard (LOGS-SEL-02), through the same OSC-52
	// path secret.copy uses (M3-08b). What lands there is the raw buffered text, never
	// what is on screen: no styling, no inserted wrap breaks, and the timestamp
	// prefixed exactly when logs.timestamps is showing it (D242 pt 4). It is
	// meaningful only while the logs view is up; elsewhere it is inert.
	ActionLogsYank Action = "logs.yank"
	// ActionRevealSecret toggles reveal (unmask/decode) of the values inside the
	// open secret viewer (M3-08a). Values start masked; this is the deliberate
	// reveal gesture. It is meaningful only while the secret viewer is up; elsewhere
	// it is inert.
	ActionRevealSecret Action = "secret.reveal"
	// ActionCopySecret copies the selected secret entry's decoded value to the
	// system clipboard (M3-08b, OSC-52). It is meaningful only while the secret
	// viewer is up and works whether or not the value is on-screen (masked or
	// revealed) — copying is itself a deliberate gesture; elsewhere it is inert.
	ActionCopySecret Action = "secret.copy"
	// ActionForwards toggles the port-forward panel (M3-13b): a global overlay
	// listing the active background forwards with their bound local:remote ports.
	// It is app-global, not row-scoped — forwards outlive the row they started on.
	ActionForwards Action = "forwards.panel"
	// ActionStopForwards stops every active port-forward at once from the panel
	// (M3-13b). It is meaningful only while the panel is up; elsewhere it is inert
	// (the panel's per-forward stop is nav.drillIn on the selected entry).
	ActionStopForwards Action = "forwards.stopAll"
	// ActionLocalPort / ActionFreeLocalPort are the two *local side* gestures on the
	// port-forward port picker (FB-pf-local-port/D139). Confirming a port (nav.drillIn)
	// forwards it with local = remote — one keystroke, D138 — and these opt into
	// choosing the local end instead: ActionLocalPort opens a prompt seeded with the
	// remote number so it can be edited, ActionFreeLocalPort forwards straight away on
	// an OS-assigned free local port. Both are meaningful only while the port picker is
	// up; elsewhere they are inert (like logs.follow / secret.reveal outside a viewer).
	ActionLocalPort     Action = "forwards.localPort"
	ActionFreeLocalPort Action = "forwards.freeLocal"
	// ActionSearch opens the cluster-search mini-app (SEARCH-02b/D131): a
	// full-screen view whose always-open query field runs a one-shot, cross-kind
	// search over the curated kind set in the current namespace, and whose
	// nav.drillIn switches the browse view to the selected hit. It is app-global
	// (not row-scoped) and distinct from app.filter, which narrows the rows of the
	// one table already open.
	ActionSearch Action = "search.cluster"
	// ActionSearchAllKinds widens a cluster search from the curated default kind
	// set to **every discovered kind** (SEARCH-04a/D131 pt 2). It is meaningful
	// only while the search view is up — the toggle belongs to the query on
	// screen, not to the app — and elsewhere it is inert, like logs.follow outside
	// the logs view. Off on every open: the widen is the expensive scope, so it is
	// something a reader asks for per search rather than a mode they can leave on.
	ActionSearchAllKinds Action = "search.allKinds"
	// ActionSearchAllNamespaces widens a cluster search from the app's current
	// namespace to **every namespace** (SEARCH-04b/D131 pt 2). It is the other half
	// of "scope" beside search.allKinds and is an independent toggle, not a step in a
	// cycle: curated kinds across every namespace, and every kind inside one, are both
	// reachable. Like the kind widen it is meaningful only while the search view is up,
	// off on every open, and it never touches the app's own namespace scope — the
	// browse table keeps watching what it was watching.
	ActionSearchAllNamespaces Action = "search.allNamespaces"
	// ActionConfirmAccept / ActionConfirmDecline resolve the confirm modal's yes/no
	// question (default `y`/`n`, plus `enter`/`esc`). They are bound in `context
	// "confirm"` (D303): `n`/`enter`/`esc` mean app.searchNext / nav.drillIn /
	// nav.back everywhere else, and the deeper context wins while the dialog is up,
	// so the modal accepts `y`/`n` while they stay registered and rebindable (D11).
	// Supersedes the "no y/n" part of D88/D115.
	ActionConfirmAccept  Action = "confirm.accept"
	ActionConfirmDecline Action = "confirm.decline"
	// ActionPin toggles the kind you are pointing at as a **pinned** kind for the
	// current kubeconfig context (CRD-PIN-02, CRD-PIN-03): pinned, it joins that
	// context's menu from now on whether or not discovery lists it, which is what
	// makes a CRD you reached for once reachable again without hunting for it;
	// pressed again on that pin, it goes back out — one key, both directions, so the
	// way to undo it is never a text editor. It reads the kind from the surface that
	// has one — the highlighted menu row, or the kind the table is browsing — and is
	// inert where neither names a kind, or with no pin persister wired. Only a kind
	// *this* gesture pinned toggles off: a hand-written `menu` entry in config.kdl
	// declines with a notice naming the file (D193 pt 3). It is `menu.pin` rather
	// than a `pin.*` id of its own because what it changes is the menu, and because
	// the `?` overlay renders one column per action namespace — a namespace holding a
	// single action costs a column on a surface that already has sixteen (D201).
	ActionPin Action = "menu.pin"
)

// actionMeta is the registry: every known Action, in a stable order, with the
// human description used by help/doc generation. Adding an Action here (and to a
// default binding below) is all that is needed to register it.
var actionMeta = []struct {
	id   Action
	desc string
}{
	{ActionUp, "Move up"},
	{ActionDown, "Move down"},
	{ActionLeft, "Focus left pane / scroll left"},
	{ActionRight, "Focus right pane / scroll right"},
	{ActionDrillIn, "Open / drill into selection"},
	{ActionBack, "Go back / up a level"},
	{ActionTop, "Jump to top"},
	{ActionBottom, "Jump to bottom"},
	{ActionHalfPageDown, "Scroll half page down"},
	{ActionHalfPageUp, "Scroll half page up"},
	{ActionPageDown, "Scroll full page down"},
	{ActionPageUp, "Scroll full page up"},
	{ActionFilter, "Filter / search"},
	{ActionSearchNext, "Next match"},
	{ActionSearchPrev, "Previous match"},
	{ActionUnhealthy, "Unhealthy only"},
	{ActionUnhealthyScan, "Unhealthy anywhere"},
	{ActionHelp, "Toggle help"},
	{ActionQuit, "Quit"},
	{ActionPalette, "Command palette"},
	{ActionNamespace, "Switch namespace"},
	{ActionResources, "Switch resource"},
	{ActionContext, "Switch cluster context"},
	{ActionTheme, "Switch color theme"},
	{ActionToggleMouse, "Toggle mouse capture (off = select text to copy)"},
	{ActionUpdate, "Update kubecom"},
	{ActionSort, "Sort: focus the column-header row"},
	{ActionClearSort, "Clear sort (restore order)"},
	{ActionToggleMenu, "Toggle left menu pane"},
	{ActionActions, "Act on the selected row"},
	{ActionDescribe, "Describe the selected row"},
	{ActionEvents, "List the selected row's events"},
	{ActionLogs, "View logs for the selected row"},
	{ActionEdit, "View / edit the selected row's YAML in $EDITOR"},
	{ActionDelete, "Delete the selected row"},
	{ActionChildren, "Show the selected owner's pods"},
	{ActionRelations, "Show what the selected row is related to"},
	{ActionLogsFollow, "Toggle log follow (auto-scroll) in the logs viewer"},
	{ActionLogsRegex, "Toggle regex matching for the logs filter"},
	{ActionLogsWrap, "Toggle line wrapping in the logs viewer"},
	{ActionLogsTimestamps, "Toggle timestamps in the logs viewer"},
	{ActionLogsPrevious, "Toggle logs of the previous (crashed) container instance"},
	{ActionLogsSelect, "Select log lines (visual mode)"},
	{ActionLogsYank, "Copy the selected log lines"},
	{ActionRevealSecret, "Reveal / hide secret values in the secret viewer"},
	{ActionCopySecret, "Copy the selected secret value to the clipboard"},
	{ActionForwards, "Toggle the port-forward panel"},
	{ActionStopForwards, "Stop all port-forwards (in the panel)"},
	{ActionLocalPort, "Set the local port for the highlighted port (port picker)"},
	{ActionFreeLocalPort, "Forward the highlighted port on a free local port (port picker)"},
	{ActionSearch, "Search the cluster across kinds"},
	{ActionSearchAllKinds, "Toggle searching all kinds (cluster search)"},
	{ActionSearchAllNamespaces, "Toggle searching all namespaces (cluster search)"},
	{ActionConfirmAccept, "Accept the confirm dialog"},
	{ActionConfirmDecline, "Decline the confirm dialog"},
	{ActionPin, "Pin/unpin the kind for this context"},
}

var registered = func() map[Action]string {
	m := make(map[Action]string, len(actionMeta))
	for _, a := range actionMeta {
		m[a.id] = a.desc
	}
	return m
}()

// Valid reports whether a is a registered action id.
func (a Action) Valid() bool { _, ok := registered[a]; return ok }

// Describe returns the human description for a registered action ("" if unknown).
func (a Action) Describe() string { return registered[a] }

// Actions returns every registered action id in registry order.
func Actions() []Action {
	out := make([]Action, len(actionMeta))
	for i, a := range actionMeta {
		out[i] = a.id
	}
	return out
}

// paletteOnlyActions are registered actions with no default key: reached by
// typing them in the command palette rather than by a binding. Updating is the
// first — it is rare and it hands the terminal over to fetch and swap the
// binary, so a bare key would invite an accidental network fetch; `:update`
// names it deliberately.
var paletteOnlyActions = map[Action]struct{}{
	ActionUpdate: {},
}

// PaletteOnly reports whether a is deliberately unbound by default.
func PaletteOnly(a Action) bool { _, ok := paletteOnlyActions[a]; return ok }
