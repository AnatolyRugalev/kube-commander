package kube

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func ptrInt64(v int64) *int64 { return &v }

// drainLogs collects every LogEvent from ch until it is closed.
func drainLogs(ch <-chan LogEvent) []LogEvent {
	var got []LogEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

// scriptReader yields data in one or more Reads and then ends with err (a
// transient drop) or, when err is nil, a clean io.EOF — so a following pump can be
// driven through both a mid-stream drop and a clean container-end without a
// cluster.
type scriptReader struct {
	data []byte
	err  error
}

func (s *scriptReader) Read(p []byte) (int, error) {
	if len(s.data) > 0 {
		n := copy(p, s.data)
		s.data = s.data[n:]
		return n, nil
	}
	if s.err != nil {
		return 0, s.err
	}
	return 0, io.EOF
}

func (s *scriptReader) Close() error { return nil }

func TestPodLogOptionsMapping(t *testing.T) {
	since := time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC)
	opts := LogOptions{
		Container:    "app",
		Follow:       true,
		Previous:     true,
		Timestamps:   true,
		TailLines:    ptrInt64(100),
		SinceSeconds: ptrInt64(60),
		SinceTime:    &since,
		LimitBytes:   ptrInt64(4096),
	}
	got := podLogOptions(opts)

	if got.Container != "app" || !got.Follow || !got.Previous || !got.Timestamps {
		t.Errorf("scalar fields not mapped: %+v", got)
	}
	if got.TailLines == nil || *got.TailLines != 100 {
		t.Errorf("TailLines = %v, want 100", got.TailLines)
	}
	if got.SinceSeconds == nil || *got.SinceSeconds != 60 {
		t.Errorf("SinceSeconds = %v, want 60", got.SinceSeconds)
	}
	if got.LimitBytes == nil || *got.LimitBytes != 4096 {
		t.Errorf("LimitBytes = %v, want 4096", got.LimitBytes)
	}
	if got.SinceTime == nil || !got.SinceTime.Time.Equal(since) {
		t.Errorf("SinceTime = %v, want %v", got.SinceTime, since)
	}
}

func TestPodLogOptionsZeroValue(t *testing.T) {
	got := podLogOptions(LogOptions{})
	if got.Container != "" || got.Follow || got.Previous || got.Timestamps {
		t.Errorf("zero LogOptions should map to zero scalars: %+v", got)
	}
	if got.TailLines != nil || got.SinceSeconds != nil || got.SinceTime != nil || got.LimitBytes != nil {
		t.Errorf("nil optionals must stay nil: %+v", got)
	}
}

func TestStreamLogsLinesVerbatim(t *testing.T) {
	// Trailing newline on the last line; each line delivered without its newline.
	r := strings.NewReader("line one\nline two\nline three\n")
	out := make(chan LogEvent, 8)
	err := streamLogs(context.Background(), r, out)
	close(out)
	if err != nil {
		t.Fatalf("streamLogs err = %v, want nil (clean EOF)", err)
	}
	got := drainLogs(out)
	want := []string{"line one", "line two", "line three"}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Err != nil {
			t.Errorf("event %d unexpected err: %v", i, got[i].Err)
		}
		if got[i].Line != w {
			t.Errorf("event %d line = %q, want %q", i, got[i].Line, w)
		}
	}
}

func TestStreamLogsNoTrailingNewline(t *testing.T) {
	// A final line without a newline must still be delivered.
	r := strings.NewReader("only line, no newline")
	out := make(chan LogEvent, 4)
	if err := streamLogs(context.Background(), r, out); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	close(out)
	got := drainLogs(out)
	if len(got) != 1 || got[0].Line != "only line, no newline" {
		t.Fatalf("got %+v, want single line delivered", got)
	}
}

func TestStreamLogsOverLongLine(t *testing.T) {
	// A line longer than logScanMaxLine ends the stream with ErrTooLong rather than
	// truncating silently — surfaced as an error, never a panic (#86).
	long := strings.Repeat("x", logScanMaxLine+1)
	out := make(chan LogEvent, 4)
	err := streamLogs(context.Background(), strings.NewReader(long), out)
	close(out)
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("err = %v, want bufio.ErrTooLong", err)
	}
}

func TestStreamLogsContextCancelled(t *testing.T) {
	// A blocked send on a full, unread channel unblocks when ctx is cancelled, and
	// streamLogs returns the context error.
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan LogEvent) // unbuffered: the first send blocks
	done := make(chan error, 1)
	go func() {
		done <- streamLogs(ctx, strings.NewReader("a\nb\nc\n"), out)
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamLogs did not return after ctx cancellation")
	}
}

func TestRunLogStreamDeliversThenCloses(t *testing.T) {
	open := func(ctx context.Context, _ *time.Time) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("hello\nworld\n")), nil
	}
	out := make(chan LogEvent, 8)
	runLogStream(context.Background(), open, false, false, out) // closes out on return
	got := drainLogs(out)
	if len(got) != 2 || got[0].Line != "hello" || got[1].Line != "world" {
		t.Fatalf("got %+v, want two lines and no error event", got)
	}
	for _, ev := range got {
		if ev.Err != nil {
			t.Errorf("unexpected error event: %v", ev.Err)
		}
	}
}

func TestRunLogStreamOpenError(t *testing.T) {
	openErr := errors.New("boom")
	open := func(ctx context.Context, _ *time.Time) (io.ReadCloser, error) { return nil, openErr }
	out := make(chan LogEvent, 4)
	runLogStream(context.Background(), open, false, false, out)
	got := drainLogs(out)
	if len(got) != 1 || !errors.Is(got[0].Err, openErr) {
		t.Fatalf("got %+v, want single terminal error event wrapping the open error", got)
	}
}

func TestRunLogStreamOpenErrorSuppressedOnCancel(t *testing.T) {
	// If ctx is already cancelled, an open failure is a requested stop, not a
	// reportable error — the channel closes with no error event.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	open := func(ctx context.Context, _ *time.Time) (io.ReadCloser, error) {
		return nil, errors.New("cancelled open")
	}
	out := make(chan LogEvent, 4)
	runLogStream(ctx, open, false, false, out)
	if got := drainLogs(out); len(got) != 0 {
		t.Fatalf("got %+v, want no events when ctx already cancelled", got)
	}
}

func TestLogsEmptyPodNameRejected(t *testing.T) {
	c := &Clients{}
	if _, err := c.Logs(context.Background(), ObjectRef{Namespace: "web"}, LogOptions{}); err == nil {
		t.Fatal("Logs with empty pod name should error, got nil")
	}
}

func TestParseLogTimestamp(t *testing.T) {
	ts, content, ok := parseLogTimestamp("2026-07-19T10:30:05.123456789Z hello world")
	if !ok {
		t.Fatal("ok = false, want a parsed timestamp")
	}
	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
	want := time.Date(2026, 7, 19, 10, 30, 5, 123456789, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("ts = %v, want %v", ts, want)
	}

	// An empty content after the timestamp is still a valid (blank) line.
	if _, c, ok := parseLogTimestamp("2026-07-19T10:30:05Z "); !ok || c != "" {
		t.Errorf("blank-content parse = (%q, %v), want (\"\", true)", c, ok)
	}

	// No space at all → not a timestamped line; content is the whole raw string.
	if _, c, ok := parseLogTimestamp("no-timestamp-here"); ok || c != "no-timestamp-here" {
		t.Errorf("no-space parse = (%q, %v), want (raw, false)", c, ok)
	}

	// A leading token that is not a timestamp → degrade, keep the whole line.
	if _, c, ok := parseLogTimestamp("hello world"); ok || c != "hello world" {
		t.Errorf("non-ts prefix parse = (%q, %v), want (raw, false)", c, ok)
	}
}

func TestLogResumerStripsTimestampByDefault(t *testing.T) {
	r := &logResumer{wantTimestamps: false}
	line, skip := r.process("2026-07-19T10:30:05.100Z payload")
	if skip || line != "payload" {
		t.Fatalf("got (%q, skip=%v), want (\"payload\", false)", line, skip)
	}
	// A line whose timestamp cannot be parsed is delivered verbatim.
	line, skip = r.process("garbage line without ts")
	if skip || line != "garbage line without ts" {
		t.Fatalf("unparseable line = (%q, %v), want it delivered verbatim", line, skip)
	}
}

func TestLogResumerKeepsTimestampWhenRequested(t *testing.T) {
	r := &logResumer{wantTimestamps: true}
	raw := "2026-07-19T10:30:05.100Z payload"
	line, skip := r.process(raw)
	if skip || line != raw {
		t.Fatalf("got (%q, %v), want the raw timestamped line kept", line, skip)
	}
}

func TestLogResumerDedupsReservedSecondOnResume(t *testing.T) {
	r := &logResumer{wantTimestamps: false}
	// Two lines delivered in the 10:30:05 second before a drop.
	mustDeliver(t, r, "2026-07-19T10:30:05.100Z one", "one")
	mustDeliver(t, r, "2026-07-19T10:30:05.200Z two", "two")

	// Reconnect: the server re-serves the whole 10:30:05 second.
	if got := r.resumeFrom(); got.Truncate(time.Second) != time.Date(2026, 7, 19, 10, 30, 5, 0, time.UTC) {
		t.Fatalf("resumeFrom second = %v, want 10:30:05", got.Truncate(time.Second))
	}
	r.beginResume()

	// The two already-delivered lines are dropped.
	mustSkip(t, r, "2026-07-19T10:30:05.100Z one")
	mustSkip(t, r, "2026-07-19T10:30:05.200Z two")
	// A line in the same second that was missed during the drop is delivered.
	mustDeliver(t, r, "2026-07-19T10:30:05.300Z three", "three")
	// The next second is delivered and clears the resume window.
	mustDeliver(t, r, "2026-07-19T10:30:06.000Z four", "four")
	if r.resuming {
		t.Error("resuming still set after advancing past the resumed second")
	}
}

func mustDeliver(t *testing.T, r *logResumer, raw, want string) {
	t.Helper()
	line, skip := r.process(raw)
	if skip || line != want {
		t.Fatalf("process(%q) = (%q, skip=%v), want (%q, false)", raw, line, skip, want)
	}
}

func mustSkip(t *testing.T, r *logResumer, raw string) {
	t.Helper()
	if _, skip := r.process(raw); !skip {
		t.Fatalf("process(%q) delivered, want it dropped as a re-served duplicate", raw)
	}
}

func TestFollowLogStreamReconnectsAndDedups(t *testing.T) {
	old := logRetryBackoff
	logRetryBackoff = time.Millisecond
	defer func() { logRetryBackoff = old }()

	conn1 := "2026-07-19T10:30:05.100Z alpha\n2026-07-19T10:30:05.200Z bravo\n"
	// Resume re-serves the whole 10:30:05 second: alpha+bravo (already delivered →
	// dropped) plus charlie (missed during the drop) then delta in the next second,
	// then a clean end.
	conn2 := "2026-07-19T10:30:05.100Z alpha\n2026-07-19T10:30:05.200Z bravo\n" +
		"2026-07-19T10:30:05.300Z charlie\n2026-07-19T10:30:06.000Z delta\n"

	var sinces []*time.Time
	call := 0
	open := func(_ context.Context, since *time.Time) (io.ReadCloser, error) {
		sinces = append(sinces, since)
		call++
		switch call {
		case 1:
			return &scriptReader{data: []byte(conn1), err: errors.New("connection reset")}, nil
		case 2:
			return &scriptReader{data: []byte(conn2)}, nil // clean EOF stops the follow
		default:
			t.Fatalf("unexpected open call %d", call)
			return nil, nil
		}
	}

	out := make(chan LogEvent, 32)
	runLogStream(context.Background(), open, true, false, out)
	got := drainLogs(out)

	wantLines := []string{"alpha", "bravo", "charlie", "delta"}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d events (%+v), want %d deduped lines", len(got), got, len(wantLines))
	}
	for i, w := range wantLines {
		if got[i].Err != nil {
			t.Errorf("event %d unexpected err: %v", i, got[i].Err)
		}
		if got[i].Line != w {
			t.Errorf("event %d = %q, want %q", i, got[i].Line, w)
		}
	}

	if len(sinces) != 2 {
		t.Fatalf("opened %d times, want exactly 2 (initial + one reconnect)", len(sinces))
	}
	if sinces[0] != nil {
		t.Errorf("initial open since = %v, want nil", sinces[0])
	}
	if sinces[1] == nil {
		t.Fatal("reconnect open since = nil, want the last-seen timestamp")
	}
	if sec := sinces[1].Truncate(time.Second); sec != time.Date(2026, 7, 19, 10, 30, 5, 0, time.UTC) {
		t.Errorf("reconnect resumed from %v, want the 10:30:05 second", sec)
	}
}

func TestFollowLogStreamCleanEndStops(t *testing.T) {
	// A clean end on the first connection (the container terminated) stops the
	// follow — it does not reconnect.
	open := func(_ context.Context, _ *time.Time) (io.ReadCloser, error) {
		return &scriptReader{data: []byte("2026-07-19T10:30:05.000Z only\n")}, nil
	}
	out := make(chan LogEvent, 8)

	done := make(chan struct{})
	go func() {
		runLogStream(context.Background(), open, true, false, out)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not stop after a clean end (reconnect loop?)")
	}

	got := drainLogs(out)
	if len(got) != 1 || got[0].Line != "only" || got[0].Err != nil {
		t.Fatalf("got %+v, want a single clean line and stop", got)
	}
}

func TestFollowLogStreamFirstOpenErrorTerminal(t *testing.T) {
	// Never connecting is a genuine failure surfaced as one terminal event (not an
	// endless silent reconnect loop).
	openErr := errors.New("no such pod")
	open := func(_ context.Context, _ *time.Time) (io.ReadCloser, error) { return nil, openErr }
	out := make(chan LogEvent, 4)
	runLogStream(context.Background(), open, true, false, out)
	got := drainLogs(out)
	if len(got) != 1 || !errors.Is(got[0].Err, openErr) {
		t.Fatalf("got %+v, want a single terminal error wrapping the open failure", got)
	}
}

func TestFollowLogStreamRetriesReconnectOpen(t *testing.T) {
	// After a first successful connection, a failed reconnect open is transient: it
	// is retried (bounded by ctx, à la watch) and never surfaced as an error event.
	old := logRetryBackoff
	logRetryBackoff = time.Millisecond
	defer func() { logRetryBackoff = old }()

	call := 0
	open := func(_ context.Context, _ *time.Time) (io.ReadCloser, error) {
		call++
		switch call {
		case 1:
			return &scriptReader{data: []byte("2026-07-19T10:30:05.000Z a\n"), err: errors.New("drop")}, nil
		case 2:
			return nil, errors.New("reconnect refused") // transient, retried silently
		case 3:
			return &scriptReader{data: []byte("2026-07-19T10:30:06.000Z b\n")}, nil // clean end
		default:
			t.Fatalf("unexpected open call %d", call)
			return nil, nil
		}
	}

	out := make(chan LogEvent, 8)
	runLogStream(context.Background(), open, true, false, out)
	got := drainLogs(out)

	if len(got) != 2 || got[0].Line != "a" || got[1].Line != "b" {
		t.Fatalf("got %+v, want lines a then b across the retried reconnect", got)
	}
	for _, ev := range got {
		if ev.Err != nil {
			t.Errorf("unexpected error event (reconnect open failure must stay silent): %v", ev.Err)
		}
	}
}

func TestFollowLogStreamStopsOnContextCancel(t *testing.T) {
	// ctx cancellation unwinds a following stream even mid-reconnect-backoff.
	old := logRetryBackoff
	logRetryBackoff = 50 * time.Millisecond
	defer func() { logRetryBackoff = old }()

	ctx, cancel := context.WithCancel(context.Background())
	open := func(_ context.Context, _ *time.Time) (io.ReadCloser, error) {
		// Always drop immediately so the loop is perpetually in reconnect/backoff.
		return &scriptReader{data: []byte("2026-07-19T10:30:05.000Z x\n"), err: errors.New("drop")}, nil
	}
	out := make(chan LogEvent, 64)
	done := make(chan struct{})
	go func() {
		runLogStream(ctx, open, true, false, out)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not stop after ctx cancellation")
	}
	// Draining must terminate (channel closed); no assertion on the exact count.
	drainLogs(out)
}
