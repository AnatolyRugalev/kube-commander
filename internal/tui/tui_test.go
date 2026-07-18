package tui

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// TestModelSmoke is the M0-05 teatest smoke test: it proves the TUI test harness
// works end-to-end — a model can be driven, its rendered output inspected, and
// the program shut down cleanly — before any feature UI exists. It is the
// template M2+ view tests build on.
func TestModelSmoke(t *testing.T) {
	tm := teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(80, 24))

	// The splash must render; WaitFor polls the program's output.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("kubecom"))
	}, teatest.WithDuration(3*time.Second))

	// The placeholder handles no keys (D11: input flows through the M2 action
	// registry, not raw-key matching), so stop the program via tea.Quit rather
	// than a keystroke.
	tm.Send(tea.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}
