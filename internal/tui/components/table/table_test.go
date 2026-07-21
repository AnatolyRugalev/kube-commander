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

// TestSelectWrapCyclesRows proves SelectNextWrap/SelectPrevWrap step the selection
// and wrap at the ends (the search-match iteration behind n/N, M2-09b).
func TestSelectWrapCyclesRows(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable()) // 3 rows, cursor at 0
	m.SetSize(40, 8)

	m.SelectNextWrap()
	if m.Cursor() != 1 {
		t.Fatalf("SelectNextWrap: cursor = %d, want 1", m.Cursor())
	}
	m.SelectNextWrap()
	if m.Cursor() != 2 {
		t.Fatalf("SelectNextWrap: cursor = %d, want 2", m.Cursor())
	}
	m.SelectNextWrap() // wraps 2 -> 0
	if m.Cursor() != 0 {
		t.Fatalf("SelectNextWrap at the last row should wrap to 0, got %d", m.Cursor())
	}
	m.SelectPrevWrap() // wraps 0 -> 2
	if m.Cursor() != 2 {
		t.Fatalf("SelectPrevWrap at the first row should wrap to 2, got %d", m.Cursor())
	}
}

// TestSelectWrapEmptyTableNoop proves the wrap helpers are safe on an empty table.
func TestSelectWrapEmptyTableNoop(t *testing.T) {
	m := newTestModel()
	m.SelectNextWrap()
	m.SelectPrevWrap()
	if m.Cursor() != 0 {
		t.Fatalf("empty-table wrap should leave the cursor at 0, got %d", m.Cursor())
	}
}

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

// wideTable is a three-column table whose columns are each 8 wide (from the cell
// values), giving column starts [0, 10, 20] and a total content width of 28 —
// wide enough to exercise horizontal scroll in a narrow pane.
func wideTable() kube.Table {
	return kube.Table{
		Columns: []kube.Column{{Name: "C1"}, {Name: "C2"}, {Name: "C3"}},
		Rows: []kube.Row{
			{Cells: []any{"aaaaaaaa", "bbbbbbbb", "cccccccc"}, Object: kube.ObjectRef{Name: "r1", UID: "1"}},
		},
	}
}

func TestHorizontalScrollSnapsToColumnsAndClamps(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(14, 6) // innerW 12; content 28 → maxHOffset 16, column starts [0,10,20]

	if m.HOffset() != 0 {
		t.Fatalf("initial HOffset = %d, want 0", m.HOffset())
	}
	// Right snaps to the next column start (10), then to maxHOffset (16, since the
	// last column start 20 is past the content edge), then stops.
	for _, want := range []int{10, 16, 16} {
		m, _ = m.Update(keymap.ActionRight)
		if m.HOffset() != want {
			t.Fatalf("after right: HOffset = %d, want %d", m.HOffset(), want)
		}
	}
	// The rightmost column is now visible, the leftmost scrolled off.
	out := m.View()
	if !strings.Contains(out, "cccccccc") {
		t.Fatalf("rightmost column not visible after scrolling right:\n%s", out)
	}
	if strings.Contains(out, "aaaaaaaa") {
		t.Fatalf("leftmost column should be scrolled off:\n%s", out)
	}
	// Left retreats to the previous column start (10), then to 0, then stops.
	for _, want := range []int{10, 0, 0} {
		m, _ = m.Update(keymap.ActionLeft)
		if m.HOffset() != want {
			t.Fatalf("after left: HOffset = %d, want %d", m.HOffset(), want)
		}
	}
	out = m.View()
	if !strings.Contains(out, "aaaaaaaa") {
		t.Fatalf("leftmost column not visible after scrolling back:\n%s", out)
	}
}

func TestHorizontalScrollNoOpWhenContentFits(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(40, 6) // innerW 38 ≥ content 28 → nothing to scroll

	m, _ = m.Update(keymap.ActionRight)
	if m.HOffset() != 0 {
		t.Fatalf("HOffset = %d, want 0 (content fits, right is a no-op)", m.HOffset())
	}
	m, _ = m.Update(keymap.ActionLeft)
	if m.HOffset() != 0 {
		t.Fatalf("HOffset = %d, want 0 (left is a no-op)", m.HOffset())
	}
}

func TestHorizontalScrollReturnsNilCmd(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(14, 6)
	if _, cmd := m.Update(keymap.ActionRight); cmd != nil {
		t.Fatal("right should not emit a command")
	}
	if _, cmd := m.Update(keymap.ActionLeft); cmd != nil {
		t.Fatal("left should not emit a command")
	}
}

func TestSetTableResetsHorizontalScroll(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(14, 6)
	m, _ = m.Update(keymap.ActionRight)
	if m.HOffset() == 0 {
		t.Fatal("precondition: expected a non-zero HOffset after scrolling right")
	}
	m.SetTable(wideTable()) // a fresh resource resets horizontal scroll to the left
	if m.HOffset() != 0 {
		t.Fatalf("HOffset = %d, want 0 after SetTable", m.HOffset())
	}
}

func TestResizeWiderClampsHorizontalScroll(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(14, 6)
	m, _ = m.Update(keymap.ActionRight)
	m, _ = m.Update(keymap.ActionRight) // scrolled to maxHOffset (16)
	if m.HOffset() == 0 {
		t.Fatal("precondition: expected a non-zero HOffset before widening")
	}
	m.SetSize(40, 6) // now the whole table fits → offset must clamp back to 0
	if m.HOffset() != 0 {
		t.Fatalf("HOffset = %d, want 0 after widening the pane", m.HOffset())
	}
}

func TestApplyEventNarrowingClampsHorizontalScroll(t *testing.T) {
	m := newTestModel()
	m.SetTable(wideTable())
	m.SetSize(14, 6)
	m, _ = m.Update(keymap.ActionRight)
	m, _ = m.Update(keymap.ActionRight)
	if m.HOffset() == 0 {
		t.Fatal("precondition: expected a non-zero HOffset before the narrowing reset")
	}
	// A RESET to a single narrow column: content now fits, so the offset clamps.
	m.ApplyEvent(kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "N"}},
		Rows:    []kube.Row{row("x", "x")},
	})
	if m.HOffset() != 0 {
		t.Fatalf("HOffset = %d, want 0 after a narrowing reset", m.HOffset())
	}
}

func TestHorizontalScrollWorksWithHeaderOnly(t *testing.T) {
	m := newTestModel()
	// A header-only table (columns, no rows yet) whose headers overflow the pane.
	m.SetTable(kube.Table{Columns: []kube.Column{
		{Name: "LONGHEADER-1"}, {Name: "LONGHEADER-2"}, {Name: "LONGHEADER-3"},
	}})
	m.SetSize(14, 6)
	m, _ = m.Update(keymap.ActionRight)
	if m.HOffset() == 0 {
		t.Fatal("header-only table should still scroll horizontally")
	}
}

// Update must return a table.Model (not tea.Model) so the root can keep a typed
// value; this compile-time check guards the signature.
var _ = func(m Model) (Model, tea.Cmd) { return m.Update(keymap.ActionDown) }

// --- M2-09a: filter core -------------------------------------------------------

func TestSetFilterNarrowsRows(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("pod-b")
	if got, want := m.RowCount(), 1; got != want {
		t.Fatalf("RowCount = %d, want %d after filtering to pod-b", got, want)
	}
	if m.TotalRowCount() != 3 {
		t.Fatalf("TotalRowCount = %d, want 3 (full set unchanged)", m.TotalRowCount())
	}
	sel, ok := m.SelectedRow()
	if !ok || sel.Object.Name != "pod-b" {
		t.Fatalf("selected row = %+v ok=%v, want pod-b", sel.Object, ok)
	}
	if m.Filter() != "pod-b" {
		t.Fatalf("Filter() = %q, want pod-b", m.Filter())
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("POD-C")
	if m.RowCount() != 1 {
		t.Fatalf("RowCount = %d, want 1 for case-insensitive POD-C", m.RowCount())
	}
	if sel, _ := m.SelectedRow(); sel.Object.Name != "pod-c" {
		t.Fatalf("selected = %q, want pod-c", sel.Object.Name)
	}
}

func TestFilterMatchesAnyVisibleCell(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	// "0/1" is only pod-b's Ready cell — a non-name visible column.
	m.SetFilter("0/1")
	if m.RowCount() != 1 {
		t.Fatalf("RowCount = %d, want 1 matching the Ready column", m.RowCount())
	}
	if sel, _ := m.SelectedRow(); sel.Object.Name != "pod-b" {
		t.Fatalf("selected = %q, want pod-b", sel.Object.Name)
	}
}

func TestFilterIgnoresHiddenColumns(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	// 10.0.0.2 lives only in the hidden priority-1 IP column, so it must not match.
	m.SetFilter("10.0.0.2")
	if m.RowCount() != 0 {
		t.Fatalf("RowCount = %d, want 0 — hidden column must not match", m.RowCount())
	}
}

func TestClearFilterRestoresAllRows(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("pod-b")
	m.ClearFilter()
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3 after clearing the filter", m.RowCount())
	}
	if m.Filter() != "" {
		t.Fatalf("Filter() = %q, want empty after ClearFilter", m.Filter())
	}
	// The selection returns to the previously-selected row (still present).
	if sel, _ := m.SelectedRow(); sel.Object.Name != "pod-b" {
		t.Fatalf("selected = %q, want pod-b preserved across clear", sel.Object.Name)
	}
}

func TestFilterPreservesSelectionByUID(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionDown) // select pod-b (row 1)
	// A filter that keeps pod-b (and pod-c) must keep the cursor on pod-b.
	m.SetFilter("pod-") // matches all three
	if sel, _ := m.SelectedRow(); sel.Object.Name != "pod-b" {
		t.Fatalf("selected = %q, want pod-b preserved when it still matches", sel.Object.Name)
	}
}

func TestFilterSelectionFallsBackWhenHidden(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m, _ = m.Update(keymap.ActionBottom) // select pod-c (last row, index 2)
	m.SetFilter("pod-a")                 // hides pod-c; only pod-a remains
	if m.RowCount() != 1 {
		t.Fatalf("RowCount = %d, want 1", m.RowCount())
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 clamped into the narrowed range", m.cursor)
	}
	if sel, _ := m.SelectedRow(); sel.Object.Name != "pod-a" {
		t.Fatalf("selected = %q, want pod-a", sel.Object.Name)
	}
}

func TestFilterToZeroRowsIsNavigableNoop(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("no-such-pod")
	if m.RowCount() != 0 {
		t.Fatalf("RowCount = %d, want 0", m.RowCount())
	}
	if _, ok := m.SelectedRow(); ok {
		t.Fatal("SelectedRow should report ok=false when the filter matches nothing")
	}
	// Navigation over an empty filtered set is a no-op, not a panic.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 on an empty filtered set", m.cursor)
	}
}

func TestSetTableClearsFilter(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("pod-b")
	// A fresh snapshot (new resource) must drop the stale filter and show all rows.
	m.SetTable(sampleTable())
	if m.Filter() != "" {
		t.Fatalf("Filter() = %q, want cleared by SetTable", m.Filter())
	}
	if m.RowCount() != 3 {
		t.Fatalf("RowCount = %d, want 3 after SetTable clears the filter", m.RowCount())
	}
}

func TestFilterSurvivesWatchDeltas(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("pod-b")
	// An ADDED for a non-matching row must not appear in the filtered view but must
	// join the full set (visible again once the filter clears).
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchAdded, Rows: []kube.Row{row("pod-d", "d")}})
	if m.RowCount() != 1 {
		t.Fatalf("RowCount = %d, want 1 — pod-d does not match pod-b filter", m.RowCount())
	}
	if m.TotalRowCount() != 4 {
		t.Fatalf("TotalRowCount = %d, want 4 after the add", m.TotalRowCount())
	}
	// An ADDED for a matching row shows up immediately.
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchAdded, Rows: []kube.Row{row("pod-bb", "bb")}})
	if m.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2 — pod-bb matches", m.RowCount())
	}
	m.ClearFilter()
	if m.RowCount() != 5 {
		t.Fatalf("RowCount = %d, want 5 after clearing (a,b,c,d,bb)", m.RowCount())
	}
}

// TestRowAtMapsContentRowToRow proves the mouse coordinate seam resolves a
// content-area row to the data row rendered on it (dogfood-08): the column-header
// line and blank/out-of-range lines map to no row, data lines map to their index.
func TestRowAtMapsContentRowToRow(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable()) // 3 rows
	m.SetSize(40, 8)          // innerHeight 6: 1 header line + 5 data lines
	// Content row 0 is the column header — not a data row.
	if _, ok := m.RowAt(0); ok {
		t.Fatal("content row 0 (column header) should map to no data row")
	}
	// Content row 1 is the first data row; content row 3 is the third.
	if idx, ok := m.RowAt(1); !ok || idx != 0 {
		t.Fatalf("content row 1: (%d,%v), want data row 0", idx, ok)
	}
	if idx, ok := m.RowAt(3); !ok || idx != 2 {
		t.Fatalf("content row 3: (%d,%v), want data row 2", idx, ok)
	}
	// Past the last data row (only 3 rows) maps to nothing.
	if _, ok := m.RowAt(4); ok {
		t.Fatal("a content row past the last data row should map to nothing")
	}
	// A content row at the bottom border (>= innerHeight) is rejected.
	if _, ok := m.RowAt(m.innerHeight()); ok {
		t.Fatal("a content row at innerHeight (bottom border) should map to nothing")
	}
}

// TestRowAtHonorsScrollOffset proves RowAt composes with the vertical scroll:
// after scrolling, an on-screen data line resolves to the row actually drawn there.
func TestRowAtHonorsScrollOffset(t *testing.T) {
	m := newTestModel()
	rows := make([]kube.Row, 0, 20)
	for i := 0; i < 20; i++ {
		name := "pod-" + string(rune('a'+i))
		rows = append(rows, row(name, name))
	}
	m.SetTable(kube.Table{Columns: []kube.Column{{Name: "Name"}}, Rows: rows})
	m.SetSize(40, 6) // innerHeight 4: 1 header + 3 data lines
	m.SelectRow(19)  // last row; offset advances to keep it visible
	if m.offset == 0 {
		t.Fatal("selecting the last row in a short pane should scroll the offset")
	}
	// The last content line resolves to offset + (contentRow - 1).
	contentRow := m.innerHeight() - 1
	if idx, ok := m.RowAt(contentRow); !ok || idx != m.offset+contentRow-1 {
		t.Fatalf("RowAt(%d) = (%d,%v) after scroll, want %d",
			contentRow, idx, ok, m.offset+contentRow-1)
	}
}

// TestSelectRowMovesCursor proves the public row setter used by a mouse click
// moves and clamps like keyboard navigation.
func TestSelectRowMovesCursor(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable()) // 3 rows
	m.SetSize(40, 8)
	m.SelectRow(2)
	if m.Cursor() != 2 {
		t.Fatalf("SelectRow: cursor = %d, want 2", m.Cursor())
	}
	m.SelectRow(1000) // clamps to the last row
	if m.Cursor() != 2 {
		t.Fatalf("SelectRow high clamp: cursor = %d, want 2", m.Cursor())
	}
	m.SelectRow(-5) // clamps to 0
	if m.Cursor() != 0 {
		t.Fatalf("SelectRow low clamp: cursor = %d, want 0", m.Cursor())
	}
}

func TestFilteredModifyThatDropsMatchHidesRow(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetFilter("pod-b")
	// MODIFY pod-b so its name no longer matches "pod-b": it leaves the filtered view.
	renamed := kube.Row{Cells: []any{"renamed", "0/1", "10.0.0.2"}, Object: kube.ObjectRef{Name: "renamed", UID: "b"}}
	m.ApplyEvent(kube.WatchEvent{Type: kube.WatchModified, Rows: []kube.Row{renamed}})
	if m.RowCount() != 0 {
		t.Fatalf("RowCount = %d, want 0 after the matching row is renamed out", m.RowCount())
	}
	if m.TotalRowCount() != 3 {
		t.Fatalf("TotalRowCount = %d, want 3 (still in the full set)", m.TotalRowCount())
	}
}
