// Package vault holds the drift guards for the rewrite vault — the Markdown
// working memory under `vault/` that every leg reads on Orient (see
// `vault/README.md`).
//
// It carries no production code and is never imported. It exists because
// `make check` is the only gate this project has (D17): a rule about a vault
// file that is written down but not executed is a convention, and the two
// conventions guarded here have already drifted back once each after being
// fixed by hand. The same reasoning put the release-artifact guards in
// internal/version (the .goreleaser.yml changelog filters, the ldflags, the
// Docker labels) and the keybindings-doc guard in internal/tui/keymap: a
// repository file that a human or an agent must keep true is checked by a test,
// not by remembering.
//
// Files are read relative to this package (`../../vault/...`), the same way
// internal/version reads `../../.goreleaser.yml`.
package vault
