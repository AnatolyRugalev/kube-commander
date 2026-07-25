package searchview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newSearch() Model {
	m := New(styles.Default())
	m.SetSize(60, 12)
	m.Show()
	return m
}

// hit builds a SearchHit of the given kind for ns/name ("" ns = cluster-scoped).
func hit(kind, ns, name string) kube.SearchHit {
	return kube.SearchHit{
		Resource: kube.Resource{
			GVK:        schema.GroupVersionKind{Kind: kind},
			Namespaced: ns != "",
		},
		Ref: kube.ObjectRef{Namespace: ns, Name: name, UID: kind + "/" + ns + "/" + name},
	}
}

// typeQuery feeds each rune of q to the query field, collecting the emitted messages.
func typeQuery(m Model, q string) (Model, []tea.Msg) {
	var msgs []tea.Msg
	for _, r := range q {
		var cmd tea.Cmd
		m, cmd = m.UpdateQuery(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		msgs = append(msgs, drain(cmd)...)
	}
	return m, msgs
}

// drain runs cmd (and a tea.Batch's children) and returns the messages produced.
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

// queries returns the query strings of every QueryChangedMsg in msgs.
func queries(msgs []tea.Msg) []string {
	var out []string
	for _, msg := range msgs {
		if q, ok := msg.(QueryChangedMsg); ok {
			out = append(out, q.Query)
		}
	}
	return out
}

func TestHiddenOrUnsizedIsEmpty(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(60, 12) // sized but hidden
	if v := m.View(); v != "" {
		t.Errorf("hidden View() = %q; want empty", v)
	}
	m2 := New(styles.Default())
	m2.Show() // shown but unsized
	if v := m2.View(); v != "" {
		t.Errorf("unsized View() = %q; want empty", v)
	}
}

func TestInactiveViewIgnoresInput(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(60, 12)
	m.AppendHit(hit("Pod", "default", "api-0"))
	if _, cmd := m.Update(keymap.ActionDrillIn); cmd != nil {
		t.Error("an inactive view should not emit on nav.drillIn")
	}
	m2, cmd := m.UpdateQuery(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	if cmd != nil || m2.Query() != "" {
		t.Error("an inactive view should not capture typing")
	}
}

func TestTypingEmitsQueryChangedAndDropsStaleHits(t *testing.T) {
	m := newSearch()
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.SetSearching(true)

	m, msgs := typeQuery(m, "ap")
	if got := queries(msgs); len(got) != 2 || got[0] != "a" || got[1] != "ap" {
		t.Fatalf("QueryChangedMsg queries = %v; want [a ap]", got)
	}
	if m.Query() != "ap" {
		t.Errorf("Query() = %q; want %q", m.Query(), "ap")
	}
	if m.Len() != 0 {
		t.Errorf("hits from the previous query should be dropped; Len() = %d", m.Len())
	}
}

func TestQueryUnchangedKeyEmitsNothing(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "api")
	m.AppendHit(hit("Pod", "default", "api-0"))

	// A cursor move inside the field leaves the text alone: no re-search, no hit loss.
	m2, cmd := m.UpdateQuery(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if got := queries(drain(cmd)); len(got) != 0 {
		t.Errorf("a key that does not change the text emitted %v; want none", got)
	}
	if m2.Len() != 1 {
		t.Errorf("hits should survive a non-editing key; Len() = %d", m2.Len())
	}
}

func TestHitsRenderAlignedKindNamespaceName(t *testing.T) {
	rows := hitItems([]kube.SearchHit{
		hit("Pod", "default", "api-0"),
		hit("Deployment", "kube-system", "coredns"),
		hit("Node", "", "worker-1"),
	})
	want := []string{
		"Pod         default/api-0",
		"Deployment  kube-system/coredns",
		"Node        worker-1",
	}
	for i, w := range want {
		if rows[i].label != w {
			t.Errorf("row %d label = %q; want %q", i, rows[i].label, w)
		}
	}
}

func TestStreamedHitsAppearInView(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "api")
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.AppendHit(hit("Service", "default", "api"))

	v := m.View()
	if !strings.Contains(v, "default/api-0") || !strings.Contains(v, "Service") {
		t.Errorf("both streamed hits should render; got:\n%s", v)
	}
	if !strings.Contains(v, "2 results") {
		t.Errorf("header should count the hits; got:\n%s", v)
	}
}

func TestAppendKeepsCursorOnRow(t *testing.T) {
	m := newSearch()
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.AppendHit(hit("Pod", "default", "api-1"))
	m, _ = m.Update(keymap.ActionDown) // cursor on api-1

	// A late hit arriving under the reader must not move their selection.
	m.AppendHit(hit("Pod", "default", "api-2"))
	got, ok := m.Selected()
	if !ok || got.Ref.Name != "api-1" {
		t.Errorf("Selected() = %+v (ok=%v); want api-1 to stay selected", got, ok)
	}
}

func TestDrillInEmitsSelectedHit(t *testing.T) {
	m := newSearch()
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.AppendHit(hit("Deployment", "default", "api"))
	m, _ = m.Update(keymap.ActionDown)

	_, cmd := m.Update(keymap.ActionDrillIn)
	msgs := drain(cmd)
	if len(msgs) != 1 {
		t.Fatalf("nav.drillIn emitted %d msgs; want 1", len(msgs))
	}
	sel, ok := msgs[0].(SelectedMsg)
	if !ok {
		t.Fatalf("nav.drillIn emitted %T; want SelectedMsg", msgs[0])
	}
	if sel.Kind != kind {
		t.Errorf("SelectedMsg.Kind = %q; want %q", sel.Kind, kind)
	}
	if sel.Hit.Resource.GVK.Kind != "Deployment" || sel.Hit.Ref.Name != "api" {
		t.Errorf("SelectedMsg.Hit = %+v; want the highlighted Deployment default/api", sel.Hit)
	}
}

func TestDrillInWithNoResultsEmitsNothing(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "nope")
	if _, cmd := m.Update(keymap.ActionDrillIn); cmd != nil {
		t.Error("nav.drillIn on an empty result list should emit nothing")
	}
}

func TestBackClearsQueryThenCloses(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "api")
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.SetSearching(true)

	// First back: the query (and its results) go, the view stays.
	m, cmd := m.Update(keymap.ActionBack)
	msgs := drain(cmd)
	if got := queries(msgs); len(got) != 1 || got[0] != "" {
		t.Fatalf("first nav.back emitted %v; want one QueryChangedMsg{\"\"}", msgs)
	}
	if m.Query() != "" || m.Len() != 0 || m.Searching() {
		t.Errorf("first nav.back should clear query/results/in-flight; got %q, %d hits, searching=%v",
			m.Query(), m.Len(), m.Searching())
	}
	if !m.Active() {
		t.Error("first nav.back should not close the view")
	}

	// Second back: the view closes.
	_, cmd = m.Update(keymap.ActionBack)
	msgs = drain(cmd)
	if len(msgs) != 1 {
		t.Fatalf("second nav.back emitted %d msgs; want 1", len(msgs))
	}
	if closed, ok := msgs[0].(ClosedMsg); !ok || closed.Kind != kind {
		t.Errorf("second nav.back emitted %#v; want ClosedMsg{%q}", msgs[0], kind)
	}
}

func TestEmptyHintDistinguishesBlankSearchingAndNoMatch(t *testing.T) {
	m := newSearch()
	if !strings.Contains(m.View(), "type to search") {
		t.Errorf("a blank query should invite typing; got:\n%s", m.View())
	}
	m, _ = typeQuery(m, "api")
	m.SetSearching(true)
	if !strings.Contains(m.View(), "searching…") {
		t.Errorf("an in-flight search should say so; got:\n%s", m.View())
	}
	m.SetSearching(false)
	if !strings.Contains(m.View(), "no matches") {
		t.Errorf("a finished search with no hits must not look hung; got:\n%s", m.View())
	}
}

// TestProgressLineCountsKindsDone proves the header's progress segment reports the
// fan-out's kind count once it is known, and only while a search is in flight — the
// SEARCH-03b ask ("searching N/M kinds…", so a slow kind reads as progress, not a hang).
func TestProgressLineCountsKindsDone(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "api")

	// Before the fan-out launches (the debounce window) the count is unknown, so the
	// in-flight state is still the bare "searching…" rather than a bogus 0/0.
	m.SetSearching(true)
	if !strings.Contains(m.View(), "searching…") || strings.Contains(m.View(), "kinds") {
		t.Errorf("pre-launch should say searching… with no kind count; got:\n%s", m.View())
	}

	m.StartProgress(3)
	if !strings.Contains(m.View(), "searching 0/3 kinds…") {
		t.Errorf("launched search should show 0/3; got:\n%s", m.View())
	}
	m.MarkKindDone()
	m.MarkKindDone()
	if !strings.Contains(m.View(), "searching 2/3 kinds…") {
		t.Errorf("two kinds done should show 2/3; got:\n%s", m.View())
	}
	if done, total := m.Progress(); done != 2 || total != 3 {
		t.Errorf("Progress() = %d/%d; want 2/3", done, total)
	}

	// A stray extra kind-done cannot render 4/3.
	m.MarkKindDone()
	m.MarkKindDone()
	if done, _ := m.Progress(); done != 3 {
		t.Errorf("kinds done clamped at the total; got %d, want 3", done)
	}

	// Completion clears the line: the result count is the whole story once idle.
	m.SetSearching(false)
	if strings.Contains(m.View(), "kinds") {
		t.Errorf("a finished search should drop the progress line; got:\n%s", m.View())
	}
}

// TestCappedStateSurfacesAndOutranksProgress proves the cap is stated in the header as
// an actionable line, wins over the progress count while the rest of the fan-out drains,
// and survives completion (the results really are truncated).
func TestCappedStateSurfacesAndOutranksProgress(t *testing.T) {
	m := newSearch()
	m, _ = typeQuery(m, "api")
	m.SetSearching(true)
	m.StartProgress(4)
	for _, n := range []string{"api-0", "api-1"} {
		m.AppendHit(hit("Pod", "default", n))
	}
	m.MarkKindDone()

	m.SetCapped(true)
	if !m.Capped() {
		t.Fatal("SetCapped(true) should be reported by Capped()")
	}
	view := m.View()
	if !strings.Contains(view, "first 2 matches — narrow the query") {
		t.Errorf("cap should name the count and the fix; got:\n%s", view)
	}
	if strings.Contains(view, "kinds") {
		t.Errorf("the cap line should replace the progress count; got:\n%s", view)
	}
	m.SetSearching(false)
	if !strings.Contains(m.View(), "narrow the query") {
		t.Errorf("cap state must survive the search ending; got:\n%s", m.View())
	}
}

// TestQueryChangeResetsProgressAndCap proves the fan-out state is dropped together with
// the results it describes — on a keystroke, on nav.back's clear, and on Reset — so the
// header never shows a previous query's progress or cap.
func TestQueryChangeResetsProgressAndCap(t *testing.T) {
	arm := func() Model {
		m := newSearch()
		m, _ = typeQuery(m, "api")
		m.SetSearching(true)
		m.StartProgress(5)
		m.MarkKindDone()
		m.AppendHit(hit("Pod", "default", "api-0"))
		m.SetCapped(true)
		return m
	}

	m, _ := typeQuery(arm(), "x")
	if done, total := m.Progress(); done != 0 || total != 0 || m.Capped() {
		t.Errorf("a keystroke should reset progress/cap; got %d/%d capped=%v", done, total, m.Capped())
	}

	m, _ = arm().Update(keymap.ActionBack) // clears query + results
	if done, total := m.Progress(); done != 0 || total != 0 || m.Capped() {
		t.Errorf("nav.back's clear should reset progress/cap; got %d/%d capped=%v", done, total, m.Capped())
	}

	m = arm()
	m.Reset()
	if done, total := m.Progress(); done != 0 || total != 0 || m.Capped() {
		t.Errorf("Reset should clear progress/cap; got %d/%d capped=%v", done, total, m.Capped())
	}

	// A relaunch re-zeroes the counters even without a query change (the scope may
	// have shrunk between two searches of the same query).
	m = arm()
	m.StartProgress(2)
	if done, total := m.Progress(); done != 0 || total != 2 || m.Capped() {
		t.Errorf("StartProgress should restart at 0/total, uncapped; got %d/%d capped=%v", done, total, m.Capped())
	}
}

func TestNavigationMovesCursorWithinResults(t *testing.T) {
	m := newSearch()
	for _, n := range []string{"api-0", "api-1", "api-2"} {
		m.AppendHit(hit("Pod", "default", n))
	}
	m, _ = m.Update(keymap.ActionBottom)
	if got, _ := m.Selected(); got.Ref.Name != "api-2" {
		t.Errorf("nav.bottom selected %q; want api-2", got.Ref.Name)
	}
	m, _ = m.Update(keymap.ActionUp)
	if got, _ := m.Selected(); got.Ref.Name != "api-1" {
		t.Errorf("nav.up selected %q; want api-1", got.Ref.Name)
	}
	m, _ = m.Update(keymap.ActionTop)
	if got, _ := m.Selected(); got.Ref.Name != "api-0" {
		t.Errorf("nav.top selected %q; want api-0", got.Ref.Name)
	}
}

func TestResetAndScopeHeader(t *testing.T) {
	m := newSearch()
	m.SetScope("ns default")
	m, _ = typeQuery(m, "api")
	m.AppendHit(hit("Pod", "default", "api-0"))
	m.SetSearching(true)
	if !strings.Contains(m.View(), "ns default") {
		t.Errorf("header should name the scope; got:\n%s", m.View())
	}

	m.Reset()
	if m.Query() != "" || m.Len() != 0 || m.Searching() {
		t.Errorf("Reset should clear query/results/in-flight; got %q, %d hits, searching=%v",
			m.Query(), m.Len(), m.Searching())
	}
}

func TestHideBlursAndShowRefocuses(t *testing.T) {
	m := newSearch()
	m.Hide()
	if m.Active() {
		t.Fatal("Hide should deactivate the view")
	}
	m2, _ := m.UpdateQuery(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if m2.Query() != "" {
		t.Error("a hidden view should not capture typing")
	}
	m.Show()
	m3, _ := m.UpdateQuery(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if m3.Query() != "x" {
		t.Errorf("Show should refocus the query field; Query() = %q", m3.Query())
	}
}
