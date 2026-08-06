package styles

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// catppuccinFlavor is one Catppuccin flavor's raw palette, named with the
// upstream role names (https://github.com/catppuccin/palette, MIT, © 2021
// Catppuccin) so a value can be checked against the source without decoding
// kubecom's semantic mapping first. Every flavor maps onto Theme identically
// (catppuccinTheme), which is what makes them one family rather than four
// separately-tuned palettes.
type catppuccinFlavor struct {
	text, overlay1, blue, surface0, surface1 string
	rosewater, mantle, red, yellow, green    string
}

// The four flavors' values, transcribed from catppuccin/palette. Latte (the
// light flavor) is deliberately absent — kubecom paints no app background, so a
// light palette's dark text lands on whatever the terminal is (D236 pt 3).
var (
	catppuccinFrappeFlavor = catppuccinFlavor{
		text: "#c6d0f5", overlay1: "#838ba7", blue: "#8caaee",
		surface0: "#414559", surface1: "#51576d", rosewater: "#f2d5cf",
		mantle: "#292c3c", red: "#e78284", yellow: "#e5c890", green: "#a6d189",
	}
	catppuccinMacchiatoFlavor = catppuccinFlavor{
		text: "#cad3f5", overlay1: "#8087a2", blue: "#8aadf4",
		surface0: "#363a4f", surface1: "#494d64", rosewater: "#f4dbd6",
		mantle: "#1e2030", red: "#ed8796", yellow: "#eed49f", green: "#a6da95",
	}
	catppuccinMochaFlavor = catppuccinFlavor{
		text: "#cdd6f4", overlay1: "#7f849c", blue: "#89b4fa",
		surface0: "#313244", surface1: "#45475a", rosewater: "#f5e0dc",
		mantle: "#181825", red: "#f38ba8", yellow: "#f9e2af", green: "#a6e3a1",
	}
)

// catppuccinTheme maps a flavor onto kubecom's semantic roles under the given
// name. The mapping is the family's, not the flavor's: text/overlay1 for the two
// text weights, blue as the accent, surface0/surface1 for selection and chrome,
// rosewater for headers, mantle behind the status bar, and the flavor's own
// red/yellow/green for the semantic trio.
func catppuccinTheme(name string, f catppuccinFlavor) Theme {
	return Theme{
		Name:        name,
		Foreground:  lipgloss.Color(f.text),
		Subtle:      lipgloss.Color(f.overlay1),
		Primary:     lipgloss.Color(f.blue),
		Selection:   lipgloss.Color(f.surface0),
		SelectionFg: lipgloss.Color(f.text),
		Border:      lipgloss.Color(f.surface1),
		BorderFocus: lipgloss.Color(f.blue),
		Header:      lipgloss.Color(f.rosewater),
		StatusBarFg: lipgloss.Color(f.text),
		StatusBarBg: lipgloss.Color(f.mantle),
		Error:       lipgloss.Color(f.red),
		Warn:        lipgloss.Color(f.yellow),
		Success:     lipgloss.Color(f.green),
	}
}

// CatppuccinFrappeTheme is Catppuccin's Frappé flavor — the mid-dark one. It is
// the same palette DefaultTheme renders: kubecom's default has always been
// Frappé, and D169 pt 1 forbids renaming a shipped theme, so the palette carries
// both names rather than one of them moving (D236 pt 2).
func CatppuccinFrappeTheme() Theme {
	return catppuccinTheme("catppuccin-frappe", catppuccinFrappeFlavor)
}

// CatppuccinMacchiatoTheme is Catppuccin's Macchiato flavor: darker and cooler
// than Frappé, lighter than Mocha.
func CatppuccinMacchiatoTheme() Theme {
	return catppuccinTheme("catppuccin-macchiato", catppuccinMacchiatoFlavor)
}

// CatppuccinMochaTheme is Catppuccin's Mocha flavor — the darkest, and the one
// most ports use as their default.
func CatppuccinMochaTheme() Theme {
	return catppuccinTheme("catppuccin-mocha", catppuccinMochaFlavor)
}

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
	CatppuccinFrappeTheme,
	CatppuccinMacchiatoTheme,
	CatppuccinMochaTheme,
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
