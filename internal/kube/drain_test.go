package kube

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// podOpt mutates a pod under construction so tests read as a list of traits.
type podOpt func(*corev1.Pod)

func pod(ns, name string, opts ...podOpt) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func controlledBy(kind string) podOpt {
	return func(p *corev1.Pod) {
		yes := true
		p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
			APIVersion: "apps/v1", Kind: kind, Name: "owner", UID: "owner-uid", Controller: &yes,
		})
	}
}

func mirror() podOpt {
	return func(p *corev1.Pod) {
		if p.Annotations == nil {
			p.Annotations = map[string]string{}
		}
		p.Annotations[mirrorPodAnnotation] = "abc123"
	}
}

func phase(ph corev1.PodPhase) podOpt {
	return func(p *corev1.Pod) { p.Status.Phase = ph }
}

func emptyDir() podOpt {
	return func(p *corev1.Pod) {
		p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}
}

func evictNames(refs []ObjectRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Namespace+"/"+r.Name)
	}
	sort.Strings(out)
	return out
}

func TestClassifyDrainPods(t *testing.T) {
	tests := []struct {
		name        string
		pods        []*corev1.Pod
		opts        DrainOptions
		wantEvict   []string
		wantBlocked int    // number of blocker lines
		blockedHas  string // substring every blocker case should mention (optional)
	}{
		{
			name:      "controller-managed pod evicts",
			pods:      []*corev1.Pod{pod("ns", "web", controlledBy("ReplicaSet"))},
			wantEvict: []string{"ns/web"},
		},
		{
			name: "mirror and terminated pods are skipped silently",
			pods: []*corev1.Pod{
				pod("kube-system", "static", mirror()),
				pod("ns", "done", phase(corev1.PodSucceeded), controlledBy("Job")),
				pod("ns", "failed", phase(corev1.PodFailed), controlledBy("Job")),
			},
			wantEvict: nil,
		},
		{
			name:        "daemonset pod blocks without IgnoreDaemonSets",
			pods:        []*corev1.Pod{pod("ns", "agent", controlledBy("DaemonSet"))},
			opts:        DrainOptions{},
			wantBlocked: 1,
			blockedHas:  "DaemonSet",
		},
		{
			name:      "daemonset pod is skipped (not evicted) with IgnoreDaemonSets",
			pods:      []*corev1.Pod{pod("ns", "agent", controlledBy("DaemonSet"))},
			opts:      DrainOptions{IgnoreDaemonSets: true},
			wantEvict: nil,
		},
		{
			name:        "standalone pod blocks without Force",
			pods:        []*corev1.Pod{pod("ns", "loose")},
			opts:        DrainOptions{},
			wantBlocked: 1,
			blockedHas:  "unmanaged",
		},
		{
			name:      "standalone pod evicts with Force",
			pods:      []*corev1.Pod{pod("ns", "loose")},
			opts:      DrainOptions{Force: true},
			wantEvict: []string{"ns/loose"},
		},
		{
			name:        "emptyDir pod blocks without DeleteEmptyDirData",
			pods:        []*corev1.Pod{pod("ns", "cache", controlledBy("StatefulSet"), emptyDir())},
			opts:        DrainOptions{},
			wantBlocked: 1,
			blockedHas:  "emptyDir",
		},
		{
			name:      "emptyDir pod evicts with DeleteEmptyDirData",
			pods:      []*corev1.Pod{pod("ns", "cache", controlledBy("StatefulSet"), emptyDir())},
			opts:      DrainOptions{DeleteEmptyDirData: true},
			wantEvict: []string{"ns/cache"},
		},
		{
			name:      "daemonset+emptyDir is a DaemonSet skip, not an emptyDir block",
			pods:      []*corev1.Pod{pod("ns", "agent", controlledBy("DaemonSet"), emptyDir())},
			opts:      DrainOptions{IgnoreDaemonSets: true},
			wantEvict: nil,
		},
		{
			name: "blockers aggregate; evictable pods still selected",
			pods: []*corev1.Pod{
				pod("ns", "web", controlledBy("ReplicaSet")),
				pod("ns", "loose"),
				pod("ns", "agent", controlledBy("DaemonSet")),
			},
			opts:        DrainOptions{},
			wantEvict:   []string{"ns/web"}, // returned by classify; DrainCandidates would discard on any blocker
			wantBlocked: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]corev1.Pod, 0, len(tc.pods))
			for _, p := range tc.pods {
				items = append(items, *p)
			}
			evict, blocked := classifyDrainPods(items, tc.opts)

			got := evictNames(evict)
			if strings.Join(got, ",") != strings.Join(tc.wantEvict, ",") {
				t.Errorf("evict = %v, want %v", got, tc.wantEvict)
			}
			if len(blocked) != tc.wantBlocked {
				t.Errorf("blocked = %d %v, want %d", len(blocked), blocked, tc.wantBlocked)
			}
			if tc.blockedHas != "" {
				joined := strings.Join(blocked, " ")
				if !strings.Contains(joined, tc.blockedHas) {
					t.Errorf("blocked %q, want a line mentioning %q", joined, tc.blockedHas)
				}
			}
		})
	}
}

func TestDrainCandidates(t *testing.T) {
	objs := []runtime.Object{
		pod("ns", "web", controlledBy("ReplicaSet")),
		pod("kube-system", "static", mirror()),
		pod("ns", "agent", controlledBy("DaemonSet")),
	}
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(objs...)}

	// With IgnoreDaemonSets the DaemonSet pod is skipped and the mirror pod is
	// dropped, leaving only the managed web pod.
	got, err := c.DrainCandidates(context.Background(), ObjectRef{Name: "node-1"}, DrainOptions{IgnoreDaemonSets: true})
	if err != nil {
		t.Fatalf("DrainCandidates: %v", err)
	}
	if names := evictNames(got); strings.Join(names, ",") != "ns/web" {
		t.Errorf("candidates = %v, want [ns/web]", names)
	}

	// A blocker (the DaemonSet pod, without IgnoreDaemonSets) refuses the whole
	// drain: nil slice, error naming the pod.
	got, err = c.DrainCandidates(context.Background(), ObjectRef{Name: "node-1"}, DrainOptions{})
	if err == nil {
		t.Fatalf("DrainCandidates with a blocking pod: want error, got nil (candidates=%v)", got)
	}
	if got != nil {
		t.Errorf("candidates on refusal = %v, want nil", got)
	}
	if !strings.Contains(err.Error(), "agent") || !strings.Contains(err.Error(), "node-1") {
		t.Errorf("error = %q, want it to name the node and the blocking pod", err)
	}
}

func TestDrainCandidatesEmptyNode(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}
	if _, err := c.DrainCandidates(context.Background(), ObjectRef{}, DrainOptions{}); err == nil {
		t.Fatal("DrainCandidates with empty node name: want error, got nil")
	}
}

func TestDrainCandidatesListError(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})
	c := &Clients{Clientset: cs}
	_, err := c.DrainCandidates(context.Background(), ObjectRef{Name: "node-1"}, DrainOptions{})
	if err == nil || !strings.Contains(err.Error(), "listing pods") {
		t.Fatalf("list error = %v, want it wrapped with \"listing pods\"", err)
	}
}

var podsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

// fastDrainTiming shrinks the eviction-retry and deletion-poll intervals to a
// millisecond for the duration of a test so the retry/wait loops run instantly,
// restoring them afterward.
func fastDrainTiming(t *testing.T) {
	t.Helper()
	oldEvict, oldPoll := evictionRetryInterval, drainPollInterval
	evictionRetryInterval = time.Millisecond
	drainPollInterval = time.Millisecond
	t.Cleanup(func() {
		evictionRetryInterval = oldEvict
		drainPollInterval = oldPoll
	})
}

// evictReactor intercepts the eviction subresource create the typed client posts
// (Resource pods, Subresource "eviction") and runs fn against the target pod's
// ObjectRef, so a test can decide per-attempt what the Eviction API returns.
// Non-eviction pod creates fall through to the tracker.
func evictReactor(fn func(pod ObjectRef) (bool, error)) clienttesting.ReactionFunc {
	return func(a clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := a.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		ev := ca.GetObject().(*policyv1.Eviction)
		handled, err := fn(ObjectRef{Namespace: ev.Namespace, Name: ev.Name})
		return handled, nil, err
	}
}

func TestEvictPodRetriesOn429ThenSucceeds(t *testing.T) {
	fastDrainTiming(t)
	cs := k8sfake.NewSimpleClientset()
	attempts := 0
	cs.PrependReactor("create", "pods", evictReactor(func(ObjectRef) (bool, error) {
		attempts++
		if attempts < 3 {
			return true, apierrors.NewTooManyRequests("pdb: no disruptions allowed", 1)
		}
		return true, nil
	}))

	c := &Clients{Clientset: cs}
	if err := c.evictPod(context.Background(), ObjectRef{Namespace: "ns", Name: "web"}); err != nil {
		t.Fatalf("evictPod: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (two 429 retries then success)", attempts)
	}
}

func TestEvictPodNotFoundIsSuccess(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("create", "pods", evictReactor(func(ObjectRef) (bool, error) {
		return true, apierrors.NewNotFound(podsGVR.GroupResource(), "web")
	}))

	c := &Clients{Clientset: cs}
	if err := c.evictPod(context.Background(), ObjectRef{Namespace: "ns", Name: "web"}); err != nil {
		t.Errorf("evictPod on an already-gone pod = %v, want nil", err)
	}
}

func TestEvictPodOtherErrorWrapped(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("create", "pods", evictReactor(func(ObjectRef) (bool, error) {
		return true, errors.New("boom")
	}))

	c := &Clients{Clientset: cs}
	err := c.evictPod(context.Background(), ObjectRef{Namespace: "ns", Name: "web"})
	if err == nil || !strings.Contains(err.Error(), "evicting pod ns/web") {
		t.Fatalf("evictPod error = %v, want it wrapped with \"evicting pod ns/web\"", err)
	}
}

func TestEvictPodContextCancelledDuringRetry(t *testing.T) {
	fastDrainTiming(t)
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("create", "pods", evictReactor(func(ObjectRef) (bool, error) {
		return true, apierrors.NewTooManyRequests("pdb: no disruptions allowed", 1)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: the first 429 lands, then the retry wait aborts

	c := &Clients{Clientset: cs}
	err := c.evictPod(ctx, ObjectRef{Namespace: "ns", Name: "web"})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("evictPod error = %v, want a wrapped context.Canceled", err)
	}
}

func TestWaitPodDeletedReturnsWhenGone(t *testing.T) {
	cs := k8sfake.NewSimpleClientset() // empty tracker: Get → NotFound
	c := &Clients{Clientset: cs}
	if err := c.waitPodDeleted(context.Background(), ObjectRef{Namespace: "ns", Name: "web", UID: "web-uid"}); err != nil {
		t.Errorf("waitPodDeleted on a gone pod = %v, want nil", err)
	}
}

func TestWaitPodDeletedReturnsOnUIDChange(t *testing.T) {
	// Same name, different UID: the original pod is gone; a new one took its place.
	cs := k8sfake.NewSimpleClientset(pod("ns", "web"))
	c := &Clients{Clientset: cs}
	if err := c.waitPodDeleted(context.Background(), ObjectRef{Namespace: "ns", Name: "web", UID: "old-uid"}); err != nil {
		t.Errorf("waitPodDeleted with a recreated pod = %v, want nil", err)
	}
}

func TestWaitPodDeletedGetErrorWrapped(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("get", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})
	c := &Clients{Clientset: cs}
	err := c.waitPodDeleted(context.Background(), ObjectRef{Namespace: "ns", Name: "web"})
	if err == nil || !strings.Contains(err.Error(), "waiting for pod ns/web to delete") {
		t.Fatalf("waitPodDeleted error = %v, want it wrapped with \"waiting for pod ns/web to delete\"", err)
	}
}

// TestDrain exercises the full sequence: candidates → cordon → evict → wait. The
// node lives in the dynamic fake (cordon patches through the dynamic client); the
// pod lives in the typed clientset (list/evict/wait). The eviction reactor deletes
// the pod from the tracker so the deletion wait observes it disappear.
func TestDrain(t *testing.T) {
	fastDrainTiming(t)
	dc := newDynamicFake(nodeObj("node-1", false))
	cs := k8sfake.NewSimpleClientset(pod("ns", "web", controlledBy("ReplicaSet")))
	cs.PrependReactor("create", "pods", evictReactor(func(p ObjectRef) (bool, error) {
		if err := cs.Tracker().Delete(podsGVR, p.Namespace, p.Name); err != nil {
			return true, err
		}
		return true, nil
	}))

	c := &Clients{Dynamic: dc, Clientset: cs}
	node := ObjectRef{Name: "node-1"}
	if err := c.Drain(context.Background(), nodesResource, node, DrainOptions{}); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !nodeUnschedulable(t, dc, "node-1") {
		t.Error("node not cordoned after Drain")
	}
	if _, err := cs.CoreV1().Pods("ns").Get(context.Background(), "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("pod still present after Drain (get err = %v), want NotFound", err)
	}
}

// TestDrainRefusesBeforeCordon proves the ordering guarantee: a drain that would
// be refused (a DaemonSet pod without IgnoreDaemonSets) errors out *without*
// cordoning the node, so no cordon is left behind.
func TestDrainRefusesBeforeCordon(t *testing.T) {
	dc := newDynamicFake(nodeObj("node-1", false))
	cs := k8sfake.NewSimpleClientset(pod("ns", "agent", controlledBy("DaemonSet")))

	c := &Clients{Dynamic: dc, Clientset: cs}
	err := c.Drain(context.Background(), nodesResource, ObjectRef{Name: "node-1"}, DrainOptions{})
	if err == nil || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("Drain error = %v, want a refusal naming the blocking pod", err)
	}
	if nodeUnschedulable(t, dc, "node-1") {
		t.Error("node cordoned despite the drain being refused; candidates must be computed before cordon")
	}
}
