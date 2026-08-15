package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// These cover LOGS-09 (D258): the palette family passes through the logs view's
// capture, so `:` opens the command palette over a log stream — listing the view's
// own verbs beside the app-globals — and `C`/`T`/`R`/`N` open their pre-typed
// stages, which is what makes ctx.switch reachable without leaving the log.

// logsPaletteModel opens the logs view on a shell that also has the context-switch
// seams wired, so a test can walk the whole `C` → `:context ` → pick → switch path
// from inside the log stream.
func logsPaletteModel(t *testing.T, s *fakeLogStreamer, fl *fakeContextLister, fc *fakeConnector) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithLogStreamer(s),
		WithContextLister(fl), WithClusterConnector(fc), WithContext("prod"))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	m = next.(Model)
	return openLogsViewHelper(t, m)
}

// TestLogsPaletteOpensOverTheLogsView is the feedback's headline: `:` inside the
// logs view opens the command palette — composited over the log, not replacing it —
// listing the logs view's own verbs (the discoverability ask: the previous-instance
// toggle among them) alongside the app-globals. The row verbs are withheld: they
// act on the browse table's selection, which the full-screen view hides, and a pick
// like Delete would open the confirm modal invisibly (D258).
func TestLogsPaletteOpensOverTheLogsView(t *testing.T) {
	m := openLogsWithLines(t, "some log line")

	m, _ = press(t, m, colon)
	if !m.cmdPicker.Active() {
		t.Fatal("`:` should open the command palette over the logs view")
	}
	if m.palArg != "" {
		t.Fatalf("`:` should open on the verb list, stage = %q", m.palArg)
	}
	if got, want := m.cmdPicker.Len(), len(paletteVerbs)+len(logsVerbs); got != want {
		t.Fatalf("palette over the logs view should list %d verbs (globals + logs), got %d", want, got)
	}
	if m.palRowByLabel != nil {
		t.Error("no row verbs may be listed over the logs view — their target is invisible")
	}
	if _, listed := m.cmdByLabel[keymap.ActionLogsPrevious.Describe()]; !listed {
		t.Error("the logs.previous verb should be listed (it is the discoverability the feedback asked for)")
	}
	f := frame(m)
	if !strings.Contains(f, paletteTitle) || !strings.Contains(f, "some log line") {
		t.Errorf("the palette should composite over the log, both visible:\n%s", f)
	}

	// esc closes the palette and the reader is back in the log stream — the surface
	// underneath was never torn down.
	m, cmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	next, _ := m.Update(cmd().(picker.CancelledMsg))
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Fatal("esc should close the palette")
	}
	if !m.logsView.Active() {
		t.Fatal("closing the palette should return to the logs view")
	}
}

// TestLogsPaletteVerbDispatchesLikeTheKey picks the previous-instance verb out of
// the palette and proves D197 on this surface: the pick runs through handleAction
// exactly as the key does, so the stream re-opens against the previous instance.
func TestLogsPaletteVerbDispatchesLikeTheKey(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "current instance"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)

	m, _ = press(t, m, colon)
	m = typeInto(t, m, "previous")
	if v, _ := m.cmdPicker.Selected(); v != keymap.ActionLogsPrevious.Describe() {
		t.Fatalf("typing \"previous\" selected %q, want %q", v, keymap.ActionLogsPrevious.Describe())
	}
	m, cmd := selectInPalette(t, m)
	m = drainLogPump(t, m, cmd)

	if m.cmdPicker.Active() {
		t.Error("a plain verb's pick should close the palette")
	}
	if !m.logsView.Active() {
		t.Error("the logs view should stay open under its own toggle")
	}
	if !s.gotOpts.Previous {
		t.Error("picking the previous-instance verb should flip the stream, exactly as the key does")
	}
	if f := frame(m); !strings.Contains(f, "[previous]") {
		t.Errorf("the header should name the previous instance: %q", f)
	}
}

// TestLogsContextKeyOpensTheContextStage is the other feedback's headline: `C` fires
// from inside a log stream, opening the palette's `:context ` stage over the view —
// and the switch it drives tears the stream down with the rest of the old cluster
// (resetCluster, already pinned by TestResetClusterDismissesSurfacesAndStashes).
func TestLogsContextKeyOpensTheContextStage(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "current instance"}}}
	fl := &fakeContextLister{contexts: twoContexts()}
	fc := &fakeConnector{}
	fc.cluster, _, _ = newClusterFake()
	m := logsPaletteModel(t, s, fl, fc)

	m, cmd := press(t, m, capitalC)
	if !m.cmdPicker.Active() || m.palArg != keymap.ActionContext {
		t.Fatalf("`C` from the logs view should open the palette's context stage, stage = %q", m.palArg)
	}
	next, _ := m.Update(pickerMsg(t, cmd)) // the kubeconfig listing lands
	m = next.(Model)

	next, cmd = m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: pickerLabelFor(t, m, "dev")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("a context pick should issue the connect Cmd")
	}
	next, _ = m.Update(cmd().(clusterConnectedMsg)) // the connect lands; the switch runs
	m = next.(Model)
	if m.logsView.Active() {
		t.Error("a context switch should dismiss the old cluster's log stream with the rest")
	}
	if m.context != "dev" {
		t.Errorf("the shell should be on the picked context, got %q", m.context)
	}
}

// TestLogsGrepKeepsTheColonAsText pins the boundary of the pass-through: with the
// grep field open it owns every key (D140 pt 1), so `:` is query text and no palette
// opens. The palette family only passes with the field closed.
func TestLogsGrepKeepsTheColonAsText(t *testing.T) {
	m := openLogsWithLines(t, "a line")
	m, _ = press(t, m, filterKey)
	if !m.logsView.Filtering() {
		t.Fatal("precondition: the grep should be open")
	}

	m, _ = press(t, m, colon)
	if m.cmdPicker.Active() {
		t.Error("`:` must not open the palette while the grep owns input")
	}
	if q := m.logsView.Query(); q != ":" {
		t.Errorf("the colon should type into the grep, query = %q", q)
	}
}

// TestLogsActionsMenuStaysInert pins the deliberate gap in the pass-through (D258):
// the actions menu acts on the browse table's selection — invisible under the
// full-screen logs view — and its picks can open the confirm modal, which would
// capture input invisibly. So the gesture keeps doing what it did before this leg:
// nothing. The gesture is `enter` on the resource table since STORY-06c; over the
// logs view the table context is not consulted (hintContext is HelpLogs), so enter
// resolves to nav.drillIn and the palette stays closed.
func TestLogsActionsMenuStaysInert(t *testing.T) {
	m := openLogsWithLines(t, "a line")

	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter})
	if m.cmdPicker.Active() || m.palArg != "" {
		t.Error("actions.menu must not open over the logs view")
	}
	if m.modal.Active() {
		t.Error("no modal may open from the logs view")
	}
	if !m.logsView.Active() {
		t.Error("the logs view should be undisturbed")
	}
}

// TestLogsPreviousTakesAPlainLetter pins the new default binding (feedback:
// "Ctrl+P is a bit weird"): `o` flips the instance with the grep closed, sitting
// with the other plain-letter logs toggles (`f`, `w`, `t`, `v`, `y`).
func TestLogsPreviousTakesAPlainLetter(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "current instance"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)

	m, cmd := press(t, m, tea.Key{Code: 'o', Text: "o"})
	m = drainLogPump(t, m, cmd)
	if !s.gotOpts.Previous {
		t.Error("`o` should toggle to the previous instance")
	}
	if !m.logsView.Previous() {
		t.Error("the view should know it is showing the previous instance")
	}
}
