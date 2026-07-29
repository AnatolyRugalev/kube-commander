package keymap

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

// TestBindingFromDefaults checks a normal action's binding carries the resolved
// keys, the display text, and the registry description, and is enabled.
func TestBindingFromDefaults(t *testing.T) {
	km := DefaultKeymap()
	b := km.Binding(ActionTop) // default keys: gg, home
	if !b.Enabled() {
		t.Fatal("nav.top binding should be enabled")
	}
	if got := b.Keys(); len(got) != 2 || got[0] != "gg" || got[1] != "home" {
		t.Errorf("Binding(nav.top).Keys() = %v; want [gg home]", got)
	}
	h := b.Help()
	if h.Key != "gg/home" {
		t.Errorf("help key = %q; want gg/home", h.Key)
	}
	if h.Desc != ActionTop.Describe() {
		t.Errorf("help desc = %q; want %q", h.Desc, ActionTop.Describe())
	}
}

// TestBindingDisabled checks a disabled (empty) binding surfaces as !Enabled so
// help renderers skip it.
func TestBindingDisabled(t *testing.T) {
	km, _, err := DefaultKeymap().Merge(map[Action][]string{ActionQuit: {}})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if b := km.Binding(ActionQuit); b.Enabled() {
		t.Error("disabled action should yield a disabled binding")
	}
	// An unregistered action also yields a disabled binding, never a panic.
	if b := km.Binding("bogus.nope"); b.Enabled() {
		t.Error("unknown action should yield a disabled binding")
	}
}

// TestBindingsCoversRegistry checks Bindings() returns one binding per action in
// registry order.
func TestBindingsCoversRegistry(t *testing.T) {
	km := DefaultKeymap()
	bs := km.Bindings()
	acts := Actions()
	if len(bs) != len(acts) {
		t.Fatalf("Bindings() len = %d; want %d", len(bs), len(acts))
	}
	for i, a := range acts {
		if got, want := bs[i].Help().Desc, a.Describe(); got != want {
			t.Errorf("Bindings()[%d].Desc = %q; want %q", i, got, want)
		}
	}
}

// TestHelpMapShortHelp checks ShortHelp returns the curated subset and drops
// disabled entries.
func TestHelpMapShortHelp(t *testing.T) {
	short := DefaultKeymap().HelpMap().ShortHelp()
	if len(short) != len(shortHelpActions) {
		t.Fatalf("ShortHelp len = %d; want %d", len(short), len(shortHelpActions))
	}
	// Disabling a short-help action drops it from ShortHelp.
	km, _, err := DefaultKeymap().Merge(map[Action][]string{ActionHelp: {}})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	got := km.HelpMap().ShortHelp()
	if len(got) != len(shortHelpActions)-1 {
		t.Errorf("ShortHelp after disabling app.help len = %d; want %d", len(got), len(shortHelpActions)-1)
	}
	for _, b := range got {
		if b.Help().Desc == ActionHelp.Describe() {
			t.Error("disabled app.help should not appear in ShortHelp")
		}
	}
}

// TestShortHelpContext checks the focus-aware hint subsets differ by context —
// the menu context offers drill-in but not filter/search, the table context the
// reverse — and drop entries the user disabled.
func TestShortHelpContext(t *testing.T) {
	hm := DefaultKeymap().HelpMap()

	descs := func(bs []key.Binding) map[string]bool {
		out := map[string]bool{}
		for _, b := range bs {
			out[b.Help().Desc] = true
		}
		return out
	}
	menu := descs(hm.ShortHelpContext(HelpMenu))
	table := descs(hm.ShortHelpContext(HelpTable))

	if !menu[ActionDrillIn.Describe()] {
		t.Error("menu context should offer drill-in")
	}
	if menu[ActionFilter.Describe()] {
		t.Error("menu context should not offer filter (no table to filter)")
	}
	if !table[ActionFilter.Describe()] || !table[ActionSearchNext.Describe()] {
		t.Error("table context should offer filter and next-match")
	}
	if !table[ActionSort.Describe()] {
		t.Error("table context should offer sort")
	}
	if menu[ActionSort.Describe()] {
		t.Error("menu context should not offer sort (no table to sort)")
	}
	if table[ActionDrillIn.Describe()] {
		t.Error("table context should not offer drill-in")
	}
	// Namespace/help/quit are relevant in both contexts.
	for _, a := range []Action{ActionNamespace, ActionHelp, ActionQuit} {
		if !menu[a.Describe()] || !table[a.Describe()] {
			t.Errorf("%q should appear in both focus contexts", a)
		}
	}

	// The search context (SEARCH-03b) offers only keys the search view actually
	// honours: its always-open query field swallows every text-producing key, so the
	// text-keyed browse actions would be a lie in that hint.
	search := descs(hm.ShortHelpContext(HelpSearch))
	if !search[ActionDrillIn.Describe()] || !search[ActionBack.Describe()] {
		t.Error("search context should offer drill-in and back")
	}
	// The all-kinds widen (SEARCH-04a) is bound to a no-text chord precisely so the
	// query field cannot swallow it, and it is the one search key that does not
	// announce itself — the header names the scope only once the widen is on. So it
	// is hinted, unlike the logs view's self-announcing display toggles.
	if !search[ActionSearchAllKinds.Describe()] {
		t.Error("search context should offer the all-kinds widen — it is a no-text chord the view honours")
	}
	// The namespace widen (SEARCH-04b) is hinted for the same reason and as its pair:
	// the header names a scope but never says it can be widened, and advertising only
	// one axis would imply the other is fixed.
	if !search[ActionSearchAllNamespaces.Describe()] {
		t.Error("search context should offer the all-namespaces widen alongside the all-kinds widen")
	}
	for _, a := range []Action{ActionFilter, ActionSort, ActionActions, ActionNamespace, ActionHelp, ActionQuit} {
		if search[a.Describe()] {
			t.Errorf("%q is typed into the search query field, not honoured — it must not be hinted", a)
		}
	}

	// The logs contexts (LOGS-02) are the same rule applied to a view with two input
	// states. Grep closed, every advertised key acts — including `q`, which closes the
	// view like any pager. Grep open, the field swallows text keys, so `/` `f` and `q`
	// drop out and only the no-text keys remain. Neither context offers help (swallowed).
	logs := descs(hm.ShortHelpContext(HelpLogs))
	for _, a := range []Action{ActionDown, ActionUp, ActionFilter, ActionLogsFollow, ActionLogsWrap, ActionBack, ActionQuit} {
		if !logs[a.Describe()] {
			t.Errorf("logs context should offer %q — the view honours it with the grep closed", a)
		}
	}
	if logs[ActionHelp.Describe()] {
		t.Error("the logs view swallows app.help; the hint must not offer it")
	}
	logsFilter := descs(hm.ShortHelpContext(HelpLogsFilter))
	if !logsFilter[ActionBack.Describe()] || !logsFilter[ActionDown.Describe()] {
		t.Error("logs-filter context should offer back (clear the grep) and scrolling")
	}
	for _, a := range []Action{ActionFilter, ActionLogsFollow, ActionLogsWrap, ActionQuit, ActionHelp} {
		if logsFilter[a.Describe()] {
			t.Errorf("%q is typed into the open logs grep, not honoured — it must not be hinted", a)
		}
	}
	// Horizontal scrolling (LOGS-04a) rides the shared nav.left/nav.right and only acts
	// while the view is *not* wrapping, so it is hinted in neither logs context — a hint
	// is a promise, and this one would hold only half the time (D143 pt 1).
	for _, a := range []Action{ActionLeft, ActionRight} {
		if logs[a.Describe()] || logsFilter[a.Describe()] {
			t.Errorf("%q is mode-dependent in the logs view and must not be hinted", a)
		}
	}
	// The timestamps toggle (LOGS-04b) is hinted in neither context, for the other
	// reason: it acts with the grep closed, but the closed-grep hint line is already the
	// scarcest in the app (six entries elide at 220 columns) and a per-session display
	// toggle that puts a stamp on every row the moment it fires does not need to be
	// advertised. `?` and the generated doc carry it.
	if logs[ActionLogsTimestamps.Describe()] || logsFilter[ActionLogsTimestamps.Describe()] {
		t.Error("logs.timestamps self-announces and must not spend a hint slot")
	}
	// The regex toggle carries no text, so unlike follow/quit it survives the open grep
	// and belongs in *both* logs contexts (LOGS-03).
	if !logsFilter[ActionLogsRegex.Describe()] || !descs(hm.ShortHelpContext(HelpLogs))[ActionLogsRegex.Describe()] {
		t.Error("logs.regex acts in both logs states and should be hinted in both")
	}

	// Disabling an action drops it from the context subset.
	km, _, err := DefaultKeymap().Merge(map[Action][]string{ActionNamespace: {}})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if descs(km.HelpMap().ShortHelpContext(HelpMenu))[ActionNamespace.Describe()] {
		t.Error("disabled ns.switch should not appear in the menu context")
	}

	// An unknown context falls back to the focus-agnostic set (never empty).
	if len(hm.ShortHelpContext(HelpContext(99))) != len(hm.ShortHelp()) {
		t.Error("unknown context should fall back to the focus-agnostic ShortHelp set")
	}
}

// TestHelpMapFullHelp checks FullHelp groups enabled bindings by namespace, in
// first-seen column order and registry order within a column, dropping disabled.
func TestHelpMapFullHelp(t *testing.T) {
	full := DefaultKeymap().HelpMap().FullHelp()
	// Registry has fifteen namespaces: nav.* then app.* then ns.* then resources.*
	// then ctx.* (M4-04b) then mouse.* then sort.* then menu.* then actions.* then
	// res.* (M3-02) then logs.* (M3-06) then secret.* (M3-08a) then forwards.*
	// (M3-13b) then search.* (SEARCH-02b) then confirm.* (D132).
	if len(full) != 15 {
		t.Fatalf("FullHelp columns = %d; want 15", len(full))
	}
	// Total enabled bindings equals the whole registry (all default-bound).
	total := 0
	for _, col := range full {
		total += len(col)
	}
	if total != len(Actions()) {
		t.Errorf("FullHelp total = %d; want %d", total, len(Actions()))
	}
	// First column is the nav group, first entry nav.up in registry order.
	if got := full[0][0].Help().Desc; got != ActionUp.Describe() {
		t.Errorf("first full-help entry = %q; want %q", got, ActionUp.Describe())
	}

	// Disabling every nav.* action collapses the nav column entirely.
	overrides := map[Action][]string{}
	for _, a := range Actions() {
		if groupOf(a) == "nav" {
			overrides[a] = nil
		}
	}
	km, _, err := DefaultKeymap().Merge(overrides)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if cols := km.HelpMap().FullHelp(); len(cols) != 14 {
		t.Errorf("FullHelp after disabling nav.* columns = %d; want 14 (app + ns + resources + ctx + mouse + sort + menu + actions + res + logs + secret + forwards + search + confirm)", len(cols))
	}
}
