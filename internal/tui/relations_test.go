package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
)

// fakeRelater is a hermetic Relater: it hands back a preset relation set (or an
// error) and records what it was asked, so a test can assert the object and the
// available-kinds snapshot reached the kube layer.
type fakeRelater struct {
	rels     []kube.Relation
	err      error
	calls    int
	gotRes   kube.Resource
	gotRef   kube.ObjectRef
	gotKinds []kube.Resource
}

func (f *fakeRelater) Relations(_ context.Context, res kube.Resource, ref kube.ObjectRef, kinds []kube.Resource) ([]kube.Relation, error) {
	f.calls++
	f.gotRes, f.gotRef, f.gotKinds = res, ref, kinds
	return f.rels, f.err
}

// podRelations is a pod's neighbour set as kube.Relations returns it: an owner
// above, a set-shaped child scope below (a ReplicaSet's pods would be listed on the
// owner; here it stands in for the down direction) and two spec links sideways.
func podRelations() []kube.Relation {
	return []kube.Relation{
		{
			Role: kube.RoleChildren, Direction: kube.RelationDown,
			Resource: kindResource("pods", "Pod"), Namespace: "default",
			Options: metav1.ListOptions{LabelSelector: "app=web"},
		},
		{
			Role: kube.RoleSecret, Direction: kube.RelationSide,
			Resource: kindResource("secrets", "Secret"), Namespace: "default", Name: "tls",
		},
		{
			Role: kube.RoleOwner, Direction: kube.RelationUp,
			Resource: kindResource("replicasets", "ReplicaSet"), Namespace: "default",
			Name: "web-7d9", UID: "rs-uid",
		},
	}
}

// openRelationsPopup runs the whole gesture on a pod table: dispatch the row action,
// deliver the resolver's answer, and return the model with the popup up.
func openRelationsPopup(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(rowActionMsg{
		Action:   rowActionRelations,
		Resource: kindResource("pods", "Pod"),
		Object:   kube.ObjectRef{Namespace: "default", Name: "pod-b", UID: "b"},
	})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the relations intent should issue a resolve command")
	}
	msg, ok := cmd().(relationsMsg)
	if !ok {
		t.Fatalf("resolve produced %T, want relationsMsg", cmd())
	}
	next, _ = m.Update(msg)
	return next.(Model)
}

// pickRelation resolves the popup by label, the way the picker's own SelectedMsg
// does when the reader hits enter on a row.
func pickRelation(t *testing.T, m Model, label string) Model {
	t.Helper()
	if _, ok := m.relByLabel[label]; !ok {
		t.Fatalf("relation label %q is not in the popup: %v", label, relationLabels(m))
	}
	next, _ := m.Update(picker.SelectedMsg{Kind: relationPickerKind, Value: label})
	return next.(Model)
}

func relationLabels(m Model) []string {
	out := make([]string, 0, len(m.relByLabel))
	for l := range m.relByLabel {
		out = append(out, l)
	}
	return out
}

// TestRelationsGestureOpensThePopup is the leg's headline: the gesture resolves the
// selected row's neighbours against the discovered kind set and lists them.
func TestRelationsGestureOpensThePopup(t *testing.T) {
	r := &fakeRelater{rels: podRelations()}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r)))

	if r.calls != 1 {
		t.Fatalf("Relations called %d times, want 1", r.calls)
	}
	if r.gotRef.Name != "pod-b" {
		t.Errorf("the resolve should be addressed to the selected row, got %q", r.gotRef.Name)
	}
	if len(r.gotKinds) == 0 {
		t.Error("the resolve should carry the available kind set, so an unreachable target is dropped")
	}
	if !m.relPicker.Active() {
		t.Fatal("the relations popup should be up")
	}
	if m.activePicker() == nil || m.activePicker().Kind() != relationPickerKind {
		t.Error("the relations picker should own input while it is up")
	}
	view := m.View().Content
	for _, want := range []string{"ReplicaSet/web-7d9", "Secret/tls", "owner"} {
		if !strings.Contains(view, want) {
			t.Errorf("the popup should render %q", want)
		}
	}
}

// TestRelationsGroupsByDirection pins the reading order: what made this object, then
// what it makes, then what it merely references — whatever order the graph returned
// them in.
func TestRelationsGroupsByDirection(t *testing.T) {
	items, _ := relationItems(podRelations(), "default")
	if len(items) != 3 {
		t.Fatalf("got %d rows, want 3", len(items))
	}
	wantArrows := []string{"↑", "↓", "→"}
	for i, want := range wantArrows {
		if !strings.HasPrefix(items[i].Name, want) {
			t.Errorf("row %d is %q, want the %q group", i, items[i].Name, want)
		}
	}
	if !strings.Contains(items[1].Label, "app=web") {
		t.Errorf("a set-shaped row should name its selector, got %q", items[1].Label)
	}
}

// TestRelationsLabelsQualifyAnotherNamespace: a cross-namespace hop says so before
// it is taken, and a relation in the browsed namespace stays unqualified.
func TestRelationsLabelsQualifyAnotherNamespace(t *testing.T) {
	rel := kube.Relation{
		Role: kube.RoleClaim, Direction: kube.RelationSide,
		Resource:  kindResource("persistentvolumeclaims", "PersistentVolumeClaim"),
		Namespace: "data", Name: "pgdata",
	}
	if got := relationLabel(rel, "default"); !strings.Contains(got, "data") {
		t.Errorf("label %q should name the target's namespace", got)
	}
	if got := relationLabel(rel, "data"); strings.Contains(got, separatorScope) {
		t.Errorf("label %q should stay unqualified inside the browsed namespace", got)
	}
}

// TestRelationsDedupeKeepsOneRowPerTarget: two roles naming the same object (a
// Secret mounted as a volume and used as an image-pull secret) are one row, since
// opening either lands in the same place.
func TestRelationsDedupeKeepsOneRowPerTarget(t *testing.T) {
	secret := kube.Relation{
		Role: kube.RoleSecret, Direction: kube.RelationSide,
		Resource: kindResource("secrets", "Secret"), Namespace: "default", Name: "regcred",
	}
	pull := secret
	pull.Role = kube.RolePullSecret
	items, byLabel := relationItems([]kube.Relation{secret, pull}, "default")
	if len(items) != 1 || len(byLabel) != 1 {
		t.Fatalf("got %d rows / %d entries, want 1 each", len(items), len(byLabel))
	}
}

// TestRelationOpensNamedTarget: picking a named relation switches the browse table
// to that kind and arms the object as the pending selection the watch applies when
// its row lands — the search drill-in's path, not a new one.
func TestRelationOpensNamedTarget(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	r := &fakeRelater{rels: podRelations()}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r), WithWatcher(fw)))
	watches := len(fw.res)

	m = pickRelation(t, m, "ReplicaSet/web-7d9"+separatorScope+"default")

	if m.relPicker.Active() {
		t.Error("the popup should close on a pick")
	}
	if len(fw.res) != watches+1 {
		t.Fatalf("the pick should start one new watch (%d → %d)", watches, len(fw.res))
	}
	if got := fw.res[len(fw.res)-1].GVK.Kind; got != "ReplicaSet" {
		t.Errorf("the new watch is on %q, want ReplicaSet", got)
	}
	if !m.hasSearchTarget || m.searchTarget.Name != "web-7d9" || m.searchTarget.UID != "rs-uid" {
		t.Errorf("the owner should be armed as the pending selection, got %+v", m.searchTarget)
	}
}

// TestRelationOpensSetTargetAsChildScope: a set-shaped relation opens through the
// same scoped watch the `P` drill-down uses, with the popup's source object as the
// owner — so the scope label names it and nav.back returns to it.
func TestRelationOpensSetTargetAsChildScope(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	r := &fakeRelater{rels: podRelations()}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r), WithWatcher(fw)))

	m = pickRelation(t, m, "Pod (app=web)")

	if !m.hasChildScope {
		t.Fatal("a set-shaped relation should open as a child scope")
	}
	if got := fw.opts[len(fw.opts)-1].LabelSelector; got != "app=web" {
		t.Errorf("the watch ran under selector %q, want app=web", got)
	}
	if m.childOwnerRef.Name != "pod-b" {
		t.Errorf("the scope's owner should be the object the popup was opened on, got %q", m.childOwnerRef.Name)
	}
}

// TestRelationToAnotherNamespaceRescopes: a target outside the browsed namespace
// re-scopes the shell first, otherwise the pending selection waits for a row the
// watch is never going to list.
func TestRelationToAnotherNamespaceRescopes(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	claim := kube.Resource{GVR: gvrResource("persistentvolumeclaims").GVR, Namespaced: true}
	claim.GVK.Kind = "PersistentVolumeClaim"
	r := &fakeRelater{rels: []kube.Relation{{
		Role: kube.RoleClaim, Direction: kube.RelationSide,
		Resource: claim, Namespace: "data", Name: "pgdata",
	}}}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r), WithWatcher(fw), WithNamespace("default")))

	m = pickRelation(t, m, "PersistentVolumeClaim/pgdata"+separatorScope+"data")

	if m.namespace != "data" {
		t.Errorf("the shell should re-scope to the target's namespace, got %q", m.namespace)
	}
	if got := fw.ns[len(fw.ns)-1]; got != "data" {
		t.Errorf("the new watch is scoped to %q, want data", got)
	}
}

// TestRelationsWithNoNeighboursSaysSo: an empty set is a true answer, not a failure,
// so it is a notice rather than an empty modal the reader has to dismiss.
func TestRelationsWithNoNeighboursSaysSo(t *testing.T) {
	r := &fakeRelater{}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r)))

	if m.relPicker.Active() {
		t.Error("an empty relation set should not open a popup")
	}
	if !strings.Contains(m.View().Content, "No related resources") {
		t.Error("the shell should say the object has no neighbours")
	}
}

// TestRelationsErrorLeavesTheReaderPut: a failed resolve is one toast, with the
// browse table untouched (principle 3).
func TestRelationsErrorLeavesTheReaderPut(t *testing.T) {
	r := &fakeRelater{err: errors.New("forbidden")}
	m := openRelationsPopup(t, openPodTable(t, "Pod", WithRelater(r)))

	if m.relPicker.Active() {
		t.Error("a failed resolve should open no popup")
	}
	if !strings.Contains(m.View().Content, "forbidden") {
		t.Error("the failure should be surfaced")
	}
}

// TestRelationsWithoutASeamIsInert: the default model has no Relater, so the gesture
// is a no-op rather than a half-open surface.
func TestRelationsWithoutASeamIsInert(t *testing.T) {
	m := openPodTable(t, "Pod")
	next, cmd := m.Update(rowActionMsg{
		Action:   rowActionRelations,
		Resource: kindResource("pods", "Pod"),
		Object:   kube.ObjectRef{Name: "pod-b"},
	})
	if cmd != nil {
		t.Error("a relations-inert model should issue no resolve")
	}
	if next.(Model).relPicker.Active() {
		t.Error("a relations-inert model should open no popup")
	}
}

// TestRelationsKeySequenceDispatchesTheAction: the `gr` gesture resolves through the
// keymap to the row action, so the key and the actions-menu entry funnel through one
// intent (D11 — no view matches a raw key).
func TestRelationsKeySequenceDispatchesTheAction(t *testing.T) {
	m := openPodTable(t, "Pod", WithRelater(&fakeRelater{rels: podRelations()}))
	m, cmd := press(t, m, tea.Key{Code: 'g', Text: "g"})
	if cmd != nil {
		if _, ok := cmd().(rowActionMsg); ok {
			t.Fatal("`g` alone should not dispatch: it is the sequence's prefix")
		}
	}
	_, cmd = press(t, m, tea.Key{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("`gr` should dispatch the relations intent")
	}
	msg, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("`gr` produced %T, want rowActionMsg", cmd())
	}
	if msg.Action != rowActionRelations {
		t.Errorf("`gr` dispatched %q, want %q", msg.Action, rowActionRelations)
	}
}
