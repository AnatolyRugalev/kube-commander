package kube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// logChanBuffer bounds how far the log goroutine may run ahead of a slow
	// consumer before it blocks — the same bounded-buffering discipline as the
	// watch channel (watchChanBuffer). Log lines arrive in bursts (a container
	// flushing a backlog on connect), so a modest buffer absorbs them without
	// unbounded memory growth; the consumer is a Bubble Tea Update.
	logChanBuffer = 256
	// logScanInitBuffer / logScanMaxLine size the line scanner. A single log line
	// can be large (a stack trace or a JSON blob on one line), so the cap is raised
	// well past bufio.Scanner's 64 KiB default; a line longer than the cap ends the
	// stream with bufio.ErrTooLong rather than silently truncating (surfaced as an
	// error event, not a panic — #86).
	logScanInitBuffer = 64 * 1024
	logScanMaxLine    = 1024 * 1024
)

// LogEvent is one item delivered on a Logs channel: either a single log Line
// (without its trailing newline — the consumer re-adds it) or a terminal Err.
// Line and Err are mutually exclusive; an event with Err set is always the last
// event before the channel closes.
type LogEvent struct {
	Line string
	Err  error
}

// LogOptions selects which container's logs to stream and how, mirroring the
// flags of `kubectl logs`. The zero value streams the current logs of the pod's
// (only/default) container once and stops at EOF; set Follow to keep the stream
// open for new lines. The *int64 / *time.Time fields are optional — a nil pointer
// means "unset", exactly as corev1.PodLogOptions treats them.
type LogOptions struct {
	// Container names the container to read from. Empty selects the pod's default
	// container (its sole container, or the one named by the default-container
	// annotation), matching `kubectl logs` with no -c.
	Container string
	// Follow keeps the stream open and delivers new lines as the container writes
	// them (`kubectl logs -f`), until the container ends or ctx is cancelled. This
	// leg streams a single connection; reconnect-on-drop is M1-07d.
	Follow bool
	// Previous reads the logs of the container's previous terminated instance
	// (`kubectl logs -p`) — useful to see why a crash-looping container died.
	Previous bool
	// Timestamps prefixes each line with the server's RFC3339Nano timestamp
	// (`kubectl logs --timestamps`). Passed through verbatim; when set, the Line
	// delivered includes the timestamp prefix.
	Timestamps bool
	// TailLines, when non-nil, starts the stream at the last N lines already in the
	// log (`kubectl logs --tail=N`) instead of the whole history.
	TailLines *int64
	// SinceSeconds, when non-nil, returns only lines newer than N seconds ago
	// (`kubectl logs --since=Ns`). Mutually exclusive with SinceTime, per the API.
	SinceSeconds *int64
	// SinceTime, when non-nil, returns only lines at or after this instant
	// (`kubectl logs --since-time=...`). Second-granularity server-side.
	SinceTime *time.Time
	// LimitBytes, when non-nil, caps the total bytes returned (`kubectl logs
	// --limit-bytes=N`); the stream ends once the cap is reached.
	LimitBytes *int64
}

// logStreamOpener opens the raw log stream for a pod. It exists so the streaming
// loop can be driven from a fake in hermetic tests (D18) — the real opener hits
// the API server (network), so a live stream is envtest territory, but the line
// pump around it is exercised without a cluster.
type logStreamOpener func(ctx context.Context) (io.ReadCloser, error)

// Logs streams a pod container's logs onto the returned channel until the stream
// ends (EOF) or ctx is cancelled — the in-TUI equivalent of `kubectl logs`, in
// process via the typed clientset's GetLogs subresource, so no kubectl binary and
// no external pager are needed (D2). Each LogEvent carries one line; a terminal
// error (open failure, decode error, an over-long line) arrives as a final event
// with Err set, after which the channel closes. The stream is opened inside a
// background goroutine, so Logs returns immediately and never blocks first paint
// on the network; that goroutine owns every send and the close, so a consumer
// only ever mutates UI state in its Update (principle 1).
//
// This leg is a single connection. Follow keeps it open for live lines, but a
// transient drop ends the stream rather than reconnecting; resume-on-drop à la
// watch (D34) is M1-07d. An empty pod name is rejected; errors are wrapped, never
// panicked (#86).
func (c *Clients) Logs(ctx context.Context, ref ObjectRef, opts LogOptions) (<-chan LogEvent, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: logs: empty pod name")
	}
	open := func(ctx context.Context) (io.ReadCloser, error) {
		req := c.Clientset.CoreV1().Pods(ref.Namespace).GetLogs(ref.Name, podLogOptions(opts))
		stream, err := req.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("kube: streaming logs for pod %s/%s: %w", ref.Namespace, ref.Name, err)
		}
		return stream, nil
	}
	out := make(chan LogEvent, logChanBuffer)
	go runLogStream(ctx, open, out)
	return out, nil
}

// podLogOptions maps kubecom's LogOptions onto client-go's corev1.PodLogOptions.
// Pure, so the mapping is unit-tested without a client; SinceTime is wrapped in a
// metav1.Time (the API's second-granularity timestamp type).
func podLogOptions(opts LogOptions) *corev1.PodLogOptions {
	o := &corev1.PodLogOptions{
		Container:    opts.Container,
		Follow:       opts.Follow,
		Previous:     opts.Previous,
		Timestamps:   opts.Timestamps,
		TailLines:    opts.TailLines,
		SinceSeconds: opts.SinceSeconds,
		LimitBytes:   opts.LimitBytes,
	}
	if opts.SinceTime != nil {
		t := metav1.NewTime(*opts.SinceTime)
		o.SinceTime = &t
	}
	return o
}

// runLogStream opens the stream and pumps its lines onto out, closing out on
// return. It owns the stream's lifecycle; a clean EOF closes the channel with no
// error event, any other failure sends one terminal Err first. It is separated
// from Logs so tests can drive it with a fake opener.
func runLogStream(ctx context.Context, open logStreamOpener, out chan<- LogEvent) {
	defer close(out)
	stream, err := open(ctx)
	if err != nil {
		// ctx cancellation while opening is not a reportable failure — the caller
		// asked to stop — so only surface a genuine open error.
		if ctx.Err() == nil {
			sendLog(ctx, out, LogEvent{Err: err})
		}
		return
	}
	defer func() { _ = stream.Close() }()

	if err := streamLogs(ctx, stream, out); err != nil {
		// io.EOF is the normal end of a non-following stream (and of a following one
		// once the container terminates); ctx cancellation is a requested stop.
		// Neither is worth an error event.
		if !errors.Is(err, io.EOF) && ctx.Err() == nil {
			sendLog(ctx, out, LogEvent{Err: err})
		}
	}
}

// streamLogs reads r line by line and delivers each as a LogEvent on out until
// the reader ends, ctx is cancelled, or a read error occurs. Lines are forwarded
// verbatim (the server already formatted them per the request's Timestamps flag),
// stripped of the trailing newline. It returns the scanner's error — nil or io.EOF
// on a clean end, bufio.ErrTooLong on an over-long line, or ctx.Err() if cancelled
// mid-stream. Pure over an io.Reader, so it is tested directly from byte streams.
func streamLogs(ctx context.Context, r io.Reader, out chan<- LogEvent) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, logScanInitBuffer), logScanMaxLine)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !sendLog(ctx, out, LogEvent{Line: sc.Text()}) {
			return ctx.Err()
		}
	}
	return sc.Err()
}

// sendLog delivers ev on out unless ctx is cancelled first; it returns false when
// the send is abandoned so callers can unwind promptly. The log-channel twin of
// sendWatch.
func sendLog(ctx context.Context, out chan<- LogEvent, ev LogEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
