package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// Drain timing knobs. Package vars (not consts) so tests can shrink them; in
// production they pace the eviction retry and the deletion poll without a busy
// loop. The overall time budget is the caller's ctx deadline, not a fixed count
// of attempts — mirroring `kubectl drain --timeout`.
var (
	// evictionRetryInterval is the wait between eviction attempts that a
	// PodDisruptionBudget is currently refusing (HTTP 429 TooManyRequests). The
	// budget frees up as other pods reschedule, so a later attempt succeeds.
	evictionRetryInterval = 5 * time.Second
	// drainPollInterval is the wait between polls while waiting for an evicted
	// pod to actually disappear from the API.
	drainPollInterval = 2 * time.Second
)

// mirrorPodAnnotation marks a static (mirror) pod the kubelet manages directly
// from a manifest on the node. Mirror pods have no controller and cannot be
// evicted through the API — deleting the mirror just makes the kubelet recreate
// it — so drain skips them, exactly as `kubectl drain` does. The value is
// kubectl's/kubelet's own annotation key (`kubernetes.io/config.mirror`),
// inlined rather than pulled from k8s.io/kubernetes (which we don't depend on).
const mirrorPodAnnotation = "kubernetes.io/config.mirror"

// DrainOptions gates which of a node's pods a drain evicts and which pods block
// it, mirroring the safety flags of `kubectl drain`. The zero value is the
// strict default: standalone pods, DaemonSet pods, and pods with local (emptyDir)
// data all *block* the drain until the caller opts into removing them.
type DrainOptions struct {
	// Force lets a drain evict standalone (unmanaged) pods — pods with no
	// controller to recreate them elsewhere. Without it such a pod blocks the
	// drain, because evicting it destroys the only copy.
	Force bool
	// IgnoreDaemonSets lets a drain proceed past DaemonSet-managed pods. Those
	// pods are never evicted either way (the DaemonSet controller would just place
	// them back on the node); without this flag their presence blocks the drain so
	// the operator notices them, with it they are silently skipped — kubectl's
	// behavior.
	IgnoreDaemonSets bool
	// DeleteEmptyDirData lets a drain evict pods that use an emptyDir volume, whose
	// contents are lost when the pod is removed. Without it such a pod blocks the
	// drain to prevent silent data loss.
	DeleteEmptyDirData bool
}

// DrainCandidates lists the pods that draining the node named by node.Name would
// evict — its live, non-mirror pods that either have a workload controller to
// reschedule them or were explicitly permitted by opts — returned as ObjectRefs
// the eviction step (M1-06e-2) acts on. It is the selection half of drain: it
// makes no changes, only decides what a drain *would* touch.
//
// Skipped without error (not evicted, not blocking): mirror pods (kubelet-owned),
// already-terminated pods (Succeeded/Failed — nothing to evict), and
// DaemonSet-managed pods when IgnoreDaemonSets is set. Blocking pods — standalone
// pods without Force, DaemonSet pods without IgnoreDaemonSets, and emptyDir-backed
// pods without DeleteEmptyDirData — are collected and returned as a single error
// naming each, matching `kubectl drain`'s upfront refusal rather than a partial
// drain. When that error is non-nil the returned slice is nil: a drain either
// proceeds on all eligible pods or refuses outright.
//
// Pods are listed with the typed clientset filtered by `spec.nodeName` (the
// server-side field selector `kubectl drain` uses), across all namespaces. An
// empty node name is rejected; a list error is wrapped, never panicked (#86).
func (c *Clients) DrainCandidates(ctx context.Context, node ObjectRef, opts DrainOptions) ([]ObjectRef, error) {
	if node.Name == "" {
		return nil, fmt.Errorf("kube: drain: empty node name")
	}
	sel := fields.OneTermEqualSelector("spec.nodeName", node.Name).String()
	list, err := c.Clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: sel})
	if err != nil {
		return nil, fmt.Errorf("kube: draining node %q: listing pods: %w", node.Name, err)
	}
	evict, blocked := classifyDrainPods(list.Items, opts)
	if len(blocked) > 0 {
		return nil, fmt.Errorf("kube: draining node %q: cannot evict %d pod(s): %s",
			node.Name, len(blocked), strings.Join(blocked, "; "))
	}
	return evict, nil
}

// classifyDrainPods is the pure decision core of DrainCandidates: given a node's
// pods and the drain options, it returns the pods to evict (as ObjectRefs) and a
// list of human-readable reasons for every pod that blocks the drain. It has no
// client or node context, so the whole selection policy is unit-testable without a
// cluster. The order of checks matters — mirror and terminated pods are dropped
// before controller classification, and DaemonSet/standalone status is decided
// before the emptyDir gate — so each pod is reported at most once.
func classifyDrainPods(pods []corev1.Pod, opts DrainOptions) (evict []ObjectRef, blocked []string) {
	for i := range pods {
		p := &pods[i]
		if isMirrorPod(p) {
			continue // kubelet-managed; cannot be evicted through the API.
		}
		if isTerminated(p) {
			continue // Succeeded/Failed; nothing running to evict.
		}
		switch ctrl := metav1.GetControllerOf(p); {
		case ctrl == nil:
			// Standalone pod: no controller to recreate it elsewhere.
			if !opts.Force {
				blocked = append(blocked, podReason(p, "unmanaged, would be lost; set Force"))
				continue
			}
		case ctrl.Kind == "DaemonSet":
			// DaemonSet pods are never evicted (the controller re-places them);
			// without IgnoreDaemonSets their presence blocks the drain.
			if !opts.IgnoreDaemonSets {
				blocked = append(blocked, podReason(p, "managed by DaemonSet; set IgnoreDaemonSets"))
			}
			continue
		}
		if hasLocalStorage(p) && !opts.DeleteEmptyDirData {
			blocked = append(blocked, podReason(p, "uses emptyDir, data would be lost; set DeleteEmptyDirData"))
			continue
		}
		evict = append(evict, ObjectRef{Namespace: p.Namespace, Name: p.Name, UID: string(p.UID)})
	}
	return evict, blocked
}

// isMirrorPod reports whether the pod is a static/mirror pod the kubelet owns.
func isMirrorPod(p *corev1.Pod) bool {
	_, ok := p.Annotations[mirrorPodAnnotation]
	return ok
}

// isTerminated reports whether the pod has already reached a terminal phase and
// so has nothing left to evict.
func isTerminated(p *corev1.Pod) bool {
	return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed
}

// hasLocalStorage reports whether the pod mounts an emptyDir volume, whose data
// does not survive the pod being removed.
func hasLocalStorage(p *corev1.Pod) bool {
	for i := range p.Spec.Volumes {
		if p.Spec.Volumes[i].EmptyDir != nil {
			return true
		}
	}
	return false
}

// podReason formats a "namespace/name: why" blocker line for the aggregated
// drain-refusal error.
func podReason(p *corev1.Pod, why string) string {
	return fmt.Sprintf("%s/%s (%s)", p.Namespace, p.Name, why)
}

// Drain empties a node so it can be taken out of service: it cordons the node,
// evicts every pod DrainCandidates selected through the policy/v1 Eviction API
// (which honors PodDisruptionBudgets), and waits for each to disappear — the
// same sequence as `kubectl drain`. nodeRes is the discovered `nodes` Resource
// (its GVR drives the cordon patch), node is the row ObjectRef, and opts gates
// which pods are drainable (see DrainCandidates / DrainOptions).
//
// Order is deliberate: candidates are computed *first*, so a drain that would be
// refused (a blocking pod, opts too strict) fails before the node is cordoned —
// a cordon is never left behind on a node that then refuses to drain. Only once
// the pod set is settled does Drain cordon (reusing M1-06c, so the scheduler
// places no new pods during the drain) and begin evicting.
//
// Eviction is PDB-aware: the Eviction API returns 429 TooManyRequests while a
// PodDisruptionBudget has no disruptions left, so evictPod retries with backoff
// until the budget frees up or ctx expires. After all evictions are requested,
// Drain waits for every pod to actually be deleted (an eviction only *requests*
// graceful termination). Pods are evicted then waited on in two passes so their
// grace periods overlap rather than serialize. The overall deadline is ctx's;
// errors are wrapped, never panicked (#86).
func (c *Clients) Drain(ctx context.Context, nodeRes Resource, node ObjectRef, opts DrainOptions) error {
	candidates, err := c.DrainCandidates(ctx, node, opts)
	if err != nil {
		return err // already wrapped, names the node and the blocking pods
	}
	if err := c.Cordon(ctx, nodeRes, node); err != nil {
		return fmt.Errorf("kube: draining node %q: cordon: %w", node.Name, err)
	}
	for _, pod := range candidates {
		if err := c.evictPod(ctx, pod); err != nil {
			return fmt.Errorf("kube: draining node %q: %w", node.Name, err)
		}
	}
	for _, pod := range candidates {
		if err := c.waitPodDeleted(ctx, pod); err != nil {
			return fmt.Errorf("kube: draining node %q: %w", node.Name, err)
		}
	}
	return nil
}

// evictPod requests eviction of one pod through the policy/v1 Eviction API — the
// same subresource `kubectl drain` posts to, so eviction is checked against any
// PodDisruptionBudget guarding the pod rather than a blind delete. A 429
// TooManyRequests means a PDB is momentarily out of allowed disruptions; evictPod
// waits evictionRetryInterval and retries, because the budget frees up as other
// pods reschedule. A NotFound means the pod already went away (raced with its
// controller or a prior drain) — nothing to do, treated as success. Any other
// error is wrapped and returned. Retrying respects ctx: a cancelled or timed-out
// context ends the loop with ctx's error.
func (c *Clients) evictPod(ctx context.Context, pod ObjectRef) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name},
	}
	for {
		err := c.Clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
		switch {
		case err == nil:
			return nil
		case apierrors.IsNotFound(err):
			return nil // already gone
		case apierrors.IsTooManyRequests(err):
			// PDB currently disallows the disruption; wait and retry.
			select {
			case <-ctx.Done():
				return fmt.Errorf("evicting pod %s/%s: %w", pod.Namespace, pod.Name, ctx.Err())
			case <-time.After(evictionRetryInterval):
			}
		default:
			return fmt.Errorf("evicting pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
}

// waitPodDeleted blocks until a pod evicted by evictPod has actually left the
// API — an eviction only *requests* graceful termination, so the object lingers
// through its grace period. It polls every drainPollInterval and returns once the
// pod is NotFound, or once the name resolves to a *different* object (a new pod
// recreated under the same name — identified by a changed UID — means the
// original is gone). A poll error other than NotFound is wrapped; ctx expiry ends
// the wait with a wrapped deadline error. When the ref carries no UID (degraded
// metadata) only disappearance counts as deletion.
func (c *Clients) waitPodDeleted(ctx context.Context, pod ObjectRef) error {
	for {
		got, err := c.Clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return nil
		case err != nil:
			return fmt.Errorf("waiting for pod %s/%s to delete: %w", pod.Namespace, pod.Name, err)
		case pod.UID != "" && string(got.UID) != pod.UID:
			return nil // a new object took the name; the original is gone
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for pod %s/%s to delete: %w", pod.Namespace, pod.Name, ctx.Err())
		case <-time.After(drainPollInterval):
		}
	}
}
