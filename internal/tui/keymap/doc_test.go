package keymap

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docPath is the committed generated reference, relative to this package dir
// (go test runs with the cwd set to the package directory).
var docPath = filepath.Join("..", "..", "..", "docs", "keybindings.md")

// update regenerates the committed doc instead of asserting it. Driven by
// `make keys-doc` (go test ./internal/tui/keymap -run TestKeybindingsDoc -update).
var update = flag.Bool("update", false, "regenerate docs/keybindings.md from the default keymap")

// TestKeybindingsDoc is the drift guard: docs/keybindings.md must equal what the
// default keymap generates. Any binding/description/registry change must be
// accompanied by a regenerated doc, or `make check` fails here — the committed
// file can't silently drift from the registry (D11). Run with -update to rewrite.
func TestKeybindingsDoc(t *testing.T) {
	want := DefaultKeymap().Markdown()

	if *update {
		if err := os.WriteFile(docPath, []byte(want), 0o644); err != nil {
			t.Fatalf("write %s: %v", docPath, err)
		}
		t.Logf("regenerated %s", docPath)
		return
	}

	got, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v (run `make keys-doc` to generate)", docPath, err)
	}
	if string(got) != want {
		t.Errorf("%s is stale — run `make keys-doc` to regenerate it from the keymap", docPath)
	}
}

// TestMarkdownFromRegistry proves the doc is derived from the registry, not
// restated literals: every registered action's id, its resolved keys, and its
// description all appear in the generated markdown.
func TestMarkdownFromRegistry(t *testing.T) {
	km := DefaultKeymap()
	md := km.Markdown()
	for _, a := range Actions() {
		if !strings.Contains(md, string(a)) || !strings.Contains(md, a.Describe()) {
			t.Errorf("markdown missing action %q or its description", a)
		}
		for _, tok := range km.Keys(a) {
			if !strings.Contains(md, "`"+tok+"`") {
				t.Errorf("markdown missing key %q for action %q", tok, a)
			}
		}
	}
}
