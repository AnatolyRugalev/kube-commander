package keymap

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/neuroplastio/kubecom/internal/keybind"
)

// hintPaths is a path for each hint context: what holds the keyboard when the
// bottom line shows that context's hint.
var hintPaths = map[HelpContext]keybind.Path{
	HelpMenu:         menuPath,
	HelpTable:        tablePath,
	HelpSearch:       {Cluster: testCluster, Table: testPods, Search: &keybind.Search{}, Field: true},
	HelpUnhealthy:    {Cluster: testCluster, Menu: true, Unhealthy: true},
	HelpLogs:         {Cluster: testCluster, Table: testPods, Logs: &keybind.Logs{Follow: true}},
	HelpLogsFilter:   {Cluster: testCluster, Table: testPods, Logs: &keybind.Logs{}, Field: true},
	HelpPickerFilter: {Cluster: testCluster, Table: testPods, Picker: &keybind.Picker{ID: "palette"}, Field: true},
	HelpPicker:       {Cluster: testCluster, Table: testPods, Picker: &keybind.Picker{ID: "containers"}},
	HelpConfirm:      confirmPath,
	HelpPrompt:       {Cluster: testCluster, Table: testPods, Prompt: true, Field: true},
	HelpKeybindings:  {Cluster: testCluster, Menu: true, Help: true},
	HelpViewer:       {Cluster: testCluster, Table: testPods, Viewer: &keybind.Viewer{Content: "describe"}},
	HelpTableFilter:  {Cluster: testCluster, Table: testPods, Field: true},
	HelpForwards:     {Cluster: testCluster, Table: testPods, Forwards: true},
	HelpSort:         {Cluster: testCluster, Table: testPods, Sort: true},
}

// hint is a context's hint in its path, from the default keymap.
func hint(ctx HelpContext) []key.Binding {
	return DefaultKeymap().HelpMap(hintPaths[ctx]).ShortHelpContext(ctx)
}

// TestBindingFromDefaults checks a normal action's binding carries the keys that
// run it in the path, the display text, and the registry description, and is
// enabled.
func TestBindingFromDefaults(t *testing.T) {
	b := DefaultKeymap().Binding(ActionTop, menuPath) // default keys: g g, home
	if !b.Enabled() {
		t.Fatal("nav.top binding should be enabled")
	}
	if got := b.Keys(); len(got) != 2 || got[0] != "g g" || got[1] != "home" {
		t.Errorf("Binding(nav.top).Keys() = %v; want [g g home]", got)
	}
	h := b.Help()
	if h.Key != "g g/home" {
		t.Errorf("help key = %q; want g g/home", h.Key)
	}
	if h.Desc != ActionTop.Describe() {
		t.Errorf("help desc = %q; want %q", h.Desc, ActionTop.Describe())
	}
}

// TestBindingDisabled checks an action nothing runs in the path surfaces as
// !Enabled so help renderers skip it: unbound by the user, bound only in another
// context, or not an action at all.
func TestBindingDisabled(t *testing.T) {
	km := withUser(t, `unbind "q"; unbind "ctrl+c"`, "")
	if b := km.Binding(ActionQuit, menuPath); b.Enabled() {
		t.Error("an unbound action should yield a disabled binding")
	}
	if b := DefaultKeymap().Binding(ActionConfirmAccept, menuPath); b.Enabled() {
		t.Error("confirm.accept runs only in the confirm modal; the menu's binding should be disabled")
	}
	if b := DefaultKeymap().Binding("bogus.nope", menuPath); b.Enabled() {
		t.Error("unknown action should yield a disabled binding")
	}
}

// The hint context is the path's innermost capturing surface, and every context
// has a path that selects it.
func TestHelpContextOf(t *testing.T) {
	for _, ctx := range HelpContexts() {
		p, ok := hintPaths[ctx]
		if !ok {
			t.Errorf("%s has no path in hintPaths", ctx)
			continue
		}
		if got := HelpContextOf(p); got != ctx {
			t.Errorf("HelpContextOf(%s) = %s, want %s", p, got, ctx)
		}
	}
	// A view or pane under an overlay that captures keys does not choose the hint.
	logsPicker := keybind.Path{Cluster: testCluster, Table: testPods, Logs: &keybind.Logs{}, Picker: &keybind.Picker{ID: "palette"}}
	if got := HelpContextOf(logsPicker); got != HelpPicker {
		t.Errorf("a picker over the logs view hints %s, want HelpPicker", got)
	}
	if got := HelpContextOf(keybind.Path{}); got != HelpMenu {
		t.Errorf("the empty path hints %s, want HelpMenu", got)
	}
}

// TestHelpMapShortHelp checks ShortHelp returns the curated subset and drops
// entries nothing runs.
func TestHelpMapShortHelp(t *testing.T) {
	short := DefaultKeymap().HelpMap(menuPath).ShortHelp()
	if len(short) != len(shortHelpActions) {
		t.Fatalf("ShortHelp len = %d; want %d", len(short), len(shortHelpActions))
	}
	got := withUser(t, `unbind "?"`, "").HelpMap(menuPath).ShortHelp()
	if len(got) != len(shortHelpActions)-1 {
		t.Errorf("ShortHelp after unbinding app.help len = %d; want %d", len(got), len(shortHelpActions)-1)
	}
	for _, b := range got {
		if b.Help().Desc == ActionHelp.Describe() {
			t.Error("unbound app.help should not appear in ShortHelp")
		}
	}
}

// TestShortHelpContext checks the focus-aware hint subsets differ by context —
// the menu context offers drill-in and (since STORY-06m) `/` to filter the kinds
// there, the table context the filter/search/sort set instead — and drop entries
// the user disabled.
func TestShortHelpContext(t *testing.T) {
	descs := func(bs []key.Binding) map[string]bool {
		out := map[string]bool{}
		for _, b := range bs {
			out[b.Help().Desc] = true
		}
		return out
	}
	menu := descs(hint(HelpMenu))
	table := descs(hint(HelpTable))

	if !menu[ActionDrillIn.Describe()] {
		t.Error("menu context should offer drill-in")
	}
	// STORY-06m: `/` with the resources pane focused narrows the kinds there, so the
	// menu context advertises it too.
	if !menu[ActionFilter.Describe()] {
		t.Error("menu context should offer filter (it narrows the kinds there)")
	}
	if !table[ActionFilter.Describe()] || !table[ActionSearchNext.Describe()] {
		t.Error("table context should offer filter and next-match")
	}
	if !table[ActionUnhealthy.Describe()] {
		t.Error("table context should offer the unhealthy quick-access")
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
	search := descs(hint(HelpSearch))
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
	logs := descs(hint(HelpLogs))
	for _, a := range []Action{ActionDown, ActionUp, ActionFilter, ActionLogsFollow, ActionLogsWrap, ActionBack, ActionQuit} {
		if !logs[a.Describe()] {
			t.Errorf("logs context should offer %q — the view honours it with the grep closed", a)
		}
	}
	if logs[ActionHelp.Describe()] {
		t.Error("the logs view swallows app.help; the hint must not offer it")
	}
	logsFilter := descs(hint(HelpLogsFilter))
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
	// — and the open grep is now the only logs context that hints it (LOGS-SEL-02).
	// That is the scarcity above being spent, not a claim that ctrl+r stopped working
	// with the grep closed: it still fires, `?` and the doc still carry it, and the
	// header still marks `[re]`. But with no query typed there is nothing for the mode
	// to re-interpret — the action's own doc says the mode matters least before typing
	// — while `v` and `y` are gestures a reader cannot discover by looking at the
	// screen, and the line has room for one of the three.
	if !logsFilter[ActionLogsRegex.Describe()] {
		t.Error("logs.regex survives the open grep and must be hinted there")
	}
	if logs[ActionLogsRegex.Describe()] {
		t.Error("with no query typed, logs.regex must not hold a slot the selection keys need")
	}
	// Visual mode and the yank (LOGS-SEL-02) are hinted with the grep closed and not
	// with it open: both are plain letters, so an open field types them.
	for _, a := range []Action{ActionLogsSelect, ActionLogsYank} {
		if !logs[a.Describe()] {
			t.Errorf("logs context should offer %q — a copy gesture nothing on screen announces", a)
		}
		if logsFilter[a.Describe()] {
			t.Errorf("%q types into the open logs grep, not honoured — it must not be hinted", a)
		}
	}

	// The picker contexts (HINT-01) apply the same rule to an overlay. Since STORY-06d
	// a picker opens in navigation mode with its field closed, so the honest set with
	// the field open (HelpPickerFilter) is the no-text keys the root routes: move,
	// confirm, cancel — plus `/`, which is what opens the closed field (HelpPicker).
	pickerFilter := descs(hint(HelpPickerFilter))
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
	pickerClosed := descs(hint(HelpPicker))
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
	confirm := descs(hint(HelpConfirm))
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
	if keys := hint(HelpConfirm)[0].Help().Key; keys != "y/enter" {
		t.Errorf("confirm accept should be hinted with its confirm-context keys, got %q", keys)
	}

	// The prompt modal is the same modal with a text field open, so it keeps the
	// *browse* control pair and drops the confirm answers: `y`/`n` type there.
	prompt := descs(hint(HelpPrompt))
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
	overlay := descs(hint(HelpKeybindings))
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
	view := descs(hint(HelpViewer))
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
	tblFilter := descs(hint(HelpTableFilter))
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
	forwards := descs(hint(HelpForwards))
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

	// Unbinding an action drops it from the context subset.
	km := withUser(t, `unbind "N"`, "")
	if descs(km.HelpMap(menuPath).ShortHelpContext(HelpMenu))[ActionNamespace.Describe()] {
		t.Error("unbound ns.switch should not appear in the menu context")
	}

	// An unknown context falls back to the focus-agnostic set (never empty).
	hm := DefaultKeymap().HelpMap(menuPath)
	if len(hm.ShortHelpContext(HelpContext(99))) != len(hm.ShortHelp()) {
		t.Error("unknown context should fall back to the focus-agnostic ShortHelp set")
	}
}

// TestEveryHelpContextHasItsOwnHintSet closes the first half of D218 pt 1 (HINT-05).
// The fallback the test above pins is right for an out-of-range integer and wrong for a
// *declared* context: a new capturing surface whose constant has no entry in
// contextShortHelpActions gets the focus-agnostic browse set — filter, help, quit — on a
// surface where none of them act, and nothing goes red. Enumerating the enum is what makes
// that impossible; the sets themselves stay curated by hand, which is the point of D143 pt 1.
func TestEveryHelpContextHasItsOwnHintSet(t *testing.T) {
	for _, ctx := range HelpContexts() {
		actions, ok := contextShortHelpActions[ctx]
		if !ok {
			t.Errorf("%s has no entry in contextShortHelpActions — it would silently fall back "+
				"to the browse set; add its curated subset", ctx)
			continue
		}
		if len(actions) == 0 {
			t.Errorf("%s has an empty hint set: a surface that captures input still owes the "+
				"reader the way back out", ctx)
		}
		// The set must survive resolution: an action that no default binding names would
		// be dropped by enabled() and the hint would render shorter than it reads here.
		if got := len(hint(ctx)); got != len(actions) {
			t.Errorf("%s resolves to %d bindings from %d actions — one runs nothing in its "+
				"path in the default keymap", ctx, got, len(actions))
		}
	}
	// The other direction: an entry keyed by a context that no longer exists is dead
	// curation, and it reads as coverage.
	for ctx := range contextShortHelpActions {
		if ctx < 0 || ctx >= helpContextCount {
			t.Errorf("contextShortHelpActions has an entry for %s, which is not a declared "+
				"context", ctx)
		}
	}
	// String() is what makes every failure above readable, so it is checked here rather
	// than in a test of its own: an unnamed context prints as an integer.
	for _, ctx := range HelpContexts() {
		if helpContextNames[ctx] == "" {
			t.Errorf("HelpContext(%d) has no name in helpContextNames", int(ctx))
		}
	}
	if got := HelpContext(99).String(); got != "HelpContext(99)" {
		t.Errorf("an out-of-range context should print its number, got %q", got)
	}
}

// TestHelpMapFullHelp checks FullHelp groups what runs in the path by namespace,
// in first-seen column order and registry order within a column.
func TestHelpMapFullHelp(t *testing.T) {
	cols := func(km *Keymap, p keybind.Path) map[string]map[Action]bool {
		out := map[string]map[Action]bool{}
		byDesc := map[string]Action{}
		for _, a := range Actions() {
			byDesc[a.Describe()] = a
		}
		for _, col := range km.HelpMap(p).FullHelp() {
			a := byDesc[col[0].Help().Desc]
			out[groupOf(a)] = map[Action]bool{}
			for _, b := range col {
				out[groupOf(a)][byDesc[b.Help().Desc]] = true
			}
		}
		return out
	}
	full := DefaultKeymap().HelpMap(tablePath).FullHelp()
	// The first column is the nav group, its first entry nav.up in registry order.
	if got := full[0][0].Help().Desc; got != ActionUp.Describe() {
		t.Errorf("first full-help entry = %q; want %q", got, ActionUp.Describe())
	}
	// On the table every action with a key runs except the confirm answers, which
	// run only in the modal.
	table := cols(DefaultKeymap(), tablePath)
	if table["confirm"] != nil {
		t.Error("the table's overlay lists the confirm modal's answers")
	}
	if !table["actions"][ActionActions] {
		t.Error("the table's overlay should list enter → actions.menu")
	}
	if menu := cols(DefaultKeymap(), menuPath); menu["actions"][ActionActions] {
		t.Error("the menu's overlay lists actions.menu, which only the table's enter runs")
	}
	if confirm := cols(DefaultKeymap(), confirmPath); !confirm["confirm"][ActionConfirmAccept] {
		t.Error("the confirm modal's overlay should list its answers")
	}
	// Unbinding every nav.* key collapses the nav column entirely.
	var unbind string
	for _, e := range DefaultKeymap().Engine().Bindings() {
		if groupOf(Action(e.Binding.Action)) == "nav" {
			unbind += "unbind \"" + keybind.FormatKeys(e.Keys) + "\"\n"
		}
	}
	if c := cols(withUser(t, unbind, ""), tablePath); c["nav"] != nil {
		t.Errorf("nav column after unbinding nav.*: %v", c["nav"])
	}
}
