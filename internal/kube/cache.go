package kube

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/client-go/discovery"
	diskcached "k8s.io/client-go/discovery/cached/disk"
	memcache "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
)

// discoveryCacheTTL bounds how long an on-disk discovery document is trusted
// before it is refetched from the server. 6h matches kubectl's default: long
// enough that the common case (unchanged API surface) never re-hits the server,
// short enough that a newly-installed API group is picked up within a working
// session without an explicit refresh. A forced refresh (Clients.Invalidate)
// bypasses the TTL entirely.
const discoveryCacheTTL = 6 * time.Hour

// illegalCacheDirChars matches everything not safe to embed verbatim in a cache
// directory name, so a cluster host (which contains ':' and '.') slugs into one
// filesystem-safe path component. Mirrors kubectl's own host-slugging.
var illegalCacheDirChars = regexp.MustCompile(`[^(\w/.)]`)

// cacheBaseDir is the directory kubecom persists disposable cross-run caches
// under: os.UserCacheDir()/kubecom (~/.cache/kubecom on Linux,
// ~/Library/Caches/kubecom on macOS). It is deliberately separate from the
// config dir (D20) — caches can be deleted at any time with no loss of user
// data. It returns an error only when no cache dir is resolvable (e.g. HOME
// unset), in which case the caller degrades to an in-memory cache.
func cacheBaseDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "kubecom"), nil
}

// computeDiscoverCacheDir slugs a cluster host into a per-host subdirectory of
// parent. The discovery cache MUST be unique per host:port — two clusters share
// group/version names but not their resource sets, so a shared dir would serve
// one cluster's cache to the other. The scheme is stripped and any remaining
// filesystem-unsafe character is replaced, so "https://a.example:6443" becomes
// "a.example_6443".
func computeDiscoverCacheDir(parent, host string) string {
	schemeless := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	safe := illegalCacheDirChars.ReplaceAllString(schemeless, "_")
	return filepath.Join(parent, safe)
}

// newCachedDiscovery builds the discovery client the whole kube layer reads
// through. Preferred path: an on-disk cache (diskcached) that persists the
// server's discovery documents under cacheBaseDir, keyed per host, with a TTL —
// so a warm start reconciles the menu from disk with zero network round-trips
// and repeated runs share the cache (D8: "cache discovery on disk"). It does no
// network I/O at construction (documents are read/written lazily) and creates
// its cache dirs on first write, so it never blocks first paint.
//
// Degrade, don't crash (goal principle 3): if no cache dir is resolvable, fall
// back to a purely in-memory cache — discovery still works, it just isn't
// persisted across runs. Both returned clients implement
// discovery.CachedDiscoveryInterface, so Invalidate/Fresh (and the deferred
// RESTMapper built on top) behave identically either way.
func newCachedDiscovery(cfg *rest.Config) (discovery.CachedDiscoveryInterface, error) {
	base, err := cacheBaseDir()
	if err != nil {
		dc, derr := discovery.NewDiscoveryClientForConfig(cfg)
		if derr != nil {
			return nil, derr
		}
		return memcache.NewMemCacheClient(dc), nil
	}
	discoveryDir := computeDiscoverCacheDir(filepath.Join(base, "discovery"), cfg.Host)
	httpDir := filepath.Join(base, "http")
	return diskcached.NewCachedDiscoveryClientForConfig(cfg, discoveryDir, httpDir, discoveryCacheTTL)
}
