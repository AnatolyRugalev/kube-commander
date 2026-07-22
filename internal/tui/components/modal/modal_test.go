package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// msgFrom runs a command (if any) and returns the message it produced, or nil.
func msgFrom(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestNewHiddenEmptyView(t *testing.T) {
	m := New(styles.Default())
	if m.Active() {
		t.Fatal("a new modal should start hidden")
	}
	m.SetSize(80, 24)
	if v := m.View(); v != "" {
		t.Fatalf("hidden modal View() = %q, want empty", v)
	}
	// An inactive modal ignores actions and emits nothing.
	if _, cmd := m.Update(keymap.ActionDrillIn); cmd != nil {
		t.Fatal("inactive modal should ignore ActionDrillIn")
	}
}

func TestConfirmRendersTitleAndMessage(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowConfirm("delete", "Delete pod", "Delete pod nginx-abc?")
	if !m.Active() {
		t.Fatal("ShowConfirm should make the modal active")
	}
	if m.Prompting() {
		t.Fatal("a confirm modal is not prompting")
	}
	if m.Kind() != "delete" {
		t.Fatalf("Kind() = %q, want delete", m.Kind())
	}
	v := m.View()
	for _, want := range []string{"Delete pod", "Delete pod nginx-abc?"} {
		if !strings.Contains(v, want) {
			t.Fatalf("View() missing %q; got:\n%s", want, v)
		}
	}
}

func TestConfirmAcceptEmitsConfirmedMsg(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowConfirm("delete", "Delete pod", "Delete pod nginx-abc?")

	_, cmd := m.Update(keymap.ActionDrillIn)
	msg := msgFrom(cmd)
	got, ok := msg.(ConfirmedMsg)
	if !ok {
		t.Fatalf("ActionDrillIn produced %T, want ConfirmedMsg", msg)
	}
	if got.Kind != "delete" {
		t.Fatalf("ConfirmedMsg.Kind = %q, want delete", got.Kind)
	}
	if got.Value != "" {
		t.Fatalf("confirm-mode ConfirmedMsg.Value = %q, want empty", got.Value)
	}
}

func TestConfirmDeclineEmitsCancelledMsg(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowConfirm("delete", "Delete pod", "Delete pod nginx-abc?")

	_, cmd := m.Update(keymap.ActionBack)
	msg := msgFrom(cmd)
	got, ok := msg.(CancelledMsg)
	if !ok {
		t.Fatalf("ActionBack produced %T, want CancelledMsg", msg)
	}
	if got.Kind != "delete" {
		t.Fatalf("CancelledMsg.Kind = %q, want delete", got.Kind)
	}
}

func TestPromptSeedsAndReportsPrompting(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	cmd := m.ShowPrompt("scale", "Scale deployment", "Replicas:", "3")
	if cmd == nil {
		t.Fatal("ShowPrompt should return the input focus cmd")
	}
	if !m.Active() || !m.Prompting() {
		t.Fatal("ShowPrompt should make the modal active and prompting")
	}
	if got := m.Value(); got != "3" {
		t.Fatalf("Value() = %q, want seeded 3", got)
	}
	v := m.View()
	for _, want := range []string{"Scale deployment", "Replicas:", "3"} {
		if !strings.Contains(v, want) {
			t.Fatalf("View() missing %q; got:\n%s", want, v)
		}
	}
}

func TestPromptCapturesTypedText(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowPrompt("scale", "Scale deployment", "Replicas:", "")

	for _, r := range "5" {
		key := tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
		m, _ = m.UpdatePrompt(key)
	}
	if got := m.Value(); got != "5" {
		t.Fatalf("after typing, Value() = %q, want 5", got)
	}

	_, cmd := m.Update(keymap.ActionDrillIn)
	got, ok := msgFrom(cmd).(ConfirmedMsg)
	if !ok {
		t.Fatalf("ActionDrillIn in prompt mode should emit ConfirmedMsg")
	}
	if got.Value != "5" {
		t.Fatalf("ConfirmedMsg.Value = %q, want the typed 5", got.Value)
	}
}

func TestUpdatePromptInertInConfirmMode(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowConfirm("delete", "Delete", "Sure?")

	key := tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})
	m, cmd := m.UpdatePrompt(key)
	if cmd != nil {
		t.Fatal("UpdatePrompt should be inert in confirm mode")
	}
	if got := m.Value(); got != "" {
		t.Fatalf("confirm-mode Value() = %q, want empty (typing swallowed)", got)
	}
}

func TestHideDismissesAndClearsInput(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowPrompt("scale", "Scale", "Replicas:", "3")
	m.Hide()
	if m.Active() {
		t.Fatal("Hide should deactivate the modal")
	}
	if v := m.View(); v != "" {
		t.Fatalf("hidden modal View() = %q, want empty", v)
	}
	// Reopening as a confirm starts clean (no stale prompt text).
	m.ShowConfirm("delete", "Delete", "Sure?")
	if got := m.Value(); got != "" {
		t.Fatalf("reopened confirm Value() = %q, want empty", got)
	}
}
