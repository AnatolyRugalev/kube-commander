package tui

import (
	"context"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
)

// This file is the STORY-06i-2 relations popup: the surface over the kube.Relations
// graph (D286). One gesture on any selected row asks "what is this thing attached
// to" and answers with a modal list of the object's navigable neighbours — the
// owner that created it, the pods it owns, the node it runs on, the claims, config
// maps and secrets it mounts — each row opening the thing it names.
//
// It is deliberately thin, because the primitive was shaped to make it thin (D286):
// a relation is either **named** (one object) or **set-shaped** (a filtered list),
// and the shell already has one path for each. A named relation opens through
// selectResource + the pending selection a search drill-in leaves behind; a
// set-shaped one opens through selectChildScope, the very call the `P` drill-down
// makes. So navigating a relation adds no third way to point the browse table at
// something, and nav.back keeps the meaning it already had on a child table.

// Relater is the narrow slice of the kube layer the relations popup needs: resolve
// one object into the neighbours a reader can navigate to. *kube.Clients satisfies
// it. As with every other seam the shell depends on the interface rather than the
// concrete client, so the popup is driveable in hermetic tests with a fake. A model
// built without one (the default) is relations-inert: the gesture is a no-op,
// exactly as a nil searcher makes cluster search inert.
//
// kinds is the caller's available resource set, so every relation comes back
// carrying a Resource discovery actually reported for this cluster — a target kind
// this cluster cannot see is dropped by the primitive rather than offered here and
// then failing on open (D286).
type Relater interface {
	Relations(ctx context.Context, res kube.Resource, ref kube.ObjectRef, kinds []kube.Resource) ([]kube.Relation, error)
}

// WithRelater wires the kube client the shell resolves an object's relations with.
// Without it the relations popup is inert.
func WithRelater(r Relater) Option {
	return func(m *Model) { m.relater = r }
}

// relationPickerKind is the Kind stamped on the relations picker
// (picker.New(s, "relation")). Every picker emits the same SelectedMsg/CancelledMsg
// types (D65), so the root branches on this Kind to route a picked relation into
// openRelation rather than the container/port/palette paths.
const relationPickerKind = "relation"

// relationsMsg carries the outcome of the async Relations resolve issued by the
// gesture. gen ties it to the relGen bumped when the resolve was requested, so a
// result belonging to a request the reader has already moved past (a second
// gesture, a namespace change, a context switch) is dropped rather than opening a
// popup about an object they are no longer looking at.
type relationsMsg struct {
	gen       int
	res       kube.Resource
	ref       kube.ObjectRef
	relations []kube.Relation
	err       error
}

// openRelations resolves the selected row's relations off the update loop. Nothing
// opens yet: the popup appears in handleRelations, when there is a list to show —
// the same connect-before-teardown ordering the children drill-down uses, and for
// the same reason (a failed resolve must leave the reader exactly where they were,
// with one toast).
func (m Model) openRelations(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.relater == nil || msg.Object.Name == "" {
		return m, nil
	}
	m.relGen++
	gen := m.relGen
	relater, kinds := m.relater, m.availableResources()
	res, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		rels, err := relater.Relations(context.Background(), res, ref, kinds)
		return relationsMsg{gen: gen, res: res, ref: ref, relations: rels, err: err}
	}
}

// handleRelations opens the popup over the resolved list. An object with no
// neighbours is a true answer rather than a failure (D286), so it says so in a
// notice instead of opening an empty modal — a popup with nothing in it is a dead
// end the reader then has to dismiss.
func (m Model) handleRelations(msg relationsMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.relGen {
		return m, nil
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("relations of "+viewerTitle(msg.res, msg.ref), msg.err))
	}
	items, byLabel := relationItems(msg.relations, m.namespace)
	if len(items) == 0 {
		return m, m.surfaceNotice("No related resources for " + viewerTitle(msg.res, msg.ref))
	}
	m.relRes, m.relRef, m.relByLabel = msg.res, msg.ref, byLabel
	m.relPicker.SetTitle("Related · " + viewerTitle(msg.res, msg.ref))
	m.relPicker.SetItemsWithAliases(items)
	return m, m.relPicker.Show()
}

// handleRelationSelected navigates to the picked relation. Which of the two paths
// it takes is the relation's shape, not its role (D286):
//
//   - **set-shaped** (an owner's pods): selectChildScope, the drill-down `P` takes,
//     with the popup's source object as the owner — so the scope label names it and
//     nav.back returns to it.
//   - **named** (everything else): selectResource plus the pending selection, the
//     path a search hit and an unhealthy hit already take, so the object is selected
//     as soon as the fresh watch delivers its row.
//
// A named target outside the browsed namespace (a PersistentVolume's claim, whose
// namespace is the claimRef's) re-scopes the shell first: without it the pending
// selection would wait for a row the watch is never going to list. The re-scope is
// display state only — it is not persisted as the reader's chosen namespace,
// because they picked a resource, not a scope — and an all-namespaces scope is
// never narrowed, since it already lists the target's row.
func (m Model) handleRelationSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	rel, ok := m.relByLabel[msg.Value]
	res, ref := m.relRes, m.relRef
	m.closeRelations()
	if !ok {
		return m, nil
	}
	if rel.IsSet() {
		return m.selectChildScope(res, ref, rel.Scope())
	}
	if rel.Resource.Namespaced && rel.Namespace != "" && m.namespace != "" && rel.Namespace != m.namespace {
		m.setNamespace(rel.Namespace)
	}
	next, cmd := m.selectResource(rel.Resource)
	sel := next.(Model)
	// Set after selectResource, which clears any pending selection of its own.
	sel.searchTarget = rel.Ref()
	sel.hasSearchTarget = true
	return sel, cmd
}

// closeRelations dismisses the popup and drops the row map and the source stash
// behind it. It mutates the receiver, so callers pass the addressable model value
// they are about to return.
func (m *Model) closeRelations() {
	m.relPicker.Hide()
	m.relByLabel = nil
	m.relRes, m.relRef = kube.Resource{}, kube.ObjectRef{}
}

// relationItems renders the relations as picker rows and the map resolving a picked
// label back to its relation (a SelectedMsg carries only the label, D65 — the
// resByLabel/ctrByLabel pattern).
//
// The rows are **grouped by direction** — what made this object, then what it
// makes, then what it merely references — because that is how a reader thinks about
// a graph they are standing in the middle of. A picker has no section headings, so
// the grouping is a stable sort plus the arrow each row's name column carries
// (`↑ owner`, `↓ pods`, `→ secret`): the list reads as three blocks without the
// component learning what a relation is.
//
// The label is the target's identity — `Kind/name`, or `Kind (selector)` for a
// set-shaped one — qualified with the namespace when it is not the one being
// browsed, so a cross-namespace hop says so before it is taken (under an
// all-namespaces scope that means every namespaced target names its own, which is
// what the browse table shows there too). Two rows can only
// collapse to one label when they name the same object (a Secret mounted as a
// volume *and* used as an image-pull secret), and navigating either would land in
// the same place, so keeping the first loses nothing.
func relationItems(rels []kube.Relation, ns string) ([]picker.Item, map[string]kube.Relation) {
	ordered := append([]kube.Relation(nil), rels...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Direction < ordered[j].Direction
	})
	items := make([]picker.Item, 0, len(ordered))
	byLabel := make(map[string]kube.Relation, len(ordered))
	for _, rel := range ordered {
		if rel.Resource.GVK.Kind == "" {
			continue // a relation with no target kind cannot be opened.
		}
		label := relationLabel(rel, ns)
		if _, dup := byLabel[label]; dup {
			continue
		}
		byLabel[label] = rel
		items = append(items, picker.Item{
			Name:  relationArrow(rel.Direction) + " " + string(rel.Role),
			Label: label,
		})
	}
	return items, byLabel
}

// relationArrow is the direction's glyph: what made this (↑), what it makes (↓),
// what it references (→). An unknown direction renders as the neutral side arrow
// rather than nothing, so a row never loses its column alignment.
func relationArrow(d kube.RelationDirection) string {
	switch d {
	case kube.RelationUp:
		return "↑"
	case kube.RelationDown:
		return "↓"
	}
	return "→"
}

// relationLabel is the target's identity as the popup lists it, and the key a pick
// resolves by. A set-shaped relation is named by its kind and the selector that
// narrows it (`Pod (app=web)`) — the same thing the status bar's scope label says
// once it is open, so the row and the table it opens agree. A named one is
// `Kind/name`, with the namespace appended when it differs from the browsed scope.
func relationLabel(rel kube.Relation, ns string) string {
	kind := rel.Resource.GVK.Kind
	if rel.IsSet() {
		if sel := rel.Scope().Selector(); sel != "" {
			return kind + " (" + sel + ")"
		}
		return kind
	}
	label := kind + "/" + rel.Name
	if rel.Namespace != "" && rel.Namespace != ns {
		label += separatorScope + rel.Namespace
	}
	return label
}
