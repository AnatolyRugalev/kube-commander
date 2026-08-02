package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// The command palette (PAL-02, the second slice of the PAL line): `:` opens one
// modal list of the app's *verbs*, you narrow it by typing what you want to do, and
// the pick runs that verb. It is the surface the palette feedback asked for — "one
// place you type to make anything happen" — and the reason it can exist at all is
// D11: every user-triggerable behaviour is already a named, described Action, so the
// palette is a picker over the registry rather than a second, hand-maintained list of
// what kubecom can do.
//
// Two properties are load-bearing and are the ones a later slice must not quietly
// drop (D197):
//
//   - **It resolves to an Action and dispatches it.** handleCommandSelected ends in
//     handleAction — the same entry point a key press reaches — so a verb picked here
//     and its key are the same code path by construction. A palette that called
//     openNamespacePicker directly would work today and drift the first time a key's
//     handler grows a precondition.
//   - **A verb is inert here exactly as its key is.** The palette does not filter its
//     list by what is currently possible: `:` with no cluster still lists "Switch
//     namespace", and picking it does what ctrl+n does — nothing. Availability
//     filtering is PAL-04's job (row verbs, where "unavailable" is a property of the
//     selected object rather than of the whole app).
//
// PAL-03 folds a verb's *argument* into this same surface (`:namespace `), and PAL-05
// turns the shortcut keys into pre-typed palette lines. Until then the shortcuts keep
// working unchanged (D194 pt 4) — this slice adds a way in, it does not take one away.

// commandPickerKind is the Kind stamped on the command palette's picker. Every
// picker emits the same SelectedMsg/CancelledMsg types (D65), so the root branches on
// this Kind to route a picked verb to handleCommandSelected.
const commandPickerKind = "command"

// paletteVerbs is the curated set of verbs the palette offers, in the order an empty
// query shows them (the switchers first, then the toggles, then help/quit). It is
// **app-global actions only**: nothing here is scoped to the selected table row, the
// open logs view, the port picker or the confirm modal, because those actions are
// meaningful only while a particular surface is up and a palette entry that did
// nothing 95% of the time would be worse than no entry. Row-scoped verbs join the
// palette in PAL-04, from the actions menu's own per-row source (never a second list).
//
// Adding a verb is adding a line here; the label and the dispatch both come from the
// registry, so an entry cannot describe itself differently from its key or run
// something else.
var paletteVerbs = []keymap.Action{
	keymap.ActionResources,
	keymap.ActionNamespace,
	keymap.ActionContext,
	keymap.ActionSearch,
	keymap.ActionForwards,
	keymap.ActionActions,
	keymap.ActionToggleMenu,
	keymap.ActionTheme,
	keymap.ActionToggleMouse,
	keymap.ActionSort,
	keymap.ActionClearSort,
	keymap.ActionHelp,
	keymap.ActionQuit,
}

// openPalette opens the command palette: the curated verb list, labelled by each
// action's registry description and ranked by the shared matcher as you type (D194).
// It is never inert — the verbs are compiled in, like the theme picker's palettes —
// so `:` opens something even with no cluster, and app.palette itself is skipped so
// the palette can never list a way to reopen itself.
func (m Model) openPalette() (tea.Model, tea.Cmd) {
	labels := make([]string, 0, len(paletteVerbs))
	byLabel := make(map[string]keymap.Action, len(paletteVerbs))
	for _, a := range paletteVerbs {
		label := a.Describe()
		if label == "" || a == keymap.ActionPalette {
			continue // unregistered (a programming error, pinned by a test) or self.
		}
		if _, dup := byLabel[label]; dup {
			continue // a description collision would make the pick ambiguous — keep the first.
		}
		byLabel[label] = a
		labels = append(labels, label)
	}
	m.cmdByLabel = byLabel
	m.cmdPicker.SetItems(labels)
	return m, m.cmdPicker.Show()
}

// handleCommandSelected runs a verb picked from the palette: it closes the palette and
// hands the resolved Action to handleAction — the same dispatch a key press reaches
// (D11), so the palette adds an entry point and no behaviour of its own. A label with
// no mapping — the palette can only list labels it mapped, so this is defensive —
// closes it without running anything.
func (m Model) handleCommandSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.cmdPicker.Hide()
	a, ok := m.cmdByLabel[msg.Value]
	if !ok {
		return m, nil
	}
	return m.handleAction(a)
}
