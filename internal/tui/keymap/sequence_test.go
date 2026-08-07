package keymap

import "testing"

func TestParseSequence(t *testing.T) {
	tests := []struct {
		token string
		want  []chord
	}{
		{"j", []chord{"j"}},
		{"G", []chord{"G"}},
		{"up", []chord{"up"}}, // special name stays one chord, not u+p
		{"ctrl+d", []chord{"ctrl+d"}},
		{"f3", []chord{"f3"}},
		{"gg", []chord{"g", "g"}}, // vim concatenated form
		{"dd", []chord{"d", "d"}},
		{"g g", []chord{"g", "g"}}, // space-separated form
		{"ctrl+w k", []chord{"ctrl+w", "k"}},
		{" gg ", []chord{"g", "g"}}, // surrounding space tolerated
	}
	for _, tt := range tests {
		got, err := parseSequence(tt.token)
		if err != nil {
			t.Errorf("parseSequence(%q) error: %v", tt.token, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseSequence(%q) = %v, want %v", tt.token, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseSequence(%q)[%d] = %q, want %q", tt.token, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseSequenceErrors(t *testing.T) {
	// A modified token that fails is a real error, never re-read rune-by-rune.
	for _, token := range []string{"", "shift+g", "ctrl+", "g shift+x"} {
		if _, err := parseSequence(token); err == nil {
			t.Errorf("parseSequence(%q) = nil error, want error", token)
		}
	}
}

// TestSequenceStringRoundTrip proves seq.String() round-trips with parseSequence
// — the property Keys()/help generation relies on.
func TestSequenceStringRoundTrip(t *testing.T) {
	for _, token := range []string{"j", "G", "up", "ctrl+d", "gg", "ctrl+w k"} {
		s, err := parseSequence(token)
		if err != nil {
			t.Fatalf("parseSequence(%q): %v", token, err)
		}
		if got := s.String(); got != token {
			t.Errorf("seq(%q).String() = %q, want %q", token, got, token)
		}
	}
}
