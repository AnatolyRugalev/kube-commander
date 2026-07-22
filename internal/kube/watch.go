package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// WatchEventType classifies a server-side Table watch delta. ADDED/MODIFIED/
// DELETED mirror the Kubernetes watch verbs (one affected row each); RESET is a
// kubecom-level signal that the stream (re)synced from a fresh List and the
// consumer must replace its whole row set with this event's Rows; ERROR carries
// a terminal watch failure in Err (the loop keeps retrying, but the consumer is
// told so it can surface the condition).
type WatchEventType string

const (
	WatchAdded    WatchEventType = "ADDED"
	WatchModified WatchEventType = "MODIFIED"
	WatchDeleted  WatchEventType = "DELETED"
	WatchReset    WatchEventType = "RESET"
	WatchError    WatchEventType = "ERROR"
)

// WatchEvent is one delta delivered on a Watch channel. Columns is set on RESET
// (and carried forward on later deltas) because the API server only guarantees
// column definitions on the first Table response of a connection — the consumer
// can always read the current columns off any event without caching them itself.
// Rows holds the affected rows: the full current set on RESET, a single row on
// ADDED/MODIFIED/DELETED. Err is set only on ERROR.
type WatchEvent struct {
	Type    WatchEventType
	Columns []Column
	Rows    []Row
	Err     error
}

const (
	// watchChanBuffer bounds how far the watch goroutine may run ahead of a slow
	// consumer before it blocks (bounded buffering) — deltas are small and the
	// consumer is a Bubble Tea Update, so a modest buffer absorbs bursts without
	// unbounded memory growth.
	watchChanBuffer = 64
	// watchRetryBackoff paces reconnect attempts so a server that immediately
	// closes every watch cannot spin the loop hot.
	watchRetryBackoff = 2 * time.Second
	// listPollInterval paces the re-List loop for kinds that don't support the
	// watch verb (e.g. componentstatuses) — there is no stream to follow, so the
	// loop degrades to periodic List+RESET so the rows stay fresh-ish without
	// hammering the apiserver (principle 3: degrade, don't blank).
	listPollInterval = 10 * time.Second
)

// errExpired signals the watch's resourceVersion is too old — HTTP 410 Gone /
// StatusReasonExpired. Unlike a transient drop (which resumes from the last
// resourceVersion) this forces a full re-List: the server can no longer replay
// history from that point, so the only correct recovery is to resync from
// scratch and emit a fresh RESET.
type errExpired struct{ msg string }

func (e *errExpired) Error() string { return e.msg }

// Watch starts a server-side Table watch for r and streams deltas onto the
// returned channel until ctx is cancelled. It is the live counterpart of List:
// the first event is always a RESET carrying the full current rows plus the
// column definitions, followed by ADDED/MODIFIED/DELETED single-row deltas that
// reuse those columns. On watch expiry (410 Gone) or a transport drop the loop
// re-lists — emitting another RESET — and reconnects, so a consumer that treats
// every RESET as a full replace stays correct across reconnects without knowing
// they happened. The channel is buffered (bounded buffering) and is closed when
// ctx is cancelled or the loop can no longer proceed; a background goroutine owns
// all sends, so UI state is only ever mutated in the consumer's Update.
func (c *Clients) Watch(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (<-chan WatchEvent, error) {
	client, err := c.restClientForGV(r.GVR.GroupVersion())
	if err != nil {
		return nil, err
	}
	out := make(chan WatchEvent, watchChanBuffer)
	go c.watchLoop(ctx, client, r, namespace, opts, out)
	return out, nil
}

// watchLoop is the reconnecting List→Watch driver (RetryWatcher-style). It lists
// to establish a baseline + resourceVersion (emitting RESET), opens a watch from
// that resourceVersion, and streams deltas until the stream ends or errors. A
// clean end or a resumable drop reconnects from the last resourceVersion with no
// re-list; a 410/Expired forces a fresh List (and RESET). It owns `out` and
// closes it on return.
//
// Kinds that don't advertise the watch verb (e.g. componentstatuses, some
// aggregated/legacy resources) never open a stream: the loop degrades to a
// periodic List+RESET (`listPollInterval`) so the rows still show instead of
// blanking the view (principle 3). If discovery's verbs are incomplete and the
// server itself rejects the watch as unsupported (405 MethodNotAllowed), the loop
// flips to that same list-only mode rather than paced-retrying the doomed watch.
func (c *Clients) watchLoop(ctx context.Context, client rest.Interface, r Resource, namespace string, opts metav1.ListOptions, out chan<- WatchEvent) {
	defer close(out)

	var (
		rv       string
		cols     []Column
		needList = true
		// listOnly: this kind can't be watched, so poll-refresh via List instead.
		// Only when the discovered verbs are *known* and lack "watch" — an empty
		// verb set is "unknown" (e.g. the seed menu, pre-discovery), so we still
		// try to watch and let the server's 405 (below) degrade it if it can't.
		listOnly = len(r.Verbs) > 0 && !hasVerb(r.Verbs, "watch")
	)
	for {
		if ctx.Err() != nil {
			return
		}

		if needList {
			tbl, listRV, err := getTableRV(ctx, client, r.GVR, r.Namespaced, namespace, opts)
			if err != nil {
				if !sendWatch(ctx, out, WatchEvent{Type: WatchError, Err: err}) {
					return
				}
				if !sleep(ctx, watchRetryBackoff) {
					return
				}
				continue
			}
			rv, cols = listRV, tbl.Columns
			if !sendWatch(ctx, out, WatchEvent{Type: WatchReset, Columns: cols, Rows: tbl.Rows}) {
				return
			}
			needList = false
		}

		if listOnly {
			// No watch stream to follow — re-List on an interval so the rows
			// stay fresh without a stream (and without a retry-looping error).
			if !sleep(ctx, listPollInterval) {
				return
			}
			needList = true
			continue
		}

		stream, err := openTableWatch(ctx, client, r.GVR, r.Namespaced, namespace, opts, rv)
		if err != nil {
			// Discovery verbs said watch was allowed but the server disagrees
			// (405): degrade to list-only polling rather than parade the error
			// and retry a watch that will never succeed.
			if apierrors.IsMethodNotSupported(err) {
				listOnly = true
				continue
			}
			if !sendWatch(ctx, out, WatchEvent{Type: WatchError, Err: err}) {
				return
			}
			needList = true
			if !sleep(ctx, watchRetryBackoff) {
				return
			}
			continue
		}

		newRV, newCols, werr := streamTableWatch(ctx, stream, cols, out)
		_ = stream.Close()
		if ctx.Err() != nil {
			return
		}
		if newRV != "" {
			rv = newRV
		}
		cols = newCols

		var exp *errExpired
		if errors.As(werr, &exp) {
			// resourceVersion too old: the server cannot resume from rv — resync.
			needList = true
		}
		if !sleep(ctx, watchRetryBackoff) {
			return
		}
	}
}

// openTableWatch opens a server-side Table watch stream from resourceVersion rv.
// It reuses tableRequest (identical endpoint + Accept header as List) and adds
// watch=true, allowWatchBookmarks=true (cheap resyncs), and the resourceVersion;
// opts is taken by value so these overrides never leak back to the caller.
func openTableWatch(
	ctx context.Context,
	client rest.Interface,
	gvr schema.GroupVersionResource,
	namespaced bool,
	namespace string,
	opts metav1.ListOptions,
	rv string,
) (io.ReadCloser, error) {
	opts.Watch = true
	opts.AllowWatchBookmarks = true
	opts.ResourceVersion = rv
	stream, err := tableRequest(client, gvr, namespaced, namespace, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("kube: watching %s: %w", gvr.Resource, err)
	}
	return stream, nil
}

// streamTableWatch decodes the stream of metav1.WatchEvent JSON objects the API
// server sends for a Table watch (watch=true) and pushes translated WatchEvents
// onto out until the stream ends, ctx is cancelled, or a decode/watch error
// occurs. cols carries the columns cached from the connection's first Table (the
// server may omit column defs on later chunks); the possibly-updated columns and
// the most recent resourceVersion are returned so the caller can resume. A clean
// end returns io.EOF; a watch ERROR event returns the decoded status error (an
// *errExpired for 410/Expired). Bookmark events only advance the resourceVersion.
func streamTableWatch(ctx context.Context, r io.Reader, cols []Column, out chan<- WatchEvent) (string, []Column, error) {
	dec := json.NewDecoder(r)
	lastRV := ""
	for {
		if err := ctx.Err(); err != nil {
			return lastRV, cols, err
		}

		var we metav1.WatchEvent
		if err := dec.Decode(&we); err != nil {
			return lastRV, cols, err // io.EOF on a clean end
		}

		switch watch.EventType(we.Type) {
		case watch.Error:
			return lastRV, cols, watchStatusError(we.Object.Raw)
		case watch.Bookmark:
			// A bookmark carries only an advanced resourceVersion, no row change.
			if _, rv, err := decodeTableRV(we.Object.Raw); err == nil && rv != "" {
				lastRV = rv
			}
			continue
		}

		tbl, rv, err := decodeTableRV(we.Object.Raw)
		if err != nil {
			return lastRV, cols, err
		}
		if rv != "" {
			lastRV = rv
		}
		if len(tbl.Columns) > 0 {
			cols = tbl.Columns
		}

		if !sendWatch(ctx, out, WatchEvent{
			Type:    WatchEventType(we.Type),
			Columns: cols,
			Rows:    tbl.Rows,
		}) {
			return lastRV, cols, ctx.Err()
		}
	}
}

// watchStatusError turns a watch ERROR event's embedded metav1.Status into a Go
// error, tagging 410 Gone / Expired as *errExpired so the loop knows to re-List
// rather than resume.
func watchStatusError(raw []byte) error {
	var st metav1.Status
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("kube: watch error event: %w", err)
	}
	if st.Reason == metav1.StatusReasonExpired || st.Code == 410 {
		return &errExpired{msg: fmt.Sprintf("kube: watch expired: %s", st.Message)}
	}
	return fmt.Errorf("kube: watch error: %s", st.Message)
}

// sendWatch delivers ev on out unless ctx is cancelled first; it returns false
// when the send is abandoned so callers can unwind promptly.
func sendWatch(ctx context.Context, out chan<- WatchEvent, ev WatchEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// sleep waits for d or ctx cancellation; it returns false if ctx was cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// getTableRV is getTable plus the listed resourceVersion — the exact point a
// watch must resume from. It shares tableRequest with getTable so List and the
// watch baseline hit an identical endpoint.
func getTableRV(
	ctx context.Context,
	client rest.Interface,
	gvr schema.GroupVersionResource,
	namespaced bool,
	namespace string,
	opts metav1.ListOptions,
) (*Table, string, error) {
	raw, err := tableRequest(client, gvr, namespaced, namespace, opts).Do(ctx).Raw()
	if err != nil {
		return nil, "", fmt.Errorf("kube: listing %s: %w", gvr.Resource, err)
	}
	return decodeTableRV(raw)
}
