package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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
//     a verb's opener directly would work today and drift the first time a key's
//     handler grows a precondition.
//   - **A verb is inert here exactly as its key is.** The palette does not filter its
//     list by what is currently possible: `:` with no cluster still lists "Switch
//     namespace", and picking it does what N does — nothing. Availability
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
// PAL-04 answers the other half of the feedback's "what can I do right now?": with a
// table row selected the verb stage also lists the **row-scoped** actions — the exact
// set the actions menu computes for that row, never a second list — and the
// palette's title names the object they would act on (D205). That title is the whole
// answer to "which object": a palette entry that can delete something must say what,
// and saying it once above the list beats repeating it on every row of a 60-column
// modal.
//
// PAL-05 turns the shortcut keys into pre-typed palette lines: a key whose verb takes
// an argument stops opening a modal of its own and opens *this* one with its verb
// already committed (openPaletteArg). The key keeps working and keeps its meaning —
// what changes is that there is one surface behind it instead of two (D207). It lands
// one key per slice: `T` → `:theme ` (PAL-05a), `R` → `:resource ` (PAL-05b),
// `N` → `:namespace ` (PAL-05c-1), `C` → `:context ` (PAL-05c-2), `a` →
// `:action ` (PAL-05d). With PAL-05d every key whose verb takes a value opens this
// surface and no menu of its own is left: ctrPicker/portPicker pick from a *row's* own
// data (its containers, its declared ports), not from a verb's argument list, so they
// are the two modals the rule does not owe (D210 pt 3).
//
// PAL-05b is the slice that shows what the conversion is worth rather than merely what
// it costs. `R`'s picker was the surface CRD-PIN-04 hardened (aliases, group
// qualification — D203), and the stage inherited every bit of that for free, because
// both were already seeded from the one resourcePickerItems snapshot; retiring the
// picker deletes the *second* place those guarantees had to hold, not the guarantees.
// What `R` gains in exchange is the rest of the line: backspace rewinds to the verb
// list, so a kind you cannot find is one keystroke from every other verb.
//
// PAL-05c-1 is the first conversion of a verb whose values are **fetched**, and the
// first with a second door: besides `N`, the menu's namespace-seam row opens the
// same surface, so it routes through openPaletteArg too — a converted key with an
// unconverted second entry point would keep the retired picker alive behind a menu
// row. Retiring nsPicker also collapses namespacesLoadedMsg's `dest` (D199): with one
// destination left, *which* surface asked stopped being a question, and only "is that
// stage still up?" (awaitingPaletteArg) remains.
//
// PAL-05c-2 makes the identical collapse to contextsLoadedMsg and carries the one thing
// no other converted key had: D158's marker rule. The `*` on a context row follows the
// **shell's** live context rather than the kubeconfig's `current-context`, and that
// lives in contextPickerItems, which the stage was already seeded from — so, like D203
// on `R`, it moved nowhere.
//
// PAL-05d finishes the line with the key that looked like the exception: `a` had no
// argument word to pre-type, so D209 pt 3 filed its picker with the two row-data ones.
// It is not one of those — its values are the compiled-in row-action registry, nameable
// before the list is seen (`:action delete`), and PAL-04 already listed them here — so
// the key converts like the other four: it opens `:action `, the palette narrowed to
// the verbs that act on the selected row, one backspace from every other verb (D210).
// Since STORY-06c the key that does this is **`enter`**, resolved in the resource-table
// context (TableAction) so the keymap can keep `enter` = nav.drillIn everywhere else
// (D132's context split); the `:action ` stage itself is unchanged, which is why `enter`
// and the typed line render the same frame (TestActionKeyOpensTheSamePaletteStageAsTheLine).

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
// palette in PAL-04, from the row-action registry's own per-kind source (never a second
// list), and `actions.menu` is the verb that narrows the palette to just those
// (PAL-05d).
//
// menu.pin is here despite its *key* reading the cursor, and that is not an exception
// to the rule above: as a palette verb it takes the kind as its argument, so it names
// what it acts on instead of depending on which pane has focus (CRD-PIN-05/D204). The
// rule is about what the entry needs in order to mean something, not about which
// surface the key happens to read.
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
	keymap.ActionPin,
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
	// paletteTargetSep joins the palette's title to the object its row verbs would
	// act on: `Command — Pod default/web-1` (PAL-04). The target is named once, above
	// the list, rather than on each row — a 60-column modal cannot spare the width,
	// and the row titles are also the labels a pick is resolved by (D203 pt 3), so
	// widening them would widen that key too.
	paletteTargetSep = " — "
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
// CRD-PIN-05 adds a fifth, `pin`, whose values are the same kinds `:resource ` offers
// — the one place a kind can be named by typing it rather than by pointing at it. Its
// word stays `pin` though the verb toggles, because that is the name of the gesture
// and of the thing it manages; a line reading `:unpin ` would have to be a second verb
// listing a different set, and D202 pt 3's "both directions or neither" is satisfied by
// one verb that does both, exactly as the key does.
//
// PAL-05d adds the sixth and last, `actions.menu`, whose argument is the row action to
// run: its word is `action` because that is what the line names, though the verb's id
// still reads `actions.menu` from when it opened one (D210).
var paletteArgVerbs = map[keymap.Action]string{
	keymap.ActionResources: "resource",
	keymap.ActionTheme:     "theme",
	keymap.ActionNamespace: "namespace",
	keymap.ActionContext:   "context",
	keymap.ActionPin:       "pin",
	keymap.ActionActions:   "action",
}

// paletteVerbItems renders the verb list and the label→action map that resolves a pick
// (the resByLabel/ctxByLabel pattern — a SelectedMsg carries only the label, D65).
// Labels are the registry's own descriptions, so an entry cannot describe itself
// differently from its key, and app.palette itself is skipped so the palette can never
// list a way to reopen the surface you are already in.
//
// Since PAL-08 each item also carries the action's **id** as its Name, drawn in a
// column before the description (D237): "Switch namespace" alone never said what the
// command was called, and the id is the name kubecom already uses for it everywhere
// else — in `config.yaml`'s `keys:` map, in `docs/keybindings.md`, and in the `?`
// overlay's own grouping. It is unique by construction, so it also gives the reader a
// way to type at a verb precisely (`ns` finds ns.switch) that a description cannot.
func paletteVerbItems() ([]picker.Item, map[string]keymap.Action) {
	items := make([]picker.Item, 0, len(paletteVerbs))
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
		items = append(items, picker.Item{Label: label, Name: string(a)})
	}
	return items, byLabel
}

// paletteRowVerbs returns the row-scoped verbs the palette offers right now and the
// title of the object they would act on, or nil/"" when there is no row to act on.
//
// The set is rowActionTitles' — the row-action registry's own per-kind source — so
// PAL-04 adds a way *in* to those actions and not a second list of what they are: an
// action the registry hides for this kind (Cordon on a Pod) is absent here for the same
// reason, in the same code. The preconditions are the ones the actions gesture has had
// since M3-02: a resource table showing and a row under the cursor, with no requirement
// that the table hold focus. Since PAL-05d the `:action ` stage (entered by `enter` on a
// row since STORY-06c) goes through this same function too, so the stage and `:` do not
// merely agree about a kind's actions — they are one
// computation.
//
// The labels are the titles marked by rowActionLabel — an action that will ask before
// it acts says so here (PAL-06/D228) — and the returned map is keyed by that label,
// since the label is what a pick comes back as.
//
// A label already claimed by an app-global verb is dropped rather than shadowing it —
// the label is the identity a SelectedMsg resolves by (D203 pt 3), so it must name one
// thing. Nothing collides today (the globals are sentences, the row titles are
// imperatives) and a test pins that, which is what makes the drop a guard rather than
// silent behaviour. That drop is a property of *co-listing*, not of the set: the
// `:action ` stage (PAL-05d) lists these titles alone, so it passes a nil `taken` and
// keeps every applicable action — a collision with a verb the stage does not show would
// hide an action from `a` for no reader-visible reason (D210 pt 2).
// Each item's Name is the row action's own id (PAL-08/D237) — `delete`, `scale`,
// `rolloutRestart` — which is the name the `:action ` line already names them by
// (D210) and, unlike the global verbs' keymap ids, the only name several of them have
// (a menu-only action has no key to be called after).
func (m Model) paletteRowVerbs(taken map[string]keymap.Action) ([]picker.Item, map[string]rowAction, string) {
	if !m.hasCurrent {
		return nil, nil, ""
	}
	row, ok := m.table.SelectedRow()
	if !ok {
		return nil, nil, ""
	}
	titles, byTitle := rowActionTitles(m.current)
	items := make([]picker.Item, 0, len(titles))
	byLabel := make(map[string]rowAction, len(titles))
	for _, title := range titles {
		// The listed label is the title plus the confirm marker where one applies
		// (PAL-06/D228), and it is what the resolution map is keyed by — a pick comes
		// back as the label the reader saw, not as the registry's bare title.
		label := rowActionLabel(byTitle[title])
		if _, dup := taken[label]; dup {
			continue
		}
		items = append(items, picker.Item{Label: label, Name: string(byTitle[title])})
		byLabel[label] = byTitle[title]
	}
	if len(items) == 0 {
		return nil, nil, ""
	}
	return items, byLabel, viewerTitle(m.current, row.Object)
}

// showPaletteVerbs puts the palette into its verb stage: the curated verb list, the
// `: ` prompt, an empty query. It seeds a fresh palette and is also how the argument
// stage is walked back out of, so "the palette showing verbs" has exactly one
// definition and a returned-to palette is indistinguishable from a just-opened one.
//
// Since PAL-04 the stage is two lists: the app-global verbs, then the ones scoped to
// the selected row. The globals keep the top in the empty-query order they have had
// since PAL-02, so every line a reader has already learned still resolves to the same
// verb whether or not a row happens to be selected — the row verbs are additive, and
// the matcher ranks them the moment anything is typed.
//
// Since LOGS-09 the stage can also open **over the logs view** (D258), and the second
// list is then the logs view's own verbs — follow, grep, wrap, previous, yank… — so
// the palette answers "what can I do here" on that surface too. The row verbs are
// withheld there: they act on the browse table's selection, which the full-screen
// logs view hides, and a pick like Delete would open the confirm modal *invisibly*
// (the View draws one body; the modal would still capture input). That is D197's
// rule, not an exception: over the logs view the row verbs' keys are swallowed, so
// the verbs are inert exactly as their keys are.
// showPaletteVerbs returns the palette to its verb stage: the list the reader lands
// on at `:` and the list backspace returns to. Leaving the theme stage this way is a
// cancel, not a commit, so the live preview's anchor is restored first — a reader who
// browsed palettes and rewound is back on the theme they had, silently (feedback
// 2026-08-09-theme-picker-live-preview).
func (m Model) showPaletteVerbs() Model {
	m.restoreThemeAnchor()
	items, byLabel := paletteVerbItems()
	m.cmdByLabel = byLabel
	m.palArg = ""
	m.palDirect = false // the reader is on the verb list now, however they got here.
	m.palRowByLabel = nil
	title := paletteTitle
	if m.logsView.Active() {
		logsItems, logsByLabel := logsVerbItems(byLabel)
		for label, a := range logsByLabel {
			m.cmdByLabel[label] = a
		}
		items = append(items, logsItems...)
	} else if rowItems, rowByLabel, target := m.paletteRowVerbs(byLabel); target != "" {
		m.palRowByLabel = rowByLabel
		items = append(items, rowItems...)
		title = paletteTitle + paletteTargetSep + target
	}
	m.cmdPicker.SetTitle(title)
	m.cmdPicker.SetPrompt(palettePrompt)
	m.cmdPicker.ClearQuery()
	m.cmdPicker.SetItemsWithAliases(items)
	return m
}

// logsVerbs is the set of logs-view actions the palette lists when it opens over the
// logs view (LOGS-09/D258) — the discoverability the feedback asked for: every
// gesture the view honours, findable by typing. app.filter joins them under its own
// registry label ("Filter / search"): on this surface it is the gesture that opens
// the live grep. Each pick dispatches through handleAction exactly as its key does
// (D197), so the toggle fires on the view the palette was floating over.
var logsVerbs = []keymap.Action{
	keymap.ActionFilter,
	keymap.ActionLogsFollow,
	keymap.ActionLogsRegex,
	keymap.ActionLogsWrap,
	keymap.ActionLogsTimestamps,
	keymap.ActionLogsPrevious,
	keymap.ActionLogsSelect,
	keymap.ActionLogsYank,
}

// logsVerbItems renders logsVerbs exactly as paletteVerbItems renders the globals:
// labels are the registry's own descriptions, keyed back to the action through the
// returned map, and a label already claimed by a global is dropped rather than
// shadowing it (none collides today — the globals are app sentences, the logs verbs
// are view toggles).
func logsVerbItems(taken map[string]keymap.Action) ([]picker.Item, map[string]keymap.Action) {
	items := make([]picker.Item, 0, len(logsVerbs))
	byLabel := make(map[string]keymap.Action, len(logsVerbs))
	for _, a := range logsVerbs {
		label := a.Describe()
		if label == "" {
			continue // unregistered — a programming error, pinned by tests.
		}
		if _, dup := taken[label]; dup {
			continue
		}
		byLabel[label] = a
		items = append(items, picker.Item{Label: label, Name: string(a)})
	}
	return items, byLabel
}

// openPalette opens the command palette on its verb stage, labelled by each action's
// registry description and ranked by the shared matcher as you type (D194). It is
// never inert — the verbs are compiled in, like the theme picker's palettes — so `:`
// opens something even with no cluster.
func (m Model) openPalette() (tea.Model, tea.Cmd) {
	m = m.showPaletteVerbs()
	return m, m.cmdPicker.Show()
}

// openPaletteArg opens the palette *already* in verb a's argument stage — the pre-typed
// line a shortcut key becomes (PAL-05a/D207). `T` does not open a theme modal any more;
// it opens the one palette, re-prompted `:theme `, over the same values `:` `theme` `␣`
// reaches. The key is unchanged as a gesture: same key, same verb, same list.
//
// It ends in enterPaletteArg, which is the whole point — the stage a key opens and the
// stage the line opens are produced by one function, so a shortcut cannot come to offer
// a different set from the palette's own. That is what keeps this sugar rather than a
// re-implementation, and backspace on the empty argument still rewinds to the verb list
// (handlePaletteFilterKey), so a key pressed by mistake is one keystroke from every
// other verb rather than a dead end.
//
// What a key-opened stage does *not* share is esc: it closes the palette outright
// instead of rewinding (palDirect/D233). Rewinding it put the reader on the verb list,
// which for someone who pressed `a` is a surface they had never seen — so esc, the app's
// "back" everywhere else, did not back out of anything.
//
// A verb whose values cannot be produced leaves its key exactly as inert as it is today:
// enterPaletteArg decides that before anything is shown (D197), so a shortcut never
// opens an empty palette that implies the verb was available.
func (m Model) openPaletteArg(a keymap.Action) (tea.Model, tea.Cmd) {
	next, load, entered := m.enterPaletteArg(a)
	if !entered {
		return m, nil
	}
	// Set after enterPaletteArg, which clears it: this is the one caller that did not
	// come off the verb list.
	next.palDirect = true
	show := next.cmdPicker.Show()
	return next, tea.Batch(show, load)
}

// closePalette dismisses the palette and returns it to the verb stage, so the next `:`
// opens on verbs with a clean prompt rather than on wherever the last line ended. It
// also restores the live preview's anchor (restoreThemeAnchor): every way out of the
// theme stage except the commit itself is a cancel, and the shell must not be left
// rendering a palette the reader only looked at. The commit path (applyPaletteArg)
// calls this first and then applyThemeNamed, which re-applies the picked theme after
// the restore — same final state, and the no-op check that follows reads the anchor
// correctly.
func (m *Model) closePalette() {
	m.restoreThemeAnchor()
	m.cmdPicker.Hide()
	m.cmdPicker.SetTitle(paletteTitle)
	m.cmdPicker.SetPrompt(palettePrompt)
	m.cmdByLabel = nil
	m.palRowByLabel = nil
	m.palArg = ""
	m.palDirect = false
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
		// title overrides the stage's default title (the verb's own description). Only
		// the row stage sets it: its chrome has to name the *object* it would act on
		// (D205 pt 2), and at 60 columns that is worth more than repeating the verb.
		title string
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
	case keymap.ActionPin:
		if m.pinner == nil {
			return m, nil, false // pin-inert, exactly as `*` is.
		}
		// The same snapshot `:resource ` lists, aliases and group qualification and
		// all (D203 pt 4): a kind is pinned by the name you find it under, and the two
		// stages cannot come to offer different kinds. Pinning does not need a watcher
		// — you may pin a kind you are not about to browse — so, unlike `:resource `,
		// this stage is live in a watch-inert shell.
		items, m.resByLabel = m.resourcePickerItems()
	case keymap.ActionTheme:
		var labels []string
		labels, m.themeByLabel = themeItems(styles.Themes(), m.styles.Theme.Name)
		items = picker.Labels(labels)
		// The anchor is the theme the live preview returns to: the palette opens on
		// the theme the shell renders now, and a cancel restores it (restoreThemeAnchor).
		m.themeAnchor = m.styles.Theme.Name
	case keymap.ActionNamespace:
		// Namespace-switch-inert. Since PAL-05c-1 this one check is also what makes
		// `N` and the menu's namespace-seam row inert, rather than each of the
		// three entry points deciding for itself.
		if m.nsLister == nil {
			return m, nil, false
		}
		load, pending = m.loadNamespaces(), true
	case keymap.ActionContext:
		// Context-switch-inert. Since PAL-05c-2 this one check is also what makes `C`
		// inert, rather than the key deciding for itself.
		if m.ctxLister == nil {
			return m, nil, false
		}
		m.ctxByLabel = nil // a stale map would resolve a row this stage never listed.
		load, pending = m.loadContexts(), true
	case keymap.ActionActions:
		// The one stage whose values are scoped to the selected *row* rather than to
		// the app: the actions applicable to the browsed kind, from rowActionTitles
		// through paletteRowVerbs — the same source the verb stage appends (D205 pt 1),
		// so `:action ` and `:` can never offer different sets. Inertness is the
		// stage's rule and is decided here, not in the key (D209 pt 2): no resource
		// table, no row under the cursor, or no applicable action and the stage does
		// not open.
		// Over the logs view the stage is inert too (D258): its verbs act on a
		// selection the full-screen view hides, and a pick like Delete would open the
		// confirm modal invisibly while it captured input.
		if m.logsView.Active() {
			return m, nil, false
		}
		var target string
		items, m.palRowByLabel, target = m.paletteRowVerbs(nil)
		if len(items) == 0 {
			return m, nil, false
		}
		title = paletteTitle + paletteTargetSep + target
	default:
		return m, nil, false
	}
	if !pending && len(items) == 0 {
		return m, nil, false
	}
	m.palArg = a
	// Committed off the verb list unless openPaletteArg says otherwise — it is the
	// caller that knows, and it sets the flag back on the model this returns.
	m.palDirect = false
	if title == "" {
		title = a.Describe()
	}
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
// the *same* function the verb's standalone gesture ends in — selectResource,
// togglePin, applyThemeNamed, applyNamespaceValue, applyContextLabel,
// dispatchRowAction — so an argument reached through the palette and one reached
// through the picker are one code path, in the spirit of D197's "no second
// implementation": the palette resolves and applies, it never re-implements what the
// verb does.
//
// The row map is read out *before* closePalette, which clears it: unlike resByLabel and
// themeByLabel it is the palette's own state, so an arm that resolved after the close
// would find nothing and the pick would silently do nothing.
func (m Model) applyPaletteArg(a keymap.Action, value string) (tea.Model, tea.Cmd) {
	rowByLabel := m.palRowByLabel
	m.closePalette()
	switch a {
	case keymap.ActionResources:
		r, ok := m.resByLabel[value]
		if !ok {
			return m, nil // the stage only lists labels it mapped — defensive.
		}
		return m.selectResource(r)
	case keymap.ActionPin:
		r, ok := m.resByLabel[value]
		if !ok {
			return m, nil // the stage only lists labels it mapped — defensive.
		}
		return m.togglePin(r)
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
	case keymap.ActionActions:
		act, ok := rowByLabel[value]
		if !ok {
			return m, nil // the stage only lists labels it mapped — defensive.
		}
		// The same dispatch the direct keys and the verb stage's row entries reach, so
		// the intent, its preconditions and its confirm modal are the row action's own
		// (D205 pt 3). The palette adds a way in, never a way past a confirmation.
		return m.dispatchRowAction(act)
	}
	return m, nil
}

// handleCommandSelected resolves a pick in the palette. In the argument stage the pick
// is the argument, so the verb runs with it; in the verb stage a verb that takes an
// argument commits into that stage (staying in this one modal — the point of the PAL
// line) and every other verb is handed to handleAction, the same dispatch a key press
// reaches (D11). A label with no mapping — the palette can only list labels it mapped,
// so this is defensive — closes it without running anything.
//
// A row verb (PAL-04) ends in dispatchRowAction — the one function `a` and the direct
// keys already end in — so the palette emits the same rowActionMsg intent against the
// same selected row, and every precondition and confirm modal on the way to the act
// itself is the row action's own. The globals are resolved first: the two maps are
// disjoint by construction (paletteRowVerbs drops a colliding title) and looking here
// first is what makes that ordering explicit rather than incidental.
func (m Model) handleCommandSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	if m.palArg != "" {
		return m.applyPaletteArg(m.palArg, msg.Value)
	}
	a, ok := m.cmdByLabel[msg.Value]
	if !ok {
		if act, isRow := m.palRowByLabel[msg.Value]; isRow {
			m.closePalette()
			return m.dispatchRowAction(act)
		}
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
//
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
