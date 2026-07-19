package kube

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// objWithManagedFields builds an unstructured object carrying a managedFields
// entry, so the stripping contract has something to remove.
func objWithManagedFields(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	meta := map[string]any{
		"name": name,
		"managedFields": []any{
			map[string]any{"manager": "kubectl", "operation": "Update"},
		},
	}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   meta,
		"spec":       map[string]any{"replicas": int64(3)},
	}}
}

// TestMarshalYAML asserts the pure renderer: managedFields are stripped, the rest
// of the object is rendered, and the caller's object is left untouched (the strip
// happens on a copy).
func TestMarshalYAML(t *testing.T) {
	obj := objWithManagedFields("apps/v1", "Deployment", "web", "api")

	got, err := marshalYAML(obj)
	if err != nil {
		t.Fatalf("marshalYAML: %v", err)
	}
	if strings.Contains(got, "managedFields") {
		t.Errorf("managedFields not stripped:\n%s", got)
	}
	for _, want := range []string{"kind: Deployment", "name: api", "namespace: web", "replicas: 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("yaml missing %q:\n%s", want, got)
		}
	}
	// The strip must not mutate the caller's object.
	if _, found, _ := unstructured.NestedSlice(obj.Object, "metadata", "managedFields"); !found {
		t.Error("marshalYAML mutated the caller's object: managedFields gone")
	}
}

func TestGetYAML(t *testing.T) {
	dc := newDynamicFake(objWithManagedFields("apps/v1", "Deployment", "web", "api"))
	c := &Clients{Dynamic: dc}

	got, err := c.GetYAML(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("GetYAML: %v", err)
	}
	if strings.Contains(got, "managedFields") {
		t.Errorf("managedFields not stripped from GetYAML output:\n%s", got)
	}
	if !strings.Contains(got, "kind: Deployment") || !strings.Contains(got, "name: api") {
		t.Errorf("GetYAML output missing identity:\n%s", got)
	}
}

// TestGetYAMLClusterScoped proves a cluster-scoped resource is fetched with the
// ref's namespace dropped (r.Namespaced == false).
func TestGetYAMLClusterScoped(t *testing.T) {
	dc := newDynamicFake(objWithManagedFields("v1", "Node", "", "node-1"))
	c := &Clients{Dynamic: dc}

	// A stray namespace on the ref must be ignored for a cluster-scoped resource.
	got, err := c.GetYAML(context.Background(), nodesResource, ObjectRef{Namespace: "ignored", Name: "node-1"})
	if err != nil {
		t.Fatalf("GetYAML node: %v", err)
	}
	if !strings.Contains(got, "kind: Node") || !strings.Contains(got, "name: node-1") {
		t.Errorf("GetYAML node output missing identity:\n%s", got)
	}
}

func TestGetYAMLEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if _, err := c.GetYAML(context.Background(), deploymentsResource, ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("GetYAML with empty name: want error, got nil")
	}
}

func TestGetYAMLNotFound(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	_, err := c.GetYAML(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "ghost"})
	if err == nil {
		t.Fatal("GetYAML on missing object: want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error not a wrapped NotFound: %v", err)
	}
}
