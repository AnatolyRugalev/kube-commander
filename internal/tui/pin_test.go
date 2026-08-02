package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// pinKey is the menu.pin default binding (`*`), fed as a live keypress so every
// test here drives the real key → Action → handler path rather than calling the
// handler directly (D11).
var pinKey = tea.Key{Code: '*', Text: "*"}

// fakePinPersister is a hermetic PinPersister: it records what it was asked to
// write and can be made to fail. The real one (cmd/kubecom's statePersister) writes
// the per-context state file.
type fakePinPersister struct {
	got []config.MenuResource
	err error
}

func (f *fakePinPersister) PersistPin(r config.MenuResource) error {
	f.got = append(f.got, r)
	return f.err
}

// crdResource is a discovered custom resource, as discovery reports it: a namespaced
// kind whose Namespaced flag is the one thing a pin must not guess (a namespaced CRD
// pinned as cluster-scoped lists nothing).
func crdResource() kube.Resource {
	return kube.Resource{
		GVK:        schema.GroupVersionKind{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret"},
		GVR:        schema.GroupVersionResource{Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets"},
		Namespaced: true,
	}
}

// menuAt moves the menu cursor onto the row for the given plural resource name,
// failing the test when the menu does not list it. It is the mouse-click entry
// (SelectItem), used here to place the cursor without simulating a run of j presses.
func menuAt(t *testing.T, m Model, resource string) Model {
	t.Helper()
	for i, it := range m.menu.Items() {
		if it.Kind == menu.ItemResource && it.Resource.GVR.Resource == resource {
			m.menu.SelectItem(i)
			return m
		}
	}
	t.Fatalf("menu does not list %q", resource)
	return m
}

// discoveredModel is a sized model whose menu has been through a discovery pass that
// reported one CRD, with the pin persister wired and the menu focused — the state a
// reader is in when they land on a custom resource they want to keep.
func discoveredModel(t *testing.T, fp *fakePinPersister, opts ...Option) Model {
	t.Helper()
	m := sizedWith(t, append([]Option{WithPinPersister(fp)}, opts...)...)
	next, _ := m.Update(DiscoveryReadyMsg{gen: m.discoveryGen, Result: kube.DiscoveryResult{
		Resources: []kube.Resource{crdResource()},
	}})
	m = next.(Model)
	if !menuHasResource(m, "externalsecrets") {
		t.Fatal("discovery should have appended the CRD to the menu")
	}
	return m
}

// TestPinFromMenuRecordsTheKindAndKeepsTheRow is the leg's headline: `*` on a menu
// row writes that kind through the persister with the identity and the Namespaced
// flag discovery reported, and the row is still there (a pin adds, never duplicates).
func TestPinFromMenuRecordsTheKindAndKeepsTheRow(t *testing.T) {
	fp := &fakePinPersister{}
	m := discoveredModel(t, fp)
	m = menuAt(t, m, "externalsecrets")

	before := len(m.menu.Items())
	m, cmd := press(t, m, pinKey)
	if cmd == nil {
		t.Fatal("pinning should issue a command (the notice and the write)")
	}
	drain(cmd)

	if len(fp.got) != 1 {
		t.Fatalf("persister calls = %d, want 1: %+v", len(fp.got), fp.got)
	}
	want := config.MenuResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
		Kind: "ExternalSecret", Namespaced: true,
	}
	if fp.got[0] != want {
		t.Errorf("persisted %+v, want %+v", fp.got[0], want)
	}
	if got := len(m.menu.Items()); got != before {
		t.Errorf("menu items = %d, want %d — a kind already listed must not gain a second row", got, before)
	}
	if !strings.Contains(m.View().Content, "pinned ExternalSecret") {
		t.Errorf("the status bar should name what was pinned:\n%s", m.View().Content)
	}
}

// TestPinIsRecordedForAKindTheMenuAlreadyShows is the property the whole line rests
// on, stated as its own test because the obvious implementation gets it backwards:
// discovery appends every kind it finds to the menu, so a "the menu already lists it,
// nothing to do" check would make pinning a discovered CRD a no-op — and a pin is
// exactly what keeps that kind in the menu when discovery does *not* list it.
func TestPinIsRecordedForAKindTheMenuAlreadyShows(t *testing.T) {
	fp := &fakePinPersister{}
	m := discoveredModel(t, fp)
	m = menuAt(t, m, "externalsecrets")

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.got) != 1 {
		t.Fatalf("a discovered kind must still be recorded as a pin, calls = %d", len(fp.got))
	}
	// And it joins the extras list, so it survives the menu being rebuilt from the
	// seed the way a context switch rebuilds it.
	found := false
	for _, e := range m.menuExtras {
		if e.Resource == "externalsecrets" {
			found = true
		}
	}
	if !found {
		t.Errorf("the pin should join the model's menu extras, got %+v", m.menuExtras)
	}
}

// TestPinFromTableUsesTheBrowsedKind covers the other surface that can name a kind:
// with the table focused, the pin is the kind on screen — which is where a reader who
// arrived from a search hit or the resource picker actually is.
func TestPinFromTableUsesTheBrowsedKind(t *testing.T) {
	fp := &fakePinPersister{}
	fw := &fakeWatcher{}
	m := browsingModel(t, fw, WithPinPersister(fp))
	if !m.table.Focused() {
		t.Fatal("browsing model should have the table focused")
	}

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.got) != 1 || fp.got[0].Resource != "pods" {
		t.Fatalf("persisted %+v, want the browsed kind (pods)", fp.got)
	}
}

// TestPinTwiceIsANoOpWithANotice: the second press has nothing to add — the kind is
// already an entry — so it writes nothing and says so, rather than growing a
// duplicate row or a duplicate line in the state file.
func TestPinTwiceIsANoOpWithANotice(t *testing.T) {
	fp := &fakePinPersister{}
	m := discoveredModel(t, fp)
	m = menuAt(t, m, "externalsecrets")

	m, cmd := press(t, m, pinKey)
	drain(cmd)
	before := len(m.menu.Items())

	m, cmd = press(t, m, pinKey)
	drain(cmd)

	if len(fp.got) != 1 {
		t.Errorf("pinning an already-pinned kind must not write again, calls = %d", len(fp.got))
	}
	if got := len(m.menu.Items()); got != before {
		t.Errorf("menu items = %d, want %d — no duplicate row", got, before)
	}
	if !strings.Contains(m.View().Content, "already in the menu") {
		t.Errorf("the second press should say why nothing happened:\n%s", m.View().Content)
	}
}

// TestPinIsANoOpForAnAuthoredEntry: a kind the user hand-wrote into
// menus/<context>.yaml is already in the menu on their terms, and the authored entry
// wins over a pin (D193 pt 3) — so pinning it would record a line that can never
// render. It is the same "already an entry" no-op as a second press.
func TestPinIsANoOpForAnAuthoredEntry(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp),
		WithMenuExtras([]config.MenuResource{certExtra("certificates")}))
	m = menuAt(t, m, "certificates")

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.got) != 0 {
		t.Errorf("an authored menu entry must not be re-recorded as a pin, got %+v", fp.got)
	}
	if !strings.Contains(m.View().Content, "already in the menu") {
		t.Errorf("the press should say why nothing happened:\n%s", m.View().Content)
	}
}

// TestPinIsInertWithoutAPersister: with no state file to write to, the gesture does
// nothing at all — it does not add a row that would silently vanish on the next
// launch, and it makes no claim in the status bar (the WithNamespacePersister rule).
func TestPinIsInertWithoutAPersister(t *testing.T) {
	m := sizedWith(t)
	before := len(m.menu.Items())

	m, cmd := press(t, m, pinKey)

	if cmd != nil {
		t.Error("a pin-inert model should issue no command")
	}
	if len(m.menu.Items()) != before || len(m.menuExtras) != 0 {
		t.Error("a pin-inert model must not change the menu")
	}
	if m.status.HasNotice() {
		t.Error("a pin-inert model must not claim it pinned anything")
	}
}

// TestPinIsInertOnTheNamespaceSeamRow: the seam row names no kind, so the gesture has
// nothing to act on there — inert, not a pin of whatever was highlighted before.
func TestPinIsInertOnTheNamespaceSeamRow(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp))
	for i, it := range m.menu.Items() {
		if it.Kind == menu.ItemNamespace {
			m.menu.SelectItem(i)
		}
	}

	m, cmd := press(t, m, pinKey)

	if cmd != nil || len(fp.got) != 0 {
		t.Errorf("the namespace seam row names no kind, so pinning it must be inert, got %+v", fp.got)
	}
	if m.status.HasNotice() {
		t.Error("an inert pin must not claim anything in the status bar")
	}
}

// TestPinWriteFailureSurfacesAnErrorAndKeepsTheRow: persistence is best-effort
// (principle 3) — the row is already what the reader asked for, so a failed write is
// a toast, not a rollback.
func TestPinWriteFailureSurfacesAnErrorAndKeepsTheRow(t *testing.T) {
	fp := &fakePinPersister{err: errors.New("read-only file system")}
	m := discoveredModel(t, fp)
	m = menuAt(t, m, "externalsecrets")

	m, cmd := press(t, m, pinKey)
	if cmd == nil {
		t.Fatal("pinning should issue a command")
	}
	var gotErr bool
	for _, msg := range drain(cmd) {
		if e, ok := msg.(ErrorMsg); ok {
			gotErr = true
			if !strings.Contains(e.Message(), "read-only file system") {
				t.Errorf("error message = %q, want the write failure", e.Message())
			}
		}
	}
	if !gotErr {
		t.Error("a failed pin write should surface an ErrorMsg")
	}
	if !menuHasResource(m, "externalsecrets") {
		t.Error("the row should stay for the session even when the write failed")
	}
}

// TestSwitchRebindsThePinPersister is the D163 property applied to this seam: a pin
// recorded after a context switch belongs in the *new* context's state file. The
// launch-time persister is bound to one context's path, so carrying it across a
// switch would file the new cluster's CRDs under the old cluster's name.
func TestSwitchRebindsThePinPersister(t *testing.T) {
	newCluster, _, _ := newClusterFake()
	fc := &fakeConnector{cluster: newCluster}
	newFP := &fakePinPersister{}
	oldFP := &fakePinPersister{}
	fs := &fakeStateLoader{state: ContextState{Pinner: newFP}}
	m := browsingModel(t, &fakeWatcher{},
		WithClusterConnector(fc), WithContextStateLoader(fs), WithContext("dev"),
		WithPinPersister(oldFP),
	)

	next, cmd := m.switchContext("prod")
	m = next.(Model)
	next, _ = m.Update(pickerMsg(t, cmd))
	m = next.(Model)

	// The reset left the menu focused on the seed, so pin whatever it is sitting on.
	_, cmd = press(t, m, pinKey)
	drain(cmd)

	if len(newFP.got) != 1 {
		t.Errorf("the pin should go to the switched-in context's state file, calls = %d", len(newFP.got))
	}
	if len(oldFP.got) != 0 {
		t.Errorf("the departed context's state file must not be written, got %+v", oldFP.got)
	}
}

// drain runs a command (batched or not) and returns every message it produced, so a
// test can assert on the write's outcome as well as the notice.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}
