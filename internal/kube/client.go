package kube

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientConfig locates and selects the cluster kubecom talks to. Both fields are
// optional: with the zero value, the standard kubeconfig loading rules apply
// (the KUBECONFIG env var, else ~/.kube/config) and the file's current-context
// is used.
type ClientConfig struct {
	// Kubeconfig is an explicit path to a kubeconfig file. Empty means defer to
	// the standard loading rules.
	Kubeconfig string
	// Context selects a context by name from the kubeconfig. Empty uses the
	// file's current-context.
	Context string
}

// Clients bundles the client-go handles the rest of kubecom builds on:
//
//   - Clientset  — typed access to built-in (core/apps/…) resources.
//   - Dynamic    — untyped access to any resource, including CRDs.
//   - Discovery  — server API discovery (groups, versions, resources), backed by
//     an on-disk cache keyed per host with a TTL (M1-04), so warm starts read the
//     discovery documents from disk with no network round-trip and repeated runs
//     share the cache. Call Invalidate to force a refetch.
//   - RESTMapper — resolves GVK↔GVR and the scope (namespaced vs cluster) of a
//     resource. It composes a static seed mapper (core GVKs, resolved instantly
//     with no network I/O — M1-02) ahead of a *deferred* discovery mapper (does no
//     network I/O at construction, populates lazily on first use), so building
//     Clients never blocks first paint and core resources are mappable before
//     discovery finishes (D8). M1-03 layers async full discovery on top.
//
// Config is the resolved *rest.Config the handles were built from, retained so
// callers (e.g. port-forward, which needs the transport) can derive more.
type Clients struct {
	Config     *rest.Config
	Clientset  kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.CachedDiscoveryInterface
	RESTMapper meta.RESTMapper

	// deferredMapper is the discovery-backed half of RESTMapper, retained so
	// Invalidate can Reset it in lockstep with the discovery cache.
	deferredMapper *restmapper.DeferredDiscoveryRESTMapper
}

// RESTConfig resolves a *rest.Config from the given ClientConfig using client-go's
// standard loading rules and context override. It performs no network I/O and
// never panics: a missing or malformed kubeconfig, or an unknown context, returns
// a wrapped error the caller can surface and degrade on — Classify reports it as
// KindBadContext (#86).
func RESTConfig(cc ClientConfig) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cc.Kubeconfig != "" {
		rules.ExplicitPath = cc.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if cc.Context != "" {
		overrides.CurrentContext = cc.Context
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		// Tag with errBadContext so Classify reports KindBadContext for any
		// kubeconfig/context load failure (missing file, unknown context, empty
		// config) — the graceful path #86 requires, never a panic.
		return nil, fmt.Errorf("kube: loading kubeconfig: %w: %w", errBadContext, err)
	}
	return cfg, nil
}

// ContextName resolves the name of the context Connect would select for cc: the
// explicit Context if set, else the kubeconfig's current-context. It performs no
// network I/O and never panics — a missing or malformed kubeconfig returns "" so
// the caller degrades to an unlabelled context (the name is cosmetic, e.g. the
// status bar and welcome page), never a startup failure.
func ContextName(cc ClientConfig) string {
	if cc.Context != "" {
		return cc.Context
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cc.Kubeconfig != "" {
		rules.ExplicitPath = cc.Kubeconfig
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return ""
	}
	return raw.CurrentContext
}

// NewClients builds the client-go handles from an already-resolved *rest.Config.
// Construction is local — it validates and wires the clients but makes no server
// call — so a reachable API server is not required here; connection errors
// surface later, on first request.
func NewClients(cfg *rest.Config) (*Clients, error) {
	if cfg == nil {
		return nil, errors.New("kube: nil rest config")
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building dynamic client: %w", err)
	}
	// On-disk cached discovery (M1-04): documents persist under the user cache dir,
	// keyed per host, with a TTL — warm starts skip the network. Construction does
	// no network I/O; the cache dirs are created lazily on first write.
	dc, err := newCachedDiscovery(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building discovery client: %w", err)
	}
	// Deferred: no discovery round-trip until the first mapping is requested; the
	// on-disk cache above then serves subsequent lookups.
	deferred := restmapper.NewDeferredDiscoveryRESTMapper(dc)
	// Compose a static seed mapper ahead of discovery: core GVKs resolve instantly
	// with zero network I/O (the seed short-circuits, discovery is never consulted
	// for them), while unknown kinds — CRDs, less-common groups — fall through to
	// the deferred discovery mapper once it warms (D8, M1-02). FirstHitRESTMapper
	// returns the first mapper that resolves, so the seed always wins for its kinds.
	mapper := meta.FirstHitRESTMapper{
		MultiRESTMapper: meta.MultiRESTMapper{newSeedRESTMapper(), deferred},
	}
	return &Clients{
		Config:         cfg,
		Clientset:      clientset,
		Dynamic:        dyn,
		Discovery:      dc,
		RESTMapper:     mapper,
		deferredMapper: deferred,
	}, nil
}

// Invalidate forces the next discovery pass and the next unseeded RESTMapping to
// refetch from the server instead of trusting the on-disk cache. Use it for a
// user-triggered refresh, or after an operation that changes the API surface
// (e.g. installing a CRD) so the freshly-added group appears without waiting for
// the cache TTL to lapse. It clears both the discovery cache and the lazily-built
// discovery RESTMapper; the static seed mapper (M1-02) is unaffected — its core
// mappings are authoritative and never stale.
func (c *Clients) Invalidate() {
	if c.Discovery != nil {
		c.Discovery.Invalidate()
	}
	if c.deferredMapper != nil {
		c.deferredMapper.Reset()
	}
}

// Connect is the one-call convenience: resolve the rest.Config from cc and build
// the Clients. It is the entry point M2+ uses to obtain a cluster handle.
func Connect(cc ClientConfig) (*Clients, error) {
	cfg, err := RESTConfig(cc)
	if err != nil {
		return nil, err
	}
	return NewClients(cfg)
}
