// Package table is kubecom's resource table: the right pane of the browse view,
// the live-updating list of objects for the resource chosen in the menu. It
// renders a server-printed kube.Table — kubectl-identical columns for any
// resource, built-ins and CRDs alike — with a header row, vertical scroll, and a
// highlighted selection.
//
// This slice (M2-06a) renders a static snapshot: SetTable installs a kube.Table,
// the user moves the selection through keymap actions, and drilling in emits a
// RowSelectedMsg. Later slices apply live watch deltas onto the snapshot without
// disturbing the selection (M2-06b) and add horizontal scroll for wide tables
// (M2-06c).
//
// The table never matches a raw key (D11): the root model resolves a KeyMsg to a
// keymap.Action and hands it to Update. Like the menu, the table owns no shared
// mutable state (principle 1) and owns its own emitted message type — RowSelectedMsg
// lives here, not in package tui, so the component does not import the root package
// (which imports it) and cause a cycle (D56).
package table

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
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
	table  kube.Table

	// visible holds the indices (into table.Columns) of the columns shown, and
	// colWidths their rendered widths — both derived from the snapshot in
	// SetTable so View stays a pure render.
	visible   []int
	colWidths []int

	cursor  int // index of the highlighted row
	offset  int // index of the first visible row (vertical scroll)
	width   int // total width incl. border
	height  int // total height incl. border
	focused bool
}

// New builds an empty table rendered through the given styles.
func New(s styles.Styles) Model {
	return Model{styles: s}
}

// SetTable installs a new snapshot, recomputing the visible columns and their
// widths and resetting the selection to the first row. M2-06b will replace this
// wholesale-reset behaviour with delta application that preserves the selection.
func (m *Model) SetTable(t kube.Table) {
	m.table = t
	m.computeColumns()
	m.cursor = 0
	m.offset = 0
	m.clampOffset()
}

// SetSize sets the table's total size (including its border).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
}

// Focus marks the table as holding focus (accented border).
func (m *Model) Focus() { m.focused = true }

// Blur marks the table as not holding focus.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the table holds focus.
func (m Model) Focused() bool { return m.focused }

// Cursor is the index of the highlighted row (for tests / the root model).
func (m Model) Cursor() int { return m.cursor }

// RowCount is the number of rows in the current snapshot.
func (m Model) RowCount() int { return len(m.table.Rows) }

// SelectedRow returns the highlighted row and true, or a zero Row and false when
// the table is empty.
func (m Model) SelectedRow() (kube.Row, bool) {
	if len(m.table.Rows) == 0 {
		return kube.Row{}, false
	}
	return m.table.Rows[m.cursor], true
}

// computeColumns selects the visible columns and measures their widths from the
// current snapshot. Only priority-0 columns are shown by default, matching
// `kubectl get`'s narrow view (higher-priority columns are the `-o wide` extras);
// if the server sends no priority-0 column, every column is shown rather than
// rendering a blank table (degrade, don't blank — principle 3). Each column is as
// wide as the widest of its header and cell values.
func (m *Model) computeColumns() {
	m.visible = m.visible[:0]
	for i, c := range m.table.Columns {
		if c.Priority == 0 {
			m.visible = append(m.visible, i)
		}
	}
	if len(m.visible) == 0 {
		for i := range m.table.Columns {
			m.visible = append(m.visible, i)
		}
	}

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
}

// Update handles a resolved keymap action. Navigation actions move the highlight
// and keep it visible; nav.drillIn emits a RowSelectedMsg for the highlighted
// row. Any other action is ignored (the root routes it elsewhere). The table
// consumes actions, never raw keys (D11).
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
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

	dataRows := inner - 1
	for row := 0; row < dataRows; row++ {
		i := m.offset + row
		if i >= len(m.table.Rows) {
			lines = append(lines, m.styles.App.Width(innerW).Render(""))
			continue
		}
		lines = append(lines, m.renderRow(m.table.Rows[i], i == m.cursor, innerW))
	}

	return frame.Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}

// renderHeader lays out the column headers padded to the computed widths and
// styled as the header row, hard-clipped to innerW.
func (m Model) renderHeader(innerW int) string {
	cells := make([]string, len(m.visible))
	for i, ci := range m.visible {
		cells[i] = padRight(m.table.Columns[ci].Name, m.colWidths[i])
	}
	line := truncate(strings.Join(cells, colGap), innerW)
	return m.styles.Header.Width(innerW).Render(line)
}

// renderRow lays out one row's cells padded to the computed widths, hard-clipped
// to innerW; the highlighted row takes the Selection style (full-width bar), a
// normal row the base style.
func (m Model) renderRow(r kube.Row, selected bool, innerW int) string {
	cells := make([]string, len(m.visible))
	for i, ci := range m.visible {
		cells[i] = padRight(formatCell(cellAt(r.Cells, ci)), m.colWidths[i])
	}
	line := truncate(strings.Join(cells, colGap), innerW)
	style := m.styles.App
	if selected {
		style = m.styles.Selection
	}
	return style.Width(innerW).Render(line)
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

// truncate hard-clips s to at most w display columns, so a row wider than the
// pane is cut rather than wrapped onto a second line (horizontal scroll for wide
// tables is M2-06c). s here is raw, unstyled text (no ANSI), so a rune cut is
// safe.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runeLen(s) <= w {
		return s
	}
	return string([]rune(s)[:w])
}
