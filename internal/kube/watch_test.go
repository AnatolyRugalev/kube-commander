package kube

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	restfake "k8s.io/client-go/rest/fake"
)

// A watch chunk: a metav1.WatchEvent whose object is a one-row Table. Column
// definitions are intentionally omitted to exercise column caching (the server
// only guarantees them on the first response of a connection).
const modifiedEventJSON = `{"type":"MODIFIED","object":{
  "kind":"Table","apiVersion":"meta.k8s.io/v1",
  "metadata":{"resourceVersion":"1005"},
  "rows":[{"cells":["nginx-abc","Running","10.0.0.9"],
    "object":{"kind":"PartialObjectMetadata","metadata":{"name":"nginx-abc","namespace":"web","uid":"uid-1"}}}]
}}`

const addedEventJSON = `{"type":"ADDED","object":{
  "kind":"Table","apiVersion":"meta.k8s.io/v1",
  "metadata":{"resourceVersion":"1006"},
  "columnDefinitions":[{"name":"Name","type":"string","format":"name"},{"name":"Status","type":"string"}],
  "rows":[{"cells":["redis-xyz","Pending"],
    "object":{"kind":"PartialObjectMetadata","metadata":{"name":"redis-xyz","namespace":"web","uid":"uid-9"}}}]
}}`

// drain collects every WatchEvent from ch until it is closed.
func drain(ch <-chan WatchEvent) []WatchEvent {
	var got []WatchEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

func TestStreamTableWatchDeltas(t *testing.T) {
	// A stream of two events; the first (MODIFIED) omits columns, so the columns
	// passed in from the prior List must be carried onto it. The second (ADDED)
	// re-declares columns and they replace the cached set.
	stream := strings.NewReader(modifiedEventJSON + addedEventJSON)
	out := make(chan WatchEvent, 8)

	cachedCols := []Column{{Name: "Name", Format: "name"}, {Name: "Status"}}
	rv, cols, err := streamTableWatch(context.Background(), stream, cachedCols, out)
	close(out)
	if err != io.EOF {
		t.Fatalf("err = %v, want io.EOF (clean end)", err)
	}
	if rv != "1006" {
		t.Errorf("resourceVersion = %q, want 1006 (latest seen)", rv)
	}
	if len(cols) != 2 || cols[0].Name != "Name" {
		t.Errorf("returned cols = %+v, want 2 cols starting with Name", cols)
	}

	got := drain(out)
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if got[0].Type != WatchModified {
		t.Errorf("event0 type = %q, want MODIFIED", got[0].Type)
	}
	// Columns omitted on the MODIFIED chunk must be filled from the cache.
	if len(got[0].Columns) != 2 || got[0].Columns[1].Name != "Status" {
		t.Errorf("event0 columns = %+v, want cached [Name Status]", got[0].Columns)
	}
	if len(got[0].Rows) != 1 || got[0].Rows[0].Object.Name != "nginx-abc" {
		t.Errorf("event0 rows = %+v, want single nginx-abc row", got[0].Rows)
	}
	if got[1].Type != WatchAdded || got[1].Rows[0].Object.UID != "uid-9" {
		t.Errorf("event1 = %+v, want ADDED redis-xyz(uid-9)", got[1])
	}
}

func TestStreamTableWatchBookmarkAdvancesRV(t *testing.T) {
	// A bookmark carries only an advanced resourceVersion and must emit no event.
	const bookmark = `{"type":"BOOKMARK","object":{"kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{"resourceVersion":"2000"}}}`
	out := make(chan WatchEvent, 4)
	rv, _, err := streamTableWatch(context.Background(), strings.NewReader(bookmark), nil, out)
	close(out)
	if err != io.EOF {
		t.Fatalf("err = %v, want io.EOF", err)
	}
	if rv != "2000" {
		t.Errorf("resourceVersion = %q, want 2000 (advanced by bookmark)", rv)
	}
	if got := drain(out); len(got) != 0 {
		t.Errorf("events = %d, want 0 (bookmark is not a row delta)", len(got))
	}
}

func TestStreamTableWatchExpired(t *testing.T) {
	// A 410 Gone ERROR event must surface as *errExpired so the loop re-Lists.
	const expired = `{"type":"ERROR","object":{"kind":"Status","apiVersion":"v1","status":"Failure","message":"too old resource version","reason":"Expired","code":410}}`
	out := make(chan WatchEvent, 1)
	_, _, err := streamTableWatch(context.Background(), strings.NewReader(expired), nil, out)
	close(out)
	var exp *errExpired
	if !errors.As(err, &exp) {
		t.Fatalf("err = %v, want *errExpired", err)
	}
	if got := drain(out); len(got) != 0 {
		t.Errorf("events = %d, want 0 for an ERROR event", len(got))
	}
}

func TestStreamTableWatchGenericError(t *testing.T) {
	// A non-410 ERROR event is a plain error, not an *errExpired (no re-List forced).
	const boom = `{"type":"ERROR","object":{"kind":"Status","apiVersion":"v1","status":"Failure","message":"boom","reason":"InternalError","code":500}}`
	_, _, err := streamTableWatch(context.Background(), strings.NewReader(boom), nil, make(chan WatchEvent, 1))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var exp *errExpired
	if errors.As(err, &exp) {
		t.Fatalf("err = %v classified as expired; a 500 must not force a re-List", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to carry the status message", err)
	}
}

func TestOpenTableWatchRequest(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	client := newTableRESTClient(gvr.GroupVersion(), "/api/v1", modifiedEventJSON)

	stream, err := openTableWatch(context.Background(), client, gvr, true, "web", metav1.ListOptions{}, "42")
	if err != nil {
		t.Fatalf("openTableWatch: %v", err)
	}
	_ = stream.Close()

	if client.Req == nil {
		t.Fatal("no request recorded")
	}
	if got := client.Req.Header.Get("Accept"); got != tableAcceptHeader {
		t.Errorf("Accept = %q, want %q", got, tableAcceptHeader)
	}
	q := client.Req.URL.Query()
	if got := q.Get("watch"); got != "true" {
		t.Errorf("watch = %q, want true", got)
	}
	if got := q.Get("resourceVersion"); got != "42" {
		t.Errorf("resourceVersion = %q, want 42", got)
	}
	if got := q.Get("allowWatchBookmarks"); got != "true" {
		t.Errorf("allowWatchBookmarks = %q, want true", got)
	}
	if got, want := client.Req.URL.Path, "/api/v1/namespaces/web/pods"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestGetTableRVReturnsResourceVersion(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	const listJSON = `{"kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{"resourceVersion":"777"},
	  "columnDefinitions":[{"name":"Name","type":"string"}],"rows":[{"cells":["a"]}]}`
	client := newTableRESTClient(gvr.GroupVersion(), "/api/v1", listJSON)

	tbl, rv, err := getTableRV(context.Background(), client, gvr, true, "web", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("getTableRV: %v", err)
	}
	if rv != "777" {
		t.Errorf("rv = %q, want 777", rv)
	}
	if len(tbl.Rows) != 1 {
		t.Errorf("rows = %d, want 1", len(tbl.Rows))
	}
}

// TestWatchLoopListThenWatch drives the full Watch goroutine over a fake
// transport that answers a List (no watch param) with a two-row Table and a
// Watch (watch=true) with one MODIFIED delta then EOF. It asserts the consumer
// sees a RESET carrying the listed rows followed by the delta, then cancels to
// unwind the loop — proving the List→Watch→channel wiring end to end without a
// live server.
func TestWatchLoopListThenWatch(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	client := &restfake.RESTClient{
		NegotiatedSerializer: scheme.Codecs,
		GroupVersion:         gvr.GroupVersion(),
		VersionedAPIPath:     "/api/v1",
		Client: restfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			body := podsTableJSON // List baseline
			if req.URL.Query().Get("watch") == "true" {
				body = modifiedEventJSON // one delta, then the body EOFs
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			}, nil
		}),
	}

	c := &Clients{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan WatchEvent, watchChanBuffer)
	go c.watchLoop(ctx, client, Resource{GVR: gvr, Namespaced: true}, "web", metav1.ListOptions{}, out)

	reset := recvEvent(t, out)
	if reset.Type != WatchReset {
		t.Fatalf("first event = %q, want RESET", reset.Type)
	}
	if len(reset.Rows) != 2 || len(reset.Columns) != 3 {
		t.Fatalf("RESET carried %d rows / %d cols, want 2 rows / 3 cols from the List", len(reset.Rows), len(reset.Columns))
	}

	delta := recvEvent(t, out)
	if delta.Type != WatchModified || delta.Rows[0].Object.Name != "nginx-abc" {
		t.Fatalf("second event = %+v, want MODIFIED nginx-abc", delta)
	}
	// Columns omitted on the delta must be carried from the RESET.
	if len(delta.Columns) != 3 {
		t.Errorf("delta columns = %d, want 3 (carried from RESET)", len(delta.Columns))
	}

	cancel() // unwind: the loop's backoff sleep returns immediately on cancel.
	if _, ok := <-out; ok {
		// Drain any already-buffered events, then require closure.
		for range out { //nolint:revive // draining to closure
		}
	}
}

// recvEvent reads one event or fails if none arrives promptly.
func recvEvent(t *testing.T, ch <-chan WatchEvent) WatchEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a watch event")
		return WatchEvent{}
	}
}
