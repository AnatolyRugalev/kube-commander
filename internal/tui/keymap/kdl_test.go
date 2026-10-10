package keymap

import (
	"os"
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/keybind"
)

// default.kdl loads, and every binding names a registered action with a value
// only where one is taken.
func TestDefaultKDLLoads(t *testing.T) {
	km := keybind.Build(DefaultDoc())
	if err := km.Err(); err != nil {
		t.Fatal(err)
	}
	bound := map[Action]bool{}
	for _, e := range km.Bindings() {
		bound[Action(e.Binding.Action)] = true
	}
	for _, a := range Actions() {
		if !bound[a] && !PaletteOnly(a) {
			t.Errorf("%s has no default binding", a)
		}
		if bound[a] && PaletteOnly(a) {
			t.Errorf("%s is palette-only but bound", a)
		}
	}
}

func TestCheckBinding(t *testing.T) {
	for _, tc := range []struct {
		action string
		args   []string
		want   string
	}{
		{"nav.down", nil, ""},
		{"ns.switch", []string{"kube-system"}, ""},
		{"resources.switch", []string{"pods"}, ""},
		{"nav.dwn", nil, `"nav.dwn" is not an action`},
		{"nav.down", []string{"1"}, "nav.down takes no value"},
		{"ns.switch", []string{"a", "b"}, "ns.switch takes one value, not 2"},
		{"theme.switch", []string{"nord"}, ""},
		{"theme.switch", []string{"nrod"}, `"nrod" is not a theme`},
	} {
		err := CheckBinding(tc.action, tc.args)
		if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s %v: %v, want %q", tc.action, tc.args, err, tc.want)
		}
	}
}

// The port keeps every key (D303 pt 6): on every surface, each key the old flat
// map, its confirm and table side tables and the text-field routes resolved, the
// context keymap resolves to the same action — and nothing more. The old engine
// is gone; testdata/oldkeys.golden is what it resolved, written from it before it
// went: one line per scenario, key and action ("" for nothing), a single key over
// every bound key, printable ASCII character, named key, ctrl+ and alt+ letter, and
// every multi-key sequence on the surfaces that fed the sequencer.
//
// The one difference allowed is KEYS-04's: an action that acts on one surface only
// is bound in that surface's context, so off it the old key — which resolved there
// and did nothing — resolves to nothing. tidied names each such action's surface,
// written out here rather than read from default.kdl.
func TestDefaultKDLKeepsTheOldKeys(t *testing.T) {
	km := keybind.Build(DefaultDoc())
	tidied := map[string]string{
		"logs.follow": "logs", "logs.regex": "logs", "logs.wrap": "logs", "logs.timestamps": "logs",
		"logs.previous": "logs", "logs.select": "logs", "logs.yank": "logs",
		"secret.reveal": "viewer.content == 'secret'", "secret.copy": "viewer.content == 'secret'",
		"forwards.stopAll":   "forwards",
		"forwards.localPort": "picker.id == 'ports'", "forwards.freeLocal": "picker.id == 'ports'",
		"search.allKinds": "search", "search.allNamespaces": "search",
	}
	surfaces := map[string]*keybind.Context{}
	for a, src := range tidied {
		c, err := keybind.CompileContext(src)
		if err != nil {
			t.Fatal(err)
		}
		surfaces[a] = c
	}
	cluster := &keybind.Cluster{Context: "kind-dev", Namespace: "default"}
	pods := &keybind.Table{Resource: "pods", Kind: "Pod", Namespaced: true}
	paths := map[string]keybind.Path{
		"menu":           {Cluster: cluster, Menu: true},
		"menu filter":    {Cluster: cluster, Menu: true, Field: true},
		"table":          {Cluster: cluster, Table: pods},
		"table filter":   {Cluster: cluster, Table: pods, Field: true},
		"sort":           {Cluster: cluster, Table: pods, Sort: true},
		"logs":           {Cluster: cluster, Table: pods, Logs: &keybind.Logs{Follow: true}},
		"logs grep":      {Cluster: cluster, Table: pods, Logs: &keybind.Logs{}, Field: true},
		"viewer":         {Cluster: cluster, Table: pods, Viewer: &keybind.Viewer{Content: "secret"}},
		"search results": {Cluster: cluster, Table: pods, Search: &keybind.Search{}},
		"search query":   {Cluster: cluster, Menu: true, Search: &keybind.Search{}, Field: true},
		"unhealthy":      {Cluster: cluster, Menu: true, Unhealthy: true},
		"forwards":       {Cluster: cluster, Table: pods, Forwards: true},
		"help":           {Cluster: cluster, Menu: true, Help: true},
		"picker":         {Cluster: cluster, Table: pods, Picker: &keybind.Picker{ID: "ports"}},
		"picker query":   {Cluster: cluster, Table: pods, Picker: &keybind.Picker{ID: "palette"}, Field: true},
		"prompt":         {Cluster: cluster, Table: pods, Prompt: true, Field: true},
		"confirm":        {Cluster: cluster, Table: pods, Confirm: true},
		"no cluster":     {},
	}
	src, err := os.ReadFile("testdata/oldkeys.golden")
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, line := range strings.Split(strings.TrimSuffix(string(src), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			t.Fatalf("golden line %q", line)
		}
		path, ok := paths[f[0]]
		if !ok {
			t.Fatalf("golden scenario %q has no path", f[0])
		}
		keys, err := keybind.ParseKeys(f[1])
		if err != nil {
			t.Fatalf("%s: %v", f[1], err)
		}
		lines++
		m := km.Match(keys, path)
		var got string
		if m.Exact != nil {
			got = m.Exact.Action
		}
		want := f[2]
		if c, ok := surfaces[want]; ok && !c.Holds(path) {
			want = ""
		}
		if got != want {
			t.Errorf("%s: %s resolves to %q, want %q (the old keymap: %q)", f[0], f[1], got, want, f[2])
		}
		if len(keys) > 1 {
			if m.Prefix {
				t.Errorf("%s: %s is a prefix of a longer binding", f[0], f[1])
			}
			if !km.Match(keys[:1], path).Prefix {
				t.Errorf("%s: %s does not start a sequence", f[0], keybind.FormatKeys(keys[:1]))
			}
		}
	}
	if lines < 3000 {
		t.Fatalf("the golden has %d lines", lines)
	}
}
