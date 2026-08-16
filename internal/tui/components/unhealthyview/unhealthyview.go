// Package unhealthyview is kubecom's cross-kind "what's broken" list: a
// full-screen view of the ScanHits a kube.Scan sweep collects — broken resources
// of any kind (pods *and* claims/volumes), each row navigable to the object. It
// is the surface half of STORY-06g-2 (D276): kube.Scan does the fan-out, the
// wiring feeds the hits in, this view shows them, and a drill-in hands the
// highlighted hit back so the wiring can switch browse to it. It is the answer to
// the S02 miss the feedback named — a failure that is not a pod is invisible to a
// pod-first walk, so a sweep that lists the broken things of every kind finds the
// operator instead of the operator guessing which kind to visit.
//
// Shape follows the established full-screen list rhythm (searchview, D134): the
// whole screen minus a header line, bubbles/list rows with a cursor, everything
// driven by keymap actions (D11). It has no query field — the sweep is launched
// once, on open, not per keystroke — so unlike the search view it is a pure
// list: every mapped key is a navigation action, and the list is the only
// surface. It emits its own message types (SelectedMsg/ClosedMsg) so it never
// imports the root package (D56) and holds no shared mutable state (principle 1).
package unhealthyview

import (
	"io"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/table"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// kind is the fixed message kind for this view (there is only ever one).
const kind = "unhealthy"

// headerHeight is the one status line above the body.
const headerHeight = 1

// SelectedMsg is emitted when the user drills into the highlighted hit
// (nav.drillIn). Hit carries the kind and the row — object identity plus the
// offending cells — so the wiring can switch browse to that resource and select
// the row, and can show the reason without re-listing. Owned by this package —
// the emitter — so the root model handles the concrete type without this package
// importing it (D56).
type SelectedMsg struct {
	Kind string
	Hit  kube.ScanHit
}

// ClosedMsg is emitted when the user dismisses the view (nav.back). Kind mirrors
// the other components' ClosedMsg shape so the root routes them uniformly.
type ClosedMsg struct {
	Kind string
}

// item is one result row: the hit plus its pre-rendered, column-aligned label.
// The label is built once per rebuild (kind column padded to the widest kind
// currently shown, the reason appended last) so the delegate stays a pure
// renderer and tests can assert exact text.
//
// cells are the label's offending runs — the reason cells, in label coordinates —
// so the delegate can paint each with its role's hue (table.UnhealthyCell.Error)
// while the rest of the line stays plain. Resolving them here keeps the delegate
// a pure renderer, exactly as searchview resolves its match spans.
type item struct {
	hit   kube.ScanHit
	label string
	cells []cellSpan
}

// cellSpan is one offending run in a label: its rune range and whether it reads
// as an error rather than a warning, so the delegate can color it.
type cellSpan struct {
	start, end int
	error      bool
}

func (i item) FilterValue() string { return i.label }

// itemDelegate renders each result on a single line, highlighting the cursor row
// with the shared Selection style and each offending cell with the shared Warn or
// Error hue. Minimal (no per-item state, no key bindings) so the list contributes
// no hard-coded keys or help of its own.
type itemDelegate struct {
	styles styles.Styles
}

func (d itemDelegate) Height() int                         { return 1 }
func (d itemDelegate) Spacing() int                        { return 0 }
func (d itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	row, _ := it.(item)
	width := m.Width()
	if width < 0 {
		width = 0
	}
	base := d.styles.App
	if index == m.Index() {
		base = d.styles.Selection
	}
	_, _ = io.WriteString(w, paintLabel(row.label, row.cells, width, base, d.styles.Warn, d.styles.Error))
}

// paintLabel renders one result row: base everywhere, Warn/Error over the reason
// cells, padded (or clipped) to width. Each segment is rendered through a
// complete style and concatenated, never wrapped in an enclosing one — the same
// rule searchview's paintLabel and the table's paintRow follow, so the padding
// after a colored span stays styled.
func paintLabel(label string, cells []cellSpan, width int, base, warn, err lipgloss.Style) string {
	if len(cells) == 0 {
		return base.Width(width).MaxWidth(width).Render(label)
	}
	runes := []rune(label)
	if len(runes) > width {
		runes = runes[:width]
	}
	var b strings.Builder
	last := 0
	for _, sp := range cells {
		s, e := sp.start, sp.end
		if s < last {
			s = last
		}
		if e > len(runes) {
			e = len(runes)
		}
		if s >= e {
			continue
		}
		if s > last {
			b.WriteString(base.Render(string(runes[last:s])))
		}
		style := warn
		if sp.error {
			style = err
		}
		b.WriteString(style.Render(string(runes[s:e])))
		last = e
	}
	if last < len(runes) {
		b.WriteString(base.Render(string(runes[last:])))
	}
	if pad := width - len(runes); pad > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// Model is the full-screen cross-kind unhealthy list. Every field is owned by the
// embedding root model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles styles.Styles
	list   list.Model
	scope  string // human label for the namespace the sweep covers, header only

	// hits is the authoritative result set, in arrival order (the scan streams;
	// there is no score to rank on, unlike a SearchHit — D152's ranking exists
	// because search matches can be ordered, an unhealthy sweep cannot).
	hits []kube.ScanHit

	searching bool // a scan is in flight (header indicator; the wiring sets it)

	// kindsDone / kindsTotal are the fan-out's progress: how many of the kinds the
	// sweep was launched over have reported done (kube's ScanKindDone, D276)
	// against how many were requested. kindsTotal is 0 until the wiring launches,
	// so a 0 total means "in flight, count unknown", not "nothing to scan".
	kindsDone  int
	kindsTotal int

	// capped marks a sweep the hit cap truncated (kube's ScanDone{Capped}): there
	// were more matches than are shown, so the list is not the whole answer. Sticky
	// for the sweep it belongs to — cleared by Reset.
	capped bool

	active bool // whether the view is shown (captures input) — "" View when false
	width  int  // full screen width
	height int  // full screen height
}

// New builds the unhealthy view rendered through the shared styles. It starts
// hidden with no hits. The list's own chrome, key bindings, and native filter
// stay disabled: navigation arrives as keymap actions (D11), so no hard-coded
// list key leaks in.
func New(s styles.Styles) Model {
	l := list.New(nil, itemDelegate{styles: s}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	return Model{styles: s, list: l}
}

// SetStyles repaints the view through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Like the search view's, the list's delegate has to be replaced and not merely
// m.styles assigned — the delegate draws every result row and its cursor bar. The
// hits and the fan-out counters are untouched, so a restyle mid-sweep neither
// loses results nor re-runs it. The row labels need no rebuild either: they are
// plain aligned text the delegate styles at draw time.
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.list.SetDelegate(itemDelegate{styles: s})
}

// Kind returns the view's kind id (always "unhealthy").
func (m Model) Kind() string { return kind }

// SetScope sets the header label describing which namespace the sweep covers.
// Purely informational; the view never scans anything itself.
func (m *Model) SetScope(s string) { m.scope = s }

// Show reveals the view (it then captures input until Hide). There is no text
// field to focus — the list owns the keys the moment it is shown — so unlike the
// search view's Show there is no focus command to return.
func (m *Model) Show() { m.active = true }

// Hide dismisses the view.
func (m *Model) Hide() { m.active = false }

// Active reports whether the view is shown and capturing input.
func (m Model) Active() bool { return m.active }

// Reset clears the hits, the in-flight indicator, the progress counters and the
// cap flag so the next open starts clean. A sweep must never resume stale: the
// list describes one scan, and a reopened view that kept the last sweep's rows
// would read as a finished current one.
func (m *Model) Reset() {
	m.hits = m.hits[:0]
	m.rebuild()
	m.list.Select(0)
	m.searching = false
	m.kindsDone, m.kindsTotal = 0, 0
	m.capped = false
}

// SetSearching records whether a scan is in flight (shown in the header). The
// wiring sets it when it launches a sweep and clears it when the fan-out
// completes.
func (m *Model) SetSearching(b bool) { m.searching = b }

// Searching reports whether a scan is in flight.
func (m Model) Searching() bool { return m.searching }

// StartProgress records that a fan-out over total kinds has just launched: the
// progress counters restart at 0/total and any previous cap state is dropped. The
// wiring calls it when it launches the sweep, which is the first moment the kind
// count is known.
func (m *Model) StartProgress(total int) {
	if total < 0 {
		total = 0
	}
	m.kindsDone = 0
	m.kindsTotal = total
	m.capped = false
}

// MarkKindDone counts one kind that has finished being swept. kube emits exactly
// one kind-done per requested kind — including for a denied kind and for one the
// cap cut short (D276) — so the count can reach the total on any outcome and a
// forbidden group can never leave the progress line stuck one short. Clamped at
// the total so a stray event can't render a nonsense 12/11.
func (m *Model) MarkKindDone() {
	if m.kindsTotal > 0 && m.kindsDone >= m.kindsTotal {
		return
	}
	m.kindsDone++
}

// Progress reports the kinds finished and the kinds requested for the current
// sweep.
func (m Model) Progress() (done, total int) { return m.kindsDone, m.kindsTotal }

// SetCapped records that the hit cap truncated this sweep's matches (there are
// more broken resources on the cluster than are shown).
func (m *Model) SetCapped(b bool) { m.capped = b }

// Capped reports whether the current sweep's matches were truncated by the cap.
func (m Model) Capped() bool { return m.capped }

// AppendHit adds one streamed hit at the end of the list. The scan has no score
// to rank on (D276 streams in arbitrary order), so arrival order is the order —
// exactly what the plain append the search view's ranking replaced did.
func (m *Model) AppendHit(h kube.ScanHit) {
	m.hits = append(m.hits, h)
	m.rebuild()
}

// Len is the number of hits currently held.
func (m Model) Len() int { return len(m.hits) }

// Selected returns the highlighted hit and true, or the zero hit and false when
// there are no hits.
func (m Model) Selected() (kube.ScanHit, bool) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return kube.ScanHit{}, false
	}
	return it.hit, true
}

// Update handles a resolved keymap action while the view is active. Navigation
// moves the result cursor (bubbles/list manages pagination); nav.drillIn emits
// SelectedMsg for the highlighted hit; nav.back dismisses the view (ClosedMsg).
//
// The view consumes actions, never raw keys (D11); an inactive view ignores
// everything. There is no focus split to read — unlike the search view, every
// mapped key is a navigation action here, because there is no query field to own
// the text keys.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionUp:
		m.list.CursorUp()
	case keymap.ActionDown:
		m.list.CursorDown()
	case keymap.ActionTop:
		m.list.GoToStart()
	case keymap.ActionBottom:
		m.list.GoToEnd()
	case keymap.ActionHalfPageDown, keymap.ActionPageDown:
		m.list.NextPage()
	case keymap.ActionHalfPageUp, keymap.ActionPageUp:
		m.list.PrevPage()
	case keymap.ActionDrillIn:
		h, ok := m.Selected()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return SelectedMsg{Kind: kind, Hit: h} }
	case keymap.ActionBack:
		return m, func() tea.Msg { return ClosedMsg{Kind: kind} }
	}
	return m, nil
}

// SetSize records the full screen size and sizes the inner list (the whole
// screen minus the header line).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	iw, ih := m.innerSize()
	m.list.SetSize(iw, ih)
}

// innerSize is the list's content size: the full screen minus the header line.
// Full width — the view is full-screen with no border.
func (m Model) innerSize() (int, int) {
	iw := m.width
	ih := m.height - headerHeight
	if iw < 0 {
		iw = 0
	}
	if ih < 0 {
		ih = 0
	}
	return iw, ih
}

// View renders the full-screen unhealthy list: a header line (scope · hit count ·
// in-flight state), then the hits — or a hint line while there is nothing to
// show. Returns "" when hidden or unsized. The root composites it as the base
// while it is up, not as a centered overlay (D134, same as searchview).
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	parts := []string{
		m.header(),
	}
	if len(m.hits) == 0 {
		parts = append(parts, m.styles.Subtle.Width(m.width).MaxWidth(m.width).Render(m.emptyHint()))
	} else {
		parts = append(parts, m.list.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// emptyHint is the line shown in place of an empty hit list: the in-flight state
// while the sweep runs, and an explicit all-clear otherwise — an empty list must
// never read as a hung sweep.
func (m Model) emptyHint() string {
	switch {
	case m.searching:
		return "scanning…"
	default:
		return "no unhealthy resources"
	}
}

// header builds the one-line status bar: "unhealthy · <scope> · N found" plus the
// progress/cap segment, clipped to the screen width. The scope names the sweep's
// coverage; the count names how many the list holds; the progress segment names
// how far the fan-out has got and whether the cap truncated the answer.
func (m Model) header() string {
	seg := kind
	if m.scope != "" {
		seg += " · " + m.scope
	}
	seg += " · " + itoa(len(m.hits)) + " found"
	if s := m.progress(); s != "" {
		seg += " · " + s
	}
	return m.styles.Header.Width(m.width).MaxWidth(m.width).Render(clip(seg, m.width))
}

// progress is the header's fan-out segment, in the same four states the search
// view's uses (SEARCH-03b): capped wins (it is the only state the reader must
// act on and stays up after the sweep ends), then searching with a known kind
// count, then searching with none, then nothing.
func (m Model) progress() string {
	switch {
	case m.capped:
		return "first " + itoa(len(m.hits)) + " matches — limit reached"
	case m.searching && m.kindsTotal > 0:
		return "scanning " + itoa(m.kindsDone) + "/" + itoa(m.kindsTotal) + " kinds…"
	case m.searching:
		return "scanning…"
	default:
		return ""
	}
}

// rebuild regenerates the list items from the hits, keeping their order.
func (m *Model) rebuild() {
	rows := hitItems(m.hits)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, r)
	}
	m.list.SetItems(items)
}

// hitItems renders the hits (already in arrival order) into aligned rows: the
// kind column padded to the widest kind present, then the object path
// (namespace/name, or a bare name for a cluster-scoped object, whose
// Object.Namespace is empty), then the offending cells — the row's cells that
// classify to warn/error, in column order, joined by ", " (table.UnhealthyCells,
// D276). The reason cells are returned as spans so the delegate can paint them
// with the role's hue. Pure, so the exact row text is testable without a rendered
// View.
func hitItems(hits []kube.ScanHit) []item {
	width := 0
	for _, h := range hits {
		if n := len(h.Resource.GVK.Kind); n > width {
			width = n
		}
	}
	out := make([]item, 0, len(hits))
	for _, h := range hits {
		kind := h.Resource.GVK.Kind
		path := h.Row.Object.Name
		if h.Row.Object.Namespace != "" {
			path = h.Row.Object.Namespace + "/" + h.Row.Object.Name
		}
		prefix := kind + strings.Repeat(" ", width-len(kind)) + "  " + path
		label, cells := reasonLabel(prefix, h.Columns, h.Row)
		out = append(out, item{hit: h, label: label, cells: cells})
	}
	return out
}

// reasonLabel appends a hit's offending cells to its row prefix and returns the
// full label plus the offending runs' spans (in the label's rune space). With no
// offending cells the prefix is the label and there are no spans — the degraded
// row a hit can only become by classification disagreement, which shows the row
// without claiming a reason it cannot prove.
func reasonLabel(prefix string, columns []kube.Column, row kube.Row) (string, []cellSpan) {
	t := &kube.Table{Columns: columns}
	cells := table.UnhealthyCells(t, row)
	if len(cells) == 0 {
		return prefix, nil
	}
	texts := make([]string, len(cells))
	spans := make([]cellSpan, len(cells))
	prefixRunes := utf8.RuneCountInString(prefix)
	at := prefixRunes + 2 // "  " between the path and the reason
	for i, c := range cells {
		texts[i] = c.Text
		spans[i] = cellSpan{start: at, end: at + utf8.RuneCountInString(c.Text), error: c.Error}
		at = spans[i].end + 2 // ", " between reasons
	}
	return prefix + "  " + strings.Join(texts, ", "), spans
}

// itoa is a tiny non-negative int→string (avoids importing strconv for one use).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// clip truncates s to at most w display cells so a long header never overflows.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ")
}
