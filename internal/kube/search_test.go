package kube

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// res builds a listable Resource for a built-in kind. Namespaced defaults to
// true; pass namespaced=false for cluster-scoped kinds (Node, etc).
func res(group, version, kind, plural string, namespaced bool) Resource {
	gvk := schema.GroupVersionKind{Group: group, Version: version, Kind: kind}
	return Resource{
		GVK:        gvk,
		GVR:        gvk.GroupVersion().WithResource(plural),
		Namespaced: namespaced,
	}
}

// nameQ is a name-substring-only SearchQuery — what every search was before the
// label selector existed, and still the common case.
func nameQ(name string) SearchQuery { return SearchQuery{Name: name} }

// tbl builds a server-printed Table stub whose rows carry only object identity
// (name/namespace) — all searchRows matches on.
func tbl(ns string, names ...string) *Table {
	t := &Table{}
	for _, n := range names {
		t.Rows = append(t.Rows, Row{
			Cells:  []any{n},
			Object: ObjectRef{Namespace: ns, Name: n, UID: n + "-uid"},
		})
	}
	return t
}

// fakeLister is a hermetic rowLister: it returns a canned Table (or error) per
// resource plural and records the namespace each kind was listed with.
type fakeLister struct {
	tables map[string]*Table
	errs   map[string]error

	mu      sync.Mutex
	nsSeen  map[string]string
	selSeen map[string]string
}

func (f *fakeLister) List(_ context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error) {
	f.mu.Lock()
	if f.nsSeen == nil {
		f.nsSeen = map[string]string{}
	}
	if f.selSeen == nil {
		f.selSeen = map[string]string{}
	}
	f.nsSeen[r.GVR.Resource] = namespace
	f.selSeen[r.GVR.Resource] = opts.LabelSelector
	f.mu.Unlock()

	if err := f.errs[r.GVR.Resource]; err != nil {
		return nil, err
	}
	return f.tables[r.GVR.Resource], nil
}

// collect drains a search channel into a name-sorted slice of "kind/name" strings,
// ignoring the progress/terminal events (asserted separately).
func collect(ch <-chan SearchEvent) []string {
	var got []string
	for ev := range ch {
		if ev.Type != SearchMatch {
			continue
		}
		got = append(got, ev.Hit.Resource.GVK.Kind+"/"+ev.Hit.Ref.Name)
	}
	sort.Strings(got)
	return got
}

// drainSearch collects every event of a search, in arrival order.
func drainSearch(ch <-chan SearchEvent) []SearchEvent {
	var got []SearchEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

// kindsDone reports the kinds that emitted a SearchKindDone, and which of them
// were marked Failed.
func kindsDone(events []SearchEvent) (done []string, failed []string) {
	for _, ev := range events {
		if ev.Type != SearchKindDone {
			continue
		}
		done = append(done, ev.Resource.GVK.Kind)
		if ev.Failed {
			failed = append(failed, ev.Resource.GVK.Kind)
		}
	}
	sort.Strings(done)
	sort.Strings(failed)
	return done, failed
}

// terminal returns the single SearchDone event, failing when the stream did not
// end with exactly one.
func terminal(t *testing.T, events []SearchEvent) SearchEvent {
	t.Helper()
	var got []SearchEvent
	for _, ev := range events {
		if ev.Type == SearchDone {
			got = append(got, ev)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one SearchDone event, got %d", len(got))
	}
	if last := events[len(events)-1]; last.Type != SearchDone {
		t.Fatalf("SearchDone must be the last event before close, got %v", last.Type)
	}
	return got[0]
}

func TestSearchMatchesAcrossKindsCaseInsensitive(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods":        tbl("web", "api-server", "API-gateway", "redis"),
		"deployments": tbl("web", "api", "worker"),
	}}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}

	got := collect(searchRows(context.Background(), lister, resources, "web", nameQ("api"), 0))
	want := []string{"Deployment/api", "Pod/API-gateway", "Pod/api-server"}
	if !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestSearchEmptyQueryMatchesAllNamedRows(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "a", "b", ""), // the empty-name row is skipped
	}}
	got := collect(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", nameQ(""), 0))
	if want := []string{"Pod/a", "Pod/b"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantName string
		wantSel  string
		wantErr  bool
	}{
		{name: "bare name", raw: "api", wantName: "api"},
		{name: "name with hyphen l inside a word", raw: "my-lb", wantName: "my-lb"},
		{name: "selector only", raw: "-l app=web", wantSel: "app=web"},
		{name: "name and selector", raw: "api -l app=web", wantName: "api", wantSel: "app=web"},
		{name: "selector with spaces", raw: "-l tier in (a, b)", wantSel: "tier in (a,b)"},
		{name: "selector list", raw: "-l app=web,tier=fe", wantSel: "app=web,tier=fe"},
		{name: "existence selector", raw: "-l !legacy", wantSel: "!legacy"},
		// A dangling -l is not an error and not a selector: the reader is mid-type.
		{name: "dangling token", raw: "api -l", wantName: "api"},
		// -l glued to the selector is not a token, so it stays part of the name
		// rather than being reinterpreted (a name that simply won't match).
		{name: "glued token is not a token", raw: "api -lapp=web", wantName: "api -lapp=web"},
		{name: "invalid selector", raw: "-l app=!!", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSearchQuery(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSearchQuery(%q) = %+v, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSearchQuery(%q): %v", tt.raw, err)
			}
			if got.Name != tt.wantName || got.LabelSelector != tt.wantSel {
				t.Fatalf("ParseSearchQuery(%q) = {Name:%q Selector:%q}, want {Name:%q Selector:%q}",
					tt.raw, got.Name, got.LabelSelector, tt.wantName, tt.wantSel)
			}
			if wantEmpty := tt.wantName == "" && tt.wantSel == ""; got.Empty() != wantEmpty {
				t.Fatalf("ParseSearchQuery(%q).Empty() = %v, want %v", tt.raw, got.Empty(), wantEmpty)
			}
		})
	}
}

func TestSearchSendsLabelSelectorToEveryKind(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods":        tbl("web", "api-server", "redis"),
		"deployments": tbl("web", "api"),
	}}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}

	q := SearchQuery{Name: "api", LabelSelector: "app=web"}
	got := collect(searchRows(context.Background(), lister, resources, "web", q, 0))
	if want := []string{"Deployment/api", "Pod/api-server"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	// The selector is the server's job, so every kind must be listed with it —
	// a kind listed without it would silently return rows the query excluded.
	for _, plural := range []string{"pods", "deployments"} {
		if sel := lister.selSeen[plural]; sel != "app=web" {
			t.Fatalf("%s listed with LabelSelector %q, want %q", plural, sel, "app=web")
		}
	}
}

func TestSearchSelectorOnlyQueryMatchesEveryReturnedRow(t *testing.T) {
	// With no name term the server's selector is the whole filter: every row it
	// returns is a hit, so the client-side matcher must not narrow it further.
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api", "redis")}}
	q := SearchQuery{LabelSelector: "app=web"}
	got := collect(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", q, 0))
	if want := []string{"Pod/api", "Pod/redis"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

func TestSearchIsolatesPerKindFailure(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{"pods": tbl("web", "api-1")},
		errs:   map[string]error{"deployments": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}
	// The failing deployments list must not abort the search — pods still match.
	got := collect(searchRows(context.Background(), lister, resources, "web", nameQ("api"), 0))
	if want := []string{"Pod/api-1"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v (per-kind failure should degrade to nothing)", got, want)
	}
}

func TestSearchCapsTotalHits(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "api-1", "api-2", "api-3", "api-4"),
	}}
	events := drainSearch(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", nameQ("api"), 2))
	n := 0
	for _, ev := range events {
		if ev.Type == SearchMatch {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("emitted %d hits, want cap of 2", n)
	}
	// The cap is *why* this search stopped — the close alone cannot say so, which is
	// the whole reason the stream carries a terminal event (SEARCH-03a).
	if !terminal(t, events).Capped {
		t.Error("a search stopped by the hit cap must report SearchDone{Capped: true}")
	}
}

// TestSearchReportsKindCompletion proves the progress signal: every kind handed to a
// search emits exactly one SearchKindDone — including the one whose List failed, so a
// consumer counting them always reaches N of M and never shows a stuck progress line
// because a kind was denied (principle 3).
func TestSearchReportsKindCompletion(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{"pods": tbl("web", "api-1"), "services": tbl("web", "api-svc")},
		errs:   map[string]error{"deployments": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("", "v1", "Service", "services", true),
		res("apps", "v1", "Deployment", "deployments", true),
	}
	events := drainSearch(searchRows(context.Background(), lister, resources, "web", nameQ("api"), 0))

	done, failed := kindsDone(events)
	if want := []string{"Deployment", "Pod", "Service"}; !equal(done, want) {
		t.Fatalf("kinds done = %v, want one per searched kind %v", done, want)
	}
	if want := []string{"Deployment"}; !equal(failed, want) {
		t.Errorf("failed kinds = %v, want %v", failed, want)
	}
	if terminal(t, events).Capped {
		t.Error("an exhaustive search must not report Capped")
	}
}

// TestSearchDoneNotCappedWhenHitsExactlyFitLimit pins the Capped semantics: it means
// "there were more matches than you were given", not "you were given limit matches" —
// so a search whose matches exactly fill the cap is still exhaustive.
func TestSearchDoneNotCappedWhenHitsExactlyFitLimit(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-1", "api-2")}}
	events := drainSearch(searchRows(context.Background(), lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", nameQ("api"), 2))
	if terminal(t, events).Capped {
		t.Error("matches exactly filling the cap truncated nothing, so Capped must be false")
	}
}

func TestSearchClusterScopedIgnoresNamespace(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"nodes": tbl("", "node-a", "node-b"),
	}}
	got := collect(searchRows(context.Background(), lister, []Resource{res("", "v1", "Node", "nodes", false)}, "web", nameQ("node"), 0))
	if want := []string{"Node/node-a", "Node/node-b"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	if ns := lister.nsSeen["nodes"]; ns != "" {
		t.Errorf("cluster-scoped Node listed with namespace %q, want empty", ns)
	}
}

func TestSearchCancellationClosesChannelWithoutTerminalEvent(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-1")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before draining
	// The channel must still close (not hang) even when ctx is already done, and an
	// abandoned search reports nothing: a consumer that cancelled is gone, so no
	// SearchDone is worth blocking on.
	for _, ev := range drainSearch(searchRows(ctx, lister, []Resource{res("", "v1", "Pod", "pods", true)}, "web", nameQ("api"), 0)) {
		if ev.Type == SearchDone {
			t.Error("a cancelled search must not emit a terminal SearchDone")
		}
	}
}

// gateLister is a rowLister that parks every List until the test lets it go: it
// announces each arrival on arrived and then blocks on proceed. That makes the
// fan-out's *in-flight* set observable, which a lister that returns immediately
// cannot be — the whole point of the bound is how many lists exist at one moment.
type gateLister struct {
	arrived chan string
	proceed chan struct{}
}

func (g *gateLister) List(ctx context.Context, r Resource, namespace string, _ metav1.ListOptions) (*Table, error) {
	g.arrived <- r.GVR.Resource
	select {
	case <-g.proceed:
		return tbl(namespace, "api-"+r.GVR.Resource), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// manyResources builds n distinct listable kinds, enough to exceed the fan-out bound.
func manyResources(n int) []Resource {
	out := make([]Resource, 0, n)
	for i := range n {
		name := "kind" + itoa(i)
		out = append(out, res("example.com", "v1", name, name+"s", true))
	}
	return out
}

// itoa is a test-local non-negative int→string (the package has no strconv import).
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

// TestSearchBoundsConcurrentLists is the load half of D131 pt 2: the opt-in widen
// hands Search every discovered kind, and an unbounded fan-out would put all of them
// on the wire at once. At most searchConcurrency lists may be in flight; the rest wait
// their turn and the sweep still completes over every kind.
func TestSearchBoundsConcurrentLists(t *testing.T) {
	const kinds = searchConcurrency * 3
	g := &gateLister{arrived: make(chan string, kinds), proceed: make(chan struct{})}
	resources := manyResources(kinds)

	ch := searchRows(context.Background(), g, resources, "web", nameQ("api"), 0)

	// The bound's worth of lists must arrive; nothing beyond it may, while they are
	// all still parked. The negative half needs a real window — an unbounded fan-out
	// would show up as extra arrivals only once those goroutines are scheduled.
	for range searchConcurrency {
		<-g.arrived
	}
	select {
	case extra := <-g.arrived:
		t.Fatalf("list for %q started while %d were already in flight: fan-out is unbounded", extra, searchConcurrency)
	case <-time.After(100 * time.Millisecond):
	}

	// Releasing them lets the queued kinds through; every kind must still be listed
	// and reported exactly once, so the bound delays work rather than dropping it.
	close(g.proceed)
	events := drainSearch(ch)
	done, failed := kindsDone(events)
	if len(done) != kinds {
		t.Fatalf("kinds done = %d, want one per requested kind (%d)", len(done), kinds)
	}
	if len(failed) != 0 {
		t.Errorf("queued-then-released kinds must not be marked failed, got %v", failed)
	}
	if got := len(hitsOf(events)); got != kinds {
		t.Errorf("hits = %d, want one per kind (%d) — a queued kind must still be searched", got, kinds)
	}
}

// TestSearchCappedStillReportsQueuedKinds pins what the bound must not break: the cap
// cancels the sweep while most kinds are still queued for a slot, and each of those has
// to unwind reporting done-but-not-failed. Otherwise the view's "searching N/M kinds…"
// line would stop short of M on every capped wide search, reading as a hang (D142 pt 2).
func TestSearchCappedStillReportsQueuedKinds(t *testing.T) {
	const kinds = searchConcurrency * 3
	g := &gateLister{arrived: make(chan string, kinds), proceed: make(chan struct{})}

	ch := searchRows(context.Background(), g, manyResources(kinds), "web", nameQ("api"), 1)
	for range searchConcurrency {
		<-g.arrived
	}
	close(g.proceed) // the first hit through trips the cap and cancels the rest

	events := drainSearch(ch)
	done, failed := kindsDone(events)
	if len(done) != kinds {
		t.Fatalf("kinds done = %d, want one per requested kind (%d) even when the cap cut the sweep short", len(done), kinds)
	}
	if len(failed) != 0 {
		t.Errorf("a kind cancelled by the cap is not a failed kind, got %v", failed)
	}
	if !terminal(t, events).Capped {
		t.Error("a search stopped by the cap must report Capped")
	}
}

// hitsOf extracts the match events from a drained stream.
func hitsOf(events []SearchEvent) []SearchHit {
	var out []SearchHit
	for _, ev := range events {
		if ev.Type == SearchMatch {
			out = append(out, ev.Hit)
		}
	}
	return out
}

func TestCommonSearchResourcesFiltersToCuratedSet(t *testing.T) {
	all := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("apps", "v1", "Deployment", "deployments", true),
		res("", "v1", "Event", "events", true),                       // not curated
		res("example.com", "v1", "Widget", "widgets", true),          // CRD, not curated
		res("networking.k8s.io", "v1", "Ingress", "ingresses", true), // curated
	}
	got := CommonSearchResources(all)
	var kinds []string
	for _, r := range got {
		kinds = append(kinds, r.GVK.Kind)
	}
	sort.Strings(kinds)
	if want := []string{"Deployment", "Ingress", "Pod"}; !equal(kinds, want) {
		t.Fatalf("curated kinds = %v, want %v", kinds, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- name matching / scoring (SEARCH-04c-2a, SEARCH-04c-2b) ---

func TestNameMatcherMatchKinds(t *testing.T) {
	m := NewNameMatcher("API")
	for _, tc := range []struct {
		name          string
		want          bool
		wantScattered bool
	}{
		{name: "api-0", want: true},                      // case-insensitive, as before
		{name: "my-api-server", want: true},              // mid-name substring
		{name: "API", want: true},                        //
		{name: "a-p-i", want: true, wantScattered: true}, // scattered: SEARCH-04c-2b
		{name: "alpha-pod-images", want: true, wantScattered: true},
		{name: "nginx", want: false}, // no `a`, `p`, `i` in order
		{name: "ipa", want: false},   // right letters, wrong order
		{name: "ap", want: false},    // needle not exhausted
		{name: "", want: false},      // an unnamed object is not a result
	} {
		_, scattered, ok := m.Match(tc.name)
		if ok != tc.want {
			t.Errorf("match(%q) ok = %v, want %v", tc.name, ok, tc.want)
			continue
		}
		if ok && scattered != tc.wantScattered {
			t.Errorf("match(%q) scattered = %v, want %v", tc.name, scattered, tc.wantScattered)
		}
	}
}

// mscore is the score of a name that is expected to match — for the tests that
// only compare rankings and do not care how the match was found.
func mscore(t *testing.T, m NameMatcher, name string) int {
	t.Helper()
	s, _, ok := m.Match(name)
	if !ok {
		t.Fatalf("expected %q to match", name)
	}
	return s
}

func TestNameMatcherEmptyNeedleMatchesEverythingUnranked(t *testing.T) {
	m := NewNameMatcher("")
	for _, n := range []string{"api-0", "nginx", "zzz"} {
		score, scattered, ok := m.Match(n)
		if !ok {
			t.Fatalf("empty needle should match %q", n)
		}
		if score != 0 {
			t.Fatalf("empty needle score for %q = %d, want 0 (label-only query ranks nothing)", n, score)
		}
		// A label-only hit scores below the substring band but is not fuzzy, so
		// it must not be charged to the scattered budget.
		if scattered {
			t.Fatalf("empty-needle hit for %q reported scattered", n)
		}
	}
	// The one thing the empty needle must still reject.
	if _, _, ok := m.Match(""); ok {
		t.Error("an unnamed object must not match even the empty needle")
	}
}

// The ordering this leg exists for: a prefix beats a separator-boundary match,
// which beats a match buried inside a word.
func TestNameMatcherRanksByMatchPosition(t *testing.T) {
	m := NewNameMatcher("api")
	names := []string{"legacyapi", "my-api", "api-server"}
	scores := make([]int, len(names))
	for i, n := range names {
		scores[i] = mscore(t, m, n)
	}
	if scores[2] <= scores[1] || scores[1] <= scores[0] {
		t.Fatalf("want api-server > my-api > legacyapi, got %v for %v", scores, names)
	}
}

// Two names that match in the same position are separated by how much name there
// is around the match — the tighter one wins.
func TestNameMatcherPrefersTighterName(t *testing.T) {
	m := NewNameMatcher("api")
	short := mscore(t, m, "api-0")
	long := mscore(t, m, "api-0-abcdefghijklmnop")
	if short <= long {
		t.Fatalf("api-0 (%d) should outrank api-0-abcdefghijklmnop (%d)", short, long)
	}
}

// The best occurrence wins, not the first: a name that matches badly early and
// well later scores as the good match.
func TestNameMatcherTakesBestOccurrence(t *testing.T) {
	m := NewNameMatcher("api")
	best := mscore(t, m, "xapiy-api-0")
	buried := mscore(t, m, "xapiy-zzz-0")
	if best <= buried {
		t.Fatalf("best occurrence = %d, want > the buried-only score %d", best, buried)
	}
}

// The bands are the invariant the whole design rests on: the *worst* possible
// substring score must still beat the *best* possible scattered one, so no fuzzy
// near-miss can ever be ranked into the middle of the exact matches.
func TestScoreBandsDoNotOverlap(t *testing.T) {
	m := NewNameMatcher("api")
	worstSubstring := mscore(t, m, "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzapizzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
	// The best a scattered match can do: at the very start of the shortest
	// possible name that is not a substring match.
	bestScattered := mscore(t, m, "apxi")

	if floor := scoreSubstringBand - maxStartPenalty - maxLenPenalty; worstSubstring < floor {
		t.Fatalf("worst substring score %d fell below the band floor %d", worstSubstring, floor)
	}
	if bestScattered > scoreScatteredBand {
		t.Fatalf("best scattered score %d rose above the band ceiling %d", bestScattered, scoreScatteredBand)
	}
	if worstSubstring <= bestScattered {
		t.Fatalf("bands overlap: worst substring %d <= best scattered %d", worstSubstring, bestScattered)
	}
}

// The scattered band reads names by tightness, not position: `apisrv` finding
// `api-server` is the match, the same six characters strewn across a long name is
// the noise.
func TestScatteredMatchesRankByTightness(t *testing.T) {
	m := NewNameMatcher("apisrv")
	tight := mscore(t, m, "api-srv")
	loose := mscore(t, m, "api-server")
	spread := mscore(t, m, "a-pod-in-some-random-vault")
	if tight <= loose || loose <= spread {
		t.Fatalf("want api-srv > api-server > a-pod-in-some-random-vault, got %d, %d, %d", tight, loose, spread)
	}

	// Position still separates two equally tight matches — the earlier one wins.
	early := mscore(t, m, "apixsrv-0")
	late := mscore(t, m, "zzz-apixsrv")
	if early <= late {
		t.Fatalf("apixsrv-0 (%d) should outrank zzz-apixsrv (%d)", early, late)
	}
}

// The backtrack, isolated. Forward-greedy alone would read the first name as a
// match spread over its whole length; walking back from where the forward pass
// ended finds the tight window at the end instead, which is what puts it above a
// name of the same shape that genuinely has no tight window anywhere. Drop the
// backtrack and this ordering inverts.
func TestScatteredMatchPrefersTheTightestWindow(t *testing.T) {
	m := NewNameMatcher("abc")
	filler := strings.Repeat("z", 20)
	// Forward-greedy takes a(0), b(21), c(last); the backtrack finds the trailing
	// `ab.c` — no contiguous `abc` anywhere, so this stays in the scattered band.
	tightened := mscore(t, m, "a"+filler+"b"+filler+"ab.c")
	// The same characters and roughly the same length, but nothing to tighten onto.
	spread := mscore(t, m, "a"+filler+"z"+"b"+filler+"z"+"c")
	if tightened <= spread {
		t.Fatalf("tightened window (%d) should outrank the genuinely spread one (%d)", tightened, spread)
	}
}

// --- matched spans (SEARCH-06) ---

// The spans are what a consumer paints, so this pins the shape of every case it
// can be handed: one contiguous run, several runs for a subsequence, and nothing
// at all — which is a normal answer, not an error.
func TestMatchSpans(t *testing.T) {
	for _, tc := range []struct {
		needle, name string
		want         []MatchSpan
	}{
		// The plain case, and the one the table could have re-derived itself.
		{needle: "api", name: "api-0", want: []MatchSpan{{Start: 0, End: 3}}},
		// Case-insensitive, and the offsets are into the *original* name.
		{needle: "api", name: "MY-API-0", want: []MatchSpan{{Start: 3, End: 6}}},
		// Scattered: the runs a consumer could not have found by searching for the
		// query, which is the whole reason the hit carries them (D153).
		{needle: "apisrv", name: "api-server", want: []MatchSpan{{Start: 0, End: 3}, {Start: 4, End: 5}, {Start: 6, End: 8}}},
		// A single-rune scattered match still coalesces into whole runs.
		{needle: "wbp", name: "web-pod", want: []MatchSpan{{Start: 0, End: 1}, {Start: 2, End: 3}, {Start: 4, End: 5}}},
		// The tightened window, not the forward-greedy one: the score is charged
		// against the trailing `ab.c` (see TestScatteredMatchPrefersTheTightestWindow),
		// so that is what the marks must point at — a(0) and b(21) are where a
		// forward-only walk would have put them.
		{
			needle: "abc",
			name:   "a" + strings.Repeat("z", 20) + "b" + strings.Repeat("z", 20) + "ab.c",
			want:   []MatchSpan{{Start: 42, End: 44}, {Start: 45, End: 46}},
		},
		// Nothing to mark: no match, no name, and no needle (a label-only query).
		{needle: "api", name: "nginx", want: nil},
		{needle: "api", name: "", want: nil},
		{needle: "", name: "api-0", want: nil},
	} {
		got := NewNameMatcher(tc.needle).MatchSpans(tc.name)
		if !equalSpans(got, tc.want) {
			t.Errorf("MatchSpans(%q, %q) = %v; want %v", tc.needle, tc.name, got, tc.want)
		}
	}
}

// The marks explain the ranking, so they must point at the occurrence the score
// was read from — not merely at *an* occurrence. `api` in `xapiy-api-0` scores on
// the second one (it follows a separator); marking the first would tell the reader
// the row ranked where it did for a reason that is not the reason.
func TestMatchSpansMarkTheScoredOccurrence(t *testing.T) {
	m := NewNameMatcher("api")
	const name = "xapiy-api-0"
	spans := m.MatchSpans(name)
	want := []MatchSpan{{Start: 6, End: 9}}
	if !equalSpans(spans, want) {
		t.Fatalf("MatchSpans(%q) = %v; want %v (the occurrence after the separator)", name, spans, want)
	}
	// And the claim that makes it load-bearing: that occurrence is the one that
	// scores, so the two would have to move together.
	if got, buried := mscore(t, m, name), mscore(t, m, "xapiy-zzz-0"); got <= buried {
		t.Fatalf("the separator occurrence should be the scoring one: %d vs %d", got, buried)
	}
}

// Whatever the matcher accepts it can explain: every match has at least one span,
// every span lies inside the name, and the spans are sorted and disjoint — the
// three properties a painter relies on and none of which it re-checks.
func TestMatchSpansAgreeWithMatch(t *testing.T) {
	names := []string{"api-0", "my-api-server", "API", "a-p-i", "alpha-pod-images", "nginx", "ipa", "ap", "", "xapiy-api-0"}
	for _, needle := range []string{"api", "apisrv", "a", ""} {
		m := NewNameMatcher(needle)
		for _, name := range names {
			_, _, ok := m.Match(name)
			spans := m.MatchSpans(name)
			if !ok || needle == "" {
				if spans != nil {
					t.Errorf("MatchSpans(%q, %q) = %v; want nil for a non-match / empty needle", needle, name, spans)
				}
				continue
			}
			if len(spans) == 0 {
				t.Errorf("MatchSpans(%q, %q) is empty though Match reported a hit", needle, name)
				continue
			}
			last := 0
			for i, sp := range spans {
				if sp.Start < 0 || sp.End > utf8.RuneCountInString(name) || sp.Start >= sp.End {
					t.Errorf("MatchSpans(%q, %q)[%d] = %v is out of range for a %d-rune name",
						needle, name, i, sp, utf8.RuneCountInString(name))
				}
				if i > 0 && sp.Start <= last {
					t.Errorf("MatchSpans(%q, %q) is not sorted and disjoint: %v", needle, name, spans)
				}
				last = sp.End
			}
		}
	}
}

func equalSpans(a, b []MatchSpan) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The spans reach the consumer on the hit itself, and only for the half of a query
// that matches a name: a label-selector hit carries none, because the selector
// matched something the name never showed.
func TestSearchCarriesMatchSpans(t *testing.T) {
	pods := res("", "v1", "Pod", "pods", true)
	f := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-0", "a-p-i")}}

	spans := map[string][]MatchSpan{}
	for ev := range searchRows(context.Background(), f, []Resource{pods}, "web", nameQ("api"), 0) {
		if ev.Type == SearchMatch {
			spans[ev.Hit.Ref.Name] = ev.Hit.Match
		}
	}
	if want := []MatchSpan{{Start: 0, End: 3}}; !equalSpans(spans["api-0"], want) {
		t.Errorf("api-0 spans = %v; want %v", spans["api-0"], want)
	}
	if want := ([]MatchSpan{{Start: 0, End: 1}, {Start: 2, End: 3}, {Start: 4, End: 5}}); !equalSpans(spans["a-p-i"], want) {
		t.Errorf("a-p-i spans = %v; want %v", spans["a-p-i"], want)
	}

	f2 := &fakeLister{tables: map[string]*Table{"pods": tbl("web", "api-0")}}
	q := SearchQuery{LabelSelector: "app=web"}
	for ev := range searchRows(context.Background(), f2, []Resource{pods}, "web", q, 0) {
		if ev.Type == SearchMatch && ev.Hit.Match != nil {
			t.Errorf("a label-selector hit carries spans %v; want none", ev.Hit.Match)
		}
	}
}

// Search puts the score on the wire: the hit for the better-matching name carries
// the higher Score, without the stream being reordered.
func TestSearchScoresHits(t *testing.T) {
	pods := res("", "v1", "Pod", "pods", true)
	f := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "zzz-api-legacy", "api-0"),
	}}

	scores := map[string]int{}
	for ev := range searchRows(context.Background(), f, []Resource{pods}, "web", nameQ("api"), 0) {
		if ev.Type == SearchMatch {
			scores[ev.Hit.Ref.Name] = ev.Hit.Score
		}
	}
	if len(scores) != 2 {
		t.Fatalf("scores = %v, want both pods", scores)
	}
	if scores["api-0"] <= scores["zzz-api-legacy"] {
		t.Fatalf("api-0 (%d) should outrank zzz-api-legacy (%d)", scores["api-0"], scores["zzz-api-legacy"])
	}
}

// The widen this leg exists for: a name the substring matcher would have missed is
// now a hit, and it arrives ranked below the ones that were already there.
func TestSearchMatchesScatteredNames(t *testing.T) {
	pods := res("", "v1", "Pod", "pods", true)
	f := &fakeLister{tables: map[string]*Table{
		"pods": tbl("web", "api-server", "apisrv-0", "redis"),
	}}

	scores := map[string]int{}
	for ev := range searchRows(context.Background(), f, []Resource{pods}, "web", nameQ("apisrv"), 0) {
		if ev.Type == SearchMatch {
			scores[ev.Hit.Ref.Name] = ev.Hit.Score
		}
	}
	if len(scores) != 2 {
		t.Fatalf("scores = %v, want the substring hit and the scattered one (and not redis)", scores)
	}
	if scores["apisrv-0"] <= scores["api-server"] {
		t.Fatalf("the substring hit apisrv-0 (%d) must outrank the scattered api-server (%d)",
			scores["apisrv-0"], scores["api-server"])
	}
}

func TestScatteredLimitTracksTheCap(t *testing.T) {
	for _, tc := range []struct{ limit, want int }{
		{limit: 0, want: 0},    // uncapped search: no budget either
		{limit: -1, want: 0},   //
		{limit: 200, want: 50}, // the app's cap
		{limit: 4, want: 1},    //
		{limit: 2, want: 1},    // too small to divide, but never zero
	} {
		if got := scatteredLimit(tc.limit); got != tc.want {
			t.Errorf("scatteredLimit(%d) = %d, want %d", tc.limit, got, tc.want)
		}
	}
}

// The cap runs at emit time, in arrival order, before any consumer has ranked
// anything — so scattered hits get a fraction of it and no more. Without the
// budget the ten junk names below would spend the whole cap and the five exact
// matches behind them would never be emitted at all.
//
// One kind, so the row order is the arrival order and the assertion is exact.
func TestSearchBudgetsScatteredHitsWithinTheCap(t *testing.T) {
	pods := res("", "v1", "Pod", "pods", true)
	names := []string{}
	for i := 0; i < 10; i++ {
		names = append(names, "a-p-i-s-r-v-junk-"+string(rune('a'+i))) // scattered only
	}
	for i := 0; i < 5; i++ {
		names = append(names, "apisrv-"+string(rune('a'+i))) // contiguous
	}

	const limit = 8 // → a scattered budget of 2
	events := drainSearch(searchRows(context.Background(), &fakeLister{
		tables: map[string]*Table{"pods": tbl("web", names...)},
	}, []Resource{pods}, "web", nameQ("apisrv"), limit))

	var scattered, contiguous int
	for _, ev := range events {
		if ev.Type != SearchMatch {
			continue
		}
		if ev.Hit.Score >= scoreSubstringBand-maxStartPenalty-maxLenPenalty {
			contiguous++
		} else {
			scattered++
		}
	}
	if want := scatteredLimit(limit); scattered != want {
		t.Errorf("emitted %d scattered hits, want the budget %d", scattered, want)
	}
	// The point of the budget: every exact match still got through, even though
	// the junk arrived first and there were more junk rows than the whole cap.
	if contiguous != 5 {
		t.Errorf("emitted %d contiguous hits, want all 5 — fuzzy must never starve exact", contiguous)
	}
	// Dropping scattered hits is not truncation worth telling the reader about:
	// they are the matches this search itself rates worst.
	if terminal(t, events).Capped {
		t.Error("a search that only dropped over-budget scattered hits must not report Capped")
	}
}

// A hit carries the printed row it was matched in — the columns of its kind and
// its own cells — so a surface can preview the object without listing the kind
// again (STORY-06k-2). The search already read that row to match its name.
func TestSearchCarriesThePrintedRow(t *testing.T) {
	pods := res("", "v1", "Pod", "pods", true)
	table := &Table{
		Columns: []Column{{Name: "Name"}, {Name: "Status"}, {Name: "Age"}},
		Rows: []Row{
			{Cells: []any{"api-0", "Running", "3d"}, Object: ObjectRef{Namespace: "web", Name: "api-0"}},
			{Cells: []any{"api-1", "CrashLoopBackOff", "5m"}, Object: ObjectRef{Namespace: "web", Name: "api-1"}},
		},
	}
	f := &fakeLister{tables: map[string]*Table{"pods": table}}

	cells := map[string][]any{}
	cols := map[string][]Column{}
	for ev := range searchRows(context.Background(), f, []Resource{pods}, "web", nameQ("api"), 0) {
		if ev.Type == SearchMatch {
			cells[ev.Hit.Ref.Name] = ev.Hit.Cells
			cols[ev.Hit.Ref.Name] = ev.Hit.Columns
		}
	}
	if got := cells["api-1"]; len(got) != 3 || got[1] != "CrashLoopBackOff" {
		t.Errorf("api-1 cells = %v; want the printed row", got)
	}
	if got := cols["api-0"]; len(got) != 3 || got[1].Name != "Status" {
		t.Errorf("api-0 columns = %v; want the kind's column set", got)
	}
	if got := cells["api-0"]; len(got) != 3 || got[1] != "Running" {
		t.Errorf("api-0 cells = %v; want its own row, not another hit's", got)
	}
}
