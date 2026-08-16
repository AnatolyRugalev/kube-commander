package statusbar

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
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

// TestSetFilterShowsInLeftSegment proves the filter indicator renders in the left
// segment (verbatim) and disappears when cleared (M2-09b).
func TestSetFilterShowsInLeftSegment(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetFilter("/web")
	if got := m.leftSegment(); !strings.Contains(got, "/web") {
		t.Fatalf("leftSegment() = %q, want the filter indicator", got)
	}
	m.SetFilter("")
	if got := m.leftSegment(); strings.Contains(got, "/web") {
		t.Fatalf("cleared filter should not render, got %q", got)
	}
}

// TestSetMouseShowsMarker proves the mouse-capture indicator renders only while
// capture is on (D97), so this otherwise-invisible mode is always visible.
func TestSetMouseShowsMarker(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	if got := m.leftSegment(); strings.Contains(got, "mouse") {
		t.Fatalf("mouse marker should be absent by default, got %q", got)
	}
	m.SetMouse(true)
	if got := m.leftSegment(); !strings.Contains(got, "mouse") {
		t.Fatalf("leftSegment() = %q, want the mouse marker while capture is on", got)
	}
	m.SetMouse(false)
	if got := m.leftSegment(); strings.Contains(got, "mouse") {
		t.Fatalf("mouse marker should disappear when capture is off, got %q", got)
	}
}

// TestSetUnhealthyShowsMarker proves the "what's broken" indicator (STORY-06g-1)
// renders only while the unhealthy-only view is narrowing the table, so a
// narrowed table never reads as an empty one.
func TestSetUnhealthyShowsMarker(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	if got := m.leftSegment(); strings.Contains(got, "unhealthy") {
		t.Fatalf("unhealthy marker should be absent by default, got %q", got)
	}
	m.SetUnhealthy(true)
	if got := m.leftSegment(); !strings.Contains(got, "unhealthy") {
		t.Fatalf("leftSegment() = %q, want the unhealthy marker while the view is on", got)
	}
	m.SetUnhealthy(false)
	if got := m.leftSegment(); strings.Contains(got, "unhealthy") {
		t.Fatalf("unhealthy marker should disappear when the view is off, got %q", got)
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

// TestViewFillsWidth proves the bar's left segment renders and the line is padded
// to the full known width (the hint no longer shares this line — it lives on the
// dedicated hintbar below, FB-hintbar-dedicated).
func TestViewFillsWidth(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetNamespace("default")
	m.SetWidth(60)

	view := m.View()
	if w := lipgloss.Width(view); w != 60 {
		t.Errorf("View width = %d, want 60 (padded to full width)", w)
	}
	if !strings.Contains(view, "prod") {
		t.Fatalf("View = %q, want the context present", view)
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

func TestViewWithErrorStaysSingleLine(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	m.SetWidth(40)
	// A long, multi-line error must not grow the bar past one line.
	m.SetError("watch pods: an extremely long error message that exceeds the bar width by a lot\nsecond line")

	view := m.View()
	if strings.Contains(view, "\n") {
		t.Errorf("View with error = %q, want a single line", view)
	}
	if w := lipgloss.Width(view); w != 40 {
		t.Errorf("View width = %d, want 40 (clamped, never wrapped)", w)
	}
}

// TestTransientMessagesCannotSteerTheTerminal is AUTH-06 at this seam. Flattening
// on whitespace was never enough: an error toast quotes an API server or a
// credential plugin, and a `\r`, a `\b` or an escape sequence is neither
// whitespace nor visible — it would survive strings.Fields into a line the bar
// measures in runes, and repaint the bar or the row above it. Both transient
// messages are stored sanitized, so the clamp downstream measures what the
// terminal will draw.
func TestTransientMessagesCannotSteerTheTerminal(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"carriage return": {"watch pods: retrying...\rfailed", "watch pods: retrying...failed"},
		"backspace":       {"scale api: 3\b\b0 replicas", "scale api: 30 replicas"},
		"sgr color":       {"exec: \x1b[31mplugin failed\x1b[0m", "exec: plugin failed"},
		"erase display":   {"\x1b[2J\x1b[Hdelete pod: forbidden", "delete pod: forbidden"},
		"tabs":            {"aws\tsso:\texpired", "aws sso: expired"},
		"bell and nul":    {"\x07denied\x00", "denied"},
	} {
		m := newBar()
		m.SetError(tc.in)
		if m.errText != tc.want {
			t.Errorf("%s: SetError(%q) stored %q, want %q", name, tc.in, m.errText, tc.want)
		}
		m.SetNotice(tc.in)
		if m.noticeText != tc.want {
			t.Errorf("%s: SetNotice(%q) stored %q, want %q", name, tc.in, m.noticeText, tc.want)
		}

		m.ClearNotice()
		m.SetWidth(60)
		view := m.View()
		if strings.Contains(view, "\n") {
			t.Errorf("%s: View = %q, want a single line", name, view)
		}
		if w := lipgloss.Width(view); w != 60 {
			t.Errorf("%s: View width = %d, want 60", name, w)
		}
	}
}

func TestViewInlineWhenWidthUnknown(t *testing.T) {
	m := newBar()
	m.SetContext("prod")
	// width defaults to 0: the left segment renders as-is.
	view := m.View()
	if !strings.Contains(view, "prod") {
		t.Errorf("View with unknown width = %q, want the context present", view)
	}
}

// TestSetStylesRestylesTheSpinner is the trap this component's SetStyles exists to
// avoid (M4-12b-1): New copies the accent role *into* the spinner bubble, so a
// SetStyles that only assigned m.styles would leave a spinning discovery indicator in
// the departed theme's accent while the bar around it moved to the new one. The
// animation frame must survive, so a restyle mid-discovery does not stutter.
func TestSetStylesRestylesTheSpinner(t *testing.T) {
	m := newBar()
	m.StartDiscovery()
	glyph := ansi.Strip(m.spinner.View())

	mono := styles.New(styles.MonokaiTheme())
	m.SetStyles(mono)

	if got, want := m.spinner.Style.Render("x"), mono.Spinner.Render("x"); got != want {
		t.Errorf("the spinner kept the old theme's style: %q, want %q", got, want)
	}
	if old := styles.Default().Spinner.Render("x"); mono.Spinner.Render("x") == old {
		t.Fatal("the two themes style the spinner identically; the check above is vacuous")
	}
	// A restyle is not a reset: the animation frame is where it was, so the indicator
	// does not stutter when a theme is picked mid-discovery.
	if got := ansi.Strip(m.spinner.View()); got != glyph {
		t.Errorf("the restyle advanced or reset the spinner frame: %q, want %q", got, glyph)
	}
	if !m.Discovering() {
		t.Error("a restyle must not stop discovery")
	}
}
