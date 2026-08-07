package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// This file is the M4-10 metrics overlay: the CPU/MEMORY columns the browse table
// grows when the cluster measures the kind being browsed (Pods, Nodes), and the
// slow poll that keeps them current.
//
// Metrics are not watchable — metrics.k8s.io serves point-in-time samples and
// offers no watch verb — so this is the one data source here that is polled rather
// than streamed (D155 pt 3). It is deliberately *not* a second table: the samples
// are joined onto the rows the ordinary watch already delivers, by namespace/name
// (kube.UsageKeyOf), and live in the table component beside the rows rather than
// inside them (table/usage.go). Rows and samples therefore refresh on their own
// clocks and neither clobbers the other.
//
// Availability is answered by kube.MetricsFor out of the discovered resource set
// already in hand — no probe request. metrics-server absent and metrics-server
// installed-but-down are the same answer (a group that fails discovery is isolated
// out of the available set, #87), and that answer is silence: no columns, no
// message, nothing to dismiss (D167 pt 1).

// MetricsLister is the narrow slice of the kube layer the overlay needs: list the
// usage samples served by a metrics resource, keyed for a join onto watched rows.
// *kube.Clients satisfies it. As with every other seam the shell depends on the
// interface, so the poll is driveable in hermetic tests; a model built without one
// is metrics-inert and the columns never appear.
//
// Only the listing goes through the seam. Availability is a pure function of the
// discovery result (kube.MetricsFor) and is called directly, exactly as the
// children drill-down calls kube.HasChildren.
type MetricsLister interface {
	Metrics(ctx context.Context, m kube.Resource, namespace string) (map[kube.UsageKey]kube.Usage, error)
}

// WithMetricsLister wires the client the metrics overlay polls (M4-10). Without it
// the browse table never grows metrics columns.
func WithMetricsLister(l MetricsLister) Option {
	return func(m *Model) { m.metricsLister = l }
}

const (
	// metricsInterval is the poll period. metrics-server's own default scrape
	// interval is 15s, so anything much faster re-fetches a sample it already has;
	// 10s keeps the columns feeling live without asking the flakiest API in the
	// cluster a question every second.
	metricsInterval = 10 * time.Second

	// metricsTimeout bounds one refresh. The chain is serial — the next tick is
	// scheduled when a result lands — so a request that hung forever would stop the
	// overlay updating without ever failing. An aggregated API is exactly where that
	// happens.
	metricsTimeout = 5 * time.Second
)

// metricsMsg carries one completed refresh back onto the update loop; metricsTickMsg
// is the poll's heartbeat. Both are tagged with the generation of the poll that
// issued them, so a sample set (or a tick) belonging to a kind, namespace, scope or
// cluster the reader has left is dropped rather than painted onto the table now
// showing something else.
type metricsMsg struct {
	gen   int
	usage map[kube.UsageKey]kube.Usage
	err   error
}

type metricsTickMsg struct{ gen int }

// startMetrics (re)starts the overlay for the resource the browse watch was just
// pointed at, scoped to the same namespace that watch uses — which for a children
// drill-down is the scope's namespace, not the app's (M4-08), so a Node's pods do
// not poll one namespace's samples.
//
// It is called from watchResource, the single place a browse watch starts, so every
// restart — a kind change, a namespace re-scope, a drill-down, a reconnect's
// re-list — re-evaluates availability and re-scopes the poll. When the kind is not
// measured (or nothing is wired) the overlay is simply off: no columns, no request,
// nothing said.
//
// It mutates the receiver, so callers pass the addressable model value they are
// about to return.
func (m *Model) startMetrics(r kube.Resource, namespace string) tea.Cmd {
	m.stopMetrics()
	m.table.SetUsage(nil)
	if m.metricsLister == nil {
		return nil
	}
	res, ok := kube.MetricsFor(r, m.availableResources())
	if !ok {
		return nil
	}
	m.metricsRes, m.metricsNS = res, namespace
	return m.refreshMetrics()
}

// stopMetrics tears the overlay down: the in-flight request is cancelled and its
// result (and any pending tick) made stale by the generation bump. Safe to call
// with no poll running. It mutates the receiver.
func (m *Model) stopMetrics() {
	if m.metricsCancel != nil {
		m.metricsCancel()
		m.metricsCancel = nil
	}
	m.metricsRes = kube.Resource{}
	m.metricsNS = ""
	m.metricsGen++
}

// refreshMetrics issues one list off the update loop. It bumps the generation
// first, so a result from a refresh this one supersedes is dropped on arrival.
// It mutates the receiver.
func (m *Model) refreshMetrics() tea.Cmd {
	if m.metricsLister == nil || m.metricsRes.GVR.Empty() {
		return nil
	}
	m.metricsGen++
	gen := m.metricsGen
	ctx, cancel := context.WithTimeout(context.Background(), metricsTimeout)
	m.metricsCancel = cancel
	lister, res, ns := m.metricsLister, m.metricsRes, m.metricsNS
	return func() tea.Msg {
		usage, err := lister.Metrics(ctx, res, ns)
		return metricsMsg{gen: gen, usage: usage, err: err}
	}
}

// scheduleMetrics arms the next poll. It returns nil when the overlay is off, which
// is what ends the chain after a kind change or a teardown.
func (m Model) scheduleMetrics() tea.Cmd {
	if m.metricsRes.GVR.Empty() {
		return nil
	}
	gen := m.metricsGen
	return tea.Tick(metricsInterval, func(time.Time) tea.Msg {
		return metricsTickMsg{gen: gen}
	})
}

// handleMetricsTick starts the next refresh, unless the poll it belongs to has been
// superseded — in which case the chain simply ends here (a newer poll has its own).
func (m Model) handleMetricsTick(msg metricsTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.metricsGen {
		return m, nil
	}
	return m, m.refreshMetrics()
}

// handleMetricsMsg installs a completed refresh and arms the next one.
//
// A failed refresh keeps the samples the table already has rather than blanking the
// columns: one 503 from an aggregated API is not evidence the metrics went away,
// and columns that blink empty every time metrics-server restarts are worse than
// slightly stale numbers. It is logged (D159) and never toasted — the overlay is a
// decoration, and the reader did not ask for it.
func (m Model) handleMetricsMsg(msg metricsMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.metricsGen {
		return m, nil // a refresh for a kind/namespace/cluster the reader has left.
	}
	if m.metricsCancel != nil {
		m.metricsCancel()
		m.metricsCancel = nil
	}
	if msg.err != nil {
		m.logger.Warn("metrics refresh failed",
			"resource", m.metricsRes.GVR.Resource, "namespace", m.metricsNS, "error", msg.err)
		return m, m.scheduleMetrics()
	}
	m.table.SetUsage(msg.usage)
	return m, m.scheduleMetrics()
}
