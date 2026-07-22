// Package keymap is kubecom's configurable action registry: the single place
// where keys exist (D11). Every user-triggerable behaviour is a named Action; a
// default keymap (vim-first, D10) maps each Action to one or more canonical
// chords; the user's config is merged onto those defaults; and views resolve a
// live keypress to an Action instead of matching raw keys. Help overlays and the
// generated keybindings doc are built from this registry so they cannot drift.
//
// This file is the registry core: Action ids, the default keymap, Merge +
// validation, and single-key KeyMsg → Action resolution. The chord model lives
// in chord.go; multi-key sequences (`gg`) and their stateful, timeout-driven
// resolution live in sequence.go/sequencer.go (M2-01b). Deferred to later slices:
// the YAML keys: config wiring (M2-01c) and bubbles/key.Binding + help generation
// (M2-01d).
package keymap

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Action is a stable string id for a user-triggerable behaviour (e.g. nav.down).
// It is the only thing views switch on; they never match a raw key (D11).
type Action string

// Registered actions. This slice covers the navigation scheme of D10 plus the
// core app actions; the action-menu set (describe/yaml/delete/…) is finalised in
// M3 (see vault/knowledge/keybindings.md) and registered there.
const (
	ActionUp           Action = "nav.up"
	ActionDown         Action = "nav.down"
	ActionLeft         Action = "nav.left"
	ActionRight        Action = "nav.right"
	ActionDrillIn      Action = "nav.drillIn"
	ActionBack         Action = "nav.back"
	ActionTop          Action = "nav.top"
	ActionBottom       Action = "nav.bottom"
	ActionHalfPageDown Action = "nav.halfPageDown"
	ActionHalfPageUp   Action = "nav.halfPageUp"
	ActionPageDown     Action = "nav.pageDown"
	ActionPageUp       Action = "nav.pageUp"
	ActionFilter       Action = "app.filter"
	ActionSearchNext   Action = "app.searchNext"
	ActionSearchPrev   Action = "app.searchPrev"
	ActionHelp         Action = "app.help"
	ActionQuit         Action = "app.quit"
	ActionNamespace    Action = "ns.switch"
	ActionToggleMouse  Action = "mouse.toggle"
	ActionSort         Action = "sort.column"
	ActionClearSort    Action = "sort.clear"
)

// actionMeta is the registry: every known Action, in a stable order, with the
// human description used by help/doc generation. Adding an Action here (and to a
// default binding below) is all that is needed to register it.
var actionMeta = []struct {
	id   Action
	desc string
}{
	{ActionUp, "Move up"},
	{ActionDown, "Move down"},
	{ActionLeft, "Focus left pane / collapse"},
	{ActionRight, "Focus right pane / expand"},
	{ActionDrillIn, "Open / drill into selection"},
	{ActionBack, "Go back / up a level"},
	{ActionTop, "Jump to top"},
	{ActionBottom, "Jump to bottom"},
	{ActionHalfPageDown, "Scroll half page down"},
	{ActionHalfPageUp, "Scroll half page up"},
	{ActionPageDown, "Scroll full page down"},
	{ActionPageUp, "Scroll full page up"},
	{ActionFilter, "Filter / search"},
	{ActionSearchNext, "Next match"},
	{ActionSearchPrev, "Previous match"},
	{ActionHelp, "Toggle help"},
	{ActionQuit, "Quit"},
	{ActionNamespace, "Switch namespace"},
	{ActionToggleMouse, "Toggle mouse capture (off = select text to copy)"},
	{ActionSort, "Sort table (cycle column / direction)"},
	{ActionClearSort, "Clear sort (restore order)"},
}

var registered = func() map[Action]string {
	m := make(map[Action]string, len(actionMeta))
	for _, a := range actionMeta {
		m[a.id] = a.desc
	}
	return m
}()

// Valid reports whether a is a registered action id.
func (a Action) Valid() bool { _, ok := registered[a]; return ok }

// Describe returns the human description for a registered action ("" if unknown).
func (a Action) Describe() string { return registered[a] }

// Actions returns every registered action id in registry order.
func Actions() []Action {
	out := make([]Action, len(actionMeta))
	for i, a := range actionMeta {
		out[i] = a.id
	}
	return out
}

// defaultBindings is the vim-first default keymap (D10). Tokens are the
// human-readable syntax of keybindings.md and may be multi-key sequences (`gg`).
// Defaults are collision-free by construction (asserted in tests): pgdn/pgup fall
// back to the half-page actions; the full-page actions keep the ctrl+f/ctrl+b vim
// keys only (pgdn/pgup can't fall back to both half- and full-page in one flat
// context without colliding — D48). `gg` → top is the vim sequence (M2-01b), with
// `home` as the single-key fallback.
var defaultBindings = map[Action][]string{
	ActionUp:           {"k", "up"},
	ActionDown:         {"j", "down"},
	ActionLeft:         {"h", "left"},
	ActionRight:        {"l", "right"},
	ActionDrillIn:      {"enter"},
	ActionBack:         {"esc"},
	ActionTop:          {"gg", "home"},
	ActionBottom:       {"G", "end"},
	ActionHalfPageDown: {"ctrl+d", "pgdn"},
	ActionHalfPageUp:   {"ctrl+u", "pgup"},
	ActionPageDown:     {"ctrl+f"},
	ActionPageUp:       {"ctrl+b"},
	ActionFilter:       {"/"},
	ActionSearchNext:   {"n"},
	ActionSearchPrev:   {"N"},
	ActionHelp:         {"?"},
	ActionQuit:         {"q", "ctrl+c"},
	ActionNamespace:    {"ctrl+n"},
	ActionToggleMouse:  {"M"},
	ActionSort:         {"s"},
	ActionClearSort:    {"S"},
}

// navChords is the set of reserved navigation chords (D10): binding an app
// action over one of these is allowed but warned about during Merge.
var navChords = map[chord]struct{}{
	"h": {}, "j": {}, "k": {}, "l": {},
	"g": {}, "G": {}, "n": {}, "N": {}, "/": {},
	"ctrl+u": {}, "ctrl+d": {}, "ctrl+f": {}, "ctrl+b": {},
}

// Keymap is a resolved, validated mapping of actions to key sequences. Construct
// with DefaultKeymap (optionally then Merge). The reverse indexes are what
// resolution uses; they are rebuilt on every construction so they never drift
// from bindings. bySeq maps a whole sequence to its action; prefix/extends
// support the sequencer (is this a prefix of a binding; does a longer binding
// extend it).
type Keymap struct {
	bindings map[Action][]seq
	bySeq    map[string]Action
	prefix   map[string]bool
	extends  map[string]bool
}

// DefaultKeymap returns the built-in vim-first keymap. It panics if the static
// default table is malformed — a programming error caught by tests, never a
// runtime condition.
func DefaultKeymap() *Keymap {
	bindings := make(map[Action][]seq, len(defaultBindings))
	for _, a := range actionMeta { // deterministic order
		for _, tok := range defaultBindings[a.id] {
			s, err := parseSequence(tok)
			if err != nil {
				panic(fmt.Sprintf("keymap: bad default binding %q for %s: %v", tok, a.id, err))
			}
			bindings[a.id] = append(bindings[a.id], s)
		}
	}
	km, err := build(bindings)
	if err != nil {
		panic("keymap: default keymap is invalid: " + err.Error())
	}
	return km
}

// Merge overlays user overrides onto this keymap and returns a new validated
// keymap. Each entry replaces that action's binding wholesale (an empty list
// disables the action). It returns an error for an unknown action id, a bad key
// token, or a chord bound to two actions; the returned warnings flag overrides
// that shadow a default navigation key (D10).
func (k *Keymap) Merge(overrides map[Action][]string) (*Keymap, []string, error) {
	next := make(map[Action][]seq, len(k.bindings))
	for a, ss := range k.bindings {
		next[a] = append([]seq(nil), ss...)
	}
	// Deterministic iteration over overrides for stable errors/warnings.
	ids := make([]Action, 0, len(overrides))
	for a := range overrides {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var warnings []string
	for _, a := range ids {
		if !a.Valid() {
			return nil, nil, fmt.Errorf("keymap: unknown action %q", a)
		}
		seqs := make([]seq, 0, len(overrides[a]))
		for _, tok := range overrides[a] {
			s, err := parseSequence(tok)
			if err != nil {
				return nil, nil, fmt.Errorf("keymap: action %q: %w", a, err)
			}
			// A single-key override onto a reserved nav key shadows navigation
			// (D10) — allowed, but warned. A multi-key sequence like `gg` does
			// not shadow the bare nav key `g`, so only length-1 seqs warn.
			if len(s) == 1 {
				if _, isNav := navChords[s[0]]; isNav && !isNavAction(a) {
					warnings = append(warnings, fmt.Sprintf(
						"action %q binds navigation key %q", a, s.String()))
				}
			}
			seqs = append(seqs, s)
		}
		next[a] = seqs
	}
	km, err := build(next)
	if err != nil {
		return nil, nil, err
	}
	return km, warnings, nil
}

// build validates bindings and constructs the reverse indexes. Two actions on
// the same sequence is a collision error naming both. Alongside the exact index
// it records, for every proper/whole prefix of every binding, whether that prefix
// is reachable (prefix) and whether a longer binding extends it (extends) — the
// two facts the sequencer needs.
func build(bindings map[Action][]seq) (*Keymap, error) {
	bySeq := make(map[string]Action)
	prefix := make(map[string]bool)
	extends := make(map[string]bool)
	// Deterministic order so a collision reports the same pair every run.
	ids := make([]Action, 0, len(bindings))
	for a := range bindings {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, a := range ids {
		for _, s := range bindings[a] {
			key := s.key()
			if other, ok := bySeq[key]; ok && other != a {
				lo, hi := other, a
				if hi < lo {
					lo, hi = hi, lo
				}
				return nil, fmt.Errorf("keymap: key %q is bound to both %q and %q", s.String(), lo, hi)
			}
			bySeq[key] = a
			for i := 1; i <= len(s); i++ {
				prefix[s[:i].key()] = true
				if i < len(s) {
					extends[s[:i].key()] = true
				}
			}
		}
	}
	return &Keymap{bindings: bindings, bySeq: bySeq, prefix: prefix, extends: extends}, nil
}

// exact returns the action a whole sequence is bound to.
func (k *Keymap) exact(s seq) (Action, bool) { a, ok := k.bySeq[s.key()]; return a, ok }

// isPrefix reports whether s is a prefix of (or equal to) some binding.
func (k *Keymap) isPrefix(s seq) bool { return k.prefix[s.key()] }

// hasExtension reports whether some binding strictly extends s.
func (k *Keymap) hasExtension(s seq) bool { return k.extends[s.key()] }

// Action resolves a single live keypress to a binding whose whole sequence is
// that one key. It ignores multi-key sequences (use a Sequencer for those); the
// bool is false when no single-key binding matches. Views that never buffer
// (modals, pickers) can use this directly.
func (k *Keymap) Action(key tea.Key) (Action, bool) {
	a, ok := k.bySeq[seq{chordFromKey(key)}.key()]
	return a, ok
}

// Keys returns the canonical key tokens bound to an action, for help/doc
// generation. Order follows the action's binding list (vim key first).
func (k *Keymap) Keys(a Action) []string {
	ss := k.bindings[a]
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.String()
	}
	return out
}

func isNavAction(a Action) bool { return strings.HasPrefix(string(a), "nav.") }
