// Package searchview is kubecom's cluster-search mini-app: a full-screen view with a
// live query field over a streaming, cross-kind result list (Kind · namespace · name).
// It is the UI half of the cluster search (feedback
// 2026-07-24-cluster-search-multi-resource, D131) whose kube-layer fan-out primitive is
// kube.Search (SEARCH-01).
//
// SEARCH-02a built the component in isolation — the query field, the streaming result
// list, cursor navigation, and the three messages the wiring reacts to. It runs no
// search itself and knows nothing about clients: SEARCH-02b registers the search.cluster
// action, runs kube.Search off the update loop, and feeds hits in. SEARCH-03b added the
// header's fan-out state (StartProgress/MarkKindDone/SetCapped → "searching N/M kinds…"
// and the cap line), still push-only: the counters are fed from kube's SearchKindDone /
// SearchDone{Capped} events (D142) by the wiring, and reset here whenever the results
// they describe are dropped. SEARCH-04a added the kind-scope widen: the view holds the
// all-kinds flag, names it in the header while it is on, and emits ScopeChangedMsg so
// the wiring re-runs the query over the wider set — it still resolves no kinds itself.
// SEARCH-04b added the namespace-scope widen on the same pattern: an independent
// all-namespaces flag that *replaces* the scope name in the header (a header carries
// one namespace scope, never two) and rides the same ScopeChangedMsg, with the
// namespace itself still resolved by the wiring. SEARCH-04c-1 added the only state the
// query line itself can be in besides "text": SetQueryError, shown in place of the empty
// hint when the wiring cannot turn what is typed into a search — the view still does not
// parse, match, or know what a label selector is.
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

// allNamespacesLabel is what the header calls a namespace-widened search (SEARCH-04b).
// It deliberately matches the wording of the app's own all-namespaces sentinel so the
// two read as one scope, not two features; the app owns its constant and this package
// owns this one, because a component never imports the root (D56).
const allNamespacesLabel = "all namespaces"

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

// ScopeChangedMsg is emitted when the user changes *what* a query covers rather than
// the query itself: the all-kinds widen (SEARCH-04a) or the all-namespaces widen
// (SEARCH-04b). It carries the whole new scope state — both flags, always — so the
// wiring can pick the kind set and the namespace and re-run the current query without
// reading anything back off the view. The view has already dropped the hits belonging
// to the narrower scope, exactly as a query change does, because those results no
// longer describe what the header says.
//
// It is a separate message from QueryChangedMsg because the two mean different things
// to a consumer that cares: the query is still whatever the reader typed, so the field
// must not be re-read or reset, only re-run.
type ScopeChangedMsg struct {
	Kind          string
	AllKinds      bool
	AllNamespaces bool
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

	// allKinds is the kind-scope widen (SEARCH-04a): false searches the curated
	// default set, true every discovered kind. The view only holds and announces the
	// flag — which kinds that actually resolves to is the wiring's business, since it
	// is the side that knows what discovery found.
	allKinds bool

	// allNamespaces is the namespace-scope widen (SEARCH-04b), the independent other
	// half of scope: false searches whatever namespace the wiring named through
	// SetScope, true every namespace. Held here for the same reason allKinds is — the
	// view announces the scope, the wiring resolves it — and independent of allKinds
	// on purpose, so "curated kinds everywhere" and "every kind here" are both
	// reachable rather than being two stops on one cycle.
	allNamespaces bool

	// hits is the authoritative result set in arrival order; the list is rebuilt from
	// it on every append so labels stay column-aligned as new kinds stream in.
	hits []kube.SearchHit

	searching bool // a search is in flight (header indicator; the wiring sets it)

	// queryErr is why the query line on screen cannot be searched — today only an
	// unparseable label selector (SEARCH-04c-1). It is shown in place of the empty
	// hint rather than in the header, because it belongs to the text one line above
	// it and because the header is clipped from the right, where a message that
	// says what to fix would be the first thing lost. Push-only, like every other
	// state here: the view does not know what a selector is, only that the wiring
	// could not use one.
	queryErr string

	// kindsDone / kindsTotal are the fan-out's progress: how many of the kinds the
	// current query was launched over have reported done (kube's SearchKindDone,
	// SEARCH-03a) against how many were requested. kindsTotal is 0 until the wiring
	// launches — during the debounce window there is no scope yet — so a 0 total means
	// "in flight, count unknown", not "nothing to search".
	kindsDone  int
	kindsTotal int

	// capped marks a search the hit cap truncated (kube's SearchDone{Capped}): there
	// were more matches than are shown, so the answer is a narrower query. Sticky for
	// the query it belongs to — it survives the fan-out completing and is cleared, like
	// the counters, when the query changes.
	capped bool

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

// SetScope sets the header label describing which namespace the search covers. Purely
// informational; the view never searches anything itself. It is overridden while the
// namespace widen is on (the header then says "all namespaces"), so the wiring can set
// it once on open and never revisit it.
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

// Reset clears the query, the results, the in-flight indicator and both scope widens so
// the next open starts clean. A widen is deliberately *not* sticky across opens: it is
// the expensive scope, and a mode left on from a search two minutes ago would make the
// next `ctrl+s` quietly sweep the whole cluster. It survives within one open — a reader
// refining a query under it keeps it — which is the span it belongs to. The namespace
// widen resets for the same reason and for one more: the app's namespace can change
// while the view is closed, so a stale "everywhere" would silently outlive the scope it
// was chosen against.
func (m *Model) Reset() {
	m.query.Reset()
	m.clearHits()
	m.searching = false
	m.allKinds = false
	m.allNamespaces = false
	m.queryErr = ""
}

// Query is the current query text.
func (m Model) Query() string { return m.query.Value() }

// AllKinds reports whether the kind scope is widened to every discovered kind. The
// wiring reads it when it picks the kind set for a query.
func (m Model) AllKinds() bool { return m.allKinds }

// AllNamespaces reports whether the namespace scope is widened to every namespace. The
// wiring reads it when it picks the namespace for a query.
func (m Model) AllNamespaces() bool { return m.allNamespaces }

// SetQueryError records why the current query line cannot be searched (""  clears it).
// The wiring sets it on every query change — so it is refreshed or dropped in step with
// the text — and a view holding one shows it instead of the empty hint. The view never
// produces one itself: what makes a query usable is the wiring's business.
func (m *Model) SetQueryError(s string) { m.queryErr = s }

// QueryError reports why the current query cannot be searched, or "" when it can.
func (m Model) QueryError() string { return m.queryErr }

// SetSearching records whether a search is in flight (shown in the header). The wiring
// sets it when it launches a query and clears it when the fan-out completes.
func (m *Model) SetSearching(b bool) { m.searching = b }

// Searching reports whether a search is in flight.
func (m Model) Searching() bool { return m.searching }

// StartProgress records that a fan-out over total kinds has just launched: the
// progress counters restart at 0/total and any previous cap state is dropped. The
// wiring calls it when it launches the debounced search, which is the first moment the
// kind count is known (the scope is derived from what discovery currently offers).
func (m *Model) StartProgress(total int) {
	if total < 0 {
		total = 0
	}
	m.kindsDone = 0
	m.kindsTotal = total
	m.capped = false
}

// MarkKindDone counts one kind that has finished being searched. kube emits exactly one
// kind-done per requested kind — including for a denied kind and for one the cap cut
// short (D142 pt 2) — so the count can reach the total on any outcome and a forbidden
// group can never leave the progress line stuck one short. Clamped at the total so a
// stray event can't render a nonsense 12/11.
func (m *Model) MarkKindDone() {
	if m.kindsTotal > 0 && m.kindsDone >= m.kindsTotal {
		return
	}
	m.kindsDone++
}

// Progress reports the kinds finished and the kinds requested for the current query.
func (m Model) Progress() (done, total int) { return m.kindsDone, m.kindsTotal }

// SetCapped records that the hit cap truncated this query's matches (there are more on
// the cluster than are shown).
func (m *Model) SetCapped(b bool) { m.capped = b }

// Capped reports whether the current query's matches were truncated by the cap.
func (m Model) Capped() bool { return m.capped }

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
// for the highlighted hit; search.allKinds and search.allNamespaces flip their scope
// widen and emit ScopeChangedMsg; nav.back clears a non-empty query first (dropping its
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
	case keymap.ActionSearchAllKinds:
		// The widen changes what the current query means, so the hits it produced
		// under the narrower scope go with it — clearHits also drops the progress
		// counters and the cap flag, which described that fan-out. An empty query
		// still toggles: the reader is choosing the scope before typing, and the
		// header says so immediately.
		m.allKinds = !m.allKinds
		m.clearHits()
		return m, m.scopeChanged()
	case keymap.ActionSearchAllNamespaces:
		// Same contract as the kind widen above, on the other axis (SEARCH-04b): the
		// results belonged to the narrower namespace scope, so they go with it, and
		// the wiring re-runs the untouched query over the wider one.
		m.allNamespaces = !m.allNamespaces
		m.clearHits()
		return m, m.scopeChanged()
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

// scopeChanged builds the ScopeChangedMsg command for the current scope. It always
// carries both flags, whichever one moved, so a consumer reads one message rather than
// merging it with remembered state.
func (m Model) scopeChanged() tea.Cmd {
	msg := ScopeChangedMsg{Kind: kind, AllKinds: m.allKinds, AllNamespaces: m.allNamespaces}
	return func() tea.Msg { return msg }
}

// clearHits drops every result and rewinds the cursor. It also drops the progress
// counters and the cap flag: they describe the fan-out that produced those results, so
// they go stale at exactly the same moment (a query change, a scope change, nav.back's
// clear, Reset) — keeping the reset in one place is why a caller never has to re-zero
// them itself. It leaves both widens alone: a scope outlives the results it produced,
// and only Reset (a fresh open) turns one back off.
func (m *Model) clearHits() {
	m.hits = m.hits[:0]
	m.rebuild()
	m.list.Select(0)
	m.kindsDone, m.kindsTotal = 0, 0
	m.capped = false
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
		// A hint is subtle; a query that cannot run is not — it is the one state
		// here the reader has to fix before anything else can happen.
		style := m.styles.Subtle
		if m.queryErr != "" {
			style = m.styles.Error
		}
		parts = append(parts, style.Width(m.width).MaxWidth(m.width).Render(m.emptyHint()))
	} else {
		parts = append(parts, m.list.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// emptyHint is the line shown in place of an empty result list: a query that cannot be
// searched at all, what to do on a blank query, the in-flight state while the fan-out
// runs, and an explicit no-match otherwise (an empty list must never read as a hung
// search).
//
// The error wins over everything because it is the only one of the four the reader must
// act on, and because the alternative — "no matches" for a query that was never run — is
// an outright lie about the cluster. The blank-query line advertises the label-selector
// syntax (SEARCH-04c-1): it is the one piece of this view that a reader cannot discover
// by pressing keys, and this line is already the moment they are looking for what to type.
func (m Model) emptyHint() string {
	switch {
	case m.queryErr != "":
		return m.queryErr
	case m.query.Value() == "":
		return "type to search this cluster · -l app=web to match labels"
	case m.searching:
		return "searching…"
	default:
		return "no matches"
	}
}

// header builds the one-line status bar: "search · <scope> · N results" plus the
// progress/cap segment, clipped to the screen width.
//
// The two widens are named differently because the two defaults are. The kind widen
// gets a segment only when it is *on* (`all kinds`), the same asymmetry LOGS-04a's
// `[wrap]` uses (D146): the curated kind scope is the default a reader already has in
// mind, the widen is the expensive exceptional state, and the header is clipped from
// the right so a segment spent naming the normal case costs the progress line. The
// namespace scope, by contrast, is *already* named in every header — that is what the
// scope segment is — so widening it **replaces** that name rather than adding to it. A
// header must never carry two namespace scopes at once; "web · all namespaces" would
// be a contradiction, not extra information. (On an app that is already unscoped, the
// wiring's own label is the same "all namespaces", so the toggle correctly changes
// nothing on screen and nothing on the wire.)
func (m Model) header() string {
	seg := kind
	switch {
	case m.allNamespaces:
		seg += " · " + allNamespacesLabel
	case m.scope != "":
		seg += " · " + m.scope
	}
	if m.allKinds {
		seg += " · all kinds"
	}
	seg += " · " + itoa(len(m.hits)) + " results"
	if s := m.progress(); s != "" {
		seg += " · " + s
	}
	return m.styles.Header.Width(m.width).MaxWidth(m.width).Render(clip(seg, m.width))
}

// progress is the header's fan-out segment, in one of four states:
//
//   - capped — the cap truncated the matches, so the count on screen is not the whole
//     answer and the fix is a narrower query. It wins over the progress line even while
//     the remaining kinds drain, because it is the only state the reader must act on,
//     and it stays up after the search ends (the counters do not).
//   - searching with a known kind count — "searching 3/11 kinds…", so a slow kind reads
//     as progress rather than as a hang, and a big scope is visibly a big scope.
//   - searching with no count yet — the debounce window, before the wiring has picked
//     the scope: still "searching…", so the keystroke is never followed by silence.
//   - idle and uncapped — nothing; the result count already says everything.
func (m Model) progress() string {
	switch {
	case m.capped:
		return "first " + itoa(len(m.hits)) + " matches — narrow the query"
	case m.searching && m.kindsTotal > 0:
		return "searching " + itoa(m.kindsDone) + "/" + itoa(m.kindsTotal) + " kinds…"
	case m.searching:
		return "searching…"
	default:
		return ""
	}
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
