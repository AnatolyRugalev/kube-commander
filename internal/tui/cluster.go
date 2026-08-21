package tui

// Cluster is every seam the shell binds to *one* cluster's client, in one value.
//
// Before M4-02 the launcher passed 21 separate With* options, each closing over the
// same *kube.Clients fixed at construction, and the Model held them as 21 independent
// fields — so "switch to another context" had no single thing to repoint (D155 pt 2).
// They now live here and the Model embeds one Cluster, which means:
//
//   - the context switch (M4-04) swaps one value — m.Cluster = <new bundle> — instead
//     of rebuilding 21 closures, and
//   - **every seam added from here on goes on Cluster, not on Model**, if it is bound
//     to the cluster. The test is simple: would it be wrong to keep using it after a
//     switch? Then it belongs here. Per-context *state* (the namespace persister, the
//     menu extras, the context name) is not a cluster client and stays on the Model —
//     M4-05 rebinds those on switch.
//
// The fields are unexported and promoted, so the shell keeps reading m.watcher,
// m.deleter, … unchanged: the indirection is structural, not a rename. Each field's
// contract is documented on the interface it holds (ResourceWatcher, Deleter, …); the
// one-liners here say only what the seam is for and what a nil one means.
//
// Cluster is a value, deliberately, not a pointer: bubbletea copies the Model on every
// Update, and a pointer would let a write through one copy be seen by every other —
// exactly the shared mutable state principle 1 forbids. A value copies with its Model
// and a swap rebinds only the copy the update loop returns. The zero Cluster is valid
// and wholly inert (every action that needs a client degrades to a no-op or a toast),
// which is what New() and the hermetic tests that wire a single fake seam rely on.
type Cluster struct {
	// watcher starts the live table watch; discoverer runs the async discovery pass
	// that reconciles the resource menu. Nil → watch-inert / discovery-inert.
	watcher    ResourceWatcher
	discoverer Discoverer

	// nsLister seeds the palette's `:namespace ` stage (nil → ns.switch inert). The namespace the
	// user picks is *state*, not a client, and stays on the Model — as does the
	// persister that writes it, which is bound to the context's state file rather
	// than to the cluster (M4-05).
	nsLister NamespaceLister

	// The read-only viewer sources: an object's YAML (the edit flow's buffer, D135),
	// its describe output, its events, a pod's log stream, a pod's container names,
	// and a Secret's decoded data. Nil → the action that needs it is inert.
	yamlGetter      YAMLGetter
	describer       Describer
	eventLister     EventLister
	logStreamer     LogStreamer
	containerLister ContainerLister
	secretGetter    SecretGetter

	// podResolver resolves a pod-owning workload to a backing pod so its logs can be
	// streamed (M3-07b); serviceResolver resolves a Service to a backing endpoint pod
	// so it can be port-forwarded (M3-13c). Nil → that hop degrades to a toast.
	podResolver     PodResolver
	serviceResolver ServiceResolver

	// The curated mutating action set: delete, scale, rollout-restart,
	// cordon/uncordon, suspend/resume, drain, and the $EDITOR apply. Nil → the action
	// is inert (no modal, no prompt, no dispatch).
	deleter   Deleter
	scaler    Scaler
	restarter RolloutRestarter
	cordoner  Cordoner
	suspender Suspender
	drainer   Drainer
	editor    Editor

	// portLister lists a forward target's declared ports for the picker and
	// portForwarder starts the background forward itself (nil → the prompt opens
	// directly / the action is inert). The forwards a switch must tear down are
	// Model state, not seams — they outlive nothing but their own cluster (M4-03).
	portLister    PortLister
	portForwarder PortForwarder

	// searcher runs the cluster-search fan-out; execer opens an interactive shell in a
	// pod's container. Nil → search-inert / exec-inert.
	searcher Searcher
	execer   Execer

	// scanner runs the cross-kind unhealthy sweep (STORY-06g-2): a one-shot,
	// cancellable kube.Scan over the menu's kinds keeping the rows the M4-06
	// classifier reads as unhealthy. Nil → the unhealthy-scan action is inert,
	// exactly as a nil searcher makes cluster search inert.
	scanner Scanner

	// relater resolves a selected object into its navigable neighbours for the
	// relations popup (STORY-06i-2): owners, the child scope, and the links the
	// object's own spec names. Nil → the relations gesture is inert, exactly as a
	// nil childResolver makes the drill-down inert.
	relater Relater

	// childResolver turns a selected owner row into the scope its pods are listed
	// under (M4-08). Nil → the children drill-down is inert.
	childResolver ChildResolver

	// metricsLister polls metrics.k8s.io for the CPU/memory samples the browse
	// table overlays on a measured kind (M4-10). Nil → no metrics columns, silently.
	metricsLister MetricsLister
}

// ClusterClient is one cluster's client as the shell sees it: the union of every
// cluster-bound seam a single client value can satisfy directly. *kube.Clients
// satisfies it, which is the point — the launcher hands NewCluster one client instead
// of naming the same value 21 times, and a future seam added to this interface is a
// compile error at the launcher until the client actually implements it.
//
// PortForwarder is the one exception and is passed separately: kube.Clients.PortForward
// returns the concrete *kube.PortForward, not the ActiveForward the shell observes, so
// it cannot satisfy the interface method without the PortForwarderFunc adapter.
type ClusterClient interface {
	ResourceWatcher
	Discoverer
	NamespaceLister
	YAMLGetter
	Describer
	EventLister
	LogStreamer
	ContainerLister
	SecretGetter
	PodResolver
	ServiceResolver
	Deleter
	Scaler
	RolloutRestarter
	Cordoner
	Suspender
	Drainer
	Editor
	PortLister
	Searcher
	Execer
	ChildResolver
	MetricsLister
	Scanner
	Relater
}

// NewCluster bundles one cluster's client into the seams the shell drives. It is the
// single constructor the launcher (and, from M4-04, a context switch) builds a Cluster
// with, so there is exactly one place a newly connected client is turned into a
// working shell. pf may be nil, leaving the port-forward action inert.
func NewCluster(c ClusterClient, pf PortForwarder) Cluster {
	return Cluster{
		watcher:         c,
		discoverer:      c,
		nsLister:        c,
		yamlGetter:      c,
		describer:       c,
		eventLister:     c,
		logStreamer:     c,
		containerLister: c,
		secretGetter:    c,
		podResolver:     c,
		serviceResolver: c,
		deleter:         c,
		scaler:          c,
		restarter:       c,
		cordoner:        c,
		suspender:       c,
		drainer:         c,
		editor:          c,
		portLister:      c,
		portForwarder:   pf,
		searcher:        c,
		execer:          c,
		childResolver:   c,
		relater:         c,
		metricsLister:   c,
		scanner:         c,
	}
}

// WithCluster wires a whole cluster bundle at construction — the launcher's one call
// in place of the 21 individual With* options, which remain for hermetic tests that
// wire a single fake seam. Applied like any other option, so a later individual
// With* still overrides one field of the bundle (order wins, as it always did).
func WithCluster(c Cluster) Option {
	return func(m *Model) { m.Cluster = c }
}
