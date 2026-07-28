package kube

import (
	"context"
	"fmt"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SearchHit is one object matched by a cluster search. Resource is the kind the
// object belongs to — enough for a caller to switch the browse view to that kind
// and select the row — and Ref is the object's identity (Ref.Namespace is empty
// for cluster-scoped kinds). A hit intentionally carries no printed cells: the
// search matches on object name only (M1-05 rows expose the name in
// Row.Object.Name), and drilling into a hit re-lists/watches the real table.
type SearchHit struct {
	Resource Resource
	Ref      ObjectRef
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
	// Name is a case-insensitive substring matched against each object's name.
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
	// Capped.
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

// rowLister is the narrow List seam the search core needs, so the concurrent
// fan-out is exercised hermetically (D18) without a live server. *Clients
// satisfies it via List.
type rowLister interface {
	List(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error)
}

// Search fans out a one-shot, cancellable search across resources in namespace and
// streams SearchEvents onto the returned channel. Each kind is listed concurrently
// (reusing the server-side Table List, M1-05a) under query.LabelSelector, and a
// returned row whose object name contains query.Name (case-insensitive substring)
// is emitted as a SearchMatch. A cluster-scoped kind ignores namespace (listed
// cluster-wide).
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
//     SearchDone reports Capped.
//   - Cancellation: the channel is closed when every kind has been searched, the
//     cap is reached, or ctx is cancelled. A background goroutine owns all sends,
//     so consumer state is only ever mutated in its own Update.
//   - Bounded load: at most searchConcurrency kinds are listed at once, so a
//     wide scope arrives as a steady stream of lists rather than all at once
//     (D131 pt 2). Ordering is therefore not guaranteed and never was — hits
//     stream in whatever order the kinds return.
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

		needle := strings.ToLower(query.Name)
		// One ListOptions for the whole fan-out: the selector is the same for
		// every kind, and it is the server that applies it, so a selector narrows
		// the rows on the wire instead of being filtered out after arriving.
		opts := metav1.ListOptions{LabelSelector: query.LabelSelector}
		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			sent   int
			capped bool
		)
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
					name := row.Object.Name
					if name == "" {
						continue
					}
					if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
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
					sent++
					mu.Unlock()

					hit := SearchEvent{Type: SearchMatch, Hit: SearchHit{Resource: r, Ref: row.Object}}
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
func sendEvent(ctx context.Context, out chan<- SearchEvent, ev SearchEvent) bool {
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
