package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

// PodForOwner resolves a backing pod for a pod-owning workload — Deployment,
// ReplicaSet, StatefulSet, DaemonSet, Job, or ReplicationController — so the logs
// viewer can stream a workload's logs by streaming one of its pods (M3-07b, #84).
// It reads the workload's pod selector (spec.selector), lists the pods matching it
// in the workload's namespace, and returns the newest Ready pod's ObjectRef —
// falling back to the newest pod overall when none is Ready, so a workload whose
// pods are still starting (or crash-looping) still yields a log target instead of an
// error.
//
// The workload is fetched through the dynamic client by GVR, so no per-kind typed
// client is needed and all six kinds are covered uniformly (a CRD with a
// pod selector would work too). Errors — a missing selector, no matching pods, an
// RBAC denial — are wrapped and returned, never panicked (principle 3); the caller
// degrades them to a status-bar toast.
func (c *Clients) PodForOwner(ctx context.Context, res Resource, ref ObjectRef) (ObjectRef, error) {
	if ref.Name == "" {
		return ObjectRef{}, fmt.Errorf("kube: pod for owner: empty object name")
	}
	obj, err := c.Dynamic.Resource(res.GVR).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return ObjectRef{}, fmt.Errorf("kube: getting %s %q: %w", res.GVR.Resource, ref.Name, err)
	}
	sel, err := podSelector(obj)
	if err != nil {
		return ObjectRef{}, fmt.Errorf("kube: pod selector for %s %q: %w", res.GVR.Resource, ref.Name, err)
	}
	pods, err := c.Clientset.CoreV1().Pods(ref.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return ObjectRef{}, fmt.Errorf("kube: listing pods for %s %q: %w", res.GVR.Resource, ref.Name, err)
	}
	pod := newestReadyPod(pods.Items)
	if pod == nil {
		return ObjectRef{}, fmt.Errorf("kube: no pods found for %s %q", res.GVR.Resource, ref.Name)
	}
	return ObjectRef{Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID)}, nil
}

// podSelector extracts a workload's pod label selector from its unstructured form.
// Every pod-owning kind but ReplicationController stores spec.selector as a
// metav1.LabelSelector (matchLabels + matchExpressions); ReplicationController (core
// v1) stores it as a plain label map. Both shapes are converted to a labels.Selector
// so the pod list can filter on it. A workload with no selector (an unexpected kind)
// is an error rather than a match-everything list.
func podSelector(obj *unstructured.Unstructured) (labels.Selector, error) {
	raw, found, err := unstructured.NestedMap(obj.Object, "spec", "selector")
	if err != nil {
		return nil, fmt.Errorf("reading spec.selector: %w", err)
	}
	if !found || len(raw) == 0 {
		return nil, fmt.Errorf("no pod selector")
	}
	// A LabelSelector carries matchLabels/matchExpressions; a plain RC selector is a
	// flat label map. Detect the LabelSelector shape by its distinctive keys.
	if _, ok := raw["matchLabels"]; ok {
		return labelSelectorFromMap(raw)
	}
	if _, ok := raw["matchExpressions"]; ok {
		return labelSelectorFromMap(raw)
	}
	// Plain label map (ReplicationController): every value is a string.
	set := labels.Set{}
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("selector value for %q is not a string", k)
		}
		set[k] = s
	}
	return set.AsSelector(), nil
}

// labelSelectorFromMap converts an unstructured metav1.LabelSelector
// (matchLabels/matchExpressions) into a labels.Selector.
func labelSelectorFromMap(raw map[string]any) (labels.Selector, error) {
	var ls metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &ls); err != nil {
		return nil, fmt.Errorf("decoding label selector: %w", err)
	}
	sel, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		return nil, fmt.Errorf("building selector: %w", err)
	}
	return sel, nil
}

// newestReadyPod picks the pod to stream logs from: the most recently created pod
// that is Ready, or — if none is Ready — the most recently created pod overall. A
// Ready pod is preferred because its container is actually running and producing
// logs, but a workload mid-rollout (or crash-looping) should still surface a log
// target rather than fail. Returns nil for an empty list.
func newestReadyPod(pods []corev1.Pod) *corev1.Pod {
	var best, newest *corev1.Pod
	for i := range pods {
		p := &pods[i]
		if newest == nil || p.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = p
		}
		if podReady(p) && (best == nil || p.CreationTimestamp.After(best.CreationTimestamp.Time)) {
			best = p
		}
	}
	if best != nil {
		return best
	}
	return newest
}

// podReady reports whether a pod's PodReady condition is True.
func podReady(p *corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
