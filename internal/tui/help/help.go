// Package help is kubecom's toggleable help overlay. It renders the effective
// keymap's bindings through bubbles/help, so the overlay (and the one-line
// status-bar hint) are generated from the action registry and can never drift
// from the actual bindings (D11). Input resolution stays in the keymap/sequencer
// the root model owns; this component only decides what help to show, never what
// a key does — it matches no raw keys.
package help

import (
	bhelp "charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// Model is the help overlay. The root model owns one, toggles it when the
// app.help action resolves, and feeds it window-size updates. It holds no shared
// mutable state (principle 1): every field is owned by the model that embeds it.
type Model struct {
	help    bhelp.Model
	keys    keymap.HelpKeyMap
	visible bool
}

// New builds a help overlay over a resolved keymap. The overlay renders the full
// (grouped) help; the status-bar hint uses the short view.
func New(km *keymap.Keymap) Model {
	h := bhelp.New()
	h.ShowAll = true
	return Model{help: h, keys: km.HelpMap()}
}

// Visible reports whether the overlay is currently shown.
func (m Model) Visible() bool { return m.visible }

// Toggle flips overlay visibility (bound to the app.help action by the root model).
func (m *Model) Toggle() { m.visible = !m.visible }

// SetVisible sets overlay visibility explicitly (e.g. app.back closes it).
func (m *Model) SetVisible(v bool) { m.visible = v }

// SetWidth informs the help renderer of the available width so it can elide
// overflowing short-help items; wire it from the root model's WindowSizeMsg.
func (m *Model) SetWidth(w int) { m.help.SetWidth(w) }

// Update forwards messages to the embedded help model (a no-op today) so the
// component satisfies the usual bubble update shape as the app shell grows.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.help, cmd = m.help.Update(msg)
	return m, cmd
}

// View renders the full help overlay when visible, and the empty string when
// hidden so the caller can lay it out unconditionally.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	return m.help.View(m.keys)
}

// ShortHelpView renders the one-line status-bar hint (the curated ShortHelp
// subset). Independent of visibility — a status bar shows it always.
func (m Model) ShortHelpView() string {
	return m.help.ShortHelpView(m.keys.ShortHelp())
}
