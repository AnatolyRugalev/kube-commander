package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// unhealthyKey is the default app.unhealthy key (`H`, shift+h — the feedback's own
// suggestion for the "healthy vs h" quick-access, STORY-06g-1).
var unhealthyKey = tea.Key{Code: 'h', ShiftedCode: 'H', Mod: tea.ModShift}

// TestUnhealthyKeyNarrowsAndRestores proves the app.unhealthy gesture end to end:
// `H` on a resource table narrows it to the rows the M4-06 classifier reads as
// unhealthy (the sortReset table has pod-a Pending → unhealthy, pod-b Running →
// healthy), the status bar shows the `unhealthy` marker, and `H` again restores
// every row. The full set is untouched either way.
func TestUnhealthyKeyNarrowsAndRestores(t *testing.T) {
	m := openSortTable(t)
	if m.table.TotalRowCount() != 2 {
		t.Fatalf("precondition: total rows = %d, want 2", m.table.TotalRowCount())
	}

	m, _ = press(t, m, unhealthyKey)
	if !m.table.UnhealthyOnly() {
		t.Fatal("`H` should turn the unhealthy-only view on")
	}
	if got, want := m.table.RowCount(), 1; got != want {
		t.Fatalf("RowCount = %d, want %d — only Pending pod-a is unhealthy", got, want)
	}
	if m.table.TotalRowCount() != 2 {
		t.Fatalf("the unhealthy view must not drop rows from the full set: total %d, want 2", m.table.TotalRowCount())
	}
	if sel, _ := m.table.SelectedRow(); sel.Object.Name != "pod-a" {
		t.Fatalf("selection = %q, want pod-a (the unhealthy row)", sel.Object.Name)
	}
	if !strings.Contains(m.status.View(), "unhealthy") {
		t.Fatalf("the status bar should carry the unhealthy marker while the view is on:\n%q", m.status.View())
	}

	// `H` again is the toggle off — every row returns.
	m, _ = press(t, m, unhealthyKey)
	if m.table.UnhealthyOnly() {
		t.Fatal("a second `H` should turn the unhealthy-only view off")
	}
	if got, want := m.table.RowCount(), 2; got != want {
		t.Fatalf("RowCount = %d, want %d after toggling off", got, want)
	}
	if strings.Contains(m.status.View(), "unhealthy") {
		t.Fatalf("the status bar marker should clear with the view:\n%q", m.status.View())
	}
}

// TestUnhealthyKeyEscClears proves esc is the "show me everything again" gesture
// for the unhealthy view exactly as it is for the substring filter (STORY-06g-1):
// with the view on, nav.back restores the full table.
func TestUnhealthyKeyEscClears(t *testing.T) {
	m := openSortTable(t)
	m, _ = press(t, m, unhealthyKey)
	if m.table.RowCount() != 1 {
		t.Fatalf("precondition: unhealthy view left %d rows, want 1", m.table.RowCount())
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.table.UnhealthyOnly() {
		t.Fatal("esc should clear the unhealthy-only view")
	}
	if got, want := m.table.RowCount(), 2; got != want {
		t.Fatalf("RowCount = %d, want %d after esc", got, want)
	}
}

// TestUnhealthyInertWithoutTable proves `H` is a no-op on the welcome page, like
// `/`: there is no resource table to narrow.
func TestUnhealthyInertWithoutTable(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, unhealthyKey)
	if m.table.UnhealthyOnly() {
		t.Fatal("`H` should be inert with no current table")
	}
}

// TestUnhealthyKeyReachableFromMenuPane proves `H` is app-global — one keypress
// from anywhere in browse, not just while the table holds focus: with the menu
// pane focused, `H` still narrows the current resource table.
func TestUnhealthyKeyReachableFromMenuPane(t *testing.T) {
	m := openSortTable(t)
	// Focus the menu pane, as browse starts.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = next.(Model)
	if m.menu.Focused() || m.table.Focused() {
		// whichever pane holds focus, drive focus to the menu explicitly.
		m.menu.Focus()
		m.table.Blur()
	}
	if !m.menu.Focused() {
		t.Fatal("precondition: menu pane should hold focus")
	}
	m, _ = press(t, m, unhealthyKey)
	if !m.table.UnhealthyOnly() {
		t.Fatal("`H` should narrow the table even while the menu pane holds focus")
	}
	if got, want := m.table.RowCount(), 1; got != want {
		t.Fatalf("RowCount = %d, want %d", got, want)
	}
}
