package keybind

import "slices"

// Match is what a typed sequence finds in a context.
type Match struct {
	// Exact is the winning binding of exactly these keys, or nil.
	Exact *Binding
	// Prefix is whether a longer binding continues these keys at least as deep as
	// Exact: the caller waits for the next key rather than running Exact, and runs
	// Exact only when the wait times out. A shallower continuation does not hold a
	// deeper exact binding up.
	Prefix bool
}

// Starts reports whether any binding, in any context, begins with k.
func (km *Keymap) Starts(k Key) bool { return len(km.byFirst[k]) > 0 }

// minDepth is the depth a binding of a sequence starting with first must reach in
// the path: a text key is the field's unless a binding at least as deep as the
// field takes it (D303 pt 5).
func minDepth(first Key, p Path) int {
	if d := p.fieldDepth(); d >= 0 && first.Text() {
		return d
	}
	return 0
}

// Match looks a typed sequence up in the context path.
func (km *Keymap) Match(keys []Key, p Path) Match { return km.match(keys, p, nil) }

// match is Match with the path's activations, when the caller has them already
// (nil computes them if a binding needs them).
func (km *Keymap) match(keys []Key, p Path, acts []map[string]any) Match {
	if len(keys) == 0 {
		return Match{}
	}
	var m Match
	floor := minDepth(keys[0], p)
	best := winner{depth: -1}
	prefixDepth := -1
	for _, i := range km.byFirst[keys[0]] {
		b := &km.binds[i]
		if len(b.keys) < len(keys) || !slices.Equal(b.keys[:len(keys)], keys) {
			continue
		}
		if acts == nil {
			acts = p.activations()
		}
		d := b.ctx.depth(acts)
		if d < floor || !km.live(b, d, acts) {
			continue
		}
		if len(b.keys) > len(keys) {
			if !b.unbind {
				prefixDepth = max(prefixDepth, d)
			}
			continue
		}
		if best.beaten(d, b.layer, b.order) {
			best = winner{depth: d, layer: b.layer, order: b.order}
			m.Exact = b.Binding
		}
	}
	m.Prefix = prefixDepth >= 0 && prefixDepth >= best.depth
	if m.Exact != nil && m.Exact.unbind {
		// An unbind won: the keys are bound to nothing here.
		m.Exact = nil
	}
	return m
}

// Effective is every binding that wins its keys in the context path — what typing
// those keys there would run — in load order, with its keys.
func (km *Keymap) Effective(p Path) []Entry {
	var out []Entry
	acts := p.activations()
	for i := range km.binds {
		b := &km.binds[i]
		if b.unbind || b.ctx.depth(acts) < 0 {
			continue
		}
		if m := km.match(b.keys, p, acts); m.Exact == b.Binding {
			out = append(out, Entry{Key: b.keys[len(b.keys)-1], Keys: b.keys, Label: b.Label, Binding: b.Binding})
		}
	}
	return out
}

// KeysFor is every sequence that runs action in the context path, in load order —
// what a hint shows for it there. args nil matches the action's bindings with no
// value; otherwise only those with exactly these values.
func (km *Keymap) KeysFor(action string, args []string, p Path) [][]Key {
	var out [][]Key
	for _, e := range km.Effective(p) {
		if e.Binding.Action == action && slices.Equal(e.Binding.Args, args) {
			out = append(out, e.Keys)
		}
	}
	return out
}

// Entry is one key that can follow a sequence: a group (more keys follow; Label is
// its group's) or a leaf (Binding is the winner).
type Entry struct {
	Key     Key
	Group   bool
	Label   string
	Binding *Binding
	// Keys is the whole sequence (Effective's and Bindings'; Next's leave it nil).
	Keys []Key
}

// Next lists what can follow a typed sequence in the context path, one entry per
// next key, in the order the keymap first binds each. With no keys typed it is
// every first key.
func (km *Keymap) Next(keys []Key, p Path) []Entry {
	acts := p.activations()
	var out []Entry
	at := map[Key]int{}
	best := map[Key]winner{}
	deeper := map[Key]int{} // the deepest longer binding, per key
	for i := range km.binds {
		b := &km.binds[i]
		if len(b.keys) <= len(keys) || !slices.Equal(b.keys[:len(keys)], keys) {
			continue
		}
		d := b.ctx.depth(acts)
		if d < minDepth(b.keys[0], p) || !km.live(b, d, acts) || (b.unbind && len(b.keys) > len(keys)+1) {
			continue
		}
		k := b.keys[len(keys)]
		j, seen := at[k]
		if !seen {
			j = len(out)
			at[k] = j
			out = append(out, Entry{Key: k})
			best[k] = winner{depth: -1}
			deeper[k] = -1
		}
		if len(b.keys) > len(keys)+1 {
			deeper[k] = max(deeper[k], d)
			continue
		}
		if w := best[k]; w.beaten(d, b.layer, b.order) {
			best[k] = winner{depth: d, layer: b.layer, order: b.order}
			out[j].Binding = b.Binding
		}
	}
	kept := out[:0]
	for _, e := range out {
		// A group as Match is a prefix: only where its longer bindings are at least
		// as deep as the key's own.
		e.Group = deeper[e.Key] >= 0 && deeper[e.Key] >= best[e.Key].depth
		switch {
		case e.Group:
			e.Binding = nil
			e.Label = km.groupLabel(append(slices.Clone(keys), e.Key), acts)
		case e.Binding == nil || e.Binding.unbind:
			continue // unbound here
		default:
			e.Label = e.Binding.Label
		}
		kept = append(kept, e)
	}
	return kept
}

// live reports whether a binding holding at depth d is not taken away by an unbind
// of its keys, or of a shorter sequence they continue (`unbind "g"` unbinds the
// group), that beats it there — so a group whose bindings were all unbound is no
// prefix.
func (km *Keymap) live(b *bound, d int, acts []map[string]any) bool {
	if b.unbind {
		return true
	}
	for _, i := range km.unbinds {
		u := &km.binds[i]
		if len(u.keys) > len(b.keys) || !slices.Equal(b.keys[:len(u.keys)], u.keys) {
			continue
		}
		if du := u.ctx.depth(acts); du >= 0 && (winner{depth: d, layer: b.layer, order: b.order}).beaten(du, u.layer, u.order) {
			return false
		}
	}
	return true
}

// groupLabel is the winning label of the group at exactly these keys, or "".
func (km *Keymap) groupLabel(seq []Key, acts []map[string]any) string {
	best := winner{depth: -1}
	label := ""
	for i := range km.groups {
		g := &km.groups[i]
		if !slices.Equal(g.keys, seq) {
			continue
		}
		d := g.ctx.depth(acts)
		if d < 0 {
			continue
		}
		if best.beaten(d, g.layer, g.order) {
			best = winner{depth: d, layer: g.layer, order: g.order}
			label = g.Label
		}
	}
	return label
}

// winner is the precedence of the best candidate so far.
type winner struct {
	depth int
	layer Layer
	order int
}

// beaten reports whether a candidate at (d, l, o) wins over w: deeper, then the
// higher layer, then the later line.
func (w winner) beaten(d int, l Layer, o int) bool {
	switch {
	case d != w.depth:
		return d > w.depth
	case l != w.layer:
		return l > w.layer
	}
	return o > w.order
}
