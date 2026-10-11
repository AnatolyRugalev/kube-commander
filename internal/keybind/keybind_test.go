package keybind

import (
	"strings"
	"testing"

	"github.com/neuroplastio/myelin"
	"github.com/neuroplastio/myelin/myelinkey"
)

// These tests read kubecom's KDL into myelin and match on kubecom's paths: they
// hold kubecom to what its docs promise (D303), whichever engine runs it.

func load(t *testing.T, layer, src string) myelin.Doc {
	t.Helper()
	d, err := ParseFile("test.kdl", []byte("keymap {\n"+src+"\n}\n"), layer)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// build builds documents with no registry check, failing the test on an error.
func build(t *testing.T, docs ...myelin.Doc) *myelin.Keymap {
	t.Helper()
	km, problems := Env().Build(docs, nil)
	if km == nil {
		t.Fatalf("the keymap was refused: %+v", problems)
	}
	return km
}

func keys(t *testing.T, s string) []myelinkey.Key {
	t.Helper()
	ks, err := ParseSeq(s)
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func exact(km *myelin.Keymap, t *testing.T, seq string, p Path) string {
	t.Helper()
	m := km.Match(keys(t, seq), p.Steps())
	if m.Exact == nil {
		return ""
	}
	return m.Exact.Action
}

// The schema is myelin's to accept: every node and attribute a context reads.
func TestSchemaIsAccepted(t *testing.T) {
	if _, err := myelin.NewEnv(Schema()); err != nil {
		t.Fatal(err)
	}
}

// Keys are read as Ghostty reads them (myelin's D15): any modifier order and
// case, a short name or the full one, and a capital is Shift with its letter
// (KeyOptions). Display writes each the way default.kdl does, and reads back.
func TestKeySpellings(t *testing.T) {
	for _, same := range [][]string{
		{"G", "shift+g", "Shift+G", "shift+G"},
		{"g", "g"},
		{"ctrl+alt+x", "alt+ctrl+x", "Ctrl+Alt+x", "control+alt+x"},
		{"ctrl+shift+d", "ctrl+D", "shift+ctrl+d"},
		{"esc", "escape", "ESC", "Escape"},
		{"pgdn", "page_down", "PageDown"},
		{"up", "arrow_up", "ArrowUp"},
		{"shift+tab", "Shift+Tab"},
		{"space", "space"},
		{"#", "#"},
	} {
		want, err := ParseSeq(same[0])
		if err != nil {
			t.Fatalf("%s: %v", same[0], err)
		}
		if got := DisplaySeq(want); got != same[0] {
			t.Errorf("%s displays as %s", same[0], got)
		}
		for _, s := range same[1:] {
			if got, err := ParseSeq(s); err != nil || DisplaySeq(got) != same[0] {
				t.Errorf("%s reads as %v (%v), want %s", s, got, err, same[0])
			}
		}
	}
	if g, _ := ParseSeq("g"); DisplaySeq(g) == "G" {
		t.Error("g and G are one key")
	}
	for bad, want := range map[string]string{
		"gg":     "separated by spaces",
		"ctrl+":  `test.kdl:2: bind: "ctrl+": no key after the last +`,
		"meta+x": "write super+",
		"ctrl":   "is a modifier",
	} {
		_, err := ParseFile("test.kdl", []byte("keymap {\nbind \""+bad+"\" \"x\"\n}\n"), User)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", bad, err, want)
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
		if _, err := Env().Compile(ok); err != nil {
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
		if _, err := Env().Compile(src); err == nil {
			t.Errorf("%s compiled; it is %s", src, why)
		}
	}
	_, err := Env().Compile(`tabel`)
	if err == nil || !strings.Contains(err.Error(), "(a context reads focus, cluster, menu, table") {
		t.Errorf("an unknown name does not list the vocabulary: %v", err)
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
		c, err := Env().Compile(src)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Depth(path.Steps()); got != want {
			t.Errorf("%s: depth %d, want %d", src, got, want)
		}
	}
	var none *myelin.Context
	if none.Depth(path.Steps()) != 0 {
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
	km := build(t, def, user)
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
	km := build(t, load(t, Default, `
		bind "g" "go.short"
		bind "g g" "nav.top"
		context "logs" { bind "g x" "go.logs" }
	`))
	m := km.Match(keys(t, "g"), nil)
	if m.Exact == nil || m.Exact.Action != "go.short" || !m.Prefix {
		t.Fatalf("g: %+v", m)
	}
	if m := km.Match(keys(t, "g g"), nil); m.Exact == nil || m.Prefix {
		t.Fatalf("g g: %+v", m)
	}
	// A continuation whose context does not hold is no prefix.
	if m := km.Match(keys(t, "g x"), nil); m.Exact != nil || m.Prefix {
		t.Fatalf("g x outside the logs: %+v", m)
	}
	if !km.Starts(keys(t, "g")[0]) || km.Starts(keys(t, "x")[0]) {
		t.Fatal("Starts does not index the first keys")
	}
}

// A shallower continuation does not hold a deeper exact binding up: `enter` on
// the table's own keys runs at once though `enter …` sequences exist everywhere.
func TestAShallowerPrefixDoesNotWait(t *testing.T) {
	km := build(t, load(t, Default, `
		bind "enter x" "long"
		context "focus == 'table'" { bind "enter" "actions.menu" }
	`))
	m := km.Match(keys(t, "enter"), Path{Table: &Table{}}.Steps())
	if m.Exact == nil || m.Exact.Action != "actions.menu" || m.Prefix {
		t.Fatalf("enter: %+v", m)
	}
}

// focus is the innermost node: a binding in `focus == 'table'` is the table's own
// and does not reach an overlay opened over it, while one in `table` does.
func TestFocusIsTheInnermostNode(t *testing.T) {
	km := build(t, load(t, Default, `
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
	km := build(t, load(t, Default, `
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
	if m := km.Match(keys(t, "g"), field.Steps()); m.Prefix {
		t.Error("g still starts a sequence in a field")
	}
	noField := Path{Table: &Table{}, Picker: &Picker{ID: "palette"}}
	if exact(km, t, "j", noField) != "nav.down" || exact(km, t, "x", noField) != "picker.x" {
		t.Error("with the field closed the text keys do not resolve")
	}
	var next []string
	for _, it := range km.Menu(nil, field.Steps(), nil).Items {
		next = append(next, Display(it.Key))
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
	km := build(t, def, user)
	prod := Path{Cluster: &Cluster{Context: "prod-eu"}, Table: &Table{}}
	dev := Path{Cluster: &Cluster{Context: "kind"}, Table: &Table{}}
	if exact(km, t, "D", prod) != "" || exact(km, t, "D", dev) != "res.delete" {
		t.Error("the prod-only unbind did not hold where, and only where, it says")
	}
	if m := km.Match(keys(t, "g"), dev.Steps()); m.Prefix || m.Exact != nil {
		t.Errorf("g is still a prefix: %+v", m)
	}
	if exact(km, t, "g g", dev) != "" {
		t.Error("g g survived its group's unbind")
	}
}

// An unbind outside any context does not reach a key a lower layer binds in one —
// the deeper context wins where it holds — so it is loaded with a warning naming
// the context to unbind it in.
func TestShallowUnbindWarns(t *testing.T) {
	def := load(t, Default, `
		bind "q" "app.quit"
		context "forwards" { bind "X" "forwards.stopAll"; }
	`)
	user := load(t, User, `
		unbind "q"
		unbind "X"
		context "forwards" { unbind "X"; }
	`)
	km, problems := Env().Build([]myelin.Doc{def, user}, nil)
	if km == nil {
		t.Fatalf("a warning refused the keymap: %+v", problems)
	}
	if len(problems) != 1 || !problems[0].Warning || problems[0].Source != "test.kdl:4" ||
		!strings.Contains(problems[0].Text, `unbind "X" leaves X → forwards.stopAll in context "forwards"`) {
		t.Fatalf("problems = %+v", problems)
	}
	if exact(km, t, "X", Path{Forwards: true}) != "" {
		t.Error("the unbind in the forwards context did not take X away there")
	}
}

// The leader is an alias the highest layer that names one sets; with none named,
// a `<leader>` binding refuses the keymap.
func TestTheLeaderIsAnAlias(t *testing.T) {
	def := load(t, Default, `bind "<leader> n" "ns.switch"`)
	km := build(t, def, load(t, User, `leader "space"`))
	if l, ok := km.Alias("leader"); !ok || DisplaySeq(l) != "space" {
		t.Fatalf("leader = %v, %v", l, ok)
	}
	if exact(km, t, "space n", Path{}) != "ns.switch" {
		t.Fatal("<leader> n did not follow the leader to space")
	}
	km, problems := Env().Build([]myelin.Doc{def}, nil)
	if km != nil || len(problems) != 1 || problems[0].Source != "test.kdl:2" || !strings.Contains(problems[0].Text, "no alias <leader>") {
		t.Fatalf("no leader: %v, %+v", km, problems)
	}
}

// base "none" leaves the defaults out.
func TestBaseNone(t *testing.T) {
	km := build(t, load(t, Default, `bind "j" "nav.down"`), load(t, User, `base "none"`+"\n"+`bind "x" "app.quit"`))
	if exact(km, t, "j", Path{}) != "" || exact(km, t, "x", Path{}) != "app.quit" {
		t.Fatal("base none kept the defaults")
	}
}

// A menu lists one item per following key, in the order the keymap first binds it;
// a key with longer bindings behind it is a group with its label.
func TestMenuIsTheHint(t *testing.T) {
	km := build(t, load(t, Default, `
		group "g" "+goto"
		bind "g g" "nav.top"
		bind "g r" "res.relations" label="relations"
		bind "G" "nav.bottom"
		context "logs" { bind "g l" "logs.only" }
	`))
	var got []string
	for _, it := range km.Menu(nil, nil, nil).Items {
		if it.Group {
			got = append(got, Display(it.Key)+"="+it.Label)
		} else {
			got = append(got, Display(it.Key)+"="+it.Binding.Action)
		}
	}
	if s := strings.Join(got, " "); s != "g=+goto G=nav.bottom" {
		t.Fatalf("menu = %s", s)
	}
	got = nil
	for _, it := range km.Menu(keys(t, "g"), nil, nil).Items {
		got = append(got, Display(it.Key)+"="+it.Label)
	}
	if s := strings.Join(got, " "); s != "g= r=relations" {
		t.Fatalf("menu after g = %s", s)
	}
}

// KeysFor is what a hint shows for an action in a context: the sequences that win
// there.
func TestKeysFor(t *testing.T) {
	km := build(t, load(t, Default, `
		bind "k" "nav.up"
		bind "up" "nav.up"
		bind "g p" "resources.switch" "pods"
		bind "R" "resources.switch"
	`), load(t, User, `unbind "up"`))
	var got []string
	for _, ks := range km.KeysFor("nav.up", nil, nil) {
		got = append(got, DisplaySeq(ks))
	}
	if s := strings.Join(got, ","); s != "k" {
		t.Fatalf("nav.up = %s", s)
	}
	if ks := km.KeysFor("resources.switch", []string{"pods"}, nil); len(ks) != 1 || DisplaySeq(ks[0]) != "g p" {
		t.Fatalf("resources.switch pods = %v", ks)
	}
}

// Every error names its line, and one bad line refuses the document.
func TestParseErrorsNameTheLine(t *testing.T) {
	for src, want := range map[string]string{
		`bind "q"`:                          `test.kdl:3: bind takes at least 2 arguments`,
		`bind "ctrl+" "x"`:                  `test.kdl:3: bind: "ctrl+"`,
		`bind "gg" "nav.top"`:               "separated by spaces: `g g`",
		`bind "a" "x" colour="red"`:         `test.kdl:3: bind has no property "colour"`,
		`bind "a" "x" label=#true`:          `test.kdl:3: bind's label is the wrong kind of value`,
		`chord "a"`:                         `test.kdl:3: "chord" is not a keymap node`,
		`context "table.kindd == 1" { }`:    `test.kdl:3: context "table.kindd == 1"`,
		`context "logs" { leader "space" }`: `test.kdl:3: leader cannot be inside a context`,
		`leader "space" "tab"`:              `test.kdl:3: leader takes one argument`,
		`leader "ctrl+"`:                    `test.kdl:3: leader: "ctrl+": no key after the last +`,
		`base "vim"`:                        `test.kdl:3: base is "kubecom" (the defaults) or "none"`,
		`bind "a" "x" { y }`:                `test.kdl:3: bind takes no block`,
		`unbind ""`:                         `test.kdl:3: unbind: an empty key sequence`,
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
	b := d.Binds[0]
	if b.When != "(picker) && (field)" || b.Label != "L" || b.Args != nil || b.Source != "test.kdl:2" {
		t.Fatalf("%+v", b)
	}
	km := build(t, d)
	if exact(km, t, "ctrl+x", Path{Picker: &Picker{}}) != "" {
		t.Fatal("the inner block held outside its outer context")
	}
	if exact(km, t, "ctrl+x", Path{Picker: &Picker{}, Field: true}) != "inner" {
		t.Fatal("the inner block did not hold inside both")
	}
}

// The cost of a context: compiling one (at load) and matching a key against a
// binding that has one (per keystroke), the path built as the model builds it.
func BenchmarkCompileContext(b *testing.B) {
	for b.Loop() {
		if _, err := Env().Compile(`table.kind == 'Pod' && !confirm`); err != nil {
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
	km, _ := Env().Build([]myelin.Doc{d}, nil)
	p := Path{Cluster: &Cluster{Context: "kind"}, Table: &Table{Kind: "Pod"}}
	k, _ := ParseSeq("L")
	for b.Loop() {
		if km.Match(k, p.Steps()).Exact == nil {
			b.Fatal("no match")
		}
	}
}
