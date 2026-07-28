package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/searchview"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// This file is the app wiring of the cluster-search mini-app (SEARCH-02b): the
// search.cluster action, the Searcher seam over kube.Search (D131), the debounce +
// generation-guarded hit pump that feeds the SEARCH-02a view, and the drill-in that
// switches the browse view to a hit. The view itself owns the query and the result
// list (D140) and never touches a client; everything concurrent lives here, behind
// messages (principle 1).

// searchHitLimit caps how many hits one query collects. kube.Search cancels the
// still-running lists once the cap is reached (D131 pt 4), so this is both a result
// bound and a load bound on a big cluster. It is far more rows than a reader will
// ever walk — the answer to "too many matches" is a narrower query, not a longer
// list, which is exactly what the view's cap state says once kube reports
// SearchDone{Capped} (SEARCH-03b).
const searchHitLimit = 200

// searchDebounce is how long the query must sit still before the fan-out launches.
// Every keystroke drops the previous query's results (D140 pt 3) and would otherwise
// start a fresh fan-out over every curated kind, so typing "nginx" would issue five
// full sweeps of the namespace; waiting a beat means one. It is short enough to feel
// immediate and long enough that ordinary typing never launches a search per key.
const searchDebounce = 250 * time.Millisecond

// Searcher runs a one-shot, cancellable cluster search over the given kinds and
// streams the matches — and its own progress (SEARCH-03a) — back (kube.Clients
// implements it via Search). It is the narrow seam the search mini-app needs,
// injected with WithSearcher — nil leaves the model search-inert (the search.cluster
// action never opens the view), exactly as a nil watcher leaves it watch-inert.
type Searcher interface {
	Search(ctx context.Context, resources []kube.Resource, namespace, query string, limit int) <-chan kube.SearchEvent
}

// WithSearcher wires the cluster-search client (nil → search-inert).
func WithSearcher(s Searcher) Option {
	return func(m *Model) { m.searcher = s }
}

// searchDebouncedMsg fires when a query has sat still for searchDebounce and the
// fan-out may launch. Its gen must still match the model's searchGen, or the query
// moved on while the timer ran and the tick is stale (the seqTimeoutMsg guard).
type searchDebouncedMsg struct {
	gen   int
	query string
}

// searchMsg wraps one message from the search pump with the generation of the query
// it belongs to. Every query change and every close bumps searchGen, so a hit from a
// superseded fan-out — whose channel is still draining after cancellation — is
// dropped rather than appended under a query it does not describe (D140 pt 3), and
// its pump chain stops instead of racing a second reader onto the live channel. The
// same stale-message guard watchGen gives the table watch.
type searchMsg struct {
	gen int
	msg tea.Msg
}

// openSearch shows the cluster-search view (search.cluster). It is a no-op without a
// searcher wired (search-inert) — an empty search box that can never return anything
// is worse than an unbound key. The view opens clean (Reset drops any previous
// query and its hits) with the scope it will search named in its header, and captures
// every keypress until it closes: while it is up the root routes text into its query
// field and control keys to it as actions (routeSearchKey, D140 pt 1).
func (m Model) openSearch() (tea.Model, tea.Cmd) {
	if m.searcher == nil {
		return m, nil
	}
	m.searchView.Reset()
	m.searchView.SetScope(m.searchScope())
	cmd := m.searchView.Show()
	m.syncHints() // the search view owns input now → search-context hints
	return m, cmd
}

// searchScope is the human label for what a search covers by default: the watched
// namespace, or the all-namespaces sentinel when the app is unscoped. It is the
// *starting* namespace label only — both widens are the view's own flags and the view
// renders them itself (SEARCH-04a/04b), overriding this label while the namespace widen
// is on.
func (m Model) searchScope() string {
	if m.namespace == "" {
		return namespaceAllItem
	}
	return m.namespace
}

// searchNamespace is the namespace one query fans out over: the app's own namespace, or
// every namespace while the view's widen is on (kube.Search takes "" for all, SEARCH-04b).
//
// The widen is per-search and one-directional: it never touches m.namespace, so the
// browse table keeps watching exactly what it was watching and closing the search leaves
// the app's scope where the reader put it. An app that is already unscoped is unaffected
// by the toggle — "" either way — which is why the header does not change there either.
func (m Model) searchNamespace() string {
	if m.searchView.AllNamespaces() {
		return ""
	}
	return m.namespace
}

// searchResources is the kind set one query fans out over, taken from the kinds the menu
// currently offers — the same source the resource palette draws on, so discovered kinds
// and per-context extras are included and an unavailable kind is skipped.
//
// Which of them are searched is the view's all-kinds flag (SEARCH-04a): off (the default,
// D131 pt 2) narrows to the curated high-signal set, on hands over everything discovery
// found. The widen has to be asked for because enumerating every type is exactly the
// expensive enumeration the fast-start design avoids (D8/principle 4) — the load it does
// cost is bounded inside kube.Search, which lists a fixed number of kinds at a time.
func (m Model) searchResources() []kube.Resource {
	items := m.menu.Items()
	all := make([]kube.Resource, 0, len(items))
	for _, it := range items {
		if it.Kind != menu.ItemResource || !it.Available {
			continue
		}
		all = append(all, it.Resource)
	}
	if m.searchView.AllKinds() {
		return all
	}
	return kube.CommonSearchResources(all)
}

// routeSearchKey resolves one keypress while the search view is up. The view's query
// field is always open (D140 pt 1), so the split is by whether the key carries text:
// a mapped key with no text (esc/enter/arrows/ctrl+…) is a control Action the view
// consumes — navigation moves the result cursor, nav.drillIn opens the hit, nav.back
// clears the query then closes — while anything text-producing or editing (a rune, or
// an unmapped no-text key like backspace) is query input. So `q` types a `q` instead
// of quitting, exactly as it does in the table filter; app.quit (ctrl+c) closes the
// search view rather than the app, the way the help overlay and the viewer own quit
// while they are open. No view matches a raw key for behaviour (D11).
func (m Model) routeSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		if action == keymap.ActionQuit {
			m.closeSearch()
			return m, nil
		}
		var cmd tea.Cmd
		m.searchView, cmd = m.searchView.Update(action)
		return m, cmd
	}
	var cmd tea.Cmd
	m.searchView, cmd = m.searchView.UpdateQuery(msg)
	return m, cmd
}

// handleSearchQueryChanged reacts to the view's QueryChangedMsg — the cue that the query
// moved (including to "" on nav.back's first press or the last character deleted). It
// restarts the search on the new text; see restartSearch for what that entails.
func (m Model) handleSearchQueryChanged(msg searchview.QueryChangedMsg) (tea.Model, tea.Cmd) {
	return m.restartSearch(msg.Query)
}

// handleSearchScopeChanged reacts to the view's ScopeChangedMsg — one of the two scope
// widens was toggled: kinds (SEARCH-04a) or namespaces (SEARCH-04b). Both take the same
// path, because both change the same thing: what the query on screen covers. The message
// carries the new scope but nothing here needs to read it — searchResources and
// searchNamespace resolve it off the view at launch, one source per axis.
//
// The scope is half of what a result set means, so changing it
// invalidates the in-flight fan-out exactly as retyping the query would, and the answer is
// the same: cancel, supersede, re-run whatever is currently typed. The query itself is
// untouched, so it is read back off the view rather than carried in the message.
//
// It goes through the same debounce as typing rather than launching at once. The delay is
// not the point — holding the widen down cannot produce a burst of full-cluster sweeps is.
// A toggle is one keystroke, but two of them (on, off again) are two queries' worth of
// listing, and under the widen that is the most expensive thing this app can be asked to
// do; letting the last press win costs a beat and bounds the damage to one sweep.
func (m Model) handleSearchScopeChanged(searchview.ScopeChangedMsg) (tea.Model, tea.Cmd) {
	return m.restartSearch(m.searchView.Query())
}

// restartSearch cancels whatever fan-out was in flight, makes its remaining hits stale
// (the searchGen bump), and arms the debounce timer for query. An empty query — or a
// search-inert model — cancels and searches nothing. The view has already dropped the
// previous results, so the screen never shows hits from a query or a scope that is no
// longer in force (D140 pt 3). The in-flight indicator goes up now rather than when the
// lists actually start, so a keystroke is never followed by a silent, blank pause.
func (m Model) restartSearch(query string) (tea.Model, tea.Cmd) {
	m.stopSearch()
	m.searchGen++
	if query == "" || m.searcher == nil {
		m.searchView.SetSearching(false)
		return m, nil
	}
	m.searchView.SetSearching(true)
	gen := m.searchGen
	return m, tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchDebouncedMsg{gen: gen, query: query}
	})
}

// handleSearchDebounced launches the fan-out for a query that has sat still long
// enough. A tick whose generation was superseded (the user typed on, or closed the
// view) is dropped, so only the latest query ever reaches the cluster. The search
// itself runs off the update loop on its own cancellable context — stopSearch tears
// it down on the next query change, on close, and on quit — and its hits stream in
// through the generation-tagged pump.
func (m Model) handleSearchDebounced(msg searchDebouncedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.searchGen || !m.searchView.Active() || m.searcher == nil {
		return m, nil
	}
	resources := m.searchResources()
	if len(resources) == 0 {
		// Nothing in the curated scope is available (discovery failed outright, or
		// every curated kind is denied): degrade to "no matches" rather than spin on
		// an in-flight indicator that will never clear (principle 3).
		m.searchView.SetSearching(false)
		return m, nil
	}
	// The kind count is only known here — the scope is derived from what discovery
	// currently offers — so this is where the view's progress line gets its
	// denominator (SEARCH-03b).
	m.searchView.StartProgress(len(resources))
	ctx, cancel := context.WithCancel(context.Background())
	m.searchCancel = cancel
	m.searchCh = m.searcher.Search(ctx, resources, m.searchNamespace(), msg.query, searchHitLimit)
	return m, m.pumpSearch(msg.gen)
}

// pumpSearch issues the tea.Cmd that pulls the next hit from the current search
// channel, tagged with the generation of the query that started it so a hit from a
// superseded fan-out is recognisable as stale. It returns nil when no search is
// running.
func (m Model) pumpSearch(gen int) tea.Cmd {
	ch := m.searchCh
	if ch == nil {
		return nil
	}
	pump := searchPump(ch)
	return func() tea.Msg { return searchMsg{gen: gen, msg: pump()} }
}

// handleSearchMsg folds one pumped event into the view and re-issues the pump to pull
// the next — the one-receive-per-Cmd loop that keeps Update from ever blocking
// (M2-02/D53), which is also what makes results appear kind by kind instead of all at
// once. A message from a superseded query, or one arriving after the view closed, is
// dropped and its chain stops. The closed channel ends the fan-out: the in-flight
// indicator clears, leaving the hits on screen (the view says "no matches" itself when
// there were none, D140 pt 5).
//
// A match is appended; a kind-done advances the view's progress line (one per requested
// kind, D142 pt 2, so it reaches N/N whatever each kind's outcome was — a denied group
// is silent, D131 pt 3, not a stalled counter); the terminal event's Capped flag turns
// on the "first N matches — narrow the query" state, the one thing the channel close
// cannot say for itself. The terminal SearchDone is deliberately *not* treated as the
// end of the stream: the channel close remains the single point where the pump chain
// stops, so there is one teardown path however the search ended.
func (m Model) handleSearchMsg(s searchMsg) (tea.Model, tea.Cmd) {
	if s.gen != m.searchGen || !m.searchView.Active() {
		return m, nil
	}
	switch inner := s.msg.(type) {
	case SearchEventMsg:
		switch inner.Event.Type {
		case kube.SearchMatch:
			m.searchView.AppendHit(inner.Event.Hit)
		case kube.SearchKindDone:
			m.searchView.MarkKindDone()
		case kube.SearchDone:
			m.searchView.SetCapped(inner.Event.Capped)
		}
		return m, m.pumpSearch(s.gen)
	case SearchClosedMsg:
		m.stopSearch()
		m.searchView.SetSearching(false)
		return m, nil
	}
	return m, nil
}

// handleSearchSelected drills into the highlighted hit: it closes the search view and
// switches the browse view to the hit's kind through the same selectResource path a
// menu drill-in or the resource palette takes (start the watch, mark the kind active,
// focus the table). The object itself cannot be selected yet — the fresh watch blanks
// the table and repopulates it when its first RESET lands — so the hit's identity is
// stashed as a pending selection the watch pump applies as soon as its row appears.
func (m Model) handleSearchSelected(msg searchview.SelectedMsg) (tea.Model, tea.Cmd) {
	m.closeSearch()
	next, cmd := m.selectResource(msg.Hit.Resource)
	sel := next.(Model)
	// Set after selectResource, which clears any pending selection of its own.
	sel.searchTarget = msg.Hit.Ref
	sel.hasSearchTarget = true
	return sel, cmd
}

// applyPendingSelect selects the object a search drill-in is waiting for, once the
// live watch has delivered its row. It is called after every applied watch delta, and
// clears the pending target the first time the row is found — so the reader's own
// navigation is never yanked back to it by a later delta. A target whose row never
// arrives (deleted between the search and the watch, or filtered out) simply stays
// pending until the next resource selection clears it, leaving the table's own
// selection untouched.
func (m *Model) applyPendingSelect() {
	if !m.hasSearchTarget {
		return
	}
	if m.table.SelectObject(m.searchTarget) {
		m.searchTarget = kube.ObjectRef{}
		m.hasSearchTarget = false
	}
}

// closeSearch dismisses the search view and cancels any fan-out feeding it, bumping
// the generation so hits still draining from the cancelled search are dropped rather
// than appended to a view that is no longer up. It mutates the receiver, so callers
// pass the addressable model they are about to return.
func (m *Model) closeSearch() {
	m.searchView.Hide()
	m.stopSearch()
	m.searchGen++
	m.syncHints() // back to the browse view → menu/table-context hints
}

// stopSearch cancels the running fan-out (if any) and clears its handle so no further
// hit is pumped. Safe to call with no search running. Called before launching a new
// query, when the fan-out completes, when the view closes, and on quit — the search
// twin of stopLogStream. It deliberately does not bump the generation: completion is
// not a supersede, and a later close/query change owns that bump.
func (m *Model) stopSearch() {
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	m.searchCh = nil
}
