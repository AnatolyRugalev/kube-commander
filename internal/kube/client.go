package kube

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	memcache "k8s.io/client-go/discovery/cached/memory"
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
//   - Discovery  — server API discovery (groups, versions, resources).
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
	Discovery  discovery.DiscoveryInterface
	RESTMapper meta.RESTMapper
}

// RESTConfig resolves a *rest.Config from the given ClientConfig using client-go's
// standard loading rules and context override. It performs no network I/O and
// never panics: a missing or malformed kubeconfig, or an unknown context, returns
// a wrapped error the caller can surface and degrade on (#86).
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
		return nil, fmt.Errorf("kube: loading kubeconfig: %w", err)
	}
	return cfg, nil
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
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building discovery client: %w", err)
	}
	// Deferred + memory-cached: no discovery round-trip until the first mapping
	// is requested, and results are cached thereafter.
	deferred := restmapper.NewDeferredDiscoveryRESTMapper(memcache.NewMemCacheClient(dc))
	// Compose a static seed mapper ahead of discovery: core GVKs resolve instantly
	// with zero network I/O (the seed short-circuits, discovery is never consulted
	// for them), while unknown kinds — CRDs, less-common groups — fall through to
	// the deferred discovery mapper once it warms (D8, M1-02). FirstHitRESTMapper
	// returns the first mapper that resolves, so the seed always wins for its kinds.
	mapper := meta.FirstHitRESTMapper{
		MultiRESTMapper: meta.MultiRESTMapper{newSeedRESTMapper(), deferred},
	}
	return &Clients{
		Config:     cfg,
		Clientset:  clientset,
		Dynamic:    dyn,
		Discovery:  dc,
		RESTMapper: mapper,
	}, nil
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
