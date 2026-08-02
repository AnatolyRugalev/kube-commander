package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// ClusterConnector connects to a kubeconfig context by name and hands back the
// cluster-bound seams for it — the shell's half of a context switch (M4-04a).
//
// It is deliberately *not* a Cluster seam. Every seam on Cluster belongs to one
// cluster and is wrong to keep using after a switch (D155 pt 2); the connector is
// the opposite — it outlives every switch and a Cluster is its product, so it lives
// on the Model beside the context name it changes. The same asymmetry is why the
// launcher, not the tui package, implements it: connecting means kube.Connect plus
// the PortForwarderFunc adapter over a concrete *kube.Clients, and keeping that in
// cmd/kubecom preserves this package's client-free boundary. Routing it through the
// launcher's clusterFor also means a seam can never be wired at launch and forgotten
// on switch — there is one place a client becomes a working shell.
//
// Implementations are called off the update loop (never from Update itself), so
// blocking is acceptable; they must be safe to call again while an earlier call is
// still running, since a second switch does not wait for the first.
type ClusterConnector interface {
	// ConnectCluster builds the seams for the named kubeconfig context. The name is
	// one of kube.Contexts' (M4-01), so it is what ClientConfig.Context accepts. An
	// error leaves the shell on its current cluster untouched — the returned Cluster
	// is then ignored, never swapped in half-built.
	ConnectCluster(name string) (Cluster, error)
}

// WithClusterConnector wires the seam a context switch connects through (M4-04a).
// Without it the model is switch-inert: switchContext is a no-op, which is every
// hermetic test that does not wire one.
func WithClusterConnector(c ClusterConnector) Option {
	return func(m *Model) { m.connector = c }
}

// ContextState is the per-context state a switch must rebind (M4-05): everything
// keyed by the *kubeconfig context* rather than by the cluster's client. It is the
// launch path's WithMenuExtras / WithNamespace / WithNamespacePersister trio,
// re-resolved for the context being switched to.
//
// It is a value with no client in it, so it is safe to compute off the update loop
// and to discard when the connect it rode with failed. Its zero value is the state
// of a context kubecom has never recorded anything for: the built-in default menu,
// all namespaces, no persistence.
type ContextState struct {
	// MenuExtras are the context's `menus/<context>.yaml` additions (D83), folded
	// into the rebuilt seed menu exactly as WithMenuExtras folds them in at launch.
	MenuExtras []config.MenuResource
	// Pinned are the kinds pinned on this context (State.PinnedResources, D193),
	// folded into the rebuilt menu behind MenuExtras exactly as WithPinnedResources
	// folds them in at launch. Separate from MenuExtras because it is the removable
	// half (D202) — and rebound here for the same reason Pinner is: the list `*`
	// unpins from must name the same context as the file it is written back to.
	Pinned []config.MenuResource
	// Namespace is the scope the context was last left in (D90/D91), or "" for all
	// namespaces — including when the context has no recorded state.
	Namespace string
	// Persister writes a namespace picked on the new context back to *its* state
	// file. Nil disables persistence for the switched-in context (an unresolvable
	// state path), exactly as a nil WithNamespacePersister does at launch.
	Persister NamespacePersister
	// Pinner writes a kind pinned on the new context back to *its* state file
	// (CRD-PIN-02). It rides here for the same reason Persister does: both are bound
	// to one context's state path, so a switch that carried the launch context's
	// writer over would record the new context's pins in the departed context's file
	// — the bug D163 exists to prevent. Nil disables pinning for the switched-in
	// context, exactly as a nil WithPinPersister does at launch.
	Pinner PinPersister
}

// ContextStateLoader resolves ContextState for a kubeconfig context (M4-05). It is
// the seam that keeps the tui package context- and storage-agnostic: the launcher
// alone knows where `menus/<context>.yaml` and the per-context state file live, so
// it alone can re-resolve them, exactly as it resolves them once at launch.
//
// Like ClusterConnector it is per-app state rather than a Cluster seam — it reads
// *config*, not the cluster, and outlives every switch. Implementations are called
// off the update loop (they read files) and must be safe to call again while an
// earlier call is still running. They never fail: a missing or malformed file
// degrades to the zero-ish state and is logged by the implementation, since a
// context switch must not be blocked by a config file (principle 3).
//
// Nil → the shell keeps whatever per-context state it launched with across a
// switch, which is the pre-M4-05 behaviour and what every hermetic test that does
// not wire one gets.
type ContextStateLoader interface {
	// LoadContextState returns the per-context state for the named kubeconfig
	// context — the same name ClusterConnector.ConnectCluster is given.
	LoadContextState(name string) ContextState
}

// WithContextStateLoader wires the seam a context switch re-resolves per-context
// state through (M4-05). Without it a switch carries the launch context's menu
// extras and namespace persister into the new context.
func WithContextStateLoader(l ContextStateLoader) Option {
	return func(m *Model) { m.ctxState = l }
}

// clusterConnectedMsg carries the outcome of a context switch's connect attempt back
// onto the update loop. gen is the ctxGen the attempt was issued under: a switch
// superseded by a later one delivers a message this model must ignore, since
// applying it would land the shell on a cluster the user has already moved off.
//
// state is the new context's per-context state (M4-05), resolved in the same
// off-loop Cmd as the connect: both are keyed by the context name, both are disk
// reads, and both are wanted only if the connect succeeded — so one goroutine and
// one message carry them, and a failed switch discards them together.
type clusterConnectedMsg struct {
	gen     int
	context string
	cluster Cluster
	state   ContextState
	err     error
	// connect is how long the off-loop half took — ConnectCluster plus the state
	// load, i.e. everything a *retained* cluster would let a switch-back skip
	// (CTX-WARM-01). Measured here rather than in the handler because the handler
	// runs whenever the message is delivered, which is not when the work happened.
	connect time.Duration
}

// switchTiming is the stopwatch a context switch carries from its connect through to
// the moment the new cluster's menu is reconciled. It exists to answer the question
// the "keep the previous cluster warm" feedback turns on and nobody has measured:
// *what* is slow about switching back — the connect, or the discovery pass (D196 pt 3).
//
// It is per-switch state on the Model rather than a counter somewhere because there is
// at most one switch in flight and a superseded one must not report: each successful
// connect overwrites it, the discovery pass that switch started consumes it, and a
// launch-time pass (no switch) leaves it zero and logs nothing.
type switchTiming struct {
	context string
	connect time.Duration
	// applied is when the swap landed on the update loop, so time.Since(applied) at
	// reconcile is the discovery half — the wall-clock the reader actually waits
	// between the picker closing and the new cluster's menu being complete.
	applied time.Time
}

// switchContext moves the shell to another kubeconfig context: it connects to it
// **off the update loop** and does nothing else. The teardown-and-swap only happens
// once the new client is in hand (handleClusterConnected), which is the ordering the
// whole slice turns on — a connect failure must leave the shell exactly as it was,
// still browsing the cluster it is on, rather than on a reset shell with no cluster
// (D155 pt 1: the reset is destructive, so it may not run speculatively).
//
// Switching to the context already live is a no-op: there is nothing to gain from
// tearing down a working cluster to rebuild the same one, and a picker whose current
// entry silently reloads everything is a surprising gesture (M4-04b's picker marks
// that entry rather than removing it, so it can be chosen). With no connector wired
// the model is switch-inert.
func (m Model) switchContext(name string) (tea.Model, tea.Cmd) {
	if m.connector == nil || name == "" || name == m.context {
		return m, nil
	}
	m.ctxGen++
	gen, conn, loader := m.ctxGen, m.connector, m.ctxState
	return m, func() tea.Msg {
		start := time.Now()
		cluster, err := conn.ConnectCluster(name)
		msg := clusterConnectedMsg{gen: gen, context: name, cluster: cluster, err: err}
		// The per-context state is resolved in the same Cmd (M4-05): it is a disk
		// read keyed by the same name, so it belongs off the update loop beside the
		// connect. Skipped when the connect failed — nothing will be applied.
		if err == nil && loader != nil {
			msg.state = loader.LoadContextState(name)
		}
		msg.connect = time.Since(start)
		return msg
	}
}

// handleClusterConnected completes a context switch with the new cluster's seams in
// hand: tear the old cluster down (resetCluster — every per-cluster async cancelled
// and generation-bumped, every surface showing its data dismissed, the browse panes
// back to their pre-drill-in state, M4-03/D156), repoint the one bundle at the new
// cluster (M4-02), rename the context on the model and the two places that show it,
// and start discovery so the new cluster's API surface reconciles into the seed menu.
//
// A connect failure degrades to a transient toast and changes nothing (principle 3):
// no reset has run at this point, so the shell is still browsing the cluster it was
// on, watches and all. A result from a superseded attempt is dropped on the same
// rule every other per-cluster async follows (D156 pt 2) — cancelling is not
// available here (a connect is a single call, not a stream), so the generation check
// is the whole guard.
//
// The old cluster's namespace scope is never carried over — it does not describe the
// new cluster (D156's corollary). Since M4-05 the new context's *own* last-used
// namespace is restored over the cleared scope instead, along with its menu extras
// and the persister that writes its state file, so a switch lands where that context
// was left rather than at a blank slate (D163).
func (m Model) handleClusterConnected(msg clusterConnectedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.ctxGen {
		return m, nil // superseded by a later switch; this cluster is not wanted.
	}
	if msg.err != nil {
		e := NewErrorMsg(fmt.Sprintf("switch to context %q", msg.context), msg.err)
		return m, m.surfaceError(e)
	}

	// The new context's menu extras are installed *before* the reset, which rebuilds
	// the menu from the seed and folds in whatever extras the model holds — so the
	// rebuilt menu is the new context's, not the departing one's (M4-05). With no
	// loader wired the launch extras stay, which is the pre-M4-05 behaviour.
	if m.ctxState != nil {
		m.menuExtras = msg.state.MenuExtras
		m.menuPinned = msg.state.Pinned // and the new context's pins, folded in behind them
	}
	m.resetCluster()
	m.Cluster = msg.cluster
	m.context = msg.context
	m.status.SetContext(msg.context)
	m.welcome.SetContext(msg.context)
	// The rest of the per-context state, after the reset cleared the old context's:
	// the persister is rebound to the new context's state file *before* the scope is
	// restored, so a namespace picked next is written to the right file, and the
	// restored scope is not written back (it came from that file — persisting it
	// would be a no-op write on every switch).
	if m.ctxState != nil {
		m.nsPersister = msg.state.Persister
		m.pinner = msg.state.Pinner // pins recorded next belong to the new context's file
		m.setNamespace(msg.state.Namespace)
	}

	// Start the stopwatch's second half now the swap has landed; the discovery pass
	// started below closes it (logSwitchComplete). Overwriting rather than appending
	// is deliberate: a switch superseded before its menu arrived is not worth a
	// timing, and the surviving switch is the one the reader is waiting on.
	m.ctxSwitch = switchTiming{context: msg.context, connect: msg.connect, applied: time.Now()}

	// After the reset, which clears the status bar the old cluster wrote.
	notice := m.surfaceNotice("switched to " + msg.context)
	next, discover := m.startDiscovery()
	return next, tea.Batch(notice, discover)
}

// logSwitchComplete closes a context switch's stopwatch when the discovery pass that
// switch started has reconciled, and writes the three numbers to the diagnostic log
// (D159's file, `~/.cache/kubecom/kubecom.log`): the connect, the discovery, and their
// sum — the wall-clock between picking a context and its menu being complete.
//
// It logs rather than renders on purpose (D196 pt 3): this is a measurement taken to
// decide whether retaining a departed cluster is worth its risk, not a number a user
// asked to see. A launch-time discovery pass has no timing pending and logs nothing,
// so the log stays quiet until someone actually switches.
//
// The watch re-arm is deliberately *outside* the total: it starts only when the reader
// drills into a resource, so folding it in would measure their reading speed. The menu
// being complete is the moment the new cluster is usable, and that is what is timed.
func (m Model) logSwitchComplete() Model {
	if m.ctxSwitch.context == "" {
		return m
	}
	t := m.ctxSwitch
	m.ctxSwitch = switchTiming{}
	discovery := time.Since(t.applied)
	m.logger.Info("context switch complete",
		"context", t.context,
		"connect", t.connect.Round(time.Microsecond),
		"discovery", discovery.Round(time.Microsecond),
		"total", (t.connect + discovery).Round(time.Microsecond),
	)
	return m
}

// ContextLister lists the kubeconfig contexts the switcher offers (M4-04b). Like
// ClusterConnector it is per-app state rather than a Cluster seam, and for a
// stronger reason: it reads *kubeconfig* data, not the cluster — the same list is
// correct before, during and after a switch, so tying it to a Cluster would rebuild
// it for no reason and make the picker unavailable exactly when a connect failed.
// The launcher implements it bound to the --kubeconfig path kubecom launched with,
// mirroring contextConnector, so both halves of a switch resolve against one file.
//
// Implementations are called off the update loop, so reading the kubeconfig from
// disk here is fine. Nil → the model is context-picker-inert: ctx.switch never opens
// a picker, which is every hermetic test that does not wire one.
type ContextLister interface {
	// Contexts returns every declared context, sorted (kube.Contexts, M4-01). An
	// error degrades to a toast; it never takes the app down (principle 3).
	Contexts() ([]kube.ContextInfo, error)
}

// WithContextLister wires the seam the context picker is seeded from (M4-04b).
// Without it the ctx.switch action is inert (the picker never opens).
func WithContextLister(l ContextLister) Option {
	return func(m *Model) { m.ctxLister = l }
}

// contextPickerKind is the Kind stamped on the context picker
// (picker.New(s, "context")). Every picker emits the same SelectedMsg/CancelledMsg
// types (D65), so the root branches on this Kind to route a picked context into
// switchContext rather than the namespace/resource/action/container/port paths.
const contextPickerKind = "context"

// contextsLoadedMsg carries the outcome of the kubeconfig context listing issued
// when a surface that needs contexts opens. It carries no generation: the list
// describes the kubeconfig, not a cluster, so a result that lands late is still
// correct — the only guard needed is that the surface is still waiting for it.
//
// dest names which surface asked (PAL-03b): the standalone picker
// (contextPickerKind) or the command palette's `:context ` stage
// (commandPickerKind). See namespacesLoadedMsg for why the routing is addressed
// rather than inferred from whichever surface is open.
type contextsLoadedMsg struct {
	contexts []kube.ContextInfo
	err      error
	dest     string
}

// loadContexts is the off-loop listing itself, addressed to the surface that asked.
// Both callers — the standalone picker and the palette's argument stage — go through
// it, so neither can drift into listing contexts its own way.
func (m Model) loadContexts(dest string) tea.Cmd {
	lister := m.ctxLister
	if lister == nil {
		return nil
	}
	return func() tea.Msg {
		cs, err := lister.Contexts()
		return contextsLoadedMsg{contexts: cs, err: err, dest: dest}
	}
}

// openContextPicker shows the context picker and kicks off the listing that seeds
// it. With no lister wired the model is context-switch-inert and this is a no-op.
// As with the namespace picker the modal is shown immediately (empty, then
// populated when the list lands) so the gesture feels instant, and a stale item set
// from a previous open is cleared first — a context could have been added to the
// kubeconfig since, and the marked entry is the one kubecom is on *now*.
func (m Model) openContextPicker() (tea.Model, tea.Cmd) {
	if m.ctxLister == nil {
		return m, nil
	}
	m.ctxPicker.SetItems(nil)
	m.ctxByLabel = nil
	show := m.ctxPicker.Show()
	return m, tea.Batch(show, m.loadContexts(contextPickerKind))
}

// handleContextsLoaded seeds whichever surface asked for the listing — the standalone
// picker or the palette's `:context ` stage (PAL-03b) — with the listed contexts. A
// listing failure (an unreadable or malformed kubeconfig) surfaces a classified error
// and dismisses that surface, and a kubeconfig that declares no contexts dismisses it
// with a notice rather than leaving an empty modal the reader can only escape from —
// both degrade, neither crashes (principle 3). A result that arrives after the surface
// moved on is dropped.
func (m Model) handleContextsLoaded(msg contextsLoadedMsg) (tea.Model, tea.Cmd) {
	palette := msg.dest == commandPickerKind
	waiting := m.ctxPicker.Active()
	if palette {
		waiting = m.awaitingPaletteArg(keymap.ActionContext)
	}
	dismiss := func() {
		if !waiting {
			return
		}
		if palette {
			m.closePalette()
			return
		}
		m.ctxPicker.Hide()
	}
	if msg.err != nil {
		dismiss()
		return m, func() tea.Msg { return NewErrorMsg("list contexts", msg.err) }
	}
	if !waiting {
		return m, nil // dismissed before the listing arrived; ignore.
	}
	if len(msg.contexts) == 0 {
		dismiss()
		notice := m.surfaceNotice("no contexts in kubeconfig")
		return m, notice
	}
	labels, byLabel := contextPickerItems(msg.contexts, m.context)
	m.ctxByLabel = byLabel
	if palette {
		return m.fillPaletteArg(keymap.ActionContext, labels), nil
	}
	m.ctxPicker.SetItems(labels)
	return m, nil
}

// contextPickerItems renders one picker row per context and the map resolving a row
// back to its context name (the picker's SelectedMsg carries only the label, D65 —
// the resByLabel/actByLabel pattern). Rows are `* name (cluster)`, the marker on the
// context the shell is **currently on**.
//
// That marker comes from the shell's own live context, not ContextInfo.Current
// (D158): kubecom's switch is session-scoped and never writes `current-context`
// back to the kubeconfig, so after one switch the kubeconfig's idea of "current" is
// the context the reader left. The cluster name is shown only when it differs from
// the context name — for the common one-cluster-per-context kubeconfig it would
// otherwise repeat every row and cost the width the names need.
func contextPickerItems(cs []kube.ContextInfo, current string) ([]string, map[string]string) {
	labels := make([]string, 0, len(cs))
	byLabel := make(map[string]string, len(cs))
	for _, c := range cs {
		label := "  " + c.Name
		if c.Name == current {
			label = "* " + c.Name
		}
		if c.Cluster != "" && c.Cluster != c.Name {
			label += " (" + c.Cluster + ")"
		}
		if _, dup := byLabel[label]; dup {
			continue // kubeconfig context names are unique, so this is defensive.
		}
		byLabel[label] = c.Name
		labels = append(labels, label)
	}
	return labels, byLabel
}

// handleContextSelected applies a context picked from the switcher: it closes the
// picker and hands the resolved name to switchContext, which connects off the update
// loop and only then tears the old cluster down (M4-04a/D157). Picking the context
// already live is a no-op there — the marked row is choosable rather than hidden, so
// it must cost nothing. A label with no mapping (the picker can only list labels it
// mapped, so this is defensive) closes the picker without switching.
func (m Model) handleContextSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.ctxPicker.Hide()
	return m.applyContextLabel(msg.Value)
}

// applyContextLabel resolves a context picker row back to its context name and hands
// it to switchContext. It is the apply both surfaces end in — the standalone picker
// and the palette's `:context ` stage — so an argument reached through the palette
// and one reached through `C` are one code path (D198 pt 2).
func (m Model) applyContextLabel(label string) (tea.Model, tea.Cmd) {
	name, ok := m.ctxByLabel[label]
	m.ctxByLabel = nil
	if !ok {
		return m, nil
	}
	return m.switchContext(name)
}
