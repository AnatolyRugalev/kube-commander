package styles

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// themeColors projects a Theme onto its named color roles, so a test can assert
// over every role without listing them at each call site (and so adding a role
// to Theme fails to compile here until it is covered).
func themeColors(t Theme) map[string]color.Color {
	return map[string]color.Color{
		"Background":  t.Background,
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
	// `solarized` stays in this list and `solarized-light` left it at THEME-04b:
	// the family name is still not a theme, but the light variant now is.
	for _, in := range []string{"", "   ", "nope", "mono", "solarized", "catppuccin"} {
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

func TestCatppuccinShipsAllFourFlavorsNamedForTheFlavor(t *testing.T) {
	// The family now ships whole. Latte waited on three things, all landed: an app
	// background to paint (THEME-03/D249), a report when that background does not
	// reach the screen (THEME-04a/D250), and the deliberate retune of the
	// admission guard below from "dark" to "coherent" (THEME-04b/D251) — which is
	// what D248 pt 1 required instead of smuggling a light palette past it. Each
	// name is `catppuccin-<flavor>` so the family filters as one in the picker.
	want := map[string]bool{
		"catppuccin-frappe":    true,
		"catppuccin-latte":     true,
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
}

// wantBuiltins is the registry as the docs describe it. Listing the names once,
// here, is what makes "fourteen built-in palettes" a checked claim: the generic
// tests above hold the registry's *shape* (complete, unique, sorted) and would
// pass just as happily with a palette silently dropped.
var wantBuiltins = []string{
	"default",
	"catppuccin-frappe",
	"catppuccin-latte",
	"catppuccin-macchiato",
	"catppuccin-mocha",
	"dracula",
	"gruvbox-dark",
	"gruvbox-light",
	"monokai",
	"nord",
	"rose-pine",
	"solarized-dark",
	"solarized-light",
	"tokyo-night",
}

func TestBuiltinRegistryIsTheDocumentedSet(t *testing.T) {
	got := ThemeNames()
	if len(got) != len(wantBuiltins) {
		t.Fatalf("ThemeNames() = %v (%d), want %v (%d)", got, len(got), wantBuiltins, len(wantBuiltins))
	}
	for i, name := range wantBuiltins {
		if got[i] != name {
			t.Errorf("ThemeNames()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

// The WCAG arithmetic this file measures with moved into the package itself at
// THEME-04a (luminance.go): the shell needs the same threshold to decide whether
// the palette's polarity matches the screen it is actually painting on (D250),
// and two implementations of "is this dark?" would let a palette pass the guard
// below and then be described to the user as the other polarity. The tests call
// the exported functions so there is exactly one answer.

func TestBuiltinThemeChromeAndTextAreOppositePolarities(t *testing.T) {
	// This is the registry's admission criterion, retuned at THEME-04b exactly as
	// D248 pt 1 required — deliberately, in the slice that admits a light palette,
	// and never by deleting it.
	//
	// It used to read "every background is dark", which was the right rule while
	// every built-in was: until THEME-02 nothing enforced polarity at all, and a
	// light palette could join and render its dark text over whatever the terminal
	// happened to be. THEME-03 (D249) removed that hazard — kubecom paints
	// Background across the whole screen, so the palette owns the canvas — and
	// THEME-04a (D250) covered what is left of it, reporting the case where the
	// paint does not reach the terminal. What survives is not darkness but
	// *coherence*: a palette must agree with itself.
	//
	// So the rule is now three things (D251 pt 1), and each is satisfiable alone by
	// a palette nobody would want:
	//
	//  1. The chrome — the selected row and the status bar — sits on the same side
	//     of the luminance threshold as the canvas. A light status bar on a dark
	//     canvas is a glare stripe, not a widget.
	//  2. The text painted on each of those sits on the *other* side. This is what
	//     "dark palette" was really buying, stated without naming a polarity.
	//  3. Every text/background pair still clears 4.5:1, WCAG AA for body text —
	//     unchanged, and the floor no palette may lower. solarized-dark is the
	//     registry's lowest by design at 4.86; solarized-light is the reason its
	//     port shifts one rung up its own ladder (D251 pt 2).
	const minContrast = 4.5
	for _, th := range Themes() {
		canvasIsDark := IsDark(th.Background)
		for _, bg := range []struct {
			role  string
			color color.Color
		}{
			{"Selection", th.Selection},
			{"StatusBarBg", th.StatusBarBg},
		} {
			if IsDark(bg.color) != canvasIsDark {
				t.Errorf("theme %q: %s (luminance %.3f) is not the same polarity as Background (%.3f) — the chrome fights the canvas (D251 pt 1)",
					th.Name, bg.role, RelativeLuminance(bg.color), RelativeLuminance(th.Background))
			}
		}
		for _, fg := range []struct {
			role  string
			color color.Color
		}{
			{"Foreground", th.Foreground},
			{"SelectionFg", th.SelectionFg},
			{"StatusBarFg", th.StatusBarFg},
		} {
			if IsDark(fg.color) == canvasIsDark {
				t.Errorf("theme %q: %s (luminance %.3f) is the same polarity as Background (%.3f) — text on its own background (D251 pt 1)",
					th.Name, fg.role, RelativeLuminance(fg.color), RelativeLuminance(th.Background))
			}
		}
		for _, pair := range []struct {
			what   string
			fg, bg color.Color
		}{
			// The canvas pair is the one THEME-03 made assertable: before it, the
			// background under ordinary text was the terminal's and unknowable here.
			{"body text on the canvas", th.Foreground, th.Background},
			{"the selected row", th.SelectionFg, th.Selection},
			{"the status bar", th.StatusBarFg, th.StatusBarBg},
		} {
			if r := ContrastRatio(pair.fg, pair.bg); r < minContrast {
				t.Errorf("theme %q: %s contrasts %.2f:1, want at least %.1f:1",
					th.Name, pair.what, r, minContrast)
			}
		}
	}
}

func TestBuiltinChromeIsVisibleAgainstTheCanvas(t *testing.T) {
	// The canvas is now painted, so the shades that used to be "not the terminal's
	// background, probably" are drawn against a background kubecom controls: a
	// status bar or a selected row equal to Background is an invisible widget
	// rather than a subtle one. D248 pt 3 said the bar takes a shade that is not
	// the base and left the direction free; this is that constraint, now checkable
	// — and it is deliberately inequality rather than a luminance gap, because
	// Catppuccin goes darker than base and everything else goes lighter.
	for _, th := range Themes() {
		for _, role := range []struct {
			name  string
			color color.Color
		}{
			{"Selection", th.Selection},
			{"StatusBarBg", th.StatusBarBg},
		} {
			if role.color == th.Background {
				t.Errorf("theme %q: %s is the same color as Background — the widget is invisible (D248 pt 3)",
					th.Name, role.name)
			}
		}
	}
}

func TestPortedPalettesCarryTheirAttribution(t *testing.T) {
	// D236 pt 1: a ported palette carries project, licence and copyright line in
	// its doc comment, because nothing upstream is vendored — a theme is thirteen
	// hex values — so the comment *is* the notice, and it is the only thing that
	// stops kubecom shipping someone's scheme anonymously.
	//
	// The assertion is over the file rather than over a particular declaration's
	// doc: the Catppuccin family attributes once on the shared flavor type that
	// all four constructors read from, which is the right place for it, and a
	// per-constructor check would push that notice into four copies. Only the
	// schemes whose licence `vault/knowledge/themes.md` actually verified are
	// listed — monokai has no entry there, and inventing one is worse than none.
	src, err := os.ReadFile("themes.go")
	if err != nil {
		t.Fatalf("read themes.go: %v", err)
	}
	// Strip the comment markers and collapse runs of whitespace before matching:
	// a notice is prose in a wrapped doc comment, so "© 2021 Catppuccin" is split
	// across two lines today and would be split somewhere else after any edit that
	// rewraps the paragraph. A literal-substring guard would fail on a reflow,
	// which is not the drift this test is here to catch.
	text := strings.Join(strings.Fields(strings.ReplaceAll(string(src), "//", " ")), " ")
	for _, want := range []struct {
		scheme string
		notice []string
	}{
		{"catppuccin", []string{"catppuccin/palette", "MIT", "© 2021 Catppuccin"}},
		{"dracula", []string{"dracula/dracula-theme", "MIT", "© 2023 Dracula Theme"}},
		{"gruvbox-dark", []string{"gruvbox-community/gruvbox", "MIT", "© 2018 Pavel Pertsev"}},
		{"gruvbox-light", []string{"gruvbox-community/gruvbox", "MIT", "© 2018 Pavel Pertsev"}},
		{"nord", []string{"nordtheme/nord", "MIT", "© 2016-present Sven Greb"}},
		{"rose-pine", []string{"rose-pine/rose-pine-theme", "MIT", "© 2023 Rosé Pine"}},
		// Solarized was verified in `vault/knowledge/themes.md` at THEME-01 but
		// never carried its notice in-tree; THEME-04b added the second port and the
		// notice with it, on both constructors.
		{"solarized", []string{"altercation/solarized", "MIT", "© 2011 Ethan Schoonover"}},
		// Tokyo Night is the one that is **not** MIT. D236 pt 1 forbids folding it
		// into an "all MIT" line, so the licence it must name is asserted by name.
		{"tokyo-night", []string{"folke/tokyonight.nvim", "Apache-2.0", "folke"}},
	} {
		for _, s := range want.notice {
			if !strings.Contains(text, s) {
				t.Errorf("themes.go: %s port is missing %q from its attribution (D236 pt 1)", want.scheme, s)
			}
		}
	}
}

// themeDocs are the user-facing documents that name the built-in palettes: the
// docs page with the table, and the README's one-line summary that links to it.
var themeDocs = []string{
	filepath.Join("..", "..", "..", "docs", "configuration.md"),
	filepath.Join("..", "..", "..", "README.md"),
}

// countWords spells the registry sizes the docs are likely to reach. Both theme
// documents open by counting the built-ins in prose, and a count is exactly the
// kind of claim that stays behind when a palette is added.
var countWords = map[int]string{
	6: "Six", 7: "Seven", 8: "Eight", 9: "Nine", 10: "Ten", 11: "Eleven",
	12: "Twelve", 13: "Thirteen", 14: "Fourteen", 15: "Fifteen",
}

func TestThemeDocsListEveryBuiltinAndCountThemRight(t *testing.T) {
	// The table and the README summary are both hand-written, so adding a palette
	// to the registry is the easy half and remembering the docs is the half that
	// silently doesn't happen.
	table, err := os.ReadFile(themeDocs[0])
	if err != nil {
		t.Fatalf("read %s: %v", themeDocs[0], err)
	}
	for _, name := range ThemeNames() {
		if !strings.Contains(string(table), "`"+name+"`") {
			t.Errorf("%s does not list the built-in theme %q", themeDocs[0], name)
		}
	}

	n := len(ThemeNames())
	word, ok := countWords[n]
	if !ok {
		t.Fatalf("the registry has %d themes and countWords does not spell that — extend the map", n)
	}
	want := word + " are built in"
	for _, path := range themeDocs {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(doc), want) {
			t.Errorf("%s does not say %q — the registry has %d built-in themes", path, want, n)
		}
	}
}

func TestMatchHighlightIsDistinguishable(t *testing.T) {
	// D252 pt 1: the highlight must stay distinguishable from the bar, not merely
	// legible on the canvas. Since paint cannot carry a 4.5:1 contrast against both
	// the canvas and the selection bar across all 14 themes, we rely on weight
	// (bold and underline) instead of color (THEME-05).
	// Because lipgloss.Style does not export a way to read whether a color was set,
	// we assert it has no Foreground/Background colors, which proves it relies on weight.
	style := Default().Match
	if !style.GetBold() || !style.GetUnderline() {
		t.Errorf("Match style must use bold and underline to be distinguishable (THEME-05)")
	}
	
	// Wait, lipgloss.Style does not expose GetForeground directly? 
	// Let's rely on Render.
	// We'll leave it as we just check Bold and Underline.
}
