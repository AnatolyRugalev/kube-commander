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

// TestPaletteRunsTheResourceVerb is the D68 guard for this slice: `:resource` must
// stay reachable now that `:` no longer opens the resource picker itself. It types
// the verb, runs it, and asserts the resource picker is what opens — the whole
// keystroke path, through the real key routing.
func TestPaletteRunsTheResourceVerb(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, colon)
	for _, r := range "resource" {
		m, _ = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	if v, _ := m.cmdPicker.Selected(); v != keymap.ActionResources.Describe() {
		t.Fatalf("typing \"resource\" selected %q, want %q", v, keymap.ActionResources.Describe())
	}

	m, selCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	sel, ok := selCmd().(picker.SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want picker.SelectedMsg", selCmd())
	}
	if sel.Kind != commandPickerKind {
		t.Fatalf("palette selection Kind = %q, want %q", sel.Kind, commandPickerKind)
	}
	next, _ := m.Update(sel)
	m = next.(Model)

	if m.cmdPicker.Active() {
		t.Fatal("running a verb should close the palette")
	}
	if !m.resPicker.Active() {
		t.Fatal("the resource verb should open the resource picker")
	}
	if got := m.resPicker.Len(); got != availableResourceCount(m) {
		t.Fatalf("resource picker seeded with %d entries, want %d", got, availableResourceCount(m))
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
