package menu

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newTestModel() Model {
	return New(styles.Default())
}

func TestSeedNonEmptyFirstSelected(t *testing.T) {
	m := newTestModel()
	if len(m.items) == 0 {
		t.Fatal("seed menu is empty")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	sel, ok := m.Selected()
	if !ok {
		t.Fatal("Selected() returned ok=false on a seeded menu")
	}
	if sel.Resource.GVR.Resource != "namespaces" {
		t.Fatalf("first item = %q, want namespaces", sel.Resource.GVR.Resource)
	}
	// Every seed item must carry a title and a listable GVR.
	for _, it := range m.items {
		if it.Title == "" {
			t.Errorf("item %v has empty title", it.Resource.GVR)
		}
		if it.Resource.GVR.Resource == "" || it.Resource.GVR.Version == "" {
			t.Errorf("item %q has an incomplete GVR: %+v", it.Title, it.Resource.GVR)
		}
		if !it.Available {
			t.Errorf("seed item %q should be available", it.Title)
		}
	}
}

func TestNavigationClampsAndJumps(t *testing.T) {
	m := newTestModel()
	last := len(m.items) - 1

	// Up at the top stays at 0.
	m, _ = m.Update(keymap.ActionUp)
	if m.cursor != 0 {
		t.Fatalf("up at top: cursor = %d, want 0", m.cursor)
	}
	// Down moves.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != 1 {
		t.Fatalf("down: cursor = %d, want 1", m.cursor)
	}
	// Bottom jumps to last.
	m, _ = m.Update(keymap.ActionBottom)
	if m.cursor != last {
		t.Fatalf("bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Down at the bottom stays at last.
	m, _ = m.Update(keymap.ActionDown)
	if m.cursor != last {
		t.Fatalf("down at bottom: cursor = %d, want %d", m.cursor, last)
	}
	// Top jumps back to 0.
	m, _ = m.Update(keymap.ActionTop)
	if m.cursor != 0 {
		t.Fatalf("top: cursor = %d, want 0", m.cursor)
	}
}

func TestUnhandledActionIsIgnored(t *testing.T) {
	m := newTestModel()
	m, cmd := m.Update(keymap.ActionFilter)
	if cmd != nil {
		t.Error("unhandled action returned a non-nil command")
	}
	if m.cursor != 0 {
		t.Errorf("unhandled action moved the cursor to %d", m.cursor)
	}
}

func TestDrillInEmitsResourceSelected(t *testing.T) {
	m := newTestModel()
	// Move to the "pods" item and drill in.
	var target int
	for i, it := range m.items {
		if it.Resource.GVR.Resource == "pods" {
			target = i
			break
		}
	}
	m, _ = m.Update(keymap.ActionBottom)
	m, _ = m.Update(keymap.ActionTop)
	for i := 0; i < target; i++ {
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.cursor != target {
		t.Fatalf("cursor = %d, want %d (pods)", m.cursor, target)
	}

	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd == nil {
		t.Fatal("drillIn returned no command")
	}
	msg := cmd()
	sel, ok := msg.(ResourceSelectedMsg)
	if !ok {
		t.Fatalf("drillIn emitted %T, want ResourceSelectedMsg", msg)
	}
	if sel.Resource.GVR.Resource != "pods" {
		t.Fatalf("selected resource = %q, want pods", sel.Resource.GVR.Resource)
	}
}

func TestDrillInOnUnavailableEmitsNothing(t *testing.T) {
	m := newTestModel()
	m.items[m.cursor].Available = false
	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd != nil {
		t.Fatal("drilling into an unavailable item emitted a command")
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := newTestModel()
	// A short pane: 4 total rows → 2 item rows visible (border eats 2).
	m.SetSize(20, 4)
	if got := m.innerHeight(); got != 2 {
		t.Fatalf("innerHeight = %d, want 2", got)
	}
	// Jump to the bottom: the offset must scroll so the cursor is in view.
	m, _ = m.Update(keymap.ActionBottom)
	last := len(m.items) - 1
	if m.cursor < m.offset || m.cursor >= m.offset+m.innerHeight() {
		t.Fatalf("cursor %d not visible in window [%d,%d)", m.cursor, m.offset, m.offset+m.innerHeight())
	}
	if m.offset != last-m.innerHeight()+1 {
		t.Fatalf("offset = %d, want %d", m.offset, last-m.innerHeight()+1)
	}
	// Back to top scrolls the window back up.
	m, _ = m.Update(keymap.ActionTop)
	if m.offset != 0 {
		t.Fatalf("offset after top = %d, want 0", m.offset)
	}
}

func TestViewEmptyUntilSized(t *testing.T) {
	m := newTestModel()
	if v := m.View(); v != "" {
		t.Fatalf("View before sizing = %q, want empty", v)
	}
	m.SetSize(24, 12)
	if v := m.View(); v == "" {
		t.Fatal("View after sizing is empty")
	}
}

func TestViewRendersTitlesAndFocusChangesFrame(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 25) // tall enough to show every seed item
	blurred := m.View()
	if !strings.Contains(blurred, "Namespace") || !strings.Contains(blurred, "Pod") {
		t.Fatal("view is missing seed titles")
	}
	m.Focus()
	focused := m.View()
	if focused == blurred {
		t.Fatal("focused view is identical to blurred view (border should change)")
	}
}

// findItem returns the index of the seed item with the given resource name.
func findItem(m Model, resource string) int {
	for i, it := range m.items {
		if it.Resource.GVR.Resource == resource {
			return i
		}
	}
	return -1
}

func TestReconcileFillsTwinAndAppendsExtras(t *testing.T) {
	m := newTestModel()
	seedLen := len(m.items)

	// A discovered twin for the seed's "pods" (fills verbs/short-names) and a CRD
	// the seed does not carry.
	pods := kube.Resource{
		GVK:        schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Namespaced: true,
		Verbs:      []string{"get", "list", "watch"},
		ShortNames: []string{"po"},
	}
	crd := kube.Resource{
		GVK:        schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR:        schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
		Namespaced: true,
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{pods, crd}})

	// The seed grew by exactly the one unknown resource, appended after the seed.
	if len(m.items) != seedLen+1 {
		t.Fatalf("item count = %d, want %d", len(m.items), seedLen+1)
	}
	last := m.items[len(m.items)-1]
	if last.Resource.GVR.Resource != "widgets" || last.Title != "Widget" || !last.Available {
		t.Fatalf("appended item = %+v, want available Widget/widgets", last)
	}

	// The pods twin metadata was merged in, keeping the seed position.
	pi := findItem(m, "pods")
	if pi < 0 {
		t.Fatal("pods item missing after reconcile")
	}
	if got := m.items[pi].Resource.ShortNames; len(got) != 1 || got[0] != "po" {
		t.Fatalf("pods short names = %v, want [po]", got)
	}
	if !m.items[pi].Available {
		t.Error("pods should stay available after reconcile")
	}
}

func TestReconcileMarksFailedGroupUnavailable(t *testing.T) {
	m := newTestModel()
	// networking.k8s.io failed discovery; core loaded fine (a pods twin).
	pods := kube.Resource{
		GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	m.Reconcile(kube.DiscoveryResult{
		Resources: []kube.Resource{pods},
		Failed:    []kube.FailedGroup{{GroupVersion: "networking.k8s.io/v1"}},
	})

	ii := findItem(m, "ingresses")
	if ii < 0 {
		t.Fatal("ingresses item missing")
	}
	if m.items[ii].Available {
		t.Error("ingresses (failed group, no twin) should be unavailable")
	}
	// A seed item with neither a twin nor a failed group is left untouched.
	if si := findItem(m, "services"); si < 0 || !m.items[si].Available {
		t.Error("services (not failed, no twin) should stay available")
	}
}

func TestReconcilePreservesSelectionAndScroll(t *testing.T) {
	m := newTestModel()
	m.SetSize(20, 6) // small window so scroll matters
	// Select "pods" and note the offset.
	target := findItem(m, "pods")
	for i := 0; i < target; i++ {
		m, _ = m.Update(keymap.ActionDown)
	}
	if m.cursor != target {
		t.Fatalf("setup: cursor = %d, want %d", m.cursor, target)
	}
	offBefore := m.offset

	// Reconcile appends a CRD; selection must still point at pods.
	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{crd}})

	if sel, _ := m.Selected(); sel.Resource.GVR.Resource != "pods" {
		t.Fatalf("selection moved to %q, want pods", sel.Resource.GVR.Resource)
	}
	if m.offset != offBefore {
		t.Fatalf("scroll offset changed from %d to %d", offBefore, m.offset)
	}
}

func TestReconcileTotalFailureLeavesSeedUntouched(t *testing.T) {
	m := newTestModel()
	before := append([]Item(nil), m.items...)
	m.Reconcile(kube.DiscoveryResult{Err: context.DeadlineExceeded})
	if len(m.items) != len(before) {
		t.Fatalf("item count changed on total failure: %d → %d", len(before), len(m.items))
	}
	for i := range before {
		if !m.items[i].Available {
			t.Errorf("item %q marked unavailable on total failure", m.items[i].Title)
		}
	}
}

// Ensure the emitted command type satisfies tea.Cmd (compile-time contract).
var _ tea.Cmd = func() tea.Msg { return ResourceSelectedMsg{} }
