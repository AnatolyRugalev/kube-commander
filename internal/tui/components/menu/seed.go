package menu

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// seedItems is the static default menu: the common core resource kinds a user
// browses first, grouped into the familiar Kubernetes-Dashboard scopes (Cluster →
// Workloads → Config → Network → Storage → Access Control) and, within each,
// ordered top-down. It is deliberately a fixed list — M2-05b reconciles it against
// live discovery (adding CRDs/extra groups, marking unavailable ones), but the
// seed lets the browse view render and be navigated immediately, before discovery
// completes (fast cold start, principle 4).
//
// The seed is authored **grouped**: all items of a section are contiguous and the
// sections appear in sectionOrder. The menu relies on that invariant to emit one
// header per section as it walks the items (D77); Reconcile preserves it by only
// appending discovered extras (into the trailing Custom Resources section).
//
// Each item carries a real kube.Resource (GVK/GVR/scope) so drilling in emits a
// ResourceSelectedMsg the kube layer's List/Watch can act on directly. The
// discovery pass later replaces a seed entry with its discovered twin (same GVR),
// filling in verbs/short-names/categories.
func seedItems() []Item {
	specs := []struct {
		group, version, resource, kind string
		namespaced                     bool
		section                        string
	}{
		// Cluster — non-namespaced overview.
		{"", "v1", "namespaces", "Namespace", false, sectionCluster},
		{"", "v1", "nodes", "Node", false, sectionCluster},
		{"", "v1", "persistentvolumes", "PersistentVolume", false, sectionCluster},
		{"storage.k8s.io", "v1", "storageclasses", "StorageClass", false, sectionCluster},
		// Workloads — namespaced compute + their events.
		{"", "v1", "pods", "Pod", true, sectionWorkloads},
		{"apps", "v1", "deployments", "Deployment", true, sectionWorkloads},
		{"apps", "v1", "statefulsets", "StatefulSet", true, sectionWorkloads},
		{"apps", "v1", "daemonsets", "DaemonSet", true, sectionWorkloads},
		{"apps", "v1", "replicasets", "ReplicaSet", true, sectionWorkloads},
		{"batch", "v1", "jobs", "Job", true, sectionWorkloads},
		{"batch", "v1", "cronjobs", "CronJob", true, sectionWorkloads},
		{"", "v1", "events", "Event", true, sectionWorkloads},
		// Config.
		{"", "v1", "configmaps", "ConfigMap", true, sectionConfig},
		{"", "v1", "secrets", "Secret", true, sectionConfig},
		// Network.
		{"", "v1", "services", "Service", true, sectionNetwork},
		{"networking.k8s.io", "v1", "ingresses", "Ingress", true, sectionNetwork},
		// Storage — namespaced claims (cluster-wide volumes/classes live above).
		{"", "v1", "persistentvolumeclaims", "PersistentVolumeClaim", true, sectionStorage},
		// Access control.
		{"", "v1", "serviceaccounts", "ServiceAccount", true, sectionAccess},
	}

	items := make([]Item, 0, len(specs)+1)
	for _, s := range specs {
		items = append(items, Item{
			Resource: kube.Resource{
				GVK:        schema.GroupVersionKind{Group: s.group, Version: s.version, Kind: s.kind},
				GVR:        schema.GroupVersionResource{Group: s.group, Version: s.version, Resource: s.resource},
				Namespaced: s.namespaced,
			},
			Title:     s.kind,
			Available: true,
			Section:   s.section,
		})
	}

	// Insert the namespace-picker seam between the cluster-scoped section and the
	// namespaced ones (feedback 2026-07-21-01): the seam marks that boundary — the
	// cluster-scoped kinds (Namespace, Node, PV, StorageClass) sit above it, the
	// namespaced kinds below. It is a non-resource row (ItemNamespace) with no
	// Section, so it renders un-grouped between the two and Reconcile skips it.
	insertAt := len(items)
	for i, it := range items {
		if it.Section != sectionCluster {
			insertAt = i
			break
		}
	}
	seam := Item{Kind: ItemNamespace, Title: "Namespace", Available: true}
	out := make([]Item, 0, len(items)+1)
	out = append(out, items[:insertAt]...)
	out = append(out, seam)
	out = append(out, items[insertAt:]...)
	return out
}
