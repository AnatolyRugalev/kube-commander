// Package config defines kubecom's plain-YAML user configuration and resolves it
// against the built-in defaults. It wires the `keys:` section (M2-01c) —
// action→bindings overrides merged onto the vim-first keymap (D10/D11) — via
// Config.Keymap, and the per-context resource-menu customization (MenuConfig, a
// separate file per kubeconfig context; see menu.go) that lets a user surface
// CRDs and other resource types the built-in menu doesn't seed (feedback
// 2026-07-21-02). Both loaders reject unknown fields so a typo surfaces as an
// error rather than a silent no-op. The one-shot migration from the legacy
// ~/.kubecom.yaml (D6) is provided by Migrate (see migrate.go): it recognises the
// old protobuf-yaml file, carries the `currentTheme` selection onto `theme:` where
// v1 still ships that palette (M5-04), and reports the rest — the menu, and any
// hand-authored theme palette — as content with no automatic home in v1. The
// launcher wiring that runs it on first start is cmd/kubecom's maybeMigrate.
package config
