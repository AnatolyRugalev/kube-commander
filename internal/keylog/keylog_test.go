package keylog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriterWritesOneJSONObjectPerLine pins the format an analyser reads. JSONL is
// the promise: one object, one line, no wrapping array — so a trace can be tailed
// while the session that is producing it is still running.
func TestWriterWritesOneJSONObjectPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	w, err := Create(path, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	w.Record(Record{Key: "j", Mode: "browse", Action: "nav.down"})
	w.Record(Record{Key: "z", Mode: "browse"})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), lines)
	}

	var first Record
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 1 is not JSON: %v", err)
	}
	if first.Key != "j" || first.Action != "nav.down" {
		t.Errorf("line 1 round-tripped to %+v", first)
	}
	if first.Time.IsZero() {
		t.Error("Create must stamp a record that arrives without a time")
	}

	// The dead-end record must not carry an action key at all — an analyser
	// counting empty strings and one counting absent fields must agree.
	if strings.Contains(lines[1], `"action"`) {
		t.Errorf("a record with no action should omit the field, got %s", lines[1])
	}
}

// TestWriterAppends protects the first walk of a story from the second: re-running
// a story must not silently erase the trace that showed the problem.
func TestWriterAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")

	for _, key := range []string{"a", "b"} {
		w, err := Create(path, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		w.Record(Record{Key: key, Mode: "browse"})
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	if lines := readLines(t, path); len(lines) != 2 {
		t.Fatalf("the second run should have appended, got %d lines: %q", len(lines), lines)
	}
}

// TestWriterKeepsTheSessionAliveOnWriteFailure is the rule that matters more than
// the trace: a recording problem must never take down the session being recorded.
// The writer reports once and goes quiet.
func TestWriterKeepsTheSessionAliveOnWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	var errs []error
	w, err := Create(path, func(err error) { errs = append(errs, err) })
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Close the file under the writer, so every subsequent write fails.
	if err := w.f.Close(); err != nil {
		t.Fatalf("closing the underlying file: %v", err)
	}

	w.Record(Record{Key: "a", Mode: "browse"}) // fails, reports
	w.Record(Record{Key: "b", Mode: "browse"}) // fails, stays quiet
	w.Record(Record{Key: "c", Mode: "browse"})

	if len(errs) != 1 {
		t.Fatalf("want exactly one report for a broken trace, got %d: %v", len(errs), errs)
	}
}

// TestNilWriterIsInert lets the launcher hold a nil *Writer without guarding every
// call site — the off-by-default path must not be the fragile one.
func TestNilWriterIsInert(t *testing.T) {
	var w *Writer
	w.Record(Record{Key: "j"})
	if err := w.Close(); err != nil {
		t.Errorf("closing a nil Writer: %v", err)
	}
}

// TestRecordTimeIsTruncatedToMillis keeps traces readable: nanosecond stamps make
// every line longer and measure nothing a human hesitation needs.
func TestRecordTimeIsTruncatedToMillis(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	w, err := Create(path, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	w.Record(Record{Key: "j", Time: time.Date(2026, 8, 15, 12, 0, 0, 123456789, time.UTC)})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var got Record
	if err := json.Unmarshal([]byte(readLines(t, path)[0]), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got.Time.Nanosecond() != 123000000 {
		t.Errorf("time = %v, want it truncated to milliseconds", got.Time)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}
