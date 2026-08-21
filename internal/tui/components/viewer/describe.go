package viewer

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/tui/components/table"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// PaintDescribe re-renders a `kubectl describe` dump as a styled panel
// (STORY-06h-2): the failing part is what the eye should land on first, instead of
// a wall of even-weight text the reader has to scan by hand.
//
// It is a pure text→text function — the viewer stays a plain pager and the shell
// stays a router; painting is done once, on the dump the describer returned, and
// the result is ordinary viewport content. Every line keeps its exact characters
// and spacing (kubectl aligns describe values in a column, and the Events/Conditions
// blocks are whitespace-aligned tables), so the panel is the same dump with colour
// added, never a reflow: a reader who knows describe output still reads describe
// output, and nothing can be lost in a re-layout.
//
// Three line shapes, in order:
//
//   - `Key:` alone — a section heading (Containers:, Conditions:, Events:) — is
//     accented, so the dump's structure is visible while scrolling.
//   - `Key:  value` — the key is muted (it is the label, not the news) and the value
//     is painted with the role the **M4-06 cell classifier** reads out of it
//     (table.ClassifyValue) under the classifier column that key speaks for
//     (describeKeyColumns). Going through that one classifier is the point: the
//     panel, the browse table's colouring and the `H`/`U` unhealthy predicates
//     cannot disagree about what is broken (D285). A key it knows nothing about —
//     Image, Node, Annotations — leaves its value as plain text.
//   - anything else — the whitespace-aligned rows of the Conditions and Events
//     blocks — has each field classified on its own, and only the fields that come
//     back **warn or worse** are painted. Success is deliberately not painted here:
//     a describe dump is mostly fine, and colouring the fine parts is what makes a
//     broken one hard to find.
func PaintDescribe(s styles.Styles, text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = paintDescribeLine(s, line)
	}
	return strings.Join(lines, "\n")
}

// describeKeyColumns maps a describe key (upper-cased, spaces collapsed) to the
// classifier column whose vocabulary reads its values. The map is small on
// purpose: it names the keys that carry a *state*, and everything else is text the
// panel must not editorialise about. "Reason" and "Last State" speak the STATUS
// vocabulary (CrashLoopBackOff, ImagePullBackOff, Error, Completed); "Ready" is
// the condition boolean readyRole already falls back to; "Restart Count" is the
// RESTARTS column under another name.
var describeKeyColumns = map[string]string{
	"STATUS":        "STATUS",
	"STATE":         "STATE",
	"LAST STATE":    "STATE",
	"PHASE":         "PHASE",
	"REASON":        "STATUS",
	"READY":         "READY",
	"RESTARTS":      "RESTARTS",
	"RESTART COUNT": "RESTARTS",
}

// maxDescribeKey bounds how long a `key:` prefix may be before the line is read as
// a column row instead. Together with the double-space rejection in describeField
// it keeps a message that happens to contain a colon ("Liveness probe failed: HTTP
// probe failed") from being mistaken for a field.
const maxDescribeKey = 40

// paintDescribeLine paints one line of the dump; see PaintDescribe for the shapes.
func paintDescribeLine(s styles.Styles, line string) string {
	indent, rest := splitIndent(line)
	if rest == "" {
		return line
	}
	key, sep, value, ok := describeField(rest)
	if !ok {
		return indent + paintFields(s, rest)
	}
	if value == "" {
		return indent + s.Accent.Render(key+":") + sep
	}
	painted := value
	if col, known := describeKeyColumns[normalizeKey(key)]; known {
		if st, ok := roleStyle(s, table.ClassifyValue(col, value)); ok {
			painted = st.Render(value)
		}
	}
	return indent + s.Subtle.Render(key+":") + sep + painted
}

// describeField splits an indent-stripped line into `key`, the whitespace that
// separates it from its value, and the value (empty for a section heading). It
// reports false for a line that is not a field: no colon, an empty key, a key
// longer than maxDescribeKey, or a key holding a double space — kubectl separates
// aligned columns by two or more spaces, so a double space inside the candidate key
// means the line is a Conditions/Events row whose message merely contains a colon.
func describeField(rest string) (key, sep, value string, ok bool) {
	i := strings.Index(rest, ":")
	if i <= 0 {
		return "", "", "", false
	}
	key = rest[:i]
	if strings.Contains(key, "  ") || len([]rune(key)) > maxDescribeKey {
		return "", "", "", false
	}
	after := rest[i+1:]
	trimmed := strings.TrimLeft(after, " \t")
	return key, after[:len(after)-len(trimmed)], trimmed, true
}

// paintFields paints the classifier-flagged fields of a column row, leaving the
// row's spacing byte-identical. Only warn-or-worse fields are painted (see
// PaintDescribe); trailing punctuation is trimmed before classifying so a word
// ending a sentence ("…probe failed:") still reads as the word it is.
func paintFields(s styles.Styles, rest string) string {
	var b strings.Builder
	for _, f := range splitFields(rest) {
		if f.space {
			b.WriteString(f.text)
			continue
		}
		role := table.ClassifyValue("STATUS", strings.TrimRight(f.text, ".,:;)"))
		if st, ok := roleStyle(s, role); ok && role >= table.RoleWarn {
			b.WriteString(st.Render(f.text))
			continue
		}
		b.WriteString(f.text)
	}
	return b.String()
}

// field is one run of a line: either whitespace or a whitespace-free token. Runs
// are kept rather than words-plus-gaps so a painted line re-joins to exactly the
// original characters.
type field struct {
	text  string
	space bool
}

// splitFields cuts s into alternating whitespace and token runs.
func splitFields(s string) []field {
	var out []field
	start, cur := 0, false
	for i, r := range s {
		isSpace := unicode.IsSpace(r)
		if i == 0 {
			cur = isSpace
			continue
		}
		if isSpace != cur {
			out = append(out, field{text: s[start:i], space: cur})
			start, cur = i, isSpace
		}
	}
	if s != "" {
		out = append(out, field{text: s[start:], space: cur})
	}
	return out
}

// splitIndent separates a line's leading whitespace from the rest, so painting can
// re-emit the indent untouched (describe's nesting is its structure).
func splitIndent(line string) (indent, rest string) {
	rest = strings.TrimLeft(line, " \t")
	return line[:len(line)-len(rest)], rest
}

// normalizeKey folds a describe key to the form describeKeyColumns is keyed by:
// upper-cased with runs of whitespace collapsed to one space.
func normalizeKey(key string) string {
	return strings.ToUpper(strings.Join(strings.Fields(key), " "))
}

// roleStyle maps a classifier role to the theme style that paints it, reporting
// false for RoleNone (render as ordinary text — a style is never applied for
// "nothing to say", so plain text stays exactly plain).
func roleStyle(s styles.Styles, r table.Role) (lipgloss.Style, bool) {
	switch r {
	case table.RoleSuccess:
		return s.Success, true
	case table.RoleWarn:
		return s.Warn, true
	case table.RoleError:
		return s.Error, true
	default:
		return s.App, false
	}
}
