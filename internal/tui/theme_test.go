package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/picker"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
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
//
// The one pair exempted from distinctness is `default`/`catppuccin-frappe`: they
// are deliberately the same palette under two names, because kubecom's default
// has always *been* Catppuccin Frappé and D169 pt 1 will not let the name move
// (D236 pt 2). The styles package pins that they stay equal.
func TestEveryBuiltinThemeRendersTheShell(t *testing.T) {
	sameOnPurpose := func(a, b string) bool {
		return (a == "default" && b == "catppuccin-frappe") ||
			(a == "catppuccin-frappe" && b == "default")
	}
	seen := map[string]string{}
	for _, th := range styles.Themes() {
		out := sizedWith(t, WithTheme(th)).View().Content
		for name, prev := range seen {
			if prev == out && !sameOnPurpose(name, th.Name) {
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
		m.ctrPicker.SetItems([]string{"Describe", "View YAML", "Logs"})
		m.ctrPicker.Show()
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
	{"port-forward panel", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.forwards = append(m.forwards, &forward{
			id:    1,
			label: "pod/api-1",
			specs: []string{"8080:80"},
			bound: []kube.ForwardedPort{{Local: 8080, Remote: 80}},
			ready: true,
		})
		m.pfPanel.Open()
		return m
	}},
	{"cluster search view", func(t *testing.T, opts ...Option) Model {
		m := sizedWith(t, opts...)
		m.searchView.Show()
		m.searchView.AppendHit(searchHit("Pod", "pods", "web", "api-1"))
		m.searchView.AppendHit(searchHit("Service", "services", "web", "api"))
		return m
	}},
	{"palette on its theme stage", func(t *testing.T, opts ...Option) Model {
		// Seeded with fixed rows rather than by pressing `T`: the marker names the theme
		// the shell is rendering in, so the real opener would put it on a different row
		// in the launch-themed and restyled builds and the comparison would fail on
		// glyphs instead of colors.
		m := sizedWith(t, opts...)
		m.cmdPicker.SetTitle(keymap.ActionTheme.Describe())
		m.cmdPicker.SetPrompt(palettePrompt + "theme ")
		m.cmdPicker.SetItems([]string{"* default", "  monokai", "  solarized-dark"})
		m.cmdPicker.Show()
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
	m.ctrPicker.SetItems([]string{"Describe", "View YAML", "Logs"})
	m.ctrPicker.Show()
	m.ctrPicker, _ = m.ctrPicker.Update(keymap.ActionDown) // move off the first row
	cursor, _ := m.ctrPicker.Selected()
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
	if got, _ := m.ctrPicker.Selected(); got != cursor {
		t.Errorf("the picker's cursor moved on restyle: %q, want %q", got, cursor)
	}
	if !m.ctrPicker.Active() || !m.logsView.Active() {
		t.Error("a restyle must not dismiss an open surface")
	}
	if m.logsView.Empty() || !m.logsView.Following() {
		t.Error("the logs buffer and its follow state must survive a restyle")
	}
}

// TestViewHandsTheCanvasToTheTerminal is THEME-03's headline invariant: the new
// Background role is not decorative, it reaches the screen. It reaches it as the
// View's BackgroundColor rather than as a Style, because nothing kubecom draws
// covers the whole canvas — the gap between the panes, a pane's border, the tail
// of a short line and every cell of an unfilled overlay row are all default
// background, and only the terminal's own default paints those (D249).
//
// Both frames are asserted, and the pre-size one is the load-bearing half: it is
// what the terminal shows for the whole first paint, and it is the return a new
// property is easiest to forget.
func TestViewHandsTheCanvasToTheTerminal(t *testing.T) {
	for _, th := range styles.Themes() {
		m := sizedWith(t, WithTheme(th))
		if got := m.View().BackgroundColor; got != th.Background {
			t.Errorf("theme %q: sized View BackgroundColor = %v, want the theme's canvas %v",
				th.Name, got, th.Background)
		}
		if got := New(WithTheme(th)).View().BackgroundColor; got != th.Background {
			t.Errorf("theme %q: pre-size View BackgroundColor = %v, want the theme's canvas %v",
				th.Name, got, th.Background)
		}
	}
}

// TestApplyStylesRepaintsTheCanvas is the runtime half. A theme picked from
// inside kubecom fans out through applyStyles to every component (M4-12b), but
// the canvas is not a component — it is read off m.styles by View on each frame,
// which is what makes it follow a live switch with nothing added to the fan-out.
// The test exists so a later leg that caches the color into a field learns that
// it has to invalidate it.
func TestApplyStylesRepaintsTheCanvas(t *testing.T) {
	m := sizedWith(t)
	if got, want := m.View().BackgroundColor, styles.DefaultTheme().Background; got != want {
		t.Fatalf("BackgroundColor = %v, want the default canvas %v", got, want)
	}
	m.applyStyles(styles.New(styles.MonokaiTheme()))
	if got, want := m.View().BackgroundColor, styles.MonokaiTheme().Background; got != want {
		t.Errorf("after a live restyle BackgroundColor = %v, want monokai's canvas %v", got, want)
	}
}

// TestZeroThemeLeavesTheTerminalAlone pins the degradation: a Theme with no
// Background hands the terminal nil, which bubbletea renders as "reset to your
// own default" rather than as black. That is kubecom's behavior before THEME-03
// and the only sane reading of an unset role — a zero Theme must not repaint the
// user's terminal in whatever a nil color coerces to.
func TestZeroThemeLeavesTheTerminalAlone(t *testing.T) {
	if got := sizedWith(t, WithTheme(styles.Theme{})).View().BackgroundColor; got != nil {
		t.Errorf("a zero Theme set BackgroundColor = %v, want nil (reset to the terminal's own)", got)
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

// fakeThemePersister is a hermetic ThemePersister: it records the names it was asked
// to write and can be made to fail. The real one (cmd/kubecom's configThemePersister)
// rewrites config.yaml, so the seam is what keeps the write-back testable without
// touching the user's config (D18).
type fakeThemePersister struct {
	names []string
	err   error
}

func (f *fakeThemePersister) PersistTheme(name string) error {
	f.names = append(f.names, name)
	return f.err
}

// themeLabelFor reads the open stage's row for a theme back out through the label
// map, which is the only thing a pick can resolve through — the picker's SelectedMsg
// carries a label, not a theme (D65).
func themeLabelFor(t *testing.T, m Model, theme string) string {
	t.Helper()
	for label, name := range m.themeByLabel {
		if name == theme {
			return label
		}
	}
	t.Fatalf("no stage row maps to theme %q (rows: %v)", theme, m.themeByLabel)
	return ""
}

// themeCmdMsgs runs a theme-pick batch and returns the messages its sub-commands
// produced *quickly*. The batch also carries the notice's auto-clear tick, which
// resolves to a noticeClearMsg (the toast duration is shrunk to ~0 in sizedWith,
// so the tick returns instantly rather than blocking for errorDisplay); it is
// the auto-clear, not a write-back result, so it is filtered like the
// copiedClipboard precedent filters the blocking tick — only the instant ones
// (the write-back) are collected.
func themeCmdMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { ch <- c() }(c)
		select {
		case m := <-ch:
			switch m.(type) {
			case noticeClearMsg, errorClearMsg:
				// the toast's auto-clear — not a write-back result.
			default:
				if m != nil { // a successful write-back reports nothing
					out = append(out, m)
				}
			}
		case <-time.After(200 * time.Millisecond):
			// a genuinely blocking sub-command — skip it and try the next one.
		}
	}
	return out
}

// openThemeStage presses `T` through the real key path (D11 — the binding is
// registry-resolved, never matched raw) and returns the shell with the palette on its
// `:theme ` stage. Since PAL-05a that is the only theme surface there is, so every
// test below reaches it the way a reader does rather than by calling an opener.
func openThemeStage(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, tea.Key{Code: 'T', Text: "T"})
	if !m.cmdPicker.Active() || m.palArg != keymap.ActionTheme {
		t.Fatalf("`T` should open the palette on its theme stage, stage = %q", m.palArg)
	}
	return m
}

// TestThemeKeyOpensThePaletteThemeStage is PAL-05a's headline assertion: `T` no longer
// opens a modal of its own, it opens the one palette with the theme verb already
// committed — the same stage `:` `theme` `␣` reaches, on the same rows, seeded
// synchronously since the palettes are compiled in (no seam, no Cmd, nothing to wait
// for) and composited over the browse body like every other overlay (D95). The line the
// reader sees is the tell that this is the palette and not a picker: the prompt reads
// `:theme `.
func TestThemeKeyOpensThePaletteThemeStage(t *testing.T) {
	m := sizedWith(t)

	m, cmd := press(t, m, tea.Key{Code: 'T', Text: "T"})
	if !m.cmdPicker.Active() {
		t.Fatal("theme.switch should open the command palette")
	}
	if m.palArg != keymap.ActionTheme {
		t.Fatalf("`T` should commit the theme verb, stage = %q", m.palArg)
	}
	if msgs := pickerMsgs(cmd); len(msgs) != 0 {
		t.Errorf("the theme stage needs no async load: the registry is compiled in, got %v", msgs)
	}
	if got, want := len(m.themeByLabel), len(styles.Themes()); got != want {
		t.Errorf("stage rows = %d, want %d (%v)", got, want, m.themeByLabel)
	}
	// The stage opens in navigation mode (STORY-06d): the `:theme ` line is the
	// filter's prompt, revealed by `/`.
	m, _ = press(t, m, slash)
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, palettePrompt+"theme ") {
		t.Errorf("the key should land on the palette's pre-typed line:\n%s", view)
	}
	if !strings.Contains(view, keymap.ActionTheme.Describe()) {
		t.Errorf("the open stage should be composited over the browse body:\n%s", view)
	}
	for _, name := range styles.ThemeNames() {
		if !strings.Contains(view, name) {
			t.Errorf("theme %q missing from the stage:\n%s", name, view)
		}
	}

	// The key is sugar, not a second surface: typing the line by hand must land on the
	// very same stage (D207) — same verb committed, same prompt, same rows.
	typed := sizedWith(t)
	typed, _ = press(t, typed, tea.Key{Code: ':', Text: ":"})
	typed = typeInto(t, typed, "theme")
	typed, _ = press(t, typed, tea.Key{Code: ' ', Text: " "})
	typed, _ = press(t, typed, slash)
	if got, want := stripANSI(typed.View().Content), view; got != want {
		t.Errorf("`T` and `:theme ` should open the same stage:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestThemePickerMarksTheRenderingTheme is the D158 rule applied to themes: the marker
// names the palette on screen *now*, not the one the shell launched with, so a reader
// who has already switched once sees where they are. Getting it wrong is invisible at
// launch (they agree) and wrong from the first pick onward.
func TestThemePickerMarksTheRenderingTheme(t *testing.T) {
	m := sizedWith(t, WithTheme(styles.MonokaiTheme()))
	m = openThemeStage(t, m)

	if got := themeLabelFor(t, m, "monokai"); !strings.HasPrefix(got, "* ") {
		t.Errorf("the rendering theme should be marked, got %q", got)
	}
	if got := themeLabelFor(t, m, styles.DefaultTheme().Name); strings.HasPrefix(got, "* ") {
		t.Errorf("only the rendering theme may be marked, got %q", got)
	}

	// After a switch the marker moves with the shell, not with the launch option.
	next, _ := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "solarized-dark")})
	m = openThemeStage(t, next.(Model))
	if got := themeLabelFor(t, m, "solarized-dark"); !strings.HasPrefix(got, "* ") {
		t.Errorf("the marker did not follow the switch, got %q", got)
	}
}

// TestThemePickRepaintsAndPersists is the headline invariant of M4-12b-2: a picked
// theme reaches the screen through the M4-12b-1 fan-out *and* is written back, so the
// next launch opens on it. The repaint is asserted against the launch-time path — the
// picked shell must render exactly like one built with that theme (the M4-12b-1
// oracle) — because "the view changed" cannot tell a complete restyle from a partial
// one. The write runs off the update loop, so nothing is persisted until the Cmd does.
func TestThemePickRepaintsAndPersists(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "monokai")})
	m = next.(Model)
	if m.cmdPicker.Active() || m.themeByLabel != nil {
		t.Error("the pick should close the picker and drop its label map")
	}
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("the shell's styles were not repointed: %q", got)
	}
	if !m.status.HasNotice() {
		t.Error("a switch should say which theme it landed on")
	}
	// The launch-time oracle (M4-12b-1): a shell built with the theme, carrying the
	// same status-bar notice, must render byte-identically to the switched one. The
	// notice has to be set on the reference too — it is the one thing the pick changes
	// besides the palette, and comparing without it would only prove the bars differ.
	want := sizedWith(t, WithTheme(styles.MonokaiTheme()))
	want.surfaceNotice("theme monokai")
	if got := m.View().Content; got != want.View().Content {
		t.Errorf("a picked theme must render like one built at launch — a component was left behind:\ngot:\n%q\nwant:\n%q", got, want.View().Content)
	}
	if len(fp.names) != 0 {
		t.Errorf("the config was written on the update loop: %v", fp.names)
	}
	if cmd == nil {
		t.Fatal("the pick should issue the write-back (and its notice) as a Cmd")
	}
	if msgs := themeCmdMsgs(t, cmd); len(msgs) != 0 {
		t.Errorf("a successful write-back should produce no message, got %v", msgs)
	}
	if len(fp.names) != 1 || fp.names[0] != "monokai" {
		t.Errorf("PersistTheme calls = %v, want [monokai]", fp.names)
	}
}

// TestThemePickOfTheRenderingThemeIsANoOp: the marked row is choosable rather than
// hidden (D158's rule), so choosing it must cost nothing — no repaint, and above all
// no config write, since a picker opened to *look* at the theme list would otherwise
// rewrite the user's config on the way out.
func TestThemePickOfTheRenderingThemeIsANoOp(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithTheme(styles.MonokaiTheme()), WithThemePersister(fp))
	m = openThemeStage(t, m)
	before := m.View().Content

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "monokai")})
	m = next.(Model)
	if cmd != nil {
		t.Error("re-picking the rendering theme should issue no work")
	}
	if len(fp.names) != 0 {
		t.Errorf("re-picking the rendering theme wrote the config: %v", fp.names)
	}
	if m.cmdPicker.Active() {
		t.Error("the pick should still close the picker")
	}
	if got := m.View().Content; got == before {
		t.Error("the picker should have closed, so the view must differ") // sanity, not the point
	}
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Errorf("the theme changed: %q", got)
	}
}

// TestThemeSwitchWithoutAPersisterStillRepaints: unlike every other picker in the
// shell, this gesture is never inert. The seam only buys *persistence*, so a build
// with no config file to write (or a launcher that wires none) still switches themes
// for the session rather than doing nothing.
func TestThemeSwitchWithoutAPersisterStillRepaints(t *testing.T) {
	m := sizedWith(t)
	m = openThemeStage(t, m)

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "monokai")})
	m = next.(Model)
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Fatalf("the theme did not apply without a persister: %q", got)
	}
	if cmd == nil {
		t.Error("the notice's auto-clear tick is still expected")
	}
	if !m.status.HasNotice() {
		t.Error("the switch should still be announced")
	}
}

// TestThemeWriteBackFailureKeepsTheTheme is the principle-3 half: an unwritable config
// (read-only file, vanished directory) must not undo the repaint the reader already
// sees. The failure is toasted — and logged, like every surfaced error (D159) — while
// the session keeps the theme.
func TestThemeWriteBackFailureKeepsTheTheme(t *testing.T) {
	fp := &fakeThemePersister{err: errors.New("permission denied")}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)

	next, cmd := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "monokai")})
	m = next.(Model)
	msgs := themeCmdMsgs(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("a failed write-back should produce one message, got %v", msgs)
	}
	e, ok := msgs[0].(ErrorMsg)
	if !ok {
		t.Fatalf("write-back failure produced %T, want ErrorMsg", msgs[0])
	}
	if !strings.Contains(e.Message(), "permission denied") {
		t.Errorf("the toast should carry the write failure, got %q", e.Message())
	}
	next, _ = m.Update(e) // routed like the shell routes any ErrorMsg (D74)
	m = next.(Model)
	if !m.status.HasError() {
		t.Error("a failed write-back should surface a toast")
	}
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Errorf("a failed write-back must not revert the applied theme: %q", got)
	}
}

// TestThemeStageCancelClosesOutright: one esc out of a key-opened stage closes the
// palette (D233, superseding D207 pt 2 for esc). `T` is a way into the palette because
// **backspace** rewinds to the verbs — TestThemeStageBackspaceRewinds below — while esc
// keeps meaning what it means everywhere else in the app: back out to where you were,
// which for someone who pressed `T` is the browse view and not a verb list they never
// saw. The esc may not touch the theme or the config either way.
func TestThemeStageCancelClosesOutright(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)

	next, _ := m.Update(picker.CancelledMsg{Kind: commandPickerKind})
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Error("esc out of a key-opened stage should close the palette")
	}
	if m.palArg != "" {
		t.Errorf("closing should clear the stage, stage = %q", m.palArg)
	}
	if got := m.styles.Theme.Name; got != styles.DefaultTheme().Name {
		t.Errorf("cancelling changed the theme: %q", got)
	}
	if len(fp.names) != 0 {
		t.Errorf("cancelling wrote the config: %v", fp.names)
	}
}

// TestThemeStageBackspaceRewinds keeps the half of D207 pt 2 that D233 did not take:
// backspace on the empty argument still uncommits the verb and brings the verb list
// back, so a key pressed by mistake is one keystroke from every other verb. Backspace
// edits the line — erasing the committed `theme ` word is what it means there — where
// esc backs out of the surface.
func TestThemeStageBackspaceRewinds(t *testing.T) {
	m := sizedWith(t, WithThemePersister(&fakeThemePersister{}))
	m = openThemeStage(t, m)

	m, _ = press(t, m, tea.Key{Code: tea.KeyBackspace})
	if !m.cmdPicker.Active() || m.palArg != "" {
		t.Fatalf("backspace should rewind to the verb list, stage = %q", m.palArg)
	}
	if got := stripANSI(m.cmdPicker.View()); !strings.Contains(got, keymap.ActionResources.Describe()) {
		t.Errorf("the rewound palette should show the verbs:\n%s", got)
	}
}

// moveThemeCursor moves the palette's cursor to the row for the named theme and
// returns the shell. Arrows are the only way to move the cursor in the palette: the
// filter is always open, so a bound letter like `j` types into it rather than
// navigating (D73/D194 pt 2). The walk goes to the top first (the first registry row)
// and then down, so a target above the current cursor is reachable without depending
// on list wrap behaviour.
func moveThemeCursor(t *testing.T, m Model, theme string) Model {
	t.Helper()
	want := themeLabelFor(t, m, theme)
	for i := 0; i < len(styles.Themes()) && m.cmdPicker.Len() > 0; i++ {
		if v, _ := m.cmdPicker.Selected(); v == want {
			return m
		}
		m, _ = press(t, m, tea.Key{Code: tea.KeyUp})
	}
	for i := 0; i < len(styles.Themes()); i++ {
		if v, _ := m.cmdPicker.Selected(); v == want {
			return m
		}
		m, _ = press(t, m, tea.Key{Code: tea.KeyDown})
	}
	t.Fatalf("the cursor never reached theme %q", theme)
	return m
}

// TestThemePreviewRepaintsAsTheCursorMoves is the headline invariant of the live
// preview: the shell renders the theme under the palette's cursor *before* enter —
// feedback `2026-08-09-theme-picker-live-preview` — so comparing palettes is move,
// look, move again instead of open, enter, look, reopen. The preview repaints (the
// theme the shell reports changes) but neither persists nor announces, and the
// picker stays open. Moving back onto the anchor row previews the anchor again, so
// esc after a tour lands exactly where the reader started.
func TestThemePreviewRepaintsAsTheCursorMoves(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)
	first := styles.Themes()[1].Name // the row below default, whatever it is

	m = moveThemeCursor(t, m, first)
	if got := m.styles.Theme.Name; got != first {
		t.Fatalf("the shell should render the theme under the cursor, got %q want %q", got, first)
	}
	if m.cmdPicker.Active() != true {
		t.Error("a preview must not close the picker")
	}
	if len(fp.names) != 0 {
		t.Errorf("a preview wrote the config: %v", fp.names)
	}
	if m.status.HasNotice() {
		t.Error("a preview should not announce a switch")
	}

	m = moveThemeCursor(t, m, styles.DefaultTheme().Name)
	if got := m.styles.Theme.Name; got != styles.DefaultTheme().Name {
		t.Errorf("the cursor on the anchor row should render the anchor, got %q", got)
	}
	if len(fp.names) != 0 {
		t.Errorf("a preview wrote the config: %v", fp.names)
	}
}

// TestThemePreviewIssuesNoWork pins that a preview is pure repaint: a navigation key
// on the theme stage produces no command at all — nothing ticks, nothing persists,
// nothing probes. The picker's own cursor update is the only effect.
func TestThemePreviewIssuesNoWork(t *testing.T) {
	m := sizedWith(t)
	m = openThemeStage(t, m)

	if _, cmd := press(t, m, tea.Key{Code: tea.KeyDown}); cmd != nil {
		t.Errorf("a preview move should issue no command, got %v", cmd)
	}
}

// TestThemePreviewFollowsTypingNarrowing covers the other way the selection can move:
// typing in the palette's filter re-narrows the list and drops the cursor on the top
// match, so the preview follows keystrokes as well as arrows — a reader who types a
// theme's name sees it applied before pressing enter.
func TestThemePreviewFollowsTypingNarrowing(t *testing.T) {
	m := sizedWith(t)
	m = openThemeStage(t, m)
	target := styles.Themes()[1].Name

	m, _ = press(t, m, slash) // the stage is navigation mode (STORY-06d); `/` opens the field
	m = typeInto(t, m, target)
	if got := m.styles.Theme.Name; got != target {
		t.Errorf("narrowing to %q should preview it, shell renders %q", target, got)
	}
	if !m.cmdPicker.Active() {
		t.Error("a narrowing preview must not close the picker")
	}
}

// TestThemePreviewCommitPersists is the other half of the pair: the repaint a preview
// already did is not a switch until enter, and enter is the switch — notice, config
// write-back and all. A theme the preview rendered is persisted on commit exactly
// like one picked without a preview (M4-12b-2), because the preview's repaint must
// not be silently lost on restart.
func TestThemePreviewCommitPersists(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)
	theme := styles.Themes()[1].Name
	m = moveThemeCursor(t, m, theme)
	if got := m.styles.Theme.Name; got != theme {
		t.Fatalf("precondition: the shell should already render %q, got %q", theme, got)
	}

	m, cmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	sel, ok := pickerMsg(t, cmd).(picker.SelectedMsg)
	if !ok {
		t.Fatalf("enter emitted %T, want picker.SelectedMsg", cmd)
	}
	upd, cmd := m.Update(sel)
	m = upd.(Model)
	if m.cmdPicker.Active() {
		t.Error("enter should commit and close the palette")
	}
	if got := m.styles.Theme.Name; got != theme {
		t.Fatalf("committed theme = %q, want %q", got, theme)
	}
	if !m.status.HasNotice() {
		t.Error("a commit should announce the switch")
	}
	if cmd == nil {
		t.Fatal("the commit should issue the write-back (and its notice) as a Cmd")
	}
	if msgs := themeCmdMsgs(t, cmd); len(msgs) != 0 {
		t.Errorf("a successful write-back should produce no message, got %v", msgs)
	}
	if len(fp.names) != 1 || fp.names[0] != theme {
		t.Errorf("PersistTheme calls = %v, want [%s]", fp.names, theme)
	}
}

// TestThemePreviewCommitOfTheAnchorIsANoOp is the no-op rule under the preview: a
// reader who browsed away and came back to the anchor row must be able to enter on
// it — the marked row is choosable (D158) — and the entry costs nothing because the
// shell already renders it. Nothing is written even though a preview happened in
// between.
func TestThemePreviewCommitOfTheAnchorIsANoOp(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)
	m = moveThemeCursor(t, m, styles.Themes()[1].Name)
	m = moveThemeCursor(t, m, styles.DefaultTheme().Name)

	m, cmd := press(t, m, tea.Key{Code: tea.KeyEnter})
	upd, cmd := m.Update(pickerMsg(t, cmd).(picker.SelectedMsg))
	m = upd.(Model)
	if m.cmdPicker.Active() {
		t.Error("the pick should close the palette")
	}
	if got := m.styles.Theme.Name; got != styles.DefaultTheme().Name {
		t.Errorf("committing the anchor changed the theme: %q", got)
	}
	if len(fp.names) != 0 {
		t.Errorf("committing the anchor wrote the config: %v", fp.names)
	}
	if cmd != nil {
		t.Errorf("committing the anchor should issue no work, got %v", cmd)
	}
}

// TestThemePreviewCancelRestoresAnchor: esc out of a key-opened stage is the cancel
// half of the preview — the shell returns to the theme that was rendering when the
// stage opened, silently, because the reader only looked (feedback
// 2026-08-09-theme-picker-live-preview). No config write, no notice: a cancel is not
// a switch.
func TestThemePreviewCancelRestoresAnchor(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)
	theme := styles.Themes()[1].Name
	m = moveThemeCursor(t, m, theme)
	if got := m.styles.Theme.Name; got != theme {
		t.Fatalf("precondition: the shell should render %q, got %q", theme, got)
	}

	next, _ := m.Update(picker.CancelledMsg{Kind: commandPickerKind})
	m = next.(Model)
	if m.cmdPicker.Active() {
		t.Error("esc should close the palette")
	}
	if got := m.styles.Theme.Name; got != styles.DefaultTheme().Name {
		t.Errorf("esc should restore the theme active when the stage opened, got %q", got)
	}
	if len(fp.names) != 0 {
		t.Errorf("cancelling wrote the config: %v", fp.names)
	}
	if m.status.HasNotice() {
		t.Error("a cancel should not announce a switch")
	}
}

// TestThemePreviewRewindRestoresAnchor covers the other way out of a *typed* stage:
// esc there rewinds to the verb list (D233) rather than closing, and backspace on the
// empty argument rewinds too (D207 pt 2) — both leave the theme stage, so both must
// restore the anchor just like a close does. A reader who browsed palettes and
// rewound is back on the theme they had.
func TestThemePreviewRewindRestoresAnchor(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m, _ = press(t, m, tea.Key{Code: ':', Text: ":"})
	m = typeInto(t, m, "theme")
	m, _ = press(t, m, tea.Key{Code: ' ', Text: " "})
	if m.palArg != keymap.ActionTheme {
		t.Fatalf("precondition: the typed line should land on the theme stage, stage = %q", m.palArg)
	}
	theme := styles.Themes()[1].Name
	m = moveThemeCursor(t, m, theme)
	if got := m.styles.Theme.Name; got != theme {
		t.Fatalf("precondition: the shell should render %q, got %q", theme, got)
	}

	// esc from a typed stage rewinds to the verb list instead of closing (D233).
	// The picker emits the rewind as a CancelledMsg cmd, exactly as in the running
	// program; feed it back in.
	m, cmd := press(t, m, tea.Key{Code: tea.KeyEsc})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if !m.cmdPicker.Active() {
		t.Fatal("esc from a typed stage should rewind, not close")
	}
	if m.palArg != "" {
		t.Errorf("the rewind should leave the theme stage, stage = %q", m.palArg)
	}
	if got := m.styles.Theme.Name; got != styles.DefaultTheme().Name {
		t.Errorf("the rewind should restore the anchor theme, got %q", got)
	}
	if len(fp.names) != 0 {
		t.Errorf("the rewind wrote the config: %v", fp.names)
	}
	if m.status.HasNotice() {
		t.Error("a rewind should not announce a switch")
	}
}

func TestThemeSwitchSurvivesAContextSwitch(t *testing.T) {
	fp := &fakeThemePersister{}
	m := sizedWith(t, WithThemePersister(fp))
	m = openThemeStage(t, m)
	next, _ := m.Update(picker.SelectedMsg{Kind: commandPickerKind, Value: themeLabelFor(t, m, "monokai")})
	m = next.(Model)

	m.resetCluster()
	if got := m.styles.Theme.Name; got != "monokai" {
		t.Errorf("a cluster teardown reverted the theme: %q", got)
	}
	if m.themePersister == nil {
		t.Error("a cluster teardown dropped the theme persister")
	}
}
