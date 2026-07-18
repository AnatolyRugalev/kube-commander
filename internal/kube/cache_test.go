package kube

import (
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

func TestComputeDiscoverCacheDir(t *testing.T) {
	const parent = "/cache/discovery"
	cases := []struct {
		name string
		host string
		want string
	}{
		{"https host with port", "https://a.example:6443", filepath.Join(parent, "a.example_6443")},
		{"http host with port", "http://b.example:8080", filepath.Join(parent, "b.example_8080")},
		{"schemeless host", "c.example:6443", filepath.Join(parent, "c.example_6443")},
		{"ip host", "https://127.0.0.1:6443", filepath.Join(parent, "127.0.0.1_6443")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeDiscoverCacheDir(parent, tc.host)
			if got != tc.want {
				t.Fatalf("computeDiscoverCacheDir(%q, %q) = %q, want %q", parent, tc.host, got, tc.want)
			}
			// The host component must never contain a path separator or scheme
			// punctuation that would escape the parent directory.
			base := filepath.Base(got)
			for _, bad := range []string{":", "/", "https", "http"} {
				if strings.Contains(base, bad) {
					t.Fatalf("slugged host %q still contains %q", base, bad)
				}
			}
		})
	}
}

func TestComputeDiscoverCacheDirPerHost(t *testing.T) {
	// Two different clusters must map to two different cache directories, or one
	// cluster's discovery cache would be served to the other.
	a := computeDiscoverCacheDir("/cache", "https://a.example:6443")
	b := computeDiscoverCacheDir("/cache", "https://b.example:6443")
	if a == b {
		t.Fatalf("distinct hosts shared a cache dir: %q", a)
	}
}

func TestNewCachedDiscoveryImplementsInterface(t *testing.T) {
	// Construction does no network I/O, so a dummy config is enough. The result
	// must satisfy CachedDiscoveryInterface (Invalidate/Fresh) — that is what the
	// deferred RESTMapper and Clients.Invalidate rely on.
	c, err := newCachedDiscovery(&rest.Config{Host: "https://localhost:6443"})
	if err != nil {
		t.Fatalf("newCachedDiscovery: %v", err)
	}
	if c == nil {
		t.Fatal("newCachedDiscovery returned nil client")
	}
	// Invalidate must be safe to call before any discovery has happened.
	c.Invalidate()
}

func TestClientsInvalidate(t *testing.T) {
	c, err := NewClients(&rest.Config{Host: "https://localhost:6443"})
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if c.deferredMapper == nil {
		t.Fatal("deferredMapper not retained; Invalidate cannot reset the mapper")
	}
	// No network is reachable; Invalidate must not panic and must not require one.
	c.Invalidate()
}
