package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// Update applies an edited object back to the cluster — the write half of the
// Edit action (M3-15): the user fetched an object as YAML (GetYAML), edited it in
// $EDITOR, and Update PUT-updates the object with the edited bytes. It is the
// in-process equivalent of `kubectl edit`'s default (client-side) apply, so the
// round-trip needs no kubectl binary (D2). Like the rest of the action set it is
// generic over every resource — built-in or CRD — because it addresses the object
// through the dynamic client by GVR (from the discovery Resource) and namespace/
// name (from the ObjectRef).
//
// The edited YAML carries the object's metadata.resourceVersion (GetYAML strips
// only managedFields, not resourceVersion), so the Update gets optimistic
// concurrency for free: if the object changed on the server since it was fetched,
// the Update fails with a Conflict rather than silently clobbering the newer
// version — exactly what `kubectl edit` reports. A buffer without that field is a
// legal *unconditional* PUT that wins the race it should have lost, so its
// presence is load-bearing, not cosmetic (D189).
//
// The buffer's metadata.uid is load-bearing the same way: the apiserver reads a
// UID on an update as a precondition, so an object deleted while the editor was
// open is refused as a **Conflict** — not the NotFound you would expect — and a
// PUT never recreates it. Either Conflict, a server validation error (an edit of
// an immutable field is a 422), or a NotFound is wrapped and surfaces to the
// caller to display (#86); nothing panics (principle 3). Editing a subresource
// (status on a workload) is not an error and not a write: the server discards it
// and applies the rest of the same PUT.
//
// Identity is guarded, not editable: the edited object's name must match the ref
// (an edit updates *this* object — it is not a rename), and for a namespaced
// resource its namespace must match too (an empty namespace in the edited YAML is
// filled in from the ref). A mismatch, an empty name, or unparseable/empty content
// is rejected *before* any request, so a botched edit never mutates the wrong
// object or wipes this one. Detecting an unchanged edit (a no-op $EDITOR exit) is
// the caller's job — it simply never calls Update — so this method always intends
// to write.
func (c *Clients) Update(ctx context.Context, r Resource, ref ObjectRef, edited []byte) error {
	obj, err := parseEdited(edited)
	if err != nil {
		return fmt.Errorf("kube: update %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	if err := guardIdentity(r, ref, obj); err != nil {
		return fmt.Errorf("kube: update %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	if r.Namespaced && obj.GetNamespace() == "" {
		obj.SetNamespace(ref.Namespace)
	}
	if _, err := c.resourceInterface(r, ref.Namespace).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("kube: updating %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	return nil
}

// parseEdited turns the bytes coming back from $EDITOR into an unstructured object,
// rejecting content that must never reach the cluster: invalid YAML, or an empty /
// null document (the user cleared the buffer). Parsing goes YAML → JSON →
// unstructured so integer fields decode as int64 (the unstructured scheme's
// contract), matching what the dynamic client round-trips — a plain YAML unmarshal
// would yield float64 numbers. Pure and side-effect free, so the parse/validation
// contract is unit-testable without a client.
func parseEdited(edited []byte) (*unstructured.Unstructured, error) {
	jsonBytes, err := yaml.YAMLToJSON(edited)
	if err != nil {
		return nil, fmt.Errorf("parsing edited yaml: %w", err)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("decoding edited object: %w", err)
	}
	if len(obj.Object) == 0 {
		return nil, fmt.Errorf("edited object is empty")
	}
	return obj, nil
}

// guardIdentity rejects an edit that would retarget a different object: the edited
// name must be non-empty and equal the ref's (an edit is not a rename), and a
// namespaced resource's edited namespace, when present, must equal the ref's. This
// keeps a fat-fingered edit (renaming in the buffer, pasting another object) from
// updating the wrong object or 404ing surprisingly. Pure, so it is unit-testable.
func guardIdentity(r Resource, ref ObjectRef, obj *unstructured.Unstructured) error {
	name := obj.GetName()
	if name == "" {
		return fmt.Errorf("edited object has no name")
	}
	if name != ref.Name {
		return fmt.Errorf("edited object name %q does not match %q (rename is not supported)", name, ref.Name)
	}
	if r.Namespaced {
		if ns := obj.GetNamespace(); ns != "" && ns != ref.Namespace {
			return fmt.Errorf("edited object namespace %q does not match %q", ns, ref.Namespace)
		}
	}
	return nil
}
