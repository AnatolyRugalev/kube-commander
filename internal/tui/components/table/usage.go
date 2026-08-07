package table

import (
	"strconv"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// This file is the M4-10 metrics overlay: the CPU/MEMORY columns the table shows
// for a kind metrics-server measures (Pods, Nodes).
//
// The samples are *not* row data. Rows belong to the watch and are replaced
// wholesale by every RESET (a reconnect, a re-list); a usage number written into
// kube.Row.Cells would be wiped by the next delta and would have to be re-joined
// on arrival anyway. So the overlay is a separate map the component holds, and the
// displayed columns/cells are derived from it in applyFilter — the one place the
// displayed view is re-derived from the authoritative set. Rows and samples then
// refresh on their own clocks (a live watch, a slow poll) and neither can clobber
// the other.
//
// Deriving them there rather than at render time is what makes the overlay
// ordinary: the two columns are real visible columns, so they are measured,
// filtered, horizontally scrolled and sortable with no special cases anywhere
// else. Sorting is the one place they need help — "128Mi" does not compare
// numerically as text — and it reads the raw sample instead (usageSortKey).

// The overlay's column names and count. CPU is millicores and MEMORY mebibytes,
// the units `kubectl top` prints, so the numbers are comparable to the tool a
// reader already knows and every cell in a column carries the same unit.
const (
	usageCPUColumn    = "CPU"
	usageMemColumn    = "MEMORY"
	usageColumnCount  = 2
	bytesPerMebibyte  = 1024 * 1024
	usageCellNoSample = "" // no sample yet: blank, never "0m" (D167 — a number nobody measured).
)

// SetUsage installs the metrics overlay: the samples keyed as kube.Metrics
// returns them, joined onto rows by namespace/name (kube.UsageKeyOf — never by
// UID, which a metrics object does not share with the object it measures).
//
// A **nil** map turns the overlay off and the columns disappear; a non-nil but
// empty map keeps them, showing blanks. That distinction is the availability
// answer the caller already has (metrics-server present for this kind, or not)
// kept separate from whether anything has been scraped yet — a cluster that has
// just started metrics-server has the columns, empty, rather than no columns.
//
// The selection is preserved by object UID, like every other re-derivation here:
// a poll landing must not move the reader's cursor.
func (m *Model) SetUsage(u map[kube.UsageKey]kube.Usage) {
	selUID := m.selectedUID()
	m.usage = u
	m.applyFilter()
	m.restoreSelection(selUID)
	m.clampOffset()
	m.clampHOffset()
}

// Usage is the installed overlay (nil when off) — what the caller last set, so a
// refresh that failed can re-install the previous samples rather than blanking.
func (m Model) Usage() map[kube.UsageKey]kube.Usage { return m.usage }

// hasUsage reports whether the metrics columns are part of the displayed view.
func (m Model) hasUsage() bool { return m.usage != nil }

// columnsWithUsage is the displayed column set: the server's columns plus the two
// overlay columns when it is on. It always builds a new slice, so the appended
// columns can never leak into the authoritative full.Columns the watch delivered.
func (m Model) columnsWithUsage() []kube.Column {
	if !m.hasUsage() {
		return m.full.Columns
	}
	cols := make([]kube.Column, 0, len(m.full.Columns)+usageColumnCount)
	cols = append(cols, m.full.Columns...)
	return append(cols,
		kube.Column{Name: usageCPUColumn, Type: "string"},
		kube.Column{Name: usageMemColumn, Type: "string"},
	)
}

// rowsWithUsage is the row set the displayed view is derived from: the
// authoritative rows, each with its two usage cells appended when the overlay is
// on. The cells are placed at the overlay columns' fixed indices — a short row
// (one whose object metadata or cells the server degraded, principle 3) is padded
// out first, so a missing cell can never shift a usage value under someone else's
// column.
//
// Rows are copied either way: the sort that follows reorders the displayed view,
// never the authoritative full.Rows.
func (m Model) rowsWithUsage() []kube.Row {
	if !m.hasUsage() {
		return append(m.table.Rows[:0], m.full.Rows...)
	}
	base := len(m.full.Columns)
	rows := make([]kube.Row, len(m.full.Rows))
	for i, r := range m.full.Rows {
		cells := make([]any, base+usageColumnCount)
		copy(cells, r.Cells)
		cpu, mem := usageCells(m.usage, r.Object)
		cells[base], cells[base+1] = cpu, mem
		rows[i] = kube.Row{Cells: cells, Object: r.Object}
	}
	return rows
}

// usageCells renders one row's sample as the two display strings, blank when the
// object has no sample — not scraped yet, or scraped between polls.
func usageCells(usage map[kube.UsageKey]kube.Usage, ref kube.ObjectRef) (cpu, mem string) {
	u, ok := usage[kube.UsageKeyOf(ref)]
	if !ok {
		return usageCellNoSample, usageCellNoSample
	}
	return formatMillicores(u.CPUMilli), formatMebibytes(u.MemoryBytes)
}

// formatMillicores renders CPU usage the way `kubectl top` does: whole millicores
// with an `m` suffix, never scaled to cores, so a column of them lines up and
// compares by eye.
func formatMillicores(milli int64) string {
	return strconv.FormatInt(milli, 10) + "m"
}

// formatMebibytes renders memory usage in whole mebibytes with a `Mi` suffix,
// again matching `kubectl top`. It truncates rather than rounds (as kubectl does),
// so a pod using less than a mebibyte reads `0Mi` — that is a real measurement, and
// distinct from the blank a missing sample renders as.
func formatMebibytes(bytes int64) string {
	return strconv.FormatInt(bytes/bytesPerMebibyte, 10) + "Mi"
}

// usageSortKey returns the numeric sort key for the overlay column at displayed
// index ci, or false when ci is not one of them.
//
// The overlay's cells are formatted strings ("128Mi"), which sort as text into
// nonsense (100m < 20m). Sorting on the raw sample instead is both exact and free:
// the map is already in hand and keyed by the same ref. A row with no sample sorts
// as -1 — below every real measurement including zero — so the unscraped rows
// gather at one end rather than mixing into the numbers.
func (m Model) usageSortKey(ci int) (func(kube.Row) int64, bool) {
	if !m.hasUsage() {
		return nil, false
	}
	base := len(m.full.Columns)
	var pick func(kube.Usage) int64
	switch ci {
	case base:
		pick = func(u kube.Usage) int64 { return u.CPUMilli }
	case base + 1:
		pick = func(u kube.Usage) int64 { return u.MemoryBytes }
	default:
		return nil, false
	}
	usage := m.usage
	return func(r kube.Row) int64 {
		u, ok := usage[kube.UsageKeyOf(r.Object)]
		if !ok {
			return -1
		}
		return pick(u)
	}, true
}
