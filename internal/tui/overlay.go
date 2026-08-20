package tui

import "charm.land/lipgloss/v2"

// overlayCenter composites a modal box centered over a base view, so a popup
// (help, namespace picker, and — once wired — the confirm modal) floats on top
// of the two-pane browse layout rather than replacing it (feedback
// 2026-07-22-popups-should-overlay, D95). The box occludes only the rectangle it
// covers; every base cell outside that rectangle stays visible underneath.
//
// It builds a lipgloss/v2 layer stack — the base at the origin (z0) and the box
// centered on top (z1) — and flattens it onto a fixed width×height canvas so the
// result is always exactly the body area, never larger. An empty box (an unsized
// or hidden modal) or a non-positive area returns the base unchanged, so a caller
// can overlay unconditionally.
func overlayCenter(base, box string, width, height int) string {
	if box == "" || width <= 0 || height <= 0 {
		return base
	}
	x := (width - lipgloss.Width(box)) / 2
	y := (height - lipgloss.Height(box)) / 2
	return overlayAt(base, box, x, y, width, height)
}

// overlayAt is overlayCenter's placed sibling: it composites the box with its
// top-left corner at (x, y) of the body area instead of centering it. The shared
// viewer uses it to land on the browse view's right pane — a pager fills the pane
// it covers rather than floating inside it (D284) — while the menu pane to its
// left stays visible. Same contract otherwise: an empty box or a non-positive
// area returns the base unchanged, negative coordinates clamp to the origin, and
// the result is always exactly width×height.
func overlayAt(base, box string, x, y, width, height int) string {
	if box == "" || width <= 0 || height <= 0 {
		return base
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	bg := lipgloss.NewLayer(base)
	fg := lipgloss.NewLayer(box).X(x).Y(y).Z(1)
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(bg, fg))
	return canvas.Render()
}
