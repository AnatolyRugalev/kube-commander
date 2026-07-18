package kube

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
)

// writeKubeconfig writes a two-context kubeconfig to a temp file and returns its
// path. current-context is ctx-a → https://a.example:6443; ctx-b →
// https://b.example:6443. Hermetic — no server is contacted.
func writeKubeconfig(t *testing.T) string {
	t.Helper()
	const cfg = `apiVersion: v1
kind: Config
current-context: ctx-a
clusters:
- name: cluster-a
  cluster:
    server: https://a.example:6443
- name: cluster-b
  cluster:
    server: https://b.example:6443
contexts:
- name: ctx-a
  context:
    cluster: cluster-a
    user: user-a
- name: ctx-b
  context:
    cluster: cluster-b
    user: user-b
users:
- name: user-a
  user:
    token: token-a
- name: user-b
  user:
    token: token-b
`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func TestRESTConfigCurrentContext(t *testing.T) {
	path := writeKubeconfig(t)
	cfg, err := RESTConfig(ClientConfig{Kubeconfig: path})
	if err != nil {
		t.Fatalf("RESTConfig: %v", err)
	}
	if got, want := cfg.Host, "https://a.example:6443"; got != want {
		t.Fatalf("Host = %q, want %q (current-context)", got, want)
	}
}

func TestRESTConfigContextOverride(t *testing.T) {
	path := writeKubeconfig(t)
	cfg, err := RESTConfig(ClientConfig{Kubeconfig: path, Context: "ctx-b"})
	if err != nil {
		t.Fatalf("RESTConfig: %v", err)
	}
	if got, want := cfg.Host, "https://b.example:6443"; got != want {
		t.Fatalf("Host = %q, want %q (context override)", got, want)
	}
}

func TestRESTConfigUnknownContext(t *testing.T) {
	path := writeKubeconfig(t)
	if _, err := RESTConfig(ClientConfig{Kubeconfig: path, Context: "nope"}); err == nil {
		t.Fatal("RESTConfig with unknown context: want error, got nil")
	}
}

func TestRESTConfigMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := RESTConfig(ClientConfig{Kubeconfig: missing}); err == nil {
		t.Fatal("RESTConfig with missing file: want error, got nil")
	}
}

func TestNewClientsWiring(t *testing.T) {
	// A syntactically valid config is enough; NewClients does no network I/O.
	c, err := NewClients(&rest.Config{Host: "https://localhost:6443"})
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if c.Clientset == nil {
		t.Error("Clientset is nil")
	}
	if c.Dynamic == nil {
		t.Error("Dynamic is nil")
	}
	if c.Discovery == nil {
		t.Error("Discovery is nil")
	}
	if c.RESTMapper == nil {
		t.Error("RESTMapper is nil")
	}
	if c.Config == nil {
		t.Error("Config is nil")
	}
}

func TestNewClientsNilConfig(t *testing.T) {
	if _, err := NewClients(nil); err == nil {
		t.Fatal("NewClients(nil): want error, got nil")
	}
}

func TestConnect(t *testing.T) {
	path := writeKubeconfig(t)
	c, err := Connect(ClientConfig{Kubeconfig: path})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Config.Host != "https://a.example:6443" {
		t.Fatalf("Config.Host = %q, want current-context host", c.Config.Host)
	}
	if c.Clientset == nil || c.Dynamic == nil || c.Discovery == nil || c.RESTMapper == nil {
		t.Fatal("Connect returned incomplete Clients")
	}
}
