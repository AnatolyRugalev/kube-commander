package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
)

// ResourcePersister records the kind last browsed on the active kubeconfig context so
// the next launch — and every switch back — reopens on it instead of the welcome pane
// (CTX-MEM-02/D240). It is the write side of config.State.LastResource, and it has the
// shape both other per-context writers have for the same reason: the launcher alone
// knows the resolved context and its state-file path, so it wires a persister already
// bound to that context and the tui package stays storage- and context-agnostic.
//
// What crosses this seam is an *address* and nothing else — a GVR plus the display
// hints that ride with it, exactly what a pin records (D240 pt 2). No row, no table and
// no client may be written here: rows fetched from a cluster kubecom is not connected to
// are stale the moment they are stored and have no correct refresh story, which is why
// the restore re-watches through the ordinary drill-in path rather than replaying data.
//
// A model built without one (the default, or an unresolved context) is memory-inert:
// browsing applies for the session and is simply not recorded.
type ResourcePersister interface {
	PersistResource(r config.MenuResource) error
	PersistViewState(sortCol string, sortAsc bool) error
}

// WithResourcePersister wires the per-context state writer the shell calls when a
// drill-in changes which kind is open (CTX-MEM-02). Without it nothing is recorded.
func WithResourcePersister(p ResourcePersister) Option {
	return func(m *Model) { m.resPersister = p }
}

// WithLastResource seeds the kind this context was last left browsing, as read from
// its state file (config.State.LastResource). Non-nil arms one restore attempt, taken
// when the launch discovery pass reconciles — by then the menu holds the cluster's real
// API surface, so a remembered CRD is resolvable rather than merely absent (D240 pt 4).
// Nil (the default, and every hermetic test that does not wire one) restores nothing.
func WithLastResource(r *config.MenuResource, sortCol string, sortAsc bool) Option {
	return func(m *Model) {
		m.lastResource = r
		m.lastSortCol = sortCol
		m.lastSortAsc = sortAsc
		m.restorePending = r != nil
	}
}

// recordResource notes r as the kind this context was last browsing and returns the
// Cmd that writes it to the state file, or nil when there is nothing to do. It is
// called from watchResource — the single point every browse watch passes through — so
// every way of opening a table (the menu, the `:resource ` stage, a search hit, a
// namespace re-scope, a children drill-down) records the same way and none can be
// forgotten by a later leg adding a sixth.
//
// A re-scope or a re-select of the kind already recorded writes nothing: the file
// already says what the caller wants it to say, which is the dedupe PersistPin makes
// for the same reason. Without it every namespace change would rewrite the state file
// to its current contents.
//
// It mutates the receiver, so callers pass the addressable model value they are about
// to return. The write itself runs off the update loop and a failure is a transient
// toast, not a fatal: the table is already open (principle 3).
func (m *Model) recordResource(entry config.MenuResource) tea.Cmd {
	if m.resPersister == nil {
		return nil
	}
	if m.lastResource != nil && indexOfGVR([]config.MenuResource{*m.lastResource}, entry) >= 0 {
		return nil
	}
	// A remembered address is the whole of what a restore replays, so the model holds
	// its own copy rather than aliasing the caller's value.
	stored := entry
	m.lastResource = &stored
	p := m.resPersister
	return func() tea.Msg {
		if err := p.PersistResource(stored); err != nil {
			return NewErrorMsg("persist resource", err)
		}
		return nil
	}
}

// recordViewState notes the table's current view state (sort column and direction)
// and returns the Cmd that writes it to the state file, or nil when there is nothing
// to do. Called when the sort changes so a context switch restores it.
func (m *Model) recordViewState() tea.Cmd {
	if m.resPersister == nil {
		return nil
	}
	colName, sorted := m.table.SortColumnName()
	var sortAsc bool
	if sorted {
		sortAsc = !m.table.SortDescending()
	}
	p := m.resPersister
	return func() tea.Msg {
		if err := p.PersistViewState(colName, sortAsc); err != nil {
			return NewErrorMsg("persist view state", err)
		}
		return nil
	}
}

// restoreLastResource reopens the kind this context was left browsing, once, after the
// discovery pass that follows a launch or a context switch has reconciled into the menu
// (handleDiscovery). Waiting for discovery is the point: before it the menu is the seed,
// so a remembered CRD would be "not served here" on every cluster that does serve it.
//
// **The degrade is the feature** (D240 pt 3). A restore is a convenience and must never
// be the reason a switch shows an error, so every way of not finding the kind — the
// cluster does not serve it, the API group failed discovery, the row is unavailable —
// ends in the same place the reader lands today: the seed menu and the welcome pane,
// silently. Nothing is toasted and nothing is logged as a failure, because nothing
// failed; a context that serves a different set of kinds is the ordinary case, not a
// fault.
//
// The attempt is consumed whatever the outcome. A later discovery pass on the same
// cluster (a reconnect) must not yank the reader back to where they started, and
// neither must a reader who drilled in themselves while the pass was still running —
// hasCurrent is that check, and it deliberately loses to the human.
func (m Model) restoreLastResource() (Model, tea.Cmd) {
	if !m.restorePending {
		return m, nil
	}
	m.restorePending = false
	if m.lastResource == nil || m.hasCurrent {
		return m, nil
	}
	i, ok := menuIndexOfGVR(m.menu.Items(), *m.lastResource)
	if !ok {
		return m, nil // this cluster does not serve the remembered kind — say nothing.
	}
	m.menu.SelectItem(i)
	next, cmd := m.selectResource(m.menu.Items()[i].Resource)
	return next.(Model), cmd
}

// menuIndexOfGVR finds the menu row addressing entry's group/version/resource, matching
// indexOfGVR's rule that a GVR is a kind's identity and title/section/Kind spelling are
// presentation. Rows that cannot be watched are skipped rather than matched: the
// namespace seam has no GVR at all, and an unavailable row names a kind whose API group
// discovery could not load, so drilling into it would start a watch that can only fail —
// which is precisely the error the restore promises never to cause.
func menuIndexOfGVR(items []menu.Item, entry config.MenuResource) (int, bool) {
	for i, it := range items {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		gvr := it.Resource.GVR
		if gvr.Group == entry.Group && gvr.Version == entry.Version && gvr.Resource == entry.Resource {
			return i, true
		}
	}
	return 0, false
}
