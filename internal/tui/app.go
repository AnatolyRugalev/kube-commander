package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/statusbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/table"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/help"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// Layout constants. The status bar takes one line at the bottom; the two browse
// panes split the width, the menu (left) sized as a fraction with sensible floors
// so the table (right) always keeps room.
const (
	statusBarHeight = 1
	minMenuWidth    = 20 // total incl. border
	minTableWidth   = 20 // total incl. border
)

// Model is kubecom's root Bubble Tea model — the M2 app shell that replaces the
// M0 placeholder. This is slice M2-07b: the two-pane browse layout. The root
// model owns the resolved keymap and the single mutable input state (a
// *keymap.Sequencer), turns every keypress into an Action through that sequencer
// (never matching a raw key — D11), embeds the toggleable help overlay, and now
// composes the resource menu (left pane), resource table (right pane), and status
// bar (bottom line). Exactly one pane holds focus; nav.left/nav.right switch
// between them (with the table's horizontal scroll taking precedence until it is
// at its left edge — D60), and every other nav action is routed to the focused
// pane. The live watch wiring (menu selection → kube.Watch → table) is M2-07c and
// the async discovery reconcile + spinner is M2-07d.
//
// It holds no shared mutable state (principle 1): the Sequencer is a pointer so
// its buffered prefix survives the value-model copy Bubble Tea makes each Update,
// but it is only ever touched from the single-threaded update loop; the component
// models are plain value fields fed actions and read for their View.
type Model struct {
	keymap *keymap.Keymap
	seq    *keymap.Sequencer
	help   help.Model
	styles styles.Styles

	menu   menu.Model
	table  table.Model
	status statusbar.Model

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
// config (one-way dependency, as in kubecom keys). The menu starts focused (the
// user picks a resource before drilling into its table).
func NewWithKeymap(km *keymap.Keymap) Model {
	s := styles.Default()
	m := Model{
		keymap: km,
		seq:    keymap.NewSequencer(km),
		help:   help.New(km),
		styles: s,
		menu:   menu.New(s),
		table:  table.New(s),
		status: statusbar.New(s),
	}
	m.menu.Focus()
	m.status.SetShortHelp(m.help.ShortHelpView())
	return m
}

// seqTimeoutMsg fires SequenceTimeout after a pending multi-key prefix (D48). Its
// gen must match the model's current seqGen or the tick is stale and ignored.
type seqTimeoutMsg struct{ gen int }

// Init implements tea.Model. The shell has no startup command yet; async
// discovery is kicked off in M2-07d.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. It sizes the layout on a window-size message,
// resolves keypresses through the sequencer, and services the sequence timeout
// tick. Every behaviour flows through an Action; no raw key is matched.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		m.status.SetWidth(msg.Width)
		m.status.SetShortHelp(m.help.ShortHelpView())
		m.resize()
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

// resize lays the panes out inside the current terminal: the status bar takes the
// bottom line, and the menu and table split the remaining width (menu a fraction
// with floors so the table always keeps room). Both panes are sized to their
// total width/height including border, as their SetSize expects.
func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	bodyH := m.height - statusBarHeight
	if bodyH < 0 {
		bodyH = 0
	}
	menuW := menuPaneWidth(m.width)
	tableW := m.width - menuW
	if tableW < 0 {
		tableW = 0
	}
	m.menu.SetSize(menuW, bodyH)
	m.table.SetSize(tableW, bodyH)
}

// menuPaneWidth is the menu pane's total width for a given terminal width: a
// quarter of the screen, floored at minMenuWidth, but never so wide the table
// pane drops below minTableWidth (on a narrow terminal the two split evenly).
func menuPaneWidth(total int) int {
	if total <= 0 {
		return 0
	}
	w := total / 4
	if w < minMenuWidth {
		w = minMenuWidth
	}
	if w > total-minTableWidth {
		w = total / 2
	}
	if w < 0 {
		w = 0
	}
	return w
}

// scheduleTimeout arms the sequence-timeout tick for the current generation.
func (m Model) scheduleTimeout() tea.Cmd {
	gen := m.seqGen
	return tea.Tick(keymap.SequenceTimeout, func(time.Time) tea.Msg {
		return seqTimeoutMsg{gen: gen}
	})
}

// handleAction applies a resolved action. App-global actions (quit, help toggle,
// back-closes-help) are serviced first; while the help overlay is open it swallows
// navigation so the panes underneath do not move. Otherwise the action is routed
// to the focused pane, with nav.left/nav.right also switching focus between panes.
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
	}
	if m.help.Visible() {
		return m, nil // the overlay swallows navigation while it is open.
	}
	return m.routeNav(a)
}

// routeNav dispatches a navigation action to the focused pane and handles the
// horizontal focus switch between the menu (left) and table (right):
//
//   - Menu focused: nav.right moves focus to the table; every other action goes
//     to the menu (nav.left is the leftmost edge, so the menu ignores it).
//   - Table focused: nav.left scrolls the table's columns left, but when the table
//     is already at its left edge (HOffset 0, nothing to scroll) it instead moves
//     focus back to the menu — the pane-focus-vs-scroll arbitration promised by the
//     table's HOffset accessor (M2-06c/D60). Every other action goes to the table.
func (m Model) routeNav(a keymap.Action) (tea.Model, tea.Cmd) {
	if m.table.Focused() {
		if a == keymap.ActionLeft && m.table.HOffset() == 0 {
			m.table.Blur()
			m.menu.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(a)
		return m, cmd
	}
	// Menu focused (the default).
	if a == keymap.ActionRight {
		m.menu.Blur()
		m.table.Focus()
		return m, nil
	}
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(a)
	return m, cmd
}

// View implements tea.Model. Until the first WindowSizeMsg it renders nothing so
// the layout is never sized to a zero terminal. Normally it lays the menu and
// table panes side by side over the status bar; when the help overlay is open it
// takes the body area, the status bar staying pinned below.
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	var body string
	if m.help.Visible() {
		body = m.help.View()
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.menu.View(), m.table.View())
	}

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, m.status.View()))
}
