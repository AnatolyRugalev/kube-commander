package safetext

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestLineRemovesWhatSteersTheTerminal is the package's whole claim, case by case:
// nothing that survives Line can move the cursor, erase a cell or change a color.
func TestLineRemovesWhatSteersTheTerminal(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain text is untouched":  {"aws sso login --profile prod", "aws sso login --profile prod"},
		"empty stays empty":        {"", ""},
		"sgr color is stripped":    {"\x1b[31mfatal\x1b[0m: expired", "fatal: expired"},
		"unreset sgr is stripped":  {"tail \x1b[1;38;5;9m", "tail "},
		"erase display is gone":    {"\x1b[2Joops", "oops"},
		"cursor home is gone":      {"\x1b[Hoops", "oops"},
		"osc title is gone":        {"\x1b]0;title\x07 failed", " failed"},
		"eight-bit csi is gone":    {"a\x9b31mb", "ab"},
		"carriage return dropped":  {"downloading...\rdone", "downloading...done"},
		"backspace dropped":        {"progress\b\b\bfailed", "progressfailed"},
		"bell dropped":             {"denied\a", "denied"},
		"nul dropped":              {"a\x00b", "ab"},
		"del dropped":              {"a\x7fb", "ab"},
		"newline dropped":          {"first\nsecond", "firstsecond"},
		"tab becomes one space":    {"ERROR\tSSO session expired", "ERROR SSO session expired"},
		"tab run becomes spaces":   {"a\t\tb", "a  b"},
		"non-ascii text is kept":   {"токен истёк", "токен истёк"},
		"wide runes are kept":      {"認証に失敗しました", "認証に失敗しました"},
		"combining marks are kept": {"échec", "échec"},
	} {
		if got := Line(tc.in); got != tc.want {
			t.Errorf("%s: Line(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

// TestLineOutputCostsWhatItMeasures is the property the width arithmetic downstream
// depends on and the one the control characters broke: every rune left in the result
// occupies the cells ansi.StringWidth says it does, so a caller that wraps, clips or
// pads by that measure gets a line the terminal draws at the same width.
func TestLineOutputCostsWhatItMeasures(t *testing.T) {
	for _, in := range []string{
		"\x1b[31mfatal\x1b[0m: token expired",
		"downloading...\rdone, but auth failed",
		"progress\b\b\b\b\bfailed",
		"\x1b[2J\x1b[Hcleared your screen",
		"ERROR\tSSO session expired\tplease login",
		"認証に失敗しました",
	} {
		got := Line(in)
		if strings.ContainsFunc(got, func(r rune) bool { return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) }) {
			t.Errorf("Line(%q) = %q still carries a control character", in, got)
		}
		if w, n := ansi.StringWidth(got), len([]rune(got)); w == 0 && n > 0 {
			t.Errorf("Line(%q) = %q measures zero cells for %d runes", in, got, n)
		}
		if ansi.Strip(got) != got {
			t.Errorf("Line(%q) = %q still carries an escape sequence", in, got)
		}
	}
}

// TestBlockKeepsLineStructure proves Block cleans inside each line and nowhere else:
// the newlines a multi-line surface lays out on are preserved, `\r\n` endings lose
// only the `\r`, and a blank line stays a blank line (the browse notice's own
// paragraph breaks are made of them).
func TestBlockKeepsLineStructure(t *testing.T) {
	in := "Cannot list Pod\n\x1b[31mIt said:\x1b[0m\r\n\n  aws\tsso: expired\rnow"
	want := "Cannot list Pod\nIt said:\n\n  aws sso: expirednow"
	if got := Block(in); got != want {
		t.Fatalf("Block(%q) = %q, want %q", in, got, want)
	}
	if got := Block(""); got != "" {
		t.Fatalf("Block(\"\") = %q, want \"\"", got)
	}
	if got := Block("a\nb\nc"); got != "a\nb\nc" {
		t.Fatalf("Block should leave clean text untouched, got %q", got)
	}
}

// TestLineIsIdempotent guards the property a defence applied at more than one seam
// needs: a surface that sanitizes text another surface already sanitized must not
// change it a second time.
func TestLineIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"\x1b[31mfatal\x1b[0m",
		"a\tb\rc",
		"plain",
		"\x1b]0;t\x07",
	} {
		once := Line(in)
		if twice := Line(once); twice != once {
			t.Errorf("Line is not idempotent on %q: %q then %q", in, once, twice)
		}
	}
}
