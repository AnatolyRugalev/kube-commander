package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// typeKeys presses each character of s as a key, returning the model and the
// last press's command.
func typeKeys(t *testing.T, m Model, s string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range s {
		m, cmd = press(t, m, tea.Key{Code: r, Text: string(r)})
	}
	return m, cmd
}

// A binding's value runs the verb with it, and no palette opens (D303 pt 2).
func TestBoundValueSwitchesResource(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := useKeymap(sizedWith(t, WithWatcher(fw)), userKeymap(t, `
		bind "g d" "resources.switch" "deployments"
		bind "g s" "resources.switch" "StatefulSet"
	`))
	m, _ = typeKeys(t, m, "gd")
	if m.cmdPicker.Active() {
		t.Fatal("a bound value opened the palette")
	}
	if !m.hasCurrent || m.current.GVR.Resource != "deployments" {
		t.Fatalf("g d browses %q, want deployments", m.current.GVR.Resource)
	}
	m, _ = typeKeys(t, m, "gs")
	if m.current.GVR.Resource != "statefulsets" {
		t.Fatalf("g s (by Kind) browses %q, want statefulsets", m.current.GVR.Resource)
	}
}

func TestBoundValueSwitchesNamespaceAndTheme(t *testing.T) {
	m := useKeymap(sizedWith(t, WithNamespaceLister(&fakeLister{})), userKeymap(t, `
		bind "g k" "ns.switch" "kube-system"
		bind "g a" "ns.switch" ""
		bind "g t" "theme.switch" "nord"
	`))
	m, _ = typeKeys(t, m, "gk")
	if m.namespace != "kube-system" || m.cmdPicker.Active() {
		t.Fatalf("g k: namespace %q, palette open %v", m.namespace, m.cmdPicker.Active())
	}
	m, _ = typeKeys(t, m, "ga")
	if m.namespace != "" {
		t.Fatalf("g a: namespace %q, want all namespaces", m.namespace)
	}
	m, _ = typeKeys(t, m, "gt")
	if m.styles.Theme.Name != "nord" {
		t.Fatalf("g t: theme %q, want nord", m.styles.Theme.Name)
	}
}

// actions.menu with a value runs that row action on the selected row, named by its
// title or its id.
func TestBoundValueRunsARowAction(t *testing.T) {
	m := useKeymap(openPodTable(t, "Pod"), userKeymap(t, `bind "g D" "actions.menu" "describe"`))
	_, cmd := typeKeys(t, m, "gD")
	for _, msg := range drainMsgs(cmd) {
		if ra, ok := msg.(rowActionMsg); ok && ra.Action == rowActionDescribe {
			return
		}
	}
	t.Fatal("g D did not dispatch the describe row action")
}

func TestBoundValuePinsAKind(t *testing.T) {
	fp := &fakePinPersister{}
	m := useKeymap(sizedWith(t, WithPinPersister(fp)), userKeymap(t, `bind "g P" "menu.pin" "deployments"`))
	m, cmd := typeKeys(t, m, "gP")
	drainMsgs(cmd)
	if len(fp.got) != 1 || fp.got[0].Resource != "deployments" {
		t.Fatalf("g P pinned %+v, want deployments", fp.got)
	}
	if !strings.Contains(m.View().Content, "pinned Deployment") {
		t.Fatal("the pin should say so on the status line")
	}
}

// A value that names nothing here says so, and runs nothing.
func TestBoundValueThatNamesNothing(t *testing.T) {
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := useKeymap(sizedWith(t, WithWatcher(fw)), userKeymap(t, `bind "g x" "resources.switch" "podz"`))
	m, _ = typeKeys(t, m, "gx")
	if m.hasCurrent {
		t.Fatalf("an unknown kind opened %q", m.current.GVR.Resource)
	}
	if !strings.Contains(m.View().Content, `nothing here is called "podz"`) {
		t.Fatalf("an unknown kind should say so:\n%s", m.View().Content)
	}
}

// A value runs only where its verb's stage would open: a surface that swallows the
// verb swallows the value too.
func TestBoundValueRespectsTheSurface(t *testing.T) {
	m := useKeymap(openPodTable(t, "Pod"), userKeymap(t, `bind "g d" "resources.switch" "deployments"`))
	m, _ = press(t, m, tea.Key{Code: 's', ShiftedCode: 'S', Mod: tea.ModShift})
	if !m.table.SortMode() {
		t.Fatal("S should enter the sort mode")
	}
	m, _ = typeKeys(t, m, "gd")
	if m.current.GVR.Resource != "pods" {
		t.Fatalf("the sort mode let g d switch to %q", m.current.GVR.Resource)
	}
}
