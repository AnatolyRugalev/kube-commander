package kube

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
