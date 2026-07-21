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
	// Every seed item must carry a title and a listable GVR — except the
	// non-resource namespace seam, which has a title but no GVR.
	for _, it := range m.items {
		if it.Title == "" {
			t.Errorf("item %v has empty title", it.Resource.GVR)
		}
		if it.Kind != ItemResource {
			continue
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
	// Jump to the bottom: the offset (a display-row offset that counts section
	// headers) must scroll so the cursor's row is in view, pinned to the last page.
	m, _ = m.Update(keymap.ActionBottom)
	cr := m.cursorRow()
	if cr < m.offset || cr >= m.offset+m.innerHeight() {
		t.Fatalf("cursor row %d not visible in window [%d,%d)", cr, m.offset, m.offset+m.innerHeight())
	}
	if want := len(m.rows()) - m.innerHeight(); m.offset != want {
		t.Fatalf("offset = %d, want %d (last page)", m.offset, want)
	}
	// Back to top scrolls the window back up (cursor's Cluster header at row 0).
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

func TestSeedItemsAreSectionGroupedContiguously(t *testing.T) {
	m := newTestModel()
	// Every seed item carries a section, and items of the same section are
	// contiguous — the invariant rows() relies on to emit one header per section.
	seen := map[string]bool{}
	prev := ""
	for _, it := range m.items {
		if it.Kind != ItemResource {
			continue // the namespace seam is a non-resource row with no section.
		}
		if it.Section == "" {
			t.Fatalf("seed item %q has no section", it.Title)
		}
		if it.Section != prev {
			if seen[it.Section] {
				t.Fatalf("section %q is not contiguous (reappears at %q)", it.Section, it.Title)
			}
			seen[it.Section] = true
			prev = it.Section
		}
	}
	// The first section is Cluster (matches Selected() == namespaces).
	if m.items[0].Section != sectionCluster {
		t.Fatalf("first section = %q, want %q", m.items[0].Section, sectionCluster)
	}
}

func TestRowsInsertOneHeaderPerSection(t *testing.T) {
	m := newTestModel()
	rows := m.rows()

	// One header per distinct section, and each header immediately precedes its
	// section's items. The namespace seam (non-resource, no section) contributes no
	// header.
	sections := map[string]bool{}
	for _, it := range m.items {
		if it.Kind != ItemResource {
			continue
		}
		sections[it.Section] = true
	}
	headers := 0
	for i, r := range rows {
		if !r.header {
			continue
		}
		headers++
		// The next row must be an item of this section.
		if i+1 >= len(rows) || rows[i+1].header {
			t.Fatalf("header %q not followed by an item", r.title)
		}
		if got := m.items[rows[i+1].itemIdx].Section; got != r.title {
			t.Fatalf("header %q precedes an item of section %q", r.title, got)
		}
	}
	if headers != len(sections) {
		t.Fatalf("rendered %d headers, want %d (one per section)", headers, len(sections))
	}
	if len(rows) != len(m.items)+headers {
		t.Fatalf("rows = %d, want items(%d)+headers(%d)", len(rows), len(m.items), headers)
	}
}

func TestNavigationSkipsHeaders(t *testing.T) {
	m := newTestModel()
	// Walking down from the top must visit every item in order and never land on a
	// header row (the cursor only indexes items).
	for i := 0; i < len(m.items); i++ {
		if m.cursor != i {
			t.Fatalf("step %d: cursor = %d", i, m.cursor)
		}
		if _, ok := m.Selected(); !ok {
			t.Fatalf("step %d: no selection", i)
		}
		m, _ = m.Update(keymap.ActionDown)
	}
}

func TestViewRendersSectionHeaders(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40) // tall enough for every row
	v := m.View()
	for _, sec := range []string{sectionCluster, sectionWorkloads, sectionConfig, sectionNetwork, sectionStorage, sectionAccess} {
		if !strings.Contains(v, sec) {
			t.Errorf("view missing section header %q", sec)
		}
	}
}

func TestReconcileAppendsCRDIntoCustomResourcesSection(t *testing.T) {
	m := newTestModel()
	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{crd}})

	last := m.items[len(m.items)-1]
	if last.Section != sectionCustom {
		t.Fatalf("appended CRD section = %q, want %q", last.Section, sectionCustom)
	}
	// A Custom Resources header now renders, once.
	headers := 0
	for _, r := range m.rows() {
		if r.header && r.title == sectionCustom {
			headers++
		}
	}
	if headers != 1 {
		t.Fatalf("Custom Resources header count = %d, want 1", headers)
	}
}

// namespaceSeamIndex returns the index of the single namespace-seam row, or -1.
func namespaceSeamIndex(m Model) int {
	idx := -1
	for i, it := range m.items {
		if it.Kind == ItemNamespace {
			if idx != -1 {
				return -2 // more than one seam — a bug the caller asserts against
			}
			idx = i
		}
	}
	return idx
}

func TestSeedHasNamespaceSeamBetweenClusterAndNamespaced(t *testing.T) {
	m := newTestModel()
	seam := namespaceSeamIndex(m)
	if seam < 0 {
		t.Fatalf("namespace seam index = %d, want exactly one seam row", seam)
	}
	// The row immediately above the seam is the last cluster-scoped item; the row
	// immediately below is the first namespaced item (the boundary the seam marks).
	if seam == 0 || seam == len(m.items)-1 {
		t.Fatalf("seam at %d sits at an edge, not between sections", seam)
	}
	if above := m.items[seam-1]; above.Section != sectionCluster {
		t.Fatalf("item above seam is section %q, want %q", above.Section, sectionCluster)
	}
	below := m.items[seam+1]
	if below.Section == sectionCluster {
		t.Fatalf("item below seam is still cluster-scoped (%q); seam not at the boundary", below.Title)
	}
	if !below.Resource.Namespaced {
		t.Fatalf("item below seam (%q) is not namespaced", below.Title)
	}
	if !m.items[seam].Available {
		t.Fatal("the namespace seam should be selectable (Available)")
	}
}

func TestDrillInOnSeamEmitsNamespaceRequested(t *testing.T) {
	m := newTestModel()
	seam := namespaceSeamIndex(m)
	if seam < 0 {
		t.Fatalf("no namespace seam (index %d)", seam)
	}
	m.cursor = seam
	_, cmd := m.Update(keymap.ActionDrillIn)
	if cmd == nil {
		t.Fatal("drilling into the seam returned no command")
	}
	if _, ok := cmd().(NamespaceRequestedMsg); !ok {
		t.Fatalf("seam drill-in emitted %T, want NamespaceRequestedMsg", cmd())
	}
}

func TestNamespaceSeamRendersScope(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40) // tall enough to show every row
	if v := m.View(); !strings.Contains(v, "all namespaces") {
		t.Fatal("unscoped seam should render \"all namespaces\"")
	}
	m.SetNamespace("kube-system")
	if v := m.View(); !strings.Contains(v, "kube-system") {
		t.Fatal("scoped seam should render the namespace name")
	}
}

func TestReconcilePreservesSeamAndItsSelection(t *testing.T) {
	m := newTestModel()
	seam := namespaceSeamIndex(m)
	m.cursor = seam

	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{crd}})

	// Exactly one seam still present and still selected (its index is unchanged —
	// nothing is inserted before it — but resolve by kind to be robust).
	if got := namespaceSeamIndex(m); got != seam {
		t.Fatalf("seam moved/duplicated: index %d, want %d", got, seam)
	}
	if sel, _ := m.Selected(); sel.Kind != ItemNamespace {
		t.Fatalf("selection after reconcile is kind %v, want the namespace seam", sel.Kind)
	}
	if !m.items[seam].Available {
		t.Fatal("reconcile marked the namespace seam unavailable")
	}
}

// Ensure the emitted command types satisfy tea.Cmd (compile-time contract).
var (
	_ tea.Cmd = func() tea.Msg { return ResourceSelectedMsg{} }
	_ tea.Cmd = func() tea.Msg { return NamespaceRequestedMsg{} }
)
