package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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
	legacy := "currentTheme: dark\n" +
		"themes:\n  - name: dark\n    colors:\n      - name: bg\n        rgb: \"#000000\"\n" +
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
