package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/modal"
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

// wide re-sizes an already-built model to a terminal that elides nothing, so a test that
// opens a surface through the real gestures can still read the whole hint line.
func wide(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	return next.(Model)
}

// TestHintBarTracksTheConfirmModal is HINT-02 on the surface whose keys are not browse
// keys at all: the confirm modal answers in the confirm key context (D132), so while it
// is up the browse set underneath is entirely unreachable and the hint must show `y`/`n`.
func TestHintBarTracksTheConfirmModal(t *testing.T) {
	m := wide(t, deleteTableModel(t, &fakeDeleter{}))
	browse := m.hintbar.View()

	m = openDeleteModal(t, m)
	if !m.modal.Active() || m.modal.Prompting() {
		t.Fatal("the delete key should open a confirm-mode modal")
	}
	hint := m.hintbar.View()
	hintOffers(t, hint, "an open confirm modal",
		keymap.ActionConfirmAccept, keymap.ActionConfirmDecline)
	hintHides(t, hint, "an open confirm modal",
		keymap.ActionFilter, keymap.ActionSort, keymap.ActionActions, keymap.ActionHelp, keymap.ActionQuit)

	// Declining closes it, and the browse hint must come back byte for byte.
	next, _ := m.Update(modal.CancelledMsg{Kind: deleteModalKind})
	if got := next.(Model).hintbar.View(); got != browse {
		t.Errorf("closing the confirm modal should restore the browse hint: %q, want %q", got, browse)
	}
}

// TestHintBarTracksThePromptModal proves the prompt mode gets its own set rather than the
// confirm one — it is the same modal, and Active() is true for both, so the ordering in
// hintContext is what makes this right. An open field types `y`/`n`; only enter and esc act.
func TestHintBarTracksThePromptModal(t *testing.T) {
	m := wide(t, workloadModel(t, WithScaler(&fakeScaler{})))

	m, _ = dispatchRowAction(t, m, rowActionScale)
	if !m.modal.Prompting() {
		t.Fatal("the scale intent should open the modal in prompt mode")
	}
	hint := m.hintbar.View()
	hintOffers(t, hint, "an open prompt modal", keymap.ActionDrillIn, keymap.ActionBack)
	hintHides(t, hint, "an open prompt modal",
		keymap.ActionConfirmAccept, keymap.ActionConfirmDecline, keymap.ActionQuit, keymap.ActionFilter)
}

// TestHintBarTracksTheKeybindingsOverlay covers the overlay that swallows navigation: the
// only promise left to make under it is how to get back out.
func TestHintBarTracksTheKeybindingsOverlay(t *testing.T) {
	m := wideSized(t)
	browse := m.hintbar.View()

	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if !m.help.Visible() {
		t.Fatal("`?` should open the keybindings overlay")
	}
	hint := m.hintbar.View()
	hintOffers(t, hint, "the open keybindings overlay",
		keymap.ActionBack, keymap.ActionHelp, keymap.ActionQuit)
	hintHides(t, hint, "the open keybindings overlay",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionDrillIn, keymap.ActionNamespace, keymap.ActionPin)

	m, _ = press(t, m, tea.Key{Code: '?', Text: "?"})
	if got := m.hintbar.View(); got != browse {
		t.Errorf("closing the overlay should restore the browse hint: %q, want %q", got, browse)
	}
}

// TestHintBarTracksTheSharedViewer covers the read-only viewer (M3-03), a pager overlay:
// it scrolls and closes, and every browse gesture underneath it is swallowed.
func TestHintBarTracksTheSharedViewer(t *testing.T) {
	m := wide(t, describeViewerModel(t, &fakeDescriber{text: "Name: web-1\n"}))

	_, cmd := press(t, m, describeKey)
	next, fetch := m.Update(cmd().(rowActionMsg))
	next, _ = next.(Model).Update(fetch().(describeLoadedMsg))
	m = next.(Model)
	if !m.viewer.Active() {
		t.Fatal("the describe key should open the shared viewer")
	}
	hint := m.hintbar.View()
	hintOffers(t, hint, "the open viewer",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionBack, keymap.ActionQuit)
	hintHides(t, hint, "the open viewer",
		keymap.ActionFilter, keymap.ActionSort, keymap.ActionActions, keymap.ActionNamespace, keymap.ActionHelp)
}

// TestHintBarTracksTheBrowseFilterField is the first of HINT-03's two: the browse
// filter field is a text field like the logs grep, so the table set underneath it —
// `/` `n` `s` `a` `?` `q` — types a character instead of firing. Typing into the open
// field must not widen the hint back out either: the context is the field's state.
func TestHintBarTracksTheBrowseFilterField(t *testing.T) {
	m, _ := tableWith(t, "web-1", "web-2", "api-1")
	m = wide(t, m)
	// The premise: the table hint advertises exactly the keys the open field takes away.
	table := m.hintbar.View()
	hintOffers(t, table, "the table hint",
		keymap.ActionFilter, keymap.ActionSearchNext, keymap.ActionSort, keymap.ActionActions,
		keymap.ActionHelp, keymap.ActionQuit)

	m, _ = press(t, m, slash)
	if !m.filtering {
		t.Fatal("`/` should open the browse filter field")
	}
	hint := m.hintbar.View()
	hintHides(t, hint, "the open filter field",
		keymap.ActionFilter, keymap.ActionSearchNext, keymap.ActionSort, keymap.ActionActions,
		keymap.ActionHelp, keymap.ActionQuit, keymap.ActionNamespace)
	hintOffers(t, hint, "the open filter field",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionDrillIn, keymap.ActionBack)

	// The keys it hides are keys it *types*: `s` narrows the rows rather than sorting.
	m = typeStr(t, m, "s")
	if got := m.hintbar.View(); got != hint {
		t.Errorf("typing into the field must not change the hint: %q, want %q", got, hint)
	}

	// enter commits the narrowing and closes the field, so the table set comes back.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEnter})
	if m.filtering {
		t.Fatal("enter should commit the filter and close the field")
	}
	if got := m.hintbar.View(); got != table {
		t.Errorf("committing the filter should restore the table hint: %q, want %q", got, table)
	}
}

// TestHintBarTracksTheForwardsPanel is HINT-03's second, and the one capturing surface
// with no text field: the panel swallows every browse key and honours only its own
// cursor / stop / close set (handleForwardsPanelAction).
func TestHintBarTracksTheForwardsPanel(t *testing.T) {
	m := wide(t, openPodTable(t, "Pod"))
	table := m.hintbar.View()

	m, _ = press(t, m, tea.Key{Code: 'F', Text: "F"})
	if !m.forwardsPanel {
		t.Fatal("`F` should open the port-forward panel")
	}
	hint := m.hintbar.View()
	hintOffers(t, hint, "the open port-forward panel",
		keymap.ActionDown, keymap.ActionUp, keymap.ActionDrillIn, keymap.ActionStopForwards,
		keymap.ActionBack, keymap.ActionQuit)
	hintHides(t, hint, "the open port-forward panel",
		keymap.ActionFilter, keymap.ActionSearchNext, keymap.ActionSort, keymap.ActionActions,
		keymap.ActionNamespace, keymap.ActionHelp)

	// Closing it the way a reader does restores the table hint byte for byte.
	m, _ = press(t, m, tea.Key{Code: tea.KeyEsc})
	if m.forwardsPanel {
		t.Fatal("esc should close the port-forward panel")
	}
	if got := m.hintbar.View(); got != table {
		t.Errorf("closing the panel should restore the table hint: %q, want %q", got, table)
	}
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
