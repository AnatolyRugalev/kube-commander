package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
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
	// Including the theme picker itself: the pick that triggers this restyle happens
	// with it open (it hides first, but a filter or a cursor there must not be the one
	// surface left on the departed palette next time it opens).
	m.themePicker.SetStyles(s)

	m.viewer.SetStyles(s)
	m.modal.SetStyles(s)
	m.welcome.SetStyles(s)
	m.searchView.SetStyles(s)
	m.logsView.SetStyles(s)
}

// ThemePersister records the chosen theme name in the user's config so the next
// launch opens on it (M4-12b-2). It is the write side of the `theme:` field M4-12a
// reads: the launcher, which alone knows the resolved config path, wires a persister
// bound to it, so the tui package stays storage-agnostic exactly as it does for the
// namespace state file (NamespacePersister/D91).
//
// It takes a *name*, not a Theme: the config carries the name and the launcher
// resolves it (D170 pt 1), so a palette never crosses this boundary. Implementations
// are called off the update loop (they read and rewrite a file) and must preserve the
// rest of the config — the whole struct is marshalled on save, so a write that does
// not load first deletes the user's `keys:` section.
//
// A model built without one (the default, and every hermetic test) is
// persistence-inert: a picked theme applies for the session but is not remembered.
// That is deliberately not the same as making the *gesture* inert — repainting works
// with no config file at all.
type ThemePersister interface {
	PersistTheme(name string) error
}

// WithThemePersister wires the config writer the shell calls when the user picks a
// theme, so the choice survives a restart (M4-12b-2). Without it (or with a nil
// persister) theme switches apply for the session only.
func WithThemePersister(p ThemePersister) Option {
	return func(m *Model) { m.themePersister = p }
}

// themePickerKind is the Kind stamped on the theme picker (picker.New(s, "theme")).
// Every picker emits the same SelectedMsg/CancelledMsg types (D65), so the root
// branches on this Kind to route a picked theme rather than a namespace/context/…
const themePickerKind = "theme"

// openThemePicker shows the theme switcher, seeded from the built-in registry
// (M4-12b-2). Unlike every other picker it needs no seam and no async load: the
// palettes are compiled in, so styles.Themes() is the whole list and the rows are
// built here rather than arriving in a later message. It is therefore never inert.
//
// The rows are rebuilt on every open so the marker follows the theme the shell is
// rendering in *now* rather than the one it launched with — the same reason the
// context picker re-seeds (D158).
func (m Model) openThemePicker() (tea.Model, tea.Cmd) {
	labels, byLabel := themePickerItems(styles.Themes(), m.styles.Theme.Name)
	m.themeByLabel = byLabel
	m.themePicker.SetItems(labels)
	return m, m.themePicker.Show()
}

// themePickerItems renders one picker row per built-in theme and the map resolving a
// row back to its theme name (the picker's SelectedMsg carries only the label, D65 —
// the resByLabel/ctxByLabel pattern). Rows are `* name`, the marker on the theme the
// shell is currently rendering in, exactly as the context picker marks the context it
// is on (D158): the active entry is choosable rather than hidden, and choosing it
// costs nothing.
//
// Order comes from the registry (default first, then sorted — D169 pt 4), which is
// the one order every theme surface agrees on.
func themePickerItems(themes []styles.Theme, current string) ([]string, map[string]string) {
	labels := make([]string, 0, len(themes))
	byLabel := make(map[string]string, len(themes))
	for _, t := range themes {
		label := "  " + t.Name
		if t.Name == current {
			label = "* " + t.Name
		}
		if _, dup := byLabel[label]; dup {
			continue // registry names are unique, so this is defensive.
		}
		byLabel[label] = t.Name
		labels = append(labels, label)
	}
	return labels, byLabel
}

// handleThemeSelected applies a theme picked from the switcher: it closes the picker,
// repaints the running shell through the M4-12b-1 fan-out (applyStyles — every
// component, colors only, no state reset, D171) and writes the name back to the
// config off the update loop.
//
// Picking the theme already rendering is a no-op — the marked row is choosable, so it
// must cost neither a repaint nor a file write. A label with no mapping, or a name the
// registry no longer knows, closes the picker and changes nothing (the picker only
// offers names it produced, so both are defensive; principle 3).
func (m Model) handleThemeSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.themePicker.Hide()
	name, ok := m.themeByLabel[msg.Value]
	m.themeByLabel = nil
	if !ok {
		return m, nil
	}
	theme, found := styles.ByName(name)
	if !found || theme.Name == m.styles.Theme.Name {
		return m, nil
	}
	m.applyStyles(styles.New(theme))
	notice := m.surfaceNotice("theme " + theme.Name)
	return m, tea.Batch(notice, m.persistTheme(theme.Name))
}

// persistTheme records the picked theme in the user's config so the next launch opens
// on it (M4-12b-2). The read-modify-write runs off the update loop (a tea.Cmd) so it
// never blocks input, and a failure surfaces as a transient toast but is otherwise
// non-fatal: the theme still applies for this session (principle 3). With no persister
// wired it is a no-op.
func (m Model) persistTheme(name string) tea.Cmd {
	if m.themePersister == nil {
		return nil
	}
	p := m.themePersister
	return func() tea.Msg {
		if err := p.PersistTheme(name); err != nil {
			return NewErrorMsg("persist theme", err)
		}
		return nil
	}
}
