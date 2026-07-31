package kube

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// stubDiscoverer is a hermetic resourceDiscoverer: it returns whatever
// lists/err it is given, so tests can inject healthy resources, partial-failure
// errors, and total failures with no server (D18).
//
// groups is the server's group list. Left nil it is synthesised from lists, so
// every test that does not care about groups gets a consistent pair; a test that
// does care (the versionless-group path, D187) sets it explicitly.
type stubDiscoverer struct {
	lists     []*metav1.APIResourceList
	err       error
	groups    *metav1.APIGroupList
	groupsErr error
}

func (s stubDiscoverer) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return s.lists, s.err
}

func (s stubDiscoverer) ServerGroups() (*metav1.APIGroupList, error) {
	if s.groupsErr != nil {
		return nil, s.groupsErr
	}
	if s.groups != nil {
		return s.groups, nil
	}
	out := &metav1.APIGroupList{}
	for _, l := range s.lists {
		if l == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(l.GroupVersion)
		if err != nil {
			continue
		}
		out.Groups = append(out.Groups, metav1.APIGroup{
			Name:     gv.Group,
			Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: l.GroupVersion, Version: gv.Version}},
		})
	}
	return out, nil
}

// groupList builds an APIGroupList from "group/version" strings; a bare group
// name yields a group with **no versions** — the shape an aggregated apiserver
// returns for a broken group (D187).
func groupList(gvs ...string) *metav1.APIGroupList {
	out := &metav1.APIGroupList{}
	for _, s := range gvs {
		gv, err := schema.ParseGroupVersion(s)
		if err != nil {
			continue
		}
		g := metav1.APIGroup{Name: gv.Group}
		if gv.Group == "" { // a bare name parses as a version; treat it as a group
			g.Name = gv.Version
		} else {
			g.Versions = []metav1.GroupVersionForDiscovery{{GroupVersion: s, Version: gv.Version}}
		}
		out.Groups = append(out.Groups, g)
	}
	return out
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
	if got := res.Failed[0]; got.Group != badGV.Group || got.Version != badGV.Version {
		t.Errorf("Failed[0] = %s, want %s", got.GroupVersion(), badGV.String())
	}
	if res.Failed[0].Err == nil {
		t.Error("Failed[0].Err = nil, want the per-group cause")
	}
	// Healthy resources still surfaced despite the failed group.
	if len(res.Resources) == 0 {
		t.Fatal("no resources returned; a failed group must not blank the menu")
	}
}

// TestDiscoverResourcesNamesAVersionlessGroup covers DISC-01/D187: on an
// aggregated-discovery server a broken group reaches the preferred-resource pass
// as *silence* — no error, no resources — and survives only as a group with no
// versions in the group list. That is the shape kubecom actually meets in the
// wild (proven live in TestEnvtestBrokenAPIGroupIsIsolated), and the group must
// still be named so the menu can mark it unavailable and DIAG-01 can log it.
func TestDiscoverResourcesNamesAVersionlessGroup(t *testing.T) {
	res := discoverResources(stubDiscoverer{
		lists:  []*metav1.APIResourceList{coreList(), appsList()},
		groups: groupList("apps/v1", "metrics.k8s.io"),
	})

	if res.Err != nil {
		t.Fatalf("Err = %v, want nil (a broken group must not fail the pass)", res.Err)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("Failed = %v, want exactly the versionless group", res.Failed)
	}
	got := res.Failed[0]
	if got.Group != "metrics.k8s.io" || got.Version != "" {
		t.Errorf("Failed[0] = %+v, want group metrics.k8s.io with no version", got)
	}
	if !errors.Is(got.Err, ErrGroupServesNoVersion) {
		t.Errorf("Failed[0].Err = %v, want ErrGroupServesNoVersion", got.Err)
	}
	if got.GroupVersion() != "metrics.k8s.io" {
		t.Errorf("GroupVersion() = %q, want the bare group when no version is known", got.GroupVersion())
	}
	// …and the healthy kinds are untouched: isolation, not a blanked menu (#87).
	if len(res.Resources) == 0 {
		t.Fatal("no resources returned; a broken group must not blank the menu")
	}
}

// TestDiscoverResourcesDoesNotDoubleReportAFailedGroup pins that the two failure
// shapes never stack: a legacy server reports the group through
// ErrGroupDiscoveryFailed *with* its version, and that entry must not be joined
// by a versionless duplicate that would tell the user the same thing twice with
// less detail.
func TestDiscoverResourcesDoesNotDoubleReportAFailedGroup(t *testing.T) {
	badGV := schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}
	gde := &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{
		badGV: errors.New("the server is currently unable to handle the request"),
	}}
	res := discoverResources(stubDiscoverer{
		lists:  []*metav1.APIResourceList{coreList()},
		err:    gde,
		groups: groupList("metrics.k8s.io"),
	})

	if len(res.Failed) != 1 {
		t.Fatalf("Failed = %v, want the group reported once", res.Failed)
	}
	if res.Failed[0].Version != "v1beta1" {
		t.Errorf("Failed[0] = %+v, want the version-bearing report to win", res.Failed[0])
	}
}

// TestDiscoverResourcesSurvivesAGroupListError: the group list is a second read,
// and failing it must cost only the extra reporting — the resources are already
// in hand (principle 3).
func TestDiscoverResourcesSurvivesAGroupListError(t *testing.T) {
	res := discoverResources(stubDiscoverer{
		lists:     []*metav1.APIResourceList{coreList()},
		groupsErr: errors.New("connection reset"),
	})
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if len(res.Failed) != 0 {
		t.Errorf("Failed = %v, want none", res.Failed)
	}
	if len(res.Resources) == 0 {
		t.Error("a group-list error must not cost the discovered resources")
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
	if len(res.Failed) != 1 || res.Failed[0].GroupVersion() != "a/b/c" {
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
