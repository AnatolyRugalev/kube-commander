// Package config defines kubecom's plain-YAML user configuration and resolves it
// against the built-in defaults. Today it wires the `keys:` section (M2-01c) —
// action→bindings overrides merged onto the vim-first keymap (D10/D11) — via
// Config.Keymap. The browse/menu/theme sections and the one-shot migration from
// the legacy ~/.kubecom.yaml (D6) land in later M2 legs; unknown top-level fields
// are rejected today so those keys surface as errors rather than silent no-ops
// until they are wired.
package config
