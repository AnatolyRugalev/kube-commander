package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/logsview"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// These cover what LOGS-02 adds on top of the M3-05/06/07 logs behaviour already
// exercised in tui_test.go: the live grep's raw-key routing, the two-state hint
// context, and the fact that a `q` typed into the grep is text rather than a close.

// filterKey is the default app.filter key (`/`) — the logs view's live grep.
var filterKey = tea.Key{Code: '/', Text: "/"}

// regexKey is the default logs.regex chord (`ctrl+r`) — the grep's mode toggle. It
// carries no text on purpose, which is what lets it act while the grep field is open.
var regexKey = tea.Key{Code: 'r', Mod: tea.ModCtrl}

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
	if m.nsPicker.Active() {
		t.Fatal("the logs view should swallow ns.switch, not open the namespace picker")
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
	wide, _ := New(WithWatcher(fw), WithLogStreamer(s)).Update(tea.WindowSizeMsg{Width: 220, Height: 24})
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
	for _, want := range []keymap.Action{keymap.ActionFilter, keymap.ActionLogsFollow, keymap.ActionBack, keymap.ActionQuit} {
		if !strings.Contains(hint, want.Describe()) {
			t.Errorf("the logs hint should offer %q with the grep closed: %q", want, hint)
		}
	}
	if strings.Contains(hint, keymap.ActionNamespace.Describe()) {
		t.Errorf("ns.switch is swallowed by the logs view; the hint must not offer it: %q", hint)
	}

	m, _ = press(t, m, filterKey)
	grepHint := m.hintbar.View()
	for _, unreachable := range []keymap.Action{keymap.ActionFilter, keymap.ActionLogsFollow, keymap.ActionQuit} {
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
	next, cmd := m.Update(logMsg{gen: gen, msg: LogLineMsg{Line: "late line"}})
	if cmd != nil {
		t.Fatal("a stale line should not re-issue the pump")
	}
	if strings.Contains(next.(Model).View().Content, "late line") {
		t.Fatal("a line from a closed view's stream should be dropped")
	}
}
