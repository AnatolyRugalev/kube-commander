package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestOverlayCenterKeepsBaseVisible proves the popup floats over the base rather
// than replacing it (feedback 2026-07-22-popups-should-overlay, D95): the base
// fills the area, and after overlaying a small box the base is still visible in
// the corners the box does not cover, while the box's own content occludes the
// center.
func TestOverlayCenterKeepsBaseVisible(t *testing.T) {
	const w, h = 40, 12
	// A base that paints a distinct marker in every corner region.
	baseRow := strings.Repeat("B", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = baseRow
	}
	base := strings.Join(rows, "\n")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render("POPUP")

	out := overlayCenter(base, box, w, h)

	if !strings.Contains(out, "POPUP") {
		t.Errorf("overlay should contain the box content; got:\n%s", out)
	}
	if !strings.Contains(out, "B") {
		t.Errorf("overlay should keep base cells visible around the box; got:\n%s", out)
	}
	// The result is exactly the sized area — never larger than the body.
	if got := lipgloss.Height(out); got != h {
		t.Errorf("overlay height = %d, want %d", got, h)
	}
	// First and last rows are untouched base (the small box is centered), so they
	// are all-base with no box border glyphs.
	lines := strings.Split(out, "\n")
	if strings.ContainsAny(lines[0], "╭╮╯╰") {
		t.Errorf("top row should be pure base, not box border; got: %q", lines[0])
	}
	if strings.ContainsAny(lines[len(lines)-1], "╭╮╯╰") {
		t.Errorf("bottom row should be pure base, not box border; got: %q", lines[len(lines)-1])
	}
}

// TestOverlayCenterDegrades proves an empty box or non-positive area returns the
// base unchanged, so the root can overlay unconditionally.
func TestOverlayCenterDegrades(t *testing.T) {
	base := "base"
	if got := overlayCenter(base, "", 40, 12); got != base {
		t.Errorf("empty box should return base unchanged; got %q", got)
	}
	if got := overlayCenter(base, "box", 0, 12); got != base {
		t.Errorf("zero width should return base unchanged; got %q", got)
	}
	if got := overlayCenter(base, "box", 40, 0); got != base {
		t.Errorf("zero height should return base unchanged; got %q", got)
	}
}

// TestOverlayAtPlacesBoxAtGivenOrigin proves the placed sibling lands the box's
// top-left corner exactly where it is told rather than centering it — the
// mechanic that puts the shared viewer on the browse view's right pane (D284):
// the columns left of x stay base on every row, and the box's own first row is
// the row at y.
func TestOverlayAtPlacesBoxAtGivenOrigin(t *testing.T) {
	const w, h, x = 40, 12, 10
	baseRow := strings.Repeat("B", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = baseRow
	}
	base := strings.Join(rows, "\n")
	// A box as tall as the area and as wide as the space right of x: the viewer's
	// shape, filling the pane it covers.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(w - x - 2).Height(h - 2).
		Render("PAGER")

	out := overlayAt(base, box, x, 0, w, h)

	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("overlayAt height = %d, want %d", len(lines), h)
	}
	for i, line := range lines {
		r := []rune(line)
		if len(r) < x {
			t.Fatalf("row %d shorter than the origin column: %q", i, line)
		}
		if got, want := string(r[:x]), strings.Repeat("B", x); got != want {
			t.Errorf("row %d left of x = %q, want untouched base %q", i, got, want)
		}
	}
	// The box occupies the very first row at its origin (no vertical centering).
	if !strings.ContainsAny(lines[0], "╭╮") {
		t.Errorf("row 0 should carry the box's top border at y=0; got %q", lines[0])
	}
	if bh := lipgloss.Height(box); !strings.ContainsAny(lines[bh-1], "╰╯") {
		t.Errorf("row %d should carry the box's bottom border; got %q", bh-1, lines[bh-1])
	}
	if !strings.Contains(out, "PAGER") {
		t.Errorf("overlayAt should contain the box content; got:\n%s", out)
	}
}

// TestOverlayAtDegrades mirrors overlayCenter's contract: an empty box or a
// non-positive area returns the base unchanged, and a negative origin clamps.
func TestOverlayAtDegrades(t *testing.T) {
	base := "base"
	if got := overlayAt(base, "", 40, 12, 40, 12); got != base {
		t.Errorf("empty box should return base unchanged; got %q", got)
	}
	if got := overlayAt(base, "box", 0, 0, 0, 12); got != base {
		t.Errorf("zero width should return base unchanged; got %q", got)
	}
	if got := overlayAt(base, "box", 0, 0, 40, 0); got != base {
		t.Errorf("zero height should return base unchanged; got %q", got)
	}
	if got := overlayAt("BBBB", "x", -5, -5, 4, 1); !strings.HasPrefix(got, "x") {
		t.Errorf("a negative origin should clamp to the origin; got %q", got)
	}
}
