package keymap

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// This file bridges the action registry to bubbles' key.Binding / help.KeyMap so
// help is generated from the resolved keymap and can never drift from actual
// bindings (D11). The Binding's display text and keys come straight from Keys()
// and Describe(); nothing here restates a literal key.

// Binding builds a bubbles key.Binding for a single action from the keymap's
// resolved bindings: WithKeys carries the canonical tokens (vim key first),
// WithHelp carries the display text ("j/up") and the registry description. An
// action with no bound keys (disabled, or unknown) yields a disabled binding so
// help renderers skip it via Enabled().
func (k *Keymap) Binding(a Action) key.Binding {
	keys := k.Keys(a)
	opts := []key.BindingOpt{
		key.WithKeys(keys...),
		key.WithHelp(strings.Join(keys, "/"), a.Describe()),
	}
	if len(keys) == 0 {
		opts = append(opts, key.WithDisabled())
	}
	return key.NewBinding(opts...)
}

// Bindings returns a key.Binding for every registered action, in registry order.
func (k *Keymap) Bindings() []key.Binding {
	acts := Actions()
	out := make([]key.Binding, len(acts))
	for i, a := range acts {
		out[i] = k.Binding(a)
	}
	return out
}

// shortHelpActions is the curated one-line status-bar subset (help.KeyMap's
// ShortHelp): the essentials a user needs at a glance. Ordered as shown. This is
// the focus-agnostic set used where there is no single focused pane (the startup
// welcome page); the browse status bar uses the focus-aware sets below.
var shortHelpActions = []Action{
	ActionDown, ActionUp, ActionDrillIn, ActionBack, ActionFilter, ActionHelp, ActionQuit,
}

// HelpContext selects which curated key-hint subset the persistent bottom hint
// shows, so the hint tracks what currently holds focus (the dogfood ask: "the
// most relevant keys for the current context, updating with focus"). It names a
// focus context, never a key — the concrete keys still come from the registry
// (D11), so a future focus context is added here, not by hard-coding keys in a
// view.
type HelpContext int

const (
	// HelpMenu is the left resource-menu focus: navigate the list, open a
	// selection, switch namespace.
	HelpMenu HelpContext = iota
	// HelpTable is the right resource-table focus: navigate rows, filter/search
	// them, and step back out to the menu.
	HelpTable
	// HelpSearch is the cluster-search mini-app (SEARCH-02b), which replaces the
	// browse body and captures every keypress while it is up: navigate the streamed
	// results, open one, or clear-then-close the query.
	HelpSearch
	// HelpUnhealthy is the cross-kind unhealthy list (STORY-06g-2), which like the
	// search mini-app replaces the browse body and captures every keypress while it
	// is up: navigate the streamed hits, open one, or close the view. It is a pure
	// list with no query field — every mapped key is a navigation action, so the
	// hint is the pager set (D276).
	HelpUnhealthy
	// HelpLogs is the dedicated logs mini-app (LOGS-02) with its live grep closed:
	// scroll the stream, open the grep, toggle follow, close the view.
	HelpLogs
	// HelpLogsFilter is the same logs mini-app with its live grep *open*, which
	// captures text — so it advertises only the keys that still act there.
	HelpLogsFilter
	// HelpPickerFilter is any modal picker with its filter field open. The overlay
	// captures all input and the open field takes every text-producing key, so only
	// the no-text keys act: move, select, cancel. Since STORY-06d this is the
	// *momentary* state — the field the reader opened with `/`, and the palette's
	// verb list on show (D197) — rather than every picker from the moment it is
	// shown (D272).
	HelpPickerFilter
	// HelpPicker is a modal picker with its filter field *closed* — the default open
	// state of every value picker since STORY-06d, where j/k move the list and `/`
	// opens the field (D272).
	HelpPicker
	// HelpConfirm is the yes/no confirm modal (M3-09/D132), which captures all input:
	// its two answers resolve in the confirm key context, and everything else is
	// swallowed so the panes underneath never move.
	HelpConfirm
	// HelpPrompt is the same modal in prompt mode, whose text field takes every
	// text-producing key — so only the no-text control keys act: submit and cancel.
	HelpPrompt
	// HelpKeybindings is the `?` keybindings overlay, which swallows navigation while
	// it is up: the only keys that act are the ways back out of it.
	HelpKeybindings
	// HelpViewer is the shared read-only viewer (YAML/describe/secret, M3-03): it
	// scrolls on navigation and closes on back/quit, swallowing everything else.
	HelpViewer
	// HelpTableFilter is the browse filter field while it is open (`/`), which since
	// STORY-06m narrows whichever pane holds focus — the table's rows or the menu's
	// kinds. It captures text exactly as the logs grep and the picker filters do — so
	// only the no-text keys act: move through the live-narrowed list, commit, clear-and-close.
	HelpTableFilter
	// HelpForwards is the port-forward panel (M3-13b), a global overlay that captures
	// input: move the cursor, stop the selected forward or all of them, close.
	HelpForwards
	// HelpSort is the table's column-header sort mode (STORY-06b), entered with `S`:
	// the header row holds the cursor and the mode captures input while it is up, so
	// only the keys that move/pick it act — h/l across the columns, enter to toggle
	// the direction, x to clear, esc/S to leave.
	HelpSort

	// helpContextCount bounds the enum — it is always one past the last real context,
	// which is what makes the set enumerable and therefore checkable (HINT-05, closing
	// D218 pt 1). Without it a new capturing surface's context is a bare constant with
	// no entry in contextShortHelpActions, and ShortHelpContext silently hands back the
	// *browse* set for a surface where none of those keys act — the exact class of lie
	// the HINT line exists to remove, with every test still green. Declare new contexts
	// **above** this line; a constant added below it is invisible to both checks.
	helpContextCount
)

// helpContextNames names each context for test failures and diagnostics, indexed by the
// context itself. It is length-checked against helpContextCount, so a context added
// without a name here is caught by the same test that catches a missing hint set.
var helpContextNames = [helpContextCount]string{
	HelpMenu:         "HelpMenu",
	HelpTable:        "HelpTable",
	HelpSearch:       "HelpSearch",
	HelpUnhealthy:    "HelpUnhealthy",
	HelpLogs:         "HelpLogs",
	HelpLogsFilter:   "HelpLogsFilter",
	HelpPickerFilter: "HelpPickerFilter",
	HelpPicker:       "HelpPicker",
	HelpConfirm:      "HelpConfirm",
	HelpPrompt:       "HelpPrompt",
	HelpKeybindings:  "HelpKeybindings",
	HelpViewer:       "HelpViewer",
	HelpTableFilter:  "HelpTableFilter",
	HelpForwards:     "HelpForwards",
	HelpSort:         "HelpSort",
}

// String names the context, so a failure reads "HelpForwards has no hint set" rather
// than naming an integer nobody can place. An out-of-range value prints its number
// rather than panicking — ShortHelpContext degrades on one too.
func (c HelpContext) String() string {
	if c < 0 || c >= helpContextCount || helpContextNames[c] == "" {
		return "HelpContext(" + strconv.Itoa(int(c)) + ")"
	}
	return helpContextNames[c]
}

// HelpContexts returns every declared focus context in declaration order. It exists so
// the completeness of the two sides of the hint — the set a context offers, and the
// state that selects it — can be asserted over the whole enum rather than over whichever
// contexts a test author remembered (HINT-05).
func HelpContexts() []HelpContext {
	out := make([]HelpContext, 0, helpContextCount)
	for c := HelpContext(0); c < helpContextCount; c++ {
		out = append(out, c)
	}
	return out
}

// contextShortHelpActions is the curated hint subset per focus context. Each set
// is ordered as shown and rendered enabled-only. Filter/search/sort appear in the
// table context (they act on a resource table, no-ops on the menu) — since
// STORY-06m `/` also acts on the menu pane (it narrows the resource kinds there),
// so filter is hinted in the menu context too, right after drill-in. Drill-in
// appears only in the menu context (opening the selected resource);
// namespace, help and quit are always-relevant and shown in both browse contexts.
//
// The pin toggle (CRD-PIN-03) is hinted in the menu context only, directly after
// drill-in — the menu is the surface it changes, and it is the one place a reader
// can see what the key did. It earns the line by the D143 pt 1 test the logs display
// toggles fail: nothing announces it. A menu row does not say it is pinned, and a
// reader on a CRD-heavy cluster has no way to learn that the kind under the cursor
// can be kept except by finding `*` in `?`. It is *not* added to the table context,
// which already carries ten entries and elides first — the gesture works there, and
// the hint is one focus-switch away.
//
// The search context is deliberately the short one. Its query field is always open
// (D140 pt 1), so the root routes every text-producing key into it: `/` `n` `s` `a`
// `?` and `q` all type a character there instead of firing their browse action, and a
// hint that advertised them would be a lie. What is left is the genuinely available
// set — move the result cursor, open a hit, widen either scope, clear-then-close —
// every one of them a no-text key the view actually consumes.
//
// Both scope widens (SEARCH-04a/04b) are hinted here, unlike the logs view's display
// toggles, because neither announces the thing a reader needs to know. The header names
// a scope, not the fact that it can be widened, so someone staring at "no matches" has
// no other way to learn that the search was curated, or that it stopped at the current
// namespace. They are hinted as a pair for the same reason they are two independent
// toggles: advertising one would imply the other axis is fixed. This is also the app's
// shortest hint line, so it is the one context with room to say so.
//
// The logs mini-app needs *two* contexts for the same reason (D143 pt 1). With its grep
// closed it honours every key it advertises, `q` included (quit closes the view, as it
// does in any pager). With the grep open the field captures text, so `/` `f` and `q`
// type a character — those drop out, leaving the no-text keys that still act: scroll,
// esc to clear the grep, and the regex toggle (LOGS-03), which is bound to a no-text
// chord precisely so it survives the open field and so is hinted in *both* logs
// contexts. Neither context offers help: the view swallows it.
//
// The wrap toggle (LOGS-04a) is hinted with the grep closed only — it is a plain letter,
// so the open field eats it exactly as it eats `f`. Its companion, horizontal scrolling
// on nav.left/nav.right, is deliberately *not* hinted in either: it only acts while the
// view is not wrapping, and a hint that is right half the time is the kind of promise
// D143 pt 1 forbids. It stays discoverable through `?` and the generated keybindings doc.
//
// Jump-to-latest (LOGS-04c) is not hinted either, for the plainer reason that the hint
// line is full: `G` is the ordinary nav.bottom key and it self-announces — the header
// flips to `[following]` the moment it is pressed. Follow, already hinted, remains the
// advertised way back to the tail. Its registry description stays the plain "Jump to
// bottom" too: a description is *global*, and lengthening it widens that column in the
// `?` overlay until the next column no longer fits (D147 pt 3).
//
// The timestamps toggle (LOGS-04b) is not hinted for the same reason, applied
// deliberately rather than by omission: with six entries already eliding at 220 columns
// (LOGS-04a), the closed-grep context is the scarcest hint line in the app, and a
// per-session display toggle is worth less of it than the grep, follow or wrap. Like
// them it self-announces — pressing it puts a timestamp on every row — so `?` and the
// generated doc carry it and the hint line stays about the keys a reader needs to be
// told about.
//
// The two picker contexts (HINT-01) are the same rule applied to an *overlay* rather
// than a full-screen view: a modal picker captures every keypress while it is up, so
// the browse hints underneath it advertised ten keys of which two acted. Since
// STORY-06d a picker opens in **navigation mode** — list focused, filter closed — so
// the honest set is move/select/cancel plus `/`, which opens the field. Only the
// field-open state (the palette's type-to-filter verb list, D197, or a `/` the reader
// pressed) drops to the four no-text keys the root actually routes: move the cursor,
// confirm, cancel. Paging (ctrl+d/u) acts too and is left out as it is in every other
// context: the hint is the keys a reader must be told about, not an inventory.
//
// HelpPickerFilter is the field-open state, and HelpPicker the navigation-mode
// default every value picker opens in (D272). The port picker's own two gestures
// (`p` local port, `0` free port) are *not* hinted in either even though they act: a
// HelpContext names an input state, not a picker kind, and this set is shared by
// every picker that does not bind them. They stay in `?` and in the port picker's own
// title.
//
// The four HINT-02 contexts are the rest of the capturing surfaces, each one the same
// rule as the pickers: what the surface's own router honours, nothing else.
//
// The confirm modal is the one set that cannot be read off the browse registry, because
// its answers live in the **confirm key context** (D132): `y`/`enter` and `n`/`esc`
// resolve through ConfirmAction, which is why `n` can mean "decline" here and
// app.searchNext everywhere else. It needs no special handling all the same — `bindings`
// is keyed by action regardless of context, so `Binding(ActionConfirmAccept)` renders the
// user's own accept keys and the hint stays registry-generated (D11). `q` also dismisses
// (handleModalAction), and is left out for the reason paging is: the decline entry
// already names the way out, and a two-entry line that reads "y accept · n decline"
// mirrors the question in the box.
//
// The prompt modal keeps the *browse* drill-in/back pair instead, and that difference is
// the honest one: an open text field takes every text-producing key, so `y`/`n` type a
// character there and only the no-text control keys — enter to submit, esc to cancel —
// still act. It is the picker-filter rule (D206 pt 3) applied to the other text field.
//
// The keybindings overlay advertises only the ways out. It swallows navigation while it
// is up and does not scroll (it renders the whole grouped table at once), so back, the
// `?` that toggles it, and quit — which dismisses the overlay rather than exiting, as it
// does over any overlay — are the complete set of keys that act. The overlay's own body
// lists every binding in the app, so the line underneath has nothing else to add.
//
// The shared viewer is a pager: it scrolls on nav.up/down and closes on back or quit,
// exactly like the logs view with its grep closed, and is hinted to match. The
// viewer-kind gestures — secret.reveal and secret.copy, which act only while the Secret
// viewer is up (M3-08a/b) — are deliberately absent for the reason the port picker's
// `p`/`0` are (D206 pt 3): a HelpContext names an input state, not which content the
// surface happens to be showing. They stay in `?` and in the secret viewer's own title.
//
// The last two capturing surfaces are HINT-03's, and D217 pt 1 makes them the *last*:
// a third would mean a surface was added without a hint case.
//
// The browse filter field is the third text field, and it takes the same set as the
// other two for the same reason (D206 pt 3): `routeFilterKey` acts only on a mapped key
// with `Text == ""`, so `/` `n` `s` `a` `?` and `q` all type a character into the query
// while the browse table set underneath advertises every one of them. What survives is
// move (the selection steps through the live-narrowed rows so a match can be previewed
// while typing), enter (commit the narrowing and close), esc (clear it and close). Its
// esc carries two meanings in sequence — clear, then close — and is hinted as the one
// entry the registry has, because a hint names a key that acts, not a state machine.
//
// The port-forward panel is the one capturing surface with no text field, so its set is
// simply what `handleForwardsPanelAction` honours: move the cursor, stop the selected
// forward, stop all of them, and the ways out. `forwards.panel` (`F`) also closes it and
// is left out for the reason `q` is left out of the confirm set — back and quit already
// name the way out, and a third is an inventory. nav.drillIn is hinted under its global
// description ("Open / drill into selection") though what it does here is *stop* the
// selected forward: a description is global (D147 pt 3), the panel's own footer says
// "stop" one line up, and the same trade is already made by the prompt modal, where
// enter submits. The hint's promise is which keys act, not a gloss of each verb.
var contextShortHelpActions = map[HelpContext][]Action{
	HelpMenu:   {ActionDown, ActionUp, ActionDrillIn, ActionFilter, ActionPin, ActionNamespace, ActionHelp, ActionQuit},
	HelpTable:  {ActionDown, ActionUp, ActionFilter, ActionUnhealthy, ActionSearchNext, ActionSort, ActionActions, ActionBack, ActionNamespace, ActionHelp, ActionQuit},
	HelpSearch: {ActionDown, ActionUp, ActionDrillIn, ActionSearchAllKinds, ActionSearchAllNamespaces, ActionBack},
	// The unhealthy list is a pager (D276): navigate the hits, open one, leave. The
	// quit key closes the view like every full-screen pager, and drillIn is what
	// opens a hit (switching browse to it), so the set is the pager's.
	HelpUnhealthy: {ActionDown, ActionUp, ActionDrillIn, ActionBack, ActionQuit},
	// logs.regex is offered by HelpLogsFilter rather than here, which is where it acts
	// on something: with the grep closed there is no query for it to re-interpret, and
	// the hint is a line, not a list — LOGS-SEL-02's `v`/`y` are the two gestures a
	// reader cannot guess, and one of them has to make room for them.
	HelpLogs:         {ActionDown, ActionUp, ActionFilter, ActionLogsSelect, ActionLogsYank, ActionLogsFollow, ActionLogsWrap, ActionBack, ActionQuit},
	HelpLogsFilter:   {ActionDown, ActionUp, ActionLogsRegex, ActionBack},
	HelpPickerFilter: {ActionDown, ActionUp, ActionDrillIn, ActionBack},
	HelpPicker:       {ActionDown, ActionUp, ActionFilter, ActionDrillIn, ActionBack},
	HelpConfirm:      {ActionConfirmAccept, ActionConfirmDecline},
	HelpPrompt:       {ActionDrillIn, ActionBack},
	HelpKeybindings:  {ActionBack, ActionHelp, ActionQuit},
	HelpViewer:       {ActionDown, ActionUp, ActionBack, ActionQuit},
	HelpTableFilter:  {ActionDown, ActionUp, ActionDrillIn, ActionBack},
	HelpForwards:     {ActionDown, ActionUp, ActionDrillIn, ActionStopForwards, ActionBack, ActionQuit},
	HelpSort:         {ActionLeft, ActionRight, ActionDrillIn, ActionClearSort, ActionBack},
}

// HelpKeyMap adapts a resolved keymap to bubbles' help.KeyMap interface so a
// help.Model can render both the short (status-bar) and full (overlay) views
// straight from the registry.
type HelpKeyMap struct{ km *Keymap }

// Compile-time check that HelpKeyMap satisfies bubbles' help.KeyMap.
var _ help.KeyMap = HelpKeyMap{}

// HelpMap returns a help.KeyMap view over this keymap.
func (k *Keymap) HelpMap() HelpKeyMap { return HelpKeyMap{km: k} }

// ShortHelp returns the curated (focus-agnostic) status-bar bindings, skipping any
// the user has disabled.
func (h HelpKeyMap) ShortHelp() []key.Binding {
	return h.enabled(shortHelpActions)
}

// ShortHelpContext returns the curated bindings for a focus context (menu vs
// table), skipping any the user has disabled — the focus-aware hint the browse
// status bar shows. An unknown context falls back to the focus-agnostic set.
func (h HelpKeyMap) ShortHelpContext(ctx HelpContext) []key.Binding {
	actions, ok := contextShortHelpActions[ctx]
	if !ok {
		actions = shortHelpActions
	}
	return h.enabled(actions)
}

// enabled maps actions to their bindings, dropping any the user disabled (no
// bound keys), preserving order.
func (h HelpKeyMap) enabled(actions []Action) []key.Binding {
	var out []key.Binding
	for _, a := range actions {
		if b := h.km.Binding(a); b.Enabled() {
			out = append(out, b)
		}
	}
	return out
}

// FullHelp returns every enabled binding grouped into columns by action namespace
// (the id prefix before the first "."), each column in registry order and the
// columns in first-seen order. Disabled actions are omitted.
func (h HelpKeyMap) FullHelp() [][]key.Binding {
	var order []string
	groups := map[string][]key.Binding{}
	for _, a := range Actions() {
		b := h.km.Binding(a)
		if !b.Enabled() {
			continue
		}
		g := groupOf(a)
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		groups[g] = append(groups[g], b)
	}
	out := make([][]key.Binding, 0, len(order))
	for _, g := range order {
		out = append(out, groups[g])
	}
	return out
}

// groupOf is the action's namespace: the id prefix before the first ".", or the
// whole id when there is none.
func groupOf(a Action) string {
	s := string(a)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}
