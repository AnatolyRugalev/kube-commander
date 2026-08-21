// Package menu is kubecom's resource-menu sidebar: the left pane of the browse
// view, a vertical list of Kubernetes resource kinds the user moves through to
// choose what the table pane shows. The list is grouped into the familiar
// Kubernetes-Dashboard scopes (Cluster / Workloads / Config / Network / Storage /
// Access Control, plus Custom Resources) with non-selectable section headers the
// cursor skips over (D77). The Custom Resources section lists only the CRDs the user
// asked for — a menus/<context>.yaml entry or a pin — since discovery finds hundreds
// on an operator-heavy cluster; the rest stay in the kind inventory (Items(), and so
// the picker, cluster search and pane memory) and reappear in the pane under an
// explicit `/` query, with a trailing row reporting how many are held back (D288). Between the cluster-scoped section
// and the namespaced ones sits a namespace-picker seam row (ItemNamespace): a
// selectable non-resource row that shows the scoped namespace and, on drill-in,
// requests the namespace picker (NamespaceRequestedMsg) — the same effect as the
// `N` shortcut — so the menu itself communicates the cluster/namespaced
// boundary. The seed provides a static list of core resource kinds; Reconcile
// merges the async discovery result (adding CRDs/extra groups, marking unavailable
// ones) without disturbing the current selection or scroll, and skips the
// non-resource seam row. AddExtras folds in per-context menu customizations
// (config.MenuResource, D83) — user-named CRDs for the current context — before
// discovery runs, deduped by GVR so a later discovered twin never double-lists.
// The list is a real viewport: rows are clipped to the
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
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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
//
// Pinned and Discovered are the row's **provenance** (CRD-PIN-03): between them
// they answer the one question Unpin has to ask — if the pin behind this row goes
// away, is there any other reason for the row to exist? Neither is a display flag;
// nothing renders differently because of them.
type Item struct {
	Resource  kube.Resource
	Title     string
	Section   string
	Available bool
	Kind      ItemKind
	// Pinned marks a row that is listed *because* a pin says so: AddPinned inserted
	// it. A pin that merely duplicates a row already present (a seed row, an authored
	// extra, a kind discovery found) inserts nothing and marks nothing, so this flag
	// means "the pin is the reason", not "a pin exists" — which is exactly what makes
	// it safe for Unpin to delete the row it is set on.
	Pinned bool
	// Discovered marks a kind the discovery pass lists: either a row Reconcile
	// matched to a discovered twin, or one it appended. A pinned row that is also
	// discovered survives its unpin as a plain discovered row (CRD-PIN-03).
	Discovered bool
	// Authored marks a row a hand-written menus/<context>.yaml entry put there
	// (AddExtras). Like Pinned it is provenance, not display — but it is the other
	// half of the answer to "did the user ask for this row?", which is what decides
	// whether a Custom Resources row is listed at all (STORY-06l/D288).
	Authored bool
}

// hiddenByDefault reports whether the row is a custom resource the user has not
// asked for: a Custom Resources row that neither a menus/<context>.yaml entry nor a
// pin accounts for. Discovery finds hundreds of these on an operator-heavy cluster
// and listing them all is what made the pane a thing to scroll past rather than read
// (STORY-06l/D288), so the pane leaves them out until a pin opts one in. They stay in
// the authoritative list — Items(), and so the resource picker, cluster search,
// relations and pane memory — and an explicit `/` query in the pane reveals them
// again, so hiding is only ever about the default frame.
func (it Item) hiddenByDefault() bool {
	return it.Kind == ItemResource && it.Section == sectionCustom && !it.Authored && !it.Pinned
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
// picker — the same effect as the ns.switch (`N`) shortcut. Like
// ResourceSelectedMsg it is owned by the menu package so the menu never imports
// the root package (D56).
type NamespaceRequestedMsg struct{}

// Model is the resource menu. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles
	full   []Item // authoritative, unfiltered item list (seed + extras + discovery)
	items  []Item // displayed view: full when no filter, else full narrowed by filter
	filter string // active case-insensitive substring query ("" = show everything)

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
	items := seedItems()
	return Model{styles: s, full: items, items: items}
}

// SetStyles repaints the menu through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Colors only: the item set, the cursor, the scroll offset and the active row are
// untouched.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

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
//
// It re-derives the displayed list: the open kind is always listed, even when it is
// a custom resource the pane would otherwise hold back (D288) — a left pane that
// cannot show what the right pane has open is worse than a long one, and reaching a
// hidden CRD through the resource picker or a search hit is an ordinary way in.
func (m *Model) SetActive(r kube.Resource) {
	m.activeGVR = r.GVR
	m.hasActive = true
	m.reapply()
}

// ClearActive drops the opened/active marker (no resource is open). Kept for
// completeness / future use when a watch is torn down without a replacement.
func (m *Model) ClearActive() {
	m.hasActive = false
	m.activeGVR = schema.GroupVersionResource{}
	m.reapply()
}

// reapply re-derives the displayed list and puts the cursor back on the row it was
// on, for the mutators that change what is *shown* without changing the item set
// (SetActive/ClearActive, which can add or drop the open kind's row under D288).
func (m *Model) reapply() {
	selGVR, selKind := m.selectionRef()
	m.applyFilter()
	m.restoreSelection(selGVR, selKind)
	m.clampOffset()
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

// Items returns the menu's authoritative items in display order — the full set
// the shell's kind inventory (availableResources, the resource picker, pane
// memory) reads, unaffected by the pane's own `/` filter. The narrowed list the
// menu renders stays internal (Selected / View / Filter); Items() returns the
// whole list so a filtered menu never shrinks what a picker or search offers.
func (m Model) Items() []Item { return m.full }

// Selected returns the currently highlighted item and true, or a zero Item and
// false when the menu is empty.
func (m Model) Selected() (Item, bool) {
	if len(m.items) == 0 {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

// SetFilter narrows the menu to the items matching q (case-insensitive substring
// across the item's title and its resource names), preserving the selection when
// the selected item still matches and clamping it into the narrowed range
// otherwise. Passing "" clears the filter and every item reappears. It re-derives
// from the authoritative items each call, so narrowing then widening never loses
// a kind — the STORY-06m mirror of the table's SetFilter.
func (m *Model) SetFilter(q string) {
	if q == m.filter {
		return
	}
	selGVR, selKind := m.selectionRef()
	m.filter = q
	m.applyFilter()
	m.restoreSelection(selGVR, selKind)
	m.clampOffset()
}

// Filter is the active pane filter query ("" when none is set).
func (m Model) Filter() string { return m.filter }

// ClearFilter removes any active filter (equivalent to SetFilter("")).
func (m *Model) ClearFilter() { m.SetFilter("") }

// selectionRef captures the selected item's identity — GVR for a resource row,
// kind for the namespace seam — so a filter change or a mutation can restore it.
func (m Model) selectionRef() (schema.GroupVersionResource, ItemKind) {
	if len(m.items) == 0 {
		return schema.GroupVersionResource{}, 0
	}
	it := m.items[m.cursor]
	return it.Resource.GVR, it.Kind
}

// restoreSelection moves the cursor back onto the item matching the captured
// GVR/kind, if it is still displayed; otherwise it clamps the cursor into the
// narrowed range (moveTo clamps and scrolls, never leaving a gap). The fallback
// is what keeps Unpin's "never at a gap" promise: removing the cursor's own row
// leaves the selection unresolvable, and the cursor must land on a row that still
// exists.
func (m *Model) restoreSelection(selGVR schema.GroupVersionResource, selKind ItemKind) {
	for i := range m.items {
		if m.items[i].Kind != selKind {
			continue
		}
		if selKind != ItemResource || m.items[i].Resource.GVR == selGVR {
			m.cursor = i
			return
		}
	}
	// The captured selection no longer matches. Clamp into the range, or to the
	// only valid cursor when the list is empty (moveTo handles both).
	if len(m.items) == 0 {
		m.moveTo(0)
		return
	}
	if m.cursor >= len(m.items) {
		m.moveTo(len(m.items) - 1)
	}
	if m.cursor < 0 {
		m.moveTo(0)
	}
}

// applyFilter re-derives the pane's displayed item list from the authoritative
// full list under the active filter: with no filter every row but the custom
// resources nobody asked for (hiddenByDefault), else the matching subset — a
// typed query reaches the hidden kinds too, which is what keeps them one `/` away
// rather than gone (STORY-06l/D288).
// Only rows that answer to the query survive — the namespace seam included, since
// it is one more row in the list (a filter that matches its "Namespace" title
// keeps it; any other query hides it with the kinds it narrows past). Callers
// that narrowed the selection restore it via restoreSelection + clampOffset.
func (m *Model) applyFilter() {
	if m.filter == "" {
		out := make([]Item, 0, len(m.full))
		for _, it := range m.full {
			if m.hides(it) {
				continue
			}
			out = append(out, it)
		}
		m.items = out
		return
	}
	q := strings.ToLower(m.filter)
	out := make([]Item, 0, len(m.full))
	for _, it := range m.full {
		if itemMatches(it, q) {
			out = append(out, it)
		}
	}
	m.items = out
}

// itemMatches reports whether an item answers to the filter query: a
// case-insensitive substring of the item's title or any of the names the kind
// answers to (the Kind, the plural resource, its short names, the API group) —
// the same alias surface the resource picker matches (D203), so `deploy` finds
// Deployments and a CRD's short name finds it.
func itemMatches(it Item, q string) bool {
	hay := strings.ToLower(it.Title + " " + it.Resource.GVK.Kind + " " + it.Resource.GVR.Resource)
	for _, sn := range it.Resource.ShortNames {
		hay += " " + strings.ToLower(sn)
	}
	if it.Resource.GVR.Group != "" {
		hay += " " + strings.ToLower(it.Resource.GVR.Group)
	}
	return strings.Contains(hay, q)
}

// RowItemAt maps a content-area row to the item index rendered on it, or false
// when that line is a section header, blank filler, or outside the visible window.
// contentRow is 0-based from the first content line inside the top border (the root
// model converts an absolute mouse Y to it), so it composes with the vertical
// scroll offset: the display row is offset+contentRow. It backs click-to-select/
// open — the root resolves the clicked line to an item, then drives the normal
// SelectItem + drill-in path, so a mouse click and a keyboard drill-in share one
// code path (no coordinate behaviour baked into rendering, D11 in spirit).
func (m Model) RowItemAt(contentRow int) (int, bool) {
	if contentRow < 0 || contentRow >= m.innerHeight() {
		return 0, false
	}
	rows := m.rows()
	i := m.offset + contentRow
	if i < 0 || i >= len(rows) || rows[i].header {
		return 0, false
	}
	return rows[i].itemIdx, true
}

// SelectItem moves the nav cursor to item index i (clamped) and scrolls it into
// view — the public entry the root model uses to place the cursor on a
// mouse-clicked row before drilling in. Keyboard navigation uses the same moveTo.
func (m *Model) SelectItem(i int) { m.moveTo(i) }

// SelectResource moves the nav cursor onto the displayed row for gvr and reports
// whether there was one. It is the by-identity entry the shell uses when it knows a
// kind but not a row number — pane memory restoring the kind a context was left on —
// and it exists because the displayed list is no longer the authoritative one: an
// index into Items() has not addressed a row since the pane started narrowing itself
// (a `/` query, the hidden custom resources of D288), and a GVR still does.
func (m *Model) SelectResource(gvr schema.GroupVersionResource) bool {
	for i := range m.items {
		if m.items[i].Kind == ItemResource && m.items[i].Resource.GVR == gvr {
			m.moveTo(i)
			return true
		}
	}
	return false
}

// HiddenCustom counts the custom-resource rows the pane is leaving out right now:
// discovered CRDs with neither an authored entry nor a pin behind them (D288). It is
// zero whenever a filter is active, since a typed query reaches them. The pane
// reports the count on a trailing row so the reader can tell "this cluster has no
// CRDs" from "kubecom is not showing you 200 of them".
func (m Model) HiddenCustom() int {
	if m.filter != "" {
		return 0
	}
	n := 0
	for _, it := range m.full {
		if m.hides(it) {
			n++
		}
	}
	return n
}

// hides is the pane's decision about one row under no filter: a custom resource
// nobody asked for is left out — unless it is the kind currently open, which the
// left pane must always be able to point at (SetActive).
func (m Model) hides(it Item) bool {
	return it.hiddenByDefault() && !m.isActive(it)
}

// AddExtras merges per-context menu customizations (config.MenuResource entries,
// D83) into the menu: extra resource kinds — chiefly CRDs the built-in seed does
// not know — that a user has named for the current kubeconfig context. Each entry
// is mapped to an Item (extraItem) and inserted into its section, keeping the
// section contiguous so the D77 one-header-per-section grouping still holds. It is
// additive and idempotent-safe by GVR: an extra whose GVR already exists among the
// items (a seed row, an earlier extra, or — if AddExtras is called after a
// discovery pass — a discovered row) is skipped, so it never double-lists a
// resource the menu already shows. Because the extras land in m.items before the
// first Reconcile, dedup against a *later* discovered twin is automatic: Reconcile
// keys its "already listed" set off every current ItemResource (D57), so a
// discovered resource matching an extra fills the extra's twin metadata rather than
// appending a duplicate. The current selection is preserved by resolving it back to
// its post-insert index, mirroring Reconcile. This is the component-level merge
// (FB-menu-config-02); wiring the per-context file into the app is a later slice.
func (m *Model) AddExtras(extras []config.MenuResource) { m.addExtras(extras, false) }

// AddPinned merges the kinds pinned for this context (State.PinnedResources, D193)
// exactly as AddExtras merges the authored ones — same insertion, same GVR dedupe —
// and marks the rows it *inserts* as Pinned so Unpin can tell which rows exist only
// because of a pin (CRD-PIN-03).
//
// Call it after AddExtras, which is what gives the authored file precedence (D193
// pt 3) without a merge step upstream: a pin naming a GVR the authored list already
// placed is skipped here, so the authored row keeps its title and section — and,
// being unmarked, is not something `*` may remove. The same dedupe makes a pin on a
// seed row inert: the seed row stays, unmarked, and outlives the pin.
func (m *Model) AddPinned(pins []config.MenuResource) { m.addExtras(pins, true) }

// addExtras is the shared merge behind AddExtras/AddPinned; pinned tags the rows it
// inserts (and only those) as Pinned, and an unpinned merge tags them Authored — the
// two provenances that keep a Custom Resources row listed (D288).
func (m *Model) addExtras(extras []config.MenuResource, pinned bool) {
	if len(extras) == 0 {
		return
	}

	// Remember the highlighted item so the cursor can be restored to it after the
	// item slice grows. Resource rows resolve back by GVR; the namespace seam by kind.
	selGVR, selKind := m.selectionRef()

	// Track the GVRs already present so an extra duplicating a seed row (or an
	// earlier extra) is dropped rather than listed twice.
	seen := make(map[schema.GroupVersionResource]bool, len(m.full)+len(extras))
	for i := range m.full {
		if m.full[i].Kind == ItemResource {
			seen[m.full[i].Resource.GVR] = true
		}
	}

	for _, e := range extras {
		it := extraItem(e)
		if seen[it.Resource.GVR] {
			// The kind is already listed. A pin over a row the pane hides — a
			// discovered CRD — is the gesture that opts it in, so it marks that row
			// rather than inserting a second one (STORY-06l/D288). A pin over any other
			// row (a seed kind, an authored entry) stays inert exactly as before: the
			// row is shown regardless, and marking it would let an unpin take it away.
			if pinned {
				m.markPinned(it.Resource.GVR)
			}
			continue
		}
		it.Pinned = pinned
		it.Authored = !pinned
		seen[it.Resource.GVR] = true
		m.full = insertExtra(m.full, it)
	}

	// Re-derive the displayed list from the authoritative one, then restore the
	// selection within it (a kind filtered out of view stays filtered out; the
	// cursor falls back onto the narrowed range).
	m.applyFilter()
	m.restoreSelection(selGVR, selKind)
	m.clampOffset()
}

// markPinned records the pin behind a row that is already listed but hidden — the
// discovered-CRD case addExtras cannot insert for, since a second row for the same
// GVR is exactly what the dedupe exists to prevent. Only a row the pane would
// otherwise leave out is marked, so Unpin's "a row a pin put there" rule keeps
// naming rows whose visibility the pin really does account for.
func (m *Model) markPinned(gvr schema.GroupVersionResource) {
	for i := range m.full {
		if m.full[i].Resource.GVR == gvr && m.full[i].hiddenByDefault() {
			m.full[i].Pinned = true
			return
		}
	}
}

// extraItem maps one per-context config.MenuResource to a menu Item. The title
// falls back Title → Kind → Resource so an entry that names only a resource still
// renders a legible row; the section defaults to the trailing Custom Resources
// bucket (where discovered CRDs also land) when the entry does not name one. The
// item is marked Available like a seed row — optimistically selectable before
// discovery — so Reconcile marks it unavailable only if its group actually fails
// discovery.
func extraItem(e config.MenuResource) Item {
	section := e.Section
	if section == "" {
		section = sectionCustom
	}
	title := e.Title
	if title == "" {
		title = e.Kind
	}
	if title == "" {
		title = e.Resource
	}
	return Item{
		Resource: kube.Resource{
			GVK:        schema.GroupVersionKind{Group: e.Group, Version: e.Version, Kind: e.Kind},
			GVR:        schema.GroupVersionResource{Group: e.Group, Version: e.Version, Resource: e.Resource},
			Namespaced: e.Namespaced,
		},
		Title:     title,
		Section:   section,
		Available: true,
		Kind:      ItemResource,
	}
}

// insertExtra inserts it into items so its section stays contiguous (the D77
// invariant rows() relies on to emit one header per section): after the last
// existing item of the same section, or appended at the end — which starts a new
// contiguous section — when no item of that section exists yet. The namespace seam
// (Section "") never matches an extra's section, so the seam is never split.
func insertExtra(items []Item, it Item) []Item {
	last := -1
	for i := range items {
		if items[i].Kind == ItemResource && items[i].Section == it.Section {
			last = i
		}
	}
	if last < 0 {
		return append(items, it)
	}
	out := make([]Item, 0, len(items)+1)
	out = append(out, items[:last+1]...)
	out = append(out, it)
	out = append(out, items[last+1:]...)
	return out
}

// Unpin drops the pin behind the row for gvr and reports whether the row went with
// it (CRD-PIN-03). It is the inverse of AddPinned and it is deliberately narrow:
//
//   - a row inserted by a pin that discovery does **not** list is removed — the pin
//     was the only reason it was there;
//   - a row inserted by a pin that discovery *does* list keeps its place and merely
//     loses the marker: unpinning means "stop keeping this for me", not "hide a kind
//     this cluster has" (the board's revert-to-a-discovered-row rule);
//   - a row no pin inserted — a seed row, an authored entry (D193 pt 3), a purely
//     discovered row — is left entirely alone, whatever the state file says. A pin
//     recorded over such a row never rendered anything, so removing it must not
//     remove a row either.
//
// The cursor follows the same rule the rest of the menu does — it keeps pointing at
// a row, never at a gap: it steps back one when the removal happened at or before
// it, and moveTo re-clamps and re-scrolls. The active marker is keyed by GVR
// (SetActive), so a row removed while its table is open simply stops matching; the
// table and its watch are the root model's business and are untouched here.
func (m *Model) Unpin(gvr schema.GroupVersionResource) bool {
	idx := -1
	for i := range m.full {
		if m.full[i].Kind == ItemResource && m.full[i].Pinned && m.full[i].Resource.GVR == gvr {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	selGVR, selKind := m.selectionRef()
	// The removed row's position in the *displayed* list, so the cursor can step
	// back onto the row that slid into its place when it was the selection.
	removedDisplayed := -1
	for i := range m.items {
		if m.items[i].Kind == ItemResource && m.items[i].Resource.GVR == gvr {
			removedDisplayed = i
			break
		}
	}
	if m.full[idx].Discovered {
		m.full[idx].Pinned = false
		// The kind stays in the inventory, but a custom-resource row with no pin
		// behind it is no longer listed in the pane (D288), so the displayed list is
		// re-derived exactly as for a real removal.
		m.rederive(selGVR, selKind, gvr, removedDisplayed)
		return false
	}
	m.full = append(m.full[:idx], m.full[idx+1:]...)
	m.rederive(selGVR, selKind, gvr, removedDisplayed)
	return true
}

// rederive is Unpin's shared tail: re-narrow the displayed list, put the cursor back
// on the row it was on, and keep the "never at a gap" rule — a row that left the
// displayed list at or before the cursor takes the cursor back one. restoreSelection
// resolves by GVR, so the step-back only fires when the departed row *was* the
// selection.
func (m *Model) rederive(selGVR schema.GroupVersionResource, selKind ItemKind,
	gone schema.GroupVersionResource, goneDisplayed int) {
	m.applyFilter()
	stillShown := false
	for i := range m.items {
		if m.items[i].Kind == ItemResource && m.items[i].Resource.GVR == gone {
			stillShown = true
			break
		}
	}
	m.restoreSelection(selGVR, selKind)
	if !stillShown && selGVR == gone && selKind == ItemResource && goneDisplayed >= 0 {
		m.moveTo(goneDisplayed - 1)
	}
	m.clampOffset()
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
//     extra groups), in discovery's stable sorted order, after the seed. They join
//     the authoritative list; whether the pane *shows* one is applyFilter's business
//     (D288) — an appended row with no pin or authored entry behind it is held back.
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
	selGVR, selKind := m.selectionRef()

	// Index the discovered resources by GVR (twin lookup) and collect the groups
	// that failed discovery (the mark-unavailable signal). A failure names a group
	// and, when the server said so, a version; we key on the group so a seed item
	// pinned to a version that differs from the failed one is still recognised as
	// unreachable — and so a failure known only at group granularity (D187) marks
	// its kinds too.
	twin := make(map[schema.GroupVersionResource]kube.Resource, len(result.Resources))
	for _, r := range result.Resources {
		twin[r.GVR] = r
	}
	failedGroups := make(map[string]bool, len(result.Failed))
	for _, f := range result.Failed {
		failedGroups[f.Group] = true
	}

	// Reconcile the seed items in place, preserving their order and title.
	seen := make(map[schema.GroupVersionResource]bool, len(m.full))
	for i := range m.full {
		if m.full[i].Kind != ItemResource {
			continue // non-resource seam rows (namespace picker) have no twin.
		}
		gvr := m.full[i].Resource.GVR
		seen[gvr] = true
		if d, ok := twin[gvr]; ok {
			// A loaded twin: fill the discovery metadata and confirm availability. The
			// row is now backed by the cluster's own list, which is what lets an unpin
			// leave it standing (CRD-PIN-03) — a pinned row that turns out to be
			// discovered too is no longer held up by its pin alone.
			m.full[i].Resource = d
			m.full[i].Available = true
			m.full[i].Discovered = true
		} else if failedGroups[gvr.Group] {
			m.full[i].Available = false
		}
	}

	// Append discovered resources the seed does not already list (CRDs, extra
	// groups), in discovery's stable order, into the trailing Custom Resources
	// section so the grouping survives discovery (D77).
	for _, r := range result.Resources {
		if seen[r.GVR] {
			continue
		}
		m.full = append(m.full, Item{
			Resource:   r,
			Title:      r.GVK.Kind,
			Section:    sectionCustom,
			Available:  true,
			Discovered: true,
		})
	}

	// Re-derive the displayed list from the authoritative one, then restore the
	// selection to the same resource and re-clamp the scroll. The seed is never
	// reordered or prepended to, so the index is stable; resolving by GVR keeps
	// the guarantee robust regardless.
	m.applyFilter()
	m.restoreSelection(selGVR, selKind)
	m.clampOffset()
}

// Update handles a resolved keymap action. Navigation actions (up/down/top/
// bottom, and the half-page/page steps) move the highlight and keep it visible;
// nav.drillIn emits a ResourceSelectedMsg for the highlighted item so the root
// model can start its watch. Any other action is ignored (the root routes it
// elsewhere). The menu consumes actions, never raw keys (D11).
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
	case keymap.ActionHalfPageDown:
		m.moveByRows(m.halfPageStep())
	case keymap.ActionHalfPageUp:
		m.moveByRows(-m.halfPageStep())
	case keymap.ActionPageDown:
		m.moveByRows(m.pageStep())
	case keymap.ActionPageUp:
		m.moveByRows(-m.pageStep())
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

// pageStep is the distance a full page scroll travels, measured in **display
// rows** rather than items: the menu's window math counts section headers, so a
// page that stepped by item index would overshoot a section-dense stretch of the
// list by exactly the headers it skipped. It is the pane's visible height, or 1
// when the pane is too short to show anything (so a page step still advances) —
// the same floor the table's pageStep takes.
func (m Model) pageStep() int {
	if h := m.innerHeight(); h > 0 {
		return h
	}
	return 1
}

// halfPageStep is the half-page distance, likewise in display rows, floored at 1
// so ctrl+d/ctrl+u still move in a one- or two-row pane instead of silently doing
// nothing (a full page already floors at 1 the same way).
func (m Model) halfPageStep() int {
	if s := m.pageStep() / 2; s > 0 {
		return s
	}
	return 1
}

// moveByRows moves the cursor delta display rows from where it currently renders,
// then lands it on the nearest **selectable** row — section headers occupy screen
// lines but the cursor never sits on one (D77), so a paged-to header is resolved to
// the next item in the direction of travel, or the nearest one back the other way
// when the clamp lands past the last item. Delegating the final placement to moveTo
// keeps the clamping and scrollToCursor behaviour identical to every other move.
func (m *Model) moveByRows(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	cr := m.cursorRow()
	if cr < 0 {
		return
	}
	target := cr + delta
	if target < 0 {
		target = 0
	}
	if target > len(rows)-1 {
		target = len(rows) - 1
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for i := target; i >= 0 && i < len(rows); i += step {
		if !rows[i].header {
			m.moveTo(rows[i].itemIdx)
			return
		}
	}
	for i := target; i >= 0 && i < len(rows); i -= step {
		if !rows[i].header {
			m.moveTo(rows[i].itemIdx)
			return
		}
	}
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
	// The hidden-custom-resources footer (D288). It is a header row, which is to say
	// non-selectable and skipped by the cursor, so it costs the navigation nothing and
	// answers the one question hiding raises: what am I not being shown, and how do I
	// get at it.
	if n := m.HiddenCustom(); n > 0 {
		rows = append(rows, row{header: true, title: fmt.Sprintf("+%d custom · / to find", n)})
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
