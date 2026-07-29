package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
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
