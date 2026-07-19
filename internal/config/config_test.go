package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

func TestLoadEmptyIsZeroConfig(t *testing.T) {
	c, err := Load(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Load(empty): %v", err)
	}
	if len(c.Keys) != 0 {
		t.Errorf("Keys = %v, want empty", c.Keys)
	}
}

func TestLoadKeys(t *testing.T) {
	c, err := Load(strings.NewReader("keys:\n  nav.down: [j, down]\n  app.quit: [q]\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Keys["nav.down"]; strings.Join(got, ",") != "j,down" {
		t.Errorf("nav.down = %v, want [j down]", got)
	}
	if got := c.Keys["app.quit"]; strings.Join(got, ",") != "q" {
		t.Errorf("app.quit = %v, want [q]", got)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := Load(strings.NewReader("bogus: true\n"))
	if err == nil {
		t.Fatal("Load(unknown field): expected error, got nil")
	}
}

func TestLoadFileMissingReturnsZeroConfig(t *testing.T) {
	c, err := LoadFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(missing): %v", err)
	}
	if c == nil || len(c.Keys) != 0 {
		t.Errorf("LoadFile(missing) = %+v, want zero config", c)
	}
}

func TestLoadFileReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  app.quit: [q]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if strings.Join(c.Keys["app.quit"], ",") != "q" {
		t.Errorf("app.quit = %v", c.Keys["app.quit"])
	}
}

func TestPath(t *testing.T) {
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(p), "kubecom/config.yaml") {
		t.Errorf("Path = %q, want it to end with kubecom/config.yaml", p)
	}
}

func TestKeymapResolvesOverride(t *testing.T) {
	c := &Config{Keys: map[string][]string{"nav.down": {"down"}}}
	km, warnings, err := c.Keymap()
	if err != nil {
		t.Fatalf("Keymap: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if got := strings.Join(km.Keys(keymap.ActionDown), ","); got != "down" {
		t.Errorf("nav.down keys = %q, want %q", got, "down")
	}
}

func TestKeymapWarnsOnNavShadow(t *testing.T) {
	// app.help binds a nav key (j); nav.down is moved off j so there is no
	// collision, leaving just the shadow warning.
	c := &Config{Keys: map[string][]string{
		"nav.down": {"down"},
		"app.help": {"j"},
	}}
	_, warnings, err := c.Keymap()
	if err != nil {
		t.Fatalf("Keymap: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "navigation key") {
		t.Errorf("warnings = %v, want one nav-shadow warning", warnings)
	}
}

func TestKeymapUnknownActionErrors(t *testing.T) {
	c := &Config{Keys: map[string][]string{"bogus.action": {"x"}}}
	if _, _, err := c.Keymap(); err == nil {
		t.Fatal("Keymap(unknown action): expected error, got nil")
	}
}

func TestKeymapBadTokenErrors(t *testing.T) {
	c := &Config{Keys: map[string][]string{"nav.down": {"ctrl+"}}}
	if _, _, err := c.Keymap(); err == nil {
		t.Fatal("Keymap(bad token): expected error, got nil")
	}
}
