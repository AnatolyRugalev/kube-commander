// Package statusbar is kubecom's status bar: a single line showing the current
// context and namespace, the live filter indicator, and a spinner while async
// discovery is in flight (or a transient error toast taking over the whole line).
// It renders purely from props the root model sets (SetContext / SetNamespace /
// SetFilter / SetError / SetWidth) plus its own spinner; it owns no shared mutable
// state (principle 1), so a goroutine never reaches into it — the root model feeds
// it messages and reads its View.
//
// The persistent keymap hint used to be right-aligned on this bar, but it now
// lives on its own dedicated line below (the hintbar component,
// FB-hintbar-dedicated) so the live state and the hint no longer compete for one
// line — the hint is never dropped under width pressure nor hidden behind an error
// toast. This bar therefore lays out only its left segment (or a full-line error),
// clamped to width, through the shared styles (D54).
package statusbar

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

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

	context      string
	namespace    string
	resourceType string
	scope        string
	errText      string
	noticeText   string
	filter       string
	mouse        bool
	discovering  bool
	width        int
}

// New builds a status bar rendering through the given styles. The spinner takes
// the styles' accent (Spinner) role so a re-theme flows through one place.
func New(s styles.Styles) Model {
	sp := spinner.New()
	sp.Style = s.Spinner
	return Model{styles: s, spinner: sp}
}

// SetStyles repaints the bar through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
//
// The spinner's Style has to be re-derived, not left alone: New copies the accent
// role *into* the bubble, so assigning m.styles by itself would leave a spinning
// discovery indicator in the previous theme's accent while the bar around it moved.
// Its animation frame is preserved, so a restyle mid-discovery does not stutter.
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.spinner.Style = s.Spinner
}

// SetContext sets the displayed kube context name.
func (m *Model) SetContext(ctx string) { m.context = ctx }

// SetNamespace sets the displayed namespace (empty renders nothing).
func (m *Model) SetNamespace(ns string) { m.namespace = ns }

// SetResourceType sets the displayed resource type — the kind currently being
// browsed (e.g. "Pod"), shown alongside context · namespace so the bar always
// names what the table is listing (feedback 2026-07-22-status-bar-top). Empty
// renders nothing (before the first drill-in there is no resource yet).
func (m *Model) SetResourceType(kind string) { m.resourceType = kind }

// SetScope sets the drill-down scope indicator shown in the left segment — the
// owner and selector the browsed table is narrowed to while a children drill-down
// is open (M4-08), e.g. "↳ Deployment/api · app=web". Empty renders nothing, which
// is the ordinary browse case. Like SetFilter the string is used verbatim: the root
// model composes it, so the bar stays a dumb renderer and does not have to know what
// a ChildScope is.
func (m *Model) SetScope(s string) { m.scope = s }

// SetFilter sets the filter indicator shown in the left segment — the live filter
// prompt while the user is typing, or the committed "/query" indicator once a
// filter is applied (both supplied by the root model, M2-09b). Empty renders
// nothing. The string is used verbatim (the caller supplies the styling/prompt),
// so it is not error-flattened like SetError.
func (m *Model) SetFilter(s string) { m.filter = s }

// SetMouse sets the persistent mouse-capture indicator. Mouse capture is off by
// default so the terminal's own select-to-copy works (D97); when the user turns
// it on (mouse.toggle) the bar shows a `mouse` marker so this otherwise-invisible
// mode is always visible. Off renders nothing.
func (m *Model) SetMouse(on bool) { m.mouse = on }

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

// SetNotice shows a transient neutral message in the bar (the secret-copy
// confirmation, M3-08b) — the non-error twin of SetError. Like the error it is
// flattened to a single line and takes over the whole bar while shown, but an error
// outranks it (View prefers errText) so a failure is never hidden behind a notice.
// The root model schedules the auto-clear; an empty string clears immediately.
func (m *Model) SetNotice(text string) { m.noticeText = strings.Join(strings.Fields(text), " ") }

// ClearNotice removes the transient notice, returning the bar to its normal content.
func (m *Model) ClearNotice() { m.noticeText = "" }

// HasNotice reports whether a transient notice is currently shown.
func (m Model) HasNotice() bool { return m.noticeText != "" }

// SetWidth informs the bar of the available terminal width so it can fill the line
// and clamp overflow; wire it from the root model's WindowSizeMsg.
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

// View renders the status bar as a single line: context · namespace · filter ·
// [spinner discovering…], or a transient error taking over the whole line. The
// persistent keymap hint lives on its own line below now (the hintbar), so the bar
// no longer lays out a right-aligned segment — it renders its left segment,
// background-filled to the known width and clamped so it can never wrap onto a
// second line (D58).
func (m Model) View() string {
	line := m.leftSegment()

	// A transient error takes over the whole bar: it is the most important thing to
	// see, and giving it the full line keeps it on one line. It is clipped to the
	// bar width *before* styling so the outer Width render can never wrap it onto a
	// second line (D58). A neutral notice (secret copy, M3-08b) does the same but is
	// outranked by an error, so a failure is never hidden behind a confirmation.
	switch {
	case m.errText != "":
		errStr := m.errText
		if m.width > 0 {
			errStr = clipRunes(errStr, m.width)
		}
		line = m.styles.Error.Render(errStr)
	case m.noticeText != "":
		noticeStr := m.noticeText
		if m.width > 0 {
			noticeStr = clipRunes(noticeStr, m.width)
		}
		line = m.styles.Accent.Render(noticeStr)
	}

	if m.width <= 0 {
		return m.styles.StatusBar.Render(line)
	}
	return m.styles.StatusBar.Width(m.width).MaxWidth(m.width).Render(line)
}

// leftSegment builds the "context · namespace · resourceType · scope · filter · mouse ·
// [spinner] discovering…" run, skipping empty pieces so a missing namespace (or a
// pre-drill-in empty resource type) doesn't leave a dangling separator.
func (m Model) leftSegment() string {
	var parts []string
	if m.context != "" {
		parts = append(parts, m.context)
	}
	if m.namespace != "" {
		parts = append(parts, m.namespace)
	}
	if m.resourceType != "" {
		parts = append(parts, m.resourceType)
	}
	// The scope sits directly after the kind it narrows: "Pod · ↳ Deployment/api ·
	// app=web" reads as one clause, and a filter typed on top of it follows.
	if m.scope != "" {
		parts = append(parts, m.scope)
	}
	if m.filter != "" {
		parts = append(parts, m.filter)
	}
	if m.mouse {
		parts = append(parts, "mouse")
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
