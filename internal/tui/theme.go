package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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
	m.ctrPicker.SetStyles(s)
	// Including the command palette, which is the surface a theme is picked from since
	// PAL-05a: it hides before the restyle lands, but a cursor or a half-typed line
	// there must not be the one overlay left on the departed palette next time it opens.
	m.cmdPicker.SetStyles(s)
	m.portPicker.SetStyles(s)

	m.viewer.SetStyles(s)
	m.modal.SetStyles(s)
	m.welcome.SetStyles(s)
	m.searchView.SetStyles(s)
	m.logsView.SetStyles(s)
	m.pfPanel.SetStyles(s)
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

// themeItems renders one picker row per built-in theme and the map resolving a
// row back to its theme name (the picker's SelectedMsg carries only the label, D65 —
// the resByLabel/ctxByLabel pattern). Rows are `* name`, the marker on the theme the
// shell is currently rendering in, exactly as the context picker marks the context it
// is on (D158): the active entry is choosable rather than hidden, and choosing it
// costs nothing.
//
// Order comes from the registry (default first, then sorted — D169 pt 4), which is
// the one order every theme surface agrees on.
//
// The rows are rebuilt every time the stage opens so the marker follows the theme the
// shell is rendering in *now* rather than the one it launched with — the same reason
// the context picker re-seeds (D158).
func themeItems(themes []styles.Theme, current string) ([]string, map[string]string) {
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

// applyThemeNamed repaints and persists the named theme: it is where the palette's
// `:theme ` stage lands, whether that stage was reached by typing the line or by
// pressing `T` (PAL-05a/D207 — one surface, so one apply). It is the **commit** half
// of the live preview: the palette's previewTheme has already repainted the shell as
// the cursor moved, and this adds what a preview must not — the notice, the canvas
// probe and the config write-back.
//
// Picking the theme already rendering is a no-op — the marked row is choosable, so it
// must cost neither a repaint nor a file write. Under the preview that check reads
// correctly because applyPaletteArg's closePalette has just restored themeAnchor, so
// "already rendering" is the theme the stage opened on, and committing it without
// having previewed anything still costs nothing. A name the registry no longer knows
// changes nothing (the stage only offers names it produced, so it is defensive;
// principle 3).
func (m Model) applyThemeNamed(name string) (tea.Model, tea.Cmd) {
	theme, found := styles.ByName(name)
	if !found || theme.Name == m.styles.Theme.Name {
		return m, nil
	}
	m.applyStyles(styles.New(theme))
	notice := m.surfaceNotice("theme " + theme.Name)
	// The new palette carries a new canvas, so the question canvas.go asks at launch
	// has to be asked again: a switch between two palettes of opposite polarity is
	// exactly when a terminal that ignores the request starts mattering. The bump
	// retires the previous palette's tick and its in-flight answer (D250 pt 2).
	m.canvasGen++
	m.awaitingCanvas = false
	return m, tea.Batch(notice, m.persistTheme(theme.Name), m.scheduleCanvasProbe())
}

// previewTheme repaints the shell to the theme under the palette's cursor — the live
// preview the theme picker feedback asked for ("theme switching should happen as I
// change selection in the palette"). It runs after every key that can move the
// palette's selection while the `:theme ` stage is open (routePickerKey), and is a
// no-op unless the cursor actually names a different theme: navigation, and a query
// that re-narrows the list and drops the cursor on a new row, both re-theme the whole
// UI immediately, painted background included (View reads m.styles.Theme.Background
// each frame).
//
// Repaint only, on purpose: no notice, no canvas probe and no config write. Those
// belong to the commit (applyThemeNamed), so a reader browsing the fourteen palettes
// pays them once, on enter, rather than once per row. The marker on the anchor row
// is deliberately not moved as the preview sweeps — rebuilding the stage's rows
// (themeItems) would reset the picker's cursor and fight the very navigation the
// preview rides on — so during a preview it names where esc will return.
func (m Model) previewTheme() Model {
	if m.palArg != keymap.ActionTheme || !m.cmdPicker.Active() {
		return m
	}
	label, ok := m.cmdPicker.Selected()
	if !ok {
		return m
	}
	name, ok := m.themeByLabel[label]
	if !ok || name == m.styles.Theme.Name {
		return m
	}
	theme, found := styles.ByName(name)
	if !found {
		return m
	}
	m.applyStyles(styles.New(theme))
	return m
}

// restoreThemeAnchor undoes an uncommitted preview when the reader leaves the theme
// stage without committing (esc, or backspace back to the verbs): the shell returns
// to the theme that was rendering when the stage opened, silently — a cancel is not
// a switch and gets no notice, no config write and no canvas probe. It clears the
// anchor either way, so a later palette close on any other stage never restores.
// It is a no-op when no theme stage is open, or when nothing was previewed.
func (m *Model) restoreThemeAnchor() {
	if m.themeAnchor != "" && m.themeAnchor != m.styles.Theme.Name {
		if theme, found := styles.ByName(m.themeAnchor); found {
			m.applyStyles(styles.New(theme))
		}
	}
	m.themeAnchor = ""
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
