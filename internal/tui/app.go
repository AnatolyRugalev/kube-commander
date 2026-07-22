package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/hintbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/statusbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/table"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/welcome"
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

// NamespaceLister is the narrow slice of the kube layer the shell needs to seed
// the namespace picker (M2-08c): it lists the cluster's namespace names. As with
// ResourceWatcher/Discoverer the shell depends on this interface, not the concrete
// client, so the tui package never constructs a client and the model is driveable
// in hermetic tests with a fake lister. A model built without a lister (the
// default) is namespace-switch-inert: the ns.switch action is a no-op (the picker
// never opens), which is what the pre-launch app and the non-picker tests want.
type NamespaceLister interface {
	Namespaces(ctx context.Context) ([]string, error)
}

// NamespacePersister records the last-selected namespace for the active context so
// the next launch can restore it as the initial watch scope (M2-11b-2). It is the
// write side of the per-context state store (config.State/state.go, D90); the
// launcher, which alone knows the resolved context and its state-file path, wires a
// persister already bound to that context, so the tui package stays storage- and
// context-agnostic. A model built without one (the default, or an unresolved
// context) is persistence-inert: switching namespace applies for the session but is
// not remembered.
type NamespacePersister interface {
	PersistNamespace(ns string) error
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
// `-n`/`--namespace` flag; the namespace picker (M2-08c) re-scopes it at runtime
// through the same field.
func WithNamespace(ns string) Option {
	return func(m *Model) { m.namespace = ns }
}

// WithNamespaceLister wires the kube client the shell uses to seed the namespace
// picker. Without it the ns.switch action is inert (the picker never opens).
func WithNamespaceLister(l NamespaceLister) Option {
	return func(m *Model) { m.nsLister = l }
}

// WithNamespacePersister wires the per-context state writer the shell calls when
// the user picks a namespace, so the choice is restored on the next launch
// (M2-11b-2). Without it (or with a nil persister) namespace switches are not
// persisted — inert, exactly like the pre-wiring app and the hermetic tests.
func WithNamespacePersister(p NamespacePersister) Option {
	return func(m *Model) { m.nsPersister = p }
}

// WithContext sets the kube context name shown on the status bar and the startup
// welcome page. Purely cosmetic; empty renders nothing.
func WithContext(name string) Option {
	return func(m *Model) { m.context = name }
}

// WithVersion sets the build version shown on the startup welcome page (e.g.
// "dev" or a release tag). Empty shows the bare name.
func WithVersion(v string) Option {
	return func(m *Model) { m.version = v }
}

// WithMenuExtras folds the current context's per-context menu customizations
// (config.MenuResource entries, D83) into the resource menu at construction — the
// extra CRDs a user named for this kubeconfig context. They are merged before
// discovery runs (menu.AddExtras) so a later discovered twin dedupes against the
// extra rather than double-listing it (FB-menu-config-02). Empty/nil adds nothing,
// leaving the built-in default menu (a context with no menu file). The launcher
// resolves the file via config.LoadMenuFile(config.MenuPath(ctx)) and passes the
// entries here; a missing/malformed file degrades to the default menu upstream.
func WithMenuExtras(extras []config.MenuResource) Option {
	return func(m *Model) { m.menuExtras = extras }
}

// WithStartupError seeds a one-shot error the model surfaces as a transient
// status-bar toast on Init (batched with any discovery start), so a startup-time
// degradation the launcher chose not to make fatal — chiefly a malformed
// per-context menu file that fell back to the default menu — is still visible to
// the user rather than silently swallowed (principle 3). Nil surfaces nothing.
func WithStartupError(e *ErrorMsg) Option {
	return func(m *Model) { m.startupErr = e }
}

// Layout constants. The status bar takes one line at the bottom; the two browse
// panes split the width, the menu (left) sized as a fraction with sensible floors
// so the table (right) always keeps room.
const (
	statusBarHeight = 1
	// hintBarHeight is the dedicated key-hint line pinned below the status bar
	// (FB-hintbar-dedicated) — always one row, so the hint is never dropped under
	// width pressure nor hidden behind an error toast.
	hintBarHeight = 1
	minMenuWidth  = 20 // total incl. border
	minTableWidth = 20 // total incl. border
	// maxMenuWidth caps the menu pane so a wide terminal doesn't hand a quarter of
	// the screen to a list whose content is far narrower. Sized close to the menu's
	// real content (a bordered pane this wide fits the standard resource names;
	// longer ones — CRDs, MutatingWebhookConfiguration — ellipsis-truncate, D84).
	maxMenuWidth = 28 // total incl. border

	// errorDisplay is how long a surfaced error stays in the status bar before it
	// auto-clears (a transient toast). A stale-generation guard (statusErrGen)
	// stops an old clear timer wiping a newer error early.
	errorDisplay = 5 * time.Second
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

	menu     menu.Model
	table    table.Model
	status   statusbar.Model
	hintbar  hintbar.Model
	nsPicker picker.Model
	welcome  welcome.Model

	// context is the resolved kube context name and version the build version;
	// both are cosmetic, shown on the status bar (context) and the startup welcome
	// page (both). Set at construction via WithContext/WithVersion.
	context string
	version string

	// menuExtras are the current context's per-context menu customizations (D83),
	// merged into the seed menu at construction (WithMenuExtras → menu.AddExtras)
	// before discovery so a discovered twin dedupes against them. startupErr is a
	// one-shot toast surfaced on Init (WithStartupError) — chiefly a malformed
	// per-context menu file that degraded to the default menu, kept visible rather
	// than swallowed. Both nil by default (the plain default menu, no toast).
	menuExtras []config.MenuResource
	startupErr *ErrorMsg

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

	// current is the resource whose live table is showing (hasCurrent guards it);
	// the namespace picker re-scopes the watch by re-selecting it with the new
	// m.namespace (M2-08c). It is set every time selectResource starts a watch.
	current    kube.Resource
	hasCurrent bool

	// nsLister seeds the namespace picker (nil → ns.switch inert). nsPicker (below,
	// with the other components) is the modal itself. nsPersister records a picked
	// namespace to the per-context state file so the next launch restores it (nil →
	// persistence-inert, M2-11b-2).
	nsLister    NamespaceLister
	nsPersister NamespacePersister

	// filterInput is the table filter field (M2-09b): app.filter (`/`) opens it over
	// the current table, typing narrows the live rows through table.SetFilter (D78),
	// and it re-scopes to whatever is showing. filtering is whether it is open and
	// capturing text — while true the root routes every keypress through
	// routeFilterKey (control/text split, D73), bypassing the sequencer, exactly as
	// the namespace picker does. The narrowing is a view over the table's
	// authoritative full set, so clearing the filter restores every live row.
	filterInput textinput.Model
	filtering   bool

	// discoverer runs the async discovery pass that reconciles the menu (nil →
	// discovery-inert; the menu stays on its static seed). discoveryCancel tears
	// the in-flight pass down on quit (the cap-1 discovery channel already keeps
	// the goroutine from leaking, D8, but cancelling drops the result promptly).
	discoverer      Discoverer
	discoveryCancel context.CancelFunc

	// seqGen tags each pending-sequence timer so a stale tick (superseded by a
	// newer pending) is ignored rather than firing the wrong action (D48/D61).
	seqGen int

	// statusErrGen tags each transient status-bar error's auto-clear timer so an
	// older timer cannot wipe a newer error early (mirrors seqGen's stale-tick
	// guard). Every surfaced error bumps it; only a clear tick whose gen still
	// matches clears the bar.
	statusErrGen int

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
	fi := textinput.New()
	fi.Prompt = "/"
	m := Model{
		keymap:      km,
		seq:         keymap.NewSequencer(km),
		help:        help.New(s, km),
		styles:      s,
		menu:        menu.New(s),
		table:       table.New(s),
		status:      statusbar.New(s),
		hintbar:     hintbar.New(s),
		nsPicker:    picker.New(s, "namespace"),
		welcome:     welcome.New(s),
		filterInput: fi,
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.menu.AddExtras(m.menuExtras)     // fold in the per-context menu customizations (D83); no-op when none
	m.menu.Focus()
	m.menu.SetNamespace(m.namespace)   // seam row reflects the initial -n scope
	m.syncHints()                      // menu starts focused → menu-context hints
	m.status.SetContext(m.context)     // reflect the resolved --context (empty renders nothing)
	m.status.SetNamespace(m.namespace) // reflect the -n scope (empty renders nothing)
	m.welcome.SetVersion(m.version)
	m.welcome.SetContext(m.context)
	m.welcome.SetNamespace(m.namespace)
	m.welcome.SetShortHelp(m.help.ShortHelpView()) // landing page keeps the focus-agnostic set
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

// errorClearMsg auto-clears a transient status-bar error after errorDisplay. Its
// gen must match the model's current statusErrGen or the timer is stale (a newer
// error superseded it) and the clear is ignored — the newer error keeps its own
// full display window.
type errorClearMsg struct{ gen int }

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
// menu stays on its static seed. A seeded startup error (WithStartupError — e.g. a
// malformed per-context menu file that degraded to the default menu) is surfaced
// as a transient toast, batched with the discovery start.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.startupErr != nil {
		e := *m.startupErr
		cmds = append(cmds, func() tea.Msg { return e })
	}
	if m.discoverer != nil {
		cmds = append(cmds, func() tea.Msg { return startDiscoveryMsg{} })
	}
	return tea.Batch(cmds...)
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
		m.hintbar.SetWidth(msg.Width)
		m.syncHints() // re-elide the focus-aware hint to the new width
		m.resize()
		return m, nil

	case tea.KeyPressMsg:
		if m.nsPicker.Active() {
			return m.routePickerKey(msg)
		}
		if m.filtering {
			return m.routeFilterKey(msg)
		}
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

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

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

	case menu.NamespaceRequestedMsg:
		// Drilling into the menu's namespace-seam row opens the namespace picker —
		// the same effect as the ns.switch (ctrl+n) shortcut.
		return m.openNamespacePicker()

	case ErrorMsg:
		// A classified error from any async seam (watch start, namespace list, a
		// watch ERROR bridged by the pump). Surface it inside the fixed layout — a
		// transient status-bar message — never on stdout or a growing pane, so the
		// panes never scroll or resize (the feedback this leg addresses).
		return m, m.surfaceError(msg)

	case errorClearMsg:
		if msg.gen == m.statusErrGen {
			m.status.ClearError()
		}
		return m, nil

	case watchMsg:
		return m.handleWatchMsg(msg)

	case startDiscoveryMsg:
		return m.startDiscovery()

	case DiscoveryReadyMsg:
		return m.handleDiscovery(msg)

	case namespacesLoadedMsg:
		return m.handleNamespacesLoaded(msg)

	case picker.SelectedMsg:
		return m.handleNamespaceSelected(msg)

	case picker.CancelledMsg:
		m.nsPicker.Hide()
		return m, nil

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
	// A fresh resource (or re-scoped namespace) starts unfiltered: SetTable clears
	// the table's filter (D78); mirror that in the shell's filter state so a stale
	// prompt/indicator from the previous resource does not linger.
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.Reset()
	m.syncFilterStatus()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.watcher.Watch(ctx, r, m.namespace, metav1.ListOptions{})
	if err != nil {
		cancel()
		return m, func() tea.Msg { return NewErrorMsg("watch "+r.GVR.Resource, err) }
	}
	m.watchCancel = cancel
	m.watchCh = ch
	m.current = r
	m.hasCurrent = true

	m.menu.SetActive(r) // mark the opened resource distinctly from the nav cursor
	m.menu.Blur()
	m.table.Focus()
	m.syncHints() // focus is now the table → table-context hints
	return m, m.pumpWatch()
}

// surfaceError shows a classified error as a transient message in the status bar
// and arms its auto-clear timer. It bumps statusErrGen so a stale clear timer from
// an earlier error cannot wipe this one early, and returns the clear Cmd for the
// caller to schedule (batched with any other work). It mutates the receiver, so
// callers pass the addressable model value they are about to return.
func (m *Model) surfaceError(e ErrorMsg) tea.Cmd {
	m.status.SetError(e.Message())
	m.statusErrGen++
	gen := m.statusErrGen
	return tea.Tick(errorDisplay, func(time.Time) tea.Msg {
		return errorClearMsg{gen: gen}
	})
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
// retries and re-lists on recovery, emitting a fresh RESET) and surfaces
// transiently in the status bar; a closed channel ends the chain.
func (m Model) handleWatchMsg(w watchMsg) (tea.Model, tea.Cmd) {
	if w.gen != m.watchGen {
		return m, nil // superseded by a newer selection; drop and stop this chain.
	}
	switch inner := w.msg.(type) {
	case ResourceEventMsg:
		m.table.ApplyEvent(inner.Event)
		return m, m.pumpWatch()
	case ErrorMsg:
		// The watch loop retries and re-lists on recovery (a fresh RESET follows),
		// so the chain stays alive; surface the error transiently in the status bar
		// meanwhile rather than swallowing it silently.
		clear := m.surfaceError(inner)
		return m, tea.Batch(clear, m.pumpWatch())
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

// namespacesLoadedMsg carries the outcome of the async namespace list issued when
// the picker opens (M2-08c). It seeds the already-shown picker; listing happens
// off the update loop so opening the picker never blocks on the network.
type namespacesLoadedMsg struct {
	namespaces []string
	err        error
}

// openNamespacePicker shows the namespace picker and kicks off the async list that
// seeds it. With no lister wired the model is namespace-switch-inert and this is a
// no-op (the picker never opens). The picker is shown immediately (empty, then
// populated when the list lands) so the gesture feels instant; a stale item set
// from a previous open is cleared first.
func (m Model) openNamespacePicker() (tea.Model, tea.Cmd) {
	if m.nsLister == nil {
		return m, nil
	}
	m.nsPicker.SetItems(nil)
	m.nsPicker.Show()
	lister := m.nsLister
	return m, func() tea.Msg {
		ns, err := lister.Namespaces(context.Background())
		return namespacesLoadedMsg{namespaces: ns, err: err}
	}
}

// namespaceAllItem is the sentinel entry pinned at the top of the namespace picker.
// Selecting it re-scopes the watch to every namespace (empty scope) — the app
// launches unscoped, so without this entry the picker (which lists only concrete
// namespaces) is a one-way door: once a namespace is picked there is no way back to
// the all-namespaces view short of restarting (dogfood-09). A concrete namespace can
// never collide with it: DNS-label names cannot contain a space.
const namespaceAllItem = "all namespaces"

// handleNamespacesLoaded seeds the open picker with the listed namespaces, pinning
// the all-namespaces sentinel at the top so the unscoped view is always reachable.
// A list failure surfaces a classified error and closes the picker (principle 3 —
// the switcher degrades, the app does not crash). A result that arrives after the
// user already dismissed the picker is dropped.
func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.nsPicker.Hide()
		return m, func() tea.Msg { return NewErrorMsg("list namespaces", msg.err) }
	}
	if !m.nsPicker.Active() {
		return m, nil // dismissed before the list arrived; ignore.
	}
	items := make([]string, 0, len(msg.namespaces)+1)
	items = append(items, namespaceAllItem)
	items = append(items, msg.namespaces...)
	m.nsPicker.SetItems(items)
	return m, nil
}

// handleNamespaceSelected applies the picked namespace: it closes the picker,
// records the new scope on the model and the status bar, and re-scopes the live
// table by re-selecting the current resource with the new m.namespace (M2-07c's
// watch reads that field). The all-namespaces sentinel maps back to the empty scope
// (watch every namespace). With no resource open yet the scope is simply stored
// for the next selection.
func (m Model) handleNamespaceSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.nsPicker.Hide()
	ns := msg.Value
	if ns == namespaceAllItem {
		ns = ""
	}
	m.namespace = ns
	m.status.SetNamespace(ns)
	m.menu.SetNamespace(ns)    // keep the seam row's scope current
	m.welcome.SetNamespace(ns) // keep the welcome scope current if shown pre-drill-in
	persist := m.persistNamespace(ns)
	if m.hasCurrent && m.watcher != nil {
		model, cmd := m.selectResource(m.current)
		return model, tea.Batch(persist, cmd)
	}
	return m, persist
}

// persistNamespace records the picked namespace as this context's last namespace so
// the next launch restores it (M2-11b-2). The file write runs off the update loop
// (a tea.Cmd) so persistence never blocks input; a write failure surfaces as a
// transient toast (ErrorMsg) but is otherwise non-fatal — the chosen scope still
// applies for this session (principle 3). With no persister wired it is a no-op.
func (m Model) persistNamespace(ns string) tea.Cmd {
	if m.nsPersister == nil {
		return nil
	}
	p := m.nsPersister
	return func() tea.Msg {
		if err := p.PersistNamespace(ns); err != nil {
			return NewErrorMsg("persist namespace", err)
		}
		return nil
	}
}

// routePickerKey resolves one keypress while the namespace picker is open. The
// picker captures all input (the panes and the sequencer never see it): a control
// key (esc/enter/arrows/page keys, and ctrl+d/u) resolves to an Action the picker
// consumes, while any text-producing or editing key is filter input routed to the
// field. The split is by whether the key carries text — a printable rune types,
// everything without text (incl. a bound vim letter like `j`) is a control action,
// and an unmapped no-text key (backspace) still reaches the filter for editing
// (D73). No view matches a raw key for behaviour (D11).
func (m Model) routePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	action, mapped := m.keymap.Action(key)
	var cmd tea.Cmd
	if m.nsPicker.Filtering() {
		if mapped && key.Text == "" {
			m.nsPicker, cmd = m.nsPicker.Update(action)
		} else {
			m.nsPicker, cmd = m.nsPicker.UpdateFilter(msg)
		}
		return m, cmd
	}
	if mapped {
		m.nsPicker, cmd = m.nsPicker.Update(action)
	}
	return m, cmd
}

// openFilter opens the live table filter input over the current resource table
// (M2-09b). It is a no-op unless a resource table is showing (hasCurrent) —
// filtering the welcome page has nothing to narrow. The field is seeded with any
// already-active filter (reopening `/` edits the current query, cursor at the end)
// and focus moves to the table; typing then narrows the rows live through
// table.SetFilter, enter commits the narrowed view, esc clears it and restores
// every row (D78). With no resource open yet it does nothing.
func (m Model) openFilter() (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	m.filtering = true
	m.filterInput.SetValue(m.table.Filter())
	m.filterInput.CursorEnd()
	cmd := m.filterInput.Focus()
	m.menu.Blur()
	m.table.Focus()
	m.syncHints() // filtering acts on the table → table-context hints
	m.syncFilterStatus()
	return m, cmd
}

// routeFilterKey resolves one keypress while the filter input is open. It mirrors
// routePickerKey's control/text split (D73): a mapped key carrying no text
// (esc/enter/arrows/ctrl+d…) is a control Action the filter mode consumes, while any
// text-producing or editing key (a rune, or an unmapped no-text key like backspace)
// is filter input fed to the field — re-narrowing the table live. No view matches a
// raw key (D11); the open field captures all input, so the sequencer and the panes
// underneath never see it.
func (m Model) routeFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		return m.handleFilterAction(action)
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.table.SetFilter(m.filterInput.Value())
	m.syncFilterStatus()
	return m, cmd
}

// handleFilterAction applies a control action while the filter input is open: enter
// (nav.drillIn) commits the narrowed view and closes the input; esc (nav.back)
// cancels — clears the filter, restoring every row — and closes it; the vertical
// navigation actions move the selection through the live-narrowed rows so matches
// can be previewed while typing; every other action is ignored (n/N, ns.switch,
// help and quit cannot fire mid-filter — their keys either type into the field or
// are dropped here).
func (m Model) handleFilterAction(a keymap.Action) (tea.Model, tea.Cmd) {
	switch a {
	case keymap.ActionDrillIn:
		return m.commitFilter()
	case keymap.ActionBack:
		m.clearFilter()
		return m, nil
	case keymap.ActionUp, keymap.ActionDown, keymap.ActionTop, keymap.ActionBottom,
		keymap.ActionHalfPageUp, keymap.ActionHalfPageDown, keymap.ActionPageUp, keymap.ActionPageDown:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(a)
		return m, cmd
	}
	return m, nil
}

// commitFilter closes the filter input while keeping the narrowed view: normal key
// routing resumes so nav (j/k) and search (n/N) step through the matching rows, and
// the status bar keeps the "/query" indicator until the filter is cleared (esc).
func (m Model) commitFilter() (tea.Model, tea.Cmd) {
	m.filtering = false
	m.filterInput.Blur()
	m.syncFilterStatus()
	return m, nil
}

// clearFilter removes any active filter and closes the input, returning the table to
// its full row set (D78) and the status bar to its normal content. Safe to call with
// no filter set. It is esc's behaviour both while editing (clears-then-closes) and
// on a committed filter (clears the applied narrowing).
func (m *Model) clearFilter() {
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.Reset()
	m.table.ClearFilter()
	m.syncFilterStatus()
}

// searchMove steps the table selection to the next (nav.down) or previous (nav.up)
// match. With a narrowing filter the displayed rows are exactly the matches (D78),
// so "next/prev match" is the next/previous displayed row, wrapping at the ends
// (vim search wraps). With no active filter there is nothing to iterate and it is a
// no-op — n/N mean something only once a filter is set (Dn, this leg's decision).
func (m Model) searchMove(dir keymap.Action) (tea.Model, tea.Cmd) {
	if m.table.Filter() == "" {
		return m, nil
	}
	if dir == keymap.ActionDown {
		m.table.SelectNextWrap()
	} else {
		m.table.SelectPrevWrap()
	}
	return m, nil
}

// syncHints refreshes the persistent bottom key-hint to match what currently holds
// focus (the dogfood ask, feedback 2026-07-21-06): the menu-context keys while the
// left resource menu is focused, the table-context keys (filter/search, back) once
// the right table is. The hint stays registry-generated (D11) — this only picks the
// focus context. It feeds the dedicated hintbar line (FB-hintbar-dedicated), not
// the status bar, so the live state and the hint never compete for one row. Call it
// wherever focus switches, and on resize (the help renderer elides the hint to the
// current width).
func (m *Model) syncHints() {
	ctx := keymap.HelpMenu
	if m.table.Focused() {
		ctx = keymap.HelpTable
	}
	m.hintbar.SetHint(m.help.ShortHelpContextView(ctx))
}

// syncFilterStatus reflects the current filter state on the status bar: the live
// input prompt while editing, the committed "/query" indicator while a filter is
// applied but the input is closed, and nothing when no filter is set.
func (m *Model) syncFilterStatus() {
	switch {
	case m.filtering:
		m.status.SetFilter(m.filterInput.View())
	case m.table.Filter() != "":
		m.status.SetFilter("/" + m.table.Filter())
	default:
		m.status.SetFilter("")
	}
}

// Mouse support is additive — the keyboard/vim path stays primary (goals
// principle 6) — so it covers only the two headline gestures the dogfood feedback
// (2026-07-21-08) asked for: click a menu item to open it, click a table row to
// select it, and scroll-wheel to move through whichever pane the pointer is over.
// Clicks and wheel notches are turned into the same keymap Actions the keyboard
// produces (nav.up/down, drill-in), so no view gains raw mouse behaviour and the
// selection/scroll/drill-in logic stays single-sourced (D11 in spirit). Mouse mode
// is enabled per-View (View sets MouseModeCellMotion, as it sets AltScreen).

// overlayActive reports whether a modal/overlay is capturing input (help overlay,
// namespace picker, or the live filter field). Mouse events are inert while one is
// up so a click cannot reach and mutate the panes underneath it.
func (m Model) overlayActive() bool {
	return m.help.Visible() || m.nsPicker.Active() || m.filtering
}

// bodyHeight is the height of the two-pane body above the status bar and hint line
// — the region mouse clicks map within; a click on the status-bar/hint lines (or
// off-screen) is ignored.
func (m Model) bodyHeight() int {
	h := m.height - statusBarHeight - hintBarHeight
	if h < 0 {
		return 0
	}
	return h
}

// inMenu reports whether the absolute column x falls in the left menu pane (vs the
// right table/welcome pane), using the same split resize() computes.
func (m Model) inMenu(x int) bool {
	return x < menuPaneWidth(m.width)
}

// handleMouseWheel scrolls the pane under the pointer: a wheel notch steps the
// selection up/down (nav.up/nav.down) in whichever pane the pointer is over,
// reusing the keyboard navigation path (each pane's scroll offset follows its
// cursor, so a notch scrolls the viewport). It never changes which pane is focused
// — scrolling is a read gesture. Inert over the welcome page's empty table and
// while an overlay is up.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.overlayActive() {
		return m, nil
	}
	var a keymap.Action
	switch msg.Button {
	case tea.MouseWheelUp:
		a = keymap.ActionUp
	case tea.MouseWheelDown:
		a = keymap.ActionDown
	default:
		return m, nil
	}
	var cmd tea.Cmd
	if m.inMenu(msg.X) {
		m.menu, cmd = m.menu.Update(a)
		return m, cmd
	}
	if m.hasCurrent {
		m.table, cmd = m.table.Update(a)
	}
	return m, cmd
}

// handleMouseClick resolves a left click to the two headline gestures: a click in
// the menu pane selects that resource row and opens it (drill-in — the same
// ResourceSelectedMsg/NamespaceRequestedMsg path a keyboard drill-in takes); a
// click in the table pane selects that row and moves focus there. Non-left buttons,
// clicks on the status-bar line, and clicks while an overlay is up are ignored;
// clicks that land on a border, header, or blank filler resolve to no row and are
// no-ops.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.overlayActive() || msg.Button != tea.MouseLeft {
		return m, nil
	}
	if msg.Y < 0 || msg.Y >= m.bodyHeight() {
		return m, nil // the status-bar line (or off-screen); nothing to select.
	}
	if m.inMenu(msg.X) {
		return m.clickMenu(msg.Y)
	}
	return m.clickTable(msg.Y)
}

// clickMenu selects the resource row under the click and opens it. The click's
// body-relative Y maps to a content row inside the pane border (the first content
// line sits one row below the top border), which the menu resolves to an item
// index; the root then drives the normal SelectItem + drill-in path, so a click and
// a keyboard drill-in are one behaviour (drilling into a resource starts its watch
// and moves focus to the table; the seam row opens the namespace picker). A click
// on a header, the border, or blank space resolves to no item and is a no-op.
func (m Model) clickMenu(y int) (tea.Model, tea.Cmd) {
	idx, ok := m.menu.RowItemAt(y - 1)
	if !ok {
		return m, nil
	}
	m.menu.SelectItem(idx)
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(keymap.ActionDrillIn)
	return m, cmd
}

// clickTable selects the row under the click and focuses the table. With no
// resource open (the welcome page is showing) there is nothing to select. The
// body-relative Y maps to a content row inside the border where row 0 is the column
// header, so only a click on a data row selects; a click on the header or blank
// filler is a no-op.
func (m Model) clickTable(y int) (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	idx, ok := m.table.RowAt(y - 1)
	if !ok {
		return m, nil
	}
	m.table.SelectRow(idx)
	m.menu.Blur()
	m.table.Focus()
	m.syncHints() // focus is now the table → table-context hints
	return m, nil
}

// resize lays the panes out inside the current terminal: the status bar and the
// dedicated hint line take the two bottom rows, and the menu and table split the
// remaining width (menu a fraction with floors so the table always keeps room).
// Both panes are sized to their total width/height including border, as their
// SetSize expects.
func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	bodyH := m.height - statusBarHeight - hintBarHeight
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
	// The welcome page stands in for the table until a resource is drilled into, so
	// it takes the same right-pane geometry.
	m.welcome.SetSize(tableW, bodyH)
	// The picker and the help overlay both overlay the body area (above the status
	// bar) and center themselves within it, so the status line stays visible below
	// the modal.
	m.nsPicker.SetSize(m.width, bodyH)
	m.help.SetHeight(bodyH)
}

// menuPaneWidth is the menu pane's total width for a given terminal width: a
// quarter of the screen, floored at minMenuWidth and capped at maxMenuWidth so a
// wide terminal keeps the pane close to the menu's content instead of over-wide,
// but never so wide the table pane drops below minTableWidth (on a narrow
// terminal the two split evenly).
func menuPaneWidth(total int) int {
	if total <= 0 {
		return 0
	}
	w := total / 4
	if w < minMenuWidth {
		w = minMenuWidth
	}
	if w > maxMenuWidth {
		w = maxMenuWidth
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
// back) are serviced first; while the help overlay is open it swallows navigation
// so the panes underneath do not move. nav.back (esc) unwinds one level per press
// (help → committed filter → table focus back to the menu). Otherwise the action
// is routed to the focused pane, with nav.left/nav.right also switching focus
// between panes.
func (m Model) handleAction(a keymap.Action) (tea.Model, tea.Cmd) {
	switch a {
	case keymap.ActionQuit:
		// While the help modal is open, quit dismisses the modal instead of the
		// app — a modal owns the quit key until it closes (esc / `?` / `q` all
		// dismiss it), matching the feedback that help is a popup, not a page.
		if m.help.Visible() {
			m.help.SetVisible(false)
			return m, nil
		}
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
		// esc is the one-level-back key, resolved top-down, one level per press:
		// close the help overlay if open; else clear a committed table filter
		// (leaving the filtered view — the live-editing esc is handled in
		// routeFilterKey); else, with the table focused, pop focus back to the left
		// menu pane (the "back to the menu" gesture). Inert when the menu already
		// holds focus and nothing is open.
		if m.help.Visible() {
			m.help.SetVisible(false)
			return m, nil
		}
		if m.table.Filter() != "" {
			m.clearFilter()
			return m, nil
		}
		if m.table.Focused() {
			m.table.Blur()
			m.menu.Focus()
			m.syncHints() // back to the menu → menu-context hints
		}
		return m, nil
	}
	if m.help.Visible() {
		return m, nil // the overlay swallows navigation while it is open.
	}
	switch a {
	case keymap.ActionNamespace:
		return m.openNamespacePicker()
	case keymap.ActionFilter:
		return m.openFilter()
	case keymap.ActionSearchNext:
		return m.searchMove(keymap.ActionDown)
	case keymap.ActionSearchPrev:
		return m.searchMove(keymap.ActionUp)
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
			m.syncHints() // focus back to the menu → menu-context hints
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
		m.syncHints() // focus to the table → table-context hints
		return m, nil
	}
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(a)
	return m, cmd
}

// View implements tea.Model. Until the first WindowSizeMsg it renders nothing so
// the layout is never sized to a zero terminal. Normally it lays the menu and
// table panes side by side over the status bar and the dedicated key-hint line;
// when the help overlay is open it takes the body area, the status bar and hint
// line staying pinned below.
//
// Every returned view sets AltScreen: in bubbletea v2 full-screen mode is a
// property of the View (v.AltScreen), not a program option — the v1-era
// tea.WithAltScreen() no longer exists — so the root model, which owns View, is
// where kubecom requests the alternate screen buffer (D70).
// browseBody renders the two-pane browse layout: the resource menu (left) beside
// the resource table (right). Until the user drills into a resource the right pane
// shows the welcome page rather than a blank table; once a watch is live
// (hasCurrent) the live table takes over the slot. The right pane's focus
// (menu-vs-table focus switch) drives whichever stand-in is shown. This is the
// base an open modal is composited over (View / overlayCenter, D95).
func (m Model) browseBody() string {
	right := m.table.View()
	if !m.hasCurrent {
		right = m.welcome.View(m.table.Focused())
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.menu.View(), right)
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	// The two-pane browse view is always drawn first; an open modal is composited
	// centered on top of it (overlayCenter, D95) rather than replacing it, so the
	// menu + table stay visible underneath the popup (feedback
	// 2026-07-22-popups-should-overlay).
	body := m.browseBody()
	switch {
	case m.help.Visible():
		body = overlayCenter(body, m.help.View(), m.width, m.bodyHeight())
	case m.nsPicker.Active():
		body = overlayCenter(body, m.nsPicker.View(), m.width, m.bodyHeight())
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, m.status.View(), m.hintbar.View()))
	v.AltScreen = true
	// Enable mouse (click + wheel) the same way AltScreen is enabled — a per-View
	// property in bubbletea v2, not a program option. The root model owns View, so
	// it is where kubecom requests mouse reporting (dogfood-08).
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
