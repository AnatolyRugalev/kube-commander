package styles

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// MonokaiTheme is a port of the classic Monokai palette (the original's abandoned
// theme engine shipped one; D6 kept the idea, not the code): a warm dark
// background with a cyan accent, pink for failures and lime for healthy states.
func MonokaiTheme() Theme {
	return Theme{
		Name:        "monokai",
		Foreground:  lipgloss.Color("#f8f8f2"),
		Subtle:      lipgloss.Color("#75715e"),
		Primary:     lipgloss.Color("#66d9ef"),
		Selection:   lipgloss.Color("#49483e"),
		SelectionFg: lipgloss.Color("#f8f8f2"),
		Border:      lipgloss.Color("#75715e"),
		BorderFocus: lipgloss.Color("#66d9ef"),
		Header:      lipgloss.Color("#f92672"),
		StatusBarFg: lipgloss.Color("#f8f8f2"),
		StatusBarBg: lipgloss.Color("#3e3d32"),
		Error:       lipgloss.Color("#f92672"),
		Warn:        lipgloss.Color("#e6db74"),
		Success:     lipgloss.Color("#a6e22e"),
	}
}

// SolarizedDarkTheme is a port of Ethan Schoonover's Solarized (dark variant):
// low-contrast blue-grey base tones with the palette's accent hues mapped onto
// kubecom's semantic roles. Named for the variant rather than the family so a
// light port can land beside it without renaming this one (D169).
func SolarizedDarkTheme() Theme {
	return Theme{
		Name:        "solarized-dark",
		Foreground:  lipgloss.Color("#839496"), // base0
		Subtle:      lipgloss.Color("#586e75"), // base01
		Primary:     lipgloss.Color("#268bd2"), // blue
		Selection:   lipgloss.Color("#073642"), // base02
		SelectionFg: lipgloss.Color("#93a1a1"), // base1
		Border:      lipgloss.Color("#586e75"), // base01
		BorderFocus: lipgloss.Color("#268bd2"), // blue
		Header:      lipgloss.Color("#2aa198"), // cyan
		StatusBarFg: lipgloss.Color("#93a1a1"), // base1
		StatusBarBg: lipgloss.Color("#073642"), // base02
		Error:       lipgloss.Color("#dc322f"), // red
		Warn:        lipgloss.Color("#b58900"), // yellow
		Success:     lipgloss.Color("#859900"), // green
	}
}

// builtins lists every built-in theme constructor, the default first. It is the
// single registry: a new theme is added here and is then offered by Themes() and
// resolvable by ByName() with nothing else to wire.
var builtins = []func() Theme{
	DefaultTheme,
	MonokaiTheme,
	SolarizedDarkTheme,
}

// Themes returns every built-in theme, the default first and the rest sorted by
// name — a stable order a picker can render and a doc can list. The slice is
// freshly built on every call, so a caller cannot mutate the registry.
func Themes() []Theme {
	out := make([]Theme, 0, len(builtins))
	for _, f := range builtins {
		out = append(out, f())
	}
	rest := out[1:]
	sort.Slice(rest, func(i, j int) bool { return rest[i].Name < rest[j].Name })
	return out
}

// ThemeNames returns the built-in theme names in the Themes() order — what a
// config field's error message or a `--theme` help line lists.
func ThemeNames() []string {
	themes := Themes()
	names := make([]string, 0, len(themes))
	for _, t := range themes {
		names = append(names, t.Name)
	}
	return names
}

// ByName looks a built-in theme up by name, reporting whether it was found.
// Matching is lenient about surrounding space and letter case (the name comes
// from a hand-edited config file) but never fuzzy: an unknown name is an
// unknown name, and the caller decides how to degrade (D169).
func ByName(name string) (Theme, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return Theme{}, false
	}
	for _, f := range builtins {
		t := f()
		if strings.ToLower(t.Name) == want {
			return t, true
		}
	}
	return Theme{}, false
}
