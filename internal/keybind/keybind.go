// Package keybind is kubecom's key bindings engine: which action a key sequence
// runs in the context that holds the keyboard (D303). It is plexos's keymap (its
// D119), ported rather than imported — plexos is private, kubecom is public — with
// kubecom's own path (context.go).
//
// A keymap is built from layers of documents — kubecom's defaults (default.kdl,
// in internal/tui/keymap) and the user's `keymap` (config.kdl's, then the
// overlay's) — each a list of `bind`s, `unbind`s, `group` labels, `context`
// blocks and a `leader`, read from KDL (Parse). A context is a CEL expression over
// the Path, compiled once at load, so a keymap that loads never fails on a key.
//
// The package knows keys, contexts and action names as strings; it does not know
// which actions exist. The registry that does checks a document (Doc.Check), so
// this package stays free of the TUI and a non-terminal front end can feed it.
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
// bindings at least as deep as itself, and runs when the sequence times out
// (Match reports both). And a text field keeps its text (D303 pt 5): with `field`
// on the path, a sequence that starts with a key that types text goes to the field
// unless a binding at least as deep as the field takes it.
package keybind

import (
	"errors"
	"fmt"
	"slices"
)

// Layer is where a binding comes from. Higher wins at equal depth.
type Layer int

const (
	// Default is kubecom's default.kdl.
	Default Layer = iota
	// User is the user's keymap: config.kdl's, then the overlay's.
	User
)

// String names the layer.
func (l Layer) String() string {
	switch l {
	case Default:
		return "default"
	case User:
		return "user"
	}
	return fmt.Sprintf("layer(%d)", int(l))
}

// source is where a declaration was written.
type source struct {
	File string
	Line int
}

// String is `file:line`.
func (s source) String() string {
	if s.File == "" {
		return fmt.Sprintf("line %d", s.Line)
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// Binding binds a key sequence to an action, or — an unbind — to nothing.
//
// Args nil is a binding with no value: an argument verb opens its palette stage,
// any other action runs. A value runs the argument verb with it (D303 pt 2);
// which actions take one is the registry's to check (Doc.Check).
type Binding struct {
	seq    sequence
	Action string
	Args   []string
	// Label is shown in a hint instead of the action's description. Display only.
	Label  string
	ctx    *Context
	unbind bool
	layer  Layer
	src    source
}

// Unbind reports whether the binding binds its keys to nothing.
func (b *Binding) Unbind() bool { return b.unbind }

// Context is the binding's context expression as written ("" for everywhere).
func (b *Binding) Context() string { return b.ctx.String() }

// Layer is the binding's layer.
func (b *Binding) Layer() Layer { return b.layer }

// Source is where the binding was written: `file:line`.
func (b *Binding) Source() string { return b.src.String() }

// group labels a prefix: the row a hint shows for it (`+goto`). Display only.
type group struct {
	seq   sequence
	Label string
	ctx   *Context
	layer Layer
	src   source
}

// Doc is one layer's keymap as read: in the order written.
type Doc struct {
	layer Layer
	// leader is the leader this document names, or nil.
	leader *Key
	binds  []Binding
	groups []group
	// noBase is `base "none"`: the default layer is left out.
	noBase bool
}

// Check runs check over every binding of the document — the registry's test that
// its action exists and takes the values given — and returns the failures joined,
// each at its file:line.
func (d Doc) Check(check func(action string, args []string) error) error {
	var errs []error
	for i := range d.binds {
		b := &d.binds[i]
		if b.unbind {
			continue
		}
		if err := check(b.Action, b.Args); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.src, err))
		}
	}
	return errors.Join(errs...)
}

// Problem is one thing wrong with a keymap, at its source: an error, which refuses
// the keymap, or a warning, which is said and loaded.
type Problem struct {
	Warning bool
	Text    string
}

// Error is the problem as a sentence with its file:line.
func (p Problem) Error() string { return p.Text }

// Keymap is the effective table: every layer's bindings with the leader resolved,
// in load order. Immutable once built.
type Keymap struct {
	leader    Key
	hasLeader bool
	problems  []Problem
	binds     []bound
	groups    []boundGroup
	// byFirst indexes binds by their first key, so a key no sequence starts with
	// costs one map lookup and builds no context at all.
	byFirst map[Key][]int
	// unbinds indexes the unbinds in binds (live).
	unbinds []int
}

// bound is a binding with its keys resolved.
type bound struct {
	*Binding
	keys  []Key
	order int
}

// boundGroup is a group with its keys resolved.
type boundGroup struct {
	*group
	keys  []Key
	order int
}

// Build resolves the documents into one keymap, in the order given (the order a
// listing shows them in; precedence is the layer's, not the position's). A
// binding that names `<leader>` when no document names a leader is left out and
// reported (Problems).
func Build(docs ...Doc) *Keymap {
	km := &Keymap{byFirst: map[Key][]int{}}
	leaderLayer := Layer(-1)
	for _, d := range docs {
		if d.leader != nil && d.layer >= leaderLayer {
			km.leader, km.hasLeader, leaderLayer = *d.leader, true, d.layer
		}
	}
	noBase := slices.ContainsFunc(docs, func(d Doc) bool { return d.noBase })
	order := 0
	for _, d := range docs {
		if noBase && d.layer == Default {
			continue
		}
		for i := range d.binds {
			b := &d.binds[i]
			keys, ok := km.resolve(b.seq, b.src)
			if !ok {
				continue
			}
			km.byFirst[keys[0]] = append(km.byFirst[keys[0]], len(km.binds))
			if b.unbind {
				km.unbinds = append(km.unbinds, len(km.binds))
			}
			km.binds = append(km.binds, bound{Binding: b, keys: keys, order: order})
			order++
		}
		for i := range d.groups {
			g := &d.groups[i]
			keys, ok := km.resolve(g.seq, g.src)
			if !ok {
				continue
			}
			km.groups = append(km.groups, boundGroup{group: g, keys: keys, order: order})
			order++
		}
	}
	km.warnShallowUnbinds()
	return km
}

// warnShallowUnbinds warns of an unbind written outside any context whose keys a
// lower layer binds inside one: the bound context reaches deeper wherever it holds,
// so it wins there and the unbind does not take that key away. The fix is to unbind
// it in that context, which the warning says.
func (km *Keymap) warnShallowUnbinds() {
	for _, i := range km.unbinds {
		u := &km.binds[i]
		if u.ctx != nil {
			continue
		}
		for j := range km.binds {
			b := &km.binds[j]
			if b.unbind || b.ctx == nil || b.layer >= u.layer ||
				len(b.keys) < len(u.keys) || !slices.Equal(b.keys[:len(u.keys)], u.keys) {
				continue
			}
			km.problems = append(km.problems, Problem{Warning: true, Text: fmt.Sprintf(
				"%s: unbind %q leaves %s → %s in context %q (%s), which wins there; unbind it in that context",
				u.src, FormatKeys(u.keys), FormatKeys(b.keys), b.Action, b.ctx, b.src)})
		}
	}
}

// resolve is a sequence's keys with the leader put in; false, and a problem, when
// it names a leader and there is none.
func (km *Keymap) resolve(s sequence, src source) ([]Key, bool) {
	out := make([]Key, len(s))
	for i, st := range s {
		if !st.leader {
			out[i] = st.key
			continue
		}
		if !km.hasLeader {
			km.problems = append(km.problems, Problem{Text: fmt.Sprintf(
				"%s: %s names %s, and no `leader` is set", src, s, leaderToken)})
			return nil, false
		}
		out[i] = km.leader
	}
	return out, true
}

// Problems are what was found wrong building the keymap: errors (any one means the
// keymap should not be put in force) and warnings.
func (km *Keymap) Problems() []Problem { return km.problems }

// Err is the keymap's errors, joined — nil when it may be put in force (warnings
// aside).
func (km *Keymap) Err() error {
	var errs []error
	for _, p := range km.problems {
		if !p.Warning {
			errs = append(errs, p)
		}
	}
	return errors.Join(errs...)
}

// Leader is the effective leader key, and false when no document names one.
func (km *Keymap) Leader() (Key, bool) { return km.leader, km.hasLeader }

// Bindings is every binding and unbind, leader resolved, in load order — what a
// listing prints.
func (km *Keymap) Bindings() []Entry {
	out := make([]Entry, len(km.binds))
	for i := range km.binds {
		b := &km.binds[i]
		out[i] = Entry{Key: b.keys[len(b.keys)-1], Keys: b.keys, Label: b.Label, Binding: b.Binding}
	}
	return out
}
