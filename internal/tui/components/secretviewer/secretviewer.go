// Package secretviewer is the secret viewer's entry list (M3-08a/08b): the
// reveal/mask toggle, the entry cursor and the body render for a Secret shown in
// the shell's shared read-only viewer. It is the M3-08 secret half of the root
// shell moved into a self-contained components/* sub-model (D264/MONO-01),
// following the seam D265 settled for a shell-owned listing: the shell keeps the
// fetched kube.SecretData (the authoritative set) and hands this model read-only
// data at render time; the model owns only its interaction state — whether
// values are unmasked and where the cursor is — and the body it renders into the
// viewer. The clipboard copy stays a shell gesture, performed against the
// shell's own data at Sel().
package secretviewer

import (
	"fmt"
	"strings"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// Model is the secret viewer's entry-list state. Every field is owned by the
// embedding root model; nothing here is shared across goroutines (principle 1).
type Model struct {
	revealed bool // whether values are unmasked (false on open — the deliberate-reveal contract, #89)
	sel      int  // entry cursor into the set (nav.up/down move it, M3-08b)
}

// Revealed reports whether values are currently unmasked.
func (m Model) Revealed() bool { return m.revealed }

// Sel returns the entry cursor's index into the set.
func (m Model) Sel() int { return m.sel }

// Reset masks the values and zeroes the cursor — the state every fresh open (or
// a context switch) wants: values stay hidden until the reader's own gesture
// (the deliberate-reveal contract) and the cursor starts at the top.
func (m *Model) Reset() {
	m.revealed = false
	m.sel = 0
}

// ToggleReveal flips mask/reveal. Values never reveal themselves: the gesture is
// always the reader's (secret.reveal / `r`).
func (m *Model) ToggleReveal() { m.revealed = !m.revealed }

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

// Mask is the fixed-width placeholder shown for a hidden secret value, so the
// value's length is not leaked while it is masked.
const Mask = "••••••••"

// Cursor / Gutter are the 2-cell prefix each entry line carries so the selected
// entry (Cursor) stands out from the rest (Gutter). Both are the same width so
// keys stay column-aligned as the cursor moves (M3-08b).
const (
	Cursor = "> "
	Gutter = "  "
)

// Render formats a secret's data for the viewer: a type header, then one line
// per key with a cursor gutter marking the selected entry (m.sel, M3-08b). While
// masked (m.revealed == false) each value is a fixed mask followed by its byte
// length, so the reader sees the keys and can decide what to reveal without the
// value ever leaking; revealed, the decoded value is shown verbatim (a
// multi-line value is indented under its key so the block stays readable). Keys
// arrive sorted from the kube layer. It also returns each entry's 0-based output
// line (its key line) so the caller can keep the selected entry on screen (nil
// when there are no entries).
func (m Model) Render(data kube.SecretData) (string, []int) {
	var b strings.Builder
	typ := data.Type
	if typ == "" {
		typ = "(none)"
	}
	b.WriteString("Type: " + typ + "\n")
	state := "hidden — press r to reveal, c to copy"
	if m.revealed {
		state = "revealed — press r to hide, c to copy"
	}
	b.WriteString("Data: " + state + "\n\n")
	if len(data.Entries) == 0 {
		b.WriteString("(no data)\n")
		return b.String(), nil
	}
	line := 3 // Type, Data, blank already emitted.
	entryLines := make([]int, len(data.Entries))
	for i, e := range data.Entries {
		entryLines[i] = line
		gutter := Gutter
		if i == m.sel {
			gutter = Cursor
		}
		if !m.revealed {
			fmt.Fprintf(&b, "%s%s: %s (%d bytes)\n", gutter, e.Key, Mask, len(e.Value))
			line++
			continue
		}
		if strings.Contains(e.Value, "\n") {
			// A multi-line value (a cert, a kubeconfig) reads best under its key,
			// each line indented so it is visually part of the entry.
			b.WriteString(gutter + e.Key + ":\n")
			line++
			for _, l := range strings.Split(e.Value, "\n") {
				b.WriteString(Gutter + "  " + l + "\n")
				line++
			}
			continue
		}
		b.WriteString(gutter + e.Key + ": " + e.Value + "\n")
		line++
	}
	return b.String(), entryLines
}
