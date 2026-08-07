package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/logsview"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// These cover what LOGS-02 adds on top of the M3-05/06/07 logs behaviour already
// exercised in tui_test.go: the live grep's raw-key routing, the two-state hint
// context, and the fact that a `q` typed into the grep is text rather than a close.

// filterKey is the default app.filter key (`/`) — the logs view's live grep.
var filterKey = tea.Key{Code: '/', Text: "/"}

// regexKey is the default logs.regex chord (`ctrl+r`) — the grep's mode toggle. It
// carries no text on purpose, which is what lets it act while the grep field is open.
var regexKey = tea.Key{Code: 'r', Mod: tea.ModCtrl}

// wrapKey is the default logs.wrap key (`w`) — the long-line toggle (LOGS-04a). Unlike
// the regex chord it is an ordinary letter, so the open grep types it.
var wrapKey = tea.Key{Code: 'w', Text: "w"}

// endKey is the default nav.bottom key (`G`) — jump to the newest line, which in this
// view also re-arms following (LOGS-04c). An ordinary letter, so the open grep types it.
var endKey = tea.Key{Code: 'G', Text: "G"}

// frame is the rendered screen with styling removed. LOGS-03 highlights matched spans,
// so a matching line is no longer a contiguous run of bytes in the frame — content
// assertions have to strip first.
func frame(m Model) string { return ansi.Strip(m.View().Content) }

// openLogsWithLines opens the logs view over a pod row and streams the given lines
// into it, returning the model with the view up and following.
func openLogsWithLines(t *testing.T, lines ...string) Model {
	t.Helper()
	events := make([]kube.LogEvent, 0, len(lines))
	for _, l := range lines {
		events = append(events, kube.LogEvent{Line: l})
	}
	m := logsViewerModel(t, &fakeLogStreamer{events: events})
	m = openLogsViewHelper(t, m)
	if !m.logsView.Active() {
		t.Fatal("precondition: the logs view should be open")
	}
	return m
}

// typeInto feeds each rune of s through the root model as a live keypress.
func typeInto(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	return m
}

// TestLogsGrepNarrowsStreamedLines proves the headline of the feedback this line
// answers: `/` opens the live grep and typing narrows the *already streamed* lines
// without re-fetching, with the header reporting matched/total.
func TestLogsGrepNarrowsStreamedLines(t *testing.T) {
	m := openLogsWithLines(t, "GET /healthz 200", "POST /api/v1 500", "GET /metrics 200")

	m, _ = press(t, m, filterKey)
	if !m.logsView.Filtering() {
		t.Fatal("app.filter should open the logs grep")
	}
	m = typeInto(t, m, "500")
	if q := m.logsView.Query(); q != "500" {
		t.Fatalf("the grep query = %q, want 500", q)
	}
	view := frame(m)
	if !strings.Contains(view, "POST /api/v1 500") {
		t.Fatalf("the matching line should stay on screen: %q", view)
	}
	if strings.Contains(view, "/healthz") || strings.Contains(view, "/metrics") {
		t.Fatalf("non-matching lines should be filtered out: %q", view)
	}
	if !strings.Contains(view, "1/3") {
		t.Fatalf("the header should report matched/total: %q", view)
	}

	// esc clears the grep and restores the whole stream; a second esc closes the view.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.logsView.Filtering() {
		t.Fatal("esc should close the grep field")
	}
	if view := frame(m); !strings.Contains(view, "/healthz") {
		t.Fatalf("clearing the grep should restore the full stream: %q", view)
	}
	if !m.logsView.Active() {
		t.Fatal("the esc that cleared the grep must not also close the view")
	}
}

// TestLogsGrepSwallowsTextKeys proves the routing split: while the grep is open every
// text-producing key types instead of firing, so `q` does not quit, `f` does not
// toggle follow and `/` does not re-open the field — the same contract the search
// view's always-open query field has (D140 pt 1) and the reason the hint narrows.
func TestLogsGrepSwallowsTextKeys(t *testing.T) {
	m := openLogsWithLines(t, "q f / line")
	m, _ = press(t, m, filterKey)
	following := m.logsView.Following()

	m = typeInto(t, m, "qf/")
	if q := m.logsView.Query(); q != "qf/" {
		t.Fatalf("text keys should be typed into the grep, got query %q", q)
	}
	if !m.logsView.Active() {
		t.Fatal("`q` typed into the grep must not close the logs view")
	}
	if m.logsView.Following() != following {
		t.Fatal("`f` typed into the grep must not toggle follow")
	}
}

// TestLogsGrepBackspaceOnEmptyQueryCancels proves D238 reaches the logs view's `/`
// too: with nothing left to erase, backspace closes the grep and restores the full
// stream — and stops there. It must never take the second step of the esc ladder and
// dismiss the view, which would throw away the stream the reader is watching.
func TestLogsGrepBackspaceOnEmptyQueryCancels(t *testing.T) {
	m := openLogsWithLines(t, "GET /healthz 200", "POST /api/v1 500")
	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "500")
	if view := frame(m); strings.Contains(view, "/healthz") {
		t.Fatalf("precondition: the grep should narrow the stream: %q", view)
	}

	// Three backspaces erase "500"; the fourth finds the line empty and cancels.
	for range 3 {
		m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
		if !m.logsView.Filtering() {
			t.Fatal("erasing the query must not close the grep field")
		}
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if m.logsView.Filtering() {
		t.Fatal("backspace into an empty query should close the grep field")
	}
	if !m.logsView.Active() {
		t.Fatal("the backspace that cancelled the grep must not also close the logs view")
	}
	if view := frame(m); !strings.Contains(view, "/healthz") {
		t.Fatalf("cancelling the grep should restore the full stream: %q", view)
	}
}

// TestLogsRegexToggleSurvivesOpenGrep proves the routing consequence of LOGS-03: the
// regex toggle is bound to a no-text chord (`ctrl+r`), so unlike `f` or `q` it still
// fires *while the grep field is open* — which is where a reader actually decides their
// substring is really a pattern. It works with the grep closed too (the sequencer path).
func TestLogsRegexToggleSurvivesOpenGrep(t *testing.T) {
	m := openLogsWithLines(t, "GET /healthz 200", "POST /api/v1 500")

	// With the grep closed the key goes through the sequencer like any browse key.
	m, _ = press(t, m, regexKey)
	if !m.logsView.Regex() {
		t.Fatal("logs.regex with the grep closed should turn regex mode on")
	}
	m, _ = press(t, m, regexKey)
	if m.logsView.Regex() {
		t.Fatal("a second logs.regex should turn it back off")
	}

	// With the grep open it carries no text, so it is routed as an action, not typed.
	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "200|500")
	if view := frame(m); strings.Contains(view, "/healthz") {
		t.Fatalf("as a literal substring `200|500` matches nothing: %q", view)
	}
	m, _ = press(t, m, regexKey)
	if !m.logsView.Regex() {
		t.Fatal("logs.regex must still fire while the grep field is open")
	}
	if q := m.logsView.Query(); q != "200|500" {
		t.Fatalf("the toggle must not type into the grep; query = %q", q)
	}
	view := frame(m)
	if !strings.Contains(view, "/healthz") || !strings.Contains(view, "/api/v1") {
		t.Fatalf("the same query read as a pattern should match both lines: %q", view)
	}
	if !strings.Contains(view, "2/2") {
		t.Fatalf("the header should re-count under the new mode: %q", view)
	}
}

// TestLogsWrapTogglesFromTheShell proves LOGS-04a's routing: `w` reaches the view
// through the ordinary sequencer path with the grep closed, and — being a plain letter,
// unlike the regex chord beside it — types into the grep field while that is open,
// leaving the wrap mode alone. It also proves the horizontal-scroll gesture reaches the
// view rather than moving the panes underneath.
func TestLogsWrapTogglesFromTheShell(t *testing.T) {
	m := openLogsWithLines(t, "GET /healthz 200")

	m, _ = press(t, m, wrapKey)
	if !m.logsView.Wrap() {
		t.Fatal("logs.wrap with the grep closed should turn wrapping on")
	}
	m, _ = press(t, m, wrapKey)
	if m.logsView.Wrap() {
		t.Fatal("a second logs.wrap should turn wrapping back off")
	}

	// nav.right is honoured by the view (it scrolls the clipped body), not by the
	// browse panes it is covering.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if !m.logsView.Active() {
		t.Fatal("nav.right should not close the logs view")
	}

	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "w")
	if m.logsView.Wrap() {
		t.Fatal("`w` typed into the open grep must not toggle wrapping")
	}
	if q := m.logsView.Query(); q != "w" {
		t.Fatalf("`w` should be typed into the grep; query = %q", q)
	}
}

// TestLogsJumpToLatestFromTheShell proves the LOGS-04c gesture survives the routing:
// `G` resolved by the sequencer reaches the logs view and rejoins the stream there
// (rather than moving a table cursor underneath), and — being a plain letter — types
// into the grep field while that is open instead of firing.
func TestLogsJumpToLatestFromTheShell(t *testing.T) {
	m := openLogsWithLines(t, "one", "two")

	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"})
	if m.logsView.Following() {
		t.Fatal("precondition: an upward scroll should pause following")
	}
	m, _ = press(t, m, endKey)
	if !m.logsView.Following() {
		t.Fatal("`G` should catch the reader up and keep tailing")
	}
	if !m.logsView.Active() {
		t.Fatal("`G` should not close the logs view")
	}

	// With the grep open the same key is text.
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"}) // pause again
	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "G")
	if m.logsView.Following() {
		t.Fatal("`G` typed into the open grep must not resume following")
	}
	if q := m.logsView.Query(); q != "G" {
		t.Fatalf("`G` should be typed into the grep; query = %q", q)
	}
}

// TestLogsCursorMovesFromTheShell is the wiring half of LOGS-SEL-01: `j`/`k` reach the
// logs view as nav actions and move its line cursor, and — like every other letter — they
// stop being navigation the moment the grep is open and become the text they type. The
// component owns what the cursor then does; this is only about the keys getting there.
func TestLogsCursorMovesFromTheShell(t *testing.T) {
	m := openLogsWithLines(t, "one", "two", "three")
	if got := m.logsView.Cursor(); got != 2 {
		t.Fatalf("precondition: a followed view holds the cursor on the newest line; got %d", got)
	}
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"})
	if got := m.logsView.Cursor(); got != 1 {
		t.Errorf("`k` should step the cursor up one line; got %d", got)
	}
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if got := m.logsView.Cursor(); got != 2 {
		t.Errorf("`j` should step it back down; got %d", got)
	}
	if !m.logsView.Active() {
		t.Fatal("moving the cursor must not close the view")
	}

	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"}) // pause, cursor on line 1
	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "j")
	if q := m.logsView.Query(); q != "j" {
		t.Errorf("`j` typed into the open grep should be text; query = %q", q)
	}
}

// TestLogsStreamTailsRecentHistory is LOGS-05a: opening a log asks the server for the
// last defaultLogTail lines rather than the container's whole history. Without it a pod
// that has been up for a week replays a week before the view reaches "now" — the wait
// feedback `2026-07-29-logs-tail-and-perf` reported. The other two options are asserted
// alongside it because the tail is only correct in their company: it must not have cost
// the stream its follow (the view opens tailing) or its timestamps (LOGS-04b's toggle
// reads them out of the buffer).
func TestLogsStreamTailsRecentHistory(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "2026-07-29T12:00:00Z hello"}}}
	m := logsViewerModel(t, s)
	_ = openLogsViewHelper(t, m)

	if s.gotOpts.TailLines == nil {
		t.Fatal("the logs stream should be opened with a TailLines, not replayed from container boot")
	}
	if got := *s.gotOpts.TailLines; got != defaultLogTail {
		t.Errorf("TailLines = %d, want %d (defaultLogTail)", got, defaultLogTail)
	}
	if !s.gotOpts.Follow {
		t.Error("bounding the history must not stop the view tailing live output")
	}
	if !s.gotOpts.Timestamps {
		t.Error("bounding the history must not drop the timestamps the buffer needs (LOGS-04b)")
	}
}

// TestLogsTailIsBoundedButScrollable guards the size of the bound rather than the
// mechanism: a tail smaller than a terminal would leave a reader with nothing to scroll
// back to, which is the failure mode of tailing too little. It fails loudly if a later
// leg tunes defaultLogTail down to a screenful.
func TestLogsTailIsBoundedButScrollable(t *testing.T) {
	if defaultLogTail < 200 {
		t.Errorf("defaultLogTail = %d — too small to scroll back through", defaultLogTail)
	}
	if defaultLogTail > 100000 {
		t.Errorf("defaultLogTail = %d — large enough to be the unbounded replay it replaced", defaultLogTail)
	}
	// The two bounds are on opposite sides of the same buffer and only one of them is
	// visible from here: the replay is what the view opens holding, the cap is what it
	// keeps. A cap at or under the replay would have the first live line start discarding
	// history the reader just asked the apiserver for (D245).
	if logsview.MaxLines <= int(defaultLogTail) {
		t.Errorf("logsview.MaxLines = %d must exceed defaultLogTail = %d, or the opening replay is trimmed on arrival",
			logsview.MaxLines, defaultLogTail)
	}
}

// tsKey is the default logs.timestamps key (`t`) — the LOGS-04b display toggle. Like
// the wrap key beside it (and unlike the regex chord) it is an ordinary letter, so the
// open grep types it.
var tsKey = tea.Key{Code: 't', Text: "t"}

// TestLogsStreamAsksForTimestamps is the wiring half of LOGS-04b/D148: the stream is
// opened with Timestamps set even though the view starts with them hidden, because that
// is what makes the toggle a redraw rather than a re-fetch. It costs nothing — a
// following stream already forces them on the wire to anchor its reconnect.
func TestLogsStreamAsksForTimestamps(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "2026-07-28T16:32:01Z hello"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)
	if !s.gotOpts.Timestamps {
		t.Error("the logs stream should be opened with Timestamps set")
	}
	if !s.gotOpts.Follow {
		t.Error("the logs stream should still follow")
	}
	// The stamp is split off at the boundary, so the view shows the message only until
	// the toggle asks for it.
	if v := frame(m); strings.Contains(v, "2026-07-28T16:32:01Z") {
		t.Errorf("the stamp must not be drawn before logs.timestamps is pressed; got:\n%s", v)
	}
	if v := frame(m); !strings.Contains(v, "hello") {
		t.Errorf("the message should be shown; got:\n%s", v)
	}
	m, _ = press(t, m, tsKey)
	if v := frame(m); !strings.Contains(v, "2026-07-28T16:32:01Z hello") {
		t.Errorf("logs.timestamps should reveal the stamp already in the buffer; got:\n%s", v)
	}
}

// TestLogsTimestampsToggleFromTheShell proves LOGS-04b's routing, the wrap test's twin:
// `t` reaches the view through the ordinary sequencer path with the grep closed, and —
// being a plain letter — types into the grep field while that is open, leaving the
// toggle alone.
func TestLogsTimestampsToggleFromTheShell(t *testing.T) {
	m := openLogsWithLines(t, "GET /healthz 200")

	m, _ = press(t, m, tsKey)
	if !m.logsView.Timestamps() {
		t.Fatal("logs.timestamps with the grep closed should turn timestamps on")
	}
	if !m.logsView.Active() {
		t.Fatal("logs.timestamps should not close the logs view")
	}
	m, _ = press(t, m, tsKey)
	if m.logsView.Timestamps() {
		t.Fatal("a second logs.timestamps should turn them back off")
	}

	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "t")
	if m.logsView.Timestamps() {
		t.Fatal("`t` typed into the open grep must not toggle timestamps")
	}
	if q := m.logsView.Query(); q != "t" {
		t.Fatalf("`t` should be typed into the grep; query = %q", q)
	}
}

// TestLogsQuitClosesViewNotApp proves `q` with the grep *closed* closes the logs view
// rather than exiting kubecom — a full-screen pager owns the quit key while it is up,
// as the help modal, the shared viewer and the search view all do — and that it tears
// the stream down on the way out.
func TestLogsQuitClosesViewNotApp(t *testing.T) {
	m := openLogsWithLines(t, "hello")
	m, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if m.logsView.Active() {
		t.Fatal("`q` should close the logs view")
	}
	if cmd != nil {
		t.Fatal("`q` in the logs view should not quit the app")
	}
	if m.logCh != nil || m.logCancel != nil {
		t.Fatal("closing the logs view should tear the stream down")
	}
}

// TestLogsViewSwallowsBrowseActions proves the capture contract: an action the logs
// view does not honour (here ns.switch) is swallowed rather than acting on the browse
// panes underneath, so no picker opens behind the full-screen view.
func TestLogsViewSwallowsBrowseActions(t *testing.T) {
	m := openLogsWithLines(t, "hello")
	m, _ = press(t, m, tea.Key{Code: 'n', Mod: tea.ModCtrl})
	if m.cmdPicker.Active() {
		t.Fatal("the logs view should swallow ns.switch, not open the namespace stage")
	}
	if !m.logsView.Active() {
		t.Fatal("an unhandled action should not close the logs view")
	}
}

// TestLogsHintBarTracksGrepState proves the two-state hint (D143 pt 1): with the grep
// closed the hint offers what the view honours (grep, follow, quit); with the grep open
// those text keys drop out because they now type; closing the view restores the browse
// hint.
func TestLogsHintBarTracksGrepState(t *testing.T) {
	// A wide terminal so the hint renderer elides nothing — the point is which
	// bindings the context offers, not how they are truncated.
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "hello"}}}
	wide, _ := New(WithWatcher(fw), WithLogStreamer(s)).Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	m := wide.(Model)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg))
	m = next.(Model)
	// Captured with the table focused — the state the logs view is opened from, and the
	// one closing it must restore.
	browseHint := m.hintbar.View()
	m = openLogsViewHelper(t, m)

	hint := m.hintbar.View()
	for _, want := range []keymap.Action{keymap.ActionFilter, keymap.ActionLogsFollow, keymap.ActionLogsWrap, keymap.ActionBack, keymap.ActionQuit} {
		if !strings.Contains(hint, want.Describe()) {
			t.Errorf("the logs hint should offer %q with the grep closed: %q", want, hint)
		}
	}
	if strings.Contains(hint, keymap.ActionNamespace.Describe()) {
		t.Errorf("ns.switch is swallowed by the logs view; the hint must not offer it: %q", hint)
	}

	m, _ = press(t, m, filterKey)
	grepHint := m.hintbar.View()
	for _, unreachable := range []keymap.Action{keymap.ActionFilter, keymap.ActionLogsFollow, keymap.ActionLogsWrap, keymap.ActionQuit} {
		if strings.Contains(grepHint, unreachable.Describe()) {
			t.Errorf("%q types into the open grep; the hint must not offer it: %q", unreachable, grepHint)
		}
	}
	if !strings.Contains(grepHint, keymap.ActionBack.Describe()) {
		t.Errorf("the grep hint should still offer back (clear the grep): %q", grepHint)
	}

	closed, _ := m.Update(logsview.ClosedMsg{Kind: "logs"})
	if got := closed.(Model).hintbar.View(); got != browseHint {
		t.Errorf("closing the logs view should restore the browse hint: %q, want %q", got, browseHint)
	}
}

// TestLogsClosedMsgBumpsGeneration proves closing the view supersedes its stream: a
// line still draining from the cancelled stream is dropped rather than reopening or
// repopulating a view that is no longer up (the searchGen/watchGen guard).
func TestLogsClosedMsgBumpsGeneration(t *testing.T) {
	m := openLogsWithLines(t, "hello")
	gen := m.viewerGen
	next, _ := m.Update(logsview.ClosedMsg{Kind: "logs"})
	m = next.(Model)
	if m.logsView.Active() {
		t.Fatal("the ClosedMsg should hide the logs view")
	}
	if m.viewerGen == gen {
		t.Fatal("closing the logs view should bump the generation so draining lines go stale")
	}
	next, cmd := m.Update(logMsg{gen: gen, msg: LogLineMsg{Lines: []string{"late line"}}})
	if cmd != nil {
		t.Fatal("a stale line should not re-issue the pump")
	}
	if strings.Contains(next.(Model).View().Content, "late line") {
		t.Fatal("a line from a closed view's stream should be dropped")
	}
}

// TestLogBatchAppliesItsLinesBeforeTheStreamEnd: the pump can hand the model a batch of
// lines *and* the terminal event that ended the stream (LOGS-05b), because the drain that
// collected them consumed it. Order matters — the lines have to land first, or a
// mid-stream drop would be treated as an open failure and close a view that has output to
// show (D74's degrade, not a blank screen).
func TestLogBatchAppliesItsLinesBeforeTheStreamEnd(t *testing.T) {
	m := openLogsWithLines(t, "hello")
	end := NewErrorMsg("logs", errors.New("stream dropped"))

	next, cmd := m.Update(logMsg{gen: m.viewerGen, msg: LogLineMsg{
		Lines: []string{"one", "two"},
		End:   end,
	}})
	m = next.(Model)

	if !m.logsView.Active() {
		t.Fatal("a mid-stream error after lines showed should keep the view open")
	}
	if f := frame(m); !strings.Contains(f, "one") || !strings.Contains(f, "two") {
		t.Errorf("the batch's lines should be on screen before the error; got:\n%s", f)
	}
	if cmd == nil {
		t.Error("the terminal error should still surface (a toast cmd), not be swallowed")
	}
}

// TestLogBatchStreamEndClosesAnEmptyView is the other side of that order: when the batch
// itself is what the view has and the stream immediately EOFs, the lines stay put and the
// pump chain stops rather than re-receiving from a closed channel.
func TestLogBatchStreamEndClosesAnEmptyView(t *testing.T) {
	m := openLogsWithLines(t, "hello")

	next, cmd := m.Update(logMsg{gen: m.viewerGen, msg: LogLineMsg{
		Lines: []string{"tail line"},
		End:   LogClosedMsg{},
	}})
	m = next.(Model)

	if cmd != nil {
		t.Error("a closed stream must not re-issue the pump")
	}
	if !m.logsView.Active() {
		t.Error("EOF should leave the lines on screen, not dismiss the view")
	}
	if f := frame(m); !strings.Contains(f, "tail line") {
		t.Errorf("the batch's line should survive the EOF; got:\n%s", f)
	}
}

// previousKey is the default logs.previous chord (`ctrl+p`) — the instance toggle
// (M5-01a). Like the regex chord it carries no text, so it acts while the grep is open.
var previousKey = tea.Key{Code: 'p', Mod: tea.ModCtrl}

// crashLoopLogStreamer serves the running instance's log and rejects any request for a
// previous one — the shape of a pod whose container has never terminated, where the
// apiserver answers `Previous` with "previous terminated container not found". It
// delivers that as a *terminal event* rather than an open error, which is how the real
// kube layer reports it: Logs returns immediately and opens the stream in a goroutine,
// so a server rejection can only arrive on the channel (internal/kube/logs.go).
type crashLoopLogStreamer struct {
	calls   int
	gotOpts kube.LogOptions
}

func (f *crashLoopLogStreamer) Logs(_ context.Context, _ kube.ObjectRef, opts kube.LogOptions) (<-chan kube.LogEvent, error) {
	f.calls++
	f.gotOpts = opts
	ch := make(chan kube.LogEvent, 1)
	if opts.Previous {
		ch <- kube.LogEvent{Err: errors.New(`previous terminated container "app" in pod "api" not found`)}
	} else {
		ch <- kube.LogEvent{Line: "still serving"}
	}
	close(ch)
	return ch, nil
}

// TestLogsPreviousTogglesTheInstanceStreamed is M5-01a's headline: `ctrl+p` inside the
// open logs view re-opens the stream against the container's previous terminated
// instance (`kubectl logs -p`) and the header names which one is on screen, and pressing
// it again comes back. The assertion that matters beyond the flag is that *only* the
// flag changed — follow, timestamps and the tail bound are the same request — since the
// toggle's whole claim is that it re-issues the open the reader already made.
func TestLogsPreviousTogglesTheInstanceStreamed(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "current instance"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)
	if s.gotOpts.Previous {
		t.Fatal("a fresh logs open should stream the running instance")
	}
	opens, ref := s.calls, s.gotRef

	m, cmd := press(t, m, previousKey)
	m = drainLogPump(t, m, cmd)
	if s.calls != opens+1 {
		t.Fatalf("logs.previous should re-open the stream: %d opens, want %d", s.calls, opens+1)
	}
	if !s.gotOpts.Previous {
		t.Fatal("logs.previous should ask the streamer for LogOptions.Previous")
	}
	if s.gotRef != ref {
		t.Fatalf("the flip should re-address the same pod: %v, want %v", s.gotRef, ref)
	}
	if !s.gotOpts.Follow || !s.gotOpts.Timestamps || s.gotOpts.TailLines == nil {
		t.Fatalf("the flip should change only Previous: %+v", s.gotOpts)
	}
	if !m.logsView.Previous() {
		t.Fatal("the view should know it is showing the previous instance")
	}
	if f := frame(m); !strings.Contains(f, "[previous]") {
		t.Fatalf("the logs header should name the previous instance: %q", f)
	}

	m, cmd = press(t, m, previousKey)
	m = drainLogPump(t, m, cmd)
	if s.gotOpts.Previous {
		t.Fatal("a second logs.previous should return to the running instance")
	}
	if m.logsView.Previous() {
		t.Fatal("the view's marker should come back with the flip")
	}
	if f := frame(m); strings.Contains(f, "[previous]") {
		t.Fatalf("the header marker should go with the flip back: %q", f)
	}
}

// TestLogsPreviousKeepsTheGrepAndReplacesTheBuffer covers the reason the flip is a
// Restream rather than a Reset: two instances are two different logs, so the buffer is
// replaced (never appended to — the counts would double), but the reader's grep is not,
// because the point of flipping is to look for the same thing in the other log. It also
// proves the chord acts through the open grep field, which a plain letter could not
// (D140 pt 1).
func TestLogsPreviousKeepsTheGrepAndReplacesTheBuffer(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "panic: boom"}, {Line: "GET /healthz 200"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)

	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "panic")
	if f := frame(m); !strings.Contains(f, "1/2") {
		t.Fatalf("precondition: the grep should narrow 2 lines to 1: %q", f)
	}

	m, cmd := press(t, m, previousKey)
	m = drainLogPump(t, m, cmd)

	if !m.logsView.Filtering() {
		t.Fatal("the flip should leave the grep field open")
	}
	if q := m.logsView.Query(); q != "panic" {
		t.Fatalf("the grep query = %q, want it to survive the flip", q)
	}
	// The fake replays the same two lines, so a buffer that was replaced counts 2 and
	// one that was appended to counts 4.
	if f := frame(m); !strings.Contains(f, "1/2") {
		t.Fatalf("the flip should replace the buffer, not extend it: %q", f)
	}
}

// TestLogsPreviousWithNoTerminatedInstanceDegrades is the answer to "what happens when
// there is no previous instance": nothing pre-checks for one (only the apiserver knows),
// so the request goes out, its rejection lands on the emptied view as any open failure
// does, and the existing degrade applies — the reason reaches the status bar and the
// view closes rather than sitting there empty (D74). No crash, no blank pager.
func TestLogsPreviousWithNoTerminatedInstanceDegrades(t *testing.T) {
	s := &crashLoopLogStreamer{}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)
	if !m.logsView.Active() {
		t.Fatal("precondition: the running instance's logs should open")
	}

	m, cmd := press(t, m, previousKey)
	lm, ok := cmd().(logMsg)
	if !ok {
		t.Fatalf("logs.previous should start a stream (a pump cmd), got %T", cmd())
	}
	next, toast := m.Update(lm)
	m = next.(Model)

	if m.logsView.Active() {
		t.Fatal("a rejected previous-instance request should close the emptied view")
	}
	if toast == nil {
		t.Error("the rejection should surface (a toast cmd), not be swallowed")
	}
	if f := frame(m); !strings.Contains(f, "previous terminated container") {
		t.Fatalf("the status bar should name the reason: %q", f)
	}
}

// TestLogsPreviousIsInertWithoutAStream guards the toggle against being pressed with no
// logs open — the browse context routes it to the panes, where it does nothing, and the
// stashed request is empty so nothing re-opens.
func TestLogsPreviousIsInertWithoutAStream(t *testing.T) {
	m := logsViewerModel(t, &fakeLogStreamer{})
	m, cmd := press(t, m, previousKey)
	if cmd != nil {
		t.Fatalf("logs.previous outside the logs view should do nothing, got a %T", cmd())
	}
	if m.logsView.Active() {
		t.Fatal("logs.previous must not open the logs view")
	}
}

// selectKey / yankKey are the default logs.select (`v`) and logs.yank (`y`) keys. Both
// are ordinary letters, so an open grep types them like any other.
var (
	selectKey = tea.Key{Code: 'v', Text: "v"}
	yankKey   = tea.Key{Code: 'y', Text: "y"}
)

// TestLogsYankReachesTheClipboard is the wiring half of LOGS-SEL-02: `v` and `y` reach
// the logs view as actions, the selected lines land on the system clipboard through the
// same OSC-52 path secret.copy uses (M3-08b), and the status bar says how much was copied
// without quoting a log line. The component owns which lines and what their raw text is;
// this is about the keys getting there and the copy leaving.
func TestLogsYankReachesTheClipboard(t *testing.T) {
	m := openLogsWithLines(t, "boot ok", "err disk", "err net")
	m, _ = press(t, m, tea.Key{Code: 'f', Text: "f"}) // pause with the cursor on the newest line
	m, _ = press(t, m, selectKey)
	if !m.logsView.Selecting() {
		t.Fatal("`v` should start a selection in the logs view")
	}
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"}) // extend over "err disk"

	m, cmd := press(t, m, yankKey)
	if got, want := copiedClipboard(t, cmd), "err disk\nerr net"; got != want {
		t.Errorf("`y` should put the selected lines on the clipboard; got %q, want %q", got, want)
	}
	if m.logsView.Selecting() {
		t.Error("a yank ends visual mode")
	}
	if !m.logsView.Active() {
		t.Fatal("a yank must not close the view")
	}
	if v := frame(m); !strings.Contains(v, "copied 2 lines") {
		t.Errorf("the status bar should confirm the copy: %q", v)
	}
}

// TestLogsYankTypesIntoAnOpenGrep: `v` and `y` are plain letters, so the moment the grep
// field is open they are text, exactly as `w` and `t` are (D140 pt 1). Neither is a
// gesture anyone reaches for mid-query, which is why they were allowed to be letters.
func TestLogsYankTypesIntoAnOpenGrep(t *testing.T) {
	m := openLogsWithLines(t, "very quiet")
	m, _ = press(t, m, filterKey)
	m = typeInto(t, m, "vy")
	if q := m.logsView.Query(); q != "vy" {
		t.Errorf("`v`/`y` should type into the open grep; query = %q", q)
	}
	if m.logsView.Selecting() {
		t.Error("typing `v` into the grep must not start a selection")
	}
}

// TestLogsYankOnAnEmptyViewSaysNothing: there is no line to copy, so the gesture is inert
// — no clipboard write and no notice, the same degrade secret.copy makes on a Secret with
// no data. An error the reader never asked for is the failure mode worth guarding here.
func TestLogsYankOnAnEmptyViewSaysNothing(t *testing.T) {
	m := openLogsWithLines(t)
	if !m.logsView.Empty() {
		t.Fatal("precondition: the view should hold no lines")
	}
	m, cmd := press(t, m, yankKey)
	if cmd != nil {
		t.Errorf("an empty view should yank nothing; got a command producing %T", cmd())
	}
	if v := frame(m); strings.Contains(v, "copied") {
		t.Errorf("nothing was copied, so nothing should be claimed: %q", v)
	}
}
