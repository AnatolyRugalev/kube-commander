package menu

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// seedItems is the static default menu: the common core resource kinds a user
// browses first, in a sensible top-down order (cluster overview → workloads →
// networking → config → storage). It is deliberately a fixed list — M2-05b
// reconciles it against live discovery (adding CRDs/extra groups, marking
// unavailable ones), but the seed lets the browse view render and be navigated
// immediately, before discovery completes (fast cold start, principle 4).
//
// Each item carries a real kube.Resource (GVK/GVR/scope) so drilling in emits a
// ResourceSelectedMsg the kube layer's List/Watch can act on directly. The
// discovery pass later replaces a seed entry with its discovered twin (same GVR),
// filling in verbs/short-names/categories.
func seedItems() []Item {
	specs := []struct {
		group, version, resource, kind string
		namespaced                     bool
	}{
		// Cluster overview.
		{"", "v1", "namespaces", "Namespace", false},
		{"", "v1", "nodes", "Node", false},
		{"", "v1", "events", "Event", true},
		// Workloads.
		{"", "v1", "pods", "Pod", true},
		{"apps", "v1", "deployments", "Deployment", true},
		{"apps", "v1", "statefulsets", "StatefulSet", true},
		{"apps", "v1", "daemonsets", "DaemonSet", true},
		{"apps", "v1", "replicasets", "ReplicaSet", true},
		{"batch", "v1", "jobs", "Job", true},
		{"batch", "v1", "cronjobs", "CronJob", true},
		// Networking.
		{"", "v1", "services", "Service", true},
		{"networking.k8s.io", "v1", "ingresses", "Ingress", true},
		// Config & access.
		{"", "v1", "configmaps", "ConfigMap", true},
		{"", "v1", "secrets", "Secret", true},
		{"", "v1", "serviceaccounts", "ServiceAccount", true},
		// Storage.
		{"", "v1", "persistentvolumeclaims", "PersistentVolumeClaim", true},
		{"", "v1", "persistentvolumes", "PersistentVolume", false},
		{"storage.k8s.io", "v1", "storageclasses", "StorageClass", false},
	}

	items := make([]Item, 0, len(specs))
	for _, s := range specs {
		items = append(items, Item{
			Resource: kube.Resource{
				GVK: schema.GroupVersionKind{Group: s.group, Version: s.version, Kind: s.kind},
				GVR: schema.GroupVersionResource{Group: s.group, Version: s.version, Resource: s.resource},
				Namespaced: s.namespaced,
			},
			Title:     s.kind,
			Available: true,
		})
	}
	return items
}
