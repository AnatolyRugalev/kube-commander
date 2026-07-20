package table

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newTestModel() Model { return New(styles.Default()) }

// sampleTable is a small three-row, two-column pod-like table with a hidden
// priority column, used across the navigation/render tests.
func sampleTable() kube.Table {
	return kube.Table{
		Columns: []kube.Column{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Ready", Type: "string"},
			{Name: "IP", Type: "string", Priority: 1}, // wide-only, hidden by default
		},
		Rows: []kube.Row{
			{Cells: []any{"pod-a", "1/1", "10.0.0.1"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-a", UID: "a"}},
			{Cells: []any{"pod-b", "0/1", "10.0.0.2"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-b", UID: "b"}},
			{Cells: []any{"pod-c", "2/2", "10.0.0.3"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-c", UID: "c"}},
		},
	}
}

func TestSetTableSelectsFirstRow(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3", m.RowCount())
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	row, ok := m.SelectedRow()
	if !ok {
		t.Fatal("SelectedRow returned ok=false on a non-empty table")
	}
	if row.Object.Name != "pod-a" {
		t.Fatalf("selected row = %q, want pod-a", row.Object.Name)
	}
}

func TestEmptyTableSelection(t *testing.T) {
	m := newTestModel()
	if _, ok := m.SelectedRow(); ok {
		t.Fatal("empty table: SelectedRow should report ok=false")
	}
	// Actions on an empty table are no-ops (and must not panic).
	m, cmd := m.Update(keymap.ActionDown)
	if cmd != nil {
		t.Fatal("empty table: Down should not emit a command")
	}
	if m.cursor != 0 {
		t.Fatalf("empty table: cursor = %d, want 0", m.cursor)
	}
}

func TestPriorityColumnsHiddenByDefault(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	if len(m.visible) != 2 {
		t.Fatalf("visible columns = %d, want 2 (priority-0 only)", len(m.visible))
	}
	m.SetSize(40, 8)
	out := m.View()
	if !strings.Contains(out, "Name") || !strings.Contains(out, "Ready") {
		t.Fatalf("header missing priority-0 columns:\n%s", out)
	}
	if strings.Contains(out, "10.0.0.1") {
		t.Fatalf("priority>0 cell 10.0.0.1 should be hidden:\n%s", out)
	}
}

func TestAllColumnsWhenNoPriorityZero(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{
		Columns: []kube.Column{
			{Name: "A", Priority: 1},
			{Name: "B", Priority: 2},
		},
		Rows: []kube.Row{{Cells: []any{"x", "y"}}},
	})
	if len(m.visible) != 2 {
		t.Fatalf("visible = %d, want 2 (fallback to all when no priority-0)", len(m.visible))
	}
}

func TestNavigationClampsAndJumps(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	last := m.RowCount() - 1

	// Up at the top stays at 0.
	m, _ = m.Update(keymap.ActionUp)
	if m.cursor != 0 {
		t.Fatalf("up at top: cursor = %d, want 0", m.cursor)
	}
	// Down moves.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != 1 {
		t.Fatalf("down: cursor = %d, want 1", m.cursor)
	}
	// Bottom jumps to last.
	m, _ = m.Update(keymap.ActionBottom)
	if m.cursor != last {
		t.Fatalf("bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Down at the bottom stays.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != last {
		t.Fatalf("down at bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Top jumps back.
	m, _ = m.Update(keymap.ActionTop)
	if m.cursor != 0 {
		t.Fatalf("top: cursor = %d, want 0", m.cursor)
	}
}

func TestDrillInEmitsRowSelected(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b

	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd == nil {
		t.Fatal("drillIn should emit a command")
	}
	msg := cmd()
	sel, ok := msg.(RowSelectedMsg)
	if !ok {
		t.Fatalf("drillIn emitted %T, want RowSelectedMsg", msg)
	}
	if sel.Row.Object.Name != "pod-b" {
		t.Fatalf("selected row = %q, want pod-b", sel.Row.Object.Name)
	}
}

func TestVerticalScrollKeepsCursorVisible(t *testing.T) {
	m := newTestModel()
	// 10 rows in a pane that shows only a few data lines forces scrolling.
	rows := make([]kube.Row, 10)
	for i := range rows {
		name := string(rune('a' + i))
		rows[i] = kube.Row{Cells: []any{name}, Object: kube.ObjectRef{Name: name}}
	}
	m.SetTable(kube.Table{Columns: []kube.Column{{Name: "N"}}, Rows: rows})
	m.SetSize(20, 5) // inner height 3 → 1 header + 2 data rows

	if got := m.dataHeight(); got != 2 {
		t.Fatalf("dataHeight = %d, want 2", got)
	}
	// Jump to the bottom: offset must have advanced so the last row is visible.
	m, _ = m.Update(keymap.ActionBottom)
	if m.cursor != 9 {
		t.Fatalf("cursor = %d, want 9", m.cursor)
	}
	if m.cursor < m.offset || m.cursor >= m.offset+m.dataHeight() {
		t.Fatalf("cursor %d not within visible window [%d,%d)", m.cursor, m.offset, m.offset+m.dataHeight())
	}
	// The rendered view must contain the selected (last) row and not the first.
	out := m.View()
	if !strings.Contains(out, "j") { // 10th row is 'j'
		t.Fatalf("bottom row 'j' not visible after scroll:\n%s", out)
	}
}

func TestHalfPageScroll(t *testing.T) {
	m := newTestModel()
	rows := make([]kube.Row, 20)
	for i := range rows {
		rows[i] = kube.Row{Cells: []any{string(rune('a' + i))}}
	}
	m.SetTable(kube.Table{Columns: []kube.Column{{Name: "N"}}, Rows: rows})
	m.SetSize(20, 9) // inner 7 → 1 header + 6 data → pageStep 6, half 3

	m, _ = m.Update(keymap.ActionHalfPageDown)
	if m.cursor != 3 {
		t.Fatalf("half-page down: cursor = %d, want 3", m.cursor)
	}
	m, _ = m.Update(keymap.ActionPageDown)
	if m.cursor != 9 {
		t.Fatalf("page down: cursor = %d, want 9", m.cursor)
	}
	m, _ = m.Update(keymap.ActionHalfPageUp)
	if m.cursor != 6 {
		t.Fatalf("half-page up: cursor = %d, want 6", m.cursor)
	}
}

func TestViewEmptyUntilSized(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	if got := m.View(); got != "" {
		t.Fatalf("View before sizing = %q, want empty", got)
	}
}

// TestViewFitsPaneNoWrap guards D58: a row wider than the pane must be clipped,
// not wrapped, so the rendered block is exactly width×height (the bordered Pane
// style counts its border inside Width/Height, so the frame is sized to the total,
// not the inner region).
func TestViewFitsPaneNoWrap(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "NAME"}, {Name: "STATUS"}},
		Rows: []kube.Row{
			{Cells: []any{"a-very-long-object-name-that-overflows", "CrashLoopBackOff"}},
			{Cells: []any{"another-really-long-one", "Running"}},
		},
	})
	const w, h = 24, 6
	m.SetSize(w, h)
	out := m.View()
	if gw := lipgloss.Width(out); gw != w {
		t.Fatalf("View width = %d, want %d (wrapping/overflow):\n%s", gw, w, out)
	}
	if gh := lipgloss.Height(out); gh != h {
		t.Fatalf("View height = %d, want %d (wrapping/overflow):\n%s", gh, h, out)
	}
}

func TestFocusChangesBorder(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetSize(30, 8)
	unfocused := m.View()
	m.Focus()
	if !m.Focused() {
		t.Fatal("Focused() should be true after Focus()")
	}
	focused := m.View()
	if unfocused == focused {
		t.Fatal("focused and unfocused views should differ (accented border)")
	}
}

func TestFormatCell(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{float64(3), "3"},     // integral JSON number → no ".0"
		{float64(2.5), "2.5"}, // fractional preserved
		{int64(7), "7"},       // explicit int64
	}
	for _, c := range cases {
		if got := formatCell(c.in); got != c.want {
			t.Errorf("formatCell(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShortRowDegradesToBlanks(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "A"}, {Name: "B"}},
		Rows:    []kube.Row{{Cells: []any{"only-one"}}}, // fewer cells than columns
	})
	m.SetSize(30, 6)
	// Must not panic and must still render the present cell.
	out := m.View()
	if !strings.Contains(out, "only-one") {
		t.Fatalf("short row's present cell missing:\n%s", out)
	}
}

// row builds a one-column pod-like row with the given name and UID for the
// ApplyEvent tests.
func row(name, uid string) kube.Row {
	return kube.Row{Cells: []any{name}, Object: kube.ObjectRef{Name: name, UID: uid}}
}

func TestApplyEventResetReplacesTable(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b (UID "b")

	// A RESET carrying a fresh column set + rows, still containing pod-b.
	m.ApplyEvent(kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "Name", Format: "name"}},
		Rows:    []kube.Row{row("pod-x", "x"), row("pod-b", "b"), row("pod-y", "y")},
	})
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3 after reset", m.RowCount())
	}
	// Selection follows pod-b to its new index (1), not reset to the top.
	if sel, _ := m.SelectedRow(); sel.Object.UID != "b" {
		t.Fatalf("selection = %q, want b (preserved across reset)", sel.Object.UID)
	}
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestApplyEventResetSelectionGoneFallsBackToTop(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionBottom) // select pod-c (UID "c")

	// RESET without pod-c: the selected object is gone.
	m.ApplyEvent(kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "Name", Format: "name"}},
		Rows:    []kube.Row{row("pod-x", "x")},
	})
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (selection gone → clamp to top)", m.cursor)
	}
}

func TestApplyEventAddedAppendsPreservingSelection(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b (index 1)

	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchAdded, Rows: []kube.Row{row("pod-d", "d")}})
	if m.RowCount() != 4 {
		t.Fatalf("RowCount = %d, want 4 after add", m.RowCount())
	}
	// The new row is appended and the selection stays on pod-b.
	if sel, _ := m.SelectedRow(); sel.Object.UID != "b" {
		t.Fatalf("selection = %q, want b (unchanged by an add)", sel.Object.UID)
	}
	if m.table.Rows[3].Object.UID != "d" {
		t.Fatalf("appended row = %q, want d", m.table.Rows[3].Object.UID)
	}
}

func TestApplyEventModifiedUpdatesRowInPlace(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())

	// Modify pod-b's first cell; it must update in place, not append.
	updated := kube.Row{Cells: []any{"pod-b", "1/1", "10.0.0.9"}, Object: kube.ObjectRef{Name: "pod-b", UID: "b"}}
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchModified, Rows: []kube.Row{updated}})
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3 (modify updates in place)", m.RowCount())
	}
	if got := m.table.Rows[1].Cells[1]; got != "1/1" {
		t.Fatalf("pod-b Ready cell = %v, want 1/1 after modify", got)
	}
}

func TestApplyEventModifiedUnknownUIDAppends(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())

	// A MODIFIED for a UID we've never seen behaves like an add (server may send a
	// modify for an object that entered scope before the watch synced it).
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchModified, Rows: []kube.Row{row("pod-z", "z")}})
	if m.RowCount() != 4 {
		t.Fatalf("RowCount = %d, want 4 (unknown modify → append)", m.RowCount())
	}
}

func TestApplyEventDeletedRemovesRowAndKeepsPosition(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b (index 1)

	// Delete pod-a (index 0, above the cursor): pod-b shifts to index 0 and the
	// selection follows it by UID.
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchDeleted, Rows: []kube.Row{row("pod-a", "a")}})
	if m.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2 after delete", m.RowCount())
	}
	if sel, _ := m.SelectedRow(); sel.Object.UID != "b" {
		t.Fatalf("selection = %q, want b (followed across a delete above it)", sel.Object.UID)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestApplyEventDeleteSelectedKeepsIndex(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b (index 1)

	// Delete the selected row: the cursor keeps its index, now pointing at pod-c
	// (the row that took pod-b's slot), rather than jumping to the top.
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchDeleted, Rows: []kube.Row{row("pod-b", "b")}})
	if m.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2", m.RowCount())
	}
	if sel, _ := m.SelectedRow(); sel.Object.UID != "c" {
		t.Fatalf("selection = %q, want c (cursor holds its index after deleting itself)", sel.Object.UID)
	}
}

func TestApplyEventDeleteLastRowClampsCursor(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionBottom) // select pod-c (last, index 2)

	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchDeleted, Rows: []kube.Row{row("pod-c", "c")}})
	if m.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2", m.RowCount())
	}
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (clamped to new last row)", m.cursor)
	}
}

func TestApplyEventDeleteToEmptyResetsCursor(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{Columns: []kube.Column{{Name: "N"}}, Rows: []kube.Row{row("only", "o")}})

	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchDeleted, Rows: []kube.Row{row("only", "o")}})
	if m.RowCount() != 0 {
		t.Fatalf("RowCount = %d, want 0", m.RowCount())
	}
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("cursor/offset = %d/%d, want 0/0 on an emptied table", m.cursor, m.offset)
	}
	if _, ok := m.SelectedRow(); ok {
		t.Fatal("emptied table: SelectedRow should report ok=false")
	}
}

func TestApplyEventRecomputesColumnWidth(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{Columns: []kube.Column{{Name: "N"}}, Rows: []kube.Row{row("ab", "1")}})
	before := m.colWidths[0]

	// Add a much longer value: the column must widen to fit it.
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchAdded, Rows: []kube.Row{row("a-much-longer-name", "2")}})
	if m.colWidths[0] <= before {
		t.Fatalf("colWidths[0] = %d, want > %d (widened for the longer cell)", m.colWidths[0], before)
	}
	if m.colWidths[0] != runeLen("a-much-longer-name") {
		t.Fatalf("colWidths[0] = %d, want %d", m.colWidths[0], runeLen("a-much-longer-name"))
	}
}

func TestApplyEventDeleteMissingUIDIsNoOp(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchDeleted, Rows: []kube.Row{row("ghost", "gone")}})
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3 (deleting an absent UID is a no-op)", m.RowCount())
	}
}

// Update must return a table.Model (not tea.Model) so the root can keep a typed
// value; this compile-time check guards the signature.
var _ = func(m Model) (Model, tea.Cmd) { return m.Update(keymap.ActionDown) }
