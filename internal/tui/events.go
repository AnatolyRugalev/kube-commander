package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/table"
)

// renderEventsTable lays a server-printed events Table out as aligned text for
// the events viewer (STORY-06f): the header and every row padded to the widest
// cell in each column except the last, which flows — a long MESSAGE wraps onto
// the next line in the viewport rather than being clipped, the way `kubectl get
// events` lets its last column run. Column order and content come from the
// server (LAST SEEN, TYPE, REASON, OBJECT, MESSAGE), so the renderer needs no
// column knowledge of its own (D33). A table with nothing in it renders the
// empty notice, so an object with no events is a statement rather than a blank.
func renderEventsTable(t *kube.Table) string {
	if len(t.Columns) == 0 || len(t.Rows) == 0 {
		return "(no events)"
	}

	widths := make([]int, len(t.Columns))
	for i, c := range t.Columns {
		widths[i] = lipgloss.Width(c.Name)
	}
	for _, r := range t.Rows {
		for i := range t.Columns {
			if w := lipgloss.Width(eventCell(r.Cells, i)); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, s := range cells {
			if i > 0 {
				b.WriteString("   ") // the 3-cell column gap, kubectl-style
			}
			if i == len(cells)-1 {
				b.WriteString(s) // the last column flows (wraps, never clips)
				continue
			}
			b.WriteString(s)
			if pad := widths[i] - lipgloss.Width(s); pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteByte('\n')
	}

	header := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = c.Name
	}
	writeRow(header)
	for _, r := range t.Rows {
		row := make([]string, len(t.Columns))
		for i := range t.Columns {
			row[i] = eventCell(r.Cells, i)
		}
		writeRow(row)
	}
	return b.String()
}

// eventCell returns the display text of a table cell, or "" when the row has
// fewer cells than columns (a short/odd row degrades to blanks rather than
// panicking — principle 3), using the same server-cell formatting the browse
// table shows.
func eventCell(cells []any, i int) string {
	if i < 0 || i >= len(cells) {
		return ""
	}
	return table.FormatCell(cells[i])
}
