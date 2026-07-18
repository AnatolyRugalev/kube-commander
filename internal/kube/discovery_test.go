package kube

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// stubDiscoverer is a hermetic preferredResourceDiscoverer: it returns whatever
// lists/err it is given, so tests can inject healthy resources, partial-failure
// errors, and total failures with no server (D18).
type stubDiscoverer struct {
	lists []*metav1.APIResourceList
	err   error
}

func (s stubDiscoverer) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return s.lists, s.err
}

func coreList() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", Verbs: metav1.Verbs{"get", "list", "watch"}, ShortNames: []string{"po"}, Categories: []string{"all"}},
			{Name: "pods/log", Namespaced: true, Kind: "Pod", Verbs: metav1.Verbs{"get"}},                         // subresource → dropped
			{Name: "bindings", Namespaced: true, Kind: "Binding", Verbs: metav1.Verbs{"create"}},                  // not listable → dropped
			{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace", Verbs: metav1.Verbs{"get", "list", "watch"}},
		},
	}
}

func appsList() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{
			{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment", Verbs: metav1.Verbs{"get", "list", "watch"}},
		},
	}
}

func TestDiscoverResourcesHappyPath(t *testing.T) {
	res := discoverResources(stubDiscoverer{lists: []*metav1.APIResourceList{appsList(), coreList()}})
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("Failed = %v, want none", res.Failed)
	}
	// Subresource + create-only resource dropped; pods, namespaces, deployments kept.
	want := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "namespaces"},
		{Group: "", Version: "v1", Resource: "pods"},
		{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	if len(res.Resources) != len(want) {
		t.Fatalf("got %d resources, want %d: %+v", len(res.Resources), len(want), res.Resources)
	}
	// Sorted by (group, resource): core group "" sorts before "apps".
	for i, w := range want {
		if res.Resources[i].GVR != w {
			t.Errorf("Resources[%d].GVR = %v, want %v", i, res.Resources[i].GVR, w)
		}
	}
	// Metadata carried through for pods.
	pod := res.Resources[1]
	if pod.GVK.Kind != "Pod" || !pod.Namespaced {
		t.Errorf("pod = %+v, want Kind=Pod Namespaced=true", pod)
	}
	if len(pod.ShortNames) != 1 || pod.ShortNames[0] != "po" {
		t.Errorf("pod.ShortNames = %v, want [po]", pod.ShortNames)
	}
}

func TestDiscoverResourcesGroupFaultIsolation(t *testing.T) {
	// metrics.k8s.io fails (classic aggregated-API outage); core still loads.
	badGV := schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}
	gde := &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{
		badGV: errors.New("the server is currently unable to handle the request"),
	}}
	res := discoverResources(stubDiscoverer{lists: []*metav1.APIResourceList{coreList()}, err: gde})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil (partial failure must not be fatal)", res.Err)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("Failed = %v, want exactly the metrics group", res.Failed)
	}
	if res.Failed[0].GroupVersion != badGV.String() {
		t.Errorf("Failed[0].GroupVersion = %q, want %q", res.Failed[0].GroupVersion, badGV.String())
	}
	if res.Failed[0].Err == nil {
		t.Error("Failed[0].Err = nil, want the per-group cause")
	}
	// Healthy resources still surfaced despite the failed group.
	if len(res.Resources) == 0 {
		t.Fatal("no resources returned; a failed group must not blank the menu")
	}
}

func TestDiscoverResourcesTotalFailure(t *testing.T) {
	boom := errors.New("connection refused")
	res := discoverResources(stubDiscoverer{err: boom})
	if !errors.Is(res.Err, boom) {
		t.Fatalf("Err = %v, want %v", res.Err, boom)
	}
	if res.Resources != nil || res.Failed != nil {
		t.Errorf("on total failure want empty result, got Resources=%v Failed=%v", res.Resources, res.Failed)
	}
}

func TestDiscoverResourcesMalformedGroupVersion(t *testing.T) {
	bad := &metav1.APIResourceList{
		GroupVersion: "a/b/c", // unparsable
		APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Verbs: metav1.Verbs{"list"}}},
	}
	res := discoverResources(stubDiscoverer{lists: []*metav1.APIResourceList{bad, coreList()}})
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Failed) != 1 || res.Failed[0].GroupVersion != "a/b/c" {
		t.Fatalf("Failed = %v, want the malformed list isolated", res.Failed)
	}
	if len(res.Resources) == 0 {
		t.Error("healthy core list should still yield resources")
	}
}

func TestStartDiscoveryDelivers(t *testing.T) {
	ch := StartDiscovery(context.Background(), stubDiscoverer{lists: []*metav1.APIResourceList{coreList()}})
	res := <-ch
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Resources) == 0 {
		t.Fatal("StartDiscovery delivered no resources")
	}
}

func TestStartDiscoveryCanceledContextDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the goroutine tries to send
	// With a cancelled ctx the sender selects ctx.Done() and exits; the buffered
	// channel (cap 1) also means the send would not block regardless. Either way
	// StartDiscovery must return without leaking or panicking.
	ch := StartDiscovery(ctx, stubDiscoverer{lists: []*metav1.APIResourceList{coreList()}})
	if ch == nil {
		t.Fatal("StartDiscovery returned nil channel")
	}
}
