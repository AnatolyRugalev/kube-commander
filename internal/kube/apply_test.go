package kube

import (
	"context"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// editedDeploymentYAML renders the YAML a user would save out of $EDITOR for a
// Deployment: name/namespace identity plus an edited replica count and a marker
// label, so an Update round-trip has an observable change to assert.
func editedDeploymentYAML(namespace, name string, replicas int) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: %s
  name: %s
  labels:
    edited: "yes"
spec:
  replicas: %d
`, namespace, name, replicas)
}

func TestUpdateAppliesEditedObject(t *testing.T) {
	dc := newDynamicFake(deploymentObj("web", "api", 1, nil))
	c := &Clients{Dynamic: dc}

	edited := []byte(editedDeploymentYAML("web", "api", 3))
	if err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, edited); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := getDeployment(t, dc, "web", "api")
	replicas, found, err := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if err != nil || !found {
		t.Fatalf("spec.replicas missing after Update (found=%v err=%v)", found, err)
	}
	if replicas != 3 {
		t.Errorf("spec.replicas = %d, want 3 (int64, not float64)", replicas)
	}
	labels, _, _ := unstructured.NestedStringMap(got.Object, "metadata", "labels")
	if labels["edited"] != "yes" {
		t.Errorf("metadata.labels[edited] = %q, want %q", labels["edited"], "yes")
	}
}

func TestUpdateClusterScopedFillsNamespaceFromRef(t *testing.T) {
	node := unstructuredObj("v1", "Node", "", "worker-1", "uid-1")
	dc := newDynamicFake(node)
	c := &Clients{Dynamic: dc}

	// A cluster-scoped edit carries no namespace; the ref's namespace is ignored.
	edited := []byte("apiVersion: v1\nkind: Node\nmetadata:\n  name: worker-1\n  labels:\n    edited: \"yes\"\n")
	if err := c.Update(context.Background(), nodesResource, ObjectRef{Name: "worker-1"}, edited); err != nil {
		t.Fatalf("Update(cluster-scoped): %v", err)
	}

	got, err := dc.Resource(nodesResource.GVR).Get(context.Background(), "worker-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(got.Object, "metadata", "labels")
	if labels["edited"] != "yes" {
		t.Errorf("node labels[edited] = %q, want %q", labels["edited"], "yes")
	}
}

func TestUpdateInvalidYAMLRejected(t *testing.T) {
	dc := newDynamicFake(deploymentObj("web", "api", 1, nil))
	c := &Clients{Dynamic: dc}

	err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, []byte("\tnot: [valid"))
	if err == nil {
		t.Fatal("Update(invalid yaml): want error, got nil")
	}
	// A parse failure must not mutate the object.
	got := getDeployment(t, dc, "web", "api")
	replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if replicas != 1 {
		t.Errorf("spec.replicas = %d after rejected edit, want 1 (untouched)", replicas)
	}
}

func TestUpdateEmptyContentRejected(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake(deploymentObj("web", "api", 1, nil))}
	for _, content := range []string{"", "   \n", "null\n"} {
		if err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, []byte(content)); err == nil {
			t.Errorf("Update(empty %q): want error, got nil", content)
		}
	}
}

func TestUpdateNameMismatchRejected(t *testing.T) {
	dc := newDynamicFake(deploymentObj("web", "api", 1, nil))
	c := &Clients{Dynamic: dc}

	// The buffer was renamed — must be refused before any request (not a rename).
	edited := []byte(editedDeploymentYAML("web", "renamed", 3))
	if err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, edited); err == nil {
		t.Fatal("Update(name mismatch): want error, got nil")
	}
	got := getDeployment(t, dc, "web", "api")
	replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if replicas != 1 {
		t.Errorf("spec.replicas = %d after rejected rename, want 1 (untouched)", replicas)
	}
}

func TestUpdateNamespaceMismatchRejected(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake(deploymentObj("web", "api", 1, nil))}
	edited := []byte(editedDeploymentYAML("other", "api", 3))
	if err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, edited); err == nil {
		t.Fatal("Update(namespace mismatch): want error, got nil")
	}
}

func TestUpdateNoNameRejected(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake(deploymentObj("web", "api", 1, nil))}
	edited := []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  namespace: web\nspec:\n  replicas: 3\n")
	if err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, edited); err == nil {
		t.Fatal("Update(no name): want error, got nil")
	}
}

func TestUpdateNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	edited := []byte(editedDeploymentYAML("web", "ghost", 3))
	err := c.Update(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "ghost"}, edited)
	if err == nil {
		t.Fatal("Update(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}
