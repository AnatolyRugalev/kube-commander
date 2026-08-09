package forwards

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

func newModel(t *testing.T) Model {
	t.Helper()
	return New(styles.Default())
}

func sampleEntries() []Entry {
	return []Entry{
		{ID: 1, Label: "pod/app-1", Specs: []string{"8080:80"}, Bound: []kube.ForwardedPort{{Local: 8080, Remote: 80}}, Ready: true},
		{ID: 2, Label: "pod/app-2", Specs: []string{"81"}},
	}
}

// TestViewEmpty proves the closed panel renders nothing, and an open panel with no
// entries shows the empty-state line rather than a bare box.
func TestViewEmpty(t *testing.T) {
	if view := newModel(t).View(nil, 80, 24, keymap.DefaultKeymap()); view != "" {
		t.Fatalf("a closed panel should render nothing, got %q", view)
	}
	m := newModel(t)
	m.Open()
	if view := m.View(nil, 80, 24, keymap.DefaultKeymap()); !strings.Contains(view, "No active port-forwards") {
		t.Fatalf("an empty open panel should say so: %q", view)
	}
}

// TestViewListsEntries proves the panel renders each entry's label, its bound
// (or requested) ports and its ready state, with the cursor row marked.
func TestViewListsEntries(t *testing.T) {
	m := newModel(t)
	m.Open()
	m.Clamp(len(sampleEntries()))
	view := m.View(sampleEntries(), 80, 24, keymap.DefaultKeymap())
	for _, want := range []string{"pod/app-1", "localhost:8080 → 80", "ready", "pod/app-2", "81", "[starting…]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the panel should list %q: %q", want, view)
		}
	}
	if !strings.Contains(view, "> pod/app-1") {
		t.Fatalf("the cursor row should be marked: %q", view)
	}
}

// TestViewBoundsTheBox is BOX-02 in the component: the panel must never render
// taller than the height the compositor gives it (overlayCenter clips bottom-first).
func TestViewBoundsTheBox(t *testing.T) {
	m := newModel(t)
	m.Open()
	for _, tc := range []struct{ h, entries int }{
		{24, 40}, // many more entries than a normal screen holds
		{24, 4},  // comfortably fitting: the bound must not shrink what fits
		{12, 20},
		{6, 20},
		{4, 3}, // height 4 → inner height 2, border only
	} {
		entries := make([]Entry, tc.entries)
		for i := range entries {
			entries[i] = Entry{ID: i + 1, Label: "pod/app", Specs: []string{"80"}}
		}
		box := m.View(entries, 80, tc.h, keymap.DefaultKeymap())
		if got, want := lipgloss.Height(box), tc.h; box != "" && got > want {
			t.Errorf("%d entries on a %d-row canvas: panel is %d rows, the canvas is %d", tc.entries, tc.h, got, want)
		}
	}
}

// TestClampAndMove pins the cursor arithmetic directly, including the degenerate
// cases a rendered panel cannot reach (empty set, negative sel).
func TestClampAndMove(t *testing.T) {
	m := newModel(t)
	if m.Active() {
		t.Fatal("a fresh panel should be closed")
	}
	m.Open()
	if !m.Active() {
		t.Fatal("Open should show the panel")
	}

	m.Clamp(0)
	if m.Sel() != 0 {
		t.Fatalf("Clamp on an empty set should pin to 0, got %d", m.Sel())
	}
	m.Clamp(2)
	if m.Sel() != 0 {
		t.Fatalf("Clamp should leave a valid cursor alone, got %d", m.Sel())
	}
	m.sel = 5
	m.Clamp(3)
	if m.Sel() != 2 {
		t.Fatalf("Clamp should pull an out-of-range cursor to the end, got %d", m.Sel())
	}

	m.sel = 1
	m.Move(-1, 3)
	if m.Sel() != 0 {
		t.Fatalf("Move up should step to 0, got %d", m.Sel())
	}
	m.Move(-1, 3)
	if m.Sel() != 0 {
		t.Fatalf("Move up should clamp at 0, got %d", m.Sel())
	}
	m.Move(1, 3)
	if m.Sel() != 1 {
		t.Fatalf("Move down should step to 1, got %d", m.Sel())
	}
	m.Move(5, 3)
	if m.Sel() != 2 {
		t.Fatalf("Move down should clamp at the end, got %d", m.Sel())
	}
	m.Move(1, 0)
	if m.Sel() != 0 {
		t.Fatalf("Move over an empty set should pin to 0, got %d", m.Sel())
	}

	m.Close()
	if m.Active() {
		t.Fatal("Close should hide the panel")
	}
	m.sel = 3
	m.Reset()
	if m.Active() || m.Sel() != 0 {
		t.Fatalf("Reset should close and zero the cursor, got open=%v sel=%d", m.Active(), m.Sel())
	}
}

// TestWindow pins the window arithmetic directly, including the degenerate sizes
// the rendered panel cannot reach (a zero-row budget on a screen so short that the
// box is title-only). It is the same table the shell test ran against
// forwards.Window before the panel moved out of app.go (MONO-01).
func TestWindow(t *testing.T) {
	for _, tc := range []struct {
		total, sel, n      int
		wantStart, wantEnd int
	}{
		{total: 0, sel: 0, n: 5, wantStart: 0, wantEnd: 0},     // nothing to show
		{total: 3, sel: 1, n: 5, wantStart: 0, wantEnd: 3},     // fits: no scrolling
		{total: 5, sel: 0, n: 5, wantStart: 0, wantEnd: 5},     // exactly fits
		{total: 20, sel: 0, n: 5, wantStart: 0, wantEnd: 5},    // top of the list
		{total: 20, sel: 4, n: 5, wantStart: 0, wantEnd: 5},    // last row of the first window
		{total: 20, sel: 5, n: 5, wantStart: 1, wantEnd: 6},    // one step past it scrolls by one
		{total: 20, sel: 19, n: 5, wantStart: 15, wantEnd: 20}, // bottom, clamped to the end
		{total: 20, sel: 3, n: 0, wantStart: 0, wantEnd: 0},    // no room for any row
		{total: 20, sel: 3, n: -1, wantStart: 0, wantEnd: 0},   // and a negative budget is not a panic
	} {
		start, end := Window(tc.total, tc.sel, tc.n)
		if start != tc.wantStart || end != tc.wantEnd {
			t.Errorf("Window(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tc.total, tc.sel, tc.n, start, end, tc.wantStart, tc.wantEnd)
		}
		if tc.wantEnd > tc.wantStart && (tc.sel < start || tc.sel >= end) {
			t.Errorf("Window(%d, %d, %d) = (%d, %d) does not contain the cursor",
				tc.total, tc.sel, tc.n, start, end)
		}
	}
}

// TestTitleCountsWhatIsHidden proves the elision announces itself (D220 pt 3): the
// counter appears only when rows are actually off screen.
func TestTitle(t *testing.T) {
	if got := Title(3, 0, 3); got != "Port-forwards" {
		t.Fatalf("a full list should not count: %q", got)
	}
	if got := Title(0, 0, 0); got != "Port-forwards" {
		t.Fatalf("an empty list should not count: %q", got)
	}
	if got := Title(20, 16, 20); !strings.Contains(got, "17–20 of 20") {
		t.Fatalf("a windowed list should name the slice and total: %q", got)
	}
	if got := Title(20, 5, 5); got != "Port-forwards (20)" {
		t.Fatalf("a title-only box should carry the bare count: %q", got)
	}
}

// TestPortsLabel proves the label prefers the bound pairs and falls back to the
// requested specs — the string the panel row and the shell's notice share.
func TestPortsLabel(t *testing.T) {
	if got := PortsLabel(Entry{Specs: []string{"80", "8080:80"}}); got != "80 8080:80" {
		t.Fatalf("unbound entries read as their requested specs: %q", got)
	}
	if got := PortsLabel(Entry{Bound: []kube.ForwardedPort{{Local: 8080, Remote: 80}}}); got != "localhost:8080 → 80" {
		t.Fatalf("bound entries read as local:remote: %q", got)
	}
}
