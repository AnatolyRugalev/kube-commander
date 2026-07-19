// Package menu is kubecom's resource-menu sidebar: the left pane of the browse
// view, a vertical list of Kubernetes resource kinds the user moves through to
// choose what the table pane shows. This slice (M2-05a) seeds a static list of
// core resource kinds; a later slice (M2-05b) reconciles the list with the async
// discovery result (adding CRDs/extra groups, marking unavailable ones) without
// disturbing the current selection or scroll.
//
// The menu never matches a raw key (D11): the root model resolves a KeyMsg to a
// keymap.Action and hands the Action to Update, which moves the selection. When
// the user drills in (nav.drillIn) the menu emits a ResourceSelectedMsg — its own
// message type, owned here rather than in package tui, so the component does not
// import the root package (which would cycle, since the root imports this one)
// (D56). It owns no shared mutable state (principle 1): the root model holds the
// one Model, feeds it actions, and reads its View.
package menu

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// Item is one row in the menu: a resource kind plus its display title and an
// availability flag. Available is true for every seed item; M2-05b sets it false
// for a discovered-but-unreachable group so the row renders muted but is not
// selectable for a watch.
type Item struct {
	Resource  kube.Resource
	Title     string
	Available bool
}

// ResourceSelectedMsg is emitted when the user drills into the highlighted menu
// item (nav.drillIn). The root model reacts by (re)starting a watch for Resource.
// It is owned by the menu package — the emitter — so the root model handles this
// concrete type; the menu never imports the root package (D56).
type ResourceSelectedMsg struct {
	Resource kube.Resource
}

// Model is the resource menu. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles
	items  []Item

	cursor  int // index of the highlighted item
	offset  int // index of the first visible item (vertical scroll)
	width   int // total width incl. border
	height  int // total height incl. border
	focused bool
}

// New builds a resource menu seeded with the default core-resource list, rendered
// through the given styles. The first item is selected.
func New(s styles.Styles) Model {
	return Model{styles: s, items: seedItems()}
}

// SetSize sets the menu's total size (including its border). The root model wires
// this from the pane geometry it computes on a WindowSizeMsg.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
}

// Focus marks the menu as holding focus (accented border).
func (m *Model) Focus() { m.focused = true }

// Blur marks the menu as not holding focus.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the menu holds focus.
func (m Model) Focused() bool { return m.focused }

// Cursor is the index of the highlighted item (for tests / the root model).
func (m Model) Cursor() int { return m.cursor }

// Items returns the menu's items in display order.
func (m Model) Items() []Item { return m.items }

// Selected returns the currently highlighted item and true, or a zero Item and
// false when the menu is empty.
func (m Model) Selected() (Item, bool) {
	if len(m.items) == 0 {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

// Update handles a resolved keymap action. Navigation actions (up/down/top/
// bottom) move the highlight and keep it visible; nav.drillIn emits a
// ResourceSelectedMsg for the highlighted item so the root model can start its
// watch. Any other action is ignored (the root routes it elsewhere). The menu
// consumes actions, never raw keys (D11).
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if len(m.items) == 0 {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.moveTo(m.cursor - 1)
	case keymap.ActionDown:
		m.moveTo(m.cursor + 1)
	case keymap.ActionTop:
		m.moveTo(0)
	case keymap.ActionBottom:
		m.moveTo(len(m.items) - 1)
	case keymap.ActionDrillIn:
		item := m.items[m.cursor]
		if !item.Available {
			return m, nil
		}
		res := item.Resource
		return m, func() tea.Msg { return ResourceSelectedMsg{Resource: res} }
	}
	return m, nil
}

// moveTo sets the cursor to i (clamped to the item range) and scrolls so it stays
// visible.
func (m *Model) moveTo(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(m.items)-1 {
		i = len(m.items) - 1
	}
	m.cursor = i
	m.scrollToCursor()
}

// innerHeight is the number of item rows the pane can show (total height minus the
// top and bottom border rows), never negative.
func (m Model) innerHeight() int {
	h := m.height - 2
	if h < 0 {
		return 0
	}
	return h
}

// scrollToCursor adjusts the scroll offset so the cursor is within the visible
// window.
func (m *Model) scrollToCursor() {
	h := m.innerHeight()
	if h == 0 {
		m.offset = m.cursor
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

// clampOffset keeps the scroll offset valid after a resize, preferring to keep the
// cursor visible.
func (m *Model) clampOffset() {
	h := m.innerHeight()
	maxOffset := len(m.items) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
	m.scrollToCursor()
}

// View renders the menu as a bordered vertical list. It returns "" until the menu
// has been sized (before the first WindowSizeMsg), so the root model lays nothing
// out prematurely.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	h := m.innerHeight()

	lines := make([]string, 0, h)
	for row := 0; row < h; row++ {
		i := m.offset + row
		if i >= len(m.items) {
			lines = append(lines, m.styles.App.Width(innerW).Render(""))
			continue
		}
		lines = append(lines, m.renderItem(m.items[i], i == m.cursor, innerW))
	}

	content := strings.Join(lines, "\n")
	frame := m.styles.Pane
	if m.focused {
		frame = m.styles.PaneFocus
	}
	return frame.Width(innerW).Height(h).Render(content)
}

// renderItem renders one item line clamped to innerW: the highlighted item takes
// the Selection style (full-width bar), an unavailable item is muted, and a normal
// item takes the base style.
func (m Model) renderItem(it Item, selected bool, innerW int) string {
	switch {
	case selected:
		return m.styles.Selection.Width(innerW).MaxWidth(innerW).Render(it.Title)
	case !it.Available:
		return m.styles.Subtle.Width(innerW).MaxWidth(innerW).Render(it.Title)
	default:
		return m.styles.App.Width(innerW).MaxWidth(innerW).Render(it.Title)
	}
}
