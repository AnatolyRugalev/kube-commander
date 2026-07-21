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

	// Base text colors.
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
// accent (a nod to the original kube-commander). Truecolor hex values; terminals
// without truecolor downsample at write time via the Bubble Tea renderer, so no
// per-terminal branching is needed here.
func DefaultTheme() Theme {
	return Theme{
		Name:        "default",
		Foreground:  lipgloss.Color("#c6d0f5"),
		Subtle:      lipgloss.Color("#838ba7"),
		Primary:     lipgloss.Color("#8caaee"),
		Selection:   lipgloss.Color("#414559"),
		SelectionFg: lipgloss.Color("#c6d0f5"),
		Border:      lipgloss.Color("#51576d"),
		BorderFocus: lipgloss.Color("#8caaee"),
		Header:      lipgloss.Color("#f2d5cf"),
		StatusBarFg: lipgloss.Color("#c6d0f5"),
		StatusBarBg: lipgloss.Color("#292c3c"),
		Error:       lipgloss.Color("#e78284"),
		Warn:        lipgloss.Color("#e5c890"),
		Success:     lipgloss.Color("#a6d189"),
	}
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

// Default is the Styles derived from DefaultTheme — the set the app uses until a
// theme is chosen. A convenience so callers don't repeat New(DefaultTheme()).
func Default() Styles {
	return New(DefaultTheme())
}
