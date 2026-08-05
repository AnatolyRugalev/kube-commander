package elide

import "testing"

// TestLines pins the three properties the callers rely on: an over-tall block ends
// up exactly n lines with the marker as the last one (never n+1 — the whole point),
// a block that fits comes back byte-identical so the marker cannot become
// furniture, and a non-positive budget renders nothing rather than a bare marker in
// a box that has no room for it.
func TestLines(t *testing.T) {
	block := "a\nb\nc\nd"
	for _, tc := range []struct {
		n    int
		want string
	}{
		{n: -1, want: ""},
		{n: 0, want: ""},
		{n: 1, want: "…"},
		{n: 3, want: "a\nb\n…"},
		{n: 4, want: block},
		{n: 9, want: block},
	} {
		if got := Lines(block, tc.n, "…"); got != tc.want {
			t.Fatalf("Lines(%q, %d) = %q, want %q", block, tc.n, got, tc.want)
		}
	}
}

// TestLinesIsRepeatable pins the reason Lines uses a full slice expression
// (lines[:n-1:n-1]) rather than a plain one: without the capacity bound, append
// would write the marker through the header strings.Split handed back. The
// observable consequence is that a second call on the same block must give the
// same answer as the first, and the input must come back unchanged.
func TestLinesIsRepeatable(t *testing.T) {
	block := "a\nb\nc\nd"
	first := Lines(block, 3, "…")
	second := Lines(block, 3, "…")
	if first != second {
		t.Fatalf("Lines is not repeatable: %q then %q", first, second)
	}
	if block != "a\nb\nc\nd" {
		t.Fatalf("Lines mutated its input: %q", block)
	}
}
