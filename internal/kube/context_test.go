package kube

import (
	"os"
	"path/filepath"
	"testing"
)

// writeKubeconfigContents writes body to a temp kubeconfig file and returns its
// path, so a test can pin the exact contexts it asserts on.
func writeKubeconfigContents(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

// Three contexts declared out of alphabetical order, one with a default
// namespace and one without; current-context is the middle one.
const contextsKubeconfig = `apiVersion: v1
kind: Config
current-context: ctx-b
clusters:
- name: cluster-a
  cluster:
    server: https://a.example:6443
- name: cluster-b
  cluster:
    server: https://b.example:6443
contexts:
- name: ctx-c
  context:
    cluster: cluster-b
    user: user-b
    namespace: kube-system
- name: ctx-a
  context:
    cluster: cluster-a
    user: user-a
- name: ctx-b
  context:
    cluster: cluster-b
    user: user-b
    namespace: apps
users:
- name: user-a
  user:
    token: token-a
- name: user-b
  user:
    token: token-b
`

func TestContextsListsSortedWithFields(t *testing.T) {
	path := writeKubeconfigContents(t, contextsKubeconfig)
	got, err := Contexts(ClientConfig{Kubeconfig: path})
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}
	want := []ContextInfo{
		{Name: "ctx-a", Cluster: "cluster-a", Namespace: "", Current: false},
		{Name: "ctx-b", Cluster: "cluster-b", Namespace: "apps", Current: true},
		{Name: "ctx-c", Cluster: "cluster-b", Namespace: "kube-system", Current: false},
	}
	if len(got) != len(want) {
		t.Fatalf("Contexts returned %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestContextsCurrentFollowsOverride(t *testing.T) {
	path := writeKubeconfigContents(t, contextsKubeconfig)
	got, err := Contexts(ClientConfig{Kubeconfig: path, Context: "ctx-c"})
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}
	// The override selects the context Connect would use, so it — not the
	// file's current-context — is the one marked Current.
	var current []string
	for _, c := range got {
		if c.Current {
			current = append(current, c.Name)
		}
	}
	if len(current) != 1 || current[0] != "ctx-c" {
		t.Fatalf("current contexts = %v, want exactly [ctx-c]", current)
	}
}

func TestContextsUnknownOverrideMarksNoneCurrent(t *testing.T) {
	path := writeKubeconfigContents(t, contextsKubeconfig)
	got, err := Contexts(ClientConfig{Kubeconfig: path, Context: "nope"})
	if err != nil {
		t.Fatalf("Contexts with unknown override: unexpected error %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d contexts, want 3", len(got))
	}
	for _, c := range got {
		if c.Current {
			t.Errorf("context %q marked current, want none (override names no declared context)", c.Name)
		}
	}
}

func TestContextsNoContextsDeclared(t *testing.T) {
	path := writeKubeconfigContents(t, "apiVersion: v1\nkind: Config\n")
	got, err := Contexts(ClientConfig{Kubeconfig: path})
	if err != nil {
		t.Fatalf("Contexts on empty config: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d contexts, want 0: %+v", len(got), got)
	}
}

func TestContextsMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := Contexts(ClientConfig{Kubeconfig: missing})
	if err == nil {
		t.Fatal("Contexts with missing kubeconfig: want error, got nil")
	}
	if got := Classify(err); got != KindBadContext {
		t.Fatalf("Classify = %v, want KindBadContext", got)
	}
}

func TestContextsMalformedFile(t *testing.T) {
	path := writeKubeconfigContents(t, "\tthis: is: not: valid: yaml\n")
	_, err := Contexts(ClientConfig{Kubeconfig: path})
	if err == nil {
		t.Fatal("Contexts with malformed kubeconfig: want error, got nil")
	}
	if got := Classify(err); got != KindBadContext {
		t.Fatalf("Classify = %v, want KindBadContext", got)
	}
}

// The list's names are exactly what ClientConfig.Context accepts: each one round
// trips through RESTConfig, and the entry marked Current is the one ContextName
// reports for the same ClientConfig.
func TestContextsNamesRoundTrip(t *testing.T) {
	path := writeKubeconfigContents(t, contextsKubeconfig)
	cc := ClientConfig{Kubeconfig: path}
	got, err := Contexts(cc)
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}
	for _, c := range got {
		if _, err := RESTConfig(ClientConfig{Kubeconfig: path, Context: c.Name}); err != nil {
			t.Errorf("RESTConfig(context %q): %v", c.Name, err)
		}
		if c.Current && c.Name != ContextName(cc) {
			t.Errorf("Current entry %q disagrees with ContextName %q", c.Name, ContextName(cc))
		}
	}
}
