package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
)

// TestWithThemeReplacesTheShellStyles proves the option reaches the model's own
// Styles — the palette every later SetStyles (M4-12b) and every direct render in
// app.go reads.
func TestWithThemeReplacesTheShellStyles(t *testing.T) {
	m := New(WithTheme(styles.MonokaiTheme()))
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("styles.Theme.Name = %q, want monokai", got)
	}
	if New().styles.Theme.Name != styles.DefaultTheme().Name {
		t.Fatalf("without the option the shell must keep the default theme")
	}
}

// TestWithThemeReachesTheComponents is the headline invariant of M4-12a: the
// components are constructed *after* the options run, so a themed shell actually
// draws in that theme. Constructing them first (the pre-M4-12a order) would leave
// every one of them holding the default Styles and make this a silent no-op —
// m.styles would say monokai while the screen stayed blue.
func TestWithThemeReachesTheComponents(t *testing.T) {
	def := sizedWith(t).View().Content
	themed := sizedWith(t, WithTheme(styles.MonokaiTheme())).View().Content
	if def == "" || themed == "" {
		t.Fatal("sized View rendered nothing; the comparison below would be vacuous")
	}
	if def == themed {
		t.Error("the browse view renders identically under monokai and the default theme")
	}
	// Same glyphs, different colors: the theme must not change the layout.
	if stripANSI(def) != stripANSI(themed) {
		t.Errorf("a theme changed the rendered text, not just its colors:\ndefault:\n%s\nmonokai:\n%s",
			stripANSI(def), stripANSI(themed))
	}
}

// TestEveryBuiltinThemeRendersTheShell walks the registry through the real
// constructor, so a theme added to `builtins` is exercised end-to-end here rather
// than only in the styles package's own unit tests.
func TestEveryBuiltinThemeRendersTheShell(t *testing.T) {
	seen := map[string]string{}
	for _, th := range styles.Themes() {
		out := sizedWith(t, WithTheme(th)).View().Content
		for name, prev := range seen {
			if prev == out {
				t.Errorf("themes %q and %q render the shell identically", name, th.Name)
			}
		}
		seen[th.Name] = out
	}
}

// TestWithThemeLeavesOtherOptionsApplied guards the constructor restructure from
// the other side: the post-option seeding (menu extras, the namespace fed to the
// menu and status bar, the welcome page's version) must still happen, since it now
// runs against components built later in the same function.
func TestWithThemeLeavesOtherOptionsApplied(t *testing.T) {
	m, _ := New(
		WithTheme(styles.SolarizedDarkTheme()),
		WithNamespace("kube-system"),
		WithVersion("v9.9.9"),
		WithContext("prod-cluster"),
	).Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := stripANSI(m.(Model).View().Content)
	for _, want := range []string{"kube-system", "v9.9.9", "prod-cluster"} {
		if !strings.Contains(view, want) {
			t.Errorf("themed shell dropped %q from its view:\n%s", want, view)
		}
	}
}

// themeSurfaces are the screen states a live restyle has to cover: the browse shell
// plus every overlay and full-screen view, each built through the shell's own
// constructor so the components under test are the ones the app really runs. Each
// entry takes construction Options so the same surface can be built two ways — themed
// at launch, and themed afterwards — which is what makes the comparison below
// possible. Surfaces are seeded with *content* (rows, items, hits, log lines) on
// purpose: an empty overlay draws little more than a border and would hide a component
// that was left behind.
var themeSurfaces = []struct {
	name  string
	build func(t *testing.T, opts ...Option) Model
}{
	{"browse shell (menu, bars, welcome page)", func(t *testing.T, opts ...Option) Model {
		return sizedWith(t, opts...)
	}},
	{"resource table (rows, filtered, sorted)", func(t *testing.T, opts ...Option) Model {
		return browsingModel(t, &fakeWatcher{}, opts...)
	}},
	{"help overlay", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.help.SetVisible(true)
		return m
	}},
	{"picker overlay", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.nsPicker.SetItems([]string{"default", "kube-system", "web"})
		m.nsPicker.Show()
		return m
	}},
	{"viewer overlay", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.viewer.SetTitle("Pod web/api-1")
		m.viewer.SetContent("apiVersion: v1\nkind: Pod\n")
		m.viewer.Show()
		return m
	}},
	{"confirm modal", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.modal.ShowConfirm(deleteModalKind, "Delete", "delete api-1?")
		return m
	}},
	{"cluster search view", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.searchView.Show()
		m.searchView.AppendHit(searchHit("Pod", "pods", "web", "api-1"))
		m.searchView.AppendHit(searchHit("Service", "services", "web", "api"))
		return m
	}},
	{"logs view", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.logsView.Show()
		m.logsView.Append("", "GET /healthz 200")
		m.logsView.Append("", "POST /api/v1 500")
		return m
	}},
}

// TestApplyStylesMatchesLaunchTimeTheme is the headline invariant of M4-12b-1, and the
// one assertion that can prove the fan-out is *complete*: for every surface, a shell
// built on the default palette and then restyled must render byte-for-byte identically
// to one built with that theme from the start (M4-12a's path, which is known to reach
// every component). A component missing from applyStyles keeps its default Styles and
// so keeps drawing in default colors, which shows up here as a diff — including for
// components that only appear in one overlay, which is exactly the failure a
// hand-written "does it look different?" test would miss.
func TestApplyStylesMatchesLaunchTimeTheme(t *testing.T) {
	theme := styles.MonokaiTheme()
	for _, sf := range themeSurfaces {
		t.Run(sf.name, func(t *testing.T) {
			want := sf.build(t, WithTheme(theme)).View().Content

			m := sf.build(t)
			plain := m.View().Content
			m.applyStyles(styles.New(theme))
			got := m.View().Content

			if plain == "" {
				t.Fatal("the surface rendered nothing; the comparisons below would be vacuous")
			}
			if plain == want {
				t.Fatal("this surface renders identically under both themes, so it cannot show a missed restyle")
			}
			if got != want {
				t.Errorf("a restyled shell does not match one built with the theme — a component was left on the old palette:\nrestyled:\n%q\nbuilt themed:\n%q", got, want)
			}
		})
	}
}

// TestApplyStylesChangesOnlyColors is the other half of the contract: a restyle
// repaints, it never re-lays-out or resets. Same glyphs in the same places on every
// surface (so no component was re-created, resized or cleared), different escape
// sequences around them.
func TestApplyStylesChangesOnlyColors(t *testing.T) {
	for _, sf := range themeSurfaces {
		t.Run(sf.name, func(t *testing.T) {
			m := sf.build(t)
			before := m.View().Content
			m.applyStyles(styles.New(styles.SolarizedDarkTheme()))
			after := m.View().Content

			if before == after {
				t.Error("the surface did not repaint at all")
			}
			if stripANSI(before) != stripANSI(after) {
				t.Errorf("a restyle changed the rendered text, not just its colors:\nbefore:\n%s\nafter:\n%s",
					stripANSI(before), stripANSI(after))
			}
			if got := m.styles.Theme.Name; got != "solarized-dark" {
				t.Errorf("the shell's own styles were not repointed: Theme.Name = %q", got)
			}
		})
	}
}

// TestApplyStylesKeepsComponentState guards the "colors only" promise where it is
// easiest to break — the components whose SetStyles has to re-derive something and so
// does more than assign a field. A reader who picks a theme mid-task must not lose
// their filter, their sort, their place in a list or their log buffer.
func TestApplyStylesKeepsComponentState(t *testing.T) {
	m := browsingModel(t, &fakeWatcher{}) // rows + a filter + a sort
	m.nsPicker.SetItems([]string{"default", "kube-system", "web"})
	m.nsPicker.Show()
	m.nsPicker, _ = m.nsPicker.Update(keymap.ActionDown) // move off the first row
	cursor, _ := m.nsPicker.Selected()
	m.logsView.Show()
	m.logsView.Append("", "GET /healthz 200")
	m.logsView.Append("", "POST /api/v1 500")
	wantCol, wantDesc := m.table.SortColumn()

	m.applyStyles(styles.New(styles.MonokaiTheme()))

	if got := m.table.Filter(); got != "api" {
		t.Errorf("the table's filter did not survive the restyle: %q", got)
	}
	if col, desc := m.table.SortColumn(); col != wantCol || desc != wantDesc {
		t.Errorf("the table's sort did not survive the restyle: col=%d desc=%v, want col=%d desc=%v",
			col, desc, wantCol, wantDesc)
	}
	if got, _ := m.nsPicker.Selected(); got != cursor {
		t.Errorf("the picker's cursor moved on restyle: %q, want %q", got, cursor)
	}
	if !m.nsPicker.Active() || !m.logsView.Active() {
		t.Error("a restyle must not dismiss an open surface")
	}
	if m.logsView.Empty() || !m.logsView.Following() {
		t.Error("the logs buffer and its follow state must survive a restyle")
	}
}

// stripANSI removes SGR escape sequences so a comparison sees the glyphs alone.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		// Skip through the sequence's final byte ('m' for the SGR codes styles emit).
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return b.String()
}
