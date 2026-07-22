// Package viewer is kubecom's shared read-only text pager: a centered, bordered
// overlay that renders a block of text (an object's YAML, a describe dump, a log
// stream, a revealed secret) in a scrollable viewport with a title bar. It is the
// common substrate every M3 viewer sits on — M3-03 (YAML), M3-04 (describe),
// M3-05..07 (logs), M3-08 (secret) all feed content into this one component rather
// than each re-implementing scroll + framing.
//
// This slice (M3-01) is the component in isolation — construction, content/size
// wiring, keymap-action scrolling, and the close message. In-viewer `/` search
// (`n`/`N`) is a later slice; the first cut is scroll + render only. There is no
// app-shell wiring here (a viewer is not reachable until M3-02 gives actions a home).
//
// Like the picker (M2-08) it wraps a bubbles component (viewport) but drives it
// entirely through keymap.Actions — it never matches a raw key (D11): the root model
// resolves a KeyMsg to an Action and hands the Action to Update. The viewport's own
// key bindings are never fed a KeyMsg, so no hard-coded key leaks into behaviour. It
// emits its own message type (ClosedMsg) so it never imports the root package (D56),
// holds no shared mutable state (principle 1), and returns a **bare box** the root
// composites over the base browse view via overlayCenter (D95) — it never replaces
// the base.
package viewer

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// ClosedMsg is emitted when the user dismisses the viewer (nav.back). Kind
// identifies which viewer closed (e.g. "yaml", "describe", "logs") so the root
// model can route the result. Owned by the viewer package — the emitter — so the
// root model handles this concrete type without the viewer importing it (D56).
type ClosedMsg struct {
	Kind string
}

// Layout: the viewer takes most of the screen (viewers show large content — YAML,
// logs, describe output — so unlike the small picker it wants room), clamped to a
// margin so the base browse view still peeks around the bordered box.
const (
	screenMargin = 4 // cells kept clear around the box on each axis
	minWidth     = 20
	minHeight    = 5
	titleHeight  = 1 // the title line above the viewport
)

// Model is the read-only pager overlay. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles   styles.Styles
	viewport viewport.Model
	kind     string // stamped into ClosedMsg
	title    string // shown above the content

	active bool // whether the viewer is shown (captures input) — "" View when false
	width  int  // full screen width  (the box is centered within it)
	height int  // full screen height
}

// New builds a viewer of the given kind rendered through the shared styles. It
// starts hidden and empty; the caller seeds it with SetContent and reveals it with
// Show. The viewport's mouse-wheel scroll is disabled so all input flows through
// keymap actions (D11) — the root model never forwards a raw KeyMsg to it.
func New(s styles.Styles, kind string) Model {
	vp := viewport.New()
	vp.MouseWheelEnabled = false
	return Model{
		styles:   s,
		viewport: vp,
		kind:     kind,
		title:    kind,
	}
}

// Kind returns the viewer's kind id.
func (m Model) Kind() string { return m.kind }

// SetTitle overrides the title shown above the content.
func (m *Model) SetTitle(t string) { m.title = t }

// SetContent replaces the displayed text and resets the scroll position to the top,
// so opening a viewer always starts at the first line regardless of a prior scroll.
func (m *Model) SetContent(text string) {
	m.viewport.SetContent(text)
	m.viewport.GotoTop()
}

// SetSize records the full screen size; the box is sized and centered within it, and
// the inner viewport is sized to the box's content area.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	iw, ih := m.innerSize()
	m.viewport.SetWidth(iw)
	m.viewport.SetHeight(ih)
}

// Show reveals the viewer (it then captures input until Hide). Hide dismisses it.
func (m *Model) Show() { m.active = true }
func (m *Model) Hide() { m.active = false }

// Active reports whether the viewer is shown and capturing input.
func (m Model) Active() bool { return m.active }

// AtBottom reports whether the viewport is scrolled to the last line — used by the
// follow-logs slice (M3-06) to decide whether to auto-scroll on new content.
func (m Model) AtBottom() bool { return m.viewport.AtBottom() }

// Update handles a resolved keymap action while the viewer is active. Navigation
// scrolls the viewport (vim nav + gg/G + half/full page, D10); nav.back closes the
// viewer (ClosedMsg) for the root model to hide. The viewer consumes actions, never
// raw keys (D11); an inactive viewer ignores everything. Scrolling never emits a cmd,
// so the returned Model is the only effect for nav; close returns the ClosedMsg cmd.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.viewport.ScrollUp(1)
	case keymap.ActionDown:
		m.viewport.ScrollDown(1)
	case keymap.ActionTop:
		m.viewport.GotoTop()
	case keymap.ActionBottom:
		m.viewport.GotoBottom()
	case keymap.ActionHalfPageUp:
		m.viewport.HalfPageUp()
	case keymap.ActionHalfPageDown:
		m.viewport.HalfPageDown()
	case keymap.ActionPageUp:
		m.viewport.PageUp()
	case keymap.ActionPageDown:
		m.viewport.PageDown()
	case keymap.ActionBack:
		kind := m.kind
		return m, func() tea.Msg { return ClosedMsg{Kind: kind} }
	}
	return m, nil
}

// boxSize is the bordered box's total width/height (including its border): the
// screen minus a small margin on each axis, floored so it never collapses, and never
// larger than the screen.
func (m Model) boxSize() (int, int) {
	w := m.width - screenMargin
	if w < minWidth {
		w = minWidth
	}
	if w > m.width {
		w = m.width
	}
	h := m.height - screenMargin
	if h < minHeight {
		h = minHeight
	}
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

// innerSize is the viewport's content size inside the box frame: the box width/height
// minus the border (2 each) and the title line.
func (m Model) innerSize() (int, int) {
	bw, bh := m.boxSize()
	iw := bw - 2
	ih := bh - 2 - titleHeight
	if iw < 0 {
		iw = 0
	}
	if ih < 0 {
		ih = 0
	}
	return iw, ih
}

// View renders the viewer as a bordered box with a title bar above the scrollable
// content, or "" when the viewer is hidden or unsized. The root model composites the
// box centered over the base browse view (overlayCenter, D95) so the two-pane layout
// stays visible underneath — the viewer floats, it never replaces the base.
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	iw, _ := m.innerSize()
	title := m.styles.Header.Width(iw).MaxWidth(iw).Render(clip(m.title, iw))
	body := lipgloss.JoinVertical(lipgloss.Left, title, m.viewport.View())
	return m.styles.PaneFocus.Render(body)
}

// clip truncates s to at most w display cells so a long title never overflows the
// box frame. A non-positive width yields "".
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Trim rune-by-rune until it fits; cheap for a one-line title.
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ")
}
