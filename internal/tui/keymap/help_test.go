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

	// The picker contexts (HINT-01) apply the same rule to an overlay. Every picker
	// opens its filter with itself since PAL-01, so the honest set with the field open
	// is the no-text keys the root routes: move, confirm, cancel. The closed-field
	// state exists only on a WithOptInFilter picker, where `/` is what opens it.
	pickerFilter := descs(hm.ShortHelpContext(HelpPickerFilter))
	for _, a := range []Action{ActionDown, ActionUp, ActionDrillIn, ActionBack} {
		if !pickerFilter[a.Describe()] {
			t.Errorf("picker-filter context should offer %q — the picker honours it", a)
		}
	}
	for _, a := range []Action{ActionFilter, ActionSort, ActionActions, ActionNamespace, ActionPin, ActionHelp, ActionQuit} {
		if pickerFilter[a.Describe()] {
			t.Errorf("%q types into an open picker filter, not honoured — it must not be hinted", a)
		}
	}
	pickerClosed := descs(hm.ShortHelpContext(HelpPicker))
	if !pickerClosed[ActionFilter.Describe()] {
		t.Error("picker context (field closed) should offer the key that opens the field")
	}
	for _, a := range []Action{ActionNamespace, ActionHelp, ActionQuit} {
		if pickerClosed[a.Describe()] {
			t.Errorf("%q is swallowed by an open picker; the hint must not offer it", a)
		}
	}

	// The confirm modal (HINT-02) is the one context whose keys are not browse keys:
	// they resolve in the confirm key context (D132), and the hint must render *their*
	// bindings — `y`/`n` — not the browse meanings of those chords.
	confirm := descs(hm.ShortHelpContext(HelpConfirm))
	for _, a := range []Action{ActionConfirmAccept, ActionConfirmDecline} {
		if !confirm[a.Describe()] {
			t.Errorf("confirm context should offer %q — it is what answers the modal", a)
		}
	}
	for _, a := range []Action{ActionDown, ActionUp, ActionFilter, ActionSearchNext, ActionHelp} {
		if confirm[a.Describe()] {
			t.Errorf("%q is swallowed by an open confirm modal; the hint must not offer it", a)
		}
	}
	if keys := hm.ShortHelpContext(HelpConfirm)[0].Help().Key; keys != "y/enter" {
		t.Errorf("confirm accept should be hinted with its confirm-context keys, got %q", keys)
	}

	// The prompt modal is the same modal with a text field open, so it keeps the
	// *browse* control pair and drops the confirm answers: `y`/`n` type there.
	prompt := descs(hm.ShortHelpContext(HelpPrompt))
	for _, a := range []Action{ActionDrillIn, ActionBack} {
		if !prompt[a.Describe()] {
			t.Errorf("prompt context should offer %q — it carries no text and still acts", a)
		}
	}
	for _, a := range []Action{ActionConfirmAccept, ActionConfirmDecline, ActionQuit, ActionHelp} {
		if prompt[a.Describe()] {
			t.Errorf("%q types into an open prompt field; the hint must not offer it", a)
		}
	}

	// The keybindings overlay advertises the ways out and nothing else — it swallows
	// navigation and does not scroll.
	overlay := descs(hm.ShortHelpContext(HelpKeybindings))
	for _, a := range []Action{ActionBack, ActionHelp, ActionQuit} {
		if !overlay[a.Describe()] {
			t.Errorf("keybindings-overlay context should offer %q — it closes the overlay", a)
		}
	}
	for _, a := range []Action{ActionDown, ActionUp, ActionDrillIn, ActionNamespace} {
		if overlay[a.Describe()] {
			t.Errorf("%q is swallowed by the open keybindings overlay; it must not be hinted", a)
		}
	}

	// The shared viewer is a pager: scroll plus the two ways to close it. Its
	// kind-specific gestures stay out — a context names an input state, not content
	// (D206 pt 3).
	view := descs(hm.ShortHelpContext(HelpViewer))
	for _, a := range []Action{ActionDown, ActionUp, ActionBack, ActionQuit} {
		if !view[a.Describe()] {
			t.Errorf("viewer context should offer %q — the viewer honours it", a)
		}
	}
	for _, a := range []Action{ActionRevealSecret, ActionCopySecret, ActionFilter, ActionHelp} {
		if view[a.Describe()] {
			t.Errorf("%q is not honoured by every viewer; the hint must not promise it", a)
		}
	}

	// The browse filter field (HINT-03) is the third text field and takes the same set
	// as the other two: only the no-text keys survive an open query.
	tblFilter := descs(hm.ShortHelpContext(HelpTableFilter))
	for _, a := range []Action{ActionDown, ActionUp, ActionDrillIn, ActionBack} {
		if !tblFilter[a.Describe()] {
			t.Errorf("filter-field context should offer %q — it carries no text and still acts", a)
		}
	}
	for _, a := range []Action{ActionFilter, ActionSearchNext, ActionSort, ActionActions, ActionHelp, ActionQuit} {
		if tblFilter[a.Describe()] {
			t.Errorf("%q types into the open filter field; the hint must not offer it", a)
		}
	}

	// The port-forward panel (HINT-03) is the capturing surface with no text field, so
	// its set is what its router honours — cursor, stop, stop-all, and the ways out.
	forwards := descs(hm.ShortHelpContext(HelpForwards))
	for _, a := range []Action{ActionDown, ActionUp, ActionDrillIn, ActionStopForwards, ActionBack, ActionQuit} {
		if !forwards[a.Describe()] {
			t.Errorf("forwards-panel context should offer %q — the panel honours it", a)
		}
	}
	for _, a := range []Action{ActionFilter, ActionSort, ActionActions, ActionNamespace, ActionHelp} {
		if forwards[a.Describe()] {
			t.Errorf("%q is swallowed by the open port-forward panel; it must not be hinted", a)
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
	// Registry has sixteen namespaces: nav.* then app.* then ns.* then resources.*
	// then ctx.* (M4-04b) then theme.* (M4-12b-2) then mouse.* then sort.* then menu.*
	// then actions.* then res.* (M3-02) then logs.* (M3-06) then secret.* (M3-08a)
	// then forwards.* (M3-13b) then search.* (SEARCH-02b) then confirm.* (D132).
	if len(full) != 16 {
		t.Fatalf("FullHelp columns = %d; want 16", len(full))
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
	if cols := km.HelpMap().FullHelp(); len(cols) != 15 {
		t.Errorf("FullHelp after disabling nav.* columns = %d; want 15 (app + ns + resources + ctx + theme + mouse + sort + menu + actions + res + logs + secret + forwards + search + confirm)", len(cols))
	}
}
