// Package logsview is kubecom's dedicated full-screen logs viewer: a streaming
// pager built for high-throughput log output with a real-time grep filter, distinct
// from the shared read-only viewer (M3-01). YAML/describe/secret stay on the shared
// viewer; logs get their own mini-app because they stream continuously and want a
// live filter that narrows the output *while following* — à la stern / k9s logs
// (feedback 2026-07-24-logs-dedicated-view-live-grep, D134).
//
// LOGS-01 built the component — an append buffer, a live case-insensitive substring
// filter, follow/pause with auto-scroll, and a full-screen header — and LOGS-02 wired
// it up: `res.logs` now streams here instead of into the shared viewer, which the app
// no longer stamps with a logs kind (D144). The wiring lives in `internal/tui/logs.go`.
// LOGS-03 added the second grep mode: `logs.regex` switches the same field between
// case-insensitive substring and case-insensitive regex, and whichever mode is active,
// the matched spans are highlighted in the shown lines (D145). The long-line / timestamp
// nice-to-haves (LOGS-04) are a later slice.
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
	"regexp"
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

// The two grep prompts, which double as the mode indicator inside the field: the
// bare `/` is the default case-insensitive substring grep, `re/` the regex one
// (LOGS-03). The header repeats the mode as `[re]` for when the field is closed.
const (
	promptSubstring = "/ "
	promptRegex     = "re/ "
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

	// regex switches the grep from case-insensitive substring to case-insensitive
	// regex (logs.regex, LOGS-03). re holds the last query that *compiled* in that
	// mode and reBad marks that the query on screen is not it — together they are the
	// degrade: a half-typed pattern keeps narrowing by the last good one instead of
	// blanking the view, and the header says so (principle 3). re is nil when the
	// mode is off, the query is empty, or nothing has compiled yet.
	regex bool
	re    *regexp.Regexp
	reBad bool

	// matched is the number of lines the current query kept, computed by render (the
	// one place the buffer is scanned) so View never re-runs the match to label it.
	matched int

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
	fi.Prompt = promptSubstring
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
// new object always starts clean and tailing regardless of a prior session. The grep
// mode resets with it: a new object's logs open on the plain substring grep, the mode
// a reader who never touched logs.regex expects (the toggle is per-session, not sticky
// across objects).
func (m *Model) Reset() {
	m.lines = m.lines[:0]
	m.following = true
	m.closeFilter()
	m.setRegex(false)
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

// Empty reports whether no lines have streamed yet — the wiring (LOGS-02) uses it to
// decide whether a stream error closes an empty view (an open failure) or leaves the
// partial output on screen (a mid-stream drop).
func (m Model) Empty() bool { return len(m.lines) == 0 }

// Following reports whether the view is auto-scrolling with the stream.
func (m Model) Following() bool { return m.following }

// Filtering reports whether the filter field is open and capturing text. The root
// model uses it to route raw text keys to UpdateFilter while it is true (D11).
func (m Model) Filtering() bool { return m.filtering }

// Query is the current (raw) filter query, or "" when no filter is applied.
func (m Model) Query() string { return m.filter.Value() }

// Regex reports whether the grep is in regex mode (logs.regex, LOGS-03).
func (m Model) Regex() bool { return m.regex }

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
// app.filter opens the live grep; logs.regex switches that grep between substring and
// regex matching (LOGS-03); logs.follow toggles follow (re-enabling jumps to the newest
// line); nav.back closes the filter if open, else closes the view (ClosedMsg).
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
	case keymap.ActionLogsRegex:
		// Toggling re-interprets the query already typed, so the shown set changes
		// under the reader's cursor — deliberately: it is how you promote a substring
		// grep you are mid-way through into a pattern without retyping it.
		m.setRegex(!m.regex)
		m.render()
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
	m.compile()
	m.render()
	return m, cmd
}

// closeFilter clears and hides the filter field, restoring the full stream. Safe to
// call when the filter is already closed. Does not re-render (callers do). The regex
// *mode* survives (only Reset clears it); its compiled pattern does not, because the
// query it came from is gone.
func (m *Model) closeFilter() {
	if !m.filtering {
		return
	}
	m.filtering = false
	m.filter.Blur()
	m.filter.Reset()
	m.compile()
	m.resizeViewport()
}

// setRegex switches the grep mode, re-points the field's prompt at the matching
// indicator, and re-derives the compiled pattern from the query already typed.
func (m *Model) setRegex(on bool) {
	m.regex = on
	if on {
		m.filter.Prompt = promptRegex
	} else {
		m.filter.Prompt = promptSubstring
	}
	m.compile()
}

// compile re-derives the regex-mode pattern from the current query. A query that does
// not compile leaves the last good pattern in place and raises reBad, so the view keeps
// narrowing by something the reader chose rather than blanking every time a pattern is
// half-typed (`err(` on the way to `err(or)?`) — principle 3. With no last good pattern
// there is nothing to fall back to and the match set is empty, which the header labels.
// Substring mode needs no compilation, so it clears both fields.
func (m *Model) compile() {
	if !m.regex || m.filter.Value() == "" {
		m.re, m.reBad = nil, false
		return
	}
	// Case-insensitive by default (D10's spirit: the common case needs no ceremony);
	// an explicit (?-i) in the query still wins, since it is applied later.
	re, err := regexp.Compile("(?i)" + m.filter.Value())
	if err != nil {
		m.reBad = true // keep m.re: the last good pattern still narrows.
		return
	}
	m.re, m.reBad = re, false
}

// matcher is a compiled query: it reports the byte spans of a line that matched and
// whether the line matched at all. The two are separate because a line can match
// without being highlightable (see spanSubstring).
type matcher func(line string) (spans [][]int, ok bool)

// matcher builds the matcher for the current query and mode, or nil when the query
// cannot match anything at all (regex mode with a query that has never compiled). The
// caller handles the empty query before asking.
func (m Model) matcher() matcher {
	if m.regex {
		re := m.re
		if re == nil {
			return nil
		}
		return func(line string) ([][]int, bool) {
			sp := re.FindAllStringIndex(line, -1)
			return sp, len(sp) > 0
		}
	}
	q := strings.ToLower(m.filter.Value())
	return func(line string) ([][]int, bool) { return spanSubstring(line, q) }
}

// spanSubstring finds every case-insensitive occurrence of the (already lowercased)
// query in line. Offsets come from the lowercased copy, so they only index the original
// when folding preserved its byte length; for the rare fold that does not (ﬁ, İ), the
// line still matches — the match is real — but is shown unhighlighted rather than
// sliced at offsets that no longer line up.
func spanSubstring(line, lowerQuery string) ([][]int, bool) {
	low := strings.ToLower(line)
	if len(low) != len(line) {
		return nil, strings.Contains(low, lowerQuery)
	}
	var spans [][]int
	for off := 0; off <= len(low)-len(lowerQuery); {
		i := strings.Index(low[off:], lowerQuery)
		if i < 0 {
			break
		}
		spans = append(spans, []int{off + i, off + i + len(lowerQuery)})
		off += i + len(lowerQuery)
	}
	return spans, len(spans) > 0
}

// highlight paints the matched spans of line with the Match style, leaving the rest as
// it streamed. Spans arrive in order and non-overlapping (both FindAllStringIndex and
// spanSubstring guarantee it); anything out of range or zero-width is skipped so a
// pathological pattern (`x*`) can only fail to highlight, never corrupt the line.
func (m Model) highlight(line string, spans [][]int) string {
	if len(spans) == 0 {
		return line
	}
	var b strings.Builder
	last := 0
	for _, s := range spans {
		if s[0] < last || s[1] > len(line) || s[0] >= s[1] {
			continue
		}
		b.WriteString(line[last:s[0]])
		b.WriteString(m.styles.Match.Render(line[s[0]:s[1]]))
		last = s[1]
	}
	b.WriteString(line[last:])
	return b.String()
}

// shown returns the body to render — the matching lines in stream order, with their
// matched spans highlighted — and how many matched. An empty query matches everything
// and takes the untouched fast path: the whole buffer joined, no matcher built and no
// highlighting done, so the unfiltered stream (the high-throughput case) costs exactly
// what it did before LOGS-03.
func (m Model) shown() (string, int) {
	if m.filter.Value() == "" {
		return strings.Join(m.lines, "\n"), len(m.lines)
	}
	match := m.matcher()
	if match == nil {
		return "", 0
	}
	kept := make([]string, 0, len(m.lines))
	for _, l := range m.lines {
		spans, ok := match(l)
		if !ok {
			continue
		}
		kept = append(kept, m.highlight(l, spans))
	}
	return strings.Join(kept, "\n"), len(kept)
}

// render rebuilds the viewport content from the filtered buffer, keeping the newest
// line pinned while following. Called on every append, filter/mode change, and resize.
// It is the only place the buffer is scanned: the match count it records is what the
// header reports, so View costs nothing beyond drawing.
func (m *Model) render() {
	content, matched := m.shown()
	m.matched = matched
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
// plus the active query, clipped to the screen width. Regex mode adds a `[re]` marker
// (the field's own `re/` prompt is only visible while it is open), and a query that
// does not compile is called out rather than left to look like a query that simply
// matched nothing — the counts beside it are the last good pattern's (D145).
func (m Model) header() string {
	state := "[paused]"
	if m.following {
		state = "[following]"
	}
	seg := m.title + "  " + state
	if m.regex {
		seg += "  [re]"
	}
	// Show the query and matched/total whenever a filter is narrowing the stream. The
	// invalid-query flag sits *before* the counts: the header is clipped from the right
	// at narrow widths, and "the counts are not this query's" outranks the counts.
	if q := m.filter.Value(); q != "" {
		seg += "  /" + q
		if m.reBad {
			seg += "  invalid regex"
		}
		seg += "  " + itoa(m.matched) + "/" + itoa(len(m.lines))
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
