package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// M1-INT-b-1: "the watch loop survives a transport drop", against a live apiserver.
//
// The hermetic suite (watch_test.go, M1-05b/D34) drives watchLoop with a fake REST
// client whose stream ends when the test says so, which proves the loop's *own*
// bookkeeping: a drop resumes from the last resourceVersion, a 410 re-lists. What a
// fake cannot prove is the half that lives on the other side of the wire — that a
// real kube-apiserver, handed the resourceVersion of the last delta kubecom saw,
// replays what happened while the connection was gone. If it did not, the resume
// path would silently lose rows: the TUI would show a table missing every change
// made during the outage, with no RESET to correct it and nothing in the logs.
//
// So this test breaks the *transport*, not the server: a raw TCP proxy sits between
// the client and the apiserver, the watch is established through it, the proxy kills
// every live connection mid-stream, and the assertion is that the loop reconnects and
// delivers a pod created while it was down — with no second RESET. The RESET matters
// as much as the delivery: a consumer treats RESET as "replace your whole row set"
// (see WatchEvent), so a needless one after every blip would rebuild the table, lose
// the cursor and re-render everything. Cheap reconnects are the point of resuming.
//
// Opt-in behind requireEnvtest (D18); not part of `make check`.

// M1-INT-b-2: "an expired resourceVersion forces a re-List", against a live apiserver.
//
// This is b-1's other half. b-1 proved the cheap recovery — a cut wire resumes from
// the last resourceVersion and costs no RESET. This proves the expensive one, which
// the loop must take when resuming is no longer possible: when the server can no
// longer replay from that resourceVersion it answers 410 Gone / Expired, and the only
// correct recovery is a fresh List and a RESET (see errExpired). Take the wrong branch
// here and kubecom resumes into a stream the server has already refused: the table
// silently stops updating, or — worse — the loop paces its way through a permanent
// error nobody sees.
//
// The hermetic test (watch_test.go, M1-05b/D34) drives that branch from a fake whose
// ERROR event the test itself wrote, so it can only prove watchLoop reacts to the
// shape kubecom *believes* an apiserver sends. Only a real server can prove the shape.
//
// Producing a real one is the whole difficulty, and it needs the plane configured
// (see the two flags below): left to itself, envtest never expires anything. Both
// tests share the killable proxy and the small helpers that follow.
//
// Opt-in behind requireEnvtest (D18); not part of `make check`.

const (
	// compactionInterval is how often the apiserver compacts etcd's revision
	// history. The default is 5m, which is why nothing expires inside a test: the
	// revision a watch holds stays replayable for longer than the plane lives.
	// Compaction drops everything below the revision seen one cycle earlier, so an
	// interval this short expires a held resourceVersion ~300 ms after the next
	// write — comfortably inside watchRetryBackoff, which is what lets the test
	// stale the loop's resourceVersion during a single reconnect gap.
	compactionInterval = "100ms"
	// watchCacheDisabled turns off the apiserver's in-memory watch cache so the
	// watch is served from etcd, where compaction is what bounds history. With the
	// cache on, the cache's own window bounds it instead — and that window cannot
	// be shrunk from a flag in any usable way: it starts at 100 events and *grows*
	// when it fills within 75 s, so the natural test (write past it) enlarges it
	// rather than evicting. Both paths answer an out-of-window watch with the same
	// 410/Expired status; this one is reachable in a second instead of minutes.
	watchCacheDisabled = "false"
)

// killableProxy is a raw TCP proxy that can drop every connection it is currently
// carrying. Raw TCP (not an HTTP proxy) so the client's TLS session terminates at
// the apiserver exactly as it normally would — the test only needs the ability to
// cut the pipe, and cutting it below TLS is the closest thing to a real network
// blip. It also counts dials, which is what makes the drop assertion non-vacuous.
type killableProxy struct {
	ln     net.Listener
	target string

	mu    sync.Mutex
	conns []net.Conn
	dials int
}

// startKillableProxy listens on a loopback port and forwards to target
// ("host:port"), registering its own shutdown with the test.
func startKillableProxy(t *testing.T, target string) *killableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := &killableProxy{ln: ln, target: target}
	go p.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return p
}

func (p *killableProxy) addr() string { return p.ln.Addr().String() }

func (p *killableProxy) serve() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return // listener closed by the test cleanup
		}
		server, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		p.mu.Lock()
		p.conns = append(p.conns, client, server)
		p.dials++
		p.mu.Unlock()
		go func() { _, _ = io.Copy(server, client) }()
		go func() { _, _ = io.Copy(client, server) }()
	}
}

// dropAll closes every connection the proxy is carrying, which the client sees as
// an unexpected EOF mid-stream — the transport drop this test is about.
func (p *killableProxy) dropAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// dialCount reports how many connections the proxy has accepted so far. A resume
// must re-dial, so this is the evidence that the drop actually happened and the
// stream that delivered the post-drop delta is a new one.
func (p *killableProxy) dialCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dials
}

// nextEvent takes one event off the watch channel or fails the test.
func nextEvent(t *testing.T, ch <-chan WatchEvent, within time.Duration, what string) WatchEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("watch channel closed while waiting for %s", what)
		}
		return ev
	case <-time.After(within):
		t.Fatalf("timed out after %s waiting for %s", within, what)
		return WatchEvent{}
	}
}

// rowNames lists the object names an event carries, for assertions and messages.
func rowNames(ev WatchEvent) []string {
	names := make([]string, 0, len(ev.Rows))
	for _, r := range ev.Rows {
		names = append(names, r.Object.Name)
	}
	return names
}

// createPod writes a minimal pod through the *direct* (unproxied) client, so the
// writer is never affected by the drop the test inflicts on the watcher.
func createPod(ctx context.Context, t *testing.T, c *Clients, name string) {
	t.Helper()
	_, err := c.Clientset.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod %s: %v", name, err)
	}
}

// TestEnvtestWatchResumesAfterTransportDrop kills the connection under a live Table
// watch and asserts the loop reconnects, resumes from its resourceVersion, and loses
// nothing that happened while it was down.
func TestEnvtestWatchResumesAfterTransportDrop(t *testing.T) {
	_, cfg := startControlPlane(t)
	admin := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// The watcher talks to the apiserver through the proxy. ServerName is carried
	// over from the real host so the apiserver's serving certificate still verifies
	// against the address it was issued for — the proxy is a pipe, not a MITM.
	apiURL, err := url.Parse(cfg.Host)
	if err != nil {
		t.Fatalf("parse control plane host %q: %v", cfg.Host, err)
	}
	proxy := startKillableProxy(t, apiURL.Host)
	proxiedCfg := rest.CopyConfig(cfg)
	proxiedCfg.Host = "https://" + proxy.addr()
	proxiedCfg.ServerName = apiURL.Hostname()
	watcher := clientsFor(t, proxiedCfg)

	pods := res("", "v1", "Pod", "pods", true)
	ch, err := watcher.Watch(ctx, pods, "default", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("start watch: %v", err)
	}

	// The stream opens with the baseline RESET…
	if ev := nextEvent(t, ch, time.Minute, "the initial RESET"); ev.Type != WatchReset {
		t.Fatalf("first event = %s (err %v), want RESET", ev.Type, ev.Err)
	}

	// …and is live: a write before the drop arrives as a single-row delta. Without
	// this the test could not tell a resumed stream from one that never worked.
	createPod(ctx, t, admin, "before-drop")
	ev := nextEvent(t, ch, 30*time.Second, "the pre-drop ADDED")
	if ev.Type != WatchAdded || len(ev.Rows) != 1 || ev.Rows[0].Object.Name != "before-drop" {
		t.Fatalf("pre-drop event = %s %v (err %v), want ADDED [before-drop]", ev.Type, rowNames(ev), ev.Err)
	}

	// Cut the wire, then change the cluster while the watcher is disconnected. The
	// pod is created *after* dropAll returns, so it cannot reach the old stream.
	dialsBefore := proxy.dialCount()
	proxy.dropAll()
	createPod(ctx, t, admin, "during-outage")

	// The loop paces reconnects (watchRetryBackoff), so allow a few attempts. Every
	// event until the delta arrives is inspected: a RESET among them would mean the
	// drop cost a full re-List, which is the regression this test exists to catch.
	deadline := time.Now().Add(90 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no ADDED for the pod created during the outage within 90s")
		}
		ev := nextEvent(t, ch, time.Until(deadline), "the post-drop ADDED")
		switch {
		case ev.Type == WatchReset:
			t.Fatalf("a transport drop re-listed: got RESET with %v, want a resumed stream "+
				"(a RESET makes the consumer replace its whole row set)", rowNames(ev))
		case ev.Type == WatchError:
			// Tolerated: reconnecting can report a transient failure. It must not
			// turn into a re-List, which the RESET case above still catches.
			t.Logf("transient watch error while reconnecting: %v", ev.Err)
		case ev.Type == WatchAdded && len(ev.Rows) == 1 && ev.Rows[0].Object.Name == "during-outage":
			// Resumed: the server replayed from the resourceVersion of the last
			// delta the loop saw, so nothing that happened while it was gone is lost.
			if got := proxy.dialCount(); got <= dialsBefore {
				t.Errorf("proxy dials = %d, was %d before the drop: the delta arrived "+
					"without a reconnect, so this test proved nothing", got, dialsBefore)
			}
			return
		default:
			t.Logf("ignoring %s %v while waiting for the resumed delta", ev.Type, rowNames(ev))
		}
	}
}

// firstWatchEventFrom opens a Table watch on pods from resourceVersion rv, reads the
// single first event and closes the stream again. It is how the test observes what
// the *server* answers for a given resourceVersion, independently of watchLoop —
// which is what turns "the loop re-listed" into "the loop re-listed because the
// server refused the resourceVersion it held".
func firstWatchEventFrom(ctx context.Context, t *testing.T, c *Clients, rv string) (metav1.WatchEvent, error) {
	t.Helper()
	pods := res("", "v1", "Pod", "pods", true)
	client, err := c.restClientForGV(pods.GVR.GroupVersion())
	if err != nil {
		return metav1.WatchEvent{}, err
	}
	stream, err := openTableWatch(ctx, client, pods.GVR, true, "default", metav1.ListOptions{}, rv)
	if err != nil {
		return metav1.WatchEvent{}, err
	}
	defer func() { _ = stream.Close() }()
	var we metav1.WatchEvent
	if err := json.NewDecoder(stream).Decode(&we); err != nil {
		return metav1.WatchEvent{}, err
	}
	return we, nil
}

// podsResourceVersion lists pods in `default` and returns the list's
// resourceVersion — the exact point a watch would resume from (getTableRV is what
// watchLoop itself uses to establish that baseline).
func podsResourceVersion(ctx context.Context, t *testing.T, c *Clients) string {
	t.Helper()
	pods := res("", "v1", "Pod", "pods", true)
	client, err := c.restClientForGV(pods.GVR.GroupVersion())
	if err != nil {
		t.Fatalf("rest client for pods: %v", err)
	}
	_, rv, err := getTableRV(ctx, client, pods.GVR, true, "default", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	return rv
}

// waitUntilExpired blocks until the server refuses a watch from rv with 410/Expired,
// writing a pod between attempts so etcd's revision keeps moving (compaction only
// ever drops history *below* a revision it has already seen, so a quiet cluster
// never expires anything). It fails the test if that does not happen in time —
// deliberately, because an unexpired resourceVersion makes the assertion that
// follows vacuous: the loop would simply resume, which is b-1's behavior, not this
// test's. Returns the status error the server produced.
func waitUntilExpired(ctx context.Context, t *testing.T, c *Clients, rv string, within time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(within)
	for i := 0; ; i++ {
		we, err := firstWatchEventFrom(ctx, t, c, rv)
		switch {
		case err != nil:
			t.Logf("probing rv %s: %v", rv, err)
		case watch.EventType(we.Type) == watch.Error:
			werr := watchStatusError(we.Object.Raw)
			var exp *errExpired
			if !errors.As(werr, &exp) {
				t.Fatalf("watch from rv %s failed with %v, want a 410/Expired the loop maps to *errExpired", rv, werr)
			}
			return werr
		}
		if time.Now().After(deadline) {
			t.Fatalf("resourceVersion %s was still replayable after %s of compaction: "+
				"the plane is not expiring anything, so this test cannot prove the re-List path", rv, within)
		}
		createPod(ctx, t, c, fmt.Sprintf("compaction-churn-%d", i))
		time.Sleep(50 * time.Millisecond)
	}
}

// TestEnvtestExpiredResourceVersionForcesReList stales the resourceVersion a live
// watch is holding — by cutting its connection and letting etcd compaction pass the
// held revision during the reconnect gap — and asserts the loop notices it cannot
// resume and re-syncs with a fresh List + RESET carrying everything it missed.
func TestEnvtestExpiredResourceVersionForcesReList(t *testing.T) {
	_, cfg := startControlPlane(t,
		withAPIServerFlag("etcd-compaction-interval", compactionInterval),
		withAPIServerFlag("watch-cache", watchCacheDisabled),
	)
	admin := clientsFor(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Same proxied setup as b-1: the watcher's wire can be cut, the writer's cannot.
	apiURL, err := url.Parse(cfg.Host)
	if err != nil {
		t.Fatalf("parse control plane host %q: %v", cfg.Host, err)
	}
	proxy := startKillableProxy(t, apiURL.Host)
	proxiedCfg := rest.CopyConfig(cfg)
	proxiedCfg.Host = "https://" + proxy.addr()
	proxiedCfg.ServerName = apiURL.Hostname()
	watcher := clientsFor(t, proxiedCfg)

	pods := res("", "v1", "Pod", "pods", true)
	ch, err := watcher.Watch(ctx, pods, "default", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("start watch: %v", err)
	}

	if ev := nextEvent(t, ch, time.Minute, "the initial RESET"); ev.Type != WatchReset {
		t.Fatalf("first event = %s (err %v), want RESET", ev.Type, ev.Err)
	}

	// A live delta first, so the loop is demonstrably streaming and its held
	// resourceVersion is the one this pod's event carried.
	createPod(ctx, t, admin, "before-outage")
	if ev := nextEvent(t, ch, 30*time.Second, "the pre-outage ADDED"); ev.Type != WatchAdded ||
		len(ev.Rows) != 1 || ev.Rows[0].Object.Name != "before-outage" {
		t.Fatalf("pre-outage event = %s %v (err %v), want ADDED [before-outage]", ev.Type, rowNames(ev), ev.Err)
	}

	// heldRV is taken after that delta, so it is at least the loop's own
	// resourceVersion. Compaction drops a contiguous prefix of history, so proving
	// heldRV expired proves the loop's (equal or older) one expired too.
	heldRV := podsResourceVersion(ctx, t, admin)

	// Cut the wire. The stream ends with an EOF, which is *not* an expiry, so the
	// loop keeps its resourceVersion and will try to resume from it in
	// watchRetryBackoff. That gap is the window this test needs.
	dialsBefore := proxy.dialCount()
	proxy.dropAll()
	createPod(ctx, t, admin, "during-outage")

	// Stale the held resourceVersion before the loop gets to reuse it.
	expiry := waitUntilExpired(ctx, t, admin, heldRV, watchRetryBackoff)
	t.Logf("server refuses rv %s: %v", heldRV, expiry)

	// The loop now reconnects into a resourceVersion the server will not replay.
	// The only correct answer is a fresh List and a RESET holding the whole current
	// set — including the pod written while it was disconnected.
	// The re-sync costs one backoff plus the List, so ~4 s; the margin is for a
	// loaded machine, not for the loop to find its way there.
	deadline := time.Now().Add(60 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no RESET after the resourceVersion expired, within 60s")
		}
		ev := nextEvent(t, ch, time.Until(deadline), "the post-expiry RESET — a silent channel here "+
			"is a loop re-opening a watch the server has already refused, which is the failure "+
			"mode a user experiences as a table that stopped updating")
		switch {
		case ev.Type == WatchAdded && len(ev.Rows) == 1 && ev.Rows[0].Object.Name == "during-outage":
			t.Fatalf("the loop resumed from an expired resourceVersion: got ADDED [during-outage] " +
				"instead of a RESET, so the re-List branch was not taken")
		case ev.Type == WatchError:
			// Tolerated: reconnecting can report a transient failure. It must still
			// end in the RESET this loop is waiting for.
			t.Logf("transient watch error while re-syncing: %v", ev.Err)
		case ev.Type == WatchReset:
			names := rowNames(ev)
			for _, want := range []string{"before-outage", "during-outage"} {
				if !contains(names, want) {
					t.Errorf("post-expiry RESET rows = %v, missing %q: a re-List that loses rows "+
						"is worse than no re-List at all", names, want)
				}
			}
			if len(ev.Columns) == 0 {
				t.Errorf("post-expiry RESET carried no columns; a consumer replacing its row set needs them")
			}
			if got := proxy.dialCount(); got <= dialsBefore {
				t.Errorf("proxy dials = %d, was %d before the drop: the RESET arrived without a "+
					"reconnect, so this test proved nothing", got, dialsBefore)
			}
			return
		default:
			t.Logf("ignoring %s %v while waiting for the re-sync", ev.Type, rowNames(ev))
		}
	}
}

// contains reports whether names holds want.
func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
