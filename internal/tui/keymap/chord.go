package keymap

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// chord is the canonical form of a single keypress: optional modifiers
// (ctrl/alt, lowercased, in a fixed order) joined by "+" to a base token. The
// base is either a single printable rune kept case-sensitive (so "G" is
// distinct from "g") or a lowercased special-key name ("up", "enter", …). Shift
// is never an explicit modifier — a shifted letter is its capital rune. Both
// parseChord (config/defaults) and chordFromKey (live keypress) normalise to
// this same form so a configured token and a real key compare equal.
type chord string

func (c chord) String() string { return string(c) }

// specialNames maps human special-key tokens to their tea key codes. The values
// double as the reverse map (code → name) via specialCodes below.
var specialNames = map[string]rune{
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"tab":       tea.KeyTab,
	"space":     tea.KeySpace,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdn":      tea.KeyPgDown,
	"backspace": tea.KeyBackspace,
	"delete":    tea.KeyDelete,
	"insert":    tea.KeyInsert,
	"f1":        tea.KeyF1,
	"f2":        tea.KeyF2,
	"f3":        tea.KeyF3,
	"f4":        tea.KeyF4,
	"f5":        tea.KeyF5,
	"f6":        tea.KeyF6,
	"f7":        tea.KeyF7,
	"f8":        tea.KeyF8,
	"f9":        tea.KeyF9,
	"f10":       tea.KeyF10,
	"f11":       tea.KeyF11,
	"f12":       tea.KeyF12,
}

var specialCodes = func() map[rune]string {
	m := make(map[rune]string, len(specialNames))
	for name, code := range specialNames {
		m[code] = name
	}
	return m
}()

// parseChord validates a human key token and returns its canonical chord.
// Accepted: "j", "G", "ctrl+d", "alt+x", "up", "enter", "f3". Modifiers are
// ctrl and alt only (case-insensitive); shift is rejected with a hint to use
// the capital letter, matching the config syntax in keybindings.md.
func parseChord(token string) (chord, error) {
	if token == "" {
		return "", fmt.Errorf("empty key binding")
	}
	parts := strings.Split(token, "+")
	base := parts[len(parts)-1]
	mods := parts[:len(parts)-1]

	if base == "" {
		return "", fmt.Errorf("key %q: empty base key", token)
	}

	ctrl, alt := false, false
	for _, m := range mods {
		switch strings.ToLower(m) {
		case "ctrl":
			ctrl = true
		case "alt":
			alt = true
		case "shift":
			return "", fmt.Errorf("key %q: shift is not a modifier; use the shifted character (e.g. G)", token)
		default:
			return "", fmt.Errorf("key %q: unknown modifier %q", token, m)
		}
	}

	// Base is either a known special name or a single printable rune.
	if _, ok := specialNames[strings.ToLower(base)]; ok {
		base = strings.ToLower(base)
	} else if utf8.RuneCountInString(base) == 1 {
		// Printable rune: kept as typed for case, but ctrl-combos are
		// canonicalised to the lowercase letter so "ctrl+D" == "ctrl+d".
		if ctrl {
			base = strings.ToLower(base)
		}
	} else {
		return "", fmt.Errorf("key %q: unknown key %q", token, base)
	}

	return assemble(ctrl, alt, base), nil
}

// chordFromKey derives the canonical chord of a live keypress.
func chordFromKey(k tea.Key) chord {
	ctrl := k.Mod&tea.ModCtrl != 0
	alt := k.Mod&tea.ModAlt != 0

	var base string
	switch {
	case specialCodes[k.Code] != "":
		base = specialCodes[k.Code]
	default:
		// Printable key. Prefer the shifted rune so shift+g reads as "G";
		// fall back to Text, then Code. Under ctrl the terminal reports no
		// text, so use Code and lowercase it.
		r := k.Code
		if !ctrl {
			if k.ShiftedCode != 0 {
				r = k.ShiftedCode
			} else if t := []rune(k.Text); len(t) == 1 {
				r = t[0]
			}
		} else {
			r = toLowerRune(r)
		}
		base = string(r)
	}
	return assemble(ctrl, alt, base)
}

// assemble builds the canonical chord string from its parts.
func assemble(ctrl, alt bool, base string) chord {
	var mods []string
	if ctrl {
		mods = append(mods, "ctrl")
	}
	if alt {
		mods = append(mods, "alt")
	}
	sort.Strings(mods) // fixed order: alt, ctrl
	if len(mods) == 0 {
		return chord(base)
	}
	return chord(strings.Join(mods, "+") + "+" + base)
}

func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
