package keymap

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestParseChord(t *testing.T) {
	tests := []struct {
		token string
		want  chord
	}{
		{"j", "j"},
		{"G", "G"},
		{"/", "/"},
		{"?", "?"},
		{"up", "up"},
		{"UP", "up"}, // special names are case-insensitive
		{"enter", "enter"},
		{"ctrl+d", "ctrl+d"},
		{"ctrl+D", "ctrl+d"}, // ctrl-combo letter lowercased
		{"CTRL+d", "ctrl+d"}, // modifier case-insensitive
		{"alt+x", "alt+x"},
		{"f3", "f3"},
	}
	for _, tt := range tests {
		got, err := parseChord(tt.token)
		if err != nil {
			t.Errorf("parseChord(%q) error: %v", tt.token, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseChord(%q) = %q, want %q", tt.token, got, tt.want)
		}
	}
}

func TestParseChordErrors(t *testing.T) {
	for _, token := range []string{"", "shift+g", "hyper+x", "notakey", "ctrl+"} {
		if _, err := parseChord(token); err == nil {
			t.Errorf("parseChord(%q) = nil error, want error", token)
		}
	}
}

func TestChordFromKey(t *testing.T) {
	tests := []struct {
		name string
		key  tea.Key
		want chord
	}{
		{"lower letter", tea.Key{Code: 'j', Text: "j"}, "j"},
		{"shifted via ShiftedCode", tea.Key{Code: 'g', ShiftedCode: 'G', Mod: tea.ModShift}, "G"},
		{"shifted via Code", tea.Key{Code: 'G', Text: "G"}, "G"},
		{"ctrl combo", tea.Key{Code: 'd', Mod: tea.ModCtrl}, "ctrl+d"},
		{"ctrl combo upper", tea.Key{Code: 'D', Mod: tea.ModCtrl}, "ctrl+d"},
		{"special up", tea.Key{Code: tea.KeyUp}, "up"},
		{"special enter", tea.Key{Code: tea.KeyEnter}, "enter"},
		{"special esc", tea.Key{Code: tea.KeyEsc}, "esc"},
		{"space", tea.Key{Code: tea.KeySpace}, "space"},
		{"slash", tea.Key{Code: '/', Text: "/"}, "/"},
	}
	for _, tt := range tests {
		if got := chordFromKey(tt.key); got != tt.want {
			t.Errorf("%s: chordFromKey = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestChordRoundTrip proves a configured token and the live keypress it
// describes canonicalise to the same chord — the property resolution relies on.
func TestChordRoundTrip(t *testing.T) {
	cases := []struct {
		token string
		key   tea.Key
	}{
		{"j", tea.Key{Code: 'j', Text: "j"}},
		{"G", tea.Key{Code: 'g', ShiftedCode: 'G', Mod: tea.ModShift}},
		{"ctrl+d", tea.Key{Code: 'd', Mod: tea.ModCtrl}},
		{"up", tea.Key{Code: tea.KeyUp}},
		{"enter", tea.Key{Code: tea.KeyEnter}},
	}
	for _, c := range cases {
		parsed, err := parseChord(c.token)
		if err != nil {
			t.Fatalf("parseChord(%q): %v", c.token, err)
		}
		if live := chordFromKey(c.key); live != parsed {
			t.Errorf("token %q → %q but key → %q", c.token, parsed, live)
		}
	}
}
