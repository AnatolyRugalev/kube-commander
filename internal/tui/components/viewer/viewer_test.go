package viewer

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newViewer() Model {
	m := New(styles.Default(), "yaml")
	m.SetSize(40, 12)
	return m
}

// numberedLines builds n lines "line-1".."line-n" so scroll assertions can name a
// line by its expected position.
func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteByte('\n')
		}
		b.WriteString("line-")
		b.WriteString(itoa(i))
	}
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	p := len(buf)
	for i > 0 {
		p--
		buf[p] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[p:])
}

func TestHiddenOrUnsizedViewIsEmpty(t *testing.T) {
	m := New(styles.Default(), "yaml")
	m.SetContent("hello")
	// Hidden: no View even when sized.
	m.SetSize(40, 12)
	if v := m.View(); v != "" {
		t.Errorf("hidden viewer View() = %q; want empty", v)
	}
	// Shown but unsized: still empty.
	m2 := New(styles.Default(), "yaml")
	m2.SetContent("hello")
	m2.Show()
	if v := m2.View(); v != "" {
		t.Errorf("unsized viewer View() = %q; want empty", v)
	}
}

func TestViewShowsTitleAndContent(t *testing.T) {
	m := newViewer()
	m.SetTitle("pod/nginx.yaml")
	m.SetContent(numberedLines(5))
	m.Show()
	v := m.View()
	if !strings.Contains(v, "pod/nginx.yaml") {
		t.Errorf("View() missing title; got:\n%s", v)
	}
	if !strings.Contains(v, "line-1") {
		t.Errorf("View() missing first content line; got:\n%s", v)
	}
}

func TestScrollDownRevealsLaterLines(t *testing.T) {
	m := newViewer()
	// Content taller than the ~7-row inner viewport so there is somewhere to scroll.
	m.SetContent(numberedLines(50))
	m.Show()

	top := m.View()
	if !strings.Contains(top, "line-1") {
		t.Fatalf("initial view should start at line-1; got:\n%s", top)
	}

	// Jump to the bottom: the last line must be visible, the first gone.
	m, cmd := m.Update(keymap.ActionBottom)
	if cmd != nil {
		t.Errorf("nav.bottom should not emit a cmd, got non-nil")
	}
	bot := m.View()
	if !strings.Contains(bot, "line-50") {
		t.Errorf("after nav.bottom, view should show line-50; got:\n%s", bot)
	}
	if strings.Contains(bot, "line-1\n") || strings.HasSuffix(bot, "line-1") {
		t.Errorf("after nav.bottom, line-1 should be scrolled off; got:\n%s", bot)
	}
	if !m.AtBottom() {
		t.Errorf("AtBottom() = false after nav.bottom; want true")
	}

	// Back to the top.
	m, _ = m.Update(keymap.ActionTop)
	if !strings.Contains(m.View(), "line-1") {
		t.Errorf("after nav.top, view should show line-1 again; got:\n%s", m.View())
	}
	if m.AtBottom() {
		t.Errorf("AtBottom() = true after nav.top on tall content; want false")
	}
}

func TestSetContentResetsScroll(t *testing.T) {
	m := newViewer()
	m.SetContent(numberedLines(50))
	m.Show()
	m, _ = m.Update(keymap.ActionBottom)
	if !m.AtBottom() {
		t.Fatalf("precondition: expected AtBottom after nav.bottom")
	}
	// New content must start back at the top.
	m.SetContent(numberedLines(50))
	if !strings.Contains(m.View(), "line-1") {
		t.Errorf("SetContent did not reset scroll to top; got:\n%s", m.View())
	}
}

func TestBackEmitsClosedMsg(t *testing.T) {
	m := newViewer()
	m.SetContent("x")
	m.Show()
	_, cmd := m.Update(keymap.ActionBack)
	if cmd == nil {
		t.Fatal("nav.back should emit a ClosedMsg cmd, got nil")
	}
	msg := cmd()
	closed, ok := msg.(ClosedMsg)
	if !ok {
		t.Fatalf("nav.back msg = %T; want ClosedMsg", msg)
	}
	if closed.Kind != "yaml" {
		t.Errorf("ClosedMsg.Kind = %q; want %q", closed.Kind, "yaml")
	}
}

func TestInactiveViewerIgnoresActions(t *testing.T) {
	m := newViewer()
	m.SetContent(numberedLines(50))
	// Not shown.
	var cmd tea.Cmd
	for _, a := range []keymap.Action{keymap.ActionBottom, keymap.ActionBack, keymap.ActionDown} {
		m, cmd = m.Update(a)
		if cmd != nil {
			t.Errorf("inactive viewer emitted a cmd for %v; want nil", a)
		}
	}
	if m.View() != "" {
		t.Errorf("inactive viewer View() = %q; want empty", m.View())
	}
}

func TestTitleClipsToBox(t *testing.T) {
	m := New(styles.Default(), "yaml")
	m.SetSize(20, 10) // small box → narrow title area
	long := strings.Repeat("verylongtitle-", 10)
	m.SetTitle(long)
	m.SetContent("body")
	m.Show()
	v := m.View()
	// Every rendered line must fit within the box width (no overflow past the frame).
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("rendered line width %d exceeds box width 20: %q", w, line)
		}
	}
}
