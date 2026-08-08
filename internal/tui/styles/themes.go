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
	rosewater, base, mantle, red             string
	yellow, green                            string
}

// All four flavors' values, transcribed from catppuccin/palette. Latte, the light
// one, joined at THEME-04b: THEME-03 gave kubecom an app background (D249),
// THEME-04a made it report a canvas that did not land (D250), and this slice
// retuned the registry's admission guard from "dark" to "coherent" (D251) — the
// three things D236 pt 3 was waiting for.
var (
	catppuccinLatteFlavor = catppuccinFlavor{
		text: "#4c4f69", overlay1: "#8c8fa1", blue: "#1e66f5",
		surface0: "#ccd0da", surface1: "#bcc0cc", rosewater: "#dc8a78",
		base: "#eff1f5", mantle: "#e6e9ef",
		red: "#d20f39", yellow: "#df8e1d", green: "#40a02b",
	}
	catppuccinFrappeFlavor = catppuccinFlavor{
		text: "#c6d0f5", overlay1: "#838ba7", blue: "#8caaee",
		surface0: "#414559", surface1: "#51576d", rosewater: "#f2d5cf",
		base: "#303446", mantle: "#292c3c",
		red: "#e78284", yellow: "#e5c890", green: "#a6d189",
	}
	catppuccinMacchiatoFlavor = catppuccinFlavor{
		text: "#cad3f5", overlay1: "#8087a2", blue: "#8aadf4",
		surface0: "#363a4f", surface1: "#494d64", rosewater: "#f4dbd6",
		base: "#24273a", mantle: "#1e2030",
		red: "#ed8796", yellow: "#eed49f", green: "#a6da95",
	}
	catppuccinMochaFlavor = catppuccinFlavor{
		text: "#cdd6f4", overlay1: "#7f849c", blue: "#89b4fa",
		surface0: "#313244", surface1: "#45475a", rosewater: "#f5e0dc",
		base: "#1e1e2e", mantle: "#181825",
		red: "#f38ba8", yellow: "#f9e2af", green: "#a6e3a1",
	}
)

// catppuccinTheme maps a flavor onto kubecom's semantic roles under the given
// name. The mapping is the family's, not the flavor's: base as the canvas,
// text/overlay1 for the two text weights, blue as the accent, surface0/surface1
// for selection and chrome, rosewater for headers, mantle behind the status bar,
// and the flavor's own red/yellow/green for the semantic trio.
//
// The mapping is polarity-agnostic, and Latte goes through it unchanged. Every
// role names a *rung* of the flavor's own ladder — text is the flavor's text,
// mantle is the step off base the status bar sits on — and Catppuccin builds
// Latte on the same rungs it builds the dark three on. So the whole palette
// inverts while not one line here changes, which is what makes Latte the
// family's fourth flavor rather than a separately-tuned light theme.
func catppuccinTheme(name string, f catppuccinFlavor) Theme {
	return Theme{
		Name:        name,
		Background:  lipgloss.Color(f.base),
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

// CatppuccinLatteTheme is Catppuccin's Latte flavor — the family's light one, and
// the first light palette in kubecom's registry. Nothing about the mapping is
// special-cased for it (see catppuccinTheme); what it needed was a kubecom that
// paints the canvas (D249) and says so when the canvas did not land (D250).
func CatppuccinLatteTheme() Theme {
	return catppuccinTheme("catppuccin-latte", catppuccinLatteFlavor)
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
		Background:  lipgloss.Color("#272822"),
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

// SolarizedDarkTheme is a port of Ethan Schoonover's Solarized, dark variant
// (https://github.com/altercation/solarized, MIT, © 2011 Ethan Schoonover):
// low-contrast blue-grey base tones with the palette's accent hues mapped onto
// kubecom's semantic roles. Named for the variant rather than the family so a
// light port can land beside it without renaming this one (D169) — which is
// exactly what SolarizedLightTheme below then did.
func SolarizedDarkTheme() Theme {
	return Theme{
		Name:        "solarized-dark",
		Background:  lipgloss.Color("#002b36"), // base03
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

// SolarizedLightTheme is the light half of the same scheme
// (https://github.com/altercation/solarized, MIT, © 2011 Ethan Schoonover).
// Solarized is designed as one palette read from either end — light swaps
// base03↔base3, base02↔base2, base01↔base1 and base00↔base0, leaving the eight
// accents alone — so this is SolarizedDarkTheme's mirror, with **one deliberate
// departure** (D251 pt 2).
//
// The departure: the scheme's canonical light body pair, base00 on base3,
// measures 4.13:1 — below the 4.5:1 floor every built-in is held to, because
// Solarized is low-contrast by design and its light end is the lower of the two.
// Rather than lower the floor or invent a value, the port takes the next rung of
// Solarized's own ladder for each text role: body text is base01 (the scheme's
// "optional emphasized content" for a light background, 4.98:1 on base3) and the
// chrome text is base02 (10.6:1 on base2). That preserves the dark port's own
// relationship — chrome text one step more emphasized than body text — and it is
// the whole ladder that shifts, not a single value picked to clear a threshold.
func SolarizedLightTheme() Theme {
	return Theme{
		Name:        "solarized-light",
		Background:  lipgloss.Color("#fdf6e3"), // base3
		Foreground:  lipgloss.Color("#586e75"), // base01
		Subtle:      lipgloss.Color("#93a1a1"), // base1
		Primary:     lipgloss.Color("#268bd2"), // blue
		Selection:   lipgloss.Color("#eee8d5"), // base2
		SelectionFg: lipgloss.Color("#073642"), // base02
		Border:      lipgloss.Color("#93a1a1"), // base1
		BorderFocus: lipgloss.Color("#268bd2"), // blue
		Header:      lipgloss.Color("#2aa198"), // cyan
		StatusBarFg: lipgloss.Color("#073642"), // base02
		StatusBarBg: lipgloss.Color("#eee8d5"), // base2
		Error:       lipgloss.Color("#dc322f"), // red
		Warn:        lipgloss.Color("#b58900"), // yellow
		Success:     lipgloss.Color("#859900"), // green
	}
}

// DraculaTheme is a port of Dracula (https://github.com/dracula/dracula-theme,
// MIT, © 2023 Dracula Theme), transcribed from the palette table in the project's
// own README. Purple is the accent and pink the header, which is how Dracula's
// spec weights its two signature hues. The published palette has exactly one
// shade above Background — "Current Line"/"Selection" — so the selected row and
// the status bar share it (solarized-dark already does the same).
func DraculaTheme() Theme {
	return Theme{
		Name:        "dracula",
		Background:  lipgloss.Color("#282a36"), // Background
		Foreground:  lipgloss.Color("#f8f8f2"), // Foreground
		Subtle:      lipgloss.Color("#6272a4"), // Comment
		Primary:     lipgloss.Color("#bd93f9"), // Purple
		Selection:   lipgloss.Color("#44475a"), // Selection
		SelectionFg: lipgloss.Color("#f8f8f2"), // Foreground
		Border:      lipgloss.Color("#6272a4"), // Comment
		BorderFocus: lipgloss.Color("#bd93f9"), // Purple
		Header:      lipgloss.Color("#ff79c6"), // Pink
		StatusBarFg: lipgloss.Color("#f8f8f2"), // Foreground
		StatusBarBg: lipgloss.Color("#44475a"), // Current Line
		Error:       lipgloss.Color("#ff5555"), // Red
		Warn:        lipgloss.Color("#f1fa8c"), // Yellow
		Success:     lipgloss.Color("#50fa7b"), // Green
	}
}

// GruvboxDarkTheme is a port of gruvbox's dark variant at medium contrast, from
// the palette block of `colors/gruvbox.vim` in the author's community fork
// (https://github.com/gruvbox-community/gruvbox, MIT, © 2018 Pavel Pertsev).
// The fork is the licence source on purpose: upstream `morhetz/gruvbox` ships no
// licence file at all (`vault/knowledge/themes.md`), so this attribution must not
// later be "corrected" to point there. Named for the variant so a light port can
// land beside it (D169 pt 1). The `bright_*` accents are the dark variant's, and
// bg1/bg2/bg3 give the status bar, the selected row and the border three
// separable shades above bg0 — which is what the terminal itself probably is.
func GruvboxDarkTheme() Theme {
	return Theme{
		Name:        "gruvbox-dark",
		Background:  lipgloss.Color("#282828"), // dark0 (bg0)
		Foreground:  lipgloss.Color("#ebdbb2"), // light1 (fg1)
		Subtle:      lipgloss.Color("#928374"), // gray_245
		Primary:     lipgloss.Color("#83a598"), // bright_blue
		Selection:   lipgloss.Color("#504945"), // dark2 (bg2)
		SelectionFg: lipgloss.Color("#ebdbb2"), // light1
		Border:      lipgloss.Color("#665c54"), // dark3 (bg3)
		BorderFocus: lipgloss.Color("#83a598"), // bright_blue
		Header:      lipgloss.Color("#d3869b"), // bright_purple
		StatusBarFg: lipgloss.Color("#ebdbb2"), // light1
		StatusBarBg: lipgloss.Color("#3c3836"), // dark1 (bg1)
		Error:       lipgloss.Color("#fb4934"), // bright_red
		Warn:        lipgloss.Color("#fabd2f"), // bright_yellow
		Success:     lipgloss.Color("#b8bb26"), // bright_green
	}
}

// NordTheme is a port of Nord (https://github.com/nordtheme/nord, MIT,
// © 2016-present Sven Greb), transcribed from `src/nord.css` on the `develop`
// branch — `main` 404s, which is worth knowing before re-checking a value.
// Nord's own docs assign most of these roles directly: nord1 is documented as
// "a lighter background color for UI elements like status bars", nord2 as the
// selection/highlight color, nord3 as comments and disabled elements, and nord8
// as the primary accent, so the mapping is upstream's rather than kubecom's.
func NordTheme() Theme {
	return Theme{
		Name:        "nord",
		Background:  lipgloss.Color("#2e3440"), // nord0
		Foreground:  lipgloss.Color("#d8dee9"), // nord4
		Subtle:      lipgloss.Color("#4c566a"), // nord3
		Primary:     lipgloss.Color("#88c0d0"), // nord8
		Selection:   lipgloss.Color("#434c5e"), // nord2
		SelectionFg: lipgloss.Color("#eceff4"), // nord6
		Border:      lipgloss.Color("#4c566a"), // nord3
		BorderFocus: lipgloss.Color("#88c0d0"), // nord8
		Header:      lipgloss.Color("#81a1c1"), // nord9
		StatusBarFg: lipgloss.Color("#d8dee9"), // nord4
		StatusBarBg: lipgloss.Color("#3b4252"), // nord1
		Error:       lipgloss.Color("#bf616a"), // nord11
		Warn:        lipgloss.Color("#ebcb8b"), // nord13
		Success:     lipgloss.Color("#a3be8c"), // nord14
	}
}

// RosePineTheme is a port of Rosé Pine's `main` (dark) variant
// (https://github.com/rose-pine/rose-pine-theme, MIT, © 2023 Rosé Pine),
// transcribed from the palette table in `rose-pine/neovim`'s
// `lua/rose-pine/palette.lua`, which is the machine-readable form the theme repo
// itself points at. Named for the family without a variant suffix because `main`
// is the scheme's own default name; `rose-pine-moon` and `rose-pine-dawn` would
// land beside it as new names (D236 pt 4). Success is `leaf` rather than `pine`:
// pine is the palette's dark teal and reads as barely-there against base, which
// is the wrong thing for "this workload is healthy".
func RosePineTheme() Theme {
	return Theme{
		Name:        "rose-pine",
		Background:  lipgloss.Color("#191724"), // base
		Foreground:  lipgloss.Color("#e0def4"), // text
		Subtle:      lipgloss.Color("#908caa"), // subtle
		Primary:     lipgloss.Color("#c4a7e7"), // iris
		Selection:   lipgloss.Color("#403d52"), // highlight_med
		SelectionFg: lipgloss.Color("#e0def4"), // text
		Border:      lipgloss.Color("#6e6a86"), // muted
		BorderFocus: lipgloss.Color("#c4a7e7"), // iris
		Header:      lipgloss.Color("#ebbcba"), // rose
		StatusBarFg: lipgloss.Color("#e0def4"), // text
		StatusBarBg: lipgloss.Color("#1f1d2e"), // surface
		Error:       lipgloss.Color("#eb6f92"), // love
		Warn:        lipgloss.Color("#f6c177"), // gold
		Success:     lipgloss.Color("#95b1ac"), // leaf
	}
}

// TokyoNightTheme is a port of Tokyo Night's `night` style — the darkest of the
// three and the one most ports mean by the bare name — from
// `lua/tokyonight/colors/{night,storm}.lua` (https://github.com/folke/tokyonight.nvim,
// **Apache-2.0**, folke). The licence is not MIT like the rest of the registry's
// ports and must not be folded into an "all MIT" line (D236 pt 1). `night` sets
// only the three background values and inherits every accent from `storm`, which
// is why the two files are cited together.
func TokyoNightTheme() Theme {
	return Theme{
		Name:        "tokyo-night",
		Background:  lipgloss.Color("#1a1b26"), // bg
		Foreground:  lipgloss.Color("#c0caf5"), // fg
		Subtle:      lipgloss.Color("#565f89"), // comment
		Primary:     lipgloss.Color("#7aa2f7"), // blue
		Selection:   lipgloss.Color("#414868"), // terminal_black
		SelectionFg: lipgloss.Color("#c0caf5"), // fg
		Border:      lipgloss.Color("#3b4261"), // fg_gutter
		BorderFocus: lipgloss.Color("#7aa2f7"), // blue
		Header:      lipgloss.Color("#bb9af7"), // magenta
		StatusBarFg: lipgloss.Color("#c0caf5"), // fg
		StatusBarBg: lipgloss.Color("#292e42"), // bg_highlight
		Error:       lipgloss.Color("#f7768e"), // red
		Warn:        lipgloss.Color("#e0af68"), // yellow
		Success:     lipgloss.Color("#9ece6a"), // green
	}
}

// builtins lists every built-in theme constructor, the default first. It is the
// single registry: a new theme is added here and is then offered by Themes() and
// resolvable by ByName() with nothing else to wire.
var builtins = []func() Theme{
	DefaultTheme,
	CatppuccinFrappeTheme,
	CatppuccinLatteTheme,
	CatppuccinMacchiatoTheme,
	CatppuccinMochaTheme,
	DraculaTheme,
	GruvboxDarkTheme,
	MonokaiTheme,
	NordTheme,
	RosePineTheme,
	SolarizedDarkTheme,
	SolarizedLightTheme,
	TokyoNightTheme,
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
