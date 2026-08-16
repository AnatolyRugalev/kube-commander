package kube

import (
	"context"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanHit is one row a cross-kind scan kept: the kind it belongs to and the row
// itself, cells included. Unlike a SearchHit — which deliberately carries no
// cells because drilling into it re-lists the real table — a scan hit is meant to
// be *shown* in a list (kind · name · namespace · the offending cell), so it
// carries the row so a surface can render the reason without a second round-trip.
// Row.Object is the object's identity.
//
// Columns are the table the row came from (D33 server columns), carried so a
// surface can classify the row's cells by column name — "which cell is the
// offending one" needs the columns the reason sits under. Without them a hit
// could only be rendered as a blob of cells with no way to name the offender.
type ScanHit struct {
	Resource Resource
	Columns  []Column
	Row      Row
}

// ScanEventType discriminates the messages a cross-kind scan streams, mirroring
// SearchEventType: a consumer switches on it, and every other ScanEvent field is
// only meaningful for the type that documents it.
type ScanEventType int

const (
	// ScanMatch carries one kept row in Hit.
	ScanMatch ScanEventType = iota
	// ScanKindDone reports that Resource has finished being scanned — it listed
	// and was filtered, its List failed (Failed set), or the cap/cancellation cut
	// it short. Exactly one is emitted per resource passed to Scan, so counting
	// them against len(resources) is the progress signal. It says nothing about
	// how many hits that kind contributed.
	ScanKindDone
	// ScanDone is the terminal event: the fan-out is over and no further event
	// follows before the channel closes. Capped tells the consumer *why* it
	// stopped — the hit cap, rather than exhausting every kind — which the close
	// alone cannot distinguish. It is not emitted once the caller's ctx is
	// cancelled: an abandoned scan reports nothing, it just closes.
	ScanDone
)

// ScanEvent is one message from a cross-kind scan. Type selects which of the
// remaining fields is set: Hit for ScanMatch, Resource/Failed for ScanKindDone,
// Capped for ScanDone.
//
// The stream is widened past bare hits so a consumer can distinguish progress
// from completion and a capped scan from an exhaustive one (mirroring
// SEARCH-03) — the channel close on its own can express neither.
type ScanEvent struct {
	Type ScanEventType

	// Hit is the kept row (ScanMatch only).
	Hit ScanHit

	// Resource is the kind that finished (ScanKindDone only).
	Resource Resource
	// Failed marks a kind whose List errored, so it contributed nothing
	// (ScanKindDone only). A List cut short by the cap or by ctx cancellation is
	// not Failed. Informational: per-kind failure is silent by default (D131
	// pt 3) and must not abort or degrade the rest of the scan.
	Failed bool

	// Capped marks a scan stopped by the hit cap rather than by exhausting every
	// kind (ScanDone only) — there were more matches than were emitted. A scan
	// that emits exactly limit hits with nothing left over is not Capped.
	Capped bool
}

// RowFilter is the predicate a cross-kind scan keeps rows on. It sees the row's
// whole table — columns included — so a caller can classify a row exactly the way
// its own surface does (the M4-06 health classifier is a TUI concern, keyed by
// column name in the table component, so the primitive takes it as a seam rather
// than importing it — D276). It must be pure: it runs once per row, from one
// goroutine per kind, concurrently, and is called again by no one after the sweep
// (hits carry the row itself, not the predicate's verdict).
type RowFilter func(tbl *Table, row Row) bool

// Scan fans out a one-shot, cancellable scan across resources in namespace and
// streams ScanEvents onto the returned channel. Each kind is listed concurrently
// (reusing the server-side Table List, M1-05a), and every returned row is passed
// to keep; the rows it accepts are emitted as ScanMatch. A cluster-scoped kind
// ignores namespace (listed cluster-wide).
//
// This is the cross-kind sweep of STORY-06g-2: Search matches rows on a name
// query, Scan keeps them on a caller's predicate — for the unhealthy surface, the
// M4-06 health classifier lifted to a row, so a failure that is not a pod finds
// the operator. It is a one-shot query, NOT a watch: it lists each kind exactly
// once and never re-lists. Cross-type enumeration is the expensive work the
// fast-cold-start design (D8/principle 4) avoids on the hot path, so it only ever
// runs on a sweep the user explicitly triggered, over the caller-chosen kind set
// — never "watch everything".
//
// Behaviour a consumer can rely on (the Search guarantees, mirrored):
//   - Per-kind failure isolation: a denied or broken kind contributes nothing and
//     never aborts the scan (principle 3) — its List error is swallowed into
//     ScanKindDone{Failed: true}.
//   - Progress: exactly one ScanKindDone per resource, so N/len(resources) is a
//     progress fraction.
//   - Cap: at most limit hits are emitted (limit <= 0 means no cap); once the cap
//     is reached the still-running lists are cancelled and the terminal ScanDone
//     reports Capped.
//   - Cancellation: the channel is closed when every kind has been scanned, the
//     cap is reached, or ctx is cancelled. A background goroutine owns all sends,
//     so consumer state is only ever mutated in its own Update.
//   - Bounded load: at most searchConcurrency kinds are listed at once, so a wide
//     scope arrives as a steady stream of lists rather than all at once (D131
//     pt 2). Ordering is therefore not guaranteed and never was — hits stream in
//     whatever order the kinds return.
func (c *Clients) Scan(ctx context.Context, resources []Resource, namespace string, keep RowFilter, limit int) <-chan ScanEvent {
	return scanRows(ctx, c, resources, namespace, keep, limit)
}

// scanRows is the injectable core of Scan: it takes a rowLister (real or fake) so
// the concurrent fan-out, the predicate, the cap, and the per-kind fault
// isolation are testable without a live apiserver (D18), mirroring the
// searchRows/List split.
func scanRows(ctx context.Context, lister rowLister, resources []Resource, namespace string, keep RowFilter, limit int) <-chan ScanEvent {
	out := make(chan ScanEvent, searchChanBuffer)
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

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			sent   int
			capped bool
		)
		// sem admits at most searchConcurrency kinds to the wire at once — the
		// same bound Search's fan-out runs under (D131 pt 2), so a wide scope
		// arrives as a steady stream of lists rather than a thundering herd.
		sem := make(chan struct{}, searchConcurrency)
		for _, r := range resources {
			wg.Add(1)
			go func(r Resource) {
				done := ScanEvent{Type: ScanKindDone, Resource: r}
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
				tbl, err := lister.List(ctx, r, ns, metav1.ListOptions{})
				if err != nil || tbl == nil {
					// A List aborted because the cap (or the caller) cancelled
					// the context is not a failing kind — only a genuine List
					// error is, and it degrades to no contribution.
					done.Failed = ctx.Err() == nil
					return
				}
				for _, row := range tbl.Rows {
					if !keep(tbl, row) {
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

					if !sendEvent(outer, out, ScanEvent{Type: ScanMatch, Hit: ScanHit{Resource: r, Columns: tbl.Columns, Row: row}}) {
						return
					}
				}
			}(r)
		}
		wg.Wait()

		mu.Lock()
		stoppedAtCap := capped
		mu.Unlock()
		sendEvent(outer, out, ScanEvent{Type: ScanDone, Capped: stoppedAtCap})
	}()
	return out
}
