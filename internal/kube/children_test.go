package kube

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// Children resolves the *child* kind out of the caller's available set, so these
// fixtures carry a GVK (the shared actions_test.go ones are GVR-only) and the pod
// entry carries verbs, to pin that the real discovered Resource is handed back
// rather than a synthesized stub.
var (
	childPodResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Namespaced: true,
		Verbs:      metav1.Verbs{"get", "list", "watch", "delete"},
	}
	childDeploymentResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		GVR:        schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		Namespaced: true,
	}
	childStatefulSetResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		GVR:        schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"},
		Namespaced: true,
	}
	childServiceResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
		Namespaced: true,
	}
	childNodeResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
		Namespaced: false,
	}
	childConfigMapResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		Namespaced: true,
	}
)

// childKinds is the "available resource set" a caller passes in — the menu's
// discovery result, in miniature.
var childKinds = []Resource{childPodResource, childDeploymentResource, childStatefulSetResource, childServiceResource, childNodeResource, childConfigMapResource}

// childDynamicFake seeds a fake dynamic client that knows every GVR the
// selector-based owners in these tests are fetched through.
func childDynamicFake(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		childPodResource.GVR:         "PodList",
		childDeploymentResource.GVR:  "DeploymentList",
		childStatefulSetResource.GVR: "StatefulSetList",
		childServiceResource.GVR:     "ServiceList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
}

// unstructuredServiceWithSelector builds a Service whose spec.selector is a flat
// label map — the second of the two on-the-wire selector shapes. It is the
// unstructured twin of podresolve_test.go's typed serviceWithSelector, because
// Children fetches every selector-based owner through the dynamic client.
func unstructuredServiceWithSelector(ns, name string, selector map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"namespace": ns, "name": name},
		"spec":       map[string]any{"selector": selector},
	}}
}

// TestChildrenDeploymentLabelSelector is the common path: a workload's
// spec.selector becomes the scope's label selector, scoped to the owner's own
// namespace, and the child Resource is the one from the caller's set — verbs and
// all — not a stub.
func TestChildrenDeploymentLabelSelector(t *testing.T) {
	dep := workloadWithSelector("apps/v1", "Deployment", "web", "api", map[string]any{"app": "api"})
	c := &Clients{Dynamic: childDynamicFake(dep)}

	scope, err := c.Children(context.Background(), childDeploymentResource, ObjectRef{Namespace: "web", Name: "api"}, childKinds)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if scope.Resource.GVR != childPodResource.GVR {
		t.Fatalf("child resource = %v, want pods", scope.Resource.GVR)
	}
	if len(scope.Resource.Verbs) != len(childPodResource.Verbs) {
		t.Fatalf("child verbs = %v, want the discovered pod verbs %v", scope.Resource.Verbs, childPodResource.Verbs)
	}
	if scope.Namespace != "web" {
		t.Fatalf("namespace = %q, want the owner's namespace %q", scope.Namespace, "web")
	}
	if scope.Options.LabelSelector != "app=api" {
		t.Fatalf("label selector = %q, want %q", scope.Options.LabelSelector, "app=api")
	}
	if scope.Options.FieldSelector != "" {
		t.Fatalf("field selector = %q, want empty for a selector-based owner", scope.Options.FieldSelector)
	}
	if got := scope.Selector(); got != "app=api" {
		t.Fatalf("Selector() = %q, want the label selector", got)
	}
}

// TestChildrenMatchExpressionsSelector pins that the richer LabelSelector shape
// (matchExpressions, which no flat map can express) survives into the scope.
func TestChildrenMatchExpressionsSelector(t *testing.T) {
	sts := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata":   map[string]any{"namespace": "db", "name": "pg"},
		"spec": map[string]any{"selector": map[string]any{
			"matchExpressions": []any{map[string]any{
				"key": "tier", "operator": "In", "values": []any{"primary", "replica"},
			}},
		}},
	}}
	c := &Clients{Dynamic: childDynamicFake(sts)}

	scope, err := c.Children(context.Background(), childStatefulSetResource, ObjectRef{Namespace: "db", Name: "pg"}, childKinds)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if !strings.Contains(scope.Options.LabelSelector, "tier in (primary,replica)") {
		t.Fatalf("label selector = %q, want a set-based tier term", scope.Options.LabelSelector)
	}
}

// TestChildrenServiceFlatSelector covers the flat-label-map selector shape and the
// deliberate inclusion of Service, which selects pods without owning them.
func TestChildrenServiceFlatSelector(t *testing.T) {
	svc := unstructuredServiceWithSelector("web", "api", map[string]any{"app": "api"})
	c := &Clients{Dynamic: childDynamicFake(svc)}

	scope, err := c.Children(context.Background(), childServiceResource, ObjectRef{Namespace: "web", Name: "api"}, childKinds)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if scope.Options.LabelSelector != "app=api" {
		t.Fatalf("label selector = %q, want %q", scope.Options.LabelSelector, "app=api")
	}
	if scope.Namespace != "web" {
		t.Fatalf("namespace = %q, want %q", scope.Namespace, "web")
	}
}

// TestChildrenNodeFieldSelectorCostsNoRequest is the Node path: the scope is a
// spec.nodeName field selector across *every* namespace, derived entirely from the
// ref. The nil Dynamic client is the assertion — any API call would panic.
func TestChildrenNodeFieldSelectorCostsNoRequest(t *testing.T) {
	c := &Clients{}

	scope, err := c.Children(context.Background(), childNodeResource, ObjectRef{Name: "node-1"}, childKinds)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if scope.Namespace != "" {
		t.Fatalf("namespace = %q, want all-namespaces for a cluster-scoped owner", scope.Namespace)
	}
	if scope.Options.FieldSelector != "spec.nodeName=node-1" {
		t.Fatalf("field selector = %q, want %q", scope.Options.FieldSelector, "spec.nodeName=node-1")
	}
	if scope.Options.LabelSelector != "" {
		t.Fatalf("label selector = %q, want empty for the Node path", scope.Options.LabelSelector)
	}
	if got := scope.Selector(); got != "spec.nodeName=node-1" {
		t.Fatalf("Selector() = %q, want the field selector", got)
	}
}

// TestChildrenUnsupportedKind: a kind with no child link is an error, not an empty
// scope that would list every pod.
func TestChildrenUnsupportedKind(t *testing.T) {
	c := &Clients{Dynamic: childDynamicFake()}

	_, err := c.Children(context.Background(), childConfigMapResource, ObjectRef{Namespace: "web", Name: "cfg"}, childKinds)
	if err == nil {
		t.Fatal("Children on a ConfigMap: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "no child resources") {
		t.Fatalf("error = %v, want it to name the missing child link", err)
	}
}

// TestChildrenEmptyName guards the ref before any request is made.
func TestChildrenEmptyName(t *testing.T) {
	c := &Clients{}

	if _, err := c.Children(context.Background(), childDeploymentResource, ObjectRef{Namespace: "web"}, childKinds); err == nil {
		t.Fatal("Children with an empty name: want an error, got nil")
	}
}

// TestChildrenChildKindUnavailable: a cluster (or an RBAC scope) that does not
// expose pods has no child scope, and says so rather than inventing a Resource.
func TestChildrenChildKindUnavailable(t *testing.T) {
	dep := workloadWithSelector("apps/v1", "Deployment", "web", "api", map[string]any{"app": "api"})
	c := &Clients{Dynamic: childDynamicFake(dep)}
	without := []Resource{childDeploymentResource, childServiceResource}

	_, err := c.Children(context.Background(), childDeploymentResource, ObjectRef{Namespace: "web", Name: "api"}, without)
	if err == nil {
		t.Fatal("Children without pods in the available set: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want it to say the child kind is unavailable", err)
	}
}

// TestChildrenOwnerGone: the owner disappearing between the row render and the
// drill-down is a wrapped error, not a panic.
func TestChildrenOwnerGone(t *testing.T) {
	c := &Clients{Dynamic: childDynamicFake()}

	_, err := c.Children(context.Background(), childDeploymentResource, ObjectRef{Namespace: "web", Name: "api"}, childKinds)
	if err == nil {
		t.Fatal("Children for a missing owner: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "getting deployments") {
		t.Fatalf("error = %v, want it to name the failed Get", err)
	}
}

// TestChildrenSelectorlessService: a headless/ExternalName Service has no
// spec.selector, so there is nothing to scope by — and no fallback to "everything".
func TestChildrenSelectorlessService(t *testing.T) {
	svc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"namespace": "web", "name": "external"},
		"spec":       map[string]any{"externalName": "example.com"},
	}}
	c := &Clients{Dynamic: childDynamicFake(svc)}

	_, err := c.Children(context.Background(), childServiceResource, ObjectRef{Namespace: "web", Name: "external"}, childKinds)
	if err == nil {
		t.Fatal("Children for a selector-less Service: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "child selector") {
		t.Fatalf("error = %v, want it to name the selector failure", err)
	}
}

// TestChildrenEmptySelectorRefused is the important negative: a spec.selector that
// is *present* but selects everything must not become a scope, because the
// resulting table would claim to show one owner's pods while showing all of them.
func TestChildrenEmptySelectorRefused(t *testing.T) {
	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": "web", "name": "api"},
		"spec":       map[string]any{"selector": map[string]any{"matchLabels": map[string]any{}, "matchExpressions": []any{}}},
	}}
	c := &Clients{Dynamic: childDynamicFake(dep)}

	_, err := c.Children(context.Background(), childDeploymentResource, ObjectRef{Namespace: "web", Name: "api"}, childKinds)
	if err == nil {
		t.Fatal("Children for a match-everything selector: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "matches everything") {
		t.Fatalf("error = %v, want the match-everything refusal", err)
	}
}

// TestHasChildren is the cheap predicate the drill-down action gates on: pure, no
// I/O, and exactly the set childLinks declares.
func TestHasChildren(t *testing.T) {
	tests := []struct {
		res  Resource
		want bool
	}{
		{childDeploymentResource, true},
		{childStatefulSetResource, true},
		{childServiceResource, true},
		{childNodeResource, true},
		{Resource{GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}}, true},
		{Resource{GVK: schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}}, true},
		{Resource{GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ReplicationController"}}, true},
		{childPodResource, false},
		{childConfigMapResource, false},
		{Resource{GVK: schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}}, false},
		{Resource{GVK: schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Widget"}}, false},
		{Resource{}, false},
	}
	for _, tt := range tests {
		if got := HasChildren(tt.res); got != tt.want {
			t.Errorf("HasChildren(%s/%s) = %v, want %v", tt.res.GVK.Group, tt.res.GVK.Kind, got, tt.want)
		}
	}
}

// TestChildScopeSelectorPrecedence pins the display helper: the label selector
// wins when both are somehow set, the field selector is the fallback, and the zero
// scope is empty rather than a stray "=".
func TestChildScopeSelectorPrecedence(t *testing.T) {
	both := ChildScope{Options: metav1.ListOptions{LabelSelector: "app=api", FieldSelector: "spec.nodeName=n1"}}
	if got := both.Selector(); got != "app=api" {
		t.Fatalf("Selector() = %q, want the label selector to win", got)
	}
	if got := (ChildScope{}).Selector(); got != "" {
		t.Fatalf("zero Selector() = %q, want empty", got)
	}
}

// TestChildrenPrefersDiscoveredVersion: the child is matched by GroupKind, so a
// cluster serving a non-default preferred version still resolves — the caller's
// discovery result is authoritative, not a hard-coded GroupVersionKind.
func TestChildrenPrefersDiscoveredVersion(t *testing.T) {
	odd := Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v2", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v2", Resource: "pods"},
		Namespaced: true,
	}
	c := &Clients{}

	scope, err := c.Children(context.Background(), childNodeResource, ObjectRef{Name: "node-1"}, []Resource{odd})
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if scope.Resource.GVR.Version != "v2" {
		t.Fatalf("child GVR = %v, want the caller's preferred version", scope.Resource.GVR)
	}
}
