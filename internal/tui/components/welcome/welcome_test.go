package welcome

import (
	"strings"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

func newPage() Model { return New(styles.Default()) }

func TestViewEmptyBeforeSized(t *testing.T) {
	m := newPage()
	if got := m.View(false); got != "" {
		t.Errorf("View() before SetSize = %q, want empty", got)
	}
}

func TestHeaderShowsNameAndVersion(t *testing.T) {
	m := newPage()
	m.SetVersion("dev")
	if got := m.header(); got != "kubecom dev" {
		t.Errorf("header() = %q, want %q", got, "kubecom dev")
	}
}

func TestHeaderOmitsVersionWhenUnset(t *testing.T) {
	m := newPage() // no version
	if got := m.header(); got != "kubecom" {
		t.Errorf("header() with no version = %q, want %q", got, "kubecom")
	}
}

func TestBodyIncludesNameAndHint(t *testing.T) {
	m := newPage()
	m.SetVersion("dev")
	got := m.body(40)
	if !strings.Contains(got, "kubecom") {
		t.Errorf("body() = %q, want the app name", got)
	}
	if !strings.Contains(got, hint) {
		t.Errorf("body() = %q, want the pick-a-resource hint", got)
	}
}

func TestScopeJoinsContextAndNamespace(t *testing.T) {
	m := newPage()
	m.SetContext("prod")
	m.SetNamespace("kube-system")
	got := m.scope()
	if !strings.Contains(got, "prod") || !strings.Contains(got, "kube-system") {
		t.Fatalf("scope() = %q, want context and namespace", got)
	}
	if !strings.Contains(got, "·") {
		t.Errorf("scope() = %q, want a separator between pieces", got)
	}
}

func TestScopeEmptyNamespaceReadsAllNamespaces(t *testing.T) {
	m := newPage()
	m.SetContext("prod") // namespace left empty
	got := m.scope()
	if !strings.Contains(got, namespaceAll) {
		t.Errorf("scope() with empty namespace = %q, want %q", got, namespaceAll)
	}
}

func TestScopeSkipsEmptyContext(t *testing.T) {
	m := newPage() // no context, no namespace
	got := m.scope()
	if strings.Contains(got, "·") {
		t.Errorf("scope() with only the namespace fallback = %q, want no dangling separator", got)
	}
	if !strings.Contains(got, namespaceAll) {
		t.Errorf("scope() = %q, want %q", got, namespaceAll)
	}
}

func TestBodyIncludesShortHelp(t *testing.T) {
	m := newPage()
	m.SetShortHelp("help quit")
	if got := m.body(40); !strings.Contains(got, "help") {
		t.Errorf("body() = %q, want the short-help hint included", got)
	}
}

func TestViewFrameReflectsFocusAndSize(t *testing.T) {
	m := newPage()
	m.SetSize(40, 12)
	m.SetVersion("dev")
	m.SetContext("prod")

	unfocused := m.View(false)
	if unfocused == "" {
		t.Fatal("View() after SetSize should render")
	}
	if !strings.Contains(unfocused, "kubecom") {
		t.Errorf("View() = %q, want the app name visible", unfocused)
	}
	// Focused and unfocused frames differ only in border color, so they must not be
	// byte-identical (the accented border is applied).
	if focused := m.View(true); focused == unfocused {
		t.Error("View(true) should differ from View(false) (focused border)")
	}
}
