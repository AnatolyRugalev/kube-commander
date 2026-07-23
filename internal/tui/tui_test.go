package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// sized returns the model after a WindowSizeMsg so View renders (it draws nothing
// until sized) and the sequencer/help are wired.
func sized(t *testing.T) Model {
	t.Helper()
	m, _ := New().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m.(Model)
}

// sizedWith is sized() with construction options (e.g. WithWatcher).
func sizedWith(t *testing.T, opts ...Option) Model {
	t.Helper()
	m, _ := New(opts...).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m.(Model)
}

// fakeWatcher is a hermetic ResourceWatcher: it hands back a preset channel (per
// call) and records the contexts it was given so a test can assert the previous
// watch was cancelled when a new resource is selected.
type fakeWatcher struct {
	chans   []chan kube.WatchEvent // one per call, in order; a fresh one each Watch
	err     error
	ctxs    []context.Context
	res     []kube.Resource
	ns      []string
	preload []kube.WatchEvent // if set, buffered into every returned channel at Watch time
}

func (f *fakeWatcher) Watch(ctx context.Context, r kube.Resource, ns string, _ metav1.ListOptions) (<-chan kube.WatchEvent, error) {
	f.ctxs = append(f.ctxs, ctx)
	f.res = append(f.res, r)
	f.ns = append(f.ns, ns)
	if f.err != nil {
		return nil, f.err
	}
	// Preloaded events let a full-program test (teatest) drive the watch without the
	// test goroutine racing to push into the channel after Watch is called: the pump
	// reads them straight off the buffer. Default (nil) preserves every existing test.
	capacity := 1
	if len(f.preload) > capacity {
		capacity = len(f.preload)
	}
	ch := make(chan kube.WatchEvent, capacity)
	for _, e := range f.preload {
		ch <- e
	}
	f.chans = append(f.chans, ch)
	return ch, nil
}

// fakeDiscoverer is a hermetic Discoverer: it hands back a preset channel and
// records the contexts it was given so a test can assert the pass is cancelled on
// completion. The test seeds the channel (cap 1) so the discovery pump reads a
// result without a real goroutine.
type fakeDiscoverer struct {
	ch   chan kube.DiscoveryResult
	ctxs []context.Context
}

func (f *fakeDiscoverer) StartDiscovery(ctx context.Context) <-chan kube.DiscoveryResult {
	f.ctxs = append(f.ctxs, ctx)
	return f.ch
}

func gvrResource(name string) kube.Resource {
	return kube.Resource{GVR: schema.GroupVersionResource{Resource: name}}
}

func resetEvent(name, uid string) kube.WatchEvent {
	return kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}},
		Rows:    []kube.Row{{Cells: []any{name}, Object: kube.ObjectRef{Name: name, UID: uid}}},
	}
}

// press feeds one live keypress through the root model's Update, as Bubble Tea
// would deliver it, and returns the next model + any command.
func press(t *testing.T, m Model, k tea.Key) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg(k))
	return next.(Model), cmd
}

// TestModelSmoke is the M0-05 teatest smoke test, carried onto the real root
// model: the app renders the browse layout and shuts down cleanly. It is the
// template M2 view tests build on. The model quits on the app.quit key, but
// teatest.Quit still works.
func TestModelSmoke(t *testing.T) {
	tm := teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(80, 24))

	// The seed resource menu renders its kinds in the left pane, so "Node" (a seed
	// item) is on screen once the browse layout is drawn. (The first item is the
	// selected row, whose background-filled cells the test terminal emulator writes
	// via a path a plain byte scan misses, so we key on an unselected row.)
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestQuitKey routes the default app.quit key (`q`) to tea.Quit through the
// keymap, proving input flows through the action registry, not a raw-key match.
func TestQuitKey(t *testing.T) {
	m := sized(t)
	_, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("app.quit produced no command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("app.quit did not resolve to tea.Quit, got %T", msg)
	}
}

// TestHelpToggle toggles the overlay via app.help (`?`) and closes it via
// nav.back (`esc`).
func TestHelpToggle(t *testing.T) {
	m := sized(t)
	if m.help.Visible() {
		t.Fatal("help should start hidden")
	}
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help did not open the overlay")
	}
	// esc (nav.back) closes it.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.help.Visible() {
		t.Fatal("nav.back did not close the overlay")
	}
}

// TestHelpModalCompositesOverBase proves the open help modal floats over the
// two-pane browse view rather than replacing it (feedback
// 2026-07-22-popups-should-overlay, D95): the composed view shows both the base
// resource menu (the "Cluster" section header at the top of the left pane, above
// the centered modal) and the modal's "Keybindings" title at the same time.
func TestHelpModalCompositesOverBase(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help did not open the overlay")
	}
	view := m.View().Content
	if !strings.Contains(view, "Keybindings") {
		t.Errorf("composed view missing the help modal title; got:\n%s", view)
	}
	if !strings.Contains(view, "Cluster") {
		t.Errorf("base resource menu should stay visible under the modal; got:\n%s", view)
	}
}

// TestHelpQuitKeyClosesOverlay proves the quit key (`q`) dismisses the open help
// modal instead of quitting the app — a modal owns the quit key until it closes.
func TestHelpQuitKeyClosesOverlay(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help did not open the overlay")
	}
	m, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if m.help.Visible() {
		t.Fatal("q did not close the help overlay")
	}
	if cmd != nil {
		t.Fatal("q should not quit the app while help is open")
	}
	// With help closed, q now quits (returns the tea.Quit command).
	if _, cmd = press(t, m, tea.Key{Code: 'q', Text: "q"}); cmd == nil {
		t.Fatal("q should quit once help is closed")
	}
}

// TestSequencePending checks the multi-key path: a lone `g` is a prefix of `gg`
// (nav.top), so it buffers and schedules a timeout tick; the tick then fires the
// buffered prefix only if it is itself a complete binding.
func TestSequencePending(t *testing.T) {
	m := sized(t)
	m, cmd := press(t, m, tea.Key{Code: 'g', Text: "g"})
	if !m.seq.Pending() {
		t.Fatal("`g` should leave a pending prefix (gg → nav.top)")
	}
	if cmd == nil {
		t.Fatal("a pending sequence must schedule a timeout tick")
	}
	tick, ok := cmd().(seqTimeoutMsg)
	if !ok {
		t.Fatalf("expected seqTimeoutMsg, got %T", cmd())
	}
	if tick.gen != m.seqGen {
		t.Fatalf("tick gen %d != model seqGen %d", tick.gen, m.seqGen)
	}
	// `g` alone is not a complete binding (only `gg`/`home` are), so the timeout
	// drops the buffer without acting.
	next, cmd := m.Update(tick)
	m = next.(Model)
	if m.seq.Pending() {
		t.Fatal("timeout should clear the pending buffer")
	}
	if cmd != nil {
		t.Fatal("`g`-only timeout should not fire an action")
	}
}

// TestStaleTimeoutIgnored proves a superseded timeout tick is dropped: pressing
// `g` twice completes `gg` (nav.top) and clears the buffer; a leftover tick from
// the first `g` must not act.
func TestStaleTimeoutIgnored(t *testing.T) {
	m := sized(t)
	m, cmd1 := press(t, m, tea.Key{Code: 'g', Text: "g"})
	stale := cmd1().(seqTimeoutMsg)
	// Second `g` completes gg → nav.top (ResultAction, no new pending/tick).
	m, _ = press(t, m, tea.Key{Code: 'g', Text: "g"})
	if m.seq.Pending() {
		t.Fatal("gg should have resolved and cleared the buffer")
	}
	// Delivering the stale tick from the first `g` must be a no-op.
	_, cmd := m.Update(stale)
	if cmd != nil {
		t.Fatal("stale timeout tick should be ignored")
	}
}

// TestViewEmptyUntilSized guards that the model draws nothing before it knows the
// terminal size (avoids sizing panes to a zero canvas).
func TestViewEmptyUntilSized(t *testing.T) {
	if got := New().View().Content; got != "" {
		t.Fatalf("unsized View should be empty, got %q", got)
	}
}

// TestViewRendersWhenSized sanity-checks the sized shell renders the browse layout
// (a menu kind in the left pane) and a non-empty keymap-derived short-help hint.
func TestViewRendersWhenSized(t *testing.T) {
	m := sized(t)
	if m.help.ShortHelpView() == "" {
		t.Fatal("short-help hint should be generated from the registry")
	}
	if !bytes.Contains([]byte(m.View().Content), []byte("Node")) {
		t.Fatalf("sized View missing the resource menu: %q", m.View().Content)
	}
}

// TestFocusStartsOnMenu proves the left (menu) pane holds focus initially — the
// user picks a resource before drilling into its table.
func TestFocusStartsOnMenu(t *testing.T) {
	m := sized(t)
	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}
	if m.table.Focused() {
		t.Fatal("table should not start focused")
	}
}

// TestFocusSwitch drives the horizontal focus switch: nav.right moves focus from
// the menu to the table, and nav.left (with the empty table at its left edge, so
// nothing to scroll) moves it back to the menu — the HOffset arbitration (D60).
func TestFocusSwitch(t *testing.T) {
	m := sized(t)

	// nav.right (`l`) → focus the table.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("nav.right should move focus to the table")
	}

	// nav.left (`h`) at the table's left edge → focus back to the menu.
	if m.table.HOffset() != 0 {
		t.Fatalf("empty table should be at HOffset 0, got %d", m.table.HOffset())
	}
	m, _ = press(t, m, tea.Key{Code: 'h', Text: "h"})
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("nav.left at the table's left edge should move focus to the menu")
	}
}

// TestMenuToggle proves menu.toggle (`m`) hides and shows the left menu pane and
// moves focus accordingly (FB-nav-menu-toggle, D96's first navigation slice):
// hiding hands the full width to the table and focuses it (a hidden pane can't hold
// focus), the menu disappears from the browse view, and no column falls in a menu
// pane any more (so mouse routing sends every click to the table); the same key
// re-shows the menu and returns focus to it. Width 200 so the shown menu pane is
// well below full width, making the hand-off observable.
func TestMenuToggle(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := next.(Model)

	if m.menuHidden {
		t.Fatal("menu should start shown")
	}
	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}
	if !strings.Contains(m.View().Content, "Workloads") {
		t.Fatalf("shown menu should render its sections in the browse view:\n%s", m.View().Content)
	}
	if !m.inMenu(0) {
		t.Fatal("with the menu shown, x=0 should fall in the menu pane")
	}

	// menu.toggle (`m`) hides the pane: focus moves to the table, the menu vanishes
	// from the view, and no column falls in a menu pane any more.
	m, _ = press(t, m, tea.Key{Code: 'm', Text: "m"})
	if !m.menuHidden {
		t.Fatal("menu.toggle did not hide the menu")
	}
	if m.menu.Focused() || !m.table.Focused() {
		t.Fatal("hiding the menu should move focus to the table")
	}
	if strings.Contains(m.View().Content, "Workloads") {
		t.Fatalf("hidden menu should not render; view:\n%s", m.View().Content)
	}
	if m.inMenu(0) {
		t.Fatal("with the menu hidden, no column should fall in a menu pane")
	}

	// The same key re-shows the menu and returns focus to it.
	m, _ = press(t, m, tea.Key{Code: 'm', Text: "m"})
	if m.menuHidden {
		t.Fatal("second menu.toggle did not re-show the menu")
	}
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("showing the menu should return focus to it")
	}
	if !strings.Contains(m.View().Content, "Workloads") {
		t.Fatalf("re-shown menu should render again; view:\n%s", m.View().Content)
	}
}

// TestHintsAreFocusAware proves the persistent bottom hint tracks focus (feedback
// 2026-07-21-06): the menu-context hint offers drill-in and not next-match, and
// once a resource is opened and focus moves to the table the hint switches to the
// table context (filter/next-match, not drill-in). The hint lives on its own
// dedicated line now (the hintbar, FB-hintbar-dedicated), rendered in isolation so
// the welcome page's focus-agnostic hint can't mask the difference. Width 200 keeps
// the help renderer from eliding.
func TestHintsAreFocusAware(t *testing.T) {
	next, _ := New(WithWatcher(&fakeWatcher{})).Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := next.(Model)

	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}
	bar := m.hintbar.View()
	if !strings.Contains(bar, keymap.ActionDrillIn.Describe()) {
		t.Errorf("menu-focused hint should offer drill-in; got %q", bar)
	}
	if strings.Contains(bar, keymap.ActionSearchNext.Describe()) {
		t.Errorf("menu-focused hint should not offer next-match; got %q", bar)
	}

	// Drill into a resource → focus moves to the table → table-context hint.
	next, _ = m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if !m.table.Focused() {
		t.Fatal("drilling in should focus the table")
	}
	bar = m.hintbar.View()
	if !strings.Contains(bar, keymap.ActionFilter.Describe()) {
		t.Errorf("table-focused hint should offer filter; got %q", bar)
	}
	if strings.Contains(bar, keymap.ActionDrillIn.Describe()) {
		t.Errorf("table-focused hint should not offer drill-in; got %q", bar)
	}
}

// TestHintPersistsThroughErrorToast proves the value of the dedicated hint line
// (FB-hintbar-dedicated): when a transient error takes over the whole status bar,
// the focus-aware key hint — on its own row below — still shows. The old
// right-aligned status-bar hint was dropped entirely behind an error toast.
func TestHintPersistsThroughErrorToast(t *testing.T) {
	next, _ := New(WithWatcher(&fakeWatcher{})).Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := next.(Model)

	next, _ = m.Update(NewErrorMsg("watch pods", context.DeadlineExceeded))
	m = next.(Model)
	if !m.status.HasError() {
		t.Fatal("error should be surfaced on the status bar")
	}

	view := m.View().Content
	if !strings.Contains(view, keymap.ActionDrillIn.Describe()) {
		t.Errorf("hint should survive an error toast; composed view %q missing the hint", view)
	}
}

// TestNavRoutedToFocusedPane proves a nav action reaches only the focused pane:
// nav.down moves the menu cursor while the menu is focused, and does not move it
// once focus is on the table.
func TestNavRoutedToFocusedPane(t *testing.T) {
	m := sized(t)
	if m.menu.Cursor() != 0 {
		t.Fatalf("menu should start at cursor 0, got %d", m.menu.Cursor())
	}

	// Menu focused: nav.down (`j`) advances the menu cursor.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 1 {
		t.Fatalf("nav.down should advance the focused menu cursor, got %d", m.menu.Cursor())
	}

	// Focus the table; nav.down no longer moves the menu.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 1 {
		t.Fatalf("nav.down should not move the blurred menu, got %d", m.menu.Cursor())
	}
}

// TestSelectResourceStartsWatch proves drilling into a resource starts a watch,
// moves focus to the table, and that the watch's first RESET populates the table
// through the pump → ApplyEvent path (M2-07c).
func TestSelectResourceStartsWatch(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)

	if len(fw.chans) != 1 {
		t.Fatalf("expected 1 Watch call, got %d", len(fw.chans))
	}
	if fw.res[0].GVR.Resource != "pods" {
		t.Fatalf("watched wrong resource: %q", fw.res[0].GVR.Resource)
	}
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("selecting a resource should move focus to the table")
	}
	if cmd == nil {
		t.Fatal("selecting a resource should issue a watch pump")
	}

	// Deliver a RESET; running the pump cmd yields a watchMsg carrying it, and
	// applying that watchMsg populates the table.
	fw.chans[0] <- resetEvent("pod-a", "u1")
	wm, ok := cmd().(watchMsg)
	if !ok {
		t.Fatalf("pump produced %T, want watchMsg", cmd())
	}
	if wm.gen != m.watchGen {
		t.Fatalf("pump tagged gen %d, want current %d", wm.gen, m.watchGen)
	}
	next, cmd = m.Update(wm)
	m = next.(Model)
	if !bytes.Contains([]byte(m.table.View()), []byte("pod-a")) {
		t.Fatalf("RESET did not populate the table: %q", m.table.View())
	}
	if cmd == nil {
		t.Fatal("a delivered event should re-issue the pump for the next event")
	}
}

// TestSelectResourceMarksMenuActive proves selecting a resource marks it as the
// opened/active item in the menu, distinct from the nav cursor (dogfood-05): the
// menu renders the "▸ " marker on the opened row even though drilling in moved
// focus to the table and never advanced the menu cursor.
func TestSelectResourceMarksMenuActive(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	// Use the seed's real Pods resource so its GVR matches the menu row it marks.
	var pods kube.Resource
	for _, it := range m.menu.Items() {
		if it.Resource.GVR.Resource == "pods" {
			pods = it.Resource
		}
	}
	if pods.GVR.Resource == "" {
		t.Fatal("seed menu has no pods item")
	}

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: pods})
	m = next.(Model)

	if m.menu.Cursor() != 0 {
		t.Fatalf("drilling in should not move the menu cursor, got %d", m.menu.Cursor())
	}
	if !strings.Contains(m.menu.View(), "▸ Pod") {
		t.Fatalf("menu does not mark the opened resource active:\n%s", m.menu.View())
	}
}

// TestSelectResourceInertWithoutWatcher proves a model built with no watcher is
// watch-inert: selecting a resource is a no-op (no focus change, no command).
func TestSelectResourceInertWithoutWatcher(t *testing.T) {
	m := sized(t) // no WithWatcher
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("watch-inert model should issue no command on selection")
	}
	if m.table.Focused() || !m.menu.Focused() {
		t.Fatal("watch-inert selection should not move focus")
	}
}

// TestSelectResourceCancelsPrevious proves selecting a second resource cancels the
// first watch's context and that a stale delta from it (old generation) is dropped
// rather than applied to the table now showing the new resource.
func TestSelectResourceCancelsPrevious(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	staleGen := m.watchGen

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("nodes")})
	m = next.(Model)

	if len(fw.ctxs) != 2 {
		t.Fatalf("expected 2 Watch calls, got %d", len(fw.ctxs))
	}
	if fw.ctxs[0].Err() == nil {
		t.Fatal("selecting a new resource should cancel the previous watch context")
	}
	if m.watchGen == staleGen {
		t.Fatal("a new selection should bump the watch generation")
	}

	// A stale delta from the first watch (old gen) must be dropped: no table change,
	// no re-issued pump.
	next, staleCmd := m.Update(watchMsg{gen: staleGen, msg: ResourceEventMsg{Event: resetEvent("pod-a", "u1")}})
	m = next.(Model)
	if staleCmd != nil {
		t.Fatal("a stale-generation watch message should not re-issue the pump")
	}
	if bytes.Contains([]byte(m.table.View()), []byte("pod-a")) {
		t.Fatal("a stale delta must not populate the table for the new resource")
	}

	// The current watch's RESET does populate it.
	next, _ = m.Update(watchMsg{gen: m.watchGen, msg: ResourceEventMsg{Event: resetEvent("node-a", "u2")}})
	m = next.(Model)
	if !bytes.Contains([]byte(m.table.View()), []byte("node-a")) {
		t.Fatalf("current RESET did not populate the table: %q", m.table.View())
	}
	_ = cmd
}

// TestWatchClosedStopsChain proves a WatchClosedMsg for the current watch clears
// the channel and does not re-issue the pump (which would busy-loop on a closed
// channel).
func TestWatchClosedStopsChain(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if m.watchCh == nil {
		t.Fatal("a started watch should hold a channel")
	}

	next, cmd := m.Update(watchMsg{gen: m.watchGen, msg: WatchClosedMsg{}})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("WatchClosedMsg must not re-issue the pump")
	}
	if m.watchCh != nil {
		t.Fatal("WatchClosedMsg should clear the watch channel")
	}
}

// TestWatchStartErrorSurfaces proves a Watch that fails to start surfaces a
// classified ErrorMsg and leaves no dangling watch state.
func TestWatchStartErrorSurfaces(t *testing.T) {
	fw := &fakeWatcher{err: context.DeadlineExceeded}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if m.watchCh != nil || m.watchCancel != nil {
		t.Fatal("a failed watch start should leave no watch state")
	}
	if cmd == nil {
		t.Fatal("a failed watch start should surface an error")
	}
	errMsg, ok := cmd().(ErrorMsg)
	if !ok {
		t.Fatalf("expected ErrorMsg, got %T", cmd())
	}
	// Delivering the ErrorMsg surfaces it inside the fixed layout (status bar),
	// never on a growing pane or stdout.
	next, clearCmd := m.Update(errMsg)
	m = next.(Model)
	if !m.status.HasError() {
		t.Error("delivering an ErrorMsg should surface it in the status bar")
	}
	if clearCmd == nil {
		t.Fatal("surfacing an error should arm an auto-clear timer")
	}
}

// TestErrorAutoClearsWithGenGuard proves a surfaced error clears when its own
// clear timer fires, but a stale timer (from an error already superseded by a
// newer one) does not wipe the newer message early.
func TestErrorAutoClearsWithGenGuard(t *testing.T) {
	m := sized(t)

	next, _ := m.Update(NewErrorMsg("list namespaces", context.DeadlineExceeded))
	m = next.(Model)
	staleGen := m.statusErrGen
	if !m.status.HasError() {
		t.Fatal("first error should be shown")
	}

	// A second error supersedes the first (bumps the generation).
	next, _ = m.Update(NewErrorMsg("watch pods", context.DeadlineExceeded))
	m = next.(Model)

	// The first error's clear timer is now stale: it must not clear the newer one.
	next, _ = m.Update(errorClearMsg{gen: staleGen})
	m = next.(Model)
	if !m.status.HasError() {
		t.Error("a stale clear timer must not wipe a newer error")
	}

	// The current clear timer fires and clears the bar.
	next, _ = m.Update(errorClearMsg{gen: m.statusErrGen})
	m = next.(Model)
	if m.status.HasError() {
		t.Error("the matching clear timer should clear the error")
	}
}

// TestInitStartsDiscovery proves a model with a discoverer wired begins the
// discovery pass on startup: Init emits the private startDiscoveryMsg, and
// delivering it calls the discoverer, starts the status-bar spinner, and issues a
// command (the batched spinner tick + discovery pump).
func TestInitStartsDiscovery(t *testing.T) {
	fd := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := sizedWith(t, WithDiscoverer(fd))

	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("Init should kick off discovery when a discoverer is wired")
	}
	if _, ok := initCmd().(startDiscoveryMsg); !ok {
		t.Fatalf("Init should emit startDiscoveryMsg, got %T", initCmd())
	}

	next, cmd := m.Update(startDiscoveryMsg{})
	m = next.(Model)
	if len(fd.ctxs) != 1 {
		t.Fatalf("expected 1 StartDiscovery call, got %d", len(fd.ctxs))
	}
	if !m.status.Discovering() {
		t.Fatal("starting discovery should start the status-bar spinner")
	}
	if cmd == nil {
		t.Fatal("starting discovery should batch the spinner tick + discovery pump")
	}
}

// TestInitInertWithoutDiscoverer proves a model with no discoverer never starts
// discovery: Init has no command and the spinner stays off.
func TestInitInertWithoutDiscoverer(t *testing.T) {
	m := sized(t) // no WithDiscoverer
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init should be a no-op without a discoverer, got a command yielding %T", cmd())
	}
	if m.status.Discovering() {
		t.Fatal("no discoverer means the spinner never starts")
	}
}

// TestMenuExtrasFoldedIn proves WithMenuExtras merges a per-context menu entry
// (FB-menu-config-03) into the seed menu at construction: an extra CRD the seed
// does not know shows up as a menu item, deduped by GVR (menu.AddExtras).
func TestMenuExtrasFoldedIn(t *testing.T) {
	extra := config.MenuResource{
		Group: "cert-manager.io", Version: "v1", Resource: "certificates",
		Kind: "Certificate", Namespaced: true,
	}
	base := len(sized(t).menu.Items())
	m := sizedWith(t, WithMenuExtras([]config.MenuResource{extra}))

	items := m.menu.Items()
	if len(items) != base+1 {
		t.Fatalf("WithMenuExtras should add one menu item: had %d, now %d", base, len(items))
	}
	found := false
	for _, it := range items {
		if it.Resource.GVR.Resource == "certificates" && it.Resource.GVR.Group == "cert-manager.io" {
			found = true
			if it.Title != "Certificate" {
				t.Errorf("extra title = %q, want %q", it.Title, "Certificate")
			}
		}
	}
	if !found {
		t.Fatal("the per-context extra CRD should appear in the menu")
	}
}

// TestStartupErrorSurfacesToast proves WithStartupError (e.g. a malformed
// per-context menu file the launcher chose not to make fatal) is surfaced as a
// transient status-bar toast on Init rather than swallowed — the app still launches
// on the default menu (principle 3).
func TestStartupErrorSurfacesToast(t *testing.T) {
	e := NewErrorMsg("menu config", errors.New("bad menu file"))
	m := New(WithStartupError(&e)) // no discoverer: Init emits only the toast

	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("Init should emit the seeded startup error")
	}
	msg := initCmd()
	errMsg, ok := msg.(ErrorMsg)
	if !ok {
		t.Fatalf("Init should yield the ErrorMsg toast, got %T", msg)
	}
	next, clearCmd := m.Update(errMsg)
	m = next.(Model)
	if !m.status.HasError() {
		t.Error("delivering the startup error should surface it in the status bar")
	}
	if clearCmd == nil {
		t.Fatal("surfacing the startup error should arm an auto-clear timer")
	}
}

// TestDiscoveryReadyReconcilesMenu proves a completed discovery pass folds into
// the menu (a CRD the seed omits is appended) and stops the spinner. It also
// checks the one-shot context is cancelled once the result is in hand.
func TestDiscoveryReadyReconcilesMenu(t *testing.T) {
	fd := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := sizedWith(t, WithDiscoverer(fd))

	next, _ := m.Update(startDiscoveryMsg{})
	m = next.(Model)
	before := len(m.menu.Items())

	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	next, cmd := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{crd}}})
	m = next.(Model)

	if m.status.Discovering() {
		t.Fatal("a ready discovery result should stop the spinner")
	}
	if got := len(m.menu.Items()); got != before+1 {
		t.Fatalf("discovery should append the CRD to the menu: had %d, now %d", before, got)
	}
	if fd.ctxs[0].Err() == nil {
		t.Fatal("the discovery context should be cancelled once its result is in hand")
	}
	if cmd != nil {
		t.Fatal("handling a discovery result should not issue a further command")
	}
}

// TestDiscoveryTotalFailureKeepsSeed proves a total discovery failure (Result.Err
// set) degrades gracefully: the spinner stops and the menu stays on its navigable
// seed rather than blanking (principle 3).
func TestDiscoveryTotalFailureKeepsSeed(t *testing.T) {
	fd := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := sizedWith(t, WithDiscoverer(fd))

	next, _ := m.Update(startDiscoveryMsg{})
	m = next.(Model)
	before := len(m.menu.Items())

	next, _ = m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Err: context.DeadlineExceeded}})
	m = next.(Model)

	if m.status.Discovering() {
		t.Fatal("a failed discovery result should still stop the spinner")
	}
	if got := len(m.menu.Items()); got != before {
		t.Fatalf("a total failure should leave the seed menu intact: had %d, now %d", before, got)
	}
}

// fakeLister is a hermetic NamespaceLister: it hands back a preset namespace set
// (or error) and counts calls.
type fakeLister struct {
	ns   []string
	err  error
	call int
}

func (f *fakeLister) Namespaces(context.Context) ([]string, error) {
	f.call++
	return f.ns, f.err
}

// ctrlN is the ns.switch default key.
var ctrlN = tea.Key{Code: 'n', Mod: tea.ModCtrl}

// TestNamespaceSwitchOpensAndSeeds proves ctrl+n (ns.switch) opens the picker and
// issues an async list whose result seeds it.
func TestNamespaceSwitchOpensAndSeeds(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	m := sizedWith(t, WithNamespaceLister(fl))

	m, cmd := press(t, m, ctrlN)
	if !m.nsPicker.Active() {
		t.Fatal("ns.switch should open the namespace picker")
	}
	if cmd == nil {
		t.Fatal("opening the picker should issue a namespace list command")
	}
	lm, ok := cmd().(namespacesLoadedMsg)
	if !ok {
		t.Fatalf("list command produced %T, want namespacesLoadedMsg", cmd())
	}
	next, _ := m.Update(lm)
	m = next.(Model)
	// 2 concrete namespaces + the pinned all-namespaces sentinel at the top.
	if got := m.nsPicker.Len(); got != 3 {
		t.Fatalf("picker seeded with %d entries, want 3 (2 namespaces + all-namespaces sentinel)", got)
	}
	if v, _ := m.nsPicker.Selected(); v != namespaceAllItem {
		t.Fatalf("sentinel should be pinned at the top, got %q", v)
	}
}

// TestNamespaceSwitchInertWithoutLister proves a model with no lister is
// namespace-switch-inert: ctrl+n opens nothing and issues no command.
func TestNamespaceSwitchInertWithoutLister(t *testing.T) {
	m := sized(t) // no WithNamespaceLister
	m, cmd := press(t, m, ctrlN)
	if m.nsPicker.Active() {
		t.Fatal("ns.switch without a lister should not open the picker")
	}
	if cmd != nil {
		t.Fatal("ns.switch without a lister should issue no command")
	}
}

// TestNamespaceListErrorClosesPicker proves a failed namespace list surfaces an
// error and closes the picker (degrade, don't crash — principle 3).
func TestNamespaceListErrorClosesPicker(t *testing.T) {
	fl := &fakeLister{err: context.DeadlineExceeded}
	m := sizedWith(t, WithNamespaceLister(fl))
	m, cmd := press(t, m, ctrlN)
	next, errCmd := m.Update(cmd()) // deliver namespacesLoadedMsg{err:…}
	m = next.(Model)
	if m.nsPicker.Active() {
		t.Fatal("a failed list should close the picker")
	}
	if errCmd == nil {
		t.Fatal("a failed list should surface an error")
	}
	if _, ok := errCmd().(ErrorMsg); !ok {
		t.Fatalf("expected ErrorMsg, got %T", errCmd())
	}
}

// TestNamespaceSelectRescopesWatch drives the whole switch: open the picker, filter
// to a namespace, drill in — the selection re-scopes the live watch (a second Watch
// with the new namespace) and updates m.namespace.
func TestNamespaceSelectRescopesWatch(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system", "monitoring"}}
	fw := &fakeWatcher{}
	m := sizedWith(t, WithNamespaceLister(fl), WithWatcher(fw))

	// Start a watch so the namespace change has something to re-scope.
	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if fw.ns[0] != "" {
		t.Fatalf("initial watch namespace = %q, want all (\"\")", fw.ns[0])
	}

	// Open + seed the picker.
	m, cmd := press(t, m, ctrlN)
	next, _ = m.Update(cmd())
	m = next.(Model)

	// Open the filter and narrow to "monitoring".
	m, _ = press(t, m, tea.Key{Code: '/', Text: "/"})
	if !m.nsPicker.Filtering() {
		t.Fatal("app.filter should open the picker filter")
	}
	for _, r := range "mon" {
		m, _ = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	if got := m.nsPicker.Len(); got != 1 {
		t.Fatalf("filter to 'mon' left %d items, want 1", got)
	}

	// Drill in (enter → nav.drillIn) selects the filtered value.
	m, selCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	sel, ok := selCmd().(picker.SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want picker.SelectedMsg", selCmd())
	}
	if sel.Value != "monitoring" {
		t.Fatalf("selected %q, want monitoring", sel.Value)
	}
	next, _ = m.Update(sel)
	m = next.(Model)

	if m.nsPicker.Active() {
		t.Fatal("selecting a namespace should close the picker")
	}
	if m.namespace != "monitoring" {
		t.Fatalf("m.namespace = %q, want monitoring", m.namespace)
	}
	if len(fw.ns) != 2 || fw.ns[1] != "monitoring" {
		t.Fatalf("selection should re-scope the watch to monitoring, got ns calls %v", fw.ns)
	}
}

// TestNamespacePickerCancels proves nav.back (esc) dismisses the picker without
// changing the namespace.
func TestNamespacePickerCancels(t *testing.T) {
	fl := &fakeLister{ns: []string{"default"}}
	m := sizedWith(t, WithNamespaceLister(fl))
	m, cmd := press(t, m, ctrlN)
	next, _ := m.Update(cmd())
	m = next.(Model)

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	// esc → nav.back → picker emits CancelledMsg; delivering it hides the picker.
	if cancelCmd == nil {
		t.Fatal("back should emit a cancel command")
	}
	next, _ = m.Update(cancelCmd())
	m = next.(Model)
	if m.nsPicker.Active() {
		t.Fatal("nav.back should close the picker")
	}
	if m.namespace != "" {
		t.Fatalf("cancelling should not change the namespace, got %q", m.namespace)
	}
}

// colon is the default resources.switch (command palette) key.
var colon = tea.Key{Code: ':', Text: ":"}

// availableResourceCount is how many menu rows the resource command palette should
// list: the available resource rows (the namespace seam and any unavailable row are
// excluded). Computed from the menu so the assertion tracks the seed instead of
// pinning a magic number.
func availableResourceCount(m Model) int {
	n := 0
	for _, it := range m.menu.Items() {
		if it.Kind == menu.ItemResource && it.Available {
			n++
		}
	}
	return n
}

// TestResourcePaletteOpensAndSeeds proves `:` (resources.switch) opens the resource
// command palette seeded with the menu's available resource kinds — the pane-free
// resource switch of FB-nav-resource-palette (D96 slice 2). It needs a watcher (the
// palette only makes sense when a resource can be watched).
func TestResourcePaletteOpensAndSeeds(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, cmd := press(t, m, colon)
	if !m.resPicker.Active() {
		t.Fatal("resources.switch should open the resource palette")
	}
	if cmd != nil {
		t.Fatalf("opening the palette should issue no command, got %T", cmd())
	}
	if got, want := m.resPicker.Len(), availableResourceCount(m); got != want {
		t.Fatalf("palette seeded with %d entries, want %d (available resource rows)", got, want)
	}
	if got := m.resPicker.Len(); got == 0 {
		t.Fatal("palette should list the seed resource kinds")
	}
}

// TestResourcePaletteInertWithoutWatcher proves a model with no watcher is
// switch-inert: `:` opens nothing (there is no live table to switch).
func TestResourcePaletteInertWithoutWatcher(t *testing.T) {
	m := sized(t) // no WithWatcher
	m, cmd := press(t, m, colon)
	if m.resPicker.Active() {
		t.Fatal("resources.switch without a watcher should not open the palette")
	}
	if cmd != nil {
		t.Fatal("resources.switch without a watcher should issue no command")
	}
}

// TestResourcePaletteSelectSwitchesResource drives the whole switch: open the palette,
// filter to a kind, drill in — the selection starts a watch for that resource, marks
// it active in the menu, and moves focus to the table (the same selectResource path a
// menu drill-in takes). Works with the menu hidden, so it is the pane-free switch.
func TestResourcePaletteSelectSwitchesResource(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	// Hide the menu first (menu.toggle = `m`) so this exercises the pane-free path.
	m, _ = press(t, m, tea.Key{Code: 'm', Text: "m"})
	if !m.menuHidden {
		t.Fatal("menu.toggle should hide the menu")
	}

	// Open the palette and filter to the unique kind "CronJob" (query "cron").
	m, _ = press(t, m, colon)
	m, _ = press(t, m, tea.Key{Code: '/', Text: "/"})
	if !m.resPicker.Filtering() {
		t.Fatal("app.filter should open the palette filter")
	}
	for _, r := range "cron" {
		m, _ = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	if got := m.resPicker.Len(); got != 1 {
		t.Fatalf("filter to 'cron' left %d items, want 1 (CronJob)", got)
	}

	// Drill in (enter → nav.drillIn) selects the filtered value, stamped with the
	// resource picker's Kind so the root routes it to selectResource, not namespaces.
	m, selCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	sel, ok := selCmd().(picker.SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want picker.SelectedMsg", selCmd())
	}
	if sel.Kind != resourcePickerKind {
		t.Fatalf("palette selection Kind = %q, want %q", sel.Kind, resourcePickerKind)
	}
	if sel.Value != "CronJob" {
		t.Fatalf("selected %q, want CronJob", sel.Value)
	}
	next, _ := m.Update(sel)
	m = next.(Model)

	if m.resPicker.Active() {
		t.Fatal("selecting a resource should close the palette")
	}
	if !m.hasCurrent || m.current.GVR.Resource != "cronjobs" {
		t.Fatalf("selection should start a watch for cronjobs, got current=%+v hasCurrent=%v", m.current.GVR, m.hasCurrent)
	}
	if !m.table.Focused() {
		t.Fatal("switching a resource should move focus to the table")
	}
	if len(fw.res) != 1 || fw.res[0].GVR.Resource != "cronjobs" {
		t.Fatalf("palette selection should watch cronjobs, got watch calls %v", fw.res)
	}
}

// TestResourcePaletteCancels proves nav.back (esc) dismisses the palette without
// switching the resource.
func TestResourcePaletteCancels(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	m, _ = press(t, m, colon)
	if !m.resPicker.Active() {
		t.Fatal("resources.switch should open the palette")
	}
	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("back should emit a cancel command")
	}
	next, _ := m.Update(cancelCmd())
	m = next.(Model)
	if m.resPicker.Active() {
		t.Fatal("nav.back should close the palette")
	}
	if m.hasCurrent {
		t.Fatal("cancelling should not start a watch")
	}
	if len(fw.res) != 0 {
		t.Fatalf("cancelling should issue no watch, got %v", fw.res)
	}
}

// TestMenuSeamOpensNamespacePicker proves the namespace-seam row in the left menu
// opens the namespace picker on drill-in — the same effect as ctrl+n — driven
// through the real update loop: walk the menu cursor down to the seam, press enter,
// and the emitted menu.NamespaceRequestedMsg opens and seeds the picker.
func TestMenuSeamOpensNamespacePicker(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	m := sizedWith(t, WithNamespaceLister(fl))

	// Walk down to the seam row (nav.down = `j`); bounded so a regression can't hang.
	seamReached := false
	for i := 0; i < len(m.menu.Items()); i++ {
		if sel, ok := m.menu.Selected(); ok && sel.Kind == menu.ItemNamespace {
			seamReached = true
			break
		}
		m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	}
	if !seamReached {
		t.Fatal("never landed on the namespace seam walking the menu down")
	}

	// Enter (nav.drillIn) emits the menu's NamespaceRequestedMsg; deliver it.
	m, cmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("drilling into the seam produced no command")
	}
	if _, ok := cmd().(menu.NamespaceRequestedMsg); !ok {
		t.Fatalf("seam drill-in emitted %T, want menu.NamespaceRequestedMsg", cmd())
	}
	next, openCmd := m.Update(menu.NamespaceRequestedMsg{})
	m = next.(Model)
	if !m.nsPicker.Active() {
		t.Fatal("the seam should open the namespace picker")
	}
	if openCmd == nil {
		t.Fatal("opening the picker should issue a namespace list command")
	}
	if _, ok := openCmd().(namespacesLoadedMsg); !ok {
		t.Fatalf("list command produced %T, want namespacesLoadedMsg", openCmd())
	}
}

// TestNamespaceSelectionUpdatesMenuSeam proves picking a namespace re-scopes the
// menu seam's displayed namespace (not just the status bar / welcome).
func TestNamespaceSelectionUpdatesMenuSeam(t *testing.T) {
	// A wide terminal so the menu pane (a quarter of the width) is broad enough to
	// render the full seam label without truncating the namespace name.
	base := New(WithNamespaceLister(&fakeLister{ns: []string{"kube-system"}}))
	sz, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m := sz.(Model)
	next, _ := m.Update(picker.SelectedMsg{Value: "kube-system"})
	m = next.(Model)
	if got := menuSeamNamespace(t, m); got != "kube-system" {
		t.Fatalf("menu seam namespace = %q, want kube-system", got)
	}
}

// TestNamespaceAllSentinelResetsScope proves the pinned "all namespaces" picker
// entry maps back to the empty (unscoped) scope, so a user who drilled into a
// concrete namespace can return to the all-namespaces view (dogfood-09 dead-end).
func TestNamespaceAllSentinelResetsScope(t *testing.T) {
	base := New(WithNamespaceLister(&fakeLister{ns: []string{"kube-system"}}))
	sz, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m := sz.(Model)
	// Scope into a concrete namespace first.
	next, _ := m.Update(picker.SelectedMsg{Value: "kube-system"})
	m = next.(Model)
	if m.namespace != "kube-system" {
		t.Fatalf("precondition: scope = %q, want kube-system", m.namespace)
	}
	// Selecting the sentinel returns to the unscoped view.
	next, _ = m.Update(picker.SelectedMsg{Value: namespaceAllItem})
	m = next.(Model)
	if m.namespace != "" {
		t.Fatalf("all-namespaces sentinel = %q scope, want empty (unscoped)", m.namespace)
	}
	if got := menuSeamNamespace(t, m); got != "(all)" {
		t.Fatalf("seam after sentinel = %q, want (all)", got)
	}
}

// fakePersister is a hermetic NamespacePersister recording the namespace it was last
// asked to persist (and an optional error to return).
type fakePersister struct {
	got    string
	called bool
	err    error
}

func (f *fakePersister) PersistNamespace(ns string) error {
	f.called = true
	f.got = ns
	return f.err
}

// namespacePersisterModel builds a sized model wired to fp (and a lister) for the
// namespace-persistence tests.
func namespacePersisterModel(fp *fakePersister) Model {
	base := New(WithNamespaceLister(&fakeLister{ns: []string{"kube-system"}}), WithNamespacePersister(fp))
	sz, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return sz.(Model)
}

// TestNamespaceSelectionPersists proves picking a namespace writes it through the
// persister seam (M2-11b-2), off the update loop — so the next launch restores it.
func TestNamespaceSelectionPersists(t *testing.T) {
	fp := &fakePersister{}
	m := namespacePersisterModel(fp)
	_, cmd := m.Update(picker.SelectedMsg{Value: "kube-system"})
	if cmd == nil {
		t.Fatal("selecting a namespace should issue a persist command")
	}
	cmd() // run the off-loop write
	if !fp.called || fp.got != "kube-system" {
		t.Fatalf("persisted namespace = %q (called=%v), want kube-system", fp.got, fp.called)
	}
}

// TestNamespaceSentinelPersistsUnscoped proves selecting the all-namespaces sentinel
// persists the empty (unscoped) scope, not the literal sentinel label.
func TestNamespaceSentinelPersistsUnscoped(t *testing.T) {
	fp := &fakePersister{}
	m := namespacePersisterModel(fp)
	_, cmd := m.Update(picker.SelectedMsg{Value: namespaceAllItem})
	if cmd == nil {
		t.Fatal("selecting the sentinel should issue a persist command")
	}
	cmd()
	if !fp.called || fp.got != "" {
		t.Fatalf("persisted namespace = %q (called=%v), want \"\" (unscoped)", fp.got, fp.called)
	}
}

// TestNamespacePersistInertWithoutPersister proves a model with no persister does
// not attempt to persist: picking a namespace still applies but issues no command.
func TestNamespacePersistInertWithoutPersister(t *testing.T) {
	base := New(WithNamespaceLister(&fakeLister{ns: []string{"kube-system"}}))
	sz, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m := sz.(Model)
	next, cmd := m.Update(picker.SelectedMsg{Value: "kube-system"})
	if cmd != nil {
		t.Fatal("no persister should issue no persist command")
	}
	if next.(Model).namespace != "kube-system" {
		t.Fatal("the picked scope should still apply without a persister")
	}
}

// TestNamespacePersistErrorSurfacesToast proves a persister write failure degrades
// to a transient error toast (principle 3) rather than crashing — the picked scope
// still applies for the session.
func TestNamespacePersistErrorSurfacesToast(t *testing.T) {
	fp := &fakePersister{err: context.DeadlineExceeded}
	m := namespacePersisterModel(fp)
	next, cmd := m.Update(picker.SelectedMsg{Value: "kube-system"})
	if cmd == nil {
		t.Fatal("selecting a namespace should issue a persist command")
	}
	if _, ok := cmd().(ErrorMsg); !ok {
		t.Fatalf("a persist failure should surface an ErrorMsg, got %T", cmd())
	}
	if next.(Model).namespace != "kube-system" {
		t.Fatal("the picked scope should still apply despite the persist failure")
	}
}

// menuSeamNamespace renders the sized menu and returns the namespace the seam row
// shows ("(all)" when unscoped), read back from the rendered view.
func menuSeamNamespace(t *testing.T, m Model) string {
	t.Helper()
	v := m.menu.View()
	if strings.Contains(v, "kube-system") {
		return "kube-system"
	}
	if strings.Contains(v, "(all)") {
		return "(all)"
	}
	t.Fatalf("menu view shows no recognisable seam scope:\n%s", v)
	return ""
}

// TestNamespacePickerCapturesInput proves the open picker captures navigation: a
// nav key does not reach the panes underneath (the menu cursor stays put).
func TestNamespacePickerCapturesInput(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	m := sizedWith(t, WithNamespaceLister(fl))
	m, cmd := press(t, m, ctrlN)
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.menu.Cursor() != 0 {
		t.Fatalf("menu should start at cursor 0, got %d", m.menu.Cursor())
	}
	// nav.down while the picker is open moves the picker cursor, not the menu. The
	// picker starts on the pinned all-namespaces sentinel (row 0), so one step lands
	// on the first concrete namespace.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 0 {
		t.Fatalf("picker should capture nav.down; menu moved to %d", m.menu.Cursor())
	}
	if v, _ := m.nsPicker.Selected(); v != "default" {
		t.Fatalf("nav.down should move the picker cursor to default, got %q", v)
	}
}

// multiReset builds a RESET watch event with one NAME row per name (UID == name),
// so a test can populate the table with several rows and then filter across them.
func multiReset(names ...string) kube.WatchEvent {
	rows := make([]kube.Row, len(names))
	for i, n := range names {
		rows[i] = kube.Row{Cells: []any{n}, Object: kube.ObjectRef{Name: n, UID: n}}
	}
	return kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}},
		Rows:    rows,
	}
}

// tableWith returns a sized model showing a live table for `pods` populated with the
// given row names, plus the fake watcher backing it. It drives the real
// select→pump→ApplyEvent path so hasCurrent is set and the rows are the displayed
// set the filter narrows.
func tableWith(t *testing.T, names ...string) (Model, *fakeWatcher) {
	t.Helper()
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	fw.chans[0] <- multiReset(names...)
	wm, ok := cmd().(watchMsg)
	if !ok {
		t.Fatalf("pump produced %T, want watchMsg", cmd())
	}
	next, _ = m.Update(wm)
	m = next.(Model)
	if m.table.RowCount() != len(names) {
		t.Fatalf("table seeded with %d rows, want %d", m.table.RowCount(), len(names))
	}
	return m, fw
}

var slash = tea.Key{Code: '/', Text: "/"}

// typeStr feeds each rune of s to the model as a keypress.
func typeStr(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	return m
}

// TestFilterOpensAndNarrows proves app.filter (`/`) opens the field over the current
// table and typing narrows the displayed rows live (D78), leaving the full set intact.
func TestFilterOpensAndNarrows(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")

	m, _ = press(t, m, slash)
	if !m.filtering {
		t.Fatal("app.filter should open the filter field")
	}
	if !m.table.Focused() {
		t.Fatal("opening the filter should focus the table")
	}
	m = typeStr(t, m, "web")
	if m.table.RowCount() != 2 {
		t.Fatalf("filter 'web' left %d rows, want 2", m.table.RowCount())
	}
	if m.table.TotalRowCount() != 3 {
		t.Fatalf("filter must not drop rows from the full set: total %d, want 3", m.table.TotalRowCount())
	}
	if m.table.Filter() != "web" {
		t.Fatalf("table filter = %q, want web", m.table.Filter())
	}
	// The active filter surfaces in the status bar (inside the fixed layout, like the
	// error toast) and the View renders without breaking.
	if !bytes.Contains([]byte(m.View().Content), []byte("web")) {
		t.Fatalf("filtering View should show the filter query in the status bar: %q", m.View().Content)
	}
}

// TestFilterInertWithoutTable proves `/` does nothing before a resource is drilled
// into (the welcome page is showing, so there is nothing to narrow).
func TestFilterInertWithoutTable(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{})) // watcher wired, but no selection yet
	m, cmd := press(t, m, slash)
	if m.filtering {
		t.Fatal("app.filter should be inert with no current table")
	}
	if cmd != nil {
		t.Fatal("inert filter open should issue no command")
	}
}

// TestFilterCommitKeepsNarrowing proves enter commits the narrowed view: the input
// closes (filtering false) but the filter stays applied so the rows remain narrowed.
func TestFilterCommitKeepsNarrowing(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")

	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter})
	if m.filtering {
		t.Fatal("enter should close the filter input")
	}
	if m.table.Filter() != "web" {
		t.Fatalf("commit should keep the filter applied, got %q", m.table.Filter())
	}
	if m.table.RowCount() != 2 {
		t.Fatalf("committed filter should keep the rows narrowed, got %d", m.table.RowCount())
	}
}

// TestFilterCancelRestores proves esc while editing clears the filter and closes the
// input, bringing every row back (D78).
func TestFilterCancelRestores(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")
	if m.table.RowCount() != 2 {
		t.Fatalf("precondition: filter should narrow to 2, got %d", m.table.RowCount())
	}

	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.filtering {
		t.Fatal("esc should close the filter input")
	}
	if m.table.Filter() != "" {
		t.Fatalf("esc should clear the filter, got %q", m.table.Filter())
	}
	if m.table.RowCount() != 3 {
		t.Fatalf("clearing the filter should restore all rows, got %d", m.table.RowCount())
	}
}

// TestCommittedFilterClearedByEsc proves esc on a committed filter (input closed)
// clears the applied narrowing — esc exits the filtered view.
func TestCommittedFilterClearedByEsc(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")
	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter}) // commit
	if m.table.Filter() != "web" {
		t.Fatalf("precondition: committed filter should be 'web', got %q", m.table.Filter())
	}

	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.table.Filter() != "" {
		t.Fatalf("esc on a committed filter should clear it, got %q", m.table.Filter())
	}
	if m.table.RowCount() != 3 {
		t.Fatalf("clearing should restore all rows, got %d", m.table.RowCount())
	}
}

// TestEscPopsTableFocusToMenu proves nav.back (esc) is the "back to the menu"
// gesture: with the table focused and no filter, esc pops focus to the left menu
// pane. Esc while the menu already holds focus is inert.
func TestEscPopsTableFocusToMenu(t *testing.T) {
	m := sized(t)

	// Focus the table (nav.right), then esc → focus back to the menu.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("precondition: nav.right should focus the table")
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("esc with the table focused should pop focus back to the menu")
	}

	// A second esc with the menu already focused is inert (no panic, focus stays).
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("esc with the menu focused should leave focus on the menu")
	}
}

// TestEscClearsFilterBeforePoppingFocus proves the one-level-per-press ordering: on
// a committed filter with the table focused, the first esc clears the filter (focus
// stays on the table), and only the next esc pops focus back to the menu.
func TestEscClearsFilterBeforePoppingFocus(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")
	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter}) // commit; table stays focused
	if m.table.Filter() != "web" || !m.table.Focused() {
		t.Fatalf("precondition: committed filter on a focused table, got %q focused=%v", m.table.Filter(), m.table.Focused())
	}

	// First esc clears the filter but keeps focus on the table (one level).
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.table.Filter() != "" {
		t.Fatalf("first esc should clear the committed filter, got %q", m.table.Filter())
	}
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("first esc should not also pop focus — one level per press")
	}

	// Second esc pops focus back to the menu.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("second esc should pop focus back to the menu")
	}
}

// TestFilterLetterKeysTypeNotNavigate proves the control/text split (D73): while the
// filter is open, a bound vim letter (`j`) types into the field rather than moving
// the selection, but a no-text nav key (down arrow) still moves it.
func TestFilterLetterKeysTypeNotNavigate(t *testing.T) {
	m, _ := tableWith(t, "j-pod", "web-1", "web-2")
	m, _ = press(t, m, slash)
	// `j` carries text, so it types into the filter (narrowing to the "j-pod" row),
	// it does not move the menu/table selection.
	m = typeStr(t, m, "j")
	if m.table.Filter() != "j" {
		t.Fatalf("a letter key should type into the filter, got %q", m.table.Filter())
	}
	if m.table.RowCount() != 1 {
		t.Fatalf("filter 'j' should narrow to the j-pod row, got %d", m.table.RowCount())
	}
	// A no-text nav key (down arrow) is a control action routed to the table.
	m, _ = press(t, m, tea.Key{Code: tea.KeyDown})
	if !m.filtering {
		t.Fatal("a nav key should not close the filter")
	}
}

// TestSearchWrapsThroughMatches proves n/N step the selection through the matching
// rows with wrap-around, once a filter is committed (this leg's n/N decision).
func TestSearchWrapsThroughMatches(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")                        // 2 matches: web-1, web-2
	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter}) // commit; cursor at 0
	if m.table.Cursor() != 0 {
		t.Fatalf("commit should leave the cursor at 0, got %d", m.table.Cursor())
	}

	n := tea.Key{Code: 'n', Text: "n"}
	shiftN := tea.Key{Code: 'N', Text: "N"}

	m, _ = press(t, m, n) // 0 -> 1
	if m.table.Cursor() != 1 {
		t.Fatalf("searchNext should advance to 1, got %d", m.table.Cursor())
	}
	m, _ = press(t, m, n) // 1 -> wrap to 0
	if m.table.Cursor() != 0 {
		t.Fatalf("searchNext at the last match should wrap to 0, got %d", m.table.Cursor())
	}
	m, _ = press(t, m, shiftN) // 0 -> wrap to 1
	if m.table.Cursor() != 1 {
		t.Fatalf("searchPrev at the first match should wrap to 1, got %d", m.table.Cursor())
	}
}

// TestSearchInertWithoutFilter proves n is a no-op with no active filter — there are
// no matches to iterate.
func TestSearchInertWithoutFilter(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	// Focus the table but set no filter.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	before := m.table.Cursor()
	m, cmd := press(t, m, tea.Key{Code: 'n', Text: "n"})
	if m.table.Cursor() != before {
		t.Fatalf("searchNext without a filter should not move the cursor: %d -> %d", before, m.table.Cursor())
	}
	if cmd != nil {
		t.Fatal("searchNext without a filter should issue no command")
	}
}

// TestNewResourceClearsFilter proves selecting a different resource resets the shell
// filter state (SetTable clears the table filter, D78; the shell mirrors it).
func TestNewResourceClearsFilter(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m, _ = press(t, m, slash)
	m = typeStr(t, m, "web")
	if m.table.Filter() == "" || !m.filtering {
		t.Fatal("precondition: a filter should be open and applied")
	}

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("nodes")})
	m = next.(Model)
	if m.filtering {
		t.Fatal("selecting a new resource should close the filter input")
	}
	if m.table.Filter() != "" {
		t.Fatalf("selecting a new resource should clear the filter, got %q", m.table.Filter())
	}
}

// TestHelpSwallowsNav proves the open help overlay swallows navigation: focus does
// not switch while help is visible.
func TestHelpSwallowsNav(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help should open the overlay")
	}
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if m.table.Focused() {
		t.Fatal("nav.right should not switch panes while help is open")
	}
}

// TestReconcilePreservesSelection proves the update loop's menu-reconcile path keeps
// the user's selection put: with the cursor moved off the first seed item, a
// DiscoveryReadyMsg that appends a CRD leaves the same resource selected (resolved by
// GVR, D57) even though the item slice grew. TestDiscoveryReadyReconcilesMenu covers
// the append + spinner stop but never moves the cursor first, so the preservation
// guarantee — the M2 menu-reconcile risk item — is only asserted here.
func TestReconcilePreservesSelection(t *testing.T) {
	m := sized(t)

	// Move the menu selection off row 0 through the real action path (nav.down).
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() == 0 {
		t.Fatal("precondition: nav.down should move the selection off the first item")
	}
	before, ok := m.menu.Selected()
	if !ok {
		t.Fatal("the seed menu should have a selection")
	}
	selectedGVR := before.Resource.GVR
	itemsBefore := len(m.menu.Items())

	// A discovery result adding a CRD the seed omits (appended after the seed).
	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	next, cmd := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{crd}}})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("handling a discovery result should not issue a further command")
	}

	if got := len(m.menu.Items()); got != itemsBefore+1 {
		t.Fatalf("reconcile should append the CRD: had %d items, now %d", itemsBefore, got)
	}
	after, ok := m.menu.Selected()
	if !ok {
		t.Fatal("the menu should still have a selection after reconcile")
	}
	if after.Resource.GVR != selectedGVR {
		t.Fatalf("reconcile must preserve the selected resource: was %v, now %v", selectedGVR, after.Resource.GVR)
	}
}

// TestProgramRoutesKeyToPicker drives the whole update loop through the real
// bubbletea program (teatest/v2, the M0-05 harness) rather than a direct Update
// call: a live ctrl+n keypress routes through the keymap to ns.switch, the async
// namespace list seeds the picker, and the seeded namespaces render — proving
// key→action→command→msg→View round-trips end to end through the running program.
func TestProgramRoutesKeyToPicker(t *testing.T) {
	fl := &fakeLister{ns: []string{"default", "kube-system"}}
	tm := teatest.NewTestModel(t, New(WithNamespaceLister(fl)), teatest.WithInitialTermSize(80, 24))

	// The browse layout draws first (a seed kind in the left pane).
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	// A live ctrl+n opens the namespace picker; its async list then seeds it and the
	// namespaces render through the program's View. "kube-system" is an unselected
	// picker row, so a plain byte scan sees it (the selected row's background-filled
	// cells a scan misses — same reason TestModelSmoke keys on an unselected row).
	tm.Send(tea.KeyPressMsg(ctrlN))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("kube-system"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestProgramFilterFlow drives the landed filter flow (M2-09b) end-to-end through
// the real bubbletea program (teatest/v2, the M0-05 harness) rather than direct
// Update calls: after a resource is drilled in and its watch delivers rows, a live
// `/` keypress routes through the keymap to app.filter, typed runes flow into the
// textinput, and enter commits. The assertion reads the *final model* rather than
// scanning the rendered output — the status-bar filter segment is background-styled,
// so the terminal emulator writes its cells via a path a plain byte scan misses (the
// same reason the other program tests key on unselected rows). FinalModel proves the
// running program delivered every key and committed the filter: the query is applied,
// the input is closed, and the row set narrowed to the matches — a full
// key→action→textinput→SetFilter round-trip that the direct-Update tests exercise
// only by hand-threading commands.
func TestProgramFilterFlow(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{multiReset("web-1", "web-2", "api-1")}}
	tm := teatest.NewTestModel(t, New(WithWatcher(fw)), teatest.WithInitialTermSize(80, 24))

	// Browse layout draws first.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	// Drill into a resource; the preloaded watch delivers rows through the real pump,
	// so the table populates and an unselected row ("web-2") renders.
	tm.Send(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("web-2"))
	}, teatest.WithDuration(3*time.Second))

	// Live `/`, type the query, commit with enter — all through the running program.
	tm.Send(tea.KeyPressMsg(slash))
	for _, r := range "web" {
		tm.Send(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	tm.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	fm, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model is %T, want Model", tm.FinalModel(t))
	}
	if fm.filtering {
		t.Fatal("enter should have committed the filter, closing the input (filtering=false)")
	}
	if fm.table.Filter() != "web" {
		t.Fatalf("committed filter = %q, want web", fm.table.Filter())
	}
	if got := fm.table.RowCount(); got != 2 {
		t.Fatalf("filter 'web' should narrow to 2 rows through the program, got %d", got)
	}
	if got := fm.table.TotalRowCount(); got != 3 {
		t.Fatalf("filter must not drop rows from the full set: total %d, want 3", got)
	}
}

// TestProgramErrorToastDegradesGracefully drives the D74 error-toast path
// (FB-errors-layout: a classified error must degrade into a transient status-bar
// toast, never break the fixed browse layout) through the real bubbletea program
// (teatest/v2, the M0-05 harness) rather than a hand-threaded direct Update. A live
// ErrorMsg is delivered to the running program — the same value selectResource,
// namespace-list, and the watch pump emit on failure (that wiring is already covered
// by TestWatchStartErrorSurfaces et al.); what this adds is the composed render: the
// program routes ErrorMsg → surfaceError → the status bar, and the final model's own
// View proves the toast reached the screen inside the fixed layout.
//
// The assertion reads fm.View().Content (the model's raw render string), not the
// emulated terminal output: the whole status bar is background-styled, so its cells —
// toast included — are written through a path a plain byte scan of teatest.Output()
// misses (the same reason TestProgramFilterFlow asserts on the final model). The raw
// View string still contains the toast text and its true line count, so it proves both
// that the toast rendered and that it stayed within one screen (no grown pane, no
// scroll — the FB-errors-layout regression this guards against).
func TestProgramErrorToastDegradesGracefully(t *testing.T) {
	tm := teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(80, 24))

	// Browse layout draws first (a seed kind in the left pane).
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	// A classified error reaches the running program; Update routes it through
	// surfaceError to the status bar (processed synchronously before the Quit that
	// follows it in the queue, so the final model is deterministic).
	tm.Send(NewErrorMsg("watch pods", errors.New("induced watch failure")))
	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	fm, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model is %T, want Model", tm.FinalModel(t))
	}
	if !fm.status.HasError() {
		t.Fatal("the program should have surfaced the error into the status bar toast")
	}
	view := fm.View().Content
	if !strings.Contains(view, "induced watch failure") {
		t.Fatal("the toast text should appear in the composed program render")
	}
	// The toast lives inside the fixed layout: the error view is still exactly one
	// screen tall — the same line count as an error-free model — so it grew no pane
	// and scrolled nothing (the regression FB-errors-layout / D74 fixed).
	baseline := strings.Count(sized(t).View().Content, "\n")
	if got := strings.Count(view, "\n"); got != baseline {
		t.Fatalf("error toast broke the layout: view has %d newlines, want %d (one screen)", got, baseline)
	}
}

// TestMouseClickMenuOpensResource proves a left click on a menu resource row
// selects it and opens it (dogfood-08): the click resolves to the clicked item and
// drives the same drill-in path a keyboard nav.drillIn takes, emitting the menu's
// ResourceSelectedMsg for that resource.
func TestMouseClickMenuOpensResource(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	// Screen row 0 is the top status bar; the body starts at row 1. Menu display
	// rows (offset 0): row0 header "Cluster", row1 namespaces, row2 nodes. Content
	// row = Y-statusBarHeight-1, so Y=4 lands on the "nodes" item.
	next, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})
	m = next.(Model)
	if m.menu.Cursor() != 1 {
		t.Fatalf("click should move the menu cursor to the clicked item (nodes, idx 1), got %d", m.menu.Cursor())
	}
	if cmd == nil {
		t.Fatal("clicking a menu resource row should emit a drill-in command")
	}
	sel, ok := cmd().(menu.ResourceSelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want menu.ResourceSelectedMsg", cmd())
	}
	if sel.Resource.GVR.Resource != "nodes" {
		t.Fatalf("clicked-open resource = %q, want nodes", sel.Resource.GVR.Resource)
	}
}

// TestMouseClickMenuHeaderInert proves a click on a section-header line (a
// non-item line) selects nothing and emits no command.
func TestMouseClickMenuHeaderInert(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	before := m.menu.Cursor()
	// Screen row 0 is the top status bar; Y=2 → body row 1 → content row 0 → the
	// "Cluster" section header.
	next, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft})
	m = next.(Model)
	if m.menu.Cursor() != before || cmd != nil {
		t.Fatal("clicking a section header should be inert (no selection, no command)")
	}
}

// TestMouseClickTableSelectsRow proves a left click on a table data row selects
// that row and moves focus to the table (dogfood-08).
func TestMouseClickTableSelectsRow(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	// Open a resource and populate its table with three rows.
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	fw.chans[0] <- kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}},
		Rows: []kube.Row{
			{Cells: []any{"pod-a"}, Object: kube.ObjectRef{Name: "pod-a", UID: "a"}},
			{Cells: []any{"pod-b"}, Object: kube.ObjectRef{Name: "pod-b", UID: "b"}},
			{Cells: []any{"pod-c"}, Object: kube.ObjectRef{Name: "pod-c", UID: "c"}},
		},
	}
	wm, ok := cmd().(watchMsg)
	if !ok {
		t.Fatalf("pump produced %T, want watchMsg", cmd())
	}
	next, _ = m.Update(wm)
	m = next.(Model)
	if m.table.RowCount() != 3 {
		t.Fatalf("setup: table has %d rows, want 3", m.table.RowCount())
	}
	// Move focus off the table to prove the click moves it back.
	m.table.Blur()
	m.menu.Focus()
	// Table pane starts at X=menuPaneWidth(80)=20; screen row 0 is the top status
	// bar, so Y=4 → body row 3 → content row 2 (row 0 is the column header) selects
	// data row 1 (pod-b).
	next, _ = m.Update(tea.MouseClickMsg{X: 30, Y: 4, Button: tea.MouseLeft})
	m = next.(Model)
	if m.table.Cursor() != 1 {
		t.Fatalf("clicking the second data row should select row 1, got %d", m.table.Cursor())
	}
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("clicking the table should move focus to it")
	}
}

// TestMouseWheelScrollsPaneUnderPointer proves a wheel notch steps the selection of
// whichever pane the pointer is over, without changing focus (dogfood-08).
func TestMouseWheelScrollsPaneUnderPointer(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	c0 := m.menu.Cursor()
	// Wheel down over the menu (X in the left pane) steps its selection down.
	next, _ := m.Update(tea.MouseWheelMsg{X: 3, Y: 5, Button: tea.MouseWheelDown})
	m = next.(Model)
	if m.menu.Cursor() != c0+1 {
		t.Fatalf("wheel-down over the menu should move its cursor to %d, got %d", c0+1, m.menu.Cursor())
	}
	// Wheel up brings it back.
	next, _ = m.Update(tea.MouseWheelMsg{X: 3, Y: 5, Button: tea.MouseWheelUp})
	m = next.(Model)
	if m.menu.Cursor() != c0 {
		t.Fatalf("wheel-up should move the menu cursor back to %d, got %d", c0, m.menu.Cursor())
	}
	// Scrolling is a read gesture — the menu stays focused, no pane switch.
	if !m.menu.Focused() {
		t.Fatal("scroll-wheel should not change focus")
	}
}

// TestStatusBarShowsBrowsedResourceType proves drilling into a resource names its
// kind on the status bar alongside context · namespace (feedback
// 2026-07-22-status-bar-top): before any drill-in the bar shows no resource type,
// and after opening a resource its GVK.Kind appears in the bar's view.
func TestStatusBarShowsBrowsedResourceType(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	if strings.Contains(m.status.View(), "Pod") {
		t.Fatalf("status bar named a resource type before any drill-in: %q", m.status.View())
	}
	podRes := kube.Resource{
		GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: podRes})
	m = next.(Model)
	if !strings.Contains(m.status.View(), "Pod") {
		t.Fatalf("status bar should name the browsed kind (Pod) after drill-in, got %q", m.status.View())
	}
}

// TestMouseInertWhileOverlayOpen proves the mouse is inert while a modal/overlay is
// capturing input, so a click cannot reach the panes underneath it.
func TestMouseInertWhileOverlayOpen(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))
	m.help.SetVisible(true)
	c0 := m.menu.Cursor()
	next, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	m = next.(Model)
	if m.menu.Cursor() != c0 || cmd != nil {
		t.Fatal("a click while the help overlay is open should be inert")
	}
	next, _ = m.Update(tea.MouseWheelMsg{X: 3, Y: 5, Button: tea.MouseWheelDown})
	m = next.(Model)
	if m.menu.Cursor() != c0 {
		t.Fatal("a wheel event while an overlay is open should be inert")
	}
}

// mouseM is the default mouse.toggle key.
var mouseM = tea.Key{Code: 'M', Text: "M"}

// TestMouseCaptureOffByDefault proves the app does not capture the mouse on start
// (D97), so the terminal keeps its native select-to-copy: View sets MouseModeNone.
func TestMouseCaptureOffByDefault(t *testing.T) {
	m := sized(t)
	if m.mouseEnabled {
		t.Fatal("mouse capture should be off by default")
	}
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("default View MouseMode = %v, want MouseModeNone (native select-to-copy)", got)
	}
}

// TestMouseToggleEnablesCapture proves the mouse.toggle key (default `M`) flips
// capture on — View then requests MouseModeCellMotion and the status bar shows the
// `mouse` marker — and a second press flips it back off (native selection restored).
func TestMouseToggleEnablesCapture(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, mouseM)
	if !m.mouseEnabled {
		t.Fatal("mouse.toggle should enable mouse capture")
	}
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("after toggle View MouseMode = %v, want MouseModeCellMotion", got)
	}
	if !strings.Contains(m.status.View(), "mouse") {
		t.Fatal("status bar should show the `mouse` marker while capture is on")
	}
	m, _ = press(t, m, mouseM)
	if m.mouseEnabled {
		t.Fatal("a second mouse.toggle should disable mouse capture")
	}
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("after second toggle View MouseMode = %v, want MouseModeNone", got)
	}
	if strings.Contains(m.status.View(), "mouse") {
		t.Fatal("status bar should not show the `mouse` marker once capture is off")
	}
}

// TestMouseToggleWorksWhileHelpOpen proves mouse.toggle is an app mode toggle, not
// navigation: it flips capture even while the help overlay is up (the overlay
// swallows navigation but not this app-level toggle).
func TestMouseToggleWorksWhileHelpOpen(t *testing.T) {
	m := sized(t)
	m.help.SetVisible(true)
	m, _ = press(t, m, mouseM)
	if !m.mouseEnabled {
		t.Fatal("mouse.toggle should work while the help overlay is open")
	}
}

func TestMenuPaneWidthNarrowsWideTerminals(t *testing.T) {
	cases := []struct {
		name  string
		total int
		want  int
	}{
		{"zero", 0, 0},
		{"eighty-floors-at-min", 80, minMenuWidth}, // 80/4 == 20 == floor
		{"wide-caps-at-max", 200, maxMenuWidth},    // 200/4 == 50, capped to 28
		{"very-wide-caps-at-max", 400, maxMenuWidth},
		{"narrow-even-split", 39, 19}, // floor(20) would starve table → total/2
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := menuPaneWidth(tc.total); got != tc.want {
				t.Fatalf("menuPaneWidth(%d) = %d, want %d", tc.total, got, tc.want)
			}
		})
	}
	// The cap must never drop the pane below the floor, and the table must always
	// keep at least minTableWidth once the terminal is wide enough to grant it.
	for total := 1; total <= 500; total++ {
		w := menuPaneWidth(total)
		if w < 0 || w > total {
			t.Fatalf("menuPaneWidth(%d) = %d out of range", total, w)
		}
		if total >= minMenuWidth+minTableWidth && w > total-minTableWidth {
			t.Fatalf("menuPaneWidth(%d) = %d starves the table (< %d)", total, w, minTableWidth)
		}
		if w > maxMenuWidth {
			t.Fatalf("menuPaneWidth(%d) = %d exceeds maxMenuWidth %d", total, w, maxMenuWidth)
		}
	}
}

// sortReset is a two-column, two-row watch RESET used to give the app a live table
// the sort-cycle test can order.
func sortReset() kube.WatchEvent {
	return kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}, {Name: "STATUS"}},
		Rows: []kube.Row{
			{Cells: []any{"pod-b", "Running"}, Object: kube.ObjectRef{Name: "pod-b", UID: "b"}},
			{Cells: []any{"pod-a", "Pending"}, Object: kube.ObjectRef{Name: "pod-a", UID: "a"}},
		},
	}
}

// sortKey is the default sort.column key (`s`).
var sortKey = tea.Key{Code: 's', Text: "s"}

// TestSortInertWithoutResource proves the sort key is a no-op before any resource
// table is open (the welcome page is showing): there is nothing to sort.
func TestSortInertWithoutResource(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, sortKey)
	if _, ok := m.table.SortColumn(); ok {
		t.Fatal("sort should be inert with no resource table open")
	}
}

// TestSortCycleAdvancesColumnsAndClears proves the sort.column key cycles the table
// through every visible column and both directions, then back to the unsorted watch
// order — the single-key sort model (M2-13b): unsorted → col0 asc → col0 desc →
// col1 asc → col1 desc → cleared → col0 asc …
func TestSortCycleAdvancesColumnsAndClears(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw))

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	// Drain the preloaded RESET into the table so it has 2 visible columns + 2 rows.
	wm, ok := cmd().(watchMsg)
	if !ok {
		t.Fatalf("pump produced %T, want watchMsg", cmd())
	}
	next, _ = m.Update(wm)
	m = next.(Model)
	if m.table.VisibleColumnCount() != 2 {
		t.Fatalf("precondition: visible columns = %d, want 2", m.table.VisibleColumnCount())
	}

	wantState := func(step string, wantCol int, wantSorted, wantDesc bool) {
		t.Helper()
		col, sorted := m.table.SortColumn()
		if sorted != wantSorted || (sorted && (col != wantCol || m.table.SortDescending() != wantDesc)) {
			t.Fatalf("%s: sort = (col %d, sorted %v, desc %v), want (col %d, sorted %v, desc %v)",
				step, col, sorted, m.table.SortDescending(), wantCol, wantSorted, wantDesc)
		}
	}

	m, _ = press(t, m, sortKey)
	wantState("1st press", 0, true, false) // col0 ascending
	m, _ = press(t, m, sortKey)
	wantState("2nd press", 0, true, true) // col0 descending
	m, _ = press(t, m, sortKey)
	wantState("3rd press", 1, true, false) // col1 ascending
	m, _ = press(t, m, sortKey)
	wantState("4th press", 1, true, true) // col1 descending
	m, _ = press(t, m, sortKey)
	wantState("5th press", 0, false, false) // past the last column → cleared
	m, _ = press(t, m, sortKey)
	wantState("6th press", 0, true, false) // cycle restarts at col0 ascending
}

// TestClearSortKeyRestoresOrder proves the sort.clear key (`S`) drops an active sort
// back to the unsorted watch order in one press.
func TestClearSortKeyRestoresOrder(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	wm := cmd().(watchMsg)
	next, _ = m.Update(wm)
	m = next.(Model)

	m, _ = press(t, m, sortKey) // sort col0 ascending
	if _, ok := m.table.SortColumn(); !ok {
		t.Fatal("precondition: table should be sorted")
	}
	m, _ = press(t, m, tea.Key{Code: 's', ShiftedCode: 'S', Mod: tea.ModShift}) // sort.clear
	if _, ok := m.table.SortColumn(); ok {
		t.Fatal("sort.clear should restore the unsorted watch order")
	}
}

// kindResource is gvrResource with the GVK.Kind set, so the row-action
// applicability predicates (which key on kind, M3-02) resolve.
func kindResource(resource, kind string) kube.Resource {
	r := gvrResource(resource)
	r.GVK = schema.GroupVersionKind{Kind: kind}
	return r
}

// openPodTable drills into a pods resource (Kind Pod) with a two-row live table so
// an actions test has a concrete selected row to act on.
func openPodTable(t *testing.T, kind string) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", kind)})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// actionsKey is the default actions.menu key (`a`).
var actionsKey = tea.Key{Code: 'a', Text: "a"}

// TestActionsMenuListsApplicableActions proves the actions.menu key opens the
// picker over the selected row and lists exactly the actions applicable to the
// browsed kind: a Pod offers Logs and Exec but not the node-only Cordon/Drain.
func TestActionsMenuListsApplicableActions(t *testing.T) {
	m := openPodTable(t, "Pod")
	m, _ = press(t, m, actionsKey)
	if !m.actPicker.Active() {
		t.Fatal("actions.menu key should open the actions picker over the selected row")
	}
	for _, title := range []string{"View YAML", "Describe", "Logs", "Exec shell", "Delete"} {
		if _, ok := m.actByLabel[title]; !ok {
			t.Errorf("Pod actions menu should list %q", title)
		}
	}
	for _, title := range []string{"Cordon", "Drain", "Suspend"} {
		if _, ok := m.actByLabel[title]; ok {
			t.Errorf("Pod actions menu should not list node/cronjob action %q", title)
		}
	}
}

// TestActionsMenuKindSpecific proves applicability tracks the kind: a Node offers
// Cordon/Drain but not Logs/Exec.
func TestActionsMenuKindSpecific(t *testing.T) {
	m := openPodTable(t, "Node")
	m, _ = press(t, m, actionsKey)
	for _, title := range []string{"Cordon", "Uncordon", "Drain"} {
		if _, ok := m.actByLabel[title]; !ok {
			t.Errorf("Node actions menu should list %q", title)
		}
	}
	for _, title := range []string{"Logs", "Exec shell", "Scale"} {
		if _, ok := m.actByLabel[title]; ok {
			t.Errorf("Node actions menu should not list %q", title)
		}
	}
}

// TestActionsMenuInertWithoutResource proves the actions key is a no-op before a
// resource table is open (the welcome page is showing): there is no row to act on.
func TestActionsMenuInertWithoutResource(t *testing.T) {
	m := sizedWith(t, WithWatcher(&fakeWatcher{}))
	m, _ = press(t, m, actionsKey)
	if m.actPicker.Active() {
		t.Fatal("actions.menu should be inert with no resource table open")
	}
}

// TestActionDirectKeyDispatchesIntent proves a direct-key M3 action (here `y`,
// res.yaml) dispatches a rowActionMsg carrying the action and the selected row's
// object, without opening the menu.
func TestActionDirectKeyDispatchesIntent(t *testing.T) {
	m := openPodTable(t, "Pod")
	_, cmd := press(t, m, tea.Key{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("res.yaml key produced no command")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.yaml produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionYAML {
		t.Errorf("intent action = %q, want %q", intent.Action, rowActionYAML)
	}
	if intent.Object.Name == "" {
		t.Error("intent should carry the selected row's object identity")
	}
}

// TestActionsMenuSelectionDispatchesIntent proves picking an action from the menu
// dispatches the same rowActionMsg intent and closes the menu.
func TestActionsMenuSelectionDispatchesIntent(t *testing.T) {
	m := openPodTable(t, "Pod")
	m, _ = press(t, m, actionsKey)
	next, cmd := m.Update(picker.SelectedMsg{Kind: actionPickerKind, Value: "Describe"})
	m = next.(Model)
	if m.actPicker.Active() {
		t.Fatal("picking an action should close the actions menu")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("action selection produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionDescribe {
		t.Errorf("intent action = %q, want %q", intent.Action, rowActionDescribe)
	}
}

// fakeYAMLGetter is a hermetic YAMLGetter: it returns a preset YAML string (or
// error) and records the object it was asked for so a test can assert the selected
// row was addressed.
type fakeYAMLGetter struct {
	yaml   string
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
}

func (f *fakeYAMLGetter) GetYAML(_ context.Context, r kube.Resource, ref kube.ObjectRef) (string, error) {
	f.calls++
	f.gotRes = r
	f.gotRef = ref
	return f.yaml, f.err
}

// yamlViewerModel drills into a pods table (Kind Pod) with a live row and the given
// YAML getter wired, so a viewer test has a concrete selected row and a fetch seam.
func yamlViewerModel(t *testing.T, getter YAMLGetter) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithYAMLGetter(getter))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// yamlKey is the default res.yaml direct key (`y`).
var yamlKey = tea.Key{Code: 'y', Text: "y"}

// TestYAMLViewerOpensAndShowsContent drives the whole M3-03 path: the `y` key
// dispatches the YAML intent, handling it opens the viewer and issues the GetYAML
// fetch against the selected row, and the fetched YAML lands in the viewer's content.
func TestYAMLViewerOpensAndShowsContent(t *testing.T) {
	g := &fakeYAMLGetter{yaml: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web-1\n"}
	m := yamlViewerModel(t, g)

	// `y` dispatches the intent; feed it back in to trigger the viewer + fetch.
	_, cmd := press(t, m, yamlKey)
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.yaml produced %T, want rowActionMsg", cmd())
	}
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("handling the YAML intent should open the viewer")
	}
	if fetchCmd == nil {
		t.Fatal("opening the YAML viewer should issue a GetYAML fetch command")
	}
	loaded, ok := fetchCmd().(yamlLoadedMsg)
	if !ok {
		t.Fatalf("fetch produced %T, want yamlLoadedMsg", fetchCmd())
	}
	if g.calls != 1 {
		t.Fatalf("GetYAML called %d times, want 1", g.calls)
	}
	if g.gotRef.Name == "" {
		t.Error("GetYAML should be addressed to the selected row's object")
	}

	next, _ = m.Update(loaded)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should stay open once its content lands")
	}
	// The fetched YAML is composited over the browse view (overlayCenter) — both the
	// YAML body and the base menu ("Cluster" section header) are visible at once.
	view := m.View().Content
	if !strings.Contains(view, "kind: Pod") {
		t.Fatalf("viewer should show the fetched YAML: %q", view)
	}
}

// TestYAMLViewerCloses proves nav.back (esc) dismisses the viewer (its ClosedMsg,
// delivered back through Update, hides it) and returns to the browse view.
func TestYAMLViewerCloses(t *testing.T) {
	g := &fakeYAMLGetter{yaml: "kind: Pod\n"}
	m := yamlViewerModel(t, g)
	_, cmd := press(t, m, yamlKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(yamlLoadedMsg))
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("precondition: the viewer should be open")
	}

	m, closeCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if closeCmd == nil {
		t.Fatal("nav.back in the viewer should emit a ClosedMsg command")
	}
	next, _ = m.Update(closeCmd())
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("delivering the viewer's ClosedMsg should hide it")
	}
}

// TestYAMLViewerFetchErrorDegrades proves a GetYAML failure closes the viewer and
// surfaces a transient status-bar toast rather than leaving an empty box (D74).
func TestYAMLViewerFetchErrorDegrades(t *testing.T) {
	g := &fakeYAMLGetter{err: errors.New("not found")}
	m := yamlViewerModel(t, g)
	_, cmd := press(t, m, yamlKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer opens immediately, before the fetch resolves")
	}
	loaded := fetchCmd().(yamlLoadedMsg)
	if loaded.err == nil {
		t.Fatal("the fetch should carry the getter's error")
	}
	next, _ = m.Update(loaded)
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("a fetch error should close the viewer")
	}
	if !m.status.HasError() {
		t.Fatal("a fetch error should surface a status-bar toast")
	}
}

// TestYAMLViewerInertWithoutGetter proves the YAML intent is a no-op with no getter
// wired (the viewer never opens) — the pre-wiring app and hermetic tests stay inert.
func TestYAMLViewerInertWithoutGetter(t *testing.T) {
	m := openPodTable(t, "Pod") // no WithYAMLGetter
	_, cmd := press(t, m, yamlKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("the YAML viewer should not open without a getter wired")
	}
	if fetchCmd != nil {
		t.Fatal("no getter → no fetch command")
	}
}

// TestYAMLViewerStaleFetchDropped proves the generation guard: a fetch that lands
// after a newer viewer open (or after the viewer closed) is dropped rather than
// overwriting the current content.
func TestYAMLViewerStaleFetchDropped(t *testing.T) {
	g := &fakeYAMLGetter{yaml: "kind: Pod\n"}
	m := yamlViewerModel(t, g)
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	intent := rowActionMsg{Action: rowActionYAML, Resource: m.current, Object: row.Object}

	// First open → gen 1 fetch (dispatched directly; a key press would route to the
	// open viewer, which is the point — a re-open comes from the intent, not the key).
	next, firstFetch := m.Update(intent)
	m = next.(Model)
	stale := firstFetch().(yamlLoadedMsg)

	// Second open → gen bumps; the first fetch is now stale.
	next, _ = m.Update(intent)
	m = next.(Model)
	if stale.gen == m.viewerGen {
		t.Fatalf("precondition: stale fetch gen %d should differ from current %d", stale.gen, m.viewerGen)
	}

	next, _ = m.Update(stale) // deliver the stale (gen-1) result
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should still be open")
	}
	// The stale result must not have populated content: content only lands from the
	// current-gen fetch, so the viewer body stays empty until that arrives.
	if strings.Contains(m.View().Content, "kind: Pod") {
		t.Fatal("a stale-generation fetch should be dropped, not shown")
	}
}
