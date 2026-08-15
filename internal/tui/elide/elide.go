// Package elide holds the one thing every bordered overlay in kubecom needs and
// none of them should re-derive: how to cut a rendered block down to the rows the
// box actually has, and how to say that it did.
//
// The reason it exists is D220 pt 1. Overlays are composited by overlayCenter onto
// a fixed width×bodyHeight canvas that clips bottom-first, silently — so a box that
// renders taller than its budget does not scroll or shrink, it loses its bottom rows
// and its border with nothing on screen saying so, and no test that reads the
// component's own View() string can see it. Each overlay therefore bounds itself,
// and the ones without a cursor (D221 covers the ones with) do it by truncating and
// marking. That shape was written twice before it was worth naming: the confirm/
// prompt modal (BOX-01) and the keybindings overlay (BOX-03).
//
// The marker wording is the convention, not decoration: browsefail.go says
// "… (truncated)" when it drops the tail of a credential plugin's stderr, and a
// reader who meets both should meet one phrase. Lines takes the marker as an
// argument rather than always using Marker because a surface whose elided content
// is reachable somewhere else should say where (the help overlay names
// docs/keybindings.md) — what is fixed is that a cut is announced, not the sentence.
package elide

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Marker is the default elision marker: the wording browsefail.go uses when it
// drops the tail of captured stderr. It is a sentence rather than a bare ellipsis
// because an ellipsis at the end of a line reads as prose — and where the elided
// block is a question the reader is about to answer (the modal), text quietly
// missing from what is being agreed to is exactly the thing worth naming.
const Marker = "… (truncated)"

// Lines truncates a rendered block to at most n lines, spending the last one on
// marker so the cut is visible. n <= 0 yields ""; a block that already fits is
// returned untouched, so the marker is evidence that something was dropped rather
// than furniture every box carries.
//
// The marker replaces the last surviving line instead of being appended to it: an
// appended row would make the block n+1 tall, which is the bug this package exists
// to prevent. Callers pass a marker already styled and width-capped for their box;
// Lines does no rendering of its own, so it is safe on blocks that carry ANSI
// styling (it splits on newlines only).
func Lines(block string, n int, marker string) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	if len(lines) <= n {
		return block
	}
	return strings.Join(append(lines[:n-1:n-1], marker), "\n")
}

// Width clamps every line of a rendered block to at most n columns, spending the
// last line on marker so the cut is visible — the horizontal twin of Lines, for
// the same reason (D220 pt 1). It exists because the source of the block may not
// honour the width it was given: bubbles/help's full layout adds a column whole
// when its ellipsis would not fit, so cumulative column widths landing exactly on
// the limit dump *every* remaining column instead of the few that fit (found when
// the letter remap narrowed a binding and pushed the keymap's columns over the
// edge). Like Lines it returns a block that already fits untouched, keeps ANSI
// styling intact, and expects a caller-styled marker — the cut and the marker are
// one decision, never two.
func Width(block string, n int, marker string) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	cut := false
	for i, ln := range lines {
		if w := ansi.StringWidth(ln); w > n {
			lines[i] = ansi.Truncate(ln, n, "")
			cut = true
		}
	}
	if !cut {
		return block
	}
	return strings.Join(append(lines, marker), "\n")
}
