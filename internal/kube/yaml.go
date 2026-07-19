package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// GetYAML fetches the object a table row references and renders it as YAML —
// the in-TUI equivalent of `kubectl get <resource> <name> -o yaml`, so a user can
// inspect the full object without an external pager (D2: in-process viewers, no
// kubectl binary). It is generic over every resource — built-in or CRD — because
// it addresses the object through the dynamic client by GVR (from the discovery
// Resource) and by namespace/name (from the row's ObjectRef), reusing the same
// resourceInterface addressing helper the action set (M1-06) is built on; the
// namespace is taken from the ref and ignored for cluster-scoped resources
// (r.Namespaced == false).
//
// managedFields are stripped before rendering: they are server-side
// apply bookkeeping, huge, and never useful to read, so kubectl itself has hidden
// them from `get`/`describe` output by default since v1.21. The remaining object
// is marshalled with sigs.k8s.io/yaml, which round-trips through JSON so the
// output honors the API types' json tags and orders keys deterministically —
// byte-for-byte what `kubectl get -o yaml` shows.
//
// Empty name is rejected. Errors are wrapped, never panicked: a NotFound (object
// gone since the row was listed) surfaces to the caller to display (#86).
func (c *Clients) GetYAML(ctx context.Context, r Resource, ref ObjectRef) (string, error) {
	if ref.Name == "" {
		return "", fmt.Errorf("kube: get yaml %s: empty object name", r.GVR.Resource)
	}
	obj, err := c.resourceInterface(r, ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("kube: getting %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	return marshalYAML(obj)
}

// marshalYAML strips managedFields from an unstructured object and renders it as
// YAML. Pure and side-effect free (it clears the field on a value copy of the
// object map, not the caller's), so it is unit-testable without a client and the
// managedFields-stripping contract is asserted directly.
func marshalYAML(obj *unstructured.Unstructured) (string, error) {
	// Copy so the caller's object is left untouched; RemoveNestedField mutates.
	out := obj.DeepCopy()
	unstructured.RemoveNestedField(out.Object, "metadata", "managedFields")
	b, err := yaml.Marshal(out.Object)
	if err != nil {
		return "", fmt.Errorf("kube: marshalling object to yaml: %w", err)
	}
	return string(b), nil
}
