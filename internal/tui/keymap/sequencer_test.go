package keymap

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyG() tea.Key { return tea.Key{Code: 'g', Text: "g"} }
func keyJ() tea.Key { return tea.Key{Code: 'j', Text: "j"} }

// gg resolves to nav.top: the first g pends (prefix of gg, g alone unbound), the
// second g completes the sequence.
func TestSequencerGG(t *testing.T) {
	s := NewSequencer(DefaultKeymap())

	if r := s.Input(keyG()); r.Kind != ResultPending {
		t.Fatalf("first g: kind %v, want pending", r.Kind)
	}
	if !s.Pending() {
		t.Fatal("expected pending after first g")
	}
	r := s.Input(keyG())
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
	r := s.Input(keyJ())
	if r.Kind != ResultAction || r.Action != ActionDown {
		t.Fatalf("j: %v/%q, want action/nav.down", r.Kind, r.Action)
	}
}

// A key that doesn't continue a pending sequence abandons the buffer and is
// retried on its own: g (pending) then j → nav.down.
func TestSequencerAbandonPrefix(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	if r := s.Input(keyG()); r.Kind != ResultPending {
		t.Fatalf("g: kind %v, want pending", r.Kind)
	}
	r := s.Input(keyJ())
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
	s.Input(keyG())
	if r := s.Timeout(); r.Kind != ResultNone {
		t.Fatalf("timeout: %v, want none", r.Kind)
	}
	if s.Pending() {
		t.Error("buffer should be cleared after timeout")
	}
	// Timeout with nothing pending is inert.
	if r := s.Timeout(); r.Kind != ResultNone {
		t.Errorf("empty timeout: %v, want none", r.Kind)
	}
}

// When a prefix is itself a complete binding *and* extends into a longer one, it
// pends (so the longer form stays reachable) and fires on timeout.
func TestSequencerTimeoutFiresShorter(t *testing.T) {
	// Bind app.filter to bare "g"; nav.top keeps its "gg" default. Now g both
	// completes (filter) and is a prefix of gg (top).
	km, _, err := DefaultKeymap().Merge(map[Action][]string{ActionFilter: {"g"}})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	s := NewSequencer(km)

	if r := s.Input(keyG()); r.Kind != ResultPending {
		t.Fatalf("g: %v, want pending (gg still reachable)", r.Kind)
	}
	if r := s.Timeout(); r.Kind != ResultAction || r.Action != ActionFilter {
		t.Fatalf("timeout: %v/%q, want action/app.filter", r.Kind, r.Action)
	}

	// The full sequence still resolves to nav.top.
	s.Input(keyG())
	if r := s.Input(keyG()); r.Kind != ResultAction || r.Action != ActionTop {
		t.Fatalf("gg: %v/%q, want action/nav.top", r.Kind, r.Action)
	}
}

// An unbound key with no buffer is inert.
func TestSequencerNoMatch(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	if r := s.Input(tea.Key{Code: 'z', Text: "z"}); r.Kind != ResultNone {
		t.Errorf("z: %v, want none", r.Kind)
	}
}

// Reset drops a buffered prefix.
func TestSequencerReset(t *testing.T) {
	s := NewSequencer(DefaultKeymap())
	s.Input(keyG())
	s.Reset()
	if s.Pending() {
		t.Error("Reset should clear the buffer")
	}
}

// The single-key Keymap.Action path still reaches nav.top via the "home"
// fallback even though its vim binding is the gg sequence.
func TestActionHomeFallback(t *testing.T) {
	km := DefaultKeymap()
	if a, ok := km.Action(tea.Key{Code: tea.KeyHome}); !ok || a != ActionTop {
		t.Errorf("home = %q,%v; want nav.top", a, ok)
	}
	// Bare g is not a single-key binding.
	if a, ok := km.Action(keyG()); ok {
		t.Errorf("g resolved to %q, want unbound (it is only a sequence prefix)", a)
	}
}
