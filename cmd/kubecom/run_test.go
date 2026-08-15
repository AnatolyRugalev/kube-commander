package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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

// TestMaybeMigrateCarriesThemeIntoTheWrittenConfig closes the loop M5-04 opened: a
// legacy theme selection is not merely *reported*, it must survive into the config
// file the launcher writes and then resolve to the palette the shell renders with.
// `solarized` also exercises the one legacy rename (it became `solarized-dark`), so
// this fails if either the alias or the write-back regresses.
func TestMaybeMigrateCarriesThemeIntoTheWrittenConfig(t *testing.T) {
	writeLegacyConfig(t, "currentTheme: solarized\n")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	toast := maybeMigrate(cfgPath)
	if toast == nil {
		t.Fatal("a carried-over theme should still be reported so the user knows")
	}
	if msg := toast.Message(); !strings.Contains(msg, "solarized-dark") {
		t.Errorf("toast %q should name the v1 theme the choice became", msg)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(data), "theme: solarized-dark") {
		t.Fatalf("migrated config must persist the theme, got %q", string(data))
	}
	// And the persisted name must resolve on the launcher's own path, not just look
	// right in YAML: a value the config writes but resolveTheme rejects would
	// silently launch on the default and warn.
	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile(migrated): %v", err)
	}
	theme, err := resolveTheme(cfg.Theme)
	if err != nil {
		t.Fatalf("resolveTheme(%q) after migration: %v", cfg.Theme, err)
	}
	if theme.Name != "solarized-dark" {
		t.Errorf("resolved theme = %q, want %q", theme.Name, "solarized-dark")
	}
}

// TestMaybeMigrateOverTheSchemaDerivedLegacyFile is M5-05's end-to-end: the whole
// launcher migration path — legacy file on disk → config.Migrate → SaveFile →
// LoadFile → resolveTheme — driven by a `~/.kubecom.yaml` the *2020 writer* produced
// (internal/config/testdata/legacy-kubecom.yaml, generated by protojson.Marshal +
// yaml.JSONToYAML over a pb.Config; see that directory's README and D180). Every
// other maybeMigrate test feeds it YAML a test author typed, which is exactly what
// exit criterion 4 was unconvinced by.
func TestMaybeMigrateOverTheSchemaDerivedLegacyFile(t *testing.T) {
	// The fixture lives with the code it tests (internal/config). Reading it across
	// packages keeps one copy: two would drift, and the schema guard only watches one.
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "testdata", "legacy-kubecom.yaml"))
	if err != nil {
		t.Fatalf("read legacy fixture: %v", err)
	}
	writeLegacyConfig(t, string(fixture))
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	toast := maybeMigrate(cfgPath)
	if toast == nil {
		t.Fatal("a real legacy file with a menu and a theme must report what it could not carry")
	}
	msg := toast.Message()
	for _, want := range []string{"4 legacy menu resources", "Deployments", "solarized-dark", "palettes"} {
		if !strings.Contains(msg, want) {
			t.Errorf("startup toast %q missing %q", msg, want)
		}
	}

	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile(migrated): %v", err)
	}
	theme, err := resolveTheme(cfg.Theme)
	if err != nil {
		t.Fatalf("resolveTheme(%q) after migrating a real legacy file: %v", cfg.Theme, err)
	}
	if theme.Name != "solarized-dark" {
		t.Errorf("resolved theme = %q, want %q — the 2020 `solarized` selection", theme.Name, "solarized-dark")
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

// TestPersistPinRoundTripsAndKeepsTheNamespace covers the other write into the same
// file (CRD-PIN-02) and the reason the seam holds the loaded state rather than
// re-reading it per call: SaveFile marshals the whole struct, so a pin written from a
// fresh State would drop the namespace the same launch recorded — and vice versa.
// A second pin of the same GVR writes nothing (State.Pin's dedupe, D193 pt 5).
func TestPersistPinRoundTripsAndKeepsTheNamespace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, path := loadState("prod")
	p := &statePersister{path: path, state: state}
	if err := p.PersistNamespace("monitoring"); err != nil {
		t.Fatalf("PersistNamespace: %v", err)
	}
	pin := config.MenuResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
		Kind: "ExternalSecret", Namespaced: true,
	}
	if err := p.PersistPin(pin); err != nil {
		t.Fatalf("PersistPin: %v", err)
	}
	if err := p.PersistPin(pin); err != nil {
		t.Fatalf("PersistPin (repeat): %v", err)
	}

	reloaded, _ := loadState("prod")
	if len(reloaded.PinnedResources) != 1 || reloaded.PinnedResources[0] != pin {
		t.Fatalf("reloaded PinnedResources = %+v, want exactly %+v", reloaded.PinnedResources, pin)
	}
	if reloaded.LastNamespace != "monitoring" {
		t.Fatalf("reloaded LastNamespace = %q — a pin must not drop the recorded namespace", reloaded.LastNamespace)
	}
	// And unpinning takes it back out (CRD-PIN-03), leaving the namespace behind for
	// the same reason: one loaded State, mutated in place, written whole.
	if err := p.PersistUnpin(pin); err != nil {
		t.Fatalf("PersistUnpin: %v", err)
	}
	if err := p.PersistUnpin(pin); err != nil {
		t.Fatalf("PersistUnpin (repeat): %v", err)
	}
	reloaded, _ = loadState("prod")
	if len(reloaded.PinnedResources) != 0 {
		t.Fatalf("reloaded PinnedResources = %+v, want none after the unpin", reloaded.PinnedResources)
	}
	if reloaded.LastNamespace != "monitoring" {
		t.Fatalf("reloaded LastNamespace = %q — an unpin must not drop the recorded namespace", reloaded.LastNamespace)
	}
}

// TestPersistResourceRoundTripsAndKeepsTheRestOfTheState covers the third write into
// the same file (CTX-MEM-02/D240): the kind the reader last browsed, restored on the
// next launch and on every switch back. Like the pin it is written through the one
// retained State, so a drill-in must not drop the namespace or the pins the same
// session recorded — SaveFile marshals the whole struct, and this is the file's third
// chance to lose the other two fields.
func TestPersistResourceRoundTripsAndKeepsTheRestOfTheState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, path := loadState("prod")
	p := &statePersister{path: path, state: state}
	if err := p.PersistNamespace("monitoring"); err != nil {
		t.Fatalf("PersistNamespace: %v", err)
	}
	pin := config.MenuResource{Group: "g", Version: "v1", Resource: "widgets", Kind: "Widget"}
	if err := p.PersistPin(pin); err != nil {
		t.Fatalf("PersistPin: %v", err)
	}
	last := config.MenuResource{
		Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets",
		Kind: "ExternalSecret", Namespaced: true,
	}
	if err := p.PersistResource(last, nil); err != nil {
		t.Fatalf("PersistResource: %v", err)
	}

	reloaded, _ := loadState("prod")
	if reloaded.LastResource == nil || *reloaded.LastResource != last {
		t.Fatalf("reloaded LastResource = %+v, want %+v", reloaded.LastResource, last)
	}
	if reloaded.LastNamespace != "monitoring" {
		t.Errorf("reloaded LastNamespace = %q — a drill-in must not drop the namespace", reloaded.LastNamespace)
	}
	if len(reloaded.PinnedResources) != 1 || reloaded.PinnedResources[0] != pin {
		t.Errorf("reloaded PinnedResources = %+v — a drill-in must not drop the pins", reloaded.PinnedResources)
	}

	// Moving on overwrites rather than accumulates: the file remembers one place.
	next := config.MenuResource{Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true}
	if err := p.PersistResource(next, nil); err != nil {
		t.Fatalf("PersistResource (second): %v", err)
	}
	reloaded, _ = loadState("prod")
	if reloaded.LastResource == nil || *reloaded.LastResource != next {
		t.Fatalf("reloaded LastResource = %+v, want %+v", reloaded.LastResource, next)
	}
}

// TestPersistResourceCarriesTheDrillOwner covers the CTX-MEM-04 half of the same write:
// a drill-in's pane is a child kind *and* an owner, and PersistResource stores both in
// one file state so the restore can re-enter the scope. A plain table clears the owner.
func TestPersistResourceCarriesTheDrillOwner(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, path := loadState("prod")
	p := &statePersister{path: path, state: state}

	child := config.MenuResource{Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true}
	drill := &config.DrillOwner{
		Resource:  config.MenuResource{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"},
		Namespace: "apps",
		Name:      "web",
	}
	if err := p.PersistResource(child, drill); err != nil {
		t.Fatalf("PersistResource: %v", err)
	}
	reloaded, _ := loadState("prod")
	if reloaded.LastDrillOwner == nil || *reloaded.LastDrillOwner != *drill {
		t.Fatalf("reloaded LastDrillOwner = %+v, want %+v", reloaded.LastDrillOwner, drill)
	}
	if reloaded.LastResource == nil || *reloaded.LastResource != child {
		t.Fatalf("reloaded LastResource = %+v, want the child kind %+v", reloaded.LastResource, child)
	}

	// A plain table overwrites the owner away: the file remembers one place.
	if err := p.PersistResource(child, nil); err != nil {
		t.Fatalf("PersistResource (plain): %v", err)
	}
	reloaded, _ = loadState("prod")
	if reloaded.LastDrillOwner != nil {
		t.Errorf("LastDrillOwner after a plain table = %+v, want nil", reloaded.LastDrillOwner)
	}
}

// TestLoadContextStateCarriesTheRememberedKind proves the switch path resolves the
// third per-context field too (CTX-MEM-02): without it a switch would rebind the menu
// and the namespace but leave the departed context's remembered kind armed, and the
// restore would fire against the wrong cluster.
func TestLoadContextStateCarriesTheRememberedKind(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeStateFile(t, "prod", "lastNamespace: apps\nlastResource:\n  group: example.com\n  version: v1\n  resource: widgets\n  kind: Widget\n")

	st := contextStateLoader{}.LoadContextState("prod")
	if st.LastResource == nil || st.LastResource.Resource != "widgets" {
		t.Fatalf("LastResource = %+v, want the widgets entry", st.LastResource)
	}
	if st.Resourcer == nil {
		t.Error("a resolved state path should wire the resource writer, as it wires the other two")
	}

	// A context with no state file arms nothing and still leaves the writer usable.
	st = contextStateLoader{}.LoadContextState("staging")
	if st.LastResource != nil {
		t.Errorf("LastResource for an unrecorded context = %+v, want nil", st.LastResource)
	}
}

// TestLoadContextStateCarriesTheRememberedDrillOwner proves the switch path carries the
// drill-in owner too (CTX-MEM-04): a remembered drill-in is a kind *plus* an owner, so a
// switch back must arm both halves or it would land on the plain child list of a pane
// that was actually scoped to an owner.
func TestLoadContextStateCarriesTheRememberedDrillOwner(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeStateFile(t, "prod", "lastNamespace: apps\nlastResource:\n  version: v1\n  resource: pods\n  kind: Pod\nlastDrillOwner:\n  resource:\n    group: apps\n    version: v1\n    resource: deployments\n    kind: Deployment\n  namespace: apps\n  name: web\n")

	st := contextStateLoader{}.LoadContextState("prod")
	if st.LastDrillOwner == nil || st.LastDrillOwner.Name != "web" {
		t.Fatalf("LastDrillOwner = %+v, want the apps/web owner", st.LastDrillOwner)
	}
	if st.LastDrillOwner.Resource.Group != "apps" || st.LastDrillOwner.Resource.Resource != "deployments" {
		t.Errorf("LastDrillOwner.Resource = %+v, want apps/deployments", st.LastDrillOwner.Resource)
	}
	if st.LastResource == nil || st.LastResource.Resource != "pods" {
		t.Errorf("the drill-in's child kind must ride alongside its owner, got %+v", st.LastResource)
	}
}

// TestPersistThemeKeepsTheRestOfTheConfig is the leg's real risk (M4-12b-2): SaveFile
// marshals the whole struct, so a write-back that does not load the file first deletes
// everything else in it — a user's entire `keys:` section for the sake of one theme
// name. The write must be load-modify-save.
func TestPersistThemeKeepsTheRestOfTheConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  app.quit: [x]\ntheme: monokai\n"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	p := &configThemePersister{path: path}
	if err := p.PersistTheme("solarized-dark"); err != nil {
		t.Fatalf("PersistTheme: %v", err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("re-read config: %v", err)
	}
	if cfg.Theme != "solarized-dark" {
		t.Errorf("theme = %q, want solarized-dark", cfg.Theme)
	}
	if got := cfg.Keys["app.quit"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("the write-back dropped the user's keys section: %v", cfg.Keys)
	}
}

// TestPersistThemeCreatesAMissingConfig: the common case is a user with no config file
// at all (kubecom runs entirely on defaults), so the first theme pick has to *create*
// it rather than fail — LoadFile treats a missing file as the zero config and SaveFile
// creates the directory.
func TestPersistThemeCreatesAMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	p := &configThemePersister{path: path}
	if err := p.PersistTheme("monokai"); err != nil {
		t.Fatalf("PersistTheme: %v", err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("re-read config: %v", err)
	}
	if cfg.Theme != "monokai" {
		t.Errorf("theme = %q, want monokai", cfg.Theme)
	}
	// It round-trips through resolveTheme, which is the whole point of writing it.
	theme, err := resolveTheme(cfg.Theme)
	if err != nil || theme.Name != "monokai" {
		t.Errorf("resolveTheme(persisted) = %q, %v; want monokai, nil", theme.Name, err)
	}
}

// TestPersistThemeRefusesToClobberAnUnparseableConfig: if the file became invalid
// while kubecom was running, writing would replace it with defaults — losing a
// hand-written keymap to save a color choice. The write fails instead; the shell
// toasts it and keeps the theme for the session (principle 3).
func TestPersistThemeRefusesToClobberAnUnparseableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "keys: [oops\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	p := &configThemePersister{path: path}
	if err := p.PersistTheme("monokai"); err == nil {
		t.Error("an unparseable config should fail the write rather than be overwritten")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != body {
		t.Errorf("the config was rewritten: %q", string(data))
	}
}

// TestLoadContextStateCarriesBothListsForTheSwitchedContext proves a context switch
// lands on the *new* context's authored entries **and** its pins, as two lists: the
// menu folds them in that order (authored wins, D193 pt 3) and `*` needs them apart
// to know which of the two it may unpin (D202). This is the path a pin would
// silently miss if only launch carried it.
func TestLoadContextStateCarriesBothListsForTheSwitchedContext(t *testing.T) {
	const ctx = "staging"
	writeMenuFile(t, ctx, `resources:
  - group: cert-manager.io
    version: v1
    resource: certificates
`)
	writeStateFile(t, ctx, `lastNamespace: apps
pinnedResources:
  - group: hub.traefik.io
    version: v1alpha1
    resource: aigateways
    kind: AIGateway
    namespaced: true
`)
	st := contextStateLoader{}.LoadContextState(ctx)
	if st.Namespace != "apps" {
		t.Errorf("Namespace = %q, want apps", st.Namespace)
	}
	if len(st.MenuExtras) != 1 || st.MenuExtras[0].Resource != "certificates" {
		t.Fatalf("MenuExtras = %+v, want just the authored entry", st.MenuExtras)
	}
	if len(st.Pinned) != 1 || st.Pinned[0].Resource != "aigateways" {
		t.Fatalf("Pinned = %+v, want just the pin", st.Pinned)
	}
	if !st.Pinned[0].Namespaced {
		t.Error("the pin lost its Namespaced flag on the way to the menu")
	}
	if st.Pinner == nil {
		t.Error("the switched-in context should carry the writer its pins go back to")
	}
}

// TestKeyLogPathPrecedence covers the three ways a trace destination is decided
// (STORY-02): the flag wins, the environment stands in when the flag is absent,
// and neither means no trace. The environment fallback exists because a story is
// walked over several launches, and a flag re-typed by hand is a flag forgotten on
// the third launch — which is how a trace ends up with a hole in it exactly where
// the interesting part was.
func TestKeyLogPathPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "off by default"},
		{name: "flag alone", flag: "/tmp/flag.jsonl", want: "/tmp/flag.jsonl"},
		{name: "env alone", env: "/tmp/env.jsonl", want: "/tmp/env.jsonl"},
		{name: "flag beats env", flag: "/tmp/flag.jsonl", env: "/tmp/env.jsonl", want: "/tmp/flag.jsonl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(keyLogEnv, tc.env)
			if got := (runOptions{keyLog: tc.flag}).keyLogPath(); got != tc.want {
				t.Errorf("keyLogPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestKeyLogFlagIsOffByDefault holds the shipped default in place. The recorder is
// a documented feature (D268 pt 2), which makes it exactly the kind of thing that
// could acquire a default value in a later refactor; a trace nobody asked for is a
// file of everything they typed.
func TestKeyLogFlagIsOffByDefault(t *testing.T) {
	f := newRootCmd().Flags().Lookup("keylog")
	if f == nil {
		t.Fatal("--keylog is not registered on the root command")
	}
	if f.DefValue != "" {
		t.Errorf("--keylog defaults to %q, want it off", f.DefValue)
	}
}
