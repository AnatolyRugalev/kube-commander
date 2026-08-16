package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/table"
	"github.com/neuroplastio/kubecom/internal/tui/components/unhealthyview"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// This file is the app wiring of the cross-kind unhealthy list (STORY-06g-2b):
// the app.unhealthyScan action, the Scanner seam over kube.Scan (D276), the
// generation-guarded hit pump that feeds the STORY-06g-2b-1 view, and the
// drill-in that switches the browse view to a hit. The view itself owns the hits
// and the cursor and never touches a client; everything concurrent lives here,
// behind messages (principle 1), exactly as the search wiring does (D131/D140).

// scanHitLimit caps how many hits one sweep collects. kube.Scan cancels the
// still-running lists once the cap is reached (D276), so this is both a result
// bound and a load bound on a big cluster. It is the same cap the search fan-out
// runs under (searchHitLimit): "what's broken" is meant to be triaged, not read
// cover to cover, and the view's cap state says when the answer is truncated.
const scanHitLimit = 200

// Scanner runs a one-shot, cancellable cross-kind sweep over the given kinds and
// streams the hits — and its own progress (ScanKindDone) — back (kube.Clients
// implements it via Scan). It is the narrow seam the unhealthy list needs,
// injected with WithScanner — nil leaves the model scan-inert (the
// app.unhealthyScan action never opens the view), exactly as a nil searcher
// leaves cluster search inert.
type Scanner interface {
	Scan(ctx context.Context, resources []kube.Resource, namespace string, keep kube.RowFilter, limit int) <-chan kube.ScanEvent
}

// WithScanner wires the cross-kind scan client (nil → scan-inert).
func WithScanner(s Scanner) Option {
	return func(m *Model) { m.scanner = s }
}

// scanMsg wraps one message from the scan pump with the generation of the sweep
// it belongs to. Every close bumps scanGen, so a hit from a superseded sweep —
// whose channel is still draining after cancellation — is dropped rather than
// appended to a view it does not describe, and its pump chain stops instead of
// racing a second reader onto the live channel. The same stale-message guard
// watchGen gives the table watch and searchGen gives the search fan-out.
type scanMsg struct {
	gen int
	msg tea.Msg
}

// openUnhealthy shows the cross-kind unhealthy list (app.unhealthyScan). It is a
// no-op without a scanner wired (scan-inert) — a list that can never fill is
// worse than an unbound key. The view opens clean (Reset drops any previous
// sweep and its hits) with the namespace it will sweep named in its header, and
// captures every keypress until it closes: while it is up the root routes keys to
// it as actions (handleUnhealthyAction). The sweep launches on open, not per
// keystroke — there is no query to debounce (D278).
func (m Model) openUnhealthy() (tea.Model, tea.Cmd) {
	if m.scanner == nil {
		return m, nil
	}
	m.unhealthyView.Reset()
	m.unhealthyView.SetScope(m.scopeLabel())
	m.unhealthyView.Show()
	m.syncHints() // the unhealthy view owns input now → unhealthy-context hints
	m, cmd := m.launchScan()
	return m, cmd
}

// scopeLabel is the human label for what the sweep covers by default: the
// watched namespace, or the all-namespaces sentinel when the app is unscoped.
// Purely the header's wording — the sweep always covers the app's own namespace
// (unhealthyScope), the same scope the browse table watches.
func (m Model) scopeLabel() string {
	if m.namespace == "" {
		return namespaceAllItem
	}
	return m.namespace
}

// unhealthyScope is the namespace one sweep lists over: the app's own namespace,
// or every namespace when the app is unscoped (kube.Scan takes "" for all). The
// sweep is a one-shot read of the same scope the browse table watches — "what's
// broken, here" across kinds — not a cluster-wide audit, so it never widens on
// its own.
func (m Model) unhealthyScope() string {
	return m.namespace
}

// scanResources is the kind set one sweep fans out over, taken from the kinds the
// menu currently offers — the same source the cluster search and the resource
// palette draw on, so discovered kinds and per-context extras are included and an
// unavailable kind is skipped. Unlike search there is no curated narrow set: the
// whole point of the cross-kind sweep is that the broken thing may be a kind the
// operator is not looking at (a PVC, a claim), so every offered kind is swept.
func (m Model) scanResources() []kube.Resource {
	return m.availableResources()
}

// handleUnhealthyAction routes a resolved action to the open unhealthy list.
// app.quit closes the view rather than exiting kubecom — a full-screen pager owns
// the quit key while it is up, exactly as the logs view and the search view do.
// Everything else is handed to the component, which moves the cursor, emits
// SelectedMsg on drill-in, and closes itself on nav.back via its own ClosedMsg.
// Actions the view does not honour are swallowed, so nothing underneath moves
// while the list is up (the capture pattern).
func (m Model) handleUnhealthyAction(a keymap.Action) (tea.Model, tea.Cmd) {
	if a == keymap.ActionQuit {
		m.closeUnhealthy()
		return m, nil
	}
	var cmd tea.Cmd
	m.unhealthyView, cmd = m.unhealthyView.Update(a)
	return m, cmd
}

// launchScan starts the cross-kind sweep and returns the pump command that pulls
// its first event, with the model carrying the sweep's handle (channel, cancel,
// generation). It cancels any sweep already running (there can only be one — the
// view is exclusive) and bumps the generation so a superseded sweep's events are
// dropped. With nothing to sweep (no available kinds) the view degrades to an
// idle empty list rather than spinning on an in-flight indicator that will never
// clear (principle 3). It mutates a copy and returns it, like every message
// handler — the caller returns this value, never reuses its own.
func (m Model) launchScan() (Model, tea.Cmd) {
	m.stopScan()
	m.scanGen++
	resources := m.scanResources()
	if len(resources) == 0 {
		m.unhealthyView.SetSearching(false)
		return m, nil
	}
	m.unhealthyView.StartProgress(len(resources))
	m.unhealthyView.SetSearching(true)
	gen := m.scanGen
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCancel = cancel
	m.scanCh = m.scanner.Scan(ctx, resources, m.unhealthyScope(), table.UnhealthyRow, scanHitLimit)
	return m, m.pumpScan(gen)
}

// pumpScan issues the tea.Cmd that pulls the next event from the current scan
// channel, tagged with the generation of the sweep that started it so an event
// from a superseded sweep is recognisable as stale. It returns nil when no scan
// is running.
func (m Model) pumpScan(gen int) tea.Cmd {
	ch := m.scanCh
	if ch == nil {
		return nil
	}
	pump := scanPump(ch)
	return func() tea.Msg { return scanMsg{gen: gen, msg: pump()} }
}

// handleScanMsg folds one pumped event into the view and re-issues the pump to
// pull the next — the one-receive-per-Cmd loop that keeps Update from ever
// blocking (M2-02/D53), which is also what makes hits appear kind by kind instead
// of all at once. A message from a superseded sweep, or one arriving after the
// view closed, is dropped and its chain stops. The closed channel ends the
// fan-out: the in-flight indicator clears, leaving the hits on screen (the view
// says "no unhealthy resources" itself when there were none).
//
// A match is appended; a kind-done advances the view's progress line (one per
// requested kind, so it reaches N/N whatever each kind's outcome was — a denied
// group is silent, D131 pt 3, not a stalled counter); the terminal event's Capped
// flag turns on the "first N matches — limit reached" state, the one thing the
// channel close cannot say for itself.
func (m Model) handleScanMsg(s scanMsg) (tea.Model, tea.Cmd) {
	if s.gen != m.scanGen || !m.unhealthyView.Active() {
		return m, nil
	}
	switch inner := s.msg.(type) {
	case ScanEventMsg:
		switch inner.Event.Type {
		case kube.ScanMatch:
			m.unhealthyView.AppendHit(inner.Event.Hit)
		case kube.ScanKindDone:
			m.unhealthyView.MarkKindDone()
		case kube.ScanDone:
			m.unhealthyView.SetCapped(inner.Event.Capped)
			m.unhealthyView.SetSearching(false)
		}
		return m, m.pumpScan(s.gen)
	case ScanClosedMsg:
		m.stopScan()
		m.unhealthyView.SetSearching(false)
		return m, nil
	}
	return m, nil
}

// handleUnhealthySelected drills into the highlighted hit: it closes the
// unhealthy list and switches the browse view to the hit's kind through the same
// selectResource path a menu drill-in or the resource palette takes (start the
// watch, mark the kind active, focus the table). The object itself cannot be
// selected yet — the fresh watch blanks the table and repopulates it when its
// first RESET lands — so the hit's identity is stashed as a pending selection the
// watch pump applies as soon as its row appears (the search drill-in's exact
// pattern, D276).
func (m Model) handleUnhealthySelected(msg unhealthyview.SelectedMsg) (tea.Model, tea.Cmd) {
	m.closeUnhealthy()
	next, cmd := m.selectResource(msg.Hit.Resource)
	sel := next.(Model)
	// Set after selectResource, which clears any pending selection of its own.
	sel.searchTarget = msg.Hit.Row.Object
	sel.hasSearchTarget = true
	return sel, cmd
}

// closeUnhealthy dismisses the unhealthy list and cancels any sweep feeding it,
// bumping the generation so hits still draining from the cancelled sweep are
// dropped rather than appended to a view that is no longer up. It mutates the
// receiver, so callers pass the addressable model they are about to return.
func (m *Model) closeUnhealthy() {
	m.unhealthyView.Hide()
	m.stopScan()
	m.scanGen++
	m.syncHints() // back to the browse view → menu/table-context hints
}

// stopScan cancels the running sweep (if any) and clears its handle so no further
// event is pumped. Safe to call with no sweep running. Called when the sweep
// completes, when the view closes, and on quit — the scan twin of stopSearch. It
// deliberately does not bump the generation: completion is not a supersede, and a
// later close owns that bump.
func (m *Model) stopScan() {
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	m.scanCh = nil
}
