// Package config reads kubecom's configuration: an authored KDL file with a
// kubecom-written overlay merged on top, and per-context runtime state (D302).
//
// Three files live in the config directory (Dir), each starting `version 1`:
//
//   - config.kdl is the person's. kubecom reads it and never writes it, so its
//     comments, order and layout are theirs.
//   - config.overlay.kdl is kubecom's: every Set writes through to it at once (the
//     theme picker), and it is read merged on top of config.kdl key by key.
//   - state/<context>.kdl is per-context runtime state (State): the last
//     namespace, kind, sort and drill-in, and the pinned kinds (D90).
//
// A setting is a node with one value (`theme "monokai"`); a dotted key is a path
// through nodes. Two top-level nodes are documents, handed to their readers whole:
// `keymap { … }` (the key bindings, D303), which the overlay may hold too, and
// `menu ["<context>"] { resource … }` (extra kinds for a context's menu, Menu).
// The one-shot migration from the 2020 ~/.kubecom.yaml (D6) is Migrate.
package config
