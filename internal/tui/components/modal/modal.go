// Package modal is kubecom's confirm/prompt overlay: a centered, bordered box that
// asks the user a yes/no question (confirm mode) or reads a single line of text
// (prompt mode) before an action proceeds. It replaces the original
// kube-commander's racy tcell popup (REWRITE_PLAN motivation) — that popup shared
// mutable state between the input goroutine and the redraw, the exact bug class
// this rewrite exists to remove. Here the modal is pure message-driven state: the
// root model owns one Model, feeds it resolved keymap actions, and reads its View;
// nothing is shared across goroutines (principle 1).
//
// Like the picker (M2-08a), the modal is generic and stamped with a Kind so the
// root model can tell which prompt resolved, and it emits its own message types
// (ConfirmedMsg/CancelledMsg) so it never imports the root package (D56). It is
// driven entirely through keymap.Actions and never matches a raw key for
// behaviour (D11). In confirm mode confirm.accept (default `y`/Enter) accepts and
// confirm.decline (default `n`/Esc) declines — the yes/no muscle memory plus the
// Enter/Esc modal convention, resolved in the confirm key context (D132). In prompt
// mode nav.drillIn submits and nav.back cancels, and the typed text is fed to the
// input through UpdatePrompt, the single raw-key entry point, mirroring the picker's
// own incremental filter.
//
// This slice (M2-10) is the component in isolation; the app-shell wiring — routing
// a would-be-destructive M3 action through a confirm before it runs — lands with
// the action that needs it.
package modal

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/elide"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/safetext"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// ConfirmedMsg is emitted when the user accepts the modal (nav.drillIn). Kind
// identifies which modal resolved so the root model can route the result; Value
// carries the entered text in prompt mode and is "" in confirm mode. Owned by the
// modal package — the emitter — so the root model handles this concrete type
// without the modal importing it (D56).
type ConfirmedMsg struct {
	Kind  string
	Value string
}

// CancelledMsg is emitted when the user declines or dismisses the modal
// (nav.back). Kind identifies the modal, as in ConfirmedMsg.
type CancelledMsg struct {
	Kind string
}

// mode is the modal's flavour: a yes/no confirm or a single-line text prompt.
type mode int

const (
	modeConfirm mode = iota
	modePrompt
)

// Layout: the modal is a fraction of the screen, clamped to sensible bounds, and
// centered over whatever is behind it. Mirrors the picker's geometry so the two
// overlays read as one family.
const (
	modalMinWidth = 24
	modalMaxWidth = 60
	// modalMinHeight is 4 rather than the picker's 3 because a prompt's frame is
	// four rows before any message (border ×2 + title + input), and View's D220
	// invariant — never render taller than modalSize says — can only hold if the
	// smallest box the geometry admits still fits what is rendered first-class.
	modalMinHeight = 4
	modalMaxHeight = 12
	screenMargin   = 4 // cells kept clear around the modal on each axis
	titleHeight    = 1 // the title line at the top of the box
	promptHeight   = 1 // the text-input line (prompt mode only)
)

// Model is the confirm/prompt modal. Every field is owned by the embedding root
// model; nothing here is shared across goroutines.
type Model struct {
	styles  styles.Styles
	kind    string // stamped into ConfirmedMsg/CancelledMsg
	title   string // shown at the top of the box
	message string // the question / instruction shown to the user
	mode    mode
	input   textinput.Model // captures text in prompt mode

	active bool // whether the modal is shown (captures input) — "" View when false
	width  int  // full screen width  (the modal is centered within it)
	height int  // full screen height
}

// New builds a hidden modal rendered through the shared styles. It captures no
// input and renders "" until one of ShowConfirm/ShowPrompt reveals it. The text
// input's own key bindings are the bubble defaults; the modal drives it only
// through UpdatePrompt so no hard-coded key leaks into behaviour (D11).
func New(s styles.Styles) Model {
	in := textinput.New()
	in.Prompt = "> "
	return Model{styles: s, input: in}
}

// SetStyles repaints the modal through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Colors only: an open confirm/prompt keeps its kind, title, message and typed
// text, so restyling under a live modal cannot lose an answer in progress.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// ShowConfirm configures the modal as a yes/no confirm and reveals it. Kind stamps
// the result messages; title and message are the box heading and the question.
// confirm.accept then accepts (ConfirmedMsg, empty Value), confirm.decline declines
// (CancelledMsg).
func (m *Model) ShowConfirm(kind, title, message string) {
	m.kind = kind
	m.title = title
	m.message = message
	m.mode = modeConfirm
	m.input.Blur()
	m.input.SetValue("")
	m.active = true
}

// ShowPrompt configures the modal as a single-line text prompt seeded with initial
// and reveals it, focusing the input. nav.drillIn submits the current text
// (ConfirmedMsg with Value), nav.back cancels (CancelledMsg). Returns the input's
// focus cmd (the cursor blink) so the caller can schedule it.
func (m *Model) ShowPrompt(kind, title, message, initial string) tea.Cmd {
	m.kind = kind
	m.title = title
	m.message = message
	m.mode = modePrompt
	m.input.SetValue(initial)
	m.input.CursorEnd()
	m.active = true
	cmd := m.input.Focus()
	m.syncInputWidth()
	return cmd
}

// Kind returns the modal's kind id.
func (m Model) Kind() string { return m.kind }

// Hide dismisses the modal and blurs any open input so it reopens clean next time.
func (m *Model) Hide() {
	m.active = false
	m.input.Blur()
}

// Active reports whether the modal is shown and capturing input.
func (m Model) Active() bool { return m.active }

// Prompting reports whether the modal is an open text prompt. The root model uses
// it to route raw text keys to UpdatePrompt while it is true (mirroring the
// picker's Filtering).
func (m Model) Prompting() bool { return m.active && m.mode == modePrompt }

// Value returns the current text-input contents (meaningful in prompt mode; "" in
// confirm mode).
func (m Model) Value() string { return m.input.Value() }

// SetSize records the full screen size; the modal is sized and centered within it.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.syncInputWidth()
}

// syncInputWidth re-applies the box's inner content width to the text input.
func (m *Model) syncInputWidth() {
	iw, _ := m.innerSize()
	m.input.SetWidth(iw)
}

// Update handles a resolved keymap action while the modal is active. nav.drillIn
// accepts (ConfirmedMsg — with the input text in prompt mode) and nav.back
// declines (CancelledMsg); both leave the modal for the root model to hide. The
// modal consumes actions, never raw keys (D11); an inactive modal ignores
// everything.
func (m Model) Update(a keymap.Action) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch a {
	case keymap.ActionDrillIn, keymap.ActionConfirmAccept:
		kind, value := m.kind, ""
		if m.mode == modePrompt {
			value = m.input.Value()
		}
		return m, func() tea.Msg { return ConfirmedMsg{Kind: kind, Value: value} }
	case keymap.ActionBack, keymap.ActionConfirmDecline:
		kind := m.kind
		return m, func() tea.Msg { return CancelledMsg{Kind: kind} }
	}
	return m, nil
}

// UpdatePrompt feeds one raw key to the text input. It is the modal's only raw-key
// entry point and is meaningful only while a prompt is open: the root model
// resolves control keys (drill-in, back) to actions and routes everything else —
// the text content — here, so the field captures typing without any view matching
// a raw key for behaviour (D11). A no-op otherwise.
func (m Model) UpdatePrompt(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.Prompting() {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// innerSize is the content size inside the box frame: the modal width/height minus
// the border (2 each), the title line, and — in prompt mode — the input line.
func (m Model) innerSize() (int, int) {
	mw, mh := m.modalSize()
	iw := mw - 2
	ih := mh - 2 - titleHeight
	if m.mode == modePrompt {
		ih -= promptHeight
	}
	if iw < 0 {
		iw = 0
	}
	if ih < 0 {
		ih = 0
	}
	return iw, ih
}

// modalSize is the box's total width/height (including its border): a fraction of
// the screen clamped to [min,max] and never larger than the screen minus a small
// margin. Mirrors the picker.
func (m Model) modalSize() (int, int) {
	w := clamp(m.width-screenMargin, modalMinWidth, modalMaxWidth)
	if w > m.width {
		w = m.width
	}
	h := clamp(m.height-screenMargin, modalMinHeight, modalMaxHeight)
	if h > m.height {
		h = m.height
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// View renders the modal as a bordered box, or "" when the modal is hidden or
// unsized. The box is a bordered frame with a title line, the message, and — in
// prompt mode — the text input below it. Same overlay approach as the picker and
// help modals: the root model composites the box centered over the base browse
// view (overlayCenter, D95) so the layout behind it stays put.
//
// The box never renders taller than modalSize computed (D220). The message is a
// free-form string that lipgloss wraps — and hard-wraps a token too long to
// break — so its line count comes from the caller's text, not from the geometry,
// and overlayCenter flattens onto a fixed width×height canvas: anything past the
// bottom is clipped, not scrolled. Unbounded, a long message therefore costs the
// box its bottom border and, in prompt mode, the input line rendered under it —
// the field the modal is asking the reader to fill in. So the title and the input
// are rendered first-class (innerSize already reserves both) and the message takes
// what is left: only the explanation is ever elided.
func (m Model) View() string {
	if !m.active || m.width <= 0 || m.height <= 0 {
		return ""
	}
	iw, ih := m.innerSize()
	parts := []string{m.styles.Header.Width(iw).MaxWidth(iw).Render(m.title)}
	if ih > 0 {
		// Sanitized before it is wrapped and elided (AUTH-06, D232): the message is
		// the one part of the box composed from text kubecom did not write — an
		// object's name, and for the re-authenticate confirm the invocation the
		// kubeconfig's exec stanza spells — and a control character in it would
		// repaint over the border of the box asking the question.
		message := m.styles.App.Width(iw).MaxWidth(iw).Render(safetext.Block(m.message))
		marker := m.styles.App.Width(iw).MaxWidth(iw).Render(elide.Marker)
		parts = append(parts, elide.Lines(message, ih, marker))
	}
	if m.mode == modePrompt {
		parts = append(parts, m.styles.App.Width(iw).MaxWidth(iw).Render(m.input.View()))
	}
	return m.styles.PaneFocus.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// The clamp and its marker live in internal/tui/elide: BOX-03 gave the keybindings
// overlay the same treatment, and a second copy of a wording convention is how a
// convention stops being one. elide.Marker is the phrase browsefail.go already uses
// when it drops the tail of a credential plugin's stderr — a sentence rather than a
// bare ellipsis because a modal is a question, and text silently missing from what
// is being agreed to is the thing worth naming.
