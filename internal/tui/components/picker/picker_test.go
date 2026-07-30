package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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

func TestFilterOpensAndNarrows(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public", "monitoring")
	m.Show()
	if m.Filtering() {
		t.Fatal("picker should not start filtering")
	}
	m, _ = m.Update(keymap.ActionFilter)
	if !m.Filtering() {
		t.Fatal("app.filter should open the filter field")
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
	m2.Show()
	m2, _ = m2.Update(keymap.ActionFilter)
	m2 = typeFilter(m2, "SYS")
	if got := m2.Len(); got != 1 {
		t.Fatalf("case-insensitive 'SYS' Len() = %d, want 1", got)
	}
}

func TestFilterBackClearsThenCancels(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
	m = typeFilter(m, "public")
	if got := m.Len(); got != 1 {
		t.Fatalf("filtered Len() = %d, want 1", got)
	}
	// First back clears the filter (does not cancel the picker) and restores all.
	m, cmd := m.Update(keymap.ActionBack)
	if msg := msgFrom(cmd); msg != nil {
		t.Fatalf("back while filtering emitted %T, want none", msg)
	}
	if m.Filtering() {
		t.Fatal("back while filtering should close the filter")
	}
	if got := m.Len(); got != 3 {
		t.Fatalf("after clearing filter, Len() = %d, want 3 (all restored)", got)
	}
	// Second back now cancels the picker.
	_, cmd = m.Update(keymap.ActionBack)
	if _, ok := msgFrom(cmd).(CancelledMsg); !ok {
		t.Fatalf("back after clear produced %T, want CancelledMsg", msgFrom(cmd))
	}
}

func TestFilterDrillInSelectsFilteredValue(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
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
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
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
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
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
	m := newTestModel("default", "kube-system")
	m.Show()
	// No filter open: raw keys are ignored and the list is untouched.
	m = typeFilter(m, "kube")
	if got := m.Len(); got != 2 {
		t.Fatalf("UpdateFilter changed the list while not filtering: Len() = %d, want 2", got)
	}
}

func TestHideClosesFilter(t *testing.T) {
	m := newTestModel("default", "kube-system", "kube-public")
	m.Show()
	m, _ = m.Update(keymap.ActionFilter)
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
