// Package styles is kubecom's single source of visual truth: a Theme (a set of
// named, semantic colors) and a Styles set (the lipgloss.Style values every
// component renders through), derived from a Theme. Keeping colors named and
// centralized here means a component never hard-codes a hex value, and a future
// theme switcher (M4) only has to swap the Theme.
//
// Everything in this package is pure, immutable data: a Theme is a struct of
// colors, a Styles is a struct of lipgloss.Style values, and lipgloss.Style is
// itself an immutable value type (every setter returns a copy). There is no
// shared mutable state, so a Styles can be copied into any model and read
// concurrently without coordination (principle 1). Components should hold a
// Styles by value and render through it; they must not reach for lipgloss.Color
// or NewStyle directly.
package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is the palette: named, semantic colors with no styling attached. A
// component asks for a role ("the selected row", "an error") rather than a
// literal color, so re-theming is a matter of building a Styles from a different
// Theme. All fields are set by the constructors below; a zero Theme renders with
// the terminal defaults (nil colors are simply not applied by lipgloss).
type Theme struct {
	// Name identifies the theme (for a future theme picker / config field).
	Name string

	// The canvas. Background is the color every cell kubecom does not explicitly
	// paint takes: the root View hands it to the terminal as its default
	// background for as long as kubecom holds the screen (D249), so panes,
	// borders, gaps and the areas between them all sit on it rather than on
	// whatever the terminal happened to be. Foreground is the text drawn there.
	Background color.Color // app background (the terminal's default while running)
	Foreground color.Color // default text
	Subtle     color.Color // muted secondary text (help descriptions, hints)

	// Accent and selection.
	Primary     color.Color // accent: focused border, spinner, key hints
	Selection   color.Color // selected-row background
	SelectionFg color.Color // selected-row foreground (on Selection)

	// Chrome.
	Border      color.Color // unfocused pane border
	BorderFocus color.Color // focused pane border
	Header      color.Color // table header text
	StatusBarFg color.Color // status-bar text
	StatusBarBg color.Color // status-bar background

	// Semantic status.
	Error   color.Color // failures, RBAC denials
	Warn    color.Color // warnings (e.g. keymap nav-shadow, degraded feature)
	Success color.Color // healthy / ready states
}

// DefaultTheme is kubecom's built-in theme: a dark-friendly palette with a blue
// accent. It has always been Catppuccin's Frappé flavor, and says so since
// THEME-01 — the values live once, in themes.go, and CatppuccinFrappeTheme
// returns the same palette under its own name (D236 pt 2; the name `default`
// cannot move, D169 pt 1). Truecolor hex values; terminals without truecolor
// downsample at write time via the Bubble Tea renderer, so no per-terminal
// branching is needed here.
func DefaultTheme() Theme {
	return catppuccinTheme("default", catppuccinFrappeFlavor)
}

// Styles is the derived set of lipgloss.Style values components render through,
// one per visual role. It is pure data (lipgloss.Style is an immutable value),
// carries its source Theme so a component can reach a raw color when it must
// (e.g. a border color for a bubbles component that wants a color, not a Style),
// and is safe to copy and read concurrently.
type Styles struct {
	// Theme is the palette this set was built from.
	Theme Theme

	// App is the base text style (default foreground); the canvas everything
	// else layers on.
	App lipgloss.Style

	// Subtle renders muted secondary text (help descriptions, inactive hints).
	Subtle lipgloss.Style

	// Selection is the highlighted row in a list, menu, or table.
	Selection lipgloss.Style

	// Accent is accented, emphasised text (Primary foreground, bold) with no
	// background bar — the role for a marked-but-not-cursor item, e.g. the menu's
	// opened/active resource shown while the nav cursor sits elsewhere. Distinct
	// from Selection (which paints a full-width background bar) so the two states
	// never look alike.
	Accent lipgloss.Style

	// Header is a table's column-header row (bold, accented).
	Header lipgloss.Style

	// Match highlights the span of text that matched an active query — the logs
	// view's live grep (LOGS-03), the table filter (FILT-02) and cluster-search
	// hits (SEARCH-06). It always carries weight (bold and underline, THEME-05),
	// and on a dark canvas it additionally paints the palette's yellow: canvas
	// text on Warn, "dark ink on a highlighter pen" (LOGS-SEL-04). That pair
	// clears D251 pt 1's 4.5:1 body floor on every dark built-in (4.68–12.91:1
	// measured) and stays 4.05–9.89:1 away from the Selection bar a match can
	// sit inside (D252 pt 1). On a light canvas no palette-native shade on Warn
	// clears the floor (D252 pt 3), so weight alone carries the match there —
	// the gate is the palette's own measured polarity (IsDark), one rule rather
	// than a per-palette exemption.
	Match lipgloss.Style

	// Pane frames a component; PaneFocus is the same frame when the pane holds
	// focus (accented border). Both use a rounded border.
	Pane      lipgloss.Style
	PaneFocus lipgloss.Style

	// StatusBar is the bottom bar (context · namespace · spinner · help hint).
	StatusBar lipgloss.Style

	// Semantic status styles.
	Error   lipgloss.Style
	Warn    lipgloss.Style
	Success lipgloss.Style

	// Spinner styles the discovery spinner (accent foreground).
	Spinner lipgloss.Style
}

// New derives a Styles from a Theme. It is the only place a color becomes a
// style, so the mapping from semantic role to concrete styling lives in exactly
// one spot. Pure: same Theme in, equal Styles out; no globals touched.
//
// Theme.Background deliberately becomes no Style here. It is not a role a
// component paints — it is the screen's default, applied once by the root View
// (D249). A Styles field for it would invite exactly the per-component painting
// that cannot work: lipgloss does not re-open an outer background after a nested
// style's reset, so a frame wrapped in a background style comes back painted at
// its margins and bare wherever a colored span already ran.
func New(t Theme) Styles {
	base := lipgloss.NewStyle().Foreground(t.Foreground)
	pane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border)

	return Styles{
		Theme:  t,
		App:    base,
		Subtle: lipgloss.NewStyle().Foreground(t.Subtle),
		Selection: lipgloss.NewStyle().
			Foreground(t.SelectionFg).
			Background(t.Selection),
		Accent: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		Header: lipgloss.NewStyle().
			Foreground(t.Header).
			Bold(true),
		Match: matchStyle(t),
		Pane:      pane,
		PaneFocus: pane.BorderForeground(t.BorderFocus),
		StatusBar: lipgloss.NewStyle().
			Foreground(t.StatusBarFg).
			Background(t.StatusBarBg),
		Error:   lipgloss.NewStyle().Foreground(t.Error).Bold(true),
		Warn:    lipgloss.NewStyle().Foreground(t.Warn),
		Success: lipgloss.NewStyle().Foreground(t.Success),
		Spinner: lipgloss.NewStyle().Foreground(t.Primary),
	}
}

// matchStyle builds the Match style for a Theme (see the Match field's doc for
// the rule): weight everywhere, paint only where the paint can carry the floor.
// A dark canvas is itself the darkest rung of a coherent dark palette (D251
// pt 1 admits it on the dark side), so canvas-on-Warn is the palette's own
// "ink on highlighter" pair; on a light canvas the canvas is light, the
// palette has nothing dark enough above its yellow (D252 pt 3's measurement),
// and the weight THEME-05 added is what distinguishes the match there.
func matchStyle(t Theme) lipgloss.Style {
	s := lipgloss.NewStyle().Bold(true).Underline(true)
	if IsDark(t.Background) {
		s = s.Foreground(t.Background).Background(t.Warn)
	}
	return s
}

// Default is the Styles derived from DefaultTheme — the set the app uses until a
// theme is chosen. A convenience so callers don't repeat New(DefaultTheme()).
func Default() Styles {
	return New(DefaultTheme())
}
