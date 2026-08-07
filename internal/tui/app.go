package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/hintbar"
	"github.com/neuroplastio/kubecom/internal/tui/components/logsview"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
	"github.com/neuroplastio/kubecom/internal/tui/components/modal"
	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
	"github.com/neuroplastio/kubecom/internal/tui/components/searchview"
	"github.com/neuroplastio/kubecom/internal/tui/components/statusbar"
	"github.com/neuroplastio/kubecom/internal/tui/components/table"
	"github.com/neuroplastio/kubecom/internal/tui/components/viewer"
	"github.com/neuroplastio/kubecom/internal/tui/components/welcome"
	"github.com/neuroplastio/kubecom/internal/tui/help"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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

// YAMLGetter is the narrow slice of the kube layer the shell needs to fetch a table
// row's object rendered as YAML (M1-07a's GetYAML). It feeds the unified View/Edit
// YAML action (M3-15b/D135): openEdit fetches the object's YAML through it before
// suspending into $EDITOR. (The standalone read-only YAML viewer that first used
// this seam was retired into the edit flow in M3-15c.) *kube.Clients satisfies it.
// As with the other seams the shell depends on this interface, not the concrete
// client, so the tui package never constructs a client and the model is driveable in
// hermetic tests with a fake getter. A model built without one (the default) — or
// without an Editor — makes the View/Edit YAML action a no-op.
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
// PodContainers, which returns the init and ephemeral containers alongside the
// regular ones (LOGS-06) — the shell narrows the set by purpose. Without it wired the
// shell falls back to streaming the pod's default/sole container (the M3-05/06
// behaviour, empty LogOptions.Container) — the container picker is simply not
// offered, which keeps the pre-wiring app and the non-picker hermetic tests inert
// without needing the extra seam.
type ContainerLister interface {
	PodContainers(ctx context.Context, ref kube.ObjectRef) ([]kube.Container, error)
}

// SecretGetter is the narrow slice of the kube layer the shell needs to open the
// secret viewer (M3-08a): fetch a Secret's type and base64-decoded data (M1-07a's
// sibling, SecretData). *kube.Clients satisfies it. As with the other viewer seams
// the shell depends on this interface, not the concrete client, so the tui package
// never constructs a client and the model is driveable in hermetic tests with a
// fake getter. A model built without one (the default) is secret-viewer-inert: the
// Reveal-secret action is a no-op (the viewer never opens), which is what the
// pre-wiring app and the non-viewer tests want.
type SecretGetter interface {
	SecretData(ctx context.Context, ref kube.ObjectRef) (kube.SecretData, error)
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

// Deleter is the narrow slice of the kube layer the shell needs to run the delete
// action (M3-09): remove the object a table row references (M1-06a's Delete).
// *kube.Clients satisfies it. As with the viewer seams the shell depends on this
// interface, not the concrete client, so the tui package never constructs a client
// and the delete flow is driveable in hermetic tests with a fake deleter. A model
// built without one (the default) is delete-inert: the res.delete action opens no
// confirm modal, which is what the pre-wiring app and the non-action tests want.
// The signature matches kube.Delete so the UID-precondition row-snapshot guard
// (M1-06a/D35) rides through unchanged — the shell passes the selected row's ref
// (carrying its UID) so a stale row deletes only that exact object.
type Deleter interface {
	Delete(ctx context.Context, r kube.Resource, ref kube.ObjectRef, opts metav1.DeleteOptions) error
}

// Scaler is the narrow slice of the kube layer the shell needs to run the scale
// action (M3-10): set a scalable workload's replica count (M1-06b's Scale, which
// patches the /scale subresource so it is uniform across Deployment/RS/StatefulSet/
// ReplicationController). *kube.Clients satisfies it. As with Deleter the shell
// depends on the interface, not the concrete client, so the tui package constructs
// no client and the scale flow is driveable in hermetic tests. A model built
// without one is scale-inert: the Scale action opens no prompt. The signature
// matches kube.Scale — scale is idempotent, so there is no UID precondition (D35).
type Scaler interface {
	Scale(ctx context.Context, r kube.Resource, ref kube.ObjectRef, replicas int32) error
}

// RolloutRestarter is the narrow slice of the kube layer the shell needs to run the
// rollout-restart action (M3-10): stamp a pod-template workload's restartedAt
// annotation so the controller rolls its pods (M1-06b's RolloutRestart, matching
// `kubectl rollout restart`). *kube.Clients satisfies it. A model built without one
// is restart-inert: the Rollout-restart action opens no confirm modal.
type RolloutRestarter interface {
	RolloutRestart(ctx context.Context, r kube.Resource, ref kube.ObjectRef) error
}

// Cordoner is the narrow slice of the kube layer the shell needs to run the
// cordon/uncordon actions on a Node (M3-11a): mark it unschedulable / schedulable
// via M1-06c's Cordon/Uncordon (each a merge patch of spec.unschedulable, matching
// `kubectl cordon`/`uncordon`). *kube.Clients satisfies it. As with the other
// mutating seams the shell depends on the interface, not the concrete client, so the
// tui package constructs no client and the flow is driveable in hermetic tests. A
// model built without one is cordon-inert: the Cordon/Uncordon actions are no-ops.
// Both operations are idempotent, so — like Scale (D35) — there is no UID precondition
// and, unlike delete/rollout-restart, no confirm modal (D115): the action dispatches
// directly and reports its outcome to the status bar.
type Cordoner interface {
	Cordon(ctx context.Context, r kube.Resource, ref kube.ObjectRef) error
	Uncordon(ctx context.Context, r kube.Resource, ref kube.ObjectRef) error
}

// Suspender is the narrow slice of the kube layer the shell needs to run the
// suspend/resume actions on a CronJob (M3-12): flip its spec.suspend flag via
// M1-06d's Suspend/Resume (each a merge patch, matching `kubectl patch cronjob
// -p '{"spec":{"suspend":…}}'`). *kube.Clients satisfies it. As with the other
// mutating seams the shell depends on the interface, not the concrete client, so
// the tui package constructs no client and the flow is driveable in hermetic
// tests. A model built without one is suspend-inert: the Suspend/Resume actions
// are no-ops. Both operations are idempotent, so — like Cordon (D120) — there is
// no UID precondition and no confirm modal (D115): the action dispatches directly
// and reports its outcome to the status bar.
type Suspender interface {
	Suspend(ctx context.Context, r kube.Resource, ref kube.ObjectRef) error
	Resume(ctx context.Context, r kube.Resource, ref kube.ObjectRef) error
}

// Drainer is the narrow slice of the kube layer the shell needs to run the drain
// action on a Node (M3-11b): stream the drain's progress (cordon → evict → wait)
// over a channel via M1-06e-2's DrainStream, so the long eviction loop reports to
// the status bar without blocking the update loop (D53). *kube.Clients satisfies
// it. As with the other mutating seams the shell depends on the interface, not the
// concrete client, so the tui package constructs no client and the flow is
// driveable in hermetic tests with a fake drainer. A model built without one is
// drain-inert: the Drain action opens no confirm modal. Unlike cordon/uncordon
// (idempotent, D120) a drain evicts pods, so — like delete/rollout-restart — it is
// gated behind the confirm modal (D115).
type Drainer interface {
	DrainStream(ctx context.Context, nodeRes kube.Resource, node kube.ObjectRef, opts kube.DrainOptions) <-chan kube.DrainEvent
}

// ActiveForward is the behaviour the shell drives on a running background
// port-forward (M3-13a) — the subset of *kube.PortForward's lifecycle it observes:
// Ready fires once the local listeners are up (after which Ports reports the bound
// local:remote pairs), Done fires when forwarding ends (Err then reports the cause,
// nil for a clean stop), and Stop tears it down. It is an interface, not the
// concrete handle, so the flow is driveable in hermetic tests with a fake (D18);
// *kube.PortForward satisfies it.
type ActiveForward interface {
	Ready() <-chan struct{}
	Done() <-chan struct{}
	Err() error
	Ports() ([]kube.ForwardedPort, error)
	Stop()
}

// PortForwarder is the narrow slice of the kube layer the shell needs to start a
// background port-forward (M3-13a): forward local ports to the selected Pod via
// M1-08's PortForward, returning an ActiveForward the shell observes and stops. The
// dial happens on the handle's own goroutine, so this never blocks the update loop
// (principle 4); the shell learns of readiness/termination through messages, never a
// mutex (principle 1). A model built without one is forward-inert: the Port-forward
// action is a no-op. Because kube.Clients.PortForward returns the concrete
// *kube.PortForward (which satisfies ActiveForward) rather than the interface, the
// launcher adapts it with PortForwarderFunc.
type PortForwarder interface {
	PortForward(ctx context.Context, ref kube.ObjectRef, ports []string) (ActiveForward, error)
}

// PortForwarderFunc adapts a plain function to a PortForwarder, so the launcher can
// wrap kube.Clients.PortForward — whose concrete *kube.PortForward return type does
// not satisfy the interface method's ActiveForward return directly — in one line.
type PortForwarderFunc func(ctx context.Context, ref kube.ObjectRef, ports []string) (ActiveForward, error)

// PortForward calls the wrapped function, satisfying PortForwarder.
func (f PortForwarderFunc) PortForward(ctx context.Context, ref kube.ObjectRef, ports []string) (ActiveForward, error) {
	return f(ctx, ref, ports)
}

// ServiceResolver is the narrow slice of the kube layer the shell needs to
// port-forward a Service (M3-13c): a Service can't be forwarded directly (M1-08's
// PortForward posts to the pod portforward subresource), so the shell resolves it to
// a backing endpoint pod first (its selector → the newest ready pod), then forwards
// that pod — mirroring the PodResolver hop the logs viewer takes for a pod-owning
// workload (M3-07b). *kube.Clients satisfies it via PodForService. Without it wired
// the Port-forward action on a Service degrades to a toast, so the pre-wiring app and
// the non-resolver hermetic tests stay inert; a Pod row forwards directly (no hop),
// so it needs no resolver.
type ServiceResolver interface {
	PodForService(ctx context.Context, ref kube.ObjectRef) (kube.ObjectRef, error)
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
// for the unified View/Edit YAML action (the edit flow's buffer source, M3-15b/D135).
// Without it — or without an Editor — that action is inert.
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

// WithSecretGetter wires the kube client the shell uses to fetch a Secret's decoded
// data for the read-only secret viewer (M3-08a). Without it the Reveal-secret action
// is inert (the viewer never opens).
func WithSecretGetter(g SecretGetter) Option {
	return func(m *Model) { m.secretGetter = g }
}

// WithDeleter wires the kube client the shell uses to delete the selected row's
// object once the confirm modal is accepted (M3-09). Without it the res.delete
// action is inert (the confirm modal never opens).
func WithDeleter(d Deleter) Option {
	return func(m *Model) { m.deleter = d }
}

// WithScaler wires the kube client the shell uses to scale the selected workload's
// replicas once the scale prompt is submitted (M3-10). Without it the Scale action
// is inert (the prompt never opens).
func WithScaler(s Scaler) Option {
	return func(m *Model) { m.scaler = s }
}

// WithRolloutRestarter wires the kube client the shell uses to rollout-restart the
// selected workload once the confirm modal is accepted (M3-10). Without it the
// Rollout-restart action is inert (the confirm modal never opens).
func WithRolloutRestarter(r RolloutRestarter) Option {
	return func(m *Model) { m.restarter = r }
}

// WithCordoner wires the kube client the shell uses to cordon/uncordon the selected
// Node (M3-11a). Without it the Cordon/Uncordon actions are inert (no-ops). The
// actions dispatch directly — cordoning is idempotent, so there is no confirm modal.
func WithCordoner(c Cordoner) Option {
	return func(m *Model) { m.cordoner = c }
}

// WithSuspender wires the kube client the shell uses to suspend/resume the selected
// CronJob (M3-12). Without it the Suspend/Resume actions are inert (no-ops). The
// actions dispatch directly — suspending is idempotent, so there is no confirm modal.
func WithSuspender(s Suspender) Option {
	return func(m *Model) { m.suspender = s }
}

// WithDrainer wires the kube client the shell uses to drain the selected Node once
// the confirm modal is accepted (M3-11b). Without it the Drain action is inert (the
// confirm modal never opens).
func WithDrainer(d Drainer) Option {
	return func(m *Model) { m.drainer = d }
}

// WithPortForwarder wires the kube client the shell uses to start a background
// port-forward to the selected Pod once its ports prompt is submitted (M3-13a).
// Without it the Port-forward action is inert (the prompt never opens). The launcher
// passes a PortForwarderFunc wrapping kube.Clients.PortForward.
func WithPortForwarder(p PortForwarder) Option {
	return func(m *Model) { m.portForwarder = p }
}

// WithServiceResolver wires the kube client the shell uses to resolve a Service to a
// backing endpoint pod before port-forwarding it (M3-13c). Without it the Port-forward
// action on a Service degrades to a toast (a Pod row still forwards directly). The
// launcher passes *kube.Clients, which satisfies it via PodForService.
func WithServiceResolver(r ServiceResolver) Option {
	return func(m *Model) { m.serviceResolver = r }
}

// WithContext sets the kube context name shown on the status bar and the startup
// welcome page. Also forwarded to the `kubectl exec` parity fallback as --context
// (M3-14b-4) so the shelled-out kubectl targets the same context. Empty renders
// nothing on the UI and omits the flag.
func WithContext(name string) Option {
	return func(m *Model) { m.context = name }
}

// WithKubeconfig records the explicit --kubeconfig path kubecom launched with ("" =
// standard resolution). It is forwarded to the `kubectl exec` parity fallback
// (M3-14b-4) as --kubeconfig so a shelled-out kubectl resolves the same cluster the
// in-process client did; empty omits the flag (kubectl uses its standard rules).
func WithKubeconfig(path string) Option {
	return func(m *Model) { m.kubeconfig = path }
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

// WithPinnedResources folds the kinds pinned for this context (State.PinnedResources,
// D193) into the menu beside the authored entries — the same merge, one call later
// (menu.AddPinned), which is what keeps the hand-written entry in front of a pin
// naming the same GVR (D193 pt 3) without either list having to be pre-merged.
//
// The two lists arrive **separately** rather than merged because `*` toggles (D202):
// only a pinned row may be unpinned, so the shell has to be able to tell a pin from
// a hand-written entry at keypress time, and a merged list cannot say. Empty/nil
// pins nothing; the launcher reads them from the context's state file and writes
// them back through WithPinPersister.
func WithPinnedResources(pins []config.MenuResource) Option {
	return func(m *Model) { m.menuPinned = pins }
}

// WithLogger points the shell's diagnostic log at l — the file logger the launcher
// set up (`~/.cache/kubecom/kubecom.log`), which is the only place a diagnostic can
// go while the TUI owns the terminal. It records what the user only ever sees as a
// transient toast, so a failure that scrolled past is still recoverable afterwards
// (D159). Unset means discard: a model built without it logs nothing, which keeps
// every hermetic test silent and makes the sink assertable by injecting one.
func WithLogger(l *slog.Logger) Option {
	return func(m *Model) {
		if l != nil {
			m.logger = l
		}
	}
}

// WithTheme renders the shell through t instead of the built-in default palette
// (M4-12a) — what the `theme:` config field resolves to. The launcher resolves the
// name through styles.ByName and passes the Theme, so this package never has to
// know which names exist; an unknown name degrades to the default upstream.
//
// It is applied before the components are constructed (see NewWithKeymap), so
// every one of them caches this Styles rather than the default. A theme picked at
// runtime is a different problem — it must restyle already-built components — and
// belongs to M4-12b.
func WithTheme(t styles.Theme) Option {
	return func(m *Model) { m.styles = styles.New(t) }
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

	menu    menu.Model
	table   table.Model
	status  statusbar.Model
	hintbar hintbar.Model
	// hintCtx is the hint subset the hintbar currently renders. It is the memo
	// refreshHints compares against at the tail of every Update (HINT-01/D206), so a
	// message that does not move input ownership re-renders nothing.
	hintCtx keymap.HelpContext

	ctrPicker picker.Model
	// cmdPicker is the command palette (PAL-02): a picker over the app's verbs rather
	// than over cluster data, resolved back to a keymap.Action through cmdByLabel and
	// dispatched through handleAction. Like the theme picker it is seeded from neither
	// the cluster nor a file — the verb list is compiled in — so `:` opens it always.
	cmdPicker picker.Model
	// portPicker offers a port-forward target's declared ports as choices
	// (FB-pf-port-picker-b); its selection is stashed against mutateRes/mutateRef.
	portPicker picker.Model
	viewer     viewer.Model
	modal      modal.Model
	welcome    welcome.Model
	// searchView is the full-screen cluster-search mini-app (SEARCH-02a/b) and logsView
	// the dedicated logs mini-app (LOGS-01/02): unlike every field above neither is an
	// overlay — while one is up it *is* the body, composited in place of the browse
	// panes (D134). Follow state and the live grep live inside logsView, not on the
	// model, so the shell holds no second copy of what the view renders.
	searchView searchview.Model
	logsView   logsview.Model

	// context is the resolved kube context name and version the build version;
	// both are cosmetic, shown on the status bar (context) and the startup welcome
	// page (both). Set at construction via WithContext/WithVersion. kubeconfig is the
	// explicit --kubeconfig path (empty = standard resolution) — not cosmetic: it is
	// forwarded to the `kubectl exec` parity fallback (M3-14b-4) so the shelled-out
	// kubectl targets the same kubeconfig/context kubecom launched with.
	context    string
	version    string
	kubeconfig string

	// connector connects to another kubeconfig context and returns the Cluster
	// bundle bound to it (M4-04a). It sits here rather than on Cluster on purpose:
	// a Cluster seam is wrong to keep using after a switch, while the connector
	// outlives every switch and a Cluster is its product (D155 pt 2). Nil → the
	// model is switch-inert. ctxGen tags each connect attempt so one superseded by
	// a later switch is dropped when it lands (D156 pt 2 — the connect has no
	// stream to cancel, so the generation is the whole guard).
	connector ClusterConnector
	ctxGen    int

	// ctxSwitch times the switch currently in flight — connect, then the wait for the
	// new cluster's menu — and is consumed (and zeroed) by the discovery pass that
	// switch started, which writes the numbers to the diagnostic log (CTX-WARM-01).
	// Zero outside a switch, which is why a launch-time discovery logs nothing.
	ctxSwitch switchTiming

	// ctxLister seeds the palette's `:context ` stage from the kubeconfig (M4-04b);
	// nil → the ctx.switch action is inert. ctxByLabel maps each listed row back to
	// its context name, the resByLabel/themeByLabel pattern (a SelectedMsg carries only
	// the label, D65). Both are kubeconfig-scoped, not cluster-scoped, so neither is
	// part of the Cluster bundle or of what a switch tears down.
	ctxLister  ContextLister
	ctxByLabel map[string]string

	// themeByLabel maps each row of the palette's `:theme ` stage back to its theme
	// name (the resByLabel/ctxByLabel pattern again, D65). themePersister writes a
	// picked theme back to config.yaml so the next launch opens on it (nil → the
	// choice applies for the session only, M4-12b-2). Neither is cluster- or
	// context-scoped: a theme is a property of the reader's terminal.
	themeByLabel   map[string]string
	themePersister ThemePersister

	// ctxState re-resolves the per-context state (menu extras, last namespace, the
	// state-file persister) for the context a switch lands on (M4-05). Like the two
	// fields above it is keyed by the kubeconfig, not by a cluster, so it outlives
	// every switch and is not part of the Cluster bundle. Nil → per-context state is
	// left as it was wired at launch.
	ctxState ContextStateLoader

	// authDiagnoser re-runs the kubeconfig's credential plugin to explain a browse
	// failure client-go classified as that plugin's (AUTH-04b, authdiag.go); nil → the
	// failure keeps its kind's generic sentence. Like the three fields above it is
	// kubeconfig-scoped rather than cluster-scoped — it is handed the ClientConfig per
	// call — so a switch neither repoints nor tears it down. authDiagCancel and
	// authDiagGen *are* per-cluster state: the cancel ends the subprocess a switch
	// leaves behind, and the generation is the watchGen already diagnosed, which
	// latches the re-run to one per browse selection (the watch loop's backoff would
	// otherwise spawn one per retry).
	authDiagnoser  AuthDiagnoser
	authDiagCancel context.CancelFunc
	authDiagGen    int

	// reauthCmd is the remediation a diagnosis substantiated, resolved to an argv and
	// an environment by the kube layer and armed for the reader to approve (AUTH-05a,
	// reauth.go); reauthRes is the resource whose request failed, retried once the
	// remediation succeeds. hasReauth is the arming flag — false means there is
	// nothing to offer and nothing to run, which is every state until AUTH-05b's
	// confirm arms one. Cleared by the run that consumes it and by resetCluster: the
	// pair names a context's credentials and a resource on one cluster.
	reauthCmd kube.RemediationCommand
	reauthRes kube.Resource
	hasReauth bool
	// reauthOffer is the browse-pane text an open offer owns (AUTH-05b): what it
	// wrote while the confirm is up, and what the pane goes back to once the reader
	// answers. Zero while no offer is open, which is every state but the one.
	reauthOffer reauthOffer

	// menuExtras are the current context's per-context menu customizations (D83),
	// merged into the seed menu at construction (WithMenuExtras → menu.AddExtras)
	// before discovery so a discovered twin dedupes against them. startupErr is a
	// one-shot toast surfaced on Init (WithStartupError) — chiefly a malformed
	// per-context menu file that degraded to the default menu, kept visible rather
	// than swallowed. Both nil by default (the plain default menu, no toast).
	menuExtras []config.MenuResource
	// menuPinned are the kinds pinned for the current context (WithPinnedResources →
	// menu.AddPinned, folded in right after menuExtras). It is kept apart from
	// menuExtras rather than merged into it because it is the *removable* half: `*`
	// unpins a kind in this list and declines on one that is only in menuExtras
	// (D202). Rebound on a context switch beside the pin persister, so the list and
	// the file it is written back to always name the same context.
	menuPinned []config.MenuResource
	startupErr *ErrorMsg

	// logger is the shell's diagnostic sink (WithLogger). It is never the user's
	// screen — the TUI owns the terminal — so it is the *only* durable record of a
	// failure whose toast has already expired (D159). Never nil: NewWithKeymap seeds
	// a discarding logger so every call site can log unconditionally.
	logger *slog.Logger

	// editorArgv is the editor the launcher resolved once at startup (ResolveEditor →
	// WithEditorArgv, EDIT-01). It sits here rather than on Cluster because it is
	// per-process, not per-cluster: a context switch changes nothing about which
	// editor is installed. Empty → the Edit action reports errNoEditor instead of
	// suspending, so an unresolved editor can never blank the terminal.
	editorArgv []string

	// Cluster holds every seam bound to the cluster kubecom is currently on — the
	// watch/discovery clients, the viewer sources, the mutating action set (M4-02).
	// It is embedded, so the shell reads them unqualified (m.watcher, m.deleter, …)
	// exactly as when they were 21 separate fields; the point of the bundle is that a
	// context switch repoints one value rather than 21 (D155 pt 2). Everything below
	// is per-cluster *state* — in-flight channels, cancels, generations, stashes —
	// which a switch tears down (M4-03) rather than swaps.
	Cluster

	// namespace scopes the watch ("" = all namespaces). watchCh and watchCancel are
	// the current live watch: watchCh is re-read to pull the next event, watchCancel
	// tears it down when a newer resource is selected (or the app quits). watchGen
	// tags every watch-pump message so a delta from a superseded watch — whose
	// channel is already being drained — is dropped rather than applied or used to
	// re-issue a pump on the new channel (the same stale-message guard seqGen gives
	// the sequence timeout, D61).
	namespace   string
	watchCh     <-chan kube.WatchEvent
	watchCancel context.CancelFunc
	watchGen    int

	// current is the resource whose live table is showing (hasCurrent guards it);
	// the namespace picker re-scopes the watch by re-selecting it with the new
	// m.namespace (M2-08c). It is set every time selectResource starts a watch.
	current    kube.Resource
	hasCurrent bool

	// The owner → children drill-down (M4-08). childScope is the kube.ChildScope the
	// browse table is narrowed to while a child table is open (hasChildScope guards
	// it): watchResource passes its Namespace and Options to Watch instead of the
	// app's own namespace and an empty ListOptions, and re-applies them on every
	// restart, so the child table is a live watch rather than a snapshot (D165 pt 1).
	// childOwner/childOwnerRef are the row it was opened from, so nav.back returns to
	// it with that row selected. childGen tags the async scope resolve so a result
	// arriving after the reader moved on is dropped rather than yanking their table.
	childScope    kube.ChildScope
	hasChildScope bool
	childOwner    kube.Resource
	childOwnerRef kube.ObjectRef
	childGen      int

	// The metrics overlay (M4-10). metricsRes is the metrics kind measuring the
	// browsed one (zero when this cluster does not measure it — the whole "no
	// columns" state, since metrics.go guards on it) and metricsNS the namespace the
	// poll is scoped to: the browse watch's, so a children drill-down polls the
	// scope's namespace rather than the app's. metricsCancel aborts the request in
	// flight and metricsGen tags each refresh and each tick, so a sample set for a
	// kind, namespace or cluster the reader has left is dropped instead of painted
	// onto the table now showing something else.
	metricsRes    kube.Resource
	metricsNS     string
	metricsCancel context.CancelFunc
	metricsGen    int

	// nsPersister records a picked namespace to the per-context state file so the next
	// launch restores it (nil → persistence-inert, M2-11b-2). It is bound to one
	// context's state path, not to the cluster client, so it is not part of the
	// Cluster bundle — M4-05 rebinds it on a context switch. The surface it feeds
	// (the palette's `:namespace ` stage) is seeded by the bundle's nsLister.
	nsPersister NamespacePersister

	// pinner records a kind pinned with menu.pin to the per-context state file so
	// it stays in this context's menu (nil → pin-inert, CRD-PIN-02). Like nsPersister
	// it is bound to one context's state path rather than to the cluster client, so
	// it is not part of the Cluster bundle and a context switch rebinds it (D163).
	pinner PinPersister

	// viewerGen tags each shared-viewer open (describe/secret and, through logMsg, the
	// log stream — the viewer is one component) so an async fetch that returns after
	// the user closed the viewer, or opened a newer one, is dropped rather than
	// populating the wrong content — the same stale-message guard watchGen/seqGen give
	// their async work.
	viewerGen int

	// The live log stream behind the dedicated logs view (LOGS-02/D144). Unlike the
	// one-shot YAML/describe fetches it is a channel pumped line by line (D53): logCh
	// is re-read to pull the next line and logCancel tears the stream's goroutine down
	// when the logs view closes or a newer open supersedes it. Each pumped line rides
	// viewerGen, so a line from a superseded stream — one whose view was closed or
	// replaced — is dropped rather than appended under the wrong object, exactly as
	// watchGen guards the table watch. Touched only from the single-threaded update loop.
	logCh     <-chan kube.LogEvent
	logCancel context.CancelFunc
	// logReq is the request the current (or last) stream was opened with — object,
	// container and which instance. It is what makes logs.previous a toggle rather than
	// a second row action (M5-01a): flipping instances re-issues this with one bit
	// changed, so the pod resolution and the container pick that got the reader here are
	// not spent again.
	logReq logRequest

	// ctrStreamRes/ctrStreamRef stash the pod the container picker's selection applies
	// to and ctrPurpose which terminal it routes to (logs stream vs exec session,
	// M3-07a/M3-14b-2) — the picker's SelectedMsg carries only the chosen container
	// name (D65), so the object + purpose are held here between the picker opening and
	// the pick landing. Only one picker is ever up at a time, so a single stash serves
	// both purposes. Touched only from the single-threaded update loop.
	ctrStreamRes kube.Resource
	ctrStreamRef kube.ObjectRef
	ctrPurpose   ctrPurpose
	// ctrByLabel maps each open container-picker row back to its container name, the
	// ctxByLabel/resByLabel pattern (D65): a row for a non-regular container is
	// labelled `name (init)`, so the label the pick carries is not the name to stream
	// (LOGS-06). Cleared with the picker.
	ctrByLabel map[string]string

	// secretData holds the fetched entries of the Secret in the shared viewer (M3-08a)
	// so the reveal toggle can re-render them without re-fetching, and secretRevealed
	// is whether values are currently unmasked (false on open — the deliberate-reveal
	// contract, #89). Both are consulted only while the secret viewer is up
	// (viewerKindSecret), so a stale value left from a closed one is harmless. Touched
	// only from the single-threaded update loop.
	secretData     kube.SecretData
	secretRevealed bool
	// secretSel is the entry cursor into secretData.Entries (M3-08b): the entry
	// secret.copy copies and the one renderSecret marks with the cursor gutter.
	// nav.up/nav.down move it while the secret viewer is up (secrets are small, so
	// the cursor is more useful than a line scroll there). secretEntryLines maps
	// each entry index to its 0-based output line so the selected entry can be kept
	// on screen (EnsureLineVisible). Reset on every open/load.
	secretSel        int
	secretEntryLines []int

	// resByLabel maps each row of the palette's `:resource ` (and `:pin `) stage back
	// to its kube.Resource. The picker is generic over strings (D65), so the stage
	// lists resource titles and this map, rebuilt each time the stage opens from the
	// menu's current item set (resourcePickerItems), resolves the picked title to the
	// resource selectResource watches (FB-nav-resource-palette; the stage is the only
	// surface since PAL-05b). It holds no shared mutable state — only the update loop
	// touches it.
	resByLabel map[string]kube.Resource

	// cmdByLabel maps each entry of the command palette (cmdPicker) back to its
	// keymap.Action, rebuilt each time the palette opens from the curated verb list
	// (openPalette). The picker is generic over strings (D65), so the palette lists
	// the actions' registry descriptions and this map resolves the picked one back to
	// the Action handleAction runs; only the update loop touches it.
	cmdByLabel map[string]keymap.Action

	// palArg is the verb the command palette has committed to, i.e. which stage its
	// line is in: empty means it is choosing a verb, and a value means the list now
	// holds that verb's *arguments* and a pick runs the verb with one (PAL-03a). It is
	// the palette's whole extra state — the items and the label map are the picker's
	// and cmdByLabel's as before — so "which stage" can never disagree with what a
	// pick does: handleCommandSelected reads this one field. Only the update loop
	// touches it.
	palArg keymap.Action

	// palDirect records that the current argument stage was opened *directly* by its
	// shortcut key (or the menu's namespace seam) rather than by committing a verb off
	// the palette's own verb list — i.e. there is no verb stage behind it that the
	// reader has actually been on. It is what makes esc mean "back" for those five keys
	// (D233): a stage the reader typed their way into rewinds to the verbs, a stage a
	// key opened closes outright, because rewinding it would land the reader on a list
	// they never saw. It is set only by openPaletteArg and cleared by every other way
	// the stage changes (enterPaletteArg, showPaletteVerbs, closePalette), so it can
	// never outlive the stage it describes. Only the update loop touches it.
	palDirect bool

	// palRowByLabel maps the palette's **row-scoped** entries back to their rowAction
	// (PAL-04). Its source is the row-action registry's own per-kind set
	// (rowActionTitles), rebuilt whenever the palette shows its verb stage or, since
	// PAL-05d, enters the `:action ` stage `a` opens — one map for both, so the two
	// ways in cannot come to offer different actions for the same row. It is nil
	// whenever there is no row to act on, which is also what makes "did the reader pick
	// a row verb?" a lookup rather than a second piece of state. Only the update loop
	// touches it.
	palRowByLabel map[string]rowAction

	// deleteRes/deleteRef stash the target the open delete confirm applies to (M3-09):
	// modal.ConfirmedMsg carries only the modal's Kind (no payload in confirm mode,
	// D88), so the resource + the row's ObjectRef (its UID guards the snapshot race,
	// M1-06a/D35) are held here between the modal opening and the accept landing.
	// Consulted only while the delete modal is up (deleteModalKind), so a stale value
	// left from a declined one is harmless. Touched only from the single-threaded
	// update loop.
	deleteRes kube.Resource
	deleteRef kube.ObjectRef

	// mutateRes/mutateRef stash the target the open scale/rollout-restart modal applies
	// to (M3-10) — like deleteRes/deleteRef, ConfirmedMsg carries only the modal Kind
	// (D88) — shared between the two because only one modal is ever up at a time;
	// consulted only while the scale/rollout modal is up (scaleModalKind/
	// rolloutRestartModalKind), so a stale value from a declined one is harmless. Both
	// actions are idempotent (no UID guard, D35). The port-forward prompt reuses the
	// same stash. Touched only from the single-threaded update loop.
	mutateRes kube.Resource
	mutateRef kube.ObjectRef

	// The cordon/uncordon (M3-11a) and suspend/resume (M3-12) actions need no target
	// stash at all: each is idempotent (no UID guard, D35) and has no confirm modal
	// (D115/D120), so the action dispatches straight from handleRowAction with the
	// row's ref in hand — nothing is held between an open modal and an accept because
	// there is no modal.

	// The live node drain behind the accepted drain confirm (M3-11b). Unlike the
	// one-shot mutating actions a drain is long-running and pumped step by step (D53):
	// drainCh is re-read to pull the next progress event and drainCancel tears the
	// drain's goroutine down on quit or when a newer drain supersedes it (stopDrain) —
	// the mutating twin of stopLogStream's cancel-on-close. drainGen tags each pumped
	// event so a step from a superseded/cancelled drain is dropped rather than
	// reported to the status bar (mirroring viewerGen for the log stream). drainRes/
	// drainRef stash the confirm's target (the ConfirmedMsg carries only the modal
	// Kind, D88), and drainLabel the human node name for the final status message.
	// All touched only from the single-threaded update loop.
	drainCh     <-chan kube.DrainEvent
	drainCancel context.CancelFunc
	drainGen    int
	drainRes    kube.Resource
	drainRef    kube.ObjectRef
	drainLabel  string

	// forwards holds the running background port-forwards (M3-13a); each carries its
	// own context cancel and the ActiveForward handle. The target of the one being
	// started is stashed in mutateRes/mutateRef between the ports prompt opening and
	// the submit landing, like scale (only one modal is ever up at a time). Lifecycle
	// (Ready → bound ports, Done → removal) flows in through messages, never a mutex
	// (principle 1); forwardSeq stamps a stable id on each so a Ready/Done message
	// finds its entry after the slice shifts. On quit stopForwards cancels them all
	// (cancel-on-exit). All fields are touched only from the single-threaded update loop.
	//
	// forwardsPanel/forwardsSel are the M3-13b listing overlay: forwards.panel (`F`)
	// toggles a global panel listing the active forwards, forwardsSel is the cursor
	// into m.forwards (nav.up/down move it), nav.drillIn stops the selected forward and
	// forwards.stopAll (`X`) stops every one. The panel reads m.forwards directly; like
	// the secret viewer's entry cursor it is inline state, not a separate component.
	forwards      []*forward
	forwardSeq    int
	forwardsPanel bool
	forwardsSel   int

	// pfResolveGen stamps the two async hops a port-forward can take before its prompt
	// opens — resolving a Service to a backing endpoint pod (M3-13c) and listing the
	// target's declared ports (FB-pf-port-picker-b) — so a superseded resolution is
	// dropped. Touched only from the single-threaded update loop.
	pfResolveGen int

	// pfPorts holds a port-forward target's listed declared ports between the picker
	// opening and the pick landing (FB-pf-port-picker-b) — the picker's SelectedMsg
	// carries only the chosen label (D65), so the label maps back to a port through
	// here. pfPort is the one port the local-port gesture (FB-pf-local-port) is acting
	// on, held across its prompt so the submitted local half can be joined to the right
	// remote port. The target itself is stashed in mutateRes/mutateRef, as it is for
	// the prompt. Touched only from the single-threaded update loop.
	pfPorts []kube.Port
	pfPort  kube.Port

	// Exec (M3-14b-1) and edit (M3-15b) hold no state here: each is a *blocking* call
	// run from a suspended terminal via tea.Exec (off the update loop, D124), so there
	// is no channel or goroutine to track — bubbletea drives the command and delivers
	// the outcome as an execDoneMsg/editDoneMsg the update loop reports. See exec.go
	// and edit.go.

	// The live cluster-search fan-out behind the search.cluster action (SEARCH-02b).
	// Like a log stream the search is a channel pumped item by item (D53): searchCh is
	// re-read to pull the next hit and searchCancel tears the fan-out down when the
	// query changes, the view closes, or the app quits. searchGen tags every debounce
	// tick and pumped hit with the query it belongs to, so a hit from a superseded
	// query — one whose channel is still draining after cancellation — is dropped
	// rather than shown under a query it does not describe (D140 pt 3), exactly as
	// watchGen guards the table watch.
	//
	// searchTarget/hasSearchTarget are the pending selection a drill-in leaves behind:
	// the hit's kind is switched to immediately, but its row only exists once the fresh
	// watch's first RESET lands, so the watch pump applies the selection when it does.
	// All are touched only from the single-threaded update loop.
	searchCh        <-chan kube.SearchEvent
	searchCancel    context.CancelFunc
	searchGen       int
	searchTarget    kube.ObjectRef
	hasSearchTarget bool

	// filterInput is the table filter field (M2-09b): app.filter (`/`) opens it over
	// the current table, typing narrows the live rows through table.SetFilter (D78),
	// and it re-scopes to whatever is showing. filtering is whether it is open and
	// capturing text — while true the root routes every keypress through
	// routeFilterKey (control/text split, D73), bypassing the sequencer, exactly as
	// the namespace picker does. The narrowing is a view over the table's
	// authoritative full set, so clearing the filter restores every live row.
	filterInput textinput.Model
	filtering   bool

	// discoveryCancel tears the in-flight discovery pass down on quit (the cap-1
	// discovery channel already keeps the goroutine from leaking, D8, but cancelling
	// drops the result promptly). discoveryGen tags each pass's result so one that
	// belongs to a cluster the model has since left is dropped rather than
	// reconciled into the new context's menu — cancelling alone cannot guarantee
	// that, since the pass may already have won the race to the cap-1 buffer
	// (M4-03). Touched only from the single-threaded update loop.
	discoveryCancel context.CancelFunc
	discoveryGen    int

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

	// statusNoticeGen is statusErrGen's twin for the neutral (non-error) status
	// notice — the transient success message the secret copy shows (M3-08b). Kept
	// separate so an error and a notice clear on independent timers.
	statusNoticeGen int

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
// Options run **before** the components are constructed, because a component
// caches the Styles it is handed (WithTheme would otherwise have to restyle
// eleven models after the fact — that is M4-12b's live-restyle problem, and a
// launch-time theme does not need it).
func NewWithKeymap(km *keymap.Keymap, opts ...Option) Model {
	fi := textinput.New()
	fi.Prompt = "/"
	m := Model{
		keymap:      km,
		seq:         keymap.NewSequencer(km),
		styles:      styles.Default(),
		filterInput: fi,
		logger:      slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Every component renders through the same resolved Styles — the default set
	// above unless WithTheme replaced it.
	s := m.styles
	m.help = help.New(s, km)
	m.menu = menu.New(s)
	m.table = table.New(s)
	m.status = statusbar.New(s)
	m.hintbar = hintbar.New(s)
	m.ctrPicker = picker.New(s, containerPickerKind)
	m.cmdPicker = picker.New(s, commandPickerKind)
	// The port picker is the one picker that does not filter as you type (D194 pt 3):
	// its own `p` (local-port prompt) and `0` (let the OS pick) gestures carry text,
	// and an always-open query field swallows every text-carrying key (D140 pt 1), so
	// it keeps the opt-in `/` filter every picker had before PAL-01.
	m.portPicker = picker.New(s, portPickerKind, picker.WithOptInFilter())
	m.viewer = viewer.New(s, viewerKindDescribe)
	m.modal = modal.New(s)
	m.welcome = welcome.New(s)
	m.searchView = searchview.New(s)
	m.logsView = logsview.New(s)
	m.cmdPicker.SetTitle("Command")
	m.ctrPicker.SetTitle("Container")
	m.portPicker.SetTitle(portPickerTitle(km)) // advertises the local-port gestures by their bound keys
	m.menu.AddExtras(m.menuExtras)             // fold in the per-context menu customizations (D83); no-op when none
	m.menu.AddPinned(m.menuPinned)             // then this context's pins, behind them (D193 pt 3 / D202)
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

// noticeClearMsg auto-clears a transient status-bar notice after errorDisplay, the
// neutral twin of errorClearMsg guarded by statusNoticeGen (M3-08b).
type noticeClearMsg struct{ gen int }

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
	// The age clock starts with the program and runs for its life (age.go): it is
	// unconditional because it is free on the ticks that change nothing, and a
	// gated one is a thing that can be left off.
	cmds = append(cmds, scheduleAgeTick())
	return tea.Batch(cmds...)
}

// Update implements tea.Model. It sizes the layout on a window-size message,
// resolves keypresses through the sequencer, and services the sequence timeout
// tick. Every behaviour flows through an Action; no raw key is matched.
//
// It wraps the real handler so the bottom hint line is re-derived after every
// message (HINT-01/D206): input ownership moves on far more messages than it has
// explicit `syncHints` calls — every picker Show and Hide, and there are ~30 of
// them — and a hint pushed from thirty places is one that goes stale. refreshHints
// re-renders only when the derived context actually changed, so the common message
// (a watch delta, a log line) costs one comparison.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if nm, ok := next.(Model); ok {
		nm.refreshHints()
		return nm, cmd
	}
	return next, cmd
}

// update is the real message handler; Update wraps it with the hint derivation.
func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		// The search mini-app captures every keypress while it is up (its query field is
		// always open, D140 pt 1) and nothing it does opens a picker or modal, so it is
		// resolved before them.
		if m.searchView.Active() {
			return m.routeSearchKey(msg)
		}
		// The logs view's live grep captures text while it is open (LOGS-02), so its keys
		// are split raw here rather than resolved through the sequencer. With the filter
		// closed the logs view takes the ordinary action path (handleLogsAction) so `gg`
		// and `G` still work in a log.
		if m.logsView.Filtering() {
			return m.routeLogsFilterKey(msg)
		}
		if m.activePicker() != nil {
			return m.routePickerKey(msg)
		}
		if m.filtering {
			return m.routeFilterKey(msg)
		}
		if m.modal.Prompting() {
			return m.routeModalPromptKey(msg)
		}
		if m.modal.Active() {
			return m.routeModalConfirmKey(msg)
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
		// Drilling into the menu's namespace-seam row opens the palette's
		// `:namespace ` stage — the same effect as the ns.switch (ctrl+n) shortcut,
		// which is the whole point of routing it through the same opener (PAL-05c-1).
		// This is the second door D207 pt 1 does not cover on its own: a key names its
		// verb, but so does this row, and leaving it on a picker of its own would keep
		// the retired surface alive behind a menu row.
		return m.openPaletteArg(keymap.ActionNamespace)

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

	case noticeClearMsg:
		if msg.gen == m.statusNoticeGen {
			m.status.ClearNotice()
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

	case clusterConnectedMsg:
		return m.handleClusterConnected(msg)

	case contextsLoadedMsg:
		return m.handleContextsLoaded(msg)

	case picker.SelectedMsg:
		switch msg.Kind {
		case commandPickerKind:
			return m.handleCommandSelected(msg)
		case containerPickerKind:
			return m.handleContainerSelected(msg)
		case portPickerKind:
			return m.handlePortSelected(msg)
		}
		// No default arm since PAL-05c-1: it used to mean "the namespace picker",
		// which was the one surface whose Kind nothing branched on. With that picker
		// retired every remaining picker is named above, so an unrecognised Kind is a
		// message from a surface that no longer exists (a list that landed after a
		// context switch) and is dropped rather than applied to whichever picker the
		// default happened to name.
		return m, nil

	case picker.CancelledMsg:
		switch msg.Kind {
		case commandPickerKind:
			// esc in an argument stage the reader *typed* their way into rewinds the
			// line one word rather than closing: the verb is uncommitted and the verb
			// list comes back, so a mistyped argument costs one key instead of
			// reopening the palette. (The picker itself already spent an esc clearing a
			// non-empty query, so this is the second one — and from the verb list the
			// next esc closes.)
			//
			// A stage a shortcut key opened (palDirect) closes outright instead: the
			// verb list is not where that reader came from, so rewinding to it was esc
			// failing to back out of anything — the complaint feedback
			// `2026-08-06-action-menu-esc-behavior` filed against `a` (D233, superseding
			// D207 pt 2 for esc). Backspace is unaffected and still rewinds: it edits
			// the line, and erasing the committed verb word is what it means there.
			if m.palArg != "" && !m.palDirect {
				return m.showPaletteVerbs(), nil
			}
			m.closePalette()
		case containerPickerKind:
			m.ctrPicker.Hide()
			m.ctrByLabel = nil
		case portPickerKind:
			m.portPicker.Hide()
		}
		// No default arm — see picker.SelectedMsg above.
		return m, nil

	case rowActionMsg:
		return m.handleRowAction(msg)

	case childScopeMsg:
		return m.handleChildScope(msg)

	case metricsMsg:
		return m.handleMetricsMsg(msg)

	case metricsTickMsg:
		return m.handleMetricsTick(msg)

	case ageTickMsg:
		return m.handleAgeTick()

	case authDiagMsg:
		return m.handleAuthDiagMsg(msg)

	case modal.ConfirmedMsg:
		return m.handleModalConfirmed(msg)

	case modal.CancelledMsg:
		// The confirm modal was declined (nav.back) or dismissed. Hide it; the stashed
		// delete target is left untouched (harmless — runDelete only fires on accept).
		m.modal.Hide()
		if msg.Kind == reauthModalKind {
			// The one stash a decline must actually drop, because it is the one nothing
			// else gates: an approval is single-use and an unanswered one may not linger
			// (D215 pt 4). The pane goes back to saying the reader can run it themselves.
			return m.declineReauth()
		}
		return m, nil

	case deleteDoneMsg:
		return m.handleDeleteDone(msg)

	case scaleDoneMsg:
		return m.handleScaleDone(msg)

	case restartDoneMsg:
		return m.handleRestartDone(msg)

	case cordonDoneMsg:
		return m.handleCordonDone(msg)

	case suspendDoneMsg:
		return m.handleSuspendDone(msg)

	case execDoneMsg:
		return m.handleExecDone(msg)

	case editFetchedMsg:
		return m.handleEditFetched(msg)

	case editDoneMsg:
		return m.handleEditDone(msg)

	case reauthDoneMsg:
		return m.handleReauthDone(msg)

	case drainMsg:
		return m.handleDrainMsg(msg)

	case forwardReadyMsg:
		return m.handleForwardReady(msg)

	case forwardDoneMsg:
		return m.handleForwardDone(msg)

	case describeLoadedMsg:
		return m.handleDescribeLoaded(msg)

	case secretLoadedMsg:
		return m.handleSecretLoaded(msg)

	case podResolvedMsg:
		return m.handlePodResolved(msg)

	case serviceResolvedMsg:
		return m.handleServiceResolved(msg)

	case portsLoadedMsg:
		return m.handlePortsLoaded(msg)

	case containersLoadedMsg:
		return m.handleContainersLoaded(msg)

	case logMsg:
		return m.handleLogMsg(msg)

	case searchview.QueryChangedMsg:
		return m.handleSearchQueryChanged(msg)

	case searchview.ScopeChangedMsg:
		return m.handleSearchScopeChanged(msg)

	case searchDebouncedMsg:
		return m.handleSearchDebounced(msg)

	case searchMsg:
		return m.handleSearchMsg(msg)

	case searchview.SelectedMsg:
		return m.handleSearchSelected(msg)

	case searchview.ClosedMsg:
		// The search view dismissed itself (nav.back on an empty query). Hide it and
		// cancel any fan-out still running; the browse view underneath is untouched, so
		// the table keeps whatever selection it had.
		m.closeSearch()
		return m, nil

	case viewer.ClosedMsg:
		// The viewer dismissed itself (nav.back). Hide it and return focus to the browse
		// view underneath (the table keeps whatever selection it had). No log stream to
		// tear down — logs have their own view now (D144).
		m.viewer.Hide()
		return m, nil

	case logsview.ClosedMsg:
		// The logs view dismissed itself (nav.back with the filter already closed). Hide
		// it and cancel the stream feeding it; the browse view underneath is untouched,
		// so the table keeps whatever selection it had.
		m.closeLogs()
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
// It is the entry point for every path that points the browse table at something
// the reader chose *directly* — a menu drill-in, the resource palette, a search hit,
// a namespace re-scope — so it drops any children drill-down scope first (M4-08): a
// scope belongs to one owner's pods and must not silently narrow the next kind, nor
// survive an explicit re-scope of the namespace. The drill-down itself starts its
// watch through watchResource, below, with the scope already installed.
func (m Model) selectResource(r kube.Resource) (tea.Model, tea.Cmd) {
	m.clearChildScope()
	return m.watchResource(r)
}

// watchResource is selectResource's body: everything above minus the scope reset, so
// the children drill-down can start the very same watch with a kube.ChildScope in
// place (M4-08). Every caller goes through one of the two — nothing else starts a
// browse watch — which is what guarantees the scope is re-applied on a restart
// (namespace change, a reconnect's re-list) instead of being dropped on the second
// pass, the failure mode the M4-07 journal flagged.
func (m Model) watchResource(r kube.Resource) (tea.Model, tea.Cmd) {
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
	// Any selection a previous search drill-in was still waiting for belongs to the
	// resource being left, so it is dropped here; a drill-in re-arms it after this
	// returns (handleSearchSelected).
	m.searchTarget = kube.ObjectRef{}
	m.hasSearchTarget = false
	// A fresh resource (or re-scoped namespace) starts unfiltered: SetTable clears
	// the table's filter (D78); mirror that in the shell's filter state so a stale
	// prompt/indicator from the previous resource does not linger.
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.Reset()
	m.syncFilterStatus()

	// A children drill-down watches the owner's pods, not the app's namespace: the
	// scope carries both halves (a Node's children are cluster-wide, Namespace ""),
	// and both are read here rather than at the call site so every restart re-applies
	// them (M4-08/D165 pt 1).
	ns, opts := m.namespace, metav1.ListOptions{}
	if m.hasChildScope {
		ns, opts = m.childScope.Namespace, m.childScope.Options
	}
	m.syncScopeStatus()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.watcher.Watch(ctx, r, ns, opts)
	if err != nil {
		cancel()
		e := NewErrorMsg("watch "+r.GVR.Resource, err)
		// No watch was ever started, so no RESET is coming and no retry is running:
		// the pane would stay blank behind a 5-second toast. Say why in the pane
		// itself (CRD-01) — the kind is r's, not m.current's, which this failure
		// leaves pointing at the resource being left.
		m.table.SetNotice(browseFailure(r.GVK.Kind, e))
		// A credential plugin's failure has more to say than that sentence, but only a
		// re-run can say it, so the pane is rewritten when the diagnosis lands (AUTH-04b).
		diag := m.diagnoseAuth(r, e)
		return m, tea.Batch(func() tea.Msg { return e }, diag)
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

	// The metrics overlay follows the watch: same kind, same namespace, re-evaluated
	// on every restart (M4-10). Off — and silent — for a kind this cluster does not
	// measure.
	metrics := m.startMetrics(r, ns)
	return m, tea.Batch(m.pumpWatch(), metrics)
}

// surfaceError shows a classified error as a transient message in the status bar
// and arms its auto-clear timer. It bumps statusErrGen so a stale clear timer from
// an earlier error cannot wipe this one early, and returns the clear Cmd for the
// caller to schedule (batched with any other work). It mutates the receiver, so
// callers pass the addressable model value they are about to return.
//
// It is also the shell's **single error funnel** — `status.SetError` is called
// nowhere else — which is why the log line lives here rather than at the ~25 call
// sites: a future leg that surfaces a new error is recorded without doing anything
// (D159). The toast is transient (5s) and clipped to the terminal width; the log
// line is neither, and carries the classified kind plus the *unwrapped* error the
// toast may have truncated, which is what makes a dogfooding bug report actionable.
func (m *Model) surfaceError(e ErrorMsg) tea.Cmd {
	m.logger.Error("surfaced error", "context", e.Context, "kind", e.Kind.String(), "error", e.Err)
	m.status.SetError(e.Message())
	m.statusErrGen++
	gen := m.statusErrGen
	return tea.Tick(errorDisplay, func(time.Time) tea.Msg {
		return errorClearMsg{gen: gen}
	})
}

// surfaceNotice shows a neutral (non-error) transient message in the status bar and
// arms its auto-clear timer — surfaceError's twin for a success confirmation (the
// secret copy, M3-08b). It bumps statusNoticeGen so a stale clear timer can't wipe a
// newer notice early, and returns the clear Cmd for the caller to batch. It mutates
// the receiver, so callers pass the addressable model value they are about to return.
func (m *Model) surfaceNotice(text string) tea.Cmd {
	m.status.SetNotice(text)
	m.statusNoticeGen++
	gen := m.statusNoticeGen
	return tea.Tick(errorDisplay, func(time.Time) tea.Msg {
		return noticeClearMsg{gen: gen}
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
		// A search drill-in switched to this kind and is waiting for its object's row;
		// select it as soon as the watch delivers it (SEARCH-02b).
		m.applyPendingSelect()
		return m, m.pumpWatch()
	case ErrorMsg:
		// The watch loop retries and re-lists on recovery (a fresh RESET follows),
		// so the chain stays alive; surface the error transiently in the status bar
		// meanwhile rather than swallowing it silently.
		//
		// The toast is gone in five seconds and the retry can fail for as long as
		// the cluster stays broken, so the reason is *also* written into the table,
		// where it shows only while the pane is empty and is cleared by the RESET
		// that recovery brings (CRD-01). A watch that drops after a good List keeps
		// its rows and the notice stays dormant.
		m.table.SetNotice(browseFailure(m.current.GVK.Kind, inner))
		clear := m.surfaceError(inner)
		// And if it was the credential plugin that failed, re-run it and rewrite the
		// notice with what it printed — once per selection, however often the loop
		// retries (AUTH-04b).
		diag := m.diagnoseAuth(m.current, inner)
		return m, tea.Batch(clear, diag, m.pumpWatch())
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
	return m, tea.Batch(spin, discoveryPump(ch, m.discoveryGen))
}

// handleDiscovery folds a completed discovery pass into the menu and stops the
// spinner. Reconcile merges the discovered resources into the seed without
// disturbing selection or scroll (M2-05b/D57) and is a no-op on a total failure
// (Result.Err set) — the menu then stays on its fully navigable seed rather than
// blanking (principle 3; visible surfacing of a discovery failure is a later
// slice). The one-shot context is cancelled now its result is in hand.
func (m Model) handleDiscovery(msg DiscoveryReadyMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.discoveryGen {
		return m, nil // a pass belonging to a cluster this model has left (M4-03).
	}
	m.status.StopDiscovery()
	if m.discoveryCancel != nil {
		m.discoveryCancel()
		m.discoveryCancel = nil
	}
	m.logDiscovery(msg.Result)
	m.menu.Reconcile(msg.Result)
	// The menu is complete now, which is the moment a switched-to cluster becomes
	// usable — so this is where a context switch's stopwatch stops (CTX-WARM-01).
	// A launch-time pass has none pending and this is a no-op.
	m = m.logSwitchComplete()
	return m, nil
}

// logDiscovery records what a discovery pass could not load. These are the shell's
// one class of failure that never reaches a toast at all: a total failure seeds the
// menu instead of erroring, and a per-group failure isolates that group and shows it
// as unavailable (#87/#76, principle 3) — both deliberately quiet on screen, and
// therefore both invisible when the question is "why is this CRD's kind missing or
// broken?". The log is where that answer belongs (D159); the menu keeps degrading
// exactly as before.
func (m Model) logDiscovery(r kube.DiscoveryResult) {
	if r.Err != nil {
		m.logger.Error("discovery failed", "kind", kube.Classify(r.Err).String(), "error", r.Err)
		return
	}
	for _, f := range r.Failed {
		m.logger.Warn("discovery group unavailable", "groupVersion", f.GroupVersion(), "error", f.Err)
	}
}

// stopClusterAsync cancels every asynchronous operation bound to the cluster the
// model is currently on — the table watch, the discovery pass, the log stream, the
// cluster-search fan-out, a running node drain and every background port-forward —
// and bumps the generation guarding each, so a message already in flight from the
// cancelled work is dropped instead of applied. Cancellation alone is not enough:
// each of these delivers over a buffered channel whose producer can win the race
// against its own context, so "cancelled" and "will never be heard from again" are
// different things (the discovery pass is the clearest case, DiscoveryReadyMsg).
//
// It is the single inventory of per-cluster async, and both callers need all of it:
// app.quit calls it before tea.Quit, and resetCluster calls it before a context
// switch repoints the Cluster bundle (M4-03/D155 pt 1). A future leg that adds a
// per-cluster async adds it here, or it survives both a quit and a switch — and a
// watch surviving a switch streams the old cluster's rows into the new context's
// table, where a row action then hits the wrong cluster.
//
// Safe to call with nothing running. It mutates the receiver, so callers pass the
// addressable model value they are about to return.
func (m *Model) stopClusterAsync() {
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.watchCh = nil
	m.watchGen++ // in-flight watch-pump messages are now stale.
	if m.discoveryCancel != nil {
		m.discoveryCancel()
		m.discoveryCancel = nil
	}
	m.discoveryGen++ // a result from the cancelled pass is now stale.
	m.stopLogStream()
	m.stopSearch()
	m.searchGen++    // in-flight search hits are now stale.
	m.viewerGen++    // in-flight describe/secret/YAML fetches and log lines are now stale.
	m.pfResolveGen++ // in-flight service→pod and port-list resolutions are now stale.
	m.childGen++     // an in-flight child-scope resolve names an object on this cluster.
	m.stopMetrics()  // the metrics poll lists this cluster's samples on a ticker.
	m.stopAuthDiag() // a credential-plugin re-run asks about the context being left.
	m.stopDrain()
	m.drainGen++ // in-flight drain steps are now stale.
	m.stopForwards()
}

// resetCluster returns the model to the state it launches in, minus the cluster:
// every per-cluster async torn down (stopClusterAsync), every surface showing the
// departing cluster's data dismissed, and the browse panes back to their
// pre-drill-in state — the seed menu (discovery's additions dropped, since they
// described the old cluster's API surface), an empty unsorted unfiltered table, no
// namespace scope, focus on the menu, the welcome page in the right pane.
//
// It is the *first* half of a context switch (M4-04): reset, then repoint the
// Cluster bundle, then restart discovery. Landing it before anything can trigger it
// is deliberate — a switch is a teardown, and a half-torn-down switch is a
// correctness bug rather than a leak (D155 pt 1). Nothing calls it outside tests
// until M4-04 wires the picker.
//
// The context name, the keymap, the help overlay and the mouse/menu-visibility
// toggles are deliberately left alone: none is cluster data. The per-context
// namespace persister and menu extras are likewise not reset here — they need the
// *new* context to resolve against, so the switch installs them around this call
// (M4-05/D163): the extras before it, because the menu rebuild below folds in
// whatever the model holds, the persister and restored scope after it.
//
// It mutates the receiver, so callers pass the addressable model value they are
// about to return.
func (m *Model) resetCluster() {
	m.stopClusterAsync()

	// Dismiss every surface showing the departing cluster's data. The two
	// full-screen views are reset as well as hidden so their buffers do not hold the
	// old cluster's lines/hits until the next open clears them.
	m.searchView.Hide()
	m.searchView.Reset()
	m.logsView.Hide()
	m.logsView.Reset()
	m.viewer.Hide()
	m.modal.Hide() // a pending confirm targets an object on the cluster being left.
	m.ctrPicker.Hide()
	m.portPicker.Hide()
	// The command palette lists verbs, not cluster data, so a switch does not make its
	// rows wrong — but a verb picked *after* the switch would act on the new cluster
	// while the reader opened the list against the old one, so it closes with the rest.
	// Since PAL-05a that closes the `:theme ` stage too, which is the one stage whose
	// values are not per-cluster in either direction; it is dismissed anyway rather than
	// special-cased, because "one surface" means one teardown rule for it (D207).
	m.closePalette()
	// closePalette closes the surface; the `:context ` stage's row map is state behind
	// it and is cleared for a different reason than the rest: its rows mark the context
	// the shell is on, and a switch is exactly what makes that marker wrong. (In
	// practice the stage is already closed — the pick that started the switch closed
	// it — so this is about the state, not the surface.)
	m.ctxByLabel = nil
	m.forwardsPanel = false
	m.forwardsSel = 0

	// Drop every stash holding an object from the departing cluster. Each is only
	// read while the surface that set it is up, and all of those are now down, but a
	// stale ObjectRef surviving a switch is exactly the kind of thing a later leg
	// would resolve against the wrong cluster.
	m.deleteRes, m.deleteRef = kube.Resource{}, kube.ObjectRef{}
	m.mutateRes, m.mutateRef = kube.Resource{}, kube.ObjectRef{}
	m.ctrStreamRes, m.ctrStreamRef, m.ctrByLabel = kube.Resource{}, kube.ObjectRef{}, nil
	m.drainRes, m.drainRef, m.drainLabel = kube.Resource{}, kube.ObjectRef{}, ""
	m.pfPorts, m.pfPort = nil, kube.Port{}
	m.secretData, m.secretRevealed, m.secretSel, m.secretEntryLines = kube.SecretData{}, false, 0, nil
	m.searchTarget, m.hasSearchTarget = kube.ObjectRef{}, false
	m.clearReauth() // an armed remediation names the departing context's credentials.
	m.resByLabel = nil
	m.clearChildScope() // the scope names an owner on the departing cluster.

	// Back to the pre-drill-in browse panes. The menu is rebuilt rather than
	// cleared: a fresh menu.New is exactly the seed, and folding the extras back in
	// mirrors construction — and the extras it folds in are already the *new*
	// context's, installed by the switch before this call (M4-05/D163).
	m.menu = menu.New(m.styles)
	m.menu.AddExtras(m.menuExtras)
	m.menu.AddPinned(m.menuPinned) // the new context's pins, in the same order as construction
	m.menu.SetNamespace("")
	m.menu.Focus()
	m.table = table.New(m.styles)
	m.namespace = ""
	m.current, m.hasCurrent = kube.Resource{}, false
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.Reset()

	// The status bar describes the old cluster down to a transient toast about it,
	// and the spinner would otherwise keep turning for a pass that was just
	// cancelled.
	m.status.StopDiscovery()
	m.status.SetResourceType("")
	m.status.SetNamespace("")
	m.status.SetScope("")
	m.status.SetFilter("")
	m.status.ClearError()
	m.status.ClearNotice()
	m.welcome.SetNamespace("")

	m.resize()    // the rebuilt menu/table start unsized.
	m.syncHints() // focus is back on the menu → menu-context hints.
}

// namespacesLoadedMsg carries the outcome of the async namespace list issued when the
// palette's `:namespace ` stage opens (M2-08c, on the palette since PAL-05c-1).
// Listing happens off the update loop so opening that stage never blocks on the
// network.
//
// It carried a `dest` from PAL-03b until PAL-05c-1: two surfaces issued this load (the
// standalone ctrl+n picker and the stage) and a result had no other way to say which
// one asked. Retiring the picker leaves one destination, so the field went with it —
// *which* surface is no longer a question, though *whether* it is still open is, and
// that is what awaitingPaletteArg answers below.
type namespacesLoadedMsg struct {
	namespaces []string
	err        error
}

// loadNamespaces is the off-loop list itself. One caller since PAL-05c-1 — the
// palette's `:namespace ` stage — reached from `ctrl+n`, the menu's namespace-seam row
// and the typed line alike, so none of the three can drift into listing namespaces its
// own way.
func (m Model) loadNamespaces() tea.Cmd {
	lister := m.nsLister
	if lister == nil {
		return nil
	}
	return func() tea.Msg {
		ns, err := lister.Namespaces(context.Background())
		return namespacesLoadedMsg{namespaces: ns, err: err}
	}
}

// namespaceAllItem is the sentinel entry pinned at the top of the namespace list.
// Selecting it re-scopes the watch to every namespace (empty scope) — the app
// launches unscoped, so without this entry the list (which holds only concrete
// namespaces) is a one-way door: once a namespace is picked there is no way back to
// the all-namespaces view short of restarting (dogfood-09). A concrete namespace can
// never collide with it: DNS-label names cannot contain a space.
const namespaceAllItem = "all namespaces"

// namespaceItems renders the rows for a listed namespace set: the all-namespaces
// sentinel pinned at the top, then the namespaces as listed.
func namespaceItems(namespaces []string) []string {
	items := make([]string, 0, len(namespaces)+1)
	items = append(items, namespaceAllItem)
	items = append(items, namespaces...)
	return items
}

// handleNamespacesLoaded seeds the palette's `:namespace ` stage with the listed
// namespaces. A list failure surfaces a classified error and dismisses the palette
// (principle 3 — the switcher degrades, the app does not crash). A result that
// arrives after the stage moved on (the palette closed, or the line rewound to its
// verbs) is dropped: it belongs to a stage that no longer exists.
func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (tea.Model, tea.Cmd) {
	waiting := m.awaitingPaletteArg(keymap.ActionNamespace)
	if msg.err != nil {
		if waiting {
			m.closePalette()
		}
		return m, func() tea.Msg { return NewErrorMsg("list namespaces", msg.err) }
	}
	if !waiting {
		return m, nil // dismissed before the list arrived; ignore.
	}
	return m.fillPaletteArg(keymap.ActionNamespace, namespaceItems(msg.namespaces)), nil
}

// applyNamespaceValue applies a picked namespace row: it records the new scope on the
// model and the status bar, and re-scopes the live table by re-selecting the current
// resource with the new m.namespace (M2-07c's watch reads that field). The
// all-namespaces sentinel maps back to the empty scope (watch every namespace). With
// no resource open yet the scope is simply stored for the next selection.
func (m Model) applyNamespaceValue(value string) (tea.Model, tea.Cmd) {
	ns := value
	if ns == namespaceAllItem {
		ns = ""
	}
	m.setNamespace(ns)
	persist := m.persistNamespace(ns)
	if m.hasCurrent && m.watcher != nil {
		model, cmd := m.selectResource(m.current)
		return model, tea.Batch(persist, cmd)
	}
	return m, persist
}

// setNamespace applies a watch scope to the model and to the three surfaces that
// show it ("" = all namespaces, which every one of them renders as nothing). It is
// display state only: it starts no watch and persists nothing, so the two callers
// pair it with what they each need — a picked namespace with a re-select and a
// persist (applyNamespaceValue), a context switch with neither, since the reset
// left no resource open and the scope it restores came out of the state file it
// would be written back to (M4-05).
//
// It mutates the receiver, so callers pass the addressable model value they are
// about to return.
func (m *Model) setNamespace(ns string) {
	m.namespace = ns
	m.status.SetNamespace(ns)
	m.menu.SetNamespace(ns)    // keep the seam row's scope current
	m.welcome.SetNamespace(ns) // keep the welcome scope current if shown pre-drill-in
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

// activePicker returns a pointer to whichever modal picker is currently open — since
// PAL-05d that is the command palette (every verb's values, including the row actions
// `a` opens) or one of the two row-data pickers, containers and ports — or nil when
// none is. At most
// one is ever active — opening one does not open the other — so the root can route
// input and composite the overlay through this single accessor rather than branching
// on each picker. The pointer aliases into the value-receiver copy, so mutations
// through it persist in the returned model exactly like a direct field assignment.
func (m *Model) activePicker() *picker.Model {
	switch {
	case m.ctrPicker.Active():
		return &m.ctrPicker
	case m.cmdPicker.Active():
		return &m.cmdPicker
	case m.portPicker.Active():
		return &m.portPicker
	}
	return nil
}

// availableResources is the kind set the shell currently knows about: the menu's own
// item list narrowed to the available resource rows, so discovered CRDs and
// per-context extras are included and an unavailable kind is skipped. It is the one
// place that snapshot is taken — the cluster search fans out over it (SEARCH-04a) and
// the children drill-down resolves the child kind in it (M4-08/D165 pt 2) — so the
// two cannot disagree about what this cluster offers.
func (m Model) availableResources() []kube.Resource {
	items := m.menu.Items()
	out := make([]kube.Resource, 0, len(items))
	for _, it := range items {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		out = append(out, it.Resource)
	}
	return out
}

// resourcePickerItems renders the switchable resource kinds and the map resolving a
// picked label back to its kube.Resource. It seeds the palette's `:resource ` argument
// stage (PAL-03a) and its `:pin ` stage (CRD-PIN-05/D203 pt 4) — since PAL-05b the two
// stages are the only surfaces that list kinds, `R` among them — so the snapshot is the
// menu's own item list, taken once, in one place, and no two of them can come to offer
// different kinds.
//
// CRD-PIN-04 made this the surface the CRD-PIN line's premise rests on — "a kind you
// reach for once" is reached *here*, not by scrolling a menu of hundreds — so two ways
// it lost kinds are closed (D203):
//
//   - **Every listed kind is reachable.** Two Kinds of the same name in different API
//     groups (ordinary on a CRD-heavy cluster: a `Cluster`, a `Certificate` or a
//     `Policy` per operator) used to collapse to one row and the second kind could not
//     be picked at all. A name more than one row answers to is now qualified by its API
//     group on *every* row that shares it, so both are listed and tell themselves apart.
//   - **Every kind answers to the names it has.** The labels are Kinds, and a
//     Kubernetes user types what kubectl takes: the plural (`externalsecrets`), a short
//     name (`es`), or the group. None is a subsequence of the Kind, so all three matched
//     nothing. They ride along as picker aliases — matched, never shown.
func (m Model) resourcePickerItems() ([]picker.Item, map[string]kube.Resource) {
	rows := m.menu.Items()

	// Count the titles first: qualification has to apply to *both* sides of a
	// collision, which is only knowable after the whole list has been seen. (A title
	// unique to one row keeps its bare Kind — the common case, and the one muscle
	// memory is built on.)
	titles := make(map[string]int, len(rows))
	for _, it := range rows {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		titles[it.Title]++
	}

	items := make([]picker.Item, 0, len(rows))
	byLabel := make(map[string]kube.Resource, len(rows))
	for _, it := range rows {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		label := it.Title
		if titles[label] > 1 {
			label = qualifiedResourceLabel(it.Title, it.Resource.GVR.Group)
		}
		if _, dup := byLabel[label]; dup {
			continue // same Kind *and* group (two versions) — still ambiguous, keep the first.
		}
		byLabel[label] = it.Resource
		items = append(items, picker.Item{Label: label, Aliases: resourceAliases(it.Resource)})
	}
	return items, byLabel
}

// qualifiedResourceLabel disambiguates a Kind shared by more than one API group by
// naming the group: `Cluster (postgresql.cnpg.io)`. The core group has no name, so it
// is written `core` — the spelling kubectl and the API docs use for it, and better
// than an empty pair of brackets.
func qualifiedResourceLabel(title, group string) string {
	if group == "" {
		group = "core"
	}
	return title + " (" + group + ")"
}

// resourceAliases are the other names a kind answers to — the plural resource name,
// its short names, and its API group — matched by the picker's filter but never
// rendered (picker.Item).
//
// Discovery is what fills ShortNames, so a kind reached before the pass returns (a
// seed row, a pinned row) matches on its plural and Kind alone and gains the rest when
// discovery lands. That is a narrowing of what matches over time, never a change to
// what is listed, so nothing disappears from under the reader.
func resourceAliases(r kube.Resource) []string {
	aliases := make([]string, 0, len(r.ShortNames)+2)
	if r.GVR.Resource != "" {
		aliases = append(aliases, r.GVR.Resource)
	}
	aliases = append(aliases, r.ShortNames...)
	if r.GVR.Group != "" {
		aliases = append(aliases, r.GVR.Group)
	}
	return aliases
}

// The M3 actions menu (D107) was a modal picker of its own until PAL-05d: `a` now
// opens the palette's `:action ` stage over the same set (rowActionTitles, through
// paletteRowVerbs), so its opener, its Kind and its picker are gone rather than
// dormant (D207 pt 3 / D210). What survives is everything that was not the surface:
// the registry, the applicability predicates, and dispatchRowAction — which the stage,
// the verb-stage row entries and the direct keys all still end in.

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
// intent here (e.g. describe, logs, the View/Edit YAML edit flow); any action not yet
// wired falls through to a transient
// status-bar toast naming the action and target, so the routing stays observable and
// the dogfooder sees the action was recognised rather than the key seeming dead (D68)
// until its leg lands (D107).
func (m Model) handleRowAction(msg rowActionMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case rowActionDescribe:
		return m.openDescribeViewer(msg)
	case rowActionLogs:
		return m.openLogsViewer(msg)
	case rowActionSecret:
		return m.openSecretViewer(msg)
	case rowActionScale:
		return m.openScalePrompt(msg)
	case rowActionRolloutRestart:
		return m.openRolloutRestartConfirm(msg)
	case rowActionCordon:
		return m.runCordon(msg, true)
	case rowActionUncordon:
		return m.runCordon(msg, false)
	case rowActionSuspend:
		return m.runSuspend(msg, true)
	case rowActionResume:
		return m.runSuspend(msg, false)
	case rowActionDrain:
		return m.openDrainConfirm(msg)
	case rowActionPortForward:
		return m.openPortForwardPrompt(msg)
	case rowActionExec:
		return m.openExec(msg)
	case rowActionEdit:
		return m.openEdit(msg)
	case rowActionDelete:
		return m.openDeleteConfirm(msg)
	case rowActionChildren:
		return m.openChildren(msg)
	}
	label := rowActionTitle(msg.Action)
	if msg.Object.Name != "" {
		label += " " + msg.Object.Name
	}
	return m, m.surfaceError(ErrorMsg{Context: label + ": not yet available"})
}

// deleteModalKind stamps the confirm modal the delete action opens so
// modal.ConfirmedMsg/CancelledMsg route back to the delete flow. It is the first
// confirm wiring (D88's "the M3 action that needs it wires the modal into the
// shell"); later mutating actions (M3-10…12) reuse the same modal with their own
// kinds.
const deleteModalKind = "delete"

// deleteDoneMsg carries the outcome of the async kube.Delete issued once the confirm
// modal is accepted (M3-09). label is the human target ("Pod default/web-1") for the
// status-bar result. Unlike the viewer fetches there is no generation guard: a delete
// is a one-shot fire-and-report with no overlay to leave stale — its result only ever
// flashes a transient status message, and a superseded one is harmless.
type deleteDoneMsg struct {
	label string
	err   error
}

// openDeleteConfirm opens the confirm modal over the selected row before deleting it
// (M3-09): it stashes the target (resource + the row's ObjectRef, whose UID guards
// the snapshot race — M1-06a/D35) and shows a yes/no confirm. nav.drillIn accepts
// (ConfirmedMsg → the delete runs), nav.back declines (CancelledMsg → nothing
// happens) — no raw y/n (D11). With no deleter wired it is delete-inert (a no-op),
// exactly as the res.delete key is absent from a kind whose menu omits Delete.
func (m Model) openDeleteConfirm(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.deleter == nil {
		return m, nil
	}
	m.deleteRes = msg.Resource
	m.deleteRef = msg.Object
	target := viewerTitle(msg.Resource, msg.Object)
	m.modal.ShowConfirm(deleteModalKind, "Delete", "Delete "+target+"?")
	return m, nil
}

// handleModalConfirmed runs the action the accepted modal stands for. Delete is the
// only wired kind so far (M3-09); later mutating actions add their own case. The
// modal is hidden first — the accept resolves it — then the kube call is issued off
// the update loop, its result reported to the status bar (handleDeleteDone).
func (m Model) handleModalConfirmed(msg modal.ConfirmedMsg) (tea.Model, tea.Cmd) {
	m.modal.Hide()
	switch msg.Kind {
	case deleteModalKind:
		return m.runDelete()
	case scaleModalKind:
		return m.runScale(msg.Value)
	case rolloutRestartModalKind:
		return m.runRolloutRestart()
	case drainModalKind:
		return m.runDrain()
	case portForwardModalKind:
		return m.runPortForward(msg.Value)
	case localPortModalKind:
		return m.runLocalPortForward(msg.Value)
	case reauthModalKind:
		// The one kind whose confirm was opened by kubecom rather than by a keypress
		// (AUTH-05b): accepting it suspends the TUI into the remediation the prompt
		// named, which is the only place the shell runs a command it composed (D215).
		return m.acceptReauth()
	}
	return m, nil
}

// runDelete issues the stashed delete off the update loop and reports its outcome via
// deleteDoneMsg. The deleted row leaves the table on its own — the delete triggers a
// watch DELETED event the live table already folds in (ApplyEvent) — so nothing here
// touches the table. Inert if the deleter went away or no target is stashed (a
// declined-then-somehow-reentered modal), so an empty ref never reaches kube.Delete.
func (m Model) runDelete() (tea.Model, tea.Cmd) {
	if m.deleter == nil || m.deleteRef.Name == "" {
		return m, nil
	}
	deleter, r, ref := m.deleter, m.deleteRes, m.deleteRef
	label := viewerTitle(r, ref)
	return m, func() tea.Msg {
		err := deleter.Delete(context.Background(), r, ref, metav1.DeleteOptions{})
		return deleteDoneMsg{label: label, err: err}
	}
}

// handleDeleteDone reports a completed delete: a failure (NotFound, RBAC, or a UID
// Conflict from the row-snapshot guard) degrades to a transient status-bar error
// toast (D74), a success to a neutral status notice. Either way the layout never
// breaks and the row itself leaves via the watch stream, not this handler.
func (m Model) handleDeleteDone(msg deleteDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("delete "+msg.label, msg.err))
	}
	return m, m.surfaceNotice("deleted " + msg.label)
}

// scaleModalKind / rolloutRestartModalKind stamp the modals the two M3-10 mutating
// workload actions open, so modal.ConfirmedMsg/CancelledMsg route back to the right
// flow — the same one-modal-many-kinds pattern delete established (D115). Scale uses
// the modal's prompt mode (a replica count), rollout-restart its confirm mode.
const (
	scaleModalKind          = "scale"
	rolloutRestartModalKind = "rolloutRestart"
)

// scaleDoneMsg / restartDoneMsg carry the outcome of the async kube call issued once
// the scale prompt is submitted / the rollout-restart confirm is accepted (M3-10).
// Like deleteDoneMsg they are one-shot fire-and-report with no generation guard: the
// result only flashes a transient status message, so a superseded one is harmless.
type scaleDoneMsg struct {
	label    string
	replicas int32
	err      error
}

type restartDoneMsg struct {
	label string
	err   error
}

// openScalePrompt opens the modal's replica prompt over the selected workload before
// scaling it (M3-10): it stashes the target (resource + the row's ObjectRef) and shows
// a single-line prompt. nav.drillIn submits the typed count (ConfirmedMsg with Value →
// the scale runs), nav.back cancels (CancelledMsg → nothing happens) — no raw digits
// matched as behaviour, the field captures them via UpdatePrompt (D11). With no scaler
// wired it is scale-inert (a no-op), exactly as the Scale entry is absent from a kind
// whose menu omits it. Returns the prompt's focus cmd so the cursor blinks.
func (m Model) openScalePrompt(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.scaler == nil {
		return m, nil
	}
	m.mutateRes = msg.Resource
	m.mutateRef = msg.Object
	target := viewerTitle(msg.Resource, msg.Object)
	cmd := m.modal.ShowPrompt(scaleModalKind, "Scale", "Replicas for "+target+":", "")
	return m, cmd
}

// runScale parses the submitted replica count and issues the stashed scale off the
// update loop, reporting its outcome via scaleDoneMsg. A blank or non-integer entry
// (or a negative count) degrades to a transient status-bar error toast (D74) and runs
// nothing — the modal is already hidden, so the user re-invokes to retry. Inert if the
// scaler went away or no target is stashed, so an empty ref never reaches kube.Scale.
func (m Model) runScale(value string) (tea.Model, tea.Cmd) {
	if m.scaler == nil || m.mutateRef.Name == "" {
		return m, nil
	}
	label := viewerTitle(m.mutateRes, m.mutateRef)
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return m, m.surfaceError(ErrorMsg{Context: "scale " + label + ": replicas must be a whole number"})
	}
	if n < 0 {
		return m, m.surfaceError(ErrorMsg{Context: "scale " + label + ": replicas must be >= 0"})
	}
	scaler, r, ref, replicas := m.scaler, m.mutateRes, m.mutateRef, int32(n)
	return m, func() tea.Msg {
		err := scaler.Scale(context.Background(), r, ref, replicas)
		return scaleDoneMsg{label: label, replicas: replicas, err: err}
	}
}

// handleScaleDone reports a completed scale: a failure (NotFound, RBAC) degrades to a
// transient error toast (D74), a success to a neutral status notice naming the new
// replica count. The workload's own row updates via the live watch stream, not here.
func (m Model) handleScaleDone(msg scaleDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("scale "+msg.label, msg.err))
	}
	return m, m.surfaceNotice(fmt.Sprintf("scaled %s to %d", msg.label, msg.replicas))
}

// openRolloutRestartConfirm opens the confirm modal over the selected workload before
// rollout-restarting it (M3-10): it stashes the target and shows a yes/no confirm.
// nav.drillIn accepts (ConfirmedMsg → the restart runs), nav.back declines
// (CancelledMsg → nothing happens) — no raw y/n (D11). With no restarter wired it is
// restart-inert (a no-op), like the Rollout-restart entry being absent from the menu.
func (m Model) openRolloutRestartConfirm(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.restarter == nil {
		return m, nil
	}
	m.mutateRes = msg.Resource
	m.mutateRef = msg.Object
	target := viewerTitle(msg.Resource, msg.Object)
	m.modal.ShowConfirm(rolloutRestartModalKind, "Rollout restart", "Rollout restart "+target+"?")
	return m, nil
}

// runRolloutRestart issues the stashed rollout-restart off the update loop and reports
// its outcome via restartDoneMsg. Inert if the restarter went away or no target is
// stashed, so an empty ref never reaches kube.RolloutRestart.
func (m Model) runRolloutRestart() (tea.Model, tea.Cmd) {
	if m.restarter == nil || m.mutateRef.Name == "" {
		return m, nil
	}
	restarter, r, ref := m.restarter, m.mutateRes, m.mutateRef
	label := viewerTitle(r, ref)
	return m, func() tea.Msg {
		err := restarter.RolloutRestart(context.Background(), r, ref)
		return restartDoneMsg{label: label, err: err}
	}
}

// handleRestartDone reports a completed rollout-restart: a failure degrades to a
// transient error toast (D74), a success to a neutral status notice. The rolling
// pods surface through the live watch stream, not here.
func (m Model) handleRestartDone(msg restartDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("rollout restart "+msg.label, msg.err))
	}
	return m, m.surfaceNotice("restarted " + msg.label)
}

// cordonDoneMsg carries the outcome of the async cordon/uncordon issued when the
// Node action is dispatched (M3-11a). cordon distinguishes the two so the status
// message and error context read naturally. Like the other mutating done-messages it
// is one-shot fire-and-report with no generation guard: the result only flashes a
// transient status message, so a superseded one is harmless.
type cordonDoneMsg struct {
	label  string
	cordon bool
	err    error
}

// runCordon dispatches the cordon (cordon==true) or uncordon (false) on the selected
// Node off the update loop, reporting its outcome via cordonDoneMsg. Unlike delete/
// scale/rollout-restart there is **no confirm modal** (D115) and **no target stash**:
// cordoning is idempotent (D35), so the action fires straight from handleRowAction
// with the row's ref in hand. With no cordoner wired, or an empty ref (a row with no
// name — cluster-scoped Nodes are always named, but guard anyway so an empty ref never
// reaches the kube layer), it is a no-op — exactly as the Cordon/Uncordon entries are
// absent from a non-Node kind's actions menu. The Node's Unschedulable status flips in
// the table via the live watch stream, not here.
func (m Model) runCordon(msg rowActionMsg, cordon bool) (tea.Model, tea.Cmd) {
	if m.cordoner == nil || msg.Object.Name == "" {
		return m, nil
	}
	cordoner, r, ref := m.cordoner, msg.Resource, msg.Object
	label := viewerTitle(r, ref)
	return m, func() tea.Msg {
		var err error
		if cordon {
			err = cordoner.Cordon(context.Background(), r, ref)
		} else {
			err = cordoner.Uncordon(context.Background(), r, ref)
		}
		return cordonDoneMsg{label: label, cordon: cordon, err: err}
	}
}

// handleCordonDone reports a completed cordon/uncordon: a failure (NotFound, RBAC)
// degrades to a transient error toast (D74), a success to a neutral status notice. The
// Node's Unschedulable status updates via the live watch stream, not here.
func (m Model) handleCordonDone(msg cordonDoneMsg) (tea.Model, tea.Cmd) {
	verb, past := "cordon", "cordoned"
	if !msg.cordon {
		verb, past = "uncordon", "uncordoned"
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg(verb+" "+msg.label, msg.err))
	}
	return m, m.surfaceNotice(past + " " + msg.label)
}

// suspendDoneMsg carries the outcome of the async suspend/resume issued when the
// CronJob action is dispatched (M3-12). suspend distinguishes the two so the status
// message and error context read naturally. Like cordonDoneMsg it is one-shot
// fire-and-report with no generation guard: the result only flashes a transient
// status message, so a superseded one is harmless.
type suspendDoneMsg struct {
	label   string
	suspend bool
	err     error
}

// runSuspend dispatches the suspend (suspend==true) or resume (false) on the selected
// CronJob off the update loop, reporting its outcome via suspendDoneMsg. Mirroring
// runCordon (D120) there is **no confirm modal** (D115) and **no target stash**:
// suspending is idempotent (D35), so the action fires straight from handleRowAction
// with the row's ref in hand. With no suspender wired, or an empty ref (a row with no
// name, guarded so an empty ref never reaches the kube layer), it is a no-op — exactly
// as the Suspend/Resume entries are absent from a non-CronJob kind's actions menu. The
// CronJob's suspended state flips in the table via the live watch stream, not here.
func (m Model) runSuspend(msg rowActionMsg, suspend bool) (tea.Model, tea.Cmd) {
	if m.suspender == nil || msg.Object.Name == "" {
		return m, nil
	}
	suspender, r, ref := m.suspender, msg.Resource, msg.Object
	label := viewerTitle(r, ref)
	return m, func() tea.Msg {
		var err error
		if suspend {
			err = suspender.Suspend(context.Background(), r, ref)
		} else {
			err = suspender.Resume(context.Background(), r, ref)
		}
		return suspendDoneMsg{label: label, suspend: suspend, err: err}
	}
}

// handleSuspendDone reports a completed suspend/resume: a failure (NotFound, RBAC)
// degrades to a transient error toast (D74), a success to a neutral status notice. The
// CronJob's suspended state updates via the live watch stream, not here.
func (m Model) handleSuspendDone(msg suspendDoneMsg) (tea.Model, tea.Cmd) {
	verb, past := "suspend", "suspended"
	if !msg.suspend {
		verb, past = "resume", "resumed"
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg(verb+" "+msg.label, msg.err))
	}
	return m, m.surfaceNotice(past + " " + msg.label)
}

// drainModalKind stamps the confirm modal the drain action opens, so
// modal.ConfirmedMsg routes back to runDrain — the same one-modal-many-kinds
// pattern delete/scale/rollout established (D115). Unlike cordon/uncordon (D120) a
// drain evicts pods, so it needs the confirm.
const drainModalKind = "drain"

// defaultDrainOptions is kubecom's drain policy (M3-11b). IgnoreDaemonSets is on
// because virtually every real cluster runs DaemonSet pods (CNI, kube-proxy, log/
// metric agents) that are never evictable anyway — without it every drain would be
// refused, which is useless as a default — while Force and DeleteEmptyDirData stay
// off: those are the data-loss-risking flags (evicting an unmanaged pod's only
// copy, discarding an emptyDir's contents), so the strict default *refuses* upfront
// with a message naming the blocking pod (DrainCandidates) rather than silently
// destroying data. A future leg can surface these as toggles on the confirm (D121).
var defaultDrainOptions = kube.DrainOptions{IgnoreDaemonSets: true}

// drainMsg wraps one message from the drain pump with the drainGen of the drain
// that started it, so a step from a drain already superseded (a newer drain started,
// bumping drainGen) or cancelled (quit) is dropped rather than reported, and its
// pump chain stopped — mirroring logMsg's stale-stream guard.
type drainMsg struct {
	gen int
	msg tea.Msg
}

// openDrainConfirm opens the confirm modal over the selected Node before draining it
// (M3-11b): it stashes the target (resource + the row's ObjectRef) and shows a yes/no
// confirm naming what will happen. nav.drillIn accepts (ConfirmedMsg → runDrain),
// nav.back declines (CancelledMsg → nothing happens) — no raw y/n (D11). With no
// drainer wired it is drain-inert (a no-op), exactly as the Drain entry is absent
// from a non-Node kind's actions menu.
func (m Model) openDrainConfirm(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.drainer == nil {
		return m, nil
	}
	m.drainRes = msg.Resource
	m.drainRef = msg.Object
	target := viewerTitle(msg.Resource, msg.Object)
	m.modal.ShowConfirm(drainModalKind, "Drain", "Drain "+target+"? Its pods will be evicted.")
	return m, nil
}

// runDrain starts the stashed node drain once the confirm is accepted (M3-11b): it
// opens a cancellable DrainStream on the shared defaultDrainOptions, bumps drainGen
// so a prior in-flight drain's pumped steps are dropped, flashes a starting notice,
// and begins pumping the progress channel step by step (D53) — a long eviction loop
// never blocks Update. stopDrain first cancels any prior drain; on quit the same
// teardown cancels this one (cancel-on-quit). Inert if the drainer went away or no
// target is stashed, so an empty ref never reaches the kube layer.
func (m Model) runDrain() (tea.Model, tea.Cmd) {
	if m.drainer == nil || m.drainRef.Name == "" {
		return m, nil
	}
	m.stopDrain() // supersede any prior drain before starting a new one.
	m.drainGen++
	gen := m.drainGen
	m.drainLabel = viewerTitle(m.drainRes, m.drainRef)
	ctx, cancel := context.WithCancel(context.Background())
	m.drainCancel = cancel
	m.drainCh = m.drainer.DrainStream(ctx, m.drainRes, m.drainRef, defaultDrainOptions)
	return m, tea.Batch(m.surfaceNotice("draining "+m.drainLabel+"…"), m.pumpDrain(gen))
}

// pumpDrain issues the tea.Cmd that pulls the next progress event from the current
// drain channel, tagged with the drainGen that started it so a step from a
// superseded/cancelled drain is recognisable as stale. Returns nil when no drain is
// active.
func (m Model) pumpDrain(gen int) tea.Cmd {
	ch := m.drainCh
	if ch == nil {
		return nil
	}
	pump := drainPump(ch)
	return func() tea.Msg { return drainMsg{gen: gen, msg: pump()} }
}

// handleDrainMsg applies one drain-pump message and re-issues the pump to pull the
// next step — the one-receive-per-Cmd loop that keeps Update from blocking on a long
// drain (D53). A message from a superseded/cancelled drain (wrong gen) is dropped and
// its chain stops. A progress step flashes a neutral status notice and re-pumps; a
// DrainDoneMsg ends the chain, reporting a failure as a transient error toast (D74)
// or a success as a neutral notice, and tears the stream down (stopDrain).
func (m Model) handleDrainMsg(d drainMsg) (tea.Model, tea.Cmd) {
	if d.gen != m.drainGen {
		return m, nil // superseded or cancelled drain; drop and stop this chain.
	}
	switch inner := d.msg.(type) {
	case DrainProgressMsg:
		return m, tea.Batch(m.surfaceNotice(inner.Message), m.pumpDrain(d.gen))
	case DrainDoneMsg:
		label := m.drainLabel
		m.stopDrain()
		if inner.Err != nil {
			return m, m.surfaceError(NewErrorMsg("drain "+label, inner.Err))
		}
		return m, m.surfaceNotice("drained " + label)
	}
	return m, nil
}

// stopDrain cancels the live drain (if any) and clears its handles, so the drain's
// goroutine is torn down and no stale step is pumped. Safe to call with no drain
// active. Called before starting a new drain, on completion, and on quit — the
// mutating twin of stopLogStream.
func (m *Model) stopDrain() {
	if m.drainCancel != nil {
		m.drainCancel()
		m.drainCancel = nil
	}
	m.drainCh = nil
}

// portForwardModalKind stamps the ports prompt the Port-forward action opens, so
// modal.ConfirmedMsg routes back to runPortForward (M3-13a) — the same one-modal-
// many-kinds dispatch scale/delete/drain use.
const portForwardModalKind = "portForward"

// forward is one running background port-forward the shell tracks (M3-13a): the
// stable id, the human label for the status message (and the M3-13b panel), the
// requested port specs, the bound local:remote pairs (filled once Ready fires), the
// ActiveForward handle, and the context cancel that stops it. Held in m.forwards and
// torn down by stopForwards on quit. Touched only from the single-threaded update loop.
type forward struct {
	id     int
	label  string
	specs  []string
	bound  []kube.ForwardedPort
	handle ActiveForward
	cancel context.CancelFunc
	ready  bool
}

// forwardReadyMsg reports that a started forward's local listeners are up (its bound
// ports are then readable). forwardDoneMsg reports it ended — a clean stop (Err nil)
// or a fatal transport error. Both carry the forward's stable id so the handler finds
// its entry regardless of slice order.
type forwardReadyMsg struct{ id int }
type forwardDoneMsg struct {
	id  int
	err error
}

// pfPodResource labels the ports prompt (and the resulting forward) for a pod
// resolved from a Service (M3-13c): the prompt title and forward label show the
// resolved *pod*, not the Service, so the user sees which endpoint pod is forwarding.
// It carries only the Pod kind — viewerTitle reads GVK.Kind — since the resolved pod's
// namespace/name come from its ObjectRef (the logs viewer's podLogResource twin).
var pfPodResource = kube.Resource{GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"}}

// serviceResolvedMsg carries the outcome of the async PodForService resolution issued
// when the Port-forward action is invoked on a Service (M3-13c). gen ties it to the
// pfResolveGen bumped when the resolution was requested, so a result that lands after
// a newer port-forward request supersedes it is dropped rather than opening a stale
// prompt. ref is the resolved backing pod; svc is the Service it was resolved from,
// carried through so its declared ports can be listed against both (ServicePorts maps
// each targetPort onto the backing pod, FB-pf-port-picker-b/D137).
type serviceResolvedMsg struct {
	gen int
	ref kube.ObjectRef
	svc kube.ObjectRef
	err error
}

// openPortForwardPrompt starts the Port-forward flow over the selected row (M3-13a/c).
// With no port-forwarder wired it is inert (a no-op), exactly as the Port-forward
// entry is absent from a kind whose menu omits it. A Pod forwards directly: its ports
// prompt opens immediately. A Service can't be forwarded directly (kube.PortForward
// posts to the pod portforward subresource), so it is first resolved to a backing
// endpoint pod off the update loop (M3-13c, mirroring the logs viewer's PodResolver
// hop); the prompt then opens over that resolved pod (handleServiceResolved). Without a
// service resolver wired a Service degrades to a toast rather than a silent no-op.
//
// With a port lister wired the resolved pod's *declared* ports are listed first and
// offered as choices (FB-pf-port-picker-b, resolvePortsFor); the free-text prompt stays
// the fallback for an object that declares nothing (D137).
func (m Model) openPortForwardPrompt(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.portForwarder == nil {
		return m, nil
	}
	if msg.Resource.GVK.Kind == "Service" {
		if m.serviceResolver == nil {
			return m, m.surfaceError(ErrorMsg{Context: "port-forward for " + msg.Resource.GVK.Kind + ": not yet available"})
		}
		m.pfResolveGen++
		gen := m.pfResolveGen
		resolver := m.serviceResolver
		ref := msg.Object
		return m, func() tea.Msg {
			pod, err := resolver.PodForService(context.Background(), ref)
			return serviceResolvedMsg{gen: gen, ref: pod, svc: ref, err: err}
		}
	}
	return m.resolvePortsFor(msg.Resource, msg.Object, kube.ObjectRef{})
}

// showPortForwardPrompt opens the modal's ports prompt over ref (a Pod — either a Pod
// row directly, or the endpoint pod a Service resolved to) before forwarding it. It
// stashes the target in the shared mutate stash and shows a single-line prompt;
// nav.drillIn submits the typed spec (ConfirmedMsg with Value → runPortForward),
// nav.back cancels — no raw keys, the field captures input (D11).
func (m Model) showPortForwardPrompt(res kube.Resource, ref kube.ObjectRef) (tea.Model, tea.Cmd) {
	m.mutateRes = res
	m.mutateRef = ref
	target := viewerTitle(res, ref)
	cmd := m.modal.ShowPrompt(portForwardModalKind, "Port-forward", "Ports for "+target+" (e.g. 8080:80, :80=free local):", "")
	return m, cmd
}

// handleServiceResolved acts on a backing pod resolved for a Service port-forward
// (M3-13c). A result whose gen no longer matches (a newer port-forward request
// superseded it) is dropped; a resolution error (a selector-less Service, no
// matching/ready pod, RBAC denial) degrades to a status-bar toast (D74) without
// opening the prompt. On success it feeds the resolved pod into the declared-ports
// resolution (FB-pf-port-picker-b) — reading the *Service's* ports, since those are
// what the user knows it by and ServicePorts maps each onto the pod-side number a
// forward must target — which opens the picker or falls back to the ports prompt over
// the resolved pod, titled as a Pod so the user sees which endpoint pod is forwarding.
func (m Model) handleServiceResolved(msg serviceResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.pfResolveGen {
		return m, nil // superseded by a newer port-forward request; drop.
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("port-forward", msg.err))
	}
	return m.resolvePortsFor(pfPodResource, msg.ref, msg.svc)
}

// runPortForward parses the submitted port spec and starts the stashed forward off
// the update loop. The specs use kubectl's syntax ("8080:80", "80", ":80"), space-
// or comma-separated for several at once. A blank entry, or a spec kube.PortForward
// rejects (empty/malformed), degrades to a transient error toast (D74) and starts
// nothing — the modal is already hidden, so the user re-invokes to retry. On success
// the forward is tracked and its lifecycle observed via waitForward. Inert if the
// forwarder went away or no target is stashed.
func (m Model) runPortForward(value string) (tea.Model, tea.Cmd) {
	if m.portForwarder == nil || m.mutateRef.Name == "" {
		return m, nil
	}
	label := viewerTitle(m.mutateRes, m.mutateRef)
	specs := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(specs) == 0 {
		return m, m.surfaceError(ErrorMsg{Context: "port-forward " + label + ": enter at least one port (e.g. 8080:80)"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, err := m.portForwarder.PortForward(ctx, m.mutateRef, specs)
	if err != nil {
		cancel()
		return m, m.surfaceError(NewErrorMsg("port-forward "+label, err))
	}
	m.forwardSeq++
	f := &forward{id: m.forwardSeq, label: label, specs: specs, handle: h, cancel: cancel}
	m.forwards = append(m.forwards, f)
	return m, tea.Batch(m.surfaceNotice("port-forward "+label+"…"), waitForward(f.id, h))
}

// waitForward blocks on the forward's Ready and Done channels and reports whichever
// fires first — a forwardReadyMsg once listeners are up, or a forwardDoneMsg if it
// ended before ever becoming ready (a dial failure). It runs on the Cmd's own
// goroutine (principle 1) so the update loop never blocks; handleForwardReady re-arms
// the Done wait via waitForwardDone.
func waitForward(id int, h ActiveForward) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-h.Ready():
			return forwardReadyMsg{id: id}
		case <-h.Done():
			return forwardDoneMsg{id: id, err: h.Err()}
		}
	}
}

// waitForwardDone blocks until the (already-ready) forward ends and reports the
// cause. Issued after Ready so a live forward's eventual stop/failure is observed.
func waitForwardDone(id int, h ActiveForward) tea.Cmd {
	return func() tea.Msg {
		<-h.Done()
		return forwardDoneMsg{id: id, err: h.Err()}
	}
}

// handleForwardReady marks the forward ready, reads its bound local:remote ports,
// flashes a neutral status notice naming them, and arms the Done wait. A forward
// removed before this lands (a race with stopForwards) is dropped. A Ports read error
// is non-fatal — the forward is up; the notice just falls back to the requested specs.
func (m Model) handleForwardReady(msg forwardReadyMsg) (tea.Model, tea.Cmd) {
	f := m.forwardByID(msg.id)
	if f == nil {
		return m, nil
	}
	f.ready = true
	if bound, err := f.handle.Ports(); err == nil {
		f.bound = bound
	}
	return m, tea.Batch(m.surfaceNotice("forwarding "+f.label+" "+forwardPortsLabel(f)), waitForwardDone(f.id, f.handle))
}

// handleForwardDone removes the ended forward and reports the outcome: a fatal
// transport error degrades to a transient error toast (D74), a clean stop to a neutral
// notice. Cancelling the forward's context (stopForwards, or a future stop gesture)
// ends it cleanly, so a user-stopped forward reads as a plain notice.
func (m Model) handleForwardDone(msg forwardDoneMsg) (tea.Model, tea.Cmd) {
	f := m.forwardByID(msg.id)
	if f == nil {
		return m, nil
	}
	label := f.label
	specs := f.specs
	f.cancel() // release the context bridged to Stop; idempotent.
	m.removeForward(msg.id)
	m.clampForwardsSel() // a removed entry may have left the panel cursor past the end.
	if msg.err != nil {
		// A local-listener bind failure (almost always: the local port is already
		// taken — e.g. forwarding Redis 6379 while Redis runs locally) surfaces from
		// client-go as an opaque "unable to listen on any of the requested ports:
		// [{6379 6379}]". Replace it with an actionable hint naming the requested
		// local port(s) and how to let the OS pick a free one (feedback 2026-07-24).
		if isPortForwardBindErr(msg.err) {
			return m, m.surfaceError(ErrorMsg{Context: "port-forward " + label + ": " + portForwardBindHint(specs)})
		}
		return m, m.surfaceError(NewErrorMsg("port-forward "+label, msg.err))
	}
	return m, m.surfaceNotice("stopped port-forward " + label)
}

// forwardByID returns the tracked forward with the given id, or nil if it is gone.
func (m Model) forwardByID(id int) *forward {
	for _, f := range m.forwards {
		if f.id == id {
			return f
		}
	}
	return nil
}

// removeForward drops the forward with the given id, preserving the order of the rest
// (the M3-13b panel lists them in start order).
func (m *Model) removeForward(id int) {
	out := make([]*forward, 0, len(m.forwards))
	for _, f := range m.forwards {
		if f.id != id {
			out = append(out, f)
		}
	}
	m.forwards = out
}

// stopForwards cancels every running forward and clears the set, so all background
// forward goroutines are torn down on quit (cancel-on-exit). Safe with none active.
func (m *Model) stopForwards() {
	for _, f := range m.forwards {
		f.cancel()
	}
	m.forwards = nil
}

// portForwardBindErr is the sentinel substring in client-go's error when it cannot
// bind the local listener for any requested port. Matching it lets the shell replace
// the opaque "unable to listen on any of the requested ports: [{6379 6379}]" with an
// actionable hint instead of a dead-end (feedback 2026-07-24). It is a substring
// (not equality) because client-go appends the offending {local remote} pairs.
const portForwardBindErr = "unable to listen on any of the requested ports"

// isPortForwardBindErr reports whether err is a local-listener bind failure — the
// case that almost always means the requested local port is already in use.
func isPortForwardBindErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), portForwardBindErr)
}

// portForwardBindHint turns the requested port specs into an actionable retry message
// for a local-listener bind failure: it names the local port(s) that couldn't bind and
// shows how to let the OS pick a free one — the leading-colon form ":<remote>", which
// kube.PortForward accepts (an empty local half means "OS-assigned"). e.g. specs
// ["6379"] → `local port 6379 already in use — retry with :6379 to auto-assign a free
// local port`. It never suggests ":0": client-go reads the half after the colon as the
// *remote* port and rejects 0 (D139). Since FB-pf-local-port the port picker also has
// a one-key free-local gesture, so this hint is the fallback for a forward that was
// already started, not the only escape.
func portForwardBindHint(specs []string) string {
	locals := make([]string, 0, len(specs))
	example := ""
	for i, s := range specs {
		local, remote := splitPortSpec(s)
		if local != "" {
			locals = append(locals, local)
		}
		if i == 0 && remote != "" {
			example = ":" + remote // the first spec gives a concrete retry example.
		}
	}
	if len(locals) == 0 || example == "" {
		// Every spec already auto-assigns the local port (or names no remote to build
		// a retry from), so a bind failure isn't a plain port clash — keep the message
		// generic rather than suggesting a retry that changes nothing.
		return "could not bind the local listener for the requested ports"
	}
	word := "port"
	if len(locals) > 1 {
		word = "ports"
	}
	return fmt.Sprintf("local %s %s already in use — retry with %s to auto-assign a free local port",
		word, strings.Join(locals, ", "), example)
}

// splitPortSpec parses one kubectl port-forward spec into its local and remote
// halves: "8080:80" → ("8080","80"), "80" → ("80","80"), ":80" → ("","80"). It does
// not validate the numbers — kube.PortForward already rejects malformed specs.
func splitPortSpec(spec string) (local, remote string) {
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, spec
}

// forwardPortsLabel renders a forward's ports for the status notice: the bound
// local:remote pairs once Ready has filled them (e.g. "localhost:8080 → 80"), else the
// requested specs as typed.
func forwardPortsLabel(f *forward) string {
	if len(f.bound) == 0 {
		return strings.Join(f.specs, " ")
	}
	parts := make([]string, len(f.bound))
	for i, p := range f.bound {
		parts[i] = fmt.Sprintf("localhost:%d → %d", p.Local, p.Remote)
	}
	return strings.Join(parts, ", ")
}

// openForwardsPanel shows the port-forward panel (M3-13b), a global overlay listing
// the active background forwards. It is not row-scoped — forwards outlive the row they
// started on — so it opens from anywhere in the browse view. The cursor is clamped to
// the current set (a forward may have ended since it was last open).
func (m Model) openForwardsPanel() (tea.Model, tea.Cmd) {
	m.forwardsPanel = true
	m.clampForwardsSel()
	return m, nil
}

// clampForwardsSel keeps forwardsSel a valid index into m.forwards: 0 when empty,
// otherwise within [0, len-1]. Called whenever the set or the panel opens changes.
func (m *Model) clampForwardsSel() {
	if m.forwardsSel < 0 || len(m.forwards) == 0 {
		m.forwardsSel = 0
		return
	}
	if m.forwardsSel >= len(m.forwards) {
		m.forwardsSel = len(m.forwards) - 1
	}
}

// handleForwardsPanelAction routes a resolved action to the open port-forward panel
// (M3-13b). forwards.panel (`F`), nav.back and app.quit close it (the overlay owns the
// quit key while up, like help/viewer); nav.up/down move the cursor; nav.drillIn stops
// the selected forward (cancelling its context; the resulting forwardDoneMsg removes it
// and reports it stopped); forwards.stopAll stops every forward at once. Everything else
// is swallowed so the browse panes underneath stay put.
func (m Model) handleForwardsPanelAction(a keymap.Action) (tea.Model, tea.Cmd) {
	switch a {
	case keymap.ActionForwards, keymap.ActionBack, keymap.ActionQuit:
		m.forwardsPanel = false
		return m, nil
	case keymap.ActionUp:
		if m.forwardsSel > 0 {
			m.forwardsSel--
		}
		return m, nil
	case keymap.ActionDown:
		if m.forwardsSel < len(m.forwards)-1 {
			m.forwardsSel++
		}
		return m, nil
	case keymap.ActionDrillIn:
		// Stop the selected forward: cancelling its context ends it, and the
		// forwardDoneMsg that follows removes the entry + flashes "stopped …" and
		// re-clamps the cursor (handleForwardDone). Inert when the set is empty.
		if m.forwardsSel < len(m.forwards) {
			m.forwards[m.forwardsSel].cancel()
		}
		return m, nil
	case keymap.ActionStopForwards:
		// Stop every forward at once: stopForwards cancels each context and clears the
		// set immediately, so the panel drops to its empty state. The background wait
		// goroutines still fire forwardDoneMsg, but those ids are already gone and are
		// dropped (handleForwardDone). A single neutral notice reports the sweep.
		if len(m.forwards) == 0 {
			return m, nil
		}
		m.stopForwards()
		m.clampForwardsSel()
		return m, m.surfaceNotice("stopped all port-forwards")
	}
	return m, nil
}

// forwardsPanelView renders the port-forward panel (M3-13b): a bordered box listing
// each active forward — its label and bound (or requested) ports and whether it is
// ready — with the cursor row highlighted, plus a footer of the panel's keys. With no
// active forwards it shows an empty-state line. Composited centered over the browse
// view by View (overlayCenter, D95), like the modal.
//
// The box bounds its own height (D220 pt 1): overlayCenter flattens onto a fixed
// width×bodyHeight canvas and clips bottom-first, so an unbounded list used to cost
// the panel its footer and its bottom border, and — because this is the one overlay
// with a *cursor* — could hide the selected row with nothing on screen saying so
// (BOX-02). The rows therefore scroll rather than truncate: a window of what fits
// that follows m.forwardsSel, derived from the cursor alone so the panel keeps no
// scroll state of its own to resize, clamp, or forget. The title and the footer are
// rendered first-class like the modal's title and input; only the list is elided,
// and the title says so by counting (see forwardsPanelTitle).
func (m Model) forwardsPanelView() string {
	iw := m.width - 6 // leave a margin; the box border adds 2 back.
	if iw > 64 {
		iw = 64
	}
	if iw < 20 {
		iw = 20
	}
	ih := m.bodyHeight() - 2 // the border takes one row at the top and one at the bottom
	if ih <= 0 {
		return "" // nothing the compositor would not clip away entirely
	}
	// The title always takes a row; the footer only when a content row survives it —
	// a box listing nothing but its keys is worse than one with no footer.
	rows := ih - 1
	footer := forwardsPanelFooter(m.keymap)
	if footer != "" && rows >= 2 {
		rows--
	} else {
		footer = ""
	}

	start, end := forwardsWindow(len(m.forwards), m.forwardsSel, rows)
	lines := []string{m.styles.Header.Width(iw).MaxWidth(iw).Render(forwardsPanelTitle(len(m.forwards), start, end))}
	switch {
	case rows <= 0: // title-only box: the screen has room for nothing else
	case len(m.forwards) == 0:
		lines = append(lines, m.styles.Subtle.Width(iw).MaxWidth(iw).Render("No active port-forwards."))
	default:
		for i := start; i < end; i++ {
			f := m.forwards[i]
			status := "starting…"
			if f.ready {
				status = "ready"
			}
			row := fmt.Sprintf("%s  %s  [%s]", f.label, forwardPortsLabel(f), status)
			gutter := "  "
			style := m.styles.App
			if i == m.forwardsSel {
				gutter = "> "
				style = m.styles.Selection
			}
			lines = append(lines, style.Width(iw).MaxWidth(iw).Render(gutter+row))
		}
	}
	if footer != "" {
		lines = append(lines, m.styles.Subtle.Width(iw).MaxWidth(iw).Render(footer))
	}
	return m.styles.PaneFocus.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// forwardsWindow is the half-open range of forward indices the panel shows when it
// has room for n rows: everything when it fits, otherwise the least-scrolled window
// that still contains sel. It is a pure function of the cursor rather than a stored
// offset, which is what keeps the panel free of scroll state that resize, a stopped
// forward, or clampForwardsSel would each have to maintain — the list is short enough
// that the sticky-offset feel a table needs is not worth that.
func forwardsWindow(total, sel, n int) (int, int) {
	if n <= 0 || total <= 0 {
		return 0, 0
	}
	if n >= total {
		return 0, total
	}
	start := 0
	if sel >= n {
		start = sel - n + 1
	}
	if start > total-n {
		start = total - n
	}
	if start < 0 {
		start = 0
	}
	return start, start + n
}

// forwardsPanelTitle names the panel and, when the window hides rows, which slice of
// the list is on screen. The count rides the title because that is the one row the
// panel is guaranteed to have: a marker row (the modal's answer, D220 pt 3) would
// have to be taken from the list it is describing, and unlike a truncated message
// this list is scrollable — the reader can reach what is hidden, they just need to
// be told it is there. A panel showing everything says nothing, so the counter is
// evidence of elision rather than furniture.
func forwardsPanelTitle(total, start, end int) string {
	switch {
	case total == 0 || end-start >= total:
		return "Port-forwards"
	case end <= start:
		// The box is so short that the title is all of it: no row is on screen to
		// number, so the title carries the bare count rather than an empty range.
		return fmt.Sprintf("Port-forwards (%d)", total)
	default:
		return fmt.Sprintf("Port-forwards (%d–%d of %d)", start+1, end, total)
	}
}

// forwardsPanelFooter builds the panel's key footer from the resolved keymap instead
// of spelling literal keys, so a rebind moves it (D11) — this was the last view in
// kubecom that wrote a key into its own body (HINT-04). Only the *keys* are generated:
// the verbs stay here because the registry's descriptions are global (D218 pt 2) and
// nav.drillIn's is "Open / drill into selection", while in the panel it stops the
// selected forward — this footer is the one place the panel's verbs are stated, which
// is why HINT-03 declined to delete it in favour of the hint line. An action the user
// disabled drops out entirely rather than rendering a bare verb, the same trade
// portPickerTitle makes; disable all three and the footer line itself disappears.
func forwardsPanelFooter(km *keymap.Keymap) string {
	entries := []struct {
		action keymap.Action
		verb   string
	}{
		{keymap.ActionDrillIn, "stop"},
		{keymap.ActionStopForwards, "stop all"},
		{keymap.ActionBack, "close"},
	}
	var parts []string
	for _, e := range entries {
		if k := firstKey(km, e.action); k != "" {
			parts = append(parts, k+": "+e.verb)
		}
	}
	return strings.Join(parts, " · ")
}

// viewerKind* are the kinds stamped on the shared read-only viewer for the content it
// is showing. The M3 viewers all reuse one viewer.Model; each open path restamps the
// kind (viewer.SetKind) so the kind rides ClosedMsg for routing and the shell can gate
// kind-specific behaviour — the secret reveal/copy keys act only while viewerKindSecret
// is up. The shell still hides the viewer uniformly on close regardless of kind. There
// is no logs kind: logs left the shared viewer for their own full-screen view (D144).
const (
	viewerKindDescribe = "describe"
	viewerKindSecret   = "secret"
)

// describeLoadedMsg carries the outcome of the async Describe render issued when the
// describe viewer opens (M3-04). gen ties it to the viewer open that requested it, so
// a render that lands after the user closed the viewer (or opened a newer one — of
// either kind) is dropped rather than populating stale content (the viewerGen guard).
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

// secretLoadedMsg carries the outcome of the async SecretData fetch issued when the
// secret viewer opens (M3-08a). gen ties it to the viewer open that requested it, so
// a fetch that lands after the user closed the viewer (or opened a newer one — of any
// kind) is dropped rather than populating stale content (the viewerGen guard,
// mirroring describeLoadedMsg).
type secretLoadedMsg struct {
	gen  int
	data kube.SecretData
	err  error
}

// openSecretViewer opens the read-only secret viewer over the selected row's object
// (M3-08a, #89): it shows the viewer immediately (empty, so the gesture feels
// instant) and kicks off the SecretData fetch off the update loop, seeding the
// content — masked — when it lands. With no getter wired it is secret-viewer-inert
// (a no-op). The fetch is tagged with a fresh viewerGen so a superseded/stale result
// is dropped (handleSecretLoaded). A fetch error degrades to a status-bar toast and
// closes the viewer (D74) rather than leaving an empty box. Values start hidden: the
// reveal (secret.reveal / `r`) is a deliberate gesture, never automatic.
func (m Model) openSecretViewer(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.secretGetter == nil {
		return m, nil
	}
	m.stopLogStream() // a new viewer supersedes any in-flight log stream.
	m.viewerGen++
	gen := m.viewerGen
	m.secretRevealed = false // every open starts masked (the deliberate-reveal contract).
	m.secretData = kube.SecretData{}
	m.secretSel = 0
	m.secretEntryLines = nil
	m.viewer.SetKind(viewerKindSecret)
	m.viewer.SetTitle(viewerTitle(msg.Resource, msg.Object))
	m.viewer.SetContent("") // clear any prior object's content before the fetch lands.
	m.viewer.Show()
	getter := m.secretGetter
	ref := msg.Object
	return m, func() tea.Msg {
		data, err := getter.SecretData(context.Background(), ref)
		return secretLoadedMsg{gen: gen, data: data, err: err}
	}
}

// handleSecretLoaded seeds the open viewer with the fetched secret, rendered masked.
// A result whose gen no longer matches (a newer open superseded it) or that arrives
// after the viewer closed is dropped. A fetch error degrades: it closes the viewer
// and surfaces a transient status-bar toast (D74), never breaking the layout or
// leaving an empty box.
func (m Model) handleSecretLoaded(msg secretLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen || !m.viewer.Active() {
		return m, nil
	}
	if msg.err != nil {
		m.viewer.Hide()
		return m, m.surfaceError(NewErrorMsg("get secret", msg.err))
	}
	m.secretData = msg.data
	m.secretRevealed = false
	m.secretSel = 0
	m = m.renderSecretViewer()
	return m, nil
}

// secretMask is the fixed-width placeholder shown for a hidden secret value, so the
// value's length is not leaked while it is masked.
const secretMask = "••••••••"

// secretCursor / secretGutter are the 2-cell prefix each entry line carries so the
// selected entry (secretCursor) stands out from the rest (secretGutter). Both are
// the same width so keys stay column-aligned as the cursor moves (M3-08b).
const (
	secretCursor = "> "
	secretGutter = "  "
)

// renderSecret formats a secret's data for the viewer: a type header, then one line
// per key with a cursor gutter marking the selected entry (sel, M3-08b). While masked
// (revealed == false) each value is a fixed mask followed by its byte length, so the
// user sees the keys and can decide what to reveal without the value ever leaking;
// revealed, the decoded value is shown verbatim (a multi-line value is indented under
// its key so the block stays readable). Keys arrive sorted from the kube layer. It
// also returns each entry's 0-based output line (its key line) so the caller can keep
// the selected entry on screen (nil when there are no entries).
func renderSecret(data kube.SecretData, revealed bool, sel int) (string, []int) {
	var b strings.Builder
	typ := data.Type
	if typ == "" {
		typ = "(none)"
	}
	b.WriteString("Type: " + typ + "\n")
	state := "hidden — press r to reveal, c to copy"
	if revealed {
		state = "revealed — press r to hide, c to copy"
	}
	b.WriteString("Data: " + state + "\n\n")
	if len(data.Entries) == 0 {
		b.WriteString("(no data)\n")
		return b.String(), nil
	}
	line := 3 // Type, Data, blank already emitted.
	entryLines := make([]int, len(data.Entries))
	for i, e := range data.Entries {
		entryLines[i] = line
		gutter := secretGutter
		if i == sel {
			gutter = secretCursor
		}
		if !revealed {
			fmt.Fprintf(&b, "%s%s: %s (%d bytes)\n", gutter, e.Key, secretMask, len(e.Value))
			line++
			continue
		}
		if strings.Contains(e.Value, "\n") {
			// A multi-line value (a cert, a kubeconfig) reads best under its key,
			// each line indented so it is visually part of the entry.
			b.WriteString(gutter + e.Key + ":\n")
			line++
			for _, l := range strings.Split(e.Value, "\n") {
				b.WriteString(secretGutter + "  " + l + "\n")
				line++
			}
			continue
		}
		b.WriteString(gutter + e.Key + ": " + e.Value + "\n")
		line++
	}
	return b.String(), entryLines
}

// renderSecretViewer re-renders the open secret viewer from the current
// data/reveal/cursor state and records the entry line offsets. It resets the scroll
// to the top (SetContent), so callers that move the cursor follow it with
// EnsureLineVisible to pull the selection back on screen.
func (m Model) renderSecretViewer() Model {
	content, lines := renderSecret(m.secretData, m.secretRevealed, m.secretSel)
	m.secretEntryLines = lines
	m.viewer.SetContent(content)
	return m
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
// openLogs rather than the namespace/resource/action paths.
const containerPickerKind = "container"

// ctrPurpose disambiguates why the shared container picker (ctrPicker) is open: the
// same resolve-then-pick path resolves a pod's container for either the logs view
// (M3-07a) or an exec session (M3-14b-2), and the pick routes to the matching terminal
// (openLogs vs execInto — see streamOrExec). It is held on the model (ctrPurpose)
// between the picker opening and the pick landing, alongside the ctrStreamRes/Ref stash.
type ctrPurpose int

const (
	ctrPurposeLogs ctrPurpose = iota
	ctrPurposeExec
)

// label names the purpose for a status-bar error context ("logs"/"exec").
func (p ctrPurpose) label() string {
	if p == ctrPurposeExec {
		return "exec"
	}
	return "logs"
}

// pickerTitle is the container picker's title for this purpose (M3-14b-2), so a
// multi-container prompt reads as either a logs or an exec container choice.
func (p ctrPurpose) pickerTitle() string {
	if p == ctrPurposeExec {
		return "Exec container"
	}
	return "Logs container"
}

// offer narrows a pod's full container set (kube.PodContainers returns regular, init
// and ephemeral — LOGS-06) to the ones this purpose can act on, preserving order:
//
//   - Logs offers everything. All three kinds have logs, and an init container's are
//     the *only* diagnosis of a pod stuck in Init:CrashLoopBackOff — the case the
//     feedback named.
//   - Exec drops the init containers. An init container has normally terminated by
//     the time anyone reaches for a shell, so exec'ing into one fails; a debug
//     (ephemeral) container is the opposite — being exec'd into is its whole purpose.
//
// The result aliases cs when nothing is dropped; callers only read it.
func (p ctrPurpose) offer(cs []kube.Container) []kube.Container {
	if p != ctrPurposeExec {
		return cs
	}
	out := make([]kube.Container, 0, len(cs))
	for _, c := range cs {
		if c.Kind != kube.ContainerInit {
			out = append(out, c)
		}
	}
	return out
}

// containerPickerItems renders one picker row per container and the map resolving a
// row back to its container name (the picker's SelectedMsg carries only the label,
// D65 — the ctxByLabel/resByLabel pattern). A regular container is its bare name; an
// init or ephemeral one is qualified — `setup (init)` — because a name alone gives
// the reader no way to tell which is which, and picking the wrong one is a confusing
// empty log rather than an error. Ordering is the lister's (regular containers
// first), so the picker's default highlight stays the pod's main container.
//
// A qualified label can in principle collide with a regular container literally named
// `setup (init)`; the map keeps the first row of a colliding pair, so a pick is always
// resolvable to *some* listed container rather than to nothing.
func containerPickerItems(cs []kube.Container) ([]string, map[string]string) {
	labels := make([]string, 0, len(cs))
	byLabel := make(map[string]string, len(cs))
	for _, c := range cs {
		label := c.Name
		if q := c.Kind.String(); q != "" {
			label = c.Name + " (" + q + ")"
		}
		labels = append(labels, label)
		if _, dup := byLabel[label]; !dup {
			byLabel[label] = c.Name
		}
	}
	return labels, byLabel
}

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
	containers []kube.Container
	err        error
	// purpose routes the resolved container to its terminal (M3-14b-2): the logs
	// viewer or an exec session. Threaded through the async fetch so the result knows
	// which flow requested it.
	purpose ctrPurpose
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
		return m.resolveContainersFor(msg.Resource, msg.Object, ctrPurposeLogs)
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

// resolveContainersFor starts the container-resolution flow over podRef (a pod of res)
// for the given purpose — logs (openLogsViewer's pod path + the M3-07b pod-owning
// resolution) or exec (openExec, M3-14b-2). It cancels any prior stream, then: with a
// container lister wired it fetches the pod's containers off the update loop (a fresh
// viewerGen so a superseded request is dropped — handleContainersLoaded), where a
// single container is used directly and multiple open the picker; with no lister wired
// it uses the pod's default/sole container directly (empty Container) — the M3-05/06
// (logs) / M3-14b-1 (exec) behaviour. The resolved container routes to its terminal via
// streamOrExec.
func (m Model) resolveContainersFor(res kube.Resource, podRef kube.ObjectRef, purpose ctrPurpose) (tea.Model, tea.Cmd) {
	m.stopLogStream()
	if m.containerLister == nil {
		m.viewerGen++
		return m.streamOrExec(res, podRef, "", purpose, m.viewerGen)
	}
	m.viewerGen++
	gen := m.viewerGen
	lister := m.containerLister
	return m, func() tea.Msg {
		names, err := lister.PodContainers(context.Background(), podRef)
		return containersLoadedMsg{gen: gen, res: res, ref: podRef, containers: names, err: err, purpose: purpose}
	}
}

// streamOrExec routes a resolved container to its purpose's terminal (M3-14b-2): a logs
// purpose opens the dedicated logs view (openLogs), an exec purpose suspends into a
// shell (execInto). It is the shared tail of both the single-container fast path and the
// picker selection, so the resolve-then-pick plumbing is identical for logs and exec and
// only the terminal differs. gen guards the logs stream; the exec path ignores it (an
// exec opens no view).
func (m Model) streamOrExec(res kube.Resource, ref kube.ObjectRef, container string, purpose ctrPurpose, gen int) (tea.Model, tea.Cmd) {
	if purpose == ctrPurposeExec {
		return m.execInto(res, ref, container)
	}
	return m.openLogs(res, ref, container, gen)
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
	return m.resolveContainersFor(podLogResource, msg.ref, ctrPurposeLogs)
}

// handleContainersLoaded acts on a resolved container set (M3-07a/M3-14b-2). A result
// whose gen no longer matches (a newer viewer/exec superseded it) is dropped. A fetch
// error, or a pod that reports no containers this purpose can act on, degrades to a
// status-bar toast (D74) without opening the viewer/session. The set is first narrowed
// to what the purpose can act on (ctrPurpose.offer — logs read init containers too,
// exec does not, LOGS-06). A single container is used directly for the requested
// purpose (reusing the fetch's gen so a logs stream is still guarded by the same
// generation); multiple open the container picker, stashing the pod + purpose so the
// pick knows what to do with the chosen container.
func (m Model) handleContainersLoaded(msg containersLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.viewerGen {
		return m, nil // superseded by a newer viewer/stream/exec open; drop.
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg(msg.purpose.label(), msg.err))
	}
	cs := msg.purpose.offer(msg.containers)
	switch len(cs) {
	case 0:
		label := msg.purpose.label() + " for " + msg.ref.Name
		return m, m.surfaceError(ErrorMsg{Context: label + ": no containers"})
	case 1:
		return m.streamOrExec(msg.res, msg.ref, cs[0].Name, msg.purpose, msg.gen)
	default:
		labels, byLabel := containerPickerItems(cs)
		m.ctrStreamRes = msg.res
		m.ctrStreamRef = msg.ref
		m.ctrPurpose = msg.purpose
		m.ctrByLabel = byLabel
		m.ctrPicker.SetTitle(msg.purpose.pickerTitle())
		m.ctrPicker.SetItems(labels)
		return m, m.ctrPicker.Show()
	}
}

// handleContainerSelected acts on the container the user picked from the container
// picker (M3-07a/M3-14b-2): it closes the picker and routes the chosen container to the
// stashed purpose's terminal (streamOrExec — logs stream or exec session) over the
// stashed pod, on a fresh viewerGen (the pick is a new open). The picked value applies
// to the pod + purpose recorded when the picker opened (ctrStreamRes/Ref/ctrPurpose),
// and is a *row label*, resolved back to the container name through ctrByLabel — an
// init or ephemeral container's row is qualified (LOGS-06). A label with no mapping
// (the picker can only list labels it mapped, so this is defensive) closes the picker
// without opening anything.
func (m Model) handleContainerSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.ctrPicker.Hide()
	name, ok := m.ctrByLabel[msg.Value]
	m.ctrByLabel = nil
	if !ok {
		return m, nil
	}
	m.viewerGen++
	return m.streamOrExec(m.ctrStreamRes, m.ctrStreamRef, name, m.ctrPurpose, m.viewerGen)
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
			return m, cmd
		}
		// The palette's line has two editing gestures of its own — space commits a verb
		// into its argument stage, backspace at the start of an argument leaves it
		// (PAL-03a) — so it sees the key before its filter field does. Everything it
		// does not consume types, exactly as in every other picker.
		if p.Kind() == commandPickerKind {
			if next, load, consumed := m.handlePaletteFilterKey(msg); consumed {
				return next, load
			}
		}
		*p, cmd = p.UpdateFilter(msg)
		return m, cmd
	}
	// The port picker carries two gestures of its own (FB-pf-local-port): they act on
	// the *highlighted* port rather than moving the cursor, so the root handles them
	// instead of the shared component — the picker stays generic (D65) and knows
	// nothing about ports. They resolve to registered actions like everything else
	// (D11) and are inert in every other picker, exactly as logs.follow is inert
	// outside the logs viewer. Checked after the filtering branch above, so while the
	// filter is open their keys type into it.
	if mapped && p.Kind() == portPickerKind {
		switch action {
		case keymap.ActionFreeLocalPort:
			return m.forwardOnFreeLocalPort()
		case keymap.ActionLocalPort:
			return m.promptLocalPort()
		}
	}
	if mapped {
		*p, cmd = p.Update(action)
	}
	return m, cmd
}

// routeModalPromptKey resolves one keypress while the modal is open in prompt mode
// (M3-10 scale). It mirrors routeFilterKey/routePickerKey's control/text split (D73):
// a mapped key carrying no text (esc/enter/arrows/ctrl+…) is a control Action the
// modal consumes — nav.drillIn submits the entry, nav.back cancels (handleModalAction)
// — while any text-producing or editing key (a digit, or backspace) is prompt input
// fed to the field via UpdatePrompt. No view matches a raw key for behaviour (D11);
// the open prompt captures all input, so the sequencer and the panes never see it.
// Confirm-mode modals are not prompting, so they still route through the sequencer.
func (m Model) routeModalPromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		return m.handleModalAction(action)
	}
	var cmd tea.Cmd
	m.modal, cmd = m.modal.UpdatePrompt(msg)
	return m, cmd
}

// routeModalConfirmKey resolves one keypress while the confirm modal is open (not
// prompting). The confirm modal fully captures input, so a key resolves in the
// confirm context first (ConfirmAction: `y`/enter → confirm.accept, `n`/esc →
// confirm.decline — the y/n muscle memory the feedback asked for, still registered
// and rebindable, D11/D132); anything the confirm context doesn't bind falls back
// to the browse keymap so app.quit still dismisses the modal, and every other key
// is swallowed so the panes underneath never move. No view matches a raw key (D11).
func (m Model) routeModalConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, ok := m.keymap.ConfirmAction(key); ok {
		return m.handleModalAction(action)
	}
	if action, ok := m.keymap.Action(key); ok {
		return m.handleModalAction(action)
	}
	return m, nil
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

// isEmptyLineBackspace reports whether a key is a backspace pressed against an empty
// query — a delete with nothing before the cursor to delete. It is the third arm of
// D73's control/text split (D238): an editing key with nothing to edit is not input,
// it is the reader asking to unwind past the start of the line, and the only thing
// before the first character of a `/` query is the gesture that opened it. Kept as one
// predicate so every `/` surface tests the same thing, and checked *after* the keymap
// so a config that binds backspace to an action still wins.
func isEmptyLineBackspace(key tea.Key, query string) bool {
	return key.Code == tea.KeyBackspace && key.Text == "" && query == ""
}

// routeFilterKey resolves one keypress while the filter input is open. It mirrors
// routePickerKey's control/text split (D73): a mapped key carrying no text
// (esc/enter/arrows/ctrl+d…) is a control Action the filter mode consumes, while any
// text-producing or editing key (a rune, or an unmapped no-text key like backspace)
// is filter input fed to the field — re-narrowing the table live. No view matches a
// raw key (D11); the open field captures all input, so the sequencer and the panes
// underneath never see it.
//
// The one editing key that is not input is a backspace on an empty query: it resolves
// to nav.back, which cancels the filter and closes the field (D238), so the line
// unwinds past its start instead of dead-ending on an empty prompt.
func (m Model) routeFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		return m.handleFilterAction(action)
	}
	if isEmptyLineBackspace(key, m.filterInput.Value()) {
		return m.handleFilterAction(keymap.ActionBack)
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
	ctx := m.hintContext()
	m.hintCtx = ctx
	m.hintbar.SetHint(m.help.ShortHelpContextView(ctx))
}

// refreshHints re-renders the hint only when the context it derives from has
// changed since the last sync — the tail of every Update (HINT-01/D206). The
// forced syncHints stays the call for a change the context does not capture (a
// resize re-elides the same context to a new width).
func (m *Model) refreshHints() {
	if m.hintContext() != m.hintCtx {
		m.syncHints()
	}
}

// hintContext derives which curated hint subset is truthful right now. It mirrors
// the precedence Update routes keys by, in the same order, because that is exactly
// what makes a hint entry a promise (D143 pt 1): whoever owns input owns the hint.
func (m *Model) hintContext() keymap.HelpContext {
	switch {
	case m.searchView.Active():
		// The search mini-app replaces the browse body and captures every keypress, so
		// the browse hints (filter, sort, actions, namespace…) are all unreachable while
		// it is up — it gets its own context (SEARCH-03b).
		return keymap.HelpSearch
	case m.logsView.Filtering():
		// The logs mini-app's live grep captures text while it is open (LOGS-02), and
		// D143 pt 1 makes the difference matter: `/`, `f` and `q` type instead of firing,
		// so only the no-text keys may be advertised. Resolved before the picker for the
		// same reason Update routes it first.
		return keymap.HelpLogsFilter
	case m.activePicker() != nil:
		// A modal picker captures all input while it is up, so the browse hints under it
		// are unreachable (HINT-01). Which of the two picker contexts applies is the
		// field's state, not the picker's kind: open (every picker since PAL-01) leaves
		// only the no-text keys, closed (WithOptInFilter) also honours `/`.
		if m.activePicker().Filtering() {
			return keymap.HelpPickerFilter
		}
		return keymap.HelpPicker
	case m.filtering:
		// The browse filter field captures text while it is open (routeFilterKey), so
		// the table set underneath — `/`, `n`, `s`, `a`, `?`, `q` — types instead of
		// firing; only the no-text keys act (HINT-03). Resolved after the picker and
		// before the modals, exactly where Update routes it.
		return keymap.HelpTableFilter
	case m.modal.Prompting():
		// The modal's text field takes every text-producing key, so only the no-text
		// control keys act — enter submits, esc cancels (routeModalPromptKey). Resolved
		// before the confirm case for the same reason Update routes it first: a prompting
		// modal is also Active().
		return keymap.HelpPrompt
	case m.modal.Active():
		// The confirm modal captures all input and answers in the confirm key context
		// (D132), so the keys that act here are not browse keys at all (HINT-02).
		return keymap.HelpConfirm
	case m.logsView.Active():
		// The logs mini-app with its grep closed honours every key it advertises, `q`
		// included — quit closes the view, as it does in any pager.
		return keymap.HelpLogs
	case m.viewer.Active():
		// The shared read-only viewer is a pager overlay: it scrolls on navigation and
		// closes on back/quit, swallowing the rest (handleViewerAction). Below the logs
		// view for the same reason handleAction orders them so.
		return keymap.HelpViewer
	case m.forwardsPanel:
		// The port-forward panel swallows everything but its own cursor/stop/close keys
		// (handleForwardsPanelAction), so the browse set underneath is unreachable
		// (HINT-03). Below the viewer and above the overlay, as handleAction orders them.
		return keymap.HelpForwards
	case m.help.Visible():
		// The keybindings overlay swallows navigation while it is open, so the only
		// promise left to make is how to close it. Last of the capturing surfaces, as it
		// is in handleAction — an overlay that owns input wins the hint over it.
		return keymap.HelpKeybindings
	case m.table.Focused():
		return keymap.HelpTable
	}
	return keymap.HelpMenu
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
// any modal picker, or the live filter field). Mouse events are inert while one is
// up so a click cannot reach and mutate the panes underneath it.
func (m Model) overlayActive() bool {
	return m.help.Visible() || m.ctrPicker.Active() || m.cmdPicker.Active() || m.portPicker.Active() || m.viewer.Active() || m.modal.Active() || m.searchView.Active() || m.logsView.Active() || m.forwardsPanel || m.filtering
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
	m.ctrPicker.SetSize(m.width, bodyH)
	m.cmdPicker.SetSize(m.width, bodyH)
	m.portPicker.SetSize(m.width, bodyH)
	// The viewer is the large overlay; it too centers within the body area (above the
	// status bar) so the top status line and bottom hint line stay visible around it.
	m.viewer.SetSize(m.width, bodyH)
	// The confirm modal (M3-09) is a small centered overlay; it too sits within the
	// body area so the top status line and bottom hint line stay visible around it.
	m.modal.SetSize(m.width, bodyH)
	// The search and logs mini-apps are not overlays: each fills the same body area
	// outright (D134), so they take the full body geometry rather than centering in it.
	m.searchView.SetSize(m.width, bodyH)
	m.logsView.SetSize(m.width, bodyH)
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
	// The confirm modal (M3-09) captures input while it is up and takes precedence
	// over every other surface: nav.drillIn accepts, nav.back/app.quit decline, and
	// everything else is swallowed so the browse panes underneath never move (the
	// help/viewer capture pattern). It consumes actions, never raw keys (D11).
	if m.modal.Active() {
		return m.handleModalAction(a)
	}
	// The dedicated logs view (LOGS-02) is a full-screen mini-app: while it is up it
	// captures every action — scrolling, the live grep, the follow toggle, close — and
	// swallows the rest, so the browse panes underneath never move. It opens no overlay
	// and no overlay can open over it, so its precedence relative to the viewer below is
	// only a formality; it is listed here beside the other capturing surfaces.
	if m.logsView.Active() {
		return m.handleLogsAction(a)
	}
	// The read-only viewer (M3-03) captures input while it is up: it scrolls on
	// navigation and closes on nav.back/quit, and swallows everything else so the
	// browse panes underneath never move (mirroring the help modal's capture).
	if m.viewer.Active() {
		return m.handleViewerAction(a)
	}
	// The port-forward panel (M3-13b) is an app-global overlay: while it is up it
	// captures input — nav.up/down move the cursor, nav.drillIn stops the selected
	// forward, forwards.stopAll stops every one, forwards.panel/nav.back/app.quit close
	// it — swallowing the rest so the browse panes underneath never move (the help /
	// viewer capture pattern).
	if m.forwardsPanel {
		return m.handleForwardsPanelAction(a)
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
		// Tear down every goroutine bound to the cluster before the program exits —
		// the watch, discovery, a log stream, a search fan-out, a node drain and every
		// background port-forward. It is the same inventory a context switch tears
		// down (M4-03), single-sourced in stopClusterAsync so the two cannot drift.
		m.stopClusterAsync()
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
		// A children drill-down is a level of its own, above the focus pop: esc out
		// of a child table returns to the owner it was opened from (M4-08), and only
		// a second esc — now on the owner's own table — hands focus back to the menu.
		if next, cmd, exited := m.exitChildren(); exited {
			return next, cmd
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
	case keymap.ActionTheme, keymap.ActionResources, keymap.ActionNamespace,
		keymap.ActionContext, keymap.ActionActions:
		// The shortcut keys converted to pre-typed palette lines (D207): `T` opens the
		// palette on `:theme ` (PAL-05a), `R` on `:resource ` (PAL-05b), `ctrl+n` on
		// `:namespace ` (PAL-05c-1), `C` on `:context ` (PAL-05c-2) and `a` on
		// `:action ` (PAL-05d), none on a modal of its own. One arm rather than one per
		// key, because the conversion is the *same* fact about every one of them — the
		// key names the verb, enterPaletteArg produces the stage, and the values, the
		// inertness and the rendered frame are the typed line's, whether the values were
		// in hand, had to be fetched, or belong to the selected row. With `a` here every
		// key that needs a value opens this one surface (D210).
		return m.openPaletteArg(a)
	case keymap.ActionPalette:
		// `:` opens the palette (PAL-02). Reached from a key only: the palette skips
		// itself when it builds its list, so this can never be a pick recursing.
		return m.openPalette()
	case keymap.ActionForwards:
		return m.openForwardsPanel()
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
	case keymap.ActionSearch:
		return m.openSearch()
	case keymap.ActionPin:
		return m.pinResource()
	case keymap.ActionDescribe, keymap.ActionLogs,
		keymap.ActionEdit, keymap.ActionDelete, keymap.ActionChildren:
		return m.triggerRowActionKey(a)
	}
	return m.routeNav(a)
}

// handleModalAction routes a resolved action to the open confirm modal (M3-09).
// app.quit dismisses it (a decline — quit never deletes, matching how the help modal
// and viewer own the quit key while open); every other action is fed to the modal,
// which accepts on nav.drillIn (ConfirmedMsg) and declines on nav.back (CancelledMsg)
// and swallows the rest. The root hides the modal when either result lands.
func (m Model) handleModalAction(a keymap.Action) (tea.Model, tea.Cmd) {
	if a == keymap.ActionQuit {
		m.modal.Hide()
		return m, nil
	}
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(a)
	return m, cmd
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
		return m, nil
	}
	// secret.reveal (`r`) toggles reveal/mask while the secret viewer is up (M3-08a); it
	// is inert on the other viewers (nothing to reveal). The reveal re-renders the same
	// fetched data (no re-fetch), scroll position preserved — SetContent resets to the
	// top, which is what a reveal/hide toggle wants (the reader re-reads from the top).
	if a == keymap.ActionRevealSecret {
		if m.viewer.Kind() == viewerKindSecret {
			m.secretRevealed = !m.secretRevealed
			m = m.renderSecretViewer()
		}
		return m, nil
	}
	// secret.copy (`c`) copies the selected entry's decoded value to the system
	// clipboard via OSC-52 (M3-08b); it is inert on the other viewers and when the
	// Secret has no data. Copy works masked or revealed — putting the value on the
	// clipboard is itself the deliberate gesture, so it need not be on screen first;
	// only the key + byte length are echoed (a neutral status notice), never the
	// value.
	if a == keymap.ActionCopySecret {
		if m.viewer.Kind() == viewerKindSecret && m.secretSel < len(m.secretData.Entries) {
			e := m.secretData.Entries[m.secretSel]
			notice := m.surfaceNotice(fmt.Sprintf("copied %q (%d bytes)", e.Key, len(e.Value)))
			return m, tea.Batch(tea.SetClipboard(e.Value), notice)
		}
		return m, nil
	}
	// nav.up/nav.down move the entry cursor while the secret viewer is up (M3-08b)
	// rather than line-scrolling: a Secret's body is small, so walking entries is the
	// useful gesture, and EnsureLineVisible keeps the selection on screen for a
	// many-key Secret (half/full-page keys still scroll for a large revealed value).
	if m.viewer.Kind() == viewerKindSecret && (a == keymap.ActionUp || a == keymap.ActionDown) {
		if n := len(m.secretData.Entries); n > 0 {
			if a == keymap.ActionUp && m.secretSel > 0 {
				m.secretSel--
			} else if a == keymap.ActionDown && m.secretSel < n-1 {
				m.secretSel++
			}
			m = m.renderSecretViewer()
			if m.secretSel < len(m.secretEntryLines) {
				m.viewer.EnsureLineVisible(m.secretEntryLines[m.secretSel])
			}
		}
		return m, nil
	}
	// logs.follow and logs.previous are inert here: both belong to the dedicated logs
	// view (LOGS-02/D144, M5-01a), and the shared viewer only ever shows one-shot
	// content (YAML/describe/secret) — there is nothing to follow and no other instance
	// of it to fetch. Swallowed rather than forwarded so they cannot scroll.
	if a == keymap.ActionLogsFollow || a == keymap.ActionLogsPrevious {
		return m, nil
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(a)
	return m, cmd
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
	case m.searchView.Active():
		// The search mini-app is a full-screen view, not an overlay: it *replaces* the
		// browse body while it is up (results span kinds and want every row, D134),
		// keeping only the status bar above and the hint line below. No overlay can be
		// open at the same time — it captures all input and opens none.
		body = m.searchView.View()
	case m.logsView.Active():
		// The logs mini-app is the other full-screen view (LOGS-02): logs want every
		// row for throughput, so it too replaces the browse body rather than centering
		// as an overlay, and it likewise opens no overlay while it is up.
		body = m.logsView.View()
	case m.modal.Active():
		// The confirm modal (M3-09) is the topmost overlay: it opens over the browse
		// view (never over another overlay), so listing it first keeps the switch's
		// single-overlay invariant while giving it precedence.
		body = overlayCenter(body, m.modal.View(), m.width, m.bodyHeight())
	case m.help.Visible():
		body = overlayCenter(body, m.help.View(), m.width, m.bodyHeight())
	case m.ctrPicker.Active():
		body = overlayCenter(body, m.ctrPicker.View(), m.width, m.bodyHeight())
	case m.cmdPicker.Active():
		body = overlayCenter(body, m.cmdPicker.View(), m.width, m.bodyHeight())
	case m.portPicker.Active():
		body = overlayCenter(body, m.portPicker.View(), m.width, m.bodyHeight())
	case m.viewer.Active():
		body = overlayCenter(body, m.viewer.View(), m.width, m.bodyHeight())
	case m.forwardsPanel:
		body = overlayCenter(body, m.forwardsPanelView(), m.width, m.bodyHeight())
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
