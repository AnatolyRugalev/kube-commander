package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/elide"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// longMessage is a message that wraps well past any box the geometry admits —
// the shape of a confirm whose question quotes a server error or a remediation
// command, which is where the D220 clipping was found.
var longMessage = strings.Repeat("this question is long enough to wrap several times. ", 12)

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

// A long message used to make the box as tall as its text wrapped — 19 rows for
// this one, on any screen — and overlayCenter clips the excess against a fixed
// canvas, so the box lost its bottom border. The rendered height must never
// exceed what modalSize computed, on a roomy screen or a cramped one (D220).
func TestBoxNeverOutgrowsItsComputedHeight(t *testing.T) {
	for _, screen := range [][2]int{{80, 24}, {100, 40}, {80, 12}, {80, 8}, {60, 5}} {
		for _, prompt := range []bool{false, true} {
			m := New(styles.Default())
			m.SetSize(screen[0], screen[1])
			if prompt {
				m.ShowPrompt("scale", "Scale deployment", longMessage, "3")
			} else {
				m.ShowConfirm("delete", "Delete pod", longMessage)
			}
			_, want := m.modalSize()
			if got := lipgloss.Height(m.View()); got > want {
				t.Fatalf("screen %dx%d prompt=%v: box is %d rows, modalSize says %d",
					screen[0], screen[1], prompt, got, want)
			}
		}
	}
}

// onCanvas is what overlayCenter's fixed canvas keeps of a box: it composites at
// y = max(0, (height-boxHeight)/2), so a box taller than the body starts at row 0
// and everything past row height-1 is dropped. Reading the box through this is the
// difference between "View emitted the input line" (true even unclamped) and "the
// reader can see it", which is the claim D220 is about.
func onCanvas(box string, height int) string {
	lines := strings.Split(box, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// The message is what gets elided, never the field the modal is asking the reader
// to fill in: the input line is rendered under the message, so an unbounded box
// clipped it away and left a prompt with no visible input (D220).
func TestPromptKeepsItsInputWhenTheMessageIsTooTall(t *testing.T) {
	const screenH = 12
	m := New(styles.Default())
	m.SetSize(80, screenH)
	m.ShowPrompt("scale", "Scale deployment", longMessage, "7")
	v := onCanvas(m.View(), screenH)
	// The input renders its prompt and value in separate styles, so the box is read
	// with the escapes stripped — what a reader sees, not what lipgloss emitted.
	plain := ansi.Strip(v)
	if !strings.Contains(plain, "Scale deployment") {
		t.Fatalf("View() dropped the title; got:\n%s", v)
	}
	if !strings.Contains(plain, "> 7") {
		t.Fatalf("View() dropped the input line; got:\n%s", v)
	}
	if !strings.Contains(plain, elide.Marker) {
		t.Fatalf("View() elided the message without saying so; got:\n%s", v)
	}
	if !strings.Contains(plain, "╰") {
		t.Fatalf("the box lost its bottom border to the canvas; got:\n%s", v)
	}
}

// A message that fits is untouched — the marker is evidence that content was
// dropped, so it must not appear on a modal that dropped nothing.
func TestShortMessageIsNotMarkedTruncated(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(80, 24)
	m.ShowConfirm("delete", "Delete pod", "Delete pod nginx-abc?")
	if v := m.View(); strings.Contains(v, elide.Marker) {
		t.Fatalf("a one-line message was marked truncated; got:\n%s", v)
	}
}

// TestMessageCannotSteerTheTerminal is AUTH-06 at the modal: the re-authenticate
// confirm quotes the invocation the kubeconfig's exec stanza spells, so the box
// asking the question renders a string kubecom did not write. A control character
// in it costs zero cells by every measure the box has — the width clamp, the
// height elision, the canvas — and would repaint over the border of the box, which
// is the one surface where what is being agreed to must be legible.
func TestMessageCannotSteerTheTerminal(t *testing.T) {
	for name, message := range map[string]string{
		"carriage return": "Run this now, in this terminal?\n  aws sso login\rrm -rf /",
		"erase display":   "Run this now?\n  \x1b[2J\x1b[Haws sso login --profile prod",
		"sgr color":       "Run this now?\n  \x1b[31maws sso login\x1b[0m",
		"backspace":       "Run this now?\n  aws sso login\b\b\b\b\blogout",
		"tabs":            "Run this now?\n  aws\tsso\tlogin",
	} {
		m := New(styles.Default())
		m.SetSize(80, 24)
		m.ShowConfirm("reauth", "Re-authenticate", message)

		v := m.View()
		for i, l := range strings.Split(ansi.Strip(v), "\n") {
			for _, r := range l {
				if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
					t.Errorf("%s: box line %d carries control %q: %q", name, i, r, l)
				}
			}
		}
		w := lipgloss.Width(v)
		for i, l := range strings.Split(v, "\n") {
			if got := lipgloss.Width(l); got != w {
				t.Errorf("%s: box line %d is %d cells wide, the box is %d: %q", name, i, got, w, l)
			}
		}
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
