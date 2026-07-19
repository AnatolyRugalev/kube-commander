package keymap

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// SequenceTimeout is how long the input layer should wait for the next key of a
// pending multi-key sequence before giving up on it (vim's `timeoutlen`). It is a
// package var so the TUI can schedule its timer from one source and tests can
// shrink it. The keymap package never runs a timer itself: the model schedules a
// tea.Tick when Input reports ResultPending and calls Timeout when it fires, so
// this stays pure, message-driven logic with no goroutines (principle 1).
var SequenceTimeout = 500 * time.Millisecond

// ResultKind describes what feeding a key to a Sequencer produced.
type ResultKind int

const (
	// ResultNone: the key (with any abandoned prefix) matched no binding; inert.
	ResultNone ResultKind = iota
	// ResultPending: the key began or extended a sequence that is a prefix of a
	// longer binding. Hold; wait for the next key or SequenceTimeout.
	ResultPending
	// ResultAction: a binding resolved; Action holds it and the buffer is cleared.
	ResultAction
)

// Result is the outcome of Sequencer.Input or Sequencer.Timeout.
type Result struct {
	Action Action
	Kind   ResultKind
}

// Sequencer resolves live keypresses to actions, buffering the keys of an
// in-flight multi-key sequence (D10 `gg`). It holds the only mutable input state;
// the model owns one Sequencer and drives it from the update loop, so there is no
// shared mutable state across goroutines. The base Keymap is immutable.
type Sequencer struct {
	km      *Keymap
	pending seq
}

// NewSequencer returns a Sequencer over an immutable keymap.
func NewSequencer(km *Keymap) *Sequencer { return &Sequencer{km: km} }

// Pending reports whether a sequence prefix is buffered awaiting resolution.
func (s *Sequencer) Pending() bool { return len(s.pending) > 0 }

// Reset drops any buffered prefix (e.g. on focus change or Esc).
func (s *Sequencer) Reset() { s.pending = nil }

// Input feeds one live keypress. A key that completes a binding with no longer
// binding extending it resolves immediately (ResultAction). A key that is (or
// extends into) a prefix of a longer binding buffers and returns ResultPending —
// even if it is also a complete binding, so its longer form stays reachable; the
// short form then fires on Timeout. A key that fits no binding abandons any
// buffered prefix and is retried on its own.
func (s *Sequencer) Input(key tea.Key) Result {
	cand := make(seq, len(s.pending)+1)
	copy(cand, s.pending)
	cand[len(s.pending)] = chordFromKey(key)

	if s.km.isPrefix(cand) {
		if a, ok := s.km.exact(cand); ok && !s.km.hasExtension(cand) {
			s.pending = nil
			return Result{Action: a, Kind: ResultAction}
		}
		s.pending = cand
		return Result{Kind: ResultPending}
	}
	// cand matches nothing. If a prefix was buffered, abandon it and reconsider
	// this key from a clean slate (it may itself start a new sequence).
	if len(s.pending) > 0 {
		s.pending = nil
		return s.Input(key)
	}
	return Result{Kind: ResultNone}
}

// Timeout resolves a buffered prefix after SequenceTimeout elapsed with no
// further key: if the buffered prefix is itself a complete binding it fires,
// otherwise it is dropped. Either way the buffer is cleared.
func (s *Sequencer) Timeout() Result {
	if len(s.pending) == 0 {
		return Result{Kind: ResultNone}
	}
	pending := s.pending
	s.pending = nil
	if a, ok := s.km.exact(pending); ok {
		return Result{Action: a, Kind: ResultAction}
	}
	return Result{Kind: ResultNone}
}
