package tui

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// fakeClusterClient satisfies tui.ClusterClient — every cluster-bound seam a single
// client can serve directly — with no-op implementations. It exists to test the
// wiring (which field points where), never behaviour: each seam's behaviour is
// covered by the flow tests that drive it with a purpose-built fake. *kube.Clients is
// the real implementation; this is the hermetic stand-in (D18).
type fakeClusterClient struct{}

func (fakeClusterClient) Watch(context.Context, kube.Resource, string, metav1.ListOptions) (<-chan kube.WatchEvent, error) {
	return nil, nil
}
func (fakeClusterClient) StartDiscovery(context.Context) <-chan kube.DiscoveryResult { return nil }
func (fakeClusterClient) Namespaces(context.Context) ([]string, error)               { return nil, nil }
func (fakeClusterClient) GetYAML(context.Context, kube.Resource, kube.ObjectRef) (string, error) {
	return "", nil
}
func (fakeClusterClient) Describe(kube.Resource, kube.ObjectRef) (string, error) { return "", nil }
func (fakeClusterClient) Logs(context.Context, kube.ObjectRef, kube.LogOptions) (<-chan kube.LogEvent, error) {
	return nil, nil
}
func (fakeClusterClient) PodContainers(context.Context, kube.ObjectRef) ([]kube.Container, error) {
	return nil, nil
}
func (fakeClusterClient) SecretData(context.Context, kube.ObjectRef) (kube.SecretData, error) {
	return kube.SecretData{}, nil
}
func (fakeClusterClient) PodForOwner(context.Context, kube.Resource, kube.ObjectRef) (kube.ObjectRef, error) {
	return kube.ObjectRef{}, nil
}
func (fakeClusterClient) PodForService(context.Context, kube.ObjectRef) (kube.ObjectRef, error) {
	return kube.ObjectRef{}, nil
}
func (fakeClusterClient) Delete(context.Context, kube.Resource, kube.ObjectRef, metav1.DeleteOptions) error {
	return nil
}
func (fakeClusterClient) Scale(context.Context, kube.Resource, kube.ObjectRef, int32) error {
	return nil
}
func (fakeClusterClient) RolloutRestart(context.Context, kube.Resource, kube.ObjectRef) error {
	return nil
}
func (fakeClusterClient) Cordon(context.Context, kube.Resource, kube.ObjectRef) error   { return nil }
func (fakeClusterClient) Uncordon(context.Context, kube.Resource, kube.ObjectRef) error { return nil }
func (fakeClusterClient) Suspend(context.Context, kube.Resource, kube.ObjectRef) error  { return nil }
func (fakeClusterClient) Resume(context.Context, kube.Resource, kube.ObjectRef) error   { return nil }
func (fakeClusterClient) DrainStream(context.Context, kube.Resource, kube.ObjectRef, kube.DrainOptions) <-chan kube.DrainEvent {
	return nil
}
func (fakeClusterClient) Update(context.Context, kube.Resource, kube.ObjectRef, []byte) error {
	return nil
}
func (fakeClusterClient) PodPorts(context.Context, kube.ObjectRef) ([]kube.Port, error) {
	return nil, nil
}
func (fakeClusterClient) ServicePorts(context.Context, kube.ObjectRef, kube.ObjectRef) ([]kube.Port, error) {
	return nil, nil
}
func (fakeClusterClient) Search(context.Context, []kube.Resource, string, kube.SearchQuery, int) <-chan kube.SearchEvent {
	return nil
}
func (fakeClusterClient) Exec(context.Context, kube.ObjectRef, kube.ExecOptions) error { return nil }
func (fakeClusterClient) Children(context.Context, kube.Resource, kube.ObjectRef, []kube.Resource) (kube.ChildScope, error) {
	return kube.ChildScope{}, nil
}

// noopForwarder is the PortForwarder half of the bundle, which cannot ride
// ClusterClient (kube.Clients.PortForward returns the concrete *kube.PortForward).
type noopForwarder struct{}

func (noopForwarder) PortForward(context.Context, kube.ObjectRef, []string) (ActiveForward, error) {
	return nil, nil
}

// TestNewClusterWiresEverySeam is the guard the M4-02 bundle exists for: a seam added
// to Cluster but forgotten in NewCluster would leave that feature silently inert in
// the real binary while every hermetic test — which wires its own fake through the
// individual With* option — still passed. Reflection rather than 21 explicit
// assertions precisely so a *new* field is covered without anyone remembering to
// extend this test.
func TestNewClusterWiresEverySeam(t *testing.T) {
	c := NewCluster(fakeClusterClient{}, noopForwarder{})

	v := reflect.ValueOf(c)
	for i := range v.NumField() {
		if v.Field(i).Kind() != reflect.Interface {
			continue // a seam is an interface; anything else is not this guard's business.
		}
		if v.Field(i).IsNil() {
			t.Errorf("NewCluster left seam %q nil — add it to the constructor (and, if the client serves it, to ClusterClient)", v.Type().Field(i).Name)
		}
	}
	if v.NumField() == 0 {
		t.Fatal("Cluster has no fields — the reflection guard is vacuous")
	}
}

// TestWithClusterReachesTheModel checks the bundle actually lands on the model's
// promoted fields (the shell reads m.watcher, m.deleter, … unqualified), and that an
// individual With* still overrides one seam of a bundle — the property every existing
// hermetic test that wires a single fake relies on.
func TestWithClusterReachesTheModel(t *testing.T) {
	m := New(WithCluster(NewCluster(fakeClusterClient{}, noopForwarder{})))
	if m.watcher == nil || m.deleter == nil || m.searcher == nil || m.portForwarder == nil {
		t.Fatal("WithCluster did not reach the model's promoted seams")
	}

	other := fakeDeleter{}
	m = New(WithCluster(NewCluster(fakeClusterClient{}, noopForwarder{})), WithDeleter(&other))
	if m.deleter != &other {
		t.Error("a later WithDeleter did not override the bundle's deleter")
	}
	if m.watcher == nil {
		t.Error("overriding one seam dropped the rest of the bundle")
	}
}

// TestClusterSwapRepointsEverySeamAtOnce is the premise the context switch (M4-03/04)
// is built on: one assignment moves the whole shell to another cluster, and — because
// Cluster is a value, not a pointer — a Model copied before the swap keeps the old
// cluster rather than silently observing the new one through a shared pointer
// (principle 1: no shared mutable state across the copies bubbletea makes).
func TestClusterSwapRepointsEverySeamAtOnce(t *testing.T) {
	before := New(WithCluster(NewCluster(fakeClusterClient{}, noopForwarder{})))
	oldWatcher, oldDeleter := before.watcher, before.deleter

	after := before
	after.Cluster = NewCluster(fakeClusterClient{}, noopForwarder{})

	if after.watcher == nil || after.deleter == nil {
		t.Fatal("the swapped-in bundle left seams unwired")
	}
	if before.watcher != oldWatcher || before.deleter != oldDeleter {
		t.Error("swapping the copy's cluster mutated the original — Cluster must be a value, not shared state")
	}
}
