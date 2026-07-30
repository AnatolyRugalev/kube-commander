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
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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

// item is one selectable string. It satisfies list.Item; the whole string is the
// substring-match target used by the picker's own incremental filter (M2-08b).
type item string

func (i item) FilterValue() string { return string(i) }

// itemDelegate renders each item on a single line, highlighting the cursor row with
// the shared Selection style. It is a minimal list.ItemDelegate (no per-item state,
// no key bindings) so the list contributes no hard-coded keys or help of its own.
type itemDelegate struct {
	styles styles.Styles
}

func (d itemDelegate) Height() int                         { return 1 }
func (d itemDelegate) Spacing() int                        { return 0 }
func (d itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	s, _ := it.(item)
	width := m.Width()
	if width < 0 {
		width = 0
	}
	style := d.styles.App
	if index == m.Index() {
		style = d.styles.Selection
	}
	_, _ = io.WriteString(w, style.Width(width).MaxWidth(width).Render(string(s)))
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
	all       []string
	filter    textinput.Model // the incremental filter field (shown only while filtering)
	filtering bool            // whether the filter field is open and capturing text

	active bool // whether the picker is shown (captures input) — "" View when false
	width  int  // full screen width  (the modal is centered within it)
	height int  // full screen height
}

// New builds a picker of the given kind (also its default title) rendered through
// the shared styles. It starts hidden and empty; the caller seeds it with SetItems
// and reveals it with Show. The list's own chrome and key bindings — including its
// native filter — stay disabled: the picker runs its own incremental filter over an
// owned textinput (M2-08b) so it fully controls input and appearance, and no
// hard-coded list key leaks into the view (D11).
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
	return Model{
		styles: s,
		list:   l,
		kind:   kind,
		title:  strings.ToUpper(kind[:1]) + kind[1:],
		filter: fi,
	}
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
	m.list.SetDelegate(itemDelegate{styles: s})
}

// Kind returns the picker's kind id.
func (m Model) Kind() string { return m.kind }

// SetTitle overrides the title shown above the list.
func (m *Model) SetTitle(t string) { m.title = t }

// SetItems replaces the picker's values (the unfiltered set) and shows the subset
// matching the current filter query, cursor reset to the top.
func (m *Model) SetItems(values []string) {
	m.all = append(m.all[:0:0], values...)
	m.applyFilter()
}

// applyFilter rebuilds the visible list from the unfiltered set, keeping only the
// values whose lowercased text contains the (lowercased) filter query, and resets the
// cursor to the top. An empty query shows everything.
func (m *Model) applyFilter() {
	q := strings.ToLower(m.filter.Value())
	var items []list.Item
	for _, v := range m.all {
		if q == "" || strings.Contains(strings.ToLower(v), q) {
			items = append(items, item(v))
		}
	}
	m.list.SetItems(items)
	m.list.Select(0)
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

// Show reveals the picker (it then captures input until Hide). Hide dismisses it and
// closes any open filter so it reopens clean next time.
func (m *Model) Show() { m.active = true }
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
	return string(it), true
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
		// While filtering, back closes the filter and restores the full list rather
		// than dismissing the picker — one esc clears the filter, a second cancels.
		if m.filtering {
			m.closeFilter()
			return m, nil
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
