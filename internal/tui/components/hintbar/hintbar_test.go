package hintbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newBar() Model { return New(styles.Default()) }

// TestSetHintRenders proves the hint set on the line shows up in its View and is
// readable back via Hint().
func TestSetHintRenders(t *testing.T) {
	m := newBar()
	m.SetHint("? help  q quit")
	if got := m.Hint(); got != "? help  q quit" {
		t.Fatalf("Hint() = %q, want the set hint", got)
	}
	if !strings.Contains(m.View(), "help") {
		t.Fatalf("View() = %q, want the hint text", m.View())
	}
}

// TestViewClampsToWidth proves a hint longer than the width is clamped to one
// screen width and never wraps onto a second line — the whole point of the
// dedicated line is that it stays exactly one row.
func TestViewClampsToWidth(t *testing.T) {
	m := newBar()
	m.SetHint("this is a very long hint that easily exceeds the tiny width given below")
	m.SetWidth(20)
	view := m.View()
	if strings.Contains(view, "\n") {
		t.Fatalf("View() = %q, want a single line", view)
	}
	if w := lipgloss.Width(view); w > 20 {
		t.Errorf("View() width = %d, want <= 20 (clamped)", w)
	}
}

// TestEmptyHintRendersEmpty proves an unset hint renders nothing (no stray styled
// blank), so the line is inert until the root model feeds it a hint.
func TestEmptyHintRendersEmpty(t *testing.T) {
	if got := newBar().View(); got != "" {
		t.Fatalf("empty hint View() = %q, want empty", got)
	}
}
