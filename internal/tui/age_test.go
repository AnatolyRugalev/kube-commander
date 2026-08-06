package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// agedReset is a watch RESET whose AGE cell says printed while the row's object
// was really created at created. That pairing is not a contrived one: it is
// exactly the state of a pane that listed a while ago and has had no delta since,
// because the server rendered AGE once, when it answered.
func agedReset(created time.Time, printed string) kube.WatchEvent {
	return kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}, {Name: "AGE", Type: "string"}},
		Rows: []kube.Row{
			{
				Cells:   []any{"pod-a", printed},
				Object:  kube.ObjectRef{Name: "pod-a", UID: "a"},
				Created: created,
			},
		},
	}
}

// openAgedTable opens a pods table holding one such row.
func openAgedTable(t *testing.T, created time.Time, printed string) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{agedReset(created, printed)}}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg))
	return next.(Model)
}

// TestInitStartsTheAgeClock proves the heartbeat is armed with the program rather
// than by whatever opens a table — the tick is unconditional by design (age.go),
// and a gated one is the thing that can be left off.
func TestInitStartsTheAgeClock(t *testing.T) {
	m := sized(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want the age tick armed")
	}
	if !producesAgeTick(t, cmd) {
		t.Fatal("Init's commands never produce an ageTickMsg")
	}
}

// TestAgeTickRefreshesTheColumnAndRearms is the feedback's complaint driven through
// the shell: the pane is left open, no delta arrives, and the AGE column has to move
// anyway. It also pins the chain — a tick that did not re-arm would refresh once and
// then be exactly as stale as before.
func TestAgeTickRefreshesTheColumnAndRearms(t *testing.T) {
	// A pod created three hours ago, listed when it was twelve seconds old.
	m := openAgedTable(t, time.Now().Add(-3*time.Hour-12*time.Second), "12s")
	if !strings.Contains(m.View().Content, "12s") {
		t.Fatalf("precondition: the server's own age should be on the first paint:\n%s", m.View().Content)
	}

	// The watch is quiet and the row is untouched; only the clock moves — which is
	// the reader's whole complaint, and the only input handleAgeTick has.
	next, tick := m.Update(ageTickMsg{})
	m = next.(Model)

	view := m.View().Content
	if strings.Contains(view, "12s") {
		t.Errorf("the age the server printed survived the tick:\n%s", view)
	}
	if !strings.Contains(view, "3h") {
		t.Errorf("want the age re-derived to 3h:\n%s", view)
	}
	if tick == nil || !producesAgeTick(t, tick) {
		t.Error("a tick must arm the next one, or the column stops again")
	}
}

// producesAgeTick reports whether cmd eventually yields an ageTickMsg. tea.Tick
// sleeps, so the message is read off the command rather than inferred from the
// closure — which is the point: what is pinned is that a tick actually arrives.
func producesAgeTick(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()
	_, ok := findMsg[ageTickMsg](initMsgs(t, cmd))
	return ok
}
