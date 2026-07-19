package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newTestModel() Model {
	return New(styles.Default())
}

func TestSeedNonEmptyFirstSelected(t *testing.T) {
	m := newTestModel()
	if len(m.items) == 0 {
		t.Fatal("seed menu is empty")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	sel, ok := m.Selected()
	if !ok {
		t.Fatal("Selected() returned ok=false on a seeded menu")
	}
	if sel.Resource.GVR.Resource != "namespaces" {
		t.Fatalf("first item = %q, want namespaces", sel.Resource.GVR.Resource)
	}
	// Every seed item must carry a title and a listable GVR.
	for _, it := range m.items {
		if it.Title == "" {
			t.Errorf("item %v has empty title", it.Resource.GVR)
		}
		if it.Resource.GVR.Resource == "" || it.Resource.GVR.Version == "" {
			t.Errorf("item %q has an incomplete GVR: %+v", it.Title, it.Resource.GVR)
		}
		if !it.Available {
			t.Errorf("seed item %q should be available", it.Title)
		}
	}
}

func TestNavigationClampsAndJumps(t *testing.T) {
	m := newTestModel()
	last := len(m.items) - 1

	// Up at the top stays at 0.
	m, _ = m.Update(keymap.ActionUp)
	if m.cursor != 0 {
		t.Fatalf("up at top: cursor = %d, want 0", m.cursor)
	}
	// Down moves.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != 1 {
		t.Fatalf("down: cursor = %d, want 1", m.cursor)
	}
	// Bottom jumps to last.
	m, _ = m.Update(keymap.ActionBottom)
	if m.cursor != last {
		t.Fatalf("bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Down at the bottom stays at last.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != last {
		t.Fatalf("down at bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Top jumps back to 0.
	m, _ = m.Update(keymap.ActionTop)
	if m.cursor != 0 {
		t.Fatalf("top: cursor = %d, want 0", m.cursor)
	}
}

func TestUnhandledActionIsIgnored(t *testing.T) {
	m := newTestModel()
	m, cmd := m.Update(keymap.ActionFilter)
	if cmd != nil {
		t.Error("unhandled action returned a non-nil command")
	}
	if m.cursor != 0 {
		t.Errorf("unhandled action moved the cursor to %d", m.cursor)
	}
}

func TestDrillInEmitsResourceSelected(t *testing.T) {
	m := newTestModel()
	// Move to the "pods" item and drill in.
	var target int
	for i, it := range m.items {
		if it.Resource.GVR.Resource == "pods" {
			target = i
			break
		}
	}
	m, _ = m.Update(keymap.ActionBottom)
	m, _ = m.Update(keymap.ActionTop)
	for i := 0; i < target; i++ {
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.cursor != target {
		t.Fatalf("cursor = %d, want %d (pods)", m.cursor, target)
	}

	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd == nil {
		t.Fatal("drillIn returned no command")
	}
	msg := cmd()
	sel, ok := msg.(ResourceSelectedMsg)
	if !ok {
		t.Fatalf("drillIn emitted %T, want ResourceSelectedMsg", msg)
	}
	if sel.Resource.GVR.Resource != "pods" {
		t.Fatalf("selected resource = %q, want pods", sel.Resource.GVR.Resource)
	}
}

func TestDrillInOnUnavailableEmitsNothing(t *testing.T) {
	m := newTestModel()
	m.items[m.cursor].Available = false
	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd != nil {
		t.Fatal("drilling into an unavailable item emitted a command")
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := newTestModel()
	// A short pane: 4 total rows → 2 item rows visible (border eats 2).
	m.SetSize(20, 4)
	if got := m.innerHeight(); got != 2 {
		t.Fatalf("innerHeight = %d, want 2", got)
	}
	// Jump to the bottom: the offset must scroll so the cursor is in view.
	m, _ = m.Update(keymap.ActionBottom)
	last := len(m.items) - 1
	if m.cursor < m.offset || m.cursor >= m.offset+m.innerHeight() {
		t.Fatalf("cursor %d not visible in window [%d,%d)", m.cursor, m.offset, m.offset+m.innerHeight())
	}
	if m.offset != last-m.innerHeight()+1 {
		t.Fatalf("offset = %d, want %d", m.offset, last-m.innerHeight()+1)
	}
	// Back to top scrolls the window back up.
	m, _ = m.Update(keymap.ActionTop)
	if m.offset != 0 {
		t.Fatalf("offset after top = %d, want 0", m.offset)
	}
}

func TestViewEmptyUntilSized(t *testing.T) {
	m := newTestModel()
	if v := m.View(); v != "" {
		t.Fatalf("View before sizing = %q, want empty", v)
	}
	m.SetSize(24, 12)
	if v := m.View(); v == "" {
		t.Fatal("View after sizing is empty")
	}
}

func TestViewRendersTitlesAndFocusChangesFrame(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 25) // tall enough to show every seed item
	blurred := m.View()
	if !strings.Contains(blurred, "Namespace") || !strings.Contains(blurred, "Pod") {
		t.Fatal("view is missing seed titles")
	}
	m.Focus()
	focused := m.View()
	if focused == blurred {
		t.Fatal("focused view is identical to blurred view (border should change)")
	}
}

// Ensure the emitted command type satisfies tea.Cmd (compile-time contract).
var _ tea.Cmd = func() tea.Msg { return ResourceSelectedMsg{} }
