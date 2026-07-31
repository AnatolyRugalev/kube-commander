package kube

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
