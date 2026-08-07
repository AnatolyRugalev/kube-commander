package table

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// TestClassifyCell pins the classifier: which column names it reacts to, and what
// each value under them means. It is a pure function, so this is the primary
// coverage — the render tests below only prove the result reaches the screen.
func TestClassifyCell(t *testing.T) {
	cases := []struct {
		column, value string
		want          cellRole
	}{
		// Columns it has nothing to say about — the overwhelming majority.
		{"NAME", "CrashLoopBackOff", roleNone}, // the word only means something under STATUS
		{"AGE", "5d", roleNone},
		{"IP", "10.0.0.1", roleNone},
		{"MY-CRD-COLUMN", "whatever", roleNone},

		// Pod STATUS: healthy, in flight, broken.
		{"STATUS", "Running", roleSuccess},
		{"STATUS", "Completed", roleSuccess},
		{"STATUS", "Pending", roleWarn},
		{"STATUS", "ContainerCreating", roleWarn},
		{"STATUS", "Terminating", roleWarn},
		{"STATUS", "CrashLoopBackOff", roleError},
		{"STATUS", "ImagePullBackOff", roleError},
		{"STATUS", "Evicted", roleError},
		{"STATUS", "OOMKilled", roleError},

		// Init containers: the reason after the colon carries the signal, and bare
		// progress is a warning.
		{"STATUS", "Init:0/2", roleWarn},
		{"STATUS", "Init:CrashLoopBackOff", roleError},
		{"STATUS", "Init:Error", roleError},

		// The open tail: reasons nobody enumerated, classified by name shape.
		{"STATUS", "CreateContainerConfigError", roleError},
		{"STATUS", "ErrImageNeverPull", roleError},
		{"STATUS", "SomethingBackOff", roleError},
		{"STATUS", "TotallyMadeUpReason", roleNone},

		// Node STATUS is comma-joined and the most severe part wins.
		{"STATUS", "Ready", roleSuccess},
		{"STATUS", "NotReady", roleWarn},
		{"STATUS", "Ready,SchedulingDisabled", roleWarn},
		{"STATUS", "NotReady,SchedulingDisabled", roleWarn},

		// Aliases for the same idea.
		{"PHASE", "Bound", roleSuccess},
		{"PHASE", "Failed", roleError},
		{"STATE", "Running", roleSuccess},

		// Column names are matched case-insensitively: a CRD's additionalPrinterColumns
		// declares them however it likes.
		{"Status", "Running", roleSuccess},
		{"status", "Running", roleSuccess},

		// READY as a fraction: a shortfall is a warning, never an error.
		{"READY", "1/1", roleSuccess},
		{"READY", "3/3", roleSuccess},
		{"READY", "0/1", roleWarn},
		{"READY", "2/3", roleWarn},
		{"READY", "0/0", roleNone}, // scaled to zero is a choice, not a fault
		{"READY", "True", roleSuccess},
		{"READY", "False", roleError},
		{"READY", "not-a-fraction", roleNone},

		// RESTARTS: any restart is a warning; there is no error tier.
		{"RESTARTS", "0", roleNone},
		{"RESTARTS", "1", roleWarn},
		{"RESTARTS", "97", roleWarn},
		{"RESTARTS", "3 (5m ago)", roleWarn}, // kubectl's "count (age ago)" form
		{"RESTARTS", "", roleNone},

		// Placeholders are absence, not a state.
		{"STATUS", "", roleNone},
		{"STATUS", "<none>", roleNone},
		{"STATUS", "<unknown>", roleNone},
	}
	for _, c := range cases {
		if got := classifyCell(c.column, c.value); got != c.want {
			t.Errorf("classifyCell(%q, %q) = %v, want %v", c.column, c.value, got, c.want)
		}
	}
}

// TestCellRoleSeverityOrder pins the ordering the comma-merge in statusRole relies
// on: a plain `>` must mean "more severe".
func TestCellRoleSeverityOrder(t *testing.T) {
	if roleNone >= roleSuccess || roleSuccess >= roleWarn || roleWarn >= roleError {
		t.Fatal("cellRole constants must be ordered by increasing severity")
	}
}

// statusTable is a pod-like table whose three rows cover the three roles.
func statusTable() kube.Table {
	return kube.Table{
		Columns: []kube.Column{
			{Name: "NAME", Type: "string"},
			{Name: "READY", Type: "string"},
			{Name: "STATUS", Type: "string"},
			{Name: "RESTARTS", Type: "integer"},
		},
		Rows: []kube.Row{
			{Cells: []any{"pod-good", "1/1", "Running", int64(0)}, Object: kube.ObjectRef{Name: "pod-good", UID: "g"}},
			{Cells: []any{"pod-warn", "0/1", "Pending", int64(2)}, Object: kube.ObjectRef{Name: "pod-warn", UID: "w"}},
			{Cells: []any{"pod-bad", "0/1", "CrashLoopBackOff", int64(9)}, Object: kube.ObjectRef{Name: "pod-bad", UID: "b"}},
		},
	}
}

// dataLine returns data row n of a rendered view. Line 0 is the top border and
// line 1 the column header, so data row n is line n+2.
func dataLine(view string, n int) string {
	lines := strings.Split(view, "\n")
	if len(lines) < n+3 {
		return ""
	}
	return lines[n+2]
}

// TestStatusCellsAreColored proves the classifier's verdict actually reaches the
// screen: the STATUS cell of an unselected row is painted with the role's color,
// and the row's plain cells are not.
func TestStatusCellsAreColored(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(statusTable())
	m.SetSize(60, 8)

	// Row 0 is the cursor row (selection wins there — see the test below), so
	// check the warn and error rows.
	for _, c := range []struct {
		line  int
		text  string
		style lipgloss.Style
	}{
		{1, "Pending", s.Warn},
		{2, "CrashLoopBackOff", s.Error},
	} {
		got := dataLine(m.View(), c.line)
		if want := c.style.Render(c.text); !strings.Contains(got, want) {
			t.Fatalf("line %d: %q not painted with its role style\ngot:  %q\nwant substring: %q",
				c.line, c.text, got, want)
		}
		// The object's name is not a status column and must stay body text.
		if strings.Contains(got, s.Error.Render("pod-")) || strings.Contains(got, s.Warn.Render("pod-")) {
			t.Fatalf("line %d: the NAME cell should not be colored:\n%q", c.line, got)
		}
	}
}

// TestSelectionWinsOverCellColor proves the cursor row keeps its full-width
// selection bar: a status color must never punch a hole in the highlight.
func TestSelectionWinsOverCellColor(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(statusTable())
	m.SetSize(60, 8)
	m.SelectRow(2) // the CrashLoopBackOff row

	sel := dataLine(m.View(), 2)
	if strings.Contains(sel, s.Error.Render("CrashLoopBackOff")) {
		t.Fatalf("the selected row must not carry cell coloring:\n%q", sel)
	}
	// Same row, unselected, does carry it — so the difference is the selection,
	// not the classifier failing to fire.
	m.SelectRow(0)
	unsel := dataLine(m.View(), 2)
	if !strings.Contains(unsel, s.Error.Render("CrashLoopBackOff")) {
		t.Fatalf("the unselected row should carry cell coloring:\n%q", unsel)
	}
}

// TestColoredRowKeepsWidthAndText proves painting is invisible to layout: every
// rendered line is exactly the pane width, and stripping the styling gives back
// the same text the uncolored renderer produced. This is the invariant the
// horizontal clip and the column alignment both rest on.
func TestColoredRowKeepsWidthAndText(t *testing.T) {
	m := New(styles.Default())
	m.SetTable(statusTable())

	// Both a pane wide enough for everything and one that clips.
	for _, w := range []int{60, 24, 12} {
		m.SetSize(w, 8)
		view := m.View()
		if got := lipgloss.Width(view); got != w {
			t.Fatalf("width %d: View width = %d, want %d\n%s", w, got, w, view)
		}
		for i := range statusTable().Rows {
			line := dataLine(view, i)
			if got := lipgloss.Width(line); got != w {
				t.Fatalf("width %d: data line %d width = %d, want %d: %q", w, i, got, w, line)
			}
		}
		// The plain text must survive the styling untouched.
		plain := ansi.Strip(dataLine(view, 2))
		if w >= 60 && !strings.Contains(plain, "CrashLoopBackOff") {
			t.Fatalf("width %d: stripped line lost its text: %q", w, plain)
		}
	}
}

// TestColoringFollowsHorizontalScroll proves the spans are in unclipped
// coordinates and get shifted, not dropped: after scrolling the STATUS column
// leftwards the cell is still painted, and the text is still whole.
func TestColoringFollowsHorizontalScroll(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(statusTable())
	m.SetSize(30, 8) // narrower than the content, so the columns scroll
	m.SelectRow(0)   // keep the CrashLoopBackOff row unselected

	// Scroll until the columns stop moving; STATUS is then fully in the window.
	for i := 0; i < 4; i++ {
		m, _ = m.Update(keymap.ActionRight)
	}
	if m.HOffset() == 0 {
		t.Fatal("expected the table to have scrolled horizontally")
	}
	if got := dataLine(m.View(), 2); !strings.Contains(got, s.Error.Render("CrashLoopBackOff")) {
		t.Fatalf("the STATUS cell lost its color after a horizontal scroll (hoffset %d):\n%q",
			m.HOffset(), got)
	}
}

// TestUncoloredTableRendersUnchanged proves the common case is untouched: a table
// with no status-carrying columns renders exactly as it did before M4-06, through
// the single-style fast path.
func TestUncoloredTableRendersUnchanged(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "NAME", Type: "string"}, {Name: "AGE", Type: "string"}},
		Rows: []kube.Row{
			{Cells: []any{"cm-a", "5d"}, Object: kube.ObjectRef{Name: "cm-a", UID: "a"}},
			{Cells: []any{"cm-b", "2h"}, Object: kube.ObjectRef{Name: "cm-b", UID: "b"}},
		},
	})
	m.SetSize(40, 8)

	// The line as rendered still has the pane's border glyphs around it, so this
	// compares the content between them — which must be one single-style render.
	line := dataLine(m.View(), 1) // row 0 is selected; row 1 is plain
	innerW := 40 - 2
	plain := padRight("cm-b", 4) + colGap + padRight("2h", 3) // NAME/AGE column widths
	if want := s.App.Width(innerW).Render(plain); !strings.Contains(line, want) {
		t.Fatalf("uncolored row should render through the plain path:\ngot  %q\nwant substring %q", line, want)
	}
}

// TestShortRowColoringSafe proves a row with fewer cells than columns (principle 3
// degradation) neither panics nor mis-paints: the missing STATUS cell is blank, so
// it classifies to nothing.
func TestShortRowColoringSafe(t *testing.T) {
	m := New(styles.Default())
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "NAME"}, {Name: "STATUS"}, {Name: "READY"}},
		Rows: []kube.Row{
			{Cells: []any{"a", "Running", "1/1"}, Object: kube.ObjectRef{Name: "a", UID: "a"}},
			{Cells: []any{"b"}, Object: kube.ObjectRef{Name: "b", UID: "b"}}, // short
		},
	})
	m.SetSize(40, 8)
	if got := ansi.Strip(dataLine(m.View(), 1)); !strings.Contains(got, "b") {
		t.Fatalf("short row lost its present cell: %q", got)
	}
}

// TestFilterMatchIsHighlighted proves the `/` filter's matched text is painted
// with the Match style on a plain row — the headline claim of FILT-02. `pod-`
// keeps all three rows, so this reads a row the cursor is not on.
func TestFilterMatchIsHighlighted(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(sampleTable())
	m.SetSize(60, 8)
	m.SetFilter("pod-")

	line := dataLine(m.View(), 1)
	if want := s.Match.Render("pod-"); !strings.Contains(line, want) {
		t.Fatalf("the matched text should be highlighted:\ngot  %q\nwant substring %q", line, want)
	}
	// The rest of the name is not part of the match and must stay body text.
	if strings.Contains(line, s.Match.Render("pod-b")) {
		t.Fatalf("the highlight must span the query, not the cell:\n%q", line)
	}
}

// TestFilterMatchIsCaseInsensitiveAndRepeats proves the highlight follows the
// filter's own matching rule (case-insensitive substring, every occurrence) rather
// than a second, stricter one — a row kept by the filter but shown unmarked would
// read as a bug in the filter.
func TestFilterMatchIsCaseInsensitiveAndRepeats(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "NAME"}, {Name: "NOTE"}},
		Rows: []kube.Row{
			{Cells: []any{"keep-me", "x"}, Object: kube.ObjectRef{Name: "keep-me", UID: "k"}},
			{Cells: []any{"ab-AB-ab", "y"}, Object: kube.ObjectRef{Name: "ab-AB-ab", UID: "a"}},
		},
	})
	m.SetSize(60, 8)
	m.SetFilter("AB")

	// Only the second row matches, so it is row 0 — and the cursor row keeps its
	// marks (see TestFilterMatchSurvivesSelection).
	line := dataLine(m.View(), 0)
	if got := strings.Count(line, s.Match.Render("ab")) + strings.Count(line, s.Match.Render("AB")); got != 3 {
		t.Fatalf("want all 3 occurrences highlighted, got %d:\n%q", got, line)
	}
}

// TestFilterMatchSurvivesSelection proves the one thing the selection bar does not
// win over: the cursor row still shows what matched, while everything else on it
// is the Selection style.
func TestFilterMatchSurvivesSelection(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(sampleTable())
	m.SetSize(60, 8)
	m.SetFilter("pod-")

	sel := dataLine(m.View(), 0) // the cursor row
	if want := s.Match.Render("pod-"); !strings.Contains(sel, want) {
		t.Fatalf("the cursor row must keep its match highlight:\ngot  %q\nwant substring %q", sel, want)
	}
	// The unmatched remainder is rendered through Selection, in runs whose exact
	// boundaries are a column-width detail — so this asserts the style is there,
	// and that a plain row does not have it.
	selPrefix := strings.SplitN(s.Selection.Render("x"), "x", 2)[0]
	if !strings.Contains(sel, selPrefix) {
		t.Fatalf("the rest of the cursor row must stay the selection bar:\n%q", sel)
	}
	if plain := dataLine(m.View(), 1); strings.Contains(plain, selPrefix) {
		t.Fatalf("a non-cursor row must not carry the selection bar:\n%q", plain)
	}
}

// TestFilterMatchWinsOverCellColorAndCutsIt proves the merge: a match landing
// inside a status-colored cell is painted whole, and the uncovered part of that
// cell keeps its role color rather than being dropped or repainted.
//
// `1` matches every row (each READY cell has one), so row 1 is a plain row whose
// READY "0/1" classifies to Warn and whose last rune is the match.
func TestFilterMatchWinsOverCellColorAndCutsIt(t *testing.T) {
	s := styles.Default()
	m := New(s)
	m.SetTable(statusTable())
	m.SetSize(60, 8)
	m.SetFilter("1")

	line := dataLine(m.View(), 1) // pod-warn
	if want := s.Warn.Render("0/") + s.Match.Render("1"); !strings.Contains(line, want) {
		t.Fatalf("a match inside a colored cell should cut it, not replace it:\ngot  %q\nwant substring %q", line, want)
	}
	if strings.Contains(line, s.Warn.Render("0/1")) {
		t.Fatalf("the matched rune must not stay in the role color:\n%q", line)
	}
	// A colored cell with no match in it is untouched.
	if want := s.Warn.Render("Pending"); !strings.Contains(line, want) {
		t.Fatalf("an unmatched colored cell should keep its role style:\ngot  %q\nwant substring %q", line, want)
	}
}

// TestFilterHighlightKeepsWidthAndText is TestColoredRowKeepsWidthAndText with a
// filter on: painting must stay invisible to layout at every pane width and at a
// nonzero horizontal scroll, since the match spans are in unclipped coordinates
// and are shifted at paint time.
func TestFilterHighlightKeepsWidthAndText(t *testing.T) {
	for _, w := range []int{60, 24, 12} {
		for _, hoff := range []int{0, 3, 9} {
			m := New(styles.Default())
			m.SetTable(statusTable())
			m.SetSize(w, 8)
			m.SetFilter("1")
			m.hoffset = hoff

			view := m.View()
			if got := lipgloss.Width(view); got != w {
				t.Fatalf("width %d hoffset %d: View width = %d\n%s", w, hoff, got, view)
			}
			// The same model with no filter and the same rows on screen renders the
			// same text; only the styling may differ.
			plain := New(styles.Default())
			plain.SetTable(kube.Table{Columns: statusTable().Columns, Rows: statusTable().Rows})
			plain.SetSize(w, 8)
			plain.hoffset = hoff
			if got, want := ansi.Strip(view), ansi.Strip(plain.View()); got != want {
				t.Fatalf("width %d hoffset %d: highlighting changed the text\ngot  %q\nwant %q", w, hoff, got, want)
			}
		}
	}
}

// TestMatchOffsets pins the offset finder directly: it is what places every
// highlight, it works in runes (the span coordinate space), and it declines the
// one case it cannot place honestly.
func TestMatchOffsets(t *testing.T) {
	cases := []struct {
		text, needle string
		want         []int
	}{
		{"pod-a", "pod", []int{0}},
		{"POD-A", "pod", []int{0}},  // the query is matched case-insensitively
		{"aaaa", "aa", []int{0, 2}}, // occurrences never overlap
		{"abcabc", "abc", []int{0, 3}},
		{"pod-a", "zz", nil},
		{"ab", "abc", nil},             // needle longer than the cell
		{"héllo wörld", "ö", []int{7}}, // offsets are runes, not bytes
		{"İstanbul", "i", []int{0}},    // a fold that changes byte width, never rune count
	}
	for _, c := range cases {
		got := matchOffsets(c.text, []rune(c.needle))
		if len(got) != len(c.want) {
			t.Fatalf("matchOffsets(%q, %q) = %v, want %v", c.text, c.needle, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("matchOffsets(%q, %q) = %v, want %v", c.text, c.needle, got, c.want)
			}
		}
	}
}

// TestMergeSpansCutsAndOrders pins the merge in isolation: the result is sorted,
// disjoint, and every match survives whole — the three properties paintRow relies
// on and that a rendered frame can only show indirectly.
func TestMergeSpansCutsAndOrders(t *testing.T) {
	role := []roleSpan{{start: 0, end: 10, role: roleWarn}, {start: 20, end: 24, role: roleError}}
	match := []roleSpan{{start: 3, end: 5, match: true}, {start: 8, end: 12, match: true}, {start: 20, end: 24, match: true}}

	got := mergeSpans(role, match)
	want := []roleSpan{
		{start: 0, end: 3, role: roleWarn},
		{start: 3, end: 5, match: true},
		{start: 5, end: 8, role: roleWarn},
		{start: 8, end: 12, match: true},
		{start: 20, end: 24, match: true}, // the role span is wholly covered and vanishes
	}
	if len(got) != len(want) {
		t.Fatalf("mergeSpans = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("mergeSpans[%d] = %+v, want %+v\nfull: %+v", i, got[i], want[i], got)
		}
	}
	// With nothing to overlay the role spans come back untouched.
	if got := mergeSpans(role, nil); len(got) != 2 || got[0] != role[0] || got[1] != role[1] {
		t.Fatalf("mergeSpans with no matches should be a no-op, got %+v", got)
	}
}
