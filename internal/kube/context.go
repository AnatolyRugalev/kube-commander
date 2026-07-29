package kube

import (
	"fmt"
	"sort"

	"k8s.io/client-go/tools/clientcmd"
)

// ContextInfo describes one context declared in the kubeconfig. It is pure
// kubeconfig data — no server was contacted to produce it, so Cluster is the
// configured server *name*, not a reachability claim, and Namespace is the
// context's declared default namespace ("" when the context sets none, which
// callers read as "default").
type ContextInfo struct {
	// Name is the context's key in the kubeconfig — what ClientConfig.Context
	// takes and what ContextName reports.
	Name string
	// Cluster is the name of the cluster entry the context points at.
	Cluster string
	// Namespace is the context's default namespace, "" when unset.
	Namespace string
	// Current is true for the single context Connect would select for the
	// ClientConfig this list was built from (the explicit Context if set, else
	// the kubeconfig's current-context). No entry is Current when that name
	// names no declared context.
	Current bool
}

// Contexts lists every context in the kubeconfig ClientConfig selects, sorted by
// name so a picker's order is stable across runs (kubeconfig contexts live in a
// map, whose iteration order is not). Exactly the ClientConfig.Kubeconfig path is
// honoured, or the standard loading rules when it is empty — the same resolution
// RESTConfig and ContextName use, so the returned names are the ones
// ClientConfig.Context accepts.
//
// It performs no network I/O and never panics: a missing or malformed kubeconfig
// returns a wrapped error tagged so Classify reports KindBadContext (#86,
// principle 3), and a readable kubeconfig that declares no contexts returns an
// empty slice and no error.
func Contexts(cc ClientConfig) ([]ContextInfo, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cc.Kubeconfig != "" {
		rules.ExplicitPath = cc.Kubeconfig
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		// Tagged like RESTConfig's failure so callers classify a broken
		// kubeconfig the same way whichever entry point hit it first.
		return nil, fmt.Errorf("kube: loading kubeconfig contexts: %w: %w", errBadContext, err)
	}
	// The selected context: the explicit override wins, else the file's
	// current-context — ContextName's rule, kept in one place.
	selected := cc.Context
	if selected == "" {
		selected = raw.CurrentContext
	}
	out := make([]ContextInfo, 0, len(raw.Contexts))
	for name, ctx := range raw.Contexts {
		if ctx == nil {
			continue
		}
		out = append(out, ContextInfo{
			Name:      name,
			Cluster:   ctx.Cluster,
			Namespace: ctx.Namespace,
			Current:   name == selected,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
