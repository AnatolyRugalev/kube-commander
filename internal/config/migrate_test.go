package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
	"sigs.k8s.io/yaml"
)

func TestMigrateEmptyYieldsZeroConfigNoNotes(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Migrate(empty): %v", err)
	}
	if cfg == nil || len(cfg.Keys) != 0 {
		t.Errorf("cfg = %+v, want zero config", cfg)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}
}

func TestMigrateWhitespaceOnlyYieldsZeroConfig(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader("   \n\t\n"))
	if err != nil {
		t.Fatalf("Migrate(whitespace): %v", err)
	}
	if cfg == nil || len(cfg.Keys) != 0 {
		t.Errorf("cfg = %+v, want zero config", cfg)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}
}

func TestMigrateMenuReportsResources(t *testing.T) {
	// protojson→YAML camelCase field names, as the old Save emitted them.
	legacy := "menu:\n" +
		"  - namespaced: true\n    group: apps\n    kind: Deployment\n    title: Deployments\n" +
		"  - kind: Node\n" + // no title → falls back to kind
		"  - namespaced: true\n    kind: Service\n    title: Services\n"
	cfg, notes, err := Migrate(strings.NewReader(legacy))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(cfg.Keys) != 0 {
		t.Errorf("cfg carries no legacy fields; Keys = %v", cfg.Keys)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the menu note", notes)
	}
	note := notes[0]
	for _, want := range []string{"3 legacy menu resources", "Deployments", "Node", "Services", "per-context menu"} {
		if !strings.Contains(note, want) {
			t.Errorf("menu note %q missing %q", note, want)
		}
	}
}

func TestMigrateSingleMenuResourceIsSingular(t *testing.T) {
	_, notes, err := Migrate(strings.NewReader("menu:\n  - kind: Pod\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "1 legacy menu resource ") {
		t.Errorf("notes = %v, want singular 'resource'", notes)
	}
}

// TestMigrateCurrentThemeCarriedOnto proves the headline of M5-04: a 2020 user whose
// currentTheme names a palette v1 still ships keeps it, on the new `theme:` field,
// and is told so — the note must no longer claim the choice was dropped.
func TestMigrateCurrentThemeCarriedOnto(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader("currentTheme: monokai\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if cfg.Theme != "monokai" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "monokai")
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the theme note", notes)
	}
	for _, want := range []string{"monokai", "carried over"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("theme note %q missing %q", notes[0], want)
		}
	}
	if strings.Contains(notes[0], "dropped") {
		t.Errorf("theme note %q still claims the theme was dropped", notes[0])
	}
}

// TestMigrateCurrentThemeIsLenientOnCaseAndSpace pins the styles.ByName contract
// through the migration path: the legacy value is hand-editable YAML.
func TestMigrateCurrentThemeIsLenientOnCaseAndSpace(t *testing.T) {
	cfg, _, err := Migrate(strings.NewReader("currentTheme: \"  MonoKai \"\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if cfg.Theme != "monokai" {
		t.Errorf("Theme = %q, want %q (lenient on case/space)", cfg.Theme, "monokai")
	}
}

// TestMigrateLegacySolarizedAliasesToSolarizedDark covers the one enumerated legacy
// rename: the 2020 build's built-in was named `solarized` and v1 named the same
// (dark) palette `solarized-dark`, so a plain ByName lookup would miss it.
func TestMigrateLegacySolarizedAliasesToSolarizedDark(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader("currentTheme: solarized\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if cfg.Theme != "solarized-dark" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "solarized-dark")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "solarized-dark") {
		t.Fatalf("notes = %v, want the note naming the v1 theme", notes)
	}
}

// TestMigrateUnportedLegacyThemeNamesTheBuiltins covers the 2020 built-ins with no
// v1 port (and any custom name): the selection cannot be honored, so the note must
// name what does exist rather than guessing the closest palette (D169 pt 2).
func TestMigrateUnportedLegacyThemeNamesTheBuiltins(t *testing.T) {
	for _, name := range []string{"base16", "paraiso", "twilight"} {
		cfg, notes, err := Migrate(strings.NewReader("currentTheme: " + name + "\n"))
		if err != nil {
			t.Fatalf("Migrate(%s): %v", name, err)
		}
		if cfg.Theme != "" {
			t.Errorf("Theme = %q for unported %q, want unset (the default)", cfg.Theme, name)
		}
		if len(notes) != 1 {
			t.Fatalf("notes = %v for %q, want exactly the theme note", notes, name)
		}
		for _, want := range []string{name, "no built-in equivalent", "default", "monokai", "solarized-dark"} {
			if !strings.Contains(notes[0], want) {
				t.Errorf("theme note %q for %q missing %q", notes[0], name, want)
			}
		}
	}
}

// TestMigrateCustomPaletteReportedButNotMigrated pins the boundary M5-04 must not
// cross: a hand-authored palette is not migratable, whatever happens to the name.
func TestMigrateCustomPaletteReportedButNotMigrated(t *testing.T) {
	// `rgb` carries a bare hex string with no leading `#` — that is what
	// theme.ColorToProto wrote (`fmt.Sprintf("%06x", …)`) and ProtoToColor read back
	// (prepending the `#` itself). See testdata/legacy-kubecom.yaml and D180.
	legacy := "currentTheme: dark\n" +
		"themes:\n  - name: dark\n    colors:\n      - name: bg\n        rgb: 002b36\n" +
		"    styles:\n      - name: statusBar\n        bg: bg\n        fg: fg\n        attrs: [BOLD]\n"
	cfg, notes, err := Migrate(strings.NewReader(legacy))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(cfg.Keys) != 0 {
		t.Errorf("Keys = %v, want empty", cfg.Keys)
	}
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want unset — `dark` is not a v1 built-in", cfg.Theme)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the theme note", notes)
	}
	for _, want := range []string{"no built-in equivalent", "palettes"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("theme note %q missing %q", notes[0], want)
		}
	}
}

// TestMigrateCarriedThemeStillReportsCustomPalettes proves the palette caveat rides
// along even on the happy path: overriding monokai's colors in the legacy file and
// selecting it means the *name* survives and the colors do not.
func TestMigrateCarriedThemeStillReportsCustomPalettes(t *testing.T) {
	legacy := "currentTheme: monokai\nthemes:\n  - name: monokai\n    colors:\n      - name: bg\n        xterm: 16\n"
	cfg, notes, err := Migrate(strings.NewReader(legacy))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if cfg.Theme != "monokai" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "monokai")
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the theme note", notes)
	}
	if !strings.Contains(notes[0], "carried over") || !strings.Contains(notes[0], "palettes") {
		t.Errorf("theme note %q must report both the carried name and the dropped palette", notes[0])
	}
}

// TestMigratePalettesWithoutSelectionReportsDefinitions covers `themes:` with no
// `currentTheme`: the 2020 build defaulted that to base16, which v1 has no port of,
// so the honest outcome is v1's own default plus a note about the definitions.
func TestMigratePalettesWithoutSelectionReportsDefinitions(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader("themes:\n  - name: a\n  - name: b\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want unset", cfg.Theme)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the theme note", notes)
	}
	for _, want := range []string{"2 legacy theme definitions were not migrated", "monokai"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("theme note %q missing %q", notes[0], want)
		}
	}
}

func TestMigrateBothMenuAndThemesReportBoth(t *testing.T) {
	legacy := "menu:\n  - kind: Pod\n    title: Pods\ncurrentTheme: dark\nthemes:\n  - name: dark\n"
	_, notes, err := Migrate(strings.NewReader(legacy))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %v, want two (menu + theme)", notes)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "menu") || !strings.Contains(joined, "theme") {
		t.Errorf("notes %v missing menu or theme mention", notes)
	}
}

// TestMigrateThemeNoteNamesEveryBuiltIn keeps the "no equivalent" note honest as the
// registry grows: a fourth built-in must appear in it without anyone remembering to
// edit a hardcoded list here or in migrate.go.
func TestMigrateThemeNoteNamesEveryBuiltIn(t *testing.T) {
	_, notes, err := Migrate(strings.NewReader("currentTheme: nosuchtheme\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want exactly the theme note", notes)
	}
	for _, name := range styles.ThemeNames() {
		if !strings.Contains(notes[0], name) {
			t.Errorf("theme note %q does not name built-in %q", notes[0], name)
		}
	}
}

// TestMigrateAliasedLegacyThemesAllResolve guards the alias table itself: every
// legacy name it claims to rename must actually resolve to a built-in, so a theme
// rename in styles/ cannot leave a dangling alias that silently degrades.
func TestMigrateAliasedLegacyThemesAllResolve(t *testing.T) {
	for legacy, v1 := range legacyThemeAliases {
		if _, ok := styles.ByName(v1); !ok {
			t.Errorf("alias %q -> %q: %q is not a built-in theme", legacy, v1, v1)
		}
		if _, ok := styles.ByName(legacy); ok {
			t.Errorf("alias %q is redundant: it already resolves directly", legacy)
		}
	}
}

func TestMigrateMalformedYAMLErrors(t *testing.T) {
	_, _, err := Migrate(strings.NewReader("menu: [oops\n"))
	if err == nil {
		t.Fatal("Migrate(malformed): expected error, got nil")
	}
}

func TestMigrateIgnoresUnknownLegacyFields(t *testing.T) {
	// A legacy field kubecom does not model (or a future proto addition) must be
	// ignored, not rejected — migration is lenient so a legacy file never blocks.
	_, notes, err := Migrate(strings.NewReader("menu:\n  - kind: Pod\nsomethingElse: 42\n"))
	if err != nil {
		t.Fatalf("Migrate(unknown field): %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v, want just the menu note", notes)
	}
}

func TestLegacyPathEndsWithLegacyName(t *testing.T) {
	p, err := LegacyPath()
	if err != nil {
		t.Fatalf("LegacyPath: %v", err)
	}
	if filepath.Base(p) != ".kubecom.yaml" {
		t.Errorf("LegacyPath base = %q, want .kubecom.yaml", filepath.Base(p))
	}
	home, err := os.UserHomeDir()
	if err == nil && filepath.Dir(p) != home {
		t.Errorf("LegacyPath dir = %q, want home %q", filepath.Dir(p), home)
	}
}

// ---------------------------------------------------------------------------
// M5-05: the schema-derived legacy fixture.
//
// Every test above feeds Migrate hand-written YAML, which is exactly the weak spot
// M5-05 exists to close: the legacy file was a protobuf message (pb.Config) written
// protojson→YAML by master:config/config.go, so its real shape is fixed by
// master:pb/config.proto and not by what a test author remembered. The tests below
// run the real path over testdata/legacy-kubecom.yaml, which was *generated* by that
// writer (see testdata/README.md), and guard it against the schema itself.
// ---------------------------------------------------------------------------

const (
	legacyFixtureYAML  = "testdata/legacy-kubecom.yaml"
	legacyFixtureProto = "testdata/legacy-config.proto"
)

// legacyFixture returns the generated legacy config fixture.
func legacyFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(legacyFixtureYAML)
	if err != nil {
		t.Fatalf("read %s: %v", legacyFixtureYAML, err)
	}
	return string(data)
}

// protoFieldLine matches a proto field declaration — `[repeated] <type> <name> = <n>;`
// — and nothing else in the file: enum values (`NONE = 0;`) have no type token, and
// `option go_package = "…";` has no numeric tag.
var protoFieldLine = regexp.MustCompile(`(?m)^\s*(?:repeated\s+)?[A-Za-z0-9_.]+\s+([a-zA-Z_][A-Za-z0-9_]*)\s*=\s*\d+\s*;`)

// TestLegacyFixtureCoversTheProtoSchema is the guard that makes the fixture evidence
// rather than another guess: every key in the YAML must be a field declared in
// master:pb/config.proto, and every field in the proto must appear in the YAML. A
// fixture with an invented key would prove nothing about a real file, and one that
// omits a field leaves part of the legacy shape untested.
func TestLegacyFixtureCoversTheProtoSchema(t *testing.T) {
	protoSrc, err := os.ReadFile(legacyFixtureProto)
	if err != nil {
		t.Fatalf("read %s: %v", legacyFixtureProto, err)
	}
	declared := map[string]bool{}
	for _, m := range protoFieldLine.FindAllStringSubmatch(string(protoSrc), -1) {
		name := m[1]
		// protojson emits lowerCamelCase JSON names. Every field in this schema is
		// already lowerCamel, so field name == JSON key; a snake_case addition would
		// break that equality and this comparison, which is the point.
		if strings.Contains(name, "_") {
			t.Errorf("proto field %q is snake_case: its JSON key differs from its field name, "+
				"so this test's name-for-name comparison no longer holds", name)
		}
		declared[name] = true
	}
	if len(declared) == 0 {
		t.Fatalf("parsed no fields out of %s — the regex or the schema moved", legacyFixtureProto)
	}

	var doc interface{}
	if err := yaml.Unmarshal([]byte(legacyFixture(t)), &doc); err != nil {
		t.Fatalf("parse %s: %v", legacyFixtureYAML, err)
	}
	used := map[string]bool{}
	collectKeys(doc, used)

	for k := range used {
		if !declared[k] {
			t.Errorf("fixture key %q is not a field in %s — the fixture is not a real legacy file", k, legacyFixtureProto)
		}
	}
	for k := range declared {
		if !used[k] {
			t.Errorf("proto field %q never appears in %s — that part of the legacy shape is untested", k, legacyFixtureYAML)
		}
	}
}

// collectKeys walks a decoded YAML document collecting every mapping key.
func collectKeys(v interface{}, into map[string]bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, sub := range t {
			into[k] = true
			collectKeys(sub, into)
		}
	case []interface{}:
		for _, sub := range t {
			collectKeys(sub, into)
		}
	}
}

// TestMigrateSchemaFixtureCarriesThemeAndReportsMenu runs Migrate over the generated
// fixture: exit criterion 4's "verify against a real legacy config file", minus the
// one half only a human can supply (see the M5-05 human task). The fixture selects
// `solarized`, so this also exercises the D179 alias against a real file rather than
// a one-line string.
func TestMigrateSchemaFixtureCarriesThemeAndReportsMenu(t *testing.T) {
	cfg, notes, err := Migrate(strings.NewReader(legacyFixture(t)))
	if err != nil {
		t.Fatalf("Migrate(fixture): %v", err)
	}
	if cfg.Theme != "solarized-dark" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "solarized-dark")
	}
	if len(cfg.Keys) != 0 {
		t.Errorf("Keys = %v, want empty — the legacy format had no keybindings", cfg.Keys)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %v, want two (menu + theme)", notes)
	}
	// The fixture's menu is what resourceMenu.saveItems wrote: titled entries, one
	// with no title (falling back to its kind), namespaced and cluster-scoped.
	for _, want := range []string{"4 legacy menu resources", "Deployments", "Certificates", "Nodes", "Pod"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("menu note %q missing %q", notes[0], want)
		}
	}
	for _, want := range []string{"solarized-dark", "carried over", "palettes"} {
		if !strings.Contains(notes[1], want) {
			t.Errorf("theme note %q missing %q", notes[1], want)
		}
	}
}

// TestMigrateSchemaFixtureIgnoresThePaletteTree pins the leniency the criterion asks
// about (principle 3): the whole themes[].colors/styles tree — rgb strings, xterm
// numbers, enum-name attrs — is parsed and dropped, changing nothing about the
// resulting Config. Proved by difference: stripping the tree from the fixture must
// yield the same Config, and the only note that moves is the palette caveat.
func TestMigrateSchemaFixtureIgnoresThePaletteTree(t *testing.T) {
	full := legacyFixture(t)
	// Guard against this test going vacuous if the fixture is ever regenerated
	// without palette detail.
	for _, must := range []string{"rgb: 002b36", "xterm: 240", "UNDERLINE"} {
		if !strings.Contains(full, must) {
			t.Fatalf("fixture no longer carries palette detail (%q): this test proves nothing", must)
		}
	}
	// `themes:` sorts last among the top-level keys (yaml.v2 orders map keys), so
	// cutting there removes the whole tree and nothing else.
	i := strings.Index(full, "themes:\n")
	if i < 0 {
		t.Fatalf("fixture has no themes: block")
	}
	stripped := full[:i]

	withPalette, notesFull, err := Migrate(strings.NewReader(full))
	if err != nil {
		t.Fatalf("Migrate(fixture): %v", err)
	}
	withoutPalette, notesStripped, err := Migrate(strings.NewReader(stripped))
	if err != nil {
		t.Fatalf("Migrate(fixture without palettes): %v", err)
	}
	if !reflect.DeepEqual(withPalette, withoutPalette) {
		t.Errorf("palette tree changed the migrated config: %+v vs %+v", *withPalette, *withoutPalette)
	}
	if len(notesFull) != len(notesStripped) {
		t.Fatalf("notes count differs: %v vs %v", notesFull, notesStripped)
	}
	if !strings.Contains(notesFull[1], "palettes") {
		t.Errorf("theme note %q should report the dropped palettes", notesFull[1])
	}
	if strings.Contains(notesStripped[1], "palettes") {
		t.Errorf("theme note %q should not mention palettes when the file defined none", notesStripped[1])
	}
}

// TestMigrateSchemaFixtureThemeMapping runs every 2020 built-in through the real
// file's shape: monokai carries by name, solarized carries through the D179 alias,
// and the three with no v1 port land on v1's default. `currentTheme` is the only line
// substituted, so the palette tree and menu stay exactly as the writer emitted them.
func TestMigrateSchemaFixtureThemeMapping(t *testing.T) {
	full := legacyFixture(t)
	for _, tc := range []struct {
		legacy string
		want   string
	}{
		{"monokai", "monokai"},
		{"solarized", "solarized-dark"},
		{"base16", ""}, // the 2020 default; no v1 port (D179 pt 2/3).
		{"paraiso", ""},
		{"twilight", ""},
	} {
		src := strings.Replace(full, "currentTheme: solarized\n", "currentTheme: "+tc.legacy+"\n", 1)
		if src == full && tc.legacy != "solarized" {
			t.Fatalf("could not substitute currentTheme for %q — the fixture's shape moved", tc.legacy)
		}
		cfg, notes, err := Migrate(strings.NewReader(src))
		if err != nil {
			t.Fatalf("Migrate(%s): %v", tc.legacy, err)
		}
		if cfg.Theme != tc.want {
			t.Errorf("currentTheme %q → Theme %q, want %q", tc.legacy, cfg.Theme, tc.want)
		}
		if len(notes) != 2 {
			t.Fatalf("notes for %q = %v, want two", tc.legacy, notes)
		}
		if !strings.Contains(notes[1], "palettes") {
			t.Errorf("theme note %q for %q must still report the dropped palettes", notes[1], tc.legacy)
		}
	}
}
