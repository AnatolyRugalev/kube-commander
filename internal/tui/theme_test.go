package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// TestWithThemeReplacesTheShellStyles proves the option reaches the model's own
// Styles — the palette every later SetStyles (M4-12b) and every direct render in
// app.go reads.
func TestWithThemeReplacesTheShellStyles(t *testing.T) {
	m := New(WithTheme(styles.MonokaiTheme()))
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("styles.Theme.Name = %q, want monokai", got)
	}
	if New().styles.Theme.Name != styles.DefaultTheme().Name {
		t.Fatalf("without the option the shell must keep the default theme")
	}
}

// TestWithThemeReachesTheComponents is the headline invariant of M4-12a: the
// components are constructed *after* the options run, so a themed shell actually
// draws in that theme. Constructing them first (the pre-M4-12a order) would leave
// every one of them holding the default Styles and make this a silent no-op —
// m.styles would say monokai while the screen stayed blue.
func TestWithThemeReachesTheComponents(t *testing.T) {
	def := sizedWith(t).View().Content
	themed := sizedWith(t, WithTheme(styles.MonokaiTheme())).View().Content
	if def == "" || themed == "" {
		t.Fatal("sized View rendered nothing; the comparison below would be vacuous")
	}
	if def == themed {
		t.Error("the browse view renders identically under monokai and the default theme")
	}
	// Same glyphs, different colors: the theme must not change the layout.
	if stripANSI(def) != stripANSI(themed) {
		t.Errorf("a theme changed the rendered text, not just its colors:\ndefault:\n%s\nmonokai:\n%s",
			stripANSI(def), stripANSI(themed))
	}
}

// TestEveryBuiltinThemeRendersTheShell walks the registry through the real
// constructor, so a theme added to `builtins` is exercised end-to-end here rather
// than only in the styles package's own unit tests.
func TestEveryBuiltinThemeRendersTheShell(t *testing.T) {
	seen := map[string]string{}
	for _, th := range styles.Themes() {
		out := sizedWith(t, WithTheme(th)).View().Content
		for name, prev := range seen {
			if prev == out {
				t.Errorf("themes %q and %q render the shell identically", name, th.Name)
			}
		}
		seen[th.Name] = out
	}
}

// TestWithThemeLeavesOtherOptionsApplied guards the constructor restructure from
// the other side: the post-option seeding (menu extras, the namespace fed to the
// menu and status bar, the welcome page's version) must still happen, since it now
// runs against components built later in the same function.
func TestWithThemeLeavesOtherOptionsApplied(t *testing.T) {
	m, _ := New(
		WithTheme(styles.SolarizedDarkTheme()),
		WithNamespace("kube-system"),
		WithVersion("v9.9.9"),
		WithContext("prod-cluster"),
	).Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := stripANSI(m.(Model).View().Content)
	for _, want := range []string{"kube-system", "v9.9.9", "prod-cluster"} {
		if !strings.Contains(view, want) {
			t.Errorf("themed shell dropped %q from its view:\n%s", want, view)
		}
	}
}

// stripANSI removes SGR escape sequences so a comparison sees the glyphs alone.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		// Skip through the sequence's final byte ('m' for the SGR codes styles emit).
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return b.String()
}
