package keybind

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func load(t *testing.T, layer Layer, src string) Doc {
	t.Helper()
	d, err := ParseFile("test.kdl", []byte("keymap {\n"+src+"\n}\n"), layer)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func keys(t *testing.T, s string) []Key {
	t.Helper()
	ks, err := ParseKeys(s)
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func exact(km *Keymap, t *testing.T, seq string, p Path) string {
	t.Helper()
	m := km.Match(keys(t, seq), p)
	if m.Exact == nil {
		return ""
	}
	return m.Exact.Action
}

func TestKeysRoundTrip(t *testing.T) {
	for in, want := range map[string]string{
		"ctrl+d": "ctrl+d", "ctrl+D": "ctrl+d", "G": "G", "shift+g": "G", "shift+tab": "shift+tab",
		"left": "left", "ESC": "esc", "Enter": "enter", "space": "space", " ": "space", "+": "+",
		"ctrl++": "ctrl++", "alt+]": "alt+]", "ctrl+alt+x": "alt+ctrl+x", "alt+H": "alt+H", "#": "#",
		"pgdn": "pgdn", "f12": "f12", "×": "×",
	} {
		k, err := ParseKey(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := k.String(); got != want {
			t.Errorf("%q writes as %q, want %q", in, got, want)
		}
		back, err := ParseKey(k.String())
		if err != nil || back != k {
			t.Errorf("%q does not read back: %+v, %v", k, back, err)
		}
	}
	for bad, why := range map[string]string{
		"": "not a key", "ctrl+": "not a key", "ctrl+bb": "not a key", "ctrl+x+y": "modifiers", "f13": "separated by spaces",
		"gg": "separated by spaces", "escape": "separated by spaces", "shift+1": "write the character",
		"ctrl+shift+d": "terminals send", "<lead>": "not a key",
	} {
		if _, err := ParseKey(bad); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("%q: %v, want an error saying %q", bad, err, why)
		}
	}
	seq, err := parseSeq("<leader> g  shift+tab")
	if err != nil || seq.String() != "<leader> g shift+tab" {
		t.Fatalf("a sequence round-trips as %q, %v", seq, err)
	}
}

// Text is what the field rule asks: does the key type something.
func TestTextKeys(t *testing.T) {
	for k, want := range map[string]bool{
		"j": true, "G": true, "space": true, "#": true, "×": true,
		"ctrl+a": false, "alt+x": false, "enter": false, "esc": false, "up": false, "shift+tab": false,
	} {
		if got := keys(t, k)[0].Text(); got != want {
			t.Errorf("%s: Text() = %v, want %v", k, got, want)
		}
	}
}

// Each kind of mistake in a context is a load error rather than a keystroke that
// silently does nothing.
func TestContextsCompileOrFailAtLoad(t *testing.T) {
	for _, ok := range []string{
		`table.kind == 'Pod'`,
		`cluster.context.startsWith('prod-') && !confirm`,
		`picker && picker.stage == 'ns.switch'`,
		`logs.follow || field`,
		`focus == 'table'`,
		`viewer.content == 'secret'`,
		`search.all_kinds && table.unhealthy_only`,
	} {
		if _, err := CompileContext(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for src, why := range map[string]string{
		`table.kindd == 'x'`:      "no such attribute",
		`pane.id == 'x'`:          "no such node",
		`table.namespaced == 'x'`: "a type error",
		`table.kind`:              "not a boolean",
		`table.kind == 'x' &&`:    "a syntax error",
	} {
		if _, err := CompileContext(src); err == nil {
			t.Errorf("%s compiled; it is %s", src, why)
		}
	}
}

// The depth is how far into the path an expression reaches: a view's context is
// deeper than its table's, saying what is not there adds none, and focus reaches
// exactly as far as the node it names.
func TestDepthIsHowFarTheContextReaches(t *testing.T) {
	path := Path{
		Cluster: &Cluster{Context: "prod-eu", Namespace: "default"},
		Table:   &Table{Resource: "pods", Kind: "Pod", Namespaced: true},
		Logs:    &Logs{Follow: true},
	}
	acts := path.activations()
	for src, want := range map[string]int{
		`cluster.context == 'prod-eu'`:  1,
		`table.kind == 'Pod'`:           2,
		`logs.follow`:                   3,
		`focus == 'logs'`:               3,
		`focus == 'table'`:              -1, // the logs view holds the keys, not the table
		`!confirm`:                      0,
		`table.kind == 'Deployment'`:    -1,
		`viewer.content == 'secret'`:    -1, // no viewer: absent, does not hold
		`!(viewer.content == 'secret')`: -1, // and its negation does not either
	} {
		c, err := CompileContext(src)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.depth(acts); got != want {
			t.Errorf("%s: depth %d, want %d", src, got, want)
		}
	}
	var none *Context
	if none.depth(acts) != 0 {
		t.Error("no context is not depth 0")
	}
	if path.Innermost() != "logs" || (Path{}).Innermost() != "" {
		t.Errorf("innermost = %q", path.Innermost())
	}
}

// Precedence, in the decided order: deepest context, then the layer (user over
// default), then the later line.
func TestPrecedence(t *testing.T) {
	def := load(t, Default, `
		bind "a" "default.shallow"
		context "logs" { bind "a" "default.deep" }
		bind "b" "default.first"
		bind "b" "default.second"
	`)
	user := load(t, User, `
		bind "a" "user.shallow"
	`)
	km := Build(def, user)
	inLogs := Path{Table: &Table{}, Logs: &Logs{}}
	inTable := Path{Table: &Table{}}
	for _, tc := range []struct {
		seq  string
		path Path
		want string
	}{
		{"a", inLogs, "default.deep"},    // depth beats the user's layer
		{"a", inTable, "user.shallow"},   // equal depth: user beats default
		{"b", inTable, "default.second"}, // same layer: the later line
	} {
		if got := exact(km, t, tc.seq, tc.path); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.seq, got, tc.want)
		}
	}
}

// An exact match that is also a prefix waits: Match says both, and the caller runs
// the exact one only when the wait times out.
func TestExactAndPrefix(t *testing.T) {
	km := Build(load(t, Default, `
		bind "g" "go.short"
		bind "g g" "nav.top"
		context "logs" { bind "g x" "go.logs" }
	`))
	m := km.Match(keys(t, "g"), Path{})
	if m.Exact == nil || m.Exact.Action != "go.short" || !m.Prefix {
		t.Fatalf("g: %+v", m)
	}
	if m := km.Match(keys(t, "g g"), Path{}); m.Exact == nil || m.Prefix {
		t.Fatalf("g g: %+v", m)
	}
	// A continuation whose context does not hold is no prefix.
	if m := km.Match(keys(t, "g x"), Path{}); m.Exact != nil || m.Prefix {
		t.Fatalf("g x outside the logs: %+v", m)
	}
	if !km.Starts(keys(t, "g")[0]) || km.Starts(keys(t, "x")[0]) {
		t.Fatal("Starts does not index the first keys")
	}
}

// A shallower continuation does not hold a deeper exact binding up: `enter` on
// the table's own keys runs at once though `enter …` sequences exist everywhere.
func TestAShallowerPrefixDoesNotWait(t *testing.T) {
	km := Build(load(t, Default, `
		bind "enter x" "long"
		context "focus == 'table'" { bind "enter" "actions.menu" }
	`))
	m := km.Match(keys(t, "enter"), Path{Table: &Table{}})
	if m.Exact == nil || m.Exact.Action != "actions.menu" || m.Prefix {
		t.Fatalf("enter: %+v", m)
	}
}

// focus is the innermost node: a binding in `focus == 'table'` is the table's own
// and does not reach an overlay opened over it, while one in `table` does.
func TestFocusIsTheInnermostNode(t *testing.T) {
	km := Build(load(t, Default, `
		bind "enter" "nav.drillIn"
		context "focus == 'table'" { bind "enter" "actions.menu" }
		context "table" { bind "x" "table.anywhere" }
		context "confirm" { bind "enter" "confirm.accept" }
	`))
	table := Path{Cluster: &Cluster{}, Table: &Table{}}
	forwards := Path{Cluster: &Cluster{}, Table: &Table{}, Forwards: true}
	confirm := Path{Cluster: &Cluster{}, Table: &Table{}, Confirm: true}
	for _, tc := range []struct {
		seq  string
		path Path
		want string
	}{
		{"enter", table, "actions.menu"},
		{"enter", forwards, "nav.drillIn"},
		{"enter", confirm, "confirm.accept"},
		{"x", forwards, "table.anywhere"},
		{"enter", Path{Cluster: &Cluster{}, Menu: true}, "nav.drillIn"},
	} {
		if got := exact(km, t, tc.seq, tc.path); got != tc.want {
			t.Errorf("%s in %s: %s, want %s", tc.seq, tc.path.Innermost(), got, tc.want)
		}
	}
}

// A path writes its nodes outside in, each with the attributes that are set, and
// two different paths never write the same.
func TestPathString(t *testing.T) {
	p := Path{
		Cluster: &Cluster{Context: "kind-dev"},
		Table:   &Table{Resource: "pods", Kind: "Pod", Namespaced: true},
		Picker:  &Picker{ID: "palette"},
		Field:   true,
	}
	want := `cluster(context="kind-dev") > table(kind="Pod", namespaced, resource="pods") > picker(id="palette") > field`
	if got := p.String(); got != want {
		t.Errorf("String() = %s\nwant       %s", got, want)
	}
	if got := (Path{}).String(); got != "" {
		t.Errorf("the empty path writes %q", got)
	}
	q := p
	q.Table = &Table{Resource: "pods", Kind: "Pod", Namespaced: true, Filtered: true}
	if p.String() == q.String() {
		t.Error("a filtered table writes as an unfiltered one")
	}
}

// A text field keeps its text (D303 pt 5): a key that types goes to the field
// unless a binding at least as deep as the field takes it; a key that does not
// type resolves as anywhere.
func TestFieldsKeepTheirText(t *testing.T) {
	km := Build(load(t, Default, `
		bind "j" "nav.down"
		bind "down" "nav.down"
		bind "g g" "nav.top"
		bind "ctrl+a" "search.allKinds"
		context "picker" { bind "x" "picker.x" }
		context "field" { bind "y" "field.y" }
	`), load(t, User, `bind "q" "user.q"`))
	field := Path{Table: &Table{}, Picker: &Picker{ID: "palette"}, Field: true}
	for _, tc := range []struct{ seq, want string }{
		{"j", ""},                     // text: the field's
		{"q", ""},                     // a user's global binding cannot steal it either
		{"x", ""},                     // nor a context shallower than the field
		{"y", "field.y"},              // one at the field's depth can
		{"down", "nav.down"},          // no text: resolves
		{"ctrl+a", "search.allKinds"}, // a ctrl chord types nothing
	} {
		if got := exact(km, t, tc.seq, field); got != tc.want {
			t.Errorf("%s in a field: %q, want %q", tc.seq, got, tc.want)
		}
	}
	if m := km.Match(keys(t, "g"), field); m.Prefix {
		t.Error("g still starts a sequence in a field")
	}
	noField := Path{Table: &Table{}, Picker: &Picker{ID: "palette"}}
	if exact(km, t, "j", noField) != "nav.down" || exact(km, t, "x", noField) != "picker.x" {
		t.Error("with the field closed the text keys do not resolve")
	}
	var next []string
	for _, e := range km.Next(nil, field) {
		next = append(next, e.Key.String())
	}
	if got := strings.Join(next, " "); got != "down ctrl+a y" {
		t.Errorf("next in a field = %s", got)
	}
}

// unbind binds keys to nothing where it wins; unbinding a prefix unbinds the
// group under it.
func TestUnbind(t *testing.T) {
	def := load(t, Default, `
		bind "D" "res.delete"
		bind "g g" "nav.top"
		bind "g r" "res.relations"
	`)
	user := load(t, User, `
		context "cluster.context.startsWith('prod-')" { unbind "D" }
		unbind "g"
	`)
	km := Build(def, user)
	prod := Path{Cluster: &Cluster{Context: "prod-eu"}, Table: &Table{}}
	dev := Path{Cluster: &Cluster{Context: "kind"}, Table: &Table{}}
	if exact(km, t, "D", prod) != "" || exact(km, t, "D", dev) != "res.delete" {
		t.Error("the prod-only unbind did not hold where, and only where, it says")
	}
	if m := km.Match(keys(t, "g"), dev); m.Prefix || m.Exact != nil {
		t.Errorf("g is still a prefix: %+v", m)
	}
	if exact(km, t, "g g", dev) != "" {
		t.Error("g g survived its group's unbind")
	}
}

// The leader is an alias the highest layer that names one sets; with none named,
// a `<leader>` binding is a problem and is left out.
func TestTheLeaderIsAnAlias(t *testing.T) {
	def := load(t, Default, `bind "<leader> n" "ns.switch"`)
	km := Build(def, load(t, User, `leader "space"`))
	if l, ok := km.Leader(); !ok || l.String() != "space" {
		t.Fatalf("leader = %v, %v", l, ok)
	}
	if exact(km, t, "space n", Path{}) != "ns.switch" {
		t.Fatal("<leader> n did not follow the leader to space")
	}
	km = Build(def)
	if km.Err() == nil || !strings.Contains(km.Err().Error(), "test.kdl:2: <leader> n names <leader>") {
		t.Fatalf("no leader: %v", km.Err())
	}
	if len(km.Bindings()) != 0 {
		t.Fatal("the leaderless binding was kept")
	}
}

// base "none" leaves the defaults out.
func TestBaseNone(t *testing.T) {
	km := Build(load(t, Default, `bind "j" "nav.down"`), load(t, User, `base "none"`+"\n"+`bind "x" "app.quit"`))
	if exact(km, t, "j", Path{}) != "" || exact(km, t, "x", Path{}) != "app.quit" {
		t.Fatal("base none kept the defaults")
	}
}

// Next lists one entry per following key, in the order the keymap first binds it;
// a key with longer bindings behind it is a group with its label.
func TestNextIsTheHint(t *testing.T) {
	km := Build(load(t, Default, `
		group "g" "+goto"
		bind "g g" "nav.top"
		bind "g r" "res.relations" label="relations"
		bind "G" "nav.bottom"
		context "logs" { bind "g l" "logs.only" }
	`))
	var got []string
	for _, e := range km.Next(nil, Path{}) {
		if e.Group {
			got = append(got, e.Key.String()+"="+e.Label)
		} else {
			got = append(got, e.Key.String()+"="+e.Binding.Action)
		}
	}
	if s := strings.Join(got, " "); s != "g=+goto G=nav.bottom" {
		t.Fatalf("next = %s", s)
	}
	got = nil
	for _, e := range km.Next(keys(t, "g"), Path{}) {
		got = append(got, e.Key.String()+"="+e.Label)
	}
	if s := strings.Join(got, " "); s != "g= r=relations" {
		t.Fatalf("next after g = %s", s)
	}
}

// KeysFor is what a hint shows for an action in a context: the sequences that win
// there.
func TestKeysFor(t *testing.T) {
	km := Build(load(t, Default, `
		bind "k" "nav.up"
		bind "up" "nav.up"
		bind "g p" "resources.switch" "pods"
		bind "R" "resources.switch"
	`), load(t, User, `unbind "up"`))
	var got []string
	for _, ks := range km.KeysFor("nav.up", nil, Path{}) {
		got = append(got, FormatKeys(ks))
	}
	if s := strings.Join(got, ","); s != "k" {
		t.Fatalf("nav.up = %s", s)
	}
	if ks := km.KeysFor("resources.switch", []string{"pods"}, Path{}); len(ks) != 1 || FormatKeys(ks[0]) != "g p" {
		t.Fatalf("resources.switch pods = %v", ks)
	}
}

// Every error names its line, and one bad line refuses the document.
func TestParseErrorsNameTheLine(t *testing.T) {
	for src, want := range map[string]string{
		`bind "q"`:                          `test.kdl:3: bind takes at least 2 arguments`,
		`bind "ctrl+" "x"`:                  `test.kdl:3: bind: "ctrl+" is not a key`,
		`bind "gg" "nav.top"`:               "separated by spaces: `g g`",
		`bind "a" "x" colour="red"`:         `test.kdl:3: bind has no property "colour"`,
		`bind "a" "x" label=#true`:          `test.kdl:3: bind's label is the wrong kind of value`,
		`chord "a"`:                         `test.kdl:3: "chord" is not a keymap node`,
		`context "table.kindd == 1" { }`:    `test.kdl:3: context "table.kindd == 1"`,
		`context "logs" { leader "space" }`: `test.kdl:3: leader cannot be inside a context`,
		`leader "space" "tab"`:              `test.kdl:3: leader takes one argument`,
		`base "vim"`:                        `test.kdl:3: base is "kubecom" (the defaults) or "none"`,
		`bind "a" "x" { y }`:                `test.kdl:3: bind takes no block`,
	} {
		_, err := ParseFile("test.kdl", []byte("version 1\nkeymap {\n"+src+"\n}\n"), User)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
	if _, err := ParseFile("test.kdl", []byte("keymap {\n  bind \"a\n}\n"), User); err == nil || !strings.HasPrefix(err.Error(), "test.kdl:") {
		t.Errorf("a KDL syntax error does not name its place: %v", err)
	}
}

// Nested contexts are both: the inner block holds only where the outer one does.
func TestNestedContextsJoin(t *testing.T) {
	d := load(t, User, `context "picker" { context "field" { bind "ctrl+x" "inner" label="L" } }`)
	b := d.binds[0]
	if b.Context() != "(picker) && (field)" || b.Label != "L" || b.Args != nil || b.Source() != "test.kdl:2" {
		t.Fatalf("%+v at %s", b, b.Source())
	}
	km := Build(d)
	if exact(km, t, "ctrl+x", Path{Picker: &Picker{}}) != "" {
		t.Fatal("the inner block held outside its outer context")
	}
	if exact(km, t, "ctrl+x", Path{Picker: &Picker{}, Field: true}) != "inner" {
		t.Fatal("the inner block did not hold inside both")
	}
}

// Check is the registry's say over a document: every failure at its line.
func TestCheck(t *testing.T) {
	d := load(t, User, `
		bind "a" "nav.down"
		bind "b" "nope"
		bind "c" "nav.down" "1"
		unbind "d"
	`)
	err := d.Check(func(action string, args []string) error {
		switch {
		case action != "nav.down":
			return fmt.Errorf("%q is not an action", action)
		case len(args) > 0:
			return errors.New("nav.down takes no value")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), `test.kdl:4: "nope" is not an action`) ||
		!strings.Contains(err.Error(), "test.kdl:5: nav.down takes no value") {
		t.Fatalf("check: %v", err)
	}
}

// The cost of a context: compiling one (at load) and matching a key against a
// binding that has one (per keystroke).
func BenchmarkCompileContext(b *testing.B) {
	for b.Loop() {
		if _, err := CompileContext(`table.kind == 'Pod' && !confirm`); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatchInAContext(b *testing.B) {
	d, err := ParseFile("b.kdl", []byte(`keymap {
		context "table.kind == 'Pod' && !confirm" { bind "L" "res.logs" }
	}`), User)
	if err != nil {
		b.Fatal(err)
	}
	km := Build(d)
	p := Path{Cluster: &Cluster{Context: "kind"}, Table: &Table{Kind: "Pod"}}
	k, _ := ParseKeys("L")
	for b.Loop() {
		if km.Match(k, p).Exact == nil {
			b.Fatal("no match")
		}
	}
}
