package kube

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// restartedAtAnnotation is the pod-template annotation kubectl stamps on a
// `rollout restart`. Reusing kubectl's exact key means a kubecom restart and a
// `kubectl rollout restart` are interchangeable — both mutate the same field, so
// neither surprises the other and a restart is visible in `kubectl describe`.
const restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

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

// Scale sets the desired replica count of a scalable workload — Deployment,
// ReplicaSet, StatefulSet, ReplicationController, or any CRD that exposes a
// `scale` subresource. Like Delete it stays generic by going through the dynamic
// client: it merge-patches the object's `scale` subresource, so the same one code
// path scales built-ins and CRDs with no per-kind wiring. Patching the scale
// subresource (not the object body) is what makes it uniform — every scalable
// kind stores replicas at `scale.spec.replicas` regardless of where it lives in
// the object's own schema.
//
// A negative replica count is rejected locally (the server would reject it too,
// but a clear local error avoids a needless round-trip). Empty name is rejected.
// Errors are wrapped, never panicked; a NotFound (object gone since the row was
// listed) surfaces for the caller to display (#86). Unlike Delete there is no UID
// precondition: PatchOptions carries none, and a scale is idempotent — re-issuing
// it converges rather than destroying a wrongly-matched object.
func (c *Clients) Scale(ctx context.Context, r Resource, ref ObjectRef, replicas int32) error {
	if ref.Name == "" {
		return fmt.Errorf("kube: scale %s: empty object name", r.GVR.Resource)
	}
	if replicas < 0 {
		return fmt.Errorf("kube: scale %s %q: replicas must be >= 0, got %d", r.GVR.Resource, ref.Name, replicas)
	}
	if _, err := c.resourceInterface(r, ref.Namespace).Patch(
		ctx, ref.Name, types.MergePatchType, scalePatch(replicas), metav1.PatchOptions{}, "scale",
	); err != nil {
		return fmt.Errorf("kube: scaling %s %q to %d: %w", r.GVR.Resource, ref.Name, replicas, err)
	}
	return nil
}

// RolloutRestart triggers a rolling restart of a pod-template workload —
// Deployment, DaemonSet, or StatefulSet — the same way `kubectl rollout restart`
// does: it stamps the current time into the pod template's restartedAt annotation
// (`kubectl.kubernetes.io/restartedAt`). Mutating the template is what the
// controller observes as a change, so it rolls all pods; reusing kubectl's exact
// annotation key keeps kubecom and kubectl restarts interchangeable and stops the
// annotation from proliferating across repeated restarts.
//
// It merge-patches through the dynamic client, so it is generic over the workload
// kinds (and any CRD with a pod template at the same path) with no per-kind
// wiring. A merge patch on the annotations map only sets restartedAt and leaves
// other annotations intact — identical in effect to kubectl's strategic merge for
// this add, but without needing a per-type schema, so it works on unstructured
// objects and CRDs. Empty name is rejected; errors are wrapped, never panicked.
func (c *Clients) RolloutRestart(ctx context.Context, r Resource, ref ObjectRef) error {
	if ref.Name == "" {
		return fmt.Errorf("kube: rollout restart %s: empty object name", r.GVR.Resource)
	}
	if _, err := c.resourceInterface(r, ref.Namespace).Patch(
		ctx, ref.Name, types.MergePatchType, restartPatch(time.Now()), metav1.PatchOptions{},
	); err != nil {
		return fmt.Errorf("kube: restarting %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	return nil
}

// Cordon marks a node unschedulable so the scheduler places no *new* pods on it
// (existing pods keep running — evicting them is drain's job, M1-06e). It is the
// same operation as `kubectl cordon`: a merge patch setting `spec.unschedulable`
// to true. Like the other actions it goes through the dynamic client so it needs
// no typed node client; nodes are cluster-scoped, so the ref's namespace is
// ignored (r.Namespaced == false). Empty name is rejected; errors are wrapped,
// never panicked; a NotFound (node gone since the row was listed) surfaces for the
// caller (#86). Cordoning is idempotent, so — like Scale — there is no UID guard.
func (c *Clients) Cordon(ctx context.Context, r Resource, ref ObjectRef) error {
	return c.setUnschedulable(ctx, r, ref, true)
}

// Uncordon reverses Cordon: it marks a node schedulable again by merge-patching
// `spec.unschedulable` to false, matching `kubectl uncordon`. Setting the field to
// the concrete value false (not null) is deliberate — a merge patch only removes a
// key when the value is null, and an explicit false reads the same to the
// scheduler while keeping the field present. Same generic dynamic-client path,
// name check, wrapped errors, and idempotence as Cordon.
func (c *Clients) Uncordon(ctx context.Context, r Resource, ref ObjectRef) error {
	return c.setUnschedulable(ctx, r, ref, false)
}

// setUnschedulable is the shared body of Cordon/Uncordon: it merge-patches a
// node's `spec.unschedulable` flag through the dynamic client. Factoring it out
// keeps the two public actions to a single line each and the wire format in one
// place (unschedulablePatch). The verb in error messages tracks the flag so a
// failure reads naturally ("cordoning"/"uncordoning").
func (c *Clients) setUnschedulable(ctx context.Context, r Resource, ref ObjectRef, unschedulable bool) error {
	if ref.Name == "" {
		return fmt.Errorf("kube: %s %s: empty object name", cordonNoun(unschedulable), r.GVR.Resource)
	}
	if _, err := c.resourceInterface(r, ref.Namespace).Patch(
		ctx, ref.Name, types.MergePatchType, unschedulablePatch(unschedulable), metav1.PatchOptions{},
	); err != nil {
		return fmt.Errorf("kube: %s %s %q: %w", cordonVerb(unschedulable), r.GVR.Resource, ref.Name, err)
	}
	return nil
}

// cordonNoun / cordonVerb name the operation for error messages, keyed off the
// target flag: unschedulable=true is a cordon, false is an uncordon.
func cordonNoun(unschedulable bool) string {
	if unschedulable {
		return "cordon"
	}
	return "uncordon"
}

func cordonVerb(unschedulable bool) string {
	if unschedulable {
		return "cordoning"
	}
	return "uncordoning"
}

// unschedulablePatch builds the RFC 7386 merge patch that toggles a node's
// spec.unschedulable flag — the wire format kubectl cordon/uncordon send. Pure and
// side-effect free so it is unit-testable without a client (the fake dynamic
// client applies the merge patch to the whole tracked object, exercising the
// round-trip in the Cordon/Uncordon tests).
func unschedulablePatch(unschedulable bool) []byte {
	return []byte(fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, unschedulable))
}

// scalePatch builds the RFC 7386 merge patch that sets a scale subresource's
// replica count. Pure and side-effect free so the wire format is unit-testable
// without a client (the fake dynamic client applies the patch to the whole
// tracked object, so the round-trip is exercised there).
func scalePatch(replicas int32) []byte {
	return []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
}

// restartPatch builds the merge patch that stamps restartedAt into a workload's
// pod-template annotations, mirroring kubectl's rollout restart. The timestamp is
// UTC RFC 3339 (kubectl's format). Pure, so both the annotation key and the
// timestamp format are unit-testable without a client or a real clock.
func restartPatch(now time.Time) []byte {
	return []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		restartedAtAnnotation, now.UTC().Format(time.RFC3339),
	))
}
