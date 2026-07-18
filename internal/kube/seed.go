package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// seedResource is one entry in the static seed set: a built-in GVK paired with
// its exact plural/singular resource names and scope. The mappings are the
// stable, GA GVK↔GVR relationships kubectl uses, so they are hard-coded rather
// than pluralized heuristically (e.g. Endpoints → endpoints, not "endpointses";
// NetworkPolicy → networkpolicies).
type seedResource struct {
	GVK        schema.GroupVersionKind
	Plural     string
	Singular   string
	Namespaced bool
}

// seedResources is the set of core, high-traffic GVKs kubecom can list and
// browse the instant it starts — before background discovery (M1-03) finishes —
// so first paint never blocks on a discovery round-trip (D8). It is intentionally
// curated, not exhaustive: everything else (CRDs, less-common groups) resolves
// through the deferred discovery mapper once it warms. Every mapping here is a
// long-stable GA relationship, so a static copy cannot drift from the server for
// these kinds.
var seedResources = []seedResource{
	// core/v1 (namespaced)
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "pods", "pod", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, "services", "service", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, "configmaps", "configmap", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, "secrets", "secret", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, "persistentvolumeclaims", "persistentvolumeclaim", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}, "serviceaccounts", "serviceaccount", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Endpoints"}, "endpoints", "endpoints", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Event"}, "events", "event", true},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ReplicationController"}, "replicationcontrollers", "replicationcontroller", true},

	// core/v1 (cluster)
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, "namespaces", "namespace", false},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, "nodes", "node", false},
	{schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"}, "persistentvolumes", "persistentvolume", false},

	// apps/v1 (namespaced)
	{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, "deployments", "deployment", true},
	{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, "replicasets", "replicaset", true},
	{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, "statefulsets", "statefulset", true},
	{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, "daemonsets", "daemonset", true},

	// batch/v1 (namespaced)
	{schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, "jobs", "job", true},
	{schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, "cronjobs", "cronjob", true},

	// networking.k8s.io/v1
	{schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}, "ingresses", "ingress", true},
	{schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, "networkpolicies", "networkpolicy", true},
	{schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "IngressClass"}, "ingressclasses", "ingressclass", false},

	// rbac.authorization.k8s.io/v1
	{schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"}, "roles", "role", true},
	{schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"}, "rolebindings", "rolebinding", true},
	{schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, "clusterroles", "clusterrole", false},
	{schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"}, "clusterrolebindings", "clusterrolebinding", false},

	// storage.k8s.io/v1 (cluster)
	{schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}, "storageclasses", "storageclass", false},
}

// newSeedRESTMapper builds a static RESTMapper over seedResources. It does no
// network I/O and is populated fully at construction, so GVK↔GVR resolution for
// core resources is available immediately — the "instant start" half of D8. It
// is composed ahead of the deferred discovery mapper (see NewClients) so seed
// lookups never trigger discovery, and unknown kinds fall through to it.
func newSeedRESTMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper(nil)
	for _, r := range seedResources {
		scope := meta.RESTScopeNamespace
		if !r.Namespaced {
			scope = meta.RESTScopeRoot
		}
		gv := r.GVK.GroupVersion()
		m.AddSpecific(r.GVK, gv.WithResource(r.Plural), gv.WithResource(r.Singular), scope)
	}
	return m
}
