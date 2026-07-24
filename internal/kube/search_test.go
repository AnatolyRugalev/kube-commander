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

// collect drains a hit channel into a name-sorted slice of "kind/name" strings.
func collect(ch <-chan SearchHit) []string {
	var got []string
	for h := range ch {
		got = append(got, h.Resource.GVK.Kind+"/"+h.Ref.Name)
	}
	sort.Strings(got)
	return got
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
	n := 0
	for range searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "api", 2) {
		n++
	}
	if n != 2 {
		t.Fatalf("emitted %d hits, want cap of 2", n)
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

func TestSearchCancellationClosesChannel(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-1")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before draining
	// The channel must still close (not hang) even when ctx is already done.
	for range searchRows(ctx, lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", "api", 0) {
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
