package kube

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// res builds a listable Resource for a built-in kind. Namespaced defaults to
// true; pass namespaced=false for cluster-scoped kinds (Node, etc).
func res(group, version, kind, plural string, namespaced bool) Resource {
	gvk := schema.GroupVersionKind{Group: group, Version: version, Kind: kind}
	return Resource{
		GVK:        gvk,
		GVR:        gvk.GroupVersion().WithResource(plural),
		Namespaced: namespaced,
	}
}

// tbl builds a server-printed Table stub whose rows carry only object identity
// (name/namespace) — all searchRows matches on.
func tbl(ns string, names ...string) *Table {
	t := &Table{}
	for _, n := range names {
		t.Rows = append(t.Rows, Row{
			Cells:  []any{n},
			Object: ObjectRef{Namespace: ns, Name: n, UID: n + "-uid"},
		})
	}
	return t
}

// fakeLister is a hermetic rowLister: it returns a canned Table (or error) per
// resource plural and records the namespace each kind was listed with.
type fakeLister struct {
	tables map[string]*Table
	errs   map[string]error

	mu     sync.Mutex
	nsSeen map[string]string
}

func (f *fakeLister) List(_ context.Context, r Resource, namespace string, _ metav1.ListOptions) (*Table, error) {
	f.mu.Lock()
	if f.nsSeen == nil {
		f.nsSeen = map[string]string{}
	}
	f.nsSeen[r.GVR.Resource] = namespace
	f.mu.Unlock()

	if err := f.errs[r.GVR.Resource]; err != nil {
		return nil, err
	}
	return f.tables[r.GVR.Resource], nil
}

// collect drains a search channel into a name-sorted slice of "kind/name" strings,
// ignoring the progress/terminal events (asserted separately).
func collect(ch <-chan SearchEvent) []string {
	var got []string
	for ev := range ch {
		if ev.Type != SearchMatch {
			continue
		}
		got = append(got, ev.Hit.Resource.GVK.Kind+"/"+ev.Hit.Ref.Name)
	}
	sort.Strings(got)
	return got
}

// drainSearch collects every event of a search, in arrival order.
func drainSearch(ch <-chan SearchEvent) []SearchEvent {
	var got []SearchEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

// kindsDone reports the kinds that emitted a SearchKindDone, and which of them
// were marked Failed.
func kindsDone(events []SearchEvent) (done []string, failed []string) {
	for _, ev := range events {
		if ev.Type != SearchKindDone {
			continue
		}
		done = append(done, ev.Resource.GVK.Kind)
		if ev.Failed {
			failed = append(failed, ev.Resource.GVK.Kind)
		}
	}
	sort.Strings(done)
	sort.Strings(failed)
	return done, failed
}

// terminal returns the single SearchDone event, failing when the stream did not
// end with exactly one.
func terminal(t *testing.T, events []SearchEvent) SearchEvent {
	t.Helper()
	var got []SearchEvent
	for _, ev := range events {
		if ev.Type == SearchDone {
			got = append(got, ev)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one SearchDone event, got %d", len(got))
	}
	if last := events[len(events)-1]; last.Type != SearchDone {
		t.Fatalf("SearchDone must be the last event before close, got %v", last.Type)
	}
	return got[0]
}

func TestSearchMatchesAcrossKindsCaseInsensitive(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods":        tbl("web", "api-server", "API-gateway", "redis"),
		"deployments": tbl("web", "api", "worker"),
	}}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}

	got := collect(searchRows(context.Background(), lister, resources, "web", "api", 0))
	want := []string{"Deployment/api", "Pod/API-gateway", "Pod/api-server"}
	if !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestSearchEmptyQueryMatchesAllNamedRows(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "a", "b", ""), // the empty-name row is skipped
	}}
	got := collect(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "", 0))
	if want := []string{"Pod/a", "Pod/b"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestSearchIsolatesPerKindFailure(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{"pods": tbl("web", "api-1")},
		errs:   map[string]error{"deployments": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}
	// The failing deployments list must not abort the search — pods still match.
	got := collect(searchRows(context.Background(), lister, resources, "web", "api", 0))
	if want := []string{"Pod/api-1"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v (per-kind failure should degrade to nothing)", got, want)
	}
}

func TestSearchCapsTotalHits(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "api-1", "api-2", "api-3", "api-4"),
	}}
	events := drainSearch(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "api", 2))
	n := 0
	for _, ev := range events {
		if ev.Type == SearchMatch {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("emitted %d hits, want cap of 2", n)
	}
	// The cap is *why* this search stopped — the close alone cannot say so, which is
	// the whole reason the stream carries a terminal event (SEARCH-03a).
	if !terminal(t, events).Capped {
		t.Error("a search stopped by the hit cap must report SearchDone{Capped: true}")
	}
}

// TestSearchReportsKindCompletion proves the progress signal: every kind handed to a
// search emits exactly one SearchKindDone — including the one whose List failed, so a
// consumer counting them always reaches N of M and never shows a stuck progress line
// because a kind was denied (principle 3).
func TestSearchReportsKindCompletion(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{"pods": tbl("web", "api-1"), "services": tbl("web", "api-svc")},
		errs:   map[string]error{"deployments": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("", "v1", "Service", "services", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}
	events := drainSearch(searchRows(context.Background(), lister, resources, "web", "api", 0))

	done, failed := kindsDone(events)
	if want := []string{"Deployment", "Pod", "Service"}; !equal(done, want) {
		t.Fatalf("kinds done = %v, want one per searched kind %v", done, want)
	}
	if want := []string{"Deployment"}; !equal(failed, want) {
		t.Errorf("failed kinds = %v, want %v", failed, want)
	}
	if terminal(t, events).Capped {
		t.Error("an exhaustive search must not report Capped")
	}
}

// TestSearchDoneNotCappedWhenHitsExactlyFitLimit pins the Capped semantics: it means
// "there were more matches than you were given", not "you were given limit matches" —
// so a search whose matches exactly fill the cap is still exhaustive.
func TestSearchDoneNotCappedWhenHitsExactlyFitLimit(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-1", "api-2")}}
	events := drainSearch(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "api", 2))
	if terminal(t, events).Capped {
		t.Error("matches exactly filling the cap truncated nothing, so Capped must be false")
	}
}

func TestSearchClusterScopedIgnoresNamespace(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"nodes": tbl("", "node-a", "node-b"),
	}}
	got := collect(searchRows(context.Background(), lister, []Resource{res("", "v1", "Node", "nodes", false)}, "web", "node", 0))
	if want := []string{"Node/node-a", "Node/node-b"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	if ns := lister.nsSeen["nodes"]; ns != "" {
		t.Errorf("cluster-scoped Node listed with namespace %q, want empty", ns)
	}
}

func TestSearchCancellationClosesChannelWithoutTerminalEvent(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-1")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before draining
	// The channel must still close (not hang) even when ctx is already done, and an
	// abandoned search reports nothing: a consumer that cancelled is gone, so no
	// SearchDone is worth blocking on.
	for _, ev := range drainSearch(searchRows(ctx, lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "api", 0)) {
		if ev.Type == SearchDone {
			t.Error("a cancelled search must not emit a terminal SearchDone")
		}
	}
}

func TestCommonSearchResourcesFiltersToCuratedSet(t *testing.T) {
	all := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
		res("", "v1", "Event", "events", true),                       // not curated
		res("example.com", "v1", "Widget", "widgets", true),          // CRD, not curated
		res("networking.k8s.io", "v1", "Ingress", "ingresses", true), // curated
	}
	got := CommonSearchResources(all)
	var kinds []string
	for _, r := range got {
		kinds = append(kinds, r.GVK.Kind)
	}
	sort.Strings(kinds)
	if want := []string{"Deployment", "Ingress", "Pod"}; !equal(kinds, want) {
		t.Fatalf("curated kinds = %v, want %v", kinds, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
