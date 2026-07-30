package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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

// legacyTheme captures just a theme's name; the palette/styles (colors[].rgb|xterm,
// styles[].bg/fg/attrs[]) are ignored. v1 selects a *named built-in* theme (D169),
// so a name can be carried over but a hand-authored palette cannot — there is no
// field to put it in, and inventing one is not this migration's job.
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
// and color `themes`/`currentTheme`. The menu cannot be carried over; the theme
// *selection* can, and is (the legacy format had no keybindings, the other thing
// the new Config models):
//
//   - Menu entries named a kind by group + kind only; the v1 per-context menu
//     (MenuResource) addresses resources by group/version/resource, which the old
//     format never stored and only live discovery can resolve. They are reported,
//     not auto-written — the user re-adds them in a per-context menu file.
//   - `currentTheme` is mapped onto Config.Theme when it names a palette v1 still
//     ships (M5-04): v1 has three built-in themes, a `theme:` field and a picker
//     (D169/D170/D172), so a 2020 `monokai` user keeps monokai. A name with no v1
//     port is reported with the names that do exist. Hand-authored palettes under
//     `themes:` are never migrated — v1 themes are built-in, and a named selection
//     is not a custom palette.
//
// Migration's other value is recognising the legacy file (so the wiring can write a
// fresh new-format config once and not re-run) and telling the user what to redo by
// hand. Parsing is lenient — leftover theme detail is ignored, not rejected — so a
// legacy file never blocks start; only unparseable YAML errors. Empty input yields
// the zero Config and no notes.
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

	// The zero Config is a valid config that runs on defaults; anything the legacy
	// file *can* set is set on it below. Returning it (rather than nil) is
	// intentional: the wiring writes it to establish the new-format file so
	// migration is one-shot.
	cfg := &Config{}
	var notes []string
	if n := legacyMenuNote(&lc); n != "" {
		notes = append(notes, n)
	}
	theme, themeNote := migrateTheme(&lc)
	cfg.Theme = theme
	if themeNote != "" {
		notes = append(notes, themeNote)
	}
	return cfg, notes, nil
}

// legacyMenuNote reports the legacy `menu` entries, which cannot be migrated
// automatically. Empty when the legacy file had no menu.
func legacyMenuNote(lc *legacyConfig) string {
	n := len(lc.Menu)
	if n == 0 {
		return ""
	}
	names := make([]string, 0, n)
	for i, r := range lc.Menu {
		names = append(names, legacyResourceName(r, i))
	}
	return fmt.Sprintf(
		"%d legacy menu %s (%s) %s not migrated automatically: the old config "+
			"stored no API version/resource, which the per-context menu now requires. "+
			"Re-add them in a per-context menu file (see README: Per-context menu).",
		n, plural(n, "resource", "resources"), strings.Join(names, ", "),
		plural(n, "was", "were"))
}

// legacyThemeAliases maps a 2020 built-in theme name onto the v1 built-in that is
// the same palette under a different name. It is an enumerated table of known
// renames, not fuzzy matching — D169 pt 2 still holds: the 2020 build shipped
// `solarized` (the dark variant: base03 background) and v1 named its port
// `solarized-dark` so a light variant could land beside it without a rename.
//
// The other 2020 built-ins — `base16` (the legacy default), `paraiso`, `twilight` —
// have no v1 port and are deliberately absent: a miss that names the themes that do
// exist is honest, and picking the "closest" palette for the user would be a guess.
var legacyThemeAliases = map[string]string{
	"solarized": "solarized-dark",
}

// themePaletteCaveat is appended whenever the legacy file defined palettes of its
// own: those colors/styles are not migratable at all, whatever happened to the
// selection, and the user should not be left thinking a carried-over name restored
// their hand-tuned colors.
const themePaletteCaveat = " Legacy theme palettes (`themes:` colors/styles) were " +
	"not migrated: v1's themes are built-in, and a named selection is not a custom palette."

// migrateTheme carries the legacy `currentTheme` selection onto the new `theme:`
// field, returning the resolved built-in name (empty = leave the field unset, i.e.
// the default theme) and a note describing what happened. Resolution is
// styles.ByName — lenient about case and surrounding space, never fuzzy (D169 pt 2)
// — after one enumerated legacy-rename lookup (legacyThemeAliases).
//
// A legacy file with no `currentTheme` and no `themes:` is silent: there was nothing
// themed to report. An empty `currentTheme` with palettes present is *not* treated
// as a selection even though the 2020 build defaulted it to `base16` — v1 has no
// base16 port, so its own default is the honest outcome.
func migrateTheme(lc *legacyConfig) (string, string) {
	current := strings.TrimSpace(lc.CurrentTheme)
	caveat := ""
	if len(lc.Themes) > 0 {
		caveat = themePaletteCaveat
	}
	if current == "" {
		if caveat == "" {
			return "", "" // nothing themed in the legacy file at all.
		}
		return "", fmt.Sprintf(
			"%d legacy theme %s %s not migrated: v1 selects one of its built-in themes "+
				"(%s) with `theme:` in the new config.",
			len(lc.Themes), plural(len(lc.Themes), "definition", "definitions"),
			plural(len(lc.Themes), "was", "were"),
			strings.Join(styles.ThemeNames(), ", "))
	}

	name := current
	if alias, ok := legacyThemeAliases[strings.ToLower(name)]; ok {
		name = alias
	}
	t, ok := styles.ByName(name)
	if !ok {
		return "", fmt.Sprintf(
			"legacy theme %q has no built-in equivalent in kubecom v1: the built-in "+
				"themes are %s. Set `theme:` in the new config to pick one.%s",
			current, strings.Join(styles.ThemeNames(), ", "), caveat)
	}
	return t.Name, fmt.Sprintf(
		"legacy theme %q was carried over as `theme: %s` in the new config.%s",
		current, t.Name, caveat)
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
