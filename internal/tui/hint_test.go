package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// wideSized builds a model on a terminal wide enough that the hint renderer elides
// nothing — these tests are about which bindings a context offers, never about how a
// full line is truncated.
func wideSized(t *testing.T, opts ...Option) Model {
	t.Helper()
	m, _ := New(opts...).Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	return m.(Model)
}

// hintOffers fails unless every action is named on the hint line.
func hintOffers(t *testing.T, hint string, what string, actions ...keymap.Action) {
	t.Helper()
	for _, a := range actions {
		if !strings.Contains(hint, a.Describe()) {
			t.Errorf("%s should offer %q: %q", what, a, hint)
		}
	}
}

// hintHides fails if any action is named on the hint line.
func hintHides(t *testing.T, hint string, what string, actions ...keymap.Action) {
	t.Helper()
	for _, a := range actions {
		if strings.Contains(hint, a.Describe()) {
			t.Errorf("%s must not offer %q — it does not act there: %q", what, a, hint)
		}
	}
}

// TestHintBarTracksOpenPicker is HINT-01's headline (D143 pt 1 applied to an overlay):
// a modal picker captures all input and, since PAL-01, opens its filter with itself, so
// the browse keys underneath it either type a character or are swallowed. The hint must
// narrow to what still acts while it is up, and widen back when it closes.
func TestHintBarTracksOpenPicker(t *testing.T) {
	m := wideSized(t)
	browse := m.hintbar.View()
	// The premise: the browse hint advertises exactly the keys the picker takes away.
	hintOffers(t, browse, "the menu hint", keymap.ActionHelp, keymap.ActionQuit, keymap.ActionNamespace)

	m, _ = press(t, m, colon)
	if !m.cmdPicker.Active() || !m.cmdPicker.Filtering() {
		t.Fatal("`:` should open the palette with its filter field open")
	}
	hint := m.hintbar.View()
	hintHides(t, hint, "an open type-to-filter picker",
		keymap.ActionHelp, keymap.ActionQuit, keymap.ActionNamespace, keymap.ActionPin, keymap.ActionFilter)
	hintOffers(t, hint, "an open type-to-filter picker",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionDrillIn, keymap.ActionBack)

	// Dismiss it the way a reader does — esc on an empty query cancels — and the browse
	// hint must come back byte for byte.
	m, cmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc on an empty palette query should emit a cancellation")
	}
	cancelled, ok := cmd().(picker.CancelledMsg)
	if !ok {
		t.Fatalf("esc produced %T, want picker.CancelledMsg", cmd())
	}
	next, _ := m.Update(cancelled)
	if got := next.(Model).hintbar.View(); got != browse {
		t.Errorf("closing the picker should restore the browse hint: %q, want %q", got, browse)
	}
}

// TestHintBarPickerContextFollowsFilterState proves the two picker contexts are chosen
// by the *field's* state, not by the picker's kind: the port picker opens with its
// filter closed (WithOptInFilter, D139), so `/` genuinely acts and is hinted — and once
// `/` opens the field it stops acting and drops out.
func TestHintBarPickerContextFollowsFilterState(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{ports: []kube.Port{{Port: 8080, Name: "http", Container: "app"}}}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))
	// openPodTable sizes for a table, not for an un-elided hint; re-size wide.
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	m = resized.(Model)

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	m = loadPorts(t, m, cmd)
	if !m.portPicker.Active() || m.portPicker.Filtering() {
		t.Fatal("the port picker should open with its opt-in filter closed")
	}

	closed := m.hintbar.View()
	hintOffers(t, closed, "a closed-filter picker",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionFilter, keymap.ActionDrillIn, keymap.ActionBack)
	hintHides(t, closed, "a closed-filter picker", keymap.ActionHelp, keymap.ActionQuit, keymap.ActionNamespace)

	m, _ = press(t, m, filterKey)
	if !m.portPicker.Filtering() {
		t.Fatal("`/` should open the port picker's filter field")
	}
	hintHides(t, m.hintbar.View(), "an opened picker filter", keymap.ActionFilter)
	hintOffers(t, m.hintbar.View(), "an opened picker filter", keymap.ActionDrillIn, keymap.ActionBack)
}

// TestHintBarRefreshesWithoutAnExplicitSync is the structural half of HINT-01 (D206):
// the hint is derived at the tail of every Update, so a state change that no
// syncHints call sits next to still lands. `a` is such a path — it opens the palette's
// action stage from a table row and nothing on that route touches the hint.
func TestHintBarRefreshesWithoutAnExplicitSync(t *testing.T) {
	m := openPodTable(t, "Pod")
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	m = resized.(Model)
	table := m.hintbar.View()

	m = openActionStage(t, m)
	if got := m.hintbar.View(); got == table {
		t.Errorf("the action stage captures input; the hint must not stay on the table set: %q", got)
	}
	hintHides(t, m.hintbar.View(), "the open action stage",
		keymap.ActionSort, keymap.ActionActions, keymap.ActionQuit, keymap.ActionHelp)
}
