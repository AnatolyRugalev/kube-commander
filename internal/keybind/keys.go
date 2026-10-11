package keybind

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neuroplastio/myelin/myelinkey"
)

// KeyOptions are how kubecom reads keys, declared to myelin with the schema:
// read as Ghostty does (any modifier order and case, `esc` or `escape`, `pgdn`
// or `page_down`), and a capital is Shift with its letter, as vim writes it — `G`
// is `shift+g` (myelin's D15).
var KeyOptions = myelinkey.Options{ShiftedCapitals: true}

// ParseSeq reads a sequence of keys, separated by spaces, as a keymap's bind
// reads them, with no alias in it: for keys kubecom meets outside a keymap, a
// trace's tokens and the tests'.
func ParseSeq(s string) ([]myelinkey.Key, error) {
	words := myelinkey.SplitSeq(s)
	if len(words) == 0 {
		return nil, errors.New("an empty key sequence")
	}
	out := make([]myelinkey.Key, len(words))
	for i, w := range words {
		k, err := myelinkey.ParseWith(w, KeyOptions)
		if err != nil {
			return nil, err
		}
		out[i] = k
	}
	return out, nil
}

// shortNames are the named keys kubecom shows by the short names default.kdl
// writes them with, which myelin reads too.
var shortNames = map[string]string{
	"escape": "esc", "page_up": "pgup", "page_down": "pgdn",
	"arrow_up": "up", "arrow_down": "down", "arrow_left": "left", "arrow_right": "right",
}

// Display writes a key as kubecom shows it — in a hint, the `?` overlay,
// keybindings.md, `kubecom keys` and a `--keylog` trace — the way default.kdl
// writes it: a capital for Shift with a letter alone (`G`), the short names of
// the keys vim users type (`esc`, `pgdn`, `up`), and otherwise myelin's own
// spelling (`ctrl+d`, `shift+tab`, `ctrl+shift+d`). KeyOptions read it back.
func Display(k myelinkey.Key) string {
	mods, value, char := myelinkey.Split(k)
	if char && mods == myelinkey.Shift {
		if up := strings.ToUpper(value); utf8.RuneCountInString(up) == 1 && isCapital(up) {
			return up
		}
	}
	if short, ok := shortNames[value]; ok && !char {
		return strings.TrimSuffix(string(k), value) + short
	}
	return string(k)
}

// DisplaySeq writes a sequence as Display writes its keys: `g g`.
func DisplaySeq(seq []myelinkey.Key) string {
	parts := make([]string, len(seq))
	for i, k := range seq {
		parts[i] = Display(k)
	}
	return strings.Join(parts, " ")
}

// isCapital reports whether a character is the capital of a small letter.
func isCapital(v string) bool {
	r, _ := utf8.DecodeRuneInString(v)
	small := unicode.ToLower(r)
	return small != r && unicode.ToUpper(small) == r
}
