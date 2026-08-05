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

// TestEveryHelpContextIsReachable closes the second half of D218 pt 1 (HINT-05). The
// keymap package's completeness test proves every declared context has a curated hint
// set; this one proves the set is ever *shown* — that some state of the root model makes
// hintContext() return it. The two failures it catches are the two halves of the same
// mistake: a context declared for a new capturing surface whose case never got added to
// the switch (the surface ships the browse hint, D218 pt 1's headline), and a context
// whose case is shadowed by one above it in the precedence order, which is a live bug
// nothing else would notice — hintContext mirrors Update's routing, so a shadowed hint
// means a shadowed router arm.
//
// Every entry drives the model through the real gesture rather than assigning state, so
// the table is a transcript of how a reader arrives at each surface. The per-context
// assertions about *which* keys each set offers stay in the tests above; this one is
// about coverage, and it is deliberately the only test in the file that must be edited
// when a context is added.
func TestEveryHelpContextIsReachable(t *testing.T) {
	reach := map[keymap.HelpContext]func(t *testing.T) Model{
		keymap.HelpMenu: func(t *testing.T) Model { return wideSized(t) },
		keymap.HelpTable: func(t *testing.T) Model {
			return wide(t, openPodTable(t, "Pod"))
		},
		keymap.HelpSearch: func(t *testing.T) Model {
			return wide(t, openSearchView(t, &fakeSearcher{}))
		},
		keymap.HelpLogs: func(t *testing.T) Model {
			return wide(t, openLogsWithLines(t, "GET /healthz 200"))
		},
		keymap.HelpLogsFilter: func(t *testing.T) Model {
			m := wide(t, openLogsWithLines(t, "GET /healthz 200"))
			m, _ = press(t, m, filterKey)
			return m
		},
		keymap.HelpPickerFilter: func(t *testing.T) Model {
			m, _ := press(t, wideSized(t), colon)
			return m
		},
		keymap.HelpPicker: func(t *testing.T) Model {
			// The only opt-in-filter picker left (D139), so the only way to reach a
			// picker context with its field closed.
			pl := &fakePortLister{ports: []kube.Port{{Port: 8080, Name: "http", Container: "app"}}}
			m := wide(t, openPodTable(t, "Pod",
				WithPortForwarder(&fakePortForwarder{handle: newFakeForward()}), WithPortLister(pl)))
			m, cmd := dispatchRowAction(t, m, rowActionPortForward)
			return loadPorts(t, m, cmd)
		},
		keymap.HelpConfirm: func(t *testing.T) Model {
			return openDeleteModal(t, wide(t, deleteTableModel(t, &fakeDeleter{})))
		},
		keymap.HelpPrompt: func(t *testing.T) Model {
			m, _ := dispatchRowAction(t, wide(t, workloadModel(t, WithScaler(&fakeScaler{}))), rowActionScale)
			return m
		},
		keymap.HelpKeybindings: func(t *testing.T) Model {
			m, _ := press(t, wideSized(t), tea.Key{Code: '?', Text: "?"})
			return m
		},
		keymap.HelpViewer: func(t *testing.T) Model {
			m := wide(t, describeViewerModel(t, &fakeDescriber{text: "Name: web-1\n"}))
			_, cmd := press(t, m, describeKey)
			next, fetch := m.Update(cmd().(rowActionMsg))
			next, _ = next.(Model).Update(fetch().(describeLoadedMsg))
			return next.(Model)
		},
		keymap.HelpTableFilter: func(t *testing.T) Model {
			m, _ := tableWith(t, "web-1", "web-2")
			m, _ = press(t, wide(t, m), slash)
			return m
		},
		keymap.HelpForwards: func(t *testing.T) Model {
			m, _ := press(t, wide(t, openPodTable(t, "Pod")), tea.Key{Code: 'F', Text: "F"})
			return m
		},
	}

	for _, ctx := range keymap.HelpContexts() {
		build, ok := reach[ctx]
		if !ok {
			t.Errorf("no model state in this table yields %s — either the surface it was "+
				"declared for has no case in hintContext (so it ships the browse hint), or "+
				"the context is dead and should be removed", ctx)
			continue
		}
		t.Run(ctx.String(), func(t *testing.T) {
			m := build(t)
			if got := m.hintContext(); got != ctx {
				t.Fatalf("this state resolves to %s, want %s — a case above it in hintContext "+
					"shadows it, and Update very likely routes the same way", got, ctx)
			}
			// Reachable is not enough: the line the hintbar actually renders must be the
			// context's, not a stale one from before the surface opened (refreshHints).
			// Contains rather than equality — the hintbar wraps what it is given in its
			// own style, and this test is about which bindings reached it.
			if got, want := m.hintbar.View(), m.help.ShortHelpContextView(ctx); !strings.Contains(got, want) {
				t.Errorf("the rendered hint is %q, want %s's %q", got, ctx, want)
			}
		})
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
