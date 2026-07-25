package kube

import (
	"context"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// rowLister is the narrow List seam the search core needs, so the concurrent
// fan-out is exercised hermetically (D18) without a live server. *Clients
// satisfies it via List.
type rowLister interface {
	List(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error)
}

// Search fans out a one-shot, cancellable name search across resources in
// namespace and streams SearchEvents onto the returned channel. Each kind is
// listed concurrently (reusing the server-side Table List, M1-05a); a row whose
// object name contains query (case-insensitive substring) is emitted as a
// SearchMatch. A cluster-scoped kind ignores namespace (listed cluster-wide).
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
func (c *Clients) Search(ctx context.Context, resources []Resource, namespace, query string, limit int) <-chan SearchEvent {
	return searchRows(ctx, c, resources, namespace, query, limit)
}

// searchRows is the injectable core of Search: it takes a rowLister (real or
// fake) so the concurrent fan-out, matching, cap, and per-kind fault isolation
// are testable without a live apiserver (D18), mirroring the getTable/List split.
func searchRows(ctx context.Context, lister rowLister, resources []Resource, namespace, query string, limit int) <-chan SearchEvent {
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

		needle := strings.ToLower(query)
		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			sent   int
			capped bool
		)
		for _, r := range resources {
			wg.Add(1)
			go func(r Resource) {
				done := SearchEvent{Type: SearchKindDone, Resource: r}
				defer func() {
					sendEvent(outer, out, done)
					wg.Done()
				}()

				ns := namespace
				if !r.Namespaced {
					ns = "" // cluster-scoped: namespace does not apply
				}
				tbl, err := lister.List(ctx, r, ns, metav1.ListOptions{})
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
// opt-in widen (a later slice), NOT the default — listing every type is the
// expensive enumeration the fast-start design (D8/principle 4) avoids.
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
// cluster does not expose simply isn't included. Passing the full set instead
// (whole-cluster search) is an opt-in widen a later slice adds; the default path
// goes through here so it never enumerates every kind (D8/principle 4).
func CommonSearchResources(all []Resource) []Resource {
	out := make([]Resource, 0, len(commonSearchGroupKinds))
	for _, r := range all {
		if _, ok := commonSearchGroupKinds[r.GVK.GroupKind()]; ok {
			out = append(out, r)
		}
	}
	return out
}
