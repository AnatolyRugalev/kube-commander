package table

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
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

// roleSpan is one colored run of a rendered row, in display columns of the
// *unclipped* line (the same coordinate space as columnStarts), so the horizontal
// scroll offset is applied once, at paint time.
type roleSpan struct {
	start, end int
	role       cellRole
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
		text := formatCell(cellAt(r.Cells, ci))
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
func (m Model) paintRow(line string, spans []roleSpan, innerW int) string {
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
			b.WriteString(m.styles.App.Render(string(runes[last:s])))
		}
		b.WriteString(m.roleStyle(sp.role).Render(string(runes[s:e])))
		last = e
	}
	if last < len(runes) {
		b.WriteString(m.styles.App.Render(string(runes[last:])))
	}
	if pad := innerW - len(runes); pad > 0 {
		b.WriteString(m.styles.App.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}
