package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// TestSeedRESTMapperMappings checks that every seeded kind resolves to the exact
// resource and scope kubectl uses, with no network I/O (the mapper is static).
func TestSeedRESTMapperMappings(t *testing.T) {
	m := newSeedRESTMapper()
	for _, r := range seedResources {
		r := r
		t.Run(r.GVK.Kind, func(t *testing.T) {
			mapping, err := m.RESTMapping(r.GVK.GroupKind(), r.GVK.Version)
			if err != nil {
				t.Fatalf("RESTMapping(%s): %v", r.GVK, err)
			}
			if got := mapping.Resource; got.Resource != r.Plural || got.Group != r.GVK.Group || got.Version != r.GVK.Version {
				t.Errorf("Resource = %v, want %s/%s/%s", got, r.GVK.Group, r.GVK.Version, r.Plural)
			}
			wantScope := meta.RESTScopeNameNamespace
			if !r.Namespaced {
				wantScope = meta.RESTScopeNameRoot
			}
			if got := mapping.Scope.Name(); got != wantScope {
				t.Errorf("Scope = %q, want %q", got, wantScope)
			}
		})
	}
}

// TestSeedRESTMapperResourceRoundTrip verifies GVR→GVK resolution for a couple of
// representative kinds, including the irregular plural NetworkPolicy →
// networkpolicies that heuristic pluralization would get wrong.
func TestSeedRESTMapperResourceRoundTrip(t *testing.T) {
	m := newSeedRESTMapper()
	cases := []struct {
		gvr schema.GroupVersionResource
		gvk schema.GroupVersionKind
	}{
		{schema.GroupVersionResource{Version: "v1", Resource: "pods"}, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}},
		{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		{schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}},
	}
	for _, c := range cases {
		gvk, err := m.KindFor(c.gvr)
		if err != nil {
			t.Fatalf("KindFor(%v): %v", c.gvr, err)
		}
		if gvk != c.gvk {
			t.Errorf("KindFor(%v) = %v, want %v", c.gvr, gvk, c.gvk)
		}
	}
}

// TestSeedRESTMapperUnknownKind confirms the seed mapper does not invent mappings:
// a kind outside the seed set returns a no-match error rather than a guess. In the
// composed mapper (NewClients) such kinds fall through to discovery instead.
func TestSeedRESTMapperUnknownKind(t *testing.T) {
	m := newSeedRESTMapper()
	_, err := m.RESTMapping(schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	if err == nil {
		t.Fatal("RESTMapping for unseeded kind: want error, got nil")
	}
	if !meta.IsNoMatchError(err) {
		t.Errorf("err = %v, want a no-match error", err)
	}
}

// TestClientsMapperResolvesSeedWithoutServer is the "instant start" guarantee:
// the composed RESTMapper on a Clients built from an unreachable dummy config can
// still map core resources, because the seed mapper short-circuits ahead of the
// discovery mapper — no server round-trip occurs.
func TestClientsMapperResolvesSeedWithoutServer(t *testing.T) {
	c, err := NewClients(&rest.Config{Host: "https://unreachable.invalid:6443"})
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	mapping, err := c.RESTMapper.RESTMapping(schema.GroupKind{Kind: "Pod"}, "v1")
	if err != nil {
		t.Fatalf("RESTMapping(Pod) via composed mapper: %v", err)
	}
	if mapping.Resource.Resource != "pods" {
		t.Errorf("Resource = %q, want pods", mapping.Resource.Resource)
	}
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		t.Errorf("Scope = %q, want namespace", mapping.Scope.Name())
	}
}
