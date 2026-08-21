package kube

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// The relation targets the child fixtures do not already cover. Relations only
// ever *looks kinds up* in this set, so a bare GVK/GVR pair is enough.
func relResource(group, version, kind, resource string, namespaced bool) Resource {
	return Resource{
		GVK:        schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
		GVR:        schema.GroupVersionResource{Group: group, Version: version, Resource: resource},
		Namespaced: namespaced,
	}
}

var (
	relReplicaSet = relResource("apps", "v1", "ReplicaSet", "replicasets", true)
	relSecret     = relResource("", "v1", "Secret", "secrets", true)
	relAccount    = relResource("", "v1", "ServiceAccount", "serviceaccounts", true)
	relClaim      = relResource("", "v1", "PersistentVolumeClaim", "persistentvolumeclaims", true)
	relVolume     = relResource("", "v1", "PersistentVolume", "persistentvolumes", false)
	relKinds      = append(append([]Resource{}, childKinds...), relReplicaSet, relSecret, relAccount, relClaim, relVolume)
)

func relDynamicFake(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		childPodResource.GVR:        "PodList",
		childDeploymentResource.GVR: "DeploymentList",
		childNodeResource.GVR:       "NodeList",
		relReplicaSet.GVR:           "ReplicaSetList",
		relClaim.GVR:                "PersistentVolumeClaimList",
		relVolume.GVR:               "PersistentVolumeList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)
}

// relPod is the fixture the headline case reads: a pod owned by a ReplicaSet,
// scheduled on a node, mounting a claim twice (the dedupe case), a config map, a
// secret, and pulling with an image-pull secret.
func relPod() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "shop-7d4",
			"namespace": "shop",
			"ownerReferences": []any{map[string]any{
				"apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "shop-7d", "uid": "rs-uid",
			}},
		},
		"spec": map[string]any{
			"nodeName":           "node-a",
			"serviceAccountName": "shop-sa",
			"volumes": []any{
				map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "shop-data"}},
				map[string]any{"name": "data-again", "persistentVolumeClaim": map[string]any{"claimName": "shop-data"}},
				map[string]any{"name": "cfg", "configMap": map[string]any{"name": "shop-config"}},
				map[string]any{"name": "tls", "secret": map[string]any{"secretName": "shop-tls"}},
				map[string]any{"name": "empty", "emptyDir": map[string]any{}},
			},
			"imagePullSecrets": []any{map[string]any{"name": "registry-cred"}},
		},
	}}
}

type relWant struct {
	role RelationRole
	dir  RelationDirection
	kind string
	ns   string
	name string
}

func checkRelations(t *testing.T, got []Relation, want []relWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d relations, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Role != w.role || g.Direction != w.dir || g.Resource.GVK.Kind != w.kind || g.Namespace != w.ns || g.Name != w.name {
			t.Errorf("relation %d = %q %v %s %s/%s, want %q %v %s %s/%s",
				i, g.Role, g.Direction, g.Resource.GVK.Kind, g.Namespace, g.Name,
				w.role, w.dir, w.kind, w.ns, w.name)
		}
	}
}

func TestRelationsPod(t *testing.T) {
	c := &Clients{Dynamic: relDynamicFake(relPod())}
	got, err := c.Relations(context.Background(), childPodResource, ObjectRef{Namespace: "shop", Name: "shop-7d4"}, relKinds)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	// Order is owners, then the child scope (a pod has none), then the spec links
	// in spec order. The duplicate claim volume collapses.
	checkRelations(t, got, []relWant{
		{RoleOwner, RelationUp, "ReplicaSet", "shop", "shop-7d"},
		{RoleNode, RelationUp, "Node", "", "node-a"},
		{RoleServiceAccount, RelationSide, "ServiceAccount", "shop", "shop-sa"},
		{RoleClaim, RelationSide, "PersistentVolumeClaim", "shop", "shop-data"},
		{RoleConfigMap, RelationSide, "ConfigMap", "shop", "shop-config"},
		{RoleSecret, RelationSide, "Secret", "shop", "shop-tls"},
		{RolePullSecret, RelationSide, "Secret", "shop", "registry-cred"},
	})
	if got[0].UID != "rs-uid" {
		t.Errorf("owner UID = %q, want rs-uid", got[0].UID)
	}
	for _, r := range got {
		if r.IsSet() {
			t.Errorf("relation %q is set-shaped, want named", r.Role)
		}
		if r.Ref().Name != r.Name {
			t.Errorf("Ref() dropped the name for %q", r.Role)
		}
	}
}

// A target kind the cluster does not offer is dropped, not synthesized — the
// popup must never list a row that fails the moment it is opened.
func TestRelationsSkipsUndiscoveredKinds(t *testing.T) {
	c := &Clients{Dynamic: relDynamicFake(relPod())}
	got, err := c.Relations(context.Background(), childPodResource, ObjectRef{Namespace: "shop", Name: "shop-7d4"}, []Resource{childPodResource, childNodeResource})
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	checkRelations(t, got, []relWant{{RoleNode, RelationUp, "Node", "", "node-a"}})
}

// The forward direction is the ChildScope, resolved from the object Relations
// already fetched — set-shaped, carrying the owner's selector.
func TestRelationsWorkloadChildScope(t *testing.T) {
	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "shop", "namespace": "shop"},
		"spec": map[string]any{"selector": map[string]any{
			"matchLabels": map[string]any{"app": "shop"},
		}},
	}}
	c := &Clients{Dynamic: relDynamicFake(dep)}
	got, err := c.Relations(context.Background(), childDeploymentResource, ObjectRef{Namespace: "shop", Name: "shop"}, relKinds)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	checkRelations(t, got, []relWant{{RoleChildren, RelationDown, "Pod", "shop", ""}})
	if !got[0].IsSet() {
		t.Fatal("workload child relation is not set-shaped")
	}
	if sel := got[0].Scope().Selector(); sel != "app=shop" {
		t.Errorf("child scope selector = %q, want app=shop", sel)
	}
}

// A Node's pods are cluster-wide and field-selected; the Node path needs no
// object, so it survives the shared childScope refactor.
func TestRelationsNodeChildScope(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Node",
		"metadata": map[string]any{"name": "node-a"},
	}}
	c := &Clients{Dynamic: relDynamicFake(node)}
	got, err := c.Relations(context.Background(), childNodeResource, ObjectRef{Name: "node-a"}, relKinds)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	checkRelations(t, got, []relWant{{RoleChildren, RelationDown, "Pod", "", ""}})
	if sel := got[0].Scope().Selector(); sel != "spec.nodeName=node-a" {
		t.Errorf("node child selector = %q, want spec.nodeName=node-a", sel)
	}
}

// Claim → volume and volume → claim are the two halves of the storage hop; the
// claimRef's own namespace wins, and a cluster-scoped target is never namespaced.
func TestRelationsStorageBothWays(t *testing.T) {
	pvc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]any{"name": "shop-data", "namespace": "shop"},
		"spec":     map[string]any{"volumeName": "pv-0007"},
	}}
	pv := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": map[string]any{"name": "pv-0007"},
		"spec": map[string]any{"claimRef": map[string]any{
			"namespace": "shop", "name": "shop-data", "uid": "pvc-uid",
		}},
	}}
	c := &Clients{Dynamic: relDynamicFake(pvc, pv)}
	got, err := c.Relations(context.Background(), relClaim, ObjectRef{Namespace: "shop", Name: "shop-data"}, relKinds)
	if err != nil {
		t.Fatalf("Relations(pvc): %v", err)
	}
	checkRelations(t, got, []relWant{{RoleVolume, RelationSide, "PersistentVolume", "", "pv-0007"}})

	got, err = c.Relations(context.Background(), relVolume, ObjectRef{Name: "pv-0007"}, relKinds)
	if err != nil {
		t.Fatalf("Relations(pv): %v", err)
	}
	checkRelations(t, got, []relWant{{RoleClaim, RelationSide, "PersistentVolumeClaim", "shop", "shop-data"}})
	if got[0].UID != "pvc-uid" {
		t.Errorf("claimRef UID = %q, want pvc-uid", got[0].UID)
	}
}

// An object with nothing to point at is an empty answer, not an error.
func TestRelationsNoNeighbours(t *testing.T) {
	cm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "shop-config", "namespace": "shop"},
	}}
	c := &Clients{Dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{childConfigMapResource.GVR: "ConfigMapList"}, cm)}
	got, err := c.Relations(context.Background(), childConfigMapResource, ObjectRef{Namespace: "shop", Name: "shop-config"}, relKinds)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d relations for a bare ConfigMap, want none: %+v", len(got), got)
	}
}

func TestRelationsErrors(t *testing.T) {
	c := &Clients{Dynamic: relDynamicFake()}
	if _, err := c.Relations(context.Background(), childPodResource, ObjectRef{Namespace: "shop"}, relKinds); err == nil ||
		!strings.Contains(err.Error(), "empty object name") {
		t.Errorf("empty name error = %v, want empty object name", err)
	}
	_, err := c.Relations(context.Background(), childPodResource, ObjectRef{Namespace: "shop", Name: "gone"}, relKinds)
	if err == nil || !strings.Contains(err.Error(), `getting pods "gone"`) {
		t.Errorf("missing object error = %v, want a wrapped Get failure", err)
	}
}
