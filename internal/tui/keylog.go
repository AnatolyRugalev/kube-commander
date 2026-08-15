package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/keylog"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// This file is STORY-02: the seam that lets a UX story be analysed after it was
// walked (D268 pt 2). The shell records one line per keypress — the key, the
// surface it was routed to, and the action it resolved to — and the recorder is
// nil unless the launcher passed `--keylog`, so an unasked-for run allocates
// nothing and writes nowhere.
//
// The recording point is the single `case tea.KeyPressMsg` arm in Update: every
// press in the program passes through it before the mode ladder splits them up,
// so there is exactly one place to keep correct. Recording happens *before* the
// press is handled, so a keypress that quits, panics or hangs is still in the
// trace that explains it.

// KeyRecorder receives one record per keypress. The interface is defined here
// rather than taken from the keylog package so a test can inject a slice.
type KeyRecorder interface {
	Record(keylog.Record)
}

// WithKeyRecorder wires the keystroke trace (`--keylog`). Unset means no
// recording at all: the shell checks for nil on every press, which is the cost of
// one nil comparison per keystroke in the overwhelmingly common case where nobody
// asked for a trace.
func WithKeyRecorder(r KeyRecorder) Option {
	return func(m *Model) {
		if r != nil {
			m.keyRecorder = r
		}
	}
}

// keyMode names the surface a keypress is about to be routed to. The values are
// written into the trace, so they are part of its format: an analyser groups by
// them, and renaming one silently makes old traces unjoinable with new ones.
type keyMode string

const (
	// keyModeBrowse is the root shell — the only surface that resolves keys
	// through the keymap, and therefore the only one where an empty action means
	// the user reached for something that does not exist.
	keyModeBrowse keyMode = "browse"
	// The rest handle their own keys. Each is a text surface except the confirm
	// modal, which is a y/n prompt.
	keyModeSearch     keyMode = "search"
	keyModeLogsFilter keyMode = "logs-filter"
	keyModePicker     keyMode = "picker"
	keyModeFilter     keyMode = "filter"
	keyModePrompt     keyMode = "modal-prompt"
	keyModeConfirm    keyMode = "modal-confirm"
)

// textModes are the surfaces where a press is (or may be) typed text, so an empty
// action is expected rather than a dead end. The confirm modal is deliberately
// absent: it takes single keys, not text, so an unhandled press there *is* a user
// reaching for something — worth seeing in a trace.
var textModes = map[keyMode]bool{
	keyModeSearch:     true,
	keyModeLogsFilter: true,
	keyModePicker:     true,
	keyModeFilter:     true,
	keyModePrompt:     true,
}

// keyMode reports which surface the next keypress will be routed to.
//
// It must test the same conditions in the same order as the ladder at the top of
// Update's KeyPressMsg arm — that ordering is behaviour (the search view captures
// everything while it is up; the logs grep captures text while it is open), and a
// trace that disagreed with it would mislabel exactly the presses a story cares
// about. The ladder calls this once and switches on the result, so the two cannot
// drift apart.
func (m Model) keyMode() keyMode {
	switch {
	case m.searchView.Active():
		return keyModeSearch
	case m.logsView.Filtering():
		return keyModeLogsFilter
	case m.activePicker() != nil:
		return keyModePicker
	case m.filter.Active():
		return keyModeFilter
	case m.modal.Prompting():
		return keyModePrompt
	case m.modal.Active():
		return keyModeConfirm
	}
	return keyModeBrowse
}

// recordKey writes one press to the trace, if one is being kept.
//
// kind carries what the sequencer made of the press on the browse surface;
// callers on every other surface pass keymap.ResultNone, since those surfaces
// never consult the sequencer.
func (m Model) recordKey(msg tea.KeyPressMsg, mode keyMode, action keymap.Action, kind keymap.ResultKind) {
	if m.keyRecorder == nil {
		return
	}
	m.keyRecorder.Record(keylog.Record{
		// Stamped here, at the press, rather than left for the sink: the gap between
		// two keys is the measurement ("where did they stop and think?"), so it must
		// not include however long a writer took to get to the record.
		Time:    time.Now(),
		Key:     msg.String(),
		Mode:    string(mode),
		Action:  string(action),
		Text:    textModes[mode],
		Pending: kind == keymap.ResultPending,
	})
}
