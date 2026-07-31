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

// FailedGroup records one API group that discovery could not load, kept isolated
// from the healthy results (#87, #76). The group degrades to unavailable in the
// menu; every other group is unaffected.
type FailedGroup struct {
	// Group is the API group that failed, and is always set — it is what the menu
	// keys on to mark a kind unavailable, and what the log names.
	Group string
	// Version is the failing version when discovery reported one, and empty when
	// the failure is only knowable at group granularity (D187): an aggregated
	// apiserver drops a stale group's versions before any cached client can see
	// them, so all that survives is "this group is serving nothing".
	Version string
	// Err is the per-group cause. ErrGroupServesNoVersion for the group-granular
	// case; the server's own error otherwise.
	Err error
}

// GroupVersion renders the failure for display and logging: "metrics.k8s.io/v1beta1"
// when the version is known, the bare group when it is not.
func (f FailedGroup) GroupVersion() string {
	if f.Version == "" {
		return f.Group
	}
	return f.Group + "/" + f.Version
}

// ErrGroupServesNoVersion is the recorded cause for an API group the server still
// lists but serves no version of — the shape a broken aggregated API (a dead
// metrics-server, an unavailable CRD conversion webhook) takes on any apiserver
// answering aggregated discovery. See versionlessGroups and D187.
var ErrGroupServesNoVersion = errors.New("API group is registered but serving no version")

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

// resourceDiscoverer is the narrow slice of discovery.DiscoveryInterface this
// package uses: the preferred-resource pass, plus the group list it needs to see
// which groups the server is serving nothing for (D187 — the preferred-resource
// pass alone cannot tell). Narrowing it keeps the discovery core trivially
// fakeable in hermetic tests (D18) and makes the dependency explicit.
// *discovery.DiscoveryClient and every cached client satisfy it.
type resourceDiscoverer interface {
	ServerPreferredResources() ([]*metav1.APIResourceList, error)
	ServerGroups() (*metav1.APIGroupList, error)
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
// That error covers the legacy discovery shape only, so the pass also asks the
// server which groups it is serving no version of (versionlessGroups) — the
// shape the same outage takes on an aggregated apiserver, where it reaches the
// preferred-resource pass as silence (DISC-01, D187).
//
// Filtering: subresources (`pods/log`) and non-listable resources
// (create-only reviews like `tokenreviews`) are dropped — the menu browses
// things you can list.
func discoverResources(d resourceDiscoverer) DiscoveryResult {
	lists, err := d.ServerPreferredResources()

	var failed []FailedGroup
	if err != nil {
		var gde *discovery.ErrGroupDiscoveryFailed
		if errors.As(err, &gde) {
			for gv, gerr := range gde.Groups {
				failed = append(failed, FailedGroup{Group: gv.Group, Version: gv.Version, Err: gerr})
			}
			// lists still holds the healthy groups — fall through and use them.
		} else {
			// Total failure (unreachable server, auth). Nothing usable.
			return DiscoveryResult{Err: err}
		}
	}
	failed = append(failed, versionlessGroups(d, failed)...)

	var resources []Resource
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, perr := schema.ParseGroupVersion(list.GroupVersion)
		if perr != nil {
			// A malformed GroupVersion string isolates that list, not the pass. It
			// cannot be split, so the raw string stands in for the group.
			failed = append(failed, FailedGroup{Group: list.GroupVersion, Err: perr})
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
	sort.Slice(failed, func(i, j int) bool { return failed[i].GroupVersion() < failed[j].GroupVersion() })

	return DiscoveryResult{Resources: resources, Failed: failed}
}

// versionlessGroups reports every API group the server lists but serves no
// version of. That is how a *broken* group reaches a modern apiserver's clients,
// and it is the only trace of the failure a cached client keeps (DISC-01, D187).
//
// Aggregated discovery (`apidiscovery.k8s.io`, the default since 1.30) marks a
// failing group/version "stale" and hands the cause to callers in a side channel
// client-go exposes only to an AggregatedDiscoveryInterface; splitting the
// response drops the stale version from the group itself. The on-disk cached
// client kubecom reads through (M1-04/D32) is not such an interface — and cannot
// become one without giving up the cache, since what it persists is the split
// group list, not the aggregated document — so ServerPreferredResources takes its
// non-aggregated branch, finds no version to fetch under the broken group, and
// returns no error at all. The group survives with an empty Versions slice, and
// that is enough to name it.
//
// A healthy group always carries at least one version, and the legacy path keeps
// the versions of a broken one (its per-version fetch is what fails), so this
// answers the aggregated case and stays silent on both others. `known` holds what
// the pass already reported, so a group named there is not reported twice.
//
// The group list is the same document ServerPreferredResources just read, so on
// the cached client this costs no round trip and D8's warm start holds. An error
// fetching it is not a failure of this pass — the resources are already in hand —
// so it degrades to reporting nothing (principle 3).
func versionlessGroups(d resourceDiscoverer, known []FailedGroup) []FailedGroup {
	groups, err := d.ServerGroups()
	if err != nil || groups == nil {
		return nil
	}
	seen := make(map[string]bool, len(known))
	for _, f := range known {
		seen[f.Group] = true
	}
	var out []FailedGroup
	for _, g := range groups.Groups {
		if g.Name == "" || len(g.Versions) > 0 || seen[g.Name] {
			continue
		}
		seen[g.Name] = true
		out = append(out, FailedGroup{Group: g.Name, Err: ErrGroupServesNoVersion})
	}
	return out
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
func StartDiscovery(ctx context.Context, d resourceDiscoverer) <-chan DiscoveryResult {
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
