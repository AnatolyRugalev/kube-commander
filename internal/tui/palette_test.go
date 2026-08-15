package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// TestPaletteOpensOnColon proves `:` opens the command palette (PAL-02) — not the
// resource list it used to open — seeded with the curated verb list, with its filter
// open from the moment it appears (PAL-01/D194 pt 1). Since PAL-05b the resource list
// is a stage of this same palette, so "not the resource list" is now the assertion
// that no verb is committed: `:` starts on the verbs, `R` on `:resource `.
func TestPaletteOpensOnColon(t *testing.T) {
	m := sized(t)
	m, cmd := press(t, m, colon)
	if !m.cmdPicker.Active() {
		t.Fatal("app.palette should open the command palette")
	}
	if m.palArg != "" {
		t.Fatalf("`:` should open on the verb list, not on a committed stage, stage = %q", m.palArg)
	}
	if got, want := m.cmdPicker.Len(), len(paletteVerbs); got != want {
		t.Fatalf("palette seeded with %d verbs, want %d", got, want)
	}
	if !m.cmdPicker.Filtering() {
		t.Fatal("the palette should open with its filter already open")
	}
	if msgs := pickerMsgs(cmd); len(msgs) != 0 {
		t.Fatalf("opening the palette should issue no command beyond the filter blink, got %v", msgs)
	}
}

// TestPaletteOpensWithoutACluster proves the palette is never inert: unlike the
// resource picker it needs no watcher, because the verbs it lists are compiled in.
// A verb that needs a cluster stays inert when it is picked, exactly as its key is.
func TestPaletteOpensWithoutACluster(t *testing.T) {
	m := sized(t) // no WithWatcher
	m, _ = press(t, m, colon)
	if !m.cmdPicker.Active() {
		t.Fatal("the palette should open with no cluster wired")
	}
}

// selectInPalette drills into the palette's highlighted row and feeds the resulting
// SelectedMsg back through Update — the whole keystroke path, as the runtime does it.
func selectInPalette(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, selCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	if selCmd == nil {
		t.Fatal("drill-in should emit a selection command")
	}
	sel, ok := selCmd().(picker.SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want picker.SelectedMsg", selCmd())
	}
	if sel.Kind != commandPickerKind {
		t.Fatalf("palette selection Kind = %q, want %q", sel.Kind, commandPickerKind)
	}
	next, cmd := m.Update(sel)
	return next.(Model), cmd
}

// TestPaletteResourceVerbCommitsInPlace is the D68 guard for this slice: `:resource`
// must stay reachable, and since PAL-03a it resolves **in the palette** rather than by
// opening a second modal. Typing the verb and confirming it swaps the same picker's
// items for the resource kinds and re-prompts the line.
func TestPaletteResourceVerbCommitsInPlace(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "resource")
	if v, _ := m.cmdPicker.Selected(); v != keymap.ActionResources.Describe() {
		t.Fatalf("typing \"resource\" selected %q, want %q", v, keymap.ActionResources.Describe())
	}

	m, _ = selectInPalette(t, m)

	if !m.cmdPicker.Active() {
		t.Fatal("an argument verb should keep the palette open, not close it")
	}
	if m.palArg != keymap.ActionResources {
		t.Fatalf("palette stage = %q, want %q", m.palArg, keymap.ActionResources)
	}
	if got := m.cmdPicker.Len(); got != availableResourceCount(m) {
		t.Fatalf("argument stage seeded with %d entries, want %d", got, availableResourceCount(m))
	}
	if got := m.cmdPicker.Query(); got != "" {
		t.Fatalf("committing a verb should clear the line's query, got %q", got)
	}
}

// TestPaletteSpaceCommitsTheHighlightedVerb proves the line's separator: space partway
// through a verb commits it, so `:res␣` reaches the resource values without confirming
// first — the one uninterrupted line of typing the PAL feedback asked for.
func TestPaletteSpaceCommitsTheHighlightedVerb(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "res")
	m, cmd := press(t, m, tea.Key{Code: ' ', Text: " "})
	if cmd != nil {
		t.Fatal("committing a verb should issue no command")
	}
	if m.palArg != keymap.ActionResources {
		t.Fatalf("space should commit the resource verb, stage = %q", m.palArg)
	}
	if got := m.cmdPicker.Len(); got != availableResourceCount(m) {
		t.Fatalf("argument stage seeded with %d entries, want %d", got, availableResourceCount(m))
	}
}

// TestPaletteSpaceOnAPlainVerbIsSwallowed pins the other half of the separator rule:
// a verb that takes no argument cannot be committed, and the space is never query text
// — the query is exactly what was typed, so the list has not moved either.
func TestPaletteSpaceOnAPlainVerbIsSwallowed(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "quit")
	before, _ := m.cmdPicker.Selected()
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg != "" {
		t.Fatalf("a plain verb should not enter an argument stage, stage = %q", m.palArg)
	}
	if got := m.cmdPicker.Query(); got != "quit" {
		t.Fatalf("query = %q, want %q (the space is a separator, not text)", got, "quit")
	}
	if after, _ := m.cmdPicker.Selected(); after != before {
		t.Fatalf("the swallowed space moved the selection from %q to %q", before, after)
	}
}

// TestPaletteArgumentSwitchesTheResource drives the whole line — `:` `resource` ␣
// `cron` enter — and asserts it lands in exactly the same place the standalone picker
// lands: a watch started for the kind, focus on the table, the palette gone.
func TestPaletteArgumentSwitchesTheResource(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "resource")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	m = typeInto(t, m, "cron")
	if v, _ := m.cmdPicker.Selected(); v != "CronJob" {
		t.Fatalf("filtering the argument to \"cron\" selected %q, want CronJob", v)
	}

	m, _ = selectInPalette(t, m)

	if m.cmdPicker.Active() {
		t.Fatal("applying an argument should close the palette")
	}
	if m.palArg != "" {
		t.Fatalf("a closed palette should be back on its verb stage, stage = %q", m.palArg)
	}
	if !m.hasCurrent || m.current.GVR.Resource != "cronjobs" {
		t.Fatalf("the line should start a watch for cronjobs, got current=%+v hasCurrent=%v", m.current.GVR, m.hasCurrent)
	}
	if !m.table.Focused() {
		t.Fatal("switching a resource should move focus to the table")
	}
	if len(fw.res) != 1 || fw.res[0].GVR.Resource != "cronjobs" {
		t.Fatalf("want one watch of cronjobs, got %v", fw.res)
	}
}

// TestPaletteArgumentAppliesATheme proves the second wired verb, and with it that an
// argument stage ends in the verb's own apply function: the theme repaints and is
// persisted exactly as picking it in the theme switcher does.
func TestPaletteArgumentAppliesATheme(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "theme")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg != keymap.ActionTheme {
		t.Fatalf("space should commit the theme verb, stage = %q", m.palArg)
	}
	m = typeInto(t, m, "monokai")

	m, _ = selectInPalette(t, m)

	if m.cmdPicker.Active() {
		t.Fatal("applying an argument should close the palette")
	}
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("theme = %q, want monokai", got)
	}
}

// TestPaletteBackspaceLeavesTheArgumentStage proves the line unwinds the way it was
// typed: with the argument empty, backspace erases the committed verb and the verb
// list comes back — indistinguishable from a freshly opened palette.
func TestPaletteBackspaceLeavesTheArgumentStage(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "resource")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	m = typeInto(t, m, "cr")

	// The first backspaces erase the argument itself; only one into an empty query
	// leaves the stage.
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if m.palArg != keymap.ActionResources {
		t.Fatalf("erasing the argument text should stay in the stage, stage = %q", m.palArg)
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if m.palArg != "" {
		t.Fatalf("backspace into an empty argument should leave the stage, stage = %q", m.palArg)
	}
	if got, want := m.cmdPicker.Len(), len(paletteVerbs); got != want {
		t.Fatalf("the verb list should be back with %d entries, got %d", want, got)
	}
	if !m.cmdPicker.Active() {
		t.Fatal("leaving the argument stage should not close the palette")
	}
}

// TestPaletteEscRewindsThenCloses proves esc walks a *typed* line back out the way it
// was typed: one esc returns the argument stage to the verb list, the next closes the
// palette. This is the path D233 left alone — the reader was on that verb list a
// keystroke ago, so rewinding to it is backing out. A stage a shortcut key opened has
// no such history and closes on the first esc (TestActionStageEscClosesToTheTable).
func TestPaletteEscRewindsThenCloses(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "resource")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	next, _ := m.Update(cancelCmd())
	m = next.(Model)
	if !m.cmdPicker.Active() {
		t.Fatal("esc in the argument stage should rewind, not close")
	}
	if m.palArg != "" {
		t.Fatalf("esc should uncommit the verb, stage = %q", m.palArg)
	}

	m, cancelCmd = press(t, m, tea.Key{Code: tea.KeyEsc})
	next, _ = m.Update(cancelCmd())
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Fatal("esc on the verb list should close the palette")
	}
}

// TestPaletteArgumentStaysInertWithoutACluster proves an inert verb does not become
// live by being reachable through the palette (D197): with no watcher there is nothing
// to switch, so the resource verb opens no argument list and no picker — it does
// nothing, exactly as `R` does.
func TestPaletteArgumentStaysInertWithoutACluster(t *testing.T) {
	m := sized(t) // no WithWatcher
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "resource")
	m, _ = selectInPalette(t, m)
	if m.palArg != "" {
		t.Fatalf("an inert verb should not open an argument stage, stage = %q", m.palArg)
	}
	if m.cmdPicker.Active() {
		t.Fatal("an inert verb should leave nothing open")
	}
}

// commitVerb opens the palette, narrows it to a verb and commits it with the line's
// separator, returning the model and whatever command the commit issued (the value
// load, for a verb whose list is fetched).
func commitVerb(t *testing.T, m Model, verb string) (Model, tea.Cmd) {
	t.Helper()
	m, _ = press(t, m, colon)
	m = typeInto(t, m, verb)
	return press(t, m, tea.Key{Code: ' ', Text: " "})
}

// TestPaletteNamespaceArgumentLoadsThenSeeds is the PAL-03b headline: a verb whose
// values are fetched enters the argument stage *immediately* — the reader never waits
// for the network to see the line advance — on an empty list that says so. Typing
// while it is in flight is kept, so a fast typist's query narrows the values the
// moment they land instead of being discarded.
func TestPaletteNamespaceArgumentLoadsThenSeeds(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	m := sizedWith(t, WithNamespaceLister(fl))

	m, cmd := commitVerb(t, m, "namespace")
	if m.palArg != keymap.ActionNamespace {
		t.Fatalf("space should commit the namespace verb, stage = %q", m.palArg)
	}
	if got := m.cmdPicker.Len(); got != 0 {
		t.Fatalf("the stage should open empty while the list is in flight, got %d entries", got)
	}
	if view := m.cmdPicker.View(); !strings.Contains(view, "loading") {
		t.Fatalf("a pending stage must say so rather than look empty:\n%s", view)
	}
	if cmd == nil {
		t.Fatal("committing the namespace verb should issue the list command")
	}
	lm, ok := pickerMsg(t, cmd).(namespacesLoadedMsg)
	if !ok {
		t.Fatalf("the commit produced %T, want namespacesLoadedMsg", pickerMsg(t, cmd))
	}

	m = typeInto(t, m, "sys") // type ahead of the answer
	next, _ := m.Update(lm)
	m = next.(Model)

	if !m.cmdPicker.Active() || m.palArg != keymap.ActionNamespace {
		t.Fatalf("the values should land in the open stage, active=%v stage=%q", m.cmdPicker.Active(), m.palArg)
	}
	if view := m.cmdPicker.View(); strings.Contains(view, "loading") {
		t.Fatalf("the loading marker should go once the values land:\n%s", view)
	}
	if got := m.cmdPicker.Len(); got != 1 {
		t.Fatalf("the query typed while waiting should narrow the arrived list to kube-system, got %d entries", got)
	}
	if v, _ := m.cmdPicker.Selected(); v != "kube-system" {
		t.Fatalf("selected %q, want kube-system", v)
	}
}

// TestPaletteNamespaceArgumentAppliesTheScope closes the namespace line: the pick
// re-scopes the app exactly as N's does, because since PAL-05c-1 they are the one
// surface ending in applyNamespaceValue (D198 pt 2) — including the all-namespaces
// sentinel, which must be offered here too or the palette line would be a one-way door.
func TestPaletteNamespaceArgumentAppliesTheScope(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	fp := &fakePersister{}
	m := sizedWith(t, WithNamespaceLister(fl), WithNamespacePersister(fp))

	m, cmd := commitVerb(t, m, "namespace")
	next, _ := m.Update(pickerMsg(t, cmd))
	m = next.(Model)
	if got := m.cmdPicker.Len(); got != 3 {
		t.Fatalf("stage seeded with %d entries, want 3 (2 namespaces + the all-namespaces sentinel)", got)
	}
	m = typeInto(t, m, "kube-sys")

	m, _ = selectInPalette(t, m)

	if m.cmdPicker.Active() {
		t.Fatal("applying an argument should close the palette")
	}
	if got := m.namespace; got != "kube-system" {
		t.Fatalf("namespace = %q, want kube-system", got)
	}
}

// TestPaletteContextArgumentSwitchesContext proves the second fetched verb, and that
// its rows are the context picker's own (marked, cluster-qualified) rows resolved
// through the same map and the same apply — a pick here connects exactly as `C` does.
func TestPaletteContextArgumentSwitchesContext(t *testing.T) {
	fl := &fakeContextLister{contexts: twoContexts()}
	fc := &fakeConnector{}
	fc.cluster, _, _ = newClusterFake()
	m := sizedWith(t, WithContextLister(fl), WithClusterConnector(fc), WithContext("prod"))

	m, cmd := commitVerb(t, m, "context")
	if m.palArg != keymap.ActionContext {
		t.Fatalf("space should commit the context verb, stage = %q", m.palArg)
	}
	if fl.calls != 0 {
		t.Errorf("the kubeconfig was read on the update loop (%d calls)", fl.calls)
	}
	next, _ := m.Update(pickerMsg(t, cmd))
	m = next.(Model)
	if got := m.cmdPicker.Len(); got != 2 {
		t.Fatalf("the argument stage should be seeded with the two contexts, got %d rows", got)
	}
	if got := pickerLabelFor(t, m, "prod"); !strings.HasPrefix(got, "* ") {
		t.Errorf("the stage should mark the context the shell is on, row = %q", got)
	}
	m = typeInto(t, m, "dev")

	m, selCmd := selectInPalette(t, m)

	if m.cmdPicker.Active() || m.ctxByLabel != nil {
		t.Error("applying the argument should close the palette and drop its label map")
	}
	if selCmd == nil {
		t.Fatal("a picked context should issue the connect Cmd")
	}
	if _, ok := selCmd().(clusterConnectedMsg); !ok {
		t.Fatalf("the argument should route into switchContext, got %T", selCmd())
	}
	if len(fc.names) != 1 || fc.names[0] != "dev" {
		t.Errorf("connected to %v, want [dev] — the row must resolve back to a context name", fc.names)
	}
}

// TestPaletteFetchedValuesDroppedAfterRewind pins why the load is addressed rather
// than delivered to whatever is open: the reader can unwind the line while the values
// are in flight, and a list that lands on a stage that no longer exists must be
// dropped, not painted over the verb list they went back to.
func TestPaletteFetchedValuesDroppedAfterRewind(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	m := sizedWith(t, WithNamespaceLister(fl))

	m, cmd := commitVerb(t, m, "namespace")
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace}) // rewind to the verbs
	if m.palArg != "" {
		t.Fatalf("backspace into an empty argument should leave the stage, stage = %q", m.palArg)
	}
	next, _ := m.Update(pickerMsg(t, cmd))
	m = next.(Model)

	if got, want := m.cmdPicker.Len(), len(paletteVerbs); got != want {
		t.Fatalf("a late list overwrote the verb stage: %d entries, want %d", got, want)
	}
	if !m.cmdPicker.Active() {
		t.Fatal("a late list must not close the palette the reader is still in")
	}
}

// TestPaletteFetchedVerbsStayInertWithoutTheirSeam proves inertness is still decided
// before the stage opens: with no lister wired there is nothing to list, so the verb
// does exactly what its key does — nothing — instead of opening a stage that would
// wait forever for values that were never going to come.
func TestPaletteFetchedVerbsStayInertWithoutTheirSeam(t *testing.T) {
	for _, tc := range []struct{ name, verb string }{
		{"namespace", "namespace"},
		{"context", "context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sized(t) // no lister of either kind
			m, cmd := commitVerb(t, m, tc.verb)
			if m.palArg != "" {
				t.Fatalf("an inert verb should not open an argument stage, stage = %q", m.palArg)
			}
			if cmd != nil {
				t.Fatalf("an inert verb should issue no command, got %v", pickerMsgs(cmd))
			}
			// Since PAL-05c-2 there is no standalone switcher left for an inert verb
			// to open *instead*, so the palette's own verb list — the one it was
			// picked from — is the only overlay that may still be up.
			if p := m.activePicker(); p != &m.cmdPicker {
				t.Fatal("an inert verb should leave the verb list up and open nothing else")
			}
		})
	}
}

// TestPaletteVerbDispatchesThroughHandleAction proves a picked verb runs the same
// dispatch its key runs (D11/D197): menu.toggle picked from the palette hides the
// menu exactly as `m` does, with no palette-specific handler in between.
func TestPaletteVerbDispatchesThroughHandleAction(t *testing.T) {
	m := sized(t)
	if m.menuHidden {
		t.Fatal("the menu should start visible")
	}
	m, _ = press(t, m, colon)
	next, _ := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: keymap.ActionToggleMenu.Describe()})
	m = next.(Model)
	if !m.menuHidden {
		t.Fatal("picking the menu-toggle verb should hide the menu, as `m` does")
	}
	if m.cmdPicker.Active() {
		t.Fatal("running a verb should close the palette")
	}
}

// TestPaletteCancels proves nav.back dismisses the palette without running a verb,
// and drops the label→action map with it.
func TestPaletteCancels(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, colon)
	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("back should emit a cancel command")
	}
	next, _ := m.Update(cancelCmd())
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Fatal("nav.back should close the palette")
	}
	if m.cmdByLabel != nil {
		t.Fatal("cancelling should drop the verb map")
	}
}

// TestPaletteVerbsAreRegisteredAndDistinct pins the two properties openPalette
// relies on: every verb is a registered action (so its label and its dispatch both
// come from the registry), no two share a description (a duplicate label would make
// a pick ambiguous), and the palette never lists itself (which would be a way to
// reopen the surface you are already in).
func TestPaletteVerbsAreRegisteredAndDistinct(t *testing.T) {
	seen := map[string]keymap.Action{}
	for _, a := range paletteVerbs {
		if !a.Valid() {
			t.Errorf("palette verb %q is not a registered action", a)
			continue
		}
		if a == keymap.ActionPalette {
			t.Error("the palette must not list itself")
		}
		desc := a.Describe()
		if desc == "" {
			t.Errorf("palette verb %q has no description to show", a)
		}
		if other, dup := seen[desc]; dup {
			t.Errorf("palette verbs %q and %q share the description %q", other, a, desc)
		}
		seen[desc] = a
	}
}

// rowVerbTitles is the set of row-action titles applicable to the browsed kind — the
// same source the actions menu lists — so a test asserts against what `a` would offer
// rather than against a second hand-written list that could drift from it.
func rowVerbTitles(t *testing.T, m Model) []string {
	t.Helper()
	titles, _ := rowActionTitles(m.current)
	return titles
}

// TestPaletteListsTheSelectedRowsActions is PAL-04's headline: with a row selected,
// `:` offers the row-scoped actions beside the app-global verbs — exactly the set the
// actions menu computes for that kind, so a Pod gets Logs and Exec shell and does not
// get the node-only Cordon.
func TestPaletteListsTheSelectedRowsActions(t *testing.T) {
	m := openPodTable(t, "Pod")
	m, _ = press(t, m, colon)

	for _, title := range []string{"Describe", "Logs", "Exec shell", "View / Edit YAML", rowActionLabel(rowActionDelete)} {
		if _, ok := m.palRowByLabel[title]; !ok {
			t.Errorf("the palette should offer the row verb %q with a Pod row selected", title)
		}
	}
	for _, title := range []string{"Cordon", "Drain", "Suspend"} {
		if _, ok := m.palRowByLabel[title]; ok {
			t.Errorf("the palette should not offer %q — it does not apply to a Pod", title)
		}
	}
	want := len(paletteVerbs) + len(rowVerbTitles(t, m))
	if got := m.cmdPicker.Len(); got != want {
		t.Fatalf("palette seeded with %d entries, want %d (verbs + row actions)", got, want)
	}
}

// TestPaletteGlobalVerbsKeepTheTopOfTheList pins the compatibility rule: the row verbs
// are *appended*, so every line a reader already types resolves to the same verb
// whether or not a row happens to be selected — with an empty query the first entry is
// still the first app-global verb.
func TestPaletteGlobalVerbsKeepTheTopOfTheList(t *testing.T) {
	m := openPodTable(t, "Pod")
	m, _ = press(t, m, colon)
	first, ok := m.cmdPicker.Selected()
	if !ok {
		t.Fatal("the palette should have a highlighted entry")
	}
	if want := paletteVerbs[0].Describe(); first != want {
		t.Fatalf("first palette entry = %q, want the first global verb %q", first, want)
	}
}

// TestPaletteTitleNamesTheRowItWouldActOn is the answer to "which object?" (D205): the
// palette's own title carries the target, on screen, above the verbs that would act on
// it — so a destructive entry can never be picked over an unnamed selection.
func TestPaletteTitleNamesTheRowItWouldActOn(t *testing.T) {
	m := openPodTable(t, "Pod")
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("setup: the table should have a selected row")
	}
	target := viewerTitle(m.current, row.Object)
	m, _ = press(t, m, colon)
	if !strings.Contains(frame(m), target) {
		t.Fatalf("the open palette should name its target %q on screen", target)
	}
}

// TestPaletteTitleIsPlainWithNoRow is the other half: with nothing to act on there is
// no target to name and no row verb to explain, so the title stays the bare one PAL-02
// shipped.
func TestPaletteTitleIsPlainWithNoRow(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, colon)
	if m.palRowByLabel != nil {
		t.Fatal("no row selected → no row verbs")
	}
	if got := frame(m); strings.Contains(got, paletteTitle+paletteTargetSep) {
		t.Fatal("the palette title should carry no target when there is no row to act on")
	}
}

// TestPaletteRowVerbDispatchesTheRowActionIntent proves a row verb picked in the
// palette lands on the same rowActionMsg the actions menu and the direct key dispatch,
// against the selected row's own object — one code path, not a palette-side copy.
func TestPaletteRowVerbDispatchesTheRowActionIntent(t *testing.T) {
	m := openPodTable(t, "Pod")
	row, _ := m.table.SelectedRow()
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "describe")
	if v, _ := m.cmdPicker.Selected(); v != "Describe" {
		t.Fatalf("typing \"describe\" selected %q, want the row verb %q", v, "Describe")
	}

	m, cmd := selectInPalette(t, m)

	if m.cmdPicker.Active() {
		t.Fatal("picking a row verb should close the palette")
	}
	if m.palRowByLabel != nil {
		t.Fatal("closing the palette should drop the row-verb map")
	}
	if cmd == nil {
		t.Fatal("a row verb should dispatch a row-action intent")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("row verb produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionDescribe {
		t.Errorf("intent action = %q, want %q", intent.Action, rowActionDescribe)
	}
	if intent.Object.Name != row.Object.Name || intent.Object.Name == "" {
		t.Errorf("intent object = %q, want the selected row's %q", intent.Object.Name, row.Object.Name)
	}
}

// TestPaletteDeleteVerbStillConfirms is the guard on the one palette entry that can do
// damage: `:delete` goes through the row action's own confirm modal, so the palette
// adds a way to reach delete and no way to skip its confirmation.
func TestPaletteDeleteVerbStillConfirms(t *testing.T) {
	d := &fakeDeleter{}
	m := openPodTable(t, "Pod", WithDeleter(d))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "delete")
	m, cmd := selectInPalette(t, m)
	if cmd == nil {
		t.Fatal("the delete row verb should dispatch an intent")
	}
	next, _ := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if !m.modal.Active() || m.modal.Kind() != deleteModalKind {
		t.Fatal("a delete picked in the palette should open the confirm modal")
	}
	if d.calls != 0 {
		t.Fatal("nothing may be deleted before the modal is accepted")
	}
}

// TestPaletteRowVerbsSurviveLeavingAnArgumentStage proves the verb stage has one
// definition: backspacing out of `:resource ` returns to the *same* list `:` opened,
// row verbs and target title included, rather than to a globals-only stage.
func TestPaletteRowVerbsSurviveLeavingAnArgumentStage(t *testing.T) {
	m := openPodTable(t, "Pod")
	want := len(paletteVerbs) + len(rowVerbTitles(t, m))
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "res")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg != keymap.ActionResources {
		t.Fatalf("setup: space should commit the resource verb, stage = %q", m.palArg)
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if m.palArg != "" {
		t.Fatalf("backspace should return to the verb stage, stage = %q", m.palArg)
	}
	if got := m.cmdPicker.Len(); got != want {
		t.Fatalf("the returned-to verb stage holds %d entries, want %d", got, want)
	}
}

// TestRowVerbTitlesDoNotCollideWithPaletteVerbs pins the assumption the label→value
// maps rest on (D203 pt 3): a palette label names exactly one thing. paletteRowVerbs
// drops a row title an app-global verb already claims, which is a guard against a
// future rename — this test is what says the guard is not currently swallowing an
// action.
func TestRowVerbTitlesDoNotCollideWithPaletteVerbs(t *testing.T) {
	globals := map[string]keymap.Action{}
	for _, a := range paletteVerbs {
		globals[a.Describe()] = a
	}
	for _, meta := range rowActions {
		if other, dup := globals[meta.title]; dup {
			t.Errorf("row action %q shares its label with the palette verb %q — it would be dropped", meta.title, other)
		}
	}
}

// TestActionKeyOpensTheSamePaletteStageAsTheLine is PAL-05d's headline and the
// assertion that makes `a` sugar rather than a second surface (D210 pt 1): the frame
// the key produces is **byte-for-byte** the one `:` `act` `␣` produces. Both go
// through enterPaletteArg, so a divergence here would mean one of the two grew its own
// idea of the stage.
func TestActionKeyOpensTheSamePaletteStageAsTheLine(t *testing.T) {
	keyed := openActionStage(t, openPodTable(t, "Pod"))

	typed, _ := commitVerb(t, openPodTable(t, "Pod"), "act")
	if typed.palArg != keymap.ActionActions {
		t.Fatalf("`:act␣` should commit the action verb, stage = %q", typed.palArg)
	}
	if frame(keyed) != frame(typed) {
		t.Fatalf("`a` and `:act␣` render different stages:\n--- key ---\n%s\n--- line ---\n%s",
			frame(keyed), frame(typed))
	}
}

// TestActionStageNamesTheRowItWouldActOn is D205 pt 2 carried onto the stage `a` opens:
// its chrome names the object, because this is the one surface in kubecom where every
// entry acts on something and one of them deletes it. The verb is *not* repeated in the
// title — the `:action ` prompt already says which stage the line is in.
func TestActionStageNamesTheRowItWouldActOn(t *testing.T) {
	m := openPodTable(t, "Pod")
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("setup: the table should have a selected row")
	}
	target := viewerTitle(m.current, row.Object)

	m = openActionStage(t, m)
	if got := frame(m); !strings.Contains(got, target) {
		t.Fatalf("the action stage should name its target %q on screen, got:\n%s", target, got)
	}
	if got := frame(m); !strings.Contains(got, palettePrompt+paletteArgVerbs[keymap.ActionActions]) {
		t.Fatalf("the action stage should prompt %q, got:\n%s",
			palettePrompt+paletteArgVerbs[keymap.ActionActions]+" ", got)
	}
}

// TestActionStageOffersTheVerbStagesOwnRowSet is the one-source guard (D205 pt 1): the
// set `a` narrows to is the same set `:` appends to its globals, computed by the same
// function — so the two ways in cannot come to disagree about what applies to a kind.
func TestActionStageOffersTheVerbStagesOwnRowSet(t *testing.T) {
	verbStage, _ := press(t, openPodTable(t, "Pod"), colon)
	actionStage := openActionStage(t, openPodTable(t, "Pod"))

	if len(verbStage.palRowByLabel) == 0 {
		t.Fatal("setup: the verb stage should carry row verbs with a Pod row selected")
	}
	if len(actionStage.palRowByLabel) != len(verbStage.palRowByLabel) {
		t.Fatalf("the action stage lists %d row verbs, the verb stage %d — they must be one set",
			len(actionStage.palRowByLabel), len(verbStage.palRowByLabel))
	}
	for title, act := range verbStage.palRowByLabel {
		if got, ok := actionStage.palRowByLabel[title]; !ok || got != act {
			t.Errorf("the action stage should offer %q → %q, got %q (present: %v)", title, act, got, ok)
		}
	}
}

// TestActionStageRewindsToTheVerbList proves `a` is a way *into* the palette rather
// than a faster dead end (D207 pt 2, the half D233 kept): backspace on the empty line
// uncommits the verb and the full list — globals and row verbs — comes back, so a key
// pressed by mistake is one keystroke from everything else. Backspace, not esc: esc
// backs out of the surface (TestActionStageEscClosesToTheTable), backspace edits the
// line, and the gesture that rewinds a line is the editing one.
func TestActionStageRewindsToTheVerbList(t *testing.T) {
	m := openPodTable(t, "Pod")
	want := len(paletteVerbs) + len(rowVerbTitles(t, m))
	m = openActionStage(t, m)

	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if m.palArg != "" {
		t.Fatalf("backspace should return to the verb stage, stage = %q", m.palArg)
	}
	if !m.cmdPicker.Active() {
		t.Fatal("the rewind should not close the palette")
	}
	if got := m.cmdPicker.Len(); got != want {
		t.Fatalf("the returned-to verb stage holds %d entries, want %d", got, want)
	}
}

// TestActionStageEscClosesToTheTable is PAL-07's headline assertion and the feedback's
// exact repro (`2026-08-06-action-menu-esc-behavior`): select a pod, press `a`, press
// esc — once. The palette is gone and the reader is back on the pod list, rather than
// left on a verb list they never asked for with a second esc still owed (D233).
func TestActionStageEscClosesToTheTable(t *testing.T) {
	m := openPodTable(t, "Pod")
	m = openActionStage(t, m)

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("esc should emit a cancel command")
	}
	next, _ := m.Update(cancelCmd())
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Fatal("one esc should close the action menu")
	}
	if m.palArg != "" {
		t.Fatalf("closing should clear the stage, stage = %q", m.palArg)
	}
	if m.palRowByLabel != nil {
		t.Errorf("closing should drop the row map: %v", m.palRowByLabel)
	}
	// "Return focus to where I was": the browse table is still the surface under the
	// closed overlay, with the row that was selected still selected.
	if !m.hasCurrent {
		t.Fatal("esc out of the action menu should leave the browse table showing")
	}
	if _, ok := m.table.SelectedRow(); !ok {
		t.Fatal("esc out of the action menu should leave the row selected")
	}
	if got := stripANSI(m.View().Content); !strings.Contains(got, "pod-b") {
		t.Errorf("the pod list should be back on screen:\n%s", got)
	}
}

// TestActionStageEscClearsATypedQueryFirst records the product call the feedback asked
// to be made explicitly (D233 pt 3): with a query typed into the action stage, the
// first esc clears the query — that is what esc means in a filter field, in this and
// every other picker — and the *next* one closes. The complaint was never about a typed
// query; it was that esc bought nothing when nothing had been typed.
func TestActionStageEscClearsATypedQueryFirst(t *testing.T) {
	m := openPodTable(t, "Pod")
	m = openActionStage(t, m)
	m = typeInto(t, m, "log")
	if m.cmdPicker.Query() == "" {
		t.Fatal("typing should reach the stage's query")
	}

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd != nil {
		t.Fatal("the esc that clears the query should not also cancel the picker")
	}
	if !m.cmdPicker.Active() || m.palArg != keymap.ActionActions {
		t.Fatalf("clearing the query should keep the action stage up, stage = %q", m.palArg)
	}
	if got := m.cmdPicker.Query(); got != "" {
		t.Fatalf("esc should clear the query, got %q", got)
	}

	m, cancelCmd = press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("the second esc should emit a cancel command")
	}
	next, _ := m.Update(cancelCmd())
	if next.(Model).cmdPicker.Active() {
		t.Fatal("the second esc should close the action menu")
	}
}

// --- PAL-08: the palette names its commands ----------------------------------

// paletteRow returns the palette row holding want, through the real program's frame —
// the feedback is about what is on screen, so the frame is what these tests read. The
// modal is composited over the two-pane browse view (D95), so the row is the bordered
// field the text lands in, not the whole screen line.
func paletteRow(t *testing.T, m Model, want string) string {
	t.Helper()
	for _, l := range strings.Split(frame(m), "\n") {
		for _, field := range strings.Split(l, "│") {
			if strings.Contains(field, want) {
				return field
			}
		}
	}
	t.Fatalf("no palette row holds %q; got:\n%s", want, frame(m))
	return ""
}

// TestPaletteRowShowsTheCommandName is the feedback's own words — "I want to see the
// command name too, not just its description" — asserted through the surface it was
// written about: `:` and the row for a verb carries the action id the rest of kubecom
// calls it by, ahead of the description it used to show alone (D237).
func TestPaletteRowShowsTheCommandName(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, colon)
	row := paletteRow(t, m, keymap.ActionNamespace.Describe())
	if !strings.HasPrefix(strings.TrimSpace(row), string(keymap.ActionNamespace)) {
		t.Fatalf("the namespace row should open with %q, got %q", keymap.ActionNamespace, row)
	}
	if i, j := strings.Index(row, string(keymap.ActionNamespace)), strings.Index(row, keymap.ActionNamespace.Describe()); i >= j {
		t.Fatalf("name at %d, description at %d — the name column must come first: %q", i, j, row)
	}
}

// TestPaletteRowNameIsTypeable is the half that makes the column worth more than
// decoration: the id on screen is matched, so a reader can narrow by what a command is
// called. `ns.sw` is not a subsequence of "Switch namespace" — before PAL-08 typing
// the visible name found nothing.
func TestPaletteRowNameIsTypeable(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "ns.sw")
	if got := m.cmdPicker.Len(); got != 1 {
		t.Fatalf("typing the command name matched %d verbs, want 1", got)
	}
	if v, _ := m.cmdPicker.Selected(); v != keymap.ActionNamespace.Describe() {
		t.Fatalf("selected %q, want %q", v, keymap.ActionNamespace.Describe())
	}
}

// TestActionStageRowsShowTheRowActionName carries the same claim onto the one stage
// whose entries are not registry actions: a row verb is named by its own id — the name
// the `:action ` line already takes (D210) — so the stage `a` opens has a name column
// too rather than reverting to bare titles.
func TestActionStageRowsShowTheRowActionName(t *testing.T) {
	m := openActionStage(t, openPodTable(t, "Pod"))
	row := paletteRow(t, m, rowActionLabel(rowActionDescribe))
	if !strings.HasPrefix(strings.TrimSpace(row), string(rowActionDescribe)) {
		t.Fatalf("the describe row should open with %q, got %q", rowActionDescribe, row)
	}
}
