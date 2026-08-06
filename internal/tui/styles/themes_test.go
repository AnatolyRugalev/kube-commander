package styles

import (
	"image/color"
	"strings"
	"testing"
)

// themeColors projects a Theme onto its named color roles, so a test can assert
// over every role without listing them at each call site (and so adding a role
// to Theme fails to compile here until it is covered).
func themeColors(t Theme) map[string]color.Color {
	return map[string]color.Color{
		"Foreground":  t.Foreground,
		"Subtle":      t.Subtle,
		"Primary":     t.Primary,
		"Selection":   t.Selection,
		"SelectionFg": t.SelectionFg,
		"Border":      t.Border,
		"BorderFocus": t.BorderFocus,
		"Header":      t.Header,
		"StatusBarFg": t.StatusBarFg,
		"StatusBarBg": t.StatusBarBg,
		"Error":       t.Error,
		"Warn":        t.Warn,
		"Success":     t.Success,
	}
}

func TestBuiltinThemesAreComplete(t *testing.T) {
	for _, th := range Themes() {
		if th.Name == "" {
			t.Error("a built-in theme has an empty Name")
			continue
		}
		// A nil role is not a fallback: lipgloss simply doesn't apply it, so the
		// terminal default leaks through and the theme is silently half-applied.
		for role, c := range themeColors(th) {
			if c == nil {
				t.Errorf("theme %q: %s is nil", th.Name, role)
			}
		}
	}
}

func TestBuiltinThemeNamesAreUniqueAndDefaultFirst(t *testing.T) {
	themes := Themes()
	if len(themes) < 3 {
		t.Fatalf("Themes() returned %d themes, want the default + at least two ports", len(themes))
	}
	if themes[0].Name != DefaultTheme().Name {
		t.Errorf("Themes()[0].Name = %q, want the default %q", themes[0].Name, DefaultTheme().Name)
	}
	seen := map[string]bool{}
	for _, th := range themes {
		if seen[th.Name] {
			t.Errorf("duplicate built-in theme name %q", th.Name)
		}
		seen[th.Name] = true
	}
	// The tail is sorted, so a picker and a generated doc list the same order.
	for i := 2; i < len(themes); i++ {
		if themes[i-1].Name > themes[i].Name {
			t.Errorf("Themes() tail is not sorted: %q before %q", themes[i-1].Name, themes[i].Name)
		}
	}
}

func TestThemeNamesMatchThemes(t *testing.T) {
	themes := Themes()
	names := ThemeNames()
	if len(names) != len(themes) {
		t.Fatalf("ThemeNames() has %d entries, Themes() has %d", len(names), len(themes))
	}
	for i, n := range names {
		if n != themes[i].Name {
			t.Errorf("ThemeNames()[%d] = %q, want %q", i, n, themes[i].Name)
		}
	}
}

func TestByNameRoundTripsEveryBuiltin(t *testing.T) {
	for _, th := range Themes() {
		got, ok := ByName(th.Name)
		if !ok {
			t.Errorf("ByName(%q) not found", th.Name)
			continue
		}
		if got.Name != th.Name {
			t.Errorf("ByName(%q).Name = %q", th.Name, got.Name)
		}
		if got.Foreground != th.Foreground {
			t.Errorf("ByName(%q) returned a different palette", th.Name)
		}
	}
}

func TestByNameIsLenientAboutCaseAndSpace(t *testing.T) {
	// The name comes from a hand-edited config file, so surrounding space and
	// letter case must not decide whether a theme exists.
	for _, in := range []string{"Monokai", "  monokai ", "MONOKAI"} {
		got, ok := ByName(in)
		if !ok || got.Name != "monokai" {
			t.Errorf("ByName(%q) = (%q, %v), want the monokai theme", in, got.Name, ok)
		}
	}
}

func TestByNameRejectsUnknownAndPartialNames(t *testing.T) {
	// Lenient about formatting, never fuzzy: a near-miss must be reported as
	// unknown so the caller can degrade loudly rather than silently pick.
	for _, in := range []string{"", "   ", "nope", "mono", "solarized", "solarized-light"} {
		if got, ok := ByName(in); ok {
			t.Errorf("ByName(%q) = %q, want not found", in, got.Name)
		}
	}
}

func TestThemesResultIsNotSharedState(t *testing.T) {
	first := Themes()
	first[1].Name = "clobbered"
	if second := Themes(); second[1].Name == "clobbered" {
		t.Error("Themes() hands out shared state; a caller mutated the registry")
	}
}

// aliasPair is the registry's one deliberate duplicate: kubecom's `default`
// palette has always *been* Catppuccin Frappé, and D169 pt 1 forbids renaming a
// shipped theme, so the palette carries both names instead (D236 pt 2). Every
// other pair of built-ins must render differently.
func aliasPair(a, b string) bool {
	return (a == "default" && b == "catppuccin-frappe") ||
		(a == "catppuccin-frappe" && b == "default")
}

func TestBuiltinThemesRenderDistinctly(t *testing.T) {
	// Selecting a theme must actually change what is drawn — two themes whose
	// styles render identically would make the picker a no-op.
	seen := map[string]string{}
	for _, th := range Themes() {
		out := New(th).Selection.Render("row")
		for name, prev := range seen {
			if prev == out && !aliasPair(name, th.Name) {
				t.Errorf("themes %q and %q render identically", name, th.Name)
			}
		}
		seen[th.Name] = out
	}
}

func TestDefaultThemeIsCatppuccinFrappe(t *testing.T) {
	// The alias is a claim about values, not a comment: if a later leg retunes
	// `default`, it has either moved off Frappé (and the docs, the README table
	// and D236 pt 2 are now wrong) or it retuned Catppuccin's published palette.
	def, frappe := themeColors(DefaultTheme()), themeColors(CatppuccinFrappeTheme())
	for role, c := range def {
		if c != frappe[role] {
			t.Errorf("default.%s = %v, catppuccin-frappe.%s = %v — the alias has drifted",
				role, c, role, frappe[role])
		}
	}
	if DefaultTheme().Name != "default" {
		t.Errorf("DefaultTheme().Name = %q, want %q — the name is API (D169 pt 1)",
			DefaultTheme().Name, "default")
	}
}

func TestCatppuccinFlavorsAreDarkAndNamedForTheFlavor(t *testing.T) {
	// The family ships its dark flavors only: kubecom paints no app background,
	// so Latte's dark text would land on whatever the terminal already is
	// (D236 pt 3). Each name is `catppuccin-<flavor>` so the family filters as
	// one in the theme picker.
	want := map[string]bool{
		"catppuccin-frappe":    true,
		"catppuccin-macchiato": true,
		"catppuccin-mocha":     true,
	}
	got := map[string]bool{}
	for _, th := range Themes() {
		if strings.HasPrefix(th.Name, "catppuccin-") {
			got[th.Name] = true
		}
	}
	if len(got) != len(want) {
		t.Errorf("catppuccin themes = %v, want %v", got, want)
	}
	for name := range want {
		if !got[name] {
			t.Errorf("built-in %q is missing", name)
		}
	}
	if _, ok := ByName("catppuccin-latte"); ok {
		t.Error("catppuccin-latte is registered, but no built-in sets an app background yet (D236 pt 3)")
	}
}
