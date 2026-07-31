package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/searchview"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// searchKey is the default search.cluster key (ctrl+s). A ctrl chord carries no text,
// so it resolves to an action rather than typing — which is exactly why it can also be
// pressed while the search view's always-open query field has focus.
var searchKey = tea.Key{Code: 's', Mod: tea.ModCtrl}

// fakeSearcher is a hermetic Searcher: it records what each query asked for and hands
// back a channel preloaded with the configured hits as SearchMatch events, then any
// extra events (progress / terminal, SEARCH-03a), closed unless keepOpen so a test can
// assert either the streaming/completion path or the cancellation path. The contexts
// are kept so a test can assert a fan-out was torn down.
type fakeSearcher struct {
	hits     []kube.SearchHit
	events   []kube.SearchEvent
	keepOpen bool

	calls    int
	ctxs     []context.Context
	gotRes   []kube.Resource
	gotNS    string
	gotQuery kube.SearchQuery
	gotLimit int
}

func (f *fakeSearcher) Search(ctx context.Context, resources []kube.Resource, namespace string, query kube.SearchQuery, limit int) <-chan kube.SearchEvent {
	f.calls++
	f.ctxs = append(f.ctxs, ctx)
	f.gotRes = resources
	f.gotNS = namespace
	f.gotQuery = query
	f.gotLimit = limit
	ch := make(chan kube.SearchEvent, len(f.hits)+len(f.events)+1)
	for _, h := range f.hits {
		ch <- kube.SearchEvent{Type: kube.SearchMatch, Hit: h}
	}
	for _, ev := range f.events {
		ch <- ev
	}
	if !f.keepOpen {
		close(ch)
	}
	return ch
}

func searchHit(kind, resource, ns, name string) kube.SearchHit {
	r := kindResource(resource, kind)
	r.Namespaced = ns != ""
	return kube.SearchHit{Resource: r, Ref: kube.ObjectRef{Namespace: ns, Name: name}}
}

// openSearchView presses the search.cluster key over a wired searcher and returns the
// model with the view up.
func openSearchView(t *testing.T, s Searcher, opts ...Option) Model {
	t.Helper()
	m := sizedWith(t, append([]Option{WithSearcher(s)}, opts...)...)
	m, _ = press(t, m, searchKey)
	if !m.searchView.Active() {
		t.Fatal("search.cluster should open the cluster-search view")
	}
	return m
}

// queryChangedFrom extracts the view's QueryChangedMsg from a keypress's command. A
// keystroke into the query field batches the textinput's own cursor command with the
// change notification, so the batch is flattened (as Bubble Tea does) to find it.
func queryChangedFrom(t *testing.T, cmd tea.Cmd) searchview.QueryChangedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("a query change should produce a command")
	}
	var find func(tea.Msg) (searchview.QueryChangedMsg, bool)
	find = func(msg tea.Msg) (searchview.QueryChangedMsg, bool) {
		switch msg := msg.(type) {
		case searchview.QueryChangedMsg:
			return msg, true
		case tea.BatchMsg:
			for _, c := range msg {
				if c == nil {
					continue
				}
				if got, ok := find(c()); ok {
					return got, true
				}
			}
		}
		return searchview.QueryChangedMsg{}, false
	}
	got, ok := find(cmd())
	if !ok {
		t.Fatalf("expected a QueryChangedMsg among the produced commands, got %T", cmd())
	}
	return got
}

// typeQuery feeds q into the open search view one keypress at a time, delivering each
// resulting QueryChangedMsg, and returns the model plus the command the *last* change
// produced (the debounce tick).
func typeQuery(t *testing.T, m Model, q string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range q {
		var keyCmd tea.Cmd
		m, keyCmd = press(t, m, tea.Key{Code: r, Text: string(r)})
		var next tea.Model
		next, cmd = m.Update(queryChangedFrom(t, keyCmd))
		m = next.(Model)
	}
	return m, cmd
}

// TestSearchInertWithoutSearcher proves the model is search-inert with no searcher
// wired: the key opens nothing rather than an empty box that can never answer.
func TestSearchInertWithoutSearcher(t *testing.T) {
	m := sized(t)
	m, _ = press(t, m, searchKey)
	if m.searchView.Active() {
		t.Fatal("with no searcher wired search.cluster must not open the view")
	}
}

// TestSearchOpensFullScreenView proves the view opens clean, names the scope it will
// search, and — unlike every other surface — *replaces* the browse body rather than
// overlaying it (D134): the menu's resource rows are gone from the frame.
func TestSearchOpensFullScreenView(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{}, WithNamespace("web"))
	if q := m.searchView.Query(); q != "" {
		t.Fatalf("the search view should open with an empty query, got %q", q)
	}
	view := m.View().Content
	if !strings.Contains(view, "web") {
		t.Fatalf("the search header should name the searched namespace: %q", view)
	}
	if !strings.Contains(view, "type to search this cluster") {
		t.Fatalf("an empty query should show the what-to-do hint: %q", view)
	}
	if strings.Contains(view, "Workloads") {
		t.Fatalf("the search view should replace the browse panes, not overlay them: %q", view)
	}
}

// TestSearchUnscopedNamesAllNamespaces proves the header says "all namespaces" when the
// app is unscoped, so a search's reach is never ambiguous.
func TestSearchUnscopedNamesAllNamespaces(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{})
	if scope := m.searchScope(); scope != namespaceAllItem {
		t.Fatalf("an unscoped app should search %q, got %q", namespaceAllItem, scope)
	}
}

// TestSearchTypingLaunchesOneDebouncedFanOut proves the debounce: typing three
// characters launches no search of its own, and only the tick of the final query
// reaches the cluster — with the curated kind set, the current namespace, and the cap.
func TestSearchTypingLaunchesOneDebouncedFanOut(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	if s.calls != 0 {
		t.Fatalf("typing must not launch a fan-out per keystroke, got %d searches", s.calls)
	}
	if !m.searchView.Searching() {
		t.Fatal("the in-flight indicator should go up as soon as the query changes")
	}
	if tick == nil {
		t.Fatal("a query change should arm the debounce tick")
	}
	debounced, ok := tick().(searchDebouncedMsg)
	if !ok {
		t.Fatalf("the query change should arm a searchDebouncedMsg, got %T", tick())
	}
	if debounced.query != (kube.SearchQuery{Name: "api"}) {
		t.Fatalf("the debounce should carry the whole typed query, got %+v", debounced.query)
	}
	next, _ := m.Update(debounced)
	m = next.(Model)
	if s.calls != 1 {
		t.Fatalf("the debounced query should launch exactly one fan-out, got %d", s.calls)
	}
	if s.gotQuery != (kube.SearchQuery{Name: "api"}) || s.gotNS != "web" || s.gotLimit != searchHitLimit {
		t.Fatalf("the fan-out should search %q in %q capped at %d, got %+v/%q/%d",
			"api", "web", searchHitLimit, s.gotQuery, s.gotNS, s.gotLimit)
	}
}

// TestSearchScopeIsCurated proves the default scope is the curated kind set (D131 pt 2),
// selected from the kinds the menu offers: Pods and Deployments are in, the
// non-curated seed kinds (Nodes, Namespaces, Events) are out — the default must never
// enumerate every type.
func TestSearchScopeIsCurated(t *testing.T) {
	m := sizedWith(t, WithSearcher(&fakeSearcher{}))
	kinds := map[string]bool{}
	for _, r := range m.searchResources() {
		kinds[r.GVK.Kind] = true
	}
	for _, want := range []string{"Pod", "Deployment", "Service", "ConfigMap", "Secret", "Ingress"} {
		if !kinds[want] {
			t.Errorf("the curated search scope should include %s", want)
		}
	}
	for _, unwanted := range []string{"Node", "Namespace", "Event", "Role"} {
		if kinds[unwanted] {
			t.Errorf("the curated search scope must not include %s (the widen is opt-in, SEARCH-04a)", unwanted)
		}
	}
}

// allKindsKey is the default search.allKinds key (ctrl+a). Like search.cluster it is a
// ctrl chord carrying no text, so it survives the search view's always-open query field
// instead of being typed into it (D140 pt 1).
var allKindsKey = tea.Key{Code: 'a', Mod: tea.ModCtrl}

// scopeChangedFrom extracts the view's ScopeChangedMsg from a keypress's command,
// flattening any batch exactly as queryChangedFrom does for a query change.
func scopeChangedFrom(t *testing.T, cmd tea.Cmd) searchview.ScopeChangedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("the all-kinds widen should produce a command")
	}
	var find func(tea.Msg) (searchview.ScopeChangedMsg, bool)
	find = func(msg tea.Msg) (searchview.ScopeChangedMsg, bool) {
		switch msg := msg.(type) {
		case searchview.ScopeChangedMsg:
			return msg, true
		case tea.BatchMsg:
			for _, c := range msg {
				if c == nil {
					continue
				}
				if got, ok := find(c()); ok {
					return got, true
				}
			}
		}
		return searchview.ScopeChangedMsg{}, false
	}
	got, ok := find(cmd())
	if !ok {
		t.Fatalf("expected a ScopeChangedMsg among the produced commands, got %T", cmd())
	}
	return got
}

// pressAllKinds toggles the widen through the sequencer and delivers the resulting
// ScopeChangedMsg, returning the model and the command that change produced (the
// debounce tick that re-runs the query, or nil when there is nothing to re-run).
func pressAllKinds(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, keyCmd := press(t, m, allKindsKey)
	next, cmd := m.Update(scopeChangedFrom(t, keyCmd))
	return next.(Model), cmd
}

// TestSearchAllKindsWidensTheFanOut is SEARCH-04a end to end: the same query, re-run
// after the widen, goes out over every kind the menu offers instead of the curated
// subset — and the kinds the curated scope deliberately excludes are exactly the ones
// that appear.
func TestSearchAllKindsWidensTheFanOut(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	curated := map[string]bool{}
	for _, r := range s.gotRes {
		curated[r.GVK.Kind] = true
	}
	if curated["Node"] || curated["Event"] {
		t.Fatalf("the default fan-out should be curated, got kinds %v", s.gotRes)
	}

	m, widened := pressAllKinds(t, m)
	if !m.searchView.AllKinds() {
		t.Fatal("search.allKinds should widen the kind scope through the sequencer")
	}
	if widened == nil {
		t.Fatal("widening should re-run the current query, not wait for another keystroke")
	}
	debounced, ok := widened().(searchDebouncedMsg)
	if !ok {
		t.Fatalf("the widen should arm a searchDebouncedMsg, got %T", widened())
	}
	if debounced.query != (kube.SearchQuery{Name: "api"}) {
		t.Fatalf("the re-run should carry the query already typed, got %+v", debounced.query)
	}
	next, _ = m.Update(debounced)
	m = next.(Model)

	if s.calls != 2 {
		t.Fatalf("the widen should launch exactly one further fan-out, got %d total", s.calls)
	}
	all := map[string]bool{}
	for _, r := range s.gotRes {
		all[r.GVK.Kind] = true
	}
	for _, want := range []string{"Pod", "Deployment", "Node", "Namespace", "Event"} {
		if !all[want] {
			t.Errorf("the widened fan-out should include %s", want)
		}
	}
	if s.gotQuery != (kube.SearchQuery{Name: "api"}) || s.gotNS != "web" {
		t.Errorf("the widen changes the kinds only: got query %+v in %q, want %q in %q",
			s.gotQuery, s.gotNS, "api", "web")
	}
}

// TestSearchAllKindsCancelsTheNarrowerFanOut proves the widen supersedes rather than
// races: the in-flight curated search is torn down (D131 pt 4) exactly as a query change
// tears it down, so two scopes never stream into one result list.
func TestSearchAllKindsCancelsTheNarrowerFanOut(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s)
	m, tick := typeQuery(t, m, "api")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	if s.calls != 1 {
		t.Fatalf("expected one in-flight fan-out, got %d", s.calls)
	}

	m, _ = pressAllKinds(t, m)
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("widening the scope should cancel the fan-out running under the narrower one")
	}
	if !m.searchView.Searching() {
		t.Error("the in-flight indicator should stay up across the re-run, not blink off")
	}
}

// TestSearchAllKindsOnEmptyQuerySearchesNothing proves choosing the scope before typing
// is free: the widen is recorded and announced, but an empty query is still not a search
// for everything — nothing goes to the cluster until something is typed.
func TestSearchAllKindsOnEmptyQuerySearchesNothing(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s)
	m, cmd := pressAllKinds(t, m)
	if !m.searchView.AllKinds() {
		t.Fatal("the widen should be settable before a query is typed")
	}
	if cmd != nil {
		t.Fatal("an empty query should arm no debounce tick, widened or not")
	}
	if s.calls != 0 {
		t.Fatalf("an empty query must launch no fan-out, got %d", s.calls)
	}
	if !strings.Contains(m.View().Content, "all kinds") {
		t.Errorf("the widened scope should be named in the frame; got:\n%s", m.View().Content)
	}
}

// TestSearchAllKindsLetterTypesIntoTheQuery is the D140 pt 1 half: the widen is bound to
// a chord, so a plain `a` is still text. Pressing it types rather than widening — the
// same split that keeps `q` from quitting while the query field is open.
func TestSearchAllKindsLetterTypesIntoTheQuery(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{})
	m, _ = typeQuery(t, m, "a")
	if m.searchView.AllKinds() {
		t.Error("a plain `a` must type into the query, not widen the search scope")
	}
	if m.searchView.Query() != "a" {
		t.Errorf("query = %q, want the typed %q", m.searchView.Query(), "a")
	}
}

// TestSearchWidenDoesNotSurviveReopen proves a widen belongs to one visit to the view:
// a fresh search.cluster starts curated and namespace-scoped, so an expensive scope
// chosen minutes ago can never quietly make the next search sweep the cluster.
func TestSearchWidenDoesNotSurviveReopen(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{}, WithNamespace("web"))
	m, _ = pressAllKinds(t, m)
	m, _ = pressAllNamespaces(t, m)
	if !m.searchView.AllKinds() || !m.searchView.AllNamespaces() {
		t.Fatal("both widens should be on before the view is closed")
	}
	m.closeSearch()
	m, _ = press(t, m, searchKey)
	if !m.searchView.Active() {
		t.Fatal("search.cluster should reopen the view")
	}
	if m.searchView.AllKinds() {
		t.Error("reopening the search view must start from the curated kind scope")
	}
	if m.searchView.AllNamespaces() {
		t.Error("reopening the search view must start from the app's own namespace")
	}
}

// allNamespacesKey is the default search.allNamespaces key (ctrl+w). Like the kind widen
// beside it, it is a ctrl chord carrying no text, so the always-open query field cannot
// swallow it (D140 pt 1).
var allNamespacesKey = tea.Key{Code: 'w', Mod: tea.ModCtrl}

// pressAllNamespaces toggles the namespace widen through the sequencer and delivers the
// resulting ScopeChangedMsg, mirroring pressAllKinds.
func pressAllNamespaces(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, keyCmd := press(t, m, allNamespacesKey)
	next, cmd := m.Update(scopeChangedFrom(t, keyCmd))
	return next.(Model), cmd
}

// TestSearchAllNamespacesWidensTheFanOut is SEARCH-04b end to end: the same query, re-run
// after the widen, goes out with the all-namespaces "" instead of the app's namespace —
// and nothing else moves. The kind scope stays curated (the two axes are independent) and,
// the part that matters beyond this view, the *app's* namespace is untouched: widening a
// search must never re-scope the browse table the reader will return to.
func TestSearchAllNamespacesWidensTheFanOut(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	if s.gotNS != "web" {
		t.Fatalf("the default fan-out should search the app's namespace, got %q", s.gotNS)
	}

	m, widened := pressAllNamespaces(t, m)
	if !m.searchView.AllNamespaces() {
		t.Fatal("search.allNamespaces should widen the namespace scope through the sequencer")
	}
	if widened == nil {
		t.Fatal("widening should re-run the current query, not wait for another keystroke")
	}
	debounced, ok := widened().(searchDebouncedMsg)
	if !ok {
		t.Fatalf("the widen should arm a searchDebouncedMsg, got %T", widened())
	}
	if debounced.query != (kube.SearchQuery{Name: "api"}) {
		t.Fatalf("the re-run should carry the query already typed, got %+v", debounced.query)
	}
	next, _ = m.Update(debounced)
	m = next.(Model)

	if s.calls != 2 {
		t.Fatalf("the widen should launch exactly one further fan-out, got %d total", s.calls)
	}
	if s.gotNS != "" {
		t.Errorf("the widened fan-out should search every namespace (\"\"), got %q", s.gotNS)
	}
	if s.gotQuery != (kube.SearchQuery{Name: "api"}) {
		t.Errorf("the widen changes the namespace only, got query %+v", s.gotQuery)
	}
	kinds := map[string]bool{}
	for _, r := range s.gotRes {
		kinds[r.GVK.Kind] = true
	}
	if kinds["Node"] || kinds["Event"] {
		t.Errorf("widening namespaces must leave the kind scope curated, got kinds %v", s.gotRes)
	}
	if m.namespace != "web" {
		t.Errorf("the app's own namespace = %q, want it untouched by a search widen", m.namespace)
	}
}

// TestSearchBothWidensCompose proves the two axes stack: with both on, one query goes out
// over every discovered kind in every namespace — the widest search the app can do, and
// only ever by asking for it twice.
func TestSearchBothWidensCompose(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, _ = typeQuery(t, m, "api")
	m, _ = pressAllKinds(t, m)
	m, widened := pressAllNamespaces(t, m)
	next, _ := m.Update(widened().(searchDebouncedMsg))
	m = next.(Model)

	if s.gotNS != "" {
		t.Errorf("namespace = %q, want every namespace", s.gotNS)
	}
	kinds := map[string]bool{}
	for _, r := range s.gotRes {
		kinds[r.GVK.Kind] = true
	}
	for _, want := range []string{"Pod", "Node", "Event"} {
		if !kinds[want] {
			t.Errorf("the doubly-widened fan-out should include %s", want)
		}
	}
	if !strings.Contains(m.View().Content, "all namespaces") || !strings.Contains(m.View().Content, "all kinds") {
		t.Errorf("both widened scopes should be named in the frame; got:\n%s", m.View().Content)
	}
}

// TestSearchAllNamespacesCancelsTheNarrowerFanOut is the kind widen's supersede contract
// on the namespace axis: the in-flight search of one namespace is torn down rather than
// left streaming into a result list that now claims to cover every namespace.
func TestSearchAllNamespacesCancelsTheNarrowerFanOut(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	if s.calls != 1 {
		t.Fatalf("expected one in-flight fan-out, got %d", s.calls)
	}

	m, _ = pressAllNamespaces(t, m)
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("widening the namespace should cancel the fan-out running under the narrower one")
	}
	if !m.searchView.Searching() {
		t.Error("the in-flight indicator should stay up across the re-run, not blink off")
	}
}

// TestSearchAllNamespacesLetterTypesIntoTheQuery is the D140 pt 1 half for the namespace
// widen: a plain `w` is text (it is logs.wrap in the browse context), so only the chord
// widens.
func TestSearchAllNamespacesLetterTypesIntoTheQuery(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{}, WithNamespace("web"))
	m, _ = typeQuery(t, m, "w")
	if m.searchView.AllNamespaces() {
		t.Error("a plain `w` must type into the query, not widen the namespace scope")
	}
	if m.searchView.Query() != "w" {
		t.Errorf("query = %q, want the typed %q", m.searchView.Query(), "w")
	}
}

// TestSearchStaleDebounceDropped proves only the latest query reaches the cluster: a
// tick armed by an earlier keystroke is dropped once the query has moved on.
func TestSearchStaleDebounceDropped(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s)
	m, firstTick := typeQuery(t, m, "a")
	stale := firstTick().(searchDebouncedMsg)
	m, _ = typeQuery(t, m, "p") // the query moved on, superseding the armed tick
	next, _ := m.Update(stale)
	if s.calls != 0 {
		t.Fatalf("a superseded debounce tick must launch no fan-out, got %d", s.calls)
	}
	if !next.(Model).searchView.Active() {
		t.Fatal("dropping a stale tick must not close the view")
	}
}

// TestSearchHitsStreamIn proves hits arrive one pump at a time (so results appear kind
// by kind, not in one batch) and that the closed channel clears the in-flight state
// while leaving the results on screen.
func TestSearchHitsStreamIn(t *testing.T) {
	s := &fakeSearcher{hits: []kube.SearchHit{
		searchHit("Pod", "pods", "web", "api-1"),
		searchHit("Deployment", "deployments", "web", "api"),
	}}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	// Each pumped hit lands in the view and re-issues the pump for the next one.
	for i := 1; i <= 2; i++ {
		if pump == nil {
			t.Fatalf("hit %d should have a pump command", i)
		}
		next, pump = m.Update(pump().(searchMsg))
		m = next.(Model)
		if m.searchView.Len() != i {
			t.Fatalf("after %d pumped hits the view should hold %d, got %d", i, i, m.searchView.Len())
		}
	}
	if !m.searchView.Searching() {
		t.Fatal("the fan-out should still read as in flight until its channel closes")
	}
	// The close terminates the chain, clears the indicator, and keeps the hits.
	next, done := m.Update(pump().(searchMsg))
	m = next.(Model)
	if done != nil {
		t.Fatal("a closed search channel must not re-issue the pump")
	}
	if m.searchView.Searching() {
		t.Fatal("a completed fan-out should clear the in-flight indicator")
	}
	if m.searchView.Len() != 2 {
		t.Fatalf("completion should keep the hits on screen, got %d", m.searchView.Len())
	}
	view := m.View().Content
	for _, want := range []string{"Pod", "web/api-1", "Deployment"} {
		if !strings.Contains(view, want) {
			t.Errorf("the streamed results should render %q: %q", want, view)
		}
	}
}

// TestSearchProgressEventsPumpThrough proves the widened stream (SEARCH-03a) keeps the
// pump chain intact: a kind-completion and the terminal done event are neither appended
// as results nor mistaken for the end of the stream — the channel close stays the single
// teardown point, so SEARCH-03b can count them without touching the lifecycle.
func TestSearchProgressEventsPumpThrough(t *testing.T) {
	s := &fakeSearcher{
		hits: []kube.SearchHit{searchHit("Pod", "pods", "web", "api-1")},
		events: []kube.SearchEvent{
			{Type: kube.SearchKindDone, Resource: kindResource("pods", "Pod")},
			{Type: kube.SearchDone, Capped: true},
		},
	}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	// hit → kind-done → terminal done: every one re-issues the pump, only the hit
	// lands in the result list.
	for i, want := range []int{1, 1, 1} {
		if pump == nil {
			t.Fatalf("event %d should have a pump command", i+1)
		}
		next, pump = m.Update(pump().(searchMsg))
		m = next.(Model)
		if got := m.searchView.Len(); got != want {
			t.Fatalf("after event %d the view should hold %d result(s), got %d", i+1, want, got)
		}
	}
	if !m.searchView.Searching() {
		t.Fatal("the terminal event is not the teardown — the close is (indicator still in flight)")
	}
	next, done := m.Update(pump().(searchMsg))
	if done != nil {
		t.Fatal("the closed channel must end the pump chain")
	}
	if next.(Model).searchView.Searching() {
		t.Fatal("the close should clear the in-flight indicator")
	}
}

// TestSearchProgressRendersAgainstTheLaunchedScope proves SEARCH-03b's progress line:
// the denominator is the kind count the fan-out was actually launched over (not a
// guess), each kind-done advances it, and the line is gone once the search completes.
func TestSearchProgressRendersAgainstTheLaunchedScope(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")

	// Before the launch there is no count to show — only "searching…".
	if strings.Contains(m.View().Content, "kinds") {
		t.Fatalf("the debounce window has no kind count yet: %q", m.View().Content)
	}
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	total := len(s.gotRes)
	if total < 2 {
		t.Fatalf("the curated scope should hold several kinds, got %d", total)
	}
	if done, got := m.searchView.Progress(); done != 0 || got != total {
		t.Fatalf("launching should arm the progress at 0/%d, got %d/%d", total, done, got)
	}
	if want := fmt.Sprintf("searching 0/%d kinds…", total); !strings.Contains(m.View().Content, want) {
		t.Fatalf("the header should render %q: %q", want, m.View().Content)
	}

	// Two kinds report done (one of them a failure — silent, D131 pt 3, but still
	// counted so a denied group can't stall the line).
	for _, ev := range []kube.SearchEvent{
		{Type: kube.SearchKindDone, Resource: kindResource("pods", "Pod")},
		{Type: kube.SearchKindDone, Resource: kindResource("secrets", "Secret"), Failed: true},
	} {
		next, _ = m.Update(searchMsg{gen: m.searchGen, msg: SearchEventMsg{Event: ev}})
		m = next.(Model)
	}
	if done, _ := m.searchView.Progress(); done != 2 {
		t.Fatalf("two kind-done events should advance the progress to 2, got %d", done)
	}
	if want := fmt.Sprintf("searching 2/%d kinds…", total); !strings.Contains(m.View().Content, want) {
		t.Fatalf("the header should render %q: %q", want, m.View().Content)
	}
	// A failed kind must not shout: nothing on screen names it or its error.
	if strings.Contains(m.View().Content, "Secret") || strings.Contains(m.View().Content, "failed") {
		t.Fatalf("per-kind failure stays silent (D131 pt 3): %q", m.View().Content)
	}

	// The channel close ends the search; the progress line goes with the in-flight state.
	next, _ = m.Update(searchMsg{gen: m.searchGen, msg: SearchClosedMsg{}})
	if strings.Contains(next.(Model).View().Content, "kinds") {
		t.Fatalf("a completed search should drop the progress line: %q", next.(Model).View().Content)
	}
}

// TestSearchCapSurfacedInView proves the cap state reaches the screen: kube's terminal
// SearchDone{Capped} (the one thing the channel close cannot express) becomes the
// actionable "first N matches — narrow the query" line, and it survives completion.
func TestSearchCapSurfacedInView(t *testing.T) {
	s := &fakeSearcher{
		hits: []kube.SearchHit{
			searchHit("Pod", "pods", "web", "api-1"),
			searchHit("Pod", "pods", "web", "api-2"),
		},
		events: []kube.SearchEvent{{Type: kube.SearchDone, Capped: true}},
	}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	// Drain hit, hit, terminal-done, then the close.
	for pump != nil {
		next, pump = m.Update(pump().(searchMsg))
		m = next.(Model)
	}
	if !m.searchView.Capped() {
		t.Fatal("the terminal SearchDone{Capped} should put the view in the capped state")
	}
	if want := "first 2 matches — narrow the query"; !strings.Contains(m.View().Content, want) {
		t.Fatalf("the header should render %q: %q", want, m.View().Content)
	}

	// An uncapped search says nothing of the sort.
	s2 := &fakeSearcher{
		hits:   []kube.SearchHit{searchHit("Pod", "pods", "web", "api-1")},
		events: []kube.SearchEvent{{Type: kube.SearchDone}},
	}
	m2 := openSearchView(t, s2, WithNamespace("web"))
	m2, tick2 := typeQuery(t, m2, "api")
	n2, p2 := m2.Update(tick2().(searchDebouncedMsg))
	m2 = n2.(Model)
	for p2 != nil {
		n2, p2 = m2.Update(p2().(searchMsg))
		m2 = n2.(Model)
	}
	if m2.searchView.Capped() || strings.Contains(m2.View().Content, "narrow the query") {
		t.Fatalf("an exhaustive search must not claim it was capped: %q", m2.View().Content)
	}
}

// TestSearchHintBarUsesSearchContext proves the bottom hint tracks the search view
// (SEARCH-03b): while it is up the hint offers only what the view actually honours —
// the browse keys are unreachable because the always-open query field eats every text
// key (D140 pt 1) — and closing the view restores the browse hint.
func TestSearchHintBarUsesSearchContext(t *testing.T) {
	// A wide terminal so the hint renderer elides nothing — the point here is which
	// bindings the context offers, not how they are truncated.
	wide, _ := New(WithSearcher(&fakeSearcher{})).Update(tea.WindowSizeMsg{Width: 220, Height: 24})
	m := wide.(Model)
	browseHint := m.hintbar.View()
	if !strings.Contains(browseHint, keymap.ActionNamespace.Describe()) {
		t.Fatalf("the browse hint should offer the namespace switch: %q", browseHint)
	}

	m, _ = press(t, m, searchKey)
	hint := m.hintbar.View()
	if !strings.Contains(hint, keymap.ActionDrillIn.Describe()) {
		t.Errorf("the search hint should offer drill-in (open the hit): %q", hint)
	}
	if !strings.Contains(hint, keymap.ActionBack.Describe()) {
		t.Errorf("the search hint should offer back (clear then close): %q", hint)
	}
	for _, unreachable := range []keymap.Action{keymap.ActionFilter, keymap.ActionNamespace, keymap.ActionHelp, keymap.ActionQuit} {
		if strings.Contains(hint, unreachable.Describe()) {
			t.Errorf("%q is unreachable while the query field owns text keys; hint must not offer it: %q",
				unreachable, hint)
		}
	}

	next, _ := m.Update(searchview.ClosedMsg{Kind: "search"})
	if got := next.(Model).hintbar.View(); got != browseHint {
		t.Errorf("closing the search view should restore the browse hint: %q, want %q", got, browseHint)
	}
}

// TestSearchStaleHitDropped proves the generation guard: a hit still draining from a
// superseded query is never appended under the query now on screen (D140 pt 3).
func TestSearchStaleHitDropped(t *testing.T) {
	s := &fakeSearcher{hits: []kube.SearchHit{searchHit("Pod", "pods", "web", "api-1")}}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	stale := pump().(searchMsg)
	m, _ = typeQuery(t, m, "x") // a new query supersedes the in-flight fan-out
	next, again := m.Update(stale)
	m = next.(Model)
	if m.searchView.Len() != 0 {
		t.Fatalf("a hit from a superseded query must be dropped, view holds %d", m.searchView.Len())
	}
	if again != nil {
		t.Fatal("a stale hit must not re-issue the pump onto the live channel")
	}
}

// TestSearchQueryChangeCancelsInFlight proves a new query tears the previous fan-out
// down rather than letting it keep listing (D131 pt 4).
func TestSearchQueryChangeCancelsInFlight(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s)
	m, tick := typeQuery(t, m, "a")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	if s.calls != 1 {
		t.Fatalf("expected one in-flight fan-out, got %d", s.calls)
	}
	typeQuery(t, m, "p") //nolint:staticcheck // only the cancellation side effect matters
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("a query change should cancel the in-flight fan-out")
	}
}

// TestSearchEmptiedQuerySearchesNothing proves clearing the query (nav.back's first
// press, or deleting the last character) cancels and launches nothing — an empty query
// is not a search for everything.
func TestSearchEmptiedQuerySearchesNothing(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s)
	m, tick := typeQuery(t, m, "a")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	// nav.back on a non-empty query clears it (and emits QueryChangedMsg{""}).
	m, backCmd := press(t, m, tea.Key{Code: tea.KeyEscape})
	changed := queryChangedFrom(t, backCmd)
	if changed.Query != "" {
		t.Fatalf("nav.back should clear the query, got %q", changed.Query)
	}
	next, emptied := m.Update(changed)
	m = next.(Model)
	if !m.searchView.Active() {
		t.Fatal("nav.back on a non-empty query should clear it, not close the view")
	}
	if emptied != nil {
		t.Fatal("an emptied query should arm no debounce tick")
	}
	if m.searchView.Searching() {
		t.Fatal("an emptied query should clear the in-flight indicator")
	}
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("an emptied query should cancel the in-flight fan-out")
	}
	if s.calls != 1 {
		t.Fatalf("an emptied query must launch no new fan-out, got %d searches", s.calls)
	}
}

// TestSearchCloseCancelsAndRestoresBrowse proves the view's ClosedMsg hides it, cancels
// the fan-out, and hands the frame back to the browse panes untouched.
func TestSearchCloseCancelsAndRestoresBrowse(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s)
	m, tick := typeQuery(t, m, "a")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)

	next, _ = m.Update(searchview.ClosedMsg{Kind: "search"})
	m = next.(Model)
	if m.searchView.Active() {
		t.Fatal("ClosedMsg should dismiss the search view")
	}
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("closing the view should cancel the in-flight fan-out")
	}
	if view := m.View().Content; !strings.Contains(view, "Workloads") {
		t.Fatalf("closing the search view should restore the browse panes: %q", view)
	}
}

// TestSearchTextKeysTypeIntoQuery proves the always-open query field owns text keys
// (D140 pt 1): `q` (app.quit) and `d` (res.delete) type instead of firing their
// actions, while app.quit's no-text chord (ctrl+c) closes the view rather than the app.
func TestSearchTextKeysTypeIntoQuery(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{})
	m, _ = typeQuery(t, m, "qd")
	if q := m.searchView.Query(); q != "qd" {
		t.Fatalf("text keys should type into the query field, got %q", q)
	}
	if !m.searchView.Active() {
		t.Fatal("typing `q` must not quit or close the search view")
	}
	m, cmd := press(t, m, tea.Key{Code: 'c', Mod: tea.ModCtrl})
	if m.searchView.Active() {
		t.Fatal("app.quit's chord should close the search view while it is up")
	}
	if cmd != nil {
		t.Fatal("app.quit inside the search view should close the view, not quit the app")
	}
}

// TestSearchDrillInSwitchesToHit proves the payoff: drilling into a hit closes the
// search, switches the browse view to that hit's kind, and — once the fresh watch
// delivers its rows — lands the selection on the searched object itself.
func TestSearchDrillInSwitchesToHit(t *testing.T) {
	fw := &fakeWatcher{}
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithWatcher(fw), WithNamespace("web"))
	hit := searchHit("Deployment", "deployments", "web", "api")

	next, cmd := m.Update(searchview.SelectedMsg{Kind: "search", Hit: hit})
	m = next.(Model)
	if m.searchView.Active() {
		t.Fatal("drilling into a hit should close the search view")
	}
	if len(fw.res) != 1 || fw.res[0].GVK.Kind != "Deployment" {
		t.Fatalf("drilling in should start a watch for the hit's kind, got %#v", fw.res)
	}
	if !m.hasCurrent || m.current.GVK.Kind != "Deployment" {
		t.Fatal("the browse view should be switched to the hit's kind")
	}
	if !m.hasSearchTarget || m.searchTarget.Name != "api" {
		t.Fatal("the hit's object should be pending selection until its row arrives")
	}
	if !m.table.Focused() {
		t.Fatal("drilling into a hit should focus the table, like any resource drill-in")
	}

	// The watch's first RESET lands: the pending object is selected, not row 0.
	fw.chans[0] <- kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: []kube.Column{{Name: "NAME"}},
		Rows: []kube.Row{
			{Cells: []any{"other"}, Object: kube.ObjectRef{Namespace: "web", Name: "other", UID: "o"}},
			{Cells: []any{"api"}, Object: kube.ObjectRef{Namespace: "web", Name: "api", UID: "a"}},
		},
	}
	next, _ = m.Update(cmd().(watchMsg))
	m = next.(Model)
	row, ok := m.table.SelectedRow()
	if !ok || row.Object.Name != "api" {
		t.Fatalf("the searched object should be selected once its row arrives, got %#v", row)
	}
	if m.hasSearchTarget {
		t.Fatal("a resolved pending selection should be cleared so later deltas don't re-select it")
	}
}

// TestSearchPendingSelectClearedOnResourceSwitch proves a pending selection belongs to
// the drill-in that armed it: selecting another resource drops it, so a later table's
// same-named row is never hijacked.
func TestSearchPendingSelectClearedOnResourceSwitch(t *testing.T) {
	fw := &fakeWatcher{}
	m := openSearchView(t, &fakeSearcher{}, WithWatcher(fw))
	next, _ := m.Update(searchview.SelectedMsg{Kind: "search", Hit: searchHit("Pod", "pods", "web", "api")})
	m = next.(Model)
	if !m.hasSearchTarget {
		t.Fatal("the drill-in should arm a pending selection")
	}
	next, _ = m.Update(menu.ResourceSelectedMsg{Resource: kindResource("services", "Service")})
	if next.(Model).hasSearchTarget {
		t.Fatal("switching resources should drop the pending search selection")
	}
}

// TestSearchDrillInInertWithoutHitResource proves a drill-in with no watcher wired is a
// no-op beyond closing the view (watch-inert), never a crash.
func TestSearchDrillInInertWithoutWatcher(t *testing.T) {
	m := openSearchView(t, &fakeSearcher{})
	next, _ := m.Update(searchview.SelectedMsg{Kind: "search", Hit: searchHit("Pod", "pods", "web", "api")})
	m = next.(Model)
	if m.searchView.Active() {
		t.Fatal("a drill-in should close the search view even when watch-inert")
	}
	if m.hasCurrent {
		t.Fatal("with no watcher wired a drill-in must not claim a browsed resource")
	}
}

// TestSearchEmptyScopeDegrades proves that when nothing in the curated scope is
// available (total discovery failure, or every curated kind denied) the view degrades
// to "no matches" instead of spinning on an indicator that would never clear.
func TestSearchEmptyScopeDegrades(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s)
	// Every group holding a curated kind failed discovery, so each curated seed row is
	// marked unavailable and the search scope comes out empty.
	m.menu.Reconcile(kube.DiscoveryResult{Failed: []kube.FailedGroup{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "networking.k8s.io", Version: "v1"},
	}})
	if len(m.searchResources()) != 0 {
		t.Fatalf("every curated group failed discovery, so the scope should be empty: %v", m.searchResources())
	}
	m, tick := typeQuery(t, m, "a")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	if s.calls != 0 {
		t.Fatalf("an empty curated scope should launch no fan-out, got %d", s.calls)
	}
	if pump != nil {
		t.Fatal("an empty curated scope should issue no pump")
	}
	if m.searchView.Searching() {
		t.Fatal("an empty curated scope should clear the in-flight indicator")
	}
	if view := m.View().Content; !strings.Contains(view, "no matches") {
		t.Fatalf("an empty curated scope should read as no matches: %q", view)
	}
}

// TestSearchCancelsOnQuit proves an in-flight fan-out is torn down when the app exits
// (cancel-on-exit, like the log stream and the forwards).
func TestSearchCancelsOnQuit(t *testing.T) {
	s := &fakeSearcher{keepOpen: true}
	m := openSearchView(t, s)
	m, tick := typeQuery(t, m, "a")
	next, _ := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	// The view is gone (a drill-in) while the fan-out is still live, so the quit comes
	// from the browse view — the search view owns the quit key while it is up.
	m.searchView.Hide()
	if _, cmd := press(t, m, tea.Key{Code: 'q', Text: "q"}); cmd == nil {
		t.Fatal("app.quit should still quit from the browse view")
	}
	if err := s.ctxs[0].Err(); err == nil {
		t.Fatal("quitting should cancel an in-flight cluster search")
	}
}

// TestSearchNavigatesResults proves navigation inside the view moves the result cursor
// and that `enter` drills into the highlighted hit (not the first one).
func TestSearchNavigatesResults(t *testing.T) {
	s := &fakeSearcher{hits: []kube.SearchHit{
		searchHit("Pod", "pods", "web", "api-1"),
		searchHit("Pod", "pods", "web", "api-2"),
	}}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api")
	next, pump := m.Update(tick().(searchDebouncedMsg))
	m = next.(Model)
	for pump != nil && m.searchView.Len() < 2 {
		next, pump = m.Update(pump().(searchMsg))
		m = next.(Model)
	}
	m, _ = press(t, m, tea.Key{Code: tea.KeyDown})
	m, drill := press(t, m, tea.Key{Code: tea.KeyEnter})
	if drill == nil {
		t.Fatal("nav.drillIn on a hit should emit a SelectedMsg command")
	}
	sel, ok := drill().(searchview.SelectedMsg)
	if !ok {
		t.Fatalf("nav.drillIn should emit a SelectedMsg, got %T", drill())
	}
	if sel.Hit.Ref.Name != "api-2" {
		t.Fatalf("nav.drillIn should open the highlighted hit, got %q", sel.Hit.Ref.Name)
	}
}

// TestSearchLabelSelectorGoesToTheServer proves the `-l` half of the query line is
// parsed out and handed to the fan-out as a selector (which kube passes to every List)
// while the name half stays a name — the two are matched in different places, so the
// split has to survive the wiring intact.
func TestSearchLabelSelectorGoesToTheServer(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "api -l app=web")
	if tick == nil {
		t.Fatal("a query with a selector should arm the debounce like any other")
	}
	debounced, ok := tick().(searchDebouncedMsg)
	if !ok {
		t.Fatalf("expected a searchDebouncedMsg, got %T", tick())
	}
	next, _ := m.Update(debounced)
	m = next.(Model)
	want := kube.SearchQuery{Name: "api", LabelSelector: "app=web"}
	if s.gotQuery != want {
		t.Fatalf("the fan-out should search %+v, got %+v", want, s.gotQuery)
	}
	if err := m.searchView.QueryError(); err != "" {
		t.Fatalf("a valid selector should leave no query error, got %q", err)
	}
}

// TestSearchSelectorOnlyQueryIsSearched proves a query that is *only* a selector is a
// real query: the name half being empty is not the empty query that means "search
// nothing", it is "every name with these labels".
func TestSearchSelectorOnlyQueryIsSearched(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "-l app=web")
	if tick == nil {
		t.Fatal("a selector-only query should still arm the debounce")
	}
	debounced, ok := tick().(searchDebouncedMsg)
	if !ok {
		t.Fatalf("expected a searchDebouncedMsg, got %T", tick())
	}
	m.Update(debounced)
	if want := (kube.SearchQuery{LabelSelector: "app=web"}); s.gotQuery != want {
		t.Fatalf("the fan-out should search %+v, got %+v", want, s.gotQuery)
	}
}

// TestSearchInvalidSelectorIsReportedNotSent proves an unparseable selector never
// reaches the cluster: sending it would fail every kind's List, and per-kind failures
// are silent by design (D131 pt 3), so the reader would see a healthy-looking empty
// result instead of their own typo. It is reported on the query line instead, and the
// in-flight indicator comes down rather than spinning on a search that never launched.
func TestSearchInvalidSelectorIsReportedNotSent(t *testing.T) {
	s := &fakeSearcher{}
	m := openSearchView(t, s, WithNamespace("web"))
	m, tick := typeQuery(t, m, "-l app=!!")
	if tick != nil {
		if _, armed := tick().(searchDebouncedMsg); armed {
			t.Fatal("an unparseable selector must not arm a search")
		}
	}
	if s.calls != 0 {
		t.Fatalf("an unparseable selector must not reach the cluster, got %d searches", s.calls)
	}
	if m.searchView.QueryError() == "" {
		t.Fatal("an unparseable selector should be reported on the query line")
	}
	if m.searchView.Searching() {
		t.Fatal("a query that never launched must not leave the in-flight indicator up")
	}
	// Checked by its leading fragment, not the whole string: the message is longer
	// than the test's terminal, and the body wraps it (unlike the header, which
	// clips) so the reader keeps the part that says what to fix.
	if view := m.View().Content; !strings.Contains(view, "invalid label selector") {
		t.Fatalf("the query error should be on screen: %q", view)
	}
}
