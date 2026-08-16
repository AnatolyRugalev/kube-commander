package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/keylog"
	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
	"github.com/neuroplastio/kubecom/internal/tui/components/modal"
)

// recorder collects records in memory, standing in for the JSONL writer.
type recorder struct{ got []keylog.Record }

func (r *recorder) Record(rec keylog.Record) { r.got = append(r.got, rec) }

// keylogBrowsing is a model with a resource open, which is the state most of the
// surfaces a trace distinguishes require: `/` is inert until there is a table to
// narrow (openFilter checks hasCurrent), so a bare model would record the *absence*
// of a filter surface and prove nothing. Pass nil to build one without a recorder.
func keylogBrowsing(t *testing.T, rec KeyRecorder) Model {
	t.Helper()
	m := sizedWith(t, WithWatcher(&fakeWatcher{}), WithKeyRecorder(rec))
	next, _ := m.selectResource(gvrResource("pods"))
	m = next.(Model)
	m.table.ApplyEvent(resetEvent("api-1", "uid-1"))
	return m
}

// TestKeyLogRecordsTheResolvedAction is the ordinary case: a bound key is traced
// with the action it ran, so a trace can be read against the keymap.
func TestKeyLogRecordsTheResolvedAction(t *testing.T) {
	var rec recorder
	m := sizedWith(t, WithKeyRecorder(&rec))

	press(t, m, tea.Key{Code: 'j', Text: "j"})

	if len(rec.got) != 1 {
		t.Fatalf("want 1 record for one keypress, got %d: %+v", len(rec.got), rec.got)
	}
	got := rec.got[0]
	if got.Key != "j" {
		t.Errorf("Key = %q, want %q", got.Key, "j")
	}
	if got.Mode != string(keyModeBrowse) {
		t.Errorf("Mode = %q, want %q", got.Mode, keyModeBrowse)
	}
	if got.Action != "nav.down" {
		t.Errorf("Action = %q, want the resolved action %q", got.Action, "nav.down")
	}
	if got.Text || got.Pending {
		t.Errorf("browse press marked Text=%v Pending=%v, want both false", got.Text, got.Pending)
	}
	if got.Time.IsZero() {
		t.Error("Time is zero — a trace cannot measure hesitation without it")
	}
}

// TestKeyLogRecordsADeadEnd is the record the whole line exists to collect: a
// press bound to nothing. It must be distinguishable from a press that typed text
// and from one holding a sequence open, because only this one means the user
// reached for a key kubecom does not have (D268 pt 2).
func TestKeyLogRecordsADeadEnd(t *testing.T) {
	var rec recorder
	m := sizedWith(t, WithKeyRecorder(&rec))

	// `z` is bound to nothing in the default keymap.
	press(t, m, tea.Key{Code: 'z', Text: "z"})

	if len(rec.got) != 1 {
		t.Fatalf("want 1 record, got %d", len(rec.got))
	}
	got := rec.got[0]
	if got.Action != "" || got.Text || got.Pending {
		t.Fatalf("an unbound browse key must read as a dead end, got %+v", got)
	}
	if got.Mode != string(keyModeBrowse) {
		t.Errorf("Mode = %q, want %q", got.Mode, keyModeBrowse)
	}
}

// TestKeyLogMarksAPendingSequence keeps the first key of `gg` out of the dead-end
// count: it resolved to nothing *yet*, which is a different thing from resolving
// to nothing.
func TestKeyLogMarksAPendingSequence(t *testing.T) {
	var rec recorder
	m := sizedWith(t, WithKeyRecorder(&rec))

	press(t, m, tea.Key{Code: 'g', Text: "g"})

	if len(rec.got) != 1 {
		t.Fatalf("want 1 record, got %d", len(rec.got))
	}
	if got := rec.got[0]; !got.Pending || got.Action != "" {
		t.Fatalf("`g` must record as pending with no action, got %+v", got)
	}
}

// TestKeyLogMarksTextSurfaces checks the field that stops every typed character
// being counted as a dead end. With the filter open, `z` is a letter in a query,
// not a missing binding.
func TestKeyLogMarksTextSurfaces(t *testing.T) {
	var rec recorder
	m := keylogBrowsing(t, &rec)

	// `/` opens the filter; the next press is text into it.
	m, _ = press(t, m, tea.Key{Code: '/', Text: "/"})
	if !m.filter.Active() {
		t.Fatal("`/` did not open the filter — the rest of this test would prove nothing")
	}
	press(t, m, tea.Key{Code: 'z', Text: "z"})

	if len(rec.got) != 2 {
		t.Fatalf("want 2 records, got %d: %+v", len(rec.got), rec.got)
	}
	opened, typed := rec.got[0], rec.got[1]
	if opened.Action != "app.filter" || opened.Text {
		t.Errorf("the `/` that opened the filter must record as a browse action, got %+v", opened)
	}
	if typed.Mode != string(keyModeFilter) || !typed.Text || typed.Action != "" {
		t.Errorf("a key typed into the filter must record as text on the filter surface, got %+v", typed)
	}
}

// TestKeyLogIsInertWithoutARecorder is the default path: no --keylog, no
// recording, and nothing on the keypress path assumes a recorder exists.
func TestKeyLogIsInertWithoutARecorder(t *testing.T) {
	m := sized(t)
	if m.keyRecorder != nil {
		t.Fatal("a model built without WithKeyRecorder must not have one")
	}
	// The assertion is that this does not panic and the keymap still works.
	_, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Error("app.quit stopped working when no recorder was wired")
	}
}

// TestKeyLogRecordsResolvedConfirmAction is STORY-06e on the recorder: the confirm
// modal resolves its keys in a dedicated context (ConfirmAction, D132), so a
// handled accept or decline must be written to the trace with the action it ran —
// not as a blank press the analyzer would read as a dead end. The S02 trace
// reported `esc 2 modal-confirm` as unresolved when both declines had worked.
func TestKeyLogRecordsResolvedConfirmAction(t *testing.T) {
	var rec recorder
	d := &fakeDeleter{}
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithDeleter(d), WithKeyRecorder(&rec))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	m = next.(Model)

	// Open the delete confirm modal, then decline with esc.
	m, delCmd := press(t, m, deleteKey)
	if delCmd == nil {
		t.Fatal("the delete key should dispatch a row-action intent")
	}
	intent, ok := delCmd().(rowActionMsg)
	if !ok {
		t.Fatalf("delete key produced %T, want rowActionMsg", delCmd())
	}
	next, _ = m.Update(intent)
	m = next.(Model)
	if !m.modal.Active() {
		t.Fatal("the delete key should open the confirm modal")
	}
	_, cancelCmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cancelCmd == nil {
		t.Fatal("esc on the confirm modal should decline")
	}

	if len(rec.got) == 0 {
		t.Fatal("the recorder recorded nothing")
	}
	decline := rec.got[len(rec.got)-1]
	if decline.Mode != string(keyModeConfirm) {
		t.Errorf("esc on the confirm modal recorded Mode = %q, want %q", decline.Mode, keyModeConfirm)
	}
	if decline.Action != "confirm.decline" {
		t.Errorf("a handled esc must record confirm.decline, got %+v", decline)
	}
	if decline.Text || decline.Pending {
		t.Errorf("a handled confirm key must not read as text or pending, got %+v", decline)
	}

	// Now accept with y: the resolved action is confirm.accept.
	m, acceptCmd := press(t, m, tea.Key{Code: 'y', Text: "y"})
	if acceptCmd == nil {
		t.Fatal("y on the confirm modal should accept")
	}
	accept := rec.got[len(rec.got)-1]
	if accept.Action != "confirm.accept" {
		t.Errorf("a handled `y` must record confirm.accept, got %+v", accept)
	}

	// An unbound key on the confirm modal is still a dead end: the modal is not a
	// text surface, so an unhandled press there is a reach, and the record keeps
	// that signal.
	next, _ = m.Update(acceptCmd().(modal.ConfirmedMsg))
	m = next.(Model)
	m = openDeleteModal(t, m)
	press(t, m, tea.Key{Code: 'z', Text: "z"})
	dead := rec.got[len(rec.got)-1]
	if dead.Action != "" || dead.Text || dead.Pending {
		t.Errorf("an unhandled confirm key must read as a dead end, got %+v", dead)
	}
}

// TestKeyModeMatchesTheRoutingLadder guards the coupling keyMode's doc comment
// claims: the mode written into a trace is the surface that actually handled the
// press. It walks the three surfaces a hermetic model can open on its own.
func TestKeyModeMatchesTheRoutingLadder(t *testing.T) {
	m := keylogBrowsing(t, nil)
	if got := m.keyMode(); got != keyModeBrowse {
		t.Errorf("a freshly sized model is in %q, want %q", got, keyModeBrowse)
	}

	filtering, _ := press(t, m, tea.Key{Code: '/', Text: "/"})
	if got := filtering.keyMode(); got != keyModeFilter {
		t.Errorf("with the filter open keyMode = %q, want %q", got, keyModeFilter)
	}

	helping, _ := press(t, m, tea.Key{Code: '?', Text: "?"})
	if got := helping.keyMode(); got != keyModeBrowse {
		t.Errorf("the help overlay is not a key surface; keyMode = %q, want %q", got, keyModeBrowse)
	}
}
