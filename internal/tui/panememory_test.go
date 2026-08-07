package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
)

// The pane-memory tests (CTX-MEM-02/D240). Two halves: what a drill-in *records*,
// and what a discovery pass *restores* — plus the degrade, which is the half that
// decides whether the feature is a convenience or a new way for a switch to break.

// errPaneMemory is the failure a watch or a discovery group is made to return when a
// test needs one; its text is never asserted.
var errPaneMemory = errors.New("boom")

// fakeResourcer is a hermetic ResourcePersister recording every kind it was asked to
// remember, so a test can assert both what was written and that nothing was.
type fakeResourcer struct {
	got []config.MenuResource
	err error
}

func (f *fakeResourcer) PersistResource(r config.MenuResource) error {
	f.got = append(f.got, r)
	return f.err
}

// widgetResource is a CRD kind the seed menu does not list, so a test that restores it
// proves the restore waits for discovery rather than resolving against the seed.
func widgetResource() kube.Resource {
	return kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
}

func widgetEntry() config.MenuResource {
	return config.MenuResource{Group: "example.com", Version: "v1", Resource: "widgets", Kind: "Widget"}
}

// preloadedWatcher is a fakeWatcher whose channel already holds a RESET, so the watch
// pump batched into every drill-in returns instead of blocking a test that drains the
// command. (tea.Batch collapses to its single non-nil member, so a drill-in that
// persists nothing *is* the pump — there is no batch wrapper to stop at.)
func preloadedWatcher() *fakeWatcher {
	return &fakeWatcher{preload: []kube.WatchEvent{resetEvent("api-1", "uid-1")}}
}

// runCmd executes cmd and everything batched under it, as the runtime does.
func runCmd(cmd tea.Cmd) { drainMsgs(cmd) }

// discover delivers a completed discovery pass to the model, as the pump does.
func discover(t *testing.T, m Model, result kube.DiscoveryResult) Model {
	t.Helper()
	next, _ := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: result})
	return next.(Model)
}

// TestDrillInRecordsTheKindForThisContext is the write half: opening a table records
// the kind through the seam, off the update loop, so the next launch can reopen it.
func TestDrillInRecordsTheKindForThisContext(t *testing.T) {
	fr := &fakeResourcer{}
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithResourcePersister(fr))

	_, cmd := m.selectResource(widgetResource())

	if len(fr.got) != 0 {
		t.Fatalf("the write must not happen inside Update, got %v", fr.got)
	}
	if cmd == nil {
		t.Fatal("drilling in should issue a persist command")
	}
	runCmd(cmd) // as the runtime would.
	if len(fr.got) != 1 || fr.got[0].Resource != "widgets" {
		t.Fatalf("recorded %v, want one entry for widgets", fr.got)
	}
	// The address carries the display hints a restore needs to rebuild a row, not just
	// the plural name.
	if fr.got[0].Group != "example.com" || fr.got[0].Version != "v1" || fr.got[0].Kind != "Widget" {
		t.Errorf("recorded entry = %+v, want the full address", fr.got[0])
	}
}

// TestAFailedWatchRecordsNothing keeps the state file honest: a kind whose watch could
// not start is not a kind the reader was browsing, and remembering it would reopen the
// same failure on every launch.
func TestAFailedWatchRecordsNothing(t *testing.T) {
	fr := &fakeResourcer{}
	fw := &fakeWatcher{err: errPaneMemory}
	m := sizedWith(t, WithWatcher(fw), WithResourcePersister(fr))

	next, cmd := m.selectResource(widgetResource())
	m = next.(Model)
	runCmd(cmd)
	if len(fr.got) != 0 {
		t.Errorf("a watch that never started must not be recorded, got %v", fr.got)
	}
	if m.lastResource != nil {
		t.Errorf("model memory after a failed watch = %+v, want none", *m.lastResource)
	}
}

// TestReSelectingTheRecordedKindWritesNothing is the dedupe. A namespace re-scope
// re-selects the *same* kind (M2-08c), so without this every scope change rewrites the
// state file to its current contents.
func TestReSelectingTheRecordedKindWritesNothing(t *testing.T) {
	fr := &fakeResourcer{}
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithResourcePersister(fr))

	next, cmd := m.selectResource(widgetResource())
	m = next.(Model)
	runCmd(cmd)

	next, cmd = m.selectResource(widgetResource())
	m = next.(Model)
	runCmd(cmd)
	if len(fr.got) != 1 {
		t.Errorf("re-selecting the recorded kind wrote again: %v", fr.got)
	}

	// A different kind is a real change and does write.
	_, cmd = m.selectResource(gvrResource("pods"))
	if cmd == nil {
		t.Fatal("opening a different kind should persist")
	}
	runCmd(cmd)
	if len(fr.got) != 2 || fr.got[1].Resource != "pods" {
		t.Errorf("recorded %v, want widgets then pods", fr.got)
	}
}

// TestLaunchRestoresTheRememberedKindAfterDiscovery is the read half's headline, and
// the reason the restore hangs off the discovery pass: `widgets` is a CRD, so it is
// resolvable only once the cluster's own API surface is in the menu (D240 pt 4).
func TestLaunchRestoresTheRememberedKindAfterDiscovery(t *testing.T) {
	fw := preloadedWatcher()
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry))

	// Before the pass the menu is the seed, which does not list the kind — and nothing
	// has been opened.
	if m.hasCurrent {
		t.Fatal("construction must not open a table")
	}

	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{widgetResource()}})

	if !m.hasCurrent || m.current.GVR.Resource != "widgets" {
		t.Fatalf("after discovery the remembered kind should be open, hasCurrent=%v current=%v", m.hasCurrent, m.current.GVR)
	}
	if len(fw.res) != 1 || fw.res[0].GVR.Resource != "widgets" {
		t.Fatalf("watches started = %v, want one on widgets", fw.res)
	}
	// The restore is the ordinary drill-in, so the menu marks the row and the table
	// takes focus — the reader lands where they would have landed by pressing the keys.
	if !strings.Contains(m.menu.View(), "Widget") {
		t.Errorf("the restored kind should be listed in the menu:\n%s", m.menu.View())
	}
	if !m.table.Focused() {
		t.Error("a restore should hand focus to the table, as a drill-in does")
	}
}

// TestARestoreIsNotAWriteBack: the address came out of the state file, so restoring it
// must not write it straight back — the same rule the restored namespace follows.
func TestARestoreIsNotAWriteBack(t *testing.T) {
	fr := &fakeResourcer{}
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithLastResource(&entry), WithResourcePersister(fr))

	next, cmd := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{
		Resources: []kube.Resource{widgetResource()},
	}})
	m = next.(Model)
	runCmd(cmd)
	if !m.hasCurrent {
		t.Fatal("the remembered kind should have been restored")
	}
	if len(fr.got) != 0 {
		t.Errorf("a restore must not rewrite the file it read from, got %v", fr.got)
	}
}

// TestARememberedKindTheClusterDoesNotServeIsSilent is D240 pt 3, the constraint the
// whole slice is gated on: a restore is a convenience and must never be the reason a
// launch or a switch shows an error. Pin a CRD on the cluster that has the operator,
// open kubecom against the one that does not, and you land exactly where you land
// today — seed menu, welcome pane, nothing said.
func TestARememberedKindTheClusterDoesNotServeIsSilent(t *testing.T) {
	fw := preloadedWatcher()
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry))

	next, cmd := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{}})
	m = next.(Model)

	if m.hasCurrent {
		t.Errorf("a kind this cluster does not serve must not open a table, got %v", m.current.GVR)
	}
	if len(fw.res) != 0 {
		t.Errorf("no watch may be started for an unserved kind, got %v", fw.res)
	}
	if cmd != nil {
		t.Error("a silent degrade issues no command — not a toast, not a retry")
	}
	if got := m.status.View(); strings.Contains(strings.ToLower(got), "widget") {
		t.Errorf("the status bar must say nothing about the missed restore: %q", got)
	}
	if !m.menu.Focused() {
		t.Error("the shell should be left in its pre-drill-in state, menu focused")
	}
}

// TestARememberedKindInAFailedGroupIsSilent is the same degrade by the other route: the
// kind is *listed* (a seed row) but its API group failed discovery, so the row is
// unavailable and watching it could only produce the error the restore promises not to
// cause.
func TestARememberedKindInAFailedGroupIsSilent(t *testing.T) {
	fw := preloadedWatcher()
	entry := config.MenuResource{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"}
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry))

	m = discover(t, m, kube.DiscoveryResult{
		Failed: []kube.FailedGroup{{Group: "apps", Version: "v1", Err: errPaneMemory}},
	})

	if m.hasCurrent || len(fw.res) != 0 {
		t.Errorf("an unavailable row must not be restored: hasCurrent=%v watches=%v", m.hasCurrent, fw.res)
	}
}

// TestARestoreYieldsToAReaderWhoDrilledInFirst: discovery can land seconds after
// launch, and by then the reader may have opened something themselves. The restore
// loses — being yanked off the table you just chose is worse than not being restored.
func TestARestoreYieldsToAReaderWhoDrilledInFirst(t *testing.T) {
	fw := preloadedWatcher()
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry))

	next, _ := m.selectResource(gvrResource("pods"))
	m = next.(Model)

	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{widgetResource()}})

	if m.current.GVR.Resource != "pods" {
		t.Errorf("the reader's own drill-in should stand, got %v", m.current.GVR)
	}
	if len(fw.res) != 1 {
		t.Errorf("the restore must not start a second watch, got %v", fw.res)
	}
}

// TestTheRestoreIsAttemptedOncePerCluster pins the guard that hasCurrent does *not*
// cover. A restore that found nothing leaves the reader on the menu with no table
// open, so nothing but the consumed attempt stops a second discovery pass — a
// reconnect, or any later leg that refreshes the API surface — from yanking them into
// a table minutes after they launched. The first pass here misses; the second serves
// the kind, and must still change nothing.
func TestTheRestoreIsAttemptedOncePerCluster(t *testing.T) {
	fw := preloadedWatcher()
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry))

	m = discover(t, m, kube.DiscoveryResult{}) // the kind is not served — restore misses.
	if m.hasCurrent {
		t.Fatal("the first pass should not have opened anything")
	}

	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{widgetResource()}})

	if m.hasCurrent {
		t.Errorf("a second discovery pass must not restore after the attempt was spent, got %v", m.current.GVR)
	}
	if len(fw.res) != 0 {
		t.Errorf("no watch may be started by a second pass, got %v", fw.res)
	}
}

// TestSwitchRebindsThePaneMemory is CTX-MEM-02 across a context switch — the gesture the
// feedback was actually about. The new context's remembered kind comes back with it, and
// the writer is rebound so a kind opened afterwards is recorded in the *new* context's
// file rather than the departed one's (the D163 bug, for the third field).
func TestSwitchRebindsThePaneMemory(t *testing.T) {
	newCluster, newWatcher, _ := newClusterFake()
	newWatcher.preload = []kube.WatchEvent{resetEvent("api-1", "uid-1")}
	fc := &fakeConnector{cluster: newCluster}
	newFR, oldFR := &fakeResourcer{}, &fakeResourcer{}
	entry := widgetEntry()
	fs := &fakeStateLoader{state: ContextState{LastResource: &entry, Resourcer: newFR}}

	oldEntry := config.MenuResource{Version: "v1", Resource: "pods", Kind: "Pod"}
	m := browsingModel(t, preloadedWatcher(),
		WithClusterConnector(fc), WithContextStateLoader(fs), WithContext("dev"),
		WithLastResource(&oldEntry), WithResourcePersister(oldFR),
	)

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, cmd))
	m = next.(Model)

	// The reset left no table open, and the seed menu cannot resolve a CRD yet.
	if m.hasCurrent {
		t.Fatal("the switch itself must not reopen a table before discovery")
	}
	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{widgetResource()}})

	if !m.hasCurrent || m.current.GVR.Resource != "widgets" {
		t.Fatalf("the new context's remembered kind should be open, got hasCurrent=%v %v", m.hasCurrent, m.current.GVR)
	}
	if len(newWatcher.res) != 1 {
		t.Fatalf("the restore should watch through the *new* cluster, got %v", newWatcher.res)
	}
	if len(newFR.got) != 0 || len(oldFR.got) != 0 {
		t.Errorf("a switch must not persist a kind it only restored: new=%v old=%v", newFR.got, oldFR.got)
	}

	// And the writer is rebound: browsing on now records against the new context.
	next, persist := m.selectResource(gvrResource("configmaps"))
	m = next.(Model)
	if persist == nil {
		t.Fatal("a drill-in after a switch should still persist")
	}
	runCmd(persist)
	if len(newFR.got) != 1 || newFR.got[0].Resource != "configmaps" {
		t.Errorf("persisted through the new context = %v, want configmaps", newFR.got)
	}
	if len(oldFR.got) != 0 {
		t.Errorf("the departed context's state file must not be written, got %v", oldFR.got)
	}
}
