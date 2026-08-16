package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// pinKey is the menu.pin default binding (`*`), fed as a live keypress so every
// test here drives the real key → Action → handler path rather than calling the
// handler directly (D11).
var pinKey = tea.Key{Code: '*', Text: "*"}

// fakePinPersister is a hermetic PinPersister: it records what it was asked to
// write, in either direction, and can be made to fail. The real one (cmd/kubecom's
// statePersister) writes the per-context state file.
type fakePinPersister struct {
	got     []config.MenuResource // pins written
	removed []config.MenuResource // pins cleared (CRD-PIN-03)
	err     error
}

func (f *fakePinPersister) PersistPin(r config.MenuResource) error {
	f.got = append(f.got, r)
	return f.err
}

func (f *fakePinPersister) PersistUnpin(r config.MenuResource) error {
	f.removed = append(f.removed, r)
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

// pinnedCRD is crdResource as the state file records it (config.MenuResource) — the
// pin a context launches with, fed in through WithPinnedResources.
func pinnedCRD() config.MenuResource {
	return config.MenuResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
		Kind: "ExternalSecret", Namespaced: true,
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
	// And it joins the live pinned list, so it survives the menu being rebuilt from
	// the seed the way a context switch rebuilds it — and so the next press knows it
	// is a pin, and unpins rather than re-recording it (CRD-PIN-03).
	found := false
	for _, e := range m.menuPinned {
		if e.Resource == "externalsecrets" {
			found = true
		}
	}
	if !found {
		t.Errorf("the pin should join the model's pinned list, got %+v", m.menuPinned)
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

// TestPinTwiceUnpinsAndRemovesTheRow is CRD-PIN-03's headline and the behaviour
// change from CRD-PIN-02, where the second press was a no-op: `*` is a toggle, so
// the second press takes the pin back out — the line leaves the state file and the
// row leaves the menu, because with discovery silent the pin was the only reason it
// was listed. That is the case the whole feature is for (a kind the menu would not
// otherwise carry), and it is the case an undo must actually undo.
func TestPinTwiceUnpinsAndRemovesTheRow(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp), WithPinnedResources([]config.MenuResource{pinnedCRD()}))
	if !menuHasResource(m, "externalsecrets") {
		t.Fatal("a pinned kind should be in the menu at launch")
	}
	m = menuAt(t, m, "externalsecrets")
	before := len(m.menu.Items())

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.removed) != 1 || fp.removed[0].Resource != "externalsecrets" {
		t.Fatalf("unpin writes = %+v, want the kind cleared from the state file", fp.removed)
	}
	if len(fp.got) != 0 {
		t.Errorf("unpinning must not also record a pin, got %+v", fp.got)
	}
	if menuHasResource(m, "externalsecrets") {
		t.Error("the row should be gone: nothing but the pin was listing it")
	}
	if got := len(m.menu.Items()); got != before-1 {
		t.Errorf("menu items = %d, want %d", got, before-1)
	}
	if !strings.Contains(m.View().Content, "unpinned ExternalSecret") {
		t.Errorf("the status bar should name what was unpinned:\n%s", m.View().Content)
	}

	// And the toggle comes back round: a third press pins it again.
	m, cmd = press(t, m, pinKey)
	drain(cmd)
	if len(fp.got) != 1 {
		t.Errorf("a press after the unpin should pin again, writes = %+v", fp.got)
	}
}

// TestUnpinKeepsARowDiscoveryAlsoLists is the board's revert-to-a-discovered-row
// rule: unpinning means "stop keeping this for me", not "hide a kind this cluster
// has". So the state file loses the line and the row stays — and it is now an
// ordinary discovered row, which the next press proves by pinning it afresh.
func TestUnpinKeepsARowDiscoveryAlsoLists(t *testing.T) {
	fp := &fakePinPersister{}
	m := discoveredModel(t, fp, WithPinnedResources([]config.MenuResource{pinnedCRD()}))
	m = menuAt(t, m, "externalsecrets")
	before := len(m.menu.Items())

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.removed) != 1 {
		t.Fatalf("unpin writes = %+v, want one", fp.removed)
	}
	if !menuHasResource(m, "externalsecrets") {
		t.Error("discovery lists this kind, so the row must survive its unpin")
	}
	if got := len(m.menu.Items()); got != before {
		t.Errorf("menu items = %d, want %d — the row stays, only the pin went", got, before)
	}
	if !strings.Contains(m.View().Content, "unpinned ExternalSecret") {
		t.Errorf("the notice names what the reader did:\n%s", m.View().Content)
	}

	m, cmd = press(t, m, pinKey)
	drain(cmd)
	if len(fp.got) != 1 {
		t.Errorf("the row is unpinned now, so a press should pin it, writes = %+v", fp.got)
	}
}

// TestUnpinFromTheTableUsesTheBrowsedKind: the toggle reads its target from the same
// place the pin does, so a reader who drilled into a pinned kind can unpin it from
// the table without walking back to the menu row.
func TestUnpinFromTheTableUsesTheBrowsedKind(t *testing.T) {
	fp := &fakePinPersister{}
	m := browsingModel(t, &fakeWatcher{}, WithPinPersister(fp),
		WithPinnedResources([]config.MenuResource{pinnedCRD()}))
	m = menuAt(t, m, "externalsecrets")
	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: crdResource()})
	m = next.(Model)
	if !m.table.Focused() {
		t.Fatal("drilling in should focus the table")
	}

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.removed) != 1 || fp.removed[0].Resource != "externalsecrets" {
		t.Fatalf("unpin writes = %+v, want the browsed kind", fp.removed)
	}
	if menuHasResource(m, "externalsecrets") {
		t.Error("the row should have gone with the pin, even with the table focused")
	}
}

// TestPinThenUnpinLeavesASeedRowAlone is the guard on the removal rule. `*` on a
// seed row records a pin that never rendered anything (the row was already there),
// so taking it back out must not take the seed row with it — a menu that can lose
// Pods to a stray keypress is worse than one that cannot be pinned at all.
func TestPinThenUnpinLeavesASeedRowAlone(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp))
	m = menuAt(t, m, "pods")
	before := len(m.menu.Items())

	m, cmd := press(t, m, pinKey) // pin the seed row
	drain(cmd)
	m, cmd = press(t, m, pinKey) // and take it back out
	drain(cmd)

	if len(fp.got) != 1 || len(fp.removed) != 1 {
		t.Fatalf("writes = %+v / removals = %+v, want one each", fp.got, fp.removed)
	}
	if !menuHasResource(m, "pods") || len(m.menu.Items()) != before {
		t.Error("a seed row must outlive a pin made on it")
	}
}

// TestUnpinWriteFailureSurfacesAnErrorAndKeepsTheRowGone mirrors the pin's failure
// rule (principle 3): the menu already shows what the reader asked for, so a failed
// write is a toast, never a rollback that puts the row back to report an error.
func TestUnpinWriteFailureSurfacesAnErrorAndKeepsTheRowGone(t *testing.T) {
	fp := &fakePinPersister{err: errors.New("read-only file system")}
	m := sizedWith(t, WithPinPersister(fp), WithPinnedResources([]config.MenuResource{pinnedCRD()}))
	m = menuAt(t, m, "externalsecrets")

	m, cmd := press(t, m, pinKey)
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
		t.Error("a failed unpin write should surface an ErrorMsg")
	}
	if menuHasResource(m, "externalsecrets") {
		t.Error("the row should stay gone for the session even when the write failed")
	}
}

// TestPinIsANoOpForAnAuthoredEntry: a kind the user hand-wrote into
// menus/<context>.yaml is already in the menu on their terms, and the authored entry
// wins over a pin (D193 pt 3) — so pinning it would record a line that can never
// render, and unpinning it would have `*` silently disagree with a file written by
// hand. Both directions decline, and the notice says where the entry actually is.
func TestPinIsANoOpForAnAuthoredEntry(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp),
		WithMenuExtras([]config.MenuResource{certExtra("certificates")}))
	m = menuAt(t, m, "certificates")
	before := len(m.menu.Items())

	m, cmd := press(t, m, pinKey)
	drain(cmd)

	if len(fp.got) != 0 || len(fp.removed) != 0 {
		t.Errorf("an authored menu entry is neither pinned nor unpinned, got %+v / %+v", fp.got, fp.removed)
	}
	if !menuHasResource(m, "certificates") || len(m.menu.Items()) != before {
		t.Error("an authored row must not be removed by the pin key")
	}
	if !strings.Contains(m.View().Content, "menu file") {
		t.Errorf("the press should say where that entry lives:\n%s", m.View().Content)
	}
}

// TestAnAuthoredEntryOutranksAPinForTheSameKind is D193 pt 3 as the menu renders it,
// now that the two lists reach the shell separately (D202): the hand-written entry
// keeps its title and section, the kind is listed once, and the shadowed pin is not
// something `*` may remove.
func TestAnAuthoredEntryOutranksAPinForTheSameKind(t *testing.T) {
	fp := &fakePinPersister{}
	authored := config.MenuResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
		Kind: "ExternalSecret", Title: "Secrets (external)", Section: "Config", Namespaced: true,
	}
	m := sizedWith(t, WithPinPersister(fp),
		WithMenuExtras([]config.MenuResource{authored}),
		WithPinnedResources([]config.MenuResource{pinnedCRD()}))

	var rows int
	var title, section string
	for _, it := range m.menu.Items() {
		if it.Resource.GVR.Resource == "externalsecrets" {
			rows++
			title, section = it.Title, it.Section
		}
	}
	if rows != 1 {
		t.Fatalf("rows for the kind = %d, want exactly 1", rows)
	}
	if title != "Secrets (external)" || section != "Config" {
		t.Errorf("row = %q in %q, want the authored title and section", title, section)
	}

	m = menuAt(t, m, "externalsecrets")
	m, cmd := press(t, m, pinKey)
	drain(cmd)
	if len(fp.removed) != 0 {
		t.Errorf("the shadowed pin is not the key's to remove, got %+v", fp.removed)
	}
	if !menuHasResource(m, "externalsecrets") {
		t.Error("the authored row must survive a press of the pin key")
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
	if len(m.menu.Items()) != before || len(m.menuExtras) != 0 || len(m.menuPinned) != 0 {
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

// TestThePinKeyIsHintedInTheMenuContext is the discoverability half of CRD-PIN-03:
// a menu row does not say it can be pinned, so the key has to be on the hint line
// where the gesture applies. It is a menu-context hint only — the table's line is
// the app's fullest and elides first — and it is rendered wide so the assertion is
// about the curated set rather than about elision.
func TestThePinKeyIsHintedInTheMenuContext(t *testing.T) {
	next, _ := New(WithWatcher(&fakeWatcher{})).Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := next.(Model)
	if !m.menu.Focused() {
		t.Fatal("menu should start focused")
	}
	if hint := m.hintbar.View(); !strings.Contains(hint, keymap.ActionPin.Describe()) {
		t.Errorf("the menu hint should advertise the pin toggle; got %q", hint)
	}

	next, _ = m.Update(menu.ResourceSelectedMsg{Resource: crdResource()})
	m = next.(Model)
	if !m.table.Focused() {
		t.Fatal("drilling in should focus the table")
	}
	if hint := m.hintbar.View(); strings.Contains(hint, keymap.ActionPin.Describe()) {
		t.Errorf("the table hint is already the longest; the pin belongs to the menu line: %q", hint)
	}
}

// CRD-PIN-05: the same gesture reached by *typing the kind's name* rather than by
// pointing at its row — the palette's `:pin ` verb (D204). The tests below drive the
// real line (`:` → type the verb → space → type the kind → Enter), so they cover the
// stage wiring and the toggle's three outcomes on the surface the CRD line's premise
// ("you reach for a kind once") actually happens on.

// pinInPalette walks the palette's pin line to its argument stage: `:`, the verb, the
// separating space. It returns the model with the kinds listed and the prompt reading
// `:pin `.
func pinInPalette(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "pin")
	if v, _ := m.cmdPicker.Selected(); v != keymap.ActionPin.Describe() {
		t.Fatalf("typing \"pin\" selected %q, want %q", v, keymap.ActionPin.Describe())
	}
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg != keymap.ActionPin {
		t.Fatalf("space should commit the pin verb, stage = %q", m.palArg)
	}
	// The argument stage opens in navigation mode (STORY-06d): `/` opens the field the
	// pin kind is typed into.
	m, _ = press(t, m, slash)
	return m
}

// TestPalettePinVerbPinsTheKindYouTyped is the leg's headline: a kind is pinned by
// naming it, with no cursor anywhere near its row (the menu here is focused on its
// first row, not the CRD). The kind is found by its **plural** — the name the reader
// types into kubectl — because the stage is seeded from the one snapshot `R` uses,
// aliases and all (D203/D204 pt 1).
func TestPalettePinVerbPinsTheKindYouTyped(t *testing.T) {
	fp := &fakePinPersister{}
	m := discoveredModel(t, fp)
	m = pinInPalette(t, m)

	m = typeInto(t, m, "externalsecrets")
	if got := m.cmdPicker.Len(); got != 1 {
		t.Fatalf("query %q left %d rows, want 1 (ExternalSecret)", "externalsecrets", got)
	}
	m, cmd := selectInPalette(t, m)
	drain(cmd)

	if m.cmdPicker.Active() {
		t.Error("applying the argument should close the palette")
	}
	if len(fp.got) != 1 || fp.got[0] != pinnedCRD() {
		t.Fatalf("persisted %+v, want exactly one %+v", fp.got, pinnedCRD())
	}
	if !strings.Contains(m.View().Content, "pinned ExternalSecret") {
		t.Errorf("the status bar should confirm the pin:\n%s", m.View().Content)
	}
}

// TestPalettePinVerbUnpinsAPinnedKind is D202 pt 3 on the new surface: one verb, both
// directions. The kind arrives already pinned (as a launch does it) and is not listed
// by discovery, so the row the pin created leaves with it.
func TestPalettePinVerbUnpinsAPinnedKind(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp), WithPinnedResources([]config.MenuResource{pinnedCRD()}))
	if !menuHasResource(m, "externalsecrets") {
		t.Fatal("precondition: the launch pin should be in the menu")
	}
	before := len(m.menu.Items())

	m = pinInPalette(t, m)
	m = typeInto(t, m, "externalsecrets")
	m, cmd := selectInPalette(t, m)
	drain(cmd)

	if len(fp.removed) != 1 || fp.removed[0] != pinnedCRD() {
		t.Fatalf("cleared %+v, want exactly one %+v", fp.removed, pinnedCRD())
	}
	if menuHasResource(m, "externalsecrets") || len(m.menu.Items()) != before-1 {
		t.Error("the row the pin created should leave with the pin")
	}
	if !strings.Contains(m.View().Content, "unpinned ExternalSecret") {
		t.Errorf("the status bar should confirm the unpin:\n%s", m.View().Content)
	}
}

// TestPalettePinVerbDeclinesAnAuthoredEntry: the refusal is the toggle's, not the
// key's, so it reaches the palette for free — and it must, because a `:pin ` that
// quietly recorded a line `menus/<context>.yaml` outranks would be a lie told on a
// second surface (D204 pt 2).
func TestPalettePinVerbDeclinesAnAuthoredEntry(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp),
		WithMenuExtras([]config.MenuResource{certExtra("certificates")}))

	m = pinInPalette(t, m)
	m = typeInto(t, m, "certificates")
	m, cmd := selectInPalette(t, m)
	drain(cmd)

	if len(fp.got) != 0 || len(fp.removed) != 0 {
		t.Errorf("an authored entry is neither pinned nor unpinned, got %+v / %+v", fp.got, fp.removed)
	}
	if !strings.Contains(m.View().Content, "menu file") {
		t.Errorf("the palette should say where that entry lives:\n%s", m.View().Content)
	}
}

// TestPalettePinVerbIsInertWithoutAPersister mirrors D197's rule for the verb stage: a
// verb that cannot produce an outcome does not open an argument list that suggests it
// can. With no persister the pin verb is as inert in the palette as `*` is on a row —
// the stage is refused and the palette closes, rather than listing kinds that would
// silently fail to be recorded.
func TestPalettePinVerbIsInertWithoutAPersister(t *testing.T) {
	m := sizedWith(t) // no WithPinPersister
	m, _ = press(t, m, colon)
	m = typeInto(t, m, "pin")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg == keymap.ActionPin {
		t.Fatal("a pin-inert shell should not enter the pin argument stage")
	}
}

// TestPalettePinVerbNeedsNoWatcher is the one way the pin stage differs from
// `:resource `: switching the table needs a watcher and pinning does not, so a shell
// that cannot browse can still record the kinds you want kept (D204 pt 1).
func TestPalettePinVerbNeedsNoWatcher(t *testing.T) {
	fp := &fakePinPersister{}
	m := sizedWith(t, WithPinPersister(fp)) // no WithWatcher
	m = pinInPalette(t, m)
	if m.cmdPicker.Len() == 0 {
		t.Fatal("the pin stage should list the seed kinds even with no watcher wired")
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
