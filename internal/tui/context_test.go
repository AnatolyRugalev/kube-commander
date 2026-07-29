package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// fakeConnector is a hermetic ClusterConnector: it records the context names it was
// asked to connect to and hands back a preset bundle or error. The real one
// (cmd/kubecom's contextConnector) calls kube.Connect, which needs a kubeconfig on
// disk — the seam exists precisely so the switch is testable without one (D18).
type fakeConnector struct {
	names   []string
	cluster Cluster
	err     error
}

func (f *fakeConnector) ConnectCluster(name string) (Cluster, error) {
	f.names = append(f.names, name)
	return f.cluster, f.err
}

// newClusterFake builds a distinguishable cluster bundle plus the two seams a test
// watches for identity: the watcher (proves the bundle was repointed) and the
// discoverer (proves discovery restarted against the *new* cluster, not the old one).
func newClusterFake() (Cluster, *fakeWatcher, *fakeDiscoverer) {
	w := &fakeWatcher{}
	d := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	return Cluster{watcher: w, discoverer: d}, w, d
}

// TestSwitchContextConnectsOffTheUpdateLoop pins the ordering the whole slice turns
// on: switchContext issues the connect as a Cmd and returns, so the update loop is
// never blocked on a kubeconfig read, and — crucially — nothing is torn down yet. The
// reset is destructive, so it may only run once the new client is in hand (D155 pt 1).
func TestSwitchContextConnectsOffTheUpdateLoop(t *testing.T) {
	fc := &fakeConnector{}
	fc.cluster, _, _ = newClusterFake()
	fw := &fakeWatcher{}
	m := browsingModel(t, fw, WithClusterConnector(fc), WithContext("dev"))

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	if cmd == nil {
		t.Fatal("switchContext should issue the connect as a Cmd")
	}
	if len(fc.names) != 0 {
		t.Errorf("connect ran on the update loop: %v", fc.names)
	}
	if !m.hasCurrent || fw.ctxs[0].Err() != nil {
		t.Error("nothing may be torn down before the new cluster is in hand")
	}

	msg, ok := cmd().(clusterConnectedMsg)
	if !ok {
		t.Fatalf("the connect Cmd should deliver a clusterConnectedMsg, got %T", cmd())
	}
	if fc.names == nil || fc.names[0] != "prod" {
		t.Errorf("connected to %v, want [prod]", fc.names)
	}
	if msg.context != "prod" || msg.gen != m.ctxGen {
		t.Errorf("got %q gen %d, want %q gen %d", msg.context, msg.gen, "prod", m.ctxGen)
	}
}

// TestSwitchContextIsInertWhereItShouldBe covers the two no-ops: a model with no
// connector wired (every hermetic test, and any build where the launcher does not
// supply one) and a switch to the context already live — tearing a working cluster
// down to rebuild the same one buys nothing, and M4-04b's picker marks the current
// entry rather than hiding it, so it can be chosen.
func TestSwitchContextIsInertWhereItShouldBe(t *testing.T) {
	fc := &fakeConnector{}
	for _, tc := range []struct {
		name   string
		model  Model
		target string
	}{
		{"no connector", sizedWith(t, WithContext("dev")), "prod"},
		{"already current", sizedWith(t, WithClusterConnector(fc), WithContext("dev")), "dev"},
		{"empty name", sizedWith(t, WithClusterConnector(fc), WithContext("dev")), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, cmd := tc.model.switchContext(tc.target)
			if cmd != nil {
				t.Error("no connect should be issued")
			}
			if got := next.(Model).ctxGen; got != 0 {
				t.Errorf("an inert switch must not burn a generation, got %d", got)
			}
		})
	}
	if len(fc.names) != 0 {
		t.Errorf("no connect should have run: %v", fc.names)
	}
}

// TestConnectFailureLeavesTheCurrentClusterUntouched is the reason the connect comes
// first: an unreachable or misconfigured context must cost nothing. The shell keeps
// browsing what it was browsing — same context, same live watch, same rows — and the
// failure degrades to a transient toast (principle 3).
func TestConnectFailureLeavesTheCurrentClusterUntouched(t *testing.T) {
	fc := &fakeConnector{err: errors.New("no such context")}
	fw := &fakeWatcher{}
	m := browsingModel(t, fw, WithClusterConnector(fc), WithContext("dev"))
	rows := m.table.TotalRowCount()

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, cmd = m.Update(cmd())
	m = next.(Model)

	if cmd == nil || !m.status.HasError() {
		t.Error("a connect failure should surface as a transient status-bar toast")
	}
	if m.context != "dev" {
		t.Errorf("the context must not change on a failed connect, got %q", m.context)
	}
	if m.watcher != fw {
		t.Error("the cluster bundle must not be repointed on a failed connect")
	}
	if fw.ctxs[0].Err() != nil {
		t.Error("the live watch on the current cluster must survive a failed connect")
	}
	if !m.hasCurrent || m.table.TotalRowCount() != rows {
		t.Error("the browse panes must be exactly as they were before the attempt")
	}
}

// TestClusterConnectedResetsSwapsAndRediscovers is the leg's headline: with the new
// client in hand the switch tears the old cluster down (M4-03), repoints the one
// bundle (M4-02), renames the context everywhere it shows, and rediscovers — in that
// order. Discovery running against the *new* cluster's discoverer is what proves the
// swap happens before the restart rather than after it.
func TestClusterConnectedResetsSwapsAndRediscovers(t *testing.T) {
	newCluster, newFW, newFD := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	oldFW := &fakeWatcher{}
	oldFD := &fakeDiscoverer{ch: make(chan kube.DiscoveryResult, 1)}
	m := browsingModel(t, oldFW, WithClusterConnector(fc), WithDiscoverer(oldFD), WithContext("dev"))

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	if oldFW.ctxs[0].Err() == nil {
		t.Error("the departed cluster's watch must be cancelled by the switch")
	}
	if m.watcher != newFW {
		t.Error("the cluster bundle should be repointed at the new cluster's seams")
	}
	if len(newFD.ctxs) != 1 {
		t.Errorf("discovery should restart against the new cluster's discoverer, ran %d times", len(newFD.ctxs))
	}
	if len(oldFD.ctxs) != 0 {
		t.Error("discovery must not re-run against the cluster that was left")
	}
	if !m.status.Discovering() {
		t.Error("the status bar should show the new cluster's discovery pass running")
	}
	if m.context != "prod" {
		t.Errorf("model context = %q, want prod", m.context)
	}
	if !strings.Contains(m.status.View(), "prod") {
		t.Errorf("the status bar should name the new context:\n%s", m.status.View())
	}
	if m.hasCurrent || m.table.TotalRowCount() != 0 || m.namespace != "" {
		t.Error("the browse panes should be back to their pre-drill-in state on the new cluster")
	}
}

// TestStaleConnectResultIsDropped guards the generation check. Two switches in quick
// succession (a mis-pick corrected immediately) race: the first connect can land
// after the second, and applying it would drop the user on a cluster they already
// moved off — with the second switch's teardown already done. Cancelling is not
// available for a single call, so the generation is the whole guard (D156 pt 2).
func TestStaleConnectResultIsDropped(t *testing.T) {
	newCluster, _, newFD := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	fw := &fakeWatcher{}
	m := browsingModel(t, fw, WithClusterConnector(fc), WithContext("dev"))

	next, first := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.switchContext("staging") // supersedes it before the first lands.
	m = next.(Model)

	stale := first()
	next, cmd := m.Update(stale)
	m = next.(Model)

	if cmd != nil {
		t.Error("a superseded connect result must not schedule any work")
	}
	if m.context != "dev" {
		t.Errorf("a superseded connect result must not switch the shell, context = %q", m.context)
	}
	if m.watcher != fw {
		t.Error("a superseded connect result must not repoint the cluster bundle")
	}
	if len(newFD.ctxs) != 0 {
		t.Error("a superseded connect result must not start discovery")
	}
}
