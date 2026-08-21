package table

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// cellRole is the semantic meaning a table cell's value carries — the bridge
// between a server-printed string and the theme's Success/Warn/Error roles.
//
// The constants are ordered by severity on purpose: a value made of several
// parts (a Node's "Ready,SchedulingDisabled") is classified part by part and the
// highest role wins, so a plain `>` comparison is the whole merge rule.
type cellRole uint8

const (
	roleNone cellRole = iota // nothing to say; render as ordinary text
	roleSuccess
	roleWarn
	roleError
)

// Column names the classifier understands, upper-cased. Coloring keys off the
// **column name**, not the resource kind: the columns come from the server-side
// Table API and are kubectl-identical (D33), so one rule set covers Pods, Nodes
// and any CRD whose printer happens to use the same names — and a kind kubecom
// has never heard of gets the same treatment for free.
const (
	colStatus   = "STATUS"
	colState    = "STATE"
	colPhase    = "PHASE"
	colReady    = "READY"
	colRestarts = "RESTARTS"
)

// Placeholders kubectl prints for "the server told us nothing". They are absence,
// not a state, so they are never colored.
var cellPlaceholders = map[string]bool{
	"":          true,
	"<none>":    true,
	"<unknown>": true,
	"<invalid>": true,
}

// statusWords maps the status values worth naming explicitly to a role, keyed
// lower-cased. It is deliberately not exhaustive: the container waiting and
// terminated reasons form an open set (any kubelet release can add one, and a CRD
// can print whatever it likes), so the long tail is left to the shape heuristics
// in statusWordRole. What is listed here is what a reader would look for.
var statusWords = map[string]cellRole{
	// Healthy, settled states.
	"running":     roleSuccess,
	"succeeded":   roleSuccess,
	"completed":   roleSuccess,
	"complete":    roleSuccess,
	"ready":       roleSuccess,
	"active":      roleSuccess,
	"bound":       roleSuccess,
	"available":   roleSuccess,
	"healthy":     roleSuccess,
	"established": roleSuccess,
	"approved":    roleSuccess,
	"true":        roleSuccess,

	// In flight, or deliberately held — normal, but not somewhere to leave things.
	"pending":                roleWarn,
	"containercreating":      roleWarn,
	"podinitializing":        roleWarn,
	"terminating":            roleWarn,
	"schedulingdisabled":     roleWarn,
	"notready":               roleWarn,
	"progressing":            roleWarn,
	"unschedulable":          roleWarn,
	"waiting":                roleWarn,
	"released":               roleWarn,
	"suspended":              roleWarn,
	"degraded":               roleWarn,
	"containerstatusunknown": roleWarn,
	// An event of type Warning, and any CRD printer that names the state
	// outright: the word is a warning by definition.
	"warning": roleWarn,
	// "Unknown" is the apiserver saying it has lost touch, not a confirmed
	// failure — a warning, so a genuinely failed object still stands out from it.
	"unknown": roleWarn,

	// Broken.
	"crashloopbackoff": roleError,
	"imagepullbackoff": roleError,
	"errimagepull":     roleError,
	"error":            roleError,
	"failed":           roleError,
	"failure":          roleError,
	"evicted":          roleError,
	"oomkilled":        roleError,
	"terminated":       roleError,
	"nodelost":         roleError,
	"lost":             roleError,
	"unhealthy":        roleError,
	"unreachable":      roleError,
	"deadlineexceeded": roleError,
	"rejected":         roleError,
	"false":            roleError,
}

// classifyCell is the classifier: the role a cell's value carries in the column
// it sits under, or roleNone for a column this knows nothing about (the common
// case — NAME, AGE, IMAGE and every CRD column are left alone). Pure, so it is
// tested directly rather than through a rendered frame.
func classifyCell(column, value string) cellRole {
	v := strings.TrimSpace(value)
	if cellPlaceholders[strings.ToLower(v)] {
		return roleNone
	}
	switch strings.ToUpper(strings.TrimSpace(column)) {
	case colStatus, colState, colPhase:
		return statusRole(v)
	case colReady:
		return readyRole(v)
	case colRestarts:
		return restartsRole(v)
	}
	return roleNone
}

// statusRole classifies a STATUS/STATE/PHASE value. A Node prints its conditions
// comma-joined ("Ready,SchedulingDisabled"), so the value is split and the most
// severe part wins — a cordoned but healthy node reads as a warning, not as
// success.
func statusRole(v string) cellRole {
	worst := roleNone
	for _, part := range strings.Split(v, ",") {
		if r := statusWordRole(strings.TrimSpace(part)); r > worst {
			worst = r
		}
	}
	return worst
}

// statusWordRole classifies one status word: the named values first, then the
// shape of the name for everything else. The heuristics exist because the
// reason set is open-ended — kubelet's container reasons are conventionally
// named (`…BackOff`, `…Error`, `Err…`, `Failed…`), and honouring the convention
// colors a reason nobody has enumerated correctly, while an unrecognised word
// simply stays uncolored.
func statusWordRole(w string) cellRole {
	if w == "" {
		return roleNone
	}
	// "Init:<reason>" is a pod still running its init containers. The part after
	// the colon carries the signal — "Init:CrashLoopBackOff" is broken, while
	// "Init:0/2" is ordinary progress — so it is classified on its own and the
	// bare-progress case falls back to a warning.
	if rest, ok := strings.CutPrefix(w, "Init:"); ok {
		if r := statusWordRole(rest); r != roleNone {
			return r
		}
		return roleWarn
	}
	lw := strings.ToLower(w)
	if r, ok := statusWords[lw]; ok {
		return r
	}
	switch {
	case strings.HasSuffix(lw, "backoff"),
		strings.HasSuffix(lw, "error"),
		strings.HasSuffix(lw, "failed"),
		strings.HasPrefix(lw, "err"):
		return roleError
	}
	return roleNone
}

// readyRole classifies a READY value. Two shapes reach it: kubectl's
// ready/desired fraction ("1/1", "0/3" — Pods, Deployments, StatefulSets,
// DaemonSets) and the plain condition boolean a CRD printer column tends to use
// ("True"/"False"), which is just a status word.
//
// A shortfall is a **warning**, never an error: a rollout in progress and a pod
// whose containers are still starting are both normal, and the object's STATUS
// cell is what says whether anything is actually wrong.
func readyRole(v string) cellRole {
	ready, desired, ok := readyFraction(v)
	if !ok {
		return statusWordRole(v)
	}
	switch {
	case desired == 0:
		// 0/0 — nothing is expected, so nothing is missing (a scaled-to-zero
		// Deployment is a choice, not a fault).
		return roleNone
	case ready >= desired:
		return roleSuccess
	default:
		return roleWarn
	}
}

// readyFraction parses a "ready/desired" cell into its two counts, reporting
// false for anything that is not exactly two non-negative integers around a
// single slash.
func readyFraction(v string) (ready, desired int, ok bool) {
	before, after, found := strings.Cut(v, "/")
	if !found || strings.Contains(after, "/") {
		return 0, 0, false
	}
	r, err := strconv.Atoi(strings.TrimSpace(before))
	if err != nil || r < 0 {
		return 0, 0, false
	}
	d, err := strconv.Atoi(strings.TrimSpace(after))
	if err != nil || d < 0 {
		return 0, 0, false
	}
	return r, d, true
}

// restartsRole classifies a RESTARTS value. kubectl prints a bare count, or the
// count followed by how long ago the last restart was ("3 (5m ago)"), so only the
// leading integer is read.
//
// Any restart at all is a warning and there is deliberately no "many restarts is
// an error" tier: the threshold would be an arbitrary constant with no evidence
// behind it, and a container that is still restarting reports it in STATUS, which
// is colored on its own.
func restartsRole(v string) cellRole {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return roleNone
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 {
		return roleNone
	}
	return roleWarn
}

// rowUnhealthy reports whether a row reads as unhealthy under the M4-06
// classifier: any visible cell classifies to a warning or an error role (STORY-06g).
// It is the predicate of the unhealthy-only view — the "what's broken, filtered"
// a quick-access key offers — and it deliberately mirrors the coloring: the rows
// the cell classifier paints as not-green are exactly the rows this narrows to.
// A row whose every cell classifies to roleNone or roleSuccess is healthy.
func (m Model) rowUnhealthy(r kube.Row) bool {
	for _, ci := range m.visible {
		if role := classifyCell(m.table.Columns[ci].Name, FormatCell(cellAt(r.Cells, ci))); role >= roleWarn {
			return true
		}
	}
	return false
}

// Role is the M4-06 cell classifier's verdict, exported for the surfaces that are
// not tables. A describe dump is text, not rows (STORY-06h-2), so the panel that
// paints it cannot go through UnhealthyCells — but it must not grow a second
// vocabulary either, or the panel and the lists would disagree about what is
// broken. It reads the same statusWords/heuristics the browse table's colouring
// and the `H`/`U` unhealthy predicates read; only the entry point differs.
//
// The values mirror cellRole's severity order, so `>= RoleWarn` is "the reader
// should look at this".
type Role uint8

const (
	RoleNone    Role = Role(roleNone)    // nothing to say; ordinary text
	RoleSuccess Role = Role(roleSuccess) // healthy, settled
	RoleWarn    Role = Role(roleWarn)    // in flight, held, or degraded
	RoleError   Role = Role(roleError)   // broken
)

// ClassifyValue reports how the classifier reads value when it sits under a column
// (or, for a describe panel, a key) of the given name. Column names it knows
// nothing about — the common case — yield RoleNone, so a caller can hand it every
// key it meets and paint only what comes back coloured.
func ClassifyValue(column, value string) Role { return Role(classifyCell(column, value)) }

// UnhealthyRow is the M4-06 classifier lifted to a whole table: it reports
// whether any of a row's cells classifies to a warning or error role under its
// column, considering every column the server printed (the scan's cross-kind
// sweep has no "visible" subset — a hit's row is the whole row). It is the
// predicate kube.Scan takes as a seam (D276) so the cross-kind unhealthy list
// and the per-kind unhealthy filter agree about what is broken (D275 pt 1): the
// same classifyCell both rely on, exported here so the TUI can feed it to a
// sweep without importing this component into the scan's fan-out.
func UnhealthyRow(t *kube.Table, r kube.Row) bool {
	return len(UnhealthyCells(t, r)) > 0
}

// UnhealthyCell is one cell of a row that the M4-06 classifier reads as a
// warning or error: the display text and whether it classifies to an error
// rather than a warning, so a surface can paint the reason with the same hue the
// browse table gives the cell.
type UnhealthyCell struct {
	Text  string
	Error bool // roleError, not roleWarn
}

// UnhealthyCells returns the row's cells that classify to a warning or error
// role, in column order, each tagged with its role — the "reason" a cross-kind
// scan hit renders (kind · name · namespace · the offending cell, D276). A hit
// carries the row *and* the columns it sat under (ScanHit.Columns), so a surface
// can name the offender without re-listing the kind: which cell is the reason
// needs the column names, and the columns are the one thing the hit now carries
// that the row alone does not. The scan itself runs this to filter rows, so the
// reason shown is exactly the reason the row was kept.
func UnhealthyCells(t *kube.Table, r kube.Row) []UnhealthyCell {
	var out []UnhealthyCell
	for i, c := range t.Columns {
		if i >= len(r.Cells) {
			continue
		}
		text := FormatCell(cellAt(r.Cells, i))
		if role := classifyCell(c.Name, text); role >= roleWarn {
			out = append(out, UnhealthyCell{Text: text, Error: role == roleError})
		}
	}
	return out
}

// roleStyle maps a role to the theme style that paints it. roleNone renders as
// ordinary body text, so every segment of a row goes through a complete style and
// none is ever nested inside another (see paintRow).
func (m Model) roleStyle(r cellRole) lipgloss.Style {
	switch r {
	case roleSuccess:
		return m.styles.Success
	case roleWarn:
		return m.styles.Warn
	case roleError:
		return m.styles.Error
	default:
		return m.styles.App
	}
}

// roleSpan is one styled run of a rendered row, in display columns of the
// *unclipped* line (the same coordinate space as columnStarts), so the horizontal
// scroll offset is applied once, at paint time.
//
// match marks a run produced by the active `/` filter rather than by the cell
// classifier (FILT-02). cursor marks the column the column-header sort mode's
// cursor is on (STORY-06b) — the sort picker's "you are here". Both are flags
// rather than cellRoles because the cellRole constants are ordered by severity
// and merged with `>`: a match or a cursor is not a severity and must not enter
// that comparison.
type roleSpan struct {
	start, end int
	role       cellRole
	match      bool
	cursor     bool
}

// spanStyle is the style one span paints with. A match wins over the cell's
// status role: the filter is why the row is on screen at all, so the reader must
// be able to see what matched even in a cell that also carries a color. Match
// paints a background of its own (styles.Match), so the two are never ambiguous.
// The sort cursor paints with the Selection bar, the same "you are here" the
// selected row uses.
func (m Model) spanStyle(sp roleSpan) lipgloss.Style {
	if sp.match {
		return m.styles.Match
	}
	if sp.cursor {
		return m.styles.Selection
	}
	return m.roleStyle(sp.role)
}

// roleSpans is the set of colored runs for one row: each visible cell whose value
// classifies to something, spanning the value's own width rather than the padded
// column width — the padding belongs to no cell, and a future theme that gives a
// role a background must not paint it.
//
// starts is passed in (rather than recomputed) because View renders many rows
// against one column layout.
func (m Model) roleSpans(r kube.Row, starts []int) []roleSpan {
	var spans []roleSpan
	for i, ci := range m.visible {
		if i >= len(starts) {
			break
		}
		text := FormatCell(cellAt(r.Cells, ci))
		role := classifyCell(m.table.Columns[ci].Name, text)
		if role == roleNone {
			continue
		}
		spans = append(spans, roleSpan{
			start: starts[i],
			end:   starts[i] + runeLen(text),
			role:  role,
		})
	}
	return spans
}

// matchSpans is the set of runs one row's cells matched the active `/` filter on,
// in the same unclipped coordinate space as roleSpans. Empty with no filter set,
// which is why an unfiltered table renders through exactly the path it always did.
//
// The scope is m.visible — the same columns rowMatches narrows on — so every
// occurrence the filter counted as a reason to keep the row is highlighted, and a
// hit in a column the reader cannot see is never claimed. Spans come out sorted
// and disjoint: the columns are walked in display order and each cell's own
// occurrences are non-overlapping.
func (m Model) matchSpans(r kube.Row, starts []int) []roleSpan {
	if m.filter == "" {
		return nil
	}
	needle := []rune(strings.ToLower(m.filter))
	if len(needle) == 0 {
		return nil
	}
	var spans []roleSpan
	for i, ci := range m.visible {
		if i >= len(starts) {
			break
		}
		text := FormatCell(cellAt(r.Cells, ci))
		for _, off := range matchOffsets(text, needle) {
			spans = append(spans, roleSpan{
				start: starts[i] + off,
				end:   starts[i] + off + len(needle),
				match: true,
			})
		}
	}
	return spans
}

// matchOffsets returns the rune offset of every case-insensitive occurrence of the
// (already lowercased) needle in text. Offsets are in runes because the whole span
// coordinate space is display columns, which is what columnStarts and padRight
// measure in.
//
// Searching the lowercased copy and indexing the original needs the two to stay in
// step, and in **runes** they always do: strings.ToLower maps rune to rune
// (unicode.ToLower), so the fold can change a rune's width in bytes (İ → i) but
// never the count. That is why there is no length guard here and why logsview's
// spanSubstring — which works in byte offsets, where the fold does change the
// length — needs one.
func matchOffsets(text string, needle []rune) []int {
	lower := []rune(strings.ToLower(text))
	if len(needle) > len(lower) {
		return nil
	}
	var out []int
	for i := 0; i+len(needle) <= len(lower); {
		if string(lower[i:i+len(needle)]) == string(needle) {
			out = append(out, i)
			i += len(needle)
			continue
		}
		i++
	}
	return out
}

// mergeSpans overlays the match runs on the role runs, returning one sorted,
// disjoint span list for paintRow. Where a match lands inside a colored cell the
// role run is cut around it, so the match is painted whole and the rest of the
// cell keeps its color — rather than the two fighting over the same columns and
// whichever paintRow reached first winning by accident.
//
// Both inputs are sorted and internally disjoint (roleSpans and matchSpans walk
// m.visible in order), which is what makes the cut a single pass.
func mergeSpans(role, match []roleSpan) []roleSpan {
	if len(match) == 0 {
		return role
	}
	out := make([]roleSpan, 0, len(role)+len(match))
	for _, rs := range role {
		out = append(out, cutSpan(rs, match)...)
	}
	out = append(out, match...)
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// cutSpan returns the parts of rs left uncovered by cuts (sorted, disjoint),
// preserving rs's role. A span wholly covered yields nothing.
func cutSpan(rs roleSpan, cuts []roleSpan) []roleSpan {
	var out []roleSpan
	at := rs.start
	for _, c := range cuts {
		if c.end <= at {
			continue
		}
		if c.start >= rs.end {
			break
		}
		if c.start > at {
			out = append(out, roleSpan{start: at, end: c.start, role: rs.role})
		}
		at = c.end
		if at >= rs.end {
			return out
		}
	}
	if at < rs.end {
		out = append(out, roleSpan{start: at, end: rs.end, role: rs.role})
	}
	return out
}

// paintRow renders an already-clipped row line with its colored spans, padded out
// to innerW. line is the horizontal window starting at m.hoffset, so the spans —
// which are in unclipped coordinates — are shifted by it and clamped to the
// window.
//
// Each segment is rendered through its own complete style and the results are
// concatenated; nothing is wrapped in an enclosing style. Rendering a
// span-containing line inside an outer style would leave the text after the first
// span's reset unstyled, which is the whole reason this does the padding itself
// instead of leaning on lipgloss's Width.
//
// base is the style every unspanned segment — and the trailing pad — is rendered
// through. It is the body style for a normal row and the Selection style for the
// cursor row, which is what lets a match stay visible on the row the reader is
// standing on without the selection bar being broken up by anything else.
func (m Model) paintRow(line string, spans []roleSpan, innerW int, base lipgloss.Style) string {
	runes := []rune(line)
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		s, e := sp.start-m.hoffset, sp.end-m.hoffset
		if s < last {
			s = last
		}
		if e > len(runes) {
			e = len(runes)
		}
		if s >= e {
			continue
		}
		if s > last {
			b.WriteString(base.Render(string(runes[last:s])))
		}
		b.WriteString(m.spanStyle(sp).Render(string(runes[s:e])))
		last = e
	}
	if last < len(runes) {
		b.WriteString(base.Render(string(runes[last:])))
	}
	if pad := innerW - len(runes); pad > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}
