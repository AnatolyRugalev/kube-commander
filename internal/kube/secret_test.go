package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// secretWith builds a fake Secret carrying the given (already-decoded) data.
func secretWith(namespace, name, secretType string, data map[string]string) *corev1.Secret {
	d := make(map[string][]byte, len(data))
	for k, v := range data {
		d[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Type:       corev1.SecretType(secretType),
		Data:       d,
	}
}

// TestSecretDataDecodesAndSorts proves SecretData returns the type and the decoded
// values, keys sorted for a deterministic render.
func TestSecretDataDecodesAndSorts(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(
		secretWith("web", "creds", "Opaque", map[string]string{"password": "s3cr3t", "username": "admin"}),
	)}

	got, err := c.SecretData(context.Background(), ObjectRef{Namespace: "web", Name: "creds"})
	if err != nil {
		t.Fatalf("SecretData: %v", err)
	}
	if got.Type != "Opaque" {
		t.Errorf("type = %q, want Opaque", got.Type)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	// Sorted by key: password before username.
	if got.Entries[0].Key != "password" || got.Entries[0].Value != "s3cr3t" {
		t.Errorf("entry[0] = %+v, want {password s3cr3t}", got.Entries[0])
	}
	if got.Entries[1].Key != "username" || got.Entries[1].Value != "admin" {
		t.Errorf("entry[1] = %+v, want {username admin}", got.Entries[1])
	}
}

// TestSecretDataEmpty proves a secret with no data yields an empty (non-nil) entry
// set and no error — the viewer shows "no data" rather than failing.
func TestSecretDataEmpty(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(
		secretWith("web", "blank", "Opaque", nil),
	)}
	got, err := c.SecretData(context.Background(), ObjectRef{Namespace: "web", Name: "blank"})
	if err != nil {
		t.Fatalf("SecretData: %v", err)
	}
	if len(got.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(got.Entries))
	}
}

func TestSecretDataEmptyName(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}
	if _, err := c.SecretData(context.Background(), ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("SecretData with empty name: want error, got nil")
	}
}

func TestSecretDataNotFound(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}
	_, err := c.SecretData(context.Background(), ObjectRef{Namespace: "web", Name: "ghost"})
	if err == nil {
		t.Fatal("SecretData on missing secret: want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error not a wrapped NotFound: %v", err)
	}
}
