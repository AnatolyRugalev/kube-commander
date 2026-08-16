package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

func newTestModel(values ...string) Model {
	m := New(styles.Default(), "namespace")
	m.SetSize(80, 24)
	m.SetItems(values)
	return m
}

// msgFrom runs a command (if any) and returns the message it produced, or nil.
func msgFrom(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestNewHiddenEmptyView(t *testing.T) {
	m := New(styles.Default(), "namespace")
	if m.Active() {
		t.Fatal("a new picker should start hidden")
	}
	if m.Kind() != "namespace" {
		t.Fatalf("Kind() = %q, want namespace", m.Kind())
	}
	// Hidden picker renders nothing even once sized/seeded.
	m.SetSize(80, 24)
	m.SetItems([]string{"default", "kube-system"})
	if v := m.View(); v != "" {
		t.Fatalf("hidden picker View() = %q, want empty", v)
	}
}

func TestShowRendersTitleAndItems(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m.Show()
	if !m.Active() {
		t.Fatal("Show() should make the picker active")
	}
	v := m.View()
	if v == "" {
		t.Fatal("active, sized, seeded picker rendered empty")
	}
	for _, want := range []string{"Namespace", "default", "kube-system", "kube-public"} {
		if !strings.Contains(v, want) {
			t.Fatalf("View() missing %q; got:\n%s", want, v)
		}
	}
}

func TestFirstItemSelectedAfterSetItems(t *testing.T) {
	m := newTestModel("default", "kube-system")
	if got := m.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	v, ok := m.Selected()
	if !ok || v != "default" {
		t.Fatalf("Selected() = %q,%v; want default,true", v, ok)
	}
}

func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel("a", "b", "c")
	m.Show()

	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "b" {
		t.Fatalf("after down, Selected() = %q, want b", v)
	}
	m, _ = m.Update(keymap.ActionBottom)
	if v, _ := m.Selected(); v != "c" {
		t.Fatalf("after bottom, Selected() = %q, want c", v)
	}
	m, _ = m.Update(keymap.ActionTop)
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("after top, Selected() = %q, want a", v)
	}
	m, _ = m.Update(keymap.ActionUp) // already at top: clamped, stays
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("after up at top, Selected() = %q, want a", v)
	}
}

func TestDrillInEmitsSelectedMsg(t *testing.T) {
	m := newTestModel("default", "kube-system")
	m.Show()
	m, _ = m.Update(keymap.ActionDown)
	_, cmd := m.Update(keymap.ActionDrillIn)
	msg := msgFrom(cmd)
	sel, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want SelectedMsg", msg)
	}
	if sel.Kind != "namespace" || sel.Value != "kube-system" {
		t.Fatalf("SelectedMsg = %+v, want {namespace kube-system}", sel)
	}
}

func TestBackEmitsCancelledMsg(t *testing.T) {
	m := newTestModel("default")
	m.Show()
	_, cmd := m.Update(keymap.ActionBack)
	msg := msgFrom(cmd)
	c, ok := msg.(CancelledMsg)
	if !ok {
		t.Fatalf("back produced %T, want CancelledMsg", msg)
	}
	if c.Kind != "namespace" {
		t.Fatalf("CancelledMsg.Kind = %q, want namespace", c.Kind)
	}
}

func TestInactivePickerIgnoresActions(t *testing.T) {
	m := newTestModel("a", "b")
	// not shown
	m, cmd := m.Update(keymap.ActionDrillIn)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("inactive picker emitted %T on drill-in, want none", msg)
	}
	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "a" {
		t.Fatalf("inactive picker moved cursor to %q, want a", v)
	}
}

func TestDrillInEmptyPickerNoMsg(t *testing.T) {
	m := New(styles.Default(), "namespace")
	m.SetSize(80, 24)
	m.Show()
	_, cmd := m.Update(keymap.ActionDrillIn)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("empty picker emitted %T on drill-in, want none", msg)
	}
}

// typeFilter feeds each rune of s to the filter field as a raw key press.
func typeFilter(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.UpdateFilter(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return m
}

// showFiltered reveals the picker with its filter open — the type-to-filter state a
// filter test needs. Navigation mode is the default open state since STORY-06d
// (D272); a test of the filter itself opens the field with the same `/` (ActionFilter)
// a reader would press.
func showFiltered(m Model) Model {
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
	return m
}

// TestShowRevealsInNavigationMode is STORY-06d's headline property: the picker opens
// with the list focused and the filter closed, so the next letter navigates instead
// of typing — the walk's dead `j`s (D272). `/` (ActionFilter) opens the field.
func TestShowRevealsInNavigationMode(t *testing.T) {
	m := newTestModel("default", "kube-system")
	if m.Filtering() {
		t.Fatal("a hidden picker should not be filtering")
	}
	m.Show()
	if !m.Active() {
		t.Fatal("Show() should make the picker active")
	}
	if m.Filtering() {
		t.Fatal("Show() should leave the filter closed — navigation mode, not type-to-filter")
	}
	// j/k navigate in this state (j is ActionDown through the keymap).
	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "kube-system" {
		t.Fatalf("j should navigate the list in navigation mode, selected %q", v)
	}
	// `/` opens the field; typing then narrows.
	m, _ = m.Update(keymap.ActionFilter)
	if !m.Filtering() {
		t.Fatal("ActionFilter should open the filter field")
	}
	m = typeFilter(m, "sys")
	if got := m.Len(); got != 1 {
		t.Fatalf("typing after `/` left %d items, want 1", got)
	}

	// ShowFiltered is the type-to-filter shape: the field opens with the picker, the
	// next keystroke narrows without a `/` first.
	p := newTestModel("default", "kube-system")
	p.ShowFiltered()
	if !p.Filtering() {
		t.Fatal("ShowFiltered() should open the filter field")
	}
	p = typeFilter(p, "sys")
	if got := p.Len(); got != 1 {
		t.Fatalf("typing into a ShowFiltered picker left %d items, want 1", got)
	}
}

// TestSelectValuePreselectsTheCurrentChoice is STORY-06d's third ask: a picker that
// opens in navigation mode can land its cursor on the current choice, so the reader
// sees the thing they are about to change already highlighted. A value absent from
// the list (a remembered scope the cluster no longer serves) leaves the cursor at the
// top rather than failing.
func TestSelectValuePreselectsTheCurrentChoice(t *testing.T) {
	m := newTestModel("default", "kube-system", "monitoring")
	m.SelectValue("kube-system")
	if v, _ := m.Selected(); v != "kube-system" {
		t.Fatalf("SelectValue moved the cursor to %q, want kube-system", v)
	}
	// Selecting a value not in the list is a no-op — the cursor stays put.
	m.SelectValue("gone")
	if v, _ := m.Selected(); v != "kube-system" {
		t.Fatalf("a missing value moved the cursor to %q, want it to stay on kube-system", v)
	}
	// Seeding a fresh set resets the cursor to the top, so preselection is applied
	// after seeding, not before.
	m.SetItems([]string{"default", "monitoring"})
	if v, _ := m.Selected(); v != "default" {
		t.Fatalf("after reseed the cursor should be at the top, got %q", v)
	}
}

// TestOpenFilterCloseFilterToggleModeInPlace covers the palette's in-place transitions:
// a picker showing the type-to-filter verb list enters an argument stage by closing its
// field (CloseFilter → navigation mode), and rewinds to the verb list by reopening it
// (OpenFilter → type-to-filter). Both keep the picker shown.
func TestOpenFilterCloseFilterToggleModeInPlace(t *testing.T) {
	m := newTestModel("a", "b", "c")
	m.ShowFiltered()
	if !m.Filtering() {
		t.Fatal("precondition: ShowFiltered should open the field")
	}
	m.CloseFilter()
	if m.Filtering() {
		t.Fatal("CloseFilter should close the field, returning to navigation mode")
	}
	// In navigation mode j/k navigate the still-shown list.
	m, _ = m.Update(keymap.ActionDown)
	if v, _ := m.Selected(); v != "b" {
		t.Fatalf("after CloseFilter j should navigate to b, got %q", v)
	}
	m.OpenFilter()
	if !m.Filtering() {
		t.Fatal("OpenFilter should reopen the field, returning to type-to-filter")
	}
	m = typeFilter(m, "a")
	if got := m.Len(); got != 1 {
		t.Fatalf("after OpenFilter typing should narrow, Len() = %d, want 1", got)
	}
}

// TestNavigationModeDrillInAndBack confirms the two ways out of a navigation-mode
// picker are intact: enter selects, esc cancels, with no filter to clear first.
func TestNavigationModeDrillInAndBack(t *testing.T) {
	m := newTestModel("a", "b", "c")
	m.Show()
	_, cmd := m.Update(keymap.ActionDrillIn)
	if sel, ok := msgFrom(cmd).(SelectedMsg); !ok || sel.Value != "a" {
		t.Fatalf("drill-in in navigation mode produced %T, want SelectedMsg{a}", msgFrom(cmd))
	}
	_, cmd = m.Update(keymap.ActionBack)
	if _, ok := msgFrom(cmd).(CancelledMsg); !ok {
		t.Fatalf("back in navigation mode produced %T, want CancelledMsg", msgFrom(cmd))
	}
}

func TestFilterRanksMatches(t *testing.T) {
	// "kube-system" contains "sys" outright; "s-y-s" is only a subsequence of the
	// other two, so both must sort below it whatever order they were seeded in.
	m := newTestModel("some-yaml-service", "kube-system", "sync-yes-status")
	m = showFiltered(m)
	m = typeFilter(m, "sys")
	if got := m.Len(); got != 3 {
		t.Fatalf("fuzzy filter matched %d values, want 3", got)
	}
	if v, _ := m.Selected(); v != "kube-system" {
		t.Fatalf("best match = %q, want kube-system (the only contiguous match)", v)
	}
	// The fuzzy half is what makes an abbreviation reach its value at all.
	m2 := newTestModel("default", "kube-system", "monitoring")
	m2 = showFiltered(m2)
	m2 = typeFilter(m2, "ksys")
	if got := m2.Len(); got != 1 {
		t.Fatalf("abbreviation 'ksys' matched %d values, want 1", got)
	}
	if v, _ := m2.Selected(); v != "kube-system" {
		t.Fatalf("'ksys' selected %q, want kube-system", v)
	}
}

func TestFilterOpensAndNarrows(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public", "monitoring")
	m = showFiltered(m)
	if !m.Filtering() {
		t.Fatal("ActionFilter should open the filter field")
	}
	m = typeFilter(m, "kube")
	if got := m.Len(); got != 2 {
		t.Fatalf("after filtering by 'kube', Len() = %d, want 2", got)
	}
	if v, _ := m.Selected(); v != "kube-system" {
		t.Fatalf("filtered selection = %q, want kube-system (first match)", v)
	}
	// The filter matches case-insensitively on a substring anywhere in the value.
	m2 := newTestModel("default", "kube-system", "monitoring")
	m2 = showFiltered(m2)
	m2 = typeFilter(m2, "SYS")
	if got := m2.Len(); got != 1 {
		t.Fatalf("case-insensitive 'SYS' Len() = %d, want 1", got)
	}
}

func TestFilterBackClearsThenCancels(t *testing.T) {
	// The navigation-mode picker (STORY-06d default): a filter opened with `/` is an
	// overlay, so esc closes it back to the list, and a second esc cancels the picker.
	m := newTestModel("default", "kube-system", "kube-public")
	m = showFiltered(m)
	m = typeFilter(m, "public")
	if got := m.Len(); got != 1 {
		t.Fatalf("filtered Len() = %d, want 1", got)
	}
	m, cmd := m.Update(keymap.ActionBack)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("back with a query emitted %T, want none", msg)
	}
	if m.Filtering() {
		t.Fatal("back on a navigation-mode picker should close the overlay filter")
	}
	if got := m.Len(); got != 3 {
		t.Fatalf("after closing the filter, Len() = %d, want 3 (all restored)", got)
	}
	// Second back — no filter to clear — cancels the picker.
	_, cmd = m.Update(keymap.ActionBack)
	if _, ok := msgFrom(cmd).(CancelledMsg); !ok {
		t.Fatalf("back after clearing produced %T, want CancelledMsg", msgFrom(cmd))
	}

	// The type-to-filter shape (ShowFiltered, the verb list): esc empties the query
	// and keeps the field open, so esc-esc dismisses.
	p := newTestModel("8080 http", "9090 metrics")
	p.ShowFiltered()
	p = typeFilter(p, "http")
	p, cmd = p.Update(keymap.ActionBack)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("type-to-filter back while filtering emitted %T, want none", msg)
	}
	if !p.Filtering() {
		t.Fatal("type-to-filter back should clear the query, not close the field")
	}
	_, cmd = p.Update(keymap.ActionBack)
	if _, ok := msgFrom(cmd).(CancelledMsg); !ok {
		t.Fatalf("type-to-filter back after clear produced %T, want CancelledMsg", msgFrom(cmd))
	}
}

func TestFilterDrillInSelectsFilteredValue(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m = showFiltered(m)
	m = typeFilter(m, "system")
	_, cmd := m.Update(keymap.ActionDrillIn)
	sel, ok := msgFrom(cmd).(SelectedMsg)
	if !ok {
		t.Fatalf("drill-in produced %T, want SelectedMsg", msgFrom(cmd))
	}
	if sel.Value != "kube-system" {
		t.Fatalf("SelectedMsg.Value = %q, want kube-system", sel.Value)
	}
}

func TestFilterNoMatchDrillInNoMsg(t *testing.T) {
	m := newTestModel("default", "kube-system")
	m = showFiltered(m)
	m = typeFilter(m, "zzz")
	if got := m.Len(); got != 0 {
		t.Fatalf("no-match filter Len() = %d, want 0", got)
	}
	_, cmd := m.Update(keymap.ActionDrillIn)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("drill-in with no matches emitted %T, want none", msg)
	}
}

func TestFilterViewShowsInputLine(t *testing.T) {
	m := newTestModel("default", "kube-system")
	m = showFiltered(m)
	m = typeFilter(m, "kube")
	v := m.View()
	if !strings.Contains(v, "/") {
		t.Fatalf("filtering View() missing the filter prompt; got:\n%s", v)
	}
	if !strings.Contains(v, "kube-system") {
		t.Fatalf("filtering View() missing the matched item; got:\n%s", v)
	}
	if strings.Contains(v, "default") {
		t.Fatalf("filtering View() still shows the non-matching item; got:\n%s", v)
	}
}

func TestUpdateFilterInertWhenNotFiltering(t *testing.T) {
	// Hidden picker: UpdateFilter is inert whatever the mode.
	m := newTestModel("default", "kube-system")
	m = typeFilter(m, "kube")
	if got := m.Len(); got != 2 {
		t.Fatalf("UpdateFilter changed the list while hidden: Len() = %d, want 2", got)
	}
	// Shown picker in navigation mode with no filter open (STORY-06d default): raw
	// keys are ignored too — the list is navigated by j/k, not typed into.
	p := newTestModel("default", "kube-system")
	p.Show()
	p = typeFilter(p, "kube")
	if got := p.Len(); got != 2 {
		t.Fatalf("UpdateFilter changed the list while not filtering: Len() = %d, want 2", got)
	}
}

func TestHideClosesFilter(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m = showFiltered(m)
	m = typeFilter(m, "public")
	m.Hide()
	if m.Filtering() {
		t.Fatal("Hide() should close the filter")
	}
	m.Show()
	if got := m.Len(); got != 3 {
		t.Fatalf("after Hide/Show, Len() = %d, want 3 (filter cleared)", got)
	}
}

// TestSetStylesRestylesRowsAndKeepsPlace is the trap this component's SetStyles exists
// to avoid (M4-12b-1): every row — including the cursor's Selection bar — is drawn by
// the list's itemDelegate, which list.New was handed a *copy* of the old Styles. A
// SetStyles that only assigned m.styles would repaint the title and the frame while the
// rows below stayed in the departed theme. Swapping the delegate must not disturb the
// list's contents or the cursor.
func TestSetStylesRestylesRowsAndKeepsPlace(t *testing.T) {
	m := newTestModel("default", "kube-system", "web")
	m.Show()
	m, _ = m.Update(keymap.ActionDown)
	before, ok := m.Selected()
	if !ok {
		t.Fatal("precondition: a row should be selected")
	}

	mono := styles.New(styles.MonokaiTheme())
	m.SetStyles(mono)
	view := m.View()

	// The cursor row is drawn through Selection; under the new theme that is the new
	// theme's bar. Width-padded by the delegate, so match on the escape prefix.
	if want := mono.Selection.Render(""); !strings.Contains(view, strings.TrimSuffix(want, "\x1b[m")) {
		t.Errorf("the cursor row was not repainted by the new theme's Selection style:\n%q", view)
	}
	if got, _ := m.Selected(); got != before {
		t.Errorf("the cursor moved on restyle: %q, want %q", got, before)
	}
	if got := m.Len(); got != 3 {
		t.Errorf("the item set changed on restyle: Len() = %d, want 3", got)
	}
}

// newAliasModel is newTestModel for the alias-carrying shape (CRD-PIN-04): a picker
// whose values answer to names the reader may type but never sees on the row.
func newAliasModel(items ...Item) Model {
	m := New(styles.Default(), "resource")
	m.SetSize(80, 24)
	m.SetItemsWithAliases(items)
	return m
}

// TestFilterMatchesAnAlias is CRD-PIN-04's headline property: a query that matches
// only a value's alias still finds it. "externalsecrets" is not a subsequence of
// "ExternalSecret" (the trailing plural `s` has nothing to match), so before aliases
// the name kubectl takes found nothing at all.
func TestFilterMatchesAnAlias(t *testing.T) {
	m := newAliasModel(
		Item{Label: "ExternalSecret", Aliases: []string{"externalsecrets", "es", "external-secrets.io"}},
		Item{Label: "Secret", Aliases: []string{"secrets"}},
	)
	m = showFiltered(m)
	m = typeFilter(m, "externalsecrets")
	if got := m.Len(); got != 1 {
		t.Fatalf("query %q matched %d rows, want 1 (ExternalSecret via its plural)", "externalsecrets", got)
	}
	if v, _ := m.Selected(); v != "ExternalSecret" {
		t.Fatalf("selected %q, want ExternalSecret", v)
	}
}

// TestAliasNeverShowsOnTheRow proves an alias is match-only: it widens what the query
// reaches without adding a word to the list, which is the whole reason it is not simply
// appended to the label.
func TestAliasNeverShowsOnTheRow(t *testing.T) {
	m := newAliasModel(Item{Label: "ExternalSecret", Aliases: []string{"externalsecrets", "es"}})
	m = showFiltered(m)
	m = typeFilter(m, "es")
	view := m.View()
	if !strings.Contains(view, "ExternalSecret") {
		t.Fatalf("the matched row should render its label; got:\n%s", view)
	}
	if strings.Contains(view, "externalsecrets") {
		t.Fatalf("an alias leaked into the rendered row:\n%s", view)
	}
}

// TestAliasHitRanksBesideALabelHit pins the unpenalised rule: a row reached through an
// alias competes on the alias's own score, so an exact short name beats a scattered
// subsequence of another row's label. Query "es": ExternalSecret's short name is the
// whole needle, while "Secret" only matches it scattered (e...s is not contiguous
// there) — so the short name must come first.
func TestAliasHitRanksBesideALabelHit(t *testing.T) {
	m := newAliasModel(
		Item{Label: "Secret", Aliases: []string{"secrets"}},
		Item{Label: "ExternalSecret", Aliases: []string{"externalsecrets", "es"}},
	)
	m = showFiltered(m)
	m = typeFilter(m, "es")
	if got := m.Len(); got < 2 {
		t.Fatalf("query %q matched %d rows, want both", "es", got)
	}
	if v, _ := m.Selected(); v != "ExternalSecret" {
		t.Fatalf("selected %q, want ExternalSecret (its short name is an exact hit)", v)
	}
}

// TestSetItemsClearsAliases proves the two setters land in one place: reseeding with
// plain values drops the previous stage's aliases, so a picker swapped in place (the
// command palette committing a new verb) cannot match against terms belonging to a
// list it no longer shows.
func TestSetItemsClearsAliases(t *testing.T) {
	m := newAliasModel(Item{Label: "ExternalSecret", Aliases: []string{"externalsecrets"}})
	m = showFiltered(m)
	m.SetItems([]string{"ExternalSecret"})
	m = typeFilter(m, "externalsecrets")
	if got := m.Len(); got != 0 {
		t.Fatalf("a stale alias still matched after SetItems: Len() = %d, want 0", got)
	}
}

// --- PAL-08: the name column -------------------------------------------------

// newNamedModel builds the shape the command palette seeds since PAL-08: each value
// carries the command's own name beside the description the reader picks by. Like the
// palette verb list it opens type-to-filter (the one surface whose identity is typing,
// D197), so a test can type straight at it.
func newNamedModel(items ...Item) Model {
	m := New(styles.Default(), "command")
	m.SetSize(80, 24)
	m.SetItemsWithAliases(items)
	m.ShowFiltered()
	return m
}

// rowLine returns the plain (un-styled, un-bordered) rendered line holding want, or
// fails. It is how the column tests read geometry: the interesting claim is *where*
// text lands on the row, which only the composed frame can answer.
func rowLine(t *testing.T, m Model, want string) string {
	t.Helper()
	for _, l := range strings.Split(m.View(), "\n") {
		plain := strings.Trim(ansi.Strip(l), "│")
		if strings.Contains(plain, want) {
			return plain
		}
	}
	t.Fatalf("no rendered row holds %q; got:\n%s", want, ansi.Strip(m.View()))
	return ""
}

// TestNamedItemsRenderInTwoColumns is PAL-08's headline and the feedback's own words:
// the palette showed only the description, and the command's name has to be visible
// beside it. The claim is alignment, not mere presence — two columns means every
// label starts at the same cell, whatever the names above it are.
func TestNamedItemsRenderInTwoColumns(t *testing.T) {
	m := newNamedModel(
		Item{Name: "resources.switch", Label: "Switch resource"},
		Item{Name: "app.quit", Label: "Quit"},
	)
	long := rowLine(t, m, "Switch resource")
	short := rowLine(t, m, "Quit")
	if !strings.HasPrefix(long, "resources.switch") || !strings.HasPrefix(short, "app.quit") {
		t.Fatalf("the name should open its row:\n%q\n%q", long, short)
	}
	if got, want := strings.Index(short, "Quit"), strings.Index(long, "Switch resource"); got != want {
		t.Fatalf("labels start at %d and %d — the name column is not aligned:\n%q\n%q",
			got, want, long, short)
	}
}

// TestUnnamedItemsKeepTheirSingleColumn is the other half: every picker but the
// palette seeds plain values, and none of them may grow a gutter for a column that
// holds nothing. The row still opens with the value itself.
func TestUnnamedItemsKeepTheirSingleColumn(t *testing.T) {
	m := newTestModel("default", "kube-system")
	m.Show()
	if line := rowLine(t, m, "kube-system"); !strings.HasPrefix(line, "kube-system") {
		t.Fatalf("an unnamed row should start with its value, got %q", line)
	}
}

// TestNameColumnFollowsTheVisibleSet is why the width is measured over the rows on
// screen rather than over the whole item set: a query that leaves only short-named
// commands gives the width back to their descriptions instead of holding a gutter for
// rows it is no longer showing.
func TestNameColumnFollowsTheVisibleSet(t *testing.T) {
	items := []Item{
		{Name: "resources.switch", Label: "Switch resource"},
		{Name: "app.quit", Label: "Quit"},
	}
	wide := strings.Index(rowLine(t, newNamedModel(items...), "Quit"), "Quit")
	narrowed := typeFilter(newNamedModel(items...), "quit")
	narrow := strings.Index(rowLine(t, narrowed, "Quit"), "Quit")
	if narrow >= wide {
		t.Fatalf("the name column stayed %d wide after narrowing to the short name (was %d)", narrow, wide)
	}
}

// TestFilterMatchesTheName is the reason the name is matched as well as shown: a
// reader who can see `ns.switch` will type it. "ns.sw" is not a subsequence of the
// label ("Switch namespace" has no `.`), so before PAL-08 typing what is on screen
// found nothing.
func TestFilterMatchesTheName(t *testing.T) {
	m := newNamedModel(
		Item{Name: "ns.switch", Label: "Switch namespace"},
		Item{Name: "app.quit", Label: "Quit"},
	)
	m = typeFilter(m, "ns.sw")
	if got := m.Len(); got != 1 {
		t.Fatalf("query %q matched %d rows, want 1", "ns.sw", got)
	}
	// The label is still the identity a pick resolves by (D203 pt 3) — matching the
	// name must not make the name the value.
	if v, ok := m.Selected(); !ok || v != "Switch namespace" {
		t.Fatalf("Selected() = %q,%v; want the label, not the name", v, ok)
	}
}

// TestNameSurvivesARestyle guards the one place the delegate is rebuilt from two
// inputs: a theme change replaces the row renderer, and the column it was carrying
// must come with it (a picker restyled while open otherwise loses its layout, which
// is exactly what SetStyles exists to avoid).
func TestNameSurvivesARestyle(t *testing.T) {
	m := newNamedModel(
		Item{Name: "resources.switch", Label: "Switch resource"},
		Item{Name: "app.quit", Label: "Quit"},
	)
	before := strings.Index(rowLine(t, m, "Quit"), "Quit")
	m.SetStyles(styles.Default())
	if after := strings.Index(rowLine(t, m, "Quit"), "Quit"); after != before {
		t.Fatalf("label starts at %d after a restyle, was %d — the column was dropped", after, before)
	}
}
