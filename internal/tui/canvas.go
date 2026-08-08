package tui

import (
	"fmt"
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// The canvas check: did the palette's background actually reach the screen?
//
// THEME-03 (D249) hands `Theme.Background` to the terminal as its default
// background via `tea.View.BackgroundColor`. That is a *request*: a terminal is
// free to ignore OSC 11, and a multiplexer without passthrough eats it outright.
// Where it does not land kubecom draws its text over whatever the terminal
// already is, which is invisible while every palette is dark on a dark terminal
// and unreadable the moment the two polarities disagree — a dark palette on a
// light terminal today, a light palette on a dark one once THEME-04b lands.
//
// So kubecom asks. `tea.RequestBackgroundColor` sends OSC 11 as a query and the
// terminal's own answer comes back as `tea.BackgroundColorMsg`, which is the only
// evidence available about what the screen actually is — the alternative,
// sniffing `$TMUX`/`$TERM` for terminals believed to filter, is a list that is
// wrong the day it is written.

const (
	// canvasProbeDelay defers the query until well after the frame that carries
	// the background request. The renderer emits the OSC on the first paint and
	// the query is a Cmd, so an immediate probe races its own set and would read
	// back the terminal's *previous* background — a false "it did not land" on a
	// terminal where it did. A single late probe is preferred to a fast one with a
	// retry: the answer is only ever used to decide whether to say one sentence.
	canvasProbeDelay = 750 * time.Millisecond
)

// canvasProbeMsg fires canvasProbeDelay after the palette reached the terminal
// (at launch, and again after a runtime theme switch). gen drops a tick left over
// from a superseded palette, the same stale-tick guard seqGen and statusErrGen
// use.
type canvasProbeMsg struct{ gen int }

// scheduleCanvasProbe arms the deferred query for the palette the model is
// holding *now*. It is the only place the probe is started, so launch and a
// runtime switch cannot drift apart in timing. It reads canvasGen rather than
// bumping it, which is what lets Init (a value receiver, whose mutations are
// discarded) arm the first probe; a switch bumps the counter itself, before
// arming, so its own tick is the only live one.
func (m Model) scheduleCanvasProbe() tea.Cmd {
	gen := m.canvasGen
	return tea.Tick(canvasProbeDelay, func(time.Time) tea.Msg {
		return canvasProbeMsg{gen: gen}
	})
}

// handleCanvasProbe issues the OSC 11 query for a live probe. tea.RequestBackground
// Color is itself a tea.Cmd (it returns a Msg the program translates into the
// escape), so nothing here talks to the terminal directly.
func (m Model) handleCanvasProbe(msg canvasProbeMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.canvasGen {
		return m, nil
	}
	m.awaitingCanvas = true
	return m, tea.RequestBackgroundColor
}

// handleBackgroundColor reads the terminal's answer. Only an answer kubecom asked
// for is acted on: a terminal may report its background unprompted (some emit one
// on a theme change of their own), and an unsolicited report says nothing about
// whether *kubecom's* request landed.
func (m Model) handleBackgroundColor(msg tea.BackgroundColorMsg) (tea.Model, tea.Cmd) {
	if !m.awaitingCanvas {
		return m, nil
	}
	m.awaitingCanvas = false
	return m, m.checkCanvas(msg.Color)
}

// checkCanvas compares the terminal's actual background against the palette's and
// warns only when the reader is looking at a screen the palette does not fit.
//
// The three silences are deliberate, because a warning nobody can act on is worse
// than none (D250 pt 3):
//
//   - The colors match: the request landed, kubecom owns the canvas.
//   - The colors differ but the polarities agree: the request did not land, and it
//     does not matter — a dark palette on a *different* dark terminal is the
//     ordinary case for the ten built-ins and reads exactly as it did before D249.
//   - Nothing was reported at all: a terminal that answers no OSC 11 query cannot
//     be distinguished from one that is merely slow, and "unknown" is not evidence.
//
// The remaining case — the polarities disagree — is the one where text is about to
// be drawn in a color close to the background it lands on, and it is stated in
// those terms rather than as "your terminal filters OSC 11", because that is a
// guess about the cause and the mismatch is the fact.
func (m *Model) checkCanvas(reported color.Color) tea.Cmd {
	want := m.styles.Theme.Background
	if want == nil || reported == nil {
		return nil
	}
	if styles.SameColor(reported, want) {
		return nil
	}
	if styles.IsDark(reported) == styles.IsDark(want) {
		return nil
	}

	name := m.styles.Theme.Name
	if name == "" {
		name = "the theme"
	}
	// The toast is clipped to the terminal width, so it carries the fact and the
	// log carries what to do about it — surfaceError's own split (D159).
	m.logger.Warn("the theme's background did not reach the terminal",
		"theme", name,
		"theme_background", hexOf(want),
		"terminal_background", hexOf(reported),
		"remedy", "kubecom asks for its background with OSC 11; a multiplexer without passthrough "+
			"(tmux: set-option -g allow-passthrough on) and some terminals drop it. Pick a palette "+
			"matching the terminal, or set the terminal's background to the palette's. "+
			"See docs/configuration.md.")
	return m.surfaceError(ErrorMsg{Context: canvasMismatch(name, want, reported)})
}

// canvasMismatch is the one line the reader gets. It is a function so the wording
// is assertable without going through the status bar's width clipping, and it
// names the *observation* — this palette, that screen — rather than a cause,
// because kubecom cannot tell a terminal that filtered the request from one that
// refused it.
func canvasMismatch(name string, want, reported color.Color) string {
	return fmt.Sprintf("%s is %s but the terminal stayed %s — background not applied",
		name, polarityWord(want), polarityWord(reported))
}

// polarityWord names a background's polarity for the one line the user reads.
func polarityWord(c color.Color) string {
	if styles.IsDark(c) {
		return "dark"
	}
	return "light"
}

// hexOf renders a color as #rrggbb for the log, so the two sides of a mismatch
// are comparable against the palette's source values by eye.
func hexOf(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
