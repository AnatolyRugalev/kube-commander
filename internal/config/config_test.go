package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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

// TestLoadTheme proves the theme: key decodes into the field the launcher resolves
// against the theme registry (M4-12a). The value is carried verbatim — validation
// is styles.ByName's job, not the decoder's.
func TestLoadTheme(t *testing.T) {
	c, err := Load(strings.NewReader("theme: monokai\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Theme != "monokai" {
		t.Errorf("Theme = %q, want %q", c.Theme, "monokai")
	}
}

// TestLoadThemeAbsentIsEmpty proves an absent theme: key decodes to "" — the value
// the launcher reads as "use the built-in default", silently. A config that names
// only keys must not imply a theme.
func TestLoadThemeAbsentIsEmpty(t *testing.T) {
	c, err := Load(strings.NewReader("keys:\n  app.quit: [q]\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Theme != "" {
		t.Errorf("Theme = %q, want empty for an absent key", c.Theme)
	}
}

// TestSaveRoundTripsTheme proves the field survives the write-back path M4-12b's
// picker will persist through: what Save emits, Load reads back equal.
func TestSaveRoundTripsTheme(t *testing.T) {
	orig := &Config{Theme: "solarized-dark", Keys: map[string][]string{"app.quit": {"q"}}}
	var buf bytes.Buffer
	if err := orig.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load(saved): %v", err)
	}
	if got.Theme != orig.Theme {
		t.Errorf("round-trip Theme = %q, want %q", got.Theme, orig.Theme)
	}
	if !reflect.DeepEqual(got.Keys, orig.Keys) {
		t.Errorf("round-trip Keys = %v, want %v", got.Keys, orig.Keys)
	}
}

// TestSaveOmitsEmptyTheme keeps the omitempty contract: an unset theme must not
// appear in the file, so a saved config stays as small as what the user wrote.
func TestSaveOmitsEmptyTheme(t *testing.T) {
	var buf bytes.Buffer
	if err := (&Config{Keys: map[string][]string{"app.quit": {"q"}}}).Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if strings.Contains(buf.String(), "theme") {
		t.Errorf("Save emitted a theme key for an unset theme:\n%s", buf.String())
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

func TestSaveRoundTrips(t *testing.T) {
	orig := &Config{Keys: map[string][]string{
		"nav.down": {"j", "down"},
		"app.quit": {"q"},
	}}
	var buf bytes.Buffer
	if err := orig.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load(saved): %v", err)
	}
	if !reflect.DeepEqual(got.Keys, orig.Keys) {
		t.Errorf("round-trip Keys = %v, want %v", got.Keys, orig.Keys)
	}
}

func TestSaveEmptyRoundTripsToZeroConfig(t *testing.T) {
	var buf bytes.Buffer
	if err := (&Config{}).Save(&buf); err != nil {
		t.Fatalf("Save(empty): %v", err)
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load(empty saved): %v", err)
	}
	if len(got.Keys) != 0 {
		t.Errorf("round-trip empty Keys = %v, want empty", got.Keys)
	}
}

func TestSaveFileCreatesParentAndRoundTrips(t *testing.T) {
	// A parent dir that does not yet exist must be created (0o700).
	dir := filepath.Join(t.TempDir(), "kubecom")
	path := filepath.Join(dir, "config.yaml")
	orig := &Config{Keys: map[string][]string{"app.quit": {"q"}}}
	if err := orig.SaveFile(path); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile(saved): %v", err)
	}
	if !reflect.DeepEqual(got.Keys, orig.Keys) {
		t.Errorf("round-trip Keys = %v, want %v", got.Keys, orig.Keys)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file perm = %o, want 600", perm)
	}
}

func TestSaveFileReplacesAtomicallyWithoutLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  nav.up: [k]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := &Config{Keys: map[string][]string{"nav.down": {"j"}}}
	if err := next.SaveFile(path); err != nil {
		t.Fatalf("SaveFile(overwrite): %v", err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if _, stale := got.Keys["nav.up"]; stale {
		t.Errorf("old content survived overwrite: %v", got.Keys)
	}
	if !reflect.DeepEqual(got.Keys, next.Keys) {
		t.Errorf("overwritten Keys = %v, want %v", got.Keys, next.Keys)
	}
	// The atomic temp file must be gone — only config.yaml should remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir entries = %v, want only [config.yaml]", names)
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
