package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// TestPaletteOpensOnColon proves `:` opens the command palette (PAL-02) — not the
// resource picker it used to open — seeded with the curated verb list, with its
// filter open from the moment it appears (PAL-01/D194 pt 1).
func TestPaletteOpensOnColon(t *testing.T) {
	m := sized(t)
	m, cmd := press(t, m, colon)
	if !m.cmdPicker.Active() {
		t.Fatal("app.palette should open the command palette")
	}
	if m.resPicker.Active() {
		t.Fatal("`:` should no longer open the resource picker directly")
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
func selectInPalette(t *testing.T, m Model) Model {
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
	next, _ := m.Update(sel)
	return next.(Model)
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

	m = selectInPalette(t, m)

	if !m.cmdPicker.Active() {
		t.Fatal("an argument verb should keep the palette open, not close it")
	}
	if m.resPicker.Active() {
		t.Fatal("the argument stage should be the palette itself, not the standalone resource picker")
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

	m = selectInPalette(t, m)

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

	m = selectInPalette(t, m)

	if m.cmdPicker.Active() {
		t.Fatal("applying an argument should close the palette")
	}
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("theme = %q, want monokai", got)
	}
	if m.themePicker.Active() {
		t.Fatal("the theme's argument stage should never open the standalone theme picker")
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

// TestPaletteEscRewindsThenCloses proves esc walks the same path back out: one esc
// returns the argument stage to the verb list, the next closes the palette.
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
	m = selectInPalette(t, m)
	if m.palArg != "" {
		t.Fatalf("an inert verb should not open an argument stage, stage = %q", m.palArg)
	}
	if m.cmdPicker.Active() || m.resPicker.Active() {
		t.Fatal("an inert verb should leave nothing open")
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
