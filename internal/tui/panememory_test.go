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
	got    []config.MenuResource
	drills []*config.DrillOwner
	err    error
}

func (f *fakeResourcer) PersistResource(r config.MenuResource, drill *config.DrillOwner) error {
	f.got = append(f.got, r)
	f.drills = append(f.drills, drill)
	return f.err
}

func (f *fakeResourcer) PersistViewState(col string, asc bool) error {
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
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry, "", false))

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
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithLastResource(&entry, "", false), WithResourcePersister(fr))

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
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry, "", false))

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
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry, "", false))

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
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry, "", false))

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
	m := sizedWith(t, WithWatcher(fw), WithLastResource(&entry, "", false))

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
		WithLastResource(&oldEntry, "", false), WithResourcePersister(oldFR),
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

// drillOwnerEntry is the address of the Deployment whose children the test drill-in
// opens — the owner half of the address a drill-in pane remembers (CTX-MEM-04).
func drillOwnerEntry() *config.DrillOwner {
	return &config.DrillOwner{
		Resource:  config.MenuResource{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"},
		Namespace: "default",
		Name:      "web",
	}
}

// TestDrillInRecordsTheOwnerAddress is the CTX-MEM-04 write half: a drill-in remembers
// the *owner* the child scope was opened from, beside the child kind, so a restore can
// re-enter the scope instead of landing on the plain child list. This test drives the
// scope-open seam directly (selectChildScope), which is the one point every drill-in
// funnels through.
func TestDrillInRecordsTheOwnerAddress(t *testing.T) {
	fr := &fakeResourcer{}
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithChildResolver(&fakeChildResolver{scope: podScope()}), WithResourcePersister(fr))

	next, cmd := m.selectChildScope(drillOwnerResource(*drillOwnerEntry()),
		kube.ObjectRef{Namespace: "default", Name: "web"}, podScope())
	_ = next
	if cmd == nil {
		t.Fatal("a drill-in should issue a persist command")
	}
	runCmd(cmd)

	if len(fr.got) != 1 || fr.got[0].Resource != "pods" {
		t.Fatalf("recorded %v, want the child kind pods", fr.got)
	}
	if len(fr.drills) != 1 || fr.drills[0] == nil {
		t.Fatalf("the drill owner should be recorded beside the kind, got %v", fr.drills)
	}
	d := *fr.drills[0]
	if d.Namespace != "default" || d.Name != "web" {
		t.Errorf("recorded drill owner = %+v, want default/web", d)
	}
	if d.Resource.Group != "apps" || d.Resource.Resource != "deployments" || d.Resource.Kind != "Deployment" {
		t.Errorf("recorded drill owner kind = %+v, want apps/Deployment", d.Resource)
	}
}

// TestAPlainTableClearsTheRememberedDrillOwner: a drill-in owner is only true while the
// scope is open. Leaving the drill-in back to a plain table (nav.back's selectResource)
// records the owner kind with no drill-in, so the next launch does not try to re-enter a
// scope the reader had already left.
func TestAPlainTableClearsTheRememberedDrillOwner(t *testing.T) {
	fr := &fakeResourcer{}
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithChildResolver(&fakeChildResolver{scope: podScope()}), WithResourcePersister(fr))

	next, cmd := m.selectChildScope(drillOwnerResource(*drillOwnerEntry()),
		kube.ObjectRef{Namespace: "default", Name: "web"}, podScope())
	m = next.(Model)
	runCmd(cmd)
	if len(fr.drills) != 1 || fr.drills[0] == nil {
		t.Fatalf("the drill-in should record the owner first, got %v", fr.drills)
	}

	next, cmd = m.selectResource(drillOwnerResource(*drillOwnerEntry()))
	_ = next
	runCmd(cmd)
	if len(fr.drills) != 2 || fr.drills[1] != nil {
		t.Errorf("leaving the drill-in should record no owner, got %v", fr.drills)
	}
	if len(fr.got) != 2 || fr.got[1].Resource != "deployments" {
		t.Errorf("leaving the drill-in should record the owner kind, got %v", fr.got)
	}
}

// TestRestoreReEntersTheRememberedDrillInScope is the CTX-MEM-04 read half's headline: a
// remembered drill-in comes back *in the scope*, not as the plain child list. The restore
// re-resolves the owner through the ChildResolver and opens the child table under the
// fresh scope — the re-derived selector, never a replayed one (D240 pt 6).
func TestRestoreReEntersTheRememberedDrillInScope(t *testing.T) {
	fr := &fakeResourcer{}
	r := &fakeChildResolver{scope: podScope()}
	fw := preloadedWatcher()
	entry := config.MenuResource{Version: "v1", Resource: "pods", Kind: "Pod"}
	m := sizedWith(t, WithWatcher(fw), WithChildResolver(r), WithResourcePersister(fr),
		WithLastResource(&entry, "", false), WithLastDrillOwner(drillOwnerEntry()))

	// The restore runs off the update loop: discovery issues the re-resolve, and the
	// resolver's answer comes back as a message, exactly as a live drill-in does.
	next, cmd := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{
		Resources: []kube.Resource{kindResource("pods", "Pod")},
	}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("a remembered drill-in should issue a scope re-resolve command")
	}
	msg, ok := cmd().(restoreDrillMsg)
	if !ok {
		t.Fatalf("the re-resolve delivered %T, want restoreDrillMsg", cmd())
	}
	next, _ = m.Update(msg)
	m = next.(Model)

	if !m.hasChildScope {
		t.Fatal("the restore should re-enter the drill-in scope")
	}
	if !m.hasCurrent || m.current.GVR.Resource != "pods" {
		t.Fatalf("the restored table should be the child kind, got %v", m.current.GVR)
	}
	if r.calls != 1 {
		t.Fatalf("the owner should be re-resolved once, got %d calls", r.calls)
	}
	if r.gotOwner.GVK.Kind != "Deployment" || r.gotRef.Name != "web" {
		t.Errorf("the resolver should be asked about the remembered owner, got %+v / %+v", r.gotOwner, r.gotRef)
	}
	// The watch runs under the re-resolved scope, not the app's namespace.
	last := fw.opts[len(fw.opts)-1]
	if last.LabelSelector != "app=web" {
		t.Errorf("the restored watch selector = %q, want the scope's %q", last.LabelSelector, "app=web")
	}
	// A restore is not a write-back: the address came out of the state file.
	if len(fr.got) != 0 {
		t.Errorf("a successful restore must not rewrite the file it read from, got %v", fr.got)
	}
}

// TestRestoreDrillOwnerGoneLandsOnThePlainListAndSaysSo is the degrade the whole
// deferral was gated on (D240 pt 6): the remembered drill-in's owner can be gone, and
// silently landing in a *different* scope is worse than landing on the plain list. A
// re-resolve that fails lands on the plain child list **and says the owner is gone on
// screen** — never a silent, possibly-wrong scope.
func TestRestoreDrillOwnerGoneLandsOnThePlainListAndSaysSo(t *testing.T) {
	fr := &fakeResourcer{}
	r := &fakeChildResolver{err: errPaneMemory}
	fw := preloadedWatcher()
	entry := config.MenuResource{Version: "v1", Resource: "pods", Kind: "Pod"}
	m := sizedWith(t, WithWatcher(fw), WithChildResolver(r), WithResourcePersister(fr),
		WithLastResource(&entry, "", false), WithLastDrillOwner(drillOwnerEntry()))

	next, cmd := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{
		Resources: []kube.Resource{kindResource("pods", "Pod")},
	}})
	m = next.(Model)
	msg, ok := cmd().(restoreDrillMsg)
	if !ok {
		t.Fatalf("the re-resolve delivered %T, want restoreDrillMsg", cmd())
	}
	next, cmd = m.Update(msg)
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the degrade should issue the plain-list watch's commands (incl. its persist)")
	}

	// Landed on the plain child list: a table on pods, no scope, one plain watch.
	if !m.hasCurrent || m.current.GVR.Resource != "pods" {
		t.Fatalf("the degrade should open the plain child list, got %v", m.current.GVR)
	}
	if m.hasChildScope {
		t.Error("a failed re-resolve must not leave a scope installed")
	}
	if got := fw.opts[len(fw.opts)-1]; got.LabelSelector != "" || got.FieldSelector != "" {
		t.Errorf("the plain fallback must be unscoped, got %+v", got)
	}
	// And the failure is said on screen, not silent: a notice naming the owner.
	if !m.status.HasNotice() {
		t.Fatal("a failed drill-in restore should say so on screen")
	}
	if view := m.status.View(); !strings.Contains(view, "Deployment") || !strings.Contains(view, "web") {
		t.Errorf("the notice should name the gone owner, got %q", view)
	}
	// The corrected state is recorded in memory — the next write is the plain list
	// with no drill owner, not a drill-in whose owner does not exist.
	if m.lastResource == nil || m.lastResource.Resource != "pods" {
		t.Errorf("the degrade should remember the plain list, got %+v", m.lastResource)
	}
	if m.lastDrillOwner != nil {
		t.Errorf("the degrade should clear the dead drill owner from memory, got %+v", m.lastDrillOwner)
	}
}

// TestALaunchWithNothingRememberedLandsOnPods is the STORY-06l startup default
// (feedback 2026-08-15, D288): a context whose state file records no kind opens the
// Pods table once discovery lands, not the welcome pane — the first frame is the one
// an operator opened kubecom to see.
func TestALaunchWithNothingRememberedLandsOnPods(t *testing.T) {
	fw := preloadedWatcher()
	m := sizedWith(t, WithWatcher(fw), WithLastResource(nil, "", false))

	if m.hasCurrent {
		t.Fatal("construction must not open a table before discovery")
	}
	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{podsResource()}})

	if !m.hasCurrent || m.current.GVR.Resource != "pods" {
		t.Fatalf("the default landing kind should be open, hasCurrent=%v current=%v", m.hasCurrent, m.current.GVR)
	}
	if len(fw.res) != 1 || fw.res[0].GVR.Resource != "pods" {
		t.Fatalf("watches started = %v, want one on pods", fw.res)
	}
	// It is the ordinary drill-in, so the table takes focus exactly as it would have
	// had the reader pressed the keys.
	if !m.table.Focused() {
		t.Error("the default landing should hand focus to the table, as a drill-in does")
	}
}

// TestARememberedKindStillWinsOverTheDefault: the landing default is a fallback, not
// an override — a context that recorded a kind reopens on it (D240 is unchanged).
func TestARememberedKindStillWinsOverTheDefault(t *testing.T) {
	entry := widgetEntry()
	m := sizedWith(t, WithWatcher(preloadedWatcher()), WithLastResource(&entry, "", false))
	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{widgetResource(), podsResource()}})

	if !m.hasCurrent || m.current.GVR.Resource != "widgets" {
		t.Fatalf("the remembered kind should win, got hasCurrent=%v current=%v", m.hasCurrent, m.current.GVR)
	}
}

// TestWithoutTheOptionNothingIsRestored keeps the seam an opt-in: a model built with
// no WithLastResource — every hermetic test that does not wire one, and any shell that
// cannot resolve a context — still lands on the welcome pane.
func TestWithoutTheOptionNothingIsRestored(t *testing.T) {
	m := sizedWith(t, WithWatcher(preloadedWatcher()))
	m = discover(t, m, kube.DiscoveryResult{Resources: []kube.Resource{podsResource()}})

	if m.hasCurrent {
		t.Fatalf("an unarmed model must open nothing, got %v", m.current.GVR)
	}
}

// podsResource is the discovered twin for the seed's Pod row, as a discovery pass
// reports it — what the landing default resolves against.
func podsResource() kube.Resource {
	return kube.Resource{
		GVK:        schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Namespaced: true,
		Verbs:      []string{"get", "list", "watch"},
	}
}
