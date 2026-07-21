package help

import (
	"strings"
	"testing"

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

// TestOverlayRendersAsCenteredModal proves the visible overlay is a bordered box
// centered within the sized area (the picker-style popup), not a full-bleed page:
// it carries the modal title and a rounded border, and is padded above/around
// rather than starting flush at the top-left.
func TestOverlayRendersAsCenteredModal(t *testing.T) {
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
	if len(lines) != 24 {
		t.Fatalf("placed modal should fill the %d-row area; got %d rows", 24, len(lines))
	}
	// Centered vertically: the top rows are blank padding, the border starts lower.
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("modal should be centered (blank top padding), got first line: %q", lines[0])
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
