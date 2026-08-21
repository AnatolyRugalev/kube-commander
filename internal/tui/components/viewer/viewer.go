// Package viewer is kubecom's shared read-only text pager: a bordered pane that
// renders a block of text (an object's YAML, a describe dump, a log
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
// holds no shared mutable state (principle 1), and returns a **bare box** sized to
// exactly the area it was given, which the root composites over the *right pane* of
// the browse view (D284): a pager is not a popup — its content is the thing being
// read, so it takes the whole pane rather than floating as an inset inside it. D95
// (popups overlay the browse view) still governs the modals — help, pickers, confirm.
package viewer

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// ClosedMsg is emitted when the user dismisses the viewer (nav.back). Kind
// identifies which viewer closed (e.g. "yaml", "describe", "logs") so the root
// model can route the result. Owned by the viewer package — the emitter — so the
// root model handles this concrete type without the viewer importing it (D56).
type ClosedMsg struct {
	Kind string
}

// Layout: the viewer fills the area it is sized to, edge to edge (D284). Viewers
// show large content — YAML, logs, describe output — and the shell hands them the
// right pane, so every cell of it is content; there is no margin to keep clear,
// since the pane the box replaces is the base that used to peek around it.
const (
	titleHeight = 1 // the title line above the viewport
)

// Model is the read-only pager overlay. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles   styles.Styles
	viewport viewport.Model
	kind     string // stamped into ClosedMsg
	title    string // shown above the content
	content  string // the accumulated text (so AppendContent can grow it in place)
	describe string // the unpainted describe dump behind a painted content, "" otherwise

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

// SetStyles repaints the viewer through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Colors only: the title and the scroll position are untouched, and so is the text
// itself — the viewport holds it unstyled, with the one exception of a painted
// describe dump (SetDescribeContent), whose colours *are* the palette: the raw dump
// is kept for exactly this, so it is repainted in place, at the same scroll offset,
// rather than left on the departed theme.
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	if m.describe == "" {
		return
	}
	m.content = PaintDescribe(s, m.describe)
	m.viewport.SetContent(m.content) // no GotoTop: a restyle is not a reopen.
}

// Kind returns the viewer's kind id.
func (m Model) Kind() string { return m.kind }

// SetKind restamps the viewer's kind so one shared component can serve every M3
// viewer (YAML / describe / logs / secret): the open path sets the kind it is
// showing, and it rides the ClosedMsg so the root can route the close. The root
// also reads it to gate kind-specific behaviour — the M3-06 follow toggle acts
// only while the logs viewer is up.
func (m *Model) SetKind(k string) { m.kind = k }

// SetTitle overrides the title shown above the content.
func (m *Model) SetTitle(t string) { m.title = t }

// SetContent replaces the displayed text and resets the scroll position to the top,
// so opening a viewer always starts at the first line regardless of a prior scroll.
func (m *Model) SetContent(text string) {
	m.describe = ""
	m.content = text
	m.viewport.SetContent(text)
	m.viewport.GotoTop()
}

// SetDescribeContent replaces the displayed text with the painted rendering of a
// describe dump (PaintDescribe, STORY-06h-2) and keeps the raw dump beside it, so a
// theme picked while the panel is open repaints it (SetStyles). It is the describe
// open path's SetContent: the shell hands over the describer's output and the
// panel's colouring is the component's business, so no styled string ever crosses
// the shell boundary.
func (m *Model) SetDescribeContent(text string) {
	m.SetContent(PaintDescribe(m.styles, text))
	m.describe = text
}

// AppendContent adds one more block of text (a single log line, M3-05) to the
// displayed content without disturbing the scroll position: the accumulated buffer
// grows and the viewport re-renders in place, so a reader scrolled partway through a
// streaming log stays where they are. It is the streaming counterpart to SetContent
// (which resets to the top for a fresh open). Auto-scroll-while-following is a later
// slice's concern (M3-06 uses AtBottom); this cut only grows the buffer. An empty
// first block seeds the buffer with the block; subsequent blocks are newline-joined.
func (m *Model) AppendContent(line string) {
	if m.content == "" {
		m.content = line
	} else {
		m.content += "\n" + line
	}
	m.viewport.SetContent(m.content)
}

// Empty reports whether the viewer has no content yet — used by the streaming logs
// viewer (M3-05) to decide whether a terminal stream error should close an
// empty box (an open failure, nothing shown yet) or leave the partial lines already
// on screen (a mid-stream drop after some output).
func (m Model) Empty() bool { return m.content == "" }

// SetSize records the area the viewer occupies — the shell's right pane, not the
// full screen (D284). The box fills it exactly and the inner viewport is sized to
// the box's content area inside the frame.
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

// GotoBottom pins the viewport to the last line. The follow-logs slice (M3-06)
// calls it after each appended line while follow is on, and when follow is
// (re-)enabled, so a following log tails the newest output.
func (m *Model) GotoBottom() { m.viewport.GotoBottom() }

// EnsureLineVisible scrolls the viewport the minimum amount so line n (0-based,
// into the current content) is on screen, leaving it where it is if already
// visible. The secret viewer (M3-08b) calls it after moving the entry cursor so
// the selected entry never scrolls out of view in a many-key Secret.
func (m *Model) EnsureLineVisible(n int) { m.viewport.EnsureVisible(n, 0, 0) }

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

// boxSize is the bordered box's total width/height (including its border): exactly
// the area the viewer was sized to (D284), floored at zero so a degenerate size
// renders nothing rather than a negative geometry.
func (m Model) boxSize() (int, int) {
	w, h := m.width, m.height
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
// box over the browse view's *right pane* (overlayAt, D284), so the resources menu
// stays beside it while the pager replaces the pane it covers.
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
