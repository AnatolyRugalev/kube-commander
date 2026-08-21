package viewer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// podDescribe is an abridged `kubectl describe pod` dump of a crash-looping pod —
// the S02 on-call case the feedback was written about. It carries every line shape
// the painter distinguishes: fields the classifier knows (Status/State/Reason/
// Ready/Restart Count), fields it does not (Node/Image/Annotations), section
// headings, and the two whitespace-aligned blocks (Conditions, Events).
const podDescribe = `Name:             web-5d4c
Namespace:        shop
Node:             agent-0/172.18.0.3
Annotations:      sidecar.istio.io/inject: false
Status:           Running
Containers:
  web:
    Image:          nginx:1.25
    State:          Waiting
      Reason:       CrashLoopBackOff
    Last State:     Terminated
      Reason:       Error
    Ready:          False
    Restart Count:  7
Conditions:
  Type              Status
  Initialized       True
  Ready             False
Events:
  Type     Reason     Age   From      Message
  ----     ------     ----  ----      -------
  Normal   Pulled     10m   kubelet   Successfully pulled image
  Warning  Unhealthy  1m    kubelet   Liveness probe failed: connection refused`

// TestPaintDescribePaintsTheKnownStates is the feedback's ask (STORY-06h-2): the
// state-carrying fields are painted with the role the M4-06 classifier reads out of
// them, so the broken part of the dump is what the eye lands on. The roles are the
// browse table's own — Running is Success, CrashLoopBackOff and a Ready of False are
// Error, a restart count is Warn — because both go through table.ClassifyValue.
func TestPaintDescribePaintsTheKnownStates(t *testing.T) {
	s := styles.Default()
	got := PaintDescribe(s, podDescribe)

	for _, c := range []struct {
		value string
		want  string
	}{
		{"Running", s.Success.Render("Running")},
		{"CrashLoopBackOff", s.Error.Render("CrashLoopBackOff")},
		{"Waiting", s.Warn.Render("Waiting")},
		{"False", s.Error.Render("False")},
		{"7", s.Warn.Render("7")},
	} {
		if !strings.Contains(got, c.want) {
			t.Errorf("%q is not painted with its classifier role", c.value)
		}
	}
	// The label is muted and the heading accented, so the structure reads without
	// competing with the states.
	if !strings.Contains(got, s.Subtle.Render("Status:")) {
		t.Error("a field's key should render muted")
	}
	if !strings.Contains(got, s.Accent.Render("Conditions:")) {
		t.Error("a section heading should render accented")
	}
}

// TestPaintDescribeKeepsTheDumpVerbatim is the property that makes the panel safe:
// painting adds colour and nothing else. Strip the styling and the result is the
// describer's output byte for byte — no reflow, no re-alignment, no lost or added
// character — so kubectl's own column alignment survives and a reader who knows
// describe output still reads describe output.
func TestPaintDescribeKeepsTheDumpVerbatim(t *testing.T) {
	got := ansi.Strip(PaintDescribe(styles.Default(), podDescribe))
	if got != podDescribe {
		t.Fatalf("painting changed the text:\n--- got ---\n%s\n--- want ---\n%s", got, podDescribe)
	}
	if PaintDescribe(styles.Default(), "") != "" {
		t.Error("an empty dump should paint to nothing")
	}
}

// TestPaintDescribeLeavesUnknownFieldsPlain pins the restraint half: a key the
// classifier has no vocabulary for keeps its value as plain text. The annotation
// line is the case that made the rule — `sidecar.istio.io/inject: false` is a
// configuration flag, not a failed condition, and painting it red would be a lie
// the reader has to learn to ignore.
func TestPaintDescribeLeavesUnknownFieldsPlain(t *testing.T) {
	s := styles.Default()
	got := PaintDescribe(s, podDescribe)

	for _, plain := range []string{"false", "nginx:1.25", "agent-0/172.18.0.3", "shop"} {
		for _, st := range []string{s.Error.Render(plain), s.Warn.Render(plain), s.Success.Render(plain)} {
			if strings.Contains(got, st) {
				t.Errorf("%q sits under a key the classifier does not know and must stay plain", plain)
			}
		}
	}
}

// TestPaintDescribePaintsColumnRows covers the third line shape: the Conditions and
// Events blocks are whitespace-aligned tables, not `key: value` fields, so each
// field is classified on its own and only the warn-or-worse ones are painted — a
// Ready condition of False, an event of type Warning, the "failed" in its message.
// Success is left unpainted here on purpose: a dump is mostly fine, and colouring
// the fine parts is what buries the broken one.
func TestPaintDescribePaintsColumnRows(t *testing.T) {
	s := styles.Default()
	got := PaintDescribe(s, podDescribe)

	if !strings.Contains(got, s.Warn.Render("Warning")) {
		t.Error("an event of type Warning should be painted")
	}
	if !strings.Contains(got, s.Error.Render("failed:")) {
		t.Error("a problem word in an event message should be painted (trailing punctuation and all)")
	}
	for _, plain := range []string{"Normal", "Pulled", "kubelet"} {
		if strings.Contains(got, s.Warn.Render(plain)) || strings.Contains(got, s.Error.Render(plain)) {
			t.Errorf("%q carries no problem and must stay plain", plain)
		}
	}
	if strings.Contains(got, s.Success.Render("True")) {
		t.Error("a healthy condition must not be painted in a column row")
	}
	// The `Liveness probe failed: connection refused` message contains a colon: it
	// must not be mistaken for a field, which would mute half the row as a key.
	if strings.Contains(got, s.Subtle.Render("Warning  Unhealthy  1m    kubelet   Liveness probe failed:")) {
		t.Error("a column row whose message contains a colon must not read as a field")
	}
}

// TestRestylingRepaintsTheDescribePanel pins the SetStyles contract for painted
// content: the panel's colours *are* the palette, so a theme picked while it is open
// must repaint it — without moving the reader, since a restyle is not a reopen. The
// raw dump is kept for exactly this, and plain content (SetContent) keeps the old
// "colours only, text untouched" behaviour.
func TestRestylingRepaintsTheDescribePanel(t *testing.T) {
	m := New(styles.Default(), "describe")
	m.SetSize(40, 8)
	m.Show()
	m.SetDescribeContent(podDescribe)
	m.viewport.ScrollDown(3)
	before := m.viewport.YOffset()

	next := styles.New(styles.SolarizedLightTheme())
	m.SetStyles(next)

	if want := next.Error.Render("CrashLoopBackOff"); !strings.Contains(m.content, want) {
		t.Error("a theme switch must repaint the describe panel")
	}
	if got := m.viewport.YOffset(); got != before {
		t.Errorf("scroll moved on restyle: offset %d, want %d", got, before)
	}
	// Reopening on plain content drops the dump, so a later restyle cannot resurrect
	// a panel that is no longer shown.
	m.SetContent("plain")
	m.SetStyles(styles.Default())
	if m.content != "plain" {
		t.Errorf("plain content changed on restyle: %q", m.content)
	}
}
