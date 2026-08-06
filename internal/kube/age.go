package kube

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/duration"
)

// This file keeps the AGE column honest while a pane sits still.
//
// Every cell of a server-printed Table is a string the API server's printer
// rendered at the instant it answered the request, AGE included — kubectl's
// printers call `duration.HumanDuration(time.Since(creationTimestamp))` and hand
// back the result. That is exactly right for `kubectl get`, which prints once and
// exits, and wrong for a TUI: kubecom keeps the pane on screen, and a row is only
// re-printed when the watch delivers a delta for it. A Pod that nothing modifies
// keeps saying `3m` for as long as the reader looks at it.
//
// So the age is re-derived here, locally, from the row's own creation timestamp
// (Row.Created, carried out of the Table's embedded object metadata) against a
// caller-supplied clock — and with the *same* formatter kubectl uses, so a
// recomputed cell is indistinguishable from a freshly printed one (D234). The
// clock is a parameter rather than time.Now so the behaviour is testable without
// sleeping.

// ageColumnName is the header the server-side printers give the creation-time
// column. It is a convention rather than a flag — metav1.TableColumnDefinition
// has no "this is an age" bit — so this is a name match, narrowed by the cell
// types that can hold a printed duration (see AgeColumn).
const ageColumnName = "age"

// AgeColumn reports the index of the column holding a printed object age, and
// whether there is one.
//
// Detection is by header name, case-insensitively, because that is all the server
// tells us: the built-in printers hard-code the column as `{Name: "Age", Type:
// "string"}` and a CRD's `additionalPrinterColumns` conventionally names its
// `.metadata.creationTimestamp` column "Age" with Type "date". Nothing in the
// Table schema distinguishes either from a CRD that prints, say, a cached
// artifact's age from a field of its own.
//
// The type check is what keeps that from mattering. A column named "Age" whose
// type is integer or number is some resource's own numeric field, not a printed
// duration, and overwriting it with an elapsed time would be a lie in a column
// the reader has no reason to distrust. Only string and date columns — the two
// the printers actually emit durations into — are eligible. When a CRD does put
// an unrelated *string* under that header, the cost is bounded and visible: the
// column shows the object's age instead of its value, which is the same failure
// `kubectl get` already has for sorting it.
func AgeColumn(cols []Column) (int, bool) {
	for i, c := range cols {
		if !strings.EqualFold(strings.TrimSpace(c.Name), ageColumnName) {
			continue
		}
		if c.Type != "" && c.Type != "string" && c.Type != "date" {
			continue
		}
		return i, true
	}
	return -1, false
}

// HumanAge formats an object's age the way the server-side printers do, so a
// locally re-derived cell reads exactly like the one it replaces — including the
// compound forms (`5d3h`, `2m30s`) and the clamp of a slightly-future timestamp
// to `0s`. now is passed in rather than read so callers can drive the clock.
func HumanAge(created, now time.Time) string {
	return duration.HumanDuration(now.Sub(created))
}

// RefreshAge re-derives the age column's cells against now, in place, and reports
// whether any cell actually changed — which is what lets a caller skip the work
// that follows (re-filter, re-sort, re-measure) on the ticks where nothing moved,
// and most ticks move nothing.
//
// It is a no-op for a table with no age column, and it skips any row whose
// Created is zero: a row that arrived without parsable object metadata keeps the
// string the server printed, because a wrong age computed from the zero time
// ("55y") is worse than a stale one. Idempotent — the result depends only on
// Created and now, never on the cell being replaced — so it is safe to run on
// every tick, after every watch delta, in any order.
func (t *Table) RefreshAge(now time.Time) bool {
	i, ok := AgeColumn(t.Columns)
	if !ok {
		return false
	}
	changed := false
	for r := range t.Rows {
		row := &t.Rows[r]
		if row.Created.IsZero() || i >= len(row.Cells) {
			continue
		}
		age := HumanAge(row.Created, now)
		if s, isStr := row.Cells[i].(string); isStr && s == age {
			continue
		}
		row.Cells[i] = age
		changed = true
	}
	return changed
}
