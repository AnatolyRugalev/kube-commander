package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// MenuConfig is the per-context resource-menu customization: a dynamic set of
// resource types to surface in the browse menu for one kubeconfig context
// (feedback 2026-07-21-02). It lives in its **own per-context file**, separate
// from the main config.yaml, because different clusters expose different CRDs —
// keying the menu on the context keeps each cluster's menu relevant. A context
// with no file falls back to the built-in default (seed + discovery) menu, so the
// zero value is a valid config that adds nothing.
//
// This is the config layer only: it defines the schema, resolves the per-context
// path, and loads/validates the file. Mapping entries into menu rows and merging
// them with the seed/discovery set is a separate slice (config stays free of the
// kube/menu packages so it has no import cycle and stays trivially testable).
type MenuConfig struct {
	// Resources are extra resource types to add to the menu beyond the built-in
	// seed + discovery set. The primary use is naming CRDs the seed doesn't know
	// (e.g. cert-manager Certificates, an Argo Rollout) so they appear in the menu
	// for this context. Order is preserved; a merge slice decides placement.
	Resources []MenuResource `json:"resources,omitempty"`
}

// MenuResource is one resource-type entry in a per-context menu file — the CRD
// entry format. It names a Kubernetes kind by group/version/resource so the kube
// layer can list/watch it generically, plus optional display hints. Example:
//
//	resources:
//	  - group: cert-manager.io
//	    version: v1
//	    resource: certificates
//	    kind: Certificate
//	    namespaced: true
//	    section: Custom Resources
type MenuResource struct {
	// Group is the API group ("" / omitted for the core group).
	Group string `json:"group,omitempty"`
	// Version is the API version (e.g. v1, v1beta1). Required.
	Version string `json:"version"`
	// Resource is the plural resource name used by the API (e.g. certificates).
	// Required — it is what the dynamic client addresses.
	Resource string `json:"resource"`
	// Kind is the display/type kind (e.g. Certificate). Optional; when empty a
	// merge slice derives a title from Resource.
	Kind string `json:"kind,omitempty"`
	// Namespaced marks the resource namespaced (vs cluster-scoped). Optional;
	// defaults to false (cluster-scoped) so an omitted value is explicit.
	Namespaced bool `json:"namespaced,omitempty"`
	// Section is the menu group heading to place the entry under. Optional; a
	// merge slice defaults it to the Custom Resources bucket.
	Section string `json:"section,omitempty"`
	// Title overrides the rendered row label. Optional; defaults to Kind.
	Title string `json:"title,omitempty"`
}

// MenuDir returns the directory holding the per-context menu files:
// os.UserConfigDir()/kubecom/menus — a subdirectory of the same kubecom config
// dir as Path() (D20), kept as its own folder so the per-context files don't
// clutter the top-level config dir.
func MenuDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locating user config dir: %w", err)
	}
	return filepath.Join(dir, "kubecom", "menus"), nil
}

// MenuPath returns the menu-config file path for a kubeconfig context:
// MenuDir()/<sanitized-context>.yaml. The context name is sanitized into a safe
// single path segment (see menuFileName) — kubeconfig context names are arbitrary
// strings that may contain path separators or other characters unsafe in a
// filename. An empty context is an error: a per-context file cannot be resolved
// without a context.
func MenuPath(context string) (string, error) {
	if strings.TrimSpace(context) == "" {
		return "", errors.New("config: empty context; cannot resolve a per-context menu file")
	}
	dir, err := MenuDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, menuFileName(context)), nil
}

// menuFileName maps a kubeconfig context name to a safe single-segment filename
// ending in .yaml. Every character outside [A-Za-z0-9._-] is replaced with '_'
// so path separators (GKE-style names, arbitrary user names) and other unsafe
// characters can never escape MenuDir() or split the path. This is intentionally
// lossy — two contexts differing only in sanitized characters collide onto one
// file — which is the conservative tradeoff (safety over perfect fidelity) for
// the rare arbitrary-character context; the common cases (k3d-foo, gke_p_z_c,
// arn:aws:... becomes arn_aws_...) stay readable.
func menuFileName(context string) string {
	var b strings.Builder
	b.Grow(len(context) + len(".yaml"))
	for _, r := range context {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	b.WriteString(".yaml")
	return b.String()
}

// LoadMenu decodes a MenuConfig from r. Unknown fields are rejected (a typo fails
// loudly instead of silently doing nothing), and every entry is validated
// (version + resource required) so a malformed menu file surfaces an error rather
// than a half-built row. Mirrors Load for the main config.
func LoadMenu(r io.Reader) (*MenuConfig, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("config: reading menu: %w", err)
	}
	return parseMenu(data)
}

// LoadMenuFile reads the per-context menu file at path. A missing file is not an
// error: it returns the zero MenuConfig, since a context with no menu file falls
// back to the default menu (principle 3 — a missing customization degrades to the
// built-in, never blocks).
func LoadMenuFile(path string) (*MenuConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &MenuConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: reading menu %s: %w", path, err)
	}
	return parseMenu(data)
}

func parseMenu(data []byte) (*MenuConfig, error) {
	var m MenuConfig
	if len(data) > 0 {
		if err := yaml.UnmarshalStrict(data, &m); err != nil {
			return nil, fmt.Errorf("config: parsing menu YAML: %w", err)
		}
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// validate rejects entries the kube layer could not address: version and resource
// are the minimum needed to list/watch generically, so a missing one is a hard
// error (loud, like UnmarshalStrict on an unknown field).
func (m *MenuConfig) validate() error {
	for i, r := range m.Resources {
		if strings.TrimSpace(r.Version) == "" {
			return fmt.Errorf("config: menu resources[%d]: version is required", i)
		}
		if strings.TrimSpace(r.Resource) == "" {
			return fmt.Errorf("config: menu resources[%d]: resource is required", i)
		}
	}
	return nil
}
