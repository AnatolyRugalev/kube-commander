package help

import (
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

func TestOverlayToggleAndView(t *testing.T) {
	m := New(keymap.DefaultKeymap())
	m.SetWidth(80)

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

func TestShortHelpViewAlwaysRenders(t *testing.T) {
	m := New(keymap.DefaultKeymap())
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
	m := New(keymap.DefaultKeymap())
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
