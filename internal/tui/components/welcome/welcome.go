// Package welcome is kubecom's startup landing page: the right pane of the browse
// view before the user has drilled into any resource. Instead of a blank table it
// shows the app name/version, the current context and namespace, a one-line hint
// to pick a resource on the left, and the registry-generated key hints — so a
// fresh launch reads as a deliberate welcome, not an empty screen (the dogfood
// feedback this addresses). Once the user drills into a resource the root model
// renders the live table in this slot instead.
//
// Like every other component it renders purely from props the root model sets
// (SetVersion / SetContext / SetNamespace / SetShortHelp / SetSize) through the
// shared styles (D54); it owns no shared mutable state (principle 1) and never
// matches a raw key — the key hints are the string help.Model.ShortHelpView()
// generated from the effective keymap upstream (D11), so the welcome page knows
// no binding, only how to lay the pieces out.
package welcome

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// namespaceAll is the label shown when no namespace is scoped ("" = every
// namespace), so the welcome page states the scope explicitly rather than leaving
// it blank the way the terse status bar does.
const namespaceAll = "all namespaces"

// hint is the one-line call to action the welcome page centers under the header.
const hint = "Pick a resource on the left to begin."

// Model is the welcome page. Every field is owned by the embedding root model;
// nothing here is shared across goroutines.
type Model struct {
	styles styles.Styles

	version   string
	context   string
	namespace string
	shortHelp string

	width  int // total width incl. border
	height int // total height incl. border
}

// New builds a welcome page rendered through the given styles.
func New(s styles.Styles) Model {
	return Model{styles: s}
}

// SetStyles repaints the page through s, replacing the palette it was built with
// (M4-12b-1). A component caches the Styles it is handed, so a theme chosen at
// runtime reaches an already-constructed model only through this (D170 pt 2).
// Colors only: no state and no layout is disturbed.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// SetSize sets the page's total size (including its border), wired from the root
// model's table-pane geometry so the welcome page occupies exactly the table's
// slot.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// SetVersion sets the displayed build version (e.g. "dev" or a release tag).
func (m *Model) SetVersion(v string) { m.version = v }

// SetContext sets the displayed kube context name (empty renders nothing).
func (m *Model) SetContext(c string) { m.context = c }

// SetNamespace sets the displayed namespace scope (empty → "all namespaces").
func (m *Model) SetNamespace(ns string) { m.namespace = ns }

// SetShortHelp sets the key-hint line. The caller passes
// help.Model.ShortHelpView() so the hint is generated from the registry (D11).
func (m *Model) SetShortHelp(hint string) { m.shortHelp = hint }

// View renders the welcome page as a bordered pane the same total size as the
// table it stands in for, its content centered. focused selects the accented
// border so the pane reflects right-pane focus exactly as the table would. It
// returns "" until sized (before the first WindowSizeMsg), so the root model lays
// nothing out prematurely.
func (m Model) View(focused bool) string {
	frame := m.styles.Pane
	if focused {
		frame = m.styles.PaneFocus
	}
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	// The Pane style has a border; lipgloss counts it inside Width/Height, so the
	// frame is sized to the total width/height and its content area is the inner
	// (innerW × innerH) region — the same framing the table uses so the two panes
	// align pixel-for-pixel (D58).
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	innerH := m.height - 2
	if innerH < 0 {
		innerH = 0
	}

	// Vertically center the content block; each line is already width-clamped to
	// innerW (see body), so a long line — the verbose key hints — wraps inside the
	// pane instead of overflowing the border.
	content := lipgloss.PlaceVertical(innerH, lipgloss.Center, m.body(innerW))
	return frame.Width(m.width).Height(m.height).Render(content)
}

// body assembles the welcome content as full-width (innerW) centered lines: the
// app name/version header, the context · namespace scope line, the pick-a-resource
// hint, and the registry-generated key hints. Every line is rendered at innerW so
// it centers within the pane and wraps rather than overflowing the border.
func (m Model) body(innerW int) string {
	center := func(style lipgloss.Style, s string) string {
		return style.Width(innerW).Align(lipgloss.Center).Render(s)
	}

	lines := []string{center(m.styles.Header, m.header())}
	if scope := m.scope(); scope != "" {
		lines = append(lines, center(m.styles.Subtle, scope))
	}
	lines = append(lines, center(m.styles.App, ""), center(m.styles.App, hint))
	if m.shortHelp != "" {
		lines = append(lines, center(m.styles.App, ""), center(m.styles.Subtle, m.shortHelp))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// header is the app name plus the build version when set ("kubecom dev"), or the
// bare name otherwise.
func (m Model) header() string {
	if m.version == "" {
		return "kubecom"
	}
	return "kubecom " + m.version
}

// scope is the "context · namespace" line, skipping an empty context and naming
// an empty namespace "all namespaces" so the scope is always explicit.
func (m Model) scope() string {
	var parts []string
	if m.context != "" {
		parts = append(parts, m.context)
	}
	if m.namespace != "" {
		parts = append(parts, m.namespace)
	} else {
		parts = append(parts, namespaceAll)
	}
	return strings.Join(parts, " · ")
}
