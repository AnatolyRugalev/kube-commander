package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
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
// drill is the address of the object a drill-in scope was opened from — nil for a plain
// table. It rides the same write so the two halves of "where I was" cannot disagree, and
// a drill-in records the child kind *and* the owner in one file state.
//
// A model built without one (the default, or an unresolved context) is memory-inert:
// browsing applies for the session and is simply not recorded.
type ResourcePersister interface {
	PersistResource(r config.MenuResource, drill *config.DrillOwner) error
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

// WithLastDrillOwner seeds the owner object of the last drill-in scope, as read from
// its state file (config.State.LastDrillOwner). It only matters alongside a remembered
// kind — a drill-in's pane is still a kind, its scope just names the owner it was
// narrowed to — so it arms nothing of its own; the restore consumes it when it exists.
func WithLastDrillOwner(o *config.DrillOwner) Option {
	return func(m *Model) { m.lastDrillOwner = o }
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
	drill := m.currentDrillOwner()
	if m.lastResource != nil && indexOfGVR([]config.MenuResource{*m.lastResource}, entry) >= 0 &&
		drillOwnerEqual(m.lastDrillOwner, drill) {
		return nil
	}
	// A remembered address is the whole of what a restore replays, so the model holds
	// its own copy rather than aliasing the caller's value.
	stored := entry
	m.lastResource = &stored
	m.lastDrillOwner = drill
	p := m.resPersister
	return func() tea.Msg {
		if err := p.PersistResource(stored, drill); err != nil {
			return NewErrorMsg("persist resource", err)
		}
		return nil
	}
}

// currentDrillOwner is the drill-in address to record right now: the owner a live
// child scope was opened from, or nil when no drill-in is active (a plain table). It
// is what makes a drill-in's pane remember *which owner's* children were open, so the
// restore can re-enter the scope instead of landing on the plain child list
// (CTX-MEM-04/D240 pt 6).
func (m Model) currentDrillOwner() *config.DrillOwner {
	if !m.hasChildScope {
		return nil
	}
	return &config.DrillOwner{
		Resource:  pinEntry(m.childOwner),
		Namespace: m.childOwnerRef.Namespace,
		Name:      m.childOwnerRef.Name,
	}
}

// drillOwnerEqual reports whether two remembered drill-in addresses name the same
// owner, used as the write-half of recordResource's dedupe: a re-scope of a plain
// table must clear a stale drill owner (nil ≠ owner), and re-drilling the same owner
// must not rewrite the file.
func drillOwnerEqual(a, b *config.DrillOwner) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	return a.Namespace == b.Namespace && a.Name == b.Name &&
		indexOfGVR([]config.MenuResource{a.Resource}, b.Resource) >= 0
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
// A remembered drill-in is the one restore that does not land on the plain list
// (CTX-MEM-04/D240 pt 6): the pane was narrowed to an owner's children, and the restore
// re-resolves that owner's scope and re-enters it. That re-resolve is an object Get that
// can fail — the owner is gone, or its scope cannot be re-derived — and the failure must
// be legible rather than silent, so it falls back to the plain child list with a notice
// saying the owner is gone. It is exactly the "degrade, don't crash" rule the rest of
// the restore follows; the difference is that this degrade has something real to say.
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
	// A remembered drill-in re-opens the *child table under the owner's scope* when
	// the owner can still be re-resolved, instead of landing on the plain child list.
	// The re-resolve is a single Get, so it runs off the loop like the live
	// drill-down's, and it rides the same gen guard: a reader who drilled in
	// themselves, or a context switch, drops the stale result.
	if m.lastDrillOwner != nil && m.childResolver != nil {
		return m.restoreDrillIn(i)
	}
	next, cmd := m.selectResource(m.menu.Items()[i].Resource)
	return next.(Model), cmd
}

// restoreDrillIn is the drill-in half of restoreLastResource: it re-resolves the
// remembered owner's child scope off the update loop, exactly as the live drill-down's
// openChildren does, and hands the result back as a restoreDrillMsg. The gen is the
// child-scope generation, so an openChildren by the reader or a resetCluster bumps it
// and the stale result is dropped by handleRestoreDrill.
func (m Model) restoreDrillIn(menuIdx int) (Model, tea.Cmd) {
	child := m.menu.Items()[menuIdx].Resource
	owner := drillOwnerResource(*m.lastDrillOwner)
	ref := kube.ObjectRef{Namespace: m.lastDrillOwner.Namespace, Name: m.lastDrillOwner.Name}
	m.childGen++
	gen := m.childGen
	resolver, kinds := m.childResolver, m.availableResources()
	return m, func() tea.Msg {
		scope, err := resolver.Children(context.Background(), owner, ref, kinds)
		return restoreDrillMsg{gen: gen, child: child, owner: owner, ref: ref, scope: scope, err: err}
	}
}

// restoreDrillMsg carries a remembered drill-in's owner re-resolve back onto the
// update loop (CTX-MEM-04). child is the child kind the pane was on, kept so the
// plain-list fallback can open it when the owner is gone.
type restoreDrillMsg struct {
	gen   int
	child kube.Resource
	owner kube.Resource
	ref   kube.ObjectRef
	scope kube.ChildScope
	err   error
}

// handleRestoreDrill applies a remembered drill-in's re-resolved scope, or degrades it
// legibly. A result for a browse state the reader has already left (drilled in, or the
// cluster changed) is dropped. A successful re-resolve opens the child table under the
// scope through the ordinary drill-in path — so the scope is re-derived fresh, never
// replayed (D240 pt 6). A failed re-resolve lands on the plain child list **and says
// on screen that the owner is gone**: silently landing in a different scope is worse
// than landing on the plain list, and the reader must be able to tell which happened.
func (m Model) handleRestoreDrill(msg restoreDrillMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.childGen || m.hasCurrent {
		return m, nil // superseded by the reader, or the cluster changed — drop.
	}
	if msg.err != nil {
		next, cmd := m.selectResource(msg.child)
		n := next.(Model)
		notice := fmt.Sprintf("the owner of the remembered drill-in (%s) is gone — showing all %s",
			viewerTitle(msg.owner, msg.ref), msg.child.GVR.Resource)
		return n, tea.Batch(cmd, n.surfaceNotice(notice))
	}
	return m.selectChildScope(msg.owner, msg.ref, msg.scope)
}

// drillOwnerResource rebuilds the owner kube.Resource a remembered drill-in address
// names, so the restore can re-resolve the scope through the ordinary ChildResolver
// seam. The Kind and Namespaced hints ride the recorded address exactly as pinEntry
// stored them, so the reconstructed owner is the one the reader drilled into.
func drillOwnerResource(o config.DrillOwner) kube.Resource {
	return kube.Resource{
		GVK:        schema.GroupVersionKind{Group: o.Resource.Group, Version: o.Resource.Version, Kind: o.Resource.Kind},
		GVR:        schema.GroupVersionResource{Group: o.Resource.Group, Version: o.Resource.Version, Resource: o.Resource.Resource},
		Namespaced: o.Resource.Namespaced,
	}
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
