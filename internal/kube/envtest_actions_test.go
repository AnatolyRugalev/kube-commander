package kube

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

// M1-INT-c-1: "Delete", against a live apiserver.
//
// The hermetic suite (actions_test.go, M1-06/D35) drives Delete through the fake
// dynamic client, which proves the addressing — namespaced vs cluster-scoped, the
// GVR the discovered Resource carries — and nothing else, because **the fake
// discards DeleteOptions entirely**. Everything kubecom puts in those options is
// therefore unit-tested only as a pure struct builder (TestWithUIDPrecondition):
// the struct is right, and until this test nothing had ever checked that a server
// reads it.
//
// That matters most for the UID precondition, which exists for a race a fake
// cannot stage. A table row is a snapshot; between the list that drew it and the
// keypress that acts on it, the object can be deleted and a *different* object
// created under the same name. Delete by name alone then removes the new object —
// the user deletes something they never saw. The precondition is what turns that
// into a refusal, and the refusal only exists if the apiserver enforces it.
//
// So the load-bearing subtest stages exactly that race and asserts a Conflict, and
// the one after it is its negative control: the same race with the UID dropped
// (the degraded-metadata path, principle 3) destroys the recreated object. If the
// precondition were silently ignored, both would end the same way and only the
// control would look correct.
//
// One control plane for the whole function, unlike the isolation and watch tests:
// those wreck cluster-global state (an APIService, cluster RBAC) or need a plane
// configured to expire watches, while deleting objects leaves nothing behind that
// another subtest can observe. Each subtest uses its own object name.
//
// Opt-in behind requireEnvtest (D18); not part of `make check`.

// TestEnvtestDeleteAgainstLiveAPIServer exercises Clients.Delete against a real
// kube-apiserver, with the Resource taken from a real discovery pass — the same
// value the menu hands the action at runtime, rather than one hand-built here.
func TestEnvtestDeleteAgainstLiveAPIServer(t *testing.T) {
	_, cfg := startControlPlane(t)
	c := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	discovered := discoverFresh(ctx, c)
	configMaps := requireDiscoveredResource(t, discovered.Resources, "", "configmaps")
	nodes := requireDiscoveredResource(t, discovered.Resources, "", "nodes")

	t.Run("removes the object it addresses", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "plain")

		if err := c.Delete(ctx, configMaps, ref, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete configmap: %v", err)
		}
		if _, err := getConfigMap(ctx, c, ref.Name); !apierrors.IsNotFound(err) {
			t.Fatalf("after delete, get returned %v, want NotFound", err)
		}
	})

	// The point of the whole slice: a real server enforces the UID precondition,
	// so a stale row cannot delete the object that replaced the one it names.
	t.Run("a stale row is refused, not honored", func(t *testing.T) {
		stale := createConfigMap(ctx, t, c, "race")
		deleteConfigMap(ctx, t, c, stale.Name)
		fresh := createConfigMap(ctx, t, c, "race")
		if fresh.UID == stale.UID {
			t.Fatalf("recreated configmap kept UID %q; the race this test stages did not happen", fresh.UID)
		}

		err := c.Delete(ctx, configMaps, stale, metav1.DeleteOptions{})
		if err == nil {
			t.Fatal("delete from a stale row succeeded; the UID precondition never reached the server")
		}
		if got := Classify(err); got != KindConflict {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindConflict)
		}

		// The refusal is only worth anything if the object it protected survived.
		live, err := getConfigMap(ctx, c, fresh.Name)
		if err != nil {
			t.Fatalf("get recreated configmap: %v", err)
		}
		if string(live.UID) != fresh.UID {
			t.Errorf("surviving configmap UID = %q, want %q", live.UID, fresh.UID)
		}
	})

	// The negative control for the subtest above, and the degraded-metadata path
	// in its own right (principle 3): a row with no UID still deletes by name —
	// which in this staged race means it takes the wrong object with it. That is
	// the behavior the precondition exists to prevent, and seeing it here is what
	// proves the Conflict above came from the precondition and not from the setup.
	t.Run("a row with no UID deletes whatever holds the name", func(t *testing.T) {
		gone := createConfigMap(ctx, t, c, "control")
		deleteConfigMap(ctx, t, c, gone.Name)
		fresh := createConfigMap(ctx, t, c, "control")

		unidentified := ObjectRef{Namespace: gone.Namespace, Name: gone.Name}
		if err := c.Delete(ctx, configMaps, unidentified, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete without a UID: %v", err)
		}
		if _, err := getConfigMap(ctx, c, fresh.Name); !apierrors.IsNotFound(err) {
			t.Fatalf("after an unguarded delete, get returned %v, want NotFound", err)
		}
	})

	// A second, independent proof that DeleteOptions travels the wire: the
	// propagation policy is a field the fake also drops, and foreground deletion
	// is observable as state the server writes onto the object.
	t.Run("the propagation policy travels with the request", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "foreground")

		foreground := metav1.DeletePropagationForeground
		if err := c.Delete(ctx, configMaps, ref, metav1.DeleteOptions{PropagationPolicy: &foreground}); err != nil {
			t.Fatalf("foreground delete: %v", err)
		}

		// Foreground deletion is a two-phase delete: the server stamps a deletion
		// timestamp and the foregroundDeletion finalizer, and the object only goes
		// once the garbage collector clears its dependents. envtest runs no
		// controller-manager, so nothing ever clears it — the object stays,
		// terminating, for the life of the plane. That is what makes this
		// assertion possible at all, and it is a trap for any later slice that
		// deletes foreground and then waits for the object to disappear.
		cm, err := getConfigMap(ctx, c, ref.Name)
		if err != nil {
			t.Fatalf("get after foreground delete: %v", err)
		}
		if cm.DeletionTimestamp == nil {
			t.Error("foreground delete left no deletionTimestamp; the policy did not reach the server")
		}
		if !hasFinalizer(cm, metav1.FinalizerDeleteDependents) {
			t.Errorf("finalizers = %v, want %q", cm.Finalizers, metav1.FinalizerDeleteDependents)
		}
	})

	// Cluster-scoped addressing, which only a real server can get wrong: a
	// namespaced request for a cluster-scoped resource is a different URL, and the
	// apiserver answers it with a 404 rather than ignoring the namespace.
	t.Run("a cluster-scoped delete ignores the ref namespace", func(t *testing.T) {
		node, err := c.Clientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create node: %v", err)
		}

		// A namespace the row could plausibly carry — the app always has one
		// selected, and Delete must drop it for a cluster-scoped resource.
		ref := ObjectRef{Namespace: "kube-system", Name: node.Name, UID: string(node.UID)}
		if err := c.Delete(ctx, nodes, ref, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete node: %v", err)
		}
		if _, err := c.Clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("after delete, get node returned %v, want NotFound", err)
		}
	})
}

// M1-INT-c-2: "Scale", against a live apiserver.
//
// Scale is the one action that does not address the object itself: it
// merge-patches the object's **scale subresource**, which a real server routes to
// a different endpoint (`…/deployments/web/scale`) with a different schema (an
// `autoscaling/v1` Scale, not the workload). The fake dynamic client has no notion
// of a subresource at all — it applies whatever patch it is handed to the whole
// tracked object — so the hermetic suite (actions_test.go) can only ever prove
// that `{"spec":{"replicas":N}}` was sent. Whether that reaches a scale endpoint,
// whether the endpoint exists for the kind, and whether the count is written back
// into the workload are all questions only a server answers.
//
// The gap is not theoretical in either direction. Under the fake, scaling a
// DaemonSet *succeeds* and grows a `spec.replicas` field the kind does not have;
// a real apiserver 404s, because DaemonSet registers no scale subresource. That
// subtest is the slice's load-bearing one: it is the case where the fake's answer
// and the server's answer disagree about whether the operation is legal.
//
// Two kinds, one contract: Deployment and StatefulSet keep replicas in different
// schemas, and the point of patching `scale` rather than the body is that neither
// needs per-kind wiring. envtest runs no controller-manager, so no pods are ever
// created and `status` stays empty — everything here asserts desired state, which
// is what the action sets.
//
// One control plane for the whole function, per the c-1 rule: scaling leaves
// nothing behind another subtest can observe, and each subtest uses its own name.

// TestEnvtestScaleAgainstLiveAPIServer exercises Clients.Scale against a real
// kube-apiserver, with the Resource taken from a real discovery pass.
func TestEnvtestScaleAgainstLiveAPIServer(t *testing.T) {
	_, cfg := startControlPlane(t)
	c := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	discovered := discoverFresh(ctx, c)
	deployments := requireDiscoveredResource(t, discovered.Resources, "apps", "deployments")
	statefulSets := requireDiscoveredResource(t, discovered.Resources, "apps", "statefulsets")
	daemonSets := requireDiscoveredResource(t, discovered.Resources, "apps", "daemonsets")

	t.Run("a deployment's replica count follows the scale subresource", func(t *testing.T) {
		ref := createDeployment(ctx, t, c, "web", 1)

		if err := c.Scale(ctx, deployments, ref, 3); err != nil {
			t.Fatalf("scale up: %v", err)
		}
		requireReplicas(ctx, t, c, ref.Name, 3)

		// Zero is a legal desired count, not a delete or a no-op — the "stop this
		// workload without losing it" case the TUI offers, and the one an
		// omitempty bug on the wire would silently turn into "leave it alone".
		if err := c.Scale(ctx, deployments, ref, 0); err != nil {
			t.Fatalf("scale to zero: %v", err)
		}
		requireReplicas(ctx, t, c, ref.Name, 0)

		// The patch merges into the scale subresource; it must not arrive as a
		// replace of anything larger. A sibling spec field set at creation is the
		// witness: it is not in the Scale schema, so a whole-object write would
		// drop it back to zero.
		d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if d.Spec.MinReadySeconds != scaleSiblingField {
			t.Errorf("minReadySeconds = %d after scaling, want %d (the scale patch overwrote a sibling field)",
				d.Spec.MinReadySeconds, scaleSiblingField)
		}
	})

	// The second kind, and the reason Scale patches `scale` instead of the body:
	// one code path, no per-kind knowledge, whatever the workload's own schema is.
	t.Run("a statefulset scales through the same one path", func(t *testing.T) {
		ref := createStatefulSet(ctx, t, c, "db", 1)

		if err := c.Scale(ctx, statefulSets, ref, 2); err != nil {
			t.Fatalf("scale statefulset: %v", err)
		}

		ss, err := c.Clientset.AppsV1().StatefulSets("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get statefulset: %v", err)
		}
		if ss.Spec.Replicas == nil {
			t.Error("statefulset spec.replicas is unset, want 2")
		} else if *ss.Spec.Replicas != 2 {
			t.Errorf("statefulset spec.replicas = %d, want 2", *ss.Spec.Replicas)
		}
	})

	// The one that only a live server can answer, and the reason this slice
	// exists: a DaemonSet has no scale subresource. The fake would apply the
	// patch to the object body and report success, inventing a spec.replicas
	// field on a kind that has none; a real apiserver has no route to answer, so
	// it 404s and the object is untouched. Both halves are asserted, because a
	// refusal that still mutated the object would be worse than either.
	t.Run("a kind with no scale subresource is refused", func(t *testing.T) {
		ref := createDaemonSet(ctx, t, c, "agent")

		err := c.Scale(ctx, daemonSets, ref, 3)
		if err == nil {
			t.Fatal("scaling a daemonset succeeded; the patch did not go to a scale subresource")
		}
		if got := Classify(err); got != KindNotFound {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindNotFound)
		}

		ds, err := c.Dynamic.Resource(daemonSets.GVR).Namespace("default").
			Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get daemonset: %v", err)
		}
		if _, found, _ := unstructured.NestedInt64(ds.Object, "spec", "replicas"); found {
			t.Error("the refused patch still added spec.replicas to the daemonset")
		}
	})

	// A stale row: the object is gone by the time the keypress lands. Scale must
	// surface that as a NotFound for the caller to display (#86), not swallow it.
	t.Run("a stale row surfaces NotFound", func(t *testing.T) {
		ref := ObjectRef{Namespace: "default", Name: "never-existed"}

		err := c.Scale(ctx, deployments, ref, 1)
		if err == nil {
			t.Fatal("scaling a missing deployment succeeded")
		}
		if got := Classify(err); got != KindNotFound {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindNotFound)
		}
	})

	// The documented asymmetry with Delete, against a server rather than against
	// its doc comment: Scale carries no UID precondition, because scaling is
	// idempotent — re-issuing it converges instead of destroying a wrongly-matched
	// object. So the same stale-row race that c-1 proves Delete refuses is one
	// Scale is expected to honor, on whatever now holds the name.
	t.Run("a stale UID does not stop a scale", func(t *testing.T) {
		stale := createDeployment(ctx, t, c, "recreated", 1)
		if err := c.Clientset.AppsV1().Deployments("default").
			Delete(ctx, stale.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete deployment: %v", err)
		}
		fresh := createDeployment(ctx, t, c, "recreated", 1)
		if fresh.UID == stale.UID {
			t.Fatalf("recreated deployment kept UID %q; the race this subtest stages did not happen", fresh.UID)
		}

		if err := c.Scale(ctx, deployments, stale, 4); err != nil {
			t.Fatalf("scale from a stale ref: %v", err)
		}
		requireReplicas(ctx, t, c, fresh.Name, 4)
	})
}

// M1-INT-c-3: the merge-patch actions, against a live apiserver.
//
// RolloutRestart, Cordon, Uncordon, Suspend and Resume are one wire format — an
// RFC 7386 merge patch of a two-key body — sent at three different schemas. The
// hermetic suite (actions_test.go) drives all five through the fake dynamic
// client, which merges whatever JSON it is handed into the tracked object and
// **validates nothing**: no schema, no field types, no notion of whether the kind
// it is patching has the field at all. So everything the fake can prove is that
// the bytes were built correctly, which the pure patch-builder tests already prove
// more directly.
//
// What a real server adds is what it *does* with a patch the fake would simply
// store, and the two answers are not symmetric — that asymmetry is the slice:
//
//   - A **known field with the wrong type** is refused (422 Invalid). The server
//     validates the fields it knows, so a builder that quoted its bool fails loudly
//     rather than writing a string into spec.unschedulable.
//   - An **unknown field** is *not* refused. It is dropped with a warning and a
//     200, so a merge-patch action aimed at a kind that does not have the field
//     does nothing, silently and successfully. The kind gating in the row-action
//     registry (D107) is the only thing standing between the user and that no-op —
//     see D188.
//
// The first is the premise of the second: without it, "the server dropped it" and
// "the server validates nothing" are the same observation, and D188 would rest on
// a server that simply never checks anything.
//
// Two more things only a server shows: the pod-template annotation patch adds
// restartedAt without disturbing its siblings *and* bumps `metadata.generation`
// (the change a controller observes — the restart itself), and "set the field to
// an explicit false" lands differently per schema, because `NodeSpec.Unschedulable`
// is a bool with `omitempty` and `CronJobSpec.Suspend` is a `*bool`.
//
// One control plane for the whole function, per the c-1 rule; each subtest uses
// its own object name and the cordon subtests use their own node.

// TestEnvtestMergePatchActionsAgainstLiveAPIServer exercises the five merge-patch
// actions against a real kube-apiserver, with each Resource taken from a real
// discovery pass — the value the menu hands the action at runtime.
func TestEnvtestMergePatchActionsAgainstLiveAPIServer(t *testing.T) {
	_, cfg := startControlPlane(t)
	c := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	discovered := discoverFresh(ctx, c)
	deployments := requireDiscoveredResource(t, discovered.Resources, "apps", "deployments")
	statefulSets := requireDiscoveredResource(t, discovered.Resources, "apps", "statefulsets")
	daemonSets := requireDiscoveredResource(t, discovered.Resources, "apps", "daemonsets")
	nodes := requireDiscoveredResource(t, discovered.Resources, "", "nodes")
	cronJobs := requireDiscoveredResource(t, discovered.Resources, "batch", "cronjobs")

	// The restart is not the annotation; it is the generation bump the annotation
	// causes. And the patch has to *add* to the annotation map rather than replace
	// it, or a restart would silently drop whatever else was annotated on the pod
	// template — which on a real deployment is where sidecar injectors, checksum
	// annotations and config-hash triggers live.
	t.Run("a rollout restart stamps restartedAt beside the annotations already there", func(t *testing.T) {
		ref := createDeployment(ctx, t, c, "restart", 1)
		before := setTemplateAnnotation(ctx, t, c, ref.Name, siblingAnnotation, "ops")

		if err := c.RolloutRestart(ctx, deployments, ref); err != nil {
			t.Fatalf("rollout restart: %v", err)
		}

		d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		stamp, ok := d.Spec.Template.Annotations[restartedAtAnnotation]
		if !ok {
			t.Fatalf("pod template annotations = %v, want %q", d.Spec.Template.Annotations, restartedAtAnnotation)
		}
		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			t.Errorf("restartedAt = %q, not RFC 3339: %v", stamp, err)
		}
		if got := d.Spec.Template.Annotations[siblingAnnotation]; got != "ops" {
			t.Errorf("sibling annotation %s = %q, want %q — the patch replaced the annotation map",
				siblingAnnotation, got, "ops")
		}
		// What actually restarts the pods: the template changed, so the
		// controller sees a new generation to roll out to.
		if d.Generation <= before {
			t.Errorf("generation = %d after restart, was %d — the pod template did not change, so nothing rolls",
				d.Generation, before)
		}
	})

	// The same patch at two more schemas, which is the reason it is a merge patch
	// on an unstructured object and not a typed strategic-merge: the pod template
	// sits at the same path in every workload kind, CRDs included, so one code
	// path covers all of them with no per-kind wiring.
	t.Run("the same patch restarts a statefulset and a daemonset", func(t *testing.T) {
		for _, w := range []struct {
			name string
			r    Resource
			ref  ObjectRef
		}{
			{"statefulset", statefulSets, createStatefulSet(ctx, t, c, "restart-ss", 1)},
			{"daemonset", daemonSets, createDaemonSet(ctx, t, c, "restart-ds")},
		} {
			if err := c.RolloutRestart(ctx, w.r, w.ref); err != nil {
				t.Fatalf("rollout restart %s: %v", w.name, err)
			}
			u, err := c.Dynamic.Resource(w.r.GVR).Namespace("default").Get(ctx, w.ref.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("get %s: %v", w.name, err)
			}
			stamp, found, err := unstructured.NestedString(u.Object,
				"spec", "template", "metadata", "annotations", restartedAtAnnotation)
			if err != nil || !found {
				t.Errorf("%s restartedAt: found=%v err=%v, want the annotation set", w.name, found, err)
			} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				t.Errorf("%s restartedAt = %q, not RFC 3339", w.name, stamp)
			}
		}
	})

	// Cordon/uncordon on a real Node, and the place where "set it to an explicit
	// false" meets a typed schema: NodeSpec.Unschedulable is a plain bool with
	// omitempty, so the server round-trips false by *dropping the key*. The
	// action's doc comment used to claim the field stays present; it does not, and
	// it does not matter — absent and false read identically to the scheduler.
	// The fake keeps the key, so only a server can show this.
	t.Run("cordon and uncordon toggle a real node", func(t *testing.T) {
		ref := createNode(ctx, t, c, "cordoned")

		if err := c.Cordon(ctx, nodes, ref); err != nil {
			t.Fatalf("cordon: %v", err)
		}
		unschedulable, found, err := liveNodeUnschedulable(ctx, t, c, nodes, ref.Name)
		if err != nil {
			t.Fatalf("spec.unschedulable is not a bool after cordon: %v", err)
		}
		if !found || !unschedulable {
			t.Errorf("after cordon: unschedulable=%v found=%v, want true", unschedulable, found)
		}

		if err := c.Uncordon(ctx, nodes, ref); err != nil {
			t.Fatalf("uncordon: %v", err)
		}
		unschedulable, _, err = liveNodeUnschedulable(ctx, t, c, nodes, ref.Name)
		if err != nil {
			t.Fatalf("spec.unschedulable is not a bool after uncordon: %v", err)
		}
		if unschedulable {
			t.Error("after uncordon: unschedulable=true, want false")
		}
		// Read it typed as well: this is the assertion that survives whichever
		// way the server chose to serialize the false.
		n, err := c.Clientset.CoreV1().Nodes().Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get node: %v", err)
		}
		if n.Spec.Unschedulable {
			t.Error("typed node is still unschedulable after uncordon")
		}
	})

	// The same explicit-false at a schema that keeps it: CronJobSpec.Suspend is a
	// *bool, so a resumed CronJob carries `suspend: false` rather than nothing.
	// Both spellings mean "not suspended" — what matters is that neither action
	// needs to know which one it will get, because both are merge patches of the
	// same shape.
	t.Run("suspend and resume toggle a real cronjob", func(t *testing.T) {
		ref := createCronJob(ctx, t, c, "nightly")

		if err := c.Suspend(ctx, cronJobs, ref); err != nil {
			t.Fatalf("suspend: %v", err)
		}
		if suspended := liveCronJobSuspended(ctx, t, c, ref.Name); suspended == nil || !*suspended {
			t.Errorf("after suspend: spec.suspend = %v, want true", suspended)
		}

		if err := c.Resume(ctx, cronJobs, ref); err != nil {
			t.Fatalf("resume: %v", err)
		}
		if suspended := liveCronJobSuspended(ctx, t, c, ref.Name); suspended == nil || *suspended {
			t.Errorf("after resume: spec.suspend = %v, want false", suspended)
		}
	})

	// The premise of the subtest after this one: a field the schema *knows* is
	// type-checked, and a wrong type is refused with a 422 the caller can display
	// (#86). The hermetic suite already catches a builder that quotes its bool (it
	// asserts the exact bytes), so this is not about catching that bug — it is
	// about establishing that the server validates at all, which is what makes the
	// silent no-op below a fact about *unknown fields* rather than about a server
	// that checks nothing.
	t.Run("a known field with the wrong type is refused", func(t *testing.T) {
		ref := createNode(ctx, t, c, "typechecked")

		_, err := c.resourceInterface(nodes, "").Patch(ctx, ref.Name, types.MergePatchType,
			[]byte(`{"spec":{"unschedulable":"true"}}`), metav1.PatchOptions{})
		if err == nil {
			t.Fatal("a string in spec.unschedulable was accepted; the field is not type-checked after all")
		}
		if got := Classify(err); got != KindInvalid {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindInvalid)
		}

		// And the value the action actually sends is accepted, at the same field
		// through the same path — so the refusal above is about the type and not
		// about the request.
		if err := c.Cordon(ctx, nodes, ref); err != nil {
			t.Fatalf("cordon after the refused patch: %v", err)
		}
	})

	// The other half of the asymmetry, and the reason D188 exists: an *unknown*
	// field is not refused, it is dropped — warning, 200, nothing changed. So a
	// merge-patch action pointed at the wrong kind reports success and does
	// nothing, which is indistinguishable from success at the UI. The fake cannot
	// show this either way: it has no schema, so it adds spec.suspend to the
	// Deployment and lets the object claim to be suspended.
	t.Run("the wrong kind is a silent no-op, not an error", func(t *testing.T) {
		ref := createDeployment(ctx, t, c, "not-a-cronjob", 1)
		before, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}

		if err := c.Suspend(ctx, deployments, ref); err != nil {
			t.Fatalf("suspending a deployment returned %v; if the server started refusing "+
				"unknown fields, D188's premise is gone and the UI gating is no longer the only guard", err)
		}

		after, err := c.Dynamic.Resource(deployments.GVR).Namespace("default").
			Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if _, found, _ := unstructured.NestedBool(after.Object, "spec", "suspend"); found {
			t.Error("the dropped field landed on the deployment anyway")
		}
		if got := after.GetResourceVersion(); got != before.ResourceVersion {
			t.Errorf("resourceVersion = %s, was %s — the no-op patch changed the object", got, before.ResourceVersion)
		}
	})

	// The #86 contract for this action group: a row can be stale by the time the
	// keypress lands, and the action must surface that rather than swallow it.
	t.Run("a stale row surfaces NotFound", func(t *testing.T) {
		err := c.Cordon(ctx, nodes, ObjectRef{Name: "never-existed"})
		if err == nil {
			t.Fatal("cordoning a missing node succeeded")
		}
		if got := Classify(err); got != KindNotFound {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindNotFound)
		}
	})
}

// M1-INT-c-4: `Update` — the Edit write-back — against a live apiserver.
//
// Update is the only action that sends a whole object rather than a patch, and
// the reason it is safe is a field it never looks at: the edited buffer still
// carries `metadata.resourceVersion` (GetYAML strips only managedFields), so the
// PUT is conditional and a concurrent change is refused instead of clobbered.
// That is the whole of D129's promise to the Edit flow, and **the fake dynamic
// client enforces no optimistic concurrency at all** — its tracker replaces the
// object whatever resourceVersion it is handed, so the hermetic suite
// (apply_test.go) can prove the parse, the identity guard and the addressing, and
// is structurally unable to prove the guarantee the flow rests on.
//
// So the load-bearing subtest stages the race the guarantee exists for — buffer
// fetched, another actor writes, buffer applied — and asserts a Conflict *and*
// that the other actor's write survived. The subtest after it is its negative
// control and the more alarming half: the same race with `resourceVersion`
// removed from the buffer **succeeds**, and the concurrent write is gone. An
// unconditional PUT is a legal request, not an error, which is what makes the
// field's presence in the buffer load-bearing rather than incidental (D189).
//
// Two more things only a server shows, both about a PUT of an object the user
// hand-edited:
//
//   - The object is **validated as a whole**. Editing a Deployment's immutable
//     `spec.selector` is a 422 (`KindInvalid`); the fake stores it.
//   - `status` is a **subresource**, so the status the user edited in the buffer
//     is silently discarded while the spec in the same PUT lands. The fake has no
//     subresources and would store both.
//
// One control plane for the whole function, per the c-1 rule; each subtest uses
// its own object name.

// TestEnvtestUpdateAgainstLiveAPIServer exercises Clients.Update against a real
// kube-apiserver, with each Resource taken from a real discovery pass — the value
// the Edit flow hands the action at runtime.
func TestEnvtestUpdateAgainstLiveAPIServer(t *testing.T) {
	_, cfg := startControlPlane(t)
	c := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	discovered := discoverFresh(ctx, c)
	configMaps := requireDiscoveredResource(t, discovered.Resources, "", "configmaps")
	deployments := requireDiscoveredResource(t, discovered.Resources, "apps", "deployments")

	t.Run("an edit round-trips through the server", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "roundtrip")

		edited := editBuffer(ctx, t, c, configMaps, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "hello", "data", "greeting")
		})
		if err := c.Update(ctx, configMaps, ref, edited); err != nil {
			t.Fatalf("update configmap: %v", err)
		}

		cm, err := getConfigMap(ctx, c, ref.Name)
		if err != nil {
			t.Fatalf("get configmap after update: %v", err)
		}
		if cm.Data["greeting"] != "hello" {
			t.Errorf("data = %v, want greeting=hello", cm.Data)
		}
	})

	// The point of the whole slice: the buffer's resourceVersion makes the PUT
	// conditional, so an object that moved under the editor is refused rather than
	// overwritten with a view of the world that is now old.
	t.Run("a stale resourceVersion is refused, not clobbered", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "race")

		// What the user has open in $EDITOR, fetched before the concurrent write.
		edited := editBuffer(ctx, t, c, configMaps, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "mine", "data", "editor")
		})

		// The other actor lands first, through the typed client — so the setup never
		// depends on the code under test.
		setConfigMapKey(ctx, t, c, ref.Name, "controller", "theirs")

		err := c.Update(ctx, configMaps, ref, edited)
		if err == nil {
			t.Fatal("update from a stale buffer succeeded; the resourceVersion never reached the server")
		}
		if got := Classify(err); got != KindConflict {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindConflict)
		}

		// The refusal is only worth anything if the write it protected survived.
		cm, err := getConfigMap(ctx, c, ref.Name)
		if err != nil {
			t.Fatalf("get configmap after the refused update: %v", err)
		}
		if cm.Data["controller"] != "theirs" {
			t.Errorf("data = %v, want the concurrent write (controller=theirs) intact", cm.Data)
		}
		if _, ok := cm.Data["editor"]; ok {
			t.Errorf("data = %v, want the refused edit not to have landed", cm.Data)
		}
	})

	// The negative control for the subtest above, and the reason the buffer's
	// resourceVersion is a correctness requirement rather than an artifact of how
	// GetYAML renders (D189): an unconditional PUT is a perfectly legal request,
	// so a buffer without the field silently wins the race it should have lost.
	t.Run("a buffer with no resourceVersion overwrites unconditionally", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "control")

		edited := editBuffer(ctx, t, c, configMaps, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "mine", "data", "editor")
			unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
		})
		setConfigMapKey(ctx, t, c, ref.Name, "controller", "theirs")

		if err := c.Update(ctx, configMaps, ref, edited); err != nil {
			t.Fatalf("update without a resourceVersion: %v", err)
		}

		cm, err := getConfigMap(ctx, c, ref.Name)
		if err != nil {
			t.Fatalf("get configmap after the unconditional update: %v", err)
		}
		if cm.Data["editor"] != "mine" {
			t.Errorf("data = %v, want the unconditional edit to have landed", cm.Data)
		}
		if _, ok := cm.Data["controller"]; ok {
			t.Errorf("data = %v, want the concurrent write clobbered — that is what the missing resourceVersion means", cm.Data)
		}
	})

	// A PUT is validated as a whole object, unlike a merge patch of two keys: the
	// user can edit anything in the buffer, including fields the API refuses to
	// change after creation. That refusal has to reach them (#86), not be swallowed.
	t.Run("an edit of an immutable field is refused", func(t *testing.T) {
		ref := createDeployment(ctx, t, c, "immutable", 1)

		// Selector *and* template labels, so the object is internally consistent and
		// the only thing wrong with it is that spec.selector may not change.
		edited := editBuffer(ctx, t, c, deployments, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "other", "spec", "selector", "matchLabels", "app")
			setNested(t, obj, "other", "spec", "template", "metadata", "labels", "app")
		})

		err := c.Update(ctx, deployments, ref, edited)
		if err == nil {
			t.Fatal("update of an immutable selector succeeded")
		}
		if got := Classify(err); got != KindInvalid {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindInvalid)
		}

		d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment after the refused update: %v", err)
		}
		if got := d.Spec.Selector.MatchLabels["app"]; got != ref.Name {
			t.Errorf("spec.selector.matchLabels[app] = %q, want %q — the refused edit landed anyway", got, ref.Name)
		}
	})

	// status is a subresource on Deployment, so the half of the buffer the user
	// cannot usefully edit is dropped while the rest of the same PUT applies. The
	// action reports success either way, which is correct and worth pinning: an
	// edit that only touched status is a no-op the UI must not present otherwise.
	t.Run("an edited status is discarded and the spec in the same PUT lands", func(t *testing.T) {
		ref := createDeployment(ctx, t, c, "statusedit", 1)

		edited := editBuffer(ctx, t, c, deployments, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, int64(9), "status", "replicas")
			setNested(t, obj, int64(2), "spec", "replicas")
		})
		if err := c.Update(ctx, deployments, ref, edited); err != nil {
			t.Fatalf("update deployment: %v", err)
		}

		d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment after update: %v", err)
		}
		if d.Spec.Replicas == nil || *d.Spec.Replicas != 2 {
			t.Errorf("spec.replicas = %v, want 2 — the spec half of the PUT did not land", d.Spec.Replicas)
		}
		if d.Status.Replicas != 0 {
			t.Errorf("status.replicas = %d, want 0 — the edited status reached the object", d.Status.Replicas)
		}
	})

	// The #86 contract for this action, and the one answer a real server gives that
	// the doc comment did not predict: the buffer carries `metadata.uid`, and an
	// apiserver reads a UID on an update as a **precondition**. So an object deleted
	// while the editor was open is a Conflict — same kind as the stale-buffer race,
	// which is the right thing for the user to be told (your row is out of date) but
	// not the NotFound the code assumed.
	t.Run("a buffer for a deleted object is a Conflict, not a NotFound", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "vanished")

		edited := editBuffer(ctx, t, c, configMaps, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "mine", "data", "editor")
		})
		deleteConfigMap(ctx, t, c, ref.Name)

		err := c.Update(ctx, configMaps, ref, edited)
		if err == nil {
			t.Fatal("update of a deleted object succeeded; a PUT must not recreate it")
		}
		if got := Classify(err); got != KindConflict {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindConflict)
		}
		if _, err := getConfigMap(ctx, c, ref.Name); !apierrors.IsNotFound(err) {
			t.Fatalf("after the refused update, get returned %v, want NotFound — the PUT recreated the object", err)
		}
	})

	// The control for the subtest above: with the uid edited out of the buffer there
	// is no precondition left, and the same request is the plain NotFound. That is
	// what says the Conflict above came from the uid and not from the deletion, and
	// it is the degraded-buffer path in its own right (principle 3) — a PUT still
	// never recreates the object.
	t.Run("without the uid, the same buffer is a NotFound", func(t *testing.T) {
		ref := createConfigMap(ctx, t, c, "unidentified")

		edited := editBuffer(ctx, t, c, configMaps, ref, func(obj *unstructured.Unstructured) {
			setNested(t, obj, "mine", "data", "editor")
			unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
		})
		deleteConfigMap(ctx, t, c, ref.Name)

		err := c.Update(ctx, configMaps, ref, edited)
		if err == nil {
			t.Fatal("update of a deleted object succeeded; a PUT must not recreate it")
		}
		if got := Classify(err); got != KindNotFound {
			t.Errorf("Classify(%v) = %v, want %v", err, got, KindNotFound)
		}
	})
}

// editBuffer is the $EDITOR round trip a test needs: fetch the object exactly as
// the Edit flow does (GetYAML — so the buffer carries whatever that renders,
// resourceVersion included), apply fn the way a user's edit would, and render it
// back to YAML. The render is sigs.k8s.io/yaml directly rather than the package's
// own marshalYAML, so the buffer under test is not produced by the code the test
// is checking.
func editBuffer(ctx context.Context, t *testing.T, c *Clients, r Resource, ref ObjectRef, fn func(*unstructured.Unstructured)) []byte {
	t.Helper()
	buf, err := c.GetYAML(ctx, r, ref)
	if err != nil {
		t.Fatalf("get yaml for %s %q: %v", r.GVR.Resource, ref.Name, err)
	}
	jsonBytes, err := yaml.YAMLToJSON([]byte(buf))
	if err != nil {
		t.Fatalf("parse the fetched buffer: %v", err)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		t.Fatalf("decode the fetched buffer: %v", err)
	}
	fn(obj)
	edited, err := yaml.Marshal(obj.Object)
	if err != nil {
		t.Fatalf("render the edited buffer: %v", err)
	}
	return edited
}

// setNested writes a value into an unstructured object, failing the test if the
// path runs through something that is not a map — the edit a user makes in a
// buffer, with the error checking Go requires of it.
func setNested(t *testing.T, obj *unstructured.Unstructured, value any, fields ...string) {
	t.Helper()
	if err := unstructured.SetNestedField(obj.Object, value, fields...); err != nil {
		t.Fatalf("set %v: %v", fields, err)
	}
}

// setConfigMapKey is the concurrent writer in the staged Edit race: it changes the
// object through the typed client while a buffer for it is open, which is what
// moves the resourceVersion out from under that buffer.
func setConfigMapKey(ctx context.Context, t *testing.T, c *Clients, name, key, value string) {
	t.Helper()
	cm, err := c.Clientset.CoreV1().ConfigMaps("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap %s: %v", name, err)
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[key] = value
	if _, err := c.Clientset.CoreV1().ConfigMaps("default").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("concurrent update of configmap %s: %v", name, err)
	}
}

// siblingAnnotation is a pod-template annotation set before a rollout restart, so
// a patch that replaced the annotation map instead of merging into it is visible
// as this key going missing. Real clusters keep sidecar-injection and config-hash
// annotations here.
const siblingAnnotation = "kubecom.test/owner"

// setTemplateAnnotation adds a pod-template annotation to a Deployment through the
// typed client — the *other* actor, so the setup never depends on the code under
// test — and returns the object's generation afterwards, which the restart must
// then bump.
func setTemplateAnnotation(ctx context.Context, t *testing.T, c *Clients, name, key, value string) int64 {
	t.Helper()
	d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s: %v", name, err)
	}
	if d.Spec.Template.Annotations == nil {
		d.Spec.Template.Annotations = map[string]string{}
	}
	d.Spec.Template.Annotations[key] = value
	updated, err := c.Clientset.AppsV1().Deployments("default").Update(ctx, d, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("annotate deployment %s: %v", name, err)
	}
	return updated.Generation
}

// createNode writes a cluster-scoped Node and returns the ObjectRef a table row
// would carry. No kubelet ever registers on an envtest plane, so the node stays
// NotReady — which is irrelevant here: spec.unschedulable is desired state and is
// all Cordon sets.
func createNode(ctx context.Context, t *testing.T, c *Clients, name string) ObjectRef {
	t.Helper()
	n, err := c.Clientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	return ObjectRef{Name: n.Name, UID: string(n.UID)}
}

// liveNodeUnschedulable reads spec.unschedulable off the *unstructured* node — the
// same view the table renders from — and reports whether the key is present at
// all, because the server drops it when it is false (omitempty). A non-nil error
// means the value is not a bool, which is the failure a quoted patch would cause.
func liveNodeUnschedulable(ctx context.Context, t *testing.T, c *Clients, r Resource, name string) (bool, bool, error) {
	t.Helper()
	n, err := c.Dynamic.Resource(r.GVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get node %s: %v", name, err)
	}
	return unstructured.NestedBool(n.Object, "spec", "unschedulable")
}

// createCronJob writes the minimum CronJob the apiserver accepts and returns its
// ObjectRef. Nothing schedules on an envtest plane, so no Job is ever created —
// spec.suspend is desired state, and all Suspend/Resume set.
func createCronJob(ctx context.Context, t *testing.T, c *Clients, name string) ObjectRef {
	t.Helper()
	cj, err := c.Clientset.BatchV1().CronJobs("default").Create(ctx, &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyOnFailure,
						Containers:    []corev1.Container{{Name: "app", Image: "busybox"}},
					},
				}},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cronjob %s: %v", name, err)
	}
	return ObjectRef{Namespace: cj.Namespace, Name: cj.Name, UID: string(cj.UID)}
}

// liveCronJobSuspended reads spec.suspend typed, where it is a *bool — so nil (unset),
// false and true are three distinguishable states, unlike the node's flag.
func liveCronJobSuspended(ctx context.Context, t *testing.T, c *Clients, name string) *bool {
	t.Helper()
	cj, err := c.Clientset.BatchV1().CronJobs("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cronjob %s: %v", name, err)
	}
	return cj.Spec.Suspend
}

// scaleSiblingField is a Deployment spec field outside the Scale schema, set at
// creation so a scale that arrived as a whole-object write is visible as this
// value reverting to zero.
const scaleSiblingField int32 = 7

// requireReplicas asserts a Deployment's desired replica count, read two ways:
// from the workload body (what the table renders) and from the scale subresource
// (the endpoint the patch addressed). Both must agree — that agreement is the
// write-through a fake cannot model, since it has only one object to answer from.
func requireReplicas(ctx context.Context, t *testing.T, c *Clients, name string, want int32) {
	t.Helper()
	d, err := c.Clientset.AppsV1().Deployments("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s: %v", name, err)
	}
	if d.Spec.Replicas == nil {
		t.Errorf("deployment %s spec.replicas is unset, want %d", name, want)
	} else if *d.Spec.Replicas != want {
		t.Errorf("deployment %s spec.replicas = %d, want %d", name, *d.Spec.Replicas, want)
	}
	scale, err := c.Clientset.AppsV1().Deployments("default").GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get scale %s: %v", name, err)
	}
	if scale.Spec.Replicas != want {
		t.Errorf("deployment %s scale.spec.replicas = %d, want %d", name, scale.Spec.Replicas, want)
	}
}

// createDeployment writes a Deployment and returns the ObjectRef a table row would
// carry for it. No controller-manager runs on an envtest plane, so the pods it
// asks for are never created — desired state is all there is, and all Scale sets.
func createDeployment(ctx context.Context, t *testing.T, c *Clients, name string, replicas int32) ObjectRef {
	t.Helper()
	d, err := c.Clientset.AppsV1().Deployments("default").Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas:        &replicas,
			MinReadySeconds: scaleSiblingField,
			Selector:        &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template:        podTemplate(name),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment %s: %v", name, err)
	}
	return ObjectRef{Namespace: d.Namespace, Name: d.Name, UID: string(d.UID)}
}

func createStatefulSet(ctx context.Context, t *testing.T, c *Clients, name string, replicas int32) ObjectRef {
	t.Helper()
	ss, err := c.Clientset.AppsV1().StatefulSets("default").Create(ctx, &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template:    podTemplate(name),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create statefulset %s: %v", name, err)
	}
	return ObjectRef{Namespace: ss.Namespace, Name: ss.Name, UID: string(ss.UID)}
}

// createDaemonSet writes the unscalable workload: a DaemonSet runs one pod per
// node by definition, so apps/v1 registers no scale subresource for it.
func createDaemonSet(ctx context.Context, t *testing.T, c *Clients, name string) ObjectRef {
	t.Helper()
	ds, err := c.Clientset.AppsV1().DaemonSets("default").Create(ctx, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: podTemplate(name),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create daemonset %s: %v", name, err)
	}
	return ObjectRef{Namespace: ds.Namespace, Name: ds.Name, UID: string(ds.UID)}
}

// podTemplate is the minimum a workload's pod template can be and still pass
// apiserver validation; nothing here ever runs.
func podTemplate(name string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "busybox"}},
		},
	}
}

// requireDiscoveredResource returns the discovered Resource for a group/resource,
// failing the test if the pass did not surface it. Taking the Resource from
// discovery rather than building one inline is deliberate: it is the value the
// menu hands an action at runtime, so a GVR or Namespaced flag that discovery gets
// wrong fails here instead of passing against a hand-written stand-in.
func requireDiscoveredResource(t *testing.T, resources []Resource, group, resource string) Resource {
	t.Helper()
	r, ok := findResource(resources, group, resource)
	if !ok {
		t.Fatalf("discovery did not surface %s/%s (%d resources discovered)", group, resource, len(resources))
	}
	return r
}

// createConfigMap writes a ConfigMap and returns the ObjectRef a table row would
// carry for it — including the server-assigned UID, which is the whole subject of
// this test.
func createConfigMap(ctx context.Context, t *testing.T, c *Clients, name string) ObjectRef {
	t.Helper()
	cm, err := c.Clientset.CoreV1().ConfigMaps("default").Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create configmap %s: %v", name, err)
	}
	return ObjectRef{Namespace: cm.Namespace, Name: cm.Name, UID: string(cm.UID)}
}

// deleteConfigMap removes a ConfigMap through the typed client — the *other*
// actor in the staged race, so the setup never depends on the code under test.
func deleteConfigMap(ctx context.Context, t *testing.T, c *Clients, name string) {
	t.Helper()
	if err := c.Clientset.CoreV1().ConfigMaps("default").Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete configmap %s: %v", name, err)
	}
}

func getConfigMap(ctx context.Context, c *Clients, name string) (*corev1.ConfigMap, error) {
	return c.Clientset.CoreV1().ConfigMaps("default").Get(ctx, name, metav1.GetOptions{})
}

func hasFinalizer(cm *corev1.ConfigMap, want string) bool {
	for _, f := range cm.Finalizers {
		if f == want {
			return true
		}
	}
	return false
}
