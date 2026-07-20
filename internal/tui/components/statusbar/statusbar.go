// Package statusbar is kubecom's bottom status bar: a single line showing the
// current context and namespace, a spinner while async discovery is in flight,
// and the one-line keymap hint. It renders purely from props the root model
// sets (SetContext / SetNamespace / SetShortHelp / SetWidth) plus its own
// spinner; it owns no shared mutable state (principle 1), so a goroutine never
// reaches into it — the root model feeds it messages and reads its View.
//
// The short-help hint is generated from the effective keymap upstream (via
// help.Model.ShortHelpView, D11) and handed in as a string, so the status bar
// never matches a raw key or knows what any binding does — it only lays out the
// pieces it is given, through the shared styles (D54).
package statusbar

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// separator joins the left-hand segments (context, namespace, discovery hint).
const separator = " · "

// discoveringLabel follows the spinner while discovery is running.
const discoveringLabel = " discovering…"

// Model is the status bar. The root model owns one, updates its props as context
// / namespace / help change, forwards spinner ticks to Update, and lays View out
// at the bottom of the screen. Every field is owned by the embedding model;
// nothing here is shared across goroutines.
type Model struct {
	styles  styles.Styles
	spinner spinner.Model

	context     string
	namespace   string
	shortHelp   string
	errText     string
	filter      string
	discovering bool
	width       int
}

// New builds a status bar rendering through the given styles. The spinner takes
// the styles' accent (Spinner) role so a re-theme flows through one place.
func New(s styles.Styles) Model {
	sp := spinner.New()
	sp.Style = s.Spinner
	return Model{styles: s, spinner: sp}
}

// SetContext sets the displayed kube context name.
func (m *Model) SetContext(ctx string) { m.context = ctx }

// SetNamespace sets the displayed namespace (empty renders nothing).
func (m *Model) SetNamespace(ns string) { m.namespace = ns }

// SetShortHelp sets the right-aligned keymap hint. The caller passes
// help.Model.ShortHelpView() so the hint is generated from the registry (D11).
func (m *Model) SetShortHelp(hint string) { m.shortHelp = hint }

// SetFilter sets the filter indicator shown in the left segment — the live filter
// prompt while the user is typing, or the committed "/query" indicator once a
// filter is applied (both supplied by the root model, M2-09b). Empty renders
// nothing. The string is used verbatim (the caller supplies the styling/prompt),
// so it is not error-flattened like SetError.
func (m *Model) SetFilter(s string) { m.filter = s }

// SetError shows a transient error message in the bar (error-styled, taking over
// the whole line while shown). The text is flattened to a single line —
// strings.Fields collapses any embedded newlines/runs of whitespace — so a
// multi-line error string can never grow the bar past its one line and scroll the
// panes (the feedback this fixes: errors must surface inside the fixed layout).
// The root model schedules the auto-clear; an empty string clears immediately.
func (m *Model) SetError(text string) { m.errText = strings.Join(strings.Fields(text), " ") }

// ClearError removes the transient error message, returning the bar to its normal
// context · namespace · help content.
func (m *Model) ClearError() { m.errText = "" }

// HasError reports whether a transient error message is currently shown.
func (m Model) HasError() bool { return m.errText != "" }

// SetWidth informs the bar of the available terminal width so it can right-align
// the help hint and clamp overflow; wire it from the root model's WindowSizeMsg.
func (m *Model) SetWidth(w int) { m.width = w }

// Discovering reports whether the discovery spinner is currently animating.
func (m Model) Discovering() bool { return m.discovering }

// StartDiscovery marks discovery in progress and returns the Cmd that begins the
// spinner animation; the caller batches it into the update loop. Calling it while
// already discovering just re-seeds the tick (harmless: the spinner's tag dedupes
// parallel tick chains).
func (m *Model) StartDiscovery() tea.Cmd {
	m.discovering = true
	return m.spinner.Tick
}

// StopDiscovery marks discovery finished. The spinner stops on the next tick,
// because Update drops spinner ticks once discovering is false (breaking the
// self-scheduling tick chain).
func (m *Model) StopDiscovery() { m.discovering = false }

// Update advances the discovery spinner. The root model forwards messages here;
// only spinner.TickMsg is consumed, and only while discovering — a tick that
// arrives after discovery finished is dropped, which stops the animation because
// the spinner only re-schedules itself from a tick it accepts.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		if !m.discovering {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the status bar as a single line: context · namespace · [spinner
// discovering…] on the left, the help hint right-aligned. When the width is
// known the help is pushed to the right edge and the whole line is clamped to
// width; with an unknown width the pieces are simply joined left-to-right.
func (m Model) View() string {
	left := m.leftSegment()
	right := m.shortHelp

	// A transient error takes over the whole bar: it is the most important thing
	// to see, and giving it the full line (help hint dropped) keeps it on one line
	// without competing for width. It is clipped to the bar width *before* styling
	// so the outer Width render can never wrap it onto a second line (D58).
	if m.errText != "" {
		errStr := m.errText
		if m.width > 0 {
			errStr = clipRunes(errStr, m.width)
		}
		left = m.styles.Error.Render(errStr)
		right = ""
	}

	var line string
	switch {
	case m.width <= 0:
		// Width not yet known: lay the pieces out inline.
		if right == "" {
			line = left
		} else if left == "" {
			line = right
		} else {
			line = left + separator + right
		}
		return m.styles.StatusBar.Render(line)
	default:
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			// Not enough room for both: keep the left segment (the live state),
			// drop the hint, and let the style clamp any overflow.
			line = left
		} else {
			line = left + strings.Repeat(" ", gap) + right
		}
		return m.styles.StatusBar.Width(m.width).MaxWidth(m.width).Render(line)
	}
}

// leftSegment builds the "context · namespace · [spinner] discovering…" run,
// skipping empty pieces so a missing namespace doesn't leave a dangling
// separator.
func (m Model) leftSegment() string {
	var parts []string
	if m.context != "" {
		parts = append(parts, m.context)
	}
	if m.namespace != "" {
		parts = append(parts, m.namespace)
	}
	if m.filter != "" {
		parts = append(parts, m.filter)
	}
	if m.discovering {
		parts = append(parts, m.spinner.View()+discoveringLabel)
	}
	return strings.Join(parts, separator)
}

// clipRunes truncates s to at most n runes (n<=0 → empty). Error text is plain
// ASCII, so a rune cut is a safe display-width clamp; the point is only to keep
// the styled line from exceeding the bar width and wrapping (D58).
func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
