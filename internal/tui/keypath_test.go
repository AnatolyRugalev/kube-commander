package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/keybind"
	"github.com/neuroplastio/kubecom/internal/tui/help"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// userKeymap is the default keymap with src as the user's `keymap` document.
func userKeymap(t *testing.T, src string) *keymap.Keymap {
	t.Helper()
	doc, err := keybind.ParseFile("config.kdl", []byte("version 1\nkeymap {\n"+src+"\n}\n"), keybind.User)
	if err != nil {
		t.Fatal(err)
	}
	km, err := keymap.New(doc)
	if err != nil {
		t.Fatal(err)
	}
	return km
}

// useKeymap swaps the model's keymap the way NewWithKeymap wires one: the
// sequencer and the help component read it too.
func useKeymap(m Model, km *keymap.Keymap) Model {
	visible := m.help.Visible()
	m.keymap, m.seq = km, keymap.NewSequencer(km)
	m.help = help.New(m.styles, km)
	m.help.SetWidth(m.width)
	m.help.SetHeight(m.bodyHeight())
	m.help.SetVisible(visible)
	m.syncHints()
	return m
}

// The path follows what holds the keyboard: the pane with focus and its kind, a
// view opened over it, an overlay over that, a field.
func TestKeyPath(t *testing.T) {
	m := sizedWith(t, WithContext("kind-dev"), WithNamespace("web"))
	if got, want := m.keyPath().String(), `cluster(context="kind-dev", namespace="web") > menu`; got != want {
		t.Errorf("at start: %s, want %s", got, want)
	}
	m = openPodTable(t, "Pod", WithContext("kind-dev"))
	if got, want := m.keyPath().String(), `cluster(context="kind-dev") > table(kind="Pod", resource="pods")`; got != want {
		t.Errorf("on the pods table: %s, want %s", got, want)
	}
	f, _ := press(t, m, tea.Key{Code: '/', Text: "/"})
	if got := f.keyPath(); got.Innermost() != "field" || got.Table == nil {
		t.Errorf("with the filter open: %s", got)
	}
	p, _ := press(t, m, tea.Key{Code: ':', Text: ":"})
	if got := p.keyPath(); got.Picker == nil || got.Picker.ID != "palette" || !got.Field {
		t.Errorf("with the palette open: %s", got)
	}
	h, _ := press(t, m, tea.Key{Code: '?', Text: "?"})
	if got := h.keyPath(); got.Innermost() != "help" {
		t.Errorf("with the help overlay open: %s", got)
	}
}

// A binding in a context acts only where the context holds: a key bound on the
// pods table runs there and nowhere else.
func TestContextBindingActsWhereItHolds(t *testing.T) {
	km := userKeymap(t, `context "table.kind == 'Pod'" { bind "Q" "app.help"; }`)
	m := useKeymap(openPodTable(t, "Pod"), km)
	m, _ = press(t, m, tea.Key{Code: 'Q', Text: "Q"})
	if !m.help.Visible() {
		t.Fatal("Q on the pods table should run app.help")
	}
	m = useKeymap(openPodTable(t, "Deployment"), km)
	m, _ = press(t, m, tea.Key{Code: 'Q', Text: "Q"})
	if m.help.Visible() {
		t.Fatal("Q ran app.help on a Deployment table")
	}
}
