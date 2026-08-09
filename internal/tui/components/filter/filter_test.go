package filter

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestOpenSeedsAndFocuses proves Open seeds the field with the caller's query and
// returns the Focus command that captures the keyboard, cursor at the end.
func TestOpenSeedsAndFocuses(t *testing.T) {
	m := New()
	m, cmd := m.Open("web")
	if !m.Active() {
		t.Fatal("Open should open the field")
	}
	if m.Value() != "web" {
		t.Fatalf("Open should seed the query, got %q", m.Value())
	}
	if cmd == nil {
		t.Fatal("Open should return the Focus command")
	}
}

// TestCommitKeepsValue proves Commit closes the field but keeps the query text, so
// a reopen seeds itself with the committed query (D238).
func TestCommitKeepsValue(t *testing.T) {
	m := New()
	m, _ = m.Open("")
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Text: "w"}))
	m = m.Commit()
	if m.Active() {
		t.Fatal("Commit should close the field")
	}
	if m.Value() != "w" {
		t.Fatalf("Commit should keep the query, got %q", m.Value())
	}
}

// TestResetClears proves Reset closes the field and drops the query — the teardown
// a cancel, a fresh resource, or a context switch wants.
func TestResetClears(t *testing.T) {
	m := New()
	m, _ = m.Open("web")
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	m = m.Reset()
	if m.Active() {
		t.Fatal("Reset should close the field")
	}
	if m.Value() != "" {
		t.Fatalf("Reset should clear the query, got %q", m.Value())
	}
}

// TestUpdateFeedsText proves Update delegates a keypress to the textinput, so the
// query reflects what the reader typed and the field stays open.
func TestUpdateFeedsText(t *testing.T) {
	m := New()
	m, _ = m.Open("")
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Text: "w"}))
	if !m.Active() {
		t.Fatal("typing should not close the field")
	}
	if m.Value() != "w" {
		t.Fatalf("typing should append to the query, got %q", m.Value())
	}
}

// TestViewRendersThePrompt proves View renders the live "/query" prompt the status
// bar shows while editing — the textinput's render, query text included.
func TestViewRendersThePrompt(t *testing.T) {
	m := New()
	m, _ = m.Open("web")
	got := m.View()
	if !strings.Contains(got, "web") {
		t.Fatalf("View should contain the query text, got %q", got)
	}
}
