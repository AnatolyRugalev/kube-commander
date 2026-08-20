package kube

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SearchHit is one object matched by a cluster search. Resource is the kind the
// object belongs to — enough for a caller to switch the browse view to that kind
// and select the row — and Ref is the object's identity (Ref.Namespace is empty
// for cluster-scoped kinds).
type SearchHit struct {
	Resource Resource
	Ref      ObjectRef

	// Columns and Cells are the object's server-printed table row — exactly what
	// the browse table would show for it — carried so a surface can *preview* a
	// hit (STORY-06k-2) without listing the kind again. The search already
	// listed that row to match its name, so carrying it is free; re-fetching it
	// per highlighted row would put a cluster round-trip on a cursor movement.
	//
	// Columns is the kind's whole column set, shared by every hit of that kind
	// (never mutated), and Cells is the row's own values in the same order —
	// short rows are normal, so a consumer indexes defensively. Both are nil
	// when the lister returned no columns; a consumer renders the hit's identity
	// alone rather than treating that as an error.
	//
	// This supersedes the "a search hit carries no printed cells because
	// drilling re-lists" half of D276 pt 2: drilling in still re-lists and
	// watches the real table — the cells here are for the preview that decides
	// *whether* to drill in.
	Columns []Column
	Cells   []any

	// Score ranks this hit against the others of the same search: higher is a
	// better match for SearchQuery.Name. It is a relative number with no meaning
	// on its own and no stable scale between releases — only the comparison
	// between two hits of one search is defined. Every hit of a query with no
	// Name half (a pure label-selector search) scores 0, so a consumer sorting
	// on it keeps arrival order there.
	//
	// Scores fall in two disjoint bands: every contiguous (substring) match
	// outranks every scattered (subsequence) one, whatever the positions inside
	// them (D153). A consumer therefore never has to know which kind of match a
	// hit was — sorting on the score alone already keeps the fuzzy matches below
	// the exact ones.
	//
	// The hits are NOT emitted in score order — the fan-out streams them as the
	// kinds return (see Search) and ranking is the consumer's job, deliberately
	// (D152).
	Score int

	// Match is which runes of Ref.Name the query matched, so a consumer can mark
	// them (SEARCH-06). The spans are in **Ref.Name's own** rune coordinate space
	// — offset 0 is the first rune of the name, not of whatever row the consumer
	// builds around it — sorted, disjoint and non-empty, half-open [Start, End).
	//
	// It is carried on the hit rather than re-derived at paint time because only
	// the matcher that scored the hit knows what to mark: a match may be a
	// *subsequence* (D153 — `wbp` matching `web-pod` marks three separate runes),
	// and a hit found by the label-selector half of the query matches nothing the
	// name shows at all. A consumer re-running a substring search over the query
	// would silently mark nothing in the first case and the wrong thing in the
	// second.
	//
	// Nil is normal and means "nothing to mark": a pure label-selector query, an
	// empty name half, or any hit the score already reports as unranked (Score 0
	// with no Name term). A consumer must render such a hit plainly rather than
	// treating nil as an error.
	Match []MatchSpan
}

// MatchSpan is one run of matched runes inside a name, half-open: [Start, End).
// Rune offsets rather than byte offsets because every consumer of one is painting
// display columns, where runes are the unit (kubecom's own convention — see the
// table's roleSpan and the logs view's grep spans).
type MatchSpan struct {
	Start, End int
}

// SearchQuery is what one cluster search matches on. The two terms are ANDed and
// each is optional, but a query with neither matches nothing worth streaming —
// callers should treat Empty as "do not search" rather than "match everything",
// which over a whole cluster is the enumeration D8/principle 4 avoids.
//
// The two halves are evaluated in different places on purpose, and that is the
// point of separating them: LabelSelector is handed to the apiserver in the List
// call, so a selector costs the client nothing and narrows the traffic on the wire,
// while Name is a client-side substring over the rows that come back (the server
// has no "name contains" filter — a field selector can only match a name exactly).
type SearchQuery struct {
	// Name is matched case-insensitively against each object's name: as a
	// contiguous substring first and, failing that, as a subsequence — its
	// characters in order but not adjacent, so `apisrv` finds `api-server`.
	// Empty matches every name.
	Name string

	// LabelSelector is a Kubernetes label selector in its standard string form
	// (`app=web`, `tier in (a,b)`, `!legacy`, comma-separated), passed straight
	// through to every List. Empty selects everything. Validate it with
	// ParseSearchQuery rather than building it by hand: an invalid selector is
	// rejected by the server per kind, which under per-kind failure isolation
	// (principle 3, D131 pt 3) would look exactly like an empty cluster.
	LabelSelector string
}

// Empty reports a query with nothing to match on.
func (q SearchQuery) Empty() bool { return q.Name == "" && q.LabelSelector == "" }

// searchSelectorToken introduces the label-selector half of a raw query. It is
// kubectl's own flag, so the syntax a reader already knows (`-l app=web`) is the
// syntax that works here.
const searchSelectorToken = "-l"

// ParseSearchQuery splits one raw query line into a SearchQuery. Everything before
// a whitespace-delimited `-l` is the name substring; everything after it is a label
// selector, parsed (and normalised) with the standard apimachinery parser, so an
// unusable selector is reported here — to the reader who typed it — instead of
// being sent to the server and coming back as a silent, empty search.
//
// The remainder after `-l` is taken whole rather than tokenised so a selector may
// contain spaces (`tier in (a, b)`), which is also why the name half is the *prefix*
// and not "every non-selector word": an object name can never contain a space, so
// there is nothing to gain from letting the name half be several terms and a real
// grammar to lose.
//
// A raw query with no `-l` is a pure name substring, exactly as before this existed.
func ParseSearchQuery(raw string) (SearchQuery, error) {
	name, selector := splitSelector(raw)
	q := SearchQuery{Name: strings.TrimSpace(name)}
	if selector == "" {
		return q, nil
	}
	sel, err := labels.Parse(selector)
	if err != nil {
		// Wrapped here rather than at the call site: apimachinery's message says
		// what is wrong with the requirement ("found '!', expected: identifier")
		// but never what a requirement is, and the reader typed this into a box
		// that mostly takes names.
		return SearchQuery{}, fmt.Errorf("invalid label selector: %w", err)
	}
	q.LabelSelector = sel.String()
	return q, nil
}

// splitSelector finds the whitespace-delimited `-l` token and returns the text
// before it and the (trimmed) remainder after it. A `-l` embedded in a word — the
// `-l` of `my-lb`, or a `-lapp=web` typed without the space — is not a token, so a
// name is never silently cut in half by its own hyphen.
func splitSelector(raw string) (name, selector string) {
	rest := raw
	offset := 0
	for {
		i := strings.Index(rest, searchSelectorToken)
		if i < 0 {
			return raw, ""
		}
		start, end := offset+i, offset+i+len(searchSelectorToken)
		beforeOK := start == 0 || isSpace(raw[start-1])
		afterOK := end == len(raw) || isSpace(raw[end])
		if beforeOK && afterOK {
			return raw[:start], strings.TrimSpace(raw[end:])
		}
		rest = rest[i+len(searchSelectorToken):]
		offset = end
	}
}

// isSpace reports the ASCII whitespace splitSelector treats as a token boundary.
func isSpace(b byte) bool { return b == ' ' || b == '\t' }

// Match scores. They exist to order hits, not to measure anything, so only their
// relative sizes matter — and the sizes encode one claim: *where* a query matches
// inside a name is what separates a good hit from a poor one. A generated pod name
// carries the workload's name at the front and entropy at the back, so `api`
// matching at the head of `api-7f9c-x2` is the object the reader meant, while the
// same three letters landing mid-suffix are a coincidence.
//
// The two bands are floors reserved for a *kind* of match, and the gap between
// them is the load-bearing part: every contiguous match scores at least
// scoreSubstringBand-maxStartPenalty-maxLenPenalty (900), every scattered one at
// most scoreScatteredBand (400), so no subsequence match can ever outrank a
// substring one however well positioned it is (D152/D153). That is what lets the
// fuzzy fallback add matches without ever pushing noise into the middle of a good
// list. The penalties are clamped for the same reason: an unbounded length penalty
// on a long name would eventually eat a whole band.
//
// The two bands weigh their terms differently, and that is deliberate rather than
// an oversight (D153). In a contiguous match the span is fixed — it is the needle
// — so *position* is the only thing left to read: a generated pod name carries the
// workload's name at the front and entropy at the back, so `api` at the head of
// `api-7f9c-x2` is the object the reader meant while the same three letters in the
// suffix are a coincidence. In a scattered match the span varies, and *tightness*
// is the signal instead: `apisrv` finding `api-server` is a real match, while the
// same six characters strewn across `a-pod-in-some-random-vault` is the noise the
// band exists to sink. So the position bonuses apply to contiguous matches only,
// and a scattered match is scored on how much filler it had to jump over —
// weighted above its start offset, so pulling a match into a tight window at the
// end of a name always beats reading it as one that merely begins early.
const (
	scoreSubstringBand = 1000 // contiguous match: the band every substring hit sits in
	scoreScatteredBand = 400  // scattered match: the ceiling every subsequence hit sits under
	scoreAtStart       = 400  // contiguous: the name begins with the match
	scoreAtBoundary    = 200  // contiguous: the match begins right after a `-`/`.`/`_` separator
	maxStartPenalty    = 50   // clamp on "how far into the name the match begins"
	maxLenPenalty      = 50   // clamp on "how much name there is around the match"
	maxGapPenalty      = 100  // clamp on "how much filler a scattered match jumped over"
	gapPenaltyWeight   = 2    // scattered: gaps outweigh the start offset, so tightness wins
)

// NameMatcher matches and scores a name against one query string — the Name half
// of a SearchQuery for the cluster search, and the typed query for every modal
// picker (PAL-01). It is built once per search and used from every kind's
// goroutine — it holds only the prepared needle and never mutates, so sharing it
// is safe.
//
// It is a type rather than a function because the needle wants preparing exactly
// once (lower-casing it per row, over every object in a wide cluster sweep, is the
// kind of waste a search feels), and because the two matchers below want one
// prepared needle between them.
//
// It is exported so there is exactly **one** fuzzy matcher in kubecom (D194 pt 1):
// a picker that grew its own would rank the same characters differently from the
// search view a keypress away, and the band-gap invariant below (D152 pt 3/D153)
// would then hold in one surface and not the other. Callers outside the cluster
// search use it purely as a ranker: the needle is what the reader typed, the name
// is whatever the row displays.
type NameMatcher struct {
	needle string // already lower-cased; "" matches everything
}

// NewNameMatcher prepares a matcher for the (raw, any-case) query name.
func NewNameMatcher(name string) NameMatcher {
	return NameMatcher{needle: strings.ToLower(name)}
}

// match reports whether name matches the query, how well, and whether the match
// was scattered. An empty needle matches every named object with score 0 (a
// label-only query ranks nothing) and is never scattered; an empty name never
// matches at all, not even the empty needle, because an object without a name is
// not a result.
//
// Matching is case-insensitive and tried in two passes: a contiguous substring
// first and, only if that fails, a subsequence — the needle's characters in order
// but not adjacent, so `apisrv` finds `api-server` and `kdns` finds `kube-dns`.
// The passes are ordered, not merged: a name that contains the needle outright is
// scored on that occurrence, so widening the matcher can only add results below
// the ones that were already there, never re-rank them.
//
// Of the possible occurrences the best-scoring one wins, and ties go to the
// earliest, so the score describes the reading of the name a human would give it.
//
// The scattered flag is not derivable from the score by a caller: an empty-needle
// hit also scores below the substring band, so "low score" and "fuzzy" are not the
// same thing. It is returned separately because searchRows budgets scattered hits
// (see searchScatteredShare) — nothing that merely orders hits needs it.
//
// Matching is byte-wise over the lower-cased strings. Object names are RFC 1123
// in practice, so this is the same as character-wise; a needle with multi-byte
// runes could in principle match across a rune boundary in the subsequence pass,
// which produces a junk hit in the lowest band and is bounded by the budget.
func (m NameMatcher) Match(name string) (score int, scattered, ok bool) {
	// The name check comes first, and stays first: an unnamed row is not a result
	// even for the empty needle that otherwise matches everything.
	if name == "" {
		return 0, false, false
	}
	if m.needle == "" {
		return 0, false, true
	}
	hay := strings.ToLower(name)
	if s, _, found := m.substringScore(hay); found {
		return s, false, true
	}
	if s, found := m.scatteredScore(hay); found {
		return s, true, true
	}
	return 0, false, false
}

// MatchSpans returns the runes of name that produced the score Match reports, as
// sorted, disjoint spans in name's own rune coordinate space (SEARCH-06). It is a
// second pass rather than an extra return value from Match so the hot path — every
// row of every kind in a cluster-wide sweep — keeps allocating nothing; only the
// hits actually emitted (at most the search's cap) pay for their spans.
//
// It marks the *same* occurrence the score was read from, which is what makes the
// marks an explanation of the ranking rather than a second opinion about it: the
// best-scoring contiguous occurrence, or — when there is none — the tightened
// subsequence window scatteredScore charges for its gaps. A needle that occurs
// several times therefore marks one occurrence, unlike the table's `/` filter
// (D239), which marks them all: there the query is a substring by construction and
// every occurrence is equally the reason the row was kept, while here the score
// names one reading of the name and the marks say which.
//
// Nil means there is nothing to mark: no match, an empty name, or an empty needle
// (a pure label-selector query matches names it never looked at).
func (m NameMatcher) MatchSpans(name string) []MatchSpan {
	if name == "" || m.needle == "" {
		return nil
	}
	hay := strings.ToLower(name)
	if _, at, found := m.substringScore(hay); found {
		start := runeOffset(hay, at)
		return []MatchSpan{{Start: start, End: start + utf8.RuneCountInString(m.needle)}}
	}
	offsets := m.scatteredOffsets(hay)
	if offsets == nil {
		return nil
	}
	total := utf8.RuneCountInString(hay)
	spans := make([]MatchSpan, 0, len(offsets))
	for _, off := range offsets {
		at := runeOffset(hay, off)
		if at >= total {
			continue
		}
		// Adjacent matched runes coalesce, so `apiserver` matching `api-server`
		// marks two runs rather than nine one-rune ones — fewer style switches on
		// the wire and, more to the point, a mark a reader reads as a word.
		if n := len(spans); n > 0 && spans[n-1].End == at {
			spans[n-1].End = at + 1
			continue
		}
		spans = append(spans, MatchSpan{Start: at, End: at + 1})
	}
	if len(spans) == 0 {
		return nil
	}
	return spans
}

// runeOffset converts a byte offset in s to a rune offset. A byte offset landing
// mid-rune (possible only for the multi-byte needle Match's doc comment calls a
// junk hit) counts the partial rune rather than panicking — a junk match may mark
// the wrong rune, but it may not crash the view drawing it.
func runeOffset(s string, byteOff int) int {
	if byteOff > len(s) {
		byteOff = len(s)
	}
	if byteOff < 0 {
		byteOff = 0
	}
	return utf8.RuneCountInString(s[:byteOff])
}

// substringScore scores the best contiguous occurrence of the needle in hay
// (already lower-cased) and reports its byte offset, or that there is none.
func (m NameMatcher) substringScore(hay string) (score, at int, found bool) {
	best, bestAt := 0, 0
	for off := 0; off <= len(hay)-len(m.needle); {
		i := strings.Index(hay[off:], m.needle)
		if i < 0 {
			break
		}
		at := off + i
		s := scoreSubstringBand
		switch {
		case at == 0:
			s += scoreAtStart
		case isNameSeparator(hay[at-1]):
			s += scoreAtBoundary
		}
		// Tie-breakers: an earlier match and a tighter name win, clamped so they
		// can never cross a band.
		s -= clamp(at, maxStartPenalty)
		s -= clamp(len(hay)-len(m.needle), maxLenPenalty)
		if !found || s > best {
			best, bestAt, found = s, at, true
		}
		off = at + 1
	}
	return best, bestAt, found
}

// scatteredScore matches the needle against hay (already lower-cased) as a
// subsequence and scores it in the scattered band, or reports no match. It is only
// reached when the contiguous pass already failed.
//
// The window is found in two greedy passes because one is not enough. Taking each
// needle character at its earliest position finds *a* match but often a needlessly
// wide one — `abc` in `a-zz-b-zz-ab.c` matches a…b…c across the whole name before
// it reaches the tight `ab.c` at the end — and the gaps are exactly what the score
// is meant to punish. Walking back from where the forward pass ended pulls the
// window as far right as it will go, giving the tightest window ending there. That
// is not provably the tightest window in the name, but it is O(len(hay)) and it
// fixes the case that actually misleads.
//
// Backtracking is only worth doing because gaps are weighted above the start
// offset: shifting the window right by n costs n in start penalty and saves
// gapPenaltyWeight*n in gaps, so the tightest window always scores best and there
// is no need to score both candidates and take the max (which is what the
// contiguous pass does over its occurrences).
func (m NameMatcher) scatteredScore(hay string) (int, bool) {
	end, n := -1, 0
	for i := 0; i < len(hay) && n < len(m.needle); i++ {
		if hay[i] == m.needle[n] {
			n++
			end = i
		}
	}
	if n < len(m.needle) {
		return 0, false
	}
	start, n := end, len(m.needle)-1
	for i := end; i >= 0 && n >= 0; i-- {
		if hay[i] == m.needle[n] {
			n--
			start = i
		}
	}

	s := scoreScatteredBand
	s -= clamp(start, maxStartPenalty)
	// The filler the match had to jump over — the term that separates `api-server`
	// from `a-pod-in-some-random-vault`, and the scattered band's main signal.
	s -= gapPenaltyWeight * clamp(end-start+1-len(m.needle), maxGapPenalty)
	s -= clamp(len(hay)-len(m.needle), maxLenPenalty)
	return s, true
}

// scatteredOffsets returns the byte offset in hay of every needle character the
// scattered window matched, ascending. It walks the *same* two passes
// scatteredScore does — forward to find where the window must end, then backward
// from there to pull it as tight as it will go — so the characters it reports are
// the ones the score was computed over. nil when the needle is not a subsequence
// of hay at all.
//
// Only the backward pass's positions are kept: the forward pass exists solely to
// find `end`, and its own choices are the needlessly-wide window the backward pass
// is there to correct (see scatteredScore).
func (m NameMatcher) scatteredOffsets(hay string) []int {
	end, n := -1, 0
	for i := 0; i < len(hay) && n < len(m.needle); i++ {
		if hay[i] == m.needle[n] {
			n++
			end = i
		}
	}
	if n < len(m.needle) {
		return nil
	}
	out := make([]int, len(m.needle))
	n = len(m.needle) - 1
	for i := end; i >= 0 && n >= 0; i-- {
		if hay[i] == m.needle[n] {
			out[n] = i
			n--
		}
	}
	return out
}

// isNameSeparator reports the characters that start a new word inside a
// Kubernetes object name. RFC 1123 names only really admit `-` and `.`, but names
// reach kubecom from CRDs and generated resources too, so `_`, `/` and `:` are
// treated the same way rather than being silently scored as ordinary letters.
func isNameSeparator(b byte) bool {
	switch b {
	case '-', '.', '_', '/', ':':
		return true
	}
	return false
}

// clamp caps a non-negative penalty term at max.
func clamp(n, max int) int {
	if n > max {
		return max
	}
	if n < 0 {
		return 0
	}
	return n
}

// SearchEventType discriminates the messages a search streams. A consumer
// switches on it; every other SearchEvent field is only meaningful for the type
// that documents it.
type SearchEventType int

const (
	// SearchMatch carries one matched object in Hit.
	SearchMatch SearchEventType = iota
	// SearchKindDone reports that Resource has finished being searched — it
	// listed and was scanned, its List failed (Failed set), or the cap/a
	// cancellation cut it short. Exactly one is emitted per resource passed to
	// Search, so counting them against len(resources) is the progress signal
	// ("searching N/M kinds…"). It says nothing about how many hits that kind
	// contributed.
	SearchKindDone
	// SearchDone is the terminal event: the fan-out is over and no further event
	// follows before the channel closes. Capped tells the consumer *why* it
	// stopped — the hit cap, rather than exhausting every kind — which the close
	// alone cannot distinguish. It is not emitted once the caller's ctx is
	// cancelled: an abandoned search reports nothing, it just closes.
	SearchDone
)

// SearchEvent is one message from a cluster search. Type selects which of the
// remaining fields is set: Hit for SearchMatch, Resource/Failed for
// SearchKindDone, Capped for SearchDone.
//
// The stream is widened past bare hits so a consumer can distinguish progress
// from completion and a capped search from an exhaustive one (SEARCH-03) — the
// channel close on its own can express neither.
type SearchEvent struct {
	Type SearchEventType

	// Hit is the matched object (SearchMatch only).
	Hit SearchHit

	// Resource is the kind that finished (SearchKindDone only).
	Resource Resource
	// Failed marks a kind whose List errored, so it contributed nothing
	// (SearchKindDone only). A List cut short by the cap or by ctx cancellation
	// is not Failed. Informational: per-kind failure is silent by default
	// (D131 pt 3) and must not abort or degrade the rest of the search.
	Failed bool

	// Capped marks a search stopped by the hit cap rather than by exhausting
	// every kind (SearchDone only) — there were more matches than were emitted.
	// A search that emits exactly limit hits with nothing left over is not
	// Capped, and neither is one that merely dropped scattered hits over the
	// scattered budget (searchScatteredShare): those are the matches the search
	// itself rates worst, and telling a reader to narrow a query because some
	// fuzzy near-misses were withheld would send them after nothing.
	Capped bool
}

// searchChanBuffer bounds how far the fan-out may run ahead of a slow consumer.
// Events are tiny and the consumer is a Bubble Tea Update, so a modest buffer
// absorbs bursts (many kinds returning at once) without unbounded growth.
const searchChanBuffer = 64

// searchConcurrency bounds how many kinds are being listed at once. The curated
// default scope is eleven kinds and a burst of eleven LISTs is nothing, but the
// opt-in widen (SEARCH-04a) hands Search *every* discovered kind — well past a
// hundred on a cluster with a few operators installed — and client-go applies no
// client-side rate limit unless one is configured, so an unbounded fan-out would
// put that whole set on the wire in one breath. That is the hazard D131 pt 2
// named ("rate-limit-aware on big clusters"), and a semaphore is the cheapest
// answer: it costs the curated path nothing (its kinds never queue) and turns the
// widen into a steady stream of lists instead of a thundering herd.
//
// The number is a deliberate compromise rather than a measurement: high enough
// that a few slow kinds cannot stall the sweep behind them, low enough to stay
// polite to an apiserver that is also serving the browse view's live watches.
// Progress stays legible either way — SearchKindDone still fires once per kind,
// so a queued kind reads as "not done yet", exactly like a slow one.
const searchConcurrency = 8

// searchScatteredShare is the fraction of the hit cap that scattered
// (subsequence) matches may occupy: at most limit/searchScatteredShare of the
// emitted hits are fuzzy.
//
// It exists because the cap and the ranking act at different moments. The cap is
// applied at emit time, in arrival order, before any consumer has ranked anything
// — so without a budget the first kind to return could spend all 200 slots on
// scattered junk and the exact match in a kind that returned a beat later would
// never be emitted at all. Ranking cannot repair that: it orders what arrived, and
// the good hit is not among it. A share, not a second cap, because the two must
// stay coupled — the fuzzy allowance has to shrink with the caller's limit.
//
// Exhausting the share drops the hit and nothing else: the sweep is not cancelled
// (only the real cap does that), the kind keeps being scanned for contiguous
// matches, and the search is not reported Capped. The asymmetry is the point —
// substring hits may fill the whole cap and starve fuzzy entirely, which is the
// correct outcome; fuzzy may never starve substring.
const searchScatteredShare = 4

// scatteredLimit derives the scattered-hit budget from a search's hit cap. An
// uncapped search (limit <= 0) budgets nothing either — the caller asked for
// everything. A cap too small to divide still leaves room for one scattered hit,
// so a tiny limit degrades to "mostly exact" rather than to "no fuzzy at all".
func scatteredLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	if b := limit / searchScatteredShare; b > 0 {
		return b
	}
	return 1
}

// rowLister is the narrow List seam the search core needs, so the concurrent
// fan-out is exercised hermetically (D18) without a live server. *Clients
// satisfies it via List.
type rowLister interface {
	List(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error)
}

// Search fans out a one-shot, cancellable search across resources in namespace and
// streams SearchEvents onto the returned channel. Each kind is listed concurrently
// (reusing the server-side Table List, M1-05a) under query.LabelSelector, and a
// returned row whose object name matches query.Name — as a substring, or failing
// that as a subsequence — is emitted as a SearchMatch. A cluster-scoped kind
// ignores namespace (listed cluster-wide).
//
// This is a one-shot query, NOT a watch: it lists each kind exactly once and
// never re-lists. Cross-type enumeration is the expensive work the fast-cold-
// start design (D8/principle 4) avoids on the hot path, so it only ever runs on
// a search the user explicitly triggered, over the caller-chosen (curated by
// default, see CommonSearchResources) kind set — never "watch everything".
//
// Behaviour a consumer can rely on:
//   - Per-kind failure isolation: a denied or broken kind contributes nothing and
//     never aborts the search (principle 3) — its List error is swallowed into
//     SearchKindDone{Failed: true}.
//   - Progress: exactly one SearchKindDone per resource, so N/len(resources) is a
//     progress fraction.
//   - Cap: at most limit hits are emitted (limit <= 0 means no cap); once the cap
//     is reached the still-running lists are cancelled and the terminal
//     SearchDone reports Capped. Within it, scattered (subsequence) hits are
//     budgeted to a fraction of the cap (searchScatteredShare) so a fuzzy
//     near-miss can never crowd out an exact match that a slower kind still owes;
//     hits dropped that way neither stop the sweep nor set Capped.
//   - Cancellation: the channel is closed when every kind has been searched, the
//     cap is reached, or ctx is cancelled. A background goroutine owns all sends,
//     so consumer state is only ever mutated in its own Update.
//   - Bounded load: at most searchConcurrency kinds are listed at once, so a
//     wide scope arrives as a steady stream of lists rather than all at once
//     (D131 pt 2). Ordering is therefore not guaranteed and never was — hits
//     stream in whatever order the kinds return.
//   - Ranking, but not ordering: every SearchMatch carries a SearchHit.Score
//     saying how well the name matched, and the consumer sorts on it. The stream
//     stays in arrival order on purpose — emitting in rank order would mean
//     holding every hit until the last kind returned, which is the streaming
//     result list itself (D152). Scattered matches score in a band strictly below
//     every substring match, so a consumer that sorts on Score alone already
//     shows the fuzzy results last.
//   - Selector faults are not special: a kind that rejects the label selector
//     fails its List like any other broken kind (SearchKindDone{Failed}) and the
//     rest of the search proceeds. Validating the selector before it is sent
//     (ParseSearchQuery) is what keeps that from being the normal case.
func (c *Clients) Search(ctx context.Context, resources []Resource, namespace string, query SearchQuery, limit int) <-chan SearchEvent {
	return searchRows(ctx, c, resources, namespace, query, limit)
}

// searchRows is the injectable core of Search: it takes a rowLister (real or
// fake) so the concurrent fan-out, matching, cap, and per-kind fault isolation
// are testable without a live apiserver (D18), mirroring the getTable/List split.
func searchRows(ctx context.Context, lister rowLister, resources []Resource, namespace string, query SearchQuery, limit int) <-chan SearchEvent {
	out := make(chan SearchEvent, searchChanBuffer)
	go func() {
		defer close(out)

		// A local child context so reaching the cap can cancel the sibling
		// lists without disturbing the caller's ctx. Sends are guarded by the
		// caller's ctx (outer), not this one: the cap cancels the *listing*, but
		// the progress and terminal events it produces must still reach the
		// consumer — only the caller walking away stops delivery.
		outer := ctx
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		matcher := NewNameMatcher(query.Name)
		// One ListOptions for the whole fan-out: the selector is the same for
		// every kind, and it is the server that applies it, so a selector narrows
		// the rows on the wire instead of being filtered out after arriving.
		opts := metav1.ListOptions{LabelSelector: query.LabelSelector}
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			sent      int
			scattered int
			capped    bool
		)
		fuzzyCap := scatteredLimit(limit)
		// sem admits at most searchConcurrency kinds to the wire at once. Every
		// kind still gets its goroutine — they are cheap, and one goroutine per
		// kind is what keeps "exactly one SearchKindDone per resource" true no
		// matter where a kind is when the cap or the caller cancels.
		sem := make(chan struct{}, searchConcurrency)
		for _, r := range resources {
			wg.Add(1)
			go func(r Resource) {
				done := SearchEvent{Type: SearchKindDone, Resource: r}
				defer func() {
					sendEvent(outer, out, done)
					wg.Done()
				}()

				// Wait for a slot, but never past cancellation: once the cap is
				// reached the queued kinds must unwind immediately rather than
				// each taking a turn to discover there is nothing left to do.
				// They still report done — a kind cut short is not a failed one.
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}

				ns := namespace
				if !r.Namespaced {
					ns = "" // cluster-scoped: namespace does not apply
				}
				tbl, err := lister.List(ctx, r, ns, opts)
				if err != nil || tbl == nil {
					// A List aborted because the cap (or the caller) cancelled
					// the context is not a failing kind — only a genuine List
					// error is, and it degrades to no contribution.
					done.Failed = ctx.Err() == nil
					return
				}
				for _, row := range tbl.Rows {
					score, isScattered, ok := matcher.Match(row.Object.Name)
					if !ok {
						continue
					}
					// Reserve a slot under the lock so the cap is exact across
					// concurrent kinds; the send itself happens off-lock.
					mu.Lock()
					if limit > 0 && sent >= limit {
						capped = true
						mu.Unlock()
						cancel() // cap reached — stop the other in-flight lists
						return
					}
					if isScattered && fuzzyCap > 0 && scattered >= fuzzyCap {
						// The scattered budget is spent. Drop this hit and keep
						// going: unlike the cap this is not a reason to stop the
						// sweep — every kind still owes the search its contiguous
						// matches, which are the ones worth waiting for.
						mu.Unlock()
						continue
					}
					sent++
					if isScattered {
						scattered++
					}
					mu.Unlock()

					// Spans are resolved here, off the lock and only for a hit that
					// survived both the cap and the scattered budget: a dropped hit
					// is never painted, so it never needs to know what it matched.
					hit := SearchEvent{Type: SearchMatch, Hit: SearchHit{
						Resource: r,
						Ref:      row.Object,
						Columns:  tbl.Columns,
						Cells:    row.Cells,
						Score:    score,
						Match:    matcher.MatchSpans(row.Object.Name),
					}}
					if !sendEvent(outer, out, hit) {
						return
					}
				}
			}(r)
		}
		wg.Wait()

		mu.Lock()
		stoppedAtCap := capped
		mu.Unlock()
		sendEvent(outer, out, SearchEvent{Type: SearchDone, Capped: stoppedAtCap})
	}()
	return out
}

// sendEvent delivers ev on out unless ctx is cancelled first; it returns false
// when the send is abandoned so the producing goroutine can unwind promptly.
//
// The explicit pre-check matters: out is buffered, so with a cancelled ctx *both*
// select arms are ready and the runtime would pick one at random — a cancelled
// search would emit events (including the terminal one) roughly half the time.
// Checking first makes "cancelled ⇒ nothing more is emitted" hold.
//
// It is generic over the event type so the shared fan-out guard serves every
// concurrent stream in this package — Search and Scan alike — rather than each
// duplicating the subtlety (D276).
func sendEvent[T any](ctx context.Context, out chan<- T, ev T) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// commonSearchGroupKinds is the curated default scope for a cluster search: the
// high-signal, common kinds a search targets by default (D131). Keyed by
// GroupKind (version-agnostic) so it selects from whatever version discovery
// resolved each kind to. Whole-cluster search over every discovered kind is an
// opt-in widen the caller performs by simply not calling through here
// (SEARCH-04a), NOT the default — listing every type is the expensive
// enumeration the fast-start design (D8/principle 4) avoids.
var commonSearchGroupKinds = map[schema.GroupKind]struct{}{
	{Group: "", Kind: "Pod"}:                      {},
	{Group: "", Kind: "Service"}:                  {},
	{Group: "", Kind: "ConfigMap"}:                {},
	{Group: "", Kind: "Secret"}:                   {},
	{Group: "", Kind: "PersistentVolumeClaim"}:    {},
	{Group: "apps", Kind: "Deployment"}:           {},
	{Group: "apps", Kind: "StatefulSet"}:          {},
	{Group: "apps", Kind: "DaemonSet"}:            {},
	{Group: "batch", Kind: "Job"}:                 {},
	{Group: "batch", Kind: "CronJob"}:             {},
	{Group: "networking.k8s.io", Kind: "Ingress"}: {},
}

// CommonSearchResources filters all (typically the discovered resource set) down
// to the curated default search scope — the common, high-signal kinds a cluster
// search targets by default (D131) — preserving all's order. A curated kind the
// cluster does not expose simply isn't included. Passing the full set to Search
// instead *is* the whole-cluster widen (SEARCH-04a): there is no widen flag
// anywhere in this package, only the caller's choice of whether to filter through
// here first. The default path does, so it never enumerates every kind
// (D8/principle 4).
func CommonSearchResources(all []Resource) []Resource {
	out := make([]Resource, 0, len(commonSearchGroupKinds))
	for _, r := range all {
		if _, ok := commonSearchGroupKinds[r.GVK.GroupKind()]; ok {
			out = append(out, r)
		}
	}
	return out
}
