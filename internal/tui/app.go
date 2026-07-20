package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/help"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
	"github.com/AnatolyRugalev/kube-commander/internal/version"
)

// Model is kubecom's root Bubble Tea model — the M2 app shell that replaces the
// M0 placeholder. This is slice M2-07a: the keymap-routed skeleton. It owns the
// resolved keymap and the single mutable input state (a *keymap.Sequencer), turns
// every keypress into an Action through that sequencer (never matching a raw key
// — D11), and embeds the toggleable help overlay. It has no panes yet: the
// two-pane browse layout (menu | table), the live watch wiring, and the async
// discovery reconcile land in M2-07b/c/d.
//
// It holds no shared mutable state (principle 1): the Sequencer is a pointer so
// its buffered prefix survives the value-model copy Bubble Tea makes each Update,
// but it is only ever touched from the single-threaded update loop.
type Model struct {
	keymap *keymap.Keymap
	seq    *keymap.Sequencer
	help   help.Model
	styles styles.Styles

	// seqGen tags each pending-sequence timer so a stale tick (superseded by a
	// newer pending) is ignored rather than firing the wrong action (D48/D61).
	seqGen int

	width  int
	height int
}

// New returns the root model wired to the default keymap. Config-driven key
// overrides are applied by the caller that constructs the keymap (M2-01c/M2-11);
// this constructor keeps the built-in vim-first defaults.
func New() Model {
	return NewWithKeymap(keymap.DefaultKeymap())
}

// NewWithKeymap returns the root model over an already-resolved keymap, so the
// command layer can hand in a config-merged keymap without this package importing
// config (one-way dependency, as in kubecom keys).
func NewWithKeymap(km *keymap.Keymap) Model {
	return Model{
		keymap: km,
		seq:    keymap.NewSequencer(km),
		help:   help.New(km),
		styles: styles.Default(),
	}
}

// seqTimeoutMsg fires SequenceTimeout after a pending multi-key prefix (D48). Its
// gen must match the model's current seqGen or the tick is stale and ignored.
type seqTimeoutMsg struct{ gen int }

// Init implements tea.Model. The skeleton has no startup command; async discovery
// is kicked off in M2-07d.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. It routes window-size to the layout and the help
// overlay, resolves keypresses through the sequencer, and services the sequence
// timeout tick. Every behaviour flows through an Action; no raw key is matched.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil

	case tea.KeyPressMsg:
		switch r := m.seq.Input(msg.Key()); r.Kind {
		case keymap.ResultAction:
			return m.handleAction(r.Action)
		case keymap.ResultPending:
			// Hold the prefix; schedule a timeout so a lone `g` still fires its
			// short form if no second key arrives. Tag it so an earlier timer,
			// left over from a prefix already resolved, cannot fire this one early.
			m.seqGen++
			return m, m.scheduleTimeout()
		default: // ResultNone: inert.
			return m, nil
		}

	case seqTimeoutMsg:
		if msg.gen != m.seqGen {
			return m, nil // superseded by a newer pending; ignore.
		}
		if r := m.seq.Timeout(); r.Kind == keymap.ResultAction {
			return m.handleAction(r.Action)
		}
		return m, nil
	}
	return m, nil
}

// scheduleTimeout arms the sequence-timeout tick for the current generation.
func (m Model) scheduleTimeout() tea.Cmd {
	gen := m.seqGen
	return tea.Tick(keymap.SequenceTimeout, func(time.Time) tea.Msg {
		return seqTimeoutMsg{gen: gen}
	})
}

// handleAction applies a resolved action. The skeleton services the app-global
// actions (quit, help toggle, back-closes-help); navigation actions become live
// once the browse panes land (M2-07b onward), so they are inert no-ops today.
func (m Model) handleAction(a keymap.Action) (tea.Model, tea.Cmd) {
	switch a {
	case keymap.ActionQuit:
		return m, tea.Quit
	case keymap.ActionHelp:
		m.help.Toggle()
		return m, nil
	case keymap.ActionBack:
		// esc closes the help overlay when it is open; otherwise inert until the
		// pane/drill-in stack exists.
		if m.help.Visible() {
			m.help.SetVisible(false)
		}
		return m, nil
	default:
		return m, nil
	}
}

// View implements tea.Model. Until the first WindowSizeMsg it renders nothing so
// the layout is never sized to a zero terminal. When the help overlay is open it
// takes the body; otherwise a placeholder marks where the browse panes will land.
// A one-line short-help hint (generated from the keymap, D11) always trails.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	var body string
	if m.help.Visible() {
		body = m.help.View()
	} else {
		body = m.styles.App.Render("kubecom " + version.Version)
		body = lipgloss.JoinVertical(lipgloss.Left, body,
			m.styles.Subtle.Render("(app shell M2-07a — browse panes land next)"))
	}

	hint := m.styles.Subtle.Render(m.help.ShortHelpView())
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, "", hint))
}
