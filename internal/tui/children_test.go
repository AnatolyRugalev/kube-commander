package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// childrenKey is the default res.children direct key (`P`).
var childrenKey = tea.Key{Code: 'P', Text: "P"}

// fakeChildResolver is a hermetic ChildResolver: it hands back a preset scope (or an
// error) and records what it was asked, so a test can assert the owner, the ref and
// the available-kinds snapshot all reached the kube layer.
type fakeChildResolver struct {
	scope    kube.ChildScope
	err      error
	calls    int
	gotOwner kube.Resource
	gotRef   kube.ObjectRef
	gotKinds []kube.Resource
}

func (f *fakeChildResolver) Children(_ context.Context, owner kube.Resource, ref kube.ObjectRef, kinds []kube.Resource) (kube.ChildScope, error) {
	f.calls++
	f.gotOwner, f.gotRef, f.gotKinds = owner, ref, kinds
	return f.scope, f.err
}

// podScope is the scope a label-selector owner (a Deployment) resolves to.
func podScope() kube.ChildScope {
	return kube.ChildScope{
		Resource:  kindResource("pods", "Pod"),
		Namespace: "default",
		Options:   metav1.ListOptions{LabelSelector: "app=web"},
	}
}

// ownerResource is kindResource with a group, which the drill-down needs: kube's
// owner→child table is keyed by GroupKind, so an apps/Deployment and a bare
// ""/Deployment are not the same kind (D165 pt 2).
func ownerResource(resource, group, kind string) kube.Resource {
	r := gvrResource(resource)
	r.GVK = schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind}
	return r
}

// ownerTable opens a live table of the given owner kind with a resolver wired, ready
// for the drill-down gesture. The rows are the sortReset pods, which is fine: the
// drill-down acts on the *selected row's* ObjectRef and the owner kind, neither of
// which the row's cells affect.
func ownerTable(t *testing.T, group, kind string, r *fakeChildResolver) (Model, *fakeWatcher) {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithChildResolver(r))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: ownerResource("owners", group, kind)})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model), fw
}

// drillToChildren runs the whole gesture: press `P`, feed the resulting intent back
// in, then deliver the resolver's answer. It returns the model showing the child
// table.
func drillToChildren(t *testing.T, m Model) Model {
	t.Helper()
	_, cmd := press(t, m, childrenKey)
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.children produced %T, want rowActionMsg", cmd())
	}
	next, resolveCmd := m.Update(intent)
	m = next.(Model)
	if resolveCmd == nil {
		t.Fatal("handling the children intent should issue a scope resolve command")
	}
	scopeMsg, ok := resolveCmd().(childScopeMsg)
	if !ok {
		t.Fatalf("resolve produced %T, want childScopeMsg", resolveCmd())
	}
	next, _ = m.Update(scopeMsg)
	return next.(Model)
}

// TestChildrenDrillDownScopesTheWatch is the leg's headline: pressing res.children on
// an owner row switches the browse table to the child kind and starts the watch under
// the resolved scope's *namespace and ListOptions* — not the app's namespace and an
// empty options — which is what makes the child table a filtered live watch rather
// than the namespace's whole pod list (D165 pt 1).
func TestChildrenDrillDownScopesTheWatch(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, fw := ownerTable(t, "apps", "Deployment", r)
	watches := len(fw.res)

	m = drillToChildren(t, m)

	if r.calls != 1 {
		t.Fatalf("Children called %d times, want 1", r.calls)
	}
	if r.gotOwner.GVK.Kind != "Deployment" {
		t.Errorf("resolver got owner kind %q, want Deployment", r.gotOwner.GVK.Kind)
	}
	if r.gotRef.Name == "" {
		t.Error("the resolve should be addressed to the selected row's object")
	}
	if len(fw.res) != watches+1 {
		t.Fatalf("the drill-down should start one new watch (%d → %d)", watches, len(fw.res))
	}
	if got := fw.res[len(fw.res)-1].GVK.Kind; got != "Pod" {
		t.Errorf("child watch is on %q, want the scope's Pod resource", got)
	}
	if got := fw.ns[len(fw.ns)-1]; got != "default" {
		t.Errorf("child watch namespace %q, want the scope's %q", got, "default")
	}
	if got := fw.opts[len(fw.opts)-1].LabelSelector; got != "app=web" {
		t.Errorf("child watch label selector %q, want the scope's %q", got, "app=web")
	}
	if !m.hasChildScope {
		t.Error("the model should be holding the drill-down scope")
	}
	if !m.hasCurrent || m.current.GVK.Kind != "Pod" {
		t.Error("the browsed resource should now be the child kind")
	}
}

// TestChildrenScopeNamedInStatusBar proves the filter is *visible*: a table of pods
// narrowed to one owner must not be mistakable for the namespace's pod list, so the
// bar names both the owner and the selector the scope applied (M4-08).
func TestChildrenScopeNamedInStatusBar(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, _ := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)

	view := m.View().Content
	if !strings.Contains(view, "Deployment/") {
		t.Errorf("the status bar should name the owner the table is scoped to: %q", firstLines(view))
	}
	if !strings.Contains(view, "app=web") {
		t.Errorf("the status bar should name the scope's selector: %q", firstLines(view))
	}
}

// TestChildScopeLabelUsesFieldSelectorForNode proves the label works for the owner
// whose scope carries a *field* selector and no namespace (a Node's pods are
// cluster-wide): ChildScope.Selector() hides which of the two ListOptions fields was
// filled in, so the bar needs no owner-kind knowledge (D165).
func TestChildScopeLabelUsesFieldSelectorForNode(t *testing.T) {
	r := &fakeChildResolver{scope: kube.ChildScope{
		Resource:  kindResource("pods", "Pod"),
		Namespace: "",
		Options:   metav1.ListOptions{FieldSelector: "spec.nodeName=node-1"},
	}}
	m, fw := ownerTable(t, "", "Node", r)
	m = drillToChildren(t, m)

	if got := fw.ns[len(fw.ns)-1]; got != "" {
		t.Errorf("a Node's children watch namespace is %q, want cluster-wide (\"\")", got)
	}
	if got := fw.opts[len(fw.opts)-1].FieldSelector; got != "spec.nodeName=node-1" {
		t.Errorf("child watch field selector %q, want the scope's", got)
	}
	if label := m.childScopeLabel(); !strings.Contains(label, "spec.nodeName=node-1") {
		t.Errorf("scope label %q should name the field selector", label)
	}
}

// TestChildrenRefusalLeavesTableUntouched is the connect-before-teardown half: a
// refused resolve (a selector-less Service, a deleted owner, an RBAC denial —
// kube.Children refuses rather than returning a match-everything scope, D165 pt 4)
// must leave the reader exactly where they were, with one toast.
func TestChildrenRefusalLeavesTableUntouched(t *testing.T) {
	r := &fakeChildResolver{err: errors.New("matches everything")}
	m, fw := ownerTable(t, "", "Service", r)
	watches := len(fw.res)

	m = drillToChildren(t, m)

	if len(fw.res) != watches {
		t.Errorf("a refused resolve should start no new watch (%d → %d)", watches, len(fw.res))
	}
	if m.hasChildScope {
		t.Error("a refused resolve should leave no drill-down scope behind")
	}
	if m.current.GVK.Kind != "Service" {
		t.Errorf("the owner's table should still be open, got %q", m.current.GVK.Kind)
	}
	if !m.status.HasError() {
		t.Error("a refused resolve should surface a transient error toast")
	}
}

// TestChildrenStaleScopeDropped proves the generation guard: a scope resolved for a
// browse state the reader has already left (a second drill-down, a namespace change,
// a context switch) is dropped rather than yanking the table to an owner they are no
// longer looking at — the same stale-message guard watchGen gives the table watch.
func TestChildrenStaleScopeDropped(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, fw := ownerTable(t, "apps", "Deployment", r)
	watches := len(fw.res)

	stale := childScopeMsg{gen: m.childGen - 1, owner: m.current, scope: podScope()}
	next, _ := m.Update(stale)
	m = next.(Model)

	if m.hasChildScope {
		t.Error("a stale scope should not be applied")
	}
	if len(fw.res) != watches {
		t.Errorf("a stale scope should start no watch (%d → %d)", watches, len(fw.res))
	}
}

// TestChildrenBackReturnsToOwner proves nav.back is the other half of the gesture: esc
// on a child table re-opens the owner's kind and arms the pending selection so the row
// it was opened from is re-selected when the fresh watch delivers it.
func TestChildrenBackReturnsToOwner(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, fw := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)
	ownerRef := m.childOwnerRef
	watches := len(fw.res)

	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})

	if m.hasChildScope {
		t.Fatal("nav.back should leave the drill-down scope")
	}
	if len(fw.res) != watches+1 {
		t.Fatalf("nav.back should re-open the owner's kind (%d → %d watches)", watches, len(fw.res))
	}
	if got := fw.res[len(fw.res)-1].GVK.Kind; got != "Deployment" {
		t.Errorf("nav.back re-opened %q, want the owner kind Deployment", got)
	}
	if got := fw.opts[len(fw.opts)-1]; got.LabelSelector != "" || got.FieldSelector != "" {
		t.Errorf("the owner's own table must be unscoped, got %+v", got)
	}
	if !m.hasSearchTarget || m.searchTarget != ownerRef {
		t.Errorf("nav.back should arm the owner row as a pending selection, got %+v", m.searchTarget)
	}
	if m.status.HasError() {
		t.Error("returning to the owner is not an error")
	}
}

// TestChildrenBackIsOneLevel proves the drill-down is its own level in the nav.back
// chain: the first esc returns to the owner (the table keeps focus, since the reader
// is still browsing), and only the second pops focus back to the menu.
func TestChildrenBackIsOneLevel(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, _ := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)

	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if !m.table.Focused() {
		t.Fatal("the first esc leaves the drill-down, it does not also pop focus")
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.table.Focused() || !m.menu.Focused() {
		t.Error("the second esc should hand focus back to the menu")
	}
}

// TestChildScopeClearedBySelectingAnotherKind proves the scope belongs to one owner's
// pods: every path that points the table at something the reader chose directly (a
// menu drill-in here) goes through selectResource, which drops it — otherwise the next
// kind would be silently narrowed by the departed owner's selector.
func TestChildScopeClearedBySelectingAnotherKind(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, fw := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("configmaps", "ConfigMap")})
	m = next.(Model)

	if m.hasChildScope {
		t.Error("selecting another kind should drop the drill-down scope")
	}
	if got := fw.opts[len(fw.opts)-1]; got.LabelSelector != "" {
		t.Errorf("the new kind's watch must be unscoped, got %+v", got)
	}
	if label := m.childScopeLabel(); label != "" {
		t.Errorf("the status-bar scope should be cleared, got %q", label)
	}
}

// TestChildScopeReappliedOnWatchRestart pins the invariant the M4-07 journal flagged:
// the scope is read where the watch is *started*, not at the drill-down call site, so
// a restart (a reconnect's re-list, any re-selection of the same kind) re-applies it
// instead of silently widening the table to the whole namespace on the second pass.
func TestChildScopeReappliedOnWatchRestart(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, fw := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)

	next, _ := m.watchResource(m.current)
	m = next.(Model)

	last := fw.opts[len(fw.opts)-1]
	if last.LabelSelector != "app=web" {
		t.Errorf("the restarted watch dropped the scope: %+v", last)
	}
	if got := fw.ns[len(fw.ns)-1]; got != "default" {
		t.Errorf("the restarted watch namespace %q, want the scope's %q", got, "default")
	}
}

// TestChildScopeClearedOnClusterReset proves the scope does not survive a context
// switch: it names an owner on the departing cluster, and a stale one would scope the
// new cluster's table by a selector nothing there was resolved against (M4-03).
func TestChildScopeClearedOnClusterReset(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, _ := ownerTable(t, "apps", "Deployment", r)
	m = drillToChildren(t, m)

	gen := m.childGen
	m.resetCluster()

	if m.hasChildScope || m.childOwnerRef.Name != "" {
		t.Error("resetCluster should drop the drill-down scope and its owner stash")
	}
	if m.childGen == gen {
		t.Error("resetCluster should stale an in-flight scope resolve (childGen bump)")
	}
	if label := m.childScopeLabel(); label != "" {
		t.Error("resetCluster should clear the status-bar scope")
	}
}

// TestChildrenInertWithoutResolver proves a model with no ChildResolver wired is
// children-inert: the action is recognised (the intent is dispatched) but resolves to
// nothing, exactly as every other seam degrades to a no-op rather than a panic.
func TestChildrenInertWithoutResolver(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: ownerResource("deployments", "apps", "Deployment")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg))
	m = next.(Model)

	_, keyCmd := press(t, m, childrenKey)
	intent, ok := keyCmd().(rowActionMsg)
	if !ok {
		t.Fatalf("res.children produced %T, want rowActionMsg", keyCmd())
	}
	next, resolveCmd := m.Update(intent)
	if resolveCmd != nil {
		t.Error("with no resolver wired the drill-down should issue no command")
	}
	if next.(Model).hasChildScope {
		t.Error("with no resolver wired there is no scope to hold")
	}
}

// TestChildrenActionAppliesToOwnerKindsOnly proves applicability is kube.HasChildren —
// the exported predicate, not a kind list in the TUI (D165) — so the owner set lives
// in one place and a Deployment offers the entry while a ConfigMap does not.
func TestChildrenActionAppliesToOwnerKindsOnly(t *testing.T) {
	owners := []struct{ group, kind string }{
		{"apps", "Deployment"}, {"apps", "StatefulSet"}, {"apps", "DaemonSet"},
		{"apps", "ReplicaSet"}, {"batch", "Job"},
		{"", "ReplicationController"}, {"", "Service"}, {"", "Node"},
	}
	for _, o := range owners {
		if !rowActionApplies(rowActionChildren, ownerResource("x", o.group, o.kind)) {
			t.Errorf("%s/%s should offer the children drill-down", o.group, o.kind)
		}
	}
	notOwners := []struct{ group, kind string }{
		{"", "Pod"}, {"", "ConfigMap"}, {"", "Secret"}, {"batch", "CronJob"},
		// The group is part of the key: a CRD that merely *calls itself* Deployment
		// is not the apps one, and must not inherit its selector rules (D165 pt 2).
		{"acme.io", "Deployment"},
	}
	for _, o := range notOwners {
		if rowActionApplies(rowActionChildren, ownerResource("x", o.group, o.kind)) {
			t.Errorf("%s/%s has no children scope and should not offer the drill-down", o.group, o.kind)
		}
	}
}

// TestChildrenActionListedInActionStage proves the gesture is reachable without
// knowing its key — `a`'s action stage lists it for an owner kind and omits it for a
// Pod, matching the direct key's own applicability guard.
func TestChildrenActionListedInActionStage(t *testing.T) {
	m, _ := ownerTable(t, "apps", "Deployment", &fakeChildResolver{scope: podScope()})
	m = openActionStage(t, m)
	if _, ok := m.palRowByLabel["Show pods"]; !ok {
		t.Error("a Deployment's action stage should list the children drill-down")
	}

	m = openPodTable(t, "Pod")
	m = openActionStage(t, m)
	if _, ok := m.palRowByLabel["Show pods"]; ok {
		t.Error("a Pod's action stage should not list the children drill-down")
	}
}

// TestChildrenKeyInertOnNonOwnerKind proves the direct key is guarded by the same
// predicate as the menu entry: `P` on a Pod dispatches nothing at all, rather than
// dispatching an intent that would resolve to an error.
func TestChildrenKeyInertOnNonOwnerKind(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, WithWatcher(fw), WithChildResolver(r))
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg))
	m = next.(Model)

	_, keyCmd := press(t, m, childrenKey)
	if keyCmd != nil {
		t.Error("res.children on a kind with no children should dispatch nothing")
	}
	if r.calls != 0 {
		t.Error("an inert key must not reach the resolver")
	}
}

// TestChildrenResolverGetsAvailableKinds proves the child Resource is looked up in the
// *caller's* discovered set (D165 pt 2), so the child table's rows carry the verbs
// this cluster actually reports and the row actions offered on them stay honest.
func TestChildrenResolverGetsAvailableKinds(t *testing.T) {
	r := &fakeChildResolver{scope: podScope()}
	m, _ := ownerTable(t, "apps", "Deployment", r)
	drillToChildren(t, m)

	if len(r.gotKinds) == 0 {
		t.Fatal("the resolver should be handed the available kind set")
	}
	var sawPods bool
	for _, k := range r.gotKinds {
		if k.GVR.Resource == "pods" {
			sawPods = true
		}
	}
	if !sawPods {
		t.Error("the available kind set should include the seed menu's pods")
	}
}

// firstLines trims a rendered view to something readable in a failure message.
func firstLines(view string) string {
	lines := strings.Split(view, "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, "\n")
}
