// Package keybind is kubecom's side of myelin, the shared keymap engine (D303,
// gov NEIO-13): kubecom's context path declared to it (path.go) and its keymaps
// read from KDL into myelin's documents (parse.go). myelin does the rest — which
// action a key sequence runs in the context that holds the keyboard, the
// sequencer, the hints — and reads no files.
//
// A keymap is built from layers of documents — kubecom's defaults (default.kdl,
// in internal/tui/keymap) and the user's `keymap` (config.kdl's, then the
// overlay's) — each a list of `bind`s, `unbind`s, `group` labels, `context`
// blocks and a `leader`. A context is a CEL expression over the Path, compiled
// once at load, so a keymap that loads never fails on a key.
//
// The package knows keys, contexts and action names as strings; it does not know
// which actions exist. The registry that does checks each binding when the keymap
// is built, so this package stays free of the TUI and a non-terminal front end can
// feed it.
//
// # What wins
//
// For bindings of the same keys whose contexts hold:
//
//  1. the deepest context wins (how far into the Path the expression reaches);
//  2. at equal depth the layer wins: user over default;
//  3. within a layer, the later line wins.
//
// An exact binding that is also a prefix of a longer one waits only for longer
// bindings at least as deep as itself, and runs when the sequence times out. And a
// text field keeps its text (D303 pt 5): with `field` on the path, a sequence that
// starts with a key that types text goes to the field unless a binding at least as
// deep as the field takes it.
package keybind

import (
	"sync"
	"time"

	"github.com/neuroplastio/myelin"
)

// The layers, lowest first. Higher wins at equal depth.
const (
	// Default is kubecom's default.kdl.
	Default = "default"
	// User is the user's keymap: config.kdl's, then the overlay's.
	User = "user"
)

// SequenceTimeout is how long a pending multi-key sequence waits for its next key
// before giving up on it (vim's `timeoutlen`).
const SequenceTimeout = 500 * time.Millisecond

// Schema is kubecom's path and layers, declared to myelin: every node of nodeTypes
// with its attributes, field the node that takes text.
func Schema() myelin.Schema {
	s := myelin.Schema{
		Layers:          []string{Default, User},
		StrictKeys:      KeyOptions.Strict,
		ShiftedCapitals: KeyOptions.ShiftedCapitals,
		Timeout:         SequenceTimeout,
	}
	for _, n := range nodeTypes {
		s.Nodes = append(s.Nodes, myelin.NodeType{Name: n.name, Attrs: n.attrs, Text: n.name == "field"})
	}
	return s
}

// Env is the one environment every context is compiled in and every keymap built
// with. The schema is kubecom's own and tested, so an error in it is a broken
// build.
func Env() *myelin.Env { return env() }

var env = sync.OnceValue(func() *myelin.Env {
	e, err := myelin.NewEnv(Schema())
	if err != nil {
		panic("keybind: " + err.Error())
	}
	return e
})
