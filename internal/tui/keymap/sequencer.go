package keymap

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/neuroplastio/myelin"
	"github.com/neuroplastio/myelin/myelintea"

	"github.com/neuroplastio/kubecom/internal/keybind"
)

// SequenceTimeout is how long the input layer waits for the next key of a pending
// multi-key sequence before giving up on it (vim's `timeoutlen`). The keymap
// package never runs a timer itself: the model schedules a tea.Tick when Input
// reports ResultPending and calls Timeout when it fires, so this stays pure,
// message-driven logic with no goroutines (principle 1).
const SequenceTimeout = keybind.SequenceTimeout

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
// keyboard, buffering the keys of an in-flight multi-key sequence (`g g`): myelin's
// sequencer, whose state it keeps. It holds the only mutable input state; the
// model owns one Sequencer and drives it from the update loop, so there is no
// shared mutable state across goroutines. The Keymap is immutable.
type Sequencer struct {
	km *Keymap
	// st is myelin's state: the keys pending, the path they were typed in, and
	// their deadline.
	st myelin.Pending
}

// NewSequencer returns a Sequencer over an immutable keymap.
func NewSequencer(km *Keymap) *Sequencer { return &Sequencer{km: km} }

// Pending reports whether a sequence prefix is buffered awaiting resolution.
func (s *Sequencer) Pending() bool { return len(s.st.Seq) > 0 }

// Reset drops any buffered prefix (e.g. on focus change or Esc).
func (s *Sequencer) Reset() { s.st = myelin.Pending{} }

// Input feeds one live keypress typed in the context path at now. A key that
// completes a binding no longer binding as deep continues resolves at once
// (ResultAction). A key that a longer binding continues buffers and returns
// ResultPending — even if it is also a complete binding, so its longer form stays
// reachable; the short form then fires on Timeout. A key that fits no binding
// abandons any buffered prefix and is fed again on its own (myelin leaves that to
// the program).
func (s *Sequencer) Input(key tea.Key, p keybind.Path, now time.Time) Result {
	k, ok := myelintea.Key(key)
	if !ok {
		s.Reset()
		return Result{Kind: ResultNone}
	}
	had := s.Pending()
	steps := p.Steps()
	st, r := myelin.Feed(s.km.km, s.st, k, steps, now)
	if had && r.Run == nil && r.Deadline.IsZero() {
		// The pending keys and this one match nothing: this key may start a new
		// sequence itself.
		st, r = myelin.Feed(s.km.km, myelin.Pending{}, k, steps, now)
	}
	s.st = st
	return result(r)
}

// Timeout resolves a buffered prefix when its deadline has passed at now with no
// further key: if the buffered prefix is itself a complete binding it fires, in the
// path it was typed in; otherwise it is dropped. Before the deadline it does
// nothing, so a stale timer is harmless.
func (s *Sequencer) Timeout(now time.Time) Result {
	st, r := myelin.Expire(s.km.km, s.st, now)
	s.st = st
	return result(r)
}

// result is what myelin's Result means here.
func result(r myelin.Result) Result {
	switch {
	case r.Run != nil:
		return Result{Action: Action(r.Run.Action), Args: r.Run.Args, Kind: ResultAction}
	case !r.Deadline.IsZero():
		return Result{Kind: ResultPending}
	}
	return Result{Kind: ResultNone}
}
