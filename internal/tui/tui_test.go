package tui

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// sized returns the model after a WindowSizeMsg so View renders (it draws nothing
// until sized) and the sequencer/help are wired.
func sized(t *testing.T) Model {
	t.Helper()
	m, _ := New().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m.(Model)
}

// press feeds one live keypress through the root model's Update, as Bubble Tea
// would deliver it, and returns the next model + any command.
func press(t *testing.T, m Model, k tea.Key) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg(k))
	return next.(Model), cmd
}

// TestModelSmoke is the M0-05 teatest smoke test, carried onto the real root
// model: the app renders the browse layout and shuts down cleanly. It is the
// template M2 view tests build on. The model quits on the app.quit key, but
// teatest.Quit still works.
func TestModelSmoke(t *testing.T) {
	tm := teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(80, 24))

	// The seed resource menu renders its kinds in the left pane, so "Node" (a seed
	// item) is on screen once the browse layout is drawn. (The first item is the
	// selected row, whose background-filled cells the test terminal emulator writes
	// via a path a plain byte scan misses, so we key on an unselected row.)
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Node"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestQuitKey routes the default app.quit key (`q`) to tea.Quit through the
// keymap, proving input flows through the action registry, not a raw-key match.
func TestQuitKey(t *testing.T) {
	m := sized(t)
	_, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("app.quit produced no command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("app.quit did not resolve to tea.Quit, got %T", msg)
	}
}

// TestHelpToggle toggles the overlay via app.help (`?`) and closes it via
// nav.back (`esc`).
func TestHelpToggle(t *testing.T) {
	m := sized(t)
	if m.help.Visible() {
		t.Fatal("help should start hidden")
	}
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help did not open the overlay")
	}
	// esc (nav.back) closes it.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.help.Visible() {
		t.Fatal("nav.back did not close the overlay")
	}
}

// TestSequencePending checks the multi-key path: a lone `g` is a prefix of `gg`
// (nav.top), so it buffers and schedules a timeout tick; the tick then fires the
// buffered prefix only if it is itself a complete binding.
func TestSequencePending(t *testing.T) {
	m := sized(t)
	m, cmd := press(t, m, tea.Key{Code: 'g', Text: "g"})
	if !m.seq.Pending() {
		t.Fatal("`g` should leave a pending prefix (gg → nav.top)")
	}
	if cmd == nil {
		t.Fatal("a pending sequence must schedule a timeout tick")
	}
	tick, ok := cmd().(seqTimeoutMsg)
	if !ok {
		t.Fatalf("expected seqTimeoutMsg, got %T", cmd())
	}
	if tick.gen != m.seqGen {
		t.Fatalf("tick gen %d != model seqGen %d", tick.gen, m.seqGen)
	}
	// `g` alone is not a complete binding (only `gg`/`home` are), so the timeout
	// drops the buffer without acting.
	next, cmd := m.Update(tick)
	m = next.(Model)
	if m.seq.Pending() {
		t.Fatal("timeout should clear the pending buffer")
	}
	if cmd != nil {
		t.Fatal("`g`-only timeout should not fire an action")
	}
}

// TestStaleTimeoutIgnored proves a superseded timeout tick is dropped: pressing
// `g` twice completes `gg` (nav.top) and clears the buffer; a leftover tick from
// the first `g` must not act.
func TestStaleTimeoutIgnored(t *testing.T) {
	m := sized(t)
	m, cmd1 := press(t, m, tea.Key{Code: 'g', Text: "g"})
	stale := cmd1().(seqTimeoutMsg)
	// Second `g` completes gg → nav.top (ResultAction, no new pending/tick).
	m, _ = press(t, m, tea.Key{Code: 'g', Text: "g"})
	if m.seq.Pending() {
		t.Fatal("gg should have resolved and cleared the buffer")
	}
	// Delivering the stale tick from the first `g` must be a no-op.
	_, cmd := m.Update(stale)
	if cmd != nil {
		t.Fatal("stale timeout tick should be ignored")
	}
}

// TestViewEmptyUntilSized guards that the model draws nothing before it knows the
// terminal size (avoids sizing panes to a zero canvas).
func TestViewEmptyUntilSized(t *testing.T) {
	if got := New().View().Content; got != "" {
		t.Fatalf("unsized View should be empty, got %q", got)
	}
}

// TestViewRendersWhenSized sanity-checks the sized shell renders the browse layout
// (a menu kind in the left pane) and a non-empty keymap-derived short-help hint.
func TestViewRendersWhenSized(t *testing.T) {
	m := sized(t)
	if m.help.ShortHelpView() == "" {
		t.Fatal("short-help hint should be generated from the registry")
	}
	if !bytes.Contains([]byte(m.View().Content), []byte("Node")) {
		t.Fatalf("sized View missing the resource menu: %q", m.View().Content)
	}
}

// TestFocusStartsOnMenu proves the left (menu) pane holds focus initially — the
// user picks a resource before drilling into its table.
func TestFocusStartsOnMenu(t *testing.T) {
	m := sized(t)
	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}
	if m.table.Focused() {
		t.Fatal("table should not start focused")
	}
}

// TestFocusSwitch drives the horizontal focus switch: nav.right moves focus from
// the menu to the table, and nav.left (with the empty table at its left edge, so
// nothing to scroll) moves it back to the menu — the HOffset arbitration (D60).
func TestFocusSwitch(t *testing.T) {
	m := sized(t)

	// nav.right (`l`) → focus the table.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if !m.table.Focused() || m.menu.Focused() {
		t.Fatal("nav.right should move focus to the table")
	}

	// nav.left (`h`) at the table's left edge → focus back to the menu.
	if m.table.HOffset() != 0 {
		t.Fatalf("empty table should be at HOffset 0, got %d", m.table.HOffset())
	}
	m, _ = press(t, m, tea.Key{Code: 'h', Text: "h"})
	if !m.menu.Focused() || m.table.Focused() {
		t.Fatal("nav.left at the table's left edge should move focus to the menu")
	}
}

// TestNavRoutedToFocusedPane proves a nav action reaches only the focused pane:
// nav.down moves the menu cursor while the menu is focused, and does not move it
// once focus is on the table.
func TestNavRoutedToFocusedPane(t *testing.T) {
	m := sized(t)
	if m.menu.Cursor() != 0 {
		t.Fatalf("menu should start at cursor 0, got %d", m.menu.Cursor())
	}

	// Menu focused: nav.down (`j`) advances the menu cursor.
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 1 {
		t.Fatalf("nav.down should advance the focused menu cursor, got %d", m.menu.Cursor())
	}

	// Focus the table; nav.down no longer moves the menu.
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	m, _ = press(t, m, tea.Key{Code: 'j', Text: "j"})
	if m.menu.Cursor() != 1 {
		t.Fatalf("nav.down should not move the blurred menu, got %d", m.menu.Cursor())
	}
}

// TestHelpSwallowsNav proves the open help overlay swallows navigation: focus does
// not switch while help is visible.
func TestHelpSwallowsNav(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("app.help should open the overlay")
	}
	m, _ = press(t, m, tea.Key{Code: 'l', Text: "l"})
	if m.table.Focused() {
		t.Fatal("nav.right should not switch panes while help is open")
	}
}
