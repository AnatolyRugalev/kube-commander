package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
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

// clusterConnectedMsg carries the outcome of a context switch's connect attempt back
// onto the update loop. gen is the ctxGen the attempt was issued under: a switch
// superseded by a later one delivers a message this model must ignore, since
// applying it would land the shell on a cluster the user has already moved off.
type clusterConnectedMsg struct {
	gen     int
	context string
	cluster Cluster
	err     error
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
	gen, conn := m.ctxGen, m.connector
	return m, func() tea.Msg {
		cluster, err := conn.ConnectCluster(name)
		return clusterConnectedMsg{gen: gen, context: name, cluster: cluster, err: err}
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
// The namespace is deliberately left cleared by the reset rather than carried over:
// the old cluster's scope does not describe the new one, and landing the new context
// in its own last-used namespace is M4-05's job (D156's corollary).
func (m Model) handleClusterConnected(msg clusterConnectedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.ctxGen {
		return m, nil // superseded by a later switch; this cluster is not wanted.
	}
	if msg.err != nil {
		e := NewErrorMsg(fmt.Sprintf("switch to context %q", msg.context), msg.err)
		return m, m.surfaceError(e)
	}

	m.resetCluster()
	m.Cluster = msg.cluster
	m.context = msg.context
	m.status.SetContext(msg.context)
	m.welcome.SetContext(msg.context)

	// After the reset, which clears the status bar the old cluster wrote.
	notice := m.surfaceNotice("switched to " + msg.context)
	next, discover := m.startDiscovery()
	return next, tea.Batch(notice, discover)
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
// when the picker opens. It carries no generation: the list describes the
// kubeconfig, not a cluster, so a result that lands late is still correct — the
// only guard needed is that the picker is still open (a dismissed picker drops it).
type contextsLoadedMsg struct {
	contexts []kube.ContextInfo
	err      error
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
	m.ctxPicker.Show()
	lister := m.ctxLister
	return m, func() tea.Msg {
		cs, err := lister.Contexts()
		return contextsLoadedMsg{contexts: cs, err: err}
	}
}

// handleContextsLoaded seeds the open picker with the listed contexts. A listing
// failure (an unreadable or malformed kubeconfig) surfaces a classified error and
// closes the picker, and a kubeconfig that declares no contexts closes it with a
// notice rather than leaving an empty modal the reader can only escape from — both
// degrade, neither crashes (principle 3). A result that arrives after the picker
// was dismissed is dropped.
func (m Model) handleContextsLoaded(msg contextsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.ctxPicker.Hide()
		return m, func() tea.Msg { return NewErrorMsg("list contexts", msg.err) }
	}
	if !m.ctxPicker.Active() {
		return m, nil // dismissed before the list arrived; ignore.
	}
	if len(msg.contexts) == 0 {
		m.ctxPicker.Hide()
		notice := m.surfaceNotice("no contexts in kubeconfig")
		return m, notice
	}
	labels, byLabel := contextPickerItems(msg.contexts, m.context)
	m.ctxByLabel = byLabel
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
	name, ok := m.ctxByLabel[msg.Value]
	m.ctxByLabel = nil
	if !ok {
		return m, nil
	}
	return m.switchContext(name)
}
