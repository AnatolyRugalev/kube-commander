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
	if table[ActionDrillIn.Describe()] {
		t.Error("table context should not offer drill-in")
	}
	// Namespace/help/quit are relevant in both contexts.
	for _, a := range []Action{ActionNamespace, ActionHelp, ActionQuit} {
		if !menu[a.Describe()] || !table[a.Describe()] {
			t.Errorf("%q should appear in both focus contexts", a)
		}
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
	// Registry has four namespaces: nav.* then app.* then ns.* then mouse.*.
	if len(full) != 4 {
		t.Fatalf("FullHelp columns = %d; want 4", len(full))
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
	if cols := km.HelpMap().FullHelp(); len(cols) != 3 {
		t.Errorf("FullHelp after disabling nav.* columns = %d; want 3 (app + ns + mouse)", len(cols))
	}
}
