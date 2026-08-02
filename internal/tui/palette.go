package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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
// PAL-03a folds a verb's *argument* into this same surface: a verb that takes one does
// not dispatch and open a modal of its own, it **commits in place**, and the same
// picker re-prompts itself `:resource ` and shows that verb's values. Two rules keep
// that from becoming a second implementation of each verb (D198): the argument stage
// ends in the same function the verb's standalone picker ends in, and a verb whose
// values cannot be produced does not enter the stage at all — it stays as inert as its
// key is.
//
// PAL-03b completes the set with the two verbs whose values are **not in hand** when
// the stage opens — `namespace` (a cluster call) and `context` (a kubeconfig read).
// They enter the stage immediately, on an empty list titled as loading, and their
// values arrive in a message addressed to the surface that asked for them (D199), so
// all four argument verbs now behave identically from the reader's side: type the
// verb, space, type the value.
//
// PAL-05 turns the shortcut keys into pre-typed palette lines. Until then the shortcuts
// keep working unchanged (D194 pt 4) — these slices add a way in, they do not take one
// away.

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

// The palette's own chrome. The title names the stage the line is in, and the prompt
// is the line's left-hand side: `:` while a verb is being chosen, `:resource ` once one
// is committed, so the modal reads as the `<verb> <argument>` line the feedback asked
// for while staying a single field (PAL-03a). The prompt is also what tells a reader
// which stage they are in mid-type, before they look at the list.
const (
	paletteTitle  = "Command"
	palettePrompt = ":"
	// paletteLoading is appended to an argument stage's title while its values are
	// still in flight. The list is empty for that moment and an untitled empty modal
	// reads as a broken one, so the title is what distinguishes "nothing here" from
	// "not here yet" (PAL-03b).
	paletteLoading = " — loading…"
)

// paletteArgVerbs are the verbs whose **argument** the palette completes in place:
// committing one keeps the same modal open and swaps the item list for that verb's
// values, instead of dispatching the verb so it opens a modal of its own. The value is
// the word the prompt shows (`:resource `), which is the verb's name in the line —
// deliberately short, not its sentence-long Describe() text.
//
// Two of the four have their values in hand when the stage opens (the resource kinds
// are the menu's current item snapshot; the themes are the compiled-in registry) and
// two do not (namespace lists against the cluster, context reads the kubeconfig) —
// the difference is invisible in the line and is confined to enterPaletteArg.
var paletteArgVerbs = map[keymap.Action]string{
	keymap.ActionResources: "resource",
	keymap.ActionTheme:     "theme",
	keymap.ActionNamespace: "namespace",
	keymap.ActionContext:   "context",
}

// paletteVerbItems renders the verb list and the label→action map that resolves a pick
// (the resByLabel/ctxByLabel pattern — a SelectedMsg carries only the label, D65).
// Labels are the registry's own descriptions, so an entry cannot describe itself
// differently from its key, and app.palette itself is skipped so the palette can never
// list a way to reopen the surface you are already in.
func paletteVerbItems() ([]string, map[string]keymap.Action) {
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
	return labels, byLabel
}

// showPaletteVerbs puts the palette into its verb stage: the curated verb list, the
// `: ` prompt, an empty query. It seeds a fresh palette and is also how the argument
// stage is walked back out of, so "the palette showing verbs" has exactly one
// definition and a returned-to palette is indistinguishable from a just-opened one.
func (m Model) showPaletteVerbs() Model {
	labels, byLabel := paletteVerbItems()
	m.cmdByLabel = byLabel
	m.palArg = ""
	m.cmdPicker.SetTitle(paletteTitle)
	m.cmdPicker.SetPrompt(palettePrompt)
	m.cmdPicker.ClearQuery()
	m.cmdPicker.SetItems(labels)
	return m
}

// openPalette opens the command palette on its verb stage, labelled by each action's
// registry description and ranked by the shared matcher as you type (D194). It is
// never inert — the verbs are compiled in, like the theme picker's palettes — so `:`
// opens something even with no cluster.
func (m Model) openPalette() (tea.Model, tea.Cmd) {
	m = m.showPaletteVerbs()
	return m, m.cmdPicker.Show()
}

// closePalette dismisses the palette and returns it to the verb stage, so the next `:`
// opens on verbs with a clean prompt rather than on wherever the last line ended.
func (m *Model) closePalette() {
	m.cmdPicker.Hide()
	m.cmdPicker.SetTitle(paletteTitle)
	m.cmdPicker.SetPrompt(palettePrompt)
	m.cmdByLabel = nil
	m.palArg = ""
}

// enterPaletteArg commits a verb into the palette's argument stage: the same modal
// stays up, re-prompted `:<verb> `, with the verb's values as its items. It reports
// false when the verb takes no argument here (dispatch it as before) and also when the
// verb's values cannot be produced — an inert verb stays inert, exactly as its key is
// (D197), rather than opening an empty argument list that suggests otherwise.
//
// A verb whose values arrive asynchronously returns a cmd that fetches them, addressed
// to this palette (D199); its stage opens empty, titled as loading, and is filled by
// fillPaletteArg when the message lands. Inertness is still decided *here*, before the
// stage opens — a missing lister is known synchronously — so "the stage opened" never
// means "the value list might turn out to be impossible".
func (m Model) enterPaletteArg(a keymap.Action) (Model, tea.Cmd, bool) {
	word, ok := paletteArgVerbs[a]
	if !ok {
		return m, nil, false
	}
	var (
		items   []picker.Item
		load    tea.Cmd
		pending bool
	)
	switch a {
	case keymap.ActionResources:
		if m.watcher == nil {
			return m, nil, false // watch-inert: there is no table to switch.
		}
		// The one stage whose values carry aliases (CRD-PIN-04/D203): the kinds match
		// their plural, short names and group here exactly as they do on `R`, since
		// both stages are seeded from the one snapshot.
		items, m.resByLabel = m.resourcePickerItems()
	case keymap.ActionTheme:
		var labels []string
		labels, m.themeByLabel = themePickerItems(styles.Themes(), m.styles.Theme.Name)
		items = picker.Labels(labels)
	case keymap.ActionNamespace:
		if m.nsLister == nil {
			return m, nil, false // namespace-switch-inert, exactly as ctrl+n is.
		}
		load, pending = m.loadNamespaces(commandPickerKind), true
	case keymap.ActionContext:
		if m.ctxLister == nil {
			return m, nil, false // context-switch-inert, exactly as `C` is.
		}
		m.ctxByLabel = nil // a stale map would resolve a row this stage never listed.
		load, pending = m.loadContexts(commandPickerKind), true
	default:
		return m, nil, false
	}
	if !pending && len(items) == 0 {
		return m, nil, false
	}
	m.palArg = a
	title := a.Describe()
	if pending {
		title += paletteLoading
	}
	m.cmdPicker.SetTitle(title)
	m.cmdPicker.SetPrompt(palettePrompt + word + " ")
	m.cmdPicker.ClearQuery() // SetItems applies the standing query; the verb's is spent.
	m.cmdPicker.SetItemsWithAliases(items)
	return m, load, true
}

// awaitingPaletteArg reports whether the palette is still the surface waiting for
// verb a's value list — i.e. it is open *and* on that verb's stage. A list that lands
// after the reader closed the palette, rewound the line to the verbs, or committed a
// different verb belongs to a stage that no longer exists and is dropped.
func (m Model) awaitingPaletteArg(a keymap.Action) bool {
	return m.cmdPicker.Active() && m.palArg == a
}

// fillPaletteArg seeds a pending argument stage with the values that have arrived and
// drops the loading marker from its title. The query the reader typed while waiting is
// deliberately kept — SetItems applies it — so typing ahead of a slow list narrows it
// the moment it lands instead of being thrown away.
func (m Model) fillPaletteArg(a keymap.Action, labels []string) Model {
	m.cmdPicker.SetTitle(a.Describe())
	m.cmdPicker.SetItems(labels)
	return m
}

// applyPaletteArg runs a verb with the argument picked in the palette. Each arm ends in
// the *same* function the verb's standalone picker ends in — selectResource,
// applyThemeNamed, applyNamespaceValue, applyContextLabel — so an argument reached
// through the palette and one reached through the picker are one code path, in the
// spirit of D197's "no second implementation": the palette resolves and applies, it
// never re-implements what the verb does.
func (m Model) applyPaletteArg(a keymap.Action, value string) (tea.Model, tea.Cmd) {
	m.closePalette()
	switch a {
	case keymap.ActionResources:
		r, ok := m.resByLabel[value]
		if !ok {
			return m, nil // the stage only lists labels it mapped — defensive.
		}
		return m.selectResource(r)
	case keymap.ActionTheme:
		name, ok := m.themeByLabel[value]
		m.themeByLabel = nil
		if !ok {
			return m, nil
		}
		return m.applyThemeNamed(name)
	case keymap.ActionNamespace:
		return m.applyNamespaceValue(value)
	case keymap.ActionContext:
		return m.applyContextLabel(value)
	}
	return m, nil
}

// handleCommandSelected resolves a pick in the palette. In the argument stage the pick
// is the argument, so the verb runs with it; in the verb stage a verb that takes an
// argument commits into that stage (staying in this one modal — the point of the PAL
// line) and every other verb is handed to handleAction, the same dispatch a key press
// reaches (D11). A label with no mapping — the palette can only list labels it mapped,
// so this is defensive — closes it without running anything.
func (m Model) handleCommandSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	if m.palArg != "" {
		return m.applyPaletteArg(m.palArg, msg.Value)
	}
	a, ok := m.cmdByLabel[msg.Value]
	if !ok {
		m.closePalette()
		return m, nil
	}
	if next, load, entered := m.enterPaletteArg(a); entered {
		return next, load
	}
	m.closePalette()
	return m.handleAction(a)
}

// handlePaletteFilterKey gives the palette's line the two editing gestures that make
// it a line rather than a list with a query box (PAL-03a). It runs before the key
// reaches the filter field and reports whether it consumed the key:
//
//   - **space commits a verb.** In the verb stage, with something typed, space commits
//     the highlighted verb if that verb takes an argument — so `:res` + space lands on
//     the resource values without a second gesture. Space is the line's separator and
//     is never query text: on a verb that takes no argument it is simply swallowed
//     (a verb label's own spaces are skippable by the subsequence matcher, so nothing
//     becomes unreachable).
//   - **backspace at the start of the argument leaves it.** With the argument query
//     empty, backspace erases the committed verb itself and returns to the verb list —
//     the line unwinds the way it was typed instead of dead-ending.
// The returned cmd is the value load of a verb whose list is fetched (PAL-03b); it is
// nil for every other outcome.
func (m Model) handlePaletteFilterKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.Key()
	switch {
	case key.Text == " ":
		if m.palArg != "" {
			return m, nil, false // inside an argument, a space is ordinary query text.
		}
		if m.cmdPicker.Query() == "" {
			return m, nil, true // no verb is being narrowed yet; swallow the leading space.
		}
		label, ok := m.cmdPicker.Selected()
		if !ok {
			return m, nil, true
		}
		if next, load, entered := m.enterPaletteArg(m.cmdByLabel[label]); entered {
			return next, load, true
		}
		return m, nil, true
	case key.Code == tea.KeyBackspace && key.Text == "":
		if m.palArg == "" || m.cmdPicker.Query() != "" {
			return m, nil, false
		}
		return m.showPaletteVerbs(), nil, true
	}
	return m, nil, false
}
