// Package keymap is kubecom's configurable action registry: the single place
// where keys exist (D11). Every user-triggerable behaviour is a named Action; a
// default keymap (vim-first, D10) maps each Action to one or more canonical
// chords; the user's config is merged onto those defaults; and views resolve a
// live keypress to an Action instead of matching raw keys. Help overlays and the
// generated keybindings doc are built from this registry so they cannot drift.
//
// This file is the M2-01a core: Action ids, the chord model (parse human tokens
// ↔ derive from a tea.Key), the default keymap, Merge + validation, and
// KeyMsg → Action resolution. Deferred to later slices: multi-key sequences
// (gg), the YAML keys: config wiring, and bubbles/key.Binding + help generation.
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
// human-readable syntax of keybindings.md. Defaults are collision-free by
// construction (asserted in tests): pgdn/pgup fall back to the half-page
// actions; the full-page actions keep the ctrl+f/ctrl+b vim keys only. gg → top
// is a multi-key sequence deferred to M2-01b, so top falls back to home here.
var defaultBindings = map[Action][]string{
	ActionUp:           {"k", "up"},
	ActionDown:         {"j", "down"},
	ActionLeft:         {"h", "left"},
	ActionRight:        {"l", "right"},
	ActionDrillIn:      {"enter"},
	ActionBack:         {"esc"},
	ActionTop:          {"home"},
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
}

// navChords is the set of reserved navigation chords (D10): binding an app
// action over one of these is allowed but warned about during Merge.
var navChords = map[chord]struct{}{
	"h": {}, "j": {}, "k": {}, "l": {},
	"g": {}, "G": {}, "n": {}, "N": {}, "/": {},
	"ctrl+u": {}, "ctrl+d": {}, "ctrl+f": {}, "ctrl+b": {},
}

// Keymap is a resolved, validated mapping of actions to chords. Construct with
// DefaultKeymap (optionally then Merge). The reverse index is what resolution
// uses; it is rebuilt on every construction so it never drifts from bindings.
type Keymap struct {
	bindings map[Action][]chord
	byChord  map[chord]Action
}

// DefaultKeymap returns the built-in vim-first keymap. It panics if the static
// default table is malformed — a programming error caught by tests, never a
// runtime condition.
func DefaultKeymap() *Keymap {
	bindings := make(map[Action][]chord, len(defaultBindings))
	for _, a := range actionMeta { // deterministic order
		for _, tok := range defaultBindings[a.id] {
			c, err := parseChord(tok)
			if err != nil {
				panic(fmt.Sprintf("keymap: bad default binding %q for %s: %v", tok, a.id, err))
			}
			bindings[a.id] = append(bindings[a.id], c)
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
	next := make(map[Action][]chord, len(k.bindings))
	for a, cs := range k.bindings {
		next[a] = append([]chord(nil), cs...)
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
		chords := make([]chord, 0, len(overrides[a]))
		for _, tok := range overrides[a] {
			c, err := parseChord(tok)
			if err != nil {
				return nil, nil, fmt.Errorf("keymap: action %q: %w", a, err)
			}
			if _, isNav := navChords[c]; isNav && !isNavAction(a) {
				warnings = append(warnings, fmt.Sprintf(
					"action %q binds navigation key %q", a, c.String()))
			}
			chords = append(chords, c)
		}
		next[a] = chords
	}
	km, err := build(next)
	if err != nil {
		return nil, nil, err
	}
	return km, warnings, nil
}

// build validates bindings and constructs the reverse index. Two actions on the
// same chord is a collision error naming both.
func build(bindings map[Action][]chord) (*Keymap, error) {
	byChord := make(map[chord]Action)
	// Deterministic order so a collision reports the same pair every run.
	ids := make([]Action, 0, len(bindings))
	for a := range bindings {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, a := range ids {
		for _, c := range bindings[a] {
			if other, ok := byChord[c]; ok && other != a {
				lo, hi := other, a
				if hi < lo {
					lo, hi = hi, lo
				}
				return nil, fmt.Errorf("keymap: key %q is bound to both %q and %q", c.String(), lo, hi)
			}
			byChord[c] = a
		}
	}
	return &Keymap{bindings: bindings, byChord: byChord}, nil
}

// Action resolves a live keypress to a bound action. The bool is false when no
// action is bound to that key.
func (k *Keymap) Action(key tea.Key) (Action, bool) {
	a, ok := k.byChord[chordFromKey(key)]
	return a, ok
}

// Keys returns the canonical chord tokens bound to an action, for help/doc
// generation. Order follows the action's binding list (vim key first).
func (k *Keymap) Keys(a Action) []string {
	cs := k.bindings[a]
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.String()
	}
	return out
}

func isNavAction(a Action) bool { return strings.HasPrefix(string(a), "nav.") }
