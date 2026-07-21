// Package menu is kubecom's resource-menu sidebar: the left pane of the browse
// view, a vertical list of Kubernetes resource kinds the user moves through to
// choose what the table pane shows. The list is grouped into the familiar
// Kubernetes-Dashboard scopes (Cluster / Workloads / Config / Network / Storage /
// Access Control, plus Custom Resources for discovered CRDs) with non-selectable
// section headers the cursor skips over (D77). Between the cluster-scoped section
// and the namespaced ones sits a namespace-picker seam row (ItemNamespace): a
// selectable non-resource row that shows the scoped namespace and, on drill-in,
// requests the namespace picker (NamespaceRequestedMsg) — the same effect as the
// ctrl+n shortcut — so the menu itself communicates the cluster/namespaced
// boundary. The seed provides a static list of core resource kinds; Reconcile
// merges the async discovery result (adding CRDs/extra groups, marking unavailable
// ones) without disturbing the current selection or scroll, and skips the
// non-resource seam row. The list is a real viewport: rows are clipped to the
// pane width (long kind names truncate with an ellipsis rather than wrapping
// outside the border), the selection is always scrolled into view, and a
// proportional scrollbar appears in the rightmost column whenever the menu has
// more rows than the pane can show.
//
// The menu shows two independent states so it is always clear both which resource
// is open and where the cursor is (dogfood-05): the nav cursor is the highlighted
// row, while the opened/active resource — the one whose table fills the right pane,
// set by SetActive — is prefixed with a "▸ " marker and accented, so it stays
// marked as open even when the cursor moves to a different item.
//
// The menu never matches a raw key (D11): the root model resolves a KeyMsg to a
// keymap.Action and hands the Action to Update, which moves the selection. When
// the user drills in (nav.drillIn) the menu emits a ResourceSelectedMsg — its own
// message type, owned here rather than in package tui, so the component does not
// import the root package (which would cycle, since the root imports this one)
// (D56). It owns no shared mutable state (principle 1): the root model holds the
// one Model, feeds it actions, and reads its View.
package menu

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// Section names group the menu into the familiar Kubernetes-Dashboard scopes.
// They double as the header titles rendered above each group (D77). sectionCustom
// is the trailing bucket every discovered CRD/extra group lands in on Reconcile,
// so the grouping survives discovery.
const (
	sectionCluster   = "Cluster"
	sectionWorkloads = "Workloads"
	sectionConfig    = "Config"
	sectionNetwork   = "Network"
	sectionStorage   = "Storage"
	sectionAccess    = "Access Control"
	sectionCustom    = "Custom Resources"
)

// namespaceAll is the label the namespace-seam row shows when no namespace is
// scoped ("" = every namespace), so the seam always states the scope explicitly.
// Rendered as a compact "(all)" so the seam reads like a dropdown value rather
// than a sentence (dogfood-09).
const namespaceAll = "(all)"

// namespaceArrow is the dropdown indicator prefixed to the namespace-seam value,
// so the seam reads "▾ <value>" like a collapsed dropdown rather than a labelled
// field (dogfood-09).
const namespaceArrow = "▾ "

// ellipsis is appended to a title that is too wide for the pane, so long kind
// names (e.g. MutatingWebhookConfiguration) are clipped to one line rather than
// wrapping outside the pane border (the dogfood-03 overflow bug).
const ellipsis = "…"

// activeMarker prefixes the opened/active resource row — the item whose table is
// showing in the right pane — so it stays visibly marked even while the nav cursor
// sits on a different item (dogfood-05: the cursor and the opened item are two
// separate states). It is two display columns wide, exactly like the plain "  "
// item indent it replaces, so it never shifts the clip width.
const activeMarker = "▸ "

// itemIndent is the plain two-column indent under a section header for a resource
// row that is neither the cursor nor the active item, matching activeMarker's width
// so rows align whether or not one is marked active.
const itemIndent = "  "

// Scrollbar glyphs for the reserved right-hand column, shown only when the menu
// has more rows than the pane can display. Neither glyph appears in the rounded
// pane border (which is ╭╮╰╯│─), so a rendered view can be scanned for them
// unambiguously. The thumb's span and position report how much is scrolled off
// above and below (the dogfood-03 "no scroll affordance" gap).
const (
	scrollThumb = "█" // the draggable position indicator
	scrollTrack = "░" // the rest of the track
)

// ItemKind distinguishes a normal resource row from a special non-resource row.
// The zero value is ItemResource so every seed/discovered resource item — and any
// value not explicitly tagged — is a resource, keeping existing construction and
// Reconcile behaviour unchanged.
type ItemKind int

const (
	// ItemResource is a Kubernetes resource kind: drilling in starts a watch.
	ItemResource ItemKind = iota
	// ItemNamespace is the namespace-picker seam row: drilling in requests the
	// namespace picker (NamespaceRequestedMsg) rather than a watch. It carries no
	// Resource/Section and sits between the cluster-scoped and namespaced sections.
	ItemNamespace
)

// Item is one row in the menu: a resource kind plus its display title, the
// section it groups under, and an availability flag. Available is true for every
// seed item; M2-05b sets it false for a discovered-but-unreachable group so the
// row renders muted but is not selectable for a watch. Section places the item
// under a header (D77); items sharing a section must be contiguous in the slice.
// Kind marks a non-resource special row (ItemNamespace, the picker seam) — such a
// row has no Resource/Section and is skipped by discovery Reconcile.
type Item struct {
	Resource  kube.Resource
	Title     string
	Section   string
	Available bool
	Kind      ItemKind
}

// ResourceSelectedMsg is emitted when the user drills into the highlighted menu
// item (nav.drillIn). The root model reacts by (re)starting a watch for Resource.
// It is owned by the menu package — the emitter — so the root model handles this
// concrete type; the menu never imports the root package (D56).
type ResourceSelectedMsg struct {
	Resource kube.Resource
}

// NamespaceRequestedMsg is emitted when the user drills into the namespace-seam
// row (the ItemNamespace item). The root model reacts by opening the namespace
// picker — the same effect as the ns.switch (ctrl+n) shortcut. Like
// ResourceSelectedMsg it is owned by the menu package so the menu never imports
// the root package (D56).
type NamespaceRequestedMsg struct{}

// Model is the resource menu. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles
	items  []Item

	namespace string // the scoped namespace shown on the seam row ("" → all)

	// active tracks the opened resource — the one whose live table is showing in the
	// right pane — as a distinct visual state from the nav cursor (dogfood-05). It is
	// keyed by GVR rather than an index so it survives Reconcile appending CRDs (the
	// same robustness the cursor-preservation resolve-by-GVR gives). hasActive guards
	// it; only ItemResource rows are ever active (drilling into the namespace seam
	// opens the picker, not a table).
	activeGVR schema.GroupVersionResource
	hasActive bool

	cursor  int // index of the highlighted item
	offset  int // index of the first visible item (vertical scroll)
	width   int // total width incl. border
	height  int // total height incl. border
	focused bool
}

// New builds a resource menu seeded with the default core-resource list, rendered
// through the given styles. The first item is selected.
func New(s styles.Styles) Model {
	return Model{styles: s, items: seedItems()}
}

// SetSize sets the menu's total size (including its border). The root model wires
// this from the pane geometry it computes on a WindowSizeMsg.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
}

// SetNamespace records the scoped namespace shown on the namespace-seam row
// ("" renders as "(all)"). The root model wires this from the initial
// -n scope and every namespace-picker selection so the seam always reflects the
// live scope.
func (m *Model) SetNamespace(ns string) { m.namespace = ns }

// SetActive marks r as the opened/active resource — the one whose live table is
// showing in the right pane — so its menu row renders in the distinct active state
// even when the nav cursor moves elsewhere (dogfood-05). The root model wires this
// from selectResource every time it (re)starts a watch. Keyed by GVR so it survives
// a discovery Reconcile that appends CRDs.
func (m *Model) SetActive(r kube.Resource) {
	m.activeGVR = r.GVR
	m.hasActive = true
}

// ClearActive drops the opened/active marker (no resource is open). Kept for
// completeness / future use when a watch is torn down without a replacement.
func (m *Model) ClearActive() {
	m.hasActive = false
	m.activeGVR = schema.GroupVersionResource{}
}

// isActive reports whether it is the opened/active resource row (the one whose
// table is showing). Only resource rows can be active; a zero activeGVR never
// matches because a real resource always carries a non-empty version+resource.
func (m Model) isActive(it Item) bool {
	return m.hasActive && it.Kind == ItemResource && it.Resource.GVR == m.activeGVR
}

// Focus marks the menu as holding focus (accented border).
func (m *Model) Focus() { m.focused = true }

// Blur marks the menu as not holding focus.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the menu holds focus.
func (m Model) Focused() bool { return m.focused }

// Cursor is the index of the highlighted item (for tests / the root model).
func (m Model) Cursor() int { return m.cursor }

// Items returns the menu's items in display order.
func (m Model) Items() []Item { return m.items }

// Selected returns the currently highlighted item and true, or a zero Item and
// false when the menu is empty.
func (m Model) Selected() (Item, bool) {
	if len(m.items) == 0 {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

// Reconcile merges an async discovery result into the seed menu without
// disturbing the current selection or scroll — the M2 risk item (D57). It:
//
//   - fills each seed item that has a discovered twin (matched by GVR) with the
//     discovery metadata (verbs/short-names/categories), keeping the seed's title
//     and its curated order, and marks it available;
//   - marks a seed item unavailable (Item.Available = false — rendered muted, a
//     no-op on drill-in) when it has no twin and its API group failed discovery;
//   - appends the discovered resources the seed does not already list (CRDs and
//     extra groups), in discovery's stable sorted order, after the seed.
//
// A total discovery failure (Result.Err != nil) leaves the menu untouched so it
// stays navigable on the seed alone (principle 3): degrade, don't blank. Merging
// into the ordered seed rather than replacing it wholesale means a partial failure
// likewise never costs the user a working menu. Selection is preserved by
// resolving the highlighted item's GVR back to its post-merge index, and the
// scroll offset is re-clamped so nothing jumps.
func (m *Model) Reconcile(result kube.DiscoveryResult) {
	if result.Err != nil {
		return
	}

	// Remember the highlighted item so we can restore the cursor to it after the
	// item slice changes. Resource rows are resolved back by GVR; the namespace
	// seam has no GVR, so it is restored by kind.
	var selectedGVR schema.GroupVersionResource
	var selectedKind ItemKind
	haveSelection := len(m.items) > 0
	if haveSelection {
		selectedGVR = m.items[m.cursor].Resource.GVR
		selectedKind = m.items[m.cursor].Kind
	}

	// Index the discovered resources by GVR (twin lookup) and collect the groups
	// that failed discovery (the mark-unavailable signal). Failures are per
	// group/version; we key on the group so a seed item pinned to a version that
	// differs from the failed one is still recognised as unreachable.
	twin := make(map[schema.GroupVersionResource]kube.Resource, len(result.Resources))
	for _, r := range result.Resources {
		twin[r.GVR] = r
	}
	failedGroups := make(map[string]bool, len(result.Failed))
	for _, f := range result.Failed {
		if gv, err := schema.ParseGroupVersion(f.GroupVersion); err == nil {
			failedGroups[gv.Group] = true
		}
	}

	// Reconcile the seed items in place, preserving their order and title.
	seen := make(map[schema.GroupVersionResource]bool, len(m.items))
	for i := range m.items {
		if m.items[i].Kind != ItemResource {
			continue // non-resource seam rows (namespace picker) have no twin.
		}
		gvr := m.items[i].Resource.GVR
		seen[gvr] = true
		if d, ok := twin[gvr]; ok {
			// A loaded twin: fill the discovery metadata and confirm availability.
			m.items[i].Resource = d
			m.items[i].Available = true
		} else if failedGroups[gvr.Group] {
			m.items[i].Available = false
		}
	}

	// Append discovered resources the seed does not already list (CRDs, extra
	// groups), in discovery's stable order, into the trailing Custom Resources
	// section so the grouping survives discovery (D77).
	for _, r := range result.Resources {
		if seen[r.GVR] {
			continue
		}
		m.items = append(m.items, Item{
			Resource:  r,
			Title:     r.GVK.Kind,
			Section:   sectionCustom,
			Available: true,
		})
	}

	// Restore the selection to the same resource and re-clamp the scroll. The seed
	// is never reordered or prepended to, so the index is stable; resolving by GVR
	// keeps the guarantee robust regardless.
	if haveSelection {
		for i := range m.items {
			if m.items[i].Kind != selectedKind {
				continue
			}
			if selectedKind != ItemResource || m.items[i].Resource.GVR == selectedGVR {
				m.cursor = i
				break
			}
		}
	}
	m.clampOffset()
}

// Update handles a resolved keymap action. Navigation actions (up/down/top/
// bottom) move the highlight and keep it visible; nav.drillIn emits a
// ResourceSelectedMsg for the highlighted item so the root model can start its
// watch. Any other action is ignored (the root routes it elsewhere). The menu
// consumes actions, never raw keys (D11).
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if len(m.items) == 0 {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.moveTo(m.cursor - 1)
	case keymap.ActionDown:
		m.moveTo(m.cursor + 1)
	case keymap.ActionTop:
		m.moveTo(0)
	case keymap.ActionBottom:
		m.moveTo(len(m.items) - 1)
	case keymap.ActionDrillIn:
		item := m.items[m.cursor]
		if !item.Available {
			return m, nil
		}
		if item.Kind == ItemNamespace {
			return m, func() tea.Msg { return NamespaceRequestedMsg{} }
		}
		res := item.Resource
		return m, func() tea.Msg { return ResourceSelectedMsg{Resource: res} }
	}
	return m, nil
}

// moveTo sets the cursor to i (clamped to the item range) and scrolls so it stays
// visible.
func (m *Model) moveTo(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(m.items)-1 {
		i = len(m.items) - 1
	}
	m.cursor = i
	m.scrollToCursor()
}

// row is one rendered line of the menu: either a non-selectable section header or
// a selectable item (indexed back into m.items). The cursor only ever lands on
// item rows — headers are visual only (D77) — but scroll accounts for both so the
// window math stays correct once headers occupy screen lines.
type row struct {
	header  bool
	title   string // header title when header; unused for items
	itemIdx int    // index into m.items when !header
}

// rows expands the flat item slice into the rendered line sequence, inserting a
// section header wherever the section changes. It relies on the seed invariant
// that items of a section are contiguous (seed authoring + Reconcile's
// append-to-Custom-Resources both preserve it), so each section yields exactly one
// header.
func (m Model) rows() []row {
	rows := make([]row, 0, len(m.items)+8)
	prev := ""
	for i := range m.items {
		if sec := m.items[i].Section; sec != "" && sec != prev {
			rows = append(rows, row{header: true, title: sec})
			prev = sec
		}
		rows = append(rows, row{itemIdx: i})
	}
	return rows
}

// cursorRow is the display-row index of the highlighted item, or -1 if the menu is
// empty. Used to keep the selection visible when the offset counts header lines.
func (m Model) cursorRow() int {
	for i, r := range m.rows() {
		if !r.header && r.itemIdx == m.cursor {
			return i
		}
	}
	return -1
}

// innerHeight is the number of rows the pane can show (total height minus the top
// and bottom border rows), never negative.
func (m Model) innerHeight() int {
	h := m.height - 2
	if h < 0 {
		return 0
	}
	return h
}

// scrollToCursor adjusts the scroll offset (a display-row offset) so the cursor's
// row is within the visible window. When scrolling up onto the first item of a
// section it also pulls in that section's header so the cursor never sits under an
// off-screen title.
func (m *Model) scrollToCursor() {
	rows := m.rows()
	cr := -1
	for i, r := range rows {
		if !r.header && r.itemIdx == m.cursor {
			cr = i
			break
		}
	}
	if cr < 0 {
		return
	}
	h := m.innerHeight()
	if h == 0 {
		m.offset = cr
		return
	}
	top := cr
	if cr > 0 && rows[cr-1].header {
		top = cr - 1
	}
	if top < m.offset {
		m.offset = top
	} else if cr >= m.offset+h {
		m.offset = cr - h + 1
	}
}

// clampOffset keeps the scroll offset valid after a resize, preferring to keep the
// cursor visible.
func (m *Model) clampOffset() {
	h := m.innerHeight()
	maxOffset := len(m.rows()) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
	m.scrollToCursor()
}

// View renders the menu as a bordered vertical list of section headers and items.
// It returns "" until the menu has been sized (before the first WindowSizeMsg), so
// the root model lays nothing out prematurely.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	// innerW is the width handed to the bordered frame. lipgloss borders are
	// border-box (the frame's total width is innerW, its inner text region is
	// innerW-2), so the actual usable text columns are `region`. Sizing content to
	// region is what keeps a full-width line from being clipped or wrapped past the
	// border — the root of the dogfood-03 overflow.
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	region := innerW - 2
	if region < 0 {
		region = 0
	}
	h := m.innerHeight()
	rows := m.rows()

	// Reserve the rightmost text column for a scrollbar whenever the content
	// overflows the pane; when everything fits, the list uses the full region and no
	// scrollbar is drawn.
	showBar := region > 1 && len(rows) > h
	contentW := region
	if showBar {
		contentW = region - 1
	}

	lines := make([]string, 0, h)
	for r := 0; r < h; r++ {
		i := m.offset + r
		if i >= len(rows) {
			lines = append(lines, m.styles.App.Width(contentW).Render(""))
			continue
		}
		if rows[i].header {
			lines = append(lines, m.renderHeader(rows[i].title, contentW))
			continue
		}
		idx := rows[i].itemIdx
		lines = append(lines, m.renderItem(m.items[idx], idx == m.cursor, contentW))
	}

	content := strings.Join(lines, "\n")
	if showBar {
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, m.scrollbar(h, len(rows)))
	}
	frame := m.styles.Pane
	if m.focused {
		frame = m.styles.PaneFocus
	}
	return frame.Width(innerW).Height(h).Render(content)
}

// scrollbar renders the reserved right-hand column as an h-line track with a
// proportional thumb: the thumb's height is the visible fraction (visible/total)
// and its top is at the same fraction of the scroll range, so its span shows how
// much of the list is on screen and its position shows how much is scrolled off
// above and below. Called only when total > h (an overflowing menu), so the
// division denominators are non-zero.
func (m Model) scrollbar(h, total int) string {
	thumb := h * h / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > h {
		thumb = h
	}
	// Map the current offset (0..total-h) onto the thumb's travel (0..h-thumb).
	pos := 0
	if span, travel := total-h, h-thumb; span > 0 && travel > 0 {
		pos = m.offset * travel / span
		if pos > travel {
			pos = travel
		}
	}
	lines := make([]string, h)
	for r := 0; r < h; r++ {
		if r >= pos && r < pos+thumb {
			lines[r] = m.styles.Spinner.Render(scrollThumb)
		} else {
			lines[r] = m.styles.Subtle.Render(scrollTrack)
		}
	}
	return strings.Join(lines, "\n")
}

// clip truncates s to at most w display columns, appending an ellipsis when it
// had to cut, so a long title stays on one line instead of wrapping outside the
// pane. Kind names are ASCII, so a rune count is an accurate display width here.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return ellipsis
	}
	return string(r[:w-1]) + ellipsis
}

// renderHeader renders a non-selectable section header clamped to innerW, in the
// accented Header style so the grouping reads at a glance.
func (m Model) renderHeader(title string, innerW int) string {
	return m.styles.Header.Width(innerW).MaxWidth(innerW).Render(clip(title, innerW))
}

// renderItem renders one item line clamped to innerW. Two independent states are
// shown (dogfood-05): the **nav cursor** (the highlighted row) takes the Selection
// full-width bar, and the **opened/active** resource (whose table is showing) is
// prefixed with activeMarker ("▸ ") and, when it is not also the cursor, drawn in
// the accented Accent style — so it stays marked as open even while the cursor sits
// elsewhere. When a row is both cursor and active it keeps the Selection bar and
// gains the marker (both states composed). An unavailable item is muted, a plain
// item takes the base style. Non-active items indent under their header ("  ") for
// the tree look; the marker occupies those same two columns so nothing shifts. The
// namespace seam is a non-resource row rendered distinctly (renderNamespace).
func (m Model) renderItem(it Item, selected bool, innerW int) string {
	if it.Kind == ItemNamespace {
		return m.renderNamespace(selected, innerW)
	}
	prefix := itemIndent
	active := m.isActive(it)
	if active {
		prefix = activeMarker
	}
	title := clip(prefix+it.Title, innerW)
	switch {
	case selected:
		return m.styles.Selection.Width(innerW).MaxWidth(innerW).Render(title)
	case active:
		return m.styles.Accent.Width(innerW).MaxWidth(innerW).Render(title)
	case !it.Available:
		return m.styles.Subtle.Width(innerW).MaxWidth(innerW).Render(title)
	default:
		return m.styles.App.Width(innerW).MaxWidth(innerW).Render(title)
	}
}

// renderNamespace renders the namespace-picker seam row: a full-width, un-indented
// line showing the scoped namespace as a dropdown ("▾ (all)" when unscoped,
// "▾ <ns>" when scoped), so it reads as the boundary between the cluster-scoped
// section above and the namespaced sections below rather than as one more resource.
// Selected → the Selection bar; otherwise the accented Header style so the seam
// stands out from resource rows.
func (m Model) renderNamespace(selected bool, innerW int) string {
	ns := m.namespace
	if ns == "" {
		ns = namespaceAll
	}
	label := clip(namespaceArrow+ns, innerW)
	if selected {
		return m.styles.Selection.Width(innerW).MaxWidth(innerW).Render(label)
	}
	return m.styles.Header.Width(innerW).MaxWidth(innerW).Render(label)
}
