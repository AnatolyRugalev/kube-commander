// Package safetext holds the one thing every kubecom surface that displays text
// kubecom did not write needs, and none of them should re-derive: how to make a
// string safe to hand to the terminal.
//
// The reason it exists is AUTH-06. A credential plugin's stderr, an API server's
// error message and a kubeconfig's own strings all end up rendered inside a
// bordered pane or a one-line bar, and none of them are kubecom's prose. Both
// layers underneath assume they are:
//
//   - the width arithmetic (lipgloss's Width/MaxWidth, ansi.Wordwrap,
//     ansi.Truncate, the status bar's rune clip) measures a string with
//     ansi.StringWidth, which counts a control character as **zero** cells — which
//     is right for what the *string* costs and wrong for what the *terminal* does
//     with it. `\r` returns the cursor to column 0 and the rest of the line
//     repaints over the pane's left border; `\b` walks it backwards so the frame's
//     right border lands short; `\x1b[2J` erases the screen the frame was drawn on.
//     Nothing downstream can see any of this, because every one of those bytes is
//     zero cells wide by the only measure the layout has.
//   - the styling assumes it owns the color. A plugin that prints an SGR sequence
//     paints its own foreground inside the pane's Subtle style and, if the sequence
//     is cut by a wrap or a truncate before its reset, leaves it painted.
//
// So the rule this package exists to make cheap is: **text kubecom did not write is
// content, never control.** Strip what steers the terminal, keep what says
// something, and let the caller's existing wrap/clip/elide do the rest — those are
// correct once every remaining rune costs what it measures.
//
// It is deliberately not a renderer and not an escaper: what comes back is a plain
// string that still means what the plugin said, only with the parts that move a
// cursor or repaint a cell removed.
package safetext

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Line returns s with everything that steers the terminal removed: ANSI escape
// sequences stripped (CSI, OSC, and their 8-bit C1 forms — ansi.Strip), each tab
// replaced by a single space, and every other control character dropped, DEL and
// the lone C1 bytes included.
//
// The result is one line: a newline in s is a control character like any other and
// is dropped rather than splitting the string, because a caller reaching for Line
// has one row to render and a second line would land wherever the surrounding
// layout happens to put it. Use Block when the line structure is part of the
// message.
//
// Tabs become one space rather than being dropped, or expanded to a tab stop,
// because a plugin that column-aligns its stderr with tabs is separating fields:
// dropping the tab runs two fields together, and expanding to the next multiple of
// eight is alignment this pane cannot honour anyway (its left edge is not the
// terminal's, and the text is about to be word-wrapped). One space keeps the words
// apart, which is the part that carries meaning.
func Line(s string) string {
	return dropControls(ansi.Strip(s))
}

// Block is Line applied to each line of s, preserving the newlines between them.
// It is what a multi-line surface wants — the browse pane's notice, a modal's
// message — where the line structure is kubecom's (or is the plugin's own, and
// carries a fact per line) and only what is *inside* each line is untrusted.
//
// A `\r\n` line ending loses its `\r` here rather than leaving one at the end of
// every line: the split is on `\n`, and the carriage return is then a control
// character inside the line Line is cleaning.
func Block(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = Line(line)
	}
	return strings.Join(lines, "\n")
}

// dropControls removes the control characters ansi.Strip leaves behind. Strip
// understands *sequences* — an escape and the bytes that belong to it — so what
// survives it is the C0/C1 bytes that steer the terminal on their own: `\r`, `\b`,
// `\a`, `\n`, NUL, DEL.
//
// unicode.IsControl is the whole test: it is true for C0 (0x00–0x1f), DEL (0x7f)
// and C1 (0x80–0x9f) and false for everything else, which is exactly the set that
// costs zero cells and does something. A rune that merely renders oddly (a
// zero-width joiner, a bidi override) is left alone — it is text, the width
// arithmetic already agrees with the terminal about it, and deciding which
// scripts a Kubernetes error may be written in is not this package's business.
func dropControls(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
