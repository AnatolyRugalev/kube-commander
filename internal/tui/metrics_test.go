package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// fakeMetricsLister is a hermetic MetricsLister: it hands back a preset sample set
// (or an error) and records what it was asked, so a test can assert the metrics
// resource and the namespace the poll was scoped to.
type fakeMetricsLister struct {
	mu    sync.Mutex
	usage map[kube.UsageKey]kube.Usage
	err   error

	calls  int
	gotRes []kube.Resource
	gotNS  []string
}

func (f *fakeMetricsLister) Metrics(_ context.Context, res kube.Resource, ns string) (map[kube.UsageKey]kube.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.gotRes = append(f.gotRes, res)
	f.gotNS = append(f.gotNS, ns)
	return f.usage, f.err
}

func (f *fakeMetricsLister) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// podMetricsResource is the metrics kind that measures Pods, as discovery reports
// it (metrics.k8s.io/v1beta1, listable).
func podMetricsResource() kube.Resource {
	return kube.Resource{
		GVK:        schema.GroupVersionKind{Group: kube.MetricsGroup, Version: "v1beta1", Kind: "PodMetrics"},
		GVR:        schema.GroupVersionResource{Group: kube.MetricsGroup, Version: "v1beta1", Resource: "pods"},
		Namespaced: true,
		Verbs:      []string{"get", "list"},
	}
}

// podSamples are the samples the fake lister serves for the sortReset rows.
func podSamples() map[kube.UsageKey]kube.Usage {
	return map[kube.UsageKey]kube.Usage{
		{Name: "pod-a"}: {Name: "pod-a", CPUMilli: 7, MemoryBytes: 32 * 1024 * 1024},
		{Name: "pod-b"}: {Name: "pod-b", CPUMilli: 250, MemoryBytes: 128 * 1024 * 1024},
	}
}

// metricsModel is a sized model with a watcher and a metrics lister wired, whose
// menu has been reconciled with a discovery result carrying the metrics kind — the
// same input kube.MetricsFor reads availability from in the real binary.
func metricsModel(t *testing.T, ml MetricsLister, discovered ...kube.Resource) (Model, *fakeWatcher) {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithMetricsLister(ml))
	next, _ := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: discovered}})
	return next.(Model), fw
}

// drainMsgs runs a command and returns every message it produced, flattening the
// batch Bubble Tea would have flattened itself.
func drainMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var out []tea.Msg
	var walk func(tea.Msg)
	walk = func(msg tea.Msg) {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					walk(c())
				}
			}
			return
		}
		out = append(out, msg)
	}
	walk(cmd())
	return out
}

// firstMetricsMsg finds the refresh result among a command's messages.
func firstMetricsMsg(t *testing.T, cmd tea.Cmd) metricsMsg {
	t.Helper()
	for _, msg := range drainMsgs(cmd) {
		if got, ok := msg.(metricsMsg); ok {
			return got
		}
	}
	t.Fatal("expected a metrics refresh among the produced commands")
	return metricsMsg{}
}

// openMeasuredPods drills into Pods on a cluster that measures them and delivers
// both the watch's RESET and the first metrics refresh, so the returned model is
// showing a live table with the overlay installed.
func openMeasuredPods(t *testing.T, ml *fakeMetricsLister) Model {
	t.Helper()
	m, _ := metricsModel(t, ml, podMetricsResource())
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	for _, msg := range drainMsgs(cmd) {
		next, _ = m.Update(msg)
		m = next.(Model)
	}
	return m
}

// TestMetricsColumnsAppearForMeasuredKind is the leg's headline: browsing a kind the
// cluster measures polls metrics.k8s.io for the samples and the browse table grows
// the CPU/MEMORY columns, joined onto the watched rows.
func TestMetricsColumnsAppearForMeasuredKind(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m := openMeasuredPods(t, ml)

	if ml.count() != 1 {
		t.Fatalf("Metrics called %d times, want exactly one refresh on opening the table", ml.count())
	}
	if got := ml.gotRes[0].GVK.Kind; got != "PodMetrics" {
		t.Errorf("polled %q, want the discovered PodMetrics resource", got)
	}
	view := m.View().Content
	if !strings.Contains(view, "CPU") || !strings.Contains(view, "MEMORY") {
		t.Fatalf("the table should show the metrics columns:\n%s", firstLines(view))
	}
	if !strings.Contains(view, "250m") || !strings.Contains(view, "128Mi") {
		t.Errorf("the samples should be joined onto the rows:\n%s", firstLines(view))
	}
}

// TestMetricsSilentForUnmeasuredKind is the exit criterion's other half: a kind with
// no metrics counterpart costs no request, shows no columns, and says nothing — no
// toast, no placeholder (D167 pt 1).
func TestMetricsSilentForUnmeasuredKind(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m, _ := metricsModel(t, ml, podMetricsResource())

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("deployments", "Deployment")})
	m = next.(Model)
	for _, msg := range drainMsgs(cmd) {
		next, _ = m.Update(msg)
		m = next.(Model)
	}

	if ml.count() != 0 {
		t.Fatalf("an unmeasured kind should cost no metrics request, got %d", ml.count())
	}
	if view := m.View().Content; strings.Contains(view, "CPU") {
		t.Errorf("no metrics columns should appear for an unmeasured kind:\n%s", firstLines(view))
	}
}

// TestMetricsSilentWithoutMetricsServer proves availability is read off the
// discovered set: with the metrics group absent — not installed, or installed and
// failing discovery, which look identical here (#87) — Pods are browsed with no
// columns and no request.
func TestMetricsSilentWithoutMetricsServer(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m, _ := metricsModel(t, ml) // discovery returned no metrics group

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	for _, msg := range drainMsgs(cmd) {
		next, _ = m.Update(msg)
		m = next.(Model)
	}

	if ml.count() != 0 {
		t.Fatalf("without metrics-server nothing should be polled, got %d calls", ml.count())
	}
	if view := m.View().Content; strings.Contains(view, "CPU") {
		t.Errorf("absence must be silent — no columns:\n%s", firstLines(view))
	}
}

// TestMetricsInertWithoutLister proves a model with no metrics seam wired is
// metrics-inert (the hermetic default, and any launcher that does not wire one).
func TestMetricsInertWithoutLister(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw))
	next, _ := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{podMetricsResource()}}})
	m = next.(Model)

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	for _, msg := range drainMsgs(cmd) {
		next, _ = m.Update(msg)
		m = next.(Model)
	}

	if !m.metricsRes.GVR.Empty() {
		t.Fatal("no lister wired: the overlay should never arm")
	}
}

// TestMetricsPollScopedToWatchNamespace pins the scope: the poll asks for the
// namespace the browse watch is on, so a namespaced view does not fetch (and pay
// for) every namespace's samples.
func TestMetricsPollScopedToWatchNamespace(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithMetricsLister(ml), WithNamespace("kube-system"))
	next, _ := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{podMetricsResource()}}})
	m = next.(Model)

	_, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	_ = firstMetricsMsg(t, cmd)

	if got := ml.gotNS[0]; got != "kube-system" {
		t.Errorf("metrics polled namespace %q, want the browse watch's %q", got, "kube-system")
	}
}

// TestMetricsPollScopedToChildScopeNamespace is the drill-down case the M4-08
// journal flagged for the watch and which applies identically here: a Node's pods
// are cluster-wide, so the poll must follow the *scope's* namespace, not the app's.
func TestMetricsPollScopedToChildScopeNamespace(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithMetricsLister(ml), WithNamespace("kube-system"))
	next, _ := m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{podMetricsResource()}}})
	m = next.(Model)

	// A Node drill-down: pods across every namespace, narrowed by a field selector.
	scope := kube.ChildScope{Resource: kindResource("pods", "Pod"), Namespace: ""}
	_, cmd := m.selectChildScope(kindResource("nodes", "Node"), kube.ObjectRef{Name: "node-1"}, scope)
	_ = firstMetricsMsg(t, cmd)

	if got := ml.gotNS[0]; got != "" {
		t.Errorf("metrics polled namespace %q, want the child scope's cluster-wide %q", got, "")
	}
}

// TestMetricsRefreshErrorKeepsPreviousSamples proves a failed poll degrades the way
// an aggregated API demands: the columns keep the samples they had rather than
// blinking empty on one 503, and nothing is toasted at the reader who never asked
// for the overlay.
func TestMetricsRefreshErrorKeepsPreviousSamples(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m := openMeasuredPods(t, ml)

	next, _ := m.Update(metricsMsg{gen: m.metricsGen, err: errors.New("the server is currently unable to handle the request")})
	m = next.(Model)

	view := m.View().Content
	if !strings.Contains(view, "250m") {
		t.Errorf("a failed refresh should keep the previous samples:\n%s", firstLines(view))
	}
	if strings.Contains(view, "unable to handle") {
		t.Errorf("a failed metrics refresh must not be toasted:\n%s", firstLines(view))
	}
}

// TestMetricsRefreshReschedules proves the poll is a chain rather than a one-shot: a
// landed result arms the next tick, and the tick issues the next refresh.
func TestMetricsRefreshReschedules(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m, _ := metricsModel(t, ml, podMetricsResource())
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	result := firstMetricsMsg(t, cmd)

	next, tick := m.Update(result)
	m = next.(Model)
	if tick == nil {
		t.Fatal("a landed refresh should schedule the next poll")
	}

	// Fire the scheduled tick directly rather than waiting out the interval.
	next, refresh := m.Update(metricsTickMsg{gen: m.metricsGen})
	m = next.(Model)
	if refresh == nil {
		t.Fatal("a tick should issue the next refresh")
	}
	_ = refresh()
	if ml.count() != 2 {
		t.Errorf("Metrics called %d times, want the initial refresh plus the ticked one", ml.count())
	}
}

// TestMetricsStaleResultDropped is the generation guard: a refresh that lands after
// the reader moved to another kind must not paint that kind's table with the
// previous one's samples.
func TestMetricsStaleResultDropped(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m := openMeasuredPods(t, ml)
	stale := m.metricsGen

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("deployments", "Deployment")})
	m = next.(Model)
	next, _ = m.Update(metricsMsg{gen: stale, usage: podSamples()})
	m = next.(Model)

	if m.table.Usage() != nil {
		t.Error("a superseded refresh must not install its samples on the new table")
	}
}

// TestMetricsStoppedOnKindChange proves switching to an unmeasured kind takes the
// columns away and ends the poll — the overlay belongs to the kind, not the session.
func TestMetricsStoppedOnKindChange(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m := openMeasuredPods(t, ml)
	calls := ml.count()

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("deployments", "Deployment")})
	m = next.(Model)
	for _, msg := range drainMsgs(cmd) {
		next, _ = m.Update(msg)
		m = next.(Model)
	}

	if !m.metricsRes.GVR.Empty() {
		t.Error("the poll should be disarmed on an unmeasured kind")
	}
	if ml.count() != calls {
		t.Errorf("no further metrics requests should be made, got %d → %d", calls, ml.count())
	}
	if view := m.View().Content; strings.Contains(view, "CPU") {
		t.Errorf("the previous kind's metrics columns should be gone:\n%s", firstLines(view))
	}
}

// TestMetricsStoppedByClusterTeardown proves the poll is part of the per-cluster
// async inventory (D155 pt 1): a context switch (or a quit) disarms it, so the new
// cluster's table is never handed the departing cluster's samples.
func TestMetricsStoppedByClusterTeardown(t *testing.T) {
	ml := &fakeMetricsLister{usage: podSamples()}
	m := openMeasuredPods(t, ml)
	stale := m.metricsGen

	m.stopClusterAsync()

	if !m.metricsRes.GVR.Empty() {
		t.Fatal("stopClusterAsync should disarm the metrics poll")
	}
	// A result from the cancelled poll can still land (the request had already been
	// issued); it must not reach the table.
	departing := map[kube.UsageKey]kube.Usage{{Name: "pod-b"}: {Name: "pod-b", CPUMilli: 999}}
	next, _ := m.Update(metricsMsg{gen: stale, usage: departing})
	if got, ok := next.(Model).table.Usage()[kube.UsageKey{Name: "pod-b"}]; !ok || got.CPUMilli != 250 {
		t.Errorf("a result from the departing cluster must be dropped, table now holds %+v", got)
	}
}
