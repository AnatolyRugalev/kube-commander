package kube

import (
	"testing"
	"time"
)

func TestAgeColumnFindsThePrintedAgeColumn(t *testing.T) {
	for _, tc := range []struct {
		name string
		cols []Column
		want int
	}{
		{
			// The built-in printers' shape: `{Name: "Age", Type: "string"}`.
			name: "builtin",
			cols: []Column{{Name: "Name", Type: "string"}, {Name: "Age", Type: "string"}},
			want: 1,
		},
		{
			// A CRD's additionalPrinterColumns over .metadata.creationTimestamp.
			name: "crd date column",
			cols: []Column{{Name: "Name", Type: "string"}, {Name: "AGE", Type: "date"}},
			want: 1,
		},
		{
			// No type at all (a server that omitted it) still matches on the name.
			name: "untyped",
			cols: []Column{{Name: "age"}},
			want: 0,
		},
		{
			// A numeric column named "Age" is some resource's own field, not a
			// printed duration; overwriting it would be a lie.
			name: "numeric age is not an age column",
			cols: []Column{{Name: "Name", Type: "string"}, {Name: "Age", Type: "integer"}},
			want: -1,
		},
		{
			name: "absent",
			cols: []Column{{Name: "Name", Type: "string"}, {Name: "Status", Type: "string"}},
			want: -1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AgeColumn(tc.cols)
			if tc.want < 0 {
				if ok {
					t.Fatalf("AgeColumn = %d, true; want not found", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("AgeColumn = %d, %v; want %d, true", got, ok, tc.want)
			}
		})
	}
}

func TestHumanAgeMatchesTheServerPrinter(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		since time.Duration
		want  string
	}{
		{5 * time.Second, "5s"},
		{90 * time.Second, "90s"},
		{5*time.Minute + 30*time.Second, "5m30s"},
		{45 * time.Minute, "45m"},
		{26 * time.Hour, "26h"},
		{5 * 24 * time.Hour, "5d"},
		// A creation timestamp slightly in the future (clock skew between the
		// reader's machine and the API server) clamps rather than going negative.
		{-time.Second, "0s"},
	} {
		if got := HumanAge(now.Add(-tc.since), now); got != tc.want {
			t.Errorf("HumanAge(-%s) = %q, want %q", tc.since, got, tc.want)
		}
	}
}

func TestRefreshAgeRecomputesFromCreationTimestamp(t *testing.T) {
	created := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	tbl := Table{
		Columns: []Column{{Name: "Name", Type: "string"}, {Name: "Age", Type: "string"}},
		Rows: []Row{{
			Cells:   []any{"nginx", "10s"}, // what the server printed, ten seconds in
			Object:  ObjectRef{Name: "nginx", UID: "uid-1"},
			Created: created,
		}},
	}

	// An hour later the pane has had no delta for this row, so the cell still
	// says "10s" — this is the reported bug — until the age is re-derived.
	if !tbl.RefreshAge(created.Add(time.Hour)) {
		t.Fatal("RefreshAge reported no change, want the stale cell replaced")
	}
	if got, want := tbl.Rows[0].Cells[1], "60m"; got != want {
		t.Fatalf("age cell = %v, want %q", got, want)
	}

	// Re-running against the same instant is a no-op: nothing changed, and the
	// caller relies on that to skip the re-filter/re-sort/re-measure behind it.
	if tbl.RefreshAge(created.Add(time.Hour)) {
		t.Error("RefreshAge reported a change on an unchanged clock")
	}

	// The name column is untouched — only the age column is re-derived.
	if got, want := tbl.Rows[0].Cells[0], "nginx"; got != want {
		t.Errorf("name cell = %v, want %q", got, want)
	}
}

func TestRefreshAgeLeavesRowsItCannotDate(t *testing.T) {
	// A row that arrived without parsable object metadata (principle 3's degraded
	// row) has a zero Created. Computing from it would print "55y"; keeping the
	// server's stale string is strictly better.
	tbl := Table{
		Columns: []Column{{Name: "Name", Type: "string"}, {Name: "Age", Type: "string"}},
		Rows: []Row{
			{Cells: []any{"no-meta", "3d"}},
			{Cells: []any{"short-row"}, Created: time.Unix(0, 0)},
		},
	}
	if tbl.RefreshAge(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)) {
		t.Error("RefreshAge reported a change, want the undatable rows left alone")
	}
	if got, want := tbl.Rows[0].Cells[1], "3d"; got != want {
		t.Errorf("age cell = %v, want the server's %q", got, want)
	}
	if got, want := len(tbl.Rows[1].Cells), 1; got != want {
		t.Errorf("short row grew to %d cells, want %d", got, want)
	}
}

func TestRefreshAgeWithoutAnAgeColumn(t *testing.T) {
	tbl := Table{
		Columns: []Column{{Name: "Name", Type: "string"}, {Name: "Data", Type: "integer"}},
		Rows:    []Row{{Cells: []any{"cm-a", int64(2)}, Created: time.Unix(1, 0)}},
	}
	if tbl.RefreshAge(time.Now()) {
		t.Error("RefreshAge reported a change on a table with no age column")
	}
	if got, want := tbl.Rows[0].Cells[1], int64(2); got != want {
		t.Errorf("cell = %v, want %v untouched", got, want)
	}
}

func TestDecodeTableCarriesCreationTimestamp(t *testing.T) {
	const j = `{
      "kind":"Table","apiVersion":"meta.k8s.io/v1",
      "columnDefinitions":[{"name":"Name","type":"string"},{"name":"Age","type":"string"}],
      "rows":[
        {"cells":["nginx","10s"],
         "object":{"kind":"PartialObjectMetadata","apiVersion":"meta.k8s.io/v1",
                   "metadata":{"name":"nginx","namespace":"web","uid":"uid-1",
                               "creationTimestamp":"2026-08-06T12:00:00Z"}}}
      ]
    }`
	tbl, err := decodeTable([]byte(j))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	want := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if got := tbl.Rows[0].Created; !got.Equal(want) {
		t.Fatalf("Created = %v, want %v", got, want)
	}
}
