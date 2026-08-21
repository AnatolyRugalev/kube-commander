package menu

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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

// findFull returns the index of the item with the given resource name in the
// authoritative list — the one that carries the custom resources the pane holds
// back (D288).
func findFull(m Model, resource string) int {
	for i, it := range m.full {
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

	// The seed grew by exactly the one unknown resource, appended after the seed. It
	// lands in the authoritative list; the pane holds an unpinned CRD back (D288), so
	// the displayed list is unchanged.
	if len(m.full) != seedLen+1 {
		t.Fatalf("item count = %d, want %d", len(m.full), seedLen+1)
	}
	if len(m.items) != seedLen {
		t.Fatalf("displayed count = %d, want the seed alone (%d)", len(m.items), seedLen)
	}
	last := m.full[len(m.full)-1]
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
		Failed:    []kube.FailedGroup{{Group: "networking.k8s.io", Version: "v1"}},
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

// pagedMenu is a menu sized so its content overflows the pane, which is the only
// state in which paging differs from a plain move. It fails the test rather than
// silently passing if the seed ever shrinks below one screenful.
func pagedMenu(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	m.SetSize(20, 12) // 10 visible display rows (border eats 2)
	if got := m.innerHeight(); got != 10 {
		t.Fatalf("innerHeight = %d, want 10", got)
	}
	if rows := len(m.rows()); rows <= m.innerHeight() {
		t.Fatalf("seed menu has %d rows, which fits a %d-row pane — paging is untestable", rows, m.innerHeight())
	}
	return m
}

func TestPagingStepsByVisibleRowsAndLandsOnItems(t *testing.T) {
	m := pagedMenu(t)
	start := m.cursorRow()

	m, _ = m.Update(keymap.ActionPageDown)
	full := m.cursor
	if m.cursorRow() <= start {
		t.Fatalf("page down: cursor row %d did not advance from %d", m.cursorRow(), start)
	}
	// A page never moves past more items than the pane could show — headers only
	// ever eat into the step, never extend it.
	if full > m.innerHeight() {
		t.Fatalf("page down moved %d items, more than the %d visible rows", full, m.innerHeight())
	}
	// The cursor indexes items, so it can never come to rest on a header row.
	if r := m.cursorRow(); m.rows()[r].header {
		t.Fatalf("page down landed the cursor on header row %d", r)
	}

	h := pagedMenu(t)
	h, _ = h.Update(keymap.ActionHalfPageDown)
	if h.cursor <= 0 || h.cursor >= full {
		t.Fatalf("half page down: cursor = %d, want strictly between 0 and the full-page %d", h.cursor, full)
	}
}

func TestPagingClampsAtBothEnds(t *testing.T) {
	m := pagedMenu(t)
	last := len(m.items) - 1

	// Enough page-downs to cross the whole list, then one more that must not move.
	for i := 0; i < len(m.items)+1; i++ {
		m, _ = m.Update(keymap.ActionPageDown)
	}
	if m.cursor != last {
		t.Fatalf("page down to the end: cursor = %d, want %d", m.cursor, last)
	}
	m, _ = m.Update(keymap.ActionPageDown)
	if m.cursor != last {
		t.Fatalf("page down at the end moved to %d, want %d", m.cursor, last)
	}

	for i := 0; i < len(m.items)+1; i++ {
		m, _ = m.Update(keymap.ActionPageUp)
	}
	if m.cursor != 0 {
		t.Fatalf("page up to the top: cursor = %d, want 0", m.cursor)
	}
	m, _ = m.Update(keymap.ActionHalfPageUp)
	if m.cursor != 0 {
		t.Fatalf("half page up at the top moved to %d, want 0", m.cursor)
	}
	if m.offset != 0 {
		t.Fatalf("offset after paging back to the top = %d, want 0", m.offset)
	}
}

func TestPagingScrollsTheWindow(t *testing.T) {
	m := pagedMenu(t)
	m, _ = m.Update(keymap.ActionPageDown)
	if m.offset == 0 {
		t.Fatal("page down did not scroll the window")
	}
	if cr := m.cursorRow(); cr < m.offset || cr >= m.offset+m.innerHeight() {
		t.Fatalf("cursor row %d not visible in window [%d,%d)", cr, m.offset, m.offset+m.innerHeight())
	}
}

func TestPagingStillMovesInAnUnsizedPane(t *testing.T) {
	// Before the first WindowSizeMsg innerHeight is 0; a page must still advance by
	// a row rather than doing nothing at all.
	m := newTestModel()
	m, _ = m.Update(keymap.ActionHalfPageDown)
	if m.cursor != 1 {
		t.Fatalf("half page down on an unsized menu: cursor = %d, want 1", m.cursor)
	}
	m, _ = m.Update(keymap.ActionPageUp)
	if m.cursor != 0 {
		t.Fatalf("page up on an unsized menu: cursor = %d, want 0", m.cursor)
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

	last := m.full[len(m.full)-1]
	if last.Section != sectionCustom {
		t.Fatalf("appended CRD section = %q, want %q", last.Section, sectionCustom)
	}
	// Pinning it is what puts it in the pane (D288); the section header follows.
	m.AddPinned([]config.MenuResource{{Group: "example.com", Version: "v1", Resource: "widgets", Kind: "Widget"}})
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
	if v := m.View(); !strings.Contains(v, "▾ (all)") {
		t.Fatal("unscoped seam should render the dropdown \"▾ (all)\"")
	}
	m.SetNamespace("kube-system")
	if v := m.View(); !strings.Contains(v, "▾ kube-system") {
		t.Fatal("scoped seam should render the dropdown \"▾ <ns>\"")
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

func TestActiveResourceMarkedDistinctFromCursor(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40) // tall enough to show every row
	// Nothing opened yet: no active marker anywhere.
	if strings.Contains(lipglossStrip(m.View()), activeMarker) {
		t.Fatal("active marker rendered before any resource is opened")
	}
	// Open Pods while the nav cursor stays at the top (namespaces): the Pods row must
	// render marked-active even though the cursor sits on a different item — the two
	// states are independent (dogfood-05).
	pods := m.items[findItem(m, "pods")].Resource
	m.SetActive(pods)
	if m.cursor != 0 {
		t.Fatalf("SetActive moved the cursor to %d; it must not touch the cursor", m.cursor)
	}
	v := lipglossStrip(m.View())
	if !strings.Contains(v, activeMarker+"Pod") {
		t.Fatalf("opened Pods row not marked active (want %q) in:\n%s", activeMarker+"Pod", v)
	}
	// Exactly one resource is marked active (the seam's own "▾" is a different glyph).
	if got := strings.Count(v, activeMarker); got != 1 {
		t.Fatalf("active marker appears %d times, want exactly 1", got)
	}
}

func TestActiveMarkerSurvivesCursorMovement(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40)
	pods := m.items[findItem(m, "pods")].Resource
	m.SetActive(pods)
	// Walk the cursor down onto and back off the opened row; the marker never moves.
	for i := 0; i < 6; i++ {
		m, _ = m.Update(keymap.ActionDown)
		v := lipglossStrip(m.View())
		if !strings.Contains(v, activeMarker+"Pod") {
			t.Fatalf("step %d: active marker lost as the cursor moved:\n%s", i, v)
		}
		if got := strings.Count(v, activeMarker); got != 1 {
			t.Fatalf("step %d: active marker count = %d, want 1", i, got)
		}
	}
}

func TestActiveSurvivesReconcileByGVR(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 60)
	// Open a CRD that is not in the seed — it only appears after discovery. Keying the
	// active state by GVR (not index) means the marker lands on the row once Reconcile
	// appends it.
	crd := kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
	}
	m.SetActive(crd)
	if strings.Contains(lipglossStrip(m.View()), activeMarker) {
		t.Fatal("marker rendered before the active resource exists in the menu")
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{crd}})
	v := lipglossStrip(m.View())
	if !strings.Contains(v, activeMarker+"Widget") {
		t.Fatalf("active marker did not follow the resource across Reconcile:\n%s", v)
	}
}

func TestClearActiveRemovesMarker(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40)
	pods := m.items[findItem(m, "pods")].Resource
	m.SetActive(pods)
	if !strings.Contains(lipglossStrip(m.View()), activeMarker) {
		t.Fatal("SetActive did not mark the row")
	}
	m.ClearActive()
	if strings.Contains(lipglossStrip(m.View()), activeMarker) {
		t.Fatal("active marker still rendered after ClearActive")
	}
}

func TestLongTitleClippedToOneLine(t *testing.T) {
	m := newTestModel()
	const long = "MutatingWebhookConfiguration"
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{{
		GVK: schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "v1", Kind: long},
		GVR: schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingwebhookconfigurations"},
	}}})
	// A narrow pane, tall enough that the appended CRD row is on screen.
	m.SetSize(16, 60)
	v := m.View()

	// No rendered line may be wider than the pane — the wrap/overflow bug produced
	// lines that spilled past the border onto a second physical line.
	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > 16 {
			t.Fatalf("line width %d exceeds pane width 16: %q", w, ln)
		}
	}
	// The full name must be truncated, not rendered whole, and the cut marked.
	if strings.Contains(v, long) {
		t.Fatal("full long title rendered without truncation (would wrap outside the pane)")
	}
	if !strings.Contains(v, "…") {
		t.Fatal("truncated title is missing the ellipsis affordance")
	}
}

func TestScrollbarShownWhenOverflowing(t *testing.T) {
	m := newTestModel()
	m.SetSize(24, 8) // fewer visible rows than the seed has
	if len(m.rows()) <= m.innerHeight() {
		t.Fatalf("test setup: seed fits in the pane (%d rows ≤ %d)", len(m.rows()), m.innerHeight())
	}
	v := m.View()
	// An overflowing menu shows both a thumb and track (the thumb is shorter than
	// the pane), so it's clear there is more above/below.
	if !strings.Contains(v, scrollThumb) {
		t.Error("overflowing menu missing the scrollbar thumb")
	}
	if !strings.Contains(v, scrollTrack) {
		t.Error("overflowing menu missing the scrollbar track")
	}
}

func TestScrollbarThumbTracksOffset(t *testing.T) {
	m := newTestModel()
	m.SetSize(24, 8)
	// At the top the thumb sits on the first scrollbar cell; at the bottom it sits
	// on the last — proving position reports how far the list is scrolled.
	m, _ = m.Update(keymap.ActionTop)
	top := scrollbarColumn(t, m)
	if top[0] != scrollThumb {
		t.Errorf("at top, first scrollbar cell = %q, want thumb", top[0])
	}
	if top[len(top)-1] != scrollTrack {
		t.Errorf("at top, last scrollbar cell = %q, want track", top[len(top)-1])
	}
	m, _ = m.Update(keymap.ActionBottom)
	bottom := scrollbarColumn(t, m)
	if bottom[len(bottom)-1] != scrollThumb {
		t.Errorf("at bottom, last scrollbar cell = %q, want thumb", bottom[len(bottom)-1])
	}
	if bottom[0] != scrollThumb && bottom[0] != scrollTrack {
		t.Errorf("unexpected first scrollbar cell %q", bottom[0])
	}
}

func TestNoScrollbarWhenAllRowsFit(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 60) // tall enough for every row
	if len(m.rows()) > m.innerHeight() {
		t.Fatalf("test setup: rows still overflow (%d > %d)", len(m.rows()), m.innerHeight())
	}
	v := m.View()
	if strings.Contains(v, scrollThumb) || strings.Contains(v, scrollTrack) {
		t.Fatal("scrollbar rendered when the whole menu already fits")
	}
}

// scrollbarColumn extracts the rightmost inner cell of each content line of a
// rendered menu — the reserved scrollbar column. It strips the border row and the
// two border columns, returning one glyph per visible row.
func scrollbarColumn(t *testing.T, m Model) []string {
	t.Helper()
	all := strings.Split(m.View(), "\n")
	// Drop the top and bottom border rows.
	if len(all) < 3 {
		t.Fatalf("view has too few lines (%d) to hold a bordered viewport", len(all))
	}
	body := all[1 : len(all)-1]
	col := make([]string, 0, len(body))
	for _, ln := range body {
		r := []rune(lipglossStrip(ln))
		// r[0] and r[len-1] are the border columns; the cell before the right border
		// is the scrollbar column.
		if len(r) < 2 {
			t.Fatalf("content line too short to hold a scrollbar column: %q", ln)
		}
		col = append(col, string(r[len(r)-2]))
	}
	return col
}

// lipglossStrip removes ANSI styling so a rendered line can be indexed by cell.
func lipglossStrip(s string) string { return ansi.Strip(s) }

// assertSectionsContiguous fails if any section's items are split across the item
// slice — the D77 invariant rows() relies on to emit exactly one header per
// section. Non-resource rows (the namespace seam) are skipped.
func assertSectionsContiguous(t *testing.T, m Model) {
	t.Helper()
	seen := map[string]bool{}
	prev := ""
	for _, it := range m.items {
		if it.Kind != ItemResource {
			continue
		}
		if it.Section != prev {
			if seen[it.Section] {
				t.Fatalf("section %q is not contiguous (reappears at %q)", it.Section, it.Title)
			}
			seen[it.Section] = true
			prev = it.Section
		}
	}
}

func findGVR(m Model, gvr schema.GroupVersionResource) int {
	for i, it := range m.items {
		if it.Kind == ItemResource && it.Resource.GVR == gvr {
			return i
		}
	}
	return -1
}

func TestAddExtrasMapsEntryIntoCustomResources(t *testing.T) {
	m := newTestModel()
	seedLen := len(m.items)

	m.AddExtras([]config.MenuResource{{
		Group:      "cert-manager.io",
		Version:    "v1",
		Resource:   "certificates",
		Kind:       "Certificate",
		Namespaced: true,
	}})

	if len(m.items) != seedLen+1 {
		t.Fatalf("item count = %d, want %d", len(m.items), seedLen+1)
	}
	// A default (no section) extra lands in the trailing Custom Resources section,
	// so it is appended last.
	it := m.items[len(m.items)-1]
	if it.Section != sectionCustom {
		t.Fatalf("extra section = %q, want %q", it.Section, sectionCustom)
	}
	if it.Title != "Certificate" || !it.Available || it.Kind != ItemResource {
		t.Fatalf("extra item = %+v, want available Certificate resource row", it)
	}
	if !it.Resource.Namespaced {
		t.Error("extra should be namespaced")
	}
	wantGVR := schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	if it.Resource.GVR != wantGVR {
		t.Fatalf("extra GVR = %+v, want %+v", it.Resource.GVR, wantGVR)
	}
	if it.Resource.GVK.Kind != "Certificate" {
		t.Fatalf("extra GVK.Kind = %q, want Certificate", it.Resource.GVK.Kind)
	}
}

func TestAddExtrasTitleFallback(t *testing.T) {
	m := newTestModel()
	m.AddExtras([]config.MenuResource{
		{Version: "v1", Resource: "widgets", Kind: "Widget", Title: "My Widget"}, // Title wins
		{Version: "v1", Resource: "gadgets", Kind: "Gadget"},                     // Kind fallback
		{Version: "v1", Resource: "sprockets"},                                   // Resource fallback
	})
	want := map[string]string{"widgets": "My Widget", "gadgets": "Gadget", "sprockets": "sprockets"}
	for res, title := range want {
		i := findItem(m, res)
		if i < 0 {
			t.Fatalf("extra %q missing", res)
		}
		if m.items[i].Title != title {
			t.Errorf("%q title = %q, want %q", res, m.items[i].Title, title)
		}
	}
}

func TestAddExtrasDedupsAgainstSeedAndItself(t *testing.T) {
	m := newTestModel()
	seedLen := len(m.items)

	m.AddExtras([]config.MenuResource{
		{Version: "v1", Resource: "pods", Kind: "Pod"},        // dup of a seed row (GVR match)
		{Version: "v1", Resource: "widgets", Kind: "Widget"},  // new
		{Version: "v1", Resource: "widgets", Kind: "Widget2"}, // dup of the extra just added
	})

	if len(m.items) != seedLen+1 {
		t.Fatalf("item count = %d, want %d (one net add)", len(m.items), seedLen+1)
	}
	// The seed's pods row is untouched (still exactly one pods row).
	pods := 0
	for _, it := range m.items {
		if it.Resource.GVR.Resource == "pods" {
			pods++
		}
	}
	if pods != 1 {
		t.Fatalf("pods rows = %d, want 1 (extra dup must not add a second)", pods)
	}
}

func TestAddExtrasKeepsSectionsContiguous(t *testing.T) {
	m := newTestModel()

	// One extra into an existing section (Workloads), two into a brand-new default
	// (Custom Resources) section — all must stay contiguous within their section.
	m.AddExtras([]config.MenuResource{
		{Group: "apps", Version: "v1", Resource: "rollouts", Kind: "Rollout", Section: sectionWorkloads},
		{Version: "v1", Resource: "widgets", Kind: "Widget"},
		{Version: "v1", Resource: "gadgets", Kind: "Gadget"},
	})

	assertSectionsContiguous(t, m)

	// rows() still emits exactly one header per section (the D77 guarantee).
	sections := map[string]bool{}
	for _, it := range m.items {
		if it.Kind == ItemResource {
			sections[it.Section] = true
		}
	}
	headers := 0
	for _, r := range m.rows() {
		if r.header {
			headers++
		}
	}
	if headers != len(sections) {
		t.Fatalf("rendered %d headers, want %d (one per section)", headers, len(sections))
	}
	// The Workloads extra sits inside the Workloads run, not after Custom Resources.
	ri := findItem(m, "rollouts")
	if ri < 0 || m.items[ri].Section != sectionWorkloads {
		t.Fatalf("rollout landed at %d with wrong section", ri)
	}
}

func TestAddExtrasPreservesSelection(t *testing.T) {
	m := newTestModel()
	target := findItem(m, "pods")
	m.SelectItem(target)
	if m.cursor != target {
		t.Fatalf("setup: cursor = %d, want %d", m.cursor, target)
	}

	m.AddExtras([]config.MenuResource{
		// An extra into Workloads (before pods? no — after events) plus a Custom one.
		{Group: "apps", Version: "v1", Resource: "rollouts", Kind: "Rollout", Section: sectionWorkloads},
		{Version: "v1", Resource: "widgets", Kind: "Widget"},
	})

	if sel, _ := m.Selected(); sel.Resource.GVR.Resource != "pods" {
		t.Fatalf("selection moved to %q, want pods", sel.Resource.GVR.Resource)
	}
}

func TestDiscoveredTwinDoesNotDuplicateExtra(t *testing.T) {
	m := newTestModel()
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}

	// Add the CRD as a per-context extra first (as app start would), then a later
	// discovery pass returns the same GVR: Reconcile must fill the twin, not append
	// a duplicate (the D57 seen-set covers extras already in the item slice).
	m.AddExtras([]config.MenuResource{{Group: "example.com", Version: "v1", Resource: "widgets", Kind: "Widget"}})
	afterExtra := len(m.items)

	twin := kube.Resource{
		GVK:        schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR:        gvr,
		Namespaced: true,
		ShortNames: []string{"wg"},
	}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{twin}})

	if len(m.items) != afterExtra {
		t.Fatalf("item count = %d after reconcile, want %d (no duplicate)", len(m.items), afterExtra)
	}
	i := findGVR(m, gvr)
	if i < 0 {
		t.Fatal("widgets row missing after reconcile")
	}
	if got := m.items[i].Resource.ShortNames; len(got) != 1 || got[0] != "wg" {
		t.Fatalf("twin metadata not merged into the extra: short names = %v", got)
	}
	if !m.items[i].Available {
		t.Error("extra with a discovered twin should be available")
	}
}

// TestAddPinnedMarksTheRowsItInserts is CRD-PIN-03's foundation: a pin merges like
// any other extra, but the row it *inserts* is marked so Unpin can tell it apart
// from a row that would have been there anyway.
func TestAddPinnedMarksTheRowsItInserts(t *testing.T) {
	m := newTestModel()
	seedLen := len(m.items)

	m.AddPinned([]config.MenuResource{
		{Version: "v1", Resource: "widgets", Kind: "Widget"}, // new: the pin is the reason
		{Version: "v1", Resource: "pods", Kind: "Pod"},       // a seed row: nothing to insert
	})

	if len(m.items) != seedLen+1 {
		t.Fatalf("item count = %d, want %d (one net add)", len(m.items), seedLen+1)
	}
	if i := findItem(m, "widgets"); i < 0 || !m.items[i].Pinned {
		t.Errorf("the inserted row should be marked pinned: %+v", m.items[findItem(m, "widgets")])
	}
	if i := findItem(m, "pods"); i < 0 || m.items[i].Pinned {
		t.Error("a pin over a seed row marks nothing — the seed row is not the pin's to remove")
	}
}

// TestAddPinnedYieldsToAnAuthoredEntry is D193 pt 3 where it now lives: the authored
// list goes in first and a pin naming the same GVR is skipped, so the hand-written
// title and section render — and, being unmarked, that row is not unpinnable.
func TestAddPinnedYieldsToAnAuthoredEntry(t *testing.T) {
	m := newTestModel()
	m.AddExtras([]config.MenuResource{
		{Version: "v1", Resource: "widgets", Kind: "Widget", Title: "Authored", Section: "Workloads"},
	})
	m.AddPinned([]config.MenuResource{
		{Version: "v1", Resource: "widgets", Kind: "Widget", Title: "Pinned"},
	})

	rows := 0
	for _, it := range m.items {
		if it.Resource.GVR.Resource == "widgets" {
			rows++
			if it.Title != "Authored" || it.Section != "Workloads" || it.Pinned {
				t.Errorf("row = %+v, want the unmarked authored entry", it)
			}
		}
	}
	if rows != 1 {
		t.Fatalf("widgets rows = %d, want 1", rows)
	}
}

// TestUnpinRemovesAPinOnlyRow: with nothing else listing the kind, the pin was the
// row's only reason to exist, so it goes — and the cursor stays on a real row rather
// than off the end.
func TestUnpinRemovesAPinOnlyRow(t *testing.T) {
	m := newTestModel()
	seedLen := len(m.items)
	m.AddPinned([]config.MenuResource{{Version: "v1", Resource: "widgets", Kind: "Widget"}})
	i := findItem(m, "widgets")
	if i < 0 {
		t.Fatal("the pinned row should be in the menu")
	}
	m.SelectItem(i)

	if !m.Unpin(schema.GroupVersionResource{Version: "v1", Resource: "widgets"}) {
		t.Fatal("Unpin should report it removed the row")
	}
	if findItem(m, "widgets") >= 0 || len(m.items) != seedLen {
		t.Fatalf("item count = %d, want the seed back (%d)", len(m.items), seedLen)
	}
	if m.cursor < 0 || m.cursor >= len(m.items) {
		t.Fatalf("cursor = %d, out of range after the removal", m.cursor)
	}
}

// TestUnpinKeepsARowDiscoveryLists is the revert-to-a-discovered-row rule at the
// component level: Reconcile marked the row discovered, so the unpin takes the
// marker and leaves the *kind* in the inventory — and a second Unpin has nothing
// left to do. Since D288 the pane no longer lists it, which is the point of the
// toggle: unpinning a CRD is how you get it out of the pane again.
func TestUnpinKeepsARowDiscoveryLists(t *testing.T) {
	m := newTestModel()
	m.AddPinned([]config.MenuResource{{Version: "v1", Resource: "widgets", Kind: "Widget"}})
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "widgets"}
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{{
		GVK: schema.GroupVersionKind{Version: "v1", Kind: "Widget"}, GVR: gvr,
	}}})
	before := len(m.full)

	if m.Unpin(gvr) {
		t.Error("a discovered row is not removed by an unpin")
	}
	i := findFull(m, "widgets")
	if i < 0 || len(m.full) != before {
		t.Fatalf("the kind should still be listed: count %d, want %d", len(m.full), before)
	}
	if m.full[i].Pinned {
		t.Error("the pin marker should be gone even though the kind stayed")
	}
	// It stays in the inventory but leaves the pane: with no pin behind it, a
	// discovered CRD is one of the rows the pane holds back (D288).
	if findItem(m, "widgets") >= 0 {
		t.Error("an unpinned custom resource should no longer be listed in the pane")
	}
	if m.Unpin(gvr) {
		t.Error("a second unpin has nothing to remove")
	}
}

// TestUnpinLeavesUnmarkedRowsAlone is the guard that keeps a stray pin from deleting
// the menu: a seed row, an authored entry or a purely discovered row was never put
// there by a pin, so no unpin may take it away, whatever the state file said.
func TestUnpinLeavesUnmarkedRowsAlone(t *testing.T) {
	m := newTestModel()
	m.AddExtras([]config.MenuResource{{Version: "v1", Resource: "widgets", Kind: "Widget"}})
	before := len(m.items)

	if m.Unpin(schema.GroupVersionResource{Version: "v1", Resource: "pods"}) {
		t.Error("a seed row must not be removable by an unpin")
	}
	if m.Unpin(schema.GroupVersionResource{Version: "v1", Resource: "widgets"}) {
		t.Error("an authored entry must not be removable by an unpin")
	}
	if len(m.items) != before || findItem(m, "pods") < 0 || findItem(m, "widgets") < 0 {
		t.Fatalf("item count = %d, want %d — no row should have gone", len(m.items), before)
	}
}

func TestAddExtrasEmptyIsNoOp(t *testing.T) {
	m := newTestModel()
	before := len(m.items)
	m.AddExtras(nil)
	m.AddExtras([]config.MenuResource{})
	if len(m.items) != before {
		t.Fatalf("empty AddExtras changed item count %d → %d", before, len(m.items))
	}
}

// TestRowItemAtMapsContentRowToItem proves the mouse coordinate seam resolves a
// content-area row to the item rendered on it (dogfood-08): section headers and
// blank/out-of-range lines map to no item, real item lines map to their index.
func TestRowItemAtMapsContentRowToItem(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 40) // tall enough that everything fits with offset 0
	if m.offset != 0 {
		t.Fatalf("expected offset 0 with a tall pane, got %d", m.offset)
	}
	// Content row 0 is the "Cluster" section header — not an item.
	if _, ok := m.RowItemAt(0); ok {
		t.Fatal("content row 0 (section header) should map to no item")
	}
	// Content row 1 is the first item (namespaces).
	if idx, ok := m.RowItemAt(1); !ok || m.items[idx].Resource.GVR.Resource != "namespaces" {
		t.Fatalf("content row 1: idx=%d ok=%v, want the namespaces item", idx, ok)
	}
	// A content row past the last rendered line maps to nothing.
	if _, ok := m.RowItemAt(1000); ok {
		t.Fatal("a content row past the end should map to no item")
	}
	// A content row at the bottom border (>= innerHeight) is rejected.
	if _, ok := m.RowItemAt(m.innerHeight()); ok {
		t.Fatal("a content row at innerHeight (bottom border) should map to no item")
	}
	// A negative content row is rejected.
	if _, ok := m.RowItemAt(-1); ok {
		t.Fatal("a negative content row should map to no item")
	}
}

// TestRowItemAtHonorsScrollOffset proves RowItemAt composes with the vertical
// scroll: after scrolling, an on-screen content row still resolves to the item
// actually drawn there.
func TestRowItemAtHonorsScrollOffset(t *testing.T) {
	m := newTestModel()
	m.SetSize(30, 6)               // short pane → scrolling
	m.SelectItem(len(m.items) - 1) // last item; offset advances to keep it visible
	if m.offset == 0 {
		t.Fatal("selecting the last item in a short pane should scroll the offset")
	}
	// The cursor's on-screen content row (its display row minus the scroll offset)
	// must resolve back to the cursor item through the offset.
	contentRow := m.cursorRow() - m.offset
	if idx, ok := m.RowItemAt(contentRow); !ok || idx != m.cursor {
		t.Fatalf("RowItemAt(%d) = (%d,%v) after scroll, want the cursor item %d",
			contentRow, idx, ok, m.cursor)
	}
}

// TestSelectItemMovesCursor proves the public cursor setter used by a mouse click
// moves and clamps like keyboard navigation.
func TestSelectItemMovesCursor(t *testing.T) {
	m := newTestModel()
	m.SelectItem(5)
	if m.cursor != 5 {
		t.Fatalf("SelectItem: cursor = %d, want 5", m.cursor)
	}
	m.SelectItem(1000) // clamps to the last item
	if m.cursor != len(m.items)-1 {
		t.Fatalf("SelectItem high clamp: cursor = %d, want %d", m.cursor, len(m.items)-1)
	}
	m.SelectItem(-5) // clamps to 0
	if m.cursor != 0 {
		t.Fatalf("SelectItem low clamp: cursor = %d, want 0", m.cursor)
	}
}

// TestSetFilterNarrowsItems is the STORY-06m mirror of the table's filter at the
// component level: SetFilter narrows the displayed list to the matching kinds
// (case-insensitive substring across title/kind/resource/short-names), the
// authoritative full set survives untouched (Items), and ClearFilter brings every
// kind back.
func TestSetFilterNarrowsItems(t *testing.T) {
	m := newTestModel()
	full := len(m.items)

	m.SetFilter("deploy")
	if got := len(m.items); got != 1 {
		t.Fatalf("filter 'deploy' left %d items, want 1 (Deployments)", got)
	}
	if sel, _ := m.Selected(); sel.Resource.GVR.Resource != "deployments" {
		t.Fatalf("selection after narrowing = %q, want deployments", sel.Resource.GVR.Resource)
	}
	if m.Filter() != "deploy" {
		t.Fatalf("Filter() = %q, want deploy", m.Filter())
	}
	if got := len(m.Items()); got != full {
		t.Fatalf("the authoritative full set must survive a filter: %d items, want %d", got, full)
	}

	// A query that matches nothing narrows to an empty list (the seam row is a row
	// too, so it filters with everything else) and still leaves the full set intact.
	m.SetFilter("zzz-nothing")
	if len(m.items) != 0 {
		t.Fatalf("a no-match filter should show an empty list, got %d items", len(m.items))
	}
	if got := len(m.Items()); got != full {
		t.Fatalf("authoritative set after a no-match filter: %d, want %d", got, full)
	}

	m.ClearFilter()
	if len(m.items) != full {
		t.Fatalf("ClearFilter should restore every kind: %d, want %d", len(m.items), full)
	}
	if m.Filter() != "" {
		t.Fatalf("Filter() after clear = %q, want empty", m.Filter())
	}
}

// TestSetFilterMatchesResourceAliases proves the menu filter answers to the names a
// kind goes by, not just its display title: the plural resource, a short name and
// the API group all narrow the same row (the D203 alias surface).
func TestSetFilterMatchesResourceAliases(t *testing.T) {
	m := newTestModel()
	for _, q := range []string{"deployment", "deployments", "apps"} {
		m.SetFilter(q)
		if sel, ok := m.Selected(); !ok || sel.Resource.GVR.Resource != "deployments" {
			t.Fatalf("filter %q selected %+v, want deployments", q, sel.Resource.GVR)
		}
	}
	// A CRD appended by discovery matches by its short name.
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{{
		GVK:        schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		GVR:        schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
		ShortNames: []string{"wg"},
	}}})
	m.SetFilter("wg")
	if sel, ok := m.Selected(); !ok || sel.Resource.GVR.Resource != "widgets" {
		t.Fatalf("short-name filter 'wg' selected %+v, want widgets", sel.Resource.GVR)
	}
}

// TestSetFilterPreservesSelectionByGVR keeps the cursor on the same resource across
// a filter change when it still matches, and lets a selection that stops matching
// fall back onto the narrowed range rather than floating out of it.
func TestSetFilterPreservesSelectionByGVR(t *testing.T) {
	m := newTestModel()
	// Move the cursor onto "pods" (a seed item), then narrow to "pod": the selection
	// should follow the same row, and drilling in should still emit its resource.
	target := findItem(m, "pods")
	m.SelectItem(target)
	m.SetFilter("pod")
	if sel, _ := m.Selected(); sel.Resource.GVR.Resource != "pods" {
		t.Fatalf("selection after narrowing to 'pod' = %q, want pods", sel.Resource.GVR.Resource)
	}
	if _, cmd := m.Update(keymap.ActionDrillIn); cmd == nil {
		t.Fatal("drill-in on a filtered row should emit a ResourceSelectedMsg")
	}

	// A selection that stops matching falls back into range: narrow past the
	// selection's row and the cursor must still point at a displayed item.
	m.SetFilter("deploy")
	if m.cursor < 0 || m.cursor >= len(m.items) {
		t.Fatalf("cursor = %d out of range after its row stopped matching", m.cursor)
	}
}

// TestReconcileKeepsFilterApplied proves a discovery pass lands inside the pane's
// filter view: new kinds appear in the displayed list only when they match, and the
// authoritative full list takes the append either way.
func TestReconcileKeepsFilterApplied(t *testing.T) {
	m := newTestModel()
	m.SetFilter("widget")
	before := len(m.items)
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{
		{
			GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
			GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"},
		},
		{
			GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Gadget"},
			GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "gadgets"},
		},
	}})
	if got := len(m.items); got != before+1 {
		t.Fatalf("reconcile under filter 'widget' shows %d items, want %d (only Widget joins)", got, before+1)
	}
	if sel, ok := m.Selected(); !ok || sel.Resource.GVR.Resource != "widgets" {
		t.Fatalf("selection after reconcile = %+v, want widgets", sel.Resource.GVR)
	}
	if m.Filter() != "widget" {
		t.Fatalf("the filter must survive reconcile, got %q", m.Filter())
	}
	// Both kinds are in the authoritative set.
	foundGadget := false
	for _, it := range m.Items() {
		if it.Resource.GVR.Resource == "gadgets" {
			foundGadget = true
		}
	}
	if !foundGadget {
		t.Error("the non-matching Gadget must still join the authoritative full set")
	}
}

// Ensure the emitted command types satisfy tea.Cmd (compile-time contract).
var (
	_ tea.Cmd = func() tea.Msg { return ResourceSelectedMsg{} }
	_ tea.Cmd = func() tea.Msg { return NamespaceRequestedMsg{} }
)

// crdFor is a discovered custom resource, the kind the pane holds back by default.
func crdFor(name, kind string) kube.Resource {
	return kube.Resource{
		GVK: schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: kind},
		GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: name},
	}
}

// discoveredCRDs is a model whose discovery pass reported two CRDs — the state the
// hiding rule is about.
func discoveredCRDs(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	m.SetSize(30, 60)
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{
		crdFor("widgets", "Widget"), crdFor("sprockets", "Sprocket"),
	}})
	return m
}

// TestDiscoveredCRDsAreHeldBackButStayInTheInventory is the STORY-06l headline
// (feedback 2026-08-15, D288): discovery finds hundreds of custom resources on an
// operator-heavy cluster and the pane stops listing them, while the kind inventory
// every other surface reads — Items(), and so the resource picker, cluster search,
// relations and pane memory — keeps every one.
func TestDiscoveredCRDsAreHeldBackButStayInTheInventory(t *testing.T) {
	m := discoveredCRDs(t)

	if findItem(m, "widgets") >= 0 || findItem(m, "sprockets") >= 0 {
		t.Error("an unpinned custom resource should not be listed in the pane")
	}
	if findFull(m, "widgets") < 0 || findFull(m, "sprockets") < 0 {
		t.Error("the kinds must stay in the inventory Items() reports")
	}
	if got := len(m.Items()); got != len(m.full) {
		t.Errorf("Items() = %d rows, want the whole authoritative list (%d)", got, len(m.full))
	}
	// And the pane says how many it is holding back, so "this cluster has no CRDs" and
	// "kubecom is not showing you two of them" are not the same frame.
	if v := lipglossStrip(m.View()); !strings.Contains(v, "+2 custom") {
		t.Errorf("the pane should report the held-back count:\n%s", v)
	}
}

// TestAPaneFilterReachesTheHeldBackCRDs is the reachability half of the rule: hiding
// is only ever about the default frame, so an explicit `/` query in the pane finds a
// custom resource the pane does not list.
func TestAPaneFilterReachesTheHeldBackCRDs(t *testing.T) {
	m := discoveredCRDs(t)
	m.SetFilter("widget")

	if findItem(m, "widgets") < 0 {
		t.Fatal("a typed query must reach a held-back custom resource")
	}
	if n := m.HiddenCustom(); n != 0 {
		t.Errorf("nothing is held back under a filter, got %d", n)
	}
	m.ClearFilter()
	if findItem(m, "widgets") >= 0 {
		t.Error("clearing the filter should hold the custom resource back again")
	}
}

// TestPinningAListedCRDOptsItIn is the opt-in the feedback asked for: `*` over a kind
// discovery already listed marks that row rather than inserting a second one, and the
// pane lists it from then on. Unpinning takes it back out without losing the kind.
func TestPinningAListedCRDOptsItIn(t *testing.T) {
	m := discoveredCRDs(t)
	before := len(m.full)
	entry := config.MenuResource{Group: "example.com", Version: "v1", Resource: "widgets", Kind: "Widget"}

	m.AddPinned([]config.MenuResource{entry})
	if len(m.full) != before {
		t.Fatalf("a pin over a listed kind must not add a row: %d, want %d", len(m.full), before)
	}
	if findItem(m, "widgets") < 0 {
		t.Fatal("a pinned custom resource should be listed in the pane")
	}
	if findItem(m, "sprockets") >= 0 {
		t.Error("pinning one kind must not reveal the others")
	}

	m.Unpin(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"})
	if findItem(m, "widgets") >= 0 {
		t.Error("unpinning should hold the custom resource back again")
	}
	if findFull(m, "widgets") < 0 {
		t.Error("unpinning must not drop the kind from the inventory")
	}
}

// TestAnAuthoredEntryIsNeverHeldBack keeps the menus/<context>.yaml file authoritative
// (D193 pt 3): a kind the user wrote down by hand is listed whether or not discovery
// also reports it, and no pin is needed to keep it.
func TestAnAuthoredEntryIsNeverHeldBack(t *testing.T) {
	m := newTestModel()
	m.AddExtras([]config.MenuResource{{Group: "example.com", Version: "v1", Resource: "widgets", Kind: "Widget"}})
	m.Reconcile(kube.DiscoveryResult{Resources: []kube.Resource{
		crdFor("widgets", "Widget"), crdFor("sprockets", "Sprocket"),
	}})

	if findItem(m, "widgets") < 0 {
		t.Error("an authored menu entry must stay listed")
	}
	if findItem(m, "sprockets") >= 0 {
		t.Error("a merely discovered CRD is still held back")
	}
}

// TestTheOpenKindIsAlwaysListed: reaching a held-back CRD through the resource picker
// or a search hit is an ordinary way in, and a left pane that cannot point at what the
// right pane has open is worse than a long one.
func TestTheOpenKindIsAlwaysListed(t *testing.T) {
	m := discoveredCRDs(t)
	m.SetActive(crdFor("widgets", "Widget"))

	if findItem(m, "widgets") < 0 {
		t.Fatal("the open kind must be listed even when the pane would hold it back")
	}
	if !m.SelectResource(crdFor("widgets", "Widget").GVR) {
		t.Error("the cursor must be able to land on the open kind's row")
	}
	m.ClearActive()
	if findItem(m, "widgets") >= 0 {
		t.Error("closing the table holds the custom resource back again")
	}
}
