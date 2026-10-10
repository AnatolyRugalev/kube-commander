package keymap

import (
	"errors"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"
	kdl "github.com/calico32/kdl-go"
	"github.com/neuroplastio/hotty-go/hottytea"
	"github.com/neuroplastio/myelin"
	"github.com/neuroplastio/myelin/key"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/keybind"
)

// Keymap is the keymap in force: default.kdl, then the user's `keymap` documents,
// resolved by myelin (D303). It is immutable; the live input state (a sequence in
// flight) is the Sequencer's.
type Keymap struct {
	km *myelin.Keymap
	// warnings are what is wrong with the keymap without refusing it.
	warnings []string
}

// DefaultKeymap is the defaults alone. default.kdl is embedded and tested, so it
// never fails.
func DefaultKeymap() *Keymap { return defaultKeymap() }

var defaultKeymap = sync.OnceValue(func() *Keymap {
	km, err := New()
	if err != nil {
		panic(fmt.Sprintf("keymap: %v", err))
	}
	return km
})

// New builds the keymap from the defaults and the user's documents, in order, each
// binding checked against the registry (CheckBinding). A keymap with an error — a
// binding the registry refuses, a context that does not compile, a `<leader>`
// with no leader named — is refused whole, every error at its file:line.
func New(user ...myelin.Doc) (*Keymap, error) {
	docs := append([]myelin.Doc{DefaultDoc()}, user...)
	km, problems := keybind.Env().Build(docs, check)
	var errs []error
	var warnings []string
	for _, p := range problems {
		text := p.Source + ": " + p.Text
		if p.Warning {
			warnings = append(warnings, text)
		} else {
			errs = append(errs, errors.New(text))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &Keymap{km: km, warnings: warnings}, nil
}

// check is the registry's check of a binding as myelin builds it.
func check(b myelin.Binding) error { return CheckBinding(b.Action, b.Args) }

// UserDocs is the user's keymap from a loaded config: config.kdl's `keymap`, then
// the overlay's — in that order, so a binding kubecom wrote beats the file's line
// for the same keys. Every binding is checked against the registry (CheckBinding).
// Any error in either refuses both: a keymap is used whole or not at all.
func UserDocs(cfg *config.Config) ([]myelin.Doc, error) {
	var docs []myelin.Doc
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
		if err := errors.Join(err, checkDoc(d)); err != nil {
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

// checkDoc is the registry's check of every binding in a document, each failure at
// its file:line.
func checkDoc(d myelin.Doc) error {
	var errs []error
	for _, b := range d.Binds {
		if b.Unbind {
			continue
		}
		if err := CheckBinding(b.Action, b.Args); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Source, err))
		}
	}
	return errors.Join(errs...)
}

// Warnings is what is wrong with the keymap without refusing it, each at its
// file:line — an unbind that does not reach a key it names.
func (k *Keymap) Warnings() []string { return k.warnings }

// Engine is the keymap as myelin built it, for what lists bindings (`kubecom
// keys`).
func (k *Keymap) Engine() *myelin.Keymap { return k.km }

// Match is what a typed sequence finds in a context: the action it runs (none
// when Action is ""), the values the binding gives it, and whether a longer
// binding continues it.
type Match struct {
	Action Action
	Args   []string
	Prefix bool
}

// Match looks a typed sequence up in the context path.
func (k *Keymap) Match(keys []key.Key, p keybind.Path) Match {
	m := k.km.Match(keys, p.Steps())
	out := Match{Prefix: m.Prefix}
	if m.Exact != nil {
		out.Action, out.Args = Action(m.Exact.Action), m.Exact.Args
	}
	return out
}

// Action resolves one live keypress, on its own, in the context path: what a
// surface that never buffers a sequence (a picker, a modal, a text field) does with
// it. ok is false when the key runs nothing there.
func (k *Keymap) Action(press tea.Key, p keybind.Path) (Action, []string, bool) {
	mk, ok := keyOf(press)
	if !ok {
		return "", nil, false
	}
	m := k.Match([]key.Key{mk}, p)
	return m.Action, m.Args, m.Action != ""
}

// Resolve reports the action a whole run of key tokens runs in the context path,
// ok false when it runs none. Tokens are the `--keylog` trace notation, the
// keymap's own (KeyName); the trace analyser asks it whether a run of keys the
// walker pressed formed a complete binding. A token that cannot be read matches
// nothing.
func (k *Keymap) Resolve(p keybind.Path, tokens ...string) (Action, bool) {
	keys := make([]key.Key, 0, len(tokens))
	for _, tok := range tokens {
		mk, err := key.Parse(tok)
		if err != nil {
			return "", false
		}
		keys = append(keys, mk)
	}
	m := k.Match(keys, p)
	return m.Action, m.Action != ""
}

// Keys is the key sequences that run a — with no value — in the context path, as
// display tokens in load order (vim key first): what a hint shows for it there.
// Nil when nothing runs it there.
func (k *Keymap) Keys(a Action, p keybind.Path) []string {
	var out []string
	for _, ks := range k.km.KeysFor(string(a), nil, p.Steps()) {
		out = append(out, key.FormatSeq(ks))
	}
	return out
}

// KeysAnywhere is every key sequence bound to a with no value, in any context and
// whether or not it wins there, as display tokens in load order: what a listing or
// a check over every surface reads.
func (k *Keymap) KeysAnywhere(a Action) []string {
	var out []string
	for _, b := range k.km.All() {
		if !b.Unbind && b.Action == string(a) && len(b.Args) == 0 {
			out = append(out, key.FormatSeq(b.Seq))
		}
	}
	return out
}

// KeyName is a live keypress in the keymap's spelling (`j`, `ctrl+d`, `shift+tab`,
// `G`), what a `--keylog` trace records; a key the keymap has no name for is
// written as Bubble Tea writes it.
func KeyName(k tea.Key) string {
	if mk, ok := keyOf(k); ok {
		return key.Format(mk)
	}
	return k.String()
}

// keyOf is a live keypress as the keymap compares it: its HOTTY name (hottytea),
// so it is the key a HOTTY host or the terminal's bytes give for the same
// keystroke. ok is false for a key with no name there.
func keyOf(k tea.Key) (key.Key, bool) {
	name := hottytea.KeyName(k)
	if name == "" {
		return "", false
	}
	return key.ParseHotty(name)
}
