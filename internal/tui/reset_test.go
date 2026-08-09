package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// armAsync fills the model's per-cluster async slots with live, cancellable work —
// a watch, a discovery pass, a log stream, a search fan-out, a drain and a running
// port-forward — and returns the contexts behind them so a test can assert each was
// cancelled. It sets the fields directly rather than driving six features through
// their own gestures: every one of those paths is already covered by its own test,
// and what this leg needs to pin is that the *teardown* reaches all of them at once.
func armAsync(t *testing.T, m *Model) []context.Context {
	t.Helper()
	var ctxs []context.Context
	arm := func(assign func(context.Context, context.CancelFunc)) {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs = append(ctxs, ctx)
		assign(ctx, cancel)
	}
	arm(func(_ context.Context, c context.CancelFunc) {
		m.watchCancel, m.watchCh = c, make(chan kube.WatchEvent)
	})
	arm(func(_ context.Context, c context.CancelFunc) { m.discoveryCancel = c })
	arm(func(_ context.Context, c context.CancelFunc) {
		m.logCancel, m.logCh = c, make(chan kube.LogEvent)
	})
	arm(func(_ context.Context, c context.CancelFunc) {
		m.searchCancel, m.searchCh = c, make(chan kube.SearchEvent)
	})
	arm(func(_ context.Context, c context.CancelFunc) {
		m.drainCancel, m.drainCh = c, make(chan kube.DrainEvent)
	})
	return ctxs
}

// TestStopClusterAsyncCancelsEveryPerClusterAsync proves the one inventory both quit
// and a context switch rely on reaches every asynchronous operation bound to the
// cluster: each context is cancelled, each handle cleared so nothing further is
// pumped, and every running port-forward stopped.
func TestStopClusterAsyncCancelsEveryPerClusterAsync(t *testing.T) {
	m := sized(t)
	ctxs := armAsync(t, &m)
	fwd := newFakeForward()
	stopped := false
	m.forwards = []*forward{{id: 1, handle: fwd, cancel: func() { stopped = true }}}

	m.stopClusterAsync()

	for i, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Errorf("per-cluster async #%d was not cancelled", i)
		}
	}
	if m.watchCh != nil || m.logCh != nil || m.searchCh != nil || m.drainCh != nil {
		t.Error("a cancelled stream's channel must be cleared so nothing further is pumped")
	}
	if len(m.forwards) != 0 || !stopped {
		t.Errorf("every port-forward should be stopped: %d left, cancelled=%v", len(m.forwards), stopped)
	}
}

// TestStopClusterAsyncLeavesNoCancelBehind walks the model's context.CancelFunc
// fields by reflection and fails on any left set. The failure it guards is the quiet
// one: a future leg adds a per-cluster async with its own cancel, wires it in its own
// test, and forgets this inventory — nothing breaks until a context switch, when the
// operation keeps running against the cluster the user just left. Reflection rather
// than one assertion per field so a *new* cancel is covered without anyone
// remembering to extend the test.
func TestStopClusterAsyncLeavesNoCancelBehind(t *testing.T) {
	m := sized(t)
	armAsync(t, &m)
	m.stopClusterAsync()

	v := reflect.ValueOf(m)
	cancelType := reflect.TypeOf(context.CancelFunc(nil))
	checked := 0
	for i := range v.NumField() {
		if v.Type().Field(i).Type != cancelType {
			continue
		}
		checked++
		if !v.Field(i).IsNil() {
			t.Errorf("%s is still set after stopClusterAsync", v.Type().Field(i).Name)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d cancel funcs found on the model; the reflection walk is not covering them", checked)
	}
}

// TestQuitTearsDownEveryPerClusterAsync proves app.quit goes through the same
// inventory a switch does, so the two cannot drift apart (the per-feature
// cancel-on-quit tests each cover one; this covers all of them at once).
func TestQuitTearsDownEveryPerClusterAsync(t *testing.T) {
	m := sized(t)
	ctxs := armAsync(t, &m)

	next, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("app.quit should issue tea.Quit")
	}
	m = next
	for i, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Errorf("quitting left per-cluster async #%d running", i)
		}
	}
}

// browsingModel drills into a resource with a live watch, applies a row, and leaves
// the table filtered, sorted and namespace-scoped — the "deep in the old cluster"
// state a context switch has to unwind.
func browsingModel(t *testing.T, fw *fakeWatcher, opts ...Option) Model {
	t.Helper()
	m := sizedWith(t, append([]Option{WithWatcher(fw), WithNamespace("kube-system")}, opts...)...)
	next, _ := m.selectResource(gvrResource("pods"))
	m = next.(Model)
	m.table.ApplyEvent(resetEvent("api-1", "uid-1"))
	m.table.SetFilter("api")
	m.table.SortBy(0)
	return m
}

// TestResetClusterReturnsBrowsePanesToPreDrillInState is the leg's headline: after a
// reset the shell looks exactly as it does before the user has drilled into anything
// — seed menu focused, no live resource, an empty table with no filter and no sort,
// no namespace scope — so the cluster M4-04 swaps in is browsed from a clean slate
// rather than through the previous one's leftovers.
func TestResetClusterReturnsBrowsePanesToPreDrillInState(t *testing.T) {
	fw := &fakeWatcher{}
	m := browsingModel(t, fw)
	seed := len(sized(t).menu.Items())

	// A discovered CRD is part of the *old* cluster's API surface, so it must not
	// survive either.
	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	m.menu.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{crd}})
	if len(m.menu.Items()) != seed+1 {
		t.Fatalf("setup: discovery should have appended the CRD (%d items, want %d)", len(m.menu.Items()), seed+1)
	}

	m.resetCluster()

	if m.hasCurrent {
		t.Error("reset should leave no resource being browsed (the welcome page shows)")
	}
	if m.table.TotalRowCount() != 0 {
		t.Errorf("reset should blank the table, %d rows left", m.table.TotalRowCount())
	}
	if m.table.Filter() != "" {
		t.Errorf("reset should clear the table filter, got %q", m.table.Filter())
	}
	if _, sorted := m.table.SortColumn(); sorted {
		t.Error("reset should clear the sort — the columns come from the old cluster's table")
	}
	if m.namespace != "" {
		t.Errorf("reset should clear the namespace scope, got %q", m.namespace)
	}
	if !m.menu.Focused() || m.table.Focused() {
		t.Error("reset should return focus to the menu (nothing is drilled into)")
	}
	if got := len(m.menu.Items()); got != seed {
		t.Errorf("reset should return the menu to its seed: %d items, want %d", got, seed)
	}
	if got, want := m.browseBody(), sized(t).browseBody(); got != want {
		t.Errorf("the browse panes should render exactly as before any drill-in:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// TestResetClusterDropsStaleWatchAndDiscoveryMessages proves the generation bumps do
// their job: a delta and a discovery result already in flight when the reset happened
// belong to the cluster being left, and applying either would put the old cluster's
// rows in the new context's table or its API surface in the new context's menu.
func TestResetClusterDropsStaleWatchAndDiscoveryMessages(t *testing.T) {
	fw := &fakeWatcher{}
	m := browsingModel(t, fw)
	stale := m.watchGen
	seed := len(sized(t).menu.Items())

	m.resetCluster()

	next, cmd := m.Update(watchMsg{gen: stale, msg: ResourceEventMsg{Event: resetEvent("api-1", "uid-1")}})
	m = next.(Model)
	if m.table.TotalRowCount() != 0 {
		t.Error("a delta from the cluster that was left must not populate the new table")
	}
	if cmd != nil {
		t.Error("a stale delta must not re-issue the pump")
	}

	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	next, _ = m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{crd}}})
	m = next.(Model)
	if got := len(m.menu.Items()); got != seed {
		t.Errorf("a discovery result from the cluster that was left must not reconcile: %d items, want %d", got, seed)
	}
}

// TestResetClusterDismissesSurfacesAndStashes proves nothing showing (or pointing at)
// the departing cluster's data survives: the full-screen views, the overlays, the
// forwards panel, and the object stashes an open modal or picker left behind.
func TestResetClusterDismissesSurfacesAndStashes(t *testing.T) {
	fw := &fakeWatcher{}
	m := browsingModel(t, fw)
	m.searchView.Show()
	m.logsView.Show()
	m.viewer.SetTitle("Pod web/api-1")
	m.viewer.SetContent("spec:")
	m.viewer.Show()
	m.modal.ShowConfirm(deleteModalKind, "Delete", "delete api-1?")
	m.ctrPicker.Show()
	m.portPicker.Show()
	m.cmdPicker.Show()
	m.pfPanel.Open()
	m.deleteRef = kube.ObjectRef{Namespace: "web", Name: "api-1", UID: "uid-1"}
	m.mutateRef = m.deleteRef
	m.hasSearchTarget = true

	m.resetCluster()

	if m.searchView.Active() || m.logsView.Active() || m.viewer.Active() || m.modal.Active() ||
		m.ctrPicker.Active() || m.portPicker.Active() || m.cmdPicker.Active() || m.pfPanel.Active() {
		t.Error("reset should dismiss every surface showing the departing cluster's data")
	}
	if m.deleteRef != (kube.ObjectRef{}) || m.mutateRef != (kube.ObjectRef{}) || m.hasSearchTarget {
		t.Error("reset should drop every stashed object from the departing cluster")
	}
	if m.status.HasError() || m.status.Discovering() {
		t.Error("reset should clear the old cluster's status bar and stop its spinner")
	}
	if hint := m.hintbar.View(); !strings.Contains(hint, keymap.ActionDrillIn.Describe()) {
		t.Errorf("reset should restore the browse hints, got %q", hint)
	}
}
