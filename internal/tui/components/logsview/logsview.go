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

// MaxLines bounds the log buffer: a followed stream drops its oldest lines rather
// than growing for as long as the view is open (LOGS-07). Nothing else bounds it —
// the wiring's TailLines bounds the *replay* that precedes the tail, and LOGS-05b
// bounds what a line costs, not how many are held — so a chatty container measured at
// ~1,900 lines/sec would otherwise grow `lines`, `stamps` and `shownLines` without
// end (D230).
//
// Ten thousand, not the thousand-line replay: the cap must exceed that replay or the
// first live line would start discarding history the reader just asked for and
// scrolled into (D245). Ten thousand lines of typical output is a few MB held twice
// over (the raw buffer and its painted cache) and still a full pager's worth of
// scroll-back.
//
// trimChunk is the slack the buffer runs past the cap before a trim, which is what
// makes the trim amortized: dropping a prefix is O(held), so trimming on every line
// past the cap would make a fast stream quadratic. One trim per trimChunk lines makes
// it O(1) per line, at the cost of holding at most MaxLines+trimChunk.
const (
	MaxLines  = 10000
	trimChunk = 1000
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
	//
	// Both are bounded at MaxLines: past it the oldest lines are dropped (trim), so a
	// stream that never ends does not grow forever. trimmed records that this has
	// happened at least once — the top of the buffer is then no longer the top of the
	// stream, which is a thing the reader standing on it must be told (the header says
	// so), and it is never unset short of a Reset.
	lines   []string
	stamps  []string
	trimmed bool

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
	//
	// It is deliberately **not** where the cursor is painted: the selection bar is
	// applied to a copy on its way to the viewport (syncContent), so the cache stays
	// the thing a rebuild would produce and moving the cursor never invalidates it.
	shownLines []string

	// shownIdx maps each shown line back to its index in lines/stamps, parallel to
	// shownLines and ascending. The cursor addresses *shown* lines — that is what
	// j/k step through — but everything a reader does with the line it lands on needs
	// the raw text: the cursor's own repaint re-runs the matcher against lines[i], and
	// the yank LOGS-SEL-02 adds must reach lines[i] rather than the painted cache, or
	// the clipboard gets escape sequences (D242 pt 4). It is also what lets the cursor
	// survive a query change: rebuildShown remembers which *log line* it was on and
	// re-finds it, instead of leaving an index pointing at whatever the new query put
	// in that slot.
	shownIdx []int

	// cursor is the index into shownLines of the highlighted line — the reader's
	// "this line", and the anchor a later visual mode extends from. -1 exactly when
	// the body is empty. It counts **log lines, not screen rows**: a soft-wrapped line
	// is one cursor step however many rows it occupies, which is the property the
	// feedback singled out as the reason terminal select-to-copy is not good enough
	// (2026-08-07-logs-selection-and-yank, D242 pt 1).
	cursor int

	// anchor is the fixed end of a visual-mode selection — the shown line logs.select
	// was pressed on — and -1 exactly when there is no selection (LOGS-SEL-02). The
	// moving end is the cursor, so the selected range is the closed interval between
	// them and every nav key extends it for free. Like the cursor it counts log lines
	// (D242 pt 1) and addresses the *shown* set, so it is carried across a query change
	// the same way: rebuildShown remembers which buffer line it was and re-finds it.
	anchor int

	// resumeFollow records that entering visual mode is what paused following, so
	// leaving it can put the stream back the way the reader had it. Following owns the
	// cursor (D242 pt 5) — it pins it to the newest line — so a selection cannot be
	// held while the stream is running; but a reader who was tailing, selected three
	// lines and yanked them meant to go on tailing, and having to press `f` afterwards
	// would make the copy cost them their place. False when the view was already paused
	// before `v`: that pause is the reader's, and visual mode does not get to undo it.
	resumeFollow bool

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
		cursor:    -1,
		anchor:    -1,
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
	m.shownIdx = m.shownIdx[:0]
	m.cursor = -1
	m.anchor = -1
	m.trimmed = false
	m.resumeFollow = false
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
	m.trim()
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
	m.trim()
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
		m.shownIdx = append(m.shownIdx, len(m.lines)-1)
	}
}

// trim drops the oldest lines once the buffer has run trimChunk past MaxLines, taking
// the buffer back to exactly MaxLines. It is called after an append and before the
// content is handed to the viewport, so nothing ever sees a half-trimmed model.
//
// The care is all in what addresses a line by index. `shownIdx` holds buffer indices,
// so every survivor shifts down by the number dropped and the entries that pointed
// *into* the dropped prefix leave with it — which in turn moves the cursor and the
// selection anchor, both of which count shown lines. And the viewport's own offset is
// rows from the top of the body: dropping rows off that top would slide the body up
// under a paused reader, so the offset is pulled down by exactly the rows that left
// and the reader keeps looking at the lines they were looking at. (A following reader
// is pinned to the newest line by syncContent, so none of the scroll arithmetic
// applies — but the index arithmetic still does.)
//
// slices.Delete, not a reslice: a reslice would keep the dropped strings alive through
// the backing array until it next grew, which is exactly the unbounded growth this
// exists to stop.
func (m *Model) trim() {
	if len(m.lines) < MaxLines+trimChunk {
		return
	}
	drop := len(m.lines) - MaxLines
	m.lines = slices.Delete(m.lines, 0, drop)
	m.stamps = slices.Delete(m.stamps, 0, drop)
	m.trimmed = true

	// shownIdx is ascending, so the first survivor is the insertion point of drop.
	cut, _ := slices.BinarySearch(m.shownIdx, drop)
	if cut > 0 {
		if !m.following {
			rows := cut
			if m.wrap {
				rows = 0
				w := m.viewport.Width()
				for i := range cut {
					rows += lineRows(m.shownLines[i], w)
				}
			}
			m.viewport.SetYOffset(max(0, m.viewport.YOffset()-rows))
		}
		m.shownLines = slices.Delete(m.shownLines, 0, cut)
		m.shownIdx = slices.Delete(m.shownIdx, 0, cut)
		// Both ends of the reader's range are clamped, not dropped: a selection whose
		// far end has streamed off the top narrows to what survives, the same answer
		// rebuildShown gives when a grep hides one of its lines.
		m.cursor = max(m.cursor-cut, 0)
		if m.anchor >= 0 {
			m.anchor = max(m.anchor-cut, 0)
		}
	}
	for i := range m.shownIdx {
		m.shownIdx[i] -= drop
	}
}

// Empty reports whether no lines have streamed yet — the wiring (LOGS-02) uses it to
// decide whether a stream error closes an empty view (an open failure) or leaves the
// partial output on screen (a mid-stream drop).
func (m Model) Empty() bool { return len(m.lines) == 0 }

// Following reports whether the view is auto-scrolling with the stream.
func (m Model) Following() bool { return m.following }

// Cursor is the index into the *shown* lines of the highlighted line, or -1 when the
// body is empty. Shown, not buffered: with a `/` query active the cursor walks the
// lines the query keeps, so what it addresses is always what is on screen.
func (m Model) Cursor() int { return m.cursor }

// Selecting reports whether visual mode is on (logs.select, LOGS-SEL-02).
func (m Model) Selecting() bool { return m.anchor >= 0 }

// Selection is the inclusive range of *shown* lines a yank would copy: the visual-mode
// range while one is being made, and the cursor's own line otherwise — which is what
// makes `y` useful without `v` first. ok is false only when there is nothing to copy
// (an empty body).
func (m Model) Selection() (lo, hi int, ok bool) {
	if m.cursor < 0 || m.cursor >= len(m.shownLines) {
		return 0, 0, false
	}
	if !m.Selecting() || m.anchor >= len(m.shownLines) {
		return m.cursor, m.cursor, true
	}
	return min(m.anchor, m.cursor), max(m.anchor, m.cursor), true
}

// startSelect anchors a selection at the cursor and suspends following for its
// duration. The suspend is not a courtesy: while following, placeCursor pins the cursor
// to the newest line (D242 pt 5), so a selection made under a live stream would have one
// end dragged along by every arriving line and the reader would be selecting the tail
// rather than what they are looking at. A view with nothing in it has nothing to anchor.
func (m *Model) startSelect() {
	if len(m.shownLines) == 0 {
		return
	}
	m.anchor = max(m.cursor, 0)
	m.resumeFollow = m.following
	m.following = false
}

// endSelect leaves visual mode. resume asks for the follow state visual mode suspended
// to be restored — true for the ways of *finishing* (esc, a second `v`, a yank), false
// for logs.follow, which is about to state the reader's own intent and must not be
// pre-empted by ours.
func (m *Model) endSelect(resume bool) {
	if !m.Selecting() {
		return
	}
	m.anchor = -1
	if resume && m.resumeFollow {
		m.following = true
	}
	m.resumeFollow = false
}

// Yank returns the text the reader asked for — the selected lines, or the cursor's line
// with no selection — and leaves visual mode. What it returns is built from `lines`, the
// raw buffer, never from the painted `shownLines` cache: the clipboard must carry no
// escape sequence, and a soft-wrapped line must come back whole rather than broken at
// the column the screen happened to fold it (D242 pt 1/pt 4 — the two defects that made
// the terminal's own select-to-copy insufficient in the first place). The timestamp is
// prefixed exactly when logs.timestamps is showing it, so the copy matches the screen.
// ok is false when there is nothing to copy.
func (m *Model) Yank() (text string, lines int, ok bool) {
	lo, hi, ok := m.Selection()
	if !ok {
		return "", 0, false
	}
	var b strings.Builder
	for n := lo; n <= hi; n++ {
		if n > lo {
			b.WriteByte('\n')
		}
		i := m.shownIdx[n]
		if m.timestamps && i < len(m.stamps) && m.stamps[i] != "" {
			b.WriteString(m.stamps[i])
			b.WriteByte(' ')
		}
		b.WriteString(m.lines[i])
	}
	m.endSelect(true)
	m.syncContent()
	return b.String(), hi - lo + 1, true
}

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
// line); logs.select starts or abandons a visual selection the nav keys then extend, and
// suspends following while it stands (LOGS-SEL-02); nav.back clears the filter if open,
// else abandons a selection, else closes the view (ClosedMsg).
//
// logs.yank is deliberately not an action this Update handles: the clipboard write and the
// status-bar confirmation belong to the shell (M3-08b), which calls Yank for the text.
// The view consumes actions, never raw keys (D11); an inactive view ignores everything.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.following = false
		m.moveCursor(-1)
	case keymap.ActionHalfPageUp:
		m.following = false
		m.moveCursor(-m.page(2))
	case keymap.ActionPageUp:
		m.following = false
		m.moveCursor(-m.page(1))
	case keymap.ActionTop:
		m.following = false
		m.moveCursor(-len(m.shownLines))
	case keymap.ActionDown:
		m.moveCursor(1)
	case keymap.ActionHalfPageDown:
		m.moveCursor(m.page(2))
	case keymap.ActionPageDown:
		m.moveCursor(m.page(1))
	case keymap.ActionBottom:
		if m.Selecting() {
			// vim's visual-mode `G`: extend the selection to the last line. It must not
			// re-arm following the way the same key does outside visual mode — that
			// would hand the cursor to the stream (D242 pt 5) and with it the moving end
			// of the selection. This is also what makes `gg v G y` — copy the whole
			// buffer — the gesture it looks like.
			m.moveCursor(len(m.shownLines))
			return m, nil
		}
		// The one "catch up and keep tailing" gesture (LOGS-04c, D147). In a streaming
		// pager the bottom is not a position: the newest line keeps moving, so a jump to
		// the end that did not rejoin the stream would be true for exactly one frame and
		// then drift upward as lines arrived. This is the exact inverse of the rule above
		// — any *upward* movement pauses following, an explicit jump to the end resumes
		// it. Incremental downward movement (nav.down, page down) deliberately does not:
		// stepping onto the last line is browsing, not a statement about the tail, and a
		// reader parked at the end of a paused view must be able to stay there.
		// It moves the cursor too: re-arming follow pins the cursor to the newest line
		// (syncContent does it), so `G` lands the reader and their cursor in the same
		// place, which is where the next line will arrive.
		m.following = true
		m.syncContent()
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
	case keymap.ActionLogsSelect:
		// A toggle, like vim's own `v`: pressed inside a selection it abandons it,
		// which is the second way out beside esc and the one a reader finds by
		// pressing the key again.
		if m.Selecting() {
			m.endSelect(true)
		} else {
			m.startSelect()
		}
		m.syncContent()
	case keymap.ActionLogsFollow:
		// Rejoining the stream ends any selection: following pins the cursor to the
		// newest line, so the two cannot both be true. The end does not restore the
		// suspended follow state — this key is about to set it explicitly, and from
		// visual mode it is always paused, so the toggle reads "resume", which is what
		// a reader pressing `f` out of a selection means.
		m.endSelect(false)
		// Resuming pins the cursor to the newest line and jumps there; pausing leaves
		// the cursor exactly where it is, so `f` twice is a no-op rather than a way to
		// lose your place. syncContent is both, because the pin is a property of
		// following rather than of this key.
		m.following = !m.following
		m.syncContent()
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
		// One esc clears an open filter (restoring the full stream); the next abandons a
		// selection; the last closes the view. Each rung undoes the innermost thing the
		// reader turned on, so nothing they built is ever lost to the key that dismisses.
		if m.filtering {
			m.closeFilter()
			m.render()
			return m, nil
		}
		if m.Selecting() {
			m.endSelect(true)
			m.syncContent()
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

// highlight paints the matched spans of line with the Match style. Spans arrive in
// order and non-overlapping (both FindAllStringIndex and spanSubstring guarantee it);
// anything out of range or zero-width is skipped so a pathological pattern (`x*`) can
// only fail to highlight, never corrupt the line.
//
// base is what the *unmatched* text is painted with, and nil means "leave it exactly
// as it streamed". The nil case is not an optimisation detail, it is the streaming
// path: every buffered line goes through it, so emitting a style there would put two
// escape sequences on every log line the view holds. Only the cursor's line has a base
// (Selection), and it is re-derived on its way to the viewport rather than cached.
//
// Match keeps its own background even under Selection, which is the point: the two are
// designed to share a line (styles.Match uses the Warn hue precisely so a highlight is
// never mistaken for the cursor), and blanking the marks on the cursor's line would
// blank the answer on exactly the line the reader is reading — the argument D239 made
// for the table's cursor row, which applies here with more force because in a grep the
// match *is* why the line is on screen.
func (m Model) highlight(line string, spans [][]int, base *lipgloss.Style) string {
	paint := func(b *strings.Builder, s string) {
		if s == "" {
			return
		}
		if base == nil {
			b.WriteString(s)
			return
		}
		b.WriteString(base.Render(s))
	}
	var b strings.Builder
	if len(spans) == 0 {
		paint(&b, line)
		return b.String()
	}
	last := 0
	for _, s := range spans {
		if s[0] < last || s[1] > len(line) || s[0] >= s[1] {
			continue
		}
		paint(&b, line[last:s[0]])
		b.WriteString(m.styles.Match.Render(line[s[0]:s[1]]))
		last = s[1]
	}
	paint(&b, line[last:])
	return b.String()
}

// stamp is line i's rendered timestamp prefix, or "" when timestamps are off or the
// server sent none for that line. st is the style to draw it in — Subtle for an ordinary
// line, because the timestamp is context for the message and should not compete with it
// for the eye, and Selection under the cursor so the bar is not broken by a gap where the
// clock is. An unstamped line is simply not padded: aligning it under its neighbours would
// mean inventing a timestamp it does not have, and in practice a stream is either wholly
// stamped or wholly not.
func (m Model) stamp(i int, st lipgloss.Style) string {
	if !m.timestamps || i >= len(m.stamps) || m.stamps[i] == "" {
		return ""
	}
	return st.Render(m.stamps[i] + " ")
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
		return m.stamp(i, m.styles.Subtle) + line, true
	}
	if match == nil {
		return "", false
	}
	spans, ok := match(line)
	if !ok {
		return "", false
	}
	return m.stamp(i, m.styles.Subtle) + m.highlight(line, spans, nil), true
}

// rebuildShown recomputes the whole rendered body from the buffer. It is the O(n) path,
// and the only one: it runs when the query, the grep mode or the timestamps toggle
// changes what every line looks like — reader gestures, not stream events — never on
// append.
// It also re-finds the cursor. The cursor is an index into the *shown* set, and a
// rebuild is exactly the event that changes what lives at that index — so it is carried
// across as the buffer line it was on and re-looked-up afterwards, landing on the nearest
// still-shown line at or after it. Without that, narrowing a grep by one keystroke would
// throw the reader's cursor onto an unrelated line every time.
func (m *Model) rebuildShown() {
	anchor := -1
	if m.cursor >= 0 && m.cursor < len(m.shownIdx) {
		anchor = m.shownIdx[m.cursor]
	}
	// The visual-mode anchor is carried the same way and for the same reason: it is the
	// other end of a range the reader chose over *log lines*, so a query that hides some
	// of them must narrow the selection to what survives, not slide its end onto whatever
	// the new query put at that index.
	selAnchor := -1
	if m.anchor >= 0 && m.anchor < len(m.shownIdx) {
		selAnchor = m.shownIdx[m.anchor]
	}
	match, filtered := m.shownFilter()
	kept, idx := m.shownLines[:0], m.shownIdx[:0]
	for i := range m.lines {
		if s, ok := m.renderLine(i, match, filtered); ok {
			kept = append(kept, s)
			idx = append(idx, i)
		}
	}
	m.shownLines, m.shownIdx = kept, idx
	if anchor >= 0 {
		// shownIdx is ascending by construction, so the insertion point is the first
		// kept line at or after the anchor (and len(shownIdx) when it was the last —
		// placeCursor clamps that back onto the body).
		m.cursor, _ = slices.BinarySearch(m.shownIdx, anchor)
	}
	if selAnchor >= 0 {
		m.anchor, _ = slices.BinarySearch(m.shownIdx, selAnchor)
	}
}

// placeCursor puts the cursor somewhere real for the body as it now stands. Following
// owns it outright — a followed view's cursor is the newest line, which is where the
// next line will arrive and where `v` would start a selection — and otherwise it is only
// clamped, so a reader's position survives everything but the line it was on leaving the
// shown set.
func (m *Model) placeCursor() {
	switch {
	case len(m.shownLines) == 0:
		m.cursor = -1
	case m.following:
		m.cursor = len(m.shownLines) - 1
	case m.cursor < 0:
		m.cursor = 0
	case m.cursor >= len(m.shownLines):
		m.cursor = len(m.shownLines) - 1
	}
	// The selection's fixed end is only ever clamped — a rebuild has already re-found it
	// by log line. Past the end of an emptied body it lands on -1, which *is* "no
	// selection": there is nothing left to have selected, so visual mode ends with it.
	if m.anchor >= len(m.shownLines) {
		m.anchor = len(m.shownLines) - 1
	}
}

// moveCursor steps the cursor d shown lines (negative is up), clamping at both ends, and
// redraws. Callers that mean "and stop tailing" clear following first: the cursor is
// pinned while following, so a move that did not would be undone by placeCursor.
func (m *Model) moveCursor(d int) {
	if len(m.shownLines) == 0 {
		return
	}
	c := max(m.cursor, 0) + d
	m.cursor = min(max(c, 0), len(m.shownLines)-1)
	m.syncContent()
}

// page is the cursor distance one page (div 1) or half page (div 2) moves: screen rows,
// which equal shown lines while clipping and over-estimate nothing while wrapping (a
// wrapped page covers fewer log lines than rows, so paging moves at most a screenful).
// Never zero — an unsized view must still move by one.
func (m Model) page(div int) int { return max(1, m.viewport.Height()/div) }

// cursorLine is shown line n repainted as the cursor — or, in visual mode, as one line
// of the selection, which is the same bar: a Selection background across the whole pane,
// with the matched spans still marked on top of it and the timestamp inside the bar
// rather than beside it. It is derived here, not cached in shownLines, so that moving the
// cursor never invalidates the append cache LOGS-05b built (and a repaint costs one line,
// not the buffer).
func (m Model) cursorLine(n int) string {
	i := m.shownIdx[n]
	base := m.styles.Selection
	var spans [][]int
	if match, filtered := m.shownFilter(); filtered && match != nil {
		spans, _ = match(m.lines[i])
	}
	s := m.stamp(i, base) + m.highlight(m.lines[i], spans, &base)
	// Pad to the pane so the bar is a bar. A line already wider than the pane needs
	// none, which is also what keeps this from changing any line's wrapped height.
	if pad := m.viewport.Width() - lipgloss.Width(s); pad > 0 {
		s += base.Render(strings.Repeat(" ", pad))
	}
	return s
}

// lineRows is how many display rows a body line occupies at width w — one while
// clipping, ceil(width/w) while wrapping. Width is measured ANSI-aware, since a shown
// line may already carry Match escapes.
func lineRows(s string, w int) int {
	if w <= 0 {
		return 1
	}
	return max(1, (lipgloss.Width(s)+w-1)/w)
}

// cursorRow reports the viewport y-offset of shown line n and how many rows it occupies.
//
// The two coordinate spaces only agree while clipping: with SoftWrap on, the viewport's
// offset counts *display rows*, so the row of a shown line is the summed height of
// everything above it. That is also why viewport.EnsureVisible is not used here — it
// compares a content-line index against a row offset, which is only correct unwrapped.
// The walk is O(shown lines) and paid solely in wrap mode, on a reader gesture; the same
// order rebuildShown already pays on every keystroke typed into the grep.
func (m Model) cursorRow(n int) (row, height int) {
	if !m.wrap {
		return n, 1
	}
	w := m.viewport.Width()
	for i := range n {
		row += lineRows(m.shownLines[i], w)
	}
	return row, lineRows(m.shownLines[n], w)
}

// showCursor scrolls the viewport the least it can to bring the cursor's line into view,
// so h/j/k/l move a cursor through a still page rather than dragging the page around. A
// line taller than the pane (wrapped) is shown from its top: its first row is the one the
// reader is looking for.
func (m *Model) showCursor() {
	if m.cursor < 0 || m.cursor >= len(m.shownLines) {
		return
	}
	row, h := m.cursorRow(m.cursor)
	top, height := m.viewport.YOffset(), m.viewport.Height()
	switch {
	case row < top || h >= height:
		m.viewport.SetYOffset(row)
	case row+h > top+height:
		m.viewport.SetYOffset(row + h - height)
	}
}

// syncContent hands the rendered body to the viewport and keeps the newest line pinned
// while following. The body is cloned because the viewport takes ownership of the slice
// it is given (it normalizes embedded line endings in place and splits them out), and
// this one is the cache every later append extends. Cloning copies string headers, not
// the log text — far cheaper than the join-and-re-split SetContent would do.
func (m *Model) syncContent() {
	m.placeCursor()
	body := slices.Clone(m.shownLines)
	if lo, hi, ok := m.Selection(); ok {
		for n := lo; n <= hi && n < len(body); n++ {
			body[n] = m.cursorLine(n)
		}
	}
	m.viewport.SetContentLines(body)
	m.clampHOffset()
	if m.following {
		m.viewport.GotoBottom()
		return
	}
	m.showCursor()
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
	// Once the buffer has dropped its oldest lines (LOGS-07) the top of the body is no
	// longer the start of the stream, so `gg` lands in the middle of a log that looks
	// like it starts there. That is state the reader cannot see and would misread, which
	// is D146's test for a marker. It is a fixed word rather than a count of what was
	// dropped: the count would change on every trim and the reader can do nothing with
	// it, while "there was more above this" is the whole of what they need to know.
	if m.trimmed {
		seg += "  [trimmed]"
	}
	// Visual mode, with the size of the selection (LOGS-SEL-02). It earns a marker on
	// D146's own test — state the reader can lose sight of — twice over. A fresh `v`
	// selects exactly the line the cursor was already on, so the screen is unchanged
	// and the only evidence that the next `j` will extend rather than move is this;
	// and a selection can be taller than the pane (`gg v G`), where the count is the
	// only way to know what a `y` is about to copy.
	if lo, hi, ok := m.Selection(); ok && m.Selecting() {
		seg += "  [visual " + itoa(hi-lo+1) + "]"
	}
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
