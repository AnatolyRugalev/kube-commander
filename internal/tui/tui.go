package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/version"
)

// Model is a placeholder Bubble Tea root model for the M0 groundwork build. It
// renders a static splash and tracks the terminal size; it deliberately does no
// input handling, because every key must flow through the configurable action
// registry (D11), which lands in M2. The real root model — action registry,
// two-pane browse UX, live-watched tables — replaces this from M2 onward.
//
// Its purpose today is to give the teatest smoke harness (M0-05) a real program
// to drive and assert against, proving the TUI test path works end-to-end before
// any feature UI exists.
type Model struct {
	width  int
	height int
}

// New returns the placeholder root model.
func New() Model {
	return Model{}
}

// Init implements tea.Model. The placeholder has no startup command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. It only records the terminal size; it matches no
// key literals (that is the action registry's job, D11). The program is stopped
// externally via tea.Quit (e.g. teatest's Quit), so no quit key is handled here.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		m.height = sz.Height
	}
	return m, nil
}

// View implements tea.Model, rendering the M0 splash.
func (m Model) View() tea.View {
	return tea.NewView("kubecom " + version.Version + "\n(groundwork build; interactive UI lands in M2)")
}
