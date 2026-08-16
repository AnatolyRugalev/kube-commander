package logsview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// plain is the view with styling removed. Since LOGS-03 highlights matched spans, a
// matching line is no longer one contiguous run of bytes in View() — the Match style
// wraps the span — so any assertion about *content* has to strip first. Assertions
// about the highlight itself compare against the rendered span (see matchSpan).
func plain(v string) string { return ansi.Strip(v) }

// matchSpan is how a highlighted span looks in the view: the text rendered through the
// shared Match style. Comparing against it keeps the test honest about the styling
// without restating an escape sequence.
func matchSpan(s string) string { return styles.Default().Match.Render(s) }

// followSpan is how the follow-state token looks in the header when the stream is live:
// the text rendered through the shared Follow badge (STORY-06j-1). Comparing against it
// keeps the test honest about the styling without restating an escape sequence.
func followSpan(s string) string { return styles.Default().Follow.Render(s) }

func newLogs() Model {
	m := New(styles.Default())
	m.SetSize(40, 12)
	m.Show()
	return m
}

// appendLines streams "line-1".."line-n" into m.
func appendLines(m *Model, n int) {
	for i := 1; i <= n; i++ {
		m.Append("", "line-"+itoa(i))
	}
}

// typeFilter opens the filter and feeds each rune of q, returning the updated model.
func typeFilter(m Model, q string) Model {
	m, _ = m.Update(keymap.ActionFilter)
	for _, r := range q {
		m, _ = m.UpdateFilter(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return m
}

func TestHiddenOrUnsizedIsEmpty(t *testing.T) {
	m := New(styles.Default())
	m.Append("", "hello")
	m.SetSize(40, 12) // sized but hidden
	if v := m.View(); v != "" {
		t.Errorf("hidden View() = %q; want empty", v)
	}
	m2 := New(styles.Default())
	m2.Append("", "hello")
	m2.Show() // shown but unsized
	if v := m2.View(); v != "" {
		t.Errorf("unsized View() = %q; want empty", v)
	}
}

func TestStartsFollowingAndTails(t *testing.T) {
	m := newLogs()
	if !m.Following() {
		t.Fatal("a fresh logs view should start following")
	}
	appendLines(&m, 50)
	v := m.View()
	if !strings.Contains(v, "[following]") {
		t.Errorf("header should show [following]; got:\n%s", v)
	}
	if !strings.Contains(v, "line-50") {
		t.Errorf("following view should tail to line-50; got:\n%s", v)
	}
}

// TestFollowStateBadgeAndPausedPlain pins STORY-06j-1: the follow state must be a
// painted, unmistakable cue, not a word in the header. Following renders `[following]`
// through the shared Follow badge — the header, title included, sits in the Header
// style and the badge replaces the token mid-line — and paused renders `[paused]` as
// plain header text with no badge anywhere.
func TestFollowStateBadgeAndPausedPlain(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	v := m.View()
	if !strings.Contains(v, followSpan("[following]")) {
		t.Errorf("live header should paint [following] with the Follow badge; got:\n%s", v)
	}
	if strings.Contains(v, followSpan("[paused]")) {
		t.Errorf("live header should not paint [paused]; got:\n%s", v)
	}
	m, _ = m.Update(keymap.ActionTop) // pause
	if m.Following() {
		t.Fatal("scrolling up should pause following")
	}
	v = m.View()
	if !strings.Contains(v, "[paused]") {
		t.Errorf("paused header should show [paused]; got:\n%s", v)
	}
	if strings.Contains(v, followSpan("[following]")) {
		t.Errorf("paused header should not paint the follow badge; got:\n%s", v)
	}
}

func TestUpwardScrollPausesAndDoesNotYank(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	// Scroll to the top: this pauses following.
	m, _ = m.Update(keymap.ActionTop)
	if m.Following() {
		t.Fatal("scrolling up should pause following")
	}
	if !strings.Contains(m.View(), "[paused]") {
		t.Errorf("header should show [paused] after scroll-up; got:\n%s", m.View())
	}
	// A new line arriving while paused must not yank the view to the bottom.
	m.Append("", "line-51")
	if !strings.Contains(m.View(), "line-1") {
		t.Errorf("paused view should stay at the top, not jump to newest; got:\n%s", m.View())
	}
}

func TestFollowToggleResumesAtBottom(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	m, _ = m.Update(keymap.ActionTop) // pause at top
	if m.Following() {
		t.Fatal("precondition: should be paused")
	}
	m, _ = m.Update(keymap.ActionLogsFollow) // resume
	if !m.Following() {
		t.Fatal("logs.follow should re-enable following")
	}
	if !strings.Contains(m.View(), "line-50") {
		t.Errorf("resuming follow should jump to the newest line; got:\n%s", m.View())
	}
	// Toggle again pauses.
	m, _ = m.Update(keymap.ActionLogsFollow)
	if m.Following() {
		t.Error("second logs.follow should pause")
	}
}

// TestJumpToBottomResumesFollowing is the headline of LOGS-04c: `G` is the one gesture
// that catches a scrolled-back reader up *and* keeps them tailing. Landing on the newest
// line is only half of it — the assertion that matters is that the line arriving after
// the jump is still on screen.
func TestJumpToBottomResumesFollowing(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	m, _ = m.Update(keymap.ActionTop) // pause at the top
	if m.Following() {
		t.Fatal("precondition: scrolling to the top should pause following")
	}
	m, _ = m.Update(keymap.ActionBottom)
	if !m.Following() {
		t.Fatal("nav.bottom should re-arm following")
	}
	if v := plain(m.View()); !strings.Contains(v, "line-50") {
		t.Errorf("nav.bottom should show the newest line; got:\n%s", v)
	}
	m.Append("", "line-51")
	v := plain(m.View())
	if !strings.Contains(v, "line-51") {
		t.Errorf("after nav.bottom the view should keep tailing; got:\n%s", v)
	}
	if !strings.Contains(v, "[following]") {
		t.Errorf("header should show [following] after nav.bottom; got:\n%s", v)
	}
}

// TestDownwardScrollDoesNotResumeFollowing draws the other half of the line: only the
// *explicit* jump to the end rejoins the stream. Stepping or paging down — even all the
// way onto the last line — is browsing, and a reader parked at the end of a paused view
// must be able to stay there.
func TestDownwardScrollDoesNotResumeFollowing(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	m, _ = m.Update(keymap.ActionTop) // pause at the top
	for _, a := range []keymap.Action{keymap.ActionDown, keymap.ActionHalfPageDown, keymap.ActionPageDown} {
		m, _ = m.Update(a)
		if m.Following() {
			t.Fatalf("%v should not resume following; only nav.bottom does", a)
		}
	}
	// Reach the last line the slow way: still paused.
	for i := 0; i < 60; i++ {
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.Following() {
		t.Fatal("scrolling onto the last line should not resume following")
	}
	if v := plain(m.View()); !strings.Contains(v, "[paused]") {
		t.Errorf("header should still show [paused]; got:\n%s", v)
	}
}

// --- LOGS-SEL-01: the line cursor (feedback 2026-08-07-logs-selection-and-yank) ---

// cursorBar is how the cursor's line looks in the view: the text rendered through the
// shared Selection style. The bar is padded to the pane, so the assertion is that the
// line's own text is inside it — not that the whole row is one Render call.
func cursorBar(s string) string { return styles.Default().Selection.Render(s) }

// TestCursorFollowsTheNewestLine: a followed view's cursor is the line the next one will
// arrive after, so the reader never has to chase it to start a selection. It is also the
// only sane answer while tailing — a cursor left behind on line 3 of a stream at 1,900
// lines/sec is not a position, it is a scroll the reader did not ask for.
func TestCursorFollowsTheNewestLine(t *testing.T) {
	m := newLogs()
	if m.Cursor() != -1 {
		t.Fatalf("an empty view has no cursor; got %d", m.Cursor())
	}
	appendLines(&m, 50)
	if m.Cursor() != 49 {
		t.Fatalf("a followed view should hold the cursor on the newest line; got %d", m.Cursor())
	}
	if v := m.View(); !strings.Contains(v, cursorBar("line-50")) {
		t.Errorf("the newest line should carry the cursor bar; got:\n%q", v)
	}
	m.Append("", "line-51")
	if m.Cursor() != 50 {
		t.Errorf("the cursor should ride the stream; got %d", m.Cursor())
	}
	if v := m.View(); !strings.Contains(v, cursorBar("line-51")) {
		t.Errorf("the cursor should have moved onto the newly streamed line; got:\n%q", v)
	}
}

// TestCursorMovesByLineAndPausesOnTheWayUp is the gesture the feedback asked for: j/k
// move a highlighted line the way they move a table row. Up pauses following (the
// pre-existing rule — a reader looking back must not be yanked forward), down does not
// resume it (LOGS-04c's other half, unchanged).
func TestCursorMovesByLineAndPausesOnTheWayUp(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)

	m, _ = m.Update(keymap.ActionUp)
	if m.Following() {
		t.Fatal("moving the cursor up should pause following")
	}
	if m.Cursor() != 48 {
		t.Fatalf("one nav.up = one line; cursor = %d, want 48", m.Cursor())
	}
	if v := m.View(); !strings.Contains(v, cursorBar("line-49")) {
		t.Errorf("the bar should have moved to line-49; got:\n%q", v)
	}
	m, _ = m.Update(keymap.ActionDown)
	if m.Cursor() != 49 {
		t.Fatalf("one nav.down = one line back; cursor = %d, want 49", m.Cursor())
	}
	if m.Following() {
		t.Error("stepping back onto the last line is browsing, not a statement about the tail")
	}
}

// TestCursorClampsAtBothEnds: gg/G and a wall of j/k land on the first and last line and
// stay there, rather than running the index off either end of the body.
func TestCursorClampsAtBothEnds(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)

	m, _ = m.Update(keymap.ActionTop)
	if m.Cursor() != 0 {
		t.Fatalf("nav.top should put the cursor on the first line; got %d", m.Cursor())
	}
	for range 5 {
		m, _ = m.Update(keymap.ActionUp)
	}
	if m.Cursor() != 0 {
		t.Errorf("the cursor should clamp at the first line; got %d", m.Cursor())
	}
	if v := plain(m.View()); !strings.Contains(v, "line-1") {
		t.Errorf("the first line should be on screen; got:\n%s", v)
	}
	for range 100 {
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.Cursor() != 49 {
		t.Errorf("the cursor should clamp at the last line; got %d", m.Cursor())
	}
	// Reached by stepping, so still paused — and G is what re-arms it, cursor included.
	m, _ = m.Update(keymap.ActionBottom)
	if !m.Following() || m.Cursor() != 49 {
		t.Errorf("nav.bottom should follow with the cursor on the newest line; following=%v cursor=%d",
			m.Following(), m.Cursor())
	}
}

// TestCursorDragsTheViewportOnlyWhenItHasTo is why this is a cursor and not a scroll: the
// page stays still while the cursor crosses it, and moves by the least it can once the
// cursor would leave it.
func TestCursorDragsTheViewportOnlyWhenItHasTo(t *testing.T) {
	m := newLogs() // 40x12 → 11 body rows
	appendLines(&m, 50)
	m, _ = m.Update(keymap.ActionTop)
	if off := m.viewport.YOffset(); off != 0 {
		t.Fatalf("precondition: nav.top should be unscrolled; offset %d", off)
	}
	h := m.viewport.Height()
	for range h - 1 { // walk to the last visible row
		m, _ = m.Update(keymap.ActionDown)
	}
	if off := m.viewport.YOffset(); off != 0 {
		t.Errorf("the page should not move while the cursor crosses it; offset %d", off)
	}
	m, _ = m.Update(keymap.ActionDown) // one past it
	if off := m.viewport.YOffset(); off != 1 {
		t.Errorf("stepping off the bottom should scroll by exactly one row; offset %d", off)
	}
	if v := plain(m.View()); !strings.Contains(v, "line-"+itoa(h+1)) {
		t.Errorf("the cursor's line should be on screen; got:\n%s", v)
	}
}

// TestCursorStepsAWrappedLineAsOneLine is the property the feedback singled out: the
// cursor is over log lines, not screen rows. With wrapping on, a line that occupies
// several rows is still one k away — which is exactly what terminal select-to-copy gets
// wrong, and the reason this is not a viewport scroll wearing a highlight.
func TestCursorStepsAWrappedLineAsOneLine(t *testing.T) {
	m := newLogs()
	m.Append("", "short one")
	m.Append("", longLine()) // several rows wide at 40 columns
	m.Append("", "short two")
	m, _ = m.Update(keymap.ActionLogsWrap)

	if m.Cursor() != 2 {
		t.Fatalf("precondition: the cursor should be on the newest line; got %d", m.Cursor())
	}
	m, _ = m.Update(keymap.ActionUp)
	if m.Cursor() != 1 {
		t.Fatalf("one nav.up should cross the whole wrapped line; cursor = %d, want 1", m.Cursor())
	}
	m, _ = m.Update(keymap.ActionUp)
	if m.Cursor() != 0 {
		t.Fatalf("the second nav.up should reach the first line; cursor = %d", m.Cursor())
	}
	if v := plain(m.View()); !strings.Contains(v, "short one") {
		t.Errorf("scrolling back over a wrapped line should show the line above it; got:\n%s", v)
	}
}

// TestCursorScrollsInRowsWhileWrapping is the arithmetic that has to be done by hand.
// Once SoftWrap is on the viewport's offset counts *display rows* while the cursor counts
// log lines, so keeping the cursor on screen means summing the heights above it — which is
// also why viewport.EnsureVisible cannot be used (it compares a line index to a row
// offset). Treating the two as the same number scrolls too little and leaves the tail of
// the cursor's own line below the fold, which is what this reads.
func TestCursorScrollsInRowsWhileWrapping(t *testing.T) {
	m := newLogs() // 40x12 → 11 body rows
	for i := range 6 {
		m.Append("", "HEAD"+itoa(i)+strings.Repeat("-", 90)+"TAIL"+itoa(i)) // 3 rows each
	}
	m, _ = m.Update(keymap.ActionLogsWrap)
	m, _ = m.Update(keymap.ActionTop)
	if off := m.viewport.YOffset(); off != 0 {
		t.Fatalf("precondition: nav.top should be unscrolled; offset %d", off)
	}
	for range 3 { // down to line 3, whose rows are 9..11 — one past the bottom
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.Cursor() != 3 {
		t.Fatalf("cursor = %d, want 3", m.Cursor())
	}
	v := plain(m.View())
	if !strings.Contains(v, "HEAD3") || !strings.Contains(v, "TAIL3") {
		t.Errorf("the cursor's wrapped line should be on screen whole; got:\n%s", v)
	}
	if off := m.viewport.YOffset(); off != 1 {
		t.Errorf("the view should have scrolled by the one row it owed; offset %d, want 1", off)
	}
}

// TestCursorKeepsItsLogLineAcrossAFilterChange: the cursor indexes the shown set, and a
// keystroke in the grep rewrites that set — so it is carried across as the *log line* it
// was on. Without this every character typed would fling the cursor onto an unrelated
// line.
func TestCursorKeepsItsLogLineAcrossAFilterChange(t *testing.T) {
	m := newLogs()
	for _, l := range []string{"boot ok", "err disk", "steady", "err net", "done"} {
		m.Append("", l)
	}
	m, _ = m.Update(keymap.ActionUp) // pause, cursor on "err net" (index 3)
	if m.Cursor() != 3 {
		t.Fatalf("precondition: cursor = %d, want 3", m.Cursor())
	}
	m = typeFilter(m, "err") // shown becomes [err disk, err net]
	if m.Cursor() != 1 {
		t.Fatalf("the cursor should still be on `err net`, now shown line 1; got %d", m.Cursor())
	}
	if v := m.View(); !strings.Contains(v, cursorBar(" net")) {
		t.Errorf("the bar should be on `err net`; got:\n%q", v)
	}
	// Clearing the grep restores the full set, and the cursor with it.
	m, _ = m.Update(keymap.ActionBack)
	if m.Cursor() != 3 {
		t.Errorf("clearing the grep should put the cursor back on the same log line; got %d", m.Cursor())
	}
}

// TestCursorLineKeepsItsMatchHighlight: Selection and Match share the cursor's line
// rather than one blanking the other (D239's rule for the table, and the reason
// styles.Match uses the Warn hue). In a grep the match is *why* the line is on screen, so
// hiding it under the cursor would hide the answer on the one line being read.
func TestCursorLineKeepsItsMatchHighlight(t *testing.T) {
	m := newLogs()
	m.Append("", "err and err again")
	m = typeFilter(m, "err")
	if m.Cursor() != 0 {
		t.Fatalf("precondition: the only shown line should hold the cursor; got %d", m.Cursor())
	}
	v := m.View()
	if n := strings.Count(v, matchSpan("err")); n != 2 {
		t.Errorf("both matches should survive on the cursor's line; got %d in:\n%q", n, v)
	}
	if !strings.Contains(v, cursorBar(" and ")) {
		t.Errorf("the unmatched run should carry the selection bar; got:\n%q", v)
	}
	if p := plain(v); !strings.Contains(p, "err and err again") {
		t.Errorf("the cursor must not alter the line's text; got:\n%s", p)
	}
}

// TestCursorIsNotBakedIntoTheAppendCache guards the seam LOGS-05b built: shownLines is
// the cache a rebuild must reproduce, so the bar is applied to a copy on its way to the
// viewport. Painting it into the cache would mean every cursor move invalidated it — and
// would put escape sequences in front of the raw text LOGS-SEL-02's yank needs.
func TestCursorIsNotBakedIntoTheAppendCache(t *testing.T) {
	m := newLogs()
	appendLines(&m, 3)
	for i, s := range m.shownLines {
		if strings.Contains(s, "\x1b") {
			t.Errorf("cached line %d carries styling: %q", i, s)
		}
	}
	if got, want := m.shownLines[m.Cursor()], "line-3"; got != want {
		t.Errorf("the cursor's cached line = %q; want the raw %q", got, want)
	}
	if v := m.View(); !strings.Contains(v, cursorBar("line-3")) {
		t.Errorf("the bar should still reach the frame; got:\n%q", v)
	}
}

// TestCursorMapsBackToTheBufferLine is the map LOGS-SEL-02 yanks through: with a grep on,
// shown line n is buffer line shownIdx[n], and lines[] holds the text as it streamed —
// unpainted, unstamped, unwrapped.
func TestCursorMapsBackToTheBufferLine(t *testing.T) {
	m := newLogs()
	for _, l := range []string{"boot ok", "err disk", "steady", "err net"} {
		m.Append("", l)
	}
	m = typeFilter(m, "err")
	if len(m.shownIdx) != 2 || m.shownIdx[0] != 1 || m.shownIdx[1] != 3 {
		t.Fatalf("shownIdx = %v; want [1 3]", m.shownIdx)
	}
	if got := m.lines[m.shownIdx[m.Cursor()]]; got != "err net" {
		t.Errorf("the cursor's buffer line = %q; want %q", got, "err net")
	}
}

// TestEmptyBodyHasNoCursor: a grep that matches nothing leaves nothing to point at, and
// the cursor says so rather than pointing at line 0 of an empty body.
func TestEmptyBodyHasNoCursor(t *testing.T) {
	m := newLogs() // nothing streamed yet
	for _, a := range []keymap.Action{keymap.ActionUp, keymap.ActionDown, keymap.ActionTop, keymap.ActionPageDown} {
		m, _ = m.Update(a) // must not panic or invent a position
		if m.Cursor() != -1 {
			t.Fatalf("%v gave an empty view a cursor: %d", a, m.Cursor())
		}
	}
	// A grep that matches nothing is the same state reached the other way.
	m = newLogs()
	appendLines(&m, 3)
	m = typeFilter(m, "zzz")
	if m.Cursor() != -1 || len(m.shownIdx) != 0 {
		t.Errorf("a grep that keeps no line leaves no cursor; cursor=%d shownIdx=%v", m.Cursor(), m.shownIdx)
	}
	// Backing the grep out restores the body and the cursor with it.
	m, _ = m.Update(keymap.ActionBack)
	if m.Cursor() != 2 {
		t.Errorf("clearing the grep should re-seat the cursor on the newest line; got %d", m.Cursor())
	}
}

// TestResetAndRestreamDropTheCursor: both empty the buffer, so the cursor cannot survive
// them — an index into a body that no longer exists is the one way this could point at
// the wrong log.
func TestResetAndRestreamDropTheCursor(t *testing.T) {
	for name, clear := range map[string]func(*Model){
		"Reset":    (*Model).Reset,
		"Restream": (*Model).Restream,
	} {
		t.Run(name, func(t *testing.T) {
			m := newLogs()
			appendLines(&m, 5)
			m, _ = m.Update(keymap.ActionUp) // pause somewhere in the middle
			clear(&m)
			if m.Cursor() != -1 || len(m.shownIdx) != 0 {
				t.Errorf("cursor = %d, shownIdx = %v; want -1 and empty", m.Cursor(), m.shownIdx)
			}
			m.Append("", "fresh")
			if m.Cursor() != 0 {
				t.Errorf("the first line of the new stream should take the cursor; got %d", m.Cursor())
			}
		})
	}
}

func TestLiveFilterNarrowsShownLines(t *testing.T) {
	m := newLogs()
	m.Append("", "alpha error one")
	m.Append("", "beta ok")
	m.Append("", "gamma ERROR two")
	m = typeFilter(m, "error") // case-insensitive substring
	if !m.Filtering() {
		t.Fatal("filter should be open")
	}
	v := plain(m.View())
	if !strings.Contains(v, "alpha error one") || !strings.Contains(v, "gamma ERROR two") {
		t.Errorf("filter should keep the two matching lines; got:\n%s", v)
	}
	if strings.Contains(v, "beta ok") {
		t.Errorf("filter should drop the non-matching line; got:\n%s", v)
	}
	// Header reflects the query and the matched/total count.
	if !strings.Contains(v, "/error") || !strings.Contains(v, "2/3") {
		t.Errorf("header should show the query and 2/3; got:\n%s", v)
	}
}

func TestFilterNarrowsWhileFollowing(t *testing.T) {
	m := newLogs()
	m = typeFilter(m, "keep")
	m.Append("", "drop this")
	m.Append("", "keep this")
	v := plain(m.View())
	if strings.Contains(v, "drop this") {
		t.Errorf("a line streamed under an active filter must be excluded if it doesn't match; got:\n%s", v)
	}
	if !strings.Contains(v, "keep this") {
		t.Errorf("a matching streamed line should show; got:\n%s", v)
	}
}

func TestBackClearsFilterThenCloses(t *testing.T) {
	m := newLogs()
	m.Append("", "only line")
	m = typeFilter(m, "zzz") // matches nothing
	if strings.Contains(plain(m.View()), "only line") {
		t.Fatalf("precondition: filter should hide the non-matching line")
	}
	// First back clears the filter (restores the stream), no ClosedMsg.
	m, cmd := m.Update(keymap.ActionBack)
	if cmd != nil {
		t.Errorf("back-while-filtering should not emit a cmd; got non-nil")
	}
	if m.Filtering() {
		t.Errorf("back should close the filter")
	}
	if !strings.Contains(plain(m.View()), "only line") {
		t.Errorf("clearing the filter should restore the stream; got:\n%s", m.View())
	}
	// Second back closes the view.
	_, cmd = m.Update(keymap.ActionBack)
	if cmd == nil {
		t.Fatal("back with no filter should emit a ClosedMsg cmd")
	}
	closed, ok := cmd().(ClosedMsg)
	if !ok {
		t.Fatalf("close msg = %T; want ClosedMsg", cmd())
	}
	if closed.Kind != "logs" {
		t.Errorf("ClosedMsg.Kind = %q; want %q", closed.Kind, "logs")
	}
}

// --- LOGS-03: regex mode + match highlighting ---

// TestRegexModeMatchesPattern: with logs.regex on, the same field is a pattern, not a
// substring — anchors and alternation work, and the header marks the mode.
func TestRegexModeMatchesPattern(t *testing.T) {
	m := newLogs()
	m.Append("", "GET /healthz 200")
	m.Append("", "GET /api/pods 500")
	m.Append("", "GET /api/pods 503")
	m, _ = m.Update(keymap.ActionLogsRegex)
	if !m.Regex() {
		t.Fatal("logs.regex should turn regex mode on")
	}
	m = typeFilter(m, "50[03]$")
	v := plain(m.View())
	if strings.Contains(v, "healthz") {
		t.Errorf("the 200 line does not match 50[03]$; got:\n%s", v)
	}
	if !strings.Contains(v, "/api/pods 500") || !strings.Contains(v, "/api/pods 503") {
		t.Errorf("both 5xx lines should match; got:\n%s", v)
	}
	if !strings.Contains(v, "[re]") || !strings.Contains(v, "2/3") {
		t.Errorf("header should mark regex mode and count 2/3; got:\n%s", v)
	}
	// Substring mode would treat the same query literally: nothing matches.
	m, _ = m.Update(keymap.ActionLogsRegex)
	if m.Regex() {
		t.Fatal("a second logs.regex should turn regex mode off")
	}
	if v := plain(m.View()); strings.Contains(v, "/api/pods 500") || !strings.Contains(v, "0/3") {
		t.Errorf("back in substring mode the pattern is literal text and matches nothing; got:\n%s", v)
	}
}

// TestRegexIsCaseInsensitiveByDefault: the regex grep matches the substring grep's
// case-insensitive default, and an explicit (?-i) in the query still overrides it.
func TestRegexIsCaseInsensitiveByDefault(t *testing.T) {
	m := newLogs()
	m.Append("", "Error: boom")
	m, _ = m.Update(keymap.ActionLogsRegex)
	m = typeFilter(m, "error")
	if !strings.Contains(plain(m.View()), "Error: boom") {
		t.Errorf("regex mode should default to case-insensitive; got:\n%s", plain(m.View()))
	}
	m2 := newLogs()
	m2.Append("", "Error: boom")
	m2, _ = m2.Update(keymap.ActionLogsRegex)
	m2 = typeFilter(m2, "(?-i)error")
	if strings.Contains(plain(m2.View()), "Error: boom") {
		t.Errorf("an explicit (?-i) should win over the default; got:\n%s", plain(m2.View()))
	}
}

// TestInvalidRegexKeepsLastGoodAndSaysSo: a half-typed pattern must not blank the view.
// The last pattern that compiled keeps narrowing and the header says the query on
// screen is not the one being applied (principle 3 / D145).
func TestInvalidRegexKeepsLastGoodAndSaysSo(t *testing.T) {
	m := newLogs()
	m.SetSize(80, 12) // a realistic terminal: the header clips from the right at 40
	m.Append("", "err one")
	m.Append("", "ok two")
	m, _ = m.Update(keymap.ActionLogsRegex)
	m = typeFilter(m, "err")
	if !strings.Contains(plain(m.View()), "err one") {
		t.Fatalf("precondition: `err` should match; got:\n%s", plain(m.View()))
	}
	// Keep typing toward `err(or)?` — `err(` alone does not compile.
	m, _ = m.UpdateFilter(tea.KeyPressMsg(tea.Key{Code: '(', Text: "("}))
	v := plain(m.View())
	if !strings.Contains(v, "err one") {
		t.Errorf("an uncompilable query should keep the last good match set; got:\n%s", v)
	}
	if strings.Contains(v, "ok two") {
		t.Errorf("the last good pattern still excludes non-matches; got:\n%s", v)
	}
	if !strings.Contains(v, "invalid regex") {
		t.Errorf("header should flag the query as invalid, not pass it off as a match; got:\n%s", v)
	}
	// Completing the pattern clears the flag and re-applies the new one.
	for _, r := range "or)?" {
		m, _ = m.UpdateFilter(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	if v := plain(m.View()); strings.Contains(v, "invalid regex") || !strings.Contains(v, "err one") {
		t.Errorf("a completed pattern should clear the flag and match; got:\n%s", v)
	}
}

// TestInvalidRegexWithNoLastGoodMatchesNothing: the other half of the degrade — when
// nothing has ever compiled there is no set to fall back to, so the view is empty (and
// labelled) rather than showing an unfiltered stream the query never asked for.
func TestInvalidRegexWithNoLastGoodMatchesNothing(t *testing.T) {
	m := newLogs()
	m.SetSize(80, 12)
	m.Append("", "err one")
	m.Append("", "ok two")
	m, _ = m.Update(keymap.ActionLogsRegex)
	m = typeFilter(m, "*")
	v := plain(m.View())
	if strings.Contains(v, "err one") || strings.Contains(v, "ok two") {
		t.Errorf("an invalid query with no last-good pattern must not show the stream; got:\n%s", v)
	}
	if !strings.Contains(v, "0/2") || !strings.Contains(v, "invalid regex") {
		t.Errorf("header should report 0/2 and flag the query; got:\n%s", v)
	}
}

// TestMatchedSpansAreHighlighted: every occurrence in a shown line is painted with the
// shared Match style, in both grep modes; the untouched text around it is left alone.
func TestMatchedSpansAreHighlighted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		regex bool
		query string
	}{
		{"substring", false, "err"},
		{"regex", true, "e.r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newLogs()
			m.Append("", "err and err again")
			if tc.regex {
				m, _ = m.Update(keymap.ActionLogsRegex)
			}
			m = typeFilter(m, tc.query)
			v := m.View()
			if n := strings.Count(v, matchSpan("err")); n != 2 {
				t.Errorf("both occurrences should be highlighted; got %d in:\n%q", n, v)
			}
			if !strings.Contains(plain(v), "err and err again") {
				t.Errorf("highlighting must not alter the line's text; got:\n%s", plain(v))
			}
		})
	}
}

// TestUnfilteredStreamIsNotHighlighted: with no query the body is the raw buffer — the
// fast path the high-throughput stream depends on, with no per-line match work at all.
func TestUnfilteredStreamIsNotHighlighted(t *testing.T) {
	m := newLogs()
	m.Append("", "plain line")
	if v := m.View(); !strings.Contains(v, "plain line") {
		t.Errorf("an unfiltered line should render verbatim, unstyled; got:\n%q", v)
	}
}

// TestNonASCIIFoldStillMatches: a fold that changes the byte length can't be sliced at
// lowered offsets, so the line is shown unhighlighted rather than dropped or corrupted.
func TestNonASCIIFoldStillMatches(t *testing.T) {
	m := newLogs()
	m.Append("", "İstanbul error") // 'İ' lowercases to two runes, changing the byte length
	m = typeFilter(m, "error")
	if !strings.Contains(plain(m.View()), "İstanbul error") {
		t.Errorf("a matching line must survive an awkward fold intact; got:\n%s", plain(m.View()))
	}
	spans, ok := spanSubstring("İstanbul error", "error")
	if !ok || spans != nil {
		t.Errorf("spanSubstring(fold-changing line) = %v, %v; want nil, true", spans, ok)
	}
}

func TestResetClearsBufferAndRearmsFollow(t *testing.T) {
	m := newLogs()
	appendLines(&m, 10)
	m, _ = m.Update(keymap.ActionTop) // pause
	m = typeFilter(m, "line-1")
	m, _ = m.Update(keymap.ActionLogsRegex)
	m, _ = m.Update(keymap.ActionLogsWrap)
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m.Reset()
	if m.Regex() {
		t.Errorf("Reset should drop regex mode so a new object opens on the plain grep")
	}
	if m.Wrap() {
		t.Errorf("Reset should drop wrap mode so a new object opens unwrapped")
	}
	if m.Timestamps() {
		t.Errorf("Reset should drop the timestamps toggle so a new object opens unstamped")
	}
	if m.HOffset() != 0 {
		t.Errorf("Reset should leave the view unscrolled; HOffset = %d", m.HOffset())
	}
	if !m.Empty() {
		t.Errorf("Reset should clear the buffer")
	}
	if !m.Following() {
		t.Errorf("Reset should re-arm following")
	}
	if m.Filtering() {
		t.Errorf("Reset should close the filter")
	}
	if q := m.Query(); q != "" {
		t.Errorf("Reset should clear the query; got %q", q)
	}
}

// The LOGS-04a fixtures: one line wider than the 40-column test view, with a marker at
// each end so "which part is on screen" is a content assertion rather than an offset one.
const (
	longHead = "HEAD"
	longTail = "TAIL"
)

func longLine() string { return longHead + strings.Repeat("-", 60) + longTail }

// TestWrapTogglesLongLineHandling is the headline of LOGS-04a: a line wider than the
// screen is clipped by default (one log line, one row) and logs.wrap folds it onto
// continuation rows so its tail is readable without scrolling.
func TestWrapTogglesLongLineHandling(t *testing.T) {
	m := newLogs()
	m.Append("", longLine())

	if v := plain(m.View()); strings.Contains(v, longTail) {
		t.Fatalf("a long line should be clipped by default: %q", v)
	}
	m, _ = m.Update(keymap.ActionLogsWrap)
	if !m.Wrap() {
		t.Fatal("logs.wrap should turn wrapping on")
	}
	v := plain(m.View())
	if !strings.Contains(v, longTail) {
		t.Errorf("wrapping should bring the line's tail on screen: %q", v)
	}
	if !strings.Contains(v, "[wrap]") {
		t.Errorf("the header should name the mode the reader turned on: %q", v)
	}
	m, _ = m.Update(keymap.ActionLogsWrap)
	if m.Wrap() {
		t.Fatal("a second logs.wrap should turn wrapping back off")
	}
	if v := plain(m.View()); strings.Contains(v, longTail) || strings.Contains(v, "[wrap]") {
		t.Errorf("unwrapping should clip again and drop the marker: %q", v)
	}
}

// TestHorizontalScrollReachesLineTail proves the other half of LOGS-04a: while the view
// is clipping, nav.left/nav.right walk it sideways to the tail of a long line, the header
// says how far, and the offset clamps at both ends.
func TestHorizontalScrollReachesLineTail(t *testing.T) {
	m := newLogs()
	m.Append("", longLine())

	m, _ = m.Update(keymap.ActionRight)
	if m.HOffset() != hStep {
		t.Fatalf("one nav.right = %d columns; got %d", hStep, m.HOffset())
	}
	v := plain(m.View())
	if strings.Contains(v, longHead) {
		t.Errorf("scrolling right should move the line's head off screen: %q", v)
	}
	if !strings.Contains(v, "[+8]") {
		t.Errorf("the header should report the hidden columns: %q", v)
	}

	for range 5 { // past the end: the offset clamps at the last reachable column.
		m, _ = m.Update(keymap.ActionRight)
	}
	if v := plain(m.View()); !strings.Contains(v, longTail) {
		t.Errorf("scrolling right should reach the line's tail: %q", v)
	}

	for range 10 { // and back past the start.
		m, _ = m.Update(keymap.ActionLeft)
	}
	if m.HOffset() != 0 {
		t.Fatalf("nav.left should clamp at column 0; got %d", m.HOffset())
	}
	v = plain(m.View())
	if !strings.Contains(v, longHead) {
		t.Errorf("back at column 0 the head should be on screen again: %q", v)
	}
	if strings.Contains(v, "[+") {
		t.Errorf("an unscrolled view should carry no offset marker: %q", v)
	}
}

// TestWrapZeroesTheHorizontalOffset guards the one way the two modes can interfere: the
// viewport ignores the horizontal offset while soft-wrapping, so an offset carried into
// wrap mode would silently scroll the view when wrapping was switched back off.
func TestWrapZeroesTheHorizontalOffset(t *testing.T) {
	m := newLogs()
	m.Append("", longLine())
	m, _ = m.Update(keymap.ActionRight)

	m, _ = m.Update(keymap.ActionLogsWrap)
	if m.HOffset() != 0 {
		t.Fatalf("turning wrap on should drop the horizontal offset; got %d", m.HOffset())
	}
	m, _ = m.Update(keymap.ActionLogsWrap)
	if m.HOffset() != 0 {
		t.Fatalf("the offset must not reappear when wrapping is turned off; got %d", m.HOffset())
	}
	if v := plain(m.View()); !strings.Contains(v, longHead) {
		t.Errorf("an unscrolled clipped view should start at the line's head: %q", v)
	}
}

// TestHorizontalScrollDoesNotPauseFollow: moving sideways says nothing about whether the
// reader still wants the newest line, unlike an upward scroll (which does pause).
func TestHorizontalScrollDoesNotPauseFollow(t *testing.T) {
	m := newLogs()
	m.Append("", longLine())
	for _, a := range []keymap.Action{keymap.ActionRight, keymap.ActionLeft} {
		m, _ = m.Update(a)
		if !m.Following() {
			t.Fatalf("%v should leave following alone", a)
		}
	}
}

// TestHorizontalOffsetClampsWhenContentNarrows: the grep can hide the very line that made
// the buffer wide, so an offset the reader chose can become unreachable — it clamps back
// rather than leaving them staring at blank rows.
func TestHorizontalOffsetClampsWhenContentNarrows(t *testing.T) {
	m := newLogs()
	m.Append("", longLine())
	m.Append("", "short")
	for range 4 {
		m, _ = m.Update(keymap.ActionRight)
	}
	if m.HOffset() == 0 {
		t.Fatal("precondition: the view should be scrolled right")
	}

	m = typeFilter(m, "short")
	if m.HOffset() != 0 {
		t.Fatalf("narrowing to a short line should clamp the offset; got %d", m.HOffset())
	}
	if v := plain(m.View()); !strings.Contains(v, "short") {
		t.Errorf("the matching line should be on screen: %q", v)
	}
}

func TestInactiveIgnoresActions(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(40, 12)
	appendLines(&m, 10) // not shown
	var cmd tea.Cmd
	for _, a := range []keymap.Action{keymap.ActionBottom, keymap.ActionBack, keymap.ActionFilter, keymap.ActionLogsFollow, keymap.ActionLogsRegex, keymap.ActionLogsWrap, keymap.ActionLogsTimestamps, keymap.ActionRight} {
		m, cmd = m.Update(a)
		if cmd != nil {
			t.Errorf("inactive view emitted a cmd for %v; want nil", a)
		}
	}
	if m.View() != "" {
		t.Errorf("inactive View() = %q; want empty", m.View())
	}
}

// The LOGS-04b fixtures: two stamped lines as the stream delivers them — the timestamp
// split off by kube.SplitLogTimestamp at the wiring boundary, the message on its own.
const (
	stamp1 = "2026-07-28T16:32:01.000000001Z"
	stamp2 = "2026-07-28T16:32:02.000000002Z"
)

// newStampedLogs is the 40-column newLogs widened to 100. An RFC3339Nano stamp is 30
// columns on its own, so a stamped line does not fit the narrow default view — which is
// the width cost the toggle exists to let a reader opt into (and which LOGS-04a's wrap /
// horizontal scroll exist to cope with). These tests are about *what* is drawn, so they
// give it the room; the clipping itself is already covered by the LOGS-04a tests.
func newStampedLogs() Model {
	m := New(styles.Default())
	m.SetSize(100, 12)
	m.Show()
	return m
}

// appendStamped streams the two stamped fixture lines into m.
func appendStamped(m *Model) {
	m.Append(stamp1, "alpha error one")
	m.Append(stamp2, "beta ok two")
}

// TestTimestampsHiddenByDefault is the half of LOGS-04b that protects the default: the
// stream carries timestamps unconditionally (so the toggle can be a redraw), which must
// not mean they are *shown*. A reader who never presses the key sees exactly the lines
// they saw before this existed.
func TestTimestampsHiddenByDefault(t *testing.T) {
	m := newLogs()
	appendStamped(&m)
	if m.Timestamps() {
		t.Fatal("a fresh logs view should start with timestamps hidden")
	}
	v := plain(m.View())
	if strings.Contains(v, stamp1) || strings.Contains(v, stamp2) {
		t.Errorf("timestamps must not be drawn while the toggle is off; got:\n%s", v)
	}
	if !strings.Contains(v, "alpha error one") || !strings.Contains(v, "beta ok two") {
		t.Errorf("the messages should show regardless; got:\n%s", v)
	}
}

// TestTimestampsToggleShowsAndHides is the gesture itself: logs.timestamps puts each
// line's stamp ahead of its message and a second press takes it away again, without
// re-fetching anything — the buffer is untouched either way.
func TestTimestampsToggleShowsAndHides(t *testing.T) {
	m := newStampedLogs()
	appendStamped(&m)

	m, _ = m.Update(keymap.ActionLogsTimestamps)
	if !m.Timestamps() {
		t.Fatal("logs.timestamps should turn timestamps on")
	}
	v := plain(m.View())
	if !strings.Contains(v, stamp1+" alpha error one") {
		t.Errorf("a stamped line should render as `<stamp> <message>`; got:\n%s", v)
	}
	if !strings.Contains(v, stamp2+" beta ok two") {
		t.Errorf("every line should carry its own stamp; got:\n%s", v)
	}

	m, _ = m.Update(keymap.ActionLogsTimestamps)
	if m.Timestamps() {
		t.Fatal("a second logs.timestamps should turn them back off")
	}
	if v := plain(m.View()); strings.Contains(v, stamp1) {
		t.Errorf("hiding should remove the stamps again; got:\n%s", v)
	}
}

// TestTimestampsSurviveTheToggleWithoutRefetch: the point of making this a display
// toggle rather than a restream (D148) is that nothing is lost. Flipping it twice with a
// grep open leaves the same lines matched by the same query.
func TestTimestampsSurviveTheToggleWithoutRefetch(t *testing.T) {
	m := newLogs()
	appendStamped(&m)
	m = typeFilter(m, "error")
	before := len(m.shownLines)

	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m, _ = m.Update(keymap.ActionLogsTimestamps)

	if len(m.shownLines) != before || len(m.shownLines) != 1 {
		t.Errorf("toggling timestamps changed the match set: %d, was %d", len(m.shownLines), before)
	}
	if q := m.Query(); q != "error" {
		t.Errorf("toggling timestamps should not disturb the grep; query = %q", q)
	}
	if len(m.lines) != 2 {
		t.Errorf("toggling timestamps must not touch the buffer; %d lines", len(m.lines))
	}
}

// TestGrepNeverMatchesTheTimestamp is the load-bearing half of D148: the query is about
// the message. A timestamp fragment must not narrow the stream in either display state —
// otherwise the same query would mean different things depending on whether the clock
// happened to be on screen.
func TestGrepNeverMatchesTheTimestamp(t *testing.T) {
	for _, shown := range []bool{false, true} {
		m := newLogs()
		appendStamped(&m)
		if shown {
			m, _ = m.Update(keymap.ActionLogsTimestamps)
		}
		m = typeFilter(m, "2026-07-28")
		if len(m.shownLines) != 0 {
			t.Errorf("timestamps shown=%v: a timestamp query matched %d lines; want 0", shown, len(m.shownLines))
		}
		if v := plain(m.View()); strings.Contains(v, "alpha error one") {
			t.Errorf("timestamps shown=%v: no line should survive a timestamp-only query; got:\n%s", shown, v)
		}
	}
}

// TestStampedLineHighlightsTheMessage: with timestamps on, the highlight still lands on
// the matched span of the *message* — the stamp is a prefix, not part of the match, and
// its presence must not shift the spans onto the wrong bytes.
func TestStampedLineHighlightsTheMessage(t *testing.T) {
	m := newStampedLogs()
	appendStamped(&m)
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m = typeFilter(m, "error")

	v := m.View()
	if !strings.Contains(v, matchSpan("error")) {
		t.Errorf("the matched span of the message should be highlighted; got:\n%s", v)
	}
	if p := plain(v); !strings.Contains(p, stamp1+" alpha error one") {
		t.Errorf("the stamped, highlighted line should read back intact; got:\n%s", p)
	}
}

// TestUnstampedLineRendersVerbatimWithTimestampsOn: a line the server did not stamp (or
// one whose prefix did not parse) has no timestamp to show, so it renders exactly as it
// streamed rather than being padded with an invented one.
func TestUnstampedLineRendersVerbatimWithTimestampsOn(t *testing.T) {
	m := newStampedLogs()
	m.Append(stamp1, "stamped line")
	m.Append("", "unstamped line")
	m, _ = m.Update(keymap.ActionLogsTimestamps)

	v := plain(m.View())
	if !strings.Contains(v, stamp1+" stamped line") {
		t.Errorf("the stamped line should carry its stamp; got:\n%s", v)
	}
	if !strings.Contains(v, "unstamped line") {
		t.Errorf("the unstamped line should still show; got:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "unstamped line") && strings.Contains(line, "2026-") {
			t.Errorf("an unstamped line must not borrow a neighbour's stamp: %q", line)
		}
	}
}

// TestTimestampsGetNoHeaderMarker: unlike wrap, the toggle is visible on every row the
// moment it is on, so it does not spend a segment of a header that is clipped from the
// right at narrow widths. The header is checked for the *markers*, not the stamps —
// with timestamps on, the body legitimately contains them.
func TestTimestampsGetNoHeaderMarker(t *testing.T) {
	m := newStampedLogs()
	appendStamped(&m)
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	header := strings.SplitN(plain(m.View()), "\n", 2)[0]
	for _, marker := range []string{"[ts]", "[time]", "[timestamps]"} {
		if strings.Contains(header, marker) {
			t.Errorf("the timestamps toggle should not add a header marker; header = %q", header)
		}
	}
	if !strings.Contains(header, "[following]") {
		t.Errorf("the header should still report the follow state; header = %q", header)
	}
}

// TestIncrementalAppendMatchesAFullRebuild is the invariant LOGS-05b rests on. Appending
// no longer re-scans the buffer — it extends a cached body — so the only thing that can
// go wrong is drift: the cache saying something a full rebuild would not. Stream lines
// with a grep and timestamps in every combination and assert the cache equals what
// render() (the O(n) path) computes from the same buffer.
func TestIncrementalAppendMatchesAFullRebuild(t *testing.T) {
	for _, tc := range []struct {
		name       string
		query      string
		regex      bool
		timestamps bool
	}{
		{name: "no filter"},
		{name: "timestamps", timestamps: true},
		{name: "substring grep", query: "err"},
		{name: "substring grep with timestamps", query: "err", timestamps: true},
		{name: "regex grep", query: "e(rr|xit)", regex: true},
		{name: "regex grep that never compiled", query: "err(", regex: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newLogs()
			if tc.regex {
				m, _ = m.Update(keymap.ActionLogsRegex)
			}
			if tc.timestamps {
				m, _ = m.Update(keymap.ActionLogsTimestamps)
			}
			if tc.query != "" {
				m = typeFilter(m, tc.query)
			}
			// Stream after the modes are set, so every line takes the incremental path.
			for i, msg := range []string{"boot ok", "err disk", "steady", "exit 1", "err net"} {
				m.Append("2026-07-29T10:0"+itoa(i)+":00Z", msg)
			}
			incremental := append([]string(nil), m.shownLines...)

			m.render() // the full rebuild

			if len(incremental) != len(m.shownLines) {
				t.Fatalf("cached body has %d lines, a rebuild has %d", len(incremental), len(m.shownLines))
			}
			for i := range incremental {
				if incremental[i] != m.shownLines[i] {
					t.Errorf("line %d drifted:\n cached = %q\nrebuilt = %q", i, incremental[i], m.shownLines[i])
				}
			}
		})
	}
}

// TestAppendBatchEqualsLineByLineAppend: batching is a cost change, not a behaviour
// change (LOGS-05b). A batch of lines must leave exactly the view that appending them one
// at a time leaves — same body, same match count, same frame.
func TestAppendBatchEqualsLineByLineAppend(t *testing.T) {
	lines := []Line{
		{Stamp: "2026-07-29T10:00:00Z", Message: "boot ok"},
		{Stamp: "2026-07-29T10:00:01Z", Message: "err disk"},
		{Stamp: "2026-07-29T10:00:02Z", Message: "steady"},
	}

	one := typeFilter(newLogs(), "err")
	for _, l := range lines {
		one.Append(l.Stamp, l.Message)
	}

	batched := typeFilter(newLogs(), "err")
	batched.AppendBatch(lines)

	if len(one.shownLines) != len(batched.shownLines) {
		t.Fatalf("batched kept %d lines, one-by-one kept %d", len(batched.shownLines), len(one.shownLines))
	}
	if one.View() != batched.View() {
		t.Errorf("batched frame differs:\none-by-one:\n%s\nbatched:\n%s", plain(one.View()), plain(batched.View()))
	}
	if len(batched.lines) != len(lines) {
		t.Errorf("batched buffer holds %d lines; want %d", len(batched.lines), len(lines))
	}
}

// TestAppendBatchOfNothingIsANoOp guards the pump's degenerate call: an empty batch must
// not touch the buffer, the body or the scroll position.
func TestAppendBatchOfNothingIsANoOp(t *testing.T) {
	m := newLogs()
	appendLines(&m, 3)
	before := m.View()
	m.AppendBatch(nil)
	if len(m.lines) != 3 || m.View() != before {
		t.Errorf("an empty batch changed the view: %d lines\n%s", len(m.lines), plain(m.View()))
	}
}

// TestSetStylesRepaintsPaintedHighlights is the trap this component's SetStyles exists
// to avoid (M4-12b-1). shownLines is a *painted* cache — since LOGS-05b each kept line
// is stored with its rendered escapes already baked in — so a SetStyles that only
// assigned the field would leave every stamp on screen in the departed theme's Subtle
// color while the header moved to the new one, and only lines streamed afterwards would
// follow. Both moving parts are theme-dependent: the stamp is painted in Subtle, and on
// a dark pair of themes the highlight is too — LOGS-SEL-04 re-painted styles.Match
// (canvas-on-Warn, weight underneath), ending THEME-05's theme-independent interlude —
// so the highlight's escapes differ between default and monokai again. Both ride the
// same painted cache, so their change is what proves the rebuild. Each assertion is
// against the theme's own rendering, so it states the invariant rather than an escape
// sequence.
func TestSetStylesRepaintsPaintedHighlights(t *testing.T) {
	m := newStampedLogs()
	m.Append(stamp1, "GET /healthz 200")
	m.Append(stamp2, "POST /api/v1 500")
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m = typeFilter(m, "0")
	if !strings.Contains(m.View(), matchSpan("0")) {
		t.Fatal("precondition: the matched span should be highlighted in the default theme")
	}
	if want := styles.Default().Subtle.Render(stamp1 + " "); !strings.Contains(m.View(), want) {
		t.Fatalf("precondition: the first line's stamp should be painted in the default theme's Subtle; want %q in:\n%q",
			want, m.View())
	}

	mono := styles.New(styles.MonokaiTheme())
	m.SetStyles(mono)

	if want := mono.Match.Render("0"); !strings.Contains(m.View(), want) {
		t.Errorf("the highlight was dropped by the restyle; want the span rendered as %q in:\n%q",
			want, m.View())
	}
	if want := mono.Subtle.Render(stamp1 + " "); !strings.Contains(m.View(), want) {
		t.Errorf("the stamp kept the old theme's color after SetStyles; want it rendered as %q in:\n%q",
			want, m.View())
	}
	if want := styles.Default().Subtle.Render(stamp1 + " "); strings.Contains(m.View(), want) {
		t.Error("the old theme's stamp is still on screen — the painted cache was not rebuilt")
	}
	// The repaint is a rebuild, not a reset: the buffer, the query and what it narrows to
	// all survive.
	if q := m.Query(); q != "0" {
		t.Errorf("the grep query did not survive the restyle: %q", q)
	}
	if got := plain(m.View()); !strings.Contains(got, "POST /api/v1 500") || !strings.Contains(got, "GET /healthz 200") {
		t.Errorf("the restyle changed which lines the grep keeps:\n%s", got)
	}
}

// TestRestreamEmptiesTheBufferButKeepsTheLens is the contract the previous-instance
// toggle rests on (M5-01a): a restream of the same container's other instance replaces
// the lines — they are a different log — while leaving every reader gesture in place, so
// flipping instances asks the same question of the other log instead of resetting the
// question. Following is re-armed, because the new stream tails from its own start.
func TestRestreamEmptiesTheBufferButKeepsTheLens(t *testing.T) {
	m := newLogs()
	appendStamped(&m)
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m, _ = m.Update(keymap.ActionLogsWrap)
	m = typeFilter(m, "error")
	m, _ = m.Update(keymap.ActionLogsFollow) // pause, so the re-arm is observable

	m.Restream()

	if v := plain(m.View()); strings.Contains(v, "alpha error one") {
		t.Errorf("a restream should empty the buffer; got:\n%s", v)
	}
	if !m.Timestamps() || !m.Wrap() || !m.Filtering() || m.Query() != "error" {
		t.Errorf("a restream must keep the reader's lens: stamps=%v wrap=%v filtering=%v query=%q",
			m.Timestamps(), m.Wrap(), m.Filtering(), m.Query())
	}
	if !m.Following() {
		t.Error("a restream should re-arm following — the new stream tails from its start")
	}
}

// TestResetClearsTheLensToo is the other half: Reset is the *new object* path, so unlike
// a restream it drops the modes with the lines. The two are one implementation (Reset
// calls Restream), and this is what keeps them distinguishable.
func TestResetClearsTheLensToo(t *testing.T) {
	m := newLogs()
	appendStamped(&m)
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	m, _ = m.Update(keymap.ActionLogsWrap)
	m = typeFilter(m, "error")
	m.SetPrevious(true)

	m.Reset()

	if v := plain(m.View()); strings.Contains(v, "alpha error one") {
		t.Errorf("Reset should empty the buffer; got:\n%s", v)
	}
	if m.Timestamps() || m.Wrap() || m.Filtering() || m.Query() != "" || m.Previous() {
		t.Errorf("Reset should clear the lens: stamps=%v wrap=%v filtering=%v query=%q previous=%v",
			m.Timestamps(), m.Wrap(), m.Filtering(), m.Query(), m.Previous())
	}
	if !m.Following() {
		t.Error("Reset should leave the view tailing")
	}
}

// TestPreviousMarkerNamesTheInstance: unlike the timestamps toggle beside it, which gets
// no header marker on purpose (it restates the body), which *instance* is on screen is
// invisible in the lines themselves — two runs of one container look alike — so the
// header has to say it, and say it ahead of the follow state (D146).
func TestPreviousMarkerNamesTheInstance(t *testing.T) {
	m := newLogs()
	appendLines(&m, 2)
	if v := plain(m.View()); strings.Contains(v, "[previous]") {
		t.Errorf("the running instance needs no marker; got:\n%s", v)
	}

	m.SetPrevious(true)
	v := plain(m.View())
	if !strings.Contains(v, "[previous]") {
		t.Errorf("the previous instance should be named in the header; got:\n%s", v)
	}
	if strings.Index(v, "[previous]") > strings.Index(v, "[following]") {
		t.Errorf("the instance marker should precede the follow state; got:\n%s", v)
	}
}

// --- LOGS-SEL-02: visual mode and yank (feedback 2026-08-07-logs-selection-and-yank) ---

// selectUp anchors a selection at the cursor and extends it n lines upward, which is the
// natural direction in a log: the interesting lines are the ones just before the one you
// stopped on.
func selectUp(m Model, n int) Model {
	m, _ = m.Update(keymap.ActionLogsSelect)
	for range n {
		m, _ = m.Update(keymap.ActionUp)
	}
	return m
}

// TestSelectionExtendsWithTheCursor is the shape of visual mode: `v` fixes one end, the
// nav keys move the other, and the range between them is what a yank takes. A selection
// of one line — what `v` alone makes — is the state the yank falls back to anyway, so the
// assertion that matters is that the *second* key extends rather than moves.
func TestSelectionExtendsWithTheCursor(t *testing.T) {
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionUp) // pause, cursor on line-4 (index 3)

	m, _ = m.Update(keymap.ActionLogsSelect)
	if !m.Selecting() {
		t.Fatal("logs.select should start a selection")
	}
	lo, hi, ok := m.Selection()
	if !ok || lo != 3 || hi != 3 {
		t.Fatalf("a fresh selection is the cursor's own line; got %d..%d (ok=%v)", lo, hi, ok)
	}
	m, _ = m.Update(keymap.ActionUp)
	m, _ = m.Update(keymap.ActionUp)
	if lo, hi, _ = m.Selection(); lo != 1 || hi != 3 {
		t.Fatalf("nav.up should extend the selection upward; got %d..%d, want 1..3", lo, hi)
	}
	// Moving back down shrinks it — the anchor is fixed, the cursor is not.
	m, _ = m.Update(keymap.ActionDown)
	if lo, hi, _ = m.Selection(); lo != 2 || hi != 3 {
		t.Fatalf("nav.down should shrink it back; got %d..%d, want 2..3", lo, hi)
	}
	// Every line of the range wears the bar, not just the cursor's.
	v := m.View()
	for _, want := range []string{"line-3", "line-4"} {
		if !strings.Contains(v, cursorBar(want)) {
			t.Errorf("%s should be inside the selection bar; got:\n%q", want, v)
		}
	}
	if strings.Contains(v, cursorBar("line-2")) {
		t.Errorf("line-2 is outside the selection and must not wear the bar; got:\n%q", v)
	}
}

// TestYankCopiesTheSelectionUnpainted is the requirement the feedback called out as the
// one that "looks right on screen and only shows up once the text is pasted somewhere":
// the clipboard gets the raw buffered lines, with no escape sequence in them, even though
// every one of those lines is on screen inside a Selection bar and carrying Match marks.
func TestYankCopiesTheSelectionUnpainted(t *testing.T) {
	m := newLogs()
	for _, l := range []string{"boot ok", "err disk", "err net", "done"} {
		m.Append("", l)
	}
	m = typeFilter(m, "err")           // shown: [err disk, err net], cursor on err net
	m, _ = m.Update(keymap.ActionBack) // close the field; the query is cleared with it
	m, _ = m.Update(keymap.ActionUp)   // pause on "err net" (index 2 of the full set)
	m = selectUp(m, 1)                 // select "err disk".."err net"

	text, n, ok := m.Yank()
	if !ok || n != 2 {
		t.Fatalf("Yank() = %q, %d, %v; want two lines", text, n, ok)
	}
	if want := "err disk\nerr net"; text != want {
		t.Errorf("yanked %q; want %q", text, want)
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("the clipboard must carry no styling; got %q", text)
	}
}

// TestYankUnderAGrepTakesTheMatchedLinesOnly: the selection is over what the query
// *displays* (the feedback's wording), so a range spanning a hidden line does not smuggle
// it into the clipboard — and the copied text still carries no highlight, though every
// line in it is painted with one on screen.
func TestYankUnderAGrepTakesTheMatchedLinesOnly(t *testing.T) {
	m := newLogs()
	for _, l := range []string{"err disk", "steady", "err net"} {
		m.Append("", l)
	}
	m = typeFilter(m, "err") // shown: [err disk, err net]; "steady" is between them
	if len(m.shownLines) != 2 {
		t.Fatalf("precondition: shown = %d lines, want 2", len(m.shownLines))
	}
	// logs.follow pauses without moving, so the cursor stays on the newest shown line.
	m, _ = m.Update(keymap.ActionLogsFollow)
	m = selectUp(m, 1) // the whole shown set

	if v := m.View(); !strings.Contains(v, matchSpan("err")) {
		t.Fatalf("precondition: the selected lines should still show their matches; got:\n%q", v)
	}
	text, n, ok := m.Yank()
	if !ok || n != 2 {
		t.Fatalf("Yank() = %q, %d, %v; want two lines", text, n, ok)
	}
	if want := "err disk\nerr net"; text != want {
		t.Errorf("a selection spanning a filtered-out line must not copy it; got %q, want %q", text, want)
	}
}

// TestYankWithNoSelectionTakesTheCursorLine: `y` is useful without `v`, which is the
// common case — you scrolled to the line you want and want that one line.
func TestYankWithNoSelectionTakesTheCursorLine(t *testing.T) {
	m := newLogs()
	appendLines(&m, 4)
	m, _ = m.Update(keymap.ActionUp) // pause on line-3
	if m.Selecting() {
		t.Fatal("precondition: no selection")
	}
	text, n, ok := m.Yank()
	if !ok || n != 1 || text != "line-3" {
		t.Errorf("Yank() = %q, %d, %v; want \"line-3\", 1, true", text, n, ok)
	}
}

// TestYankOfAWrappedLineIsWhole is why the cursor counts log lines rather than screen
// rows (D242 pt 1): the wrapped line occupies three rows and comes back as one, with no
// break the log did not have. It is the defect the feedback filed against the terminal's
// own select-to-copy.
func TestYankOfAWrappedLineIsWhole(t *testing.T) {
	long := "HEAD" + strings.Repeat("-", 90) + "TAIL"
	m := newLogs() // 40 columns wide
	m.Append("", "short one")
	m.Append("", long)
	m, _ = m.Update(keymap.ActionLogsWrap)
	if got := lineRows(m.shownLines[1], m.viewport.Width()); got < 3 {
		t.Fatalf("precondition: the long line should wrap onto several rows; got %d", got)
	}
	m, _ = m.Update(keymap.ActionLogsFollow) // pause with the cursor on the long line
	text, n, ok := m.Yank()
	if !ok || n != 1 {
		t.Fatalf("Yank() = %q, %d, %v; want one line", text, n, ok)
	}
	if text != long {
		t.Errorf("a wrapped line must yank whole and unbroken; got %q", text)
	}
	if strings.Contains(text, "\n") {
		t.Error("the yank inserted a break the log never had")
	}
}

// TestYankMatchesTheTimestampsToggle: the copy is what is on screen, so the stamp comes
// with it exactly when logs.timestamps is showing it — and never otherwise, since the
// buffer holds it either way.
func TestYankMatchesTheTimestampsToggle(t *testing.T) {
	m := newStampedLogs()
	appendStamped(&m)
	m, _ = m.Update(keymap.ActionUp) // pause on the newest line

	text, _, ok := m.Yank()
	if !ok {
		t.Fatal("Yank() should copy the cursor's line")
	}
	if strings.Contains(text, "T12:00:0") {
		t.Errorf("timestamps are off; the copy must not carry one: %q", text)
	}
	m, _ = m.Update(keymap.ActionLogsTimestamps)
	stamped, _, _ := m.Yank()
	if !strings.HasPrefix(stamped, m.stamps[m.shownIdx[m.Cursor()]]+" ") {
		t.Errorf("with timestamps on the copy should be stamped as the screen is: %q", stamped)
	}
	if !strings.HasSuffix(stamped, text) {
		t.Errorf("the stamp is a prefix, not a replacement: %q vs %q", stamped, text)
	}
}

// TestVisualModePausesFollowAndGivesItBack is the rule LOGS-SEL-02 rests on (D242 pt 5):
// a selection cannot be held while following, because following owns the cursor — so `v`
// suspends it, and finishing puts it back, because a reader who was tailing when they
// copied a line meant to go on tailing.
func TestVisualModePausesFollowAndGivesItBack(t *testing.T) {
	m := newLogs()
	appendLines(&m, 3)
	if !m.Following() {
		t.Fatal("precondition: a fresh view tails")
	}
	m, _ = m.Update(keymap.ActionLogsSelect)
	if m.Following() {
		t.Fatal("entering visual mode must pause following")
	}
	// The stream keeps arriving while the selection stands, and must not drag it.
	m.Append("", "line-4")
	if lo, hi, _ := m.Selection(); lo != 2 || hi != 2 {
		t.Errorf("an arriving line must not move the selection; got %d..%d, want 2..2", lo, hi)
	}
	if _, _, ok := m.Yank(); !ok {
		t.Fatal("Yank() should copy")
	}
	if !m.Following() {
		t.Error("a yank should hand the stream back to the reader who was tailing")
	}
	if m.Selecting() {
		t.Error("a yank ends visual mode")
	}
}

// TestVisualModeDoesNotResumeAFollowTheReaderPaused: the resume is the undo of the pause
// visual mode itself did, never a decision about a view the reader had already parked.
func TestVisualModeDoesNotResumeAFollowTheReaderPaused(t *testing.T) {
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionTop) // the reader's own pause
	if m.Following() {
		t.Fatal("precondition: paused")
	}
	m = selectUp(m, 0)
	m, _ = m.Update(keymap.ActionBack) // abandon the selection
	if m.Selecting() {
		t.Fatal("nav.back should abandon the selection")
	}
	if m.Following() {
		t.Error("leaving visual mode must not resume a follow the reader turned off")
	}
}

// TestBackLaddersThroughTheSelection: esc undoes the innermost thing the reader turned
// on — the grep, then the selection, then the view itself — so no rung ever costs them
// the one below it.
func TestBackLaddersThroughTheSelection(t *testing.T) {
	m := newLogs()
	appendLines(&m, 4)
	m, _ = m.Update(keymap.ActionUp)
	m = selectUp(m, 1)
	m = typeFilter(m, "line") // a grep opened over a live selection

	m, cmd := m.Update(keymap.ActionBack)
	if m.Filtering() || cmd != nil {
		t.Fatalf("the first esc clears the grep; filtering=%v cmd=%v", m.Filtering(), cmd != nil)
	}
	if !m.Selecting() {
		t.Fatal("clearing the grep must not take the selection with it")
	}
	m, cmd = m.Update(keymap.ActionBack)
	if m.Selecting() || cmd != nil {
		t.Fatalf("the second esc abandons the selection; selecting=%v cmd=%v", m.Selecting(), cmd != nil)
	}
	m, cmd = m.Update(keymap.ActionBack)
	if cmd == nil {
		t.Fatal("the third esc closes the view")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("expected ClosedMsg; got %T", cmd())
	}
}

// TestSecondSelectCancels: `v` is a toggle, as it is in vim — the way out a reader finds
// by pressing the key they pressed to get in.
func TestSecondSelectCancels(t *testing.T) {
	m := newLogs()
	appendLines(&m, 4)
	m, _ = m.Update(keymap.ActionUp)
	m = selectUp(m, 2)
	if lo, hi, _ := m.Selection(); lo == hi {
		t.Fatalf("precondition: a multi-line selection; got %d..%d", lo, hi)
	}
	m, _ = m.Update(keymap.ActionLogsSelect)
	if m.Selecting() {
		t.Fatal("a second logs.select should cancel the selection")
	}
	if lo, hi, _ := m.Selection(); lo != hi || lo != m.Cursor() {
		t.Errorf("with no selection the range is the cursor's line; got %d..%d, cursor %d", lo, hi, m.Cursor())
	}
}

// TestBottomExtendsTheSelectionInsteadOfResumingFollow: `G` is the one nav key whose
// meaning changes inside visual mode. Outside it, it re-arms following (LOGS-04c); inside
// it, re-arming would hand the cursor to the stream and the selection with it — so it
// extends to the last line instead, which is what makes `gg v G y` copy the buffer.
func TestBottomExtendsTheSelectionInsteadOfResumingFollow(t *testing.T) {
	m := newLogs()
	appendLines(&m, 6)
	m, _ = m.Update(keymap.ActionTop) // pause on line-1
	m, _ = m.Update(keymap.ActionLogsSelect)
	m, _ = m.Update(keymap.ActionBottom)

	if m.Following() {
		t.Error("nav.bottom inside a selection must not re-arm following")
	}
	if !m.Selecting() {
		t.Fatal("nav.bottom must not end the selection")
	}
	if lo, hi, _ := m.Selection(); lo != 0 || hi != 5 {
		t.Fatalf("nav.bottom should extend to the last line; got %d..%d, want 0..5", lo, hi)
	}
	text, n, _ := m.Yank()
	if n != 6 || !strings.HasPrefix(text, "line-1\n") || !strings.HasSuffix(text, "\nline-6") {
		t.Errorf("`gg v G y` should copy the whole buffer; got %d lines: %q", n, text)
	}
}

// TestFollowEndsTheSelection: `f` is "back to the stream", and following owns the cursor,
// so the selection cannot survive it. It resumes rather than toggling into a second
// pause — from visual mode the view is always paused, so the toggle has only one honest
// reading.
func TestFollowEndsTheSelection(t *testing.T) {
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionUp)
	m = selectUp(m, 2)
	m, _ = m.Update(keymap.ActionLogsFollow)
	if m.Selecting() {
		t.Error("logs.follow should end the selection")
	}
	if !m.Following() {
		t.Error("logs.follow out of visual mode should resume the stream")
	}
	if m.Cursor() != len(m.shownLines)-1 {
		t.Errorf("resuming pins the cursor to the newest line; got %d", m.Cursor())
	}
}

// TestSelectionKeepsItsLogLinesAcrossAFilterChange: both ends of the range are log lines,
// so a keystroke in the grep narrows the selection to the ones that survive rather than
// leaving either end pointing at whatever the new query put at that index.
// It is deliberately set up so that clamping the anchor into the new body would give a
// *different* answer than re-finding it: the anchor sits in the middle of the buffer with
// a still-shown line after it, so an unclamped stale index stays in range and silently
// swallows a line the reader never selected.
func TestSelectionKeepsItsLogLinesAcrossAFilterChange(t *testing.T) {
	m := newLogs()
	for _, l := range []string{"err a", "x1", "err b", "x2", "x3", "x4", "err c"} {
		m.Append("", l)
	}
	m, _ = m.Update(keymap.ActionLogsFollow) // pause on "err c" (6) without moving
	for range 4 {
		m, _ = m.Update(keymap.ActionUp) // up to "err b" (2)
	}
	m = selectUp(m, 2) // anchor "err b" (2), cursor up to "err a" (0)
	if lo, hi, _ := m.Selection(); lo != 0 || hi != 2 {
		t.Fatalf("precondition: selection %d..%d, want 0..2", lo, hi)
	}

	m = typeFilter(m, "err") // shown becomes [err a, err b, err c]
	if lo, hi, _ := m.Selection(); lo != 0 || hi != 1 {
		t.Fatalf("the selection should still end on `err b`; got %d..%d, want 0..1", lo, hi)
	}
	text, n, _ := m.Yank()
	if n != 2 || text != "err a\nerr b" {
		t.Errorf("yanked %d lines %q; want the two the reader had selected", n, text)
	}
}

// TestVisualModeIsNamedInTheHeader: a fresh `v` selects the line the cursor was already
// on, so nothing on screen changes — the header is the only evidence that the next `j`
// will extend rather than move, and the count is the only way to size a selection taller
// than the pane.
func TestVisualModeIsNamedInTheHeader(t *testing.T) {
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionUp)
	if v := plain(m.View()); strings.Contains(v, "[visual") {
		t.Errorf("no selection, no marker; got:\n%s", v)
	}
	m, _ = m.Update(keymap.ActionLogsSelect)
	if v := plain(m.View()); !strings.Contains(v, "[visual 1]") {
		t.Errorf("a fresh selection should be named and sized; got:\n%s", v)
	}
	m, _ = m.Update(keymap.ActionUp)
	m, _ = m.Update(keymap.ActionUp)
	if v := plain(m.View()); !strings.Contains(v, "[visual 3]") {
		t.Errorf("the marker should size the selection; got:\n%s", v)
	}
	m, _ = m.Update(keymap.ActionBack)
	if v := plain(m.View()); strings.Contains(v, "[visual") {
		t.Errorf("abandoning the selection should clear the marker; got:\n%s", v)
	}
}

// TestSelectionCannotOutliveItsLines: Reset and Restream empty the buffer, and a range
// over lines that no longer exist is the one way this could copy the wrong log.
func TestSelectionCannotOutliveItsLines(t *testing.T) {
	for name, clear := range map[string]func(*Model){
		"Reset":    (*Model).Reset,
		"Restream": (*Model).Restream,
	} {
		t.Run(name, func(t *testing.T) {
			m := newLogs()
			appendLines(&m, 5)
			m, _ = m.Update(keymap.ActionUp)
			m = selectUp(m, 2)
			clear(&m)
			if m.Selecting() {
				t.Error("emptying the buffer must end visual mode")
			}
			if _, _, ok := m.Yank(); ok {
				t.Error("there is nothing to yank from an empty view")
			}
		})
	}
	// A grep that keeps nothing is the same state reached without emptying the buffer.
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionUp)
	m = selectUp(m, 2)
	m = typeFilter(m, "zzz")
	if m.Selecting() {
		t.Error("a grep that keeps no line leaves no selection")
	}
	if _, _, ok := m.Yank(); ok {
		t.Error("an empty body yanks nothing")
	}
}

// TestSelectionIsNotBakedIntoTheAppendCache extends LOGS-05b's invariant over the whole
// range rather than the one cursor line: shownLines stays what a rebuild would produce,
// so a selection is not an invalidation of it — and the raw text a yank reads stays raw.
func TestSelectionIsNotBakedIntoTheAppendCache(t *testing.T) {
	m := newLogs()
	appendLines(&m, 5)
	m, _ = m.Update(keymap.ActionUp)
	m = selectUp(m, 3)
	for i, s := range m.shownLines {
		if strings.Contains(s, "\x1b") {
			t.Errorf("cached line %d carries styling: %q", i, s)
		}
	}
	if v := m.View(); !strings.Contains(v, cursorBar("line-2")) {
		t.Errorf("the bar should still reach the frame; got:\n%q", v)
	}
}

// streamLines batches "line-<from>".."line-<to>" into m the way the log pump does, in
// runs of 500 — one syncContent per run rather than per line, which is what makes a
// volume test cheap enough to be a unit test.
func streamLines(m *Model, from, to int) {
	const batch = 500
	for i := from; i <= to; i += batch {
		var b []Line
		for j := i; j < i+batch && j <= to; j++ {
			b = append(b, Line{Message: "line-" + itoa(j)})
		}
		m.AppendBatch(b)
	}
}

// TestBufferIsBounded is LOGS-07's whole point: a followed stream must not grow for as
// long as the view is open. Nothing else bounded it — TailLines bounds the replay before
// the tail, not the tail (D230) — so at ~1,900 lines/sec an afternoon's follow was an
// afternoon's worth of RAM.
func TestBufferIsBounded(t *testing.T) {
	m := newLogs()
	streamLines(&m, 1, MaxLines+3*trimChunk)
	if got := len(m.lines); got > MaxLines+trimChunk {
		t.Errorf("held %d lines; want at most %d (MaxLines+trimChunk)", got, MaxLines+trimChunk)
	}
	if got := len(m.stamps); got != len(m.lines) {
		t.Errorf("stamps (%d) must stay parallel to lines (%d)", got, len(m.lines))
	}
	if got := len(m.shownLines); got != len(m.lines) {
		t.Errorf("with no grep every held line is shown: shown %d, held %d", got, len(m.lines))
	}
	// It is the *oldest* that go: the newest line is the one a follower is reading.
	if m.lines[0] == "line-1" {
		t.Error("the buffer dropped nothing — the trim never ran")
	}
	if last := m.lines[len(m.lines)-1]; last != "line-"+itoa(MaxLines+3*trimChunk) {
		t.Errorf("newest held line = %q; the trim must drop from the top", last)
	}
	if !m.trimmed {
		t.Error("a trimmed buffer must know it was trimmed")
	}
}

// TestTrimKeepsShownIdxAddressingTheRightText: shownIdx is the only sanctioned route from
// a cursor to raw text (D242 pt 2), and a trim renumbers every buffer index under it. Off
// by the dropped count, a yank would copy some other line — silently, since both are log
// lines.
func TestTrimKeepsShownIdxAddressingTheRightText(t *testing.T) {
	m := newLogs()
	m = typeFilter(m, "7")
	streamLines(&m, 1, MaxLines+2*trimChunk)
	if len(m.shownIdx) != len(m.shownLines) {
		t.Fatalf("shownIdx (%d) and shownLines (%d) must stay parallel", len(m.shownIdx), len(m.shownLines))
	}
	if len(m.shownIdx) == 0 {
		t.Fatal("the grep should keep something")
	}
	for n, i := range m.shownIdx {
		if i < 0 || i >= len(m.lines) {
			t.Fatalf("shownIdx[%d] = %d, out of a buffer of %d", n, i, len(m.lines))
		}
		if !strings.Contains(m.lines[i], "7") {
			t.Fatalf("shownIdx[%d] points at %q, which the grep does not keep", n, m.lines[i])
		}
	}
	// The yank reads through the same map, so it is the end-to-end check of it.
	text, _, ok := m.Yank()
	if !ok || !strings.Contains(text, "7") {
		t.Errorf("yank after a trim = %q (ok=%v); want the cursor's matched line", text, ok)
	}
}

// TestTrimHoldsThePausedReaderStill: while paused, the reader is standing on lines that a
// trim slides out from under them — the body loses rows off its top, so the same viewport
// offset points somewhere else. The offset has to come down with it or a paused reader is
// scrolled by the stream, which is exactly what pausing is for.
func TestTrimHoldsThePausedReaderStill(t *testing.T) {
	m := newLogs()
	streamLines(&m, 1, MaxLines)
	m, _ = m.Update(keymap.ActionUp) // pause, cursor off the tail
	for range 200 {
		m, _ = m.Update(keymap.ActionUp)
	}
	if m.Following() {
		t.Fatal("nav.up must pause following")
	}
	// The body only: the header is *expected* to change, since it grows the [trimmed]
	// marker this leg added.
	body := func(m Model) string { _, rest, _ := strings.Cut(plain(m.View()), "\n"); return rest }
	before := body(m)
	streamLines(&m, MaxLines+1, MaxLines+2*trimChunk)
	if !m.trimmed {
		t.Fatal("this test needs the trim to have run")
	}
	if after := body(m); after != before {
		t.Errorf("a trim moved a paused reader's frame:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestTrimmedIsNamedInTheHeader: past a trim the top of the body is no longer the top of
// the stream, so `gg` lands mid-log in something that looks like its start. That is state
// with no other evidence on screen (D146). A re-stream starts a new log, so it clears.
func TestTrimmedIsNamedInTheHeader(t *testing.T) {
	m := newLogs()
	streamLines(&m, 1, 10)
	if h := plain(m.View()); strings.Contains(h, "[trimmed]") {
		t.Errorf("an untrimmed buffer must not claim otherwise; got:\n%s", h)
	}
	streamLines(&m, 11, MaxLines+2*trimChunk)
	if h := plain(m.View()); !strings.Contains(h, "[trimmed]") {
		t.Errorf("header should say the oldest lines are gone; got:\n%s", h)
	}
	m.Restream()
	if h := plain(m.View()); strings.Contains(h, "[trimmed]") {
		t.Errorf("a re-streamed buffer has dropped nothing; got:\n%s", h)
	}
}

// TestTrimNarrowsASelectionInsteadOfSlidingIt: a selection counts shown lines, so a trim
// renumbers both its ends. Sliding them by the wrong amount would silently re-aim a range
// the reader is about to copy; a far end that streamed off the top narrows to the oldest
// line left, the same answer a grep that hides it gives (rebuildShown).
func TestTrimNarrowsASelectionInsteadOfSlidingIt(t *testing.T) {
	m := newLogs()
	streamLines(&m, 1, MaxLines)
	m = selectUp(m, 3) // four lines, ending at the newest
	lo, hi, ok := m.Selection()
	if !ok || hi-lo+1 != 4 {
		t.Fatalf("selection = (%d,%d,%v); want four lines", lo, hi, ok)
	}
	want, _, _ := m.Yank()
	m = selectUp(m, 3) // re-arm the same range: Yank left visual mode

	streamLines(&m, MaxLines+1, MaxLines+2*trimChunk)
	if !m.trimmed {
		t.Fatal("this test needs the trim to have run")
	}
	got, n, ok := m.Yank()
	if !ok {
		t.Fatal("the selection should survive a trim of lines it does not cover")
	}
	if got != want || n != 4 {
		t.Errorf("yank after a trim = %q (%d lines); want the same four lines %q", got, n, want)
	}
}
