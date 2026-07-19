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
	open := func(ctx context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("hello\nworld\n")), nil
	}
	out := make(chan LogEvent, 8)
	runLogStream(context.Background(), open, out) // closes out on return
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
	open := func(ctx context.Context) (io.ReadCloser, error) { return nil, openErr }
	out := make(chan LogEvent, 4)
	runLogStream(context.Background(), open, out)
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
	open := func(ctx context.Context) (io.ReadCloser, error) { return nil, errors.New("cancelled open") }
	out := make(chan LogEvent, 4)
	runLogStream(ctx, open, out)
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
