package table

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// agedTable is a two-row pod-like table with an AGE column, printed by the
// "server" at printedAt.
func agedTable(printedAt time.Time) kube.Table {
	return kube.Table{
		Columns: []kube.Column{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Age", Type: "string"},
		},
		Rows: []kube.Row{
			{
				Cells:   []any{"pod-a", "10s"},
				Object:  kube.ObjectRef{Namespace: "default", Name: "pod-a", UID: "a"},
				Created: printedAt.Add(-10 * time.Second),
			},
			{
				Cells:   []any{"pod-b", "3m"},
				Object:  kube.ObjectRef{Namespace: "default", Name: "pod-b", UID: "b"},
				Created: printedAt.Add(-3 * time.Minute),
			},
		},
	}
}

// TestRefreshAgesUpdatesTheRenderedColumn is the feedback's repro: a pane left
// open keeps showing the age the server printed. Assert on the rendered frame,
// not the cell — the frame is what the reader said was wrong.
func TestRefreshAgesUpdatesTheRenderedColumn(t *testing.T) {
	printed := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	m := newTestModel()
	m.SetTable(agedTable(printed))
	m.SetSize(40, 8)

	if !strings.Contains(m.View(), "10s") {
		t.Fatalf("the server's own age is missing from the first paint:\n%s", m.View())
	}

	if !m.RefreshAges(printed.Add(5 * time.Hour)) {
		t.Fatal("RefreshAges reported no change five hours on")
	}
	view := m.View()
	if strings.Contains(view, "10s") {
		t.Errorf("the stale age survived the refresh:\n%s", view)
	}
	if !strings.Contains(view, "5h ") || !strings.Contains(view, "5h3m") {
		t.Errorf("want pod-a at 5h and pod-b at 5h3m:\n%s", view)
	}
}

// TestRefreshAgesWidensTheColumn pins the reason RefreshAges re-derives the whole
// view instead of only rewriting cells: padRight never truncates, so an age that
// outgrows the width measured at SetTable time would shift every column to its
// right on that row alone.
func TestRefreshAgesWidensTheColumn(t *testing.T) {
	printed := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	tbl := kube.Table{
		Columns: []kube.Column{{Name: "N", Type: "string"}, {Name: "Age", Type: "string"}},
		Rows: []kube.Row{
			{Cells: []any{"a", "5s"}, Object: kube.ObjectRef{UID: "a"}, Created: printed.Add(-5 * time.Second)},
			{Cells: []any{"b", "5s"}, Object: kube.ObjectRef{UID: "b"}, Created: printed.Add(-5 * time.Second)},
		},
	}
	m := newTestModel()
	m.SetTable(tbl)
	m.SetSize(40, 8)

	// "Age" (3) is the widest thing in the column at first paint.
	if got, want := m.colWidths[1], 3; got != want {
		t.Fatalf("initial age width = %d, want %d", got, want)
	}
	// Five days and three hours prints as "5d3h" — one column wider than "Age".
	m.RefreshAges(printed.Add(5*24*time.Hour + 3*time.Hour))
	if got, want := m.colWidths[1], len("5d3h"); got != want {
		t.Fatalf("age width = %d, want %d — the column did not grow with its cells", got, want)
	}
	for _, line := range strings.Split(strings.TrimRight(m.View(), "\n"), "\n") {
		if w := ansi.StringWidth(line); w != 40 {
			t.Fatalf("line %q is %d wide, want 40 (the frame must stay square)", line, w)
		}
	}
}

// TestRefreshAgesKeepsSelectionAndFilter proves the tick is not a reset: it runs
// once a second under whatever the reader is doing.
func TestRefreshAgesKeepsSelectionAndFilter(t *testing.T) {
	printed := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	m := newTestModel()
	m.SetTable(agedTable(printed))
	m.SetSize(40, 8)
	m.SetFilter("pod-")
	m.SelectRow(1)

	m.RefreshAges(printed.Add(time.Hour))

	if got, want := m.Filter(), "pod-"; got != want {
		t.Errorf("filter = %q, want %q", got, want)
	}
	if got, want := m.RowCount(), 2; got != want {
		t.Errorf("visible rows = %d, want %d", got, want)
	}
	row, ok := m.SelectedRow()
	if !ok || row.Object.UID != "b" {
		t.Errorf("selection = %+v, want the row it was on (uid b)", row.Object)
	}
}

// TestRefreshAgesNoopWithoutAnAgeColumn keeps the tick free on the tables that
// have nothing to refresh — the whole reason it can be unconditional.
func TestRefreshAgesNoopWithoutAnAgeColumn(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable()) // Name / Ready / IP — no age
	m.SetSize(40, 8)
	before := m.View()
	if m.RefreshAges(time.Now()) {
		t.Fatal("RefreshAges reported a change on a table with no age column")
	}
	if m.View() != before {
		t.Error("the view moved on a table with no age column")
	}
}
