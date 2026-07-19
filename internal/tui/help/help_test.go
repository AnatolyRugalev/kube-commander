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
