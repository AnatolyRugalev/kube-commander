package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts a file in dir.
func write(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustLoad(t *testing.T, dir string) *Config {
	t.Helper()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// No files is the zero config: kubecom runs on defaults.
func TestLoadEmptyDirIsDefaults(t *testing.T) {
	c := mustLoad(t, t.TempDir())
	if c.Theme() != "" || len(c.Overrides()) != 0 || len(c.Menu("any")) != 0 {
		t.Fatalf("an empty dir is not the defaults: theme %q, overrides %v", c.Theme(), c.Overrides())
	}
	if _, ok := c.Keymap(); ok {
		t.Fatal("an empty dir has a keymap")
	}
}

func TestLoadTheme(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, AuthoredName, "version 1\n// mine\ntheme \"monokai\"\n")
	if got := mustLoad(t, dir).Theme(); got != "monokai" {
		t.Fatalf("theme = %q", got)
	}
}

// The overlay is read on top of the authored file, key by key.
func TestTheOverlayWins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, AuthoredName, "version 1\ntheme \"monokai\"\n")
	write(t, dir, OverlayName, "version 1\ntheme \"solarized-dark\"\n")
	c := mustLoad(t, dir)
	if c.Theme() != "solarized-dark" {
		t.Fatalf("theme = %q, want the overlay's", c.Theme())
	}
	if got := c.Overrides(); len(got) != 1 || got[0] != "theme" {
		t.Fatalf("overrides = %v", got)
	}
}

// Every mistake is an error naming the file and the line, and no Config.
func TestLoadErrorsNameTheLine(t *testing.T) {
	for src, want := range map[string]string{
		"theme \"x\"\n":                        "the file must start with `version 1`",
		"version 2\n":                          "version 2 is newer than this kubecom understands (1)",
		"version 0\n":                          "unknown version 0",
		"version \"1\"\n":                      "`version` takes one integer",
		"version 1\ncolour \"red\"\n":          ":2:1: colour is not a setting (kubecom reads `theme`",
		"version 1\ntheme 3\n":                 ":2:1: theme takes a string, not a int",
		"version 1\ntheme \"a\" \"b\"\n":       "theme takes one value, not 2",
		"version 1\ntheme x=\"a\"\n":           "a setting takes no properties",
		"version 1\nkeys { nav.down \"j\" }\n": "keys.nav.down is not a setting",
		"version 1\ntheme \"a\n":               config_kdl_line(2),
		"version 1\nversion 1\n":               "`version` is given twice",
	} {
		dir := t.TempDir()
		write(t, dir, AuthoredName, src)
		c, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", src, err, want)
		}
		if c != nil {
			t.Errorf("%q: a Config came back with the error", src)
		}
		if err != nil && !strings.Contains(err.Error(), AuthoredName) {
			t.Errorf("%q: the error does not name the file: %v", src, err)
		}
	}
}

// config_kdl_line is how a KDL syntax error is placed: file:line:column.
func config_kdl_line(line int) string { return AuthoredName + ":" + string(rune('0'+line)) + ":" }

// A keymap is a document: handed over whole, from either file.
func TestKeymapDocuments(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, AuthoredName, "version 1\nkeymap {\n    bind \"x\" \"app.quit\"\n}\n")
	write(t, dir, OverlayName, "version 1\nkeymap { unbind \"q\"; }\n")
	c := mustLoad(t, dir)
	nodes, ok := c.Keymap()
	if !ok || len(nodes) != 1 || nodes[0].Name() != "bind" || nodes[0].Location().Line != 3 {
		t.Fatalf("authored keymap = %v, %v", nodes, ok)
	}
	nodes, ok = c.OverlayKeymap()
	if !ok || len(nodes) != 1 || nodes[0].Name() != "unbind" {
		t.Fatalf("overlay keymap = %v, %v", nodes, ok)
	}
}

// Set writes the overlay and never the authored file; a second Set changes the
// node where it stands and keeps the rest of the overlay, comments included.
func TestSetWritesTheOverlayOnly(t *testing.T) {
	dir := t.TempDir()
	authored := "version 1\n// my theme\ntheme \"monokai\"\n"
	write(t, dir, AuthoredName, authored)
	c := mustLoad(t, dir)
	if err := c.Set("theme", "solarized-dark"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, AuthoredName)); string(got) != authored {
		t.Fatalf("config.kdl was written:\n%s", got)
	}
	if c.Theme() != "solarized-dark" || mustLoad(t, dir).Theme() != "solarized-dark" {
		t.Fatal("the overlay's theme did not take")
	}
	overlay := filepath.Join(dir, OverlayName)
	src, _ := os.ReadFile(overlay)
	write(t, dir, OverlayName, strings.Replace(string(src), "theme", "// kubecom wrote this\ntheme", 1))
	c = mustLoad(t, dir)
	if err := c.Set("theme", "monokai"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(overlay)
	if !strings.Contains(string(got), "// kubecom wrote this") || strings.Count(string(got), "theme") != 1 ||
		!strings.Contains(string(got), `theme monokai`) && !strings.Contains(string(got), `theme "monokai"`) {
		t.Fatalf("overlay after a second Set:\n%s", got)
	}
	if fi, err := os.Stat(overlay); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("overlay mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("a temp file was left behind: %v", entries)
	}
}

// Set creates the directory and the overlay when neither exists.
func TestSetCreatesTheOverlay(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "kubecom")
	c := mustLoad(t, dir)
	if err := c.Set("theme", "monokai"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, OverlayName)); !strings.HasPrefix(string(got), "version 1\n") {
		t.Fatalf("overlay:\n%s", got)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, %v; want 0700", fi.Mode().Perm(), err)
	}
}

func TestSetRefusesWhatIsNotASetting(t *testing.T) {
	c := mustLoad(t, t.TempDir())
	if err := c.Set("colour", "red"); err == nil || !strings.Contains(err.Error(), "colour is not a setting") {
		t.Errorf("unknown key: %v", err)
	}
	if err := c.Set("theme", 3); err == nil || !strings.Contains(err.Error(), "theme takes a string") {
		t.Errorf("wrong kind: %v", err)
	}
}

// WriteEmptyOverlay marks a migration with nothing to carry, once.
func TestWriteEmptyOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := WriteEmptyOverlay(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, OverlayName)); string(got) != "version 1\n" {
		t.Fatalf("overlay = %q", got)
	}
	write(t, dir, OverlayName, "version 1\ntheme \"monokai\"\n")
	if err := WriteEmptyOverlay(dir); err != nil || mustLoad(t, dir).Theme() != "monokai" {
		t.Fatal("an existing overlay was replaced")
	}
}

// Retired names the YAML an older kubecom read.
func TestRetired(t *testing.T) {
	dir := t.TempDir()
	if got := Retired(dir); len(got) != 0 {
		t.Fatalf("retired in an empty dir: %v", got)
	}
	write(t, dir, "config.yaml", "theme: monokai\n")
	if err := os.Mkdir(filepath.Join(dir, "menus"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := Retired(dir); len(got) != 2 || !strings.HasSuffix(got[0], "config.yaml") || !strings.HasSuffix(got[1], "menus") {
		t.Fatalf("retired = %v", got)
	}
}

func TestDir(t *testing.T) {
	d, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := os.UserConfigDir()
	if d != filepath.Join(base, "kubecom") {
		t.Fatalf("Dir() = %q", d)
	}
}
