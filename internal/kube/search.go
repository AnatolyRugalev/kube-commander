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

// searchChanBuffer bounds how far the fan-out may run ahead of a slow consumer.
// Hits are tiny and the consumer is a Bubble Tea Update, so a modest buffer
// absorbs bursts (many kinds returning at once) without unbounded growth.
const searchChanBuffer = 64

// rowLister is the narrow List seam the search core needs, so the concurrent
// fan-out is exercised hermetically (D18) without a live server. *Clients
// satisfies it via List.
type rowLister interface {
	List(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error)
}

// Search fans out a one-shot, cancellable name search across resources in
// namespace and streams matching objects onto the returned channel. Each kind is
// listed concurrently (reusing the server-side Table List, M1-05a); a row whose
// object name contains query (case-insensitive substring) is emitted as a
// SearchHit. A cluster-scoped kind ignores namespace (listed cluster-wide).
//
// This is a one-shot query, NOT a watch: it lists each kind exactly once and
// never re-lists. Cross-type enumeration is the expensive work the fast-cold-
// start design (D8/principle 4) avoids on the hot path, so it only ever runs on
// a search the user explicitly triggered, over the caller-chosen (curated by
// default, see CommonSearchResources) kind set — never "watch everything".
//
// Behaviour a consumer can rely on:
//   - Per-kind failure isolation: a denied or broken kind contributes nothing and
//     never aborts the search (principle 3) — its List error is swallowed.
//   - Cap: at most limit hits are emitted (limit <= 0 means no cap); once the cap
//     is reached the still-running lists are cancelled.
//   - Cancellation: the channel is closed when every kind has been searched, the
//     cap is reached, or ctx is cancelled. A background goroutine owns all sends,
//     so consumer state is only ever mutated in its own Update.
func (c *Clients) Search(ctx context.Context, resources []Resource, namespace, query string, limit int) <-chan SearchHit {
	return searchRows(ctx, c, resources, namespace, query, limit)
}

// searchRows is the injectable core of Search: it takes a rowLister (real or
// fake) so the concurrent fan-out, matching, cap, and per-kind fault isolation
// are testable without a live apiserver (D18), mirroring the getTable/List split.
func searchRows(ctx context.Context, lister rowLister, resources []Resource, namespace, query string, limit int) <-chan SearchHit {
	out := make(chan SearchHit, searchChanBuffer)
	go func() {
		defer close(out)

		// A local child context so reaching the cap can cancel the sibling
		// lists without disturbing the caller's ctx.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		needle := strings.ToLower(query)
		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			sent int
		)
		for _, r := range resources {
			wg.Add(1)
			go func(r Resource) {
				defer wg.Done()
				ns := namespace
				if !r.Namespaced {
					ns = "" // cluster-scoped: namespace does not apply
				}
				tbl, err := lister.List(ctx, r, ns, metav1.ListOptions{})
				if err != nil || tbl == nil {
					return // per-kind failure degrades to no contribution
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
						mu.Unlock()
						cancel() // cap reached — stop the other in-flight lists
						return
					}
					sent++
					mu.Unlock()

					if !sendHit(ctx, out, SearchHit{Resource: r, Ref: row.Object}) {
						return
					}
				}
			}(r)
		}
		wg.Wait()
	}()
	return out
}

// sendHit delivers h on out unless ctx is cancelled first; it returns false when
// the send is abandoned so the producing goroutine can unwind promptly.
func sendHit(ctx context.Context, out chan<- SearchHit, h SearchHit) bool {
	select {
	case out <- h:
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
