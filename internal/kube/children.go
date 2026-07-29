package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ChildScope is the answer to "what does this object drill into?": the child
// Resource, the namespace to browse it in, and the metav1.ListOptions that narrow
// it to *this* owner's children. It is deliberately **not** a fetched list —
// List and Watch both already take a namespace and ListOptions, so handing the
// caller a scope gives it a live, watched child table for free, instead of a
// second snapshot-only data path that would go stale the moment a pod restarted
// (D155 pt 3, M4-07).
//
// Namespace is the owner's namespace for a namespaced owner, and "" (every
// namespace) for a cluster-scoped one — a Node's pods are spread across the
// cluster, so scoping them to one namespace would hide most of them.
type ChildScope struct {
	// Resource is the child kind to list/watch. Today it is always Pod; the
	// shape is general so a future owner→child pair needs no signature change.
	Resource Resource
	// Namespace is the namespace to list the children in; "" means all
	// namespaces.
	Namespace string
	// Options carries exactly one of LabelSelector (workload/service owners,
	// from spec.selector) or FieldSelector (Node, spec.nodeName). It is passed
	// verbatim to List/Watch, so the server does the filtering.
	Options metav1.ListOptions
}

// Selector returns the scope's selector as the single string a UI can show —
// the label selector if there is one, else the field selector. It exists so the
// TUI can name the filter it applied (M4-08 puts it in the status bar) without
// having to know which of the two ListOptions fields a given owner kind fills in.
// The zero ChildScope yields "".
func (s ChildScope) Selector() string {
	if s.Options.LabelSelector != "" {
		return s.Options.LabelSelector
	}
	return s.Options.FieldSelector
}

// podGroupKind is the child kind every rule below resolves to. Pods are the only
// children kubecom drills into today: they are the leaf of every workload chain
// and the thing a reader actually wants after selecting an owner. An owner→owner
// hop (CronJob→Job, Deployment→ReplicaSet) is deliberately absent — neither is
// reachable by a server-side selector (the link is metadata.ownerReferences,
// which no field selector indexes), so it would need a client-side filter over a
// full list and could not be a *scope* at all.
var podGroupKind = schema.GroupKind{Group: "", Kind: "Pod"}

// childLink says how one owner kind reaches its children. byNodeName picks the
// mechanism: false resolves the owner's spec.selector into a label selector (the
// generalized PodForOwner path, M3-07b); true builds a spec.nodeName field
// selector, which is safe here — and was not safe for cluster search
// (SEARCH-04c note) — precisely because the child kind is known to be Pod, the
// one kind the apiserver indexes that field on.
type childLink struct {
	child      schema.GroupKind
	byNodeName bool
}

// childLinks is the owner→child table. Every entry but Node reads spec.selector,
// whose two on-the-wire shapes (a metav1.LabelSelector for the apps/batch kinds,
// a flat label map for ReplicationController and Service) podSelector already
// handles — so extending this map is usually a one-line change with no new
// parsing.
//
// Service is included even though it is not an *owner* in the ownerReferences
// sense: it selects pods by exactly the same mechanism, and "which pods is this
// Service actually sending traffic to" is the same question the drill-down
// answers everywhere else. Node is in the same position (a Node owns nothing;
// it hosts). The gesture is "related pods", and the scope names how.
var childLinks = map[schema.GroupKind]childLink{
	{Group: "apps", Kind: "Deployment"}:        {child: podGroupKind},
	{Group: "apps", Kind: "ReplicaSet"}:        {child: podGroupKind},
	{Group: "apps", Kind: "StatefulSet"}:       {child: podGroupKind},
	{Group: "apps", Kind: "DaemonSet"}:         {child: podGroupKind},
	{Group: "batch", Kind: "Job"}:              {child: podGroupKind},
	{Group: "", Kind: "ReplicationController"}: {child: podGroupKind},
	{Group: "", Kind: "Service"}:               {child: podGroupKind},
	{Group: "", Kind: "Node"}:                  {child: podGroupKind, byNodeName: true},
}

// HasChildren reports whether owner is a kind Children can resolve a scope for.
// It is a pure lookup with no network I/O, so a caller can gate the drill-down
// action on a selected row without paying for a request — the same
// cheap-predicate role canGet plays for the row actions.
func HasChildren(owner Resource) bool {
	_, ok := childLinks[owner.GVK.GroupKind()]
	return ok
}

// Children resolves the object ref of an owner into the ChildScope that lists
// its children (M4-07). kinds is the caller's available resource set — typically
// the discovery result the menu is built from — and the child Resource is looked
// up in it rather than synthesized, so the returned Resource carries the real
// verbs the cluster reports and the caller's verb-gating stays honest. A cluster
// that does not expose pods to this user simply has no child scope.
//
// The only network I/O is a single Get of the owner, and only for the
// selector-based kinds: the Node path is derived entirely from the ref, so a
// Node drill-down costs nothing. Every failure — an unsupported kind, an owner
// that has since been deleted, a selector-less Service, an RBAC denial — is a
// wrapped error the caller degrades to a status-bar toast (principle 3), never a
// panic and never a scope that would silently list *everything*.
func (c *Clients) Children(ctx context.Context, owner Resource, ref ObjectRef, kinds []Resource) (ChildScope, error) {
	link, ok := childLinks[owner.GVK.GroupKind()]
	if !ok {
		return ChildScope{}, fmt.Errorf("kube: %s has no child resources to drill into", owner.GVK.Kind)
	}
	if ref.Name == "" {
		return ChildScope{}, fmt.Errorf("kube: children of %s: empty object name", owner.GVK.Kind)
	}
	child, ok := resourceForGroupKind(kinds, link.child)
	if !ok {
		return ChildScope{}, fmt.Errorf("kube: children of %s %q: %s is not available in this cluster", owner.GVK.Kind, ref.Name, link.child.Kind)
	}
	if link.byNodeName {
		// A Node's pods live in every namespace, so the scope is cluster-wide and
		// the field selector is the whole filter. spec.nodeName is one of the few
		// fields the apiserver indexes for pods.
		return ChildScope{
			Resource:  child,
			Namespace: "",
			Options:   metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", ref.Name).String()},
		}, nil
	}
	obj, err := c.Dynamic.Resource(owner.GVR).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return ChildScope{}, fmt.Errorf("kube: getting %s %q: %w", owner.GVR.Resource, ref.Name, err)
	}
	sel, err := podSelector(obj)
	if err != nil {
		return ChildScope{}, fmt.Errorf("kube: child selector for %s %q: %w", owner.GVR.Resource, ref.Name, err)
	}
	// An empty selector string matches every pod in the namespace. A selector
	// that parses to Everything (spec.selector present but with no terms) would
	// therefore turn a drill-down into "all pods", which is a wrong answer
	// dressed as a right one — refuse it instead.
	if sel.Empty() {
		return ChildScope{}, fmt.Errorf("kube: child selector for %s %q matches everything", owner.GVR.Resource, ref.Name)
	}
	return ChildScope{
		Resource:  child,
		Namespace: ref.Namespace,
		Options:   metav1.ListOptions{LabelSelector: sel.String()},
	}, nil
}

// resourceForGroupKind finds the resource in kinds whose GroupKind is gk. The
// caller's set is authoritative: matching on GroupKind (not GroupVersionKind)
// means the cluster's preferred version wins, which is what discovery already
// resolved.
func resourceForGroupKind(kinds []Resource, gk schema.GroupKind) (Resource, bool) {
	for _, r := range kinds {
		if r.GVK.GroupKind() == gk {
			return r, true
		}
	}
	return Resource{}, false
}
