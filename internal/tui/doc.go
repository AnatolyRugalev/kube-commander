// Package tui is kubecom's Bubble Tea user interface: the root model (app.go), the
// action registry / configurable keymap (keymap/), the styles, components, and
// resource views. The root Model routes every keypress through the keymap's
// Sequencer into an Action — no view matches a raw key (D11) — and embeds the
// toggleable help overlay. The app shell is built up across M2-07a..d: today it is
// the two-pane browse layout (resource menu | resource table over the status bar,
// with focus switching between panes); the live watch wiring and discovery
// reconcile follow.
package tui
