package vault

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// boardPath is the live task board, read relative to this package (the pattern
// internal/version uses for `.goreleaser.yml`).
var boardPath = filepath.Join("..", "..", "vault", "tasks", "board.md")

const (
	// doneHeading opens the section these guards cover. Everything above it —
	// In Progress, Blocked, Backlog and the per-line planning prose — is a live
	// working area with its own shapes, and is deliberately not checked here.
	doneHeading = "## Done"

	// doneEntryMaxRunes is a backstop, not a target. D102's own example is about
	// seventy runes and the median entry is around a hundred and fifty; what this
	// number catches is the failure that has actually happened twice — a leg
	// pasting its journal paragraph onto the board, which produced entries of 400
	// to 712 runes. Runes rather than bytes: the entries are full of em-dashes,
	// arrows and `…`, so a byte budget would quietly be a third smaller than it
	// reads.
	doneEntryMaxRunes = 400
)

// doneEntry is the shape D102 fixes for a completed item:
//
//	- [x] **ID** <short title> — done YYYY-MM-DD (Dnn, …)
//
// The trailing parenthetical is optional (a few legs are their own reference),
// and it must not itself contain parentheses, so a wrapped-up sentence cannot
// pass as the decision list.
var doneEntry = regexp.MustCompile(`^- \[x\] \*\*[A-Za-z0-9._-]+\*\* .+ — done \d{4}-\d{2}-\d{2}( \([^()]*\))?$`)

// TestBoardDoneEntriesAreOneLine is the executable half of D102.
//
// The rule it enforces was written down after the Done list had swollen from
// ~17KB to ~50KB (D102, 2026-07-22), and the list was collapsed by hand then —
// and had swollen again to 94KB with 52 paragraph entries by 2026-08-05, which
// is what BOARD-01 collapsed. Twice is enough to say the convention does not
// hold on its own: the board is read on every Orient, so the cost of the drift
// is paid by every leg, while the drift itself is invisible to the leg that
// causes it (its own entry reads fine).
func TestBoardDoneEntriesAreOneLine(t *testing.T) {
	entries := doneEntries(t)
	if len(entries) == 0 {
		t.Fatalf("no `- [x]` entries found under %q in %s — has the section been renamed?",
			doneHeading, boardPath)
	}
	for _, e := range entries {
		if !doneEntry.MatchString(e.text) {
			t.Errorf("board.md:%d is not a D102 Done entry\n"+
				"  want: - [x] **ID** <short title> — done YYYY-MM-DD (Dnn, …)\n"+
				"  got:  %s", e.line, truncate(e.text, 120))
			continue
		}
		if n := utf8.RuneCountInString(e.text); n > doneEntryMaxRunes {
			t.Errorf("board.md:%d is %d runes, over the %d-rune cap — the detail belongs in "+
				"the journal entry for that leg, not on the board (D67/D102)\n  %s",
				e.line, n, doneEntryMaxRunes, truncate(e.text, 120))
		}
	}
}

// TestBoardDoneSectionHoldsNothingButEntries guards the other half of the rule:
// "no continuation lines".
//
// It is the one that catches the drift *early*. A leg that has more to say than
// fits reaches for a second line long before it writes a 400-rune first one, so
// the rune cap is the backstop and this is the tripwire. It also keeps the Done
// list a flat index — anything that wants a heading, a note or a paragraph is
// describing state, and state lives in the Backlog section or in the milestone.
func TestBoardDoneSectionHoldsNothingButEntries(t *testing.T) {
	for _, l := range doneSection(t) {
		if strings.TrimSpace(l.text) == "" || strings.HasPrefix(l.text, "- [x] ") {
			continue
		}
		t.Errorf("board.md:%d is neither blank nor a Done entry — the Done list is a flat "+
			"index, and prose about a finished leg belongs in its journal entry (D67/D102)\n  %s",
			l.line, truncate(l.text, 120))
	}
}

// boardLine is one line of the board with its 1-based number, so a failure names
// a place the reader can open.
type boardLine struct {
	line int
	text string
}

// doneSection is every line below the Done heading (to the next `## ` heading, if
// a later leg ever adds one).
func doneSection(t *testing.T) []boardLine {
	t.Helper()
	data, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read %s: %v", boardPath, err)
	}
	var out []boardLine
	inSection := false
	for i, text := range strings.Split(string(data), "\n") {
		switch {
		case text == doneHeading:
			inSection = true
		case inSection && strings.HasPrefix(text, "## "):
			return out
		case inSection:
			out = append(out, boardLine{line: i + 1, text: text})
		}
	}
	if !inSection {
		t.Fatalf("%s has no %q heading", boardPath, doneHeading)
	}
	return out
}

// doneEntries is the section's `- [x]` lines.
func doneEntries(t *testing.T) []boardLine {
	t.Helper()
	var out []boardLine
	for _, l := range doneSection(t) {
		if strings.HasPrefix(l.text, "- [x]") {
			out = append(out, l)
		}
	}
	return out
}

// truncate keeps a failure message readable when the offending line is the very
// thing being complained about.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
