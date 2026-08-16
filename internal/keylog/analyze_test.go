package keylog

import (
	"strings"
	"testing"
	"time"
)

// tick advances a synthetic clock by d, so a test can lay out a trace's timing
// without a wall clock.
var tick = 0 * time.Second

func at(d time.Duration) time.Time {
	tick += d
	return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC).Add(tick)
}

// fakeResolver answers Resolve for the tokens a test hands it. It is the keymap
// in miniature: "g" "g" is nav.top, anything else resolves to nothing, so a test
// can exercise sequence-completion without importing the TUI.
type fakeResolver struct{}

func (fakeResolver) Resolve(tokens ...string) (string, bool) {
	if len(tokens) == 2 && tokens[0] == "g" && tokens[1] == "g" {
		return "nav.top", true
	}
	return "", false
}

// TestAnalyzeDeadEndsAreRanked is the headline output: presses that resolved to
// nothing, most-frequent first, with the surface they were pressed on — the same
// key on a different surface is a different dead end, because the surface is what
// made it one.
func TestAnalyzeDeadEndsAreRanked(t *testing.T) {
	recs := []Record{
		{Key: "x", Mode: "browse", Time: at(0)},
		{Key: "x", Mode: "browse", Time: at(1 * time.Second)},
		{Key: "z", Mode: "browse", Time: at(1 * time.Second)},
		{Key: "x", Mode: "modal-confirm", Time: at(1 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)

	if len(rep.DeadEnds) != 3 {
		t.Fatalf("want 3 distinct dead ends, got %d: %+v", len(rep.DeadEnds), rep.DeadEnds)
	}
	if first := rep.DeadEnds[0]; first.Key != "x" || first.Mode != "browse" || first.Count != 2 {
		t.Errorf("top dead end = %+v, want x/browse with count 2", first)
	}
	// The count-1 ties sort by key: x before z.
	if mid := rep.DeadEnds[1]; mid.Key != "x" || mid.Mode != "modal-confirm" || mid.Count != 1 {
		t.Errorf("middle dead end = %+v, want x/modal-confirm with count 1", mid)
	}
}

// TestAnalyzeTextAndPendingAreNotDeadEnds keeps the headline signal clean: typed
// characters and sequence-in-progress presses are expected empties, not reaches.
func TestAnalyzeTextAndPendingAreNotDeadEnds(t *testing.T) {
	recs := []Record{
		{Key: "w", Mode: "filter", Text: true, Time: at(0)},
		{Key: "g", Mode: "browse", Pending: true, Time: at(1 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.DeadEnds) != 0 {
		t.Errorf("want no dead ends, got %+v", rep.DeadEnds)
	}
	if len(rep.TextPresses) != 1 {
		t.Errorf("want 1 text press, got %+v", rep.TextPresses)
	}
}

// TestAnalyzeTextPressesReportedSeparately is STORY-06e's headline: a press on a
// text surface that resolved to nothing — the picker `j`s of the S01 walk — must
// be a finding, not silently skipped. It is reported in its own section (it may
// be ordinary typing, so it is not a dead end), ranked like dead ends.
func TestAnalyzeTextPressesReportedSeparately(t *testing.T) {
	recs := []Record{
		{Key: "j", Mode: "picker", Text: true, Time: at(0)},
		{Key: "j", Mode: "picker", Text: true, Time: at(1 * time.Second)},
		{Key: "j", Mode: "picker", Text: true, Time: at(1 * time.Second)},
		{Key: "j", Mode: "picker", Text: true, Time: at(1 * time.Second)},
		{Key: "s", Mode: "search", Text: true, Time: at(1 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.DeadEnds) != 0 {
		t.Errorf("text presses must not read as dead ends, got %+v", rep.DeadEnds)
	}
	if len(rep.TextPresses) != 2 {
		t.Fatalf("want 2 distinct text presses, got %d: %+v", len(rep.TextPresses), rep.TextPresses)
	}
	// The four picker `j`s rank first — the S01 finding, now visible.
	if first := rep.TextPresses[0]; first.Key != "j" || first.Mode != "picker" || first.Count != 4 {
		t.Errorf("top text press = %+v, want j/picker with count 4", first)
	}
	if second := rep.TextPresses[1]; second.Key != "s" || second.Mode != "search" || second.Count != 1 {
		t.Errorf("second text press = %+v, want s/search with count 1", second)
	}
}

// TestAnalyzeDeadEndsAndTextPressesAreRankedTogether keeps the two sections from
// merging: a genuine dead end and a text-surface press coexist, each in its own
// list.
func TestAnalyzeDeadEndsAndTextPressesAreRankedTogether(t *testing.T) {
	recs := []Record{
		{Key: "x", Mode: "browse", Time: at(0)},
		{Key: "j", Mode: "picker", Text: true, Time: at(1 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.DeadEnds) != 1 || rep.DeadEnds[0].Key != "x" {
		t.Errorf("dead ends = %+v, want [x/browse]", rep.DeadEnds)
	}
	if len(rep.TextPresses) != 1 || rep.TextPresses[0].Key != "j" {
		t.Errorf("text presses = %+v, want [j/picker]", rep.TextPresses)
	}
}

// TestAnalyzeActionCounts tallies the resolved actions, most-frequent first.
func TestAnalyzeActionCounts(t *testing.T) {
	recs := []Record{
		{Key: "j", Mode: "browse", Action: "nav.down", Time: at(0)},
		{Key: "j", Mode: "browse", Action: "nav.down", Time: at(1 * time.Second)},
		{Key: "L", Mode: "browse", Action: "res.logs", Time: at(1 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.Actions) != 2 {
		t.Fatalf("want 2 actions, got %d: %+v", len(rep.Actions), rep.Actions)
	}
	if rep.Actions[0].Action != "nav.down" || rep.Actions[0].Count != 2 {
		t.Errorf("top action = %+v, want nav.down × 2", rep.Actions[0])
	}
}

// TestAnalyzePausesFindsTheLongestGaps is the "where did they stop to think"
// output: the gaps between presses, biggest first, each tied to the press that
// finally came.
func TestAnalyzePausesFindsTheLongestGaps(t *testing.T) {
	recs := []Record{
		{Key: "j", Mode: "browse", Action: "nav.down", Time: at(0)},
		{Key: "j", Mode: "browse", Action: "nav.down", Time: at(1 * time.Second)},
		{Key: "L", Mode: "browse", Action: "res.logs", Time: at(20 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.Pauses) != 2 {
		t.Fatalf("want 2 pauses, got %d: %+v", len(rep.Pauses), rep.Pauses)
	}
	// Longest first.
	if got := rep.Pauses[0]; got.Gap != 20*time.Second || got.NextKey != "L" || got.NextMode != "browse" {
		t.Errorf("longest pause = %+v, want 20s before L/browse", got)
	}
}

// TestAnalyzeCompletedSequenceIsNotReported is the case that must *not* show up:
// `g g` resolves to nav.top, so the run of pending presses completed and is not an
// abandoned sequence.
func TestAnalyzeCompletedSequenceIsNotReported(t *testing.T) {
	recs := []Record{
		{Key: "g", Mode: "browse", Pending: true, Time: at(0)},
		{Key: "g", Mode: "browse", Action: "nav.top", Time: at(100 * time.Millisecond)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.Sequences) != 0 {
		t.Errorf("a completed `gg` must not be an abandoned sequence, got %+v", rep.Sequences)
	}
}

// TestAnalyzeAbandonedSequenceIsReported is the finding: `g` was pressed, the
// walker never pressed the second `g`, and instead pressed something that does not
// extend the run — the sequence died.
func TestAnalyzeAbandonedSequenceIsReported(t *testing.T) {
	recs := []Record{
		{Key: "g", Mode: "browse", Pending: true, Time: at(0)},
		{Key: "x", Mode: "browse", Time: at(2 * time.Second)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.Sequences) != 1 {
		t.Fatalf("want 1 abandoned sequence, got %d: %+v", len(rep.Sequences), rep.Sequences)
	}
	seq := rep.Sequences[0]
	if len(seq.Keys) != 1 || seq.Keys[0] != "g" {
		t.Errorf("abandoned keys = %v, want [g]", seq.Keys)
	}
	if seq.AfterKey != "x" || seq.AfterMode != "browse" {
		t.Errorf("abandoned then = %q/%q, want x/browse", seq.AfterKey, seq.AfterMode)
	}
}

// TestAnalyzeSequenceTimedOutSplitsRuns: a gap longer than the keymap timeout
// between two pending presses means the first run died to the timer before the
// second press — the presses belong to two abandoned sequences, not one completed
// (or one long) run.
func TestAnalyzeSequenceTimedOutSplitsRuns(t *testing.T) {
	recs := []Record{
		{Key: "g", Mode: "browse", Pending: true, Time: at(0)},
		{Key: "g", Mode: "browse", Pending: true, Time: at(2 * time.Second)},
	}
	// seqTimeout is 500ms: the 2s gap kills the first run before the second `g`.
	rep := Analyze(recs, fakeResolver{}, 500*time.Millisecond)
	if len(rep.Sequences) != 2 {
		t.Errorf("a 2s gap past a 500ms timeout must split into 2 abandoned runs, got %d: %+v",
			len(rep.Sequences), rep.Sequences)
	}
}

// TestAnalyzeEndOfTracePendingIsReported: the trace ends with a pending press —
// the walker started a sequence and stopped (quit, or the file was cut).
func TestAnalyzeEndOfTracePendingIsReported(t *testing.T) {
	recs := []Record{
		{Key: "g", Mode: "browse", Pending: true, Time: at(0)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if len(rep.Sequences) != 1 {
		t.Fatalf("want 1 abandoned sequence, got %d", len(rep.Sequences))
	}
	if rep.Sequences[0].AfterKey != "" {
		t.Errorf("end-of-trace sequence AfterKey = %q, want empty", rep.Sequences[0].AfterKey)
	}
}

// TestAnalyzeNilResolverReportsEverythingPending is the safe fallback: without a
// keymap, the analyser cannot tell a completed chord from an abandoned one, so it
// reports every pending run. Over-reporting beats silently hiding a finding.
func TestAnalyzeNilResolverReportsEverythingPending(t *testing.T) {
	recs := []Record{
		{Key: "g", Mode: "browse", Pending: true, Time: at(0)},
		{Key: "g", Mode: "browse", Action: "nav.top", Time: at(100 * time.Millisecond)},
	}
	rep := Analyze(recs, nil, time.Second)
	if len(rep.Sequences) != 1 {
		t.Errorf("with no resolver, a completed-looking `gg` must still be reported, got %d", len(rep.Sequences))
	}
}

// TestAnalyzeElapsedMeasuresTheWholeRun: the story's wall time, first to last
// press.
func TestAnalyzeElapsedMeasuresTheWholeRun(t *testing.T) {
	recs := []Record{
		{Key: "j", Mode: "browse", Action: "nav.down", Time: at(0)},
		{Key: "q", Mode: "browse", Action: "app.quit", Time: at(3 * time.Minute)},
	}
	rep := Analyze(recs, fakeResolver{}, time.Second)
	if rep.Elapsed != 3*time.Minute {
		t.Errorf("elapsed = %v, want 3m", rep.Elapsed)
	}
	if rep.Records != 2 {
		t.Errorf("records = %d, want 2", rep.Records)
	}
}

// TestReadFromRoundTrips: what the writer wrote, ReadFrom reads back.
func TestReadFromRoundTrips(t *testing.T) {
	const trace = `{"t":"2026-08-15T12:00:00.000Z","key":"j","mode":"browse","action":"nav.down"}
{"t":"2026-08-15T12:00:01.000Z","key":"x","mode":"browse"}

`
	recs, err := ReadFrom(strings.NewReader(trace))
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d: %+v", len(recs), recs)
	}
	if recs[0].Key != "j" || recs[0].Action != "nav.down" {
		t.Errorf("record 0 = %+v", recs[0])
	}
	if recs[1].Key != "x" || recs[1].Action != "" {
		t.Errorf("record 1 = %+v", recs[1])
	}
}

// TestReadFromRejectsGarbage keeps a corrupted trace from being trusted: a line
// that is not a record is an error, not silently skipped.
func TestReadFromRejectsGarbage(t *testing.T) {
	if _, err := ReadFrom(strings.NewReader("not-json\n")); err == nil {
		t.Error("a non-JSON line must fail ReadFrom")
	}
}
