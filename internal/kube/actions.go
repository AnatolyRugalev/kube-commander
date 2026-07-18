package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// resourceInterface returns the dynamic client scoped to a resource: namespaced
// to ns when the resource is namespaced, cluster-scoped otherwise. It is the
// shared addressing helper the in-process action set (M1-06) builds on — every
// mutating action targets an object generically by its GVR and namespace/name,
// through the dynamic client, so no per-kind typed client is needed and CRDs work
// with zero extra wiring (D2: in-process client-go first).
func (c *Clients) resourceInterface(r Resource, ns string) dynamic.ResourceInterface {
	nri := c.Dynamic.Resource(r.GVR)
	if r.Namespaced {
		return nri.Namespace(ns)
	}
	return nri
}

// Delete removes the object a table row references. It is generic over every
// resource — built-in or CRD — because it addresses the object through the
// dynamic client by GVR (from the discovery Resource) and by namespace/name (from
// the row's ObjectRef); the namespace is taken from the ref and ignored for
// cluster-scoped resources (r.Namespaced == false).
//
// UID precondition: when the ObjectRef carries a UID, Delete sets it as a delete
// precondition so the request only removes *that* object. If the named object was
// deleted and a new one recreated under the same name since the table was listed,
// the delete fails (Conflict) rather than removing the wrong object — a classic
// TUI race, since a table row is a snapshot. A row with no UID (degraded metadata,
// principle 3) deletes by name alone. A caller that sets its own preconditions in
// opts keeps them; the UID guard is added only when opts has none.
//
// opts also carries the propagation policy (foreground/background/orphan) and an
// optional grace period. Errors are wrapped, never panicked: a NotFound (already
// gone) or a Conflict (UID mismatch) surfaces to the caller to display (#86).
func (c *Clients) Delete(ctx context.Context, r Resource, ref ObjectRef, opts metav1.DeleteOptions) error {
	if ref.Name == "" {
		return fmt.Errorf("kube: delete %s: empty object name", r.GVR.Resource)
	}
	if err := c.resourceInterface(r, ref.Namespace).Delete(ctx, ref.Name, withUIDPrecondition(ref, opts)); err != nil {
		return fmt.Errorf("kube: deleting %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	return nil
}

// withUIDPrecondition returns opts guarded by ref's UID: when the ref carries a
// UID and opts has no preconditions of its own, it adds a UID precondition so a
// mutation only lands on that exact object (see Delete). A caller that already set
// preconditions keeps them untouched, and a ref with no UID (degraded metadata)
// passes opts through unchanged. Pure and side-effect free — opts is copied, not
// mutated — so the whole action set can reuse it and it is unit-testable without a
// client (the fake dynamic client discards DeleteOptions).
func withUIDPrecondition(ref ObjectRef, opts metav1.DeleteOptions) metav1.DeleteOptions {
	if ref.UID != "" && opts.Preconditions == nil {
		uid := types.UID(ref.UID)
		opts.Preconditions = &metav1.Preconditions{UID: &uid}
	}
	return opts
}
