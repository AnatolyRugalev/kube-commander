package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// legacyName is the filename of the 2020 kube-commander config: ~/.kubecom.yaml.
const legacyName = ".kubecom.yaml"

// legacyConfig is the subset of the old kube-commander config that migration
// inspects. The old file was a protobuf message (pb.Config) serialized as
// protojson→YAML, so field names are the proto camelCase names. Only the
// user-authored parts we can report on are modeled; the rich theme color/style
// detail is intentionally left out so a lenient parse ignores it rather than
// failing — a legacy file must never block start (principle 3).
type legacyConfig struct {
	Menu         []legacyResource `json:"menu,omitempty"`
	CurrentTheme string           `json:"currentTheme,omitempty"`
	Themes       []legacyTheme    `json:"themes,omitempty"`
}

// legacyResource is one entry of the old `menu` list. It named a kind by group +
// kind only — it never stored the API version or the plural resource name, which
// the v1 per-context menu (MenuResource) needs to address a resource generically.
type legacyResource struct {
	Namespaced bool   `json:"namespaced,omitempty"`
	Group      string `json:"group,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Title      string `json:"title,omitempty"`
}

// legacyTheme captures just a theme's name; the palette/styles are ignored (v1 has
// no runtime theming to migrate them into, D6).
type legacyTheme struct {
	Name string `json:"name,omitempty"`
}

// LegacyPath returns the location of the legacy 2020 kube-commander config file,
// ~/.kubecom.yaml (the old DefaultPath). The one-shot migration reads it on first
// start when no new-format config exists yet (wiring: M2-12b).
func LegacyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: locating home dir: %w", err)
	}
	return filepath.Join(home, legacyName), nil
}

// Migrate reads a legacy ~/.kubecom.yaml (the 2020 kube-commander protobuf-yaml
// config) from r and produces the equivalent new-format Config plus human-readable
// notes describing anything that could not be carried over automatically.
//
// The legacy config held only two user-authored things — a custom resource `menu`
// and color `themes`/`currentTheme` — and neither maps cleanly into kubecom v1, so
// the returned Config carries no fields from it (the legacy format had no
// keybindings, the one thing the new Config models):
//
//   - Menu entries named a kind by group + kind only; the v1 per-context menu
//     (MenuResource) addresses resources by group/version/resource, which the old
//     format never stored and only live discovery can resolve. They are reported,
//     not auto-written — the user re-adds them in a per-context menu file.
//   - v1 uses a single fixed theme (D6); there is no runtime theming to migrate to.
//
// Migration's real value is therefore recognising the legacy file (so the wiring
// can write a fresh new-format config once and not re-run) and telling the user
// what to redo by hand. Parsing is lenient — leftover theme detail is ignored, not
// rejected — so a legacy file never blocks start; only unparseable YAML errors.
// Empty input yields the zero Config and no notes.
func Migrate(r io.Reader) (*Config, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("config: reading legacy config: %w", err)
	}
	var lc legacyConfig
	if len(strings.TrimSpace(string(data))) > 0 {
		// Lenient (non-strict) on purpose: the old file carries theme fields we do
		// not model, and unknown/leftover detail must be ignored, not rejected.
		if err := yaml.Unmarshal(data, &lc); err != nil {
			return nil, nil, fmt.Errorf("config: parsing legacy config: %w", err)
		}
	}

	notes := migrationNotes(&lc)
	// The new Config carries nothing from the legacy file; the zero value is a valid
	// config that runs on defaults. Returning it (rather than nil) is intentional:
	// the wiring writes it to establish the new-format file so migration is one-shot.
	return &Config{}, notes, nil
}

// migrationNotes builds the human-readable report of legacy content that could not
// be migrated automatically.
func migrationNotes(lc *legacyConfig) []string {
	var notes []string
	if n := len(lc.Menu); n > 0 {
		names := make([]string, 0, n)
		for i, r := range lc.Menu {
			names = append(names, legacyResourceName(r, i))
		}
		notes = append(notes, fmt.Sprintf(
			"%d legacy menu %s (%s) were not migrated automatically: the old config "+
				"stored no API version/resource, which the per-context menu now requires. "+
				"Re-add them in a per-context menu file (see README: Per-context menu).",
			n, plural(n, "resource", "resources"), strings.Join(names, ", ")))
	}
	if len(lc.Themes) > 0 || strings.TrimSpace(lc.CurrentTheme) != "" {
		notes = append(notes, "legacy theme configuration was dropped: kubecom v1 "+
			"uses a single fixed theme and has no runtime theming.")
	}
	return notes
}

// legacyResourceName picks the most human-friendly label for a legacy menu entry:
// its Title, else its Kind, else a positional placeholder.
func legacyResourceName(r legacyResource, i int) string {
	if s := strings.TrimSpace(r.Title); s != "" {
		return s
	}
	if s := strings.TrimSpace(r.Kind); s != "" {
		return s
	}
	return fmt.Sprintf("resource %d", i+1)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
