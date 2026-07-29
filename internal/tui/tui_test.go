package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/modal"
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
	opts    []metav1.ListOptions // the scope each watch was started under (M4-08)
	preload []kube.WatchEvent    // if set, buffered into every returned channel at Watch time
}

func (f *fakeWatcher) Watch(ctx context.Context, r kube.Resource, ns string, opts metav1.ListOptions) (<-chan kube.WatchEvent, error) {
	f.ctxs = append(f.ctxs, ctx)
	f.res = append(f.res, r)
	f.ns = append(f.ns, ns)
	f.opts = append(f.opts, opts)
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

// TestMenuPagingKeysReachTheMenu proves the half-page/page keys are live in the
// left pane (M2-15): with the menu focused, ctrl+d moves its cursor by more than
// the one row nav.down would, and ctrl+u brings it back. routeNav forwards every
// non-focus-switching action to the focused pane, so this guards against a future
// special-case there quietly making the menu inert again — which is exactly the
// state M2-EXIT found.
func TestMenuPagingKeysReachTheMenu(t *testing.T) {
	m := sized(t)
	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}

	m, _ = press(t, m, tea.Key{Code: 'd', Mod: tea.ModCtrl})
	paged := m.menu.Cursor()
	if paged <= 1 {
		t.Fatalf("ctrl+d moved the menu cursor to %d, want a multi-row jump", paged)
	}

	m, _ = press(t, m, tea.Key{Code: 'u', Mod: tea.ModCtrl})
	if got := m.menu.Cursor(); got != 0 {
		t.Fatalf("ctrl+u after ctrl+d: menu cursor = %d, want 0", got)
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
func openPodTable(t *testing.T, kind string, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
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
	for _, title := range []string{"View / Edit YAML", "Describe", "Logs", "Exec shell", "Delete"} {
		if _, ok := m.actByLabel[title]; !ok {
			t.Errorf("Pod actions menu should list %q", title)
		}
	}
	// The retired standalone "View YAML" folded into "View / Edit YAML" (D135/M3-15c).
	if _, ok := m.actByLabel["View YAML"]; ok {
		t.Error("the standalone \"View YAML\" entry should be gone after the unify (M3-15c)")
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

// TestActionsMenuCronJob proves a CronJob offers Suspend/Resume but not the
// node-only Cordon/Drain (M3-12).
func TestActionsMenuCronJob(t *testing.T) {
	m := openPodTable(t, "CronJob")
	m, _ = press(t, m, actionsKey)
	for _, title := range []string{"Suspend", "Resume"} {
		if _, ok := m.actByLabel[title]; !ok {
			t.Errorf("CronJob actions menu should list %q", title)
		}
	}
	for _, title := range []string{"Cordon", "Drain", "Logs"} {
		if _, ok := m.actByLabel[title]; ok {
			t.Errorf("CronJob actions menu should not list %q", title)
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

// TestActionDirectKeyDispatchesIntent proves a direct-key M3 action (here `e`,
// res.edit — the unified View/Edit YAML action) dispatches a rowActionMsg carrying
// the action and the selected row's object, without opening the menu.
func TestActionDirectKeyDispatchesIntent(t *testing.T) {
	m := openPodTable(t, "Pod")
	_, cmd := press(t, m, tea.Key{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("res.edit key produced no command")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.edit produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionEdit {
		t.Errorf("intent action = %q, want %q", intent.Action, rowActionEdit)
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

// The standalone read-only YAML viewer (M3-03) was retired into the unified View/Edit
// YAML action (M3-15c/D135): view and edit an object's YAML are one act, so the edit
// flow (openEdit → $EDITOR → apply) is the only YAML surface. Its coverage lives in
// edit_test.go; fakeYAMLGetter above is now the edit flow's buffer-source fake. The
// former YAMLViewer* tests were removed with the feature.

// fakeDescriber is a hermetic Describer: it returns a preset describe string (or
// error) and records the object it was asked for so a test can assert the selected
// row was addressed. Like kube.Describe it takes no context.
type fakeDescriber struct {
	text   string
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
}

func (f *fakeDescriber) Describe(r kube.Resource, ref kube.ObjectRef) (string, error) {
	f.calls++
	f.gotRes = r
	f.gotRef = ref
	return f.text, f.err
}

// describeViewerModel drills into a pods table (Kind Pod) with a live row and the
// given describer wired, so a viewer test has a concrete selected row and a render
// seam.
func describeViewerModel(t *testing.T, describer Describer) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithDescriber(describer))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// describeKey is the default res.describe direct key (`D`).
var describeKey = tea.Key{Code: 'D', Text: "D"}

// TestDescribeViewerOpensAndShowsContent drives the whole M3-04 path: the `d` key
// dispatches the describe intent, handling it opens the viewer and issues the Describe
// render against the selected row, and the rendered output lands in the viewer content.
func TestDescribeViewerOpensAndShowsContent(t *testing.T) {
	d := &fakeDescriber{text: "Name:         web-1\nNamespace:    default\nStatus:       Running\n"}
	m := describeViewerModel(t, d)

	// `d` dispatches the intent; feed it back in to trigger the viewer + render.
	_, cmd := press(t, m, describeKey)
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.describe produced %T, want rowActionMsg", cmd())
	}
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("handling the describe intent should open the viewer")
	}
	if fetchCmd == nil {
		t.Fatal("opening the describe viewer should issue a Describe render command")
	}
	loaded, ok := fetchCmd().(describeLoadedMsg)
	if !ok {
		t.Fatalf("render produced %T, want describeLoadedMsg", fetchCmd())
	}
	if d.calls != 1 {
		t.Fatalf("Describe called %d times, want 1", d.calls)
	}
	if d.gotRef.Name == "" {
		t.Error("Describe should be addressed to the selected row's object")
	}

	next, _ = m.Update(loaded)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should stay open once its content lands")
	}
	// The rendered describe output is composited over the browse view (overlayCenter).
	view := m.View().Content
	if !strings.Contains(view, "Status:") {
		t.Fatalf("viewer should show the rendered describe output: %q", view)
	}
}

// TestDescribeViewerCloses proves nav.back (esc) dismisses the viewer (its ClosedMsg,
// delivered back through Update, hides it) and returns to the browse view.
func TestDescribeViewerCloses(t *testing.T) {
	d := &fakeDescriber{text: "Name: web-1\n"}
	m := describeViewerModel(t, d)
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(describeLoadedMsg))
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

// TestDescribeViewerRenderErrorDegrades proves a Describe failure closes the viewer
// and surfaces a transient status-bar toast rather than leaving an empty box (D74).
func TestDescribeViewerRenderErrorDegrades(t *testing.T) {
	d := &fakeDescriber{err: errors.New("not found")}
	m := describeViewerModel(t, d)
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer opens immediately, before the render resolves")
	}
	loaded := fetchCmd().(describeLoadedMsg)
	if loaded.err == nil {
		t.Fatal("the render should carry the describer's error")
	}
	next, _ = m.Update(loaded)
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("a render error should close the viewer")
	}
	if !m.status.HasError() {
		t.Fatal("a render error should surface a status-bar toast")
	}
}

// TestDescribeViewerInertWithoutDescriber proves the describe intent is a no-op with
// no describer wired (the viewer never opens) — the pre-wiring app and hermetic tests
// stay inert.
func TestDescribeViewerInertWithoutDescriber(t *testing.T) {
	m := openPodTable(t, "Pod") // no WithDescriber
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("the describe viewer should not open without a describer wired")
	}
	if fetchCmd != nil {
		t.Fatal("no describer → no render command")
	}
}

// TestDescribeViewerStaleRenderDropped proves the generation guard: a render that
// lands after a newer viewer open (or after the viewer closed) is dropped rather than
// overwriting the current content.
func TestDescribeViewerStaleRenderDropped(t *testing.T) {
	d := &fakeDescriber{text: "Name: web-1\n"}
	m := describeViewerModel(t, d)
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	intent := rowActionMsg{Action: rowActionDescribe, Resource: m.current, Object: row.Object}

	// First open → gen-N render (dispatched directly; a key press would route to the
	// open viewer, which is the point — a re-open comes from the intent, not the key).
	next, firstFetch := m.Update(intent)
	m = next.(Model)
	stale := firstFetch().(describeLoadedMsg)

	// Second open → gen bumps; the first render is now stale.
	next, _ = m.Update(intent)
	m = next.(Model)
	if stale.gen == m.viewerGen {
		t.Fatalf("precondition: stale render gen %d should differ from current %d", stale.gen, m.viewerGen)
	}

	next, _ = m.Update(stale) // deliver the stale result
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should still be open")
	}
	if strings.Contains(m.View().Content, "Name: web-1") {
		t.Fatal("a stale-generation render should be dropped, not shown")
	}
}

// fakeLogStreamer is a hermetic LogStreamer: it hands back a channel preloaded with a
// preset sequence of log events (then closed, the EOF of a non-following stream), or
// fails the open with err. It records the object it was asked for so a test can assert
// the selected row was addressed.
type fakeLogStreamer struct {
	events  []kube.LogEvent // delivered in order, then the channel closes
	err     error           // an open failure (Logs returns it, no channel)
	calls   int
	gotRef  kube.ObjectRef
	gotOpts kube.LogOptions // records the options the last open was asked for
}

func (f *fakeLogStreamer) Logs(_ context.Context, ref kube.ObjectRef, opts kube.LogOptions) (<-chan kube.LogEvent, error) {
	f.calls++
	f.gotRef = ref
	f.gotOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan kube.LogEvent, len(f.events))
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// logsViewerModel drills into a pods table (Kind Pod) with a live row and the given
// streamer wired, so a viewer test has a concrete selected row and a stream seam.
// Mirrors describeViewerModel.
func logsViewerModel(t *testing.T, streamer LogStreamer) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithLogStreamer(streamer))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// logsKey is the default res.logs direct key (`L`).
var logsKey = tea.Key{Code: 'L', Text: "L"}

// drainLogPump runs the log-pump chain to completion: it delivers each logMsg the
// pump produces back through Update, appending a line and re-issuing the pump, until
// a non-line pump message (a closed channel or a bridged error) ends the chain,
// returning the final model. Only a LogLineMsg continues the chain — a closed/error
// message is delivered (so its effect, e.g. the error toast, is applied) and then the
// loop stops rather than following the resulting cmd (which is not a pump: a nil for a
// clean close, the toast's clear-timer for an error). It is the test-side equivalent
// of Bubble Tea servicing the pump.
func drainLogPump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 100 {
			t.Fatal("log pump did not terminate")
		}
		lm, ok := cmd().(logMsg)
		if !ok {
			break // not a pump message — stop draining.
		}
		next, nextCmd := m.Update(lm)
		m = next.(Model)
		if _, isLine := lm.msg.(LogLineMsg); !isLine {
			break // closed/error message delivered; the chain ends here.
		}
		cmd = nextCmd
	}
	return m
}

// TestLogsViewOpensAndStreamsContent drives the whole M3-05 path, now landing in the
// dedicated logs view (LOGS-02): the `L` key dispatches the logs intent, handling it
// opens the logs view and starts the Logs stream against the selected row, and each
// streamed line is appended into it. The shared viewer stays shut — logs left it (D144).
func TestLogsViewOpensAndStreamsContent(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "line one"}, {Line: "line two"}}}
	m := logsViewerModel(t, s)

	// `L` dispatches the intent; feed it back in to open the viewer + start the stream.
	_, cmd := press(t, m, logsKey)
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.logs produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionLogs {
		t.Fatalf("intent action = %q, want %q", intent.Action, rowActionLogs)
	}
	next, pumpCmd := m.Update(intent)
	m = next.(Model)
	if !m.logsView.Active() {
		t.Fatal("handling the logs intent should open the logs view")
	}
	if m.viewer.Active() {
		t.Fatal("logs must not open the shared viewer any more (D144)")
	}
	if pumpCmd == nil {
		t.Fatal("opening the logs viewer should issue a log-pump command")
	}
	if s.calls != 1 {
		t.Fatalf("Logs called %d times, want 1", s.calls)
	}
	if s.gotRef.Name == "" {
		t.Error("Logs should be addressed to the selected row's object")
	}

	m = drainLogPump(t, m, pumpCmd)
	if !m.logsView.Active() {
		t.Fatal("the logs view should stay open across the stream")
	}
	view := m.View().Content
	if !strings.Contains(view, "line one") || !strings.Contains(view, "line two") {
		t.Fatalf("the logs view should show the streamed log lines: %q", view)
	}
	// Full-screen, not an overlay (D134): the browse panes are replaced, so the menu's
	// own rows are gone from the frame while the logs view is up.
	if strings.Contains(view, "Workloads") {
		t.Fatalf("the logs view should replace the browse body, not overlay it: %q", view)
	}
}

// TestLogsViewCloses proves nav.back (esc) dismisses the logs view and tears the log
// stream down (its ClosedMsg, delivered back through Update, hides it).
func TestLogsViewCloses(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "hello"}}}
	m := logsViewerModel(t, s)
	_, cmd := press(t, m, logsKey)
	next, pumpCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	m = drainLogPump(t, m, pumpCmd)
	if !m.logsView.Active() {
		t.Fatal("precondition: the logs view should be open")
	}

	m, closeCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if closeCmd == nil {
		t.Fatal("nav.back in the logs view should emit a ClosedMsg command")
	}
	next, _ = m.Update(closeCmd())
	m = next.(Model)
	if m.logsView.Active() {
		t.Fatal("delivering the logs view's ClosedMsg should hide it")
	}
	if m.logCh != nil || m.logCancel != nil {
		t.Fatal("closing the logs view should tear down the log stream")
	}
}

// TestLogsViewOpenErrorDegrades proves a Logs open failure leaves the logs view closed
// and surfaces a transient status-bar toast rather than an empty full-screen frame (D74).
func TestLogsViewOpenErrorDegrades(t *testing.T) {
	s := &fakeLogStreamer{err: errors.New("forbidden")}
	m := logsViewerModel(t, s)
	_, cmd := press(t, m, logsKey)
	next, _ := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if m.logsView.Active() {
		t.Fatal("an open failure should not leave the logs view showing an empty frame")
	}
	if !m.status.HasError() {
		t.Fatal("an open failure should surface a status-bar toast")
	}
}

// TestLogsViewStreamErrorKeepsShownLines proves a mid-stream error after some lines
// already showed leaves those lines on screen (the view stays open) while surfacing a
// toast — a streaming view degrades without discarding partial output.
func TestLogsViewStreamErrorKeepsShownLines(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "before the drop"}, {Err: errors.New("connection reset")}}}
	m := logsViewerModel(t, s)
	_, cmd := press(t, m, logsKey)
	next, pumpCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	m = drainLogPump(t, m, pumpCmd)
	if !m.logsView.Active() {
		t.Fatal("a mid-stream error after output should leave the logs view open")
	}
	if !strings.Contains(m.View().Content, "before the drop") {
		t.Fatal("the lines shown before the error should remain on screen")
	}
	if !m.status.HasError() {
		t.Fatal("a stream error should surface a status-bar toast")
	}
}

// TestLogsViewInertWithoutStreamer proves the logs intent is a no-op with no streamer
// wired (the view never opens) — the pre-wiring app and hermetic tests stay inert.
func TestLogsViewInertWithoutStreamer(t *testing.T) {
	m := openPodTable(t, "Pod") // no WithLogStreamer
	_, cmd := press(t, m, logsKey)
	next, pumpCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if m.logsView.Active() {
		t.Fatal("the logs view should not open without a streamer wired")
	}
	if pumpCmd != nil {
		t.Fatal("no streamer → no log-pump command")
	}
}

// TestLogsViewerNonPodDegrades proves logs on a pod-owning kind (here a Deployment)
// degrades to a toast when no pod resolver is wired (M3-07b resolves a backing pod
// only when WithPodResolver is set; without it the app and the non-resolver hermetic
// tests stay inert) rather than opening an empty viewer.
func TestLogsViewNonPodDegrades(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "x"}}}
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithLogStreamer(s))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("deployments", "Deployment")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg))
	m = next.(Model)

	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	next, _ = m.Update(rowActionMsg{Action: rowActionLogs, Resource: m.current, Object: row.Object})
	m = next.(Model)
	if m.logsView.Active() {
		t.Fatal("logs on a non-pod kind should not open the logs view yet (M3-07)")
	}
	if s.calls != 0 {
		t.Fatal("logs on a non-pod kind should not call the streamer")
	}
	if !m.status.HasError() {
		t.Fatal("logs on a non-pod kind should surface a not-yet-available toast")
	}
}

// TestLogsViewStaleLineDropped proves the generation guard: a line from a stream whose
// open was superseded by a newer one is dropped rather than appended to the current
// content.
func TestLogsViewStaleLineDropped(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "stale line"}}}
	m := logsViewerModel(t, s)
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	intent := rowActionMsg{Action: rowActionLogs, Resource: m.current, Object: row.Object}

	// First open → gen-N stream; grab its first pumped line without delivering it.
	next, firstPump := m.Update(intent)
	m = next.(Model)
	stale, ok := firstPump().(logMsg)
	if !ok {
		t.Fatalf("first pump produced %T, want logMsg", firstPump())
	}

	// Second open → viewerGen bumps; the first stream's line is now stale.
	next, _ = m.Update(intent)
	m = next.(Model)
	if stale.gen == m.viewerGen {
		t.Fatalf("precondition: stale line gen %d should differ from current %d", stale.gen, m.viewerGen)
	}

	next, _ = m.Update(stale) // deliver the stale line
	m = next.(Model)
	if strings.Contains(m.View().Content, "stale line") {
		t.Fatal("a stale-generation log line should be dropped, not shown")
	}
}

// followKey is the default logs.follow toggle key (`f`).
var followKey = tea.Key{Code: 'f', Text: "f"}

// openLogsViewHelper opens the logs view over the selected pod row and drains the
// initial stream, returning the model with the view up.
func openLogsViewHelper(t *testing.T, m Model) Model {
	t.Helper()
	_, cmd := press(t, m, logsKey)
	next, pumpCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	return drainLogPump(t, m, pumpCmd)
}

// TestLogsViewOpensFollowing proves M3-06's default survives the rehome: the logs view
// opens in follow mode and asks the streamer for a following stream
// (LogOptions{Follow:true}), so the stream stays open and reconnects (M1-07d) rather
// than ending at EOF. Follow state is the component's now, not the model's (D144).
func TestLogsViewOpensFollowing(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "line one"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)
	if !m.logsView.Following() {
		t.Fatal("the logs view should open in follow mode")
	}
	if !s.gotOpts.Follow {
		t.Fatal("the following logs view should open the stream with Follow:true")
	}
	if !strings.Contains(m.View().Content, "[following]") {
		t.Fatalf("the logs header should mark it as following: %q", m.View().Content)
	}
}

// TestLogsFollowToggle proves the `f` key still toggles follow off and back on inside
// the logs view, and the header marker tracks the state — routed through the view's
// action path now that the shared viewer's logs special-casing is gone.
func TestLogsFollowToggle(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "hello"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)

	m, _ = press(t, m, followKey) // pause
	if m.logsView.Following() {
		t.Fatal("pressing follow while following should pause it")
	}
	if !strings.Contains(m.View().Content, "[paused]") {
		t.Fatalf("a paused logs view should mark [paused]: %q", m.View().Content)
	}

	m, _ = press(t, m, followKey) // resume
	if !m.logsView.Following() {
		t.Fatal("pressing follow while paused should resume it")
	}
	if !strings.Contains(m.View().Content, "[following]") {
		t.Fatalf("a resumed logs view should mark [following]: %q", m.View().Content)
	}
}

// TestLogsFollowPausesOnManualUpScroll proves a manual up-scroll (`k`) while following
// pauses follow so the reader can inspect earlier output without being yanked back to
// the tail; a down-scroll (`j`) leaves follow untouched.
func TestLogsFollowPausesOnManualUpScroll(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "hello"}}}
	m := logsViewerModel(t, s)
	m = openLogsViewHelper(t, m)

	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"}) // down: still following
	if !m.logsView.Following() {
		t.Fatal("a down-scroll should not pause follow")
	}
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"}) // up: pauses
	if m.logsView.Following() {
		t.Fatal("a manual up-scroll while following should pause follow")
	}
}

// TestLogsFollowInertOnDescribeViewer proves logs.follow is inert on the shared viewer:
// pressing `f` while the describe viewer is up does nothing (there is nothing to follow —
// the shared viewer only shows one-shot content now, D144) and leaves the viewer open.
func TestLogsFollowInertOnDescribeViewer(t *testing.T) {
	d := &fakeDescriber{text: "Name: web-1\n"}
	m := describeViewerModel(t, d)
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(describeLoadedMsg))
	m = next.(Model)
	if m.viewer.Kind() != viewerKindDescribe {
		t.Fatalf("precondition: the describe viewer should be up, kind = %q", m.viewer.Kind())
	}
	m, followCmd := press(t, m, followKey)
	if followCmd != nil {
		t.Fatal("logs.follow on the describe viewer should be inert (no command)")
	}
	if !m.viewer.Active() {
		t.Fatal("logs.follow should not close the describe viewer")
	}
}

// fakeContainerLister is a hermetic ContainerLister: it returns a preset set of
// containers (or an error) and records the object it was asked for, so a test can
// drive the M3-07a container-resolution flow without a cluster. names is the common
// case — regular containers, named — while containers spells out the kinds when the
// test is about init/ephemeral ones (LOGS-06); set one or the other.
type fakeContainerLister struct {
	names      []string
	containers []kube.Container
	err        error
	calls      int
	gotRef     kube.ObjectRef
}

func (f *fakeContainerLister) PodContainers(_ context.Context, ref kube.ObjectRef) ([]kube.Container, error) {
	f.calls++
	f.gotRef = ref
	if f.err != nil {
		return nil, f.err
	}
	if f.containers != nil {
		return f.containers, nil
	}
	cs := make([]kube.Container, 0, len(f.names))
	for _, n := range f.names {
		cs = append(cs, kube.Container{Name: n, Kind: kube.ContainerRegular})
	}
	return cs, nil
}

// logsModelWithContainers drills into a pods table with both a log streamer and a
// container lister wired, so a test exercises the M3-07a resolve-then-stream flow.
func logsModelWithContainers(t *testing.T, streamer LogStreamer, lister ContainerLister) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithLogStreamer(streamer), WithContainerLister(lister))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// openLogsFetch presses `L` and delivers the resulting logs intent, returning the
// model and the async container-fetch cmd openLogsViewer issues when a lister is wired.
func openLogsFetch(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	_, cmd := press(t, m, logsKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	return next.(Model), fetchCmd
}

// resolveContainers delivers the container-fetch cmd and returns the resulting model +
// follow-on cmd (the log pump for a single container; nil once the picker is shown).
func resolveContainers(t *testing.T, m Model, fetchCmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	msg := fetchCmd()
	clm, ok := msg.(containersLoadedMsg)
	if !ok {
		t.Fatalf("opening logs with a lister produced %T, want containersLoadedMsg", msg)
	}
	next, follow := m.Update(clm)
	return next.(Model), follow
}

// TestLogsSingleContainerStreamsDirectly proves a single-container pod skips the
// picker: resolving its one container streams it directly into the viewer, addressing
// the stream to that container.
func TestLogsSingleContainerStreamsDirectly(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "hello"}}}
	l := &fakeContainerLister{names: []string{"app"}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	if m.viewer.Active() {
		t.Fatal("the viewer should not open until the container resolves")
	}
	m, pumpCmd := resolveContainers(t, m, fetchCmd)
	if l.calls != 1 {
		t.Fatalf("PodContainers called %d times, want 1", l.calls)
	}
	if m.ctrPicker.Active() {
		t.Fatal("a single-container pod should not open the container picker")
	}
	if !m.logsView.Active() {
		t.Fatal("a single-container pod should stream directly into the logs view")
	}
	if s.gotOpts.Container != "app" {
		t.Fatalf("stream container = %q, want app", s.gotOpts.Container)
	}
	m = drainLogPump(t, m, pumpCmd)
	if !strings.Contains(m.View().Content, "hello") {
		t.Fatalf("the streamed line should show: %q", m.View().Content)
	}
}

// TestLogsMultiContainerOpensPicker proves a multi-container pod prompts which
// container to stream (the picker opens listing them) rather than streaming blindly.
func TestLogsMultiContainerOpensPicker(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "x"}}}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, follow := resolveContainers(t, m, fetchCmd)
	if !m.ctrPicker.Active() {
		t.Fatal("a multi-container pod should open the container picker")
	}
	if m.viewer.Active() {
		t.Fatal("the viewer should not open until a container is picked")
	}
	if s.calls != 0 {
		t.Fatal("no stream should start before a container is picked")
	}
	if follow != nil {
		t.Fatal("opening the picker issues no follow-on command")
	}
	if m.ctrPicker.Len() != 2 {
		t.Fatalf("the picker should list 2 containers, got %d", m.ctrPicker.Len())
	}
}

// TestLogsContainerPickStreamsChosen proves picking a container from the picker opens
// the logs viewer streaming exactly that container, its name shown in the title.
func TestLogsContainerPickStreamsChosen(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "sidecar log"}}}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)

	next, pumpCmd := m.Update(picker.SelectedMsg{Kind: containerPickerKind, Value: "sidecar"})
	m = next.(Model)
	if m.ctrPicker.Active() {
		t.Fatal("picking a container should close the picker")
	}
	if !m.logsView.Active() {
		t.Fatal("picking a container should open the logs view")
	}
	if s.gotOpts.Container != "sidecar" {
		t.Fatalf("stream container = %q, want sidecar", s.gotOpts.Container)
	}
	if !strings.Contains(m.View().Content, "sidecar") {
		t.Fatalf("the logs header should name the chosen container: %q", m.View().Content)
	}
	m = drainLogPump(t, m, pumpCmd)
	if !strings.Contains(m.View().Content, "sidecar log") {
		t.Fatalf("the chosen container's log should show: %q", m.View().Content)
	}
}

// TestLogsContainerPickerCancel proves dismissing the container picker (nav.back) hides
// it without starting a stream or opening the viewer.
func TestLogsContainerPickerCancel(t *testing.T) {
	s := &fakeLogStreamer{}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)

	next, _ := m.Update(picker.CancelledMsg{Kind: containerPickerKind})
	m = next.(Model)
	if m.ctrPicker.Active() {
		t.Fatal("cancelling should hide the container picker")
	}
	if m.viewer.Active() {
		t.Fatal("cancelling should not open the viewer")
	}
	if s.calls != 0 {
		t.Fatal("cancelling should not start a stream")
	}
}

// TestLogsInitContainerOfferedAndStreamable is the LOGS-06 contract on the logs path:
// a pod whose only regular container is joined by an init container prompts (there is
// now more than one container with logs), the init row is *marked* as one, and picking
// it streams that container — the pod-stuck-in-init case the feedback named, which the
// regular-containers-only set made unreachable.
func TestLogsInitContainerOfferedAndStreamable(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "waiting for db"}}}
	l := &fakeContainerLister{containers: []kube.Container{
		{Name: "app", Kind: kube.ContainerRegular},
		{Name: "wait-for-db", Kind: kube.ContainerInit},
	}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)
	if !m.ctrPicker.Active() {
		t.Fatal("a pod with an init container should offer the choice, not stream the regular one blindly")
	}
	if m.ctrPicker.Len() != 2 {
		t.Fatalf("the picker should list both containers, got %d", m.ctrPicker.Len())
	}
	const initRow = "wait-for-db (init)"
	if got, ok := m.ctrByLabel[initRow]; !ok || got != "wait-for-db" {
		t.Fatalf("the init container's row %q should map to wait-for-db, got %q (rows: %v)", initRow, got, m.ctrByLabel)
	}
	if _, ok := m.ctrByLabel["app"]; !ok {
		t.Fatalf("a regular container's row should be its bare name (rows: %v)", m.ctrByLabel)
	}

	next, pumpCmd := m.Update(picker.SelectedMsg{Kind: containerPickerKind, Value: initRow})
	m = next.(Model)
	if !m.logsView.Active() {
		t.Fatal("picking the init container should open the logs view")
	}
	if s.gotOpts.Container != "wait-for-db" {
		t.Fatalf("stream container = %q, want wait-for-db (the name, not the row label)", s.gotOpts.Container)
	}
	m = drainLogPump(t, m, pumpCmd)
	if !strings.Contains(m.View().Content, "waiting for db") {
		t.Fatalf("the init container's log should show: %q", m.View().Content)
	}
}

// TestLogsSoleInitContainerStreamsDirectly proves the single-container fast path counts
// the whole offered set, not just the regular containers: a pod whose one loggable
// container is an init container streams it without a pointless one-row picker.
func TestLogsSoleInitContainerStreamsDirectly(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "migrating"}}}
	l := &fakeContainerLister{containers: []kube.Container{
		{Name: "migrate", Kind: kube.ContainerInit},
	}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)
	if m.ctrPicker.Active() {
		t.Fatal("one loggable container should not open the picker")
	}
	if s.gotOpts.Container != "migrate" {
		t.Fatalf("stream container = %q, want migrate", s.gotOpts.Container)
	}
}

// TestLogsContainerPickerCancelClearsRows proves dismissing the picker drops the row→name
// mapping with it, so a later pick can never resolve against a stale container set.
func TestLogsContainerPickerCancelClearsRows(t *testing.T) {
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWithContainers(t, &fakeLogStreamer{}, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)
	next, _ := m.Update(picker.CancelledMsg{Kind: containerPickerKind})
	if m = next.(Model); m.ctrByLabel != nil {
		t.Fatalf("cancelling should clear the picker's rows: %v", m.ctrByLabel)
	}
}

// TestLogsContainerResolveErrorDegrades proves a PodContainers failure degrades to a
// toast (D74) without opening the viewer or the picker or starting a stream.
func TestLogsContainerResolveErrorDegrades(t *testing.T) {
	s := &fakeLogStreamer{}
	l := &fakeContainerLister{err: errors.New("forbidden")}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)
	if m.viewer.Active() || m.ctrPicker.Active() {
		t.Fatal("a resolve error should open neither the viewer nor the picker")
	}
	if s.calls != 0 {
		t.Fatal("a resolve error should not start a stream")
	}
	if !m.status.HasError() {
		t.Fatal("a resolve error should surface a status-bar toast")
	}
}

// TestLogsNoContainersDegrades proves a pod that reports no containers degrades to a
// toast rather than opening an empty viewer or a picker with nothing to choose.
func TestLogsNoContainersDegrades(t *testing.T) {
	s := &fakeLogStreamer{}
	l := &fakeContainerLister{names: nil}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	m, _ = resolveContainers(t, m, fetchCmd)
	if m.viewer.Active() || m.ctrPicker.Active() {
		t.Fatal("no containers should open neither the viewer nor the picker")
	}
	if s.calls != 0 {
		t.Fatal("no containers should not start a stream")
	}
	if !m.status.HasError() {
		t.Fatal("no containers should surface a status-bar toast")
	}
}

// TestLogsContainerFetchStaleDropped proves the generation guard: a container-resolution
// result that lands after a newer viewer open (viewerGen bumped) is dropped, opening
// nothing.
func TestLogsContainerFetchStaleDropped(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "x"}}}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWithContainers(t, s, l)

	m, fetchCmd := openLogsFetch(t, m)
	stale, ok := fetchCmd().(containersLoadedMsg)
	if !ok {
		t.Fatalf("fetch produced %T, want containersLoadedMsg", fetchCmd())
	}
	m.viewerGen++ // a newer viewer open supersedes the pending fetch.
	next, _ := m.Update(stale)
	m = next.(Model)
	if m.ctrPicker.Active() || m.viewer.Active() {
		t.Fatal("a stale-generation container fetch should be dropped, opening nothing")
	}
}

// fakePodResolver is a hermetic PodResolver: it returns a preset backing-pod ref (or
// an error) and records the workload it was asked to resolve, so a test can drive the
// M3-07b pod-owning-kind logs flow without a cluster.
type fakePodResolver struct {
	pod    kube.ObjectRef
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
}

func (f *fakePodResolver) PodForOwner(_ context.Context, res kube.Resource, ref kube.ObjectRef) (kube.ObjectRef, error) {
	f.calls++
	f.gotRes = res
	f.gotRef = ref
	if f.err != nil {
		return kube.ObjectRef{}, f.err
	}
	return f.pod, nil
}

// logsModelWorkload drills into a workload table (a pod-owning kind) with the given
// seams wired, so a test exercises the M3-07b resolve-a-backing-pod flow.
func logsModelWorkload(t *testing.T, resourceName, kind string, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	all := append([]Option{WithWatcher(fw)}, opts...)
	m := sizedWith(t, all...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource(resourceName, kind)})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// openWorkloadLogsResolve presses `L` on a workload row and delivers the logs intent,
// returning the model and the async pod-resolution cmd openLogsViewer issues for a
// pod-owning kind with a resolver wired.
func openWorkloadLogsResolve(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	_, cmd := press(t, m, logsKey)
	next, resolveCmd := m.Update(cmd().(rowActionMsg))
	return next.(Model), resolveCmd
}

// TestLogsPodOwningKindResolvesPod proves M3-07b's happy path: logs on a Deployment
// resolves a backing pod (the resolver asked for the deployment row), then that pod
// takes the pod path — a single container streams directly, addressed to the resolved
// pod and titled as a Pod so the user sees which pod is tailing.
func TestLogsPodOwningKindResolvesPod(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "workload log"}}}
	r := &fakePodResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz", UID: "uid-1"}}
	l := &fakeContainerLister{names: []string{"app"}}
	m := logsModelWorkload(t, "deployments", "Deployment", WithLogStreamer(s), WithPodResolver(r), WithContainerLister(l))

	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	deployRef := row.Object

	m, resolveCmd := openWorkloadLogsResolve(t, m)
	if m.viewer.Active() {
		t.Fatal("the viewer should not open until the backing pod resolves")
	}
	msg := resolveCmd()
	prm, ok := msg.(podResolvedMsg)
	if !ok {
		t.Fatalf("opening logs on a workload produced %T, want podResolvedMsg", msg)
	}
	next, fetchCmd := m.Update(prm)
	m = next.(Model)
	if r.calls != 1 {
		t.Fatalf("PodForOwner called %d times, want 1", r.calls)
	}
	if r.gotRef != deployRef {
		t.Fatalf("resolver asked for %+v, want the deployment row %+v", r.gotRef, deployRef)
	}
	if r.gotRes.GVK.Kind != "Deployment" {
		t.Fatalf("resolver got kind %q, want Deployment", r.gotRes.GVK.Kind)
	}

	m, pumpCmd := resolveContainers(t, m, fetchCmd)
	if l.gotRef.Name != "api-xyz" {
		t.Fatalf("the container lister was asked for %q, want the resolved pod api-xyz", l.gotRef.Name)
	}
	if !m.logsView.Active() {
		t.Fatal("a single-container resolved pod should stream directly")
	}
	if s.gotRef.Name != "api-xyz" {
		t.Fatalf("the stream ref = %q, want the resolved pod api-xyz", s.gotRef.Name)
	}
	m = drainLogPump(t, m, pumpCmd)
	content := m.View().Content
	if !strings.Contains(content, "workload log") {
		t.Fatalf("the resolved pod's log line should show: %q", content)
	}
	if !strings.Contains(content, "Pod web/api-xyz") {
		t.Fatalf("the logs header should name the resolved pod, not the workload: %q", content)
	}
}

// TestLogsPodOwningKindNoListerStreamsDirectly proves the resolved pod streams its
// default container directly when no container lister is wired (the M3-05/06 behaviour
// still applies after resolution).
func TestLogsPodOwningKindNoListerStreamsDirectly(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "x"}}}
	r := &fakePodResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz"}}
	m := logsModelWorkload(t, "deployments", "Deployment", WithLogStreamer(s), WithPodResolver(r)) // no lister

	m, resolveCmd := openWorkloadLogsResolve(t, m)
	next, pumpCmd := m.Update(resolveCmd().(podResolvedMsg))
	m = next.(Model)
	if !m.logsView.Active() {
		t.Fatal("no lister → the resolved pod should stream directly into the logs view")
	}
	if s.gotRef.Name != "api-xyz" {
		t.Fatalf("the stream ref = %q, want the resolved pod api-xyz", s.gotRef.Name)
	}
	if s.gotOpts.Container != "" {
		t.Fatalf("no lister → the default container (empty), got %q", s.gotOpts.Container)
	}
	_ = drainLogPump(t, m, pumpCmd)
}

// TestLogsPodOwningKindMultiContainerOpensPicker proves a resolved pod with multiple
// containers still opens the container picker (M3-07a) rather than streaming blindly.
func TestLogsPodOwningKindMultiContainerOpensPicker(t *testing.T) {
	s := &fakeLogStreamer{}
	r := &fakePodResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz"}}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := logsModelWorkload(t, "deployments", "Deployment", WithLogStreamer(s), WithPodResolver(r), WithContainerLister(l))

	m, resolveCmd := openWorkloadLogsResolve(t, m)
	next, fetchCmd := m.Update(resolveCmd().(podResolvedMsg))
	m = next.(Model)
	m, _ = resolveContainers(t, m, fetchCmd)
	if !m.ctrPicker.Active() {
		t.Fatal("a multi-container resolved pod should open the container picker")
	}
	if s.calls != 0 {
		t.Fatal("no stream should start before a container is picked")
	}
}

// TestLogsPodOwningKindResolveErrorDegrades proves a resolution failure (no matching/
// ready pod, RBAC) degrades to a status-bar toast without opening the viewer or
// starting a stream.
func TestLogsPodOwningKindResolveErrorDegrades(t *testing.T) {
	s := &fakeLogStreamer{}
	r := &fakePodResolver{err: errors.New("no pods found")}
	m := logsModelWorkload(t, "deployments", "Deployment", WithLogStreamer(s), WithPodResolver(r))

	m, resolveCmd := openWorkloadLogsResolve(t, m)
	next, _ := m.Update(resolveCmd().(podResolvedMsg))
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("a resolution error should not open the viewer")
	}
	if s.calls != 0 {
		t.Fatal("a resolution error should not start a stream")
	}
	if !m.status.HasError() {
		t.Fatal("a resolution error should surface a status-bar toast")
	}
}

// TestLogsPodResolvedStaleDropped proves the generation guard on the resolution: a
// backing-pod result that lands after a newer logs open (viewerGen bumped) is dropped,
// starting nothing.
func TestLogsPodResolvedStaleDropped(t *testing.T) {
	s := &fakeLogStreamer{events: []kube.LogEvent{{Line: "x"}}}
	r := &fakePodResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz"}}
	m := logsModelWorkload(t, "deployments", "Deployment", WithLogStreamer(s), WithPodResolver(r))

	m, resolveCmd := openWorkloadLogsResolve(t, m)
	stale, ok := resolveCmd().(podResolvedMsg)
	if !ok {
		t.Fatalf("want podResolvedMsg, got %T", resolveCmd())
	}
	// A second open bumps viewerGen, staleifying the first resolution.
	m, _ = openWorkloadLogsResolve(t, m)
	if stale.gen == m.viewerGen {
		t.Fatalf("precondition: stale gen %d should differ from current %d", stale.gen, m.viewerGen)
	}
	next, follow := m.Update(stale)
	m = next.(Model)
	if follow != nil {
		t.Fatal("a stale pod-resolution should be dropped (no follow-on command)")
	}
	if s.calls != 0 {
		t.Fatal("a stale pod-resolution should not start a stream")
	}
}

// --- M3-08a: secret viewer ------------------------------------------------

// fakeSecretGetter is a hermetic SecretGetter: it returns preset secret data (or
// an error) and records the object it was asked for so a test can assert the
// selected row was addressed.
type fakeSecretGetter struct {
	data   kube.SecretData
	err    error
	calls  int
	gotRef kube.ObjectRef
}

func (f *fakeSecretGetter) SecretData(_ context.Context, ref kube.ObjectRef) (kube.SecretData, error) {
	f.calls++
	f.gotRef = ref
	return f.data, f.err
}

// secretViewerModel drills into a secrets table (Kind Secret) with a live row and
// the given getter wired, so a viewer test has a concrete selected row and a fetch
// seam.
func secretViewerModel(t *testing.T, getter SecretGetter) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithSecretGetter(getter))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("secrets", "Secret")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// openSecret dispatches the secret intent over the selected row (the actions-menu
// path — rowActionSecret has no direct key) and returns the model plus the fetch cmd.
func openSecret(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	intent := rowActionMsg{Action: rowActionSecret, Resource: m.current, Object: row.Object}
	next, cmd := m.Update(intent)
	return next.(Model), cmd
}

// revealKey is the default secret.reveal key (`r`).
var revealKey = tea.Key{Code: 'r', Text: "r"}

// TestSecretViewerOpensMaskedThenReveals drives the whole M3-08a path: the secret
// intent opens the viewer and fetches the selected row's data, the data lands
// masked (the value never shown), and the reveal key unmasks it — then re-masks.
func TestSecretViewerOpensMaskedThenReveals(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{
		Type:    "Opaque",
		Entries: []kube.SecretEntry{{Key: "password", Value: "s3cr3t"}},
	}}
	m := secretViewerModel(t, g)

	m, fetchCmd := openSecret(t, m)
	if !m.viewer.Active() {
		t.Fatal("the secret intent should open the viewer")
	}
	if fetchCmd == nil {
		t.Fatal("opening the secret viewer should issue a SecretData fetch")
	}
	loaded, ok := fetchCmd().(secretLoadedMsg)
	if !ok {
		t.Fatalf("fetch produced %T, want secretLoadedMsg", fetchCmd())
	}
	if g.calls != 1 || g.gotRef.Name == "" {
		t.Fatalf("SecretData should be called once for the selected row (calls=%d ref=%q)", g.calls, g.gotRef.Name)
	}
	next, _ := m.Update(loaded)
	m = next.(Model)

	// Masked: the key and its byte count show, the value does not.
	view := m.View().Content
	if !strings.Contains(view, "password") || !strings.Contains(view, secretMask) {
		t.Fatalf("masked view should show the key + mask: %q", view)
	}
	if strings.Contains(view, "s3cr3t") {
		t.Fatal("the value must not be shown before an explicit reveal")
	}

	// Reveal: `r` unmasks.
	m, cmd := press(t, m, revealKey)
	if cmd != nil {
		t.Fatal("secret.reveal should not emit a command (in-place re-render)")
	}
	if !m.secretRevealed {
		t.Fatal("reveal key should flip secretRevealed on")
	}
	if !strings.Contains(m.View().Content, "s3cr3t") {
		t.Fatalf("revealed view should show the value: %q", m.View().Content)
	}

	// Re-mask: a second `r` hides it again.
	m, _ = press(t, m, revealKey)
	if m.secretRevealed {
		t.Fatal("a second reveal press should hide the values again")
	}
	if strings.Contains(m.View().Content, "s3cr3t") {
		t.Fatal("re-masking should hide the value")
	}
}

// TestSecretViewerFetchErrorDegrades proves a SecretData failure closes the viewer
// and surfaces a transient status-bar toast rather than leaving an empty box (D74).
func TestSecretViewerFetchErrorDegrades(t *testing.T) {
	g := &fakeSecretGetter{err: errors.New("forbidden")}
	m := secretViewerModel(t, g)
	m, fetchCmd := openSecret(t, m)
	if !m.viewer.Active() {
		t.Fatal("the viewer opens immediately, before the fetch resolves")
	}
	loaded := fetchCmd().(secretLoadedMsg)
	if loaded.err == nil {
		t.Fatal("the fetch should carry the getter's error")
	}
	next, _ := m.Update(loaded)
	m = next.(Model)
	if m.viewer.Active() {
		t.Fatal("a fetch error should close the viewer")
	}
	if !m.status.HasError() {
		t.Fatal("a fetch error should surface a status-bar toast")
	}
}

// TestSecretViewerInertWithoutGetter proves the secret intent is a no-op with no
// getter wired (the viewer never opens).
func TestSecretViewerInertWithoutGetter(t *testing.T) {
	m := openPodTable(t, "Secret") // no WithSecretGetter
	m, fetchCmd := openSecret(t, m)
	if m.viewer.Active() {
		t.Fatal("the secret viewer should not open without a getter wired")
	}
	if fetchCmd != nil {
		t.Fatal("no getter → no fetch command")
	}
}

// TestSecretViewerCloses proves nav.back (esc) dismisses the secret viewer.
func TestSecretViewerCloses(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{Type: "Opaque"}}
	m := secretViewerModel(t, g)
	m, fetchCmd := openSecret(t, m)
	next, _ := m.Update(fetchCmd().(secretLoadedMsg))
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

// TestSecretViewerStaleFetchDropped proves the generation guard: a fetch that lands
// after a newer viewer open is dropped rather than populating stale content.
func TestSecretViewerStaleFetchDropped(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{
		Type:    "Opaque",
		Entries: []kube.SecretEntry{{Key: "token", Value: "abc"}},
	}}
	m := secretViewerModel(t, g)
	m, firstFetch := openSecret(t, m)
	stale := firstFetch().(secretLoadedMsg)

	m, _ = openSecret(t, m) // second open bumps viewerGen
	if stale.gen == m.viewerGen {
		t.Fatalf("precondition: stale gen %d should differ from current %d", stale.gen, m.viewerGen)
	}
	next, _ := m.Update(stale) // deliver the stale (gen-1) result
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should still be open")
	}
	// Even revealed, a dropped fetch left no data, so the value never appears.
	m, _ = press(t, m, revealKey)
	if strings.Contains(m.View().Content, "abc") {
		t.Fatal("a stale-generation fetch should be dropped, not shown")
	}
}

// TestRevealInertOnDescribeViewer proves secret.reveal is inert on a non-secret viewer.
func TestRevealInertOnDescribeViewer(t *testing.T) {
	d := &fakeDescriber{text: "Name: web-1\n"}
	m := describeViewerModel(t, d)
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(describeLoadedMsg))
	m = next.(Model)
	if m.viewer.Kind() != viewerKindDescribe {
		t.Fatalf("precondition: the describe viewer should be up, kind = %q", m.viewer.Kind())
	}
	m, revealCmd := press(t, m, revealKey)
	if revealCmd != nil {
		t.Fatal("secret.reveal on the describe viewer should be inert (no command)")
	}
	if m.secretRevealed {
		t.Fatal("secret.reveal should not toggle reveal on a non-secret viewer")
	}
	if !m.viewer.Active() {
		t.Fatal("secret.reveal should not close the describe viewer")
	}
}

// copyKey is the default secret.copy key (`c`).
var copyKey = tea.Key{Code: 'c', Text: "c"}

// copiedClipboard runs a secret-copy batch and returns the string handed to
// tea.SetClipboard. The batch also carries the notice auto-clear tick, which blocks
// for errorDisplay, so each sub-command runs with a short timeout and only the
// instant clipboard write is collected. setClipboardMsg is a string-kinded message,
// so fmt.Sprint yields its content.
func copiedClipboard(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		t.Fatal("copy should return a command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("copy cmd produced %T, want tea.BatchMsg", cmd())
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { ch <- c() }(c)
		select {
		case msg := <-ch:
			return fmt.Sprint(msg)
		case <-time.After(200 * time.Millisecond):
			// the blocking auto-clear tick — skip it and try the next sub-command.
		}
	}
	t.Fatal("copy batch carried no clipboard write")
	return ""
}

// loadedSecret opens the secret viewer over the selected row and delivers the fetch,
// returning a model whose secret viewer is populated and masked.
func loadedSecret(t *testing.T, g SecretGetter) Model {
	t.Helper()
	m := secretViewerModel(t, g)
	m, fetchCmd := openSecret(t, m)
	next, _ := m.Update(fetchCmd().(secretLoadedMsg))
	return next.(Model)
}

// TestRenderSecretCursorAndLines unit-tests the render: the cursor gutter marks the
// selected entry, the returned line offsets point at each entry's key line, the
// masked render never contains a value, and a revealed multi-line value is indented
// under its key with the offsets tracking it (M3-08b).
func TestRenderSecretCursorAndLines(t *testing.T) {
	data := kube.SecretData{
		Type: "Opaque",
		Entries: []kube.SecretEntry{
			{Key: "password", Value: "s3cr3t"},
			{Key: "token", Value: "line1\nline2"},
		},
	}

	content, lines := renderSecret(data, false, 1)
	if len(lines) != 2 {
		t.Fatalf("want a line offset per entry, got %d", len(lines))
	}
	ls := strings.Split(content, "\n")
	if !strings.HasPrefix(ls[lines[1]], secretCursor+"token") {
		t.Fatalf("the cursor gutter should mark the selected entry: %q", ls[lines[1]])
	}
	if !strings.HasPrefix(ls[lines[0]], secretGutter+"password") {
		t.Fatalf("an unselected entry should carry the plain gutter: %q", ls[lines[0]])
	}
	if strings.Contains(content, "s3cr3t") || strings.Contains(content, "line1") {
		t.Fatalf("a masked render must not contain any value: %q", content)
	}

	content, lines = renderSecret(data, true, 0)
	ls = strings.Split(content, "\n")
	if !strings.HasPrefix(ls[lines[0]], secretCursor+"password") {
		t.Fatalf("revealed cursor gutter wrong: %q", ls[lines[0]])
	}
	if !strings.Contains(content, "s3cr3t") {
		t.Fatal("a revealed render should contain the single-line value")
	}
	if !strings.HasPrefix(ls[lines[1]], secretGutter+"token:") {
		t.Fatalf("a multi-line entry's key line wrong: %q", ls[lines[1]])
	}
	if ls[lines[1]+1] != secretGutter+"  line1" {
		t.Fatalf("a multi-line value should be indented under its key: %q", ls[lines[1]+1])
	}
}

// TestSecretCopyCopiesSelectedValueMasked proves secret.copy puts the selected
// entry's decoded value on the clipboard while the value stays masked on screen — a
// copy is a deliberate gesture that need not reveal first — and surfaces a neutral
// notice naming the key (never the value).
func TestSecretCopyCopiesSelectedValueMasked(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{
		Type: "Opaque",
		Entries: []kube.SecretEntry{
			{Key: "password", Value: "s3cr3t"},
			{Key: "token", Value: "abc123"},
		},
	}}
	m := loadedSecret(t, g)

	if strings.Contains(m.View().Content, "s3cr3t") {
		t.Fatal("precondition: the value must be masked before any reveal")
	}
	m, copyCmd := press(t, m, copyKey)
	if got := copiedClipboard(t, copyCmd); got != "s3cr3t" {
		t.Fatalf("copy should place the selected entry's decoded value on the clipboard, got %q", got)
	}
	view := m.View().Content
	if !strings.Contains(view, "copied") || !strings.Contains(view, "password") {
		t.Fatalf("copy should surface a notice naming the key: %q", view)
	}
	if strings.Contains(view, "s3cr3t") {
		t.Fatal("copying must not reveal the value on screen")
	}
	if !m.status.HasNotice() {
		t.Fatal("copy should set a status-bar notice")
	}
}

// TestSecretCopyFollowsCursor proves nav.up/down move the entry cursor (not scroll)
// and copy targets the selected entry, clamping at both ends.
func TestSecretCopyFollowsCursor(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{
		Type: "Opaque",
		Entries: []kube.SecretEntry{
			{Key: "password", Value: "s3cr3t"},
			{Key: "token", Value: "abc123"},
		},
	}}
	m := loadedSecret(t, g)

	// down moves the entry cursor to the second entry.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.secretSel != 1 {
		t.Fatalf("nav.down should move the entry cursor to 1, got %d", m.secretSel)
	}
	m, copyCmd := press(t, m, copyKey)
	if got := copiedClipboard(t, copyCmd); got != "abc123" {
		t.Fatalf("copy should follow the cursor to the second entry, got %q", got)
	}

	// down clamps at the last entry.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.secretSel != 1 {
		t.Fatalf("nav.down should clamp at the last entry, got %d", m.secretSel)
	}
	// up walks back and clamps at the first entry.
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"})
	m, _ = press(t, m, tea.Key{Code: 'k', Text: "k"})
	if m.secretSel != 0 {
		t.Fatalf("nav.up should clamp at the first entry, got %d", m.secretSel)
	}
}

// TestSecretCopyInertWithoutEntries proves copy is a no-op (no command, no notice)
// when the Secret has no data.
func TestSecretCopyInertWithoutEntries(t *testing.T) {
	g := &fakeSecretGetter{data: kube.SecretData{Type: "Opaque"}}
	m := loadedSecret(t, g)
	m, copyCmd := press(t, m, copyKey)
	if copyCmd != nil {
		t.Fatal("copy should be inert when the secret has no data")
	}
	if m.status.HasNotice() {
		t.Fatal("an inert copy should not surface a notice")
	}
}

// TestCopyInertOnDescribeViewer proves secret.copy is inert on a non-secret viewer.
func TestCopyInertOnDescribeViewer(t *testing.T) {
	d := &fakeDescriber{text: "Name: web-1\n"}
	m := describeViewerModel(t, d)
	_, cmd := press(t, m, describeKey)
	next, fetchCmd := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(describeLoadedMsg))
	m = next.(Model)
	m, copyCmd := press(t, m, copyKey)
	if copyCmd != nil {
		t.Fatal("secret.copy on the describe viewer should be inert (no command)")
	}
	if m.status.HasNotice() {
		t.Fatal("secret.copy on a non-secret viewer should not surface a notice")
	}
	if !m.viewer.Active() {
		t.Fatal("secret.copy should not close the describe viewer")
	}
}

// fakeDeleter is a hermetic Deleter: it records the object it was asked to delete
// (so a test can assert the selected row — with its UID — was addressed) and returns
// a preset error. *kube.Clients is the real one; this stands in for the seam.
type fakeDeleter struct {
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
}

func (f *fakeDeleter) Delete(_ context.Context, r kube.Resource, ref kube.ObjectRef, _ metav1.DeleteOptions) error {
	f.calls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

// deleteTableModel drills into a pods table with a deleter wired so a delete test has
// a concrete selected row (carrying a UID) and a delete seam.
func deleteTableModel(t *testing.T, d Deleter) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithDeleter(d))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// deleteKey is the default res.delete key (`d`).
var deleteKey = tea.Key{Code: 'd', Text: "d"}

// openDeleteModal presses the delete key over the selected row and delivers the
// resulting rowActionMsg, returning the model with the confirm modal open.
func openDeleteModal(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := press(t, m, deleteKey)
	if cmd == nil {
		t.Fatal("the delete key should dispatch a row-action intent")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("delete key produced %T, want rowActionMsg", cmd())
	}
	next, _ := m.Update(intent)
	return next.(Model)
}

// TestDeleteOpensConfirmModal proves the delete key opens the confirm modal over the
// selected row (naming the target) rather than deleting outright — no kube call yet.
func TestDeleteOpensConfirmModal(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	row, _ := m.table.SelectedRow()

	m = openDeleteModal(t, m)
	if !m.modal.Active() {
		t.Fatal("the delete key should open the confirm modal")
	}
	if m.modal.Kind() != deleteModalKind {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind(), deleteModalKind)
	}
	if d.calls != 0 {
		t.Fatal("opening the confirm modal must not delete anything yet")
	}
	// The modal names the target so the user knows what they are confirming.
	if view := m.View().Content; !strings.Contains(view, row.Object.Name) {
		t.Fatalf("the confirm modal should name the target row %q: %q", row.Object.Name, view)
	}
}

// TestDeleteConfirmRunsDelete drives the whole accept path: open the modal, accept
// (enter → nav.drillIn) → the modal closes and kube.Delete runs against the selected
// row (its UID rides through for the snapshot guard), and the success surfaces a
// neutral status notice.
func TestDeleteConfirmRunsDelete(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	row, _ := m.table.SelectedRow()
	m = openDeleteModal(t, m)

	// Accept: enter (nav.drillIn) emits ConfirmedMsg.
	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	if confirmCmd == nil {
		t.Fatal("accepting the modal should emit a confirmed command")
	}
	confirmed, ok := confirmCmd().(modal.ConfirmedMsg)
	if !ok {
		t.Fatalf("accept produced %T, want modal.ConfirmedMsg", confirmCmd())
	}
	next, delCmd := m.Update(confirmed)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("accepting the modal should hide it")
	}
	if delCmd == nil {
		t.Fatal("a confirmed delete should issue the kube.Delete command")
	}
	done, ok := delCmd().(deleteDoneMsg)
	if !ok {
		t.Fatalf("delete produced %T, want deleteDoneMsg", delCmd())
	}
	if d.calls != 1 {
		t.Fatalf("Delete should be called exactly once, got %d", d.calls)
	}
	if d.gotRef.Name != row.Object.Name || d.gotRef.UID != row.Object.UID {
		t.Fatalf("Delete addressed %+v, want the selected row %+v", d.gotRef, row.Object)
	}
	next, _ = m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() {
		t.Fatal("a successful delete should surface a neutral status notice")
	}
	if m.status.HasError() {
		t.Fatal("a successful delete should not surface an error")
	}
}

// TestDeleteConfirmDeclineDoesNothing proves declining (esc → nav.back) closes the
// modal without deleting.
func TestDeleteConfirmDeclineDoesNothing(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	m = openDeleteModal(t, m)

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("declining the modal should emit a cancelled command")
	}
	cancelled, ok := cancelCmd().(modal.CancelledMsg)
	if !ok {
		t.Fatalf("decline produced %T, want modal.CancelledMsg", cancelCmd())
	}
	next, _ := m.Update(cancelled)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("declining the modal should hide it")
	}
	if d.calls != 0 {
		t.Fatal("a declined confirm must not delete anything")
	}
}

// TestDeleteConfirmAcceptWithY proves `y` accepts the confirm modal (confirm.accept)
// just like enter — the yes/no muscle memory, resolved in the dedicated confirm key
// context so it can't collide with the browse context's `n`/enter/esc (D132). (`y`
// itself is now browse-free since res.yaml was retired into edit, D135/M3-15c.)
func TestDeleteConfirmAcceptWithY(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	row, _ := m.table.SelectedRow()
	m = openDeleteModal(t, m)

	m, confirmCmd := press(t, m, tea.Key{Code: 'y', Text: "y"})
	if confirmCmd == nil {
		t.Fatal("`y` should accept the confirm modal")
	}
	confirmed, ok := confirmCmd().(modal.ConfirmedMsg)
	if !ok {
		t.Fatalf("`y` produced %T, want modal.ConfirmedMsg", confirmCmd())
	}
	next, delCmd := m.Update(confirmed)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("accepting with `y` should hide the modal")
	}
	if delCmd == nil {
		t.Fatal("accepting with `y` should issue the kube.Delete command")
	}
	if _, ok := delCmd().(deleteDoneMsg); !ok {
		t.Fatalf("`y` delete produced %T, want deleteDoneMsg", delCmd())
	}
	if d.calls != 1 || d.gotRef.Name != row.Object.Name {
		t.Fatalf("Delete addressed %+v x%d, want the selected row %+v once", d.gotRef, d.calls, row.Object)
	}
}

// TestDeleteConfirmDeclineWithN proves `n` declines the confirm modal
// (confirm.decline) just like esc — closing it without deleting.
func TestDeleteConfirmDeclineWithN(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	m = openDeleteModal(t, m)

	m, cancelCmd := press(t, m, tea.Key{Code: 'n', Text: "n"})
	if cancelCmd == nil {
		t.Fatal("`n` should decline the confirm modal")
	}
	cancelled, ok := cancelCmd().(modal.CancelledMsg)
	if !ok {
		t.Fatalf("`n` produced %T, want modal.CancelledMsg", cancelCmd())
	}
	next, _ := m.Update(cancelled)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("declining with `n` should hide the modal")
	}
	if d.calls != 0 {
		t.Fatal("declining with `n` must not delete anything")
	}
}

// TestDeleteErrorDegrades proves a failed delete (e.g. RBAC or a UID conflict from
// the snapshot guard) degrades to a transient status-bar error toast (D74).
func TestDeleteErrorDegrades(t *testing.T) {
	d := &fakeDeleter{err: errors.New("forbidden")}
	m := deleteTableModel(t, d)
	m = openDeleteModal(t, m)
	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	next, delCmd := m.Update(confirmCmd().(modal.ConfirmedMsg))
	m = next.(Model)
	done := delCmd().(deleteDoneMsg)
	if done.err == nil {
		t.Fatal("the delete result should carry the deleter's error")
	}
	next, _ = m.Update(done)
	m = next.(Model)
	if !m.status.HasError() {
		t.Fatal("a failed delete should surface a status-bar error toast")
	}
}

// TestDeleteInertWithoutDeleter proves the delete key is a no-op with no deleter
// wired: the confirm modal never opens.
func TestDeleteInertWithoutDeleter(t *testing.T) {
	m := openPodTable(t, "Pod") // no WithDeleter
	m, cmd := press(t, m, deleteKey)
	if cmd == nil {
		t.Fatal("the delete key still dispatches an intent (routing stays observable)")
	}
	next, _ := m.Update(cmd().(rowActionMsg))
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("with no deleter wired the confirm modal must not open")
	}
}

// TestDeleteModalSwallowsNav proves the open confirm modal captures input: a nav key
// does not move the browse panes underneath it.
func TestDeleteModalSwallowsNav(t *testing.T) {
	d := &fakeDeleter{}
	m := deleteTableModel(t, d)
	before, _ := m.table.SelectedRow()
	m = openDeleteModal(t, m)
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"}) // nav.down
	if !m.modal.Active() {
		t.Fatal("a nav key must not close the modal")
	}
	after, _ := m.table.SelectedRow()
	if after.Object.Name != before.Object.Name {
		t.Fatal("the table selection must not move while the confirm modal is up")
	}
}

// fakeScaler / fakeRestarter are hermetic stand-ins for the M3-10 mutating seams:
// each records what it was asked to do (so a test can assert the selected row was
// addressed and, for scale, the replica count) and returns a preset error.
type fakeScaler struct {
	err         error
	calls       int
	gotRes      kube.Resource
	gotRef      kube.ObjectRef
	gotReplicas int32
}

func (f *fakeScaler) Scale(_ context.Context, r kube.Resource, ref kube.ObjectRef, replicas int32) error {
	f.calls++
	f.gotRes = r
	f.gotRef = ref
	f.gotReplicas = replicas
	return f.err
}

type fakeRestarter struct {
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
}

func (f *fakeRestarter) RolloutRestart(_ context.Context, r kube.Resource, ref kube.ObjectRef) error {
	f.calls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

// workloadModel drills into a scalable-workload table (Kind Deployment) with the
// given options wired so a scale/rollout test has a concrete selected row.
func workloadModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("deployments", "Deployment")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// dispatchRowAction delivers a row-action intent over the selected row (the
// actions-menu path — scale/rollout have no direct key) and returns the model plus
// any resulting cmd.
func dispatchRowAction(t *testing.T, m Model, action rowAction) (Model, tea.Cmd) {
	t.Helper()
	row, ok := m.table.SelectedRow()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}
	next, cmd := m.Update(rowActionMsg{Action: action, Resource: m.current, Object: row.Object})
	return next.(Model), cmd
}

// TestScaleOpensPrompt proves the scale intent opens the modal's replica prompt over
// the selected row (naming the target) rather than scaling outright — no kube call yet.
func TestScaleOpensPrompt(t *testing.T) {
	s := &fakeScaler{}
	m := workloadModel(t, WithScaler(s))
	row, _ := m.table.SelectedRow()

	m, _ = dispatchRowAction(t, m, rowActionScale)
	if !m.modal.Active() {
		t.Fatal("the scale intent should open the modal")
	}
	if !m.modal.Prompting() {
		t.Fatal("the scale modal should be in prompt mode")
	}
	if m.modal.Kind() != scaleModalKind {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind(), scaleModalKind)
	}
	if s.calls != 0 {
		t.Fatal("opening the prompt must not scale anything yet")
	}
	if view := m.View().Content; !strings.Contains(view, row.Object.Name) {
		t.Fatalf("the scale prompt should name the target row %q: %q", row.Object.Name, view)
	}
}

// TestScalePromptRunsScale drives the whole accept path: open the prompt, type a
// replica count (routed to the field while Prompting), accept (enter → nav.drillIn) →
// the modal closes and kube.Scale runs against the selected row with the typed count,
// and the success surfaces a neutral status notice.
func TestScalePromptRunsScale(t *testing.T) {
	s := &fakeScaler{}
	m := workloadModel(t, WithScaler(s))
	row, _ := m.table.SelectedRow()
	m, _ = dispatchRowAction(t, m, rowActionScale)

	// Typed digits are routed to the prompt field (not the sequencer) while Prompting.
	m, _ = press(t, m, tea.Key{Code: '1', Text: "1"})
	m, _ = press(t, m, tea.Key{Code: '2', Text: "2"})
	if got := m.modal.Value(); got != "12" {
		t.Fatalf("prompt field = %q, want %q — digits must reach the input", got, "12")
	}

	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	if confirmCmd == nil {
		t.Fatal("accepting the prompt should emit a confirmed command")
	}
	confirmed, ok := confirmCmd().(modal.ConfirmedMsg)
	if !ok {
		t.Fatalf("accept produced %T, want modal.ConfirmedMsg", confirmCmd())
	}
	if confirmed.Value != "12" {
		t.Fatalf("ConfirmedMsg carried %q, want the typed %q", confirmed.Value, "12")
	}
	next, scaleCmd := m.Update(confirmed)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("accepting the prompt should hide the modal")
	}
	if scaleCmd == nil {
		t.Fatal("a submitted scale should issue the kube.Scale command")
	}
	done, ok := scaleCmd().(scaleDoneMsg)
	if !ok {
		t.Fatalf("scale produced %T, want scaleDoneMsg", scaleCmd())
	}
	if s.calls != 1 {
		t.Fatalf("Scale should be called exactly once, got %d", s.calls)
	}
	if s.gotReplicas != 12 {
		t.Fatalf("Scale replicas = %d, want 12", s.gotReplicas)
	}
	if s.gotRef.Name != row.Object.Name {
		t.Fatalf("Scale addressed %+v, want the selected row %+v", s.gotRef, row.Object)
	}
	next, _ = m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful scale should surface a neutral status notice, no error")
	}
}

// TestScaleInvalidReplicasDegrades proves a non-integer (or blank) entry degrades to a
// status-bar error toast and runs no scale — the modal is already hidden.
func TestScaleInvalidReplicasDegrades(t *testing.T) {
	s := &fakeScaler{}
	m := workloadModel(t, WithScaler(s))
	m, _ = dispatchRowAction(t, m, rowActionScale)
	m, _ = press(t, m, tea.Key{Code: 'x', Text: "x"}) // not a number
	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	next, scaleCmd := m.Update(confirmCmd().(modal.ConfirmedMsg))
	m = next.(Model)
	if scaleCmd != nil {
		if _, ok := scaleCmd().(scaleDoneMsg); ok {
			t.Fatal("an invalid replica count must not run a scale")
		}
	}
	if s.calls != 0 {
		t.Fatal("an invalid replica count must not call Scale")
	}
	if !m.status.HasError() {
		t.Fatal("an invalid replica count should surface a status-bar error toast")
	}
}

// TestScaleErrorDegrades proves a failed scale (e.g. RBAC, NotFound) degrades to a
// transient status-bar error toast (D74).
func TestScaleErrorDegrades(t *testing.T) {
	s := &fakeScaler{err: errors.New("forbidden")}
	m := workloadModel(t, WithScaler(s))
	m, _ = dispatchRowAction(t, m, rowActionScale)
	m, _ = press(t, m, tea.Key{Code: '2', Text: "2"})
	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	next, scaleCmd := m.Update(confirmCmd().(modal.ConfirmedMsg))
	m = next.(Model)
	done := scaleCmd().(scaleDoneMsg)
	if done.err == nil {
		t.Fatal("the scale result should carry the scaler's error")
	}
	next, _ = m.Update(done)
	m = next.(Model)
	if !m.status.HasError() {
		t.Fatal("a failed scale should surface a status-bar error toast")
	}
}

// TestScaleInertWithoutScaler proves the scale intent is a no-op with no scaler wired:
// the prompt never opens.
func TestScaleInertWithoutScaler(t *testing.T) {
	m := workloadModel(t) // no WithScaler
	m, _ = dispatchRowAction(t, m, rowActionScale)
	if m.modal.Active() {
		t.Fatal("with no scaler wired the scale prompt must not open")
	}
}

// TestRolloutRestartOpensConfirm proves the rollout-restart intent opens a confirm
// modal over the selected row (naming the target) rather than restarting outright.
func TestRolloutRestartOpensConfirm(t *testing.T) {
	r := &fakeRestarter{}
	m := workloadModel(t, WithRolloutRestarter(r))
	row, _ := m.table.SelectedRow()

	m, _ = dispatchRowAction(t, m, rowActionRolloutRestart)
	if !m.modal.Active() {
		t.Fatal("the rollout-restart intent should open the confirm modal")
	}
	if m.modal.Prompting() {
		t.Fatal("rollout-restart is a yes/no confirm, not a prompt")
	}
	if m.modal.Kind() != rolloutRestartModalKind {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind(), rolloutRestartModalKind)
	}
	if r.calls != 0 {
		t.Fatal("opening the confirm must not restart anything yet")
	}
	if view := m.View().Content; !strings.Contains(view, row.Object.Name) {
		t.Fatalf("the confirm should name the target row %q: %q", row.Object.Name, view)
	}
}

// TestRolloutRestartRunsRestart drives the accept path: accept (enter) → the modal
// closes and kube.RolloutRestart runs against the selected row, success → notice.
func TestRolloutRestartRunsRestart(t *testing.T) {
	r := &fakeRestarter{}
	m := workloadModel(t, WithRolloutRestarter(r))
	row, _ := m.table.SelectedRow()
	m, _ = dispatchRowAction(t, m, rowActionRolloutRestart)

	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	next, restartCmd := m.Update(confirmCmd().(modal.ConfirmedMsg))
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("accepting the confirm should hide the modal")
	}
	done, ok := restartCmd().(restartDoneMsg)
	if !ok {
		t.Fatalf("restart produced %T, want restartDoneMsg", restartCmd())
	}
	if r.calls != 1 {
		t.Fatalf("RolloutRestart should be called exactly once, got %d", r.calls)
	}
	if r.gotRef.Name != row.Object.Name {
		t.Fatalf("RolloutRestart addressed %+v, want the selected row %+v", r.gotRef, row.Object)
	}
	next, _ = m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful restart should surface a neutral status notice, no error")
	}
}

// TestRolloutRestartDeclineDoesNothing proves declining (esc → nav.back) closes the
// modal without restarting.
func TestRolloutRestartDeclineDoesNothing(t *testing.T) {
	r := &fakeRestarter{}
	m := workloadModel(t, WithRolloutRestarter(r))
	m, _ = dispatchRowAction(t, m, rowActionRolloutRestart)
	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	next, _ := m.Update(cancelCmd().(modal.CancelledMsg))
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("declining the confirm should hide the modal")
	}
	if r.calls != 0 {
		t.Fatal("a declined confirm must not restart anything")
	}
}

// TestRolloutRestartInertWithoutRestarter proves the intent is a no-op with no
// restarter wired: the confirm modal never opens.
func TestRolloutRestartInertWithoutRestarter(t *testing.T) {
	m := workloadModel(t) // no WithRolloutRestarter
	m, _ = dispatchRowAction(t, m, rowActionRolloutRestart)
	if m.modal.Active() {
		t.Fatal("with no restarter wired the confirm modal must not open")
	}
}

// fakeCordoner is a hermetic Cordoner: it records which verb it was asked to run and
// the object addressed (so a test can assert the selected Node row was targeted) and
// returns a preset error for both verbs.
type fakeCordoner struct {
	err           error
	cordonCalls   int
	uncordonCalls int
	gotRes        kube.Resource
	gotRef        kube.ObjectRef
}

func (f *fakeCordoner) Cordon(_ context.Context, r kube.Resource, ref kube.ObjectRef) error {
	f.cordonCalls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

func (f *fakeCordoner) Uncordon(_ context.Context, r kube.Resource, ref kube.ObjectRef) error {
	f.uncordonCalls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

// nodeModel drills into a Node table (cluster-scoped) with the given options wired so
// a cordon/uncordon test has a concrete selected row.
func nodeModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("nodes", "Node")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// TestCordonRunsCordon drives the cordon path: the intent dispatches kube.Cordon
// against the selected Node **directly** — no confirm modal (cordoning is idempotent,
// D115/D35) — and the success surfaces a neutral status notice.
func TestCordonRunsCordon(t *testing.T) {
	c := &fakeCordoner{}
	m := nodeModel(t, WithCordoner(c))
	row, _ := m.table.SelectedRow()

	m, cmd := dispatchRowAction(t, m, rowActionCordon)
	if m.modal.Active() {
		t.Fatal("cordon is idempotent — it must not open a confirm modal")
	}
	if cmd == nil {
		t.Fatal("the cordon intent should issue the kube.Cordon command")
	}
	done, ok := cmd().(cordonDoneMsg)
	if !ok {
		t.Fatalf("cordon produced %T, want cordonDoneMsg", cmd())
	}
	if c.cordonCalls != 1 {
		t.Fatalf("Cordon should be called exactly once, got %d", c.cordonCalls)
	}
	if c.uncordonCalls != 0 {
		t.Fatal("cordon must not call Uncordon")
	}
	if c.gotRef.Name != row.Object.Name {
		t.Fatalf("Cordon addressed %+v, want the selected row %+v", c.gotRef, row.Object)
	}
	if !done.cordon {
		t.Fatal("the done message should mark this as a cordon, not an uncordon")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful cordon should surface a neutral status notice, no error")
	}
}

// TestUncordonRunsUncordon proves the uncordon intent dispatches kube.Uncordon
// (the reverse verb) against the selected Node, also directly.
func TestUncordonRunsUncordon(t *testing.T) {
	c := &fakeCordoner{}
	m := nodeModel(t, WithCordoner(c))
	row, _ := m.table.SelectedRow()

	m, cmd := dispatchRowAction(t, m, rowActionUncordon)
	if cmd == nil {
		t.Fatal("the uncordon intent should issue the kube.Uncordon command")
	}
	done, ok := cmd().(cordonDoneMsg)
	if !ok {
		t.Fatalf("uncordon produced %T, want cordonDoneMsg", cmd())
	}
	if c.uncordonCalls != 1 || c.cordonCalls != 0 {
		t.Fatalf("uncordon should call Uncordon once and Cordon zero, got %d/%d", c.uncordonCalls, c.cordonCalls)
	}
	if c.gotRef.Name != row.Object.Name {
		t.Fatalf("Uncordon addressed %+v, want the selected row %+v", c.gotRef, row.Object)
	}
	if done.cordon {
		t.Fatal("the done message should mark this as an uncordon")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful uncordon should surface a neutral status notice, no error")
	}
}

// TestCordonErrorDegrades proves a failed cordon (e.g. RBAC, NotFound) degrades to a
// transient status-bar error toast (D74).
func TestCordonErrorDegrades(t *testing.T) {
	c := &fakeCordoner{err: errors.New("forbidden")}
	m := nodeModel(t, WithCordoner(c))
	m, cmd := dispatchRowAction(t, m, rowActionCordon)
	done := cmd().(cordonDoneMsg)
	if done.err == nil {
		t.Fatal("the cordon result should carry the cordoner's error")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasError() {
		t.Fatal("a failed cordon should surface a status-bar error toast")
	}
}

// TestCordonInertWithoutCordoner proves the cordon/uncordon intents are no-ops with no
// cordoner wired: no kube command is issued.
func TestCordonInertWithoutCordoner(t *testing.T) {
	m := nodeModel(t) // no WithCordoner
	if _, cmd := dispatchRowAction(t, m, rowActionCordon); cmd != nil {
		t.Fatal("with no cordoner wired the cordon intent must not issue a command")
	}
	if _, cmd := dispatchRowAction(t, m, rowActionUncordon); cmd != nil {
		t.Fatal("with no cordoner wired the uncordon intent must not issue a command")
	}
}

// fakeSuspender is a hermetic Suspender: it records which verb it was asked to run
// and the object addressed (so a test can assert the selected CronJob row was
// targeted) and returns a preset error for both verbs.
type fakeSuspender struct {
	err          error
	suspendCalls int
	resumeCalls  int
	gotRes       kube.Resource
	gotRef       kube.ObjectRef
}

func (f *fakeSuspender) Suspend(_ context.Context, r kube.Resource, ref kube.ObjectRef) error {
	f.suspendCalls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

func (f *fakeSuspender) Resume(_ context.Context, r kube.Resource, ref kube.ObjectRef) error {
	f.resumeCalls++
	f.gotRes = r
	f.gotRef = ref
	return f.err
}

// cronJobModel drills into a CronJob table with the given options wired so a
// suspend/resume test has a concrete selected row.
func cronJobModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("cronjobs", "CronJob")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// TestSuspendRunsSuspend drives the suspend path: the intent dispatches kube.Suspend
// against the selected CronJob **directly** — no confirm modal (suspending is
// idempotent, D120/D35) — and the success surfaces a neutral status notice.
func TestSuspendRunsSuspend(t *testing.T) {
	s := &fakeSuspender{}
	m := cronJobModel(t, WithSuspender(s))
	row, _ := m.table.SelectedRow()

	m, cmd := dispatchRowAction(t, m, rowActionSuspend)
	if m.modal.Active() {
		t.Fatal("suspend is idempotent — it must not open a confirm modal")
	}
	if cmd == nil {
		t.Fatal("the suspend intent should issue the kube.Suspend command")
	}
	done, ok := cmd().(suspendDoneMsg)
	if !ok {
		t.Fatalf("suspend produced %T, want suspendDoneMsg", cmd())
	}
	if s.suspendCalls != 1 {
		t.Fatalf("Suspend should be called exactly once, got %d", s.suspendCalls)
	}
	if s.resumeCalls != 0 {
		t.Fatal("suspend must not call Resume")
	}
	if s.gotRef.Name != row.Object.Name {
		t.Fatalf("Suspend addressed %+v, want the selected row %+v", s.gotRef, row.Object)
	}
	if !done.suspend {
		t.Fatal("the done message should mark this as a suspend, not a resume")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful suspend should surface a neutral status notice, no error")
	}
}

// TestResumeRunsResume proves the resume intent dispatches kube.Resume (the reverse
// verb) against the selected CronJob, also directly.
func TestResumeRunsResume(t *testing.T) {
	s := &fakeSuspender{}
	m := cronJobModel(t, WithSuspender(s))
	row, _ := m.table.SelectedRow()

	m, cmd := dispatchRowAction(t, m, rowActionResume)
	if cmd == nil {
		t.Fatal("the resume intent should issue the kube.Resume command")
	}
	done, ok := cmd().(suspendDoneMsg)
	if !ok {
		t.Fatalf("resume produced %T, want suspendDoneMsg", cmd())
	}
	if s.resumeCalls != 1 || s.suspendCalls != 0 {
		t.Fatalf("resume should call Resume once and Suspend zero, got %d/%d", s.resumeCalls, s.suspendCalls)
	}
	if s.gotRef.Name != row.Object.Name {
		t.Fatalf("Resume addressed %+v, want the selected row %+v", s.gotRef, row.Object)
	}
	if done.suspend {
		t.Fatal("the done message should mark this as a resume")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful resume should surface a neutral status notice, no error")
	}
}

// TestSuspendErrorDegrades proves a failed suspend (e.g. RBAC, NotFound) degrades to a
// transient status-bar error toast (D74).
func TestSuspendErrorDegrades(t *testing.T) {
	s := &fakeSuspender{err: errors.New("forbidden")}
	m := cronJobModel(t, WithSuspender(s))
	m, cmd := dispatchRowAction(t, m, rowActionSuspend)
	done := cmd().(suspendDoneMsg)
	if done.err == nil {
		t.Fatal("the suspend result should carry the suspender's error")
	}
	next, _ := m.Update(done)
	m = next.(Model)
	if !m.status.HasError() {
		t.Fatal("a failed suspend should surface a status-bar error toast")
	}
}

// TestSuspendInertWithoutSuspender proves the suspend/resume intents are no-ops with
// no suspender wired: no kube command is issued.
func TestSuspendInertWithoutSuspender(t *testing.T) {
	m := cronJobModel(t) // no WithSuspender
	if _, cmd := dispatchRowAction(t, m, rowActionSuspend); cmd != nil {
		t.Fatal("with no suspender wired the suspend intent must not issue a command")
	}
	if _, cmd := dispatchRowAction(t, m, rowActionResume); cmd != nil {
		t.Fatal("with no suspender wired the resume intent must not issue a command")
	}
}

// fakeDrainer is a hermetic Drainer: its DrainStream returns a channel pre-loaded
// with a fixed sequence of events (then closed), and it records the target and the
// ctx so a test can assert the selected Node was drained with the expected options
// and that the drain is cancelled on quit.
type fakeDrainer struct {
	events  []kube.DrainEvent
	calls   int
	gotRes  kube.Resource
	gotRef  kube.ObjectRef
	gotOpts kube.DrainOptions
	ctx     context.Context
}

func (f *fakeDrainer) DrainStream(ctx context.Context, r kube.Resource, node kube.ObjectRef, opts kube.DrainOptions) <-chan kube.DrainEvent {
	f.calls++
	f.ctx = ctx
	f.gotRes = r
	f.gotRef = node
	f.gotOpts = opts
	ch := make(chan kube.DrainEvent, len(f.events))
	for _, ev := range f.events {
		ch <- ev
	}
	close(ch)
	return ch
}

// acceptModal accepts the open confirm modal (enter → nav.drillIn → ConfirmedMsg)
// and delivers the ConfirmedMsg back through Update, returning the resulting model
// and the command the confirmed action issued. It mirrors the delete accept flow.
func acceptModal(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, confirmCmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	if confirmCmd == nil {
		t.Fatal("accepting the modal should emit a confirmed command")
	}
	confirmed, ok := confirmCmd().(modal.ConfirmedMsg)
	if !ok {
		t.Fatalf("accept produced %T, want modal.ConfirmedMsg", confirmCmd())
	}
	next, cmd := m.Update(confirmed)
	return next.(Model), cmd
}

// runDrainToDone drives the drain pump chain to completion by pulling one event at a
// time straight off the model's live channel (bypassing the batched notice timer),
// delivering each drainMsg back through Update until the terminal DrainDoneMsg lands.
// It is the test-side equivalent of Bubble Tea servicing the pump.
func runDrainToDone(t *testing.T, m Model) Model {
	t.Helper()
	for i := 0; ; i++ {
		if i > 200 {
			t.Fatal("drain pump did not terminate")
		}
		cmd := m.pumpDrain(m.drainGen)
		if cmd == nil {
			break // no active drain (already torn down).
		}
		dm, ok := cmd().(drainMsg)
		if !ok {
			t.Fatalf("drain pump produced %T, want drainMsg", cmd())
		}
		next, _ := m.Update(dm)
		m = next.(Model)
		if _, prog := dm.msg.(DrainProgressMsg); !prog {
			break // DrainDoneMsg delivered; the chain ends here.
		}
	}
	return m
}

// TestDrainOpensConfirmModal proves the drain intent opens the confirm modal over the
// selected Node (naming the target) rather than draining outright — no DrainStream yet.
func TestDrainOpensConfirmModal(t *testing.T) {
	d := &fakeDrainer{}
	m := nodeModel(t, WithDrainer(d))
	row, _ := m.table.SelectedRow()

	m, _ = dispatchRowAction(t, m, rowActionDrain)
	if !m.modal.Active() {
		t.Fatal("the drain intent should open the confirm modal")
	}
	if m.modal.Kind() != drainModalKind {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind(), drainModalKind)
	}
	if d.calls != 0 {
		t.Fatal("opening the confirm modal must not start a drain yet")
	}
	if view := m.View().Content; !strings.Contains(view, row.Object.Name) {
		t.Fatalf("the confirm modal should name the target Node %q: %q", row.Object.Name, view)
	}
}

// TestDrainConfirmRunsDrain drives the whole accept path: open the modal, accept
// (enter → nav.drillIn) → the modal closes and DrainStream runs against the selected
// Node with the default options, its progress pumps to the status bar, and the clean
// close surfaces a success notice.
func TestDrainConfirmRunsDrain(t *testing.T) {
	d := &fakeDrainer{events: []kube.DrainEvent{
		{Message: "cordoned node-1; evicting 1 pod(s)"},
		{Message: "evicted ns/web (1/1)"},
		{Message: "removed ns/web (1/1)"},
	}}
	m := nodeModel(t, WithDrainer(d))
	row, _ := m.table.SelectedRow()

	m, _ = dispatchRowAction(t, m, rowActionDrain)
	m, drainCmd := acceptModal(t, m)
	if m.modal.Active() {
		t.Fatal("accepting the modal should hide it")
	}
	if drainCmd == nil {
		t.Fatal("a confirmed drain should start the drain stream")
	}
	if d.calls != 1 {
		t.Fatalf("DrainStream should be called exactly once, got %d", d.calls)
	}
	if d.gotRef.Name != row.Object.Name {
		t.Fatalf("DrainStream addressed %+v, want the selected Node %+v", d.gotRef, row.Object)
	}
	if !d.gotOpts.IgnoreDaemonSets || d.gotOpts.Force || d.gotOpts.DeleteEmptyDirData {
		t.Fatalf("drain options = %+v, want the strict default (IgnoreDaemonSets only)", d.gotOpts)
	}
	if m.drainCh == nil {
		t.Fatal("a started drain should hold a live progress channel")
	}
	m = runDrainToDone(t, m)
	if m.drainCh != nil {
		t.Fatal("a finished drain should tear down its channel")
	}
	if !m.status.HasNotice() || m.status.HasError() {
		t.Fatal("a successful drain should surface a neutral status notice, no error")
	}
}

// TestDrainDeclineDoesNothing proves declining (esc → nav.back) closes the modal
// without starting a drain.
func TestDrainDeclineDoesNothing(t *testing.T) {
	d := &fakeDrainer{}
	m := nodeModel(t, WithDrainer(d))
	m, _ = dispatchRowAction(t, m, rowActionDrain)

	m, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("declining the modal should emit a cancelled command")
	}
	cancelled, ok := cancelCmd().(modal.CancelledMsg)
	if !ok {
		t.Fatalf("decline produced %T, want modal.CancelledMsg", cancelCmd())
	}
	next, _ := m.Update(cancelled)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("declining the modal should hide it")
	}
	if d.calls != 0 {
		t.Fatal("a declined confirm must not start a drain")
	}
}

// TestDrainErrorDegrades proves a drain that fails (a terminal DrainEvent with Err —
// e.g. a blocking pod refusal or RBAC) degrades to a transient status-bar error toast
// (D74) rather than breaking the layout.
func TestDrainErrorDegrades(t *testing.T) {
	d := &fakeDrainer{events: []kube.DrainEvent{
		{Err: errors.New("cannot evict ns/loose (unmanaged)")},
	}}
	m := nodeModel(t, WithDrainer(d))
	m, _ = dispatchRowAction(t, m, rowActionDrain)
	m, _ = acceptModal(t, m)

	m = runDrainToDone(t, m)
	if !m.status.HasError() {
		t.Fatal("a failed drain should surface a status-bar error toast")
	}
}

// TestDrainCancelsOnQuit proves an in-flight drain is cancelled when the app quits
// (cancel-on-quit): the drain's context is done after the quit action runs.
func TestDrainCancelsOnQuit(t *testing.T) {
	d := &fakeDrainer{events: []kube.DrainEvent{{Message: "cordoned node-1; evicting 1 pod(s)"}}}
	m := nodeModel(t, WithDrainer(d))
	m, _ = dispatchRowAction(t, m, rowActionDrain)
	m, _ = acceptModal(t, m)
	if d.ctx == nil {
		t.Fatal("the drain should have been started with a context")
	}
	if d.ctx.Err() != nil {
		t.Fatal("the drain context should be live before quit")
	}

	// Quit tears down the in-flight drain via stopDrain (cancel-on-quit); the cancel
	// acts on the shared cancel func the model holds, so the side effect lands on the
	// drainer's stored context regardless of the returned model copy.
	_, _ = m.handleAction(keymap.ActionQuit)
	if d.ctx.Err() == nil {
		t.Fatal("quitting should cancel the in-flight drain's context")
	}
}

// TestDrainInertWithoutDrainer proves the drain intent is a no-op with no drainer
// wired: no confirm modal opens and no command is issued.
func TestDrainInertWithoutDrainer(t *testing.T) {
	m := nodeModel(t) // no WithDrainer
	next, cmd := dispatchRowAction(t, m, rowActionDrain)
	if next.modal.Active() {
		t.Fatal("with no drainer wired the drain intent must not open a modal")
	}
	if cmd != nil {
		t.Fatal("with no drainer wired the drain intent must not issue a command")
	}
}

// signalDeleter is a fakeDeleter that also announces on a channel the moment Delete
// is invoked, so a full-program (teatest) test can block on the delete actually
// running before it inspects state — the deterministic barrier the accept flow needs.
// The confirm accept resolves through an async round-trip (KeyMsg → ConfirmedMsg cmd →
// runDelete cmd → Delete), so a plain Send(enter)+Quit would race the Quit ahead of
// the delete; receiving on `called` guarantees the whole chain ran first. The field
// writes precede the channel send, and the test reads them only after receiving, so
// the receive's happens-before makes the read race-free (go test -race clean).
type signalDeleter struct {
	err    error
	calls  int
	gotRes kube.Resource
	gotRef kube.ObjectRef
	called chan struct{}
}

func (s *signalDeleter) Delete(_ context.Context, r kube.Resource, ref kube.ObjectRef, _ metav1.DeleteOptions) error {
	s.calls++
	s.gotRes = r
	s.gotRef = ref
	if s.called != nil {
		s.called <- struct{}{}
	}
	return s.err
}

// TestProgramModalConfirmAcceptRunsDelete is the M2-14b full-program (teatest/v2, the
// M0-05 harness) coverage of the confirm-modal accept flow M3-09 wired (D115), driving
// the whole chain through the running bubbletea program rather than hand-threaded
// Update calls: a live `x` over the selected row opens the confirm modal, a nav key is
// swallowed by the open modal (input capture), and `enter` (nav.drillIn) accepts —
// ConfirmedMsg closes the modal and runs kube.Delete against the selected row. The
// signalDeleter is the sync barrier: receiving on `called` proves the async
// KeyMsg→ConfirmedMsg→runDelete→Delete chain fully executed before the program quits,
// so the final-model assertions are deterministic (the modal is hidden in
// handleModalConfirmed strictly before the delete cmd fires). Proves the modal opens,
// captures input, and resolves — end to end through the program.
func TestProgramModalConfirmAcceptRunsDelete(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	d := &signalDeleter{called: make(chan struct{}, 1)}
	tm := teatest.NewTestModel(t, New(WithWatcher(fw), WithDeleter(d)), teatest.WithInitialTermSize(80, 24))

	// Browse layout draws first (a seed kind in the left pane).
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	// Drill into pods; the preloaded RESET populates the table so an unselected row
	// ("pod-a"; the selected "pod-b" is background-filled and a byte scan misses it)
	// renders — the same reason the other program tests key on an unselected row.
	tm.Send(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("pod-a"))
	}, teatest.WithDuration(3*time.Second))

	// Live `x` opens the confirm modal over the selected row (pod-b). The modal's
	// message line is foreground-only styled (styles.App), so a plain byte scan sees
	// it — unlike the background-filled status bar. Waiting on it is the barrier that
	// the `x`→rowActionMsg→openDeleteConfirm chain resolved before we send more keys.
	tm.Send(tea.KeyPressMsg(deleteKey))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Delete Pod pod-b?"))
	}, teatest.WithDuration(3*time.Second))

	// A nav key is swallowed by the open modal (input capture); then enter accepts.
	tm.Send(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	tm.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	// Block until the delete actually runs — the deterministic barrier before Quit.
	select {
	case <-d.called:
	case <-time.After(3 * time.Second):
		t.Fatal("accepting the confirm modal should run the delete through the program")
	}
	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	if d.calls != 1 {
		t.Fatalf("Delete should run exactly once through the program, got %d", d.calls)
	}
	// The delete addressed the selected row, its UID riding through for the snapshot
	// guard (M1-06a/D35) — not whatever a swallowed nav might have moved to.
	if d.gotRef.Name != "pod-b" || d.gotRef.UID != "b" {
		t.Fatalf("Delete addressed %+v, want the selected row pod-b (uid b)", d.gotRef)
	}
	fm, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model is %T, want Model", tm.FinalModel(t))
	}
	if fm.modal.Active() {
		t.Fatal("accepting the confirm modal should have hidden it")
	}
	if row, _ := fm.table.SelectedRow(); row.Object.Name != "pod-b" {
		t.Fatalf("the swallowed nav must not move the selection: selected %q, want pod-b", row.Object.Name)
	}
}

// TestProgramModalConfirmDeclineNoDelete is the M2-14b full-program (teatest/v2)
// coverage of the confirm-modal decline flow: a live `x` opens the modal, a nav key is
// swallowed (capture), and `esc` (nav.back) declines — no kube.Delete runs. The
// deterministic assertions are that no delete ever fired (decline never deletes,
// regardless of the async CancelledMsg/Quit interleave) and that the swallowed nav left
// the selection put. The modal's async close on CancelledMsg races the trailing Quit so
// it is not asserted here on the final model; that close is proven deterministically by
// the accept test above (modal hidden after accept) and by the direct-Update
// TestDeleteConfirmDeclineDoesNothing. A deleter is wired so a decline that wrongly ran
// the delete would be caught.
func TestProgramModalConfirmDeclineNoDelete(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	d := &signalDeleter{called: make(chan struct{}, 1)}
	tm := teatest.NewTestModel(t, New(WithWatcher(fw), WithDeleter(d)), teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("pod-a"))
	}, teatest.WithDuration(3*time.Second))

	// Open the confirm modal, wait for it (barrier), swallow a nav, then decline.
	tm.Send(tea.KeyPressMsg(deleteKey))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Delete Pod pod-b?"))
	}, teatest.WithDuration(3*time.Second))
	tm.Send(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	tm.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))

	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	if d.calls != 0 {
		t.Fatalf("declining the confirm modal must not delete anything, got %d calls", d.calls)
	}
	fm, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model is %T, want Model", tm.FinalModel(t))
	}
	if row, _ := fm.table.SelectedRow(); row.Object.Name != "pod-b" {
		t.Fatalf("the swallowed nav must not move the selection: selected %q, want pod-b", row.Object.Name)
	}
}

// fakeForward is a hermetic ActiveForward: its Ready/Done channels are driven by the
// test, Ports returns preset bound pairs, and Stop is counted. It stands in for
// *kube.PortForward so the M3-13a flow is exercised without a cluster (D18).
type fakeForward struct {
	readyCh chan struct{}
	doneCh  chan struct{}
	ports   []kube.ForwardedPort
	err     error
	stops   int
}

func newFakeForward() *fakeForward {
	return &fakeForward{readyCh: make(chan struct{}), doneCh: make(chan struct{})}
}

func (f *fakeForward) Ready() <-chan struct{}              { return f.readyCh }
func (f *fakeForward) Done() <-chan struct{}               { return f.doneCh }
func (f *fakeForward) Err() error                          { return f.err }
func (f *fakeForward) Ports() ([]kube.ForwardedPort, error) { return f.ports, nil }
func (f *fakeForward) Stop()                               { f.stops++ }

// fakePortForwarder is a hermetic PortForwarder: it records the ctx/ref/ports it was
// asked to forward (so a test can assert the selected row and the parsed specs) and
// returns a preset handle or error.
type fakePortForwarder struct {
	err      error
	calls    int
	gotCtx   context.Context
	gotRef   kube.ObjectRef
	gotPorts []string
	handle   *fakeForward
}

func (f *fakePortForwarder) PortForward(ctx context.Context, ref kube.ObjectRef, ports []string) (ActiveForward, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotRef = ref
	f.gotPorts = ports
	if f.err != nil {
		return nil, f.err
	}
	return f.handle, nil
}

// TestPortForwardOpensPrompt proves the port-forward intent opens the ports prompt
// over the selected Pod (naming the target) rather than forwarding outright.
func TestPortForwardOpensPrompt(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	row, _ := m.table.SelectedRow()

	m, _ = dispatchRowAction(t, m, rowActionPortForward)
	if !m.modal.Active() || !m.modal.Prompting() {
		t.Fatal("the port-forward intent should open the modal in prompt mode")
	}
	if m.modal.Kind() != portForwardModalKind {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind(), portForwardModalKind)
	}
	if pf.calls != 0 {
		t.Fatal("opening the prompt must not start a forward yet")
	}
	if view := m.View().Content; !strings.Contains(view, row.Object.Name) {
		t.Fatalf("the port-forward prompt should name the target row %q: %q", row.Object.Name, view)
	}
}

// TestPortForwardLifecycle drives start → ready → done: submitting the ports starts a
// tracked forward addressed to the selected Pod with the parsed specs; a Ready fills
// its bound ports; a Done removes it.
func TestPortForwardLifecycle(t *testing.T) {
	fw := newFakeForward()
	fw.ports = []kube.ForwardedPort{{Local: 8080, Remote: 80}}
	pf := &fakePortForwarder{handle: fw}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	row, _ := m.table.SelectedRow()
	m, _ = dispatchRowAction(t, m, rowActionPortForward)

	next, cmd := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "8080:80"})
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("submitting the prompt should hide the modal")
	}
	if cmd == nil {
		t.Fatal("a submitted port-forward should issue the wait command")
	}
	if pf.calls != 1 {
		t.Fatalf("PortForward should be called once, got %d", pf.calls)
	}
	if pf.gotRef.Name != row.Object.Name {
		t.Fatalf("PortForward addressed %q, want the selected row %q", pf.gotRef.Name, row.Object.Name)
	}
	if len(pf.gotPorts) != 1 || pf.gotPorts[0] != "8080:80" {
		t.Fatalf("PortForward ports = %v, want [8080:80]", pf.gotPorts)
	}
	if len(m.forwards) != 1 {
		t.Fatalf("the started forward should be tracked, got %d", len(m.forwards))
	}
	id := m.forwards[0].id

	// Ready fills the bound ports and re-arms the Done wait.
	next, readyCmd := m.Update(forwardReadyMsg{id: id})
	m = next.(Model)
	if !m.forwards[0].ready {
		t.Fatal("Ready should mark the forward ready")
	}
	if got := m.forwards[0].bound; len(got) != 1 || got[0].Local != 8080 || got[0].Remote != 80 {
		t.Fatalf("Ready should fill the bound ports, got %v", got)
	}
	if readyCmd == nil {
		t.Fatal("Ready should re-arm the Done wait")
	}

	// Done removes the forward and reports it stopped.
	next, _ = m.Update(forwardDoneMsg{id: id})
	m = next.(Model)
	if len(m.forwards) != 0 {
		t.Fatalf("Done should drop the ended forward, got %d", len(m.forwards))
	}
}

// TestPortForwardBlankPromptDegrades proves a blank ports entry surfaces an error and
// starts nothing.
func TestPortForwardBlankPromptDegrades(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	m, _ = dispatchRowAction(t, m, rowActionPortForward)

	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "   "})
	m = next.(Model)
	if pf.calls != 0 {
		t.Fatal("a blank ports entry must not start a forward")
	}
	if len(m.forwards) != 0 {
		t.Fatal("a blank ports entry must track no forward")
	}
	if !m.status.HasError() {
		t.Fatal("a blank ports entry should surface a status-bar error")
	}
}

// TestPortForwardStartErrorDegrades proves a start error (a malformed spec kube
// rejects, or a transport failure) degrades to a toast and tracks no forward.
func TestPortForwardStartErrorDegrades(t *testing.T) {
	pf := &fakePortForwarder{err: errors.New("bad port spec")}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	m, _ = dispatchRowAction(t, m, rowActionPortForward)

	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "not-a-port"})
	m = next.(Model)
	if pf.calls != 1 {
		t.Fatalf("PortForward should be attempted once, got %d", pf.calls)
	}
	if len(m.forwards) != 0 {
		t.Fatal("a failed start must track no forward")
	}
	if !m.status.HasError() {
		t.Fatal("a failed start should surface a status-bar error")
	}
}

// TestPortForwardBindFailureDegrades proves a local-listener bind failure (the local
// port already in use) drops the forward and surfaces a status-bar error rather than
// leaving a phantom entry — the raw client-go error is replaced by an actionable hint
// (feedback 2026-07-24; the hint text itself is covered by TestPortForwardBindHint).
func TestPortForwardBindFailureDegrades(t *testing.T) {
	fw := newFakeForward()
	pf := &fakePortForwarder{handle: fw}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	m, _ = dispatchRowAction(t, m, rowActionPortForward)

	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "6379"})
	m = next.(Model)
	if len(m.forwards) != 1 {
		t.Fatalf("the started forward should be tracked, got %d", len(m.forwards))
	}
	id := m.forwards[0].id

	// The forward dies before ever becoming ready with client-go's bind-failure error.
	bindErr := errors.New("unable to listen on any of the requested ports: [{6379 6379}]")
	next, _ = m.Update(forwardDoneMsg{id: id, err: bindErr})
	m = next.(Model)
	if len(m.forwards) != 0 {
		t.Fatalf("a bind failure should drop the forward, got %d", len(m.forwards))
	}
	if !m.status.HasError() {
		t.Fatal("a bind failure should surface a status-bar error")
	}
}

// TestPortForwardBindErrDetection pins the sentinel-substring match that separates a
// local-listener bind failure from any other transport error.
func TestPortForwardBindErrDetection(t *testing.T) {
	if !isPortForwardBindErr(errors.New("unable to listen on any of the requested ports: [{6379 6379}]")) {
		t.Fatal("client-go's bind-failure error should be recognised")
	}
	if isPortForwardBindErr(errors.New("dial tcp: connection refused")) {
		t.Fatal("an unrelated transport error must not be treated as a bind failure")
	}
	if isPortForwardBindErr(nil) {
		t.Fatal("a nil error is not a bind failure")
	}
}

// TestPortForwardBindHint pins the actionable retry message built from the requested
// specs: it names the clashing local port(s) and shows the ":<remote>" escape — never
// ":0", which client-go rejects as remote port 0 (D139).
func TestPortForwardBindHint(t *testing.T) {
	cases := []struct {
		name  string
		specs []string
		want  []string // substrings that must all be present
	}{
		{"same-port", []string{"6379"}, []string{"local port 6379 already in use", "retry with :6379"}},
		{"explicit-local", []string{"8080:80"}, []string{"local port 8080 already in use", "retry with :80"}},
		{"multiple", []string{"8080:80", "6379"}, []string{"local ports 8080, 6379 already in use", "retry with :80"}},
		{"already-auto", []string{":80"}, []string{"could not bind the local listener"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := portForwardBindHint(tc.specs)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("hint %q missing %q", got, want)
				}
			}
			if strings.Contains(got, ":0 ") || strings.HasSuffix(got, ":0") {
				t.Fatalf("the hint must not suggest :0 (client-go rejects remote port 0): %q", got)
			}
		})
	}
}

// TestPortForwardInertWithoutForwarder proves the port-forward intent is a no-op with
// no forwarder wired: no prompt opens and no command is issued.
func TestPortForwardInertWithoutForwarder(t *testing.T) {
	m := openPodTable(t, "Pod") // no WithPortForwarder
	next, cmd := dispatchRowAction(t, m, rowActionPortForward)
	if next.modal.Active() {
		t.Fatal("with no forwarder wired the port-forward intent must not open a modal")
	}
	if cmd != nil {
		t.Fatal("with no forwarder wired the port-forward intent must not issue a command")
	}
}

// fakeServiceResolver is a hermetic ServiceResolver (M3-13c): it returns a preset
// backing-pod ref (or an error) and records the Service ref it was asked to resolve,
// so a test can drive the Service→pod port-forward hop without a cluster.
type fakeServiceResolver struct {
	pod    kube.ObjectRef
	err    error
	calls  int
	gotRef kube.ObjectRef
}

func (f *fakeServiceResolver) PodForService(_ context.Context, ref kube.ObjectRef) (kube.ObjectRef, error) {
	f.calls++
	f.gotRef = ref
	if f.err != nil {
		return kube.ObjectRef{}, f.err
	}
	return f.pod, nil
}

// TestPortForwardServiceResolvesPod proves M3-13c's happy path: port-forwarding a
// Service first resolves it to a backing endpoint pod (the resolver asked for the
// Service row), then the ports prompt opens over — and the started forward targets —
// the *resolved pod*, not the Service.
func TestPortForwardServiceResolvesPod(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	sr := &fakeServiceResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz", UID: "uid-1"}}
	m := openPodTable(t, "Service", WithPortForwarder(pf), WithServiceResolver(sr))
	row, _ := m.table.SelectedRow()

	// The action resolves off the update loop; no prompt yet.
	m, resolveCmd := dispatchRowAction(t, m, rowActionPortForward)
	if m.modal.Active() {
		t.Fatal("a Service port-forward must not open the prompt until its backing pod resolves")
	}
	if resolveCmd == nil {
		t.Fatal("a Service port-forward should issue the async resolution command")
	}
	msg, ok := resolveCmd().(serviceResolvedMsg)
	if !ok {
		t.Fatalf("resolution command should produce a serviceResolvedMsg, got %T", resolveCmd())
	}
	if sr.calls != 1 || sr.gotRef.Name != row.Object.Name {
		t.Fatalf("resolver should be asked for the Service row %q once, got calls=%d ref=%q", row.Object.Name, sr.calls, sr.gotRef.Name)
	}

	// Delivering the resolution opens the prompt over the resolved pod.
	next, _ := m.Update(msg)
	m = next.(Model)
	if !m.modal.Active() || !m.modal.Prompting() || m.modal.Kind() != portForwardModalKind {
		t.Fatal("the resolved Service should open the port-forward ports prompt")
	}
	if view := m.View().Content; !strings.Contains(view, "api-xyz") {
		t.Fatalf("the prompt should name the resolved pod api-xyz: %q", view)
	}

	// Submitting starts the forward against the resolved pod.
	next, _ = m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "8080:80"})
	m = next.(Model)
	if pf.calls != 1 || pf.gotRef.Name != "api-xyz" {
		t.Fatalf("the forward should target the resolved pod api-xyz, got calls=%d ref=%q", pf.calls, pf.gotRef.Name)
	}
	if len(m.forwards) != 1 {
		t.Fatalf("the started forward should be tracked, got %d", len(m.forwards))
	}
}

// TestPortForwardServiceResolveErrorDegrades proves a resolution failure (a
// selector-less Service, no ready pod, RBAC denial) degrades to a status-bar toast and
// opens no prompt.
func TestPortForwardServiceResolveErrorDegrades(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	sr := &fakeServiceResolver{err: errors.New("no pods found for service")}
	m := openPodTable(t, "Service", WithPortForwarder(pf), WithServiceResolver(sr))

	m, resolveCmd := dispatchRowAction(t, m, rowActionPortForward)
	next, _ := m.Update(resolveCmd().(serviceResolvedMsg))
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("a failed Service resolution must not open the prompt")
	}
	if !m.status.HasError() {
		t.Fatal("a failed Service resolution should surface a status-bar error")
	}
	if pf.calls != 0 {
		t.Fatal("a failed Service resolution must start no forward")
	}
}

// TestPortForwardServiceInertWithoutResolver proves that with a forwarder but no
// service resolver wired, port-forwarding a Service degrades to a toast (not a silent
// no-op) and opens no prompt — a Pod still forwards directly.
func TestPortForwardServiceInertWithoutResolver(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := openPodTable(t, "Service", WithPortForwarder(pf)) // no WithServiceResolver
	m, _ = dispatchRowAction(t, m, rowActionPortForward)
	if m.modal.Active() {
		t.Fatal("with no service resolver wired a Service port-forward must not open a prompt")
	}
	if !m.status.HasError() {
		t.Fatal("with no service resolver wired a Service port-forward should surface a status-bar error")
	}
	if pf.calls != 0 {
		t.Fatal("with no service resolver wired a Service port-forward must start no forward")
	}
}

// TestPortForwardServiceStaleResolutionDropped proves the generation guard: a resolution
// that lands after a newer port-forward request superseded it is dropped, opening no
// prompt.
func TestPortForwardServiceStaleResolutionDropped(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	sr := &fakeServiceResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz"}}
	m := openPodTable(t, "Service", WithPortForwarder(pf), WithServiceResolver(sr))

	m, resolveCmd := dispatchRowAction(t, m, rowActionPortForward)
	stale := resolveCmd().(serviceResolvedMsg)
	m.pfResolveGen++ // a newer request supersedes the in-flight resolution
	next, _ := m.Update(stale)
	if next.(Model).modal.Active() {
		t.Fatal("a superseded Service resolution should be dropped, opening no prompt")
	}
}

// TestPortForwardStopsOnQuit proves every running forward is torn down when the app
// quits (cancel-on-exit): the forward's context is done and the set is cleared.
func TestPortForwardStopsOnQuit(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	m, _ = dispatchRowAction(t, m, rowActionPortForward)
	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "80"})
	m = next.(Model)
	if pf.gotCtx == nil {
		t.Fatal("the forward should have been started with a context")
	}
	if pf.gotCtx.Err() != nil {
		t.Fatal("the forward context should be live before quit")
	}

	// Quit tears down every forward via stopForwards (cancel-on-exit); the cancel acts
	// on the context the forwarder captured regardless of the returned model copy.
	quit, _ := m.handleAction(keymap.ActionQuit)
	if pf.gotCtx.Err() == nil {
		t.Fatal("quitting should cancel the running forward's context")
	}
	if len(quit.(Model).forwards) != 0 {
		t.Fatal("quitting should clear the tracked forwards")
	}
}

// startForwardOnPod drills into a Pod, submits the ports prompt, and returns the model
// with one tracked (not-yet-ready) forward — the fixture the M3-13b panel tests build on.
func startForwardOnPod(t *testing.T, pf *fakePortForwarder, value string) Model {
	t.Helper()
	m := openPodTable(t, "Pod", WithPortForwarder(pf))
	m, _ = dispatchRowAction(t, m, rowActionPortForward)
	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: value})
	return next.(Model)
}

// TestForwardsPanelToggle proves forwards.panel (`F`) opens the panel (an active
// overlay) and forwards.panel/nav.back both close it.
func TestForwardsPanelToggle(t *testing.T) {
	m := openPodTable(t, "Pod")
	if m.forwardsPanel {
		t.Fatal("the port-forward panel should start closed")
	}
	next, _ := m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	if !m.forwardsPanel {
		t.Fatal("forwards.panel should open the panel")
	}
	if !m.overlayActive() {
		t.Fatal("the open panel should count as an active overlay (swallows browse input)")
	}
	// forwards.panel again toggles it closed.
	next, _ = m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	if m.forwardsPanel {
		t.Fatal("forwards.panel should toggle the panel closed")
	}
	// nav.back also closes it.
	next, _ = m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	next, _ = m.handleAction(keymap.ActionBack)
	m = next.(Model)
	if m.forwardsPanel {
		t.Fatal("nav.back should close the panel")
	}
}

// TestForwardsPanelLists proves the panel renders each active forward with its bound
// ports and ready state, and shows an empty-state line when none are active.
func TestForwardsPanelLists(t *testing.T) {
	// Empty state first.
	m := openPodTable(t, "Pod")
	next, _ := m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	if view := m.View().Content; !strings.Contains(view, "No active port-forwards") {
		t.Fatalf("the empty panel should show the empty-state line: %q", view)
	}

	// With a ready forward the panel lists its label + bound ports + "ready".
	fw := newFakeForward()
	fw.ports = []kube.ForwardedPort{{Local: 8080, Remote: 80}}
	pf := &fakePortForwarder{handle: fw}
	m = startForwardOnPod(t, pf, "8080:80")
	row := m.forwards[0]
	next, _ = m.Update(forwardReadyMsg{id: row.id})
	m = next.(Model)
	next, _ = m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	view := m.View().Content
	if !strings.Contains(view, "localhost:8080 → 80") {
		t.Fatalf("the panel should list the bound ports: %q", view)
	}
	if !strings.Contains(view, "ready") {
		t.Fatalf("a ready forward should read as ready in the panel: %q", view)
	}
	if !strings.Contains(view, row.label) {
		t.Fatalf("the panel should name the forward target %q: %q", row.label, view)
	}
}

// TestForwardsPanelStopSelected proves nav.drillIn on the selected forward cancels its
// context; the forwardDoneMsg that follows removes it from the set.
func TestForwardsPanelStopSelected(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := startForwardOnPod(t, pf, "80")
	id := m.forwards[0].id
	next, _ := m.handleAction(keymap.ActionForwards)
	m = next.(Model)

	next, _ = m.handleAction(keymap.ActionDrillIn)
	m = next.(Model)
	if pf.gotCtx.Err() == nil {
		t.Fatal("nav.drillIn should cancel the selected forward's context")
	}
	if len(m.forwards) != 1 {
		t.Fatal("the entry stays listed until its Done lands")
	}
	// The Done that the cancellation triggers removes the entry.
	next, _ = m.Update(forwardDoneMsg{id: id})
	m = next.(Model)
	if len(m.forwards) != 0 {
		t.Fatalf("the stopped forward should be dropped, got %d", len(m.forwards))
	}
}

// TestForwardsPanelStopAll proves forwards.stopAll cancels every forward and clears the
// set immediately, reporting the sweep, and is inert with nothing active.
func TestForwardsPanelStopAll(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := startForwardOnPod(t, pf, "80")
	next, _ := m.handleAction(keymap.ActionForwards)
	m = next.(Model)

	next, cmd := m.handleAction(keymap.ActionStopForwards)
	m = next.(Model)
	if pf.gotCtx.Err() == nil {
		t.Fatal("forwards.stopAll should cancel the forward's context")
	}
	if len(m.forwards) != 0 {
		t.Fatalf("forwards.stopAll should clear the set, got %d", len(m.forwards))
	}
	if cmd == nil {
		t.Fatal("forwards.stopAll should report the sweep with a notice")
	}
	// Inert with nothing active (no notice, no panic).
	if _, cmd := m.handleAction(keymap.ActionStopForwards); cmd != nil {
		t.Fatal("forwards.stopAll should be inert with no forwards")
	}
}

// TestForwardsPanelCursorMoves proves nav.up/down move the panel cursor over the
// forwards and clamp at both ends.
func TestForwardsPanelCursorMoves(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := startForwardOnPod(t, pf, "80")
	m = startForwardOnPod2(t, m, pf, "81") // a second forward on the same pod
	if len(m.forwards) != 2 {
		t.Fatalf("expected two tracked forwards, got %d", len(m.forwards))
	}
	next, _ := m.handleAction(keymap.ActionForwards)
	m = next.(Model)
	if m.forwardsSel != 0 {
		t.Fatalf("the cursor should start at 0, got %d", m.forwardsSel)
	}
	next, _ = m.handleAction(keymap.ActionDown)
	m = next.(Model)
	if m.forwardsSel != 1 {
		t.Fatalf("nav.down should move the cursor to 1, got %d", m.forwardsSel)
	}
	next, _ = m.handleAction(keymap.ActionDown) // clamps at the last entry
	m = next.(Model)
	if m.forwardsSel != 1 {
		t.Fatalf("nav.down should clamp at the last entry, got %d", m.forwardsSel)
	}
	next, _ = m.handleAction(keymap.ActionUp)
	m = next.(Model)
	next, _ = m.handleAction(keymap.ActionUp) // clamps at 0
	m = next.(Model)
	if m.forwardsSel != 0 {
		t.Fatalf("nav.up should clamp at 0, got %d", m.forwardsSel)
	}
}

// startForwardOnPod2 starts a second forward on the already-open Pod table model,
// reusing the row action + prompt path (openPodTable is not re-run so the first
// forward is preserved).
func startForwardOnPod2(t *testing.T, m Model, pf *fakePortForwarder, value string) Model {
	t.Helper()
	m, _ = dispatchRowAction(t, m, rowActionPortForward)
	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: value})
	return next.(Model)
}
