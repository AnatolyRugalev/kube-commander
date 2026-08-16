package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/unhealthyview"
)

// unhealthyScanKey is the default app.unhealthyScan key (`U`, STORY-06g-2b-2): the
// cross-kind partner to the per-kind `H` filter (unhealthyKey above). It carries
// text, so it resolves to an action only in the browse context — the unhealthy
// list has no query field, so no key types into it (D278 pt 3).
var unhealthyScanKey = tea.Key{Code: 'U', Text: "U"}

// fakeScanner is a hermetic Scanner: it records what each sweep asked for and
// hands back a channel preloaded with the configured hits as ScanMatch events,
// then the extra events (progress / terminal), closed unless keepOpen so a test
// can assert either the streaming/completion path or the cancellation path. The
// contexts are kept so a test can assert a fan-out was torn down.
type fakeScanner struct {
	hits     []kube.ScanHit
	events   []kube.ScanEvent
	keepOpen bool

	calls    int
	ctxs     []context.Context
	gotRes   []kube.Resource
	gotNS    string
	gotLimit int
}

func (f *fakeScanner) Scan(ctx context.Context, resources []kube.Resource, namespace string, _ kube.RowFilter, limit int) <-chan kube.ScanEvent {
	f.calls++
	f.ctxs = append(f.ctxs, ctx)
	f.gotRes = resources
	f.gotNS = namespace
	f.gotLimit = limit
	ch := make(chan kube.ScanEvent, len(f.hits)+len(f.events)+1)
	for _, h := range f.hits {
		ch <- kube.ScanEvent{Type: kube.ScanMatch, Hit: h}
	}
	for _, ev := range f.events {
		ch <- ev
	}
	if !f.keepOpen {
		close(ch)
	}
	return ch
}

// scanHit builds a ScanHit of the given kind for ns/name whose STATUS column
// carries the given status, mirroring the component's own helper.
func scanHit(kind, resource, ns, name, status string) kube.ScanHit {
	r := kindResource(resource, kind)
	r.Namespaced = ns != ""
	return kube.ScanHit{
		Resource: r,
		Columns: []kube.Column{
			{Name: "NAME"},
			{Name: "STATUS"},
		},
		Row: kube.Row{
			Cells:  []any{name, status},
			Object: kube.ObjectRef{Namespace: ns, Name: name, UID: kind + "/" + ns + "/" + name},
		},
	}
}

// openUnhealthyView presses the app.unhealthyScan key over a wired scanner and
// returns the model with the view up, plus the open's pump command so a test can
// drive the sweep's events in (M2-02/D53).
func openUnhealthyView(t *testing.T, s Scanner, opts ...Option) (Model, tea.Cmd) {
	t.Helper()
	m := sizedWith(t, append([]Option{WithScanner(s)}, opts...)...)
	m, cmd := press(t, m, unhealthyScanKey)
	if !m.unhealthyView.Active() {
		t.Fatal("app.unhealthyScan should open the cross-kind unhealthy list")
	}
	return m, cmd
}

// drainScanPump feeds the scan pump's returned commands back into the model until
// the chain stops (the channel closed), so a test that opened the view can then
// assert on the fully-streamed state. It mirrors how search tests drive a pump
// message by message (M2-02/D53).
func drainScanPump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m
}

// TestUnhealthyScanInertWithoutScanner proves the model is scan-inert with no
// scanner wired: the key opens nothing rather than an empty list that can never
// fill (the search-inert pattern, D131).
func TestUnhealthyScanInertWithoutScanner(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, unhealthyScanKey)
	if m.unhealthyView.Active() {
		t.Fatal("with no scanner wired the key must not open the view")
	}
}

// TestOpenUnhealthyRunsOneSweepOverTheMenuKinds proves the open gesture launches a
// single sweep over the kinds the menu currently offers, in the app's namespace,
// with the scan cap, and the view reflects it: searching indicator on, progress
// started over that many kinds, and the hits streamed in — the PVC beside the
// pod, the non-pod failure the S02 walk missed.
func TestOpenUnhealthyRunsOneSweepOverTheMenuKinds(t *testing.T) {
	s := &fakeScanner{
		hits: []kube.ScanHit{
			scanHit("Pod", "pods", "web", "web-1", "CrashLoopBackOff"),
			scanHit("PersistentVolumeClaim", "persistentvolumeclaims", "web", "web-pvc", "Lost"),
		},
	}
	m, cmd := openUnhealthyView(t, s, WithNamespace("web"))

	if s.calls != 1 {
		t.Fatalf("opening the view should run exactly one sweep, got %d", s.calls)
	}
	if s.gotNS != "web" {
		t.Fatalf("the sweep should cover the app's namespace, got %q", s.gotNS)
	}
	if s.gotLimit != scanHitLimit {
		t.Fatalf("the sweep should run under scanHitLimit, got %d", s.gotLimit)
	}
	if !m.unhealthyView.Searching() {
		t.Fatal("the sweep should mark the view as in-flight on open")
	}
	done, total := m.unhealthyView.Progress()
	if total == 0 || done != 0 {
		t.Fatalf("the sweep should report its kind count as progress started, got %d/%d", done, total)
	}
	m = drainScanPump(t, m, cmd)
	if m.unhealthyView.Len() != 2 {
		t.Fatalf("the streamed hits should land in the view, got %d", m.unhealthyView.Len())
	}
}

// TestUnhealthyScanStreamsHitsAndClearsInFlight proves the pump delivers the
// sweep's events into the view kind by kind and clears the in-flight indicator
// when the fan-out ends (ScanDone before the channel closes).
func TestUnhealthyScanStreamsHitsAndClearsInFlight(t *testing.T) {
	s := &fakeScanner{
		hits: []kube.ScanHit{scanHit("Pod", "pods", "web", "web-1", "CrashLoopBackOff")},
		events: []kube.ScanEvent{
			{Type: kube.ScanKindDone, Resource: kindResource("pods", "Pod")},
			{Type: kube.ScanDone},
		},
	}
	m, cmd := openUnhealthyView(t, s, WithNamespace("web"))

	if !m.unhealthyView.Searching() {
		t.Fatal("the view should still be in-flight before the pump drains")
	}
	if m.unhealthyView.Len() != 0 {
		t.Fatal("no event has been pumped yet, so the view should still be empty")
	}

	// The first pumped event appends the hit; the kind-done advances progress; the
	// terminal ScanDone clears in-flight; the channel close ends the pump.
	m = drainScanPump(t, m, cmd)
	if m.unhealthyView.Len() != 1 {
		t.Fatalf("the pumped hits should land in the view, got %d", m.unhealthyView.Len())
	}
	if done, _ := m.unhealthyView.Progress(); done != 1 {
		t.Fatalf("a kind-done should advance progress, got %d", done)
	}
	if m.unhealthyView.Searching() {
		t.Fatal("the terminal ScanDone should clear the in-flight indicator")
	}
}

// TestUnhealthyCapped reports the cap state: a sweep truncated by the hit cap
// leaves the view saying so (SetCapped), which the channel close alone cannot.
func TestUnhealthyCapped(t *testing.T) {
	s := &fakeScanner{
		hits: []kube.ScanHit{scanHit("Pod", "pods", "web", "web-1", "CrashLoopBackOff")},
		events: []kube.ScanEvent{
			{Type: kube.ScanKindDone, Resource: kindResource("pods", "Pod")},
			{Type: kube.ScanDone, Capped: true},
		},
	}
	m, cmd := openUnhealthyView(t, s, WithNamespace("web"))
	if m.unhealthyView.Capped() {
		t.Fatal("the view must not claim a cap before the terminal event lands")
	}
	m = drainScanPump(t, m, cmd)
	if !m.unhealthyView.Capped() {
		t.Fatal("a capped sweep should report the cap on the view")
	}
}

// TestUnhealthyEmptyScopeDegrades proves that when no kind is available the view
// degrades to an idle empty list instead of spinning on an in-flight indicator
// that would never clear (principle 3).
func TestUnhealthyEmptyScopeDegrades(t *testing.T) {
	s := &fakeScanner{}
	m := sizedWith(t, WithScanner(s))
	// Every group holding a kind failed discovery, so each seed row is marked
	// unavailable and the sweep scope comes out empty.
	m.menu.Reconcile(kube.DiscoveryResult{Failed: []kube.FailedGroup{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "networking.k8s.io", Version: "v1"},
		{Group: "storage.k8s.io", Version: "v1"},
	}})
	if len(m.scanResources()) != 0 {
		t.Fatalf("every group failed discovery, so the sweep scope should be empty: %v", m.scanResources())
	}

	next, cmd := m.Update(tea.KeyPressMsg(unhealthyScanKey))
	m = next.(Model)
	if !m.unhealthyView.Active() {
		t.Fatal("the key should still open the view with nothing to sweep")
	}
	if m.unhealthyView.Searching() {
		t.Fatal("with no kinds to sweep the view must not spin on an in-flight indicator")
	}
	if s.calls != 0 {
		t.Fatalf("with no kinds to sweep no scan should launch, got %d calls", s.calls)
	}
	if cmd != nil {
		t.Fatal("an empty scope should issue no pump")
	}
}

// TestUnhealthyDrillInSwitchesToHit proves drilling into a hit closes the view
// and switches the browse view to the hit's kind, stashing the hit's object as a
// pending selection the watch pump applies once its row arrives.
func TestUnhealthyDrillInSwitchesToHit(t *testing.T) {
	fw := &fakeWatcher{}
	m, _ := openUnhealthyView(t, &fakeScanner{}, WithWatcher(fw), WithNamespace("web"))
	hit := scanHit("Deployment", "deployments", "web", "api", "Available")

	next, cmd := m.Update(unhealthyview.SelectedMsg{Kind: "unhealthy", Hit: hit})
	m = next.(Model)
	if m.unhealthyView.Active() {
		t.Fatal("drilling into a hit should close the unhealthy list")
	}
	if len(fw.res) != 1 || fw.res[0].GVK.Kind != "Deployment" {
		t.Fatalf("drilling in should start a watch for the hit's kind, got %#v", fw.res)
	}
	if !m.hasCurrent || m.current.GVK.Kind != "Deployment" {
		t.Fatal("the browse view should be switched to the hit's kind")
	}
	if !m.hasSearchTarget || m.searchTarget.Name != "api" || m.searchTarget.Namespace != "web" {
		t.Fatal("the hit's object should be pending selection until its row arrives")
	}
	if !m.table.Focused() {
		t.Fatal("drilling into a hit should focus the table, like any resource drill-in")
	}
	_ = cmd
}

// TestUnhealthyClosedMsgDismisses proves the view's own ClosedMsg (nav.back) is
// handled: the view hides and any sweep feeding it is cancelled.
func TestUnhealthyClosedMsgDismisses(t *testing.T) {
	s := &fakeScanner{keepOpen: true}
	m, _ := openUnhealthyView(t, s)
	next, _ := m.Update(unhealthyview.ClosedMsg{Kind: "unhealthy"})
	m = next.(Model)
	if m.unhealthyView.Active() {
		t.Fatal("ClosedMsg should hide the unhealthy list")
	}
	if s.calls != 1 {
		t.Fatal("a sweep should have been running for the close to cancel")
	}
	for _, c := range s.ctxs {
		if err := c.Err(); err == nil {
			t.Fatal("closing the view should cancel the in-flight sweep")
		}
	}
}

// TestUnhealthyActionQuitClosesView proves `q` closes the view rather than
// quitting the app, exactly as the search view and logs view own the quit key.
func TestUnhealthyActionQuitClosesView(t *testing.T) {
	m, _ := openUnhealthyView(t, &fakeScanner{})
	m, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if m.unhealthyView.Active() {
		t.Fatal("q should close the unhealthy list")
	}
	if cmd != nil {
		t.Fatal("q on the unhealthy list must not quit the app")
	}
}
