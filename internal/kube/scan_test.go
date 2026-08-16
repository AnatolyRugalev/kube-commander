package kube

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

// statusTbl builds a server-printed Table whose rows carry a STATUS column, so a
// scan's predicate can be exercised against real column names the way the M4-06
// health classifier will be (STORY-06g). name/status pairs.
func statusTbl(ns string, rows ...[2]string) *Table {
	t := &Table{Columns: []Column{{Name: "NAME"}, {Name: "STATUS"}}}
	for _, r := range rows {
		t.Rows = append(t.Rows, Row{
			Cells:  []any{r[0], r[1]},
			Object: ObjectRef{Namespace: ns, Name: r[0], UID: r[0] + "-uid"},
		})
	}
	return t
}

// unhealthy keeps rows whose STATUS cell is one of the given values — a stand-in
// for the TUI's M4-06 health predicate, which lives outside this package (D276).
func unhealthy(keep ...string) RowFilter {
	set := make(map[string]bool, len(keep))
	for _, s := range keep {
		set[s] = true
	}
	return func(tbl *Table, row Row) bool {
		for i, c := range tbl.Columns {
			if c.Name != "STATUS" || i >= len(row.Cells) {
				continue
			}
			if s, ok := row.Cells[i].(string); ok && set[s] {
				return true
			}
		}
		return false
	}
}

// collectScan drains a scan channel into a name-sorted slice of "kind/name"
// strings, ignoring the progress/terminal events (asserted separately).
func collectScan(ch <-chan ScanEvent) []string {
	var got []string
	for ev := range ch {
		if ev.Type != ScanMatch {
			continue
		}
		got = append(got, ev.Hit.Resource.GVK.Kind+"/"+ev.Hit.Row.Object.Name)
	}
	sort.Strings(got)
	return got
}

// drainScan collects every event of a scan, in arrival order.
func drainScan(ch <-chan ScanEvent) []ScanEvent {
	var got []ScanEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

// scanKindsDone reports the kinds that emitted a ScanKindDone, and which of them
// were marked Failed.
func scanKindsDone(events []ScanEvent) (done []string, failed []string) {
	for _, ev := range events {
		if ev.Type != ScanKindDone {
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

// scanTerminal returns the single ScanDone event, failing when the stream did
// not end with exactly one.
func scanTerminal(t *testing.T, events []ScanEvent) ScanEvent {
	t.Helper()
	var got []ScanEvent
	for _, ev := range events {
		if ev.Type == ScanDone {
			got = append(got, ev)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one ScanDone event, got %d", len(got))
	}
	if last := events[len(events)-1]; last.Type != ScanDone {
		t.Fatalf("ScanDone must be the last event before close, got %v", last.Type)
	}
	return got[0]
}

// TestScanKeepsOnlyPredicateRows drives the whole point of the primitive: across
// kinds, only the rows the caller's predicate accepts are emitted, and the
// healthy rows of the same tables are not — the S02 miss (a broken PVC) surfaced
// beside the broken pods.
func TestScanKeepsOnlyPredicateRows(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": statusTbl("web",
			[2]string{"web-1", "Running"},
			[2]string{"web-2", "CrashLoopBackOff"},
		),
		"persistentvolumeclaims": statusTbl("web",
			[2]string{"data", "Bound"},
			[2]string{"stuck", "Pending"},
		),
	}}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("", "v1", "PersistentVolumeClaim", "persistentvolumeclaims", true),
	}

	got := collectScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("CrashLoopBackOff", "Pending", "ImagePullBackOff"), 0))
	want := []string{"PersistentVolumeClaim/stuck", "Pod/web-2"}
	if !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
}

// TestScanIsolatesPerKindFailure pins the degrade-don't-crash half (principle 3):
// a kind the caller cannot list contributes nothing and never aborts the scan —
// the broken pods still surface.
func TestScanIsolatesPerKindFailure(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{
			"pods": statusTbl("web", [2]string{"web-2", "CrashLoopBackOff"}),
		},
		errs: map[string]error{"persistentvolumeclaims": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("", "v1", "PersistentVolumeClaim", "persistentvolumeclaims", true),
	}

	got := collectScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("CrashLoopBackOff"), 0))
	if want := []string{"Pod/web-2"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v (per-kind failure should degrade to nothing)", got, want)
	}
}

// TestScanCapsTotalHits mirrors Search's cap: at most limit rows are emitted and
// the terminal event says Capped, so a sweep cannot flood a consumer with every
// broken object of a big cluster.
func TestScanCapsTotalHits(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": statusTbl("web",
			[2]string{"a", "Pending"}, [2]string{"b", "Pending"},
			[2]string{"c", "Pending"}, [2]string{"d", "Pending"},
		),
	}}
	resources := []Resource{res("", "v1", "Pod", "pods", true)}

	events := drainScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("Pending"), 2))
	n := 0
	for _, ev := range events {
		if ev.Type == ScanMatch {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("emitted %d hits, want cap of 2", n)
	}
	if !scanTerminal(t, events).Capped {
		t.Error("a scan stopped by the hit cap must report ScanDone{Capped: true}")
	}
}

// TestScanDoneNotCappedWhenHitsExactlyFitLimit pins the Capped semantics: it
// means "there were more matches than you were given", not "you were given limit
// matches".
func TestScanDoneNotCappedWhenHitsExactlyFitLimit(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": statusTbl("web", [2]string{"a", "Pending"}, [2]string{"b", "Pending"}),
	}}
	resources := []Resource{res("", "v1", "Pod", "pods", true)}

	events := drainScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("Pending"), 2))
	if scanTerminal(t, events).Capped {
		t.Error("matches exactly filling the cap truncated nothing, so Capped must be false")
	}
}

// TestScanReportsKindCompletion proves the progress signal: every kind handed to
// a scan emits exactly one ScanKindDone — including the one whose List failed.
func TestScanReportsKindCompletion(t *testing.T) {
	lister := &fakeLister{
		tables: map[string]*Table{
			"pods":  statusTbl("web", [2]string{"web-2", "CrashLoopBackOff"}),
			"nodes": statusTbl("", [2]string{"node-1", "NotReady"}),
		},
		errs: map[string]error{"persistentvolumeclaims": errors.New("forbidden")},
	}
	resources := []Resource{
		res("", "v1", "Pod", "pods", true),
		res("", "v1", "PersistentVolumeClaim", "persistentvolumeclaims", true),
		res("", "v1", "Node", "nodes", false),
	}

	events := drainScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("CrashLoopBackOff", "NotReady"), 0))

	done, failed := scanKindsDone(events)
	if want := []string{"Node", "PersistentVolumeClaim", "Pod"}; !equal(done, want) {
		t.Fatalf("kinds done = %v, want one per scanned kind %v", done, want)
	}
	if want := []string{"PersistentVolumeClaim"}; !equal(failed, want) {
		t.Errorf("failed kinds = %v, want %v", failed, want)
	}
	if scanTerminal(t, events).Capped {
		t.Error("an exhaustive scan must not report Capped")
	}
}

// TestScanClusterScopedIgnoresNamespace pins that a cluster-scoped kind is listed
// with an empty namespace even when the sweep runs scoped.
func TestScanClusterScopedIgnoresNamespace(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"nodes": statusTbl("", [2]string{"node-1", "NotReady"}),
	}}
	resources := []Resource{res("", "v1", "Node", "nodes", false)}

	got := collectScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("NotReady"), 0))
	if want := []string{"Node/node-1"}; !equal(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	if ns := lister.nsSeen["nodes"]; ns != "" {
		t.Errorf("cluster-scoped Node listed with namespace %q, want empty", ns)
	}
}

// TestScanCancellationClosesChannelWithoutTerminalEvent mirrors Search: an
// abandoned scan reports nothing, it just closes.
func TestScanCancellationClosesChannelWithoutTerminalEvent(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": statusTbl("web", [2]string{"a", "Pending"}),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before draining
	resources := []Resource{res("", "v1", "Pod", "pods", true)}

	for _, ev := range drainScan(scanRows(ctx, lister, resources, "web",
		unhealthy("Pending"), 0)) {
		if ev.Type == ScanDone {
			t.Error("a cancelled scan must not emit a terminal ScanDone")
		}
	}
}

// TestScanBoundsConcurrentLists pins the shared load bound: at most
// searchConcurrency kinds are listed at once — the same guard Search's fan-out
// runs under (D131 pt 2), so a wide unhealthy sweep stays polite to the
// apiserver.
func TestScanBoundsConcurrentLists(t *testing.T) {
	const kinds = searchConcurrency * 3
	g := &gateLister{arrived: make(chan string, kinds), proceed: make(chan struct{})}
	resources := manyResources(kinds)
	keepAll := func(tbl *Table, row Row) bool { return true }

	ch := scanRows(context.Background(), g, resources, "web", keepAll, 0)

	for range searchConcurrency {
		<-g.arrived
	}
	select {
	case extra := <-g.arrived:
		t.Fatalf("list for %q started while %d were already in flight: fan-out is unbounded", extra, searchConcurrency)
	case <-time.After(100 * time.Millisecond):
	}

	close(g.proceed)
	events := drainScan(ch)
	done, failed := scanKindsDone(events)
	if len(done) != kinds {
		t.Fatalf("kinds done = %d, want one per requested kind (%d)", len(done), kinds)
	}
	if len(failed) != 0 {
		t.Errorf("queued-then-released kinds must not be marked failed, got %v", failed)
	}
	if got := len(hitsOfScan(events)); got != kinds {
		t.Errorf("hits = %d, want one per kind (%d) — a queued kind must still be scanned", got, kinds)
	}
}

// hitsOfScan extracts the match events from a drained stream.
func hitsOfScan(events []ScanEvent) []ScanHit {
	var out []ScanHit
	for _, ev := range events {
		if ev.Type == ScanMatch {
			out = append(out, ev.Hit)
		}
	}
	return out
}

// TestScanHitCarriesTheRow pins why a scan hit differs from a search hit: the row
// cells ride along so a surface can render the reason (the offending cell)
// without re-listing the kind.
func TestScanHitCarriesTheRow(t *testing.T) {
	lister := &fakeLister{tables: map[string]*Table{
		"pods": statusTbl("web", [2]string{"web-2", "CrashLoopBackOff"}),
	}}
	resources := []Resource{res("", "v1", "Pod", "pods", true)}

	hits := hitsOfScan(drainScan(scanRows(context.Background(), lister, resources, "web",
		unhealthy("CrashLoopBackOff"), 0)))
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	h := hits[0]
	if h.Resource.GVK.Kind != "Pod" {
		t.Errorf("hit kind = %q, want Pod", h.Resource.GVK.Kind)
	}
	if h.Row.Object.Name != "web-2" {
		t.Errorf("hit object = %+v, want web-2", h.Row.Object)
	}
	if len(h.Row.Cells) != 2 || h.Row.Cells[1] != "CrashLoopBackOff" {
		t.Errorf("hit should carry the offending cell, got %v", h.Row.Cells)
	}
}
