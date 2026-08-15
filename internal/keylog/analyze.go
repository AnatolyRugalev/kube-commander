package keylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// This file is STORY-03: the read side of a keystroke trace. The writer (STORY-02)
// records one JSONL object per press; Analyze turns a trace into the four things
// a UX pass asks — the dead ends, the actions, the pacing, the abandoned
// sequences — so a story walked against the fixture (STORY-01) can be read back
// as findings rather than remembered (D268 pt 2).

// maxPauses is how many of the longest pauses Analyze reports. Pacing is the
// one question the trace answers implicitly — every key carries a timestamp, so
// the gaps between them are where the walker stopped to think — and the top few
// gaps are the signal. Everything below the headline pauses is noise a reviewer
// can see in the raw trace if they want it.
const maxPauses = 5

// Report is the shape of an analysis: the four findings, in the order a UX pass
// reads them.
type Report struct {
	// Records is how many keypresses the trace held.
	Records int
	// Elapsed is the wall time from the first to the last press.
	Elapsed time.Duration
	// DeadEnds are presses that resolved to no action, ranked by frequency. Each
	// carries the surface it was pressed on, because the same key means different
	// things in different modes.
	DeadEnds []DeadEnd
	// Actions are the resolved actions, ranked by frequency.
	Actions []ActionCount
	// Pauses are the longest gaps between consecutive presses — the places the
	// walker stopped to think.
	Pauses []Pause
	// Sequences are multi-key sequences the walker started but abandoned before
	// a completed binding (or a timeout) resolved them.
	Sequences []AbandonedSequence
}

// DeadEnd is one distinct unresolved press (a key/mode pair), and how often it
// was hit.
type DeadEnd struct {
	Key   string
	Mode  string
	Count int
}

// ActionCount is one resolved action, and how often it ran.
type ActionCount struct {
	Action string
	Count  int
}

// Pause is one gap between consecutive presses.
type Pause struct {
	// Gap is how long the walker waited.
	Gap time.Duration
	// Next is the press that finally came — the key and the surface it was
	// routed to, so the pause is attachable to a moment.
	NextKey  string
	NextMode string
}

// AbandonedSequence is a run of pending presses that never resolved: the walker
// reached for a multi-key binding and stopped before completing it.
type AbandonedSequence struct {
	// Keys are the presses of the abandoned run, in order ("g", "g").
	Keys []string
	// Mode is the surface the run was pressed on (browse — the only surface
	// that resolves sequences).
	Mode string
	// First is when the run began.
	First time.Time
	// Last is when the run ended (the pending press that was never answered).
	Last time.Time
	// After is what the walker did instead, if anything: the key and mode of
	// the next press, or "" at end of trace.
	AfterKey  string
	AfterMode string
}

// Resolver resolves a run of key tokens to the action a completed binding would
// produce. The command supplies the keymap's Resolve; it is an interface here
// (rather than a *keymap.Keymap) so this package stays free of the TUI.
type Resolver interface {
	Resolve(tokens ...string) (action string, ok bool)
}

// Read loads a trace file into records, in the order they were pressed. A trace
// is JSONL and may hold several appended runs (the writer appends, so a story
// walked twice is one file); runs are separated by the timestamps, not by Read.
func Read(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("keylog: opening trace %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	records, err := ReadFrom(f)
	return records, err
}

// ReadFrom decodes a JSONL stream of records. A trailing newline and blank lines
// are fine; a line that is not a valid record is an error, because a truncated or
// corrupted trace cannot be trusted to say what the walker actually pressed.
func ReadFrom(r io.Reader) ([]Record, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var records []Record
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("keylog: decoding trace line %q: %w", line, err)
		}
		records = append(records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("keylog: reading trace: %w", err)
	}
	return records, nil
}

// Analyze reduces a trace to its Report. resolve may be nil: sequence-completion
// detection then cannot tell a finished chord from an abandoned one, so every
// pending run is reported as abandoned — the safe reading when no keymap is
// available. seqTimeout is the keymap's SequenceTimeout: a gap longer than it
// between pending presses means the first run died to the timer, so the presses
// belong to separate sequences.
func Analyze(records []Record, resolve Resolver, seqTimeout time.Duration) Report {
	rep := Report{Records: len(records)}
	if len(records) == 0 {
		return rep
	}
	rep.Elapsed = records[len(records)-1].Time.Sub(records[0].Time)

	// First pass: tally dead ends and actions, and find the pauses.
	dead := map[[2]string]int{} // [key, mode] -> count
	actions := map[string]int{}
	var gaps []time.Duration
	for i, rec := range records {
		if rec.Action != "" {
			actions[rec.Action]++
		} else if !rec.Text && !rec.Pending {
			// An unresolved press on a surface that neither takes text nor is
			// holding a sequence open: a reach for something kubecom does not have.
			dead[[2]string{rec.Key, rec.Mode}]++
		}
		if i > 0 {
			gaps = append(gaps, rec.Time.Sub(records[i-1].Time))
		}
	}

	rep.DeadEnds = rankedDead(dead)
	rep.Actions = rankedActions(actions)
	rep.Pauses = topPauses(gaps, records)

	// Second pass: find abandoned sequences.
	rep.Sequences = abandonedSequences(records, resolve, seqTimeout)

	return rep
}

// rankedDead tallies dead ends, most-frequent first, breaking ties by key then
// mode so the order is deterministic.
func rankedDead(m map[[2]string]int) []DeadEnd {
	out := make([]DeadEnd, 0, len(m))
	for pair, n := range m {
		out = append(out, DeadEnd{Key: pair[0], Mode: pair[1], Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Mode < out[j].Mode
	})
	return out
}

// rankedActions tallies resolved actions, most-frequent first, ties by id.
func rankedActions(m map[string]int) []ActionCount {
	out := make([]ActionCount, 0, len(m))
	for a, n := range m {
		out = append(out, ActionCount{Action: a, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Action < out[j].Action
	})
	return out
}

// topPauses finds the longest gaps, each reported with the press that followed
// it. Only positive gaps count — two presses stamped the same millisecond are
// not a pause.
func topPauses(gaps []time.Duration, records []Record) []Pause {
	type gapAt struct {
		gap time.Duration
		at  int // index of the press after the gap
	}
	all := make([]gapAt, 0, len(gaps))
	for i, g := range gaps {
		if g > 0 {
			all = append(all, gapAt{gap: g, at: i + 1})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].gap > all[j].gap })
	if len(all) > maxPauses {
		all = all[:maxPauses]
	}
	out := make([]Pause, 0, len(all))
	for _, ga := range all {
		out = append(out, Pause{
			Gap:      ga.gap,
			NextKey:  records[ga.at].Key,
			NextMode: records[ga.at].Mode,
		})
	}
	return out
}

// abandonedSequences finds runs of pending presses that never resolved into a
// completed binding. It walks the trace tracking the current pending run; when
// the run ends it asks the resolver whether the run plus the press that followed
// it form a binding — if yes, the sequence completed and is not reported; if no
// (or the trace ended), it was abandoned and is reported.
//
// A run can also be interrupted by the keymap's sequence timeout: a gap longer
// than seqTimeout between two pending presses means the timer cleared the buffer
// before the second press, so the run died and the presses belong to separate
// sequences. (The timeout is not recorded in the trace — the writer only sees
// presses — so this is the one thing the analyser must infer from timestamps.)
func abandonedSequences(records []Record, resolve Resolver, seqTimeout time.Duration) []AbandonedSequence {
	var out []AbandonedSequence
	var run []Record
	flush := func(after Record, afterSet bool) {
		if len(run) == 0 {
			return
		}
		// Does the run, extended by whatever came next, complete a binding?
		if resolve != nil && afterSet {
			tokens := make([]string, 0, len(run)+1)
			for _, r := range run {
				tokens = append(tokens, r.Key)
			}
			tokens = append(tokens, after.Key)
			if _, ok := resolve.Resolve(tokens...); ok {
				run = nil
				return
			}
		}
		keys := make([]string, 0, len(run))
		for _, r := range run {
			keys = append(keys, r.Key)
		}
		seq := AbandonedSequence{
			Keys:  keys,
			Mode:  run[0].Mode,
			First: run[0].Time,
			Last:  run[len(run)-1].Time,
		}
		if afterSet {
			seq.AfterKey = after.Key
			seq.AfterMode = after.Mode
		}
		out = append(out, seq)
		run = nil
	}

	for _, rec := range records {
		if rec.Pending {
			// A gap past the sequence timeout means the previous run died to the
			// timer before this press — close that run as abandoned, then start a
			// new one with this press.
			if len(run) > 0 && rec.Time.Sub(run[len(run)-1].Time) > seqTimeout {
				flush(Record{}, false)
			}
			run = append(run, rec)
			continue
		}
		flush(rec, true)
		// rec itself may begin a new pending run — it is processed next iteration
		// via its own Pending flag.
	}
	flush(Record{}, false)
	return out
}
