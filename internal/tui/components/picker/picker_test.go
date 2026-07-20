package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newTestModel(values ...string) Model {
	m := New(styles.Default(), "namespace")
	m.SetSize(80, 24)
	m.SetItems(values)
	return m
}

// msgFrom runs a command (if any) and returns the message it produced, or nil.
func msgFrom(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestNewHiddenEmptyView(t *testing.T) {
	m := New(styles.Default(), "namespace")
	if m.Active() {
		t.Fatal("a new picker should start hidden")
	}
	if m.Kind() != "namespace" {
		t.Fatalf("Kind() = %q, want namespace", m.Kind())
	}
	// Hidden picker renders nothing even once sized/seeded.
	m.SetSize(80, 24)
	m.SetItems([]string{"default", "kube-system"})
	if v := m.View(); v != "" {
		t.Fatalf("hidden picker View() = %q, want empty", v)
	}
}

func TestShowRendersTitleAndItems(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m.Show()
	if !m.Active() {
		t.Fatal("Show() should make the picker active")
	}
	v := m.View()
	if v == "" {
		t.Fatal("active, sized, seeded picker rendered empty")
	}
	for _, want := range []string{"Namespace", "default", "kube-system", "kube-public"} {
		if !strings.Contains(v, want) {
			t.Fatalf("View() missing %q; got:\n%s", want, v)
		}
	}
}

func TestFirstItemSelectedAfterSetItems(t *testing.T) {
	m := newTestModel("default", "kube-system")
	if got := m.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	v, ok := m.Selected()
	if !ok || v != "default" {
		t.Fatalf("Selected() = %q,%v; want default,true", v, ok)
	}
}

func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel("a", "b", "c")
	m.Show()

	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "b" {
		t.Fatalf("after down, Selected() = %q, want b", v)
	}
	m, _ = m.Update(keymap.ActionBottom)
	if v, _ := m.Selected(); v != "c" {
		t.Fatalf("after bottom, Selected() = %q, want c", v)
	}
	m, _ = m.Update(keymap.ActionTop)
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("after top, Selected() = %q, want a", v)
	}
	m, _ = m.Update(keymap.ActionUp) // already at top: clamped, stays
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("after up at top, Selected() = %q, want a", v)
	}
}

func TestDrillInEmitsSelectedMsg(t *testing.T) {
	m := newTestModel("default", "kube-system")
	m.Show()
	m, _ = m.Update(keymap.ActionDown)
	_, cmd := m.Update(keymap.ActionDrillIn)
	msg := msgFrom(cmd)
	sel, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want SelectedMsg", msg)
	}
	if sel.Kind != "namespace" || sel.Value != "kube-system" {
		t.Fatalf("SelectedMsg = %+v, want {namespace kube-system}", sel)
	}
}

func TestBackEmitsCancelledMsg(t *testing.T) {
	m := newTestModel("default")
	m.Show()
	_, cmd := m.Update(keymap.ActionBack)
	msg := msgFrom(cmd)
	c, ok := msg.(CancelledMsg)
	if !ok {
		t.Fatalf("back produced %T, want CancelledMsg", msg)
	}
	if c.Kind != "namespace" {
		t.Fatalf("CancelledMsg.Kind = %q, want namespace", c.Kind)
	}
}

func TestInactivePickerIgnoresActions(t *testing.T) {
	m := newTestModel("a", "b")
	// not shown
	m, cmd := m.Update(keymap.ActionDrillIn)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("inactive picker emitted %T on drill-in, want none", msg)
	}
	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("inactive picker moved cursor to %q, want a", v)
	}
}

func TestDrillInEmptyPickerNoMsg(t *testing.T) {
	m := New(styles.Default(), "namespace")
	m.SetSize(80, 24)
	m.Show()
	_, cmd := m.Update(keymap.ActionDrillIn)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("empty picker emitted %T on drill-in, want none", msg)
	}
}
