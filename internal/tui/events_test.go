package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
)

// fakeEventLister is a hermetic EventLister: it returns preset event rows (or an
// error) and records the ref it was asked for, so the events viewer's open/fetch
// path is exercised without a client.
type fakeEventLister struct {
	tbl    *kube.Table
	err    error
	calls  int
	gotRef kube.ObjectRef
}

func (f *fakeEventLister) Events(_ context.Context, ref kube.ObjectRef) (*kube.Table, error) {
	f.calls++
	f.gotRef = ref
	return f.tbl, f.err
}

// eventsTable is a minimal server-printed events Table shaped like a real
// `kubectl get events` answer (LAST SEEN, TYPE, REASON, OBJECT, MESSAGE).
func eventsTable(rows ...kube.Row) *kube.Table {
	cols := []kube.Column{
		{Name: "LAST SEEN"},
		{Name: "TYPE"},
		{Name: "REASON"},
		{Name: "OBJECT"},
		{Name: "MESSAGE"},
	}
	return &kube.Table{Columns: cols, Rows: rows}
}

// eventsViewerModel drills into a pods table (Kind Pod) with a live row and the
// given lister wired, so a viewer test has a concrete selected row and a fetch
// seam — the events twin of describeViewerModel.
func eventsViewerModel(t *testing.T, lister EventLister) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithEventLister(lister))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// eventsKey is the default res.events direct key (`E`).
var eventsKey = tea.Key{Code: 'E', Text: "E"}

// TestEventsViewerOpensAndShowsContent drives the whole STORY-06f path: the `E`
// key dispatches the events intent, handling it opens the viewer and issues the
// Events list against the selected row, and the rendered, aligned table lands in
// the viewer content.
func TestEventsViewerOpensAndShowsContent(t *testing.T) {
	l := &fakeEventLister{tbl: eventsTable(
		kube.Row{Cells: []any{"3m", "Warning", "BackOff", "pod/web-1", "Back-off restarting failed container"}},
	)}
	m := eventsViewerModel(t, l)

	_, cmd := press(t, m, eventsKey)
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.events produced %T, want rowActionMsg", cmd())
	}
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("handling the events intent should open the viewer")
	}
	if fetchCmd == nil {
		t.Fatal("opening the events viewer should issue an Events list command")
	}
	loaded, ok := fetchCmd().(eventsLoadedMsg)
	if !ok {
		t.Fatalf("the fetch produced %T, want eventsLoadedMsg", fetchCmd())
	}
	if l.calls != 1 {
		t.Fatalf("Events called %d times, want 1", l.calls)
	}
	if l.gotRef.Name == "" {
		t.Error("Events should be addressed to the selected row's object")
	}

	next, _ = m.Update(loaded)
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the viewer should stay open once its content lands")
	}
	view := m.View().Content
	for _, want := range []string{"LAST SEEN", "TYPE", "REASON", "BackOff", "pod/web-1"} {
		if !strings.Contains(view, want) {
			t.Errorf("viewer should show %q in the rendered events list:\n%s", want, view)
		}
	}
}

// TestEventsViewerEmptyShowsNotice proves an object with no events is a statement
// in the viewer, not a blank box.
func TestEventsViewerEmptyShowsNotice(t *testing.T) {
	l := &fakeEventLister{tbl: eventsTable()}
	m := eventsViewerModel(t, l)

	_, cmd := press(t, m, eventsKey)
	intent := cmd().(rowActionMsg)
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(eventsLoadedMsg))
	m = next.(Model)

	if !m.viewer.Active() {
		t.Fatal("the viewer should stay open over an empty list")
	}
	if !strings.Contains(m.View().Content, "(no events)") {
		t.Errorf("an empty events list should render the notice:\n%s", m.View().Content)
	}
}

// TestEventsViewerErrorDegrades proves a list error closes the viewer and lands a
// status-bar toast (D74) rather than leaving an empty box.
func TestEventsViewerErrorDegrades(t *testing.T) {
	l := &fakeEventLister{err: errors.New("forbidden")}
	m := eventsViewerModel(t, l)

	_, cmd := press(t, m, eventsKey)
	intent := cmd().(rowActionMsg)
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	next, _ = m.Update(fetchCmd().(eventsLoadedMsg))
	m = next.(Model)

	if m.viewer.Active() {
		t.Error("a failed events list should close the viewer")
	}
	if !strings.Contains(m.View().Content, "forbidden") {
		t.Errorf("the failure should surface as a status-bar toast:\n%s", m.View().Content)
	}
}

// TestEventsViewerInertWithoutLister proves an unwired model is events-viewer-
// inert: the key still resolves to the intent, but with no lister the viewer
// never opens and nothing is fetched.
func TestEventsViewerInertWithoutLister(t *testing.T) {
	m := describeViewerModel(t, &fakeDescriber{text: "Name: web-1\n"})

	_, cmd := press(t, m, eventsKey)
	if cmd == nil {
		t.Fatal("res.events should still dispatch a row-action intent")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok || intent.Action != rowActionEvents {
		t.Fatalf("res.events produced %#v, want a rowActionEvents intent", cmd())
	}
	next, fetchCmd := m.Update(intent)
	m = next.(Model)
	if m.viewer.Active() {
		t.Error("without a lister the events viewer should not open")
	}
	if fetchCmd != nil {
		t.Errorf("without a lister no fetch should be issued, got a command")
	}
}

// TestRenderEventsTable pins the text layout: the header and rows pad to the
// widest cell per column, the last (MESSAGE) column flows unclipped, and an
// empty table renders the notice.
func TestRenderEventsTable(t *testing.T) {
	tbl := eventsTable(
		kube.Row{Cells: []any{"3m", "Warning", "BackOff", "pod/web-1", "Back-off restarting failed container"}},
		kube.Row{Cells: []any{"38s", "Normal", "Scheduled", "pod/web-1", "Successfully assigned web/web-1 to node-1"}},
	)
	got := renderEventsTable(tbl)

	if !strings.Contains(got, "LAST SEEN") {
		t.Errorf("the header should be present:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines:\n%s", len(lines), got)
	}

	// Every column but the last pads to its widest cell, so the MESSAGE column —
	// the only one with different content per row — starts at the same offset in
	// both data rows, and the same-named OBJECT cells line up across the header.
	msgCol := strings.Index(lines[1], "Back-off restarting failed container")
	if msgCol <= 0 || strings.Index(lines[2], "Successfully assigned web/web-1 to node-1") != msgCol {
		t.Errorf("the flowing MESSAGE column should start at the same offset in every row:\n%s", got)
	}
	for i, line := range lines[1:] {
		if got := strings.Index(line, "pod/web-1"); got == -1 {
			t.Errorf("row %d should show the OBJECT cell:\n%s", i+1, line)
			continue
		} else if i > 0 {
			prev := strings.Index(lines[i], "pod/web-1")
			if prev != got {
				t.Errorf("row %d's OBJECT cell starts at %d, row %d's at %d — columns must align:\n%s",
					i+1, got, i, prev, line)
			}
		}
	}

	if empty := renderEventsTable(eventsTable()); empty != "(no events)" {
		t.Errorf("an empty table should render the notice, got %q", empty)
	}
}
