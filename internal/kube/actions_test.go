package kube

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	podsResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Namespaced: true,
	}
	nodesResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
		Namespaced: false,
	}
)

func unstructuredObj(apiVersion, kind, namespace, name, uid string) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "uid": uid}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   meta,
	}}
}

// newDynamicFake returns a fake dynamic client seeded with the given objects and
// the list-kind mapping the pods/nodes tests need. Custom list kinds are supplied
// explicitly so the fake never has to guess them from a scheme.
func newDynamicFake(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		podsResource.GVR:  "PodList",
		nodesResource.GVR: "NodeList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
}

func exists(t *testing.T, dc *dynamicfake.FakeDynamicClient, r Resource, ns, name string) bool {
	t.Helper()
	ri := dc.Resource(r.GVR)
	var err error
	if r.Namespaced {
		_, err = ri.Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	} else {
		_, err = ri.Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return err == nil
}

// withUIDPrecondition owns the object-identity guard; the fake dynamic client
// discards DeleteOptions (see its Delete: NewDeleteAction takes no opts), so the
// precondition logic is verified here as a pure function, and the fake exercises
// the round-trip (removal, scoping, errors) below.
func TestWithUIDPrecondition(t *testing.T) {
	rv := "12345"

	// UID present, no caller preconditions → a UID precondition is added.
	got := withUIDPrecondition(ObjectRef{Name: "p", UID: "uid-1"}, metav1.DeleteOptions{})
	if got.Preconditions == nil || got.Preconditions.UID == nil || string(*got.Preconditions.UID) != "uid-1" {
		t.Errorf("UID guard = %+v, want uid-1", got.Preconditions)
	}

	// No UID (degraded metadata) → opts unchanged, no precondition invented.
	got = withUIDPrecondition(ObjectRef{Name: "p"}, metav1.DeleteOptions{})
	if got.Preconditions != nil {
		t.Errorf("preconditions = %+v, want nil when ref has no UID", got.Preconditions)
	}

	// Caller already set preconditions → they win; no UID is layered on.
	got = withUIDPrecondition(
		ObjectRef{Name: "p", UID: "uid-1"},
		metav1.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &rv}},
	)
	if got.Preconditions == nil || got.Preconditions.ResourceVersion == nil || *got.Preconditions.ResourceVersion != "12345" {
		t.Errorf("ResourceVersion precondition = %+v, want preserved 12345", got.Preconditions)
	}
	if got.Preconditions.UID != nil {
		t.Errorf("UID = %v, want nil (caller preconditions preserved)", *got.Preconditions.UID)
	}
}

func TestDeleteNamespacedRemovesObject(t *testing.T) {
	dc := newDynamicFake(unstructuredObj("v1", "Pod", "web", "nginx-abc", "uid-1"))

	var captured clienttesting.Action
	dc.PrependReactor("delete", "pods", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil // observe only; let the tracker perform the delete
	})

	c := &Clients{Dynamic: dc}
	ref := ObjectRef{Namespace: "web", Name: "nginx-abc", UID: "uid-1"}
	if err := c.Delete(context.Background(), podsResource, ref, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists(t, dc, podsResource, "web", "nginx-abc") {
		t.Error("pod still present after Delete")
	}
	if captured == nil {
		t.Fatal("no delete action recorded")
	}
	if got := captured.GetNamespace(); got != "web" {
		t.Errorf("namespace = %q, want web", got)
	}
}

func TestDeleteClusterScopedIgnoresNamespace(t *testing.T) {
	dc := newDynamicFake(unstructuredObj("v1", "Node", "", "node-1", "uid-n"))

	var captured clienttesting.Action
	dc.PrependReactor("delete", "nodes", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil
	})

	c := &Clients{Dynamic: dc}
	// A namespace on the ref must be ignored for a cluster-scoped resource.
	ref := ObjectRef{Namespace: "ignored", Name: "node-1", UID: "uid-n"}
	if err := c.Delete(context.Background(), nodesResource, ref, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists(t, dc, nodesResource, "", "node-1") {
		t.Error("node still present after Delete")
	}
	if captured == nil {
		t.Fatal("no delete action recorded")
	}
	if got := captured.GetNamespace(); got != "" {
		t.Errorf("namespace = %q, want empty (cluster-scoped)", got)
	}
}

func TestDeleteEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.Delete(context.Background(), podsResource, ObjectRef{Namespace: "web"}, metav1.DeleteOptions{}); err == nil {
		t.Fatal("Delete(empty name): want error, got nil")
	}
}

func TestDeleteNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	ref := ObjectRef{Namespace: "web", Name: "ghost", UID: "uid-x"}
	err := c.Delete(context.Background(), podsResource, ref, metav1.DeleteOptions{})
	if err == nil {
		t.Fatal("Delete(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}
