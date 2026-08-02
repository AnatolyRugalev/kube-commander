package tui

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
)

// CRD-PIN-04: the resource picker is the surface a kind is reached from once, so
// every kind it lists has to be *findable* (by the names a Kubernetes user types,
// not only its Kind) and *reachable* (a Kind two API groups share used to collapse
// to one row, and the second kind could not be picked at all). D203.

// discoveredWith opens a sized, watchable, discoverable shell and folds one discovery
// pass carrying rs into its menu — the state every test below asserts against.
func discoveredWith(t *testing.T, rs ...kube.Resource) Model {
	t.Helper()
	fd := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := sizedWith(t, WithWatcher(&fakeWatcher{}), WithDiscoverer(fd))
	next, _ := m.Update(startDiscoveryMsg{})
	next, _ = next.(Model).Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: rs}})
	return next.(Model)
}

// crd builds a discovered custom resource: Kind, group and plural, plus whatever
// short names the server advertises.
func crd(group, kind, plural string, short ...string) kube.Resource {
	return kube.Resource{
		GVK:        schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind},
		GVR:        schema.GroupVersionResource{Group: group, Version: "v1", Resource: plural},
		Namespaced: true,
		ShortNames: short,
	}
}

// pickerLabels is the label set the resource picker would list, in order.
func pickerLabels(m Model) []string {
	items, _ := m.resourcePickerItems()
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}

// TestResourcePickerFindsAKindByItsPlural drives the case the CRD feedback describes:
// the reader knows the kind as `kubectl get externalsecrets` and types that. The plural
// is not a subsequence of "ExternalSecret", so before CRD-PIN-04 this query left the
// picker empty and the kind was unreachable by the only name the reader had.
func TestResourcePickerFindsAKindByItsPlural(t *testing.T) {
	m := discoveredWith(t, crd("external-secrets.io", "ExternalSecret", "externalsecrets", "es"))
	m, _ = press(t, m, capitalR)
	if !m.resPicker.Active() {
		t.Fatal("precondition: resources.switch should open the picker")
	}
	m = typeInto(t, m, "externalsecrets")
	if got := m.resPicker.Len(); got != 1 {
		t.Fatalf("query %q left %d rows, want 1 (ExternalSecret)", "externalsecrets", got)
	}
	if v, _ := m.resPicker.Selected(); v != "ExternalSecret" {
		t.Fatalf("selected %q, want ExternalSecret", v)
	}
}

// TestResourcePickerFindsAKindByItsShortName is the same property through the other
// name the server advertises — the one `kubectl get es` takes.
func TestResourcePickerFindsAKindByItsShortName(t *testing.T) {
	m := discoveredWith(t, crd("external-secrets.io", "ExternalSecret", "externalsecrets", "es"))
	m, _ = press(t, m, capitalR)
	m = typeInto(t, m, "es")
	if v, _ := m.resPicker.Selected(); v != "ExternalSecret" {
		t.Fatalf("selected %q, want ExternalSecret (its short name is the exact query)", v)
	}
}

// TestResourcePickerFindsAKindByItsGroup proves the third alias: half-remembering the
// operator ("external-secrets…") narrows to that group's kinds, which is how a CRD is
// hunted for when its Kind is the thing you cannot recall.
func TestResourcePickerFindsAKindByItsGroup(t *testing.T) {
	m := discoveredWith(t,
		crd("external-secrets.io", "ExternalSecret", "externalsecrets"),
		crd("external-secrets.io", "SecretStore", "secretstores"),
	)
	m, _ = press(t, m, capitalR)
	m = typeInto(t, m, "external-secrets.io")
	if got := m.resPicker.Len(); got != 2 {
		t.Fatalf("query by group left %d rows, want the group's 2 kinds", got)
	}
}

// TestCollidingKindsAreBothListedAndQualified is the reachability half. Two operators
// each own a `Cluster`; both rows must be listed, and each must say which group it is —
// a picker showing "Cluster" twice is not a choice the reader can make.
func TestCollidingKindsAreBothListedAndQualified(t *testing.T) {
	m := discoveredWith(t,
		crd("postgresql.cnpg.io", "Cluster", "clusters"),
		crd("cluster.x-k8s.io", "Cluster", "clusters"),
	)
	labels := pickerLabels(m)
	want := map[string]bool{
		"Cluster (postgresql.cnpg.io)": false,
		"Cluster (cluster.x-k8s.io)":   false,
	}
	for _, l := range labels {
		if _, ok := want[l]; ok {
			want[l] = true
		}
		if l == "Cluster" {
			t.Fatalf("a colliding Kind was listed unqualified: %v", labels)
		}
	}
	for l, seen := range want {
		if !seen {
			t.Fatalf("%q missing from the picker: %v", l, labels)
		}
	}
}

// TestCollidingKindsResolveToTheirOwnResource proves the qualification is not cosmetic:
// picking one of the two `Cluster` rows starts the watch for *that* group's GVR.
func TestCollidingKindsResolveToTheirOwnResource(t *testing.T) {
	fw := &fakeWatcher{}
	fd := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := sizedWith(t, WithWatcher(fw), WithDiscoverer(fd))
	next, _ := m.Update(startDiscoveryMsg{})
	next, _ = next.(Model).Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{Resources: []kube.Resource{
		crd("postgresql.cnpg.io", "Cluster", "clusters"),
		crd("cluster.x-k8s.io", "Cluster", "clusters"),
	}}})
	m = next.(Model)

	m, _ = press(t, m, capitalR)
	next, _ = m.Update(picker.SelectedMsg{Kind: resourcePickerKind, Value: "Cluster (cluster.x-k8s.io)"})
	m = next.(Model)

	if m.resPicker.Active() {
		t.Fatal("picking a resource should close the picker")
	}
	if len(fw.res) != 1 {
		t.Fatalf("picking a resource started %d watches, want 1", len(fw.res))
	}
	if got := fw.res[0].GVR.Group; got != "cluster.x-k8s.io" {
		t.Fatalf("watched group %q, want cluster.x-k8s.io — the qualified label resolved to the wrong kind", got)
	}
}

// TestUniqueKindKeepsItsBareLabel guards the other direction: qualification applies
// only where a name is ambiguous, so the seed kinds every reader's muscle memory is
// built on ("Pod", not "Pod (core)") are untouched by a CRD-heavy cluster.
func TestUniqueKindKeepsItsBareLabel(t *testing.T) {
	m := discoveredWith(t, crd("postgresql.cnpg.io", "Cluster", "clusters"))
	labels := pickerLabels(m)
	var sawPod, sawCluster bool
	for _, l := range labels {
		switch l {
		case "Pod":
			sawPod = true
		case "Cluster":
			sawCluster = true
		}
	}
	if !sawPod {
		t.Fatalf("the seed's Pod row lost its bare label: %v", labels)
	}
	if !sawCluster {
		t.Fatalf("an unambiguous CRD should keep its bare Kind: %v", labels)
	}
}

// TestResourceAliasesAreMatchOnly pins the aliases at the seam they are built, so the
// set stays "the names this kind answers to" rather than drifting into presentation.
func TestResourceAliasesAreMatchOnly(t *testing.T) {
	got := resourceAliases(crd("external-secrets.io", "ExternalSecret", "externalsecrets", "es"))
	want := []string{"externalsecrets", "es", "external-secrets.io"}
	if len(got) != len(want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aliases = %v, want %v", got, want)
		}
	}
	// A core-group kind has no group alias — "" would match every query.
	core := kube.Resource{
		GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	if got := resourceAliases(core); len(got) != 1 || got[0] != "pods" {
		t.Fatalf("core-group aliases = %v, want [pods]", got)
	}
}
