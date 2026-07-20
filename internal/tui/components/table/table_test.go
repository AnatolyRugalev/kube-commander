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

// Update must return a table.Model (not tea.Model) so the root can keep a typed
// value; this compile-time check guards the signature.
var _ = func(m Model) (Model, tea.Cmd) { return m.Update(keymap.ActionDown) }
