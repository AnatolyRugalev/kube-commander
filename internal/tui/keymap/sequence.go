package keymap

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// seq is a multi-key binding: an ordered run of one or more chords that must be
// pressed in sequence to trigger an action (vim's `gg`). A length-1 seq is an
// ordinary single-key binding, so single chords and sequences share one model.
type seq []chord

// key is the canonical map key for a sequence. Chords are NUL-joined so distinct
// sequences never collide (a chord can't contain NUL).
func (s seq) key() string {
	parts := make([]string, len(s))
	for i, c := range s {
		parts[i] = string(c)
	}
	return strings.Join(parts, "\x00")
}

// String renders a sequence back to its human token, round-tripping with
// parseSequence: a run of bare single-rune chords concatenates ("gg"), anything
// with a modifier or special-name chord space-joins ("ctrl+w k").
func (s seq) String() string {
	parts := make([]string, len(s))
	concat := true
	for i, c := range s {
		parts[i] = string(c)
		if strings.Contains(string(c), "+") || utf8.RuneCountInString(string(c)) != 1 {
			concat = false
		}
	}
	if concat {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, " ")
}

// parseSequence turns a human key token into a canonical sequence. It accepts a
// lone chord ("j", "G", "up", "ctrl+d", "/", "f3"), a space-separated multi-chord
// form ("g g", "ctrl+w k"), or the vim concatenated single-rune form ("gg", "dd").
// A modified token that fails to parse (e.g. "shift+g", "ctrl+") is a real error,
// never silently re-read as a run of single-rune chords.
func parseSequence(token string) (seq, error) {
	fields := strings.Fields(token)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty key binding")
	}
	// Space-separated multi-chord form: each field is a full chord.
	if len(fields) > 1 {
		s := make(seq, 0, len(fields))
		for _, f := range fields {
			c, err := parseChord(f)
			if err != nil {
				return nil, err
			}
			s = append(s, c)
		}
		return s, nil
	}
	tok := fields[0]
	// A single lone chord.
	if c, err := parseChord(tok); err == nil {
		return seq{c}, nil
	} else if strings.Contains(tok, "+") {
		return nil, err
	}
	// Concatenated single-rune sequence (vim "gg").
	s := make(seq, 0, utf8.RuneCountInString(tok))
	for _, r := range tok {
		c, err := parseChord(string(r))
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", tok, err)
		}
		s = append(s, c)
	}
	return s, nil
}
