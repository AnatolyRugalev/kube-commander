package tui

import (
	"strings"
	"testing"
)

// This file is PAL-06's pin. The registry declares, per row action, whether committing
// it opens a yes/no confirm (`rowActionMeta.confirms`, spelled asksFirst/actsAtOnce),
// and the palette marks the declared ones so `Delete` and `Describe` stop reading the
// same. A declaration is only worth the name if something checks it against the code it
// claims to describe, which is what TestRowActionConfirmsMatchesTheHandlers does: it
// drives **every** registered action through its real handler with every seam wired and
// asserts a confirm appeared exactly when the registry said it would (D228).

// rowActionKinds names a kind each row action is dispatched over, so the pin drives all
// sixteen rather than the three it already suspects. A new action has no entry and fails
// the test until it gets one — deliberately: the point is that nothing joins the registry
// without its confirm answer being checked against its handler.
var rowActionKinds = map[rowAction]string{
	rowActionDescribe:       "Pod",
	rowActionEvents:         "Pod",
	rowActionLogs:           "Pod",
	rowActionChildren:       "Deployment",
	rowActionRelations:      "Pod",
	rowActionSecret:         "Secret",
	rowActionScale:          "Deployment",
	rowActionRolloutRestart: "Deployment",
	rowActionCordon:         "Node",
	rowActionUncordon:       "Node",
	rowActionDrain:          "Node",
	rowActionSuspend:        "CronJob",
	rowActionResume:         "CronJob",
	rowActionPortForward:    "Pod",
	rowActionExec:           "Pod",
	rowActionEdit:           "Pod",
	rowActionDelete:         "Pod",
}

// allActionSeams wires every seam the row actions reach. Wiring all of them is what
// makes the *negative* half of the pin mean anything: an unwired action is inert and
// would pass "no confirm" without its handler ever running, so a ShowConfirm added to
// Scale or Cordon tomorrow would go unnoticed.
func allActionSeams() []Option {
	return []Option{
		WithDescriber(&fakeDescriber{}),
		WithEventLister(&fakeEventLister{}),
		WithLogStreamer(&fakeLogStreamer{}),
		WithChildResolver(&fakeChildResolver{scope: podScope()}),
		WithRelater(&fakeRelater{}),
		WithSecretGetter(&fakeSecretGetter{}),
		WithScaler(&fakeScaler{}),
		WithRolloutRestarter(&fakeRestarter{}),
		WithCordoner(&fakeCordoner{}),
		WithSuspender(&fakeSuspender{}),
		WithDrainer(&fakeDrainer{}),
		WithPortForwarder(&fakePortForwarder{}),
		WithExecer(&fakeExecer{}),
		WithEditor(&fakeEditor{}),
		WithYAMLGetter(&fakeYAMLGetter{}),
		WithDeleter(&fakeDeleter{}),
	}
}

// TestRowActionConfirmsMatchesTheHandlers is the guard that keeps the declared set from
// becoming a fourth hand-maintained list: every registered action is dispatched over a
// real selected row with every seam wired, and the registry's answer must match what the
// handler did. Delete, Rollout restart and Drain open a confirm stamped with their own
// modal kind; the other thirteen open no confirm at all — Scale and Port-forward open a
// *prompt*, which is a different question and is not what this column marks.
func TestRowActionConfirmsMatchesTheHandlers(t *testing.T) {
	for _, meta := range rowActions {
		kind, ok := rowActionKinds[meta.action]
		if !ok {
			t.Fatalf("row action %q has no kind in rowActionKinds — add one so its "+
				"confirm answer is pinned to its handler", meta.action)
		}
		t.Run(string(meta.action), func(t *testing.T) {
			m := openPodTable(t, kind, allActionSeams()...)
			m, _ = dispatchRowAction(t, m, meta.action)

			confirmed := m.modal.Active() && !m.modal.Prompting()
			if confirmed != meta.confirms {
				t.Fatalf("%q: the registry says confirms=%v, the handler %s",
					meta.action, meta.confirms, describeModal(m))
			}
			if confirmed && m.modal.Kind() != string(meta.action) {
				t.Errorf("%q opened a confirm stamped %q, want the action's own id",
					meta.action, m.modal.Kind())
			}
		})
	}
}

// describeModal says what the modal did, in the terms the assertion is about, so a
// failure reads as "the handler opened a prompt" rather than as two booleans.
func describeModal(m Model) string {
	switch {
	case !m.modal.Active():
		return "opened no modal"
	case m.modal.Prompting():
		return "opened a prompt (" + m.modal.Kind() + "), which is not a confirm"
	default:
		return "opened a confirm (" + m.modal.Kind() + ")"
	}
}

// TestActionStageMarksTheVerbsThatConfirm is PAL-06's headline: on the stage that
// answers "what can I do to this row right now?", the verbs that will stop and ask say
// so, and the ones that act on the spot do not. A Node offers Cordon bare and Drain
// marked — the two that a bare label made indistinguishable.
func TestActionStageMarksTheVerbsThatConfirm(t *testing.T) {
	m := openPodTable(t, "Node", allActionSeams()...)
	m = openActionStage(t, m)

	marked := rowActionLabel(rowActionDrain)
	if !strings.HasSuffix(marked, confirmMarker) {
		t.Fatalf("setup: Drain should be marked, label = %q", marked)
	}
	if got, ok := m.palRowByLabel[marked]; !ok || got != rowActionDrain {
		t.Errorf("the action stage should list %q → %q, got %q (present: %v)",
			marked, rowActionDrain, got, ok)
	}
	if _, ok := m.palRowByLabel["Drain"]; ok {
		t.Error("the bare \"Drain\" label should be gone — the marker is part of the label")
	}
	if got, ok := m.palRowByLabel["Cordon"]; !ok || got != rowActionCordon {
		t.Errorf("Cordon acts at once and should be listed bare, got %q (present: %v)", got, ok)
	}
	if screen := frame(m); !strings.Contains(screen, marked) {
		t.Fatalf("the action stage should show %q on screen, got:\n%s", marked, screen)
	}
}

// TestMarkedActionStillDispatchesItsIntent proves the marker is display, not a rename:
// the label carries it, the pick resolves through it, and what comes out is the same
// rowActionMsg the direct key emits — the palette adds a way in, never a way past the
// confirm the action is marked for (D205 pt 3).
func TestMarkedActionStillDispatchesItsIntent(t *testing.T) {
	m := openPodTable(t, "Node", allActionSeams()...)
	m = openActionStage(t, m)
	m, _ = press(t, m, slash) // the stage is navigation mode (STORY-06d); `/` opens the field
	m = typeInto(t, m, "drain")

	if v, _ := m.cmdPicker.Selected(); v != rowActionLabel(rowActionDrain) {
		t.Fatalf("typing \"drain\" selected %q, want the marked label %q",
			v, rowActionLabel(rowActionDrain))
	}

	m, cmd := selectInPalette(t, m)
	if cmd == nil {
		t.Fatal("picking the marked verb should dispatch a row-action intent")
	}
	intent, ok := cmd().(rowActionMsg)
	if !ok {
		t.Fatalf("the marked verb produced %T, want rowActionMsg", cmd())
	}
	if intent.Action != rowActionDrain {
		t.Fatalf("intent action = %q, want %q", intent.Action, rowActionDrain)
	}

	next, _ := m.Update(intent)
	m = next.(Model)
	if !m.modal.Active() || m.modal.Kind() != drainModalKind {
		t.Fatal("the marked verb should still reach its confirm modal")
	}
}

// TestConfirmMarkerIsNotAnEllipsis pins the wording choice D228 records, because it is
// the kind of thing a later leg "tidies" into the GUI convention: an ellipsis reads as
// "opens a dialog", which the *unmarked* Scale and Port-forward would then contradict,
// since both open a prompt. The marker claims only what it marks.
func TestConfirmMarkerIsNotAnEllipsis(t *testing.T) {
	if strings.ContainsAny(confirmMarker, "…...") {
		t.Errorf("confirmMarker = %q — an ellipsis says \"opens a dialog\", which is also "+
			"true of the unmarked Scale and Port-forward prompts", confirmMarker)
	}
	if !strings.HasPrefix(confirmMarker, " ") {
		t.Errorf("confirmMarker = %q — it is a suffix appended to a title and needs its "+
			"own separator", confirmMarker)
	}
}
