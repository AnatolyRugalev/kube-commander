package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// applyStyles repoints the shell and every component it owns at s — the live
// restyle a theme chosen from inside kubecom needs (M4-12b-1).
//
// It exists because a launch-time theme and a runtime one are different problems.
// WithTheme works by running before the components are constructed (D170 pt 2), so
// each of them is *built* from the resolved palette; nothing has to be repainted
// because nothing has been drawn yet. Once the shell is running that door is shut:
// every component below has cached the Styles it was handed, and assigning m.styles
// alone would change only app.go's own handful of direct renders — the menu, the
// table, the bar and the overlays would keep drawing in the old theme while
// m.styles.Theme.Name reported the new one. That is a silent half-restyle, and the
// reason this fan-out is a named method with a test rather than a line inside the
// action that will call it (M4-12b-2).
//
// Two properties it must keep, since a restyle can land at any moment:
//
//   - **Every component, every time.** A new component field added to Model must be
//     added here too, or picking a theme leaves it behind — the failure is invisible
//     until someone happens to have that overlay open. TestApplyStylesRestylesEvery
//     Component walks the rendered shell to catch the common cases.
//   - **Colors only.** No component's state is reset: a filter stays typed, a table
//     keeps its rows/sort/selection, an open picker keeps its cursor, the logs view
//     keeps its buffer and scroll. Restyling is not a reset, so it is safe to call
//     while any overlay is up (each component's SetStyles documents what it
//     preserves; three of them re-derive rather than assign).
//
// It is deliberately not an Option: options run pre-construction, and this runs
// post. The two paths are separate on purpose and must not be collapsed.
func (m *Model) applyStyles(s styles.Styles) {
	m.styles = s

	m.help.SetStyles(s)
	m.menu.SetStyles(s)
	m.table.SetStyles(s)
	m.status.SetStyles(s)
	m.hintbar.SetStyles(s)

	// Every picker is the same component in a different role, so they restyle
	// together; missing one would leave a single overlay off-theme.
	m.nsPicker.SetStyles(s)
	m.resPicker.SetStyles(s)
	m.actPicker.SetStyles(s)
	m.ctrPicker.SetStyles(s)
	m.portPicker.SetStyles(s)
	m.ctxPicker.SetStyles(s)

	m.viewer.SetStyles(s)
	m.modal.SetStyles(s)
	m.welcome.SetStyles(s)
	m.searchView.SetStyles(s)
	m.logsView.SetStyles(s)
}
