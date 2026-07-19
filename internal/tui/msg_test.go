package tui

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// TestNewErrorMsg checks Kind is classified from Err and stays in sync, and that
// a nil error classifies to Unknown.
func TestNewErrorMsg(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "p", errors.New("nope"))
	m := NewErrorMsg("watch pods", forbidden)
	if m.Context != "watch pods" {
		t.Errorf("Context = %q, want %q", m.Context, "watch pods")
	}
	if m.Kind != kube.KindForbidden {
		t.Errorf("Kind = %v, want %v", m.Kind, kube.KindForbidden)
	}
	if !errors.Is(m.Err, forbidden) {
		t.Errorf("Err not preserved: %v", m.Err)
	}

	if got := NewErrorMsg("x", nil).Kind; got != kube.KindUnknown {
		t.Errorf("nil err Kind = %v, want %v", got, kube.KindUnknown)
	}
}

// TestWatchPumpDelta: a data delta becomes a ResourceEventMsg carrying the event
// verbatim.
func TestWatchPumpDelta(t *testing.T) {
	ch := make(chan kube.WatchEvent, 1)
	ev := kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "Name"}},
		Rows:    []kube.Row{{Cells: []any{"pod-a"}, Object: kube.ObjectRef{Name: "pod-a"}}},
	}
	ch <- ev

	msg := watchPump(ch)()
	got, ok := msg.(ResourceEventMsg)
	if !ok {
		t.Fatalf("msg type = %T, want ResourceEventMsg", msg)
	}
	if got.Event.Type != kube.WatchReset || len(got.Event.Rows) != 1 || got.Event.Rows[0].Object.Name != "pod-a" {
		t.Errorf("event not carried verbatim: %+v", got.Event)
	}
}

// TestWatchPumpError: an ERROR watch event is bridged to a classified ErrorMsg,
// not delivered as a ResourceEventMsg.
func TestWatchPumpError(t *testing.T) {
	ch := make(chan kube.WatchEvent, 1)
	cause := apierrors.NewUnauthorized("bad token")
	ch <- kube.WatchEvent{Type: kube.WatchError, Err: cause}

	msg := watchPump(ch)()
	got, ok := msg.(ErrorMsg)
	if !ok {
		t.Fatalf("msg type = %T, want ErrorMsg", msg)
	}
	if got.Kind != kube.KindUnauthorized {
		t.Errorf("Kind = %v, want %v", got.Kind, kube.KindUnauthorized)
	}
	if got.Context != "watch" {
		t.Errorf("Context = %q, want %q", got.Context, "watch")
	}
}

// TestWatchPumpClosed: a closed channel becomes the terminal WatchClosedMsg.
func TestWatchPumpClosed(t *testing.T) {
	ch := make(chan kube.WatchEvent)
	close(ch)

	msg := watchPump(ch)()
	if _, ok := msg.(WatchClosedMsg); !ok {
		t.Fatalf("msg type = %T, want WatchClosedMsg", msg)
	}
}

// TestDiscoveryPumpReady: the single discovery result — including one with
// isolated per-group failures and a total error — is delivered as a
// DiscoveryReadyMsg carrying the whole result.
func TestDiscoveryPumpReady(t *testing.T) {
	ch := make(chan kube.DiscoveryResult, 1)
	res := kube.DiscoveryResult{
		Resources: []kube.Resource{{GVK: schema.GroupVersionKind{Kind: "Pod", Version: "v1"}}},
		Failed:    []kube.FailedGroup{{GroupVersion: "metrics.k8s.io/v1beta1", Err: errors.New("down")}},
	}
	ch <- res

	msg := discoveryPump(ch)()
	got, ok := msg.(DiscoveryReadyMsg)
	if !ok {
		t.Fatalf("msg type = %T, want DiscoveryReadyMsg", msg)
	}
	if len(got.Result.Resources) != 1 || got.Result.Resources[0].GVK.Kind != "Pod" {
		t.Errorf("resources not carried: %+v", got.Result.Resources)
	}
	if len(got.Result.Failed) != 1 || got.Result.Failed[0].GroupVersion != "metrics.k8s.io/v1beta1" {
		t.Errorf("failed groups not carried: %+v", got.Result.Failed)
	}
}

// TestDiscoveryPumpTotalFailure: a total discovery failure is still delivered as
// a DiscoveryReadyMsg (Result.Err set), keeping the reconcile signal rather than
// swapping in a generic ErrorMsg.
func TestDiscoveryPumpTotalFailure(t *testing.T) {
	ch := make(chan kube.DiscoveryResult, 1)
	ch <- kube.DiscoveryResult{Err: errors.New("api server unreachable")}

	msg := discoveryPump(ch)()
	got, ok := msg.(DiscoveryReadyMsg)
	if !ok {
		t.Fatalf("msg type = %T, want DiscoveryReadyMsg", msg)
	}
	if got.Result.Err == nil {
		t.Errorf("Result.Err not carried")
	}
}

// TestDiscoveryPumpClosed: a closed channel with no value yields a nil message
// (Bubble Tea ignores nil), never a panic.
func TestDiscoveryPumpClosed(t *testing.T) {
	ch := make(chan kube.DiscoveryResult)
	close(ch)

	if msg := discoveryPump(ch)(); msg != nil {
		t.Fatalf("msg = %v, want nil", msg)
	}
}
