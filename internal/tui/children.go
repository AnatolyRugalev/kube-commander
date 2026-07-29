package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is the M4-08 owner → children drill-down: the gesture that turns a
// selected Deployment / StatefulSet / DaemonSet / Job / ReplicaSet /
// ReplicationController / Service / Node row into a **live** table of that object's
// pods, and the nav.back that returns to the owner.
//
// It is deliberately thin, because M4-07 chose the shape that makes it thin (D165):
// kube.Children hands back a ChildScope — the child Resource, a namespace and the
// metav1.ListOptions that narrow a list/watch to this owner's pods — rather than a
// fetched list. So the drill-down is not a new data path at all: it is the ordinary
// browse watch started with a namespace and a ListOptions the shell carries
// alongside m.current. Sorting, filtering, cell coloring, every row action and the
// watch's own reconnect keep working with no new code, and the child table is live
// rather than a snapshot that would rot the moment a pod restarted.

// ChildResolver is the narrow slice of the kube layer the children drill-down needs
// (M4-07): resolve a selected owner row into the scope its pods are listed under.
// *kube.Clients satisfies it. As with the other seams the shell depends on this
// interface rather than the concrete client, so the drill-down is driveable in
// hermetic tests with a fake resolver. A model built without one (the default) is
// children-inert: the action is a no-op, exactly as it is on a kind with no children.
//
// kinds is the caller's available resource set, so the child Resource comes back
// carrying the verbs discovery actually reported for this cluster (D165 pt 2) — the
// row actions offered on the child table are then gated on the real thing.
type ChildResolver interface {
	Children(ctx context.Context, owner kube.Resource, ref kube.ObjectRef, kinds []kube.Resource) (kube.ChildScope, error)
}

// WithChildResolver wires the kube client the shell resolves an owner's child scope
// with (M4-08). Without it the children drill-down is inert.
func WithChildResolver(c ChildResolver) Option {
	return func(m *Model) { m.childResolver = c }
}

// childScopeMsg carries a resolved child scope back onto the update loop. The
// resolve is a single Get off the loop (and no request at all for a Node), so it is
// a one-shot Cmd rather than a pumped channel; gen is the guard that drops a result
// belonging to a browse state the reader has already left — a second drill-down, a
// namespace change, a context switch — since applying it would yank the table to an
// owner they are no longer looking at.
type childScopeMsg struct {
	gen   int
	owner kube.Resource
	ref   kube.ObjectRef
	scope kube.ChildScope
	err   error
}

// openChildren resolves the selected owner row's child scope off the update loop
// (M4-08). Nothing about the table changes yet: the switch happens in
// handleChildScope, when there is a scope to switch *to*. That ordering is the same
// connect-before-teardown the context switch uses (D157) and matters for the same
// reason — a selector-less Service or a deleted owner must leave the reader exactly
// where they were, with one toast, rather than on a blanked table.
//
// With no resolver (or no watcher) wired the model is children-inert and this is a
// no-op.
func (m Model) openChildren(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.childResolver == nil || m.watcher == nil {
		return m, nil
	}
	m.childGen++
	gen := m.childGen
	resolver, kinds := m.childResolver, m.availableResources()
	owner, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		scope, err := resolver.Children(context.Background(), owner, ref, kinds)
		return childScopeMsg{gen: gen, owner: owner, ref: ref, scope: scope, err: err}
	}
}

// handleChildScope switches the browse table to the resolved child scope, or
// surfaces the refusal. A resolve that failed leaves everything untouched: the
// owner's table is still there, still watching, and the reader sees why the
// drill-down did not happen (principle 3) — kube.Children refuses rather than
// returning a scope that would silently list every pod in the namespace (D165 pt 4).
//
// On success the owner is stashed so nav.back can return to it with the right row
// selected, the scope is stored (selectChildScope), and the ordinary watch is started
// for the child kind.
func (m Model) handleChildScope(msg childScopeMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.childGen {
		return m, nil // the reader has moved on; applying this would yank their table.
	}
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("children of "+viewerTitle(msg.owner, msg.ref), msg.err))
	}
	next, cmd := m.selectChildScope(msg.owner, msg.ref, msg.scope)
	return next, cmd
}

// selectChildScope opens the child table under scope. It sets the scope fields
// *before* starting the watch because watchResource reads them for the namespace and
// the ListOptions it passes to Watch — a child scope's namespace is not the app's
// (a Node's pods are cluster-wide, `Namespace: ""`), so reusing m.namespace would
// silently show one namespace's pods.
func (m Model) selectChildScope(owner kube.Resource, ref kube.ObjectRef, scope kube.ChildScope) (tea.Model, tea.Cmd) {
	m.childOwner, m.childOwnerRef = owner, ref
	m.childScope, m.hasChildScope = scope, true
	return m.watchResource(scope.Resource)
}

// exitChildren leaves a child table and returns to the owner it was opened from —
// the nav.back half of the drill-down. The owner's kind is re-selected through the
// ordinary selectResource path (which clears the scope, so the owner's own table is
// unfiltered) and the owner row is re-selected as soon as the fresh watch delivers
// it, reusing the pending-selection mechanism the search drill-in already has.
//
// It reports whether it did anything, so the nav.back chain can fall through to its
// next level when no child table is open.
func (m Model) exitChildren() (Model, tea.Cmd, bool) {
	if !m.hasChildScope {
		return m, nil, false
	}
	owner, ref := m.childOwner, m.childOwnerRef
	next, cmd := m.selectResource(owner)
	back := next.(Model)
	// Set after selectResource, which clears any pending selection of its own.
	back.searchTarget = ref
	back.hasSearchTarget = true
	return back, cmd, true
}

// clearChildScope drops the drill-down scope and its owner stash. Every path that
// re-points the browse table at something the reader chose directly — a menu
// drill-in, the resource palette, a search hit, a namespace re-scope — goes through
// selectResource, which calls this: the scope belongs to one owner's pods, so it
// must not survive a switch to another kind, and a namespace pick is an explicit
// re-scope of the browse table that a stale selector would silently fight.
//
// It mutates the receiver, so callers pass the addressable model value they are
// about to return.
func (m *Model) clearChildScope() {
	m.childScope, m.hasChildScope = kube.ChildScope{}, false
	m.childOwner, m.childOwnerRef = kube.Resource{}, kube.ObjectRef{}
}

// childScopeLabel is the status-bar rendering of an open drill-down: the owner it
// came from and the selector that narrows the table, e.g.
// "↳ Deployment/api · app=web". Naming the selector is the point (M4-08): a table of
// pods that is *filtered* must not be mistakable for the namespace's pod list.
// Selector() hides which of the two ListOptions fields a given owner kind filled in
// (D165), so this works unchanged for the Node field-selector path. Empty when no
// drill-down is open.
func (m Model) childScopeLabel() string {
	if !m.hasChildScope {
		return ""
	}
	label := "↳ " + m.childOwner.GVK.Kind + "/" + m.childOwnerRef.Name
	if sel := m.childScope.Selector(); sel != "" {
		label += separatorScope + sel
	}
	return label
}

// separatorScope joins the owner and its selector inside the one scope segment. It
// matches the status bar's own segment separator so the whole left side reads as one
// list, while the parts of the scope stay visibly a single clause.
const separatorScope = " · "

// syncScopeStatus pushes the current drill-down label to the status bar. It is
// called wherever the scope changes (opening a child table, leaving one, a reset) so
// the bar and the scope cannot drift. It mutates the receiver.
func (m *Model) syncScopeStatus() {
	m.status.SetScope(m.childScopeLabel())
}
