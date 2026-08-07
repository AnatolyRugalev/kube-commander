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

	"github.com/neuroplastio/kubecom/internal/tui/elide"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// helpTitle labels the modal box; helpMargin is the horizontal room kept clear of
// the box (its border plus a small gap) so the framed, centered modal never
// exceeds the screen width. helpBorder and helpTitleHeight are what the box spends
// before a single binding is on screen, and are what View subtracts from the height
// it is given (BOX-03).
const (
	helpTitle       = "Keybindings"
	helpMargin      = 6
	helpBorder      = 2 // the pane frame: one row at the top, one at the bottom
	helpTitleHeight = 1 // the title line under the top border
)

// helpTruncated marks a keymap too tall for the box. It departs from elide.Marker
// deliberately: the modal's elided text is an explanation the reader cannot get at
// any other way, so naming the loss is the whole remedy — but every row cut here is
// a binding someone opened this overlay to look up, and the full list is committed
// at docs/keybindings.md, generated from this same registry (D11). A marker that
// only said "truncated" would leave the reader stuck; this one says where the rest
// is.
const helpTruncated = "… (truncated — full list in docs/keybindings.md)"

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

// SetStyles repaints the overlay's frame and title through s, replacing the palette
// it was built with (M4-12b-1). A component caches the Styles it is handed, so a
// theme chosen at runtime reaches an already-constructed model only through this
// (D170 pt 2). The embedded bubbles/help renderer keeps its own styling, which this
// package has never themed — the bindings it lays out are unaffected, as are the
// keymap, the visibility and the recorded size.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

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
//
// The box bounds its own height (D220 pt 1). bubbles/help with ShowAll lays the
// registry's namespaces out as columns and elides them *horizontally* to the width
// it is given, so the height is whatever the tallest column needs — a number this
// package does not choose and that grows with the action registry (fifteen rows
// including the frame when BOX-03 measured it, at every screen size from 200×60
// down to 60×6). overlayCenter flattens onto a fixed width×bodyHeight canvas and
// clips bottom-first, so on any shorter body the overlay silently lost its bottom
// bindings and its border. The title is rendered first-class and the bindings take
// what is left; there is no cursor here, so the elision is marked rather than
// scrolled (D221 covers the surfaces that can be walked).
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

	// Rows left for the bindings once the frame and the title are paid for. At zero
	// the box would be nothing but a clipped border, which is not more honest than
	// no box — the hint line one row below still says `?` toggles this overlay, so
	// the reader is not left without a way back (the forwards panel does the same).
	ih := m.height - helpBorder - helpTitleHeight
	if ih <= 0 {
		return ""
	}
	marker := m.styles.App.MaxWidth(innerW).Render(helpTruncated)

	title := m.styles.Header.Render(helpTitle)
	body := lipgloss.JoinVertical(lipgloss.Left, title, elide.Lines(inner.View(m.keys), ih, marker))
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
