package secretviewer

import (
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// testData is a two-entry Opaque secret: a single-line value and a multi-line one,
// which is what exercises the mask, the reveal and the indented block.
func testData() kube.SecretData {
	return kube.SecretData{
		Type: "Opaque",
		Entries: []kube.SecretEntry{
			{Key: "password", Value: "s3cr3t"},
			{Key: "token", Value: "line1\nline2"},
		},
	}
}

// TestRenderCursorAndLines is the render half of M3-08b, moved with the component:
// the cursor gutter marks the selected entry, the returned line offsets point at
// each entry's key line, the masked render never contains a value, and a revealed
// multi-line value is indented under its key with the offsets tracking it.
func TestRenderCursorAndLines(t *testing.T) {
	// Masked, cursor on the second entry (moved from 0).
	m := Model{}
	m.Move(1, 2)
	content, lines := m.Render(testData())
	if len(lines) != 2 {
		t.Fatalf("want a line offset per entry, got %d", len(lines))
	}
	ls := strings.Split(content, "\n")
	if !strings.HasPrefix(ls[lines[1]], Cursor+"token") {
		t.Fatalf("the cursor gutter should mark the selected entry: %q", ls[lines[1]])
	}
	if !strings.HasPrefix(ls[lines[0]], Gutter+"password") {
		t.Fatalf("an unselected entry should carry the plain gutter: %q", ls[lines[0]])
	}
	if strings.Contains(content, "s3cr3t") || strings.Contains(content, "line1") {
		t.Fatalf("a masked render must not contain any value: %q", content)
	}
	if !strings.Contains(content, "hidden") {
		t.Fatalf("a masked render should say it is hidden: %q", content)
	}

	// Revealed, cursor back on the first entry.
	m = Model{}
	m.ToggleReveal()
	content, lines = m.Render(testData())
	ls = strings.Split(content, "\n")
	if !strings.HasPrefix(ls[lines[0]], Cursor+"password") {
		t.Fatalf("revealed cursor gutter wrong: %q", ls[lines[0]])
	}
	if !strings.Contains(content, "s3cr3t") {
		t.Fatal("a revealed render should contain the single-line value")
	}
	if !strings.Contains(content, "revealed") {
		t.Fatalf("a revealed render should say it is revealed: %q", content)
	}
	if !strings.HasPrefix(ls[lines[1]], Gutter+"token:") {
		t.Fatalf("a multi-line entry's key line wrong: %q", ls[lines[1]])
	}
	if ls[lines[1]+1] != Gutter+"  line1" {
		t.Fatalf("a multi-line value should be indented under its key: %q", ls[lines[1]+1])
	}
}

// TestRenderEmptyData proves a Secret with no data renders the type header, a
// "(no data)" body and no entry lines (so the shell's EnsureLineVisible is never
// asked for line -1).
func TestRenderEmptyData(t *testing.T) {
	m := Model{}
	content, lines := m.Render(kube.SecretData{Type: "Opaque"})
	if lines != nil {
		t.Fatalf("no entries → no line offsets, got %v", lines)
	}
	if !strings.Contains(content, "Opaque") || !strings.Contains(content, "(no data)") {
		t.Fatalf("empty secret should name its type and say there is no data: %q", content)
	}
}

// TestResetMasksAndZerosCursor proves Reset restores the deliberate-reveal
// contract — values hidden, cursor at the top — from any state, which is what a
// fresh open (openSecretViewer) and a context switch (resetCluster) both want.
func TestResetMasksAndZerosCursor(t *testing.T) {
	m := Model{}
	m.ToggleReveal()
	m.Move(1, 2)
	m.Reset()
	if m.Revealed() {
		t.Fatal("Reset should mask the values again")
	}
	if m.Sel() != 0 {
		t.Fatalf("Reset should zero the cursor, got %d", m.Sel())
	}
	// The reset state is exactly a fresh model.
	if want := (Model{}); m != want {
		t.Fatalf("Reset should restore the zero state, got %+v", m)
	}
}

// TestToggleRevealFlips proves ToggleReveal flips the Revealed flag each press,
// and only by an explicit gesture.
func TestToggleRevealFlips(t *testing.T) {
	m := Model{}
	if m.Revealed() {
		t.Fatal("a fresh model is masked")
	}
	m.ToggleReveal()
	if !m.Revealed() {
		t.Fatal("ToggleReveal should unmask")
	}
	m.ToggleReveal()
	if m.Revealed() {
		t.Fatal("a second ToggleReveal should re-mask")
	}
}

// TestMoveClamps proves Move steps the cursor and clamps at both ends, and that
// an empty set pins the cursor to 0 (a down/up press against an empty secret is
// inert).
func TestMoveClamps(t *testing.T) {
	m := Model{}
	m.Move(1, 2)
	if m.Sel() != 1 {
		t.Fatalf("Move(1, 2) should land on 1, got %d", m.Sel())
	}
	m.Move(1, 2)
	if m.Sel() != 1 {
		t.Fatalf("Move past the end should clamp at the last entry, got %d", m.Sel())
	}
	m.Move(-1, 2)
	m.Move(-1, 2)
	if m.Sel() != 0 {
		t.Fatalf("Move before the start should clamp at the first entry, got %d", m.Sel())
	}
	m.Move(1, 0)
	if m.Sel() != 0 {
		t.Fatalf("Move against an empty set should pin the cursor to 0, got %d", m.Sel())
	}
}
