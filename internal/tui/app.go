package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/hintbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/statusbar"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/table"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/viewer"
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

// YAMLGetter is the narrow slice of the kube layer the shell needs to open the
// YAML viewer (M3-03): fetch a table row's object rendered as YAML (M1-07a's
// GetYAML). *kube.Clients satisfies it. As with the other seams the shell depends
// on this interface, not the concrete client, so the tui package never constructs
// a client and the model is driveable in hermetic tests with a fake getter. A
// model built without one (the default) is yaml-viewer-inert: the res.yaml action
// is a no-op (the viewer never opens), which is what the pre-wiring app and the
// non-viewer tests want.
type YAMLGetter interface {
	GetYAML(ctx context.Context, r kube.Resource, ref kube.ObjectRef) (string, error)
}

// Describer is the narrow slice of the kube layer the shell needs to open the
// describe viewer (M3-04): render a table row's object as `kubectl describe`
// output (M1-07b's Describe). *kube.Clients satisfies it. As with YAMLGetter the
// shell depends on this interface, not the concrete client, so the tui package
// never constructs a client and the model is driveable in hermetic tests with a
// fake describer. A model built without one (the default) is describe-viewer-inert:
// the res.describe action is a no-op (the viewer never opens), which is what the
// pre-wiring app and the non-viewer tests want. Unlike GetYAML, Describe takes no
// context — kubectl's describe package exposes no context-aware entry point (D2's
// describe note), so the seam matches that shape and the shell abandons a stale
// result via the viewerGen guard rather than cancellation.
type Describer interface {
	Describe(r kube.Resource, ref kube.ObjectRef) (string, error)
}

// LogStreamer is the narrow slice of the kube layer the shell needs to open the
// logs viewer (M3-05): stream a pod's logs onto a channel until the passed context
// is cancelled or the stream ends (M1-07c's Logs). *kube.Clients satisfies it. As
// with the other viewer seams the shell depends on this interface, not the concrete
// client, so the tui package never constructs a client and the model is driveable in
// hermetic tests with a fake streamer. A model built without one (the default) is
// logs-viewer-inert: the res.logs action is a no-op (the viewer never opens), which
// is what the pre-wiring app and the non-viewer tests want. Unlike the one-shot YAML
// and describe seams the result is a channel the shell pumps line by line (D53), so a
// large or slow log never blocks the update loop; a cancellable context tears the
// stream's goroutine down when the viewer closes or a newer viewer supersedes it.
type LogStreamer interface {
	Logs(ctx context.Context, ref kube.ObjectRef, opts kube.LogOptions) (<-chan kube.LogEvent, error)
}

// ContainerLister is the narrow slice of the kube layer the shell needs to resolve a
// pod's containers before streaming its logs (M3-07a): a multi-container pod must
// prompt which container to read (`kubectl logs` requires -c to disambiguate), while
// a single-container pod streams directly. *kube.Clients satisfies it via
// PodContainers. Without it wired the shell falls back to streaming the pod's
// default/sole container (the M3-05/06 behaviour, empty LogOptions.Container) — the
// container picker is simply not offered, which keeps the pre-wiring app and the
// non-picker hermetic tests inert without needing the extra seam.
type ContainerLister interface {
	PodContainers(ctx context.Context, ref kube.ObjectRef) ([]string, error)
}

// PodResolver is the narrow slice of the kube layer the shell needs to stream logs
// for a pod-owning workload kind (Deployment/ReplicaSet/StatefulSet/DaemonSet/Job/
// ReplicationController): it resolves the workload to a backing pod (its selector →
// the newest ready pod), which the shell then feeds into the same container
// resolution/stream path a pod row takes (M3-07b, #84). *kube.Clients satisfies it
// via PodForOwner. Without it wired logs on a non-pod kind degrade to a toast (the
// M3-05…07a behaviour), so the pre-wiring app and the non-resolver hermetic tests
// stay inert.
type PodResolver interface {
	PodForOwner(ctx context.Context, res kube.Resource, ref kube.ObjectRef) (kube.ObjectRef, error)
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

// WithYAMLGetter wires the kube client the shell uses to fetch an object's YAML
// for the read-only YAML viewer (M3-03). Without it the res.yaml action is inert
// (the viewer never opens).
func WithYAMLGetter(g YAMLGetter) Option {
	return func(m *Model) { m.yamlGetter = g }
}

// WithDescriber wires the kube client the shell uses to render an object's describe
// output for the read-only describe viewer (M3-04). Without it the res.describe
// action is inert (the viewer never opens).
func WithDescriber(d Describer) Option {
	return func(m *Model) { m.describer = d }
}

// WithLogStreamer wires the kube client the shell uses to stream a pod's logs into
// the read-only logs viewer (M3-05). Without it the res.logs action is inert (the
// viewer never opens).
func WithLogStreamer(s LogStreamer) Option {
	return func(m *Model) { m.logStreamer = s }
}

// WithContainerLister wires the kube client the shell uses to resolve a pod's
// containers so a multi-container pod prompts which to stream (M3-07a). Without it
// the logs viewer streams the pod's default/sole container directly (no picker).
func WithContainerLister(l ContainerLister) Option {
	return func(m *Model) { m.containerLister = l }
}

// WithPodResolver wires the kube client the shell uses to resolve a backing pod for
// a pod-owning workload kind, so its logs can be streamed (M3-07b). Without it logs
// on a non-pod kind degrade to a toast (the M3-05…07a behaviour).
func WithPodResolver(r PodResolver) Option {
	return func(m *Model) { m.podResolver = r }
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

// Layout constants. The status bar takes one line at the top (feedback
// 2026-07-22-status-bar-top) and the dedicated key-hint line one at the bottom;
// the two browse panes split the width between them, the menu (left) sized as a
// fraction with sensible floors so the table (right) always keeps room.
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

	menu      menu.Model
	table     table.Model
	status    statusbar.Model
	hintbar   hintbar.Model
	nsPicker  picker.Model
	resPicker picker.Model
	actPicker picker.Model
	ctrPicker picker.Model
	viewer    viewer.Model
	welcome   welcome.Model

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

	// yamlGetter fetches a row's object as YAML for the read-only viewer (M3-03; nil
	// → the res.yaml action is inert, the viewer never opens). viewerGen tags each
	// viewer open so an async fetch (yamlLoadedMsg) that returns after the user closed
	// the viewer, or opened a newer one, is dropped rather than populating the wrong
	// content — the same stale-message guard watchGen/seqGen give their async work.
	// describer renders an object's describe output for the same shared viewer (M3-04;
	// nil → the res.describe action is inert). Both viewer fetches share viewerGen: the
	// viewer is one component, so opening either kind bumps the generation and drops any
	// other in-flight fetch (a describe open supersedes a pending YAML fetch and vice
	// versa).
	yamlGetter YAMLGetter
	describer  Describer
	viewerGen  int

	// logStreamer streams a pod's logs into the same shared viewer (M3-05; nil → the
	// res.logs action is inert). Unlike the one-shot YAML/describe fetches a log stream
	// is a channel pumped line by line (D53): logCh is re-read to pull the next line and
	// logCancel tears the stream's goroutine down when the viewer closes or a newer
	// viewer supersedes it. Each pumped line rides the shared viewerGen (a logMsg), so a
	// line from a superseded stream — one whose viewer was closed or replaced — is
	// dropped rather than appended to the wrong content, exactly as watchGen guards the
	// table watch. logCh/logCancel are touched only from the single-threaded update loop.
	logStreamer LogStreamer
	logCh       <-chan kube.LogEvent
	logCancel   context.CancelFunc

	// containerLister resolves a pod's containers before streaming (M3-07a; nil → the
	// logs viewer streams the default/sole container directly, no picker). When wired,
	// opening logs on a pod first fetches its container names: a single container
	// streams directly, multiple open ctrPicker so the user chooses which to stream.
	// logStreamRes/logStreamRef stash the pod the picker's selection streams — the
	// picker's SelectedMsg carries only the chosen container name (D65), so the object
	// it applies to is held here between the picker opening and the pick landing. Touched
	// only from the single-threaded update loop.
	containerLister ContainerLister
	logStreamRes    kube.Resource
	logStreamRef    kube.ObjectRef

	// podResolver resolves a backing pod for a pod-owning workload kind so its logs
	// can be streamed (M3-07b; nil → logs on a non-pod kind degrade to a toast). When
	// wired, opening logs on a Deployment/RS/StatefulSet/DaemonSet/Job/RC first
	// resolves it to a pod off the update loop, then feeds that pod into the same
	// container resolution/stream path a pod row takes. Touched only from the
	// single-threaded update loop.
	podResolver PodResolver

	// logFollow is whether the open logs viewer is following (M3-06): the stream is
	// opened with LogOptions{Follow:true} so it stays open and reconnects (M1-07d),
	// and while logFollow is true each appended line snaps the viewport to the bottom
	// (viewer.GotoBottom) so the newest output is always shown. logs.follow (`f`)
	// toggles it inside the viewer; a manual up-scroll pauses it (so history can be
	// read without being yanked back down), and re-enabling snaps to the bottom.
	// logTitle is the base viewer title (without the follow marker) so the toggle can
	// re-render "[following]"/"[paused]" without re-deriving the object ref. Both are
	// consulted only while the logs viewer is up (viewerKindLogs), so a stale value
	// left from a closed logs viewer is harmless. Touched only from the update loop.
	logFollow bool
	logTitle  string

	// resByLabel maps each entry of the resource command palette (resPicker) back to
	// its kube.Resource. The picker is generic over strings (D65), so the palette
	// lists resource titles and this map, rebuilt each time the palette opens from the
	// menu's current item set (openResourcePicker), resolves the picked title to the
	// resource selectResource watches (FB-nav-resource-palette). It holds no shared
	// mutable state — only the update loop touches it.
	resByLabel map[string]kube.Resource

	// actByLabel maps each entry of the actions menu (actPicker) back to its
	// rowAction, rebuilt each time the menu opens from the actions applicable to the
	// browsed kind (openActionsMenu). Mirrors resByLabel for the resource palette
	// (D107); only the update loop touches it.
	actByLabel map[string]rowAction

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

	// menuHidden gates the left resource-menu pane (FB-nav-menu-toggle, D96's first
	// navigation slice). It is false by default (the menu shows). menu.toggle
	// (default `m`) flips it at runtime: when hidden the table (+ top status bar)
	// take the full width and focus lives on the table (a hidden pane can't hold
	// focus); the same key re-shows the menu, so it is never a one-way door even
	// before the pane-free command-palette resource switch (FB-nav-resource-palette)
	// lands. resize()/inMenu()/browseBody() treat a hidden menu as zero-width. It is
	// touched only from the single-threaded update loop, so no shared mutable state
	// (principle 1), exactly like filtering/mouseEnabled.
	menuHidden bool

	// mouseEnabled gates mouse reporting (D97). It is false by default so the
	// terminal keeps its own click-drag select-to-copy — capturing the mouse
	// (MouseModeCellMotion) makes the terminal send events to the app instead, which
	// broke native selection (feedback 2026-07-22-text-selection-select-to-copy).
	// mouse.toggle (default `M`) flips it at runtime; View only sets MouseMode when
	// it is true, and the status bar shows a `mouse` marker while on. The D86 mouse
	// handlers are unchanged — they simply receive no events until it is enabled.
	mouseEnabled bool

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
		resPicker:   picker.New(s, "resource"),
		actPicker:   picker.New(s, actionPickerKind),
		ctrPicker:   picker.New(s, containerPickerKind),
		viewer:      viewer.New(s, viewerKindYAML),
		welcome:     welcome.New(s),
		filterInput: fi,
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.resPicker.SetTitle("Switch resource")
	m.actPicker.SetTitle("Actions")
	m.ctrPicker.SetTitle("Container")
	m.menu.AddExtras(m.menuExtras) // fold in the per-context menu customizations (D83); no-op when none
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
		if m.activePicker() != nil {
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
		switch msg.Kind {
		case resourcePickerKind:
			return m.handleResourceSelected(msg)
		case actionPickerKind:
			return m.handleActionSelected(msg)
		case containerPickerKind:
			return m.handleContainerSelected(msg)
		default:
			return m.handleNamespaceSelected(msg)
		}

	case picker.CancelledMsg:
		switch msg.Kind {
		case resourcePickerKind:
			m.resPicker.Hide()
		case actionPickerKind:
			m.actPicker.Hide()
		case containerPickerKind:
			m.ctrPicker.Hide()
		default:
			m.nsPicker.Hide()
		}
		return m, nil

	case rowActionMsg:
		return m.handleRowAction(msg)

	case yamlLoadedMsg:
		return m.handleYAMLLoaded(msg)

	case describeLoadedMsg:
		return m.handleDescribeLoaded(msg)

	case podResolvedMsg:
		return m.handlePodResolved(msg)

	case containersLoadedMsg:
		return m.handleContainersLoaded(msg)

	case logMsg:
		return m.handleLogMsg(msg)

	case viewer.ClosedMsg:
		// The viewer dismissed itself (nav.back). Hide it, tear down any live log
		// stream feeding it, and return focus to the browse view underneath (the table
		// keeps whatever selection it had).
		m.viewer.Hide()
		m.stopLogStream()
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

	m.status.SetResourceType(r.GVK.Kind) // name the browsed kind on the top status bar
	m.menu.SetActive(r)                  // mark the opened resource distinctly from the nav cursor
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

// activePicker returns a pointer to whichever modal picker is currently open (the
// namespace switcher or the resource command palette), or nil when none is. At most
// one is ever active — opening one does not open the other — so the root can route
// input and composite the overlay through this single accessor rather than branching
// on each picker. The pointer aliases into the value-receiver copy, so mutations
// through it persist in the returned model exactly like a direct field assignment.
func (m *Model) activePicker() *picker.Model {
	switch {
	case m.nsPicker.Active():
		return &m.nsPicker
	case m.resPicker.Active():
		return &m.resPicker
	case m.actPicker.Active():
		return &m.actPicker
	case m.ctrPicker.Active():
		return &m.ctrPicker
	}
	return nil
}

// resourcePickerKind is the Kind stamped on the resource command palette's picker
// (picker.New(s, "resource")). Both the namespace switcher and the palette emit the
// same picker.SelectedMsg/CancelledMsg types (D65), so the root branches on this Kind
// to route a resolved palette selection to selectResource rather than the namespace
// path. An empty Kind (as the hermetic tests deliver) is treated as the namespace
// picker, keeping those tests unchanged.
const resourcePickerKind = "resource"

// openResourcePicker opens the resource command palette (FB-nav-resource-palette,
// D96's k9s `:`-style switch): a modal list of the browsable resource kinds, filtered
// with `/` and confirmed with Enter to switch the table to that kind — a pane-free way
// to change the browsed resource that does not need the left menu shown (it is what
// makes the toggled-off menu of D99 fully usable). The source list is the menu's own
// current item set (so discovered CRDs and per-context extras are included), filtered
// to the available resource rows; the namespace seam and unavailable rows are skipped,
// mirroring what a menu drill-in can act on. resByLabel is rebuilt from that snapshot
// so the picked title resolves back to its resource. With no watcher wired the model
// is watch-inert and switching a resource is a no-op, so the palette does not open.
func (m Model) openResourcePicker() (tea.Model, tea.Cmd) {
	if m.watcher == nil {
		return m, nil
	}
	items := m.menu.Items()
	labels := make([]string, 0, len(items))
	byLabel := make(map[string]kube.Resource, len(items))
	for _, it := range items {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		if _, dup := byLabel[it.Title]; dup {
			continue // a title collision would make the pick ambiguous — keep the first.
		}
		byLabel[it.Title] = it.Resource
		labels = append(labels, it.Title)
	}
	m.resByLabel = byLabel
	m.resPicker.SetItems(labels)
	m.resPicker.Show()
	return m, nil
}

// handleResourceSelected applies a resource picked from the command palette: it closes
// the palette and drives the same selectResource path a menu drill-in takes (start a
// watch for the kind, mark it active in the menu, move focus to the table). The picked
// title is resolved through resByLabel (built when the palette opened); a title with no
// mapping — the palette can only list titles it mapped, so this is defensive — closes
// the palette without switching.
func (m Model) handleResourceSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.resPicker.Hide()
	r, ok := m.resByLabel[msg.Value]
	if !ok {
		return m, nil
	}
	return m.selectResource(r)
}

// actionPickerKind is the Kind stamped on the actions menu's picker
// (picker.New(s, "action")). Every picker emits the same SelectedMsg/CancelledMsg
// types (D65), so the root branches on this Kind to route a picked action to
// dispatchRowAction rather than the namespace/resource paths.
const actionPickerKind = "action"

// openActionsMenu opens the M3 actions menu (D107): a picker listing the actions
// applicable to the browsed kind, over the selected table row. It is inert unless a
// resource table is showing (hasCurrent) with a row selected — the actions operate
// on a concrete object. The applicable titles come from the row-action registry
// (rowActionTitles), and actByLabel resolves the picked title back to its action.
// Picking one (or a direct key) dispatches a rowActionMsg the individual M3 legs
// handle; this leg only opens the menu and routes.
func (m Model) openActionsMenu() (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	if _, ok := m.table.SelectedRow(); !ok {
		return m, nil
	}
	titles, byTitle := rowActionTitles(m.current)
	if len(titles) == 0 {
		return m, nil
	}
	m.actByLabel = byTitle
	m.actPicker.SetItems(titles)
	m.actPicker.Show()
	return m, nil
}

// handleActionSelected applies an action picked from the actions menu: it closes the
// menu and dispatches the chosen row action's intent. The picked title is resolved
// through actByLabel (built when the menu opened); a title with no mapping — the
// menu can only list titles it mapped, so this is defensive — closes it without
// dispatching.
func (m Model) handleActionSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.actPicker.Hide()
	act, ok := m.actByLabel[msg.Value]
	if !ok {
		return m, nil
	}
	return m.dispatchRowAction(act)
}

// triggerRowActionKey handles a direct-key M3 action (describe/yaml/logs/edit/
// delete). It resolves the keymap.Action to its rowAction and dispatches it against
// the selected row — but only when the action applies to the browsed kind, so e.g.
// `L` (logs) on a non-pod kind is inert, exactly as the entry is absent from that
// kind's actions menu. Inert with no resource table open or no row selected.
func (m Model) triggerRowActionKey(a keymap.Action) (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	act, ok := keyToRowAction[a]
	if !ok || !rowActionApplies(act, m.current) {
		return m, nil
	}
	return m.dispatchRowAction(act)
}

// dispatchRowAction emits the typed rowActionMsg intent for act on the selected
// row's object (D107). It is a no-op with no resource table open or no row selected.
// The intent is carried as a Cmd (not applied inline) so the routing is uniform for
// both entry points (a direct key and an actions-menu pick) and each later M3 leg
// handles its intent in one place.
func (m Model) dispatchRowAction(act rowAction) (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	row, ok := m.table.SelectedRow()
	if !ok {
		return m, nil
	}
	intent := rowActionMsg{Action: act, Resource: m.current, Object: row.Object}
	return m, func() tea.Msg { return intent }
}

// handleRowAction dispatches a row action to its handler. Each M3 leg wires its own
// intent here (M3-03: YAML); the actions not yet wired fall through to a transient
// status-bar toast naming the action and target, so the routing stays observable and
// the dogfooder sees the action was recognised rather than the key seeming dead (D68)
// until its leg lands (D107).
func (m Model) handleRowAction(msg rowActionMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case rowActionYAML:
		return m.openYAMLViewer(msg)
	case rowActionDescribe:
		return m.openDescribeViewer(msg)
	case rowActionLogs:
		return m.openLogsViewer(msg)
	}
	label := rowActionTitle(msg.Action)
	if msg.Object.Name != "" {
		label += " " + msg.Object.Name
	}
	return m, m.surfaceError(ErrorMsg{Context: label + ": not yet available"})
}

// viewerKind* are the kinds stamped on the shared read-only viewer for the content it
// is showing. The M3 viewers all reuse one viewer.Model; each open path restamps the
// kind (viewer.SetKind) so the kind rides ClosedMsg for routing and the shell can gate
// kind-specific behaviour — the M3-06 follow toggle acts only while viewerKindLogs is
// up. The shell still hides the viewer uniformly on close regardless of kind.
const (
	viewerKindYAML     = "yaml"
	viewerKindDescribe = "describe"
	viewerKindLogs     = "logs"
)

// yamlLoadedMsg carries the outcome of the async GetYAML fetch issued when the YAML
// viewer opens (M3-03). gen ties it to the viewer open that requested it, so a fetch
// that lands after the user closed the viewer (or opened a newer one) is dropped
// rather than populating stale content (the watchGen/seqGen stale-message guard).
type yamlLoadedMsg struct {
	gen     int
	content string
	err     error
}

// openYAMLViewer opens the read-only YAML viewer over the selected row's object
// (M3-03): it shows the viewer immediately (empty, so the gesture feels instant) and
// kicks off the GetYAML fetch off the update loop, seeding the content when it lands.
// With no getter wired it is yaml-viewer-inert (a no-op). The fetch is tagged with a
// fresh viewerGen so a superseded/stale result is dropped (handleYAMLLoaded). A fetch
// error degrades to a status-bar toast and closes the viewer (D74) rather than
// leaving an empty box.
func (m Model) openYAMLViewer(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.yamlGetter == nil {
		return m, nil
	}
	m.stopLogStream() // a new viewer supersedes any in-flight log stream.
	m.viewerGen++
	gen := m.viewerGen
	m.viewer.SetKind(viewerKindYAML)
	m.viewer.SetTitle(viewerTitle(msg.Resource, msg.Object))
	m.viewer.SetContent("") // clear any prior object's YAML before the fetch lands.
	m.viewer.Show()
	getter := m.yamlGetter
	r, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		content, err := getter.GetYAML(context.Background(), r, ref)
		return yamlLoadedMsg{gen: gen, content: content, err: err}
	}
}

// handleYAMLLoaded seeds the open viewer with the fetched YAML. A result whose gen no
// longer matches (a newer open superseded it) or that arrives after the viewer closed
// is dropped. A fetch error degrades: it closes the viewer and surfaces a transient
// status-bar toast (D74), never breaking the layout or leaving an empty box.
func (m Model) handleYAMLLoaded(msg yamlLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen || !m.viewer.Active() {
		return m, nil
	}
	if msg.err != nil {
		m.viewer.Hide()
		return m, m.surfaceError(NewErrorMsg("get yaml", msg.err))
	}
	m.viewer.SetContent(msg.content)
	return m, nil
}

// describeLoadedMsg carries the outcome of the async Describe render issued when the
// describe viewer opens (M3-04). gen ties it to the viewer open that requested it, so
// a render that lands after the user closed the viewer (or opened a newer one — of
// either kind) is dropped rather than populating stale content (the viewerGen guard,
// mirroring yamlLoadedMsg).
type describeLoadedMsg struct {
	gen     int
	content string
	err     error
}

// openDescribeViewer opens the read-only describe viewer over the selected row's
// object (M3-04): it shows the viewer immediately (empty, so the gesture feels
// instant) and kicks off the Describe render off the update loop, seeding the content
// when it lands. With no describer wired it is describe-viewer-inert (a no-op). The
// render is tagged with a fresh viewerGen so a superseded/stale result is dropped
// (handleDescribeLoaded). A render error degrades to a status-bar toast and closes the
// viewer (D74) rather than leaving an empty box. Describe takes no context (D2's
// describe note), so — unlike GetYAML — there is nothing to thread through; the
// generation guard is the sole staleness defence.
func (m Model) openDescribeViewer(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.describer == nil {
		return m, nil
	}
	m.stopLogStream() // a new viewer supersedes any in-flight log stream.
	m.viewerGen++
	gen := m.viewerGen
	m.viewer.SetKind(viewerKindDescribe)
	m.viewer.SetTitle(viewerTitle(msg.Resource, msg.Object))
	m.viewer.SetContent("") // clear any prior object's content before the render lands.
	m.viewer.Show()
	describer := m.describer
	r, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		content, err := describer.Describe(r, ref)
		return describeLoadedMsg{gen: gen, content: content, err: err}
	}
}

// handleDescribeLoaded seeds the open viewer with the rendered describe output. A
// result whose gen no longer matches (a newer open superseded it) or that arrives
// after the viewer closed is dropped. A render error degrades: it closes the viewer
// and surfaces a transient status-bar toast (D74), never breaking the layout or
// leaving an empty box.
func (m Model) handleDescribeLoaded(msg describeLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen || !m.viewer.Active() {
		return m, nil
	}
	if msg.err != nil {
		m.viewer.Hide()
		return m, m.surfaceError(NewErrorMsg("describe", msg.err))
	}
	m.viewer.SetContent(msg.content)
	return m, nil
}

// logMsg wraps one message from the log pump with the viewerGen of the viewer open
// that started the stream. The model tags every pumped line this way so a line from a
// stream already superseded — its viewer closed, or a newer viewer (of any kind)
// opened and bumped viewerGen — can be dropped rather than appended to the current
// content, and its pump chain stopped (mirroring watchMsg's stale-delta guard). gen
// must equal the model's viewerGen or the message is ignored.
type logMsg struct {
	gen int
	msg tea.Msg
}

// containerPickerKind is the Kind stamped on the logs container picker
// (picker.New(s, "container")). Every picker emits the same SelectedMsg/CancelledMsg
// types (D65), so the root branches on this Kind to route a picked container into
// streamLogsInto rather than the namespace/resource/action paths.
const containerPickerKind = "container"

// containersLoadedMsg carries the outcome of the async PodContainers fetch issued
// when logs are opened over a pod with a container lister wired (M3-07a). gen ties it
// to the viewerGen bumped when the fetch was requested, so a result that lands after
// the user opened a newer viewer (of any kind, which bumps viewerGen) is dropped
// rather than opening a stale stream. res/ref are the pod the fetch was for, threaded
// back so the single-container fast path and the multi-container picker act on it.
type containersLoadedMsg struct {
	gen        int
	res        kube.Resource
	ref        kube.ObjectRef
	containers []string
	err        error
}

// podLogResource labels the logs viewer for a pod resolved from a pod-owning
// workload (M3-07b): the stream and title show the resolved *pod*, not the workload,
// so the user sees which pod is tailing. It carries only the Pod kind — viewerTitle
// reads GVK.Kind — since the resolved pod's namespace/name come from its ObjectRef.
var podLogResource = kube.Resource{GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"}}

// podResolvedMsg carries the outcome of the async PodForOwner resolution issued when
// logs are opened over a pod-owning workload kind (M3-07b). gen ties it to the
// viewerGen bumped when the resolution was requested, so a result that lands after the
// user opened a newer viewer (of any kind) is dropped rather than opening a stale
// stream. ref is the resolved backing pod.
type podResolvedMsg struct {
	gen int
	ref kube.ObjectRef
	err error
}

// openLogsViewer starts the logs flow over the selected row's object (M3-05/06/07a/b).
// With no streamer wired it is logs-viewer-inert (a no-op). A pod streams directly; a
// pod-owning workload kind (Deployment/RS/StatefulSet/DaemonSet/Job/RC — the kinds the
// actions menu lists for logs) is first resolved to a backing pod (M3-07b), then that
// pod takes the same path. Resolution runs off the update loop tagged with a fresh
// viewerGen so a superseded request is dropped (handlePodResolved); with no resolver
// wired a non-pod kind degrades to a toast (the M3-05…07a behaviour).
func (m Model) openLogsViewer(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.logStreamer == nil {
		return m, nil
	}
	if msg.Resource.GVK.Kind == "Pod" {
		return m.resolveContainersFor(msg.Resource, msg.Object)
	}
	// Pod-owning workload kind: resolve a backing pod first (its container resolution
	// then applies to the resolved pod). Without a resolver wired keep the routing
	// observable with a toast rather than opening an empty viewer.
	if m.podResolver == nil {
		label := "logs for " + msg.Resource.GVK.Kind
		return m, m.surfaceError(ErrorMsg{Context: label + ": not yet available"})
	}
	m.stopLogStream() // cancel any prior stream before resolving/starting a new one.
	m.viewerGen++
	gen := m.viewerGen
	resolver := m.podResolver
	res, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		pod, err := resolver.PodForOwner(context.Background(), res, ref)
		return podResolvedMsg{gen: gen, ref: pod, err: err}
	}
}

// resolveContainersFor starts the container-resolution/stream flow over podRef (a pod
// of res) — the shared tail of openLogsViewer's pod path and the pod-owning resolution
// (M3-07b). It cancels any prior stream, then: with a container lister wired it fetches
// the pod's containers off the update loop (a fresh viewerGen so a superseded request
// is dropped — handleContainersLoaded), where a single container streams directly and
// multiple open the picker; with no lister wired it streams the pod's default/sole
// container directly (empty Container) — the M3-05/06 behaviour.
func (m Model) resolveContainersFor(res kube.Resource, podRef kube.ObjectRef) (tea.Model, tea.Cmd) {
	m.stopLogStream()
	if m.containerLister == nil {
		m.viewerGen++
		return m.streamLogsInto(res, podRef, "", m.viewerGen)
	}
	m.viewerGen++
	gen := m.viewerGen
	lister := m.containerLister
	return m, func() tea.Msg {
		names, err := lister.PodContainers(context.Background(), podRef)
		return containersLoadedMsg{gen: gen, res: res, ref: podRef, containers: names, err: err}
	}
}

// handlePodResolved acts on a backing pod resolved for a pod-owning kind (M3-07b). A
// result whose gen no longer matches (a newer viewer superseded it) is dropped; a
// resolution error (no matching/ready pod, RBAC denial) degrades to a status-bar toast
// (D74) without opening the viewer. On success it feeds the resolved pod into the same
// container-resolution/stream path a pod row takes, titled as a Pod so the user sees
// which pod is streaming.
func (m Model) handlePodResolved(msg podResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen {
		return m, nil // superseded by a newer viewer/stream open; drop.
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("logs", msg.err))
	}
	return m.resolveContainersFor(podLogResource, msg.ref)
}

// handleContainersLoaded acts on a resolved container set (M3-07a). A result whose gen
// no longer matches (a newer viewer superseded it) is dropped. A fetch error, or a pod
// that reports no containers, degrades to a status-bar toast (D74) without opening the
// viewer. A single container streams directly (reusing the fetch's gen so the stream is
// still guarded by the same generation); multiple open the container picker, stashing
// the pod so the pick knows what to stream.
func (m Model) handleContainersLoaded(msg containersLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen {
		return m, nil // superseded by a newer viewer/stream open; drop.
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("logs", msg.err))
	}
	switch len(msg.containers) {
	case 0:
		label := "logs for " + msg.ref.Name
		return m, m.surfaceError(ErrorMsg{Context: label + ": no containers"})
	case 1:
		return m.streamLogsInto(msg.res, msg.ref, msg.containers[0], msg.gen)
	default:
		m.logStreamRes = msg.res
		m.logStreamRef = msg.ref
		m.ctrPicker.SetItems(msg.containers)
		m.ctrPicker.Show()
		return m, nil
	}
}

// handleContainerSelected streams the container the user picked from the container
// picker (M3-07a): it closes the picker and opens the logs stream over the stashed pod
// with the chosen container, on a fresh viewerGen (the pick is a new open). A picked
// value applies to the pod recorded when the picker opened (logStreamRes/Ref).
func (m Model) handleContainerSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.ctrPicker.Hide()
	m.stopLogStream()
	m.viewerGen++
	return m.streamLogsInto(m.logStreamRes, m.logStreamRef, msg.Value, m.viewerGen)
}

// streamLogsInto opens the read-only logs viewer over ref (a pod of res) and starts
// streaming container's logs into the shared viewer at generation gen. It shows the
// viewer immediately (empty, so the gesture feels instant) and pumps the log channel
// line by line off the update loop (D53), appending each line as it lands — a large or
// slow log never blocks Update. The viewer opens following (like `kubectl logs -f`); a
// non-empty container is named in the title so the user sees which one is tailing. The
// stream runs on a cancellable context torn down when the viewer closes or a newer
// viewer supersedes it (stopLogStream); its lines are tagged with gen so a superseded
// stream's lines are dropped (handleLogMsg). An open failure degrades to a status-bar
// toast + closes the viewer (D74); a mid-stream error after some lines already showed
// leaves them on screen. container "" streams the pod's default/sole container.
func (m Model) streamLogsInto(res kube.Resource, ref kube.ObjectRef, container string, gen int) (tea.Model, tea.Cmd) {
	m.stopLogStream() // idempotent; ensures no prior stream survives this open.
	m.viewer.SetKind(viewerKindLogs)
	m.logFollow = true // the logs viewer opens following, like `kubectl logs -f`.
	m.logTitle = "Logs " + viewerTitle(res, ref)
	if container != "" {
		m.logTitle += " · " + container
	}
	m.syncLogViewerTitle()
	m.viewer.SetContent("") // clear any prior object's content before the stream lands.
	m.viewer.Show()

	ctx, cancel := context.WithCancel(context.Background())
	// Follow keeps the stream open and reconnects transparently across transport
	// drops (M1-07d), so the viewer tails live output; stopLogStream cancels it on
	// close/supersede/quit.
	ch, err := m.logStreamer.Logs(ctx, ref, kube.LogOptions{Follow: true, Container: container})
	if err != nil {
		cancel()
		m.viewer.Hide()
		return m, m.surfaceError(NewErrorMsg("logs", err))
	}
	m.logCancel = cancel
	m.logCh = ch
	return m, m.pumpLogs(gen)
}

// syncLogViewerTitle re-renders the logs viewer's title with a follow marker so the
// user always sees whether the log is tailing live ("[following]") or paused for
// scrollback ("[paused]"). It is a no-op-safe helper called on open and whenever
// follow toggles; it reads logTitle (the base, object-named title) so it never needs
// the object ref again.
func (m *Model) syncLogViewerTitle() {
	marker := " [paused]"
	if m.logFollow {
		marker = " [following]"
	}
	m.viewer.SetTitle(m.logTitle + marker)
}

// pumpLogs issues the tea.Cmd that pulls the next line from the current log channel,
// tagged with the viewer generation that started the stream so a line from a
// superseded viewer is recognisable as stale. It returns nil when no stream is active.
func (m Model) pumpLogs(gen int) tea.Cmd {
	ch := m.logCh
	if ch == nil {
		return nil
	}
	pump := logPump(ch)
	return func() tea.Msg { return logMsg{gen: gen, msg: pump()} }
}

// handleLogMsg applies one log-pump message to the viewer and re-issues the pump to
// pull the next line — the one-receive-per-Cmd loop that keeps Update from ever
// blocking (M2-02/D53). A message from a superseded stream (wrong gen) or one that
// arrives after the viewer closed is dropped and its chain stops. A line is appended
// preserving the scroll position (AppendContent); a closed channel ends the chain (the
// normal EOF of a non-following stream); a bridged stream error degrades — it surfaces
// a transient status-bar toast (D74) and closes the viewer only if nothing was shown
// yet (an open failure), leaving any partial lines on screen for a mid-stream drop.
func (m Model) handleLogMsg(l logMsg) (tea.Model, tea.Cmd) {
	if l.gen != m.viewerGen || !m.viewer.Active() {
		return m, nil // superseded viewer or closed; drop and stop this chain.
	}
	switch inner := l.msg.(type) {
	case LogLineMsg:
		m.viewer.AppendContent(inner.Line)
		if m.logFollow {
			m.viewer.GotoBottom() // follow mode tails the newest output (M3-06).
		}
		return m, m.pumpLogs(l.gen)
	case LogClosedMsg:
		m.stopLogStream() // stream ended (EOF); release the context, keep the lines shown.
		return m, nil
	case ErrorMsg:
		empty := m.viewer.Empty()
		m.stopLogStream()
		if empty {
			m.viewer.Hide() // nothing shown yet (an open failure) → close the empty box.
		}
		return m, m.surfaceError(inner)
	}
	return m, nil
}

// stopLogStream cancels the live log stream (if any) and clears its handles, so the
// stream's goroutine is torn down and no stale line is pumped. Safe to call with no
// stream active. Called before starting a new stream, when the viewer closes, and on
// quit — the log twin of the watch's cancel-on-reselect teardown.
func (m *Model) stopLogStream() {
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.logCh = nil
}

// viewerTitle labels the viewer with the browsed kind and the object's name
// (namespace-qualified when the object is namespaced), e.g. "Pod default/web-1" or
// "Node node-1", so the user always sees which object they are viewing.
func viewerTitle(r kube.Resource, ref kube.ObjectRef) string {
	name := ref.Name
	if ref.Namespace != "" {
		name = ref.Namespace + "/" + ref.Name
	}
	if kind := r.GVK.Kind; kind != "" {
		return kind + " " + name
	}
	return name
}

// routePickerKey resolves one keypress while a modal picker (namespace switcher or
// resource palette) is open. The picker captures all input (the panes and the
// sequencer never see it): a control key (esc/enter/arrows/page keys, and ctrl+d/u)
// resolves to an Action the picker consumes, while any text-producing or editing key
// is filter input routed to the field. The split is by whether the key carries text —
// a printable rune types, everything without text (incl. a bound vim letter like `j`)
// is a control action, and an unmapped no-text key (backspace) still reaches the
// filter for editing (D73). No view matches a raw key for behaviour (D11).
func (m Model) routePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.activePicker()
	if p == nil {
		return m, nil
	}
	key := msg.Key()
	action, mapped := m.keymap.Action(key)
	var cmd tea.Cmd
	if p.Filtering() {
		if mapped && key.Text == "" {
			*p, cmd = p.Update(action)
		} else {
			*p, cmd = p.UpdateFilter(msg)
		}
		return m, cmd
	}
	if mapped {
		*p, cmd = p.Update(action)
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

// sortNext advances the table's column sort one step through a single cycle driven
// by the sort.column key (default `s`), so every visible column and both directions
// are reachable without a separate column-selection gesture (there is no column
// cursor — the table sorts by a visible-column position, D94). The cycle, derived
// entirely from the table's own SortColumn/SortDescending state (no shared mutable
// UI state — principle 1), is: unsorted → column 0 ascending → column 0 descending
// → column 1 ascending → … → last column descending → unsorted (ClearSort). It is a
// no-op with no resource table open or no visible columns. Sort itself is a view
// over the authoritative row set (SortBy re-derives through applyFilter), so it
// never reorders full.Rows and survives watch deltas (D94).
func (m Model) sortNext() (tea.Model, tea.Cmd) {
	if !m.hasCurrent {
		return m, nil
	}
	n := m.table.VisibleColumnCount()
	if n == 0 {
		return m, nil
	}
	cur, sorted := m.table.SortColumn()
	switch {
	case !sorted:
		m.table.SortBy(0) // start at the first column, ascending
	case !m.table.SortDescending():
		m.table.SortBy(cur) // same column: ascending → descending (SortBy toggles)
	case cur+1 < n:
		m.table.SortBy(cur + 1) // advance to the next column, ascending
	default:
		m.table.ClearSort() // past the last column: back to the watch order
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
// selection/scroll/drill-in logic stays single-sourced (D11 in spirit). Mouse
// capture is opt-in and off by default (D97) so the terminal keeps its native
// select-to-copy; these handlers only receive events once mouse.toggle turns it on
// (View then sets MouseModeCellMotion, as it sets AltScreen).

// overlayActive reports whether a modal/overlay is capturing input (help overlay,
// namespace picker, or the live filter field). Mouse events are inert while one is
// up so a click cannot reach and mutate the panes underneath it.
func (m Model) overlayActive() bool {
	return m.help.Visible() || m.nsPicker.Active() || m.resPicker.Active() || m.actPicker.Active() || m.ctrPicker.Active() || m.viewer.Active() || m.filtering
}

// bodyHeight is the height of the two-pane body between the top status bar and the
// bottom hint line — the region mouse clicks map within; a click on the
// status-bar/hint lines (or off-screen) is ignored.
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
	if m.menuHidden {
		return false // no menu pane on screen — every click is over the table.
	}
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
	// The status bar occupies the top row now (feedback 2026-07-22-status-bar-top),
	// so the body starts one row down: map the screen Y to a body-relative Y before
	// resolving a row. A click on the status-bar line (y<0) or the hint line / below
	// (y>=bodyHeight) selects nothing.
	y := msg.Y - statusBarHeight
	if y < 0 || y >= m.bodyHeight() {
		return m, nil
	}
	if m.inMenu(msg.X) {
		return m.clickMenu(y)
	}
	return m.clickTable(y)
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

// toggleMenu shows or hides the left resource-menu pane (menu.toggle, D96's first
// navigation slice). Hiding it hands the full width to the table (+ top status
// bar) and moves focus to the table, since a hidden pane cannot hold focus;
// showing it again returns focus to the menu so the user can pick a resource. The
// same key re-shows the menu, so it is never a one-way door even before the
// command-palette resource switch (FB-nav-resource-palette) lands. The layout is
// re-split immediately (resize) and the focus-aware hint refreshed.
func (m Model) toggleMenu() (tea.Model, tea.Cmd) {
	m.menuHidden = !m.menuHidden
	if m.menuHidden {
		m.menu.Blur()
		m.table.Focus()
	} else {
		m.table.Blur()
		m.menu.Focus()
	}
	m.resize()
	m.syncHints()
	return m, nil
}

// resize lays the panes out inside the current terminal: the status bar and the
// dedicated hint line take the two bottom rows, and the menu and table split the
// remaining width (menu a fraction with floors so the table always keeps room).
// A hidden menu (menu.toggle) is zero-width, handing the full width to the table.
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
	if m.menuHidden {
		menuW = 0 // hidden menu: the table takes the full width.
	}
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
	m.resPicker.SetSize(m.width, bodyH)
	m.actPicker.SetSize(m.width, bodyH)
	m.ctrPicker.SetSize(m.width, bodyH)
	// The viewer is the large overlay; it too centers within the body area (above the
	// status bar) so the top status line and bottom hint line stay visible around it.
	m.viewer.SetSize(m.width, bodyH)
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
	// The read-only viewer (M3-03) captures input while it is up: it scrolls on
	// navigation and closes on nav.back/quit, and swallows everything else so the
	// browse panes underneath never move (mirroring the help modal's capture).
	if m.viewer.Active() {
		return m.handleViewerAction(a)
	}
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
		m.stopLogStream() // and any in-flight log stream.
		return m, tea.Quit
	case keymap.ActionHelp:
		m.help.Toggle()
		return m, nil
	case keymap.ActionToggleMouse:
		// Flip mouse capture (D97). Off (the default) leaves the terminal's own
		// select-to-copy working; on enables the D86 click/wheel gestures. View
		// reflects the new MouseMode next frame; the status bar shows the state so
		// this invisible mode is always visible.
		m.mouseEnabled = !m.mouseEnabled
		m.status.SetMouse(m.mouseEnabled)
		return m, nil
	case keymap.ActionToggleMenu:
		return m.toggleMenu()
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
	case keymap.ActionResources:
		return m.openResourcePicker()
	case keymap.ActionFilter:
		return m.openFilter()
	case keymap.ActionSearchNext:
		return m.searchMove(keymap.ActionDown)
	case keymap.ActionSearchPrev:
		return m.searchMove(keymap.ActionUp)
	case keymap.ActionSort:
		return m.sortNext()
	case keymap.ActionClearSort:
		if m.hasCurrent {
			m.table.ClearSort()
		}
		return m, nil
	case keymap.ActionActions:
		return m.openActionsMenu()
	case keymap.ActionDescribe, keymap.ActionYAML, keymap.ActionLogs,
		keymap.ActionEdit, keymap.ActionDelete:
		return m.triggerRowActionKey(a)
	}
	return m.routeNav(a)
}

// handleViewerAction routes a resolved action to the open read-only viewer (M3-03).
// Navigation scrolls the viewport; nav.back and app.quit both dismiss the viewer —
// nav.back via the viewer's own ClosedMsg (which the shell hides on), app.quit
// directly (a viewer is a transient pager overlay, so `q` closes it rather than
// exiting kubecom, exactly as the help modal owns quit while it is open). Every other
// action is swallowed so the browse view underneath stays put.
func (m Model) handleViewerAction(a keymap.Action) (tea.Model, tea.Cmd) {
	if a == keymap.ActionQuit {
		m.viewer.Hide()
		m.stopLogStream() // tear down any log stream feeding the viewer.
		return m, nil
	}
	// logs.follow (`f`) toggles follow while the logs viewer is up (M3-06); it is inert
	// on the YAML/describe viewers (nothing to follow). Re-enabling snaps to the bottom
	// so a re-followed log resumes tailing the newest line.
	if a == keymap.ActionLogsFollow {
		if m.viewer.Kind() == viewerKindLogs {
			m.logFollow = !m.logFollow
			if m.logFollow {
				m.viewer.GotoBottom()
			}
			m.syncLogViewerTitle()
		}
		return m, nil
	}
	// A manual up-scroll while following pauses follow (M3-06): the reader wants to
	// inspect earlier output without the next line yanking the viewport back to the
	// bottom. `f` (or nav.bottom's own scroll) resumes it. Down-scrolls keep following.
	if m.viewer.Kind() == viewerKindLogs && m.logFollow && isScrollUp(a) {
		m.logFollow = false
		m.syncLogViewerTitle()
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(a)
	return m, cmd
}

// isScrollUp reports whether a is an upward-scroll navigation action — the gestures
// that move away from the tail of a following log and so pause follow (M3-06).
func isScrollUp(a keymap.Action) bool {
	switch a {
	case keymap.ActionUp, keymap.ActionTop, keymap.ActionHalfPageUp, keymap.ActionPageUp:
		return true
	}
	return false
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
// the layout is never sized to a zero terminal. Normally it stacks the status bar
// (top), the two-pane body (menu + table side by side), and the dedicated key-hint
// line (bottom); when the help overlay is open it is composited over the body area,
// the status bar staying pinned above and the hint line below.
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
	if m.menuHidden {
		return right // hidden menu: the table (+ top status bar) fills the width.
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.menu.View(), right)
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		if m.mouseEnabled {
			v.MouseMode = tea.MouseModeCellMotion
		}
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
	case m.resPicker.Active():
		body = overlayCenter(body, m.resPicker.View(), m.width, m.bodyHeight())
	case m.actPicker.Active():
		body = overlayCenter(body, m.actPicker.View(), m.width, m.bodyHeight())
	case m.ctrPicker.Active():
		body = overlayCenter(body, m.ctrPicker.View(), m.width, m.bodyHeight())
	case m.viewer.Active():
		body = overlayCenter(body, m.viewer.View(), m.width, m.bodyHeight())
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, m.status.View(), body, m.hintbar.View()))
	v.AltScreen = true
	// Mouse reporting is a per-View property in bubbletea v2 (like AltScreen), not a
	// program option — the root model owns View, so it is where kubecom requests it.
	// It is opt-in (off by default) so the terminal keeps its native select-to-copy;
	// only when the user toggles mouse capture on does View request it, and switching
	// back to MouseModeNone tears the reporting down again (D97, superseding D86's
	// unconditional capture).
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
