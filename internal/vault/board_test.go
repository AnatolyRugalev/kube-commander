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
//   - [x] **ID** <short title> — done YYYY-MM-DD (Dnn, …)
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

// TestBoardDoneIndexIsComplete guards what makes the Done list worth keeping at
// all: it is the *canonical* record of every finished item, and the `- [x]` line
// inside a Backlog line's own section is a working view of it.
//
// The distinction is not stylistic. When a line closes, its section collapses to
// a sentence — `_(none — M2 is done)_` — and the per-slice entries in it go with
// it; 207 of the 229 entries in the index today exist nowhere else on the board
// for exactly that reason. So an entry that never reaches the index is not
// duplicated-then-tidied, it is deleted the day its line is wrapped up, and the
// deletion looks like housekeeping to the leg that does it.
//
// That is what had happened by 2026-08-06: the index had not been appended to
// since 2026-08-02 and 26 finished items (AUTH-02…05b, PAL-01…05d, CRD-PIN-02…05,
// HINT-02…05, BOX-01…03) were one section-collapse away from vanishing. This test
// is the direction that matters — every finished entry above the heading has a
// line below it. The reverse is deliberately not checked: the index outliving its
// section is the whole point.
func TestBoardDoneIndexIsComplete(t *testing.T) {
	indexed := map[string]bool{}
	for _, e := range doneEntries(t) {
		if id := entryID(e.text); id != "" {
			indexed[id] = true
		}
	}
	for _, l := range workingArea(t) {
		if !strings.HasPrefix(l.text, "- [x] ") {
			continue
		}
		id := entryID(l.text)
		if id == "" {
			t.Errorf("board.md:%d is a finished entry with no **ID** to index it by\n  %s",
				l.line, truncate(l.text, 120))
			continue
		}
		if !indexed[id] {
			t.Errorf("board.md:%d — %s is done but is not in the %q index, so it is lost the "+
				"day its line closes and the section collapses (D225)\n"+
				"  add the same line, unchanged, under %q", l.line, id, doneHeading, doneHeading)
		}
	}
}

// TestBoardDoneIDsAreUnique guards the assumption every other check here makes
// without stating it: that an **ID** names exactly one leg.
//
// The id is the join key (D15) — a commit subject carries it, the journal entry
// is found by it, and the Done index is where a reader looks it up. Two entries
// sharing one id break all three at once: the id resolves to two unrelated pieces
// of work, and nothing on the board says which commit or which journal belongs to
// which. That is what had happened by 2026-08-12, when the install-docs leg took
// `DOC-02` three days after the README prose pass had already been indexed under
// it — neither leg could see the collision, because each was looking at a board
// where its own entry read fine.
//
// TestBoardDoneIndexIsComplete cannot catch this, and the reason is worth writing
// down so a later leg does not fold the two together: it collects ids into a
// `map[string]bool` and only ever asks whether a key is present, so a duplicate is
// indistinguishable from the entry it collides with — and worse, one entry's
// presence silently satisfies completeness for *both* working-area lines.
func TestBoardDoneIDsAreUnique(t *testing.T) {
	seen := map[string]int{}
	for _, e := range doneEntries(t) {
		id := entryID(e.text)
		if id == "" {
			continue // shape is TestBoardDoneEntriesAreOneLine's to complain about
		}
		if first, dup := seen[id]; dup {
			t.Errorf("board.md:%d — %s is already indexed at board.md:%d, but an **ID** is the "+
				"join key between the board, the journal entry and the commit subject (D15/D267)\n"+
				"  rename the side with fewer references — a suffix (`-01a`/`-01b`) if the two are "+
				"halves of one item, an unused id otherwise — and record the id its commits carry\n"+
				"  %s", e.line, id, first, truncate(e.text, 120))
			continue
		}
		seen[id] = e.line
	}
}

// deferralPhrases are the ways the board has actually deferred work in prose. It
// is a backstop and not the spec (D226 pt 2, under D224 pt 3): the reply to a
// deferral phrased some other way is to name its destination like every other
// one, never to add the phrase here so the check keeps passing.
var deferralPhrases = []string{
	"deliberately left",
	"its own small item",
	"is a candidate to",
	"raise it as",
}

// boldDestination is the two written-down endings D226 pt 2 allows: a filed item
// id, or the decision a constraint was moved into. Both must be **bold**, and
// that is the load-bearing part of the pattern. A bare `Dnn` would not do —
// almost every paragraph on this board cites one, so accepting an unbolded
// reference would have passed all four of the paragraphs this test exists for
// (the CRD-PIN one names D202, the PAL one D209, the AUTH one D216). Bolding is
// what makes the reference a *destination* rather than a citation.
//
// `**no**` and `**shows**` — both real, both in the paragraph this was written
// against — are emphasis, so an id needs a hyphen and a decision needs its
// digits. A bold sentence contains spaces and matches neither.
var (
	boldDestination = regexp.MustCompile(`\*\*(?:[A-Z][A-Z0-9]*(?:-[A-Za-z0-9]+)+|D\d{1,4})\*\*`)
	untilAsked      = "no item until asked"
	blankLine       = regexp.MustCompile(`^\s*$`)
)

// TestBoardDeferralsNameTheirDestination is the executable half of D226 pt 2.
//
// The failure it guards is one an Orient cannot see. Four closed lines had ended
// with "two things it deliberately left, either its own small item if a dogfood
// wants them", and none of the seven things named was a `- [ ]`. So the board
// truthfully showed five open items — four of them blocked — while a real,
// unblocked one sat in a paragraph, and three consecutive legs reported that
// nothing unblocked remained.
//
// The paragraph, not the line, is the unit: these sentences wrap across four or
// five lines at the board's margin, and the id that answers them is routinely on
// a different line from the phrase that raises them.
func TestBoardDeferralsNameTheirDestination(t *testing.T) {
	for _, p := range prosePargraphs(t) {
		joined := strings.Join(strings.Fields(p.text), " ")
		phrase := ""
		for _, d := range deferralPhrases {
			if strings.Contains(joined, d) {
				phrase = d
				break
			}
		}
		if phrase == "" {
			continue
		}
		if boldDestination.MatchString(joined) || strings.Contains(joined, untilAsked) {
			continue
		}
		t.Errorf("board.md:%d defers work (%q) without naming where it went (D226 pt 2)\n"+
			"  want one of: a filed **ID**, a bold **Dnn**, or the words %q\n  got:  %s",
			p.line, phrase, untilAsked, truncate(joined, 160))
	}
}

// prosePargraphs splits the working area into blank-line-separated blocks, each
// tagged with the line its first line sits on.
func prosePargraphs(t *testing.T) []boardLine {
	t.Helper()
	var out []boardLine
	cur := boardLine{}
	flush := func() {
		if strings.TrimSpace(cur.text) != "" {
			out = append(out, cur)
		}
		cur = boardLine{}
	}
	for _, l := range workingArea(t) {
		if blankLine.MatchString(l.text) {
			flush()
			continue
		}
		if cur.line == 0 {
			cur.line = l.line
		}
		cur.text += l.text + "\n"
	}
	flush()
	return out
}

// entryID is the `**ID**` a board entry opens with, or "" if it has none.
func entryID(text string) string {
	m := entryIDPattern.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

var entryIDPattern = regexp.MustCompile(`^- \[[ x]\] \*\*([A-Za-z0-9._-]+)\*\*`)

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

// workingArea is every line *above* the Done heading — In Progress, Blocked,
// Backlog and the per-line planning prose. The shape guards deliberately stop at
// the heading; only the index-completeness check reads this side.
func workingArea(t *testing.T) []boardLine {
	t.Helper()
	data, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read %s: %v", boardPath, err)
	}
	var out []boardLine
	for i, text := range strings.Split(string(data), "\n") {
		if text == doneHeading {
			return out
		}
		out = append(out, boardLine{line: i + 1, text: text})
	}
	t.Fatalf("%s has no %q heading", boardPath, doneHeading)
	return nil
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
