package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/describe"
)

// describeConfig is a throwaway rest.Config for the describer-selection tests.
// The describers build their clients from it but make no server call during
// construction, so the host is never dialed and the tests stay hermetic.
var describeConfig = &rest.Config{Host: "https://example.invalid"}

// podDescribeResource / crdDescribeResource carry the GVK describerFor keys on
// (the shared actions_test fixtures set only GVR/scope). Pods have a specialized
// built-in describer; the fictional CRD kind has none, so it exercises the generic
// fallback.
var (
	podDescribeResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Namespaced: true,
	}
	crdDescribeResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR:        schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
		Namespaced: true,
	}
)

// TestRestMappingFor asserts the pure mapping builder: GVR/GVK are carried through
// and the scope reflects Namespaced.
func TestRestMappingFor(t *testing.T) {
	m := restMappingFor(podDescribeResource)
	if m.Resource != podDescribeResource.GVR {
		t.Errorf("Resource = %v, want %v", m.Resource, podDescribeResource.GVR)
	}
	if m.GroupVersionKind != podDescribeResource.GVK {
		t.Errorf("GroupVersionKind = %v, want %v", m.GroupVersionKind, podDescribeResource.GVK)
	}
	if got := m.Scope.Name(); got != meta.RESTScopeNameNamespace {
		t.Errorf("namespaced scope = %q, want %q", got, meta.RESTScopeNameNamespace)
	}

	clusterScoped := Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
		Namespaced: false,
	}
	if got := restMappingFor(clusterScoped).Scope.Name(); got != meta.RESTScopeNameRoot {
		t.Errorf("cluster-scoped scope = %q, want %q", got, meta.RESTScopeNameRoot)
	}
}

// TestDescriberForBuiltin proves a built-in kind resolves to its specialized
// describer (the PodDescriber), not the generic fallback.
func TestDescriberForBuiltin(t *testing.T) {
	d, err := describerFor(podDescribeResource, describeConfig)
	if err != nil {
		t.Fatalf("describerFor(pod): %v", err)
	}
	if _, ok := d.(*describe.PodDescriber); !ok {
		t.Errorf("pod describer = %T, want *describe.PodDescriber", d)
	}
}

// TestDescriberForGeneric proves a kind with no built-in describer (a CRD) falls
// back to the generic unstructured describer — non-nil, and not the built-in path.
func TestDescriberForGeneric(t *testing.T) {
	d, err := describerFor(crdDescribeResource, describeConfig)
	if err != nil {
		t.Fatalf("describerFor(crd): %v", err)
	}
	if d == nil {
		t.Fatal("describerFor(crd) = nil, want a generic describer")
	}
	if _, ok := d.(*describe.PodDescriber); ok {
		t.Error("crd resolved to the built-in PodDescriber, want the generic describer")
	}
}

// TestDescribeEmptyName asserts the empty-name guard fires before any describer is
// constructed.
func TestDescribeEmptyName(t *testing.T) {
	c := &Clients{Config: describeConfig}
	if _, err := c.Describe(podDescribeResource, ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("Describe with empty name: want error, got nil")
	}
}
