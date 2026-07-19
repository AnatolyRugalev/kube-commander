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
