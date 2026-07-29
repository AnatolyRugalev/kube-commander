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

	msg := discoveryPump(ch, 0)()
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

	msg := discoveryPump(ch, 0)()
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

	if msg := discoveryPump(ch, 0)(); msg != nil {
		t.Fatalf("msg = %v, want nil", msg)
	}
}

// TestLogPumpBatchesBufferedLines: the drain half of LOGS-05b. Lines already sitting in
// the channel when the pump runs come back as one message, so a fast stream costs one
// render per frame rather than one per line.
func TestLogPumpBatchesBufferedLines(t *testing.T) {
	ch := make(chan kube.LogEvent, 8)
	for _, l := range []string{"a", "b", "c"} {
		ch <- kube.LogEvent{Line: l}
	}

	msg := logPump(ch)()
	got, ok := msg.(LogLineMsg)
	if !ok {
		t.Fatalf("msg type = %T, want LogLineMsg", msg)
	}
	if len(got.Lines) != 3 || got.Lines[0] != "a" || got.Lines[2] != "c" {
		t.Errorf("Lines = %q; want the three buffered lines in order", got.Lines)
	}
	if got.End != nil {
		t.Errorf("End = %v; want nil on a stream that is still open", got.End)
	}
}

// TestLogPumpStopsAtAnEmptyChannel: the drain is non-blocking, so a producer that is not
// ahead of the UI yields exactly the old one-line-per-Cmd behaviour — the pump must not
// wait for a second line that may never come.
func TestLogPumpStopsAtAnEmptyChannel(t *testing.T) {
	ch := make(chan kube.LogEvent, 4)
	ch <- kube.LogEvent{Line: "only"}

	got, ok := logPump(ch)().(LogLineMsg)
	if !ok {
		t.Fatalf("want LogLineMsg")
	}
	if len(got.Lines) != 1 || got.Lines[0] != "only" {
		t.Errorf("Lines = %q; want just the one available line", got.Lines)
	}
}

// TestLogPumpCarriesTheStreamEndWithItsLines: a channel receive is destructive, so a
// terminal event met *during* the drain cannot be put back — it rides along in End and
// the model applies it after the lines it followed. Dropping it would strand the stream
// (an EOF nobody ever sees) or lose an error.
func TestLogPumpCarriesTheStreamEndWithItsLines(t *testing.T) {
	ch := make(chan kube.LogEvent, 4)
	ch <- kube.LogEvent{Line: "a"}
	ch <- kube.LogEvent{Line: "b"}
	close(ch)

	got, ok := logPump(ch)().(LogLineMsg)
	if !ok {
		t.Fatalf("want LogLineMsg")
	}
	if len(got.Lines) != 2 {
		t.Errorf("Lines = %q; want both lines before the close", got.Lines)
	}
	if _, isClosed := got.End.(LogClosedMsg); !isClosed {
		t.Errorf("End = %#v; want LogClosedMsg", got.End)
	}
}

// TestLogPumpCarriesAMidDrainError is the same rule for the error edge: the lines that
// arrived before the failure still show, and the classified error follows them.
func TestLogPumpCarriesAMidDrainError(t *testing.T) {
	ch := make(chan kube.LogEvent, 4)
	ch <- kube.LogEvent{Line: "a"}
	ch <- kube.LogEvent{Err: errors.New("stream dropped")}

	got, ok := logPump(ch)().(LogLineMsg)
	if !ok {
		t.Fatalf("want LogLineMsg")
	}
	if len(got.Lines) != 1 || got.Lines[0] != "a" {
		t.Errorf("Lines = %q; want the line that preceded the error", got.Lines)
	}
	end, isErr := got.End.(ErrorMsg)
	if !isErr {
		t.Fatalf("End = %#v; want ErrorMsg", got.End)
	}
	if end.Context != "logs" || end.Err == nil {
		t.Errorf("End not classified as a logs error: %+v", end)
	}
}

// TestLogPumpFirstEventStillTerminates: an empty stream and an open failure are still
// delivered as the bare terminal messages, never as an empty batch — the model's
// "nothing was shown yet" open-failure path depends on it.
func TestLogPumpFirstEventStillTerminates(t *testing.T) {
	closed := make(chan kube.LogEvent)
	close(closed)
	if _, ok := logPump(closed)().(LogClosedMsg); !ok {
		t.Errorf("a closed channel should pump a bare LogClosedMsg")
	}

	failed := make(chan kube.LogEvent, 1)
	failed <- kube.LogEvent{Err: errors.New("boom")}
	if _, ok := logPump(failed)().(ErrorMsg); !ok {
		t.Errorf("a first-event error should pump a bare ErrorMsg")
	}
}
