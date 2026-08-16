// Package picker is kubecom's modal list picker: a centered, bordered overlay for
// choosing one value from a set. The namespace switcher (M2-08) is the first user;
// the same scaffolding is reused for the context/container/port pickers later, so
// the picker is generic over string values and stamped with a Kind so the root
// model can tell which picker resolved.
//
// This slice (M2-08a) is the component in isolation — construction, item/size
// wiring, keymap-action navigation, and selection/cancel messages. Incremental
// filtering is a later slice (M2-08b); the namespace-list plumbing and the app-shell
// wiring are M2-08c.
//
// Since STORY-06d a picker opens in **navigation mode**: the list is focused, `j`/`k`
// move the cursor, and the filter stays closed until `/` opens it — the walk's
// finding that a picker whose field swallowed every letter was its sharpest dead end
// (D272). The filter, when open, ranks by `kube.NameMatcher` — the cluster search's
// own matcher — so a picker narrows as you type with the same substring-above-
// subsequence ordering the search view has (D194). The one surface whose identity is
// typing — the command palette's verb list — opens filtered instead (ShowFiltered,
// D197), and a caller may preselect the current choice with SelectValue.
//
// Since PAL-08 a value may also carry a **Name** — its own short id — which is drawn
// in a left-hand column before the label and matched alongside it (D237). Only the
// command palette seeds names today; a list where nothing is named renders exactly as
// it always has, one column of labels.
//
// It wraps bubbles/list for cursor and pagination management (and, later, its native
// filter) but drives it entirely through keymap.Actions — it never matches a raw key
// (D11): the root model resolves a KeyMsg to an Action and hands the Action to Update.
// The list's own key bindings and chrome (title/help/status/filter) are disabled so
// no hard-coded key leaks into the view; the picker frames and titles the list itself
// through the shared styles. Like the other components it emits its own message types
// (SelectedMsg/CancelledMsg) so it never imports the root package (D56), and it holds
// no shared mutable state (principle 1): the root model owns the one Model, feeds it
// actions, and reads its View.
package picker

import (
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// SelectedMsg is emitted when the user confirms the highlighted value (nav.drillIn).
// Kind identifies which picker resolved (e.g. "namespace") so the root model can
// route the result; Value is the chosen string. Owned by the picker package — the
// emitter — so the root model handles this concrete type without the picker
// importing it (D56).
type SelectedMsg struct {
	Kind  string
	Value string
}

// CancelledMsg is emitted when the user dismisses the picker without choosing
// (nav.back). Kind identifies the picker, as in SelectedMsg.
type CancelledMsg struct {
	Kind string
}

// Layout: the modal is a fraction of the screen, clamped to sensible bounds, and
// centered over whatever is behind it.
const (
	modalMinWidth  = 24
	modalMaxWidth  = 60
	modalMinHeight = 5
	modalMaxHeight = 20
	screenMargin   = 4 // cells kept clear around the modal on each axis
	titleHeight    = 1 // the title line above the list
	filterHeight   = 1 // the filter input line (only while filtering)
)

// item is one visible row: the label (the value, and the identity a pick resolves
// by) plus the optional name rendered in the left-hand column before it (PAL-08).
// It satisfies list.Item; the label is the substring-match target used by the
// picker's own incremental filter (M2-08b).
type item struct {
	name  string
	label string
}

func (i item) FilterValue() string { return i.label }

// Item is one pickable value: the Label the reader sees, picks and gets back in
// SelectedMsg, an optional Name shown in a column *before* it, plus Aliases —
// extra terms the filter matches against but never shows (CRD-PIN-04).
//
// The aliases exist because a label is a *name for a reader* while a query is
// whatever the reader happens to know the thing as. The resource picker is the
// case that forced it: its labels are Kinds ("Pod", "ExternalSecret") and the name
// a Kubernetes user types is very often the one kubectl takes — the plural
// ("externalsecrets"), a short name ("es"), or the API group — none of which is a
// subsequence of the Kind, so typing them matched nothing at all.
//
// An alias is match-only on purpose: putting the extra terms in the label instead
// would make every row of every picker carry text nobody reads, and the label is
// also the identity a SelectedMsg is resolved by (resByLabel and its siblings), so
// widening it would widen that key too.
//
// Name is the value's own short name where it has one — the command palette's
// verbs are the case that forced it (PAL-08): their labels are the registry's
// *descriptions* ("Switch namespace"), which left the reader unable to see what the
// command is actually called. A named item renders as two columns, `name  label`,
// with the column sized to the widest visible name; an unnamed one renders exactly
// as it did before, so every other picker is untouched.
//
// Like an alias the Name is matched against, so typing a command's name finds it —
// but unlike an alias it is *shown*, and unlike the Label it is not the identity a
// SelectedMsg resolves by (D203 pt 3): the Label remains that, so a caller can add
// names without rekeying the maps that resolve a pick.
type Item struct {
	Label   string
	Name    string
	Aliases []string
}

// Labels lifts a plain value list into Items with no aliases — the shape every
// picker but the resource one wants, and what SetItems does internally. It exists
// so a caller that mixes the two (the command palette, whose verbs each seed the
// same picker) can hand one type to one setter.
func Labels(values []string) []Item {
	items := make([]Item, 0, len(values))
	for _, v := range values {
		items = append(items, Item{Label: v})
	}
	return items
}

// nameGap separates the name column from the label column of a named item. Two
// spaces rather than one because the columns are ragged text with no rule between
// them, and a single space reads as a word break inside one sentence.
const nameGap = "  "

// itemDelegate renders each item on a single line, highlighting the cursor row with
// the shared Selection style. It is a minimal list.ItemDelegate (no per-item state,
// no key bindings) so the list contributes no hard-coded keys or help of its own.
//
// nameW is the width of the name column — the widest name among the *visible*
// items, or 0 when nothing in the list is named. The picker recomputes it whenever
// the item set changes (syncDelegate), so a narrowing query shrinks the column and
// gives the width back to the labels rather than reserving room for names that are
// no longer on screen.
type itemDelegate struct {
	styles styles.Styles
	nameW  int
}

func (d itemDelegate) Height() int                         { return 1 }
func (d itemDelegate) Spacing() int                        { return 0 }
func (d itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// rowEllipsis marks a row cut short by the modal's width. A picker row is one line
// by contract (Height() == 1), so the alternative to cutting is not a wider row, it
// is a *wrapped* one — which silently costs the list a row and pushes everything
// below it down (D237 pt 3).
const rowEllipsis = "…"

// Render draws one row: the label alone in an unnamed list, or `name  label` with
// the name padded to the column width. The cursor row is drawn in the Selection
// style as one piece — its background is a full-width bar, and a second foreground
// inside it would either fight the bar or break it (D170's "a component draws
// through the Styles it was handed"); every other row dims the name to Subtle, since
// the label is what a reader scans and the name is what they look for.
//
// The composed line is truncated to the list width *before* it is styled, because
// lipgloss's Width() wraps rather than clips: a row longer than the modal would
// otherwise become two rows, and the row after it would fall off the bottom.
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	row, _ := it.(item)
	width := m.Width()
	if width < 0 {
		width = 0
	}
	selected := index == m.Index()
	if d.nameW == 0 {
		style := d.styles.App
		if selected {
			style = d.styles.Selection
		}
		_, _ = io.WriteString(w, style.Width(width).Render(fit(row.label, width)))
		return
	}
	name := padRight(row.name, d.nameW)
	if selected {
		line := fit(name+nameGap+row.label, width)
		_, _ = io.WriteString(w, d.styles.Selection.Width(width).Render(line))
		return
	}
	line := d.styles.Subtle.Render(name) + d.styles.App.Render(nameGap+row.label)
	_, _ = io.WriteString(w, d.styles.App.Width(width).Render(fit(line, width)))
}

// fit truncates s to w cells, marking the cut, and is ANSI-aware so a styled segment
// keeps its escape sequences intact when the text inside it is cut.
func fit(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, rowEllipsis)
}

// padRight pads s with spaces to w cells (measured as the terminal sees them, so a
// wide rune costs what it draws), and leaves an over-long name alone — the row is
// clipped once, at the list width, exactly as an over-long label always has been.
func padRight(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// Model is the modal picker. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles
	list   list.Model
	kind   string // stamped into SelectedMsg/CancelledMsg
	title  string // shown above the list

	// all is the unfiltered value set (SetItems input). The list always shows the
	// subset matching the current filter query; all is the source it is rebuilt from
	// so clearing the filter restores every value without re-seeding.
	all       []Item
	filter    textinput.Model // the incremental filter field (shown only while filtering)
	filtering bool            // whether the filter field is open and capturing text

	// filterOnShow records whether this surface's filter opens with the picker
	// (ShowFiltered) instead of staying closed until `/` opens it (Show, the
	// navigation-mode default since STORY-06d). It is a per-show property: the
	// command palette's one picker serves both a type-to-filter verb list and
	// navigation-mode argument stages, so the palette toggles it as the stage
	// changes (OpenFilter/CloseFilter). It drives what esc does while filtering:
	// on a filter-on-show surface it clears the query; on a navigation-mode picker
	// it closes the field it opened.
	filterOnShow bool

	// nameW is the current name-column width (0 in a list where nothing is named).
	// It is derived from the visible items by applyFilter and handed to the delegate.
	nameW int

	active bool // whether the picker is shown (captures input) — "" View when false
	width  int  // full screen width  (the modal is centered within it)
	height int  // full screen height
}

// New builds a picker of the given kind (also its default title) rendered through
// the shared styles. It starts hidden and empty; the caller seeds it with SetItems
// and reveals it with Show (navigation mode) or ShowFiltered (type-to-filter). The
// list's own chrome and key bindings — including its native filter — stay disabled:
// the picker runs its own incremental filter over an owned textinput (M2-08b) so it
// fully controls input and appearance, and no hard-coded list key leaks into the
// view (D11).
func New(s styles.Styles, kind string) Model {
	l := list.New(nil, itemDelegate{styles: s}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false) // the picker filters itself; the list's native filter stays off
	l.DisableQuitKeybindings()   // this picker never quits the app

	fi := textinput.New()
	fi.Prompt = "/ "
	m := Model{
		styles: s,
		list:   l,
		kind:   kind,
		title:  strings.ToUpper(kind[:1]) + kind[1:],
		filter: fi,
	}
	return m
}

// SetStyles repaints the picker through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
//
// The list's delegate has to be replaced, not just m.styles assigned: the delegate
// is what draws every row (and the cursor's Selection bar), and list.New was handed
// a *copy* of the old Styles. Swapping it leaves the items, the cursor position and
// the scroll page exactly as they were — only the row that draws them changes — so
// an open picker can be restyled without losing the reader's place.
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.syncDelegate()
}

// syncDelegate rebuilds the row renderer from the picker's current styles and name
// column width. It is the one place the delegate is replaced, so the two inputs
// cannot drift apart — a set of items seeded after a theme change is rendered
// through the new palette, and a restyle keeps the column the items need.
func (m *Model) syncDelegate() {
	m.list.SetDelegate(itemDelegate{styles: m.styles, nameW: m.nameW})
}

// Kind returns the picker's kind id.
func (m Model) Kind() string { return m.kind }

// SetTitle overrides the title shown above the list.
func (m *Model) SetTitle(t string) { m.title = t }

// SetPrompt overrides the filter field's prompt (the text drawn before the query).
// The default is the `/ ` of an ordinary filter; the command palette re-prompts
// itself as its line advances — `: ` for a verb, `:resource ` once that verb is
// committed (PAL-03a) — so the one line reads as `<verb> <argument>` while staying
// one field. The picker itself attaches no meaning to the prompt: it is chrome.
func (m *Model) SetPrompt(p string) { m.filter.Prompt = p }

// Query is the current filter text (empty when the filter is closed or untouched).
// It lets the embedder distinguish "the reader has typed something" from "this is a
// fresh field" — which is what makes an editing key at the start of the line
// meaningful (the palette leaves its argument stage on a backspace into an empty
// query) without the embedder tracking a second copy of the field's contents.
func (m Model) Query() string { return m.filter.Value() }

// ClearQuery empties the filter query and restores the full list, leaving the field
// open and focused. It is the swap-in-place counterpart to Hide/Show: a surface that
// replaces its item set while staying on screen (the palette committing a verb) must
// clear the query first, since SetItems applies whatever query is standing.
func (m *Model) ClearQuery() {
	m.filter.Reset()
	m.applyFilter()
}

// SetItems replaces the picker's values (the unfiltered set) and shows the subset
// matching the current filter query, cursor reset to the top. Each value is both
// what is shown and the only thing matched; a picker whose values answer to other
// names uses SetItemsWithAliases.
func (m *Model) SetItems(values []string) {
	m.SetItemsWithAliases(Labels(values))
}

// SetItemsWithAliases is SetItems with match-only terms attached to each value
// (Item.Aliases) — see Item. Both setters land here, so aliases are replaced with
// the item set rather than accumulating across seeds: a picker reseeded in place
// (the palette committing a verb) can never match against the last stage's terms.
func (m *Model) SetItemsWithAliases(items []Item) {
	m.all = append(m.all[:0:0], items...)
	m.applyFilter()
}

// applyFilter rebuilds the visible list from the unfiltered set, keeping the values
// that match the filter query and ordering them best-match-first, cursor reset to the
// top. An empty query shows everything **in the caller's order** — SetItems order is
// meaningful (the namespace sentinel is pinned first, the context picker marks the
// current context) and there is nothing to rank against, so it is left alone.
//
// Matching and ranking are kube.NameMatcher's, the cluster search's own matcher
// (D194 pt 1): a contiguous match first and, failing that, the needle as a
// subsequence, with every contiguous match scoring above every scattered one
// (D152 pt 3/D153). So typing `ksys` reaches `kube-system` while an exact hit can
// never be pushed below a scattered one — the property the search view relies on,
// now the same one keystroke for keystroke in every picker.
//
// The sort is stable, so values the matcher scores equally keep the caller's order.
//
// A value with Aliases is matched against its label *and* each alias, and scored by
// the best of them, unpenalised (CRD-PIN-04): an alias is a name the reader may
// genuinely have meant — `es` for ExternalSecret is the name kubectl answers to —
// so a row hit through one ranks beside a row hit through its label, and the
// contiguous-over-scattered ordering (D194 pt 1) still decides between them.
func (m *Model) applyFilter() {
	items := make([]list.Item, 0, len(m.all))
	if strings.TrimSpace(m.filter.Value()) == "" {
		for _, v := range m.all {
			items = append(items, item{name: v.Name, label: v.Label})
		}
		m.setVisible(items)
		return
	}
	matcher := kube.NewNameMatcher(m.filter.Value())
	type hit struct {
		row   item
		score int
	}
	hits := make([]hit, 0, len(m.all))
	for _, v := range m.all {
		if score, ok := matchItem(matcher, v); ok {
			hits = append(hits, hit{row: item{name: v.Name, label: v.Label}, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	for _, h := range hits {
		items = append(items, h.row)
	}
	m.setVisible(items)
}

// setVisible installs the visible rows, re-measures the name column against them
// and resets the cursor to the top. Measuring here rather than over m.all is what
// makes the column follow the query: a list narrowed to one short name gives the
// width back to the labels instead of holding a gutter for rows it no longer shows.
func (m *Model) setVisible(items []list.Item) {
	nameW := 0
	for _, it := range items {
		row, ok := it.(item)
		if !ok {
			continue
		}
		if w := lipgloss.Width(row.name); w > nameW {
			nameW = w
		}
	}
	if nameW != m.nameW {
		m.nameW = nameW
		m.syncDelegate()
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

// matchItem scores one item against the query: the best score among its label, its
// name and its aliases, and whether anything matched at all. Taking the best rather
// than the first means the order aliases are listed in carries no meaning — a caller
// adds the names a value answers to, not a ranked list.
//
// The Name is matched for the same reason an alias is, and more strongly: it is on
// screen, so a reader who can see `ns.switch` will type it (PAL-08).
func matchItem(matcher kube.NameMatcher, it Item) (int, bool) {
	best, _, ok := matcher.Match(it.Label)
	for _, alt := range append([]string{it.Name}, it.Aliases...) {
		if alt == "" {
			continue
		}
		score, _, hit := matcher.Match(alt)
		if hit && (!ok || score > best) {
			best, ok = score, true
		}
	}
	return best, ok
}

// SetSize records the full screen size; the modal is sized and centered within it.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.syncListSize()
}

// syncListSize re-applies the inner content size to the list and filter field. It is
// called on resize and whenever the filter opens/closes, since the filter line steals
// one row from the list while it is shown.
func (m *Model) syncListSize() {
	iw, ih := m.innerSize()
	m.list.SetSize(iw, ih)
	m.filter.SetWidth(iw)
}

// Show reveals the picker in navigation mode: the list is focused and j/k move the
// cursor, while the filter stays closed until `/` (ActionFilter) opens it. This is
// the default open state for every picker since STORY-06d (D272) — a picker whose
// field swallowed every letter was the walk's sharpest dead end. A surface whose
// identity is typing opens with ShowFiltered instead (the command palette verb list,
// D197). Returns nil — navigation mode focuses no field. Hide dismisses the picker
// and closes the filter so it reopens clean next time.
func (m *Model) Show() tea.Cmd {
	m.active = true
	m.filterOnShow = false
	if m.filtering {
		m.closeFilter()
	}
	return nil
}

// ShowFiltered reveals the picker in type-to-filter mode: the filter field opens
// with it, focused and capturing text. Only the command palette's verb list wants
// this — its identity is "one place you type to make anything happen" (D197), and
// the walk that overturned type-to-filter for value pickers gave the palette a clean
// bill (D272). Returns the filter field's cursor blink cmd.
func (m *Model) ShowFiltered() tea.Cmd {
	m.active = true
	m.filterOnShow = true
	if m.filtering {
		return nil
	}
	m.filtering = true
	m.syncListSize()
	return m.filter.Focus()
}

// OpenFilter opens and focuses the filter field on an already-shown picker, leaving
// it in type-to-filter mode. It is the in-place counterpart of ShowFiltered — how
// the palette returns to its verb list from a navigation-mode argument stage — and
// how a navigation-mode picker's `/` is routed. Returns the cursor blink cmd.
func (m *Model) OpenFilter() tea.Cmd {
	m.filterOnShow = true
	if !m.active || m.filtering {
		return nil
	}
	m.filtering = true
	m.syncListSize()
	return m.filter.Focus()
}

// CloseFilter closes the filter field, returning an already-shown picker to
// navigation mode. It is the in-place counterpart of Show — how the palette enters
// an argument stage from its type-to-filter verb list. Safe to call when the filter
// is already closed.
func (m *Model) CloseFilter() {
	m.filterOnShow = false
	m.closeFilter()
}

func (m *Model) Hide() {
	m.active = false
	m.closeFilter()
}
func (m Model) Active() bool { return m.active }

// Len is the number of currently visible (post-filter) values.
func (m Model) Len() int { return len(m.list.Items()) }

// Filtering reports whether the filter field is open and capturing text. The root
// model uses it to route raw text keys to UpdateFilter (M2-08c) while it is true.
func (m Model) Filtering() bool { return m.filtering }

// Selected returns the highlighted value and true, or "" and false when the picker
// is empty.
func (m Model) Selected() (string, bool) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return "", false
	}
	return it.label, true
}

// SelectValue moves the cursor to the row whose label is v, if present, and reports
// whether it did. It is how a caller preselects the current choice when a picker
// opens (STORY-06d): the namespace stage preselects the current workspace, the theme
// stage the current theme, and so on. A value absent from the list (a scope that no
// longer exists) leaves the cursor at the top, which is the sensible fallback.
func (m *Model) SelectValue(v string) bool {
	items := m.list.Items()
	for i, it := range items {
		if row, ok := it.(item); ok && row.label == v {
			m.list.Select(i)
			return true
		}
	}
	return false
}

// Update handles a resolved keymap action while the picker is active. Navigation
// moves the cursor (bubbles/list manages pagination); nav.drillIn confirms the
// selection (SelectedMsg) and nav.back dismisses it (CancelledMsg) — both leave the
// picker for the root model to hide. The picker consumes actions, never raw keys
// (D11); an inactive picker ignores everything.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.list.CursorUp()
	case keymap.ActionDown:
		m.list.CursorDown()
	case keymap.ActionTop:
		m.list.GoToStart()
	case keymap.ActionBottom:
		m.list.GoToEnd()
	case keymap.ActionHalfPageDown, keymap.ActionPageDown:
		m.list.NextPage()
	case keymap.ActionHalfPageUp, keymap.ActionPageUp:
		m.list.PrevPage()
	case keymap.ActionFilter:
		// Open the filter field (no-op if already open). Focus returns the cursor
		// blink cmd; the field then steals a row from the list.
		if m.filtering {
			return m, nil
		}
		m.filtering = true
		cmd := m.filter.Focus()
		m.syncListSize()
		return m, cmd
	case keymap.ActionDrillIn:
		v, ok := m.Selected()
		if !ok {
			return m, nil
		}
		kind := m.kind
		return m, func() tea.Msg { return SelectedMsg{Kind: kind, Value: v} }
	case keymap.ActionBack:
		// One esc clears the filter, a second cancels the picker. What "clears" means
		// differs by mode: a navigation-mode picker's filter is an overlay `/` opened
		// — esc closes it, returning to the plain list it was opened from — while a
		// filter-on-show surface (the verb list, D197) only empties the query: closing
		// its field would leave a picker that no longer does the one thing it exists
		// to do. An already-empty query falls through to the cancel below, so esc-esc
		// dismisses in both modes.
		if m.filtering {
			if !m.filterOnShow {
				m.closeFilter()
				return m, nil
			}
			if m.filter.Value() != "" {
				m.filter.Reset()
				m.applyFilter()
				return m, nil
			}
		}
		kind := m.kind
		return m, func() tea.Msg { return CancelledMsg{Kind: kind} }
	}
	return m, nil
}

// UpdateFilter feeds one raw key to the filter field and re-narrows the visible list.
// It is the picker's only raw-key entry point and is meaningful only while the filter
// is open: the root model resolves control keys (nav, drill-in, back) to actions and
// routes everything else — the text content — here, so the field captures typing
// without any view matching a raw key for behaviour (D11). A no-op otherwise.
func (m Model) UpdateFilter(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.active || !m.filtering {
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()
	return m, cmd
}

// closeFilter clears and hides the filter field, restoring the full list. Safe to call
// when the filter is already closed.
func (m *Model) closeFilter() {
	if !m.filtering {
		return
	}
	m.filtering = false
	m.filter.Blur()
	m.filter.Reset()
	m.applyFilter()
	m.syncListSize()
}

// innerSize is the list's content size inside the modal frame: the modal width/height
// minus the border (2 each), the title line, and — while the filter is open — the
// filter input line.
func (m Model) innerSize() (int, int) {
	mw, mh := m.modalSize()
	iw := mw - 2
	ih := mh - 2 - titleHeight
	if m.filtering {
		ih -= filterHeight
	}
	if iw < 0 {
		iw = 0
	}
	if ih < 0 {
		ih = 0
	}
	return iw, ih
}

// modalSize is the modal box's total width/height (including its border): a fraction
// of the screen clamped to [min,max] and never wider/taller than the screen minus a
// small margin.
func (m Model) modalSize() (int, int) {
	w := clamp(m.width-screenMargin, modalMinWidth, modalMaxWidth)
	if w > m.width {
		w = m.width
	}
	h := clamp(m.height-screenMargin, modalMinHeight, modalMaxHeight)
	if h > m.height {
		h = m.height
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// View renders the picker as a bordered modal box, or "" when the picker is
// hidden or unsized. The box is a bordered frame with a title line above the
// list; the root model composites it centered over the base browse view
// (overlayCenter, D95) so the two-pane layout stays visible underneath.
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	iw, _ := m.innerSize()
	title := m.styles.Header.Width(iw).MaxWidth(iw).Render(m.title)
	parts := []string{title}
	if m.filtering {
		parts = append(parts, m.styles.App.Width(iw).MaxWidth(iw).Render(m.filter.View()))
	}
	parts = append(parts, m.list.View())
	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.styles.PaneFocus.Render(body)
}
