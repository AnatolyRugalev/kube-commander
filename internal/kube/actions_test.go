package kube

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	podsResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Namespaced: true,
	}
	nodesResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
		Namespaced: false,
	}
	deploymentsResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		Namespaced: true,
	}
	cronJobsResource = Resource{
		GVR:        schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"},
		Namespaced: true,
	}
)

// deploymentObj builds an unstructured Deployment with a replica count and an
// (optional) pre-existing pod-template annotation, so the scale/restart
// round-trips have something realistic to patch.
func deploymentObj(namespace, name string, replicas int64, tmplAnnotations map[string]any) *unstructured.Unstructured {
	tmplMeta := map[string]any{}
	if tmplAnnotations != nil {
		tmplMeta["annotations"] = tmplAnnotations
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec": map[string]any{
			"replicas": replicas,
			"template": map[string]any{"metadata": tmplMeta},
		},
	}}
}

func unstructuredObj(apiVersion, kind, namespace, name, uid string) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "uid": uid}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   meta,
	}}
}

// newDynamicFake returns a fake dynamic client seeded with the given objects and
// the list-kind mapping the pods/nodes tests need. Custom list kinds are supplied
// explicitly so the fake never has to guess them from a scheme.
func newDynamicFake(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		podsResource.GVR:        "PodList",
		nodesResource.GVR:       "NodeList",
		deploymentsResource.GVR: "DeploymentList",
		cronJobsResource.GVR:    "CronJobList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
}

func exists(t *testing.T, dc *dynamicfake.FakeDynamicClient, r Resource, ns, name string) bool {
	t.Helper()
	ri := dc.Resource(r.GVR)
	var err error
	if r.Namespaced {
		_, err = ri.Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	} else {
		_, err = ri.Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return err == nil
}

// withUIDPrecondition owns the object-identity guard; the fake dynamic client
// discards DeleteOptions (see its Delete: NewDeleteAction takes no opts), so the
// precondition logic is verified here as a pure function, and the fake exercises
// the round-trip (removal, scoping, errors) below.
func TestWithUIDPrecondition(t *testing.T) {
	rv := "12345"

	// UID present, no caller preconditions → a UID precondition is added.
	got := withUIDPrecondition(ObjectRef{Name: "p", UID: "uid-1"}, metav1.DeleteOptions{})
	if got.Preconditions == nil || got.Preconditions.UID == nil || string(*got.Preconditions.UID) != "uid-1" {
		t.Errorf("UID guard = %+v, want uid-1", got.Preconditions)
	}

	// No UID (degraded metadata) → opts unchanged, no precondition invented.
	got = withUIDPrecondition(ObjectRef{Name: "p"}, metav1.DeleteOptions{})
	if got.Preconditions != nil {
		t.Errorf("preconditions = %+v, want nil when ref has no UID", got.Preconditions)
	}

	// Caller already set preconditions → they win; no UID is layered on.
	got = withUIDPrecondition(
		ObjectRef{Name: "p", UID: "uid-1"},
		metav1.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &rv}},
	)
	if got.Preconditions == nil || got.Preconditions.ResourceVersion == nil || *got.Preconditions.ResourceVersion != "12345" {
		t.Errorf("ResourceVersion precondition = %+v, want preserved 12345", got.Preconditions)
	}
	if got.Preconditions.UID != nil {
		t.Errorf("UID = %v, want nil (caller preconditions preserved)", *got.Preconditions.UID)
	}
}

func TestDeleteNamespacedRemovesObject(t *testing.T) {
	dc := newDynamicFake(unstructuredObj("v1", "Pod", "web", "nginx-abc", "uid-1"))

	var captured clienttesting.Action
	dc.PrependReactor("delete", "pods", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil // observe only; let the tracker perform the delete
	})

	c := &Clients{Dynamic: dc}
	ref := ObjectRef{Namespace: "web", Name: "nginx-abc", UID: "uid-1"}
	if err := c.Delete(context.Background(), podsResource, ref, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists(t, dc, podsResource, "web", "nginx-abc") {
		t.Error("pod still present after Delete")
	}
	if captured == nil {
		t.Fatal("no delete action recorded")
	}
	if got := captured.GetNamespace(); got != "web" {
		t.Errorf("namespace = %q, want web", got)
	}
}

func TestDeleteClusterScopedIgnoresNamespace(t *testing.T) {
	dc := newDynamicFake(unstructuredObj("v1", "Node", "", "node-1", "uid-n"))

	var captured clienttesting.Action
	dc.PrependReactor("delete", "nodes", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil
	})

	c := &Clients{Dynamic: dc}
	// A namespace on the ref must be ignored for a cluster-scoped resource.
	ref := ObjectRef{Namespace: "ignored", Name: "node-1", UID: "uid-n"}
	if err := c.Delete(context.Background(), nodesResource, ref, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if exists(t, dc, nodesResource, "", "node-1") {
		t.Error("node still present after Delete")
	}
	if captured == nil {
		t.Fatal("no delete action recorded")
	}
	if got := captured.GetNamespace(); got != "" {
		t.Errorf("namespace = %q, want empty (cluster-scoped)", got)
	}
}

func TestDeleteEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.Delete(context.Background(), podsResource, ObjectRef{Namespace: "web"}, metav1.DeleteOptions{}); err == nil {
		t.Fatal("Delete(empty name): want error, got nil")
	}
}

func TestDeleteNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	ref := ObjectRef{Namespace: "web", Name: "ghost", UID: "uid-x"}
	err := c.Delete(context.Background(), podsResource, ref, metav1.DeleteOptions{})
	if err == nil {
		t.Fatal("Delete(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}

// scalePatch/restartPatch own the wire format; assert it directly since the fake
// dynamic client applies a merge patch to the whole tracked object, which the
// round-trip tests below then exercise.
func TestScalePatch(t *testing.T) {
	if got, want := string(scalePatch(3)), `{"spec":{"replicas":3}}`; got != want {
		t.Errorf("scalePatch(3) = %s, want %s", got, want)
	}
	if got, want := string(scalePatch(0)), `{"spec":{"replicas":0}}`; got != want {
		t.Errorf("scalePatch(0) = %s, want %s", got, want)
	}
}

func TestRestartPatch(t *testing.T) {
	now := time.Date(2026, 7, 18, 9, 30, 0, 0, time.FixedZone("CEST", 2*3600))
	want := `{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"2026-07-18T07:30:00Z"}}}}}`
	if got := string(restartPatch(now)); got != want {
		t.Errorf("restartPatch = %s, want %s (UTC RFC3339, kubectl's key)", got, want)
	}
}

func TestScaleUpdatesReplicas(t *testing.T) {
	dc := newDynamicFake(deploymentObj("web", "api", 1, nil))

	var captured clienttesting.Action
	dc.PrependReactor("patch", "deployments", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil // observe only; let the tracker apply the patch
	})

	c := &Clients{Dynamic: dc}
	ref := ObjectRef{Namespace: "web", Name: "api"}
	if err := c.Scale(context.Background(), deploymentsResource, ref, 3); err != nil {
		t.Fatalf("Scale: %v", err)
	}

	got := getDeployment(t, dc, "web", "api")
	replicas, found, err := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if err != nil || !found {
		t.Fatalf("spec.replicas missing after Scale (found=%v err=%v)", found, err)
	}
	if replicas != 3 {
		t.Errorf("spec.replicas = %d, want 3", replicas)
	}
	if pa, ok := captured.(clienttesting.PatchAction); !ok || pa.GetSubresource() != "scale" {
		t.Errorf("patch subresource = %q, want scale", captured.GetSubresource())
	}
}

func TestScaleEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.Scale(context.Background(), deploymentsResource, ObjectRef{Namespace: "web"}, 3); err == nil {
		t.Fatal("Scale(empty name): want error, got nil")
	}
}

func TestScaleNegativeReplicas(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake(deploymentObj("web", "api", 1, nil))}
	if err := c.Scale(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}, -1); err == nil {
		t.Fatal("Scale(-1): want error, got nil")
	}
}

func TestScaleNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	err := c.Scale(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "ghost"}, 2)
	if err == nil {
		t.Fatal("Scale(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}

func TestRolloutRestartStampsAnnotation(t *testing.T) {
	// Seed a pre-existing template annotation to prove the merge patch adds
	// restartedAt without clobbering siblings.
	dc := newDynamicFake(deploymentObj("web", "api", 2, map[string]any{"team": "core"}))

	c := &Clients{Dynamic: dc}
	before := time.Now().Add(-time.Second)
	if err := c.RolloutRestart(context.Background(), deploymentsResource, ObjectRef{Namespace: "web", Name: "api"}); err != nil {
		t.Fatalf("RolloutRestart: %v", err)
	}

	got := getDeployment(t, dc, "web", "api")
	anns, found, err := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	if err != nil || !found {
		t.Fatalf("template annotations missing after RolloutRestart (found=%v err=%v)", found, err)
	}
	if anns["team"] != "core" {
		t.Errorf("sibling annotation clobbered: team = %q, want core", anns["team"])
	}
	stamp, ok := anns["kubectl.kubernetes.io/restartedAt"]
	if !ok {
		t.Fatalf("restartedAt annotation not set; got %v", anns)
	}
	ts, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("restartedAt %q not RFC3339: %v", stamp, err)
	}
	if ts.Before(before) {
		t.Errorf("restartedAt %v older than call time %v", ts, before)
	}
}

func TestRolloutRestartEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.RolloutRestart(context.Background(), deploymentsResource, ObjectRef{Namespace: "web"}); err == nil {
		t.Fatal("RolloutRestart(empty name): want error, got nil")
	}
}

// unschedulablePatch owns the cordon/uncordon wire format; assert it directly, the
// round-trip through the fake dynamic client is exercised below.
func TestUnschedulablePatch(t *testing.T) {
	if got, want := string(unschedulablePatch(true)), `{"spec":{"unschedulable":true}}`; got != want {
		t.Errorf("unschedulablePatch(true) = %s, want %s", got, want)
	}
	if got, want := string(unschedulablePatch(false)), `{"spec":{"unschedulable":false}}`; got != want {
		t.Errorf("unschedulablePatch(false) = %s, want %s", got, want)
	}
}

// nodeObj builds an unstructured Node with an (optional) pre-set unschedulable
// flag so the cordon/uncordon round-trips have something realistic to patch.
func nodeObj(name string, unschedulable bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"unschedulable": unschedulable},
	}}
}

func nodeUnschedulable(t *testing.T, dc *dynamicfake.FakeDynamicClient, name string) bool {
	t.Helper()
	obj, err := dc.Resource(nodesResource.GVR).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get node %s: %v", name, err)
	}
	u, found, err := unstructured.NestedBool(obj.Object, "spec", "unschedulable")
	if err != nil || !found {
		t.Fatalf("spec.unschedulable missing on node %s (found=%v err=%v)", name, found, err)
	}
	return u
}

func TestCordonMarksUnschedulable(t *testing.T) {
	dc := newDynamicFake(nodeObj("node-1", false))

	var captured clienttesting.Action
	dc.PrependReactor("patch", "nodes", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil // observe only; let the tracker apply the patch
	})

	c := &Clients{Dynamic: dc}
	// A namespace on the ref must be ignored for a cluster-scoped node.
	ref := ObjectRef{Namespace: "ignored", Name: "node-1"}
	if err := c.Cordon(context.Background(), nodesResource, ref); err != nil {
		t.Fatalf("Cordon: %v", err)
	}
	if !nodeUnschedulable(t, dc, "node-1") {
		t.Error("spec.unschedulable = false after Cordon, want true")
	}
	if captured == nil {
		t.Fatal("no patch action recorded")
	}
	if got := captured.GetNamespace(); got != "" {
		t.Errorf("namespace = %q, want empty (cluster-scoped)", got)
	}
}

func TestUncordonMarksSchedulable(t *testing.T) {
	dc := newDynamicFake(nodeObj("node-1", true))

	c := &Clients{Dynamic: dc}
	if err := c.Uncordon(context.Background(), nodesResource, ObjectRef{Name: "node-1"}); err != nil {
		t.Fatalf("Uncordon: %v", err)
	}
	if nodeUnschedulable(t, dc, "node-1") {
		t.Error("spec.unschedulable = true after Uncordon, want false")
	}
}

func TestCordonEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.Cordon(context.Background(), nodesResource, ObjectRef{}); err == nil {
		t.Fatal("Cordon(empty name): want error, got nil")
	}
	if err := c.Uncordon(context.Background(), nodesResource, ObjectRef{}); err == nil {
		t.Fatal("Uncordon(empty name): want error, got nil")
	}
}

func TestCordonNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	err := c.Cordon(context.Background(), nodesResource, ObjectRef{Name: "ghost"})
	if err == nil {
		t.Fatal("Cordon(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}

// suspendPatch owns the suspend/resume wire format; assert it directly, the
// round-trip through the fake dynamic client is exercised below.
func TestSuspendPatch(t *testing.T) {
	if got, want := string(suspendPatch(true)), `{"spec":{"suspend":true}}`; got != want {
		t.Errorf("suspendPatch(true) = %s, want %s", got, want)
	}
	if got, want := string(suspendPatch(false)), `{"spec":{"suspend":false}}`; got != want {
		t.Errorf("suspendPatch(false) = %s, want %s", got, want)
	}
}

// cronJobObj builds an unstructured CronJob with an (optional) pre-set suspend
// flag so the suspend/resume round-trips have something realistic to patch.
func cronJobObj(namespace, name string, suspend bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"suspend": suspend, "schedule": "* * * * *"},
	}}
}

func cronJobSuspended(t *testing.T, dc *dynamicfake.FakeDynamicClient, ns, name string) bool {
	t.Helper()
	obj, err := dc.Resource(cronJobsResource.GVR).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cronjob %s/%s: %v", ns, name, err)
	}
	s, found, err := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if err != nil || !found {
		t.Fatalf("spec.suspend missing on cronjob %s/%s (found=%v err=%v)", ns, name, found, err)
	}
	return s
}

func TestSuspendMarksSuspended(t *testing.T) {
	dc := newDynamicFake(cronJobObj("batch", "report", false))

	var captured clienttesting.Action
	dc.PrependReactor("patch", "cronjobs", func(a clienttesting.Action) (bool, runtime.Object, error) {
		captured = a
		return false, nil, nil // observe only; let the tracker apply the patch
	})

	c := &Clients{Dynamic: dc}
	ref := ObjectRef{Namespace: "batch", Name: "report"}
	if err := c.Suspend(context.Background(), cronJobsResource, ref); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if !cronJobSuspended(t, dc, "batch", "report") {
		t.Error("spec.suspend = false after Suspend, want true")
	}
	if captured == nil {
		t.Fatal("no patch action recorded")
	}
	if got := captured.GetNamespace(); got != "batch" {
		t.Errorf("namespace = %q, want batch (namespaced)", got)
	}
}

func TestResumeMarksActive(t *testing.T) {
	dc := newDynamicFake(cronJobObj("batch", "report", true))

	c := &Clients{Dynamic: dc}
	if err := c.Resume(context.Background(), cronJobsResource, ObjectRef{Namespace: "batch", Name: "report"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if cronJobSuspended(t, dc, "batch", "report") {
		t.Error("spec.suspend = true after Resume, want false")
	}
}

func TestSuspendEmptyName(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	if err := c.Suspend(context.Background(), cronJobsResource, ObjectRef{Namespace: "batch"}); err == nil {
		t.Fatal("Suspend(empty name): want error, got nil")
	}
	if err := c.Resume(context.Background(), cronJobsResource, ObjectRef{Namespace: "batch"}); err == nil {
		t.Fatal("Resume(empty name): want error, got nil")
	}
}

func TestSuspendNotFoundWrapped(t *testing.T) {
	c := &Clients{Dynamic: newDynamicFake()}
	err := c.Suspend(context.Background(), cronJobsResource, ObjectRef{Namespace: "batch", Name: "ghost"})
	if err == nil {
		t.Fatal("Suspend(missing): want error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("error = %v, want a wrapped NotFound", err)
	}
}

func getDeployment(t *testing.T, dc *dynamicfake.FakeDynamicClient, ns, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := dc.Resource(deploymentsResource.GVR).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s/%s: %v", ns, name, err)
	}
	return obj
}
