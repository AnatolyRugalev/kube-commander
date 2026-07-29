package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The measured kinds and the metrics kinds that measure them, as a caller's
// available set would carry them. The metrics entries carry verbs so the tests
// can pin that MetricsFor hands back the *discovered* Resource rather than a
// synthesized stub — the same property TestChildrenDeploymentLabelSelector pins
// for children.
var (
	metricsPodResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Namespaced: true,
		Verbs:      metav1.Verbs{"get", "list", "watch"},
	}
	metricsNodeResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"},
		GVR:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
		Namespaced: false,
		Verbs:      metav1.Verbs{"get", "list", "watch"},
	}
	metricsDeploymentResource = Resource{
		GVK:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		GVR:        schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		Namespaced: true,
	}
	podMetricsResource = Resource{
		GVK:        schema.GroupVersionKind{Group: MetricsGroup, Version: "v1beta1", Kind: "PodMetrics"},
		GVR:        schema.GroupVersionResource{Group: MetricsGroup, Version: "v1beta1", Resource: "pods"},
		Namespaced: true,
		Verbs:      metav1.Verbs{"get", "list"},
	}
	nodeMetricsResource = Resource{
		GVK:        schema.GroupVersionKind{Group: MetricsGroup, Version: "v1beta1", Kind: "NodeMetrics"},
		GVR:        schema.GroupVersionResource{Group: MetricsGroup, Version: "v1beta1", Resource: "nodes"},
		Namespaced: false,
		Verbs:      metav1.Verbs{"get", "list"},
	}
)

// metricsKinds is a cluster *with* metrics-server; metricsKindsNoServer is the
// same cluster without it (or with the group isolated into DiscoveryResult.Failed
// because the aggregated API is down — indistinguishable here, by design).
var (
	metricsKinds         = []Resource{metricsPodResource, metricsNodeResource, metricsDeploymentResource, podMetricsResource, nodeMetricsResource}
	metricsKindsNoServer = []Resource{metricsPodResource, metricsNodeResource, metricsDeploymentResource}
)

// metricsDynamicFake seeds a fake dynamic client with metrics items. They are
// added through the tracker under an *explicit* GVR rather than passed to the
// constructor, because the fake infers a seeded object's resource from its kind
// (UnsafeGuessKindToResource) — which for the metrics API is wrong twice over:
// PodMetrics is served at `pods`, NodeMetrics at `nodes`.
func metricsDynamicFake(t *testing.T, objs ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{
		podMetricsResource.GVR:  "PodMetricsList",
		nodeMetricsResource.GVR: "NodeMetricsList",
	}
	f := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	for _, o := range objs {
		gvr := podMetricsResource.GVR
		if o.GetKind() == "NodeMetrics" {
			gvr = nodeMetricsResource.GVR
		}
		if err := f.Tracker().Create(gvr, o, o.GetNamespace()); err != nil {
			t.Fatalf("seeding %s %s: %v", o.GetKind(), o.GetName(), err)
		}
	}
	return f
}

// podMetricsItem builds a PodMetrics as metrics-server serves it: per-container
// usage maps under `containers`, plus the sample's window and timestamp.
func podMetricsItem(ns, name string, containers ...map[string]any) *unstructured.Unstructured {
	entries := make([]any, 0, len(containers))
	for _, c := range containers {
		entries = append(entries, c)
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": MetricsGroup + "/v1beta1",
		"kind":       "PodMetrics",
		"metadata":   map[string]any{"namespace": ns, "name": name},
		"timestamp":  "2026-07-29T10:00:00Z",
		"window":     "30s",
		"containers": entries,
	}}
}

func metricsContainer(name, cpu, mem string) map[string]any {
	return map[string]any{"name": name, "usage": map[string]any{"cpu": cpu, "memory": mem}}
}

// nodeMetricsItem builds a NodeMetrics: one flat top-level usage map, no
// namespace.
func nodeMetricsItem(name, cpu, mem string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": MetricsGroup + "/v1beta1",
		"kind":       "NodeMetrics",
		"metadata":   map[string]any{"name": name},
		"timestamp":  "2026-07-29T10:00:00Z",
		"window":     "30s",
		"usage":      map[string]any{"cpu": cpu, "memory": mem},
	}}
}

func TestMetricsForResolvesDiscoveredResource(t *testing.T) {
	m, ok := MetricsFor(metricsPodResource, metricsKinds)
	if !ok {
		t.Fatal("MetricsFor(Pod): want available")
	}
	if m.GVR != podMetricsResource.GVR {
		t.Errorf("GVR = %v, want %v", m.GVR, podMetricsResource.GVR)
	}
	if !m.Namespaced {
		t.Error("PodMetrics should be namespaced")
	}
	if len(m.Verbs) == 0 {
		t.Error("want the discovered Resource (verbs intact), got a stub")
	}

	n, ok := MetricsFor(metricsNodeResource, metricsKinds)
	if !ok {
		t.Fatal("MetricsFor(Node): want available")
	}
	if n.GVR != nodeMetricsResource.GVR {
		t.Errorf("GVR = %v, want %v", n.GVR, nodeMetricsResource.GVR)
	}
	if n.Namespaced {
		t.Error("NodeMetrics should be cluster-scoped")
	}
}

func TestMetricsForAbsentWithoutMetricsServer(t *testing.T) {
	if _, ok := MetricsFor(metricsPodResource, metricsKindsNoServer); ok {
		t.Error("want unavailable when the metrics group is not in the discovered set")
	}
	if _, ok := MetricsFor(metricsNodeResource, metricsKindsNoServer); ok {
		t.Error("want unavailable for nodes too")
	}
}

func TestMetricsForUnmeasuredKind(t *testing.T) {
	if _, ok := MetricsFor(metricsDeploymentResource, metricsKinds); ok {
		t.Error("Deployment has no metrics counterpart; want unavailable")
	}
}

// A metrics resource the user may not list is as good as absent: the columns must
// not appear only to fail on every refresh.
func TestMetricsForRequiresListVerb(t *testing.T) {
	noList := podMetricsResource
	noList.Verbs = metav1.Verbs{"get"}
	kinds := []Resource{metricsPodResource, noList}
	if _, ok := MetricsFor(metricsPodResource, kinds); ok {
		t.Error("want unavailable when the metrics resource cannot be listed")
	}
}

func TestHasMetrics(t *testing.T) {
	cases := []struct {
		name  string
		kind  Resource
		kinds []Resource
		want  bool
	}{
		{"pod with server", metricsPodResource, metricsKinds, true},
		{"node with server", metricsNodeResource, metricsKinds, true},
		{"pod without server", metricsPodResource, metricsKindsNoServer, false},
		{"node without server", metricsNodeResource, metricsKindsNoServer, false},
		{"deployment", metricsDeploymentResource, metricsKinds, false},
		{"zero resource", Resource{}, metricsKinds, false},
		{"no kinds at all", metricsPodResource, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasMetrics(tc.kind, tc.kinds); got != tc.want {
				t.Errorf("HasMetrics = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsageKeyOfDropsUID(t *testing.T) {
	// The join key must ignore the row's UID: a PodMetrics is a different object
	// from the Pod it measures, so a UID-bearing key would never match.
	ref := ObjectRef{Namespace: "default", Name: "api-0", UID: "abc-123"}
	want := UsageKey{Namespace: "default", Name: "api-0"}
	if got := UsageKeyOf(ref); got != want {
		t.Errorf("UsageKeyOf = %#v, want %#v", got, want)
	}
	u := Usage{Namespace: "default", Name: "api-0"}
	if u.Key() != want {
		t.Errorf("Usage.Key = %#v, want %#v", u.Key(), want)
	}
}

func TestPodMetricsSumsContainers(t *testing.T) {
	c := &Clients{Dynamic: metricsDynamicFake(t,
		podMetricsItem("default", "api-0", metricsContainer("app", "250m", "64Mi"), metricsContainer("sidecar", "1", "128Mi")),
		podMetricsItem("default", "web-0", metricsContainer("app", "12m", "8Mi")),
	)}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2: %#v", len(got), got)
	}
	api := got[UsageKey{Namespace: "default", Name: "api-0"}]
	if api.CPUMilli != 1250 {
		t.Errorf("api-0 CPUMilli = %d, want 1250 (250m + 1)", api.CPUMilli)
	}
	if want := int64(64+128) * 1024 * 1024; api.MemoryBytes != want {
		t.Errorf("api-0 MemoryBytes = %d, want %d", api.MemoryBytes, want)
	}
	web := got[UsageKey{Namespace: "default", Name: "web-0"}]
	if web.CPUMilli != 12 || web.MemoryBytes != 8*1024*1024 {
		t.Errorf("web-0 = %d milli / %d bytes, want 12 / %d", web.CPUMilli, web.MemoryBytes, 8*1024*1024)
	}
}

func TestPodMetricsCarriesWindowAndTimestamp(t *testing.T) {
	c := &Clients{Dynamic: metricsDynamicFake(t,
		podMetricsItem("default", "api-0", metricsContainer("app", "1m", "1Mi")),
	)}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	u := got[UsageKey{Namespace: "default", Name: "api-0"}]
	if u.Window != 30*time.Second {
		t.Errorf("Window = %v, want 30s", u.Window)
	}
	if want := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC); !u.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", u.Timestamp, want)
	}
}

// Staleness metadata is decoration; a sample with an unreadable window/timestamp
// still carries usable numbers and must survive.
func TestMetricsMalformedWindowKeepsSample(t *testing.T) {
	item := podMetricsItem("default", "api-0", metricsContainer("app", "5m", "2Mi"))
	item.Object["window"] = "half a minute"
	item.Object["timestamp"] = "yesterday"
	c := &Clients{Dynamic: metricsDynamicFake(t, item)}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	u, ok := got[UsageKey{Namespace: "default", Name: "api-0"}]
	if !ok {
		t.Fatal("sample dropped over unreadable staleness metadata")
	}
	if u.CPUMilli != 5 || u.MemoryBytes != 2*1024*1024 {
		t.Errorf("usage = %d / %d, want 5 / %d", u.CPUMilli, u.MemoryBytes, 2*1024*1024)
	}
	if u.Window != 0 || !u.Timestamp.IsZero() {
		t.Errorf("want zeroed staleness metadata, got %v / %v", u.Window, u.Timestamp)
	}
}

func TestNodeMetricsClusterScoped(t *testing.T) {
	c := &Clients{Dynamic: metricsDynamicFake(t,
		nodeMetricsItem("node-a", "1500m", "2Gi"),
		nodeMetricsItem("node-b", "2", "512Mi"),
	)}
	// The namespace argument is ignored for a cluster-scoped metrics kind; passing
	// a bogus one must not scope (or empty) the result.
	got, err := c.Metrics(context.Background(), nodeMetricsResource, "kube-system")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2: %#v", len(got), got)
	}
	a, ok := got[UsageKey{Name: "node-a"}]
	if !ok {
		t.Fatalf("node-a not keyed with an empty namespace: %#v", got)
	}
	if a.CPUMilli != 1500 || a.MemoryBytes != 2*1024*1024*1024 {
		t.Errorf("node-a = %d / %d, want 1500 / %d", a.CPUMilli, a.MemoryBytes, 2*1024*1024*1024)
	}
	if b := got[UsageKey{Name: "node-b"}]; b.CPUMilli != 2000 {
		t.Errorf("node-b CPUMilli = %d, want 2000 (whole cores → millicores)", b.CPUMilli)
	}
}

func TestPodMetricsNamespaceScoping(t *testing.T) {
	c := &Clients{Dynamic: metricsDynamicFake(t,
		podMetricsItem("default", "api-0", metricsContainer("app", "1m", "1Mi")),
		podMetricsItem("kube-system", "dns-0", metricsContainer("app", "2m", "2Mi")),
	)}
	one, err := c.Metrics(context.Background(), podMetricsResource, "kube-system")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(one) != 1 {
		t.Fatalf("namespaced list returned %d samples, want 1: %#v", len(one), one)
	}
	if _, ok := one[UsageKey{Namespace: "kube-system", Name: "dns-0"}]; !ok {
		t.Errorf("want the kube-system sample, got %#v", one)
	}

	all, err := c.Metrics(context.Background(), podMetricsResource, "")
	if err != nil {
		t.Fatalf("Metrics(all namespaces): %v", err)
	}
	if len(all) != 2 {
		t.Errorf(`namespace "" returned %d samples, want every namespace (2)`, len(all))
	}
}

// One unreadable item must not cost the whole refresh — but it must not be
// reported with a fabricated number either, so it is dropped.
func TestMetricsDropsUnreadableItems(t *testing.T) {
	badQuantity := podMetricsItem("default", "bad-cpu", metricsContainer("app", "lots", "1Mi"))
	missingMemory := podMetricsItem("default", "no-mem", map[string]any{"name": "app", "usage": map[string]any{"cpu": "1m"}})
	noContainers := podMetricsItem("default", "no-containers")
	partialSum := podMetricsItem("default", "partial", metricsContainer("app", "10m", "1Mi"), metricsContainer("broken", "???", "1Mi"))
	good := podMetricsItem("default", "fine", metricsContainer("app", "7m", "3Mi"))

	c := &Clients{Dynamic: metricsDynamicFake(t, badQuantity, missingMemory, noContainers, partialSum, good)}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d samples, want only the readable one: %#v", len(got), got)
	}
	u, ok := got[UsageKey{Namespace: "default", Name: "fine"}]
	if !ok {
		t.Fatalf("the readable sample was dropped: %#v", got)
	}
	if u.CPUMilli != 7 {
		t.Errorf("CPUMilli = %d, want 7", u.CPUMilli)
	}
	if _, ok := got[UsageKey{Namespace: "default", Name: "partial"}]; ok {
		t.Error("a pod with one unreadable container was reported with a partial sum")
	}
}

// Nothing scraped yet is a normal answer, not a failure.
func TestMetricsEmptyListIsNotAnError(t *testing.T) {
	c := &Clients{Dynamic: metricsDynamicFake(t)}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d samples, want none", len(got))
	}
}

// The aggregated API being present but down is the common case: the request
// fails, and the caller gets an error to log rather than a half-answer.
func TestMetricsListErrorWrapped(t *testing.T) {
	fake := metricsDynamicFake(t)
	fake.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the server is currently unable to handle the request")
	})
	c := &Clients{Dynamic: fake}
	got, err := c.Metrics(context.Background(), podMetricsResource, "default")
	if err == nil {
		t.Fatal("want an error when the metrics API is down")
	}
	if got != nil {
		t.Errorf("want no samples alongside the error, got %#v", got)
	}
}

func TestMetricsRefusesWithoutClientOrResource(t *testing.T) {
	if _, err := (*Clients)(nil).Metrics(context.Background(), podMetricsResource, ""); err == nil {
		t.Error("nil Clients: want an error, not a panic")
	}
	if _, err := (&Clients{}).Metrics(context.Background(), podMetricsResource, ""); err == nil {
		t.Error("no dynamic client: want an error")
	}
	c := &Clients{Dynamic: metricsDynamicFake(t)}
	if _, err := c.Metrics(context.Background(), Resource{}, ""); err == nil {
		t.Error("zero metrics Resource: want an error rather than a request for nothing")
	}
}
