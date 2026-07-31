package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// M1-INT-a: "a denied/broken API group is isolated", against a live apiserver.
//
// The hermetic suite already covers the isolation *logic* — discovery_test.go
// feeds discoverResources a fake whose ServerPreferredResources returns partial
// results plus an *ErrGroupDiscoveryFailed. What a fake cannot prove is that a
// real kube-apiserver produces that shape in the first place: the 2020 bug (#87,
// #76) was not a mishandled error, it was a whole menu blanked by one aggregated
// API being down, and the only way to know kubecom survives it is to break a
// group on a real control plane and look.
//
// Two failure modes, one per test, because they fail differently:
//
//   - A *broken* group (an aggregated APIService whose backing service is gone —
//     the classic metrics-server outage) fails at discovery time, and must land
//     in DiscoveryResult.Failed with every healthy group still in Resources.
//   - A *denied* resource (RBAC withholds `list`) does not fail discovery at all:
//     the menu still advertises it, and the denial surfaces per call as
//     KindForbidden. That distinction is the reason the TUI can show a Secrets
//     row to a user who may not read Secrets and degrade only when they open it.
//
// Both are opt-in behind requireEnvtest (D18) and are not part of `make check`.

// brokenAPIServiceGV is the group/version deliberately broken in the isolation
// test. metrics.k8s.io is not an arbitrary choice: an unreachable metrics-server
// is the single most common cause of a partially-failing discovery in the wild,
// and it is the one that motivated the fault isolation (#87).
const (
	brokenAPIServiceGroup = "metrics.k8s.io"
	brokenAPIServiceGV    = brokenAPIServiceGroup + "/v1beta1"
)

// apiServicesGVR addresses the aggregation layer's own registry, which is how a
// test registers an API group that is served by nothing.
var apiServicesGVR = schema.GroupVersionResource{
	Group:    "apiregistration.k8s.io",
	Version:  "v1",
	Resource: "apiservices",
}

// clientsFor builds the real Clients bundle against a live control plane, with
// the on-disk discovery cache redirected into the test's temp dir. The redirect
// matters twice: it keeps the test from writing into the developer's real
// ~/.cache/kubecom, and — more importantly — it keeps two Clients built in one
// test (admin and restricted) from sharing a cached discovery document.
func clientsFor(t *testing.T, cfg *rest.Config) *Clients {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir) // os.UserCacheDir on Linux
	t.Setenv("HOME", dir)           // …and on macOS
	c, err := NewClients(cfg)
	if err != nil {
		t.Fatalf("build clients: %v", err)
	}
	return c
}

// discoverFresh runs one full discovery pass through the production entry point
// (D8's async StartDiscovery), bypassing the on-disk cache so a pass taken after
// the cluster changed does not answer from a document taken before it.
func discoverFresh(ctx context.Context, c *Clients) DiscoveryResult {
	c.Invalidate()
	return <-c.StartDiscovery(ctx)
}

// findResource returns the discovered Resource for a group/resource, if the pass
// surfaced one. It is the lookup the action tests address objects through
// (requireDiscoveredResource), so they act on the same value the menu would.
func findResource(resources []Resource, group, resource string) (Resource, bool) {
	for _, r := range resources {
		if r.GVR.Group == group && r.GVR.Resource == resource {
			return r, true
		}
	}
	return Resource{}, false
}

// hasResource reports whether a discovery pass surfaced the given group/resource.
func hasResource(resources []Resource, group, resource string) bool {
	_, ok := findResource(resources, group, resource)
	return ok
}

// requireResource fails the test unless the resource is in the discovered set.
func requireResource(t *testing.T, resources []Resource, group, resource string) {
	t.Helper()
	if !hasResource(resources, group, resource) {
		t.Errorf("discovery lost %s/%s (%d resources discovered)", group, resource, len(resources))
	}
}

// failedGroup returns the recorded failure for an API group, if discovery
// isolated one. It keys on the group rather than the group/version because a
// broken group's version is not always knowable — see the assertion in
// TestEnvtestBrokenAPIGroupIsIsolated and D187.
func failedGroup(failed []FailedGroup, group string) (FailedGroup, bool) {
	for _, f := range failed {
		if f.Group == group {
			return f, true
		}
	}
	return FailedGroup{}, false
}

// TestEnvtestBrokenAPIGroupIsIsolated breaks one API group on a live control
// plane and asserts the rest of the menu survives it.
func TestEnvtestBrokenAPIGroupIsIsolated(t *testing.T) {
	_, cfg := startControlPlane(t)
	c := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Baseline: a healthy cluster discovers cleanly. Asserting this first is what
	// makes the second half meaningful — without it, a Failed entry could be
	// pre-existing noise rather than the group this test broke.
	base := discoverFresh(ctx, c)
	if base.Err != nil {
		t.Fatalf("healthy discovery failed wholesale: %v", base.Err)
	}
	if len(base.Failed) != 0 {
		t.Fatalf("healthy control plane reported failed groups: %v", base.Failed)
	}
	requireResource(t, base.Resources, "", "pods")
	requireResource(t, base.Resources, "apps", "deployments")

	// Break exactly one group: register an aggregated API whose backing Service
	// does not exist. The apiserver will advertise metrics.k8s.io/v1beta1 and then
	// fail to serve it — a real 503 from the aggregation layer, not a stubbed one.
	broken := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
		"metadata":   map[string]any{"name": "v1beta1.metrics.k8s.io"},
		"spec": map[string]any{
			"group":                 "metrics.k8s.io",
			"version":               "v1beta1",
			"groupPriorityMinimum":  int64(100),
			"versionPriority":       int64(100),
			"insecureSkipTLSVerify": true,
			"service": map[string]any{
				"name":      "metrics-server",
				"namespace": "kube-system",
				"port":      int64(443),
			},
		},
	}}
	if _, err := c.Dynamic.Resource(apiServicesGVR).Create(ctx, broken, metav1.CreateOptions{}); err != nil {
		t.Fatalf("register broken APIService: %v", err)
	}

	// The aggregator needs a moment to notice the service is unreachable, so wait
	// for the group to actually start failing before asserting anything about it.
	// Without this the test would be vacuous: a pass over a cluster that is not
	// yet broken proves nothing about isolation.
	if err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 2*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			c.Invalidate()
			_, err := c.Discovery.ServerResourcesForGroupVersion(brokenAPIServiceGV)
			return err != nil, nil
		}); err != nil {
		t.Fatalf("%s never started failing discovery: %v", brokenAPIServiceGV, err)
	}

	res := discoverFresh(ctx, c)

	// The criterion: one broken group yields a *partial* result, never a total
	// one, and takes nothing healthy down with it. This is the bug the 2020 build
	// had (#87, #76), and it is what this test exists to hold.
	if res.Err != nil {
		t.Fatalf("one broken group failed the whole pass: %v", res.Err)
	}
	requireResource(t, res.Resources, "", "pods")
	requireResource(t, res.Resources, "apps", "deployments")
	if len(res.Resources) < len(base.Resources) {
		t.Errorf("healthy resource set shrank from %d to %d when one group broke", len(base.Resources), len(res.Resources))
	}

	// …and the reporting half, which this test found broken and DISC-01 fixed.
	//
	// DiscoveryResult.Failed must *name* the group that failed, so the menu can mark
	// its kinds unavailable and DIAG-01 can log which group it was. Until D187 it was
	// empty here and on every modern cluster: the apiserver answers *aggregated*
	// discovery, which reports a down group as one entry with no versions plus a
	// "stale GroupVersion" marker carried in a side channel client-go hands only to an
	// AggregatedDiscoveryInterface — which the on-disk cached client kubecom reads
	// through (M1-04) is not, so ServerPreferredResources fell back to walking
	// ServerGroups(), found no version to fetch under the broken group, and returned
	// no error at all. Isolation worked while the reason was unreportable.
	//
	// The fix reads the same group list for groups the server serves no version of.
	// Only the group is knowable — the version was dropped before any cached client
	// saw it — so this asserts the group, not brokenAPIServiceGV. This assertion is
	// the whole point of running against a live apiserver: no fake produces the
	// aggregated shape, which is exactly why the defect survived eleven days of them.
	f, ok := failedGroup(res.Failed, brokenAPIServiceGroup)
	if !ok {
		t.Fatalf("broken group %s is isolated but not named: Failed = %v (DISC-01 regressed)",
			brokenAPIServiceGroup, res.Failed)
	}
	if !errors.Is(f.Err, ErrGroupServesNoVersion) {
		t.Errorf("Failed[%s].Err = %v, want ErrGroupServesNoVersion", brokenAPIServiceGroup, f.Err)
	}
	// Isolation is still the criterion: exactly the broken group is named, and no
	// healthy group is dragged in with it.
	if len(res.Failed) != 1 {
		t.Errorf("Failed = %v, want only %s", res.Failed, brokenAPIServiceGroup)
	}
}

// TestEnvtestRestrictedRBACIsolatesTheDeniedResource proves the other half of
// the criterion: a resource the user may not list degrades that one call, not
// discovery and not the menu.
func TestEnvtestRestrictedRBACIsolatesTheDeniedResource(t *testing.T) {
	env, adminCfg := startControlPlane(t)
	admin := clientsFor(t, adminCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const userName = "kubecom-restricted"

	// A user who may read Pods and nothing else. Secrets are the interesting
	// denial: they are in the seed set and on the menu, so the TUI will offer a
	// row that this user cannot open.
	role, err := admin.Clientset.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "kubecom-pods-only"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cluster role: %v", err)
	}
	if _, err := admin.Clientset.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "kubecom-pods-only"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: userName}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create cluster role binding: %v", err)
	}

	user, err := env.AddUser(envtest.User{Name: userName, Groups: []string{"kubecom-tests"}}, nil)
	if err != nil {
		t.Fatalf("add restricted user: %v", err)
	}
	restricted := clientsFor(t, user.Config())

	pods := res("", "v1", "Pod", "pods", true)
	secrets := res("", "v1", "Secret", "secrets", true)

	// The authorizer reads RBAC through an informer, so the grant lands a moment
	// after the write. Poll the allowed call; everything after it is deterministic.
	if err := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, err := restricted.List(ctx, pods, "default", metav1.ListOptions{})
			return err == nil, nil
		}); err != nil {
		t.Fatalf("granted list on pods never succeeded: %v", err)
	}

	// The denied one fails, and fails *legibly* — KindForbidden is what tells the
	// TUI to degrade one feature rather than treat the cluster as unreachable (#86).
	if _, err := restricted.List(ctx, secrets, "default", metav1.ListOptions{}); err == nil {
		t.Fatal("listing secrets succeeded for a pods-only user")
	} else if got := Classify(err); got != KindForbidden {
		t.Errorf("Classify(list secrets) = %v, want %v (err: %v)", got, KindForbidden, err)
	}

	// And the denial does not reach discovery: the restricted user still gets the
	// full menu, secrets included. Discovery answers "what exists", not "what you
	// may read" — hiding a kind because a list was refused would be the wrong fix.
	d := discoverFresh(ctx, restricted)
	if d.Err != nil {
		t.Fatalf("discovery failed wholesale for a restricted user: %v", d.Err)
	}
	if len(d.Failed) != 0 {
		t.Errorf("restricted RBAC produced failed groups: %v", d.Failed)
	}
	requireResource(t, d.Resources, "", "pods")
	requireResource(t, d.Resources, "", "secrets")
}
