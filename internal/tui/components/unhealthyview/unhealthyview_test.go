package unhealthyview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

func newUnhealthy() Model {
	m := New(styles.Default())
	m.SetSize(60, 12)
	m.Show()
	return m
}

// hit builds a ScanHit of the given kind for ns/name ("" ns = cluster-scoped)
// whose STATUS column carries the given status. Columns are what a surface needs
// to render the reason (the offending cell) from the hit alone (STORY-06g-2b).
func hit(kind, ns, name, status string) kube.ScanHit {
	return kube.ScanHit{
		Resource: kube.Resource{
			GVK:        schema.GroupVersionKind{Kind: kind},
			Namespaced: ns != "",
		},
		Columns: []kube.Column{
			{Name: "NAME"},
			{Name: "STATUS"},
		},
		Row: kube.Row{
			Cells:  []any{name, status},
			Object: kube.ObjectRef{Namespace: ns, Name: name, UID: kind + "/" + ns + "/" + name},
		},
	}
}

// drain runs cmd (and a tea.Batch's children) and returns the messages produced.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestHiddenOrUnsizedIsEmpty(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(60, 12) // sized but hidden
	if v := m.View(); v != "" {
		t.Errorf("hidden View() = %q; want empty", v)
	}
	m2 := New(styles.Default())
	m2.Show() // shown but unsized
	if v := m2.View(); v != "" {
		t.Errorf("unsized View() = %q; want empty", v)
	}
}

func TestInactiveViewIgnoresInput(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(60, 12)
	m.AppendHit(hit("Pod", "default", "web-2", "CrashLoopBackOff"))
	if _, cmd := m.Update(keymap.ActionDrillIn); cmd != nil {
		t.Error("an inactive view should not emit on nav.drillIn")
	}
	if _, cmd := m.Update(keymap.ActionDown); cmd != nil {
		t.Error("an inactive view should not navigate")
	}
}

func TestAppendHitGrowsListAndDrillInSelects(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("Pod", "web", "web-2", "CrashLoopBackOff"))
	m.AppendHit(hit("PersistentVolumeClaim", "web", "stuck", "Pending"))

	got, ok := m.Selected()
	if !ok {
		t.Fatal("a non-empty list should have a selected hit")
	}
	if got.Row.Object.Name != "web-2" {
		t.Errorf("selected = %q, want web-2 (the first hit)", got.Row.Object.Name)
	}
	if got.Resource.GVK.Kind != "Pod" {
		t.Errorf("selected kind = %q, want Pod", got.Resource.GVK.Kind)
	}

	// Moving the cursor selects the second hit; drill-in emits SelectedMsg for it.
	m, _ = m.Update(keymap.ActionDown)
	var msg tea.Msg
	m, cmd := m.Update(keymap.ActionDrillIn)
	for _, out := range drain(cmd) {
		msg = out
	}
	sel, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("drill-in should emit SelectedMsg, got %T", msg)
	}
	if sel.Kind != "unhealthy" {
		t.Errorf("SelectedMsg.Kind = %q, want unhealthy", sel.Kind)
	}
	if sel.Hit.Resource.GVK.Kind != "PersistentVolumeClaim" || sel.Hit.Row.Object.Name != "stuck" {
		t.Errorf("SelectedMsg.Hit = %+v, want the PVC/stuck hit", sel.Hit)
	}
}

func TestBackEmitsClosedMsg(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("Pod", "web", "web-2", "CrashLoopBackOff"))

	var msg tea.Msg
	m, cmd := m.Update(keymap.ActionBack)
	for _, out := range drain(cmd) {
		msg = out
	}
	if _, ok := msg.(ClosedMsg); !ok {
		t.Fatalf("nav.back should emit ClosedMsg, got %T", msg)
	}
}

func TestNavigationActionsMoveTheCursor(t *testing.T) {
	m := newUnhealthy()
	for i := 0; i < 5; i++ {
		m.AppendHit(hit("Pod", "web", "pod-"+string(rune('a'+i)), "CrashLoopBackOff"))
	}
	m, _ = m.Update(keymap.ActionDown)
	m, _ = m.Update(keymap.ActionDown)
	if got, _ := m.Selected(); got.Row.Object.Name != "pod-c" {
		t.Errorf("after 2 downs selected = %q, want pod-c", got.Row.Object.Name)
	}
	m, _ = m.Update(keymap.ActionTop)
	if got, _ := m.Selected(); got.Row.Object.Name != "pod-a" {
		t.Errorf("after gg selected = %q, want pod-a", got.Row.Object.Name)
	}
	m, _ = m.Update(keymap.ActionBottom)
	if got, _ := m.Selected(); got.Row.Object.Name != "pod-e" {
		t.Errorf("after G selected = %q, want pod-e", got.Row.Object.Name)
	}
	m, _ = m.Update(keymap.ActionUp)
	if got, _ := m.Selected(); got.Row.Object.Name != "pod-d" {
		t.Errorf("after k selected = %q, want pod-d", got.Row.Object.Name)
	}
}

func TestResetClearsHitsAndProgress(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("Pod", "web", "web-2", "CrashLoopBackOff"))
	m.SetSearching(true)
	m.StartProgress(3)
	m.MarkKindDone()
	m.MarkKindDone()
	m.SetCapped(true)

	m.Reset()
	if m.Len() != 0 {
		t.Errorf("Reset() left %d hits, want 0", m.Len())
	}
	if m.Searching() {
		t.Error("Reset() should clear the in-flight indicator")
	}
	if m.Capped() {
		t.Error("Reset() should clear the cap flag")
	}
	if done, total := m.Progress(); done != 0 || total != 0 {
		t.Errorf("Reset() progress = %d/%d, want 0/0", done, total)
	}
}

func TestProgressAndCap(t *testing.T) {
	m := newUnhealthy()
	m.SetScope("web")
	m.SetSearching(true)
	m.StartProgress(3)

	if v := m.View(); !strings.Contains(v, "scanning 0/3 kinds") {
		t.Errorf("progress header missing early count:\n%v", v)
	}
	m.MarkKindDone()
	m.MarkKindDone()
	if v := m.View(); !strings.Contains(v, "scanning 2/3 kinds") {
		t.Errorf("progress header missing mid count:\n%v", v)
	}
	m.MarkKindDone()
	m.SetSearching(false)
	if v := m.View(); !strings.Contains(v, "no unhealthy resources") {
		t.Errorf("an empty, finished sweep should say so:\n%v", v)
	}
	m.AppendHit(hit("Pod", "web", "web-2", "CrashLoopBackOff"))
	m.SetCapped(true)
	if v := m.View(); !strings.Contains(v, "first 1 matches") {
		t.Errorf("cap state missing from header:\n%v", v)
	}
}

// TestHitRowRendersKindPathAndReason pins the row text a hit produces: kind
// padded, object path, then the offending cell — the reason a surface shows
// without re-listing (D276).
func TestHitRowRendersKindPathAndReason(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("Pod", "web", "web-2", "CrashLoopBackOff"))

	rows := hitItems(m.hits)
	if len(rows) != 1 {
		t.Fatalf("hitItems = %d rows, want 1", len(rows))
	}
	want := "Pod  web/web-2  CrashLoopBackOff"
	if rows[0].label != want {
		t.Errorf("row label = %q, want %q", rows[0].label, want)
	}
	if len(rows[0].cells) != 1 || rows[0].cells[0].error != true {
		t.Errorf("row cells = %+v, want one error-classified span", rows[0].cells)
	}
	if got := rows[0].cells[0].start; got != len("Pod  web/web-2  ") {
		t.Errorf("reason span starts at %d, want %d (right after the path)", got, len("Pod  web/web-2  "))
	}
}

// TestHitRowReasonsAreRoleTagged pins that a warning cell renders as a warning
// span, not an error one — the reason a surface shows carries the same role the
// browse table paints the cell with (M4-06).
func TestHitRowReasonsAreRoleTagged(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("PersistentVolumeClaim", "web", "stuck", "Pending"))

	rows := hitItems(m.hits)
	if len(rows[0].cells) != 1 || rows[0].cells[0].error != false {
		t.Errorf("a Pending cell should be a warning span, got %+v", rows[0].cells)
	}
}

// TestClusterScopedHitHasBareName pins the path rendering for a cluster-scoped
// object: no namespace to prefix, so the row is kind · name.
func TestClusterScopedHitHasBareName(t *testing.T) {
	m := newUnhealthy()
	m.AppendHit(hit("Node", "", "node-1", "NotReady"))

	rows := hitItems(m.hits)
	if want := "Node  node-1  NotReady"; rows[0].label != want {
		t.Errorf("row label = %q, want %q", rows[0].label, want)
	}
}

// TestMultipleOffendingCells pins the joined-reason rendering: a row with two
// broken cells (a not-ready pod also crash-looping) shows both, comma-joined,
// with one span each.
func TestMultipleOffendingCells(t *testing.T) {
	m := newUnhealthy()
	h := kube.ScanHit{
		Resource: kube.Resource{GVK: schema.GroupVersionKind{Kind: "Pod"}, Namespaced: true},
		Columns: []kube.Column{
			{Name: "NAME"},
			{Name: "READY"},
			{Name: "STATUS"},
		},
		Row: kube.Row{
			Cells:  []any{"web-2", "0/1", "CrashLoopBackOff"},
			Object: kube.ObjectRef{Namespace: "web", Name: "web-2"},
		},
	}
	m.AppendHit(h)

	rows := hitItems(m.hits)
	if want := "Pod  web/web-2  0/1, CrashLoopBackOff"; rows[0].label != want {
		t.Errorf("row label = %q, want %q", rows[0].label, want)
	}
	if len(rows[0].cells) != 2 {
		t.Fatalf("cells = %d, want 2 (0/1 and CrashLoopBackOff)", len(rows[0].cells))
	}
	if rows[0].cells[0].error {
		t.Error("0/1 is a warning, not an error")
	}
	if !rows[0].cells[1].error {
		t.Error("CrashLoopBackOff is an error, not a warning")
	}
}

// TestPaintLabelColoring pins the delegate's rendering: the reason cells come out
// colored through their own styles (Warn/Error), so a reader sees the offending
// cell painted with the same hue the browse table uses — the surface's "the red
// things find the operator" (STORY-06g-2).
func TestPaintLabelColoring(t *testing.T) {
	s := styles.Default()
	row := hitItems([]kube.ScanHit{hit("Pod", "web", "web-2", "CrashLoopBackOff")})[0]
	got := paintLabel(row.label, row.cells, 60, s.App, s.Warn, s.Error)
	if !strings.Contains(got, "CrashLoopBackOff") {
		t.Errorf("painted row lost the reason:\n%q", got)
	}
	if got == row.label {
		t.Error("painted row should not equal the plain label (the reason must be styled)")
	}
}
