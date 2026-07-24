package logsview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newLogs() Model {
	m := New(styles.Default())
	m.SetSize(40, 12)
	m.Show()
	return m
}

// appendLines streams "line-1".."line-n" into m.
func appendLines(m *Model, n int) {
	for i := 1; i <= n; i++ {
		m.Append("line-" + itoa(i))
	}
}

// typeFilter opens the filter and feeds each rune of q, returning the updated model.
func typeFilter(m Model, q string) Model {
	m, _ = m.Update(keymap.ActionFilter)
	for _, r := range q {
		m, _ = m.UpdateFilter(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return m
}

func TestHiddenOrUnsizedIsEmpty(t *testing.T) {
	m := New(styles.Default())
	m.Append("hello")
	m.SetSize(40, 12) // sized but hidden
	if v := m.View(); v != "" {
		t.Errorf("hidden View() = %q; want empty", v)
	}
	m2 := New(styles.Default())
	m2.Append("hello")
	m2.Show() // shown but unsized
	if v := m2.View(); v != "" {
		t.Errorf("unsized View() = %q; want empty", v)
	}
}

func TestStartsFollowingAndTails(t *testing.T) {
	m := newLogs()
	if !m.Following() {
		t.Fatal("a fresh logs view should start following")
	}
	appendLines(&m, 50)
	v := m.View()
	if !strings.Contains(v, "[following]") {
		t.Errorf("header should show [following]; got:\n%s", v)
	}
	if !strings.Contains(v, "line-50") {
		t.Errorf("following view should tail to line-50; got:\n%s", v)
	}
}

func TestUpwardScrollPausesAndDoesNotYank(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	// Scroll to the top: this pauses following.
	m, _ = m.Update(keymap.ActionTop)
	if m.Following() {
		t.Fatal("scrolling up should pause following")
	}
	if !strings.Contains(m.View(), "[paused]") {
		t.Errorf("header should show [paused] after scroll-up; got:\n%s", m.View())
	}
	// A new line arriving while paused must not yank the view to the bottom.
	m.Append("line-51")
	if !strings.Contains(m.View(), "line-1") {
		t.Errorf("paused view should stay at the top, not jump to newest; got:\n%s", m.View())
	}
}

func TestFollowToggleResumesAtBottom(t *testing.T) {
	m := newLogs()
	appendLines(&m, 50)
	m, _ = m.Update(keymap.ActionTop) // pause at top
	if m.Following() {
		t.Fatal("precondition: should be paused")
	}
	m, _ = m.Update(keymap.ActionLogsFollow) // resume
	if !m.Following() {
		t.Fatal("logs.follow should re-enable following")
	}
	if !strings.Contains(m.View(), "line-50") {
		t.Errorf("resuming follow should jump to the newest line; got:\n%s", m.View())
	}
	// Toggle again pauses.
	m, _ = m.Update(keymap.ActionLogsFollow)
	if m.Following() {
		t.Error("second logs.follow should pause")
	}
}

func TestLiveFilterNarrowsShownLines(t *testing.T) {
	m := newLogs()
	m.Append("alpha error one")
	m.Append("beta ok")
	m.Append("gamma ERROR two")
	m = typeFilter(m, "error") // case-insensitive substring
	if !m.Filtering() {
		t.Fatal("filter should be open")
	}
	v := m.View()
	if !strings.Contains(v, "alpha error one") || !strings.Contains(v, "gamma ERROR two") {
		t.Errorf("filter should keep the two matching lines; got:\n%s", v)
	}
	if strings.Contains(v, "beta ok") {
		t.Errorf("filter should drop the non-matching line; got:\n%s", v)
	}
	// Header reflects the query and the matched/total count.
	if !strings.Contains(v, "/error") || !strings.Contains(v, "2/3") {
		t.Errorf("header should show the query and 2/3; got:\n%s", v)
	}
}

func TestFilterNarrowsWhileFollowing(t *testing.T) {
	m := newLogs()
	m = typeFilter(m, "keep")
	m.Append("drop this")
	m.Append("keep this")
	v := m.View()
	if strings.Contains(v, "drop this") {
		t.Errorf("a line streamed under an active filter must be excluded if it doesn't match; got:\n%s", v)
	}
	if !strings.Contains(v, "keep this") {
		t.Errorf("a matching streamed line should show; got:\n%s", v)
	}
}

func TestBackClearsFilterThenCloses(t *testing.T) {
	m := newLogs()
	m.Append("only line")
	m = typeFilter(m, "zzz") // matches nothing
	if strings.Contains(m.View(), "only line") {
		t.Fatalf("precondition: filter should hide the non-matching line")
	}
	// First back clears the filter (restores the stream), no ClosedMsg.
	m, cmd := m.Update(keymap.ActionBack)
	if cmd != nil {
		t.Errorf("back-while-filtering should not emit a cmd; got non-nil")
	}
	if m.Filtering() {
		t.Errorf("back should close the filter")
	}
	if !strings.Contains(m.View(), "only line") {
		t.Errorf("clearing the filter should restore the stream; got:\n%s", m.View())
	}
	// Second back closes the view.
	_, cmd = m.Update(keymap.ActionBack)
	if cmd == nil {
		t.Fatal("back with no filter should emit a ClosedMsg cmd")
	}
	closed, ok := cmd().(ClosedMsg)
	if !ok {
		t.Fatalf("close msg = %T; want ClosedMsg", cmd())
	}
	if closed.Kind != "logs" {
		t.Errorf("ClosedMsg.Kind = %q; want %q", closed.Kind, "logs")
	}
}

func TestResetClearsBufferAndRearmsFollow(t *testing.T) {
	m := newLogs()
	appendLines(&m, 10)
	m, _ = m.Update(keymap.ActionTop) // pause
	m = typeFilter(m, "line-1")
	m.Reset()
	if !m.Empty() {
		t.Errorf("Reset should clear the buffer")
	}
	if !m.Following() {
		t.Errorf("Reset should re-arm following")
	}
	if m.Filtering() {
		t.Errorf("Reset should close the filter")
	}
	if q := m.Query(); q != "" {
		t.Errorf("Reset should clear the query; got %q", q)
	}
}

func TestInactiveIgnoresActions(t *testing.T) {
	m := New(styles.Default())
	m.SetSize(40, 12)
	appendLines(&m, 10) // not shown
	var cmd tea.Cmd
	for _, a := range []keymap.Action{keymap.ActionBottom, keymap.ActionBack, keymap.ActionFilter, keymap.ActionLogsFollow} {
		m, cmd = m.Update(a)
		if cmd != nil {
			t.Errorf("inactive view emitted a cmd for %v; want nil", a)
		}
	}
	if m.View() != "" {
		t.Errorf("inactive View() = %q; want empty", m.View())
	}
}
