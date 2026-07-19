package styles

import (
	"image/color"
	"strings"
	"testing"
)

func TestDefaultThemeColorsSet(t *testing.T) {
	th := DefaultTheme()
	if th.Name == "" {
		t.Error("DefaultTheme().Name is empty")
	}
	// Every semantic color must be set; a nil color silently falls back to the
	// terminal default, which would mask a missing palette entry.
	colors := map[string]color.Color{
		"Foreground":  th.Foreground,
		"Subtle":      th.Subtle,
		"Primary":     th.Primary,
		"Selection":   th.Selection,
		"SelectionFg": th.SelectionFg,
		"Border":      th.Border,
		"BorderFocus": th.BorderFocus,
		"Header":      th.Header,
		"StatusBarFg": th.StatusBarFg,
		"StatusBarBg": th.StatusBarBg,
		"Error":       th.Error,
		"Warn":        th.Warn,
		"Success":     th.Success,
	}
	for name, c := range colors {
		if c == nil {
			t.Errorf("DefaultTheme().%s is nil", name)
		}
	}
}

func TestNewCarriesTheme(t *testing.T) {
	th := DefaultTheme()
	s := New(th)
	if s.Theme.Name != th.Name {
		t.Errorf("Styles.Theme.Name = %q, want %q", s.Theme.Name, th.Name)
	}
}

func TestStylesRenderContent(t *testing.T) {
	s := Default()
	const content = "kubecom"
	// Each derived style must render (and preserve) its content — styling wraps
	// text in ANSI escapes but never drops it.
	renders := map[string]string{
		"App":       s.App.Render(content),
		"Subtle":    s.Subtle.Render(content),
		"Selection": s.Selection.Render(content),
		"Header":    s.Header.Render(content),
		"StatusBar": s.StatusBar.Render(content),
		"Error":     s.Error.Render(content),
		"Warn":      s.Warn.Render(content),
		"Success":   s.Success.Render(content),
		"Spinner":   s.Spinner.Render(content),
	}
	for name, out := range renders {
		if !strings.Contains(out, content) {
			t.Errorf("Styles.%s.Render(%q) = %q, missing content", name, content, out)
		}
	}
}

func TestPaneFocusDiffersFromPane(t *testing.T) {
	s := Default()
	// The focused pane must be visually distinct (accented border), otherwise
	// focus is invisible.
	if s.Pane.Render("x") == s.PaneFocus.Render("x") {
		t.Error("Pane and PaneFocus render identically; focus is not distinguishable")
	}
}

func TestNewIsPure(t *testing.T) {
	// Same theme in → identical rendered output out; New touches no global state.
	a := New(DefaultTheme())
	b := New(DefaultTheme())
	if a.Selection.Render("row") != b.Selection.Render("row") {
		t.Error("New is not deterministic for the same Theme")
	}
}
