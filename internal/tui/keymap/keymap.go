// Package keymap is kubecom's configurable action registry: the single place
// where keys exist (D11). Every user-triggerable behaviour is a named Action; a
// default keymap (vim-first, D10) maps each Action to one or more canonical
// chords; the user's config is merged onto those defaults; and views resolve a
// live keypress to an Action instead of matching raw keys. Help overlays and the
// generated keybindings doc are built from this registry so they cannot drift.
//
// This file is the registry core: Action ids, the default keymap, Merge +
// validation, and single-key KeyMsg → Action resolution. The chord model lives
// in chord.go; multi-key sequences (`gg`) and their stateful, timeout-driven
// resolution live in sequence.go/sequencer.go (M2-01b). Deferred to later slices:
// the YAML keys: config wiring (M2-01c) and bubbles/key.Binding + help generation
// (M2-01d).
package keymap

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

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
	// D171) and is written back to config.yaml so the next launch opens on it. It is
	// app-global like ctx.switch and ns.switch, and — unlike them — never inert: the
	// themes it offers are compiled in, so it needs no cluster and no seam. Only the
	// write-back needs one, and a missing persister costs the choice its persistence,
	// not the gesture.
	ActionTheme       Action = "theme.switch"
	ActionToggleMouse Action = "mouse.toggle"
	ActionSort        Action = "sort.column"
	ActionClearSort   Action = "sort.clear"
	ActionToggleMenu  Action = "menu.toggle"
	// M3 row actions (operate on the selected resource row). ActionActions is the
	// way to *all* of them (D107); the rest are direct-key shortcuts for the
	// most-used ones, all off the reserved nav keys (D10). The full curated action
	// set (scale, cordon, drain, suspend, exec, …) is reachable through it rather
	// than through a key of its own (see vault/knowledge/keybindings.md). Since
	// PAL-05d it is an *argument* verb like ns.switch and resources.switch: it takes
	// the action as its argument and opens the palette's `:action ` stage rather than
	// a menu of its own (D210), so its id keeps the `.menu` suffix for compatibility
	// with existing keymap config while the surface it names is the palette's.
	ActionActions  Action = "actions.menu"
	ActionDescribe Action = "res.describe"
	ActionLogs     Action = "res.logs"
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
	// question (default `y`/`n`, plus `enter`/`esc`). They live in the dedicated
	// **confirm key context** (contextOf), not the browse context: `n`/`enter`/`esc`
	// already mean app.searchNext / nav.drillIn / nav.back in the browse view, so a
	// flat keymap could not bind them twice. (`y` is unbound in browse since res.yaml
	// was retired, D135/M3-15c — but the context split stays for `n`/`enter`/`esc`.)
	// Resolving them in a separate context (ConfirmAction) lets the modal accept
	// `y`/`n` while keeping them registered and rebindable (D11). Supersedes the
	// "no y/n" part of D88/D115.
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
	// *this* gesture pinned toggles off: a hand-written `menus/<context>.yaml` entry
	// declines with a notice naming the file (D193 pt 3). It is `menu.pin` rather
	// than a `pin.*` id of its own because what it changes is the menu, and because
	// the `?` overlay renders one column per action namespace — a namespace holding a
	// single action costs a column on a surface that already has sixteen (D201).
	ActionPin Action = "menu.pin"
)

// keyContext scopes key resolution: a chord means different actions in different
// contexts, and collisions are checked per-context (keybindings.md: "two actions
// bound to the same key **in the same context**"). Today there are two: the browse
// context (the sequencer + Action) and the confirm-modal context (ConfirmAction).
type keyContext int

const (
	ctxBrowse keyContext = iota
	ctxConfirm
)

// confirmContextActions is the set of actions resolved in the confirm-modal
// context rather than the browse context. contextOf routes every other action to
// the browse context.
var confirmContextActions = map[Action]struct{}{
	ActionConfirmAccept:  {},
	ActionConfirmDecline: {},
}

// contextOf returns the key context an action's bindings live in. Confirm
// accept/decline resolve only while the confirm modal is up (ConfirmAction); every
// other action resolves in the browse context (the sequencer + Action).
func contextOf(a Action) keyContext {
	if _, ok := confirmContextActions[a]; ok {
		return ctxConfirm
	}
	return ctxBrowse
}

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
	{ActionHelp, "Toggle help"},
	{ActionQuit, "Quit"},
	{ActionPalette, "Command palette"},
	{ActionNamespace, "Switch namespace"},
	{ActionResources, "Switch resource"},
	{ActionContext, "Switch cluster context"},
	{ActionTheme, "Switch color theme"},
	{ActionToggleMouse, "Toggle mouse capture (off = select text to copy)"},
	{ActionSort, "Sort table (cycle column / direction)"},
	{ActionClearSort, "Clear sort (restore order)"},
	{ActionToggleMenu, "Toggle left menu pane"},
	{ActionActions, "Act on the selected row"},
	{ActionDescribe, "Describe the selected row"},
	{ActionLogs, "View logs for the selected row"},
	{ActionEdit, "View / edit the selected row's YAML in $EDITOR"},
	{ActionDelete, "Delete the selected row"},
	{ActionChildren, "Show the selected owner's pods"},
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

// defaultBindings is the vim-first default keymap (D10). Tokens are the
// human-readable syntax of keybindings.md and may be multi-key sequences (`gg`).
// Defaults are collision-free by construction (asserted in tests): pgdn/pgup fall
// back to the half-page actions; the full-page actions keep the ctrl+f/ctrl+b vim
// keys only (pgdn/pgup can't fall back to both half- and full-page in one flat
// context without colliding — D48). `gg` → top is the vim sequence (M2-01b), with
// `home` as the single-key fallback.
var defaultBindings = map[Action][]string{
	ActionUp:           {"k", "up"},
	ActionDown:         {"j", "down"},
	ActionLeft:         {"h", "left"},
	ActionRight:        {"l", "right"},
	ActionDrillIn:      {"enter"},
	ActionBack:         {"esc"},
	ActionTop:          {"gg", "home"},
	ActionBottom:       {"G", "end"},
	ActionHalfPageDown: {"ctrl+d", "pgdn"},
	ActionHalfPageUp:   {"ctrl+u", "pgup"},
	ActionPageDown:     {"ctrl+f"},
	ActionPageUp:       {"ctrl+b"},
	ActionFilter:       {"/"},
	ActionSearchNext:   {"n"},
	ActionSearchPrev:   {"N"},
	ActionHelp:         {"?"},
	ActionQuit:         {"q", "ctrl+c"},
	ActionNamespace:    {"ctrl+n"},
	// `:` is the palette's key (D194 pt 2), not the resource picker's: the palette is
	// the surface that answers "what do I want to do", and the resource switch is one
	// verb inside it. So resources.switch hands `:` over and takes `R` — the mnemonic
	// **R**esource, free in the browse context, not a reserved nav chord (D10), and it
	// joins the capital-letter app-global family (`C` context, `T` theme, `M` mouse,
	// `F` forwards, `P` pods). Lowercase `r` is secret.reveal. It keeps a direct key
	// rather than becoming palette-only because every registered action has one
	// (TestDefaultKeymapValid) and because PAL-05, not this slice, is where the
	// shortcut keys are reconsidered (D194 pt 4).
	ActionPalette:   {":"},
	ActionResources: {"R"},
	// The context switcher does *not* join the ctrl+<letter> family its sibling
	// ns.switch belongs to, and the difference is not cosmetic: that family exists
	// for gestures that must survive an always-open text field (search.cluster and
	// both its widens, D140 pt 1). The context picker has no such field — like the
	// namespace picker its filter is opt-in (`/`) — so a plain key is safe, and the
	// free ctrl+<letter> keys are better spent on the surfaces that need them.
	// `C` is the mnemonic **C**ontext, free in the browse context, not a reserved
	// nav chord (D10), and sits with the other capital-letter app-global gestures
	// (`M` mouse, `F` forwards, `X` stop-all). Lowercase `c` is secret.copy.
	ActionContext: {"C"},
	// The theme switcher takes `T` for the same reasons the context switcher takes
	// `C`: its picker has no always-open text field (its filter is opt-in, `/`), so
	// it needs none of the ctrl+<letter> keys the search surfaces do; `T` is the
	// mnemonic **T**heme, free in the browse context, not a reserved nav chord (D10),
	// and it joins the capital-letter app-global family (`C` context, `M` mouse,
	// `F` forwards, `X` stop-all, `P` pods). Lowercase `t` is logs.timestamps.
	ActionTheme:       {"T"},
	ActionToggleMouse: {"M"},
	ActionSort:        {"s"},
	ActionClearSort:   {"S"},
	ActionToggleMenu:  {"m"},
	ActionActions:     {"a"},
	ActionDescribe:    {"D"},
	ActionLogs:        {"L"},
	// ActionEdit keeps `e` (edit); the retired res.yaml (`y`) is left unbound in the
	// browse context (D135/M3-15c) — one object-YAML action on one key (D133 pinned
	// delete=`d`/describe=`D`; `y` stays free for a future rebind or user config).
	ActionEdit:   {"e"},
	ActionDelete: {"d"},
	// The children drill-down takes `P` — the mnemonic **P**ods, since pods are the
	// one child kind kubecom drills into (D165). Lowercase `p` is the port picker's
	// local-port prompt, so the capital keeps it with the other capital-letter
	// gestures (`D` describe, `L` logs, `C` context) and stays clear of the reserved
	// nav chords (D10).
	ActionChildren:   {"P"},
	ActionLogsFollow: {"f"},
	// The regex toggle joins the ctrl+<letter> family for the reason given at its
	// declaration: it has to keep working with the grep field open, and only a key
	// carrying no text survives that field. ctrl+r is free in the browse context.
	ActionLogsRegex: {"ctrl+r"},
	// The wrap toggle is an ordinary letter, unlike the regex toggle beside it: it
	// changes how lines are *laid out*, not how the grep field is read, so there is no
	// reason to reach for it mid-query — and with the grep open `w` types a `w` like
	// every other letter (D140 pt 1). `w` is free in the browse context.
	ActionLogsWrap: {"w"},
	// The timestamps toggle is a plain letter for the same reason as the wrap toggle
	// beside it: it changes how lines are *drawn*, not how the grep reads them, so
	// there is no need to reach for it mid-query — and with the grep open `t` types a
	// `t` like every other letter (D140 pt 1). `t` is free in the browse context.
	ActionLogsTimestamps: {"t"},
	// The previous-instance toggle takes a plain letter like the other logs display
	// toggles (feedback `2026-08-09-logs-view-palette-bindings`: "Ctrl+P is a bit
	// weird"), `o` reading as the **o**ld/**o**ther instance — the mnemonic `p` is
	// the port picker's local-port prompt (FB-pf-local-port/D139) and `P` is
	// res.children (D165), so the letter this gesture wants is spent twice over, and
	// `o` is its QWERTY neighbour, free in the browse context and not a reserved nav
	// chord (D10). ctrl+p stays as the second binding rather than retiring: it is
	// the one form of the gesture that survives an open grep field (D140 pt 1) —
	// mid-query, when the stack trace is not in the running instance's tail, `o`
	// would type into the field — and the mid-query flip was the reason the chord
	// existed (D177). Discoverability no longer depends on either key: the palette
	// opens over the logs view and lists this verb (LOGS-09/D258).
	ActionLogsPrevious: {"o", "ctrl+p"},
	// Visual mode and yank take vim's own keys, and both are plain letters for the
	// reason the wrap and timestamps toggles are: neither is a gesture anyone reaches
	// for mid-query — you select lines you can already see — so being swallowed by an
	// open grep field (D140 pt 1) costs them nothing. `v` is free in the browse
	// context and is not a reserved nav chord (D10).
	ActionLogsSelect: {"v"},
	// `y` is the one binding here with a twin: it is also confirm.accept. That is
	// legal rather than lucky — the two resolve in different key contexts (contextOf),
	// and they are modal besides: the confirm modal captures every key while it is up,
	// so the logs view cannot be reading `y` at the same moment the modal is. It was
	// left browse-free when res.yaml retired (D135/M3-15c) and this is what claims it.
	ActionLogsYank:     {"y"},
	ActionRevealSecret: {"r"},
	ActionCopySecret:   {"c"},
	ActionForwards:     {"F"},
	ActionStopForwards: {"X"},
	// Port-picker local-port gestures (FB-pf-local-port): `p` for the local **p**ort
	// prompt, `0` for "let the OS pick one" — the port-0 convention, though the spec
	// kubectl/client-go actually accepts is the leading-colon form `:<remote>` (`:0`
	// is rejected: remote port must be > 0), which the shell builds itself (D139).
	ActionLocalPort:     {"p"},
	ActionFreeLocalPort: {"0"},
	// Cluster search joins the ctrl+<letter> family of app-global switchers
	// (ctrl+n = ns.switch): a mnemonic **s**earch key that is not a plain letter, so
	// it cannot be swallowed by the always-open query field it opens (every
	// text-carrying key types into that field, D140 pt 1). Raw mode clears the
	// terminal's IXON flow control, so ctrl+s reaches the app rather than pausing it.
	ActionSearch: {"ctrl+s"},
	// The all-kinds widen has to be a no-text chord for the same reason
	// search.cluster is: it acts *inside* the search view, whose query field is
	// always open and swallows every text-carrying key (D140 pt 1) — a plain `a`
	// would type an `a`. ctrl+a is free in the browse context and reads as "all".
	// It costs the query field its readline start-of-line binding, which the field
	// had already lost to nav.top's `home`; a search query is one short line.
	ActionSearchAllKinds: {"ctrl+a"},
	// The namespace widen is a no-text chord for the same reason the kind widen beside
	// it is: it acts inside the always-open query field. `ctrl+a` was the mnemonic
	// ("all") and is spent, and `ctrl+n` — the obvious second choice — is ns.switch in
	// the flat browse context, so `ctrl+w` takes the *cluster-**w**ide* reading, which
	// is how Kubernetes itself names "not scoped to a namespace". It costs the query
	// field readline's delete-previous-word, the same kind of price ctrl+a paid for
	// start-of-line: a search query is one short line, and backspace still works.
	ActionSearchAllNamespaces: {"ctrl+w"},
	// Confirm-context bindings (contextOf → ctxConfirm): `y`/`n` are the yes/no
	// muscle memory, `enter`/`esc` the modal convention. `n`/`enter`/`esc` also bind
	// in the browse context (app.searchNext/nav.drillIn/nav.back); `y` is browse-free
	// since res.yaml retired (D135/M3-15c) — legal either way because they resolve in
	// a different context (build partitions them).
	ActionConfirmAccept:  {"y", "enter"},
	ActionConfirmDecline: {"n", "esc"},
	// The pin gesture cannot have the letter it wants: `p` is the port picker's
	// local-port prompt (D139) and `P` is res.children (D165), and the browse context
	// is flat — one action per key — so the mnemonic is spent twice over, exactly as
	// it was when logs.previous had to take ctrl+p (D177). `*` takes the *starred /
	// favourite* reading instead of a mnemonic one, which is what a pin is: it is
	// free in the browse context, is not a reserved nav chord (D10), and is a symbol
	// rather than a letter, so it stays clear of the letters the row actions keep
	// claiming. It carries text, so it would type into an always-open query field —
	// harmless here, since the surfaces this action reads (the menu, the browse
	// table) have no such field.
	ActionPin: {"*"},
}

// navChords is the set of reserved navigation chords (D10): binding an app
// action over one of these is allowed but warned about during Merge.
var navChords = map[chord]struct{}{
	"h": {}, "j": {}, "k": {}, "l": {},
	"g": {}, "G": {}, "n": {}, "N": {}, "/": {},
	"ctrl+u": {}, "ctrl+d": {}, "ctrl+f": {}, "ctrl+b": {},
}

// Keymap is a resolved, validated mapping of actions to key sequences. Construct
// with DefaultKeymap (optionally then Merge). The reverse indexes are what
// resolution uses; they are rebuilt on every construction so they never drift
// from bindings. bySeq maps a whole sequence to its action; prefix/extends
// support the sequencer (is this a prefix of a binding; does a longer binding
// extend it).
type Keymap struct {
	bindings     map[Action][]seq
	bySeq        map[string]Action // browse-context single/sequence resolution
	confirmBySeq map[string]Action // confirm-modal-context resolution (ConfirmAction)
	prefix       map[string]bool
	extends      map[string]bool
}

// DefaultKeymap returns the built-in vim-first keymap. It panics if the static
// default table is malformed — a programming error caught by tests, never a
// runtime condition.
func DefaultKeymap() *Keymap {
	bindings := make(map[Action][]seq, len(defaultBindings))
	for _, a := range actionMeta { // deterministic order
		for _, tok := range defaultBindings[a.id] {
			s, err := parseSequence(tok)
			if err != nil {
				panic(fmt.Sprintf("keymap: bad default binding %q for %s: %v", tok, a.id, err))
			}
			bindings[a.id] = append(bindings[a.id], s)
		}
	}
	km, err := build(bindings)
	if err != nil {
		panic("keymap: default keymap is invalid: " + err.Error())
	}
	return km
}

// Merge overlays user overrides onto this keymap and returns a new validated
// keymap. Each entry replaces that action's binding wholesale (an empty list
// disables the action). It returns an error for an unknown action id, a bad key
// token, or a chord bound to two actions; the returned warnings flag overrides
// that shadow a default navigation key (D10).
func (k *Keymap) Merge(overrides map[Action][]string) (*Keymap, []string, error) {
	next := make(map[Action][]seq, len(k.bindings))
	for a, ss := range k.bindings {
		next[a] = append([]seq(nil), ss...)
	}
	// Deterministic iteration over overrides for stable errors/warnings.
	ids := make([]Action, 0, len(overrides))
	for a := range overrides {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var warnings []string
	for _, a := range ids {
		if !a.Valid() {
			return nil, nil, fmt.Errorf("keymap: unknown action %q", a)
		}
		seqs := make([]seq, 0, len(overrides[a]))
		for _, tok := range overrides[a] {
			s, err := parseSequence(tok)
			if err != nil {
				return nil, nil, fmt.Errorf("keymap: action %q: %w", a, err)
			}
			// A single-key override onto a reserved nav key shadows navigation
			// (D10) — allowed, but warned. A multi-key sequence like `gg` does
			// not shadow the bare nav key `g`, so only length-1 seqs warn.
			if len(s) == 1 {
				if _, isNav := navChords[s[0]]; isNav && !isNavAction(a) {
					warnings = append(warnings, fmt.Sprintf(
						"action %q binds navigation key %q", a, s.String()))
				}
			}
			seqs = append(seqs, s)
		}
		next[a] = seqs
	}
	km, err := build(next)
	if err != nil {
		return nil, nil, err
	}
	return km, warnings, nil
}

// build validates bindings and constructs the reverse indexes. Two actions on
// the same sequence is a collision error naming both. Alongside the exact index
// it records, for every proper/whole prefix of every binding, whether that prefix
// is reachable (prefix) and whether a longer binding extends it (extends) — the
// two facts the sequencer needs.
func build(bindings map[Action][]seq) (*Keymap, error) {
	bySeq := make(map[string]Action)
	confirmBySeq := make(map[string]Action)
	prefix := make(map[string]bool)
	extends := make(map[string]bool)
	// Deterministic order so a collision reports the same pair every run.
	ids := make([]Action, 0, len(bindings))
	for a := range bindings {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, a := range ids {
		// Each action's chords resolve in exactly one context; a collision is only a
		// collision within that context (keybindings.md). The confirm context has no
		// multi-key sequences, so only the browse context feeds prefix/extends (the
		// sequencer). Both indexes are keyed by chord, so the same key can map to a
		// browse action and a confirm action without clashing.
		idx := bySeq
		browse := true
		if contextOf(a) == ctxConfirm {
			idx, browse = confirmBySeq, false
		}
		for _, s := range bindings[a] {
			key := s.key()
			if other, ok := idx[key]; ok && other != a {
				lo, hi := other, a
				if hi < lo {
					lo, hi = hi, lo
				}
				return nil, fmt.Errorf("keymap: key %q is bound to both %q and %q", s.String(), lo, hi)
			}
			idx[key] = a
			if !browse {
				continue
			}
			for i := 1; i <= len(s); i++ {
				prefix[s[:i].key()] = true
				if i < len(s) {
					extends[s[:i].key()] = true
				}
			}
		}
	}
	return &Keymap{bindings: bindings, bySeq: bySeq, confirmBySeq: confirmBySeq, prefix: prefix, extends: extends}, nil
}

// exact returns the action a whole sequence is bound to.
func (k *Keymap) exact(s seq) (Action, bool) { a, ok := k.bySeq[s.key()]; return a, ok }

// isPrefix reports whether s is a prefix of (or equal to) some binding.
func (k *Keymap) isPrefix(s seq) bool { return k.prefix[s.key()] }

// hasExtension reports whether some binding strictly extends s.
func (k *Keymap) hasExtension(s seq) bool { return k.extends[s.key()] }

// Action resolves a single live keypress to a binding whose whole sequence is
// that one key. It ignores multi-key sequences (use a Sequencer for those); the
// bool is false when no single-key binding matches. Views that never buffer
// (modals, pickers) can use this directly.
func (k *Keymap) Action(key tea.Key) (Action, bool) {
	a, ok := k.bySeq[seq{chordFromKey(key)}.key()]
	return a, ok
}

// ConfirmAction resolves a single live keypress in the confirm-modal context: the
// confirm-only bindings (default `y`/`enter` → confirm.accept, `n`/`esc` →
// confirm.decline). It is separate from Action so the modal can accept `y`/`n`
// without `n`/`enter`/`esc` losing their browse-context meaning (searchNext /
// drillIn / back). The confirm context has only single-key bindings, so no
// sequencer is needed.
func (k *Keymap) ConfirmAction(key tea.Key) (Action, bool) {
	a, ok := k.confirmBySeq[seq{chordFromKey(key)}.key()]
	return a, ok
}

// Resolve reports the action a whole run of key tokens is bound to, or ok=false
// when the run matches no binding. Tokens are in the keybindings.md / `--keylog`
// trace notation ("j", "ctrl+s", "gg"). It is the read side of the sequencer:
// the trace analyser (STORY-03) asks whether a run of keys the walker actually
// pressed forms a complete binding, which is how it tells a finished chord
// (`g g` → nav.top) from an abandoned one (`g` then `j`). A token that cannot be
// parsed simply matches nothing, like a key that fits no binding.
func (k *Keymap) Resolve(tokens ...string) (Action, bool) {
	s := make(seq, 0, len(tokens))
	for _, tok := range tokens {
		c, err := parseChord(tok)
		if err != nil {
			return "", false
		}
		s = append(s, c)
	}
	return k.exact(s)
}

// Keys returns the canonical key tokens bound to an action, for help/doc
// generation. Order follows the action's binding list (vim key first).
func (k *Keymap) Keys(a Action) []string {
	ss := k.bindings[a]
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.String()
	}
	return out
}

func isNavAction(a Action) bool { return strings.HasPrefix(string(a), "nav.") }
