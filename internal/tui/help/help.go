// Package help is kubecom's toggleable help overlay. It renders the effective
// keymap's bindings through bubbles/help, so the overlay (and the one-line
// status-bar hint) are generated from the action registry and can never drift
// from the actual bindings (D11). Input resolution stays in the keymap/sequencer
// the root model owns; this component only decides what help to show, never what
// a key does — it matches no raw keys.
//
// The full overlay renders as a centered, bordered modal box over the body area —
// the same overlay approach as the modal picker (M2-08a): the browse layout stays
// laid out around it (the status bar below it) rather than being replaced by a
// full-screen help page. Dismissal (esc / `?` / `q`) is resolved by the root model.
package help

import (
	bhelp "charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// helpTitle labels the modal box; helpMargin is the horizontal room kept clear of
// the box (its border plus a small gap) so the framed, centered modal never
// exceeds the screen width.
const (
	helpTitle  = "Keybindings"
	helpMargin = 6
)

// Model is the help overlay. The root model owns one, toggles it when the
// app.help action resolves, and feeds it window-size updates. It holds no shared
// mutable state (principle 1): every field is owned by the model that embeds it.
type Model struct {
	help    bhelp.Model
	keys    keymap.HelpKeyMap
	styles  styles.Styles
	visible bool
	width   int // full screen width  (the modal is centered within it)
	height  int // body-area height    (the status bar sits below, so pass bodyH)
}

// New builds a help overlay over a resolved keymap, framed through the shared
// styles. The overlay renders the full (grouped) help as a centered modal box;
// the status-bar hint uses the short view.
func New(s styles.Styles, km *keymap.Keymap) Model {
	h := bhelp.New()
	h.ShowAll = true
	return Model{help: h, keys: km.HelpMap(), styles: s}
}

// Visible reports whether the overlay is currently shown.
func (m Model) Visible() bool { return m.visible }

// Toggle flips overlay visibility (bound to the app.help action by the root model).
func (m *Model) Toggle() { m.visible = !m.visible }

// SetVisible sets overlay visibility explicitly (e.g. app.back closes it).
func (m *Model) SetVisible(v bool) { m.visible = v }

// SetWidth informs the help renderer of the available width so it can elide
// overflowing short-help items, and records it as the modal's centering width;
// wire it from the root model's WindowSizeMsg.
func (m *Model) SetWidth(w int) {
	m.width = w
	m.help.SetWidth(w)
}

// SetHeight records the body-area height the modal centers within (pass the height
// above the status bar, not the full screen, so the bar stays visible below the
// box). Wire it from the root model's resize, alongside the picker's SetSize.
func (m *Model) SetHeight(h int) { m.height = h }

// Update forwards messages to the embedded help model (a no-op today) so the
// component satisfies the usual bubble update shape as the app shell grows.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.help, cmd = m.help.Update(msg)
	return m, cmd
}

// View renders the full help as a bordered modal box, and the empty string when
// hidden or not yet sized so the caller can lay it out unconditionally. The box
// is framed with the focused-pane style and titled; the root model composites it
// centered over the base browse view (overlayCenter, D95), so the two-pane layout
// stays visible underneath rather than the whole TUI being replaced.
func (m Model) View() string {
	if !m.visible || m.width <= 0 || m.height <= 0 {
		return ""
	}
	// Constrain the full-help layout to the modal's inner width so the framed box
	// never exceeds the screen. m is a value copy — mutating the embedded help's
	// width here doesn't disturb the short-help width the status bar hint reads
	// from the model the root owns.
	inner := m.help
	innerW := m.width - helpMargin
	if innerW < 1 {
		innerW = 1
	}
	inner.SetWidth(innerW)

	title := m.styles.Header.Render(helpTitle)
	body := lipgloss.JoinVertical(lipgloss.Left, title, inner.View(m.keys))
	return m.styles.PaneFocus.Render(body)
}

// ShortHelpView renders the one-line status-bar hint (the curated, focus-agnostic
// ShortHelp subset). Independent of visibility — a status bar shows it always.
func (m Model) ShortHelpView() string {
	return m.help.ShortHelpView(m.keys.ShortHelp())
}

// ShortHelpContextView renders the one-line hint for a focus context (menu vs
// table), so the persistent bottom hint tracks what holds focus (D11 keeps it
// registry-generated; this component only lays out the bindings it is given).
func (m Model) ShortHelpContextView(ctx keymap.HelpContext) string {
	return m.help.ShortHelpView(m.keys.ShortHelpContext(ctx))
}
