package kube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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

// logRetryBackoff paces reconnect attempts for a following log stream so a server
// that immediately drops every connection cannot spin the loop hot — the log twin
// of watchRetryBackoff. A package var (not const) so hermetic reconnect tests can
// shrink it, matching the drain timing knobs (D40).
var logRetryBackoff = 2 * time.Second

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
	// them (`kubectl logs -f`), until the container ends or ctx is cancelled. A
	// transient transport drop is transparently reconnected from the last-seen
	// timestamp (see Logs); a clean end (the container terminated) stops following.
	Follow bool
	// Previous reads the logs of the container's previous terminated instance
	// (`kubectl logs -p`) — useful to see why a crash-looping container died.
	Previous bool
	// Timestamps prefixes each line with the server's RFC3339Nano timestamp
	// (`kubectl logs --timestamps`). When Follow reconnects it forces timestamps on
	// the wire regardless, to anchor the resume point, and strips them again before
	// delivery unless this is set.
	Timestamps bool
	// TailLines, when non-nil, starts the stream at the last N lines already in the
	// log (`kubectl logs --tail=N`) instead of the whole history. It governs only
	// the initial read; a follow reconnect anchors on SinceTime instead.
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

// logStreamOpener opens the raw log stream for a pod, optionally resuming at
// `since` (set on a follow reconnect). It exists so the streaming loop can be
// driven from a fake in hermetic tests (D18) — the real opener hits the API
// server (network), so a live stream is envtest territory, but the pump and the
// reconnect loop around it are exercised without a cluster.
type logStreamOpener func(ctx context.Context, since *time.Time) (io.ReadCloser, error)

// Logs streams a pod container's logs onto the returned channel until the stream
// ends or ctx is cancelled — the in-TUI equivalent of `kubectl logs`, in process
// via the typed clientset's GetLogs subresource, so no kubectl binary and no
// external pager are needed (D2). Each LogEvent carries one line; a terminal
// error (open failure, decode error, an over-long line) arrives as a final event
// with Err set, after which the channel closes. The stream is opened inside a
// background goroutine, so Logs returns immediately and never blocks first paint
// on the network; that goroutine owns every send and the close, so a consumer
// only ever mutates UI state in its Update (principle 1).
//
// When Follow is set, a transient transport drop is transparently reconnected à
// la watch (D34): the stream reopens from the last-seen line's timestamp (server
// timestamps are forced on the wire to anchor it) and the lines the server
// re-serves for that second — SinceTime is second-granular — are dropped, so the
// consumer sees an uninterrupted, duplicate-free line stream across reconnects
// without knowing they happened. A clean end (the container terminated) stops
// following. An empty pod name is rejected; errors are wrapped, never panicked
// (#86).
func (c *Clients) Logs(ctx context.Context, ref ObjectRef, opts LogOptions) (<-chan LogEvent, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: logs: empty pod name")
	}
	open := func(ctx context.Context, since *time.Time) (io.ReadCloser, error) {
		req := c.Clientset.CoreV1().Pods(ref.Namespace).GetLogs(ref.Name, podLogOptions(logOpenOptions(opts, since)))
		stream, err := req.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("kube: streaming logs for pod %s/%s: %w", ref.Namespace, ref.Name, err)
		}
		return stream, nil
	}
	out := make(chan LogEvent, logChanBuffer)
	go runLogStream(ctx, open, opts.Follow, opts.Timestamps, out)
	return out, nil
}

// PodContainers returns the names of a pod's regular containers, in spec order —
// the set the logs container picker offers when a pod has more than one container
// (`kubectl logs` requires -c to disambiguate a multi-container pod). It is the
// same set kubectl's default-container logic counts, so a pod with a single
// regular container needs no picker (the TUI streams it directly), while a
// multi-container pod prompts which to stream. Only spec.containers are returned;
// init- and ephemeral-container logs are a later refinement. An empty pod name is
// rejected; a get error is wrapped, never panicked (#86).
func (c *Clients) PodContainers(ctx context.Context, ref ObjectRef) ([]string, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: pod containers: empty pod name")
	}
	pod, err := c.Clientset.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: getting containers for pod %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		names = append(names, pod.Spec.Containers[i].Name)
	}
	return names, nil
}

// logOpenOptions derives the options for one open of a log stream: the caller's own for
// the initial connect (since == nil), and the resume shape for a follow reconnect. It is
// pure and takes opts by value, so it is unit-tested without a clientset — worth its own
// function because it holds the one rule that is invisible at the call site: the
// selectors that mean "where to *start*" are initial-read-only.
//
//   - A following stream forces server-side timestamps on the wire so a reconnect can
//     anchor on the last line's time; they are stripped again before delivery unless the
//     caller asked for them (LogOptions.Timestamps).
//   - A reconnect anchors at the last-seen second (the dedup window drops the lines the
//     server re-serves for it) and **clears TailLines and SinceSeconds**: both select a
//     starting point relative to *now*, so carrying them across a drop would re-serve the
//     last N lines (or the last N seconds) on top of output the reader has already seen,
//     instead of resuming where they were. That matters as of LOGS-05a, where the TUI
//     opens every log stream with a TailLines of its own.
func logOpenOptions(opts LogOptions, since *time.Time) LogOptions {
	if opts.Follow {
		opts.Timestamps = true
	}
	if since != nil {
		opts.SinceTime = since
		opts.SinceSeconds = nil
		opts.TailLines = nil
	}
	return opts
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

// runLogStream drives the log channel to completion and closes out on return. For
// a non-following stream it is a single connection: open once, pump verbatim, done
// (a clean EOF closes with no error, any other failure sends one terminal Err). A
// following stream delegates to followLogStream, which reconnects on a transient
// drop. It is separated from Logs so tests can drive it with a fake opener.
func runLogStream(ctx context.Context, open logStreamOpener, follow, wantTimestamps bool, out chan<- LogEvent) {
	defer close(out)
	if follow {
		followLogStream(ctx, open, wantTimestamps, out)
		return
	}

	stream, err := open(ctx, nil)
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
		// io.EOF is the normal end of a non-following stream; ctx cancellation is a
		// requested stop. Neither is worth an error event.
		if !errors.Is(err, io.EOF) && ctx.Err() == nil {
			sendLog(ctx, out, LogEvent{Err: err})
		}
	}
}

// followLogStream keeps a following log stream open across transient drops. It
// opens a connection, pumps its lines through a logResumer (which tracks the
// resume point and drops re-served lines), and on a non-EOF read error backs off
// and reopens from the last-seen timestamp. A clean end (bufio.Scanner reports
// nil at io.EOF) means the container's log ended → stop; ctx cancellation stops
// too. The very first open failure is terminal (surfaced like the non-follow
// path); a later reconnect failure is transient (back off and retry, bounded by
// ctx, à la watch) — it is not surfaced, since a LogEvent with Err set is the
// terminal event by contract.
func followLogStream(ctx context.Context, open logStreamOpener, wantTimestamps bool, out chan<- LogEvent) {
	res := &logResumer{wantTimestamps: wantTimestamps}
	for {
		if ctx.Err() != nil {
			return
		}

		var since *time.Time
		if res.haveLast {
			t := res.resumeFrom()
			since = &t
			res.beginResume()
		}

		stream, err := open(ctx, since)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !res.haveLast {
				// Never connected: a genuine open failure, terminal like non-follow.
				sendLog(ctx, out, LogEvent{Err: err})
				return
			}
			// A reconnect attempt failed: transient, back off and retry.
			if !sleep(ctx, logRetryBackoff) {
				return
			}
			continue
		}

		perr := pumpFollow(ctx, stream, res, out)
		_ = stream.Close()
		if ctx.Err() != nil {
			return
		}
		if perr == nil {
			// Clean EOF: the container's log ended → stop following (as `kubectl -f`
			// exits when the container dies).
			return
		}
		// Any other read error is a transient drop → reconnect from the last line.
		if !sleep(ctx, logRetryBackoff) {
			return
		}
	}
}

// streamLogs reads r line by line and delivers each verbatim as a LogEvent on out
// until the reader ends, ctx is cancelled, or a read error occurs (the
// non-following pump). Lines are forwarded as-is (the server already formatted
// them per the request's Timestamps flag), stripped of the trailing newline. It
// returns the scanner's error — nil on a clean end (bufio.Scanner reports nil at
// io.EOF), bufio.ErrTooLong on an over-long line, or ctx.Err() if cancelled
// mid-stream. Pure over an io.Reader, so it is tested directly from byte streams.
func streamLogs(ctx context.Context, r io.Reader, out chan<- LogEvent) error {
	sc := newLogScanner(r)
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

// pumpFollow is streamLogs for a following stream: it runs each raw line through
// res (which strips the forced timestamp, tracks the resume point, and drops the
// lines the server re-serves after a reconnect) before delivering it. It returns
// the scanner's error so followLogStream can tell a clean end (nil) from a
// transient drop.
func pumpFollow(ctx context.Context, r io.Reader, res *logResumer, out chan<- LogEvent) error {
	sc := newLogScanner(r)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, skip := res.process(sc.Text())
		if skip {
			continue
		}
		if !sendLog(ctx, out, LogEvent{Line: line}) {
			return ctx.Err()
		}
	}
	return sc.Err()
}

// newLogScanner builds the shared line scanner: bufio.Scanner with a raised max
// line so a large single line (stack trace / one-line JSON) ends the stream with
// bufio.ErrTooLong rather than truncating silently (#86).
func newLogScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, logScanInitBuffer), logScanMaxLine)
	return sc
}

// logResumer tracks a following stream's resume point across reconnects. The wire
// request forces server-side timestamps on, so every raw line is
// "<RFC3339Nano> <content>"; the resumer records the last timestamp seen (the
// point to resume from) and the raw lines already delivered within that
// timestamp's whole second. On a reconnect the stream reopens with SinceTime at
// that second — SinceTime is second-granular, so the server re-serves the whole
// second — and the resumer drops the lines it recorded, delivering only what is
// genuinely new. Delivered lines have their timestamp stripped unless the caller
// asked for timestamps.
type logResumer struct {
	wantTimestamps bool

	haveLast bool
	lastTS   time.Time

	// curSec is the whole second of the most recent delivered line; secLines is the
	// set of raw (timestamped) lines delivered within it — the dedup window a
	// reconnect resumes into.
	curSec   time.Time
	secLines map[string]struct{}

	// resuming is true from a reconnect until the stream advances past resumeSec;
	// while set, a raw line already in secLines is a server re-serve and is dropped.
	resuming  bool
	resumeSec time.Time
}

// resumeFrom is the timestamp a reconnect resumes from: the last delivered line's
// time. metav1.Time serializes it to second precision, so the server re-serves
// from the start of that second (which process dedups against secLines).
func (r *logResumer) resumeFrom() time.Time { return r.lastTS }

// beginResume marks the dedup window for the reconnect about to happen: the second
// of the last delivered line, whose already-delivered lines are held in secLines.
func (r *logResumer) beginResume() {
	r.resuming = true
	r.resumeSec = r.curSec
}

// process handles one raw line from a following stream. It returns the line to
// deliver (timestamp stripped unless wantTimestamps) and whether to skip it (a
// duplicate the server re-served after a reconnect). It advances the resume point
// for every delivered line.
func (r *logResumer) process(raw string) (string, bool) {
	ts, content, ok := parseLogTimestamp(raw)
	if !ok {
		// No parseable timestamp — unexpected, since we force Timestamps on the wire.
		// Deliver verbatim and leave the resume point untouched rather than guess.
		return raw, false
	}

	sec := ts.Truncate(time.Second)
	if r.resuming {
		if sec.After(r.resumeSec) {
			// Past the re-served second: nothing more can be a duplicate.
			r.resuming = false
		} else if _, dup := r.secLines[raw]; dup {
			// Already delivered before the drop — drop the server's re-serve.
			return "", true
		}
	}

	if !sec.Equal(r.curSec) {
		r.curSec = sec
		r.secLines = make(map[string]struct{})
	}
	r.secLines[raw] = struct{}{}
	r.lastTS = ts
	r.haveLast = true

	if r.wantTimestamps {
		return raw, false
	}
	return content, false
}

// parseLogTimestamp splits a server log line "<RFC3339Nano> <content>" into its
// timestamp and the content after it. ok is false when there is no space or the
// prefix is not a timestamp (then content is the whole line) so callers degrade
// gracefully instead of dropping the line.
func parseLogTimestamp(raw string) (time.Time, string, bool) {
	i := strings.IndexByte(raw, ' ')
	if i < 0 {
		return time.Time{}, raw, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw[:i])
	if err != nil {
		return time.Time{}, raw, false
	}
	return ts, raw[i+1:], true
}

// SplitLogTimestamp splits a server-timestamped log line — "<RFC3339Nano> <message>",
// the shape the API returns when LogOptions.Timestamps is set — into the timestamp
// prefix and the message after it. It is the read side of that option: a consumer that
// wants to show the timestamp *and* the message as separate things (the TUI's logs view,
// which toggles the timestamp's display and always greps the message) asks for
// Timestamps and splits here, rather than re-deriving the format.
//
// A line the server did not stamp — or one whose prefix does not parse — yields an empty
// stamp and the whole line as content, so a caller that concatenates them gets the line
// back unchanged. Both results are slices of line: splitting copies nothing.
func SplitLogTimestamp(line string) (stamp, content string) {
	if _, c, ok := parseLogTimestamp(line); ok {
		// c is line's suffix after the single separating space (see parseLogTimestamp),
		// so the stamp is everything before that space.
		return line[:len(line)-len(c)-1], c
	}
	return "", line
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
