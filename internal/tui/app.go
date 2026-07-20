package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/statusbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/table"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/help"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// ResourceWatcher is the narrow slice of the kube layer the shell needs to start
// a live table: a server-side Table watch for a resource, streaming deltas on a
// channel until the passed context is cancelled. *kube.Clients satisfies it. The
// shell depends on this interface, not the concrete client, so the root model is
// driveable in hermetic tests with a fake watch channel (D18) and so the tui
// package never has to construct a client. A model built without a watcher (the
// default — New()/NewWithKeymap with no WithWatcher) is watch-inert: selecting a
// resource is a no-op, which is what the pre-launch app and the M2-07b tests want.
type ResourceWatcher interface {
	Watch(ctx context.Context, r kube.Resource, namespace string, opts metav1.ListOptions) (<-chan kube.WatchEvent, error)
}

// Discoverer is the narrow slice of the kube layer the shell needs to reconcile
// the resource menu with the cluster's full resource set: it kicks off one async
// discovery pass and delivers its outcome exactly once on the returned channel
// (D8). *kube.Clients satisfies it. As with ResourceWatcher the shell depends on
// this interface, not the concrete client, so the tui package never constructs a
// client and the model is driveable in hermetic tests with a fake channel (D18).
// A model built without a discoverer (the default) never starts discovery: the
// menu stays on its static seed, which is a fully navigable browse experience
// (principle 4 — fast cold start doesn't block on discovery anyway).
type Discoverer interface {
	StartDiscovery(ctx context.Context) <-chan kube.DiscoveryResult
}

// Option configures a Model at construction. It keeps New()/NewWithKeymap(km)
// working unchanged (no dependencies) while letting the launcher inject a live
// client (WithWatcher for live tables, WithDiscoverer for the menu reconcile)
// without churning the constructor signature.
type Option func(*Model)

// WithWatcher wires the kube watch client the shell uses to start live tables.
// Without it the model is watch-inert.
func WithWatcher(w ResourceWatcher) Option {
	return func(m *Model) { m.watcher = w }
}

// WithDiscoverer wires the kube discovery client the shell uses to reconcile the
// resource menu on startup. Without it the model never runs discovery (the menu
// stays on its static seed).
func WithDiscoverer(d Discoverer) Option {
	return func(m *Model) { m.discoverer = d }
}

// WithNamespace scopes the initial live watch to ns ("" = all namespaces, the
// default). It seeds m.namespace at construction so the launcher can honour a
// `-n`/`--namespace` flag; the M2-08 namespace picker later re-scopes it at
// runtime through the same field.
func WithNamespace(ns string) Option {
	return func(m *Model) { m.namespace = ns }
}

// Layout constants. The status bar takes one line at the bottom; the two browse
// panes split the width, the menu (left) sized as a fraction with sensible floors
// so the table (right) always keeps room.
const (
	statusBarHeight = 1
	minMenuWidth    = 20 // total incl. border
	minTableWidth   = 20 // total incl. border
)

// Model is kubecom's root Bubble Tea model — the M2 app shell that replaces the
// M0 placeholder. Through slice M2-07c it owns the resolved keymap and the single
// mutable input state (a *keymap.Sequencer), turns every keypress into an Action
// through that sequencer (never matching a raw key — D11), embeds the toggleable
// help overlay, and composes the resource menu (left pane), resource table (right
// pane), and status bar (bottom line). Exactly one pane holds focus;
// nav.left/nav.right switch between them (with the table's horizontal scroll
// taking precedence until it is at its left edge — D60), and every other nav
// action is routed to the focused pane. Drilling into a menu item (M2-07c) starts
// a live kube.Watch for that resource, streams its deltas into the table through
// the M2-02 watch pump (ApplyEvent), and moves focus to the table; selecting
// another resource cancels the previous watch, a stale-generation guard dropping
// any in-flight deltas from it. On startup (M2-07d) it kicks off async discovery,
// runs the status-bar spinner while it is in flight, and folds the result into
// the menu (Reconcile) when it arrives — all without disturbing the seed menu the
// user is already browsing.
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

	// watcher is the kube watch client (nil → watch-inert). namespace scopes the
	// watch ("" = all namespaces until the M2-08 namespace picker lands). watchCh
	// and watchCancel are the current live watch: watchCh is re-read to pull the
	// next event, watchCancel tears it down when a newer resource is selected (or
	// the app quits). watchGen tags every watch-pump message so a delta from a
	// superseded watch — whose channel is already being drained — is dropped rather
	// than applied or used to re-issue a pump on the new channel (the same stale-
	// message guard seqGen gives the sequence timeout, D61).
	watcher     ResourceWatcher
	namespace   string
	watchCh     <-chan kube.WatchEvent
	watchCancel context.CancelFunc
	watchGen    int

	// discoverer runs the async discovery pass that reconciles the menu (nil →
	// discovery-inert; the menu stays on its static seed). discoveryCancel tears
	// the in-flight pass down on quit (the cap-1 discovery channel already keeps
	// the goroutine from leaking, D8, but cancelling drops the result promptly).
	discoverer      Discoverer
	discoveryCancel context.CancelFunc

	// seqGen tags each pending-sequence timer so a stale tick (superseded by a
	// newer pending) is ignored rather than firing the wrong action (D48/D61).
	seqGen int

	width  int
	height int
}

// New returns the root model wired to the default keymap. Config-driven key
// overrides are applied by the caller that constructs the keymap (M2-01c/M2-11);
// this constructor keeps the built-in vim-first defaults. Options (e.g.
// WithWatcher) are forwarded to NewWithKeymap.
func New(opts ...Option) Model {
	return NewWithKeymap(keymap.DefaultKeymap(), opts...)
}

// NewWithKeymap returns the root model over an already-resolved keymap, so the
// command layer can hand in a config-merged keymap without this package importing
// config (one-way dependency, as in kubecom keys). Options wire optional
// dependencies (the watch client via WithWatcher); with none the model is
// watch-inert. The menu starts focused (the user picks a resource before drilling
// into its table).
func NewWithKeymap(km *keymap.Keymap, opts ...Option) Model {
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
	for _, opt := range opts {
		opt(&m)
	}
	m.menu.Focus()
	m.status.SetShortHelp(m.help.ShortHelpView())
	return m
}

// seqTimeoutMsg fires SequenceTimeout after a pending multi-key prefix (D48). Its
// gen must match the model's current seqGen or the tick is stale and ignored.
type seqTimeoutMsg struct{ gen int }

// watchMsg wraps one message from a watch pump with the generation of the watch
// it belongs to. The model tags every pump this way so a message from a watch
// already superseded by a newer selection (its channel is being drained after
// cancellation) can be dropped: a stale delta must not mutate the table now
// showing a different resource, nor re-issue a pump that would then read the
// *current* watch's channel and race a second reader onto it. gen must equal the
// model's watchGen or the message is ignored (mirrors seqTimeoutMsg's guard).
type watchMsg struct {
	gen int
	msg tea.Msg
}

// startDiscoveryMsg is the private self-message Init emits to begin discovery.
// Init cannot start it directly — a value receiver returning only a tea.Cmd
// cannot store the cancel func or flip the spinner's discovering flag — so the
// work is deferred one message hop into Update, where the model is mutated and
// returned (mirroring how every other state change is applied). Nothing outside
// this package sends it.
type startDiscoveryMsg struct{}

// Init implements tea.Model. With a discoverer wired it kicks off the async
// discovery pass (via the startDiscoveryMsg hop, so the spinner starts and the
// cancel func is retained in Update); with none it has no startup command and the
// menu stays on its static seed.
func (m Model) Init() tea.Cmd {
	if m.discoverer == nil {
		return nil
	}
	return func() tea.Msg { return startDiscoveryMsg{} }
}

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

	case menu.ResourceSelectedMsg:
		return m.selectResource(msg.Resource)

	case watchMsg:
		return m.handleWatchMsg(msg)

	case startDiscoveryMsg:
		return m.startDiscovery()

	case DiscoveryReadyMsg:
		return m.handleDiscovery(msg)

	case spinner.TickMsg:
		// The status bar owns the discovery spinner; forward its ticks so the
		// animation advances while discovery is in flight (it drops ticks once
		// discovery finished, breaking the self-scheduling chain — M2-04).
		var cmd tea.Cmd
		m.status, cmd = m.status.Update(msg)
		return m, cmd
	}
	return m, nil
}

// selectResource (re)starts the live table for the resource the user drilled into.
// The previous watch is cancelled and its in-flight pump messages made stale (the
// watchGen bump); the table is blanked so the old resource's rows do not linger
// behind the new one — the new watch's first RESET event repopulates it (Watch
// lists internally before streaming deltas, so no separate List is needed). Focus
// moves to the table: drilling into a resource is the gesture to start browsing
// its rows, so the table takes over from the menu (nav.left at the table's left
// edge returns focus to the menu, D60/D62). With no watcher wired the model is
// watch-inert and this is a no-op.
func (m Model) selectResource(r kube.Resource) (tea.Model, tea.Cmd) {
	if m.watcher == nil {
		return m, nil
	}
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.watchGen++
	m.watchCh = nil
	m.table.SetTable(kube.Table{}) // blank until the watch's first RESET arrives.

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.watcher.Watch(ctx, r, m.namespace, metav1.ListOptions{})
	if err != nil {
		cancel()
		return m, func() tea.Msg { return NewErrorMsg("watch "+r.GVR.Resource, err) }
	}
	m.watchCancel = cancel
	m.watchCh = ch

	m.menu.Blur()
	m.table.Focus()
	return m, m.pumpWatch()
}

// pumpWatch issues the tea.Cmd that pulls the next event from the current watch
// channel, tagged with the current watchGen so a message from a superseded watch
// is recognisable as stale. It returns nil when no watch is active.
func (m Model) pumpWatch() tea.Cmd {
	gen, ch := m.watchGen, m.watchCh
	if ch == nil {
		return nil
	}
	pump := watchPump(ch)
	return func() tea.Msg { return watchMsg{gen: gen, msg: pump()} }
}

// handleWatchMsg applies one watch-pump message to the table and re-issues the
// pump to pull the next event — the one-receive-per-Cmd loop that keeps Update
// from ever blocking (M2-02). A message from a superseded watch (wrong gen) is
// dropped and its chain stops. A data delta is folded onto the table preserving
// the selection (ApplyEvent); a watch ERROR keeps the chain alive (the watch loop
// retries and re-lists on recovery, emitting a fresh RESET — visible error
// surfacing on the pane is a later slice); a closed channel ends the chain.
func (m Model) handleWatchMsg(w watchMsg) (tea.Model, tea.Cmd) {
	if w.gen != m.watchGen {
		return m, nil // superseded by a newer selection; drop and stop this chain.
	}
	switch inner := w.msg.(type) {
	case ResourceEventMsg:
		m.table.ApplyEvent(inner.Event)
		return m, m.pumpWatch()
	case ErrorMsg:
		return m, m.pumpWatch()
	case WatchClosedMsg:
		m.watchCh = nil
		m.watchCancel = nil
		return m, nil
	}
	return m, nil
}

// startDiscovery kicks off the async discovery pass and starts the status-bar
// spinner. The pass runs on its own cancellable context (torn down on quit or
// when its result arrives) and delivers exactly once on a cap-1 channel (D8); the
// discovery pump reads that single result as a DiscoveryReadyMsg. The spinner Cmd
// and the pump are batched so the animation runs alongside the wait. With no
// discoverer this is a no-op (Init never emits startDiscoveryMsg without one, but
// the guard keeps it safe if called directly).
func (m Model) startDiscovery() (tea.Model, tea.Cmd) {
	if m.discoverer == nil {
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.discoveryCancel = cancel
	ch := m.discoverer.StartDiscovery(ctx)
	spin := m.status.StartDiscovery()
	return m, tea.Batch(spin, discoveryPump(ch))
}

// handleDiscovery folds a completed discovery pass into the menu and stops the
// spinner. Reconcile merges the discovered resources into the seed without
// disturbing selection or scroll (M2-05b/D57) and is a no-op on a total failure
// (Result.Err set) — the menu then stays on its fully navigable seed rather than
// blanking (principle 3; visible surfacing of a discovery failure is a later
// slice). The one-shot context is cancelled now its result is in hand.
func (m Model) handleDiscovery(msg DiscoveryReadyMsg) (tea.Model, tea.Cmd) {
	m.status.StopDiscovery()
	if m.discoveryCancel != nil {
		m.discoveryCancel()
		m.discoveryCancel = nil
	}
	m.menu.Reconcile(msg.Result)
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
		if m.watchCancel != nil {
			m.watchCancel() // tear the watch goroutine down before the program exits.
		}
		if m.discoveryCancel != nil {
			m.discoveryCancel() // and any in-flight discovery pass.
		}
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
//
// Every returned view sets AltScreen: in bubbletea v2 full-screen mode is a
// property of the View (v.AltScreen), not a program option — the v1-era
// tea.WithAltScreen() no longer exists — so the root model, which owns View, is
// where kubecom requests the alternate screen buffer (D70).
func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	var body string
	if m.help.Visible() {
		body = m.help.View()
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.menu.View(), m.table.View())
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, m.status.View()))
	v.AltScreen = true
	return v
}
