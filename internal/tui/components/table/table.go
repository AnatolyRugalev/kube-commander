// Package table is kubecom's resource table: the right pane of the browse view,
// the live-updating list of objects for the resource chosen in the menu. It
// renders a server-printed kube.Table — kubectl-identical columns for any
// resource, built-ins and CRDs alike — with a header row, vertical scroll, and a
// highlighted selection.
//
// SetTable installs a static kube.Table snapshot (M2-06a): the user moves the
// selection through keymap actions, and drilling in emits a RowSelectedMsg.
// ApplyEvent (M2-06b) folds a live watch delta onto that snapshot — RESET replaces
// the column set and rows, ADDED/MODIFIED/DELETED touch a single row keyed by
// object UID — always preserving the selection by re-resolving the selected UID to
// its new index. A table wider than the pane scrolls horizontally on nav.left/
// nav.right, snapping to column boundaries (M2-06c/D60).
//
// SetFilter (M2-09a) narrows the displayed rows to a case-insensitive substring
// match across the visible columns. Filtering is a view over an authoritative,
// unfiltered row set: watch deltas keep updating every row, and clearing the
// filter brings them all back. The root model owns triggering it from the keymap
// filter action and a text field (M2-09b); this package only narrows.
//
// The table never matches a raw key (D11): the root model resolves a KeyMsg to a
// keymap.Action and hands it to Update. Like the menu, the table owns no shared
// mutable state (principle 1) and owns its own emitted message type — RowSelectedMsg
// lives here, not in package tui, so the component does not import the root package
// (which imports it) and cause a cycle (D56).
package table

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/safetext"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// colGap is the spacing between rendered columns, matching `kubectl get`'s
// two-space column separation.
const colGap = "  "

// RowSelectedMsg is emitted when the user drills into the highlighted row
// (nav.drillIn). The root model reacts by opening the row's object (describe /
// logs / YAML in later milestones). It is owned by the table package — the
// emitter — so the table never imports the root package (D56).
type RowSelectedMsg struct {
	Row kube.Row
}

// Model is the resource table. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles

	// full is the authoritative, unfiltered row set — every row the watch has
	// delivered. table is the displayed view: full when no filter is set, else
	// full narrowed to the rows matching filter. All rendering, navigation, and
	// selection operate on table (the visible rows); watch deltas (ApplyEvent)
	// mutate full and re-derive table via applyFilter. filter is the active
	// case-insensitive substring query ("" = show everything). Keeping the two
	// separate means a narrowing filter never loses rows from the live set: clear
	// it and every row reappears (M2-09a).
	full   kube.Table
	table  kube.Table
	filter string

	// sortCol is the visible-column position (index into visible) the displayed
	// rows are sorted on, or -1 for the unsorted, authoritative watch order.
	// sortDesc flips the direction. Sorting, like filtering, is a view over the
	// authoritative full set re-derived by applyFilter — watch deltas keep
	// flowing and re-sort in place, ClearSort restores the watch order, and
	// SetTable resets it (a sort chosen for one resource's columns must not carry
	// to a different resource). Selection is preserved by object UID across a
	// re-sort.
	sortCol  int
	sortDesc bool

	// visible holds the indices (into table.Columns) of the columns shown, and
	// colWidths their rendered widths — both derived from the snapshot in
	// SetTable so View stays a pure render.
	visible   []int
	colWidths []int

	// usage is the metrics overlay (M4-10): point-in-time CPU/memory samples for
	// the browsed kind, keyed by namespace/name, or nil when the cluster does not
	// measure this kind. It is deliberately *not* folded into full.Rows — those
	// are the watch's and are replaced by every RESET — so the two displayed
	// columns it adds are derived alongside the filter and the sort in
	// applyFilter. See usage.go.
	usage map[kube.UsageKey]kube.Usage

	// notice is the reason there is nothing to show, rendered *in place of* the
	// empty body when the table holds no rows (CRD-01). A LIST that fails leaves
	// the pane blank forever — the watch loop retries behind a 5-second toast that
	// is gone by the time the reader looks up — so the pane itself has to carry the
	// reason. Its first line is the headline (Error style) and the rest is detail
	// (Subtle); the embedder composes the text, this package only lays it out.
	// It is shown only while there are no rows: rows on screen are the answer, and
	// a stale reason must never cover them. SetTable and a RESET both clear it —
	// a new snapshot, or a List that finally succeeded, is the end of the failure.
	notice string

	cursor  int // index of the highlighted row
	offset  int // index of the first visible row (vertical scroll)
	hoffset int // first visible display column (horizontal scroll)
	width   int // total width incl. border
	height  int // total height incl. border
	focused bool
}

// New builds an empty table rendered through the given styles. It starts in the
// unsorted state (sortCol -1): rows are shown in the authoritative watch order
// until SortBy is called.
func New(s styles.Styles) Model {
	return Model{styles: s, sortCol: -1}
}

// SetStyles repaints the table through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Colors only: the row set, the filter, the sort, the metrics overlay and the
// selection all survive, and the column widths need no recompute because they are
// derived from the data rather than from the palette.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// SetTable installs a new snapshot, recomputing the visible columns and their
// widths and resetting the selection to the first row. It is the deliberate
// reset entry point (a fresh List for a newly selected resource starts at the
// top): the filter is cleared too, since a filter typed against the previous
// resource must not silently hide rows of a different one. Live watch deltas that
// must preserve the selection (and any active filter) go through ApplyEvent.
func (m *Model) SetTable(t kube.Table) {
	m.full = t
	m.filter = ""
	m.sortCol = -1
	m.sortDesc = false
	// Samples measure the objects of the resource being left, and the overlay's
	// availability is that kind's answer — both are wrong for the new snapshot, so
	// the overlay is dropped here exactly as the filter and the sort are. The shell
	// re-installs it for the new kind if the cluster measures it (M4-10).
	m.usage = nil
	m.notice = "" // a fresh snapshot supersedes whatever the last one failed with.
	m.applyFilter()
	m.cursor = 0
	m.offset = 0
	m.hoffset = 0
	m.clampOffset()
}

// SetNotice records why the table has nothing to show, to be rendered in the
// body while it holds no rows (CRD-01). text's first line is the headline and
// any further lines are detail; both wrap to the pane width. Passing "" clears
// it, exactly as ClearNotice does.
//
// Setting a notice never hides rows: an error that arrives while a populated
// table is on screen (a watch that drops after a good List) leaves the rows
// visible and the notice dormant, ready for the moment the rows go away. That is
// why it is a separate field rather than a substitute row set.
func (m *Model) SetNotice(text string) { m.notice = text }

// ClearNotice drops the recorded reason. The embedder calls it when the
// condition behind it is gone; SetTable and a RESET delta clear it on their own.
func (m *Model) ClearNotice() { m.notice = "" }

// Notice returns the recorded reason ("" when none). For the embedder and tests
// — the rendering is View's business.
func (m Model) Notice() string { return m.notice }

// SetFilter narrows the displayed rows to those matching q (case-insensitive
// substring across the visible columns' cells), preserving the selection by
// object UID: if the selected row still matches, the cursor follows it; if the
// filter hides it, the cursor clamps into the narrowed range. Passing "" clears
// the filter and every row reappears. It re-derives from the authoritative full
// set each call, so narrowing then widening never loses rows.
func (m *Model) SetFilter(q string) {
	if q == m.filter {
		return
	}
	selUID := m.selectedUID()
	m.filter = q
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
	m.clampHOffset()
}

// ClearFilter removes any active filter (equivalent to SetFilter("")).
func (m *Model) ClearFilter() { m.SetFilter("") }

// Filter is the active filter query ("" when none is set).
func (m Model) Filter() string { return m.filter }

// SortBy sets the sort column to the visible-column position col (an index into
// the visible columns — what the user sees and navigates) and applies a stable,
// type-aware sort to the displayed rows. Calling it again with the same column
// toggles the direction (ascending → descending); a different column starts
// ascending. An out-of-range col is ignored. The selection is preserved by
// object UID, and the sort is a view over the authoritative set: watch deltas
// keep flowing and re-sort in place, and ClearSort restores the watch order.
func (m *Model) SortBy(col int) {
	if col < 0 || col >= len(m.visible) {
		return
	}
	selUID := m.selectedUID()
	if m.sortCol == col {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortCol = col
		m.sortDesc = false
	}
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
}

// ClearSort restores the authoritative (watch-delivered) row order, preserving
// the selection by object UID. A no-op when the table is already unsorted.
func (m *Model) ClearSort() {
	if m.sortCol < 0 {
		return
	}
	selUID := m.selectedUID()
	m.sortCol = -1
	m.sortDesc = false
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
}

// SortColumn returns the visible-column position currently sorted on and true,
// or false when the table is in its unsorted (watch) order. It backs the header
// sort indicator the app wiring (M2-13b) will render.
func (m Model) SortColumn() (int, bool) {
	if m.sortCol < 0 {
		return 0, false
	}
	return m.sortCol, true
}

// SortDescending reports whether the active sort is descending (false when
// unsorted or ascending).
func (m Model) SortDescending() bool { return m.sortCol >= 0 && m.sortDesc }

// VisibleColumnCount is the number of columns currently shown (the priority-0 set,
// or every column when the server sent none). It backs the app's sort-column cycle
// (M2-13b): the shell steps SortBy across [0, VisibleColumnCount) then ClearSort,
// so it needs to know how many visible columns there are without reaching into the
// component's internals.
func (m Model) VisibleColumnCount() int { return len(m.visible) }

// applyFilter re-derives the displayed table from the authoritative full set and
// the current filter, applies the active sort, then measures the column widths
// from the resulting rows. The visible column set depends only on the columns
// (priority-0), so it is selected first and reused as the match scope. Sorting
// runs after filtering (only the visible rows are ordered) and, like the filter,
// is re-applied on every derivation so it survives watch deltas. Callers restore
// the selection and re-clamp the scroll afterwards.
func (m *Model) applyFilter() {
	m.table.Columns = m.columnsWithUsage()
	m.selectVisible()
	// The candidate rows carry the metrics overlay's cells when it is on (usage.go),
	// so everything below — the filter's match scope, the sort, the measured widths —
	// sees the same columns the reader does.
	candidates := m.rowsWithUsage()
	if m.filter == "" {
		m.table.Rows = candidates
	} else {
		needle := strings.ToLower(m.filter)
		rows := make([]kube.Row, 0, len(candidates))
		for _, r := range candidates {
			if m.rowMatches(r, needle) {
				rows = append(rows, r)
			}
		}
		m.table.Rows = rows
	}
	m.sortRows()
	m.measureWidths()
}

// sortRows stably orders the displayed rows in place by the active sort column,
// a no-op in the unsorted state (sortCol < 0) or when the column is out of range
// — leaving the authoritative watch order. Integer/number columns compare
// numerically (falling back to text when a cell doesn't parse); every other
// column type compares case-insensitively as text. sort.SliceStable keeps rows
// with equal keys in their existing (watch-delivered) order, in both directions.
func (m *Model) sortRows() {
	if m.sortCol < 0 || m.sortCol >= len(m.visible) {
		return
	}
	ci := m.visible[m.sortCol]
	// A metrics overlay column sorts on the raw sample, not on its formatted cell
	// ("128Mi" as text orders 100m below 20m). usage.go owns that key.
	if key, ok := m.usageSortKey(ci); ok {
		sort.SliceStable(m.table.Rows, func(i, j int) bool {
			a, b := key(m.table.Rows[i]), key(m.table.Rows[j])
			if a == b {
				return false
			}
			if m.sortDesc {
				return a > b
			}
			return a < b
		})
		return
	}
	numeric := isNumericColumn(m.table.Columns[ci].Type)
	sort.SliceStable(m.table.Rows, func(i, j int) bool {
		c := compareCells(
			formatCell(cellAt(m.table.Rows[i].Cells, ci)),
			formatCell(cellAt(m.table.Rows[j].Cells, ci)),
			numeric,
		)
		if c == 0 {
			return false
		}
		if m.sortDesc {
			return c > 0
		}
		return c < 0
	})
}

// rowMatches reports whether any of the row's visible cells contains needle
// (which the caller has already lower-cased) as a case-insensitive substring.
// Only the visible (priority-0) columns are searched, so the filter matches what
// the user can actually see, not hidden -o-wide extras.
func (m Model) rowMatches(r kube.Row, needle string) bool {
	for _, ci := range m.visible {
		if strings.Contains(strings.ToLower(formatCell(cellAt(r.Cells, ci))), needle) {
			return true
		}
	}
	return false
}

// ApplyEvent folds one live watch delta (delivered as a ResourceEventMsg carrying
// a kube.WatchEvent) onto the current snapshot, preserving the selection by object
// UID. A RESET replaces the columns and the whole row set — the watch layer emits
// it on the first sync and again on every reconnect, so preserving the selected
// UID across it keeps the cursor put through a transient reconnect (D59), falling
// back to the first row when the previously selected object is gone. ADDED and
// MODIFIED upsert a row keyed by ObjectRef.UID (unknown UID → appended, matching
// the server treating a modify of an unseen object as an add); DELETED removes the
// matching row. A watch ERROR never reaches here (the pump bridges it to an
// ErrorMsg), and any other event type is ignored. After the change the visible
// columns and widths are recomputed (a new or wider cell can widen a column) and
// the scroll is re-clamped so the selection stays visible.
func (m *Model) ApplyEvent(ev kube.WatchEvent) {
	selUID := m.selectedUID()
	switch ev.Type {
	case kube.WatchReset:
		m.full = kube.Table{Columns: ev.Columns, Rows: ev.Rows}
		// A RESET is a List that succeeded — including the re-List the watch loop
		// runs after a failure — so whatever reason the pane was carrying is over,
		// even when the successful List returned no rows (CRD-01).
		m.notice = ""
	case kube.WatchAdded, kube.WatchModified:
		for _, r := range ev.Rows {
			m.upsertRow(r)
		}
	case kube.WatchDeleted:
		for _, r := range ev.Rows {
			m.deleteRow(r.Object.UID)
		}
	default:
		return
	}
	// Deltas land on the authoritative full set; re-derive the displayed (possibly
	// filtered) view. An active filter is preserved across the delta (unlike
	// SetTable): a watch reconnect emits a fresh RESET, and the user's filter must
	// survive it (D59 keeps the selection; the filter is part of that continuity).
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
	m.clampHOffset()
}

// RefreshAges re-derives the AGE column from each row's creation timestamp
// against now, reporting whether anything changed (AGE-01/D234). The embedder
// calls it on a clock tick: every cell here is a string the server printed once,
// so an untouched row's age is frozen at the moment of its last watch delta, and
// a pane left open drifts stale for as long as it is left open.
//
// It re-derives the whole view rather than only rewriting cells, because a wider
// age ("9h" → "10h", "59m" → "1h2m") has to widen the column — otherwise padRight
// pads past the measured width and shifts every column to its right on that one
// row. The re-derivation is skipped entirely when no cell moved, which is the
// common tick; the selection, scroll and filter survive it exactly as they do a
// watch delta.
func (m *Model) RefreshAges(now time.Time) bool {
	if !m.full.RefreshAge(now) {
		return false
	}
	selUID := m.selectedUID()
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
	m.clampHOffset()
	return true
}

// selectedUID is the object UID of the highlighted row, or "" when the table is
// empty. Captured before a delta is applied so the selection can be re-resolved
// against the new row set afterwards.
func (m Model) selectedUID() string {
	if len(m.table.Rows) == 0 {
		return ""
	}
	return m.table.Rows[m.cursor].Object.UID
}

// indexOfUID returns the index of the row with the given object UID in rows and
// true, or false when it is absent. An empty UID never matches: a degraded row
// with no object metadata (principle 3) cannot be identified, so it is never
// collapsed with another empty-UID row.
func indexOfUID(rows []kube.Row, uid string) (int, bool) {
	if uid == "" {
		return 0, false
	}
	for i, r := range rows {
		if r.Object.UID == uid {
			return i, true
		}
	}
	return 0, false
}

// upsertRow replaces the row with r's UID in the authoritative full set in place,
// or appends r when its UID is absent (or empty — an unidentifiable row is always
// appended rather than merged). Deltas always touch full, never the filtered view.
func (m *Model) upsertRow(r kube.Row) {
	if i, ok := indexOfUID(m.full.Rows, r.Object.UID); ok {
		m.full.Rows[i] = r
		return
	}
	m.full.Rows = append(m.full.Rows, r)
}

// deleteRow removes the row with the given UID from the authoritative full set,
// if present.
func (m *Model) deleteRow(uid string) {
	if i, ok := indexOfUID(m.full.Rows, uid); ok {
		m.full.Rows = append(m.full.Rows[:i], m.full.Rows[i+1:]...)
	}
}

// restoreSelection re-points the cursor at the row that held UID before the delta.
// If that row is gone (deleted, or the selection was empty), the cursor keeps its
// index position — clamped to the new row range — so the selection stays near
// where it was rather than jumping to the top. An empty table resets to row 0.
func (m *Model) restoreSelection(uid string) {
	if len(m.table.Rows) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if i, ok := indexOfUID(m.table.Rows, uid); ok {
		m.cursor = i
	} else if m.cursor > len(m.table.Rows)-1 {
		m.cursor = len(m.table.Rows) - 1
	}
}

// RowAt maps a content-area row to the data-row index rendered on it, or false
// when that line is the column header, blank filler, or outside the visible window.
// contentRow is 0-based from the first content line inside the top border (the root
// model converts an absolute mouse Y to it): content row 0 is the column header, so
// data rows start at content row 1 and index offset+contentRow-1. It backs
// click-to-select — the root resolves the clicked line to a row, then drives
// SelectRow, so a mouse click reuses the keyboard selection path.
func (m Model) RowAt(contentRow int) (int, bool) {
	if contentRow <= 0 || contentRow >= m.innerHeight() {
		return 0, false // row 0 is the header; >= innerHeight is the bottom border.
	}
	i := m.offset + contentRow - 1
	if i < 0 || i >= len(m.table.Rows) {
		return 0, false
	}
	return i, true
}

// SelectRow moves the selection to data-row index i (clamped) and scrolls it into
// view — the public entry the root model uses for a mouse click on a row. Keyboard
// navigation uses the same moveTo.
func (m *Model) SelectRow(i int) { m.moveTo(i) }

// SelectObject moves the selection to the displayed row holding the object ref
// identifies — matched on namespace + name, the identity a caller that did not read
// this table's rows can supply (a cluster-search hit, SEARCH-02b) — and reports
// whether such a row is currently displayed. It matches on name rather than UID so a
// recreated object still resolves, and searches the *displayed* rows, so a row hidden
// by an active filter is not selectable (the selection must always be something the
// reader can see). Not found leaves the selection untouched, so a caller can retry as
// more of a live watch's rows arrive.
func (m *Model) SelectObject(ref kube.ObjectRef) bool {
	for i, r := range m.table.Rows {
		if r.Object.Name == ref.Name && r.Object.Namespace == ref.Namespace {
			m.moveTo(i)
			return true
		}
	}
	return false
}

// SetSize sets the table's total size (including its border).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
	m.clampHOffset()
}

// Focus marks the table as holding focus (accented border).
func (m *Model) Focus() { m.focused = true }

// Blur marks the table as not holding focus.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the table holds focus.
func (m Model) Focused() bool { return m.focused }

// Cursor is the index of the highlighted row (for tests / the root model).
func (m Model) Cursor() int { return m.cursor }

// RowCount is the number of rows currently displayed — the whole snapshot when no
// filter is set, or the matching rows when one is. TotalRowCount is the unfiltered
// count.
func (m Model) RowCount() int { return len(m.table.Rows) }

// TotalRowCount is the number of rows in the authoritative unfiltered set,
// regardless of any active filter. With no filter it equals RowCount; with one it
// is the denominator for a "matched / total" indicator (M2-09b).
func (m Model) TotalRowCount() int { return len(m.full.Rows) }

// HOffset is the first visible display column — the horizontal scroll position
// (for tests and, later, the root model's pane-focus-vs-scroll arbitration in
// M2-07b, which can compare it before/after a left/right to detect an edge).
func (m Model) HOffset() int { return m.hoffset }

// SelectedRow returns the highlighted row and true, or a zero Row and false when
// the table is empty.
func (m Model) SelectedRow() (kube.Row, bool) {
	if len(m.table.Rows) == 0 {
		return kube.Row{}, false
	}
	return m.table.Rows[m.cursor], true
}

// selectVisible chooses the visible columns from the current snapshot. Only
// priority-0 columns are shown by default, matching `kubectl get`'s narrow view
// (higher-priority columns are the `-o wide` extras); if the server sends no
// priority-0 column, every column is shown rather than rendering a blank table
// (degrade, don't blank — principle 3). The choice depends only on the columns,
// not the rows, so applyFilter can select the visible set before narrowing and
// reuse it as the filter's match scope.
// The metrics overlay's columns (usage.go) are excluded from the priority scan and
// appended afterwards: they are always shown when the overlay is on, and counting
// them as priority-0 columns would defeat the "server sent none" fallback above —
// a server table whose columns are all -o-wide extras would then show *only* the
// two usage columns.
func (m *Model) selectVisible() {
	m.visible = m.visible[:0]
	server := m.table.Columns
	if m.hasUsage() {
		server = server[:len(server)-usageColumnCount]
	}
	for i, c := range server {
		if c.Priority == 0 {
			m.visible = append(m.visible, i)
		}
	}
	if len(m.visible) == 0 {
		for i := range server {
			m.visible = append(m.visible, i)
		}
	}
	for i := len(server); i < len(m.table.Columns); i++ {
		m.visible = append(m.visible, i)
	}
}

// sortIndicator is the header suffix marking the sorted column: a leading space
// then an arrow for the direction (ascending ▲ / descending ▼). Empty for any
// other column. sortIndicatorWidth is its display width, reserved in the sorted
// column's measured width so the arrow never overflows the column and misaligns
// the data rows below it.
const (
	sortAscMark        = " ▲"
	sortDescMark       = " ▼"
	sortIndicatorWidth = 2
)

// measureWidths sizes each visible column to the widest of its header and the
// cell values across the currently displayed rows. The sorted column additionally
// reserves room for its header sort indicator (M2-13b), so appending the arrow in
// renderHeader never pushes the header past the measured width and shifts the
// columns to its right.
func (m *Model) measureWidths() {
	m.colWidths = make([]int, len(m.visible))
	for i, ci := range m.visible {
		m.colWidths[i] = runeLen(m.table.Columns[ci].Name)
	}
	for _, row := range m.table.Rows {
		for i, ci := range m.visible {
			if w := runeLen(formatCell(cellAt(row.Cells, ci))); w > m.colWidths[i] {
				m.colWidths[i] = w
			}
		}
	}
	if m.sortCol >= 0 && m.sortCol < len(m.visible) {
		ci := m.visible[m.sortCol]
		if need := runeLen(m.table.Columns[ci].Name) + sortIndicatorWidth; need > m.colWidths[m.sortCol] {
			m.colWidths[m.sortCol] = need
		}
	}
}

// Update handles a resolved keymap action. Vertical navigation actions move the
// highlight and keep it visible; nav.left/nav.right scroll the columns
// horizontally when the table is wider than the pane (D60); nav.drillIn emits a
// RowSelectedMsg for the highlighted row. Any other action is ignored (the root
// routes it elsewhere). The table consumes actions, never raw keys (D11).
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	// Horizontal scroll is handled first: it applies even to a header-only table
	// (columns can be wider than the pane with no rows yet), so it precedes the
	// empty-rows guard below.
	switch a {
	case keymap.ActionLeft:
		m.scrollLeft()
		return m, nil
	case keymap.ActionRight:
		m.scrollRight()
		return m, nil
	}
	if len(m.table.Rows) == 0 {
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
		m.moveTo(len(m.table.Rows) - 1)
	case keymap.ActionHalfPageDown:
		m.moveTo(m.cursor + m.pageStep()/2)
	case keymap.ActionHalfPageUp:
		m.moveTo(m.cursor - m.pageStep()/2)
	case keymap.ActionPageDown:
		m.moveTo(m.cursor + m.pageStep())
	case keymap.ActionPageUp:
		m.moveTo(m.cursor - m.pageStep())
	case keymap.ActionDrillIn:
		row := m.table.Rows[m.cursor]
		return m, func() tea.Msg { return RowSelectedMsg{Row: row} }
	}
	return m, nil
}

// SelectNextWrap moves the selection to the next displayed row, wrapping from the
// last row back to the first. It backs the root model's search-next (n) over a
// narrowing filter, where the displayed rows are exactly the matches (M2-09b): a
// plain nav.down clamps at the bottom, but stepping through matches wraps, as vim's
// search does. A no-op on an empty table.
func (m *Model) SelectNextWrap() {
	if len(m.table.Rows) == 0 {
		return
	}
	if m.cursor >= len(m.table.Rows)-1 {
		m.moveTo(0)
		return
	}
	m.moveTo(m.cursor + 1)
}

// SelectPrevWrap moves the selection to the previous displayed row, wrapping from
// the first row to the last — the search-prev (N) counterpart to SelectNextWrap.
func (m *Model) SelectPrevWrap() {
	if len(m.table.Rows) == 0 {
		return
	}
	if m.cursor <= 0 {
		m.moveTo(len(m.table.Rows) - 1)
		return
	}
	m.moveTo(m.cursor - 1)
}

// pageStep is the number of data rows a full page scroll moves — the visible data
// height, or 1 when the pane is too short to show any (so a page step still
// advances).
func (m Model) pageStep() int {
	if h := m.dataHeight(); h > 0 {
		return h
	}
	return 1
}

// moveTo sets the cursor to i (clamped to the row range) and scrolls so it stays
// visible.
func (m *Model) moveTo(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(m.table.Rows)-1 {
		i = len(m.table.Rows) - 1
	}
	m.cursor = i
	m.scrollToCursor()
}

// innerHeight is the number of content rows inside the border (total height minus
// the top and bottom border rows), never negative.
func (m Model) innerHeight() int {
	h := m.height - 2
	if h < 0 {
		return 0
	}
	return h
}

// dataHeight is the number of row lines the pane can show — the inner height less
// the one header line — never negative.
func (m Model) dataHeight() int {
	h := m.innerHeight() - 1
	if h < 0 {
		return 0
	}
	return h
}

// scrollToCursor adjusts the scroll offset so the cursor is within the visible
// data window.
func (m *Model) scrollToCursor() {
	h := m.dataHeight()
	if h == 0 {
		m.offset = m.cursor
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

// clampOffset keeps the scroll offset valid after a resize or a new snapshot,
// preferring to keep the cursor visible.
func (m *Model) clampOffset() {
	h := m.dataHeight()
	maxOffset := len(m.table.Rows) - h
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

// innerWidth is the content width inside the border (total width minus the left
// and right border columns), never negative.
func (m Model) innerWidth() int {
	w := m.width - 2
	if w < 0 {
		return 0
	}
	return w
}

// contentWidth is the display width of a fully-rendered row: the sum of the
// computed column widths plus the inter-column gaps. Header and every data row
// render to exactly this width (each cell is padded to its column width), so one
// horizontal offset windows them all identically.
func (m Model) contentWidth() int {
	n := len(m.colWidths)
	if n == 0 {
		return 0
	}
	w := runeLen(colGap) * (n - 1)
	for _, cw := range m.colWidths {
		w += cw
	}
	return w
}

// maxHOffset is the largest valid horizontal offset — how far past the pane the
// content extends — never negative.
func (m Model) maxHOffset() int {
	if over := m.contentWidth() - m.innerWidth(); over > 0 {
		return over
	}
	return 0
}

// columnStarts is the display-column offset at which each visible column begins
// in a rendered line (column i follows the earlier columns and their gaps). These
// are the snap targets for horizontal scroll, so a left/right lands a column flush
// against the pane's left edge.
func (m Model) columnStarts() []int {
	if len(m.colWidths) == 0 {
		return nil
	}
	starts := make([]int, len(m.colWidths))
	x := 0
	gap := runeLen(colGap)
	for i, cw := range m.colWidths {
		starts[i] = x
		x += cw + gap
	}
	return starts
}

// scrollRight advances the horizontal offset to the start of the next column
// (revealing the leftmost hidden column), clamped so it never scrolls past the
// content. When no column start remains within range — e.g. a final column wider
// than the pane — it snaps to maxHOffset so that column's tail is still reachable.
func (m *Model) scrollRight() {
	max := m.maxHOffset()
	for _, s := range m.columnStarts() {
		if s > m.hoffset && s <= max {
			m.hoffset = s
			return
		}
	}
	m.hoffset = max
}

// scrollLeft retreats the horizontal offset to the start of the previous column,
// or to 0 when already at or before the first column start. This always lands on
// a column boundary and eventually returns to the fully-left position.
func (m *Model) scrollLeft() {
	target := 0
	for _, s := range m.columnStarts() {
		if s >= m.hoffset {
			break
		}
		target = s
	}
	m.hoffset = target
}

// clampHOffset keeps the horizontal offset valid after a resize or a column-width
// change (a snapshot/delta can widen or narrow the content).
func (m *Model) clampHOffset() {
	if max := m.maxHOffset(); m.hoffset > max {
		m.hoffset = max
	}
	if m.hoffset < 0 {
		m.hoffset = 0
	}
}

// View renders the table as a bordered header row plus the visible window of data
// rows. It returns "" until the table has been sized (before the first
// WindowSizeMsg), so the root model lays nothing out prematurely.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	inner := m.innerHeight()

	// The Pane style has a border; lipgloss counts the border inside Width/Height,
	// so the frame is sized to the total width/height and its content area is the
	// inner (innerW × inner) region the lines are rendered to. Sizing the frame to
	// innerW instead would leave a content area two columns short and wrap every
	// full-width row (D58).
	frame := m.styles.Pane
	if m.focused {
		frame = m.styles.PaneFocus
	}
	if inner == 0 {
		return frame.Width(m.width).Height(m.height).Render("")
	}

	lines := make([]string, 0, inner)
	lines = append(lines, m.renderHeader(innerW))

	// One column layout for every row in this frame; renderRow needs it to place
	// its colored cell spans (M4-06).
	starts := m.columnStarts()

	dataRows := inner - 1
	if len(m.table.Rows) == 0 && m.notice != "" {
		lines = append(lines, m.noticeBody(innerW, dataRows)...)
	} else {
		for row := 0; row < dataRows; row++ {
			i := m.offset + row
			if i >= len(m.table.Rows) {
				lines = append(lines, m.styles.App.Width(innerW).Render(""))
				continue
			}
			lines = append(lines, m.renderRow(m.table.Rows[i], i == m.cursor, innerW, starts))
		}
	}

	return frame.Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}

// noticeBody renders the notice into exactly rows lines of width innerW: the
// first source line as the headline (Error), the rest as detail (Subtle), each
// word-wrapped to the pane and the remainder blank-filled so the frame keeps its
// height. Text past the available rows is dropped rather than scrolled — the
// pane is not a viewer, and the headline (the part that must be read) is first.
//
// Every source line is sanitized before it is measured (AUTH-06, D232). A notice
// is the one thing this pane renders that kubecom did not write: it quotes an API
// server's error text and, for a credential-plugin failure, a subprocess's stderr
// (browsefail.go). Both may carry control characters and escape sequences, which
// the wrap and the truncate below measure as zero cells and pass through — so a
// `\r` in a plugin's progress line repaints over the left border, a `\b` leaves
// the right one short, and `\x1b[2J` erases the frame this body is inside. It is
// done here rather than in SetNotice so the stored text stays exactly what the
// embedder composed (restoreReauthNotice compares against it).
func (m Model) noticeBody(innerW, rows int) []string {
	if rows <= 0 || innerW <= 0 {
		return nil
	}
	out := make([]string, 0, rows)
	for i, src := range strings.Split(m.notice, "\n") {
		style := m.styles.Subtle
		if i == 0 {
			style = m.styles.Error
		}
		for _, line := range strings.Split(ansi.Wordwrap(safetext.Line(src), innerW, ""), "\n") {
			if len(out) == rows {
				return out
			}
			out = append(out, style.Width(innerW).Render(ansi.Truncate(line, innerW, "")))
		}
	}
	for len(out) < rows {
		out = append(out, m.styles.App.Width(innerW).Render(""))
	}
	return out
}

// renderHeader lays out the column headers padded to the computed widths and
// styled as the header row, windowed to innerW at the current horizontal offset.
// The sorted column's header carries a direction arrow (M2-13b); its extra width
// is reserved in measureWidths so the marker never overflows and misaligns the
// data rows.
func (m Model) renderHeader(innerW int) string {
	cells := make([]string, len(m.visible))
	for i, ci := range m.visible {
		name := m.table.Columns[ci].Name
		if i == m.sortCol {
			name += m.sortMark()
		}
		cells[i] = padRight(name, m.colWidths[i])
	}
	line := m.hclip(strings.Join(cells, colGap), innerW)
	return m.styles.Header.Width(innerW).Render(line)
}

// sortMark is the header arrow for the active sort direction (▲ ascending / ▼
// descending). Only ever appended to the sorted column's header.
func (m Model) sortMark() string {
	if m.sortDesc {
		return sortDescMark
	}
	return sortAscMark
}

// renderRow lays out one row's cells padded to the computed widths, windowed to
// innerW at the current horizontal offset; the highlighted row takes the Selection
// style (full-width bar), a normal row the base style with its status-carrying
// cells colored (M4-06).
//
// Selection wins outright over cell coloring: the cursor row's one job is to say
// "you are here", and a row whose STATUS is repainted mid-bar reads as a broken
// highlight rather than as information. The color is still one keystroke away —
// move off the row and it is there.
func (m Model) renderRow(r kube.Row, selected bool, innerW int, starts []int) string {
	cells := make([]string, len(m.visible))
	for i, ci := range m.visible {
		cells[i] = padRight(formatCell(cellAt(r.Cells, ci)), m.colWidths[i])
	}
	line := m.hclip(strings.Join(cells, colGap), innerW)
	if selected {
		return m.styles.Selection.Width(innerW).Render(line)
	}
	spans := m.roleSpans(r, starts)
	if len(spans) == 0 {
		return m.styles.App.Width(innerW).Render(line)
	}
	return m.paintRow(line, spans, innerW)
}

// cellAt returns the cell at index i, or nil when the row has fewer cells than
// columns (a short/odd row degrades to blanks rather than panicking — principle 3).
func cellAt(cells []any, i int) any {
	if i < 0 || i >= len(cells) {
		return nil
	}
	return cells[i]
}

// formatCell renders a server-printed table cell (string, number, bool, or null)
// to its display string. JSON decoding gives numbers as float64; an integral
// value prints without a trailing ".0" to match kubectl.
func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprint(x)
	}
}

// isNumericColumn reports whether a server column type sorts numerically. The
// server-side Table column types follow the OpenAPI names ("integer", "number");
// anything else — string, boolean, or date — sorts as text. Dates in particular
// print as kubectl's human ages ("5d", "2h"), which don't parse as numbers, so
// text order is the honest cheap choice (D94).
func isNumericColumn(t string) bool {
	return t == "integer" || t == "number"
}

// compareCells three-way compares two formatted cell strings (returns <0, 0, or
// >0 for a<b, a==b, a>b). When numeric is set and both parse as numbers they
// compare by value; otherwise, or on a parse failure, they compare
// case-insensitively as text.
func compareCells(a, b string, numeric bool) int {
	if numeric {
		fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
		fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if ea == nil && eb == nil {
			switch {
			case fa < fb:
				return -1
			case fa > fb:
				return 1
			default:
				return 0
			}
		}
	}
	la, lb := strings.ToLower(a), strings.ToLower(b)
	switch {
	case la < lb:
		return -1
	case la > lb:
		return 1
	default:
		return 0
	}
}

// padRight pads s with spaces to w display columns (never truncates; the widest
// cell defines the column width, and the whole line is clipped to the pane in
// View).
func padRight(s string, w int) string {
	if n := runeLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// runeLen is the display width of s in characters.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// hclip returns the horizontal window of s that is visible in a w-column pane at
// the current scroll offset: the runes in [hoffset, hoffset+w). A row wider than
// the pane is cut rather than wrapped onto a second line, and the offset scrolls
// the whole content left (D60); the enclosing style pads a short window back out
// to w. s here is raw, unstyled text (no ANSI), so a rune cut is safe.
func (m Model) hclip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	if m.hoffset >= len(runes) {
		return ""
	}
	end := m.hoffset + w
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[m.hoffset:end])
}
