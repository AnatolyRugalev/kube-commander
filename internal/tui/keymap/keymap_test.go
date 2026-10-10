package keymap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/keybind"
)

// The context paths the tests read keys in.
var (
	testCluster = &keybind.Cluster{Context: "kind-dev", Namespace: "default"}
	testPods    = &keybind.Table{Resource: "pods", Kind: "Pod", Namespaced: true}
	menuPath    = keybind.Path{Cluster: testCluster, Menu: true}
	tablePath   = keybind.Path{Cluster: testCluster, Table: testPods}
	confirmPath = keybind.Path{Cluster: testCluster, Table: testPods, Confirm: true}
)

// withUser is the keymap with a config.kdl holding src as its `keymap` document,
// and an overlay holding over (none when it is ""), read the way the launcher
// reads them.
func withUser(t *testing.T, src, over string) *Keymap {
	t.Helper()
	km, err := loadUser(t, src, over)
	if err != nil {
		t.Fatal(err)
	}
	return km
}

func loadUser(t *testing.T, src, over string) (*Keymap, error) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("version 1\nkeymap {\n"+body+"\n}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(config.AuthoredName, src)
	if over != "" {
		write(config.OverlayName, over)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := UserDocs(cfg)
	if err != nil {
		return nil, err
	}
	return New(docs...)
}

func TestDefaultResolution(t *testing.T) {
	km := DefaultKeymap()
	tests := []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: 'j', Text: "j"}, ActionDown},
		{tea.Key{Code: 'k', Text: "k"}, ActionUp},
		{tea.Key{Code: tea.KeyDown}, ActionDown},
		{tea.Key{Code: 'd', Mod: tea.ModCtrl}, ActionHalfPageDown},
		{tea.Key{Code: tea.KeyPgDown}, ActionHalfPageDown},
		{tea.Key{Code: 'g', ShiftedCode: 'G', Mod: tea.ModShift}, ActionBottom},
		{tea.Key{Code: '/', Text: "/"}, ActionFilter},
		{tea.Key{Code: 'c', Mod: tea.ModCtrl}, ActionQuit},
		{tea.Key{Code: 'm', ShiftedCode: 'M', Mod: tea.ModShift}, ActionToggleMouse},
		{tea.Key{Code: 's', ShiftedCode: 'S', Mod: tea.ModShift}, ActionSort},
		{tea.Key{Code: 'x', Text: "x"}, ActionClearSort},
		{tea.Key{Code: 'm', Text: "m"}, ActionToggleMenu},
		{tea.Key{Code: 'f', Mod: tea.ModCtrl}, ActionSearch},
		{tea.Key{Code: 'n', ShiftedCode: 'N', Mod: tea.ModShift}, ActionNamespace},
		{tea.Key{Code: '#', Text: "#"}, ActionSearchPrev},
		{tea.Key{Code: 'h', ShiftedCode: 'H', Mod: tea.ModShift}, ActionUnhealthy},
		{tea.Key{Code: tea.KeySpace}, ActionPageDown},
		{tea.Key{Code: 'D', Text: "D"}, ActionDelete},
		{tea.Key{Code: 'd', Text: "d"}, ActionDescribe},
		{tea.Key{Code: 'V', Text: "V"}, ActionLogsSelect},
		{tea.Key{Code: tea.KeyHome}, ActionTop},
	}
	for _, tt := range tests {
		got, _, ok := km.Action(tt.key, menuPath)
		if !ok || got != tt.want {
			t.Errorf("Action(%+v) = %q,%v; want %q", tt.key, got, ok, tt.want)
		}
	}
	// An unbound key resolves to nothing. `s` is deliberately unbound — the letter
	// remap freed it entirely (D270): the s-family is now `S` sort, `x` clear,
	// `ctrl+f` search, nothing shares a letter. `a` is deliberately unbound too — the
	// actions menu moved onto the resource table's `enter` (STORY-06c), freeing the
	// letter. A lone `g` only starts `g g`: on its own it runs nothing.
	for _, k := range []tea.Key{{Code: 'z', Text: "z"}, {Code: 's', Text: "s"}, {Code: 'a', Text: "a"}, {Code: 'g', Text: "g"}} {
		if a, _, ok := km.Action(k, menuPath); ok {
			t.Errorf("Action(%+v) = %q, want unbound", k, a)
		}
	}
}

// KeyOf reads a live keypress the way the keymap writes keys: shift folds into a
// printable character, ctrl folds case, a named key keeps its shift.
func TestKeyOf(t *testing.T) {
	for _, tc := range []struct {
		key  tea.Key
		want string
	}{
		{tea.Key{Code: 'g', Text: "g"}, "g"},
		{tea.Key{Code: 'g', ShiftedCode: 'G', Mod: tea.ModShift}, "G"},
		{tea.Key{Code: 'D', Text: "D"}, "D"},
		{tea.Key{Code: '?', Text: "?"}, "?"},
		{tea.Key{Code: 'd', Mod: tea.ModCtrl}, "ctrl+d"},
		{tea.Key{Code: 'D', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+d"},
		{tea.Key{Code: 'b', Mod: tea.ModAlt}, "alt+b"},
		{tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, "shift+tab"},
		{tea.Key{Code: tea.KeyEnter}, "enter"},
		{tea.Key{Code: tea.KeySpace, Text: " "}, "space"},
		{tea.Key{Code: tea.KeyPgUp}, "pgup"},
		{tea.Key{Code: tea.KeyF5}, "f5"},
	} {
		if got := KeyOf(tc.key).String(); got != tc.want {
			t.Errorf("KeyOf(%+v) = %s, want %s", tc.key, got, tc.want)
		}
	}
}

// The confirm modal's answers live in `context "confirm"`: `y`/`enter` →
// confirm.accept and `n`/`esc` → confirm.decline there, while the same keys keep
// their meaning everywhere else (D132, D303).
func TestConfirmContextResolution(t *testing.T) {
	km := DefaultKeymap()
	for _, tc := range []struct {
		key  tea.Key
		path keybind.Path
		want Action
	}{
		{tea.Key{Code: 'y', Text: "y"}, confirmPath, ActionConfirmAccept},
		{tea.Key{Code: tea.KeyEnter}, confirmPath, ActionConfirmAccept},
		{tea.Key{Code: 'n', Text: "n"}, confirmPath, ActionConfirmDecline},
		{tea.Key{Code: tea.KeyEsc}, confirmPath, ActionConfirmDecline},
		// The confirm keys keep their browse meaning outside the modal, `y`
		// included: left free when res.yaml was retired (D135/M3-15c), it is
		// logs.yank since LOGS-SEL-02 — one key, two live actions, told apart by
		// nothing but the context.
		{tea.Key{Code: 'n', Text: "n"}, menuPath, ActionSearchNext},
		{tea.Key{Code: tea.KeyEnter}, menuPath, ActionDrillIn},
		{tea.Key{Code: tea.KeyEsc}, menuPath, ActionBack},
		{tea.Key{Code: 'y', Text: "y"}, menuPath, ActionLogsYank},
		// Under the modal the rest of the keymap still resolves, so app.quit
		// dismisses it.
		{tea.Key{Code: 'q', Text: "q"}, confirmPath, ActionQuit},
	} {
		if a, _, ok := km.Action(tc.key, tc.path); !ok || a != tc.want {
			t.Errorf("%+v in %s: %q,%v; want %q", tc.key, tc.path.Innermost(), a, ok, tc.want)
		}
	}
}

// `enter` opens the actions menu while the resource table itself has the keys
// (STORY-06c, `context "focus == 'table'"`), and keeps nav.drillIn everywhere
// else — over the table too, once something is opened over it.
func TestTableContextResolution(t *testing.T) {
	km := DefaultKeymap()
	enter := tea.Key{Code: tea.KeyEnter}
	if a, _, _ := km.Action(enter, tablePath); a != ActionActions {
		t.Errorf("enter on the table = %q; want actions.menu", a)
	}
	forwards := tablePath
	forwards.Forwards = true
	for _, p := range []keybind.Path{menuPath, forwards, {Cluster: testCluster, Table: testPods, Sort: true}} {
		if a, _, _ := km.Action(enter, p); a != ActionDrillIn {
			t.Errorf("enter in %s = %q; want nav.drillIn", p.Innermost(), a)
		}
	}
	if a, _, ok := km.Action(tea.Key{Code: 'a', Text: "a"}, tablePath); ok {
		t.Errorf("a on the table = %q, want unbound after STORY-06c freed it", a)
	}
}

// A field keeps its text (D303 pt 5): `/` types into an open filter, while a key
// that types nothing still acts.
func TestFieldsKeepTheirText(t *testing.T) {
	km := DefaultKeymap()
	field := tablePath
	field.Field = true
	if a, _, ok := km.Action(tea.Key{Code: '/', Text: "/"}, field); ok {
		t.Errorf("/ in the filter field = %q, want it typed", a)
	}
	if a, _, _ := km.Action(tea.Key{Code: tea.KeyEsc}, field); a != ActionBack {
		t.Errorf("esc in the filter field = %q, want nav.back", a)
	}
	if a, _, _ := km.Action(tea.Key{Code: tea.KeyEnter}, field); a != ActionDrillIn {
		t.Errorf("enter in the filter field = %q, want nav.drillIn", a)
	}
}

// The user's keymap is laid over the defaults: config.kdl's, then the overlay's.
func TestUserLayer(t *testing.T) {
	km := withUser(t, `
		bind "b" "app.filter"
		unbind "/"
		bind "z" "app.help"
		context "confirm" { bind "Q" "confirm.accept" }
	`, `bind "z" "app.quit"`)
	key := func(r rune) tea.Key { return tea.Key{Code: r, Text: string(r)} }
	for _, tc := range []struct {
		key  tea.Key
		path keybind.Path
		want Action
	}{
		{key('b'), menuPath, ActionFilter},
		{key('/'), menuPath, ""},
		{key('z'), menuPath, ActionQuit}, // the overlay's line beats config.kdl's
		{key('Q'), confirmPath, ActionConfirmAccept},
		{key('Q'), menuPath, ""},
		{key('y'), confirmPath, ActionConfirmAccept}, // the default still answers too
	} {
		if a, _, _ := km.Action(tc.key, tc.path); a != tc.want {
			t.Errorf("%s in %s = %q; want %q", tc.key.Text, tc.path.Innermost(), a, tc.want)
		}
	}
	// The default keymap is untouched.
	if a, _, _ := DefaultKeymap().Action(key('/'), menuPath); a != ActionFilter {
		t.Errorf("the default / = %q after a user keymap was built", a)
	}
}

// Unbinding every key of an action leaves nothing to run it, and the hints drop it.
func TestUserUnbindsAnAction(t *testing.T) {
	km := withUser(t, `unbind "q"; unbind "ctrl+c"`, "")
	if keys := km.Keys(ActionQuit, menuPath); len(keys) != 0 {
		t.Errorf("quit keys = %v after unbinding them", keys)
	}
	if _, _, ok := km.Action(tea.Key{Code: 'q', Text: "q"}, menuPath); ok {
		t.Error("q still resolves after unbinding it")
	}
}

// A user keymap that names no action, gives a value where none is taken, or
// writes `<leader>` with no leader, is refused whole, at its file and line.
func TestUserKeymapErrors(t *testing.T) {
	for src, want := range map[string]string{
		`bind "b" "app.filtr"`:          `config.kdl:3:`,
		`bind "b" "nav.down" "2"`:       "nav.down takes no value",
		`bind "<leader> b" "app.help"`:  "leader",
		`context "tabel" { bind "b" }`:  "config.kdl:3:",
		`bind "b" "ns.switch" "a" "b"`:  "takes one value, not 2",
		`context "table.knd == 'Pod'"`:  "config.kdl:3:",
		`bind "ctrl+" "app.help"`:       "config.kdl:3:",
		`unbind "b" "app.help" "extra"`: "config.kdl:3:",
	} {
		if _, err := loadUser(t, src, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
}

// Resolve reads a run of trace tokens as one sequence.
func TestResolve(t *testing.T) {
	km := DefaultKeymap()
	for _, tc := range []struct {
		tokens []string
		want   Action
	}{
		{[]string{"g", "g"}, ActionTop},
		{[]string{"j"}, ActionDown},
		{[]string{"ctrl+d"}, ActionHalfPageDown},
		{[]string{"g"}, ""},
		{[]string{"ctrl+"}, ""},
	} {
		if a, ok := km.Resolve(keybind.Path{}, tc.tokens...); a != tc.want || ok != (tc.want != "") {
			t.Errorf("Resolve(%v) = %q,%v; want %q", tc.tokens, a, ok, tc.want)
		}
	}
}

func TestActionsRegistered(t *testing.T) {
	for _, a := range Actions() {
		if !a.Valid() {
			t.Errorf("Actions() returned unregistered %q", a)
		}
		if a.Describe() == "" {
			t.Errorf("action %q has no description", a)
		}
	}
}
