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
)

// contextShortHelpActions is the curated hint subset per focus context. Each set
// is ordered as shown and rendered enabled-only. Filter/search/sort appear only in
// the table context (they act on a resource table, no-ops on the menu), while
// drill-in appears only in the menu context (opening the selected resource);
// namespace, help and quit are always-relevant and shown in both browse contexts.
//
// The search context is deliberately the short one. Its query field is always open
// (D140 pt 1), so the root routes every text-producing key into it: `/` `n` `s` `a`
// `?` and `q` all type a character there instead of firing their browse action, and a
// hint that advertised them would be a lie. What is left is the genuinely available
// set — move the result cursor, open a hit, clear-then-close — every one of them a
// no-text key the view actually consumes.
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
var contextShortHelpActions = map[HelpContext][]Action{
	HelpMenu:       {ActionDown, ActionUp, ActionDrillIn, ActionNamespace, ActionHelp, ActionQuit},
	HelpTable:      {ActionDown, ActionUp, ActionFilter, ActionSearchNext, ActionSort, ActionActions, ActionBack, ActionNamespace, ActionHelp, ActionQuit},
	HelpSearch:     {ActionDown, ActionUp, ActionDrillIn, ActionBack},
	HelpLogs:       {ActionDown, ActionUp, ActionFilter, ActionLogsRegex, ActionLogsFollow, ActionLogsWrap, ActionBack, ActionQuit},
	HelpLogsFilter: {ActionDown, ActionUp, ActionLogsRegex, ActionBack},
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
