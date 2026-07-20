package tui

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
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
	chans []chan kube.WatchEvent // one per call, in order; a fresh one each Watch
	err   error
	ctxs  []context.Context
	res   []kube.Resource
	ns    []string
}

func (f *fakeWatcher) Watch(ctx context.Context, r kube.Resource, ns string, _ metav1.ListOptions) (<-chan kube.WatchEvent, error) {
	f.ctxs = append(f.ctxs, ctx)
	f.res = append(f.res, r)
	f.ns = append(f.ns, ns)
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan kube.WatchEvent, 1)
	f.chans = append(f.chans, ch)
	return ch, nil
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
	if _, ok := cmd().(ErrorMsg); !ok {
		t.Fatalf("expected ErrorMsg, got %T", cmd())
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
