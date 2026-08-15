// Package keylog records what a user actually pressed, so a UX story can be
// analysed after it was walked rather than remembered (D268 pt 2).
//
// It is off unless asked for. The interesting output is not the keys that worked
// — it is a press that resolved to **no action**, which is somebody reaching for a
// key kubecom does not have. That is the signal the stories are run to collect,
// and it is invisible in a screen recording: the user presses `x`, nothing
// happens, and the video shows nothing happening.
//
// One JSON object per line (JSONL), so a trace is greppable, tailable while the
// UI is still running, and streamable into an analyser without loading it whole.
//
// # Privacy
//
// A trace of every keypress reconstructs everything typed: filter queries, search
// terms, namespace and resource names. It is a local file the user opted into by
// passing a flag, and the docs say so. Keys pressed inside an exec shell or the
// YAML editor never reach here — those suspend the TUI, so bubbletea is not
// reading the terminal while they run.
package keylog

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Record is one keypress, as one line of a trace.
//
// The zero value of every optional field is the common case, so they are all
// `omitempty`: a browse-mode press that resolved to an action writes four fields
// and nothing else.
type Record struct {
	// Time is when the press was handled, to millisecond precision — enough to
	// measure hesitation between keys, which is what a "where did they get stuck"
	// question actually asks.
	Time time.Time `json:"t"`
	// Key is the press in the same notation the keymap and `kubecom keys` use
	// ("j", "ctrl+s", "shift+tab"), so a trace can be read against the keymap doc
	// without a translation step.
	Key string `json:"key"`
	// Mode is the surface the press was routed to: "browse", "filter", "search",
	// "logs-filter", "picker", "modal-prompt" or "modal-confirm". Without it an
	// empty Action is ambiguous — a `j` typed into a filter box is text, the same
	// `j` in browse is navigation.
	Mode string `json:"mode"`
	// Action is the resolved action id, empty when the press resolved to none.
	// Only the browse surface resolves actions through the keymap; every other
	// mode handles its own keys, which is why Text exists.
	Action string `json:"action,omitempty"`
	// Text marks a surface that accepts typed text, where an empty Action is
	// expected rather than a dead end. An analyser counts dead ends as
	// `Action == "" && !Text && !Pending`.
	Text bool `json:"text,omitempty"`
	// Pending marks a press that began or extended a multi-key sequence (`g` of
	// `gg`). It resolved to nothing *yet*, so it is not a dead end — but a run of
	// them ending in no action is a sequence the user abandoned, which is worth
	// seeing.
	Pending bool `json:"pending,omitempty"`
}

// Writer appends records to a trace file.
//
// A trace is written for the user's benefit, so a failure to write one must never
// take down the session they are in the middle of: the first write error is
// reported once through onError and the Writer then goes quiet for the rest of the
// run. Losing a trace is a nuisance; losing the run it was recording is the thing
// the story was for.
type Writer struct {
	mu     sync.Mutex
	f      *os.File
	enc    *json.Encoder
	failed bool
	// onError is called at most once, with the write error that disabled the
	// Writer. The launcher points it at the log file.
	onError func(error)
	// now is the clock, swappable in tests.
	now func() time.Time
}

// Create opens path for appending and returns a Writer over it.
//
// Appending rather than truncating: a story is often walked twice (once to find
// the snag, once to check the fix), and a second run silently erasing the first
// trace is a bad surprise. Traces carry timestamps, so runs are separable.
func Create(path string, onError func(error)) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening key log %s: %w", path, err)
	}
	if onError == nil {
		onError = func(error) {}
	}
	return &Writer{f: f, enc: json.NewEncoder(f), onError: onError, now: time.Now}, nil
}

// Record appends one record. It stamps Time if the caller left it zero.
//
// Called from the Bubble Tea update loop, which is single-threaded, so the mutex
// guards against nothing today — it is here because a Writer is a file handle
// shared by reference, and the cost of a lock at human typing speed is not
// measurable.
func (w *Writer) Record(r Record) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failed {
		return
	}
	if r.Time.IsZero() {
		r.Time = w.now()
	}
	r.Time = r.Time.UTC().Truncate(time.Millisecond)
	if err := w.enc.Encode(r); err != nil {
		w.failed = true
		w.onError(err)
	}
}

// Close flushes and closes the trace file.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
