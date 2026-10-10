package keymap

import (
	"errors"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"
	kdl "github.com/calico32/kdl-go"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/keybind"
)

// Keymap is the keymap in force: default.kdl, then the user's `keymap` documents,
// resolved by the context engine (D303). It is immutable; the live input state (a
// sequence in flight) is the Sequencer's.
type Keymap struct {
	km *keybind.Keymap
}

// DefaultKeymap is the defaults alone. default.kdl is embedded and tested, so it
// never fails.
func DefaultKeymap() *Keymap { return defaultKeymap() }

var defaultKeymap = sync.OnceValue(func() *Keymap {
	return &Keymap{km: keybind.Build(DefaultDoc())}
})

// New builds the keymap from the defaults and the user's documents, in order. A
// keymap with an error — a `<leader>` with no leader named — is refused whole:
// the error names the file and line.
func New(user ...keybind.Doc) (*Keymap, error) {
	km := keybind.Build(append([]keybind.Doc{DefaultDoc()}, user...)...)
	if err := km.Err(); err != nil {
		return nil, err
	}
	return &Keymap{km: km}, nil
}

// UserDocs is the user's keymap from a loaded config: config.kdl's `keymap`, then
// the overlay's — in that order, so a binding kubecom wrote beats the file's line
// for the same keys. Every binding is checked against the registry (CheckBinding).
// Any error in either refuses both: a keymap is used whole or not at all.
func UserDocs(cfg *config.Config) ([]keybind.Doc, error) {
	var docs []keybind.Doc
	var errs []error
	for _, f := range []struct {
		name string
		read func() ([]*kdl.Node, bool)
	}{{config.AuthoredName, cfg.Keymap}, {config.OverlayName, cfg.OverlayKeymap}} {
		nodes, ok := f.read()
		if !ok {
			continue
		}
		// A document that fails to parse is still checked as far as it was read, so
		// one run of `kubecom keys check` names every mistake it can.
		d, err := keybind.Parse(f.name, nodes, keybind.User)
		if err := errors.Join(err, d.Check(CheckBinding)); err != nil {
			errs = append(errs, err)
			continue
		}
		docs = append(docs, d)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("keymap: %w", errors.Join(errs...))
	}
	return docs, nil
}

// Warnings is what is wrong with the keymap without refusing it, each at its
// file:line — an unbind that does not reach a key it names.
func (k *Keymap) Warnings() []string {
	var out []string
	for _, p := range k.km.Problems() {
		if p.Warning {
			out = append(out, p.Text)
		}
	}
	return out
}

// Engine is the keymap's engine, for what lists bindings (`kubecom keys`).
func (k *Keymap) Engine() *keybind.Keymap { return k.km }

// Match is what a typed sequence finds in a context: the action it runs (none
// when Action is ""), the values the binding gives it, and whether a longer
// binding continues it.
type Match struct {
	Action Action
	Args   []string
	Prefix bool
}

// Match looks a typed sequence up in the context path.
func (k *Keymap) Match(keys []keybind.Key, p keybind.Path) Match {
	m := k.km.Match(keys, p)
	out := Match{Prefix: m.Prefix}
	if m.Exact != nil {
		out.Action, out.Args = Action(m.Exact.Action), m.Exact.Args
	}
	return out
}

// Action resolves one live keypress, on its own, in the context path: what a
// surface that never buffers a sequence (a picker, a modal, a text field) does with
// it. ok is false when the key runs nothing there.
func (k *Keymap) Action(key tea.Key, p keybind.Path) (Action, []string, bool) {
	m := k.Match([]keybind.Key{KeyOf(key)}, p)
	return m.Action, m.Args, m.Action != ""
}

// Resolve reports the action a whole run of key tokens runs in the context path,
// ok false when it runs none. Tokens are the `--keylog` trace notation (`j`,
// `ctrl+s`, `g`); the trace analyser asks it whether a run of keys the walker
// pressed formed a complete binding. A token that cannot be read matches nothing.
func (k *Keymap) Resolve(p keybind.Path, tokens ...string) (Action, bool) {
	keys := make([]keybind.Key, 0, len(tokens))
	for _, tok := range tokens {
		key, err := keybind.ParseKey(tok)
		if err != nil {
			return "", false
		}
		keys = append(keys, key)
	}
	m := k.Match(keys, p)
	return m.Action, m.Action != ""
}

// Keys is the key sequences that run a — with no value — in the context path, as
// display tokens in load order (vim key first): what a hint shows for it there.
// Nil when nothing runs it there.
func (k *Keymap) Keys(a Action, p keybind.Path) []string {
	var out []string
	for _, ks := range k.km.KeysFor(string(a), nil, p) {
		out = append(out, keybind.FormatKeys(ks))
	}
	return out
}

// KeysAnywhere is every key sequence bound to a with no value, in any context and
// whether or not it wins there, as display tokens in load order: what a listing or
// a check over every surface reads.
func (k *Keymap) KeysAnywhere(a Action) []string {
	var out []string
	for _, e := range k.km.Bindings() {
		if b := e.Binding; !b.Unbind() && b.Action == string(a) && len(b.Args) == 0 {
			out = append(out, keybind.FormatKeys(e.Keys))
		}
	}
	return out
}

// KeyOf is a live keypress as the keymap compares it. Shift folds into the
// character for a printable key (the terminal reports `G`, or `g` plus shift), and
// ctrl folds case (`ctrl+D` is `ctrl+d`); a named key keeps its shift (shift+tab).
func KeyOf(k tea.Key) keybind.Key {
	ctrl := k.Mod&tea.ModCtrl != 0
	alt := k.Mod&tea.ModAlt != 0
	if name, ok := namedCodes[k.Code]; ok {
		return keybind.Key{Ctrl: ctrl, Alt: alt, Shift: k.Mod&tea.ModShift != 0, Name: name}
	}
	r := k.Code
	if !ctrl {
		if k.ShiftedCode != 0 {
			r = k.ShiftedCode
		} else if t := []rune(k.Text); len(t) == 1 {
			r = t[0]
		}
	} else if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	return keybind.Key{Ctrl: ctrl, Alt: alt, Name: string(r)}
}

// namedCodes are the tea key codes the keymap writes by name.
var namedCodes = map[rune]string{
	tea.KeyUp: "up", tea.KeyDown: "down", tea.KeyLeft: "left", tea.KeyRight: "right",
	tea.KeyEnter: "enter", tea.KeyEsc: "esc", tea.KeyTab: "tab", tea.KeySpace: "space",
	tea.KeyHome: "home", tea.KeyEnd: "end", tea.KeyPgUp: "pgup", tea.KeyPgDown: "pgdn",
	tea.KeyBackspace: "backspace", tea.KeyDelete: "delete", tea.KeyInsert: "insert",
	tea.KeyF1: "f1", tea.KeyF2: "f2", tea.KeyF3: "f3", tea.KeyF4: "f4", tea.KeyF5: "f5", tea.KeyF6: "f6",
	tea.KeyF7: "f7", tea.KeyF8: "f8", tea.KeyF9: "f9", tea.KeyF10: "f10", tea.KeyF11: "f11", tea.KeyF12: "f12",
}
