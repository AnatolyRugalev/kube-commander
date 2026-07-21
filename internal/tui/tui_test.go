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
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
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
	if got := m.nsPicker.Len(); got != 2 {
		t.Fatalf("picker seeded with %d namespaces, want 2", got)
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
	// nav.down while the picker is open moves the picker cursor, not the menu.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 0 {
		t.Fatalf("picker should capture nav.down; menu moved to %d", m.menu.Cursor())
	}
	if v, _ := m.nsPicker.Selected(); v != "kube-system" {
		t.Fatalf("nav.down should move the picker cursor to kube-system, got %q", v)
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
	m = typeStr(t, m, "web")               // 2 matches: web-1, web-2
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
