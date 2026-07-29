package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// writeMenuFile points the user config dir at a temp dir and writes body to the
// per-context menu file for context, returning its path. It uses config.MenuPath
// so the test exercises the real path resolution the launcher uses.
func writeMenuFile(t *testing.T, context, body string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := config.MenuPath(context)
	if err != nil {
		t.Fatalf("MenuPath(%q): %v", context, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir menus dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write menu file: %v", err)
	}
}

// TestLoadMenuExtrasBlankContext proves an unresolved context yields no extras and
// no error — the launcher must not block when it cannot name a context (principle 3).
func TestLoadMenuExtrasBlankContext(t *testing.T) {
	extras, err := loadMenuExtras("  ")
	if err != nil {
		t.Fatalf("blank context should not error, got %v", err)
	}
	if extras != nil {
		t.Fatalf("blank context should yield no extras, got %v", extras)
	}
}

// TestLoadMenuExtrasMissingFile proves a context with no menu file degrades to the
// default menu: no extras, no error (the common case).
func TestLoadMenuExtrasMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	extras, err := loadMenuExtras("no-such-context")
	if err != nil {
		t.Fatalf("missing menu file should not error, got %v", err)
	}
	if len(extras) != 0 {
		t.Fatalf("missing menu file should yield no extras, got %v", extras)
	}
}

// TestLoadMenuExtrasReadsFile proves a valid per-context menu file's entries are
// returned to the launcher (which feeds them to the menu via WithMenuExtras).
func TestLoadMenuExtrasReadsFile(t *testing.T) {
	const ctx = "prod"
	writeMenuFile(t, ctx, `resources:
  - group: cert-manager.io
    version: v1
    resource: certificates
    kind: Certificate
    namespaced: true
`)
	extras, err := loadMenuExtras(ctx)
	if err != nil {
		t.Fatalf("valid menu file should load, got %v", err)
	}
	if len(extras) != 1 {
		t.Fatalf("expected 1 extra resource, got %d", len(extras))
	}
	got := extras[0]
	want := config.MenuResource{
		Group: "cert-manager.io", Version: "v1", Resource: "certificates",
		Kind: "Certificate", Namespaced: true,
	}
	if got != want {
		t.Fatalf("extra = %+v, want %+v", got, want)
	}
}

// TestLoadMenuExtrasMalformedFile proves a malformed/unreadable menu file returns
// an error (the launcher turns it into a startup toast) rather than silently
// dropping — but never a fatal launch failure (the caller degrades to the default
// menu). Here an unknown field trips the strict decoder.
func TestLoadMenuExtrasMalformedFile(t *testing.T) {
	writeMenuFile(t, "broken", "resources:\n  - bogusField: nope\n")
	if _, err := loadMenuExtras("broken"); err == nil {
		t.Fatal("a malformed menu file should return an error to surface as a toast")
	}
}

// writeStateFile points the user config dir at a temp dir and writes body to the
// per-context state file for context, using the real config.StatePath resolution.
func writeStateFile(t *testing.T, context, body string) {
	t.Helper()
	path, err := config.StatePath(context)
	if err != nil {
		t.Fatalf("StatePath(%q): %v", context, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}

// writeLegacyConfig points $HOME at a temp dir and writes body to the legacy
// ~/.kubecom.yaml, so maybeMigrate resolves a real legacy file. It uses
// config.LegacyPath so the same resolution the launcher uses is exercised.
func writeLegacyConfig(t *testing.T, body string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path, err := config.LegacyPath()
	if err != nil {
		t.Fatalf("LegacyPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
}

// TestMaybeMigrateNoLegacyFile proves a fresh install with no legacy ~/.kubecom.yaml
// performs no migration and writes no config: nothing to migrate is the common case.
func TestMaybeMigrateNoLegacyFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // an empty home: no legacy file present.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if toast := maybeMigrate(cfgPath); toast != nil {
		t.Fatalf("no legacy file should migrate nothing, got toast %+v", toast)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("no legacy file should write no config; stat err = %v", err)
	}
}

// TestMaybeMigrateExistingConfigSuppresses proves a present new-format config makes
// migration a no-op (one-shot): the legacy file is left for the user and the config
// is not overwritten, even though a legacy file exists.
func TestMaybeMigrateExistingConfigSuppresses(t *testing.T) {
	writeLegacyConfig(t, "menu:\n  - kind: Certificate\n")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("keys:\n  app.quit: [x]\n"), 0o600); err != nil {
		t.Fatalf("seed existing config: %v", err)
	}
	if toast := maybeMigrate(cfgPath); toast != nil {
		t.Fatalf("existing config should suppress migration, got toast %+v", toast)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "app.quit") {
		t.Fatalf("existing config must not be overwritten, got %q", string(data))
	}
}

// TestMaybeMigrateReportsNotes proves a legacy file with content that cannot be
// carried over (menu resources, themes) is migrated: a fresh config is written and
// the notes are surfaced as a startup toast.
func TestMaybeMigrateReportsNotes(t *testing.T) {
	writeLegacyConfig(t, "menu:\n  - kind: Certificate\ncurrentTheme: dark\n")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	toast := maybeMigrate(cfgPath)
	if toast == nil {
		t.Fatal("a legacy file with menu/theme content should surface migration notes")
	}
	if msg := toast.Message(); !strings.Contains(msg, "menu") || !strings.Contains(msg, "theme") {
		t.Fatalf("toast should report the un-migratable menu and theme, got %q", msg)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("migration should write the new config once, stat err = %v", err)
	}
}

// TestMaybeMigrateEmptyLegacyNoNotes proves an empty legacy file still establishes
// the new config (so migration is one-shot) but surfaces no toast — there is
// nothing to report.
func TestMaybeMigrateEmptyLegacyNoNotes(t *testing.T) {
	writeLegacyConfig(t, "")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if toast := maybeMigrate(cfgPath); toast != nil {
		t.Fatalf("an empty legacy file has nothing to report, got toast %+v", toast)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("migration should still write the new config to be one-shot, stat err = %v", err)
	}
}

// TestMaybeMigrateMalformedLegacyDegrades proves an unparseable legacy file degrades
// to no migration and writes no config (so a later start can migrate a fixed file)
// — it never blocks launch (D92).
func TestMaybeMigrateMalformedLegacyDegrades(t *testing.T) {
	writeLegacyConfig(t, "menu: [oops\n")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if toast := maybeMigrate(cfgPath); toast != nil {
		t.Fatalf("a malformed legacy file should not surface a toast, got %+v", toast)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("a malformed legacy file must write no config; stat err = %v", err)
	}
}

// TestResolveThemeEmptyIsDefaultAndSilent proves a config with no theme: key runs
// on the built-in palette without reporting anything — the overwhelmingly common
// case must be silent (M4-12a).
func TestResolveThemeEmptyIsDefaultAndSilent(t *testing.T) {
	for _, name := range []string{"", "   "} {
		theme, err := resolveTheme(name)
		if err != nil {
			t.Fatalf("resolveTheme(%q) errored: %v", name, err)
		}
		if theme.Name != styles.DefaultTheme().Name {
			t.Errorf("resolveTheme(%q) = %q, want the default theme", name, theme.Name)
		}
	}
}

// TestResolveThemeResolvesEveryBuiltin proves the config field reaches every theme
// the registry ships, through the same lenient lookup (D169): a name a picker could
// offer is a name the config file accepts.
func TestResolveThemeResolvesEveryBuiltin(t *testing.T) {
	for _, want := range styles.ThemeNames() {
		for _, written := range []string{want, strings.ToUpper(want), "  " + want + " "} {
			theme, err := resolveTheme(written)
			if err != nil {
				t.Fatalf("resolveTheme(%q): %v", written, err)
			}
			if theme.Name != want {
				t.Errorf("resolveTheme(%q) = %q, want %q", written, theme.Name, want)
			}
		}
	}
}

// TestResolveThemeUnknownDegradesToDefault is the principle-3 half: a typo'd or
// removed theme name must not keep kubecom from launching. It returns the default
// palette *and* an error the launcher toasts once, naming the available themes so
// the message is actionable — never a guess at what was meant (D169 pt 2).
func TestResolveThemeUnknownDegradesToDefault(t *testing.T) {
	theme, err := resolveTheme("solarized") // the family name; the built-in is solarized-dark
	if err == nil {
		t.Fatal("resolveTheme(unknown): expected an error to report, got nil")
	}
	if theme.Name != styles.DefaultTheme().Name {
		t.Fatalf("resolveTheme(unknown) = %q, want the default theme", theme.Name)
	}
	if !strings.Contains(err.Error(), "solarized-dark") {
		t.Errorf("error should list the available themes, got %q", err)
	}
}

// TestInitialNamespaceExplicitWins proves an explicit -n overrides the stored
// last-namespace for the run (D91).
func TestInitialNamespaceExplicitWins(t *testing.T) {
	opts := runOptions{namespace: "flag-ns", namespaceSet: true}
	if got := initialNamespace(opts, &config.State{LastNamespace: "stored-ns"}); got != "flag-ns" {
		t.Fatalf("initialNamespace = %q, want flag-ns (explicit -n wins)", got)
	}
}

// TestInitialNamespaceExplicitEmptyWins proves an explicit `-n ""` (all namespaces)
// overrides a stored scope — the flag being *set* is what matters, not its value.
func TestInitialNamespaceExplicitEmptyWins(t *testing.T) {
	opts := runOptions{namespace: "", namespaceSet: true}
	if got := initialNamespace(opts, &config.State{LastNamespace: "stored-ns"}); got != "" {
		t.Fatalf("initialNamespace = %q, want \"\" (explicit -n \"\" wins over stored)", got)
	}
}

// TestInitialNamespaceFromState proves that with no -n given, the stored
// last-namespace is restored.
func TestInitialNamespaceFromState(t *testing.T) {
	opts := runOptions{namespaceSet: false}
	if got := initialNamespace(opts, &config.State{LastNamespace: "stored-ns"}); got != "stored-ns" {
		t.Fatalf("initialNamespace = %q, want stored-ns (restore last namespace)", got)
	}
}

// TestLoadStateBlankContext proves an unresolved context disables persistence: zero
// state and an empty path (no context ⇒ no per-context state file).
func TestLoadStateBlankContext(t *testing.T) {
	state, path := loadState("  ")
	if state == nil || state.LastNamespace != "" {
		t.Fatalf("blank context should yield the zero State, got %+v", state)
	}
	if path != "" {
		t.Fatalf("blank context should yield no state path, got %q", path)
	}
}

// TestLoadStateMissingFile proves a context kubecom has never saved state for starts
// on defaults (zero state) but still reports a path to persist future changes to.
func TestLoadStateMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, path := loadState("fresh-context")
	if state == nil || state.LastNamespace != "" {
		t.Fatalf("missing state file should yield the zero State, got %+v", state)
	}
	if path == "" {
		t.Fatal("a resolved context should still report a state path for persistence")
	}
}

// TestLoadStateReadsFile proves a saved last-namespace is loaded back for the
// context (the restore path).
func TestLoadStateReadsFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeStateFile(t, "prod", "lastNamespace: kube-system\n")
	state, path := loadState("prod")
	if state.LastNamespace != "kube-system" {
		t.Fatalf("loaded LastNamespace = %q, want kube-system", state.LastNamespace)
	}
	if path == "" {
		t.Fatal("loadState should report the state path")
	}
}

// TestStatePersisterRoundTrip proves the launcher's persister seam writes a picked
// namespace to the state file so the next loadState restores it.
func TestStatePersisterRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, path := loadState("prod")
	p := &statePersister{path: path, state: state}
	if err := p.PersistNamespace("monitoring"); err != nil {
		t.Fatalf("PersistNamespace: %v", err)
	}
	reloaded, _ := loadState("prod")
	if reloaded.LastNamespace != "monitoring" {
		t.Fatalf("reloaded LastNamespace = %q, want monitoring", reloaded.LastNamespace)
	}
}
