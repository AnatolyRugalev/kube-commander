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
// the matched spans are highlighted in the shown lines (D145). LOGS-04a gave long lines
// two ways to be read — `logs.wrap` soft-wraps them, and while it is off nav.left/right
// scroll the clipped view horizontally. LOGS-04c made `nav.bottom` mean "the newest line,
// and keep it newest": jumping to the end re-arms following (D147). LOGS-04b closed the
// line with `logs.timestamps`, a pure *display* toggle over stamps the stream already
// carries (D148) — no restream, and the grep still matches only the message.
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
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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

// hStep is how many columns one nav.left/nav.right shifts the view while it is
// clipping long lines (LOGS-04a). A single column would make reaching the tail of a
// stack trace a chore and a full page would lose the reader's place; eight is the
// usual pager compromise and lands on tab stops.
const hStep = 8

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

	// lines is the authoritative unfiltered log buffer (every streamed line's
	// *message*, never its timestamp). The viewport always renders the subset matching
	// the current filter query; lines is the source it is rebuilt from, so clearing the
	// filter restores the full stream without re-fetching (mirrors the picker's `all`
	// set).
	//
	// stamps holds each line's server timestamp, parallel to lines and empty where the
	// server sent none. Keeping it *beside* the message rather than prefixed into it is
	// what makes the timestamps toggle free (LOGS-04b): with timestamps off, render
	// joins exactly the bytes it joined before this existed, so the high-throughput
	// default costs nothing; and the grep matches the message in either state, so a
	// query can never be satisfied by the clock.
	lines  []string
	stamps []string

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

	// wrap switches long lines between soft-wrapped continuation rows and clipping
	// at the right edge (logs.wrap, LOGS-04a). It is the viewport's own SoftWrap;
	// the field mirrors it so View/header can read it off the value model without
	// reaching into the viewport. While clipping, nav.left/nav.right move the
	// viewport's horizontal offset; wrapping makes that offset meaningless (the
	// viewport ignores it), which is why one toggle covers both modes.
	wrap bool

	// timestamps shows each line's server timestamp ahead of its message
	// (logs.timestamps, LOGS-04b). Display only: the stream always requests timestamps
	// (for a followed stream the kube layer forces them on the wire anyway, to anchor
	// its reconnect), so toggling costs no restream and loses no buffer — the reader
	// keeps their scroll position, their grep and every line already streamed.
	timestamps bool

	// previous marks that the lines on screen are the container's *previous*
	// terminated instance rather than the running one (logs.previous, M5-01a). Unlike
	// every other flag here it is not a mode this view implements — the app sets it
	// when it opens the stream with kube.LogOptions.Previous — it is held only so the
	// header can name it. It has to be named: two instances of one container produce
	// output that looks alike, so without the marker a reader cannot tell an
	// explanation of the crash from the aftermath of it (D146 — state the reader can
	// lose sight of).
	previous bool

	// shownLines is the rendered body: the subset of lines the current query keeps,
	// each already timestamp-prefixed and highlight-painted, in stream order. It is
	// the cache that makes appending a line cost a line (LOGS-05b): before it, every
	// Append re-scanned the whole buffer and re-joined it, so streaming n lines cost
	// O(n²) and a fast pod made the view — and the keys — progressively slower.
	//
	// The invariant is that shownLines always equals what a full rebuild would
	// produce: rebuildShown recomputes it whenever the query, the grep mode or the
	// timestamps toggle changes what a line looks like, and appendLine only ever
	// extends it, through the same renderLine both use. Its length is the match
	// count the header reports, so View still never re-runs the match to label it.
	shownLines []string

	active bool // whether the view is shown (captures input) — "" View when false
	width  int  // full screen width
	height int  // full screen height
}

// New builds a logs view rendered through the shared styles. It starts hidden, empty,
// following (a fresh logs open tails the stream), with the filter closed and long lines
// clipped rather than wrapped — one log line stays one screen row until asked otherwise,
// which is what keeps a fast stream readable (logs.wrap opts in). The
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

// SetStyles repaints the view through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
//
// This one has to re-render, not just assign: shownLines is a *painted* cache — each
// kept line already carries the previous theme's Match escape sequences around its
// matched spans (LOGS-05b) — so a restyle that only swapped m.styles would leave every
// highlight already on screen in the old theme's colors, and only lines streamed
// afterwards would follow the new one. rebuildShown is the same O(n) path a filter or
// timestamps change takes, and a restyle is exactly that class of event: a reader
// gesture, not a stream event, so it never costs an append anything. The buffer, the
// query, every mode and the scroll position survive it, and following stays as it was
// (syncContent only pins the newest line if it was already following).
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.render()
}

// Kind returns the view's kind id (always "logs").
func (m Model) Kind() string { return kind }

// SetTitle sets the object label shown in the header.
func (m *Model) SetTitle(t string) { m.title = t }

// SetPrevious records whether the stream now feeding the view is the container's
// previous terminated instance (logs.previous, M5-01a), which the header names. The app
// sets it with each open, since which instance is on screen is a property of the
// request, not a mode this view can flip on its own.
func (m *Model) SetPrevious(p bool) { m.previous = p }

// Reset clears the buffer and filter and re-arms following, so opening the view over a
// new object always starts clean and tailing regardless of a prior session. The grep
// mode, the wrap mode and the timestamps toggle reset with it: a new object's logs open
// on the plain substring grep, unwrapped, unscrolled and unstamped — the state a reader
// who never touched logs.regex, logs.wrap or logs.timestamps expects (all three are
// per-session, not sticky across objects).
func (m *Model) Reset() {
	m.timestamps = false
	m.previous = false
	m.closeFilter()
	m.setRegex(false)
	m.setWrap(false)
	m.Restream()
}

// Restream clears the buffer for a re-open of the *same* container's other instance
// (logs.previous, M5-01a) and keeps the reader's lens on it: the grep query and its
// mode, wrapping and the timestamps toggle all survive, so flipping to the instance
// that died answers "is the same thing in that log?" without retyping the query. Only
// the lines go, and following is re-armed because the new stream tails from its own
// start — the same reason a fresh open starts following.
//
// It is Reset minus the lens, and Reset is written in terms of it, so the two can never
// disagree about what emptying the buffer means.
func (m *Model) Restream() {
	m.lines = m.lines[:0]
	m.stamps = m.stamps[:0]
	m.shownLines = m.shownLines[:0]
	m.following = true
	m.render()
}

// Line is one streamed log line as the view holds it: the Message the grep matches and
// always draws, and the server Stamp ("" when the server sent none) shown only while
// logs.timestamps is on. Keeping the two apart is what makes that toggle a redraw rather
// than a restream (LOGS-04b/D148) and stops a query being satisfied by the clock.
type Line struct {
	Stamp   string
	Message string
}

// Append adds one streamed log line to the buffer and re-renders the filtered view. If
// following, the viewport is pinned to the newest line so the stream tails; if paused
// (the reader scrolled up), the scroll position is left undisturbed.
func (m *Model) Append(stamp, line string) {
	match, filtered := m.shownFilter()
	m.appendLine(match, filtered, stamp, line)
	m.syncContent()
}

// AppendBatch adds a run of streamed log lines in one go — the streaming entry point the
// log pump (D53) feeds. Batching is half of what keeps a fast stream cheap (LOGS-05b):
// the per-line work is O(1) either way now, but handing the body to the viewport is not
// (it re-measures every line to find the longest), so a burst of a thousand lines costs
// one such pass instead of a thousand. The batch is applied atomically — nothing is shown
// until all of it is — which is also what the reader wants: a frame is a frame.
func (m *Model) AppendBatch(batch []Line) {
	if len(batch) == 0 {
		return
	}
	// The matcher is built once for the whole batch rather than per line: in substring
	// mode building it lowercases the query, which is per-query work, not per-line work.
	match, filtered := m.shownFilter()
	for _, l := range batch {
		m.appendLine(match, filtered, l.Stamp, l.Message)
	}
	m.syncContent()
}

// appendLine buffers one line and extends the rendered body if the current query keeps
// it. It does no scanning of the lines already held — that is the whole point — so a
// filtered stream where most lines miss costs nothing but the match itself.
func (m *Model) appendLine(match matcher, filtered bool, stamp, line string) {
	m.lines = append(m.lines, line)
	m.stamps = append(m.stamps, stamp)
	if s, ok := m.renderLine(len(m.lines)-1, match, filtered); ok {
		m.shownLines = append(m.shownLines, s)
	}
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

// Wrap reports whether long lines are soft-wrapped rather than clipped (logs.wrap,
// LOGS-04a). HOffset is the current horizontal scroll position in columns, always 0
// while wrapping.
func (m Model) Wrap() bool   { return m.wrap }
func (m Model) HOffset() int { return m.viewport.XOffset() }

// Timestamps reports whether each line's server timestamp is shown ahead of its message
// (logs.timestamps, LOGS-04b).
func (m Model) Timestamps() bool { return m.timestamps }

// Previous reports whether the lines on screen are the container's previous terminated
// instance rather than its running one (logs.previous, M5-01a).
func (m Model) Previous() bool { return m.previous }

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
// nav.bottom is the inverse gesture: an explicit jump to the end *resumes* following, so
// a reader who scrolled back has one key that catches up and keeps tailing (LOGS-04c).
// app.filter opens the live grep; logs.regex switches that grep between substring and
// regex matching (LOGS-03); logs.wrap switches long lines between soft-wrapped and
// clipped, and while clipped nav.left/nav.right scroll horizontally to the tail of a
// long line (LOGS-04a); logs.timestamps shows or hides each line's server timestamp
// without touching the stream (LOGS-04b); logs.follow toggles follow (re-enabling jumps to the newest
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
		// The one "catch up and keep tailing" gesture (LOGS-04c, D147). In a streaming
		// pager the bottom is not a position: the newest line keeps moving, so a jump to
		// the end that did not rejoin the stream would be true for exactly one frame and
		// then drift upward as lines arrived. This is the exact inverse of the rule above
		// — any *upward* movement pauses following, an explicit jump to the end resumes
		// it. Incremental downward movement (nav.down, page down) deliberately does not:
		// stepping onto the last line is browsing, not a statement about the tail, and a
		// reader parked at the end of a paused view must be able to stay there.
		m.following = true
		m.viewport.GotoBottom()
	case keymap.ActionLeft:
		// Horizontal movement says nothing about whether the reader still wants the
		// tail, so unlike an upward scroll it leaves following alone. Inert while
		// wrapping — there is nothing off-screen to scroll to (the viewport ignores
		// the offset), which is exactly why the wrap toggle covers both modes.
		m.viewport.ScrollLeft(hStep)
	case keymap.ActionRight:
		m.viewport.ScrollRight(hStep)
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
	case keymap.ActionLogsWrap:
		// Wrapping changes how many display rows the buffer occupies, so a followed
		// view has to be re-pinned to the newest line — render does that.
		m.setWrap(!m.wrap)
		m.render()
	case keymap.ActionLogsTimestamps:
		// Display only — the timestamps are already in the buffer (LOGS-04b), so this
		// re-renders what is on screen rather than re-requesting the stream: nothing is
		// re-fetched, no line is lost, and the grep and scroll position survive. It does
		// widen every line by the stamp, which is why render re-clamps the horizontal
		// offset and re-pins a followed view.
		m.timestamps = !m.timestamps
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

// setWrap switches long-line handling between soft wrap and clip-and-scroll. Enabling
// wrap zeroes the horizontal offset *first*: the viewport ignores SetXOffset once
// SoftWrap is on, so an offset left behind would silently reappear the moment wrapping
// was switched back off, scrolling a view the reader never scrolled.
func (m *Model) setWrap(on bool) {
	if on {
		m.viewport.SetXOffset(0)
	}
	m.wrap = on
	m.viewport.SoftWrap = on
}

// clampHOffset keeps the horizontal offset inside the content after the content or the
// width changed — a filter that hides the one very long line, or a wider terminal, both
// shrink how far right there is to go. SetXOffset does the clamping; re-setting the
// current value is how you ask for it. A no-op while wrapping (no offset to clamp).
func (m *Model) clampHOffset() {
	if m.wrap {
		return
	}
	m.viewport.SetXOffset(m.viewport.XOffset())
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

// stamp is line i's rendered timestamp prefix, or "" when timestamps are off or the
// server sent none for that line. Muted, because the timestamp is context for the
// message and should not compete with it for the eye. An unstamped line is simply not
// padded: aligning it under its neighbours would mean inventing a timestamp it does not
// have, and in practice a stream is either wholly stamped or wholly not.
func (m Model) stamp(i int) string {
	if !m.timestamps || i >= len(m.stamps) || m.stamps[i] == "" {
		return ""
	}
	return m.styles.Subtle.Render(m.stamps[i]) + " "
}

// shownFilter returns the matcher for the current query and whether a query is narrowing
// the stream at all. The two are separate because a nil matcher is meaningful: with no
// query nothing is filtered and every line is kept without a matcher being built (the
// high-throughput default costs no match work), whereas a query in regex mode that has
// never compiled yields a nil matcher that keeps *nothing* — the header labels it.
func (m Model) shownFilter() (matcher, bool) {
	if m.filter.Value() == "" {
		return nil, false
	}
	return m.matcher(), true
}

// renderLine renders buffer line i as it appears in the body — its matched spans
// highlighted and, while logs.timestamps is on, its server timestamp ahead of it — and
// reports whether the current query keeps it. It is the single definition of what a shown
// line looks like: both the full rebuild and the incremental append go through it, so the
// cached body cannot drift from a rebuilt one. With no query and no timestamps it returns
// the streamed line untouched.
//
// The matcher only ever sees the message: a timestamp is not something the reader typed a
// query about, and letting it match would mean the same query narrowed differently
// depending on whether the clock happened to be on screen.
func (m Model) renderLine(i int, match matcher, filtered bool) (string, bool) {
	line := m.lines[i]
	if !filtered {
		return m.stamp(i) + line, true
	}
	if match == nil {
		return "", false
	}
	spans, ok := match(line)
	if !ok {
		return "", false
	}
	return m.stamp(i) + m.highlight(line, spans), true
}

// rebuildShown recomputes the whole rendered body from the buffer. It is the O(n) path,
// and the only one: it runs when the query, the grep mode or the timestamps toggle
// changes what every line looks like — reader gestures, not stream events — never on
// append.
func (m *Model) rebuildShown() {
	match, filtered := m.shownFilter()
	kept := m.shownLines[:0]
	for i := range m.lines {
		if s, ok := m.renderLine(i, match, filtered); ok {
			kept = append(kept, s)
		}
	}
	m.shownLines = kept
}

// syncContent hands the rendered body to the viewport and keeps the newest line pinned
// while following. The body is cloned because the viewport takes ownership of the slice
// it is given (it normalizes embedded line endings in place and splits them out), and
// this one is the cache every later append extends. Cloning copies string headers, not
// the log text — far cheaper than the join-and-re-split SetContent would do.
func (m *Model) syncContent() {
	m.viewport.SetContentLines(slices.Clone(m.shownLines))
	m.clampHOffset()
	if m.following {
		m.viewport.GotoBottom()
	}
}

// render rebuilds the body from the buffer and shows it. Called whenever something other
// than an appended line changed what the view should show (filter/mode change, resize,
// reset); appends take the incremental path instead.
func (m *Model) render() {
	m.rebuildShown()
	m.syncContent()
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
// plus the active query, clipped to the screen width. Wrapping adds a `[wrap]` marker and
// a horizontally scrolled clip adds `[+N]` (columns hidden to the left, LOGS-04a) —
// without it a view scrolled past the start of every line looks like a view of blank
// lines. The timestamps toggle deliberately gets **no** marker: unlike wrap (which looks
// identical to clipping until some line is wider than the screen) a timestamp is on every
// row the moment it is on, so a marker would restate the body and cost a segment of a
// header that is already clipped from the right at narrow widths (D146 names state the
// reader can *lose sight of*; this is not that). Regex mode adds a `[re]` marker
// (the field's own `re/` prompt is only visible while it is open), and a query that
// does not compile is called out rather than left to look like a query that simply
// matched nothing — the counts beside it are the last good pattern's (D145).
//
// The previous-instance marker (M5-01a) is the one piece of state that sits *before* the
// follow state, because it is the only one that changes what the lines below mean rather
// than how they are shown: a header clipped to a narrow terminal may lose `[following]`
// or the counts without misleading anyone, but losing `[previous]` turns a dead
// instance's log into what looks like the running one's.
func (m Model) header() string {
	state := "[paused]"
	if m.following {
		state = "[following]"
	}
	seg := m.title
	if m.previous {
		seg += "  [previous]"
	}
	seg += "  " + state
	// Long-line state (LOGS-04a): wrapping is a mode the reader turned on, so it is
	// always named; clipping is the default and only worth a marker once it is actually
	// hiding something to the left — the column offset doubles as "you are scrolled".
	if m.wrap {
		seg += "  [wrap]"
	} else if x := m.viewport.XOffset(); x > 0 {
		seg += "  [+" + itoa(x) + "]"
	}
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
		seg += "  " + itoa(len(m.shownLines)) + "/" + itoa(len(m.lines))
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
