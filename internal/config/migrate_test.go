package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestMigrateThemesDropped(t *testing.T) {
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
	if len(notes) != 1 || !strings.Contains(notes[0], "theme configuration was dropped") {
		t.Fatalf("notes = %v, want the theme-dropped note", notes)
	}
}

func TestMigrateCurrentThemeWithoutThemesListStillDropped(t *testing.T) {
	_, notes, err := Migrate(strings.NewReader("currentTheme: solarized\n"))
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "theme") {
		t.Errorf("notes = %v, want a theme note", notes)
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
