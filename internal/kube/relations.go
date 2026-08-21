package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RelationDirection says which way a relation points, so a surface can group the
// list the way a reader thinks about it rather than the way the object stores it.
type RelationDirection int

const (
	// RelationUp points at what created or hosts this object: an
	// ownerReferences parent, a pod's node.
	RelationUp RelationDirection = iota
	// RelationDown points at what this object creates or selects — the
	// ChildScope Children already resolves.
	RelationDown
	// RelationSide points at what this object merely references: its claims,
	// config maps, secrets, service account.
	RelationSide
)

// String names the direction for a UI heading. Unknown values render as
// "related" rather than a number — a display string is never worth a panic.
func (d RelationDirection) String() string {
	switch d {
	case RelationUp:
		return "up"
	case RelationDown:
		return "down"
	case RelationSide:
		return "side"
	}
	return "related"
}

// RelationRole is the human name of the edge — what the target *is* to this
// object. It is a display label, not an identifier: the surface prints it, and
// two relations with the same role are two rows, not a conflict.
type RelationRole string

const (
	RoleOwner          RelationRole = "owner"
	RoleChildren       RelationRole = "pods"
	RoleNode           RelationRole = "node"
	RoleServiceAccount RelationRole = "service account"
	RoleClaim          RelationRole = "claim"
	RoleVolume         RelationRole = "volume"
	RoleConfigMap      RelationRole = "config map"
	RoleSecret         RelationRole = "secret"
	RolePullSecret     RelationRole = "pull secret"
)

// Relation is one navigable edge out of an object. It comes in two shapes and
// the shape decides how a caller opens it:
//
//   - **named** (Name set): exactly one object, opened by browsing Resource and
//     selecting Ref().
//   - **set-shaped** (Name empty): a filtered list, opened by handing Scope() to
//     the drill-down path Children already feeds.
//
// Resource is always a member of the caller's discovered kind set, never a
// synthesized stub — a relation whose target kind this cluster (or this user)
// cannot see is dropped rather than offered and then failing on open.
//
// Namespace is the namespace to look the target up in: the object's own for a
// namespaced target, "" for a cluster-scoped one (a pod's Node) and for a
// cluster-wide set (a Node's pods).
type Relation struct {
	Role      RelationRole
	Direction RelationDirection
	Resource  Resource
	Namespace string
	// Name is the target's name for a named relation, and "" when the relation
	// is set-shaped.
	Name string
	// UID is the target's UID when the source recorded one (ownerReferences do);
	// "" for a spec-named link, where only the name is written down.
	UID string
	// Options narrows a set-shaped relation, verbatim as List/Watch take it. Zero
	// for a named relation.
	Options metav1.ListOptions
}

// IsSet reports whether the relation points at a filtered list rather than one
// named object.
func (r Relation) IsSet() bool { return r.Name == "" }

// Ref is the named target's identity. Zero-valued Name for a set-shaped relation.
func (r Relation) Ref() ObjectRef {
	return ObjectRef{Namespace: r.Namespace, Name: r.Name, UID: r.UID}
}

// Scope is the set-shaped target as the ChildScope the drill-down path already
// consumes. For a named relation it yields an unfiltered scope of the target's
// kind, which is not what the caller wants — check IsSet first.
func (r Relation) Scope() ChildScope {
	return ChildScope{Resource: r.Resource, Namespace: r.Namespace, Options: r.Options}
}

// Relation target kinds resolved out of the caller's discovered set. podGroupKind
// lives in children.go, which is the other half of this graph.
var (
	nodeGroupKind           = schema.GroupKind{Group: "", Kind: "Node"}
	serviceAccountGroupKind = schema.GroupKind{Group: "", Kind: "ServiceAccount"}
	claimGroupKind          = schema.GroupKind{Group: "", Kind: "PersistentVolumeClaim"}
	volumeGroupKind         = schema.GroupKind{Group: "", Kind: "PersistentVolume"}
	configMapGroupKind      = schema.GroupKind{Group: "", Kind: "ConfigMap"}
	secretGroupKind         = schema.GroupKind{Group: "", Kind: "Secret"}
)

// Relations resolves one object into the neighbours a reader can navigate to:
// its ownerReferences parents (the reverse of the drill-down — pod → ReplicaSet →
// Deployment), its child scope when the kind has one (Children's forward
// direction, reused verbatim), and the links a kind names in its own spec (a
// pod's node, service account, claims, config maps and secrets; a claim's volume).
//
// The only network I/O is a single Get of the object; everything else is derived
// from what that Get returned, so the cost does not grow with the number of
// relations found. That is deliberate — a relation that needs its own List (which
// Services select this pod) is a different cost class and is not resolved here.
//
// kinds is the caller's available resource set, typically the discovery result
// the menu is built from. A target kind missing from it is skipped silently: an
// edge nobody can open is noise, not a finding. Only the Get can fail, and it
// fails wrapped (principle 3) — an object with no neighbours yields an empty
// slice and no error, which is a true answer, not a degraded one.
//
// The order is stable: owners, then the child scope, then the spec links in the
// order the spec writes them. Duplicates (two volumes from one claim) collapse to
// the first occurrence.
func (c *Clients) Relations(ctx context.Context, res Resource, ref ObjectRef, kinds []Resource) ([]Relation, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: relations of %s: empty object name", res.GVK.Kind)
	}
	obj, err := c.Dynamic.Resource(res.GVR).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: getting %s %q: %w", res.GVR.Resource, ref.Name, err)
	}
	var out []Relation
	for _, or := range obj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(or.APIVersion)
		if err != nil {
			continue
		}
		out = appendNamed(out, kinds, schema.GroupKind{Group: gv.Group, Kind: or.Kind},
			RoleOwner, RelationUp, ref.Namespace, or.Name, string(or.UID))
	}
	// The child scope is resolved from the object already in hand, so the
	// forward direction costs no second Get. Every failure here — an unsupported
	// kind, a selector-less Service, a match-everything selector — means "no
	// child relation", which is exactly what dropping it says.
	if scope, err := childScope(res, ref, obj, kinds); err == nil {
		out = append(out, Relation{
			Role: RoleChildren, Direction: RelationDown, Resource: scope.Resource,
			Namespace: scope.Namespace, Options: scope.Options,
		})
	}
	out = append(out, specRelations(obj, res, ref, kinds)...)
	return dedupeRelations(out), nil
}

// specRelations reads the links a kind writes into its own spec. Only the kinds
// with a link worth navigating are listed; every other kind contributes nothing
// and returns early, which is why this is a switch and not a table — the shapes
// have nothing in common beyond "a name in the spec".
func specRelations(obj *unstructured.Unstructured, res Resource, ref ObjectRef, kinds []Resource) []Relation {
	var out []Relation
	switch res.GVK.GroupKind() {
	case podGroupKind:
		if node, _, _ := unstructured.NestedString(obj.Object, "spec", "nodeName"); node != "" {
			out = appendNamed(out, kinds, nodeGroupKind, RoleNode, RelationUp, "", node, "")
		}
		if sa, _, _ := unstructured.NestedString(obj.Object, "spec", "serviceAccountName"); sa != "" {
			out = appendNamed(out, kinds, serviceAccountGroupKind, RoleServiceAccount, RelationSide, ref.Namespace, sa, "")
		}
		vols, _, _ := unstructured.NestedSlice(obj.Object, "spec", "volumes")
		for _, v := range vols {
			vol, ok := v.(map[string]any)
			if !ok {
				continue
			}
			// The three volume sources that name another object. A projected
			// source nests them one level deeper and is not read here; the
			// token/downward-API projections that dominate it are not
			// navigation targets anyway.
			if n, _, _ := unstructured.NestedString(vol, "persistentVolumeClaim", "claimName"); n != "" {
				out = appendNamed(out, kinds, claimGroupKind, RoleClaim, RelationSide, ref.Namespace, n, "")
			}
			if n, _, _ := unstructured.NestedString(vol, "configMap", "name"); n != "" {
				out = appendNamed(out, kinds, configMapGroupKind, RoleConfigMap, RelationSide, ref.Namespace, n, "")
			}
			if n, _, _ := unstructured.NestedString(vol, "secret", "secretName"); n != "" {
				out = appendNamed(out, kinds, secretGroupKind, RoleSecret, RelationSide, ref.Namespace, n, "")
			}
		}
		// imagePullSecrets earn a row because ImagePullBackOff is one of the
		// failures the unhealthy sweep surfaces, and the secret is where that
		// investigation goes next.
		pull, _, _ := unstructured.NestedSlice(obj.Object, "spec", "imagePullSecrets")
		for _, p := range pull {
			m, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if n, _, _ := unstructured.NestedString(m, "name"); n != "" {
				out = appendNamed(out, kinds, secretGroupKind, RolePullSecret, RelationSide, ref.Namespace, n, "")
			}
		}
	case claimGroupKind:
		// A bound claim names its volume; an unbound one has no volumeName yet.
		if n, _, _ := unstructured.NestedString(obj.Object, "spec", "volumeName"); n != "" {
			out = appendNamed(out, kinds, volumeGroupKind, RoleVolume, RelationSide, "", n, "")
		}
	case volumeGroupKind:
		// The reverse: a volume's claimRef carries the claim's own namespace,
		// which need not be the (cluster-scoped) volume's browsing namespace.
		name, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "name")
		ns, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "namespace")
		if name != "" {
			uid, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "uid")
			out = appendNamed(out, kinds, claimGroupKind, RoleClaim, RelationSide, ns, name, uid)
		}
	}
	return out
}

// appendNamed appends a named relation to out if gk is a kind the caller can
// actually open, and drops it otherwise. A cluster-scoped target's namespace is
// forced empty whatever the source said, so a claimRef-style namespace on a
// cluster-scoped kind cannot produce an unopenable ref.
func appendNamed(out []Relation, kinds []Resource, gk schema.GroupKind, role RelationRole, dir RelationDirection, ns, name, uid string) []Relation {
	target, ok := resourceForGroupKind(kinds, gk)
	if !ok {
		return out
	}
	if !target.Namespaced {
		ns = ""
	}
	return append(out, Relation{Role: role, Direction: dir, Resource: target, Namespace: ns, Name: name, UID: uid})
}

// dedupeRelations collapses relations that would open the same thing under the
// same role, keeping the first. A pod mounting one claim through two volumes is
// the common case; the role is part of the key so a secret that is both a mounted
// volume and an image-pull secret stays two rows, which is the honest answer.
func dedupeRelations(rels []Relation) []Relation {
	type key struct {
		role RelationRole
		gk   schema.GroupKind
		ns   string
		name string
		opts string
	}
	seen := make(map[key]bool, len(rels))
	out := make([]Relation, 0, len(rels))
	for _, r := range rels {
		k := key{r.Role, r.Resource.GVK.GroupKind(), r.Namespace, r.Name, r.Scope().Selector()}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}
