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
