// Package searchview is kubecom's cluster-search mini-app: a full-screen view with a
// live query field over a streaming, cross-kind result list (Kind · namespace · name).
// It is the UI half of the cluster search (feedback
// 2026-07-24-cluster-search-multi-resource, D131) whose kube-layer fan-out primitive is
// kube.Search (SEARCH-01).
//
// This slice (SEARCH-02a) is the component in isolation — the query field, the
// streaming result list, cursor navigation, and the three messages the wiring reacts to.
// It runs no search itself and knows nothing about clients: SEARCH-02b registers the
// search.cluster action, runs kube.Search off the update loop, and feeds hits in.
// Streaming progress/cap (SEARCH-03) and the scope widen (SEARCH-04) are later slices.
//
// Shape follows the two established component rhythms: full-screen like the logs view
// (results span kinds and want every row, D134) and list/delegate like the picker
// (M2-08a). Input is keymap-driven — it never matches a raw key for behaviour (D11):
// the root resolves a KeyMsg to an Action and hands the Action to Update; the one
// exception is UpdateQuery, which receives the raw text content, exactly as the picker's
// filter and the logs view's grep do. Unlike those two the query field is *always* open
// while the view is up — the query is the view, not a mode within it. It emits its own
// message types (SelectedMsg/ClosedMsg/QueryChangedMsg) so it never imports the root
// package (D56) and holds no shared mutable state (principle 1).
package searchview

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// kind is the fixed message kind for this view (there is only ever one search view).
const kind = "search"

// headerHeight is the one status line above the body; queryHeight is the always-shown
// query input line.
const (
	headerHeight = 1
	queryHeight  = 1
)

// SelectedMsg is emitted when the user drills into the highlighted hit (nav.drillIn).
// Hit carries the kind and object identity the wiring needs to switch the browse view
// to that resource and select the row. Owned by this package — the emitter — so the
// root model handles the concrete type without this package importing it (D56).
type SelectedMsg struct {
	Kind string
	Hit  kube.SearchHit
}

// ClosedMsg is emitted when the user dismisses the view (nav.back on an empty query).
// Kind mirrors the other components' ClosedMsg shape so the root routes them uniformly.
type ClosedMsg struct {
	Kind string
}

// QueryChangedMsg is emitted whenever the query text actually changes — including the
// change to "" when nav.back clears it. It is the wiring's cue to cancel the in-flight
// search and (for a non-empty query) start a new one; debouncing and cancellation are
// the wiring's job (SEARCH-02b), not the view's. The view has already dropped the hits
// belonging to the previous query by the time this is delivered, so a consumer never
// has to reconcile stale results.
type QueryChangedMsg struct {
	Kind  string
	Query string
}

// item is one result row: the hit plus its pre-rendered, column-aligned label. The
// label is built once per rebuild (kind column padded to the widest kind currently
// shown) so the delegate stays a pure renderer and tests can assert exact text.
type item struct {
	hit   kube.SearchHit
	label string
}

func (i item) FilterValue() string { return i.label }

// itemDelegate renders each result on a single line, highlighting the cursor row with
// the shared Selection style. Minimal (no per-item state, no key bindings) so the list
// contributes no hard-coded keys or help of its own.
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
	style := d.styles.App
	if index == m.Index() {
		style = d.styles.Selection
	}
	_, _ = io.WriteString(w, style.Width(width).MaxWidth(width).Render(row.label))
}

// Model is the full-screen search view. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles styles.Styles
	query  textinput.Model // always focused while the view is active
	list   list.Model
	scope  string // human label for what is being searched (e.g. a namespace), header only

	// hits is the authoritative result set in arrival order; the list is rebuilt from
	// it on every append so labels stay column-aligned as new kinds stream in.
	hits []kube.SearchHit

	searching bool // a search is in flight (header indicator; the wiring sets it)

	active bool // whether the view is shown (captures input) — "" View when false
	width  int  // full screen width
	height int  // full screen height
}

// New builds a search view rendered through the shared styles. It starts hidden with an
// empty query and no results. The list's own chrome, key bindings, and native filter
// stay disabled: navigation arrives as keymap actions (D11) and the query field is the
// view's own textinput, so no hard-coded list key leaks in.
func New(s styles.Styles) Model {
	l := list.New(nil, itemDelegate{styles: s}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	qi := textinput.New()
	qi.Prompt = "search: "
	return Model{
		styles: s,
		query:  qi,
		list:   l,
	}
}

// Kind returns the view's kind id (always "search").
func (m Model) Kind() string { return kind }

// SetScope sets the header label describing what the search covers (e.g. the current
// namespace). Purely informational; the view never searches anything itself.
func (m *Model) SetScope(s string) { m.scope = s }

// Show reveals the view and focuses the query field (it then captures input until
// Hide). Hide dismisses it and blurs the field; the query and results survive so the
// caller decides whether reopening resumes or starts clean (Reset).
func (m *Model) Show() tea.Cmd {
	m.active = true
	return m.query.Focus()
}

func (m *Model) Hide() {
	m.active = false
	m.query.Blur()
}

// Active reports whether the view is shown and capturing input.
func (m Model) Active() bool { return m.active }

// Reset clears the query, the results, and the in-flight indicator so the next open
// starts clean.
func (m *Model) Reset() {
	m.query.Reset()
	m.clearHits()
	m.searching = false
}

// Query is the current query text.
func (m Model) Query() string { return m.query.Value() }

// SetSearching records whether a search is in flight (shown in the header). The wiring
// sets it when it launches a query and clears it when the fan-out completes.
func (m *Model) SetSearching(b bool) { m.searching = b }

// Searching reports whether a search is in flight.
func (m Model) Searching() bool { return m.searching }

// AppendHit adds one streamed result and re-renders the list. The cursor stays on the
// row it was on, so results arriving under the reader never move their selection.
func (m *Model) AppendHit(h kube.SearchHit) {
	m.hits = append(m.hits, h)
	idx := m.list.Index()
	m.rebuild()
	m.list.Select(idx)
}

// Len is the number of results currently held.
func (m Model) Len() int { return len(m.hits) }

// Selected returns the highlighted hit and true, or the zero hit and false when there
// are no results.
func (m Model) Selected() (kube.SearchHit, bool) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return kube.SearchHit{}, false
	}
	return it.hit, true
}

// Update handles a resolved keymap action while the view is active. Navigation moves
// the result cursor (bubbles/list manages pagination); nav.drillIn emits SelectedMsg
// for the highlighted hit; nav.back clears a non-empty query first (dropping its
// results and emitting QueryChangedMsg{""} so the wiring cancels the in-flight search)
// and only closes the view (ClosedMsg) on a second press — one esc must never lose both
// the query and the view. The view consumes actions, never raw keys (D11); an inactive
// view ignores everything.
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
		if m.query.Value() != "" {
			m.query.Reset()
			m.clearHits()
			m.searching = false
			return m, queryChanged("")
		}
		return m, func() tea.Msg { return ClosedMsg{Kind: kind} }
	}
	return m, nil
}

// UpdateQuery feeds one raw key to the query field. It is the view's only raw-key entry
// point: the root resolves control keys (nav, drill-in, back) to actions and routes the
// remaining text content here, so the field captures typing without any view matching a
// raw key for behaviour (D11). A change to the text drops the previous query's results
// (they no longer describe what is on screen) and emits QueryChangedMsg for the wiring
// to act on; keys that leave the text alone (cursor moves inside the field) emit
// nothing, so they never restart a search.
func (m Model) UpdateQuery(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	before := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	after := m.query.Value()
	if after == before {
		return m, cmd
	}
	m.clearHits()
	return m, tea.Batch(cmd, queryChanged(after))
}

// queryChanged builds the QueryChangedMsg command for q.
func queryChanged(q string) tea.Cmd {
	return func() tea.Msg { return QueryChangedMsg{Kind: kind, Query: q} }
}

// clearHits drops every result and rewinds the cursor.
func (m *Model) clearHits() {
	m.hits = m.hits[:0]
	m.rebuild()
	m.list.Select(0)
}

// rebuild regenerates the list items from the hits, keeping arrival order.
func (m *Model) rebuild() {
	rows := hitItems(m.hits)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, r)
	}
	m.list.SetItems(items)
}

// hitItems renders the hits into aligned rows: the kind column padded to the widest
// kind present, then the object path (namespace/name, or a bare name for a
// cluster-scoped object, whose Ref.Namespace is empty). Pure, so the exact row text is
// testable without a rendered View.
func hitItems(hits []kube.SearchHit) []item {
	width := 0
	for _, h := range hits {
		if n := len(h.Resource.GVK.Kind); n > width {
			width = n
		}
	}
	out := make([]item, 0, len(hits))
	for _, h := range hits {
		k := h.Resource.GVK.Kind
		path := h.Ref.Name
		if h.Ref.Namespace != "" {
			path = h.Ref.Namespace + "/" + h.Ref.Name
		}
		out = append(out, item{hit: h, label: k + strings.Repeat(" ", width-len(k)) + "  " + path})
	}
	return out
}

// SetSize records the full screen size and sizes the inner list (the whole screen minus
// the header and query lines).
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	iw, ih := m.innerSize()
	m.list.SetSize(iw, ih)
	m.query.SetWidth(iw)
}

// innerSize is the list's content size: the full screen minus the header and query
// lines. Full width — the view is full-screen with no border.
func (m Model) innerSize() (int, int) {
	iw := m.width
	ih := m.height - headerHeight - queryHeight
	if iw < 0 {
		iw = 0
	}
	if ih < 0 {
		ih = 0
	}
	return iw, ih
}

// View renders the full-screen search view: a header line (scope · result count ·
// in-flight state), the query field, then the result list — or a hint line in the
// list's place while there is nothing to show. Returns "" when hidden or unsized. The
// root composites it as the base while it is up, not as a centered overlay (D134).
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	parts := []string{
		m.header(),
		m.styles.App.Width(m.width).MaxWidth(m.width).Render(m.query.View()),
	}
	if len(m.hits) == 0 {
		parts = append(parts, m.styles.Subtle.Width(m.width).MaxWidth(m.width).Render(m.emptyHint()))
	} else {
		parts = append(parts, m.list.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// emptyHint is the line shown in place of an empty result list: what to do on a blank
// query, the in-flight state while the fan-out runs, and an explicit no-match otherwise
// (an empty list must never read as a hung search).
func (m Model) emptyHint() string {
	switch {
	case m.query.Value() == "":
		return "type to search this cluster"
	case m.searching:
		return "searching…"
	default:
		return "no matches"
	}
}

// header builds the one-line status bar: "search · <scope> · N results" plus the
// in-flight marker, clipped to the screen width.
func (m Model) header() string {
	seg := kind
	if m.scope != "" {
		seg += " · " + m.scope
	}
	seg += " · " + itoa(len(m.hits)) + " results"
	if m.searching {
		seg += " · searching…"
	}
	return m.styles.Header.Width(m.width).MaxWidth(m.width).Render(clip(seg, m.width))
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
