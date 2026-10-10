package keymap

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// t0 is when the keys are typed, and expired when a sequence typed then has
// timed out: the sequencer reads no clock.
var (
	t0      = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	expired = t0.Add(SequenceTimeout)
)

func keyG() tea.Key { return tea.Key{Code: 'g', Text: "g"} }
func keyJ() tea.Key { return tea.Key{Code: 'j', Text: "j"} }

// gg resolves to nav.top: the first g pends (prefix of g g, g alone unbound), the
// second g completes the sequence.
func TestSequencerGG(t *testing.T) {
	s := NewSequencer(DefaultKeymap())

	if r := s.Input(keyG(), menuPath, t0); r.Kind != ResultPending {
		t.Fatalf("first g: kind %v, want pending", r.Kind)
	}
	if !s.Pending() {
		t.Fatal("expected pending after first g")
	}
	r := s.Input(keyG(), menuPath, t0)
	if r.Kind != ResultAction || r.Action != ActionTop {
		t.Fatalf("second g: %v/%q, want action/nav.top", r.Kind, r.Action)
	}
	if s.Pending() {
		t.Error("buffer should be cleared after resolving")
	}
}

// A single fully-bound key with no longer extension resolves immediately.
func TestSequencerImmediate(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	r := s.Input(keyJ(), menuPath, t0)
	if r.Kind != ResultAction || r.Action != ActionDown {
		t.Fatalf("j: %v/%q, want action/nav.down", r.Kind, r.Action)
	}
}

// A key that doesn't continue a pending sequence abandons the buffer and is
// retried on its own: g (pending) then j → nav.down.
func TestSequencerAbandonPrefix(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	if r := s.Input(keyG(), menuPath, t0); r.Kind != ResultPending {
		t.Fatalf("g: kind %v, want pending", r.Kind)
	}
	r := s.Input(keyJ(), menuPath, t0)
	if r.Kind != ResultAction || r.Action != ActionDown {
		t.Fatalf("j after g: %v/%q, want action/nav.down", r.Kind, r.Action)
	}
	if s.Pending() {
		t.Error("buffer should be cleared")
	}
}

// A pending prefix that is not itself a complete binding resolves to nothing on
// timeout (bare g has no standalone action in the defaults).
func TestSequencerTimeoutDrops(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	s.Input(keyG(), menuPath, t0)
	if r := s.Timeout(expired); r.Kind != ResultNone {
		t.Fatalf("timeout: %v, want none", r.Kind)
	}
	if s.Pending() {
		t.Error("buffer should be cleared after timeout")
	}
	// Timeout with nothing pending is inert.
	if r := s.Timeout(expired); r.Kind != ResultNone {
		t.Errorf("empty timeout: %v, want none", r.Kind)
	}
}

// When a prefix is itself a complete binding *and* extends into a longer one, it
// pends (so the longer form stays reachable) and fires on timeout.
func TestSequencerTimeoutFiresShorter(t *testing.T) {
	// Bind app.filter to bare "g"; nav.top keeps its "g g" default. Now g both
	// completes (filter) and is a prefix of g g (top).
	s := NewSequencer(withUser(t, `bind "g" "app.filter"`, ""))

	if r := s.Input(keyG(), menuPath, t0); r.Kind != ResultPending {
		t.Fatalf("g: %v, want pending (gg still reachable)", r.Kind)
	}
	if r := s.Timeout(expired); r.Kind != ResultAction || r.Action != ActionFilter {
		t.Fatalf("timeout: %v/%q, want action/app.filter", r.Kind, r.Action)
	}

	// The full sequence still resolves to nav.top.
	s.Input(keyG(), menuPath, t0)
	if r := s.Input(keyG(), menuPath, t0); r.Kind != ResultAction || r.Action != ActionTop {
		t.Fatalf("gg: %v/%q, want action/nav.top", r.Kind, r.Action)
	}
}

// A timer that fires before the pending keys' deadline — a stale one — does
// nothing: they stay pending.
func TestSequencerEarlyTimeoutWaits(t *testing.T) {
	s := NewSequencer(withUser(t, `bind "g" "app.filter"`, ""))
	s.Input(keyG(), menuPath, t0)
	if r := s.Timeout(expired.Add(-time.Millisecond)); r.Kind != ResultNone || !s.Pending() {
		t.Fatalf("an early timeout: %v, pending %v", r.Kind, s.Pending())
	}
	if r := s.Timeout(expired); r.Action != ActionFilter {
		t.Fatalf("the timeout: %v/%q, want app.filter", r.Kind, r.Action)
	}
}

// An unbound key with no buffer is inert.
func TestSequencerNoMatch(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	if r := s.Input(tea.Key{Code: 'z', Text: "z"}, menuPath, t0); r.Kind != ResultNone {
		t.Errorf("z: %v, want none", r.Kind)
	}
}

// Reset drops a buffered prefix.
func TestSequencerReset(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	s.Input(keyG(), menuPath, t0)
	s.Reset()
	if s.Pending() {
		t.Error("Reset should clear the buffer")
	}
}

// A pending prefix resolves on timeout in the context it was typed in, not in
// whatever holds the keyboard when the timer fires.
func TestSequencerTimeoutKeepsItsContext(t *testing.T) {
	s := NewSequencer(withUser(t, `context "confirm" { bind "g" "confirm.accept"; bind "g x" "confirm.decline"; }`, ""))
	if r := s.Input(keyG(), confirmPath, t0); r.Kind != ResultPending {
		t.Fatalf("g in the confirm modal: %v, want pending", r.Kind)
	}
	if r := s.Timeout(expired); r.Action != ActionConfirmAccept {
		t.Fatalf("timeout: %v/%q, want confirm.accept", r.Kind, r.Action)
	}
}

// A binding's values come out with its action (D303 pt 2).
func TestSequencerCarriesValues(t *testing.T) {
	s := NewSequencer(withUser(t, `bind "g p" "resources.switch" "pods"`, ""))
	s.Input(keyG(), menuPath, t0)
	r := s.Input(tea.Key{Code: 'p', Text: "p"}, menuPath, t0)
	if r.Action != ActionResources || len(r.Args) != 1 || r.Args[0] != "pods" {
		t.Fatalf("g p: %+v, want resources.switch pods", r)
	}
}
