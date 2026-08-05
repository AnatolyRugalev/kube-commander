package help

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func TestOverlayToggleAndView(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(80)
	m.SetHeight(24)

	if m.Visible() {
		t.Error("overlay should start hidden")
	}
	if m.View() != "" {
		t.Error("hidden overlay should render empty")
	}

	m.Toggle()
	if !m.Visible() {
		t.Fatal("Toggle should show the overlay")
	}
	view := m.View()
	if view == "" {
		t.Fatal("visible overlay should render non-empty help")
	}
	// The overlay is generated from the registry: a known description appears.
	if !strings.Contains(view, keymap.ActionQuit.Describe()) {
		t.Errorf("full help should mention %q; got:\n%s", keymap.ActionQuit.Describe(), view)
	}

	m.SetVisible(false)
	if m.Visible() || m.View() != "" {
		t.Error("SetVisible(false) should hide the overlay")
	}
}

// TestOverlayRendersAsBorderedBox proves the visible overlay is a bordered modal
// box (the picker-style popup), not a full-bleed page: it carries the modal title
// and a rounded border, and — since the root now composites it over the base
// browse view (overlayCenter, D95) — the box itself is no longer padded to fill
// the whole area, but starts flush at its own top-left border.
func TestOverlayRendersAsBorderedBox(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(80)
	m.SetHeight(24)
	m.Toggle()

	view := m.View()
	if !strings.Contains(view, helpTitle) {
		t.Errorf("modal should carry the %q title; got:\n%s", helpTitle, view)
	}
	// A rounded pane border frames the box.
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Errorf("modal should be bordered; got:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	// The bare box is smaller than the sized area (the root centers it over the
	// base view), and its first line is the top border, not blank padding.
	if len(lines) >= 24 {
		t.Errorf("bare box should be smaller than the %d-row area; got %d rows", 24, len(lines))
	}
	if !strings.Contains(lines[0], "╭") {
		t.Errorf("bare box first line should be the top border, got: %q", lines[0])
	}
}

// TestUnsizedOverlayRendersEmpty proves the modal renders nothing until it has a
// size to center within, so an unsized program never paints a zero-area box.
func TestUnsizedOverlayRendersEmpty(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.Toggle()
	if got := m.View(); got != "" {
		t.Errorf("unsized visible overlay should render empty; got %q", got)
	}
	m.SetWidth(80) // width alone, still no height
	if got := m.View(); got != "" {
		t.Errorf("overlay without a height should render empty; got %q", got)
	}
}

func TestShortHelpViewAlwaysRenders(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(200)
	// Short help renders regardless of overlay visibility (it's the status bar).
	if got := m.ShortHelpView(); got == "" {
		t.Error("ShortHelpView should render the status-bar hint even when hidden")
	}
}

// TestShortHelpContextViewTracksFocus proves the focus-aware hint differs between
// the menu and table contexts (so the persistent bottom hint updates with focus)
// and surfaces the context-only descriptions from the registry.
func TestShortHelpContextViewTracksFocus(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(200) // wide enough that nothing is elided

	menu := m.ShortHelpContextView(keymap.HelpMenu)
	table := m.ShortHelpContextView(keymap.HelpTable)
	if menu == "" || table == "" {
		t.Fatal("both focus-context hints should render")
	}
	if menu == table {
		t.Fatal("menu and table hints should differ with focus")
	}
	if !strings.Contains(menu, keymap.ActionDrillIn.Describe()) {
		t.Errorf("menu hint should mention drill-in; got %q", menu)
	}
	if !strings.Contains(table, keymap.ActionFilter.Describe()) {
		t.Errorf("table hint should mention filter; got %q", table)
	}
}

// onCanvas is what overlayCenter does to a box that does not fit: it composites
// onto a fixed width×height canvas and keeps the first height rows. Reading the
// overlay through it is the only way a test in this package can see the BOX-03 bug,
// because View()'s own string is complete — it is the frame that is short (D220 pt 1).
func onCanvas(box string, height int) string {
	lines := strings.Split(box, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// TestOverlayFitsTheCanvas is the BOX-03 regression: the overlay used to render a
// flat fifteen rows at every screen size, because bubbles/help lays ShowAll out at
// whatever height its tallest namespace column needs and this package never looked
// at the height it was handed. Every body height here must produce a box that fits.
func TestOverlayFitsTheCanvas(t *testing.T) {
	for _, sz := range []struct{ w, h int }{
		{200, 60}, {120, 40}, {80, 24}, {80, 15}, {80, 14}, {80, 12}, {80, 8}, {60, 6}, {40, 5},
	} {
		m := New(styles.Default(), keymap.DefaultKeymap())
		m.SetWidth(sz.w)
		m.SetHeight(sz.h)
		m.Toggle()
		v := m.View()
		if got := lipgloss.Height(v); got > sz.h {
			t.Errorf("at %dx%d the box is %d rows, the canvas is %d:\n%s", sz.w, sz.h, got, sz.h, v)
		}
		if got := lipgloss.Width(v); got > sz.w {
			t.Errorf("at %dx%d the box is %d cols, the screen is %d", sz.w, sz.h, got, sz.w)
		}
	}
}

// A clamped overlay keeps the chrome that tells the reader what they are looking at
// and where it ends: the title is rendered first-class above the bindings and the
// bottom border survives, both read through the canvas clip rather than from the
// component's own string.
func TestClampedOverlayKeepsItsTitleAndBorder(t *testing.T) {
	const bodyH = 10
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(80)
	m.SetHeight(bodyH)
	m.Toggle()
	plain := ansi.Strip(onCanvas(m.View(), bodyH))
	if !strings.Contains(plain, helpTitle) {
		t.Errorf("clamped overlay lost its title; got:\n%s", plain)
	}
	if !strings.Contains(plain, "╰") {
		t.Errorf("clamped overlay lost its bottom border to the canvas; got:\n%s", plain)
	}
	// Not a bare "…": bubbles/help emits one of those itself when it elides a
	// column horizontally, so an ellipsis alone would pass with no vertical marker
	// at all. The marker's own words are what distinguishes the two.
	if !strings.Contains(plain, helpTruncated) {
		t.Errorf("clamped overlay dropped bindings without saying so; got:\n%s", plain)
	}
}

// The marker names where the rest is. Every row this overlay elides is a binding
// someone opened it to look up, and unlike the modal's message it exists somewhere
// the reader can reach — the committed reference generated from the same registry.
func TestElisionMarkerPointsAtTheGeneratedDoc(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(120)
	m.SetHeight(10)
	m.Toggle()
	if plain := ansi.Strip(m.View()); !strings.Contains(plain, "docs/keybindings.md") {
		t.Errorf("marker should name the full reference; got:\n%s", plain)
	}
}

// The marker is evidence that something was dropped, not furniture: an overlay with
// room for every binding must not carry it.
func TestRoomyOverlayIsNotMarkedTruncated(t *testing.T) {
	m := New(styles.Default(), keymap.DefaultKeymap())
	m.SetWidth(200)
	m.SetHeight(60)
	m.Toggle()
	if plain := ansi.Strip(m.View()); strings.Contains(plain, "truncated") {
		t.Errorf("an unclamped overlay was marked truncated; got:\n%s", plain)
	}
}

// Below the frame plus its title there is nothing left to show, and a box that is
// only a clipped border is not more honest than no box. The hint line below the
// body still names the toggle, so the reader is not stranded.
func TestOverlayTooShortToFrameAnythingRendersEmpty(t *testing.T) {
	for _, h := range []int{1, 2, 3} {
		m := New(styles.Default(), keymap.DefaultKeymap())
		m.SetWidth(80)
		m.SetHeight(h)
		m.Toggle()
		if got := m.View(); got != "" {
			t.Errorf("a %d-row body should render no overlay; got:\n%s", h, got)
		}
	}
}
