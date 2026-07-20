package statusbar

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newBar() Model { return New(styles.Default()) }

func TestLeftSegmentJoinsSetPieces(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetNamespace("kube-system")
	got := m.leftSegment()
	if !strings.Contains(got, "prod") || !strings.Contains(got, "kube-system") {
		t.Fatalf("leftSegment() = %q, want context and namespace", got)
	}
	if !strings.Contains(got, separator) {
		t.Errorf("leftSegment() = %q, want a separator between pieces", got)
	}
}

func TestLeftSegmentSkipsEmptyPieces(t *testing.T) {
	m := newBar()
	m.SetContext("prod") // namespace left empty
	got := m.leftSegment()
	if got != "prod" {
		t.Errorf("leftSegment() with empty namespace = %q, want %q (no dangling separator)", got, "prod")
	}

	m2 := newBar() // nothing set
	if got := m2.leftSegment(); got != "" {
		t.Errorf("leftSegment() with nothing set = %q, want empty", got)
	}
}

func TestStartStopDiscovery(t *testing.T) {
	m := newBar()
	if m.Discovering() {
		t.Fatal("new bar should not be discovering")
	}
	cmd := m.StartDiscovery()
	if !m.Discovering() {
		t.Error("StartDiscovery should set discovering true")
	}
	if cmd == nil {
		t.Error("StartDiscovery should return a spinner tick Cmd")
	}
	m.StopDiscovery()
	if m.Discovering() {
		t.Error("StopDiscovery should clear discovering")
	}
}

func TestDiscoveringShowsInLeftSegment(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	if strings.Contains(m.leftSegment(), strings.TrimSpace(discoveringLabel)) {
		t.Error("idle bar should not show the discovering label")
	}
	_ = m.StartDiscovery()
	got := m.leftSegment()
	if !strings.Contains(got, strings.TrimSpace(discoveringLabel)) {
		t.Errorf("discovering leftSegment() = %q, want the discovering label", got)
	}
}

func TestUpdateAdvancesSpinnerOnlyWhileDiscovering(t *testing.T) {
	m := newBar()
	// A tick while idle is dropped and reschedules nothing (spinner stops).
	m2, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Error("tick while idle should not reschedule the spinner")
	}
	_ = m2

	// While discovering, a tick is consumed and reschedules the next frame.
	_ = m.StartDiscovery()
	m3, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Error("tick while discovering should reschedule the next frame")
	}
	_ = m3

	// A non-tick message is ignored with no Cmd.
	if _, cmd := m.Update(struct{}{}); cmd != nil {
		t.Error("non-tick message should produce no Cmd")
	}
}

func TestViewRightAlignsHelpAndFillsWidth(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetNamespace("default")
	m.SetShortHelp("? help")
	m.SetWidth(60)

	view := m.View()
	if w := lipgloss.Width(view); w != 60 {
		t.Errorf("View width = %d, want 60 (padded to full width)", w)
	}
	ci := strings.Index(view, "prod")
	hi := strings.Index(view, "? help")
	if ci < 0 || hi < 0 {
		t.Fatalf("View = %q, want both context and help present", view)
	}
	if hi < ci {
		t.Errorf("help should render to the right of context (ctx@%d, help@%d)", ci, hi)
	}
}

func TestViewDropsHelpWhenNoRoom(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetShortHelp("this is a very long help hint that will not fit")
	m.SetWidth(len("prod")) // no room for anything but the left segment

	view := m.View()
	if strings.Contains(view, "very long help") {
		t.Errorf("View = %q, want the help hint dropped when there is no room", view)
	}
	if !strings.Contains(view, "prod") {
		t.Errorf("View = %q, want the context kept when room is tight", view)
	}
}

func TestSetErrorFlattensToSingleLine(t *testing.T) {
	m := newBar()
	m.SetError("watch pods:\nconnection refused\n\n  by peer")
	if !m.HasError() {
		t.Fatal("HasError should be true after SetError")
	}
	if strings.Contains(m.errText, "\n") {
		t.Errorf("errText = %q, want no newlines (flattened to one line)", m.errText)
	}
	if m.errText != "watch pods: connection refused by peer" {
		t.Errorf("errText = %q, want whitespace collapsed to single spaces", m.errText)
	}
	m.ClearError()
	if m.HasError() {
		t.Error("ClearError should clear the error")
	}
}

func TestViewWithErrorStaysSingleLineAndDropsHelp(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetShortHelp("? help")
	m.SetWidth(40)
	// A long, multi-line error must not grow the bar past one line or keep the hint.
	m.SetError("watch pods: an extremely long error message that exceeds the bar width by a lot\nsecond line")

	view := m.View()
	if strings.Contains(view, "\n") {
		t.Errorf("View with error = %q, want a single line", view)
	}
	if w := lipgloss.Width(view); w != 40 {
		t.Errorf("View width = %d, want 40 (clamped, never wrapped)", w)
	}
	if strings.Contains(view, "? help") {
		t.Errorf("View with error = %q, want the help hint dropped while erroring", view)
	}
}

func TestViewInlineWhenWidthUnknown(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetShortHelp("? help")
	// width defaults to 0: pieces are joined inline, both present.
	view := m.View()
	if !strings.Contains(view, "prod") || !strings.Contains(view, "? help") {
		t.Errorf("View with unknown width = %q, want context and help joined inline", view)
	}
}
