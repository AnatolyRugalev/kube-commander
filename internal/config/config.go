package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"sigs.k8s.io/yaml"
)

// Config is kubecom's plain-YAML user configuration. Only the keys: section is
// wired today (M2-01c); the zero value is a valid config that runs on defaults.
type Config struct {
	// Keys overrides the default keymap: action id -> key tokens (e.g.
	//   keys:
	//     nav.down: [j, down]
	//     app.quit: [q]
	// An entry replaces that action's default binding wholesale; an empty list
	// disables the action. Resolved against the vim-first defaults by Keymap.
	Keys map[string][]string `json:"keys,omitempty"`
}

// Path returns the canonical config file location (D20):
// os.UserConfigDir()/kubecom/config.yaml — ~/.config/kubecom/config.yaml on
// Linux, ~/Library/Application Support/kubecom/config.yaml on macOS. It is kept
// separate from ~/.kube/ (kubeconfig-shaped) and from the discovery cache dir.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locating user config dir: %w", err)
	}
	return filepath.Join(dir, "kubecom", "config.yaml"), nil
}

// Load decodes a config from r. Unknown top-level fields are rejected so a typo
// (or a not-yet-wired section) fails loudly instead of being silently ignored.
func Load(r io.Reader) (*Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("config: reading: %w", err)
	}
	return parse(data)
}

// LoadFile reads the config at path. A missing file is not an error: it returns
// the zero config, since kubecom runs on defaults with no config file present.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}
	return parse(data)
}

func parse(data []byte) (*Config, error) {
	var c Config
	if len(data) > 0 {
		if err := yaml.UnmarshalStrict(data, &c); err != nil {
			return nil, fmt.Errorf("config: parsing YAML: %w", err)
		}
	}
	return &c, nil
}

// Save writes the config as plain YAML to w. It is the write-back counterpart of
// Load: what Save emits, Load reads back into an equal Config (the omitempty json
// tags keep an all-zero config a single "{}\n"). This is the primitive later legs
// use to persist config changes (last namespace, M2-11b) and to write the migrated
// file (M2-12); it does not touch the filesystem itself — see SaveFile.
func (c *Config) Save(w io.Writer) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshalling YAML: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("config: writing: %w", err)
	}
	return nil
}

// SaveFile writes the config to path, creating the parent directory if needed and
// replacing any existing file atomically: it marshals to a temp file in the same
// directory (so os.Rename stays on one filesystem) and renames it over path, so a
// crash mid-write never leaves a truncated config in place. The file is written
// 0o600 and the directory 0o700 — it is user config, kept private like a kubeconfig.
func (c *Config) SaveFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshalling YAML: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("config: creating temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename; a successful rename makes
	// this a no-op (the temp name no longer exists).
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: writing %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: closing %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config: replacing %s: %w", path, err)
	}
	return nil
}

// Keymap resolves the configured key overrides against the built-in vim-first
// defaults (D10/D11). It returns the merged keymap, any non-fatal warnings (e.g.
// an override that shadows a navigation key), and an error for an unknown action
// id, a malformed key token, or a chord bound to two actions.
func (c *Config) Keymap() (*keymap.Keymap, []string, error) {
	overrides := make(map[keymap.Action][]string, len(c.Keys))
	for id, tokens := range c.Keys {
		overrides[keymap.Action(id)] = tokens
	}
	return keymap.DefaultKeymap().Merge(overrides)
}
