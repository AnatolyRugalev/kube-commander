// Package forwards is kubecom's port-forward panel (M3-13b): a global overlay
// listing the active background port-forwards. It is the M3-13b half of the
// port-forward feature — the *listing* surface — moved out of the root shell into
// a self-contained components/* sub-model (D264/MONO-01): the shell still owns
// the running forwards (their handles and cancels, which only it can tear down),
// and hands this panel a read-only Entry for each one. The panel owns only its
// own state — whether it is open and where the cursor is — plus the geometry
// (BOX-02) and key-footer (HINT-04) rules that make it a good overlay.
//
// The panel is not row-scoped: forwards outlive the row they started on, so it
// opens from anywhere in the browse view. It captures input while it is up —
// nav.up/down move the cursor, nav.drillIn stops the selected forward (the shell
// performs the cancel), forwards.stopAll stops every one, forwards.panel/nav.back/
// app.quit close it — swallowing the rest so the browse panes underneath never
// move. All of that routing lives in the shell; here the panel is pure state and
// rendering, driven through keymap.Actions and never a raw key (D11).
package forwards

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// Entry is the panel's read-only view of one running forward: what the list needs
// to render (label, ports, readiness), with no handle or cancel — those are the
// shell's, which is what lets it stop a forward the panel can only display.
type Entry struct {
	ID    int
	Label string
	Specs []string
	Bound []kube.ForwardedPort
	Ready bool
}

// Model is the port-forward panel. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	styles styles.Styles
	open   bool // whether the panel is up (captures input) — "" View when false
	sel    int  // cursor into the entry set (nav.up/down move it)
}

// New builds a closed panel rendered through the shared styles.
func New(s styles.Styles) Model {
	return Model{styles: s}
}

// SetStyles repaints the panel through s, replacing the palette it was built with
// (M4-12b-1). Colors only: an open panel keeps its cursor, so restyling under a
// live panel loses nothing.
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// Active reports whether the panel is shown and capturing input.
func (m Model) Active() bool { return m.open }

// Sel returns the panel cursor's index into the entry set.
func (m Model) Sel() int { return m.sel }

// SetSel seeds the cursor — used by the shell's tests to position the panel for a
// specific geometry. The caller is responsible for the value being valid for the
// entry set; Move and Clamp are the behaviour paths.
func (m *Model) SetSel(n int) { m.sel = n }

// Open shows the panel. The cursor is clamped by the caller to the current set (a
// forward may have ended since the panel was last open).
func (m *Model) Open() {
	m.open = true
}

// Close hides the panel, keeping the cursor where it was (clamped again on the
// next Open).
func (m *Model) Close() {
	m.open = false
}

// Reset hides the panel and zeroes the cursor — the teardown a context switch
// uses, so a fresh cluster's panel starts at the top.
func (m *Model) Reset() {
	m.open = false
	m.sel = 0
}

// Clamp keeps sel a valid index into a set of n entries: 0 when the set is empty,
// otherwise within [0, n-1]. Called whenever the set or the panel open changes.
func (m *Model) Clamp(n int) {
	if m.sel < 0 || n == 0 {
		m.sel = 0
		return
	}
	if m.sel >= n {
		m.sel = n - 1
	}
}

// Move steps the cursor by delta over a set of n entries, clamping at both ends.
// n == 0 pins the cursor to 0, so a down/up press against an empty set is inert.
func (m *Model) Move(delta, n int) {
	if n == 0 {
		m.sel = 0
		return
	}
	if m.sel += delta; m.sel < 0 {
		m.sel = 0
	} else if m.sel >= n {
		m.sel = n - 1
	}
}

// View renders the panel over entries: a bordered box listing each entry — its
// label and bound (or requested) ports and whether it is ready — with the cursor
// row highlighted, plus a footer of the panel's keys. With no entries it shows an
// empty-state line. The box is composited centered over the browse view by the
// shell (overlayCenter, D95), which is why width/height are the shell's full
// screen size.
//
// The box bounds its own height (D220 pt 1): the shell's overlayCenter flattens
// onto a fixed width×bodyHeight canvas and clips bottom-first, so an unbounded
// list used to cost the panel its footer and its bottom border, and — because this
// is the one overlay with a *cursor* — could hide the selected row with nothing on
// screen saying so (BOX-02). The rows therefore scroll rather than truncate: a
// window of what fits that follows m.sel, derived from the cursor alone so the
// panel keeps no scroll state of its own to resize, clamp, or forget (Window). The
// title and the footer are rendered first-class like the modal's title and input;
// only the list is elided, and the title says so by counting (Title).
func (m Model) View(entries []Entry, width, height int, km *keymap.Keymap) string {
	if !m.open {
		return ""
	}
	iw := width - 6 // leave a margin; the box border adds 2 back.
	if iw > 64 {
		iw = 64
	}
	if iw < 20 {
		iw = 20
	}
	ih := height - 2 // the border takes one row at the top and one at the bottom
	if ih <= 0 {
		return "" // nothing the compositor would not clip away entirely
	}
	// The title always takes a row; the footer only when a content row survives it —
	// a box listing nothing but its keys is worse than one with no footer.
	rows := ih - 1
	footer := Footer(km)
	if footer != "" && rows >= 2 {
		rows--
	} else {
		footer = ""
	}

	start, end := Window(len(entries), m.sel, rows)
	lines := []string{m.styles.Header.Width(iw).MaxWidth(iw).Render(Title(len(entries), start, end))}
	switch {
	case rows <= 0: // title-only box: the screen has room for nothing else
	case len(entries) == 0:
		lines = append(lines, m.styles.Subtle.Width(iw).MaxWidth(iw).Render("No active port-forwards."))
	default:
		for i := start; i < end; i++ {
			e := entries[i]
			status := "starting…"
			if e.Ready {
				status = "ready"
			}
			row := fmt.Sprintf("%s  %s  [%s]", e.Label, PortsLabel(e), status)
			gutter := "  "
			style := m.styles.App
			if i == m.sel {
				gutter = "> "
				style = m.styles.Selection
			}
			lines = append(lines, style.Width(iw).MaxWidth(iw).Render(gutter+row))
		}
	}
	if footer != "" {
		lines = append(lines, m.styles.Subtle.Width(iw).MaxWidth(iw).Render(footer))
	}
	return m.styles.PaneFocus.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// PortsLabel renders an entry's ports: the bound local:remote pairs once Ready has
// filled them (e.g. "localhost:8080 → 80"), else the requested specs as typed.
// It is the string the panel row and the shell's "forwarding …" notice share.
func PortsLabel(e Entry) string {
	if len(e.Bound) == 0 {
		return strings.Join(e.Specs, " ")
	}
	parts := make([]string, len(e.Bound))
	for i, p := range e.Bound {
		parts[i] = fmt.Sprintf("localhost:%d → %d", p.Local, p.Remote)
	}
	return strings.Join(parts, ", ")
}

// Window is the half-open range of entry indices the panel shows when it has room
// for n rows: everything when it fits, otherwise the least-scrolled window that
// still contains sel. It is a pure function of the cursor rather than a stored
// offset, which is what keeps the panel free of scroll state that resize, a
// stopped forward, or Clamp would each have to maintain — the list is short enough
// that the sticky-offset feel a table needs is not worth that.
func Window(total, sel, n int) (int, int) {
	if n <= 0 || total <= 0 {
		return 0, 0
	}
	if n >= total {
		return 0, total
	}
	start := 0
	if sel >= n {
		start = sel - n + 1
	}
	if start > total-n {
		start = total - n
	}
	if start < 0 {
		start = 0
	}
	return start, start + n
}

// Title names the panel and, when the window hides rows, which slice of the list
// is on screen. The count rides the title because that is the one row the panel is
// guaranteed to have: a marker row (the modal's answer, D220 pt 3) would have to
// be taken from the list it is describing, and unlike a truncated message this
// list is scrollable — the reader can reach what is hidden, they just need to be
// told it is there. A panel showing everything says nothing, so the counter is
// evidence of elision rather than furniture.
func Title(total, start, end int) string {
	switch {
	case total == 0 || end-start >= total:
		return "Port-forwards"
	case end <= start:
		// The box is so short that the title is all of it: no row is on screen to
		// number, so the title carries the bare count rather than an empty range.
		return fmt.Sprintf("Port-forwards (%d)", total)
	default:
		return fmt.Sprintf("Port-forwards (%d–%d of %d)", start+1, end, total)
	}
}

// Footer builds the panel's key footer from the resolved keymap instead of
// spelling literal keys, so a rebind moves it (D11) — this was the last view in
// kubecom that wrote a key into its own body (HINT-04). Only the *keys* are
// generated: the verbs stay here because the registry's descriptions are global
// (D218 pt 2) and nav.drillIn's is "Open / drill into selection", while in the
// panel it stops the selected forward — this footer is the one place the panel's
// verbs are stated, which is why HINT-03 declined to delete it in favour of the
// hint line. An action the user disabled drops out entirely rather than rendering
// a bare verb, the same trade portPickerTitle makes; disable all three and the
// footer line itself disappears.
func Footer(km *keymap.Keymap) string {
	entries := []struct {
		action keymap.Action
		verb   string
	}{
		{keymap.ActionDrillIn, "stop"},
		{keymap.ActionStopForwards, "stop all"},
		{keymap.ActionBack, "close"},
	}
	var parts []string
	for _, e := range entries {
		if k := firstKey(km, e.action); k != "" {
			parts = append(parts, k+": "+e.verb)
		}
	}
	return strings.Join(parts, " · ")
}

// firstKey is an action's primary bound key ("" when the user disabled it), the
// token a hint shows. It mirrors the shell's helper of the same name; the
// component cannot import the root package (which imports it) and the three lines
// are not worth a shared seam.
func firstKey(km *keymap.Keymap, a keymap.Action) string {
	if ks := km.Keys(a); len(ks) > 0 {
		return ks[0]
	}
	return ""
}
