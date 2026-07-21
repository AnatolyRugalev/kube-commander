// Package hintbar is kubecom's dedicated key-hint line: a single row of its own,
// pinned below the status bar, that always shows the persistent focus-aware keymap
// hint. Splitting it off the status bar (FB-hintbar-dedicated) means the hint and
// the live state (context · namespace · spinner · error toast) no longer compete
// for one line — so the hint is never dropped under width pressure and never
// hidden behind a transient error toast the way the old status-bar right-aligned
// hint was (D85's deferred remainder).
//
// The hint string is generated from the effective keymap upstream (via
// help.Model.ShortHelpContextView, already elided to the screen width, D11) and
// handed in with SetHint, so this component never matches a raw key or knows what
// any binding does — it only lays out the string it is given, clamped to width,
// through the shared styles (D54). It owns no shared mutable state (principle 1):
// the root model sets its props and reads its View.
package hintbar

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// Model is the hint line. The root model owns one, refreshes its hint whenever
// focus switches (via the app's syncHints), sizes it on a window-size message, and
// lays View out below the status bar. Every field is owned by the embedding model.
type Model struct {
	styles styles.Styles
	hint   string
	width  int
}

// New builds a hint line rendering through the given styles.
func New(s styles.Styles) Model { return Model{styles: s} }

// SetHint sets the displayed hint. The caller passes
// help.Model.ShortHelpContextView(ctx) so the hint stays registry-generated and
// focus-aware (D11/D85).
func (m *Model) SetHint(h string) { m.hint = h }

// SetWidth informs the line of the available terminal width so it can clamp any
// overflow; wire it from the root model's WindowSizeMsg.
func (m *Model) SetWidth(w int) { m.width = w }

// Hint returns the current hint string (for tests / introspection).
func (m Model) Hint() string { return m.hint }

// View renders the hint as a single line, clamped to the known width so it can
// never wrap onto a second line and grow the layout. The upstream hint is already
// elided to width by the help renderer, so this only guards the edge; with an
// unknown width the string is rendered as-is.
func (m Model) View() string {
	if m.hint == "" {
		return "" // inert until the root model feeds it a hint (no stray styled blank).
	}
	s := m.styles.App
	if m.width > 0 {
		s = s.MaxWidth(m.width)
	}
	return s.Render(m.hint)
}
