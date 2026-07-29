package table

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// usageSamples is a sample set covering the pods of sampleTable — deliberately not
// pod-c, so every test has a row the server has no sample for.
func usageSamples() map[kube.UsageKey]kube.Usage {
	return map[kube.UsageKey]kube.Usage{
		{Namespace: "default", Name: "pod-a"}: {
			Namespace: "default", Name: "pod-a", CPUMilli: 20, MemoryBytes: 512 * 1024 * 1024,
		},
		{Namespace: "default", Name: "pod-b"}: {
			Namespace: "default", Name: "pod-b", CPUMilli: 100, MemoryBytes: 64 * 1024 * 1024,
		},
	}
}

// usageTable is a sized table showing sampleTable with the overlay installed.
func usageTable(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetSize(80, 8)
	m.SetUsage(usageSamples())
	return m
}

// rowText is one rendered data row, unstyled. Row 0 is the first row under the
// header (line 0 is the border, line 1 the header).
func rowText(view string, row int) string {
	lines := strings.Split(view, "\n")
	if row+2 >= len(lines) {
		return ""
	}
	return ansi.Strip(lines[row+2])
}

// TestUsageColumnsAppearWithSamples is the leg's headline on the component side: an
// installed overlay grows the two columns and joins each row's sample onto it by
// namespace/name — kubectl top's units, so the numbers are comparable to the tool
// the reader already has.
func TestUsageColumnsAppearWithSamples(t *testing.T) {
	m := usageTable(t)

	header := headerLine(m.View())
	if !strings.Contains(header, usageCPUColumn) || !strings.Contains(header, usageMemColumn) {
		t.Fatalf("header should carry the metrics columns, got %q", header)
	}
	if got := rowText(m.View(), 0); !strings.Contains(got, "20m") || !strings.Contains(got, "512Mi") {
		t.Errorf("pod-a row should show its sample (20m / 512Mi), got %q", got)
	}
	if got := rowText(m.View(), 1); !strings.Contains(got, "100m") || !strings.Contains(got, "64Mi") {
		t.Errorf("pod-b row should show its sample (100m / 64Mi), got %q", got)
	}
}

// TestUsageBlankWithoutSample proves an unmeasured row renders blank rather than
// "0m": a pod metrics-server has not scraped yet has no usage, and printing zero
// would be a number nobody measured (D167).
func TestUsageBlankWithoutSample(t *testing.T) {
	m := usageTable(t)

	got := rowText(m.View(), 2) // pod-c, absent from the sample set
	if !strings.Contains(got, "pod-c") {
		t.Fatalf("expected pod-c's row, got %q", got)
	}
	if strings.Contains(got, "0m") || strings.Contains(got, "Mi") {
		t.Errorf("a row with no sample must render blank usage cells, got %q", got)
	}
}

// TestUsageNilRemovesColumns proves the overlay is switchable: nil is the "this
// cluster does not measure this kind" answer and the columns simply are not there —
// no placeholder, nothing said (the metrics exit criterion's "absence is silent").
func TestUsageNilRemovesColumns(t *testing.T) {
	m := usageTable(t)
	m.SetUsage(nil)

	if header := headerLine(m.View()); strings.Contains(header, usageCPUColumn) {
		t.Fatalf("clearing the overlay should remove its columns, header %q", header)
	}
	if got := len(m.table.Columns); got != len(sampleTable().Columns) {
		t.Errorf("displayed columns = %d, want the server's %d", got, len(sampleTable().Columns))
	}
}

// TestUsageEmptyMapKeepsColumns is the distinction the nil/non-nil contract exists
// for: metrics-server is present for this kind but has scraped nothing yet. The
// columns belong on screen (blank), because availability is what decides them, not
// whether a sample happens to have landed.
func TestUsageEmptyMapKeepsColumns(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetSize(80, 8)
	m.SetUsage(map[kube.UsageKey]kube.Usage{})

	if header := headerLine(m.View()); !strings.Contains(header, usageCPUColumn) {
		t.Fatalf("an empty (but non-nil) sample set should keep the columns, header %q", header)
	}
}

// TestUsageSurvivesWatchDelta is why the samples are not row data: rows are replaced
// wholesale by every RESET (a watch reconnect re-lists), and an overlay written into
// the cells would vanish with them. Here the overlay outlives the reset and re-joins
// onto the new rows.
func TestUsageSurvivesWatchDelta(t *testing.T) {
	m := usageTable(t)

	m.ApplyEvent(kube.WatchEvent{
		Type:    kube.WatchReset,
		Columns: sampleTable().Columns,
		Rows: []kube.Row{
			{Cells: []any{"pod-b", "0/1", "10.0.0.2"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-b", UID: "b"}},
		},
	})

	if got := rowText(m.View(), 0); !strings.Contains(got, "100m") {
		t.Errorf("the overlay should survive a RESET and re-join the new rows, got %q", got)
	}
}

// TestSetTableClearsUsage proves the overlay does not outlive the resource it
// measures: a snapshot for a different kind must not carry the previous kind's
// samples (the same reason SetTable clears the filter and the sort).
func TestSetTableClearsUsage(t *testing.T) {
	m := usageTable(t)
	m.SetTable(sampleTable())

	if m.Usage() != nil {
		t.Fatal("SetTable should drop the metrics overlay")
	}
	if header := headerLine(m.View()); strings.Contains(header, usageCPUColumn) {
		t.Errorf("a new snapshot should not show the previous kind's metrics columns: %q", header)
	}
}

// TestUsageSortsNumerically is the one place the overlay needs help: its cells are
// formatted strings, and "100m" sorts below "20m" as text. Sorting reads the raw
// sample instead, so the CPU column orders by what was measured.
func TestUsageSortsNumerically(t *testing.T) {
	m := usageTable(t)
	// SortBy takes a *visible* position, and sampleTable hides one wide-only column,
	// so the overlay's columns are the last two visible ones.
	cpuCol := len(m.visible) - usageColumnCount

	m.SortBy(cpuCol)

	order := []string{}
	for _, r := range m.table.Rows {
		order = append(order, r.Object.Name)
	}
	// Ascending: the unmeasured row sorts below every measurement (-1), then 20m, then 100m.
	want := []string{"pod-c", "pod-a", "pod-b"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("ascending CPU order = %v, want %v (text order would put 100m first)", order, want)
		}
	}

	m.SortBy(cpuCol) // toggles to descending
	if got := m.table.Rows[0].Object.Name; got != "pod-b" {
		t.Errorf("descending CPU should start at the busiest pod, got %q", got)
	}
}

// TestUsageMemorySortsByBytes covers the second overlay column: "64Mi" vs "512Mi"
// happens to sort correctly as text, so the column is asserted on the byte value to
// pin that it is the raw sample being compared.
func TestUsageMemorySortsByBytes(t *testing.T) {
	m := usageTable(t)
	memCol := len(m.visible) - 1

	m.SortBy(memCol)

	if got := m.table.Rows[len(m.table.Rows)-1].Object.Name; got != "pod-a" {
		t.Errorf("ascending memory should end at the largest sample (pod-a, 512Mi), got %q", got)
	}
}

// TestUsageFilterMatchesUsageCells proves the overlay's cells are ordinary as far as
// the rest of the component is concerned: the filter's match scope is the visible
// columns, and the metrics columns are visible.
func TestUsageFilterMatchesUsageCells(t *testing.T) {
	m := usageTable(t)

	m.SetFilter("512Mi")

	if m.RowCount() != 1 {
		t.Fatalf("filtering on a usage value matched %d rows, want 1", m.RowCount())
	}
	if got := m.table.Rows[0].Object.Name; got != "pod-a" {
		t.Errorf("matched %q, want pod-a", got)
	}
}

// TestUsageLeavesAuthoritativeRowsAlone is the aliasing guard: the displayed rows
// carry two extra cells, and building them by appending in place would write into
// the watch's own row slice — corrupting the authoritative set the next derivation
// reads.
func TestUsageLeavesAuthoritativeRowsAlone(t *testing.T) {
	m := usageTable(t)

	for _, r := range m.full.Rows {
		if len(r.Cells) != len(sampleTable().Columns) {
			t.Fatalf("authoritative row %q grew to %d cells; the overlay must not touch full.Rows",
				r.Object.Name, len(r.Cells))
		}
	}
	if len(m.full.Columns) != len(sampleTable().Columns) {
		t.Errorf("authoritative columns = %d, want the server's %d", len(m.full.Columns), len(sampleTable().Columns))
	}
}

// TestUsageShortRowKeepsColumnAlignment covers the degraded row (principle 3): a row
// the server sent fewer cells for is padded to the column count before the usage
// cells are placed, so a usage value can never land under someone else's column.
func TestUsageShortRowKeepsColumnAlignment(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{
		Columns: sampleTable().Columns,
		Rows: []kube.Row{
			{Cells: []any{"pod-a"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-a", UID: "a"}},
		},
	})
	m.SetSize(80, 8)
	m.SetUsage(usageSamples())

	base := len(sampleTable().Columns)
	row := m.table.Rows[0]
	if len(row.Cells) != base+usageColumnCount {
		t.Fatalf("displayed row has %d cells, want %d", len(row.Cells), base+usageColumnCount)
	}
	if got := formatCell(row.Cells[base]); got != "20m" {
		t.Errorf("CPU cell = %q, want it at the CPU column's index", got)
	}
	if got := formatCell(row.Cells[base+1]); got != "512Mi" {
		t.Errorf("memory cell = %q, want it at the memory column's index", got)
	}
}

// TestUsageKeepsSelectionByUID proves a poll landing does not move the reader's
// cursor — the overlay refreshes every few seconds, under whatever row they are on.
func TestUsageKeepsSelectionByUID(t *testing.T) {
	m := newTestModel()
	m.SetTable(sampleTable())
	m.SetSize(80, 8)
	m.SelectRow(2)

	m.SetUsage(usageSamples())

	row, ok := m.SelectedRow()
	if !ok || row.Object.UID != "c" {
		t.Fatalf("selection = %+v, want the row the reader was on (pod-c)", row.Object)
	}
}

// TestUsageColumnsShownWithNoPriorityZeroColumns guards the interaction with the
// "server sent no priority-0 columns" fallback: the overlay's columns must not count
// as the priority-0 set, or a table of wide-only columns would show *only* CPU and
// MEMORY.
func TestUsageColumnsShownWithNoPriorityZeroColumns(t *testing.T) {
	m := newTestModel()
	m.SetTable(kube.Table{
		Columns: []kube.Column{{Name: "NAME", Priority: 1}, {Name: "IP", Priority: 1}},
		Rows: []kube.Row{
			{Cells: []any{"pod-a", "10.0.0.1"}, Object: kube.ObjectRef{Namespace: "default", Name: "pod-a", UID: "a"}},
		},
	})
	m.SetSize(80, 8)
	m.SetUsage(usageSamples())

	if got := len(m.visible); got != 4 {
		t.Fatalf("visible columns = %d, want the 2 fallback columns plus the 2 overlay ones", got)
	}
	if header := headerLine(m.View()); !strings.Contains(header, "NAME") || !strings.Contains(header, usageCPUColumn) {
		t.Errorf("header should show both the fallback and the overlay columns: %q", header)
	}
}

// TestFormatUsageUnits pins the two formatters: whole millicores and whole
// mebibytes, truncated — so a sub-mebibyte pod reads 0Mi, which is a measurement,
// distinct from the blank of no sample at all.
func TestFormatUsageUnits(t *testing.T) {
	if got := formatMillicores(0); got != "0m" {
		t.Errorf("formatMillicores(0) = %q, want 0m", got)
	}
	if got := formatMillicores(2500); got != "2500m" {
		t.Errorf("formatMillicores(2500) = %q, want 2500m (never scaled to cores)", got)
	}
	if got := formatMebibytes(1024 * 1024); got != "1Mi" {
		t.Errorf("formatMebibytes(1MiB) = %q, want 1Mi", got)
	}
	if got := formatMebibytes(1024*1024 - 1); got != "0Mi" {
		t.Errorf("formatMebibytes truncates: got %q, want 0Mi", got)
	}
}
