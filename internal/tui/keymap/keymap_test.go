package keymap

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestDefaultKeymapValid is the load-bearing invariant: the built-in defaults
// must parse and validate (no collisions). DefaultKeymap panics otherwise.
func TestDefaultKeymapValid(t *testing.T) {
	km := DefaultKeymap()
	if err := (&Keymap{bindings: km.bindings}).validateForTest(); err != nil {
		t.Fatalf("default keymap invalid: %v", err)
	}
	// Every registered action has at least one default binding.
	for _, a := range Actions() {
		if len(km.Keys(a)) == 0 {
			t.Errorf("action %q has no default binding", a)
		}
	}
}

// validateForTest re-runs build to surface any collision as an error.
func (k *Keymap) validateForTest() error {
	_, err := build(k.bindings)
	return err
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
		{tea.Key{Code: 's', Text: "s"}, ActionSort},
		{tea.Key{Code: 's', ShiftedCode: 'S', Mod: tea.ModShift}, ActionClearSort},
		{tea.Key{Code: 'm', Text: "m"}, ActionToggleMenu},
	}
	for _, tt := range tests {
		got, ok := km.Action(tt.key)
		if !ok || got != tt.want {
			t.Errorf("Action(%+v) = %q,%v; want %q", tt.key, got, ok, tt.want)
		}
	}
	// An unbound key resolves to nothing.
	if a, ok := km.Action(tea.Key{Code: 'z', Text: "z"}); ok {
		t.Errorf("Action(z) = %q, want unbound", a)
	}
}

func TestMergeOverride(t *testing.T) {
	km := DefaultKeymap()
	merged, warns, err := km.Merge(map[Action][]string{
		ActionFilter: {"t"}, // replace "/" with "t" (a free, non-nav key)
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	// New binding wins.
	if a, ok := merged.Action(tea.Key{Code: 't', Text: "t"}); !ok || a != ActionFilter {
		t.Errorf("t resolved to %q,%v; want app.filter", a, ok)
	}
	// Old default no longer resolves.
	if a, ok := merged.Action(tea.Key{Code: '/', Text: "/"}); ok {
		t.Errorf("/ still resolves to %q after override", a)
	}
	// Original keymap is untouched (Merge returns a new value).
	if a, ok := km.Action(tea.Key{Code: '/', Text: "/"}); !ok || a != ActionFilter {
		t.Errorf("original keymap mutated: / = %q,%v", a, ok)
	}
}

func TestMergeUnknownAction(t *testing.T) {
	_, _, err := DefaultKeymap().Merge(map[Action][]string{"bogus.action": {"x"}})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("want unknown-action error, got %v", err)
	}
}

func TestMergeBadToken(t *testing.T) {
	_, _, err := DefaultKeymap().Merge(map[Action][]string{ActionHelp: {"shift+x"}})
	if err == nil {
		t.Fatal("want error for bad token")
	}
}

func TestMergeCollision(t *testing.T) {
	// Bind help to "j", which nav.down already owns.
	_, _, err := DefaultKeymap().Merge(map[Action][]string{ActionHelp: {"j"}})
	if err == nil || !strings.Contains(err.Error(), "bound to both") {
		t.Fatalf("want collision error, got %v", err)
	}
	if !strings.Contains(err.Error(), "app.help") || !strings.Contains(err.Error(), "nav.down") {
		t.Errorf("collision error should name both actions: %v", err)
	}
}

// TestConfirmContextResolution proves the confirm actions resolve in their own key
// context: `y`/`enter` → confirm.accept and `n`/`esc` → confirm.decline via
// ConfirmAction, while the same keys keep their browse meaning via Action. The two
// contexts coexist without a collision even though they share chords (D132).
func TestConfirmContextResolution(t *testing.T) {
	km := DefaultKeymap()

	confirm := []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: 'y', Text: "y"}, ActionConfirmAccept},
		{tea.Key{Code: tea.KeyEnter}, ActionConfirmAccept},
		{tea.Key{Code: 'n', Text: "n"}, ActionConfirmDecline},
		{tea.Key{Code: tea.KeyEsc}, ActionConfirmDecline},
	}
	for _, tt := range confirm {
		if a, ok := km.ConfirmAction(tt.key); !ok || a != tt.want {
			t.Errorf("ConfirmAction(%+v) = %q,%v; want %q", tt.key, a, ok, tt.want)
		}
	}

	// The confirm chords `n`/`enter`/`esc` keep their browse meaning in the default
	// context. `y` is unbound in the browse context since res.yaml was retired into
	// the edit action (D135/M3-15c) — its confirm-context meaning stands alone.
	browse := []struct {
		key  tea.Key
		want Action
	}{
		{tea.Key{Code: 'n', Text: "n"}, ActionSearchNext},
		{tea.Key{Code: tea.KeyEnter}, ActionDrillIn},
		{tea.Key{Code: tea.KeyEsc}, ActionBack},
	}
	for _, tt := range browse {
		if a, ok := km.Action(tt.key); !ok || a != tt.want {
			t.Errorf("Action(%+v) = %q,%v; want %q", tt.key, a, ok, tt.want)
		}
	}
	if a, ok := km.Action(tea.Key{Code: 'y', Text: "y"}); ok {
		t.Errorf("Action(y) = %q,true; want unbound in browse after res.yaml retired (D135/M3-15c)", a)
	}

	// Browse resolution never yields a confirm action, and vice versa.
	if a, ok := km.Action(tea.Key{Code: 'y', Text: "y"}); ok && a == ActionConfirmAccept {
		t.Error("Action leaked a confirm-context action into the browse context")
	}
	if a, ok := km.ConfirmAction(tea.Key{Code: 'd', Text: "d"}); ok {
		t.Errorf("ConfirmAction(d) = %q, want unbound in the confirm context", a)
	}
}

func TestMergeNavShadowWarns(t *testing.T) {
	// Rebinding help onto a nav key is allowed but warns. Use "h" and also
	// disable nav.left so there's no collision, isolating the warning.
	_, warns, err := DefaultKeymap().Merge(map[Action][]string{
		ActionLeft: {},    // free up "h"
		ActionHelp: {"h"}, // shadow nav key
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "app.help") {
		t.Errorf("want one nav-shadow warning naming app.help, got %v", warns)
	}
}

func TestMergeDisableAction(t *testing.T) {
	merged, _, err := DefaultKeymap().Merge(map[Action][]string{ActionQuit: {}})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(merged.Keys(ActionQuit)) != 0 {
		t.Errorf("quit should be disabled, got %v", merged.Keys(ActionQuit))
	}
	if _, ok := merged.Action(tea.Key{Code: 'q', Text: "q"}); ok {
		t.Error("q still resolves after disabling quit")
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
