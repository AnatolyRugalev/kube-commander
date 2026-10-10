package keymap

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/keybind"
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
	// ResultPending: the key began or extended a sequence that a longer binding
	// continues. Hold; wait for the next key or SequenceTimeout.
	ResultPending
	// ResultAction: a binding resolved; Action holds it and the buffer is cleared.
	ResultAction
)

// Result is the outcome of Sequencer.Input or Sequencer.Timeout: the action, and
// the values its binding gives it (D303 pt 2).
type Result struct {
	Action Action
	Args   []string
	Kind   ResultKind
}

// Sequencer resolves live keypresses to actions in the context that holds the
// keyboard, buffering the keys of an in-flight multi-key sequence (`g g`). It holds
// the only mutable input state; the model owns one Sequencer and drives it from the
// update loop, so there is no shared mutable state across goroutines. The Keymap is
// immutable.
type Sequencer struct {
	km      *Keymap
	pending []keybind.Key
	// path is the context the pending sequence was typed in, which Timeout
	// resolves it against.
	path keybind.Path
}

// NewSequencer returns a Sequencer over an immutable keymap.
func NewSequencer(km *Keymap) *Sequencer { return &Sequencer{km: km} }

// Pending reports whether a sequence prefix is buffered awaiting resolution.
func (s *Sequencer) Pending() bool { return len(s.pending) > 0 }

// Reset drops any buffered prefix (e.g. on focus change or Esc).
func (s *Sequencer) Reset() { s.pending = nil }

// Input feeds one live keypress typed in the context path. A key that completes a
// binding no longer binding as deep continues resolves at once (ResultAction). A
// key that a longer binding continues buffers and returns ResultPending — even if
// it is also a complete binding, so its longer form stays reachable; the short
// form then fires on Timeout. A key that fits no binding abandons any buffered
// prefix and is retried on its own.
func (s *Sequencer) Input(key tea.Key, p keybind.Path) Result {
	cand := append(append([]keybind.Key(nil), s.pending...), KeyOf(key))
	m := s.km.Match(cand, p)
	switch {
	case m.Prefix:
		s.pending, s.path = cand, p
		return Result{Kind: ResultPending}
	case m.Action != "":
		s.pending = nil
		return Result{Action: m.Action, Args: m.Args, Kind: ResultAction}
	case len(s.pending) > 0:
		// cand matches nothing: abandon the prefix and reconsider this key from a
		// clean slate (it may itself start a new sequence).
		s.pending = nil
		return s.Input(key, p)
	}
	return Result{Kind: ResultNone}
}

// Timeout resolves a buffered prefix after SequenceTimeout elapsed with no further
// key: if the buffered prefix is itself a complete binding it fires, otherwise it
// is dropped. Either way the buffer is cleared.
func (s *Sequencer) Timeout() Result {
	if len(s.pending) == 0 {
		return Result{Kind: ResultNone}
	}
	pending := s.pending
	s.pending = nil
	if m := s.km.Match(pending, s.path); m.Action != "" {
		return Result{Action: m.Action, Args: m.Args, Kind: ResultAction}
	}
	return Result{Kind: ResultNone}
}
