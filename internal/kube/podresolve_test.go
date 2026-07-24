package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var rcGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "replicationcontrollers"}

// dynamicFakeWith returns a fake dynamic client that also knows the
// replicationcontrollers GVR (which newDynamicFake omits), so PodForOwner's Get of an
// RC resolves.
func dynamicFakeWith(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		podsResource.GVR:        "PodList",
		deploymentsResource.GVR: "DeploymentList",
		rcGVR:                   "ReplicationControllerList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
}

// workloadWithSelector builds an unstructured workload (Deployment/RS/StatefulSet/
// DaemonSet/Job) whose spec.selector is a metav1.LabelSelector over matchLabels — the
// shape PodForOwner reads for every pod-owning kind but ReplicationController.
func workloadWithSelector(apiVersion, kind, ns, name string, matchLabels map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"namespace": ns, "name": name},
		"spec": map[string]any{
			"selector": map[string]any{"matchLabels": matchLabels},
		},
	}}
}

// labeledPod builds a fake pod with labels, a Ready condition, and a creation time,
// so the newest-ready selection can be exercised.
func labeledPod(ns, name string, labels map[string]string, ready bool, created time.Time) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         ns,
			Name:              name,
			Labels:            labels,
			UID:               types.UID("uid-" + name),
			CreationTimestamp: metav1.NewTime(created),
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

// TestPodForOwnerNewestReadyPod resolves a Deployment to the newest *Ready* pod
// matching its selector — not the newest pod overall (which is not ready) and not an
// unrelated pod (excluded by the selector).
func TestPodForOwnerNewestReadyPod(t *testing.T) {
	dep := workloadWithSelector("apps/v1", "Deployment", "web", "api", map[string]any{"app": "api"})
	sel := map[string]string{"app": "api"}
	c := &Clients{
		Dynamic: newDynamicFake(dep),
		Clientset: k8sfake.NewSimpleClientset(
			labeledPod("web", "api-old", sel, true, epoch),
			labeledPod("web", "api-new", sel, true, epoch.Add(2*time.Hour)),
			labeledPod("web", "api-newest", sel, false, epoch.Add(3*time.Hour)),        // newest but not ready
			labeledPod("web", "other", map[string]string{"app": "db"}, true, epoch.Add(9*time.Hour)), // excluded by selector
		),
	}

	got, err := c.PodForOwner(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodForOwner: %v", err)
	}
	if got.Name != "api-new" {
		t.Errorf("resolved pod = %q, want api-new (newest ready matching)", got.Name)
	}
	if got.Namespace != "web" || got.UID != "uid-api-new" {
		t.Errorf("resolved ref = %+v, want ns=web uid=uid-api-new", got)
	}
}

// TestPodForOwnerFallsBackToNewestWhenNoneReady resolves to the newest pod overall
// when the workload has no Ready pod (mid-rollout / crash-loop), so logs stay
// reachable rather than erroring.
func TestPodForOwnerFallsBackToNewestWhenNoneReady(t *testing.T) {
	dep := workloadWithSelector("apps/v1", "Deployment", "web", "api", map[string]any{"app": "api"})
	sel := map[string]string{"app": "api"}
	c := &Clients{
		Dynamic: newDynamicFake(dep),
		Clientset: k8sfake.NewSimpleClientset(
			labeledPod("web", "api-old", sel, false, epoch),
			labeledPod("web", "api-newest", sel, false, epoch.Add(3*time.Hour)),
		),
	}

	got, err := c.PodForOwner(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodForOwner: %v", err)
	}
	if got.Name != "api-newest" {
		t.Errorf("resolved pod = %q, want api-newest (newest overall, none ready)", got.Name)
	}
}

// TestPodForOwnerReplicationController reads a plain-map spec.selector (the RC shape,
// not a LabelSelector) and still resolves a backing pod.
func TestPodForOwnerReplicationController(t *testing.T) {
	rc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ReplicationController",
		"metadata":   map[string]any{"namespace": "web", "name": "legacy"},
		"spec": map[string]any{
			"selector": map[string]any{"app": "legacy"}, // plain label map, not matchLabels
		},
	}}
	rcResource := Resource{
		GVR:        rcGVR,
		Namespaced: true,
	}
	c := &Clients{
		Dynamic:   dynamicFakeWith(rc),
		Clientset: k8sfake.NewSimpleClientset(labeledPod("web", "legacy-1", map[string]string{"app": "legacy"}, true, epoch)),
	}

	got, err := c.PodForOwner(context.Background(), rcResource, ObjectRef{Namespace: "web", Name: "legacy"})
	if err != nil {
		t.Fatalf("PodForOwner: %v", err)
	}
	if got.Name != "legacy-1" {
		t.Errorf("resolved pod = %q, want legacy-1", got.Name)
	}
}

// TestPodForOwnerNoSelector errors (rather than listing every pod) when the workload
// has no pod selector.
func TestPodForOwnerNoSelector(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": "web", "name": "api"},
		"spec":       map[string]any{},
	}}
	c := &Clients{Dynamic: newDynamicFake(obj), Clientset: k8sfake.NewSimpleClientset()}

	_, err := c.PodForOwner(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"})
	if err == nil {
		t.Fatal("PodForOwner with no selector: want error, got nil")
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Errorf("error = %v, want it to mention the missing selector", err)
	}
}

// TestPodForOwnerNoPods errors when the selector matches no pods, so the caller can
// toast "no pods" rather than open an empty viewer.
func TestPodForOwnerNoPods(t *testing.T) {
	dep := workloadWithSelector("apps/v1", "Deployment", "web", "api", map[string]any{"app": "api"})
	c := &Clients{Dynamic: newDynamicFake(dep), Clientset: k8sfake.NewSimpleClientset()}

	_, err := c.PodForOwner(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"})
	if err == nil {
		t.Fatal("PodForOwner with no pods: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no pods") {
		t.Errorf("error = %v, want it to mention no pods", err)
	}
}

// TestPodForOwnerEmptyName rejects an empty object name before touching the API.
func TestPodForOwnerEmptyName(t *testing.T) {
	c := &Clients{}
	if _, err := c.PodForOwner(context.Background(), deploymentsResource, ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("PodForOwner with empty name: want error, got nil")
	}
}

// serviceWithSelector builds a fake Service whose spec.selector is a flat label map
// (the shape PodForService reads via the typed clientset).
func serviceWithSelector(ns, name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

// TestPodForServiceNewestReadyPod resolves a Service to the newest *Ready* pod
// matching its selector — not the newest pod overall (not ready) and not an unrelated
// pod (excluded by the selector) — mirroring PodForOwner's selection.
func TestPodForServiceNewestReadyPod(t *testing.T) {
	sel := map[string]string{"app": "api"}
	c := &Clients{
		Clientset: k8sfake.NewSimpleClientset(
			serviceWithSelector("web", "api", sel),
			labeledPod("web", "api-old", sel, true, epoch),
			labeledPod("web", "api-new", sel, true, epoch.Add(2*time.Hour)),
			labeledPod("web", "api-newest", sel, false, epoch.Add(3*time.Hour)),          // newest but not ready
			labeledPod("web", "other", map[string]string{"app": "db"}, true, epoch.Add(9*time.Hour)), // excluded by selector
		),
	}

	got, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodForService: %v", err)
	}
	if got.Name != "api-new" {
		t.Errorf("resolved pod = %q, want api-new (newest ready matching)", got.Name)
	}
	if got.Namespace != "web" || got.UID != "uid-api-new" {
		t.Errorf("resolved ref = %+v, want ns=web uid=uid-api-new", got)
	}
}

// TestPodForServiceFallsBackToNewestWhenNoneReady resolves to the newest pod overall
// when the Service's endpoint pods are all not-Ready (mid-rollout), so a forward target
// is still surfaced rather than erroring.
func TestPodForServiceFallsBackToNewestWhenNoneReady(t *testing.T) {
	sel := map[string]string{"app": "api"}
	c := &Clients{
		Clientset: k8sfake.NewSimpleClientset(
			serviceWithSelector("web", "api", sel),
			labeledPod("web", "api-old", sel, false, epoch),
			labeledPod("web", "api-newest", sel, false, epoch.Add(3*time.Hour)),
		),
	}

	got, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodForService: %v", err)
	}
	if got.Name != "api-newest" {
		t.Errorf("resolved pod = %q, want api-newest (newest overall, none ready)", got.Name)
	}
}

// TestPodForServiceNoSelector errors (rather than listing every pod) for a selector-less
// Service — a headless service with manual Endpoints, or an ExternalName service — which
// has no backing pods to forward to.
func TestPodForServiceNoSelector(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(serviceWithSelector("web", "external", nil))}

	_, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web", Name: "external"})
	if err == nil {
		t.Fatal("PodForService with no selector: want error, got nil")
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Errorf("error = %v, want it to mention the missing selector", err)
	}
}

// TestPodForServiceNoPods errors when the selector matches no pods, so the caller can
// toast "no pods" rather than start a forward to nothing.
func TestPodForServiceNoPods(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(serviceWithSelector("web", "api", map[string]string{"app": "api"}))}

	_, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web", Name: "api"})
	if err == nil {
		t.Fatal("PodForService with no pods: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no pods") {
		t.Errorf("error = %v, want it to mention no pods", err)
	}
}

// TestPodForServiceMissingService wraps the typed Get error (NotFound) rather than
// panicking when the Service itself is gone.
func TestPodForServiceMissingService(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}

	_, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web", Name: "ghost"})
	if err == nil {
		t.Fatal("PodForService for a missing service: want error, got nil")
	}
}

// TestPodForServiceEmptyName rejects an empty service name before touching the API.
func TestPodForServiceEmptyName(t *testing.T) {
	c := &Clients{}
	if _, err := c.PodForService(context.Background(), ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("PodForService with empty name: want error, got nil")
	}
}
