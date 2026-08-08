package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// probe drives the model to the point where an answer would be accepted: the
// deferred tick has fired and the OSC 11 query has gone out. It returns the model
// and the Cmd the probe produced, so a test can assert both the state and the
// request.
func probe(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(canvasProbeMsg{gen: m.canvasGen})
	return next.(Model), cmd
}

// answer feeds the terminal's reply for a live probe.
func answer(t *testing.T, m Model, hex string) Model {
	t.Helper()
	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color(hex)})
	return next.(Model)
}

func TestCanvasProbeAsksTheTerminalForItsBackground(t *testing.T) {
	// The probe is not a self-check: kubecom cannot know from inside whether the
	// escape it emitted survived the trip, so the tick's whole job is to send the
	// OSC 11 *query* and wait for the terminal's own answer.
	m, cmd := probe(t, sizedWith(t))
	if !m.awaitingCanvas {
		t.Error("after the probe tick the model is not awaiting an answer")
	}
	if cmd == nil {
		t.Fatal("the probe tick produced no command; the query was never sent")
	}
	// tea.RequestBackgroundColor returns an unexported msg the program translates
	// into the escape, so the type is the only observable claim from out here —
	// the same reflection handover_test.go uses on tea's own unexported types.
	if got, want := reflect.TypeOf(cmd()).String(), "tea.backgroundColorMsg"; got != want {
		t.Errorf("the probe sent %s, want %s (tea.RequestBackgroundColor)", got, want)
	}
}

func TestStaleCanvasProbeIsDropped(t *testing.T) {
	// A tick armed for a palette the user has switched away from must not query on
	// its behalf: its answer would be measured against the *new* theme.
	m := sizedWith(t)
	stale := canvasProbeMsg{gen: m.canvasGen}
	m.canvasGen++
	next, cmd := m.Update(stale)
	if cmd != nil {
		t.Error("a superseded probe tick still sent a query")
	}
	if next.(Model).awaitingCanvas {
		t.Error("a superseded probe tick armed the answer gate")
	}
}

func TestUnsolicitedBackgroundReportIsIgnored(t *testing.T) {
	// A terminal may report its background without being asked (some do on a theme
	// change of their own). That says nothing about whether *kubecom's* request
	// landed, so it must not be read as evidence — here it disagrees in polarity
	// with the dark default and still raises nothing.
	m := answer(t, sizedWith(t), "#ffffff")
	if m.status.HasError() {
		t.Error("an unsolicited background report was treated as an answer to the probe")
	}
}

func TestCanvasThatLandedIsSilent(t *testing.T) {
	// The terminal reporting the palette's own canvas is the success case: the
	// request was honoured and kubecom owns the screen.
	th := styles.GruvboxDarkTheme()
	m, _ := probe(t, sizedWith(t, WithTheme(th)))
	if m = answer(t, m, "#282828"); m.status.HasError() {
		t.Error("the terminal reported the palette's own canvas and kubecom still warned")
	}
	if m.awaitingCanvas {
		t.Error("the answer did not close the gate; a later report would be read as a second answer")
	}
}

func TestADifferentBackgroundOfTheSamePolarityIsSilent(t *testing.T) {
	// This is the ordinary case for the eleven dark built-ins on a terminal that ignores
	// the request, and it is exactly how kubecom rendered before D249 — dark text
	// weights on a dark terminal that is merely a different dark. Warning here would
	// put a toast on a working screen at every launch, which is the failure mode
	// D250 pt 3 is written against.
	m, _ := probe(t, sizedWith(t, WithTheme(styles.GruvboxDarkTheme())))
	if m = answer(t, m, "#000000"); m.status.HasError() {
		t.Errorf("a different *dark* terminal background raised a warning")
	}
}

func TestOppositePolarityIsReported(t *testing.T) {
	// The case that matters: the request did not land and the screen kubecom is
	// drawing on is the other polarity, so its text is about to be near-invisible.
	th := styles.GruvboxDarkTheme()
	m, _ := probe(t, sizedWith(t, WithTheme(th)))
	m = answer(t, m, "#fbf1c7") // gruvbox *light*'s bg0 — a real light terminal
	if !m.status.HasError() {
		t.Fatal("a dark palette on a light terminal raised nothing")
	}
	want := "gruvbox-dark is dark but the terminal stayed light — background not applied"
	if got := canvasMismatch(th.Name, th.Background, lipgloss.Color("#fbf1c7")); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestCanvasMismatchReadsBothWays(t *testing.T) {
	// The line is written from the palette's side, so it must be right for the
	// light palette THEME-04b lands as well as for today's dark registry — the
	// direction is not hard-coded.
	got := canvasMismatch("catppuccin-latte", lipgloss.Color("#eff1f5"), lipgloss.Color("#1e1e2e"))
	want := "catppuccin-latte is light but the terminal stayed dark — background not applied"
	if got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestZeroThemeMakesNoClaimAboutTheCanvas(t *testing.T) {
	// A palette with no canvas never asked for anything, so nothing can have failed
	// to land — it renders on the terminal's own background by design (D249 pt 3).
	m, _ := probe(t, sizedWith(t, WithTheme(styles.Theme{})))
	if m = answer(t, m, "#ffffff"); m.status.HasError() {
		t.Error("a zero Theme warned about a background it never requested")
	}
}

func TestThemeSwitchReArmsTheProbeAndRetiresTheOldOne(t *testing.T) {
	// A live switch changes the canvas, so the question has to be asked again —
	// and the previous palette's in-flight answer must not be graded against the
	// new palette.
	m, _ := probe(t, sizedWith(t))
	before := m.canvasGen

	next, _ := m.applyThemeNamed("monokai")
	m = next.(Model)
	if m.canvasGen == before {
		t.Error("a theme switch did not retire the previous palette's probe")
	}
	if m.awaitingCanvas {
		t.Error("a theme switch left the old palette's answer gate open")
	}
	// The old palette's reply now arrives. It is dropped by the gate rather than
	// measured against monokai.
	if m = answer(t, m, "#ffffff"); m.status.HasError() {
		t.Error("an answer from before the switch was graded against the new palette")
	}
}
