package keymap

import (
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
	// HelpLogs is the dedicated logs mini-app (LOGS-02) with its live grep closed:
	// scroll the stream, open the grep, toggle follow, close the view.
	HelpLogs
	// HelpLogsFilter is the same logs mini-app with its live grep *open*, which
	// captures text — so it advertises only the keys that still act there.
	HelpLogsFilter
	// HelpPickerFilter is any modal picker with its filter field open — which is
	// every picker from the moment it is shown (PAL-01/D194 pt 2), the port picker
	// excepted. The overlay captures all input and the open field takes every
	// text-producing key, so only the no-text keys act: move, select, cancel.
	HelpPickerFilter
	// HelpPicker is a modal picker with its filter field *closed* — reachable only on
	// a WithOptInFilter picker (the port picker, D139), where the letter keys are free
	// and `/` opens the field.
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
)

// contextShortHelpActions is the curated hint subset per focus context. Each set
// is ordered as shown and rendered enabled-only. Filter/search/sort appear only in
// the table context (they act on a resource table, no-ops on the menu), while
// drill-in appears only in the menu context (opening the selected resource);
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
// the browse hints underneath it advertised ten keys of which two acted. The
// type-to-filter picker — every picker but the port one, since PAL-01 — opens its
// field with itself, so `/`, `s`, `a`, `?` and `q` type a character and the honest set
// is the four no-text keys the root actually routes: move the cursor, confirm, cancel.
// Paging (ctrl+d/u) acts too and is left out as it is in every other context: the hint
// is the keys a reader must be told about, not an inventory.
//
// HelpPicker is the closed-field state, which only a WithOptInFilter picker can be in
// (the port picker, D139). It adds `/` — the key that opens the field — and nothing
// else. The port picker's own two gestures (`p` local port, `0` free port) are *not*
// hinted here even though they act: a HelpContext names an input state, not a picker
// kind, and this set is shared by any future opt-in picker that does not bind them.
// They stay in `?` and in the port picker's own title.
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
var contextShortHelpActions = map[HelpContext][]Action{
	HelpMenu:         {ActionDown, ActionUp, ActionDrillIn, ActionPin, ActionNamespace, ActionHelp, ActionQuit},
	HelpTable:        {ActionDown, ActionUp, ActionFilter, ActionSearchNext, ActionSort, ActionActions, ActionBack, ActionNamespace, ActionHelp, ActionQuit},
	HelpSearch:       {ActionDown, ActionUp, ActionDrillIn, ActionSearchAllKinds, ActionSearchAllNamespaces, ActionBack},
	HelpLogs:         {ActionDown, ActionUp, ActionFilter, ActionLogsRegex, ActionLogsFollow, ActionLogsWrap, ActionBack, ActionQuit},
	HelpLogsFilter:   {ActionDown, ActionUp, ActionLogsRegex, ActionBack},
	HelpPickerFilter: {ActionDown, ActionUp, ActionDrillIn, ActionBack},
	HelpPicker:       {ActionDown, ActionUp, ActionFilter, ActionDrillIn, ActionBack},
	HelpConfirm:      {ActionConfirmAccept, ActionConfirmDecline},
	HelpPrompt:       {ActionDrillIn, ActionBack},
	HelpKeybindings:  {ActionBack, ActionHelp, ActionQuit},
	HelpViewer:       {ActionDown, ActionUp, ActionBack, ActionQuit},
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
