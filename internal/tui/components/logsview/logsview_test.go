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
// is stored with the Match escape sequences already wrapped around its matched spans —
// so a SetStyles that only assigned the field would leave every highlight on screen in
// the departed theme's colors while the header moved to the new one, and only lines
// streamed afterwards would follow. The assertion is against the theme's own rendering
// of the span, so it states the invariant rather than an escape sequence.
func TestSetStylesRepaintsPaintedHighlights(t *testing.T) {
	m := newLogs()
	m.Append("", "GET /healthz 200")
	m.Append("", "POST /api/v1 500")
	m = typeFilter(m, "500")
	if !strings.Contains(m.View(), matchSpan("500")) {
		t.Fatal("precondition: the matched span should be highlighted in the default theme")
	}

	mono := styles.New(styles.MonokaiTheme())
	m.SetStyles(mono)

	if want := mono.Match.Render("500"); !strings.Contains(m.View(), want) {
		t.Errorf("the highlight kept the old theme's colors after SetStyles; want the span rendered as %q in:\n%q",
			want, m.View())
	}
	if strings.Contains(m.View(), matchSpan("500")) {
		t.Error("the old theme's highlight is still on screen — the painted cache was not rebuilt")
	}
	// The repaint is a rebuild, not a reset: the buffer, the query and what it narrows to
	// all survive.
	if q := m.Query(); q != "500" {
		t.Errorf("the grep query did not survive the restyle: %q", q)
	}
	if got := plain(m.View()); !strings.Contains(got, "POST /api/v1 500") || strings.Contains(got, "GET /healthz 200") {
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
