package kube

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func ns(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// TestNamespacesSorted lists namespaces from a fake cluster and returns them
// sorted, regardless of the order the API server hands them back.
func TestNamespacesSorted(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(
		ns("monitoring"), ns("default"), ns("kube-system"),
	)}
	got, err := c.Namespaces(context.Background())
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	want := []string{"default", "kube-system", "monitoring"}
	if len(got) != len(want) {
		t.Fatalf("Namespaces() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Namespaces() = %v, want %v", got, want)
		}
	}
}

// TestNamespacesEmpty returns an empty slice and no error on a namespace-less
// cluster (never nil-panics).
func TestNamespacesEmpty(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}
	got, err := c.Namespaces(context.Background())
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Namespaces() = %v, want empty", got)
	}
}

// TestNamespacesError wraps a list failure (e.g. RBAC denial) rather than
// panicking, so the caller can degrade gracefully (principle 3).
func TestNamespacesError(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	c := &Clients{Clientset: cs}
	if _, err := c.Namespaces(context.Background()); err == nil {
		t.Fatal("Namespaces should return an error when the list fails")
	}
}
