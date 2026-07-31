package kube

import (
	"context"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
