// Package logsview is kubecom's dedicated full-screen logs viewer: a streaming
// pager built for high-throughput log output with a real-time grep filter, distinct
// from the shared read-only viewer (M3-01). YAML/describe/secret stay on the shared
// viewer; logs get their own mini-app because they stream continuously and want a
// live filter that narrows the output *while following* — à la stern / k9s logs
// (feedback 2026-07-24-logs-dedicated-view-live-grep, D134).
//
// This slice (LOGS-01) is the component in isolation — an append buffer, a live
// case-insensitive substring filter, follow/pause with auto-scroll, and a full-screen
// header. It is not wired to the app yet (LOGS-02 retires the shared-viewer logs path
// and streams into this instead). Regex + match highlighting (LOGS-03) and the
// long-line / timestamp nice-to-haves (LOGS-04) are later slices.
//
// Like the shared viewer and the picker it wraps a bubbles component (viewport +
// textinput) but drives it entirely through keymap.Actions — it never matches a raw
// key for behaviour (D11): the root model resolves a KeyMsg to an Action and hands the
// Action to Update; the one exception, UpdateFilter, receives raw text only while the
// filter field is open (exactly as the picker does). It emits its own message type
// (ClosedMsg) so it never imports the root package (D56), holds no shared mutable state
// (principle 1), and — unlike the centered shared viewer — renders a **full-screen**
// block (logs want every row), which the root composites as the base while it is up.
package logsview

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// kind is the fixed ClosedMsg kind for this view (there is only ever one logs view).
const kind = "logs"

// headerHeight is the one status line above the scrolling body; filterHeight is the
// filter input line, shown only while the filter is open.
const (
	headerHeight = 1
	filterHeight = 1
)

// ClosedMsg is emitted when the user dismisses the logs view (nav.back with the filter
// already closed). Owned by this package — the emitter — so the root model handles the
// concrete type without this package importing it (D56). Kind mirrors the shared
// viewer's ClosedMsg shape so the root can route both uniformly.
type ClosedMsg struct {
	Kind string
}

// Model is the full-screen logs view. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles   styles.Styles
	viewport viewport.Model
	filter   textinput.Model // the live grep field (shown only while filtering)
	title    string          // object label shown in the header (e.g. "pod/api")

	// lines is the authoritative unfiltered log buffer (every streamed line). The
	// viewport always renders the subset matching the current filter query; lines is
	// the source it is rebuilt from, so clearing the filter restores the full stream
	// without re-fetching (mirrors the picker's `all` set).
	lines []string

	following bool // auto-scroll to the newest line as it streams (toggled by logs.follow)
	filtering bool // whether the filter field is open and capturing text

	active bool // whether the view is shown (captures input) — "" View when false
	width  int  // full screen width
	height int  // full screen height
}

// New builds a logs view rendered through the shared styles. It starts hidden, empty,
// and following (a fresh logs open tails the stream), with the filter closed. The
// viewport's mouse-wheel scroll is disabled so all input flows through keymap actions
// (D11) — the root model never forwards a raw KeyMsg to it.
func New(s styles.Styles) Model {
	vp := viewport.New()
	vp.MouseWheelEnabled = false
	fi := textinput.New()
	fi.Prompt = "/ "
	return Model{
		styles:    s,
		viewport:  vp,
		filter:    fi,
		title:     kind,
		following: true,
	}
}

// Kind returns the view's kind id (always "logs").
func (m Model) Kind() string { return kind }

// SetTitle sets the object label shown in the header.
func (m *Model) SetTitle(t string) { m.title = t }

// Reset clears the buffer and filter and re-arms following, so opening the view over a
// new object always starts clean and tailing regardless of a prior session.
func (m *Model) Reset() {
	m.lines = m.lines[:0]
	m.following = true
	m.closeFilter()
	m.render()
}

// Append adds one streamed log line to the buffer and re-renders the filtered view. If
// following, the viewport is pinned to the newest line so the stream tails; if paused
// (the reader scrolled up), the scroll position is left undisturbed. This is the
// streaming entry point the log pump (D53) feeds line by line.
func (m *Model) Append(line string) {
	m.lines = append(m.lines, line)
	m.render()
}

// Empty reports whether no lines have streamed yet — used by the wiring (LOGS-02) to
// decide whether a terminal open failure closes an empty view or leaves partial output.
func (m Model) Empty() bool { return len(m.lines) == 0 }

// Following reports whether the view is auto-scrolling with the stream.
func (m Model) Following() bool { return m.following }

// Filtering reports whether the filter field is open and capturing text. The root
// model uses it to route raw text keys to UpdateFilter while it is true (D11).
func (m Model) Filtering() bool { return m.filtering }

// Query is the current (raw) filter query, or "" when no filter is applied.
func (m Model) Query() string { return m.filter.Value() }

// Show reveals the view (it then captures input until Hide). Hide dismisses it and
// closes any open filter so it reopens clean next time.
func (m *Model) Show() { m.active = true }
func (m *Model) Hide() {
	m.active = false
	m.closeFilter()
}

// Active reports whether the view is shown and capturing input.
func (m Model) Active() bool { return m.active }

// SetSize records the full screen size and sizes the inner viewport (the whole screen
// minus the header and, while open, the filter line).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	iw, ih := m.innerSize()
	m.viewport.SetWidth(iw)
	m.viewport.SetHeight(ih)
	m.filter.SetWidth(iw)
	m.render()
}

// Update handles a resolved keymap action while the view is active. Navigation scrolls
// the viewport (vim nav + gg/G + half/full page, D10); any *upward* scroll pauses
// following so the reader can look back without the stream yanking them to the bottom
// (mirrors the shared viewer's M3-06 follow semantics, now inside the component).
// app.filter opens the live grep; logs.follow toggles follow (re-enabling jumps to the
// newest line); nav.back closes the filter if open, else closes the view (ClosedMsg).
// The view consumes actions, never raw keys (D11); an inactive view ignores everything.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.following = false
		m.viewport.ScrollUp(1)
	case keymap.ActionHalfPageUp:
		m.following = false
		m.viewport.HalfPageUp()
	case keymap.ActionPageUp:
		m.following = false
		m.viewport.PageUp()
	case keymap.ActionTop:
		m.following = false
		m.viewport.GotoTop()
	case keymap.ActionDown:
		m.viewport.ScrollDown(1)
	case keymap.ActionHalfPageDown:
		m.viewport.HalfPageDown()
	case keymap.ActionPageDown:
		m.viewport.PageDown()
	case keymap.ActionBottom:
		m.viewport.GotoBottom()
	case keymap.ActionFilter:
		if m.filtering {
			return m, nil
		}
		m.filtering = true
		cmd := m.filter.Focus()
		m.resizeViewport()
		m.render()
		return m, cmd
	case keymap.ActionLogsFollow:
		m.following = !m.following
		if m.following {
			m.viewport.GotoBottom()
		}
	case keymap.ActionBack:
		// One esc clears an open filter (restoring the full stream); a second closes
		// the view — the filter must never be lost by the same key that dismisses.
		if m.filtering {
			m.closeFilter()
			m.render()
			return m, nil
		}
		return m, func() tea.Msg { return ClosedMsg{Kind: kind} }
	}
	return m, nil
}

// UpdateFilter feeds one raw key to the filter field and re-narrows the shown lines
// live. It is the view's only raw-key entry point and is meaningful only while the
// filter is open: the root model resolves control keys (nav, follow, back) to actions
// and routes the remaining text content here, so the field captures typing without any
// view matching a raw key for behaviour (D11). A no-op otherwise.
func (m Model) UpdateFilter(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.active || !m.filtering {
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.render()
	return m, cmd
}

// closeFilter clears and hides the filter field, restoring the full stream. Safe to
// call when the filter is already closed. Does not re-render (callers do).
func (m *Model) closeFilter() {
	if !m.filtering {
		return
	}
	m.filtering = false
	m.filter.Blur()
	m.filter.Reset()
	m.resizeViewport()
}

// shown returns the filtered lines (case-insensitive substring; LOGS-03 upgrades this
// to optional regex) and how many matched, in stream order. An empty query matches all.
func (m Model) shown() (string, int) {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return strings.Join(m.lines, "\n"), len(m.lines)
	}
	kept := make([]string, 0, len(m.lines))
	for _, l := range m.lines {
		if strings.Contains(strings.ToLower(l), q) {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n"), len(kept)
}

// render rebuilds the viewport content from the filtered buffer, keeping the newest
// line pinned while following. Called on every append, filter change, and resize.
func (m *Model) render() {
	content, _ := m.shown()
	m.viewport.SetContent(content)
	if m.following {
		m.viewport.GotoBottom()
	}
}

// resizeViewport re-applies the inner content height, which shrinks by one row while
// the filter line is shown. Width is unaffected by the filter.
func (m *Model) resizeViewport() {
	_, ih := m.innerSize()
	m.viewport.SetHeight(ih)
}

// innerSize is the viewport's content size: the full screen minus the header line and,
// while the filter is open, the filter line. Full width (logs are full-screen, no
// border) so long lines have every column.
func (m Model) innerSize() (int, int) {
	iw := m.width
	ih := m.height - headerHeight
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

// View renders the full-screen logs view: a header line (object label · follow state ·
// active-filter + match count), the filter input while open, then the scrolling body.
// Returns "" when hidden or unsized. Full-screen (no border) — the root composites it
// as the base while it is up, not as a centered overlay (D134).
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	parts := []string{m.header()}
	if m.filtering {
		parts = append(parts, m.styles.App.Width(m.width).MaxWidth(m.width).Render(m.filter.View()))
	}
	parts = append(parts, m.viewport.View())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// header builds the one-line status bar: "<title>  [following]/[paused]  <matched/total>"
// plus the active query, clipped to the screen width.
func (m Model) header() string {
	state := "[paused]"
	if m.following {
		state = "[following]"
	}
	seg := m.title + "  " + state
	// Show the query and matched/total whenever a filter is narrowing the stream.
	if q := m.filter.Value(); q != "" {
		_, matched := m.shown()
		seg += "  /" + q + "  " + itoa(matched) + "/" + itoa(len(m.lines))
	}
	return m.styles.Header.Width(m.width).MaxWidth(m.width).Render(clip(seg, m.width))
}

// itoa is a tiny non-negative int→string (avoids importing strconv for one use).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// clip truncates s to at most w display cells so a long header never overflows.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ")
}
