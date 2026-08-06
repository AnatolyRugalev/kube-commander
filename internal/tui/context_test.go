package tui

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
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
	// The bundle carries a namespace lister because a switched-to cluster has one:
	// since PAL-05c-1 it is what makes ctrl+n open the palette's `:namespace ` stage,
	// so a test that picks a namespace after a switch drives the real surface.
	return Cluster{watcher: w, discoverer: d, nsLister: &fakeLister{ns: []string{"kube-system"}}}, w, d
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
	next, cmd = m.Update(pickerMsg(t, cmd))
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
	next, _ = m.Update(pickerMsg(t, cmd))
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

// fakeContextLister is a hermetic ContextLister: it counts how many times it was
// asked and hands back a preset list or error. The real one (cmd/kubecom's
// contextLister) reads a kubeconfig off disk — the seam exists so the picker is
// testable without one (D18).
type fakeContextLister struct {
	contexts []kube.ContextInfo
	err      error
	calls    int
}

func (f *fakeContextLister) Contexts() ([]kube.ContextInfo, error) {
	f.calls++
	return f.contexts, f.err
}

// twoContexts is the list the `:context ` stage tests are seeded with. `dev` is
// flagged Current on purpose while the tests run a shell that is on `prod`: the
// kubeconfig's current-context is the one the reader *launched* from, and kubecom never
// rewrites it, so it is precisely the wrong thing to mark (D158).
func twoContexts() []kube.ContextInfo {
	return []kube.ContextInfo{
		{Name: "dev", Cluster: "dev", Current: true},
		{Name: "prod", Cluster: "gke_prod_eu"},
	}
}

// capitalC is the ctx.switch default key.
var capitalC = tea.Key{Code: 'C', Text: "C"}

// openContextStage presses `C` through the real key path (D11 — the binding is
// registry-resolved, never matched raw) and delivers the context listing the stage asks
// for, returning the shell with the palette on its seeded `:context ` stage. Since
// PAL-05c-2 that is the only context-switching surface there is, so every test that used
// to open the standalone picker reaches it this way — the way a reader does, rather than
// by calling an opener.
func openContextStage(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := press(t, m, capitalC)
	if !m.cmdPicker.Active() || m.palArg != keymap.ActionContext {
		t.Fatalf("`C` should open the palette on its context stage, stage = %q", m.palArg)
	}
	next, _ := m.Update(pickerMsg(t, cmd))
	return next.(Model)
}

// pickerLabelFor reads the open stage's rows back out through the label map, which is
// the only thing a pick can resolve through — a SelectedMsg carries a label, not a
// context (D65).
func pickerLabelFor(t *testing.T, m Model, context string) string {
	t.Helper()
	for label, name := range m.ctxByLabel {
		if name == context {
			return label
		}
	}
	t.Fatalf("no stage row maps to context %q (rows: %v)", context, m.ctxByLabel)
	return ""
}

// TestContextKeyOpensThePaletteContextStage is PAL-05c-2's headline assertion: `C` no
// longer opens a modal of its own, it opens the one palette with the context verb
// already committed. Like ctrl+n the values are not in hand, so the stage opens empty
// and titled as loading (PAL-03b) and the kubeconfig read happens off the update loop —
// the difference from a stage whose values were compiled in is invisible in the line,
// which is the point.
func TestContextKeyOpensThePaletteContextStage(t *testing.T) {
	fl := &fakeContextLister{contexts: twoContexts()}
	m := sizedWith(t, WithContextLister(fl), WithContext("prod"))

	m, cmd := press(t, m, capitalC)
	if !m.cmdPicker.Active() {
		t.Fatal("ctx.switch should open the command palette")
	}
	if m.palArg != keymap.ActionContext {
		t.Fatalf("`C` should commit the context verb, stage = %q", m.palArg)
	}
	if cmd == nil {
		t.Fatal("the listing should be issued as a Cmd")
	}
	if fl.calls != 0 {
		t.Errorf("the kubeconfig was read on the update loop (%d calls)", fl.calls)
	}

	next, _ := m.Update(pickerMsg(t, cmd))
	m = next.(Model)
	if fl.calls != 1 {
		t.Errorf("Contexts called %d times, want 1", fl.calls)
	}
	if len(m.ctxByLabel) != 2 {
		t.Errorf("stage rows = %d, want 2 (%v)", len(m.ctxByLabel), m.ctxByLabel)
	}
	if !m.cmdPicker.Active() {
		t.Error("the stage should stay open once the rows land")
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, palettePrompt+"context ") {
		t.Errorf("the key should land on the palette's pre-typed line:\n%s", view)
	}

	// The key is sugar, not a second surface: typing the line by hand must land on the
	// very same stage (D207) — same verb committed, same prompt, same rows.
	typed := sizedWith(t, WithContextLister(&fakeContextLister{contexts: twoContexts()}), WithContext("prod"))
	typed, _ = press(t, typed, colon)
	typed = typeInto(t, typed, "context")
	typed, spaceCmd := press(t, typed, tea.Key{Code: ' ', Text: " "})
	next, _ = typed.Update(pickerMsg(t, spaceCmd))
	typed = next.(Model)
	if got, want := stripANSI(typed.View().Content), view; got != want {
		t.Errorf("`C` and `:context ` should open the same stage:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestContextStageMarksTheShellsContextNotTheKubeconfigs is the D158 assertion, and the
// one thing PAL-05c-2 carried that no other converted key had. The list flags `dev` as
// the kubeconfig's current-context while the shell is on `prod`; the marker must follow
// the shell. Getting this backwards is invisible at launch (they agree) and wrong after
// every switch, which is exactly when a reader reaches for the switcher to check where
// they are. It lives in contextPickerItems, which the stage was already seeded from, so
// the conversion moved it nowhere.
func TestContextStageMarksTheShellsContextNotTheKubeconfigs(t *testing.T) {
	fl := &fakeContextLister{contexts: twoContexts()}
	m := openContextStage(t, sizedWith(t, WithContextLister(fl), WithContext("prod")))

	if got := pickerLabelFor(t, m, "prod"); !strings.HasPrefix(got, "* ") {
		t.Errorf("the shell's own context should be marked, row = %q", got)
	}
	if got := pickerLabelFor(t, m, "dev"); strings.HasPrefix(got, "* ") {
		t.Errorf("the kubeconfig's current-context must not be marked, row = %q", got)
	}
	// The cluster name is shown only where it adds something: `prod` points at
	// `gke_prod_eu`, `dev` at a cluster of its own name.
	if got := pickerLabelFor(t, m, "prod"); !strings.Contains(got, "(gke_prod_eu)") {
		t.Errorf("a differing cluster name should be shown, row = %q", got)
	}
	if got := pickerLabelFor(t, m, "dev"); strings.Contains(got, "(") {
		t.Errorf("a cluster name equal to the context name should be elided, row = %q", got)
	}
}

// TestContextPickRoutesIntoSwitchContext closes the loop M4-04b exists to close: a
// picked row resolves back to its context name and reaches switchContext (M4-04a), which
// connects. The stage closes and drops its label map either way — and the map is read
// *after* the close (applyPaletteArg closes first), which is why closePalette must not
// clear it.
func TestContextPickRoutesIntoSwitchContext(t *testing.T) {
	fl := &fakeContextLister{contexts: twoContexts()}
	fc := &fakeConnector{}
	fc.cluster, _, _ = newClusterFake()
	m := openContextStage(t, sizedWith(t, WithContextLister(fl), WithClusterConnector(fc), WithContext("prod")))
	label := pickerLabelFor(t, m, "dev")

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: label})
	m = next.(Model)
	if m.cmdPicker.Active() || m.ctxByLabel != nil {
		t.Error("the stage should close and drop its label map on a pick")
	}
	if cmd == nil {
		t.Fatal("a pick should issue the connect Cmd")
	}
	if _, ok := cmd().(clusterConnectedMsg); !ok {
		t.Fatalf("a pick should route into switchContext, got %T", cmd())
	}
	if len(fc.names) != 1 || fc.names[0] != "dev" {
		t.Errorf("connected to %v, want [dev] — the label must resolve back to a context name", fc.names)
	}
}

// TestPickingTheCurrentContextCostsNothing: the marked row is choosable rather than
// hidden, so choosing it must be a no-op — not a teardown and rebuild of the working
// cluster (D157's scope note). The stage still closes, as any pick does.
func TestPickingTheCurrentContextCostsNothing(t *testing.T) {
	fl := &fakeContextLister{contexts: twoContexts()}
	fc := &fakeConnector{}
	fw := &fakeWatcher{}
	m := openContextStage(t, browsingModel(t, fw, WithContextLister(fl), WithClusterConnector(fc), WithContext("prod")))

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: pickerLabelFor(t, m, "prod")})
	m = next.(Model)
	if cmd != nil {
		t.Error("picking the live context should issue no work")
	}
	if len(fc.names) != 0 {
		t.Errorf("picking the live context must not reconnect: %v", fc.names)
	}
	if m.ctxGen != 0 {
		t.Errorf("picking the live context must not burn a generation, got %d", m.ctxGen)
	}
	if !m.hasCurrent || fw.ctxs[0].Err() != nil {
		t.Error("picking the live context must not disturb the running cluster")
	}
	if m.cmdPicker.Active() {
		t.Error("the stage should close on a pick, no-op or not")
	}
}

// TestContextListingDegradesRatherThanBlocking covers the two ways the list can be
// unusable. Both close the palette and say something — an empty stage a reader can only
// escape from is the failure mode this avoids (principle 3).
func TestContextListingDegradesRatherThanBlocking(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lister   *fakeContextLister
		wantErr  bool
		wantNote bool
	}{
		{"unreadable kubeconfig", &fakeContextLister{err: errors.New("no kubeconfig")}, true, false},
		{"no contexts declared", &fakeContextLister{}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sizedWith(t, WithContextLister(tc.lister), WithContext("prod"))
			m, cmd := press(t, m, capitalC)
			next, cmd := m.Update(pickerMsg(t, cmd))
			m = next.(Model)
			if m.cmdPicker.Active() {
				t.Error("the stage should close rather than sit empty")
			}
			if cmd == nil {
				t.Fatal("the failure should say something rather than close silently")
			}
			if tc.wantErr {
				// An error toast arrives as its own ErrorMsg; a notice is written
				// straight onto the status bar (its Cmd is only the auto-clear
				// timer, which would sleep out the display window if run here).
				next, _ = m.Update(pickerMsg(t, cmd))
				m = next.(Model)
			}
			if got := m.status.HasError(); got != tc.wantErr {
				t.Errorf("status error = %v, want %v", got, tc.wantErr)
			}
			if got := m.status.HasNotice(); got != tc.wantNote {
				t.Errorf("status notice = %v, want %v", got, tc.wantNote)
			}
		})
	}
}

// TestContextSwitchInertWithoutALister: no seam wired (every hermetic test, and any
// build whose launcher supplies none) means the gesture opens nothing at all rather than
// an empty stage — the ns.switch precedent. Since PAL-05c-2 the check that makes the key
// inert is enterPaletteArg's, the same one that makes the typed `:context ` line inert,
// so the two cannot drift apart.
func TestContextSwitchInertWithoutALister(t *testing.T) {
	m := sizedWith(t, WithContext("prod"))
	m, cmd := press(t, m, capitalC)
	if m.cmdPicker.Active() {
		t.Fatal("ctx.switch without a lister should not open the palette")
	}
	if m.palArg != "" {
		t.Fatalf("an inert verb should not commit a stage, stage = %q", m.palArg)
	}
	if cmd != nil {
		t.Error("ctx.switch should be inert with no context lister wired")
	}
}

// TestLateContextListIsDropped: the listing is not generation-tagged (it describes the
// kubeconfig, not a cluster, so it cannot go stale), which leaves exactly one guard to
// get right, and it is the guard the collapsed `dest` left behind (D208 pt 3) — *which*
// surface asked stopped being a question, *whether that stage is still up* did not. Both
// ways out of the stage are covered: rewinding the line to the verbs must not paint
// contexts over them, and closing the palette outright must not resurrect it or seed
// rows behind it.
func TestLateContextListIsDropped(t *testing.T) {
	t.Run("rewound to the verbs", func(t *testing.T) {
		fl := &fakeContextLister{contexts: twoContexts()}
		m := sizedWith(t, WithContextLister(fl), WithContext("prod"))

		m, cmd := press(t, m, capitalC)
		// Backspace on the empty argument rewinds to the verb list (D207 pt 2).
		m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
		if m.palArg != "" {
			t.Fatalf("backspace should rewind to the verbs, stage = %q", m.palArg)
		}
		verbs := m.cmdPicker.Len()

		next, _ := m.Update(pickerMsg(t, cmd))
		m = next.(Model)
		if got := m.cmdPicker.Len(); got != verbs {
			t.Fatalf("a late listing seeded the rewound verb list: %d rows, want %d", got, verbs)
		}
		if m.ctxByLabel != nil {
			t.Errorf("a late listing must not seed rows behind the verbs: %v", m.ctxByLabel)
		}
	})

	t.Run("palette closed", func(t *testing.T) {
		fl := &fakeContextLister{contexts: twoContexts()}
		m := sizedWith(t, WithContextLister(fl), WithContext("prod"))

		m, cmd := press(t, m, capitalC)
		// One nav.back closes a key-opened stage — there is no verb list behind `C`
		// to rewind to (D233).
		next, _ := m.Update(picker.CancelledMsg{Kind: commandPickerKind})
		m = next.(Model)
		if m.cmdPicker.Active() {
			t.Fatal("nav.back should close the key-opened palette")
		}

		next, _ = m.Update(pickerMsg(t, cmd))
		m = next.(Model)
		if m.cmdPicker.Active() {
			t.Error("a listing landing after dismissal must not reopen the palette")
		}
		if m.ctxByLabel != nil {
			t.Errorf("a listing landing after dismissal must not seed rows: %v", m.ctxByLabel)
		}
	})
}

// fakeStateLoader is a hermetic ContextStateLoader: it records the context names it
// was asked for and hands back one preset state. The real one (cmd/kubecom's
// contextStateLoader) reads the per-context menu and state files off disk — the seam
// exists so a switch's rebind is testable without them (D18).
type fakeStateLoader struct {
	names []string
	state ContextState
}

func (f *fakeStateLoader) LoadContextState(name string) ContextState {
	f.names = append(f.names, name)
	return f.state
}

// certExtra is a per-context menu entry the seed menu does not know, used to tell
// one context's menu additions from another's.
func certExtra(resource string) config.MenuResource {
	return config.MenuResource{
		Group: "cert-manager.io", Version: "v1", Resource: resource,
		Kind: "Certificate", Namespaced: true,
	}
}

// menuHasResource reports whether the menu lists an entry for the given plural
// resource name — how a test tells whose menu extras the rebuilt menu carries.
func menuHasResource(m Model, resource string) bool {
	for _, it := range m.menu.Items() {
		if it.Resource.GVR.Resource == resource {
			return true
		}
	}
	return false
}

// TestSwitchRebindsPerContextState is M4-05's headline: the state that is keyed by
// the *context* — its menu extras, its last-used namespace and the state file a
// namespace is persisted to — follows the switch. Before this, a switch kept the
// launch context's menu additions and wrote the new cluster's namespace choices into
// the old context's state file.
func TestSwitchRebindsPerContextState(t *testing.T) {
	newCluster, _, _ := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	newFP := &fakePersister{}
	fs := &fakeStateLoader{state: ContextState{
		MenuExtras: []config.MenuResource{certExtra("certificates")},
		Namespace:  "team-a",
		Persister:  newFP,
	}}
	oldFP := &fakePersister{}
	m := browsingModel(t, &fakeWatcher{},
		WithClusterConnector(fc), WithContextStateLoader(fs), WithContext("dev"),
		WithMenuExtras([]config.MenuResource{certExtra("issuers")}),
		WithNamespacePersister(oldFP),
	)

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, cmd))
	m = next.(Model)

	if len(fs.names) != 1 || fs.names[0] != "prod" {
		t.Fatalf("the state loader should be asked for the context being switched to, got %v", fs.names)
	}
	if !menuHasResource(m, "certificates") {
		t.Error("the new context's menu extras should be folded into the rebuilt menu")
	}
	if menuHasResource(m, "issuers") {
		t.Error("the departed context's menu extras must not survive the switch")
	}
	if m.namespace != "team-a" {
		t.Errorf("namespace after switch = %q, want the new context's last-used team-a", m.namespace)
	}
	if !strings.Contains(m.menu.View(), "team-a") {
		t.Errorf("the menu's namespace seam row should show the restored scope:\n%s", m.menu.View())
	}
	// The welcome page is the right pane after a reset, and it names the scope. (The
	// status bar carries it too, but the switch's own "switched to prod" notice owns
	// that line for the next few seconds.)
	if !strings.Contains(m.welcome.View(false), "team-a") {
		t.Errorf("the welcome page should show the restored scope:\n%s", m.welcome.View(false))
	}

	// The restored scope came out of the new context's state file, so the switch
	// itself must not write anything back — only a *pick* persists.
	if newFP.called || oldFP.called {
		t.Error("a switch must not persist a namespace it only restored")
	}

	// And the persister is rebound: a namespace picked now goes to the new context's
	// state file, never the departed one's.
	m = openNamespaceStage(t, m)
	_, persist := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: "kube-system"})
	if persist == nil {
		t.Fatal("picking a namespace should still issue a persist command after a switch")
	}
	persist()
	if !newFP.called || newFP.got != "kube-system" {
		t.Errorf("persisted through the new context = %q (called=%v), want kube-system", newFP.got, newFP.called)
	}
	if oldFP.called {
		t.Errorf("the departed context's state file must not be written, got %q", oldFP.got)
	}
}

// TestContextStateLoadsOffTheUpdateLoopAndOnlyOnSuccess pins where the per-context
// state is resolved: in the same off-loop Cmd as the connect (it is a disk read, so
// it may not run in Update), and not at all when the connect failed — a switch that
// did not happen must leave the shell's own per-context state exactly as it was.
func TestContextStateLoadsOffTheUpdateLoopAndOnlyOnSuccess(t *testing.T) {
	fs := &fakeStateLoader{state: ContextState{Namespace: "team-a"}}
	fc := &fakeConnector{err: errors.New("no such context")}
	m := browsingModel(t, &fakeWatcher{},
		WithClusterConnector(fc), WithContextStateLoader(fs), WithContext("dev"),
		WithMenuExtras([]config.MenuResource{certExtra("issuers")}),
	)

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	if len(fs.names) != 0 {
		t.Errorf("the state load ran on the update loop: %v", fs.names)
	}

	next, _ = m.Update(pickerMsg(t, cmd))
	m = next.(Model)
	if len(fs.names) != 0 {
		t.Errorf("a failed connect must not resolve the new context's state: %v", fs.names)
	}
	if m.namespace != "kube-system" || !menuHasResource(m, "issuers") {
		t.Error("a failed connect must leave the launch context's state untouched")
	}
}

// TestSwitchWithoutStateLoaderKeepsLaunchState: with no loader wired the shell has
// no way to resolve another context's files, so it keeps what it launched with
// rather than blanking it — the pre-M4-05 behaviour, and what every hermetic test
// that wires no loader gets. The namespace is still cleared, by the reset.
func TestSwitchWithoutStateLoaderKeepsLaunchState(t *testing.T) {
	newCluster, _, _ := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	fp := &fakePersister{}
	m := browsingModel(t, &fakeWatcher{},
		WithClusterConnector(fc), WithContext("dev"),
		WithMenuExtras([]config.MenuResource{certExtra("issuers")}),
		WithNamespacePersister(fp),
	)

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, cmd))
	m = next.(Model)

	if !menuHasResource(m, "issuers") {
		t.Error("with no loader the launch context's menu extras should be kept, not dropped")
	}
	if m.nsPersister != fp {
		t.Error("with no loader the launch persister should be kept")
	}
	if m.namespace != "" {
		t.Errorf("the reset still clears the scope, got %q", m.namespace)
	}
}

// switchTimingModel drives one whole switch — connect, swap, and the discovery pass
// the swap started — against a logger writing into a buffer, and hands back what the
// diagnostic log recorded. It is the harness for CTX-WARM-01's measurement: the point
// of the instrumentation is that a dogfooding human can read these numbers out of
// `~/.cache/kubecom/kubecom.log` after switching, so the tests assert on the same
// text rather than on an internal field.
func switchTimingModel(t *testing.T, to string, extra ...Option) (Model, *bytes.Buffer) {
	t.Helper()
	newCluster, _, _ := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	opts := append([]Option{
		WithClusterConnector(fc), WithContext("dev"), WithLogger(slog.New(h)),
	}, extra...)
	m := browsingModel(t, &fakeWatcher{}, opts...)

	next, cmd := m.switchContext(to)
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, cmd))
	return next.(Model), &buf
}

// discoveryLanded delivers the discovery pass the *current* cluster is waiting on —
// gen-tagged from the model so it is not dropped by the M4-03 generation guard, which
// a switch bumps.
func discoveryLanded(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{}})
	return next.(Model)
}

// TestSwitchIsTimedIntoTheLog is CTX-WARM-01's headline: a completed context switch
// writes its cost to the diagnostic log — the connect, the wait for the new cluster's
// menu, and their sum. Nothing about the switch's behaviour changes; the numbers are
// what decides whether retaining a departed cluster (CTX-WARM-02/03) is worth its
// risk, and without them that decision would be guesswork (D196 pt 3).
func TestSwitchIsTimedIntoTheLog(t *testing.T) {
	m, buf := switchTimingModel(t, "prod")
	if m.ctxSwitch.context != "prod" {
		t.Fatalf("the swap should have started the stopwatch, got %q", m.ctxSwitch.context)
	}
	if strings.Contains(buf.String(), "context switch complete") {
		t.Fatal("the timing must not be reported before the new cluster's menu is complete")
	}

	m = discoveryLanded(t, m)

	got := buf.String()
	for _, want := range []string{"context switch complete", "context=prod", "connect=", "discovery=", "total="} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q\ngot: %s", want, got)
		}
	}
	if m.ctxSwitch.context != "" {
		t.Error("the stopwatch should be consumed by the pass that closed it, not left pending")
	}
}

// TestSwitchTimingIsReportedOnce guards against the number turning into noise: the
// timing belongs to one switch and one discovery pass. A later pass on the same
// cluster (a refresh, a re-discovery) has no switch behind it and must stay silent,
// or the log would read as though the user kept switching.
func TestSwitchTimingIsReportedOnce(t *testing.T) {
	m, buf := switchTimingModel(t, "prod")
	m = discoveryLanded(t, m)
	discoveryLanded(t, m)

	if n := strings.Count(buf.String(), "context switch complete"); n != 1 {
		t.Errorf("switch timing logged %d times, want exactly 1\n%s", n, buf.String())
	}
}

// TestLaunchDiscoveryIsNotTimedAsASwitch pins the other side of the same rule: the
// launch-time discovery pass is not a switch and logs no timing. The log is a
// dogfooding surface, so a line that appears when nobody switched would be actively
// misleading about what the number measures.
func TestLaunchDiscoveryIsNotTimedAsASwitch(t *testing.T) {
	m, buf := logSink(t)
	m = discoveryLanded(t, m)

	if strings.Contains(buf.String(), "context switch complete") {
		t.Errorf("a launch discovery pass reported a switch timing:\n%s", buf.String())
	}
	if m.ctxSwitch.context != "" {
		t.Error("a launch discovery pass should leave the stopwatch zero")
	}
}

// TestSupersededSwitchReportsOnlyTheSurvivingOne: two switches in quick succession (a
// mis-pick corrected immediately) must report the switch the reader is actually
// waiting on, not the one they moved off. The superseded connect is dropped by the
// generation guard before it can start a stopwatch, so this falls out of D156 pt 2
// rather than needing a rule of its own — which is what the test exists to keep true.
func TestSupersededSwitchReportsOnlyTheSurvivingOne(t *testing.T) {
	newCluster, _, _ := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	m := browsingModel(t, &fakeWatcher{},
		WithClusterConnector(fc), WithContext("dev"), WithLogger(slog.New(h)))

	next, first := m.switchContext("prod")
	m = next.(Model)
	next, second := m.switchContext("staging") // supersedes it before the first lands.
	m = next.(Model)
	next, _ = m.Update(first())
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, second))
	discoveryLanded(t, next.(Model))

	got := buf.String()
	if !strings.Contains(got, "context=staging") {
		t.Errorf("the surviving switch should be the one timed:\n%s", got)
	}
	if strings.Contains(got, "context=prod") {
		t.Errorf("a superseded switch must not be timed:\n%s", got)
	}
}
