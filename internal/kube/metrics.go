package kube

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// MetricsGroup is the aggregated API group metrics-server serves. It is reached
// through the ordinary dynamic client like any other group — kubecom takes no
// dependency on k8s.io/metrics and shells out to nothing (D2).
const MetricsGroup = "metrics.k8s.io"

// UsageKey identifies a measured object by namespace and name — deliberately
// *not* by UID. A PodMetrics is a different object from the Pod it measures: its
// own metadata.uid is unrelated (and usually empty), so joining a usage sample to
// a table row on ObjectRef, UID included, would silently never match. Namespace +
// name is the identity the metrics API actually shares with the measured object.
// Namespace is "" for cluster-scoped kinds (nodes).
type UsageKey struct {
	Namespace string
	Name      string
}

// UsageKeyOf projects a row's ObjectRef onto the key its usage sample is stored
// under, dropping the UID. Callers joining metrics onto watched rows should use
// it rather than building the key by hand.
func UsageKeyOf(ref ObjectRef) UsageKey {
	return UsageKey{Namespace: ref.Namespace, Name: ref.Name}
}

// Usage is one object's point-in-time CPU/memory sample, decoded off the metrics
// API into plain Go numbers so the TUI never handles a resource.Quantity.
//
// CPUMilli is millicores (the quantity's MilliValue: "250m" → 250, "2" → 2000)
// and MemoryBytes is bytes ("128Mi" → 134217728). Both are absolute usage, not a
// fraction of a limit — kubecom has no request/limit context at this seam.
//
// Window and Timestamp come straight off the sample and are the caller's only
// staleness signal: metrics-server keeps serving the last scrape it took, so a
// Timestamp minutes in the past means the sample is stale even though the request
// succeeded. Either may be zero when the server omitted or malformed the field —
// that never invalidates the numbers, which are the point.
type Usage struct {
	Namespace   string
	Name        string
	CPUMilli    int64
	MemoryBytes int64
	Window      time.Duration
	Timestamp   time.Time
}

// Key is the sample's UsageKey — the key it is stored under in the Metrics map.
func (u Usage) Key() UsageKey {
	return UsageKey{Namespace: u.Namespace, Name: u.Name}
}

// metricsLinks maps a browsable kind to the metrics kind that measures it. Only
// the two the metrics API defines exist; there is no generic mechanism to extend
// this with, so the map is closed by the upstream API rather than by choice.
var metricsLinks = map[schema.GroupKind]schema.GroupKind{
	{Group: "", Kind: "Pod"}:  {Group: MetricsGroup, Kind: "PodMetrics"},
	{Group: "", Kind: "Node"}: {Group: MetricsGroup, Kind: "NodeMetrics"},
}

// MetricsFor resolves the metrics Resource that measures kind, looked up in the
// caller's available set (the discovery result the menu is built from) rather
// than synthesized — so the returned Resource carries the version and verbs the
// cluster actually reports, exactly as Children does for child kinds.
//
// The second return is the availability answer, and it is the *whole* answer:
// when metrics-server is not installed the group is absent from discovery, and
// when it is installed but down ServerPreferredResources isolates the group into
// DiscoveryResult.Failed instead of Resources (#87) — so a broken aggregated API
// reads here as plainly unavailable, which is the degrade the common case needs
// (principle 3). No request is made.
func MetricsFor(kind Resource, kinds []Resource) (Resource, bool) {
	gk, ok := metricsLinks[kind.GVK.GroupKind()]
	if !ok {
		return Resource{}, false
	}
	m, ok := resourceForGroupKind(kinds, gk)
	if !ok {
		return Resource{}, false
	}
	if !hasVerb(m.Verbs, "list") {
		return Resource{}, false
	}
	return m, true
}

// HasMetrics reports whether kind has a metrics counterpart available in kinds.
// It is the pure, request-free gate a caller hangs the metrics columns on — the
// same role HasChildren plays for the drill-down.
func HasMetrics(kind Resource, kinds []Resource) bool {
	_, ok := MetricsFor(kind, kinds)
	return ok
}

// Metrics lists the usage samples served by the metrics resource m — which must
// be one MetricsFor returned — and returns them keyed for a join against the rows
// already on screen. namespace scopes a namespaced metrics kind (PodMetrics); ""
// means every namespace, and it is ignored for a cluster-scoped one (NodeMetrics).
//
// This is a one-shot List and never a watch: the metrics API serves point-in-time
// samples and supports no watch verb, so a caller refreshes on a slow ticker and
// joins the result onto its watched rows (D155 pt 3). An empty map is a valid,
// non-error answer — metrics-server returns nothing for objects it has not
// scraped yet.
//
// A single unreadable item (a quantity the server did not format as a quantity, a
// sample with no name) is dropped and the rest are returned, rather than failing
// the whole refresh over one bad row; a wrong number would be worse than a missing
// one, so a partial container sum is never used. Only the request itself failing
// is an error, and the caller is expected to degrade to "no columns" and log it
// (D159) rather than surfacing it — an aggregated API is the flakiest thing in a
// cluster and this is a decoration.
func (c *Clients) Metrics(ctx context.Context, m Resource, namespace string) (map[UsageKey]Usage, error) {
	if c == nil || c.Dynamic == nil {
		return nil, fmt.Errorf("kube: metrics: no dynamic client")
	}
	if m.GVR.Empty() {
		return nil, fmt.Errorf("kube: metrics: no metrics resource")
	}
	ri := c.Dynamic.Resource(m.GVR)
	var lister dynamic.ResourceInterface = ri
	if m.Namespaced {
		lister = ri.Namespace(namespace)
	}
	list, err := lister.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing %s: %w", m.GVR.Resource, err)
	}
	out := make(map[UsageKey]Usage, len(list.Items))
	for i := range list.Items {
		u, ok := usageFromItem(list.Items[i])
		if !ok {
			continue
		}
		out[u.Key()] = u
	}
	return out, nil
}

// usageFromItem decodes one metrics item into a Usage. It handles both shapes the
// metrics API serves: a NodeMetrics carries a single top-level `usage` map, while
// a PodMetrics carries a `containers` list whose per-container `usage` maps are
// summed — a pod's usage is the sum of its containers, which is also what
// `kubectl top pod` reports.
//
// It reports false rather than a zeroed Usage whenever the sample cannot be read
// in full, so a caller never shows a number the server did not say.
func usageFromItem(item unstructured.Unstructured) (Usage, bool) {
	name := item.GetName()
	if name == "" {
		return Usage{}, false
	}
	u := Usage{
		Namespace: item.GetNamespace(),
		Name:      name,
		Window:    nestedDuration(item.Object, "window"),
		Timestamp: nestedTime(item.Object, "timestamp"),
	}
	if containers, found, err := unstructured.NestedSlice(item.Object, "containers"); found && err == nil {
		if len(containers) == 0 {
			return Usage{}, false
		}
		for _, entry := range containers {
			c, ok := entry.(map[string]any)
			if !ok {
				return Usage{}, false
			}
			cpu, mem, ok := usageAmounts(c)
			if !ok {
				return Usage{}, false
			}
			u.CPUMilli += cpu
			u.MemoryBytes += mem
		}
		return u, true
	}
	cpu, mem, ok := usageAmounts(item.Object)
	if !ok {
		return Usage{}, false
	}
	u.CPUMilli, u.MemoryBytes = cpu, mem
	return u, true
}

// usageAmounts reads the `usage` map hanging off obj (a node metrics item or one
// container entry) as millicores and bytes. Both fields must be present and
// parseable — a sample with only one of them is not a usable row.
func usageAmounts(obj map[string]any) (cpuMilli, memBytes int64, ok bool) {
	cpu, ok := nestedQuantity(obj, "usage", "cpu")
	if !ok {
		return 0, 0, false
	}
	mem, ok := nestedQuantity(obj, "usage", "memory")
	if !ok {
		return 0, 0, false
	}
	return cpu.MilliValue(), mem.Value(), true
}

// nestedQuantity reads a resource.Quantity at the given path. Quantities are
// always JSON strings on the wire (Quantity marshals as one, even for whole
// numbers), so a non-string is a malformed sample, not a shape to be lenient
// about.
func nestedQuantity(obj map[string]any, fields ...string) (resource.Quantity, bool) {
	s, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil || s == "" {
		return resource.Quantity{}, false
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.Quantity{}, false
	}
	return q, true
}

// nestedDuration reads a Go-parseable duration string ("30s") at the given path,
// returning 0 when it is absent or malformed — staleness metadata never
// invalidates a sample.
func nestedDuration(obj map[string]any, fields ...string) time.Duration {
	s, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// nestedTime reads an RFC3339 timestamp at the given path, returning the zero
// time when it is absent or malformed.
func nestedTime(obj map[string]any, fields ...string) time.Time {
	s, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
