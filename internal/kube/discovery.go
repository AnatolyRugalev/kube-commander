package kube

import (
	"context"
	"errors"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// Resource is one browsable API resource surfaced by discovery: a listable
// (non-subresource) kind the menu can show and the table layer can list/watch.
// It carries everything a caller needs to address the resource generically —
// GVK for typing, GVR for the dynamic client, scope, and the metadata kubectl
// exposes (verbs, short names, categories) — so the TUI never re-derives it.
type Resource struct {
	GVK        schema.GroupVersionKind
	GVR        schema.GroupVersionResource
	Namespaced bool
	Verbs      metav1.Verbs
	ShortNames []string
	Categories []string
}

// FailedGroup records one API group/version that discovery could not load, kept
// isolated from the healthy results (#87, #76). The group degrades to unavailable
// in the menu; every other group is unaffected. Err is the per-group cause.
type FailedGroup struct {
	GroupVersion string
	Err          error
}

// DiscoveryResult is the outcome of one full-discovery pass — the payload of the
// "discovery ready" reconcile signal (D8). Resources holds the healthy, listable
// resources (sorted, stable). Failed holds the per-group failures that were
// isolated. Err is set only on a *total* failure (e.g. the API server is
// unreachable), in which case Resources/Failed are empty and the caller should
// surface the error and retry later rather than reconcile an empty menu.
type DiscoveryResult struct {
	Resources []Resource
	Failed    []FailedGroup
	Err       error
}

// preferredResourceDiscoverer is the narrow slice of discovery.DiscoveryInterface
// this package uses. Narrowing it keeps the discovery core trivially fakeable in
// hermetic tests (D18) — a stub need implement only this one method — and makes
// the dependency explicit. *discovery.DiscoveryClient satisfies it.
type preferredResourceDiscoverer interface {
	ServerPreferredResources() ([]*metav1.APIResourceList, error)
}

// discoverResources runs one synchronous full-discovery pass and flattens the
// server's preferred resources into the browsable menu set.
//
// Preferred (not all) versions: one entry per resource at its server-preferred
// version, so the menu shows `deployments` once, not once per served version.
//
// Per-group fault isolation (#87, #76): ServerPreferredResources returns the
// resources it *could* load together with an *ErrGroupDiscoveryFailed listing the
// groups it could not. We keep the partial results and record each failed group,
// so one broken/denied aggregated API (a classic metrics-server outage) degrades
// only itself instead of blanking the whole menu — the original's central bug.
// Any other error means discovery wholesale failed; that is returned in Err.
//
// Filtering: subresources (`pods/log`) and non-listable resources
// (create-only reviews like `tokenreviews`) are dropped — the menu browses
// things you can list.
func discoverResources(d preferredResourceDiscoverer) DiscoveryResult {
	lists, err := d.ServerPreferredResources()

	var failed []FailedGroup
	if err != nil {
		var gde *discovery.ErrGroupDiscoveryFailed
		if errors.As(err, &gde) {
			for gv, gerr := range gde.Groups {
				failed = append(failed, FailedGroup{GroupVersion: gv.String(), Err: gerr})
			}
			sort.Slice(failed, func(i, j int) bool { return failed[i].GroupVersion < failed[j].GroupVersion })
			// lists still holds the healthy groups — fall through and use them.
		} else {
			// Total failure (unreachable server, auth). Nothing usable.
			return DiscoveryResult{Err: err}
		}
	}

	var resources []Resource
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, perr := schema.ParseGroupVersion(list.GroupVersion)
		if perr != nil {
			// A malformed GroupVersion string isolates that list, not the pass.
			failed = append(failed, FailedGroup{GroupVersion: list.GroupVersion, Err: perr})
			continue
		}
		for _, r := range list.APIResources {
			if strings.ContainsRune(r.Name, '/') {
				continue // subresource
			}
			if !hasVerb(r.Verbs, "list") {
				continue // not browsable
			}
			rgv := gv
			if r.Group != "" {
				rgv.Group = r.Group
			}
			if r.Version != "" {
				rgv.Version = r.Version
			}
			resources = append(resources, Resource{
				GVK:        rgv.WithKind(r.Kind),
				GVR:        rgv.WithResource(r.Name),
				Namespaced: r.Namespaced,
				Verbs:      r.Verbs,
				ShortNames: r.ShortNames,
				Categories: r.Categories,
			})
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].GVR.Group != resources[j].GVR.Group {
			return resources[i].GVR.Group < resources[j].GVR.Group
		}
		return resources[i].GVR.Resource < resources[j].GVR.Resource
	})

	return DiscoveryResult{Resources: resources, Failed: failed}
}

// hasVerb reports whether verbs contains want.
func hasVerb(verbs metav1.Verbs, want string) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}

// StartDiscovery kicks off one full-discovery pass in a background goroutine and
// delivers its result exactly once on the returned channel — the "discovery
// ready" reconcile signal of D8. It returns immediately and never blocks the
// caller, preserving fast cold start: the seed RESTMapper (M1-02) already makes
// core resources usable, and this reconciles the menu with the full set once it
// arrives.
//
// The channel is buffered (cap 1) so the sending goroutine never leaks even if
// the caller stops listening. If ctx is cancelled before the pass completes, the
// result is dropped rather than sent. The TUI turns the received DiscoveryResult
// into a reconcile tea.Msg; this package stays free of any TUI import.
func StartDiscovery(ctx context.Context, d preferredResourceDiscoverer) <-chan DiscoveryResult {
	ch := make(chan DiscoveryResult, 1)
	go func() {
		res := discoverResources(d)
		select {
		case ch <- res:
		case <-ctx.Done():
		}
	}()
	return ch
}

// StartDiscovery runs async full discovery against this cluster's discovery
// client. See the package-level StartDiscovery for semantics.
func (c *Clients) StartDiscovery(ctx context.Context) <-chan DiscoveryResult {
	return StartDiscovery(ctx, c.Discovery)
}
