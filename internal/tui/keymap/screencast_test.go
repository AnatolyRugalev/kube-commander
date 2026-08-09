package keymap

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Guards for the committed screencast tape (docs/screencast.tape, M5-09) and the
// asset it produces. The tape is a *script of keypresses*, so it restates keys
// that live in this registry (D11) — the one thing a generated doc is never
// allowed to do. It cannot be generated (the tour's pacing, prose and cluster
// tuning are editorial), so instead every keypress in it is annotated with the
// action it means, and these tests bind the annotation to the registry.
//
// Without them the failure mode is silent and long-lived: a rebinding lands, the
// tape still records — vhs types keys, it does not check them — and the published
// GIF shows kubecom "responding" to keys it no longer has, or shows nothing
// happening at all. `make check` cannot record a GIF, but it can hold the script
// the GIF is made from to the registry (D181).
var (
	tapePath   = filepath.Join("..", "..", "..", "docs", "screencast", "screencast.tape")
	readmePath = filepath.Join("..", "..", "..", "README.md")
)

// Annotation prefixes. Every keypress line in the tape must be preceded by one:
// an action annotation names the action the press triggers (checked against the
// default keymap), an input annotation marks the press as plain text — a shell
// command, a filter query — which no keymap can validate.
const (
	actionAnnotation = "# kubecom-action:"
	inputAnnotation  = "# kubecom-input:"
)

// keyCommands are the vhs commands that send a keypress to the recorded program
// (as opposed to Set/Sleep/Hide/Show/Output/Require, which do not). Mapped to the
// keymap token the press produces, so an annotated key can be compared with what
// the registry says the action is bound to. Type is handled separately: its token
// is the quoted string it types.
var keyCommands = map[string]string{
	"Enter":     "enter",
	"Escape":    "esc",
	"Space":     "space",
	"Tab":       "tab",
	"Backspace": "backspace",
	"Up":        "up",
	"Down":      "down",
	"Left":      "left",
	"Right":     "right",
	"PageUp":    "pgup",
	"PageDown":  "pgdn",
	"Home":      "home",
	"End":       "end",
}

// typeLine matches `Type "…"` (with vhs's optional @speed modifier) and captures
// the typed string; ctrlLine matches a modifier chord like `Ctrl+D`.
var (
	typeLine = regexp.MustCompile(`^Type(?:@\S+)?\s+"([^"]*)"\s*$`)
	ctrlLine = regexp.MustCompile(`^(Ctrl|Alt|Shift)\+(\S+)\s*$`)
)

// tapeKey is one annotated keypress read out of the tape.
type tapeKey struct {
	line   int    // 1-based line number of the command, for failure messages
	cmd    string // the tape line itself
	token  string // the keymap token the press produces ("L", "ctrl+s", "esc")
	action string // the annotated action id, "" for an input annotation
}

// TestScreencastTapeMatchesTheKeymap is the drift guard: every key the tape
// presses must still be bound to the action the tape says it is, and no keypress
// may go unannotated. A rebinding (or an unchecked key slipped into the tour) now
// fails `make check` rather than producing a GIF that lies (D181).
func TestScreencastTapeMatchesTheKeymap(t *testing.T) {
	km := DefaultKeymap()
	keys := readTape(t)
	if len(keys) == 0 {
		t.Fatal("no keypresses found in the tape — the guard is not reading it")
	}

	var checked int
	for _, k := range keys {
		if k.action == "" {
			continue // plain text: a shell command or a filter query
		}
		a := Action(k.action)
		if !a.Valid() {
			t.Errorf("%s:%d: annotated action %q is not registered", tapePath, k.line, k.action)
			continue
		}
		bound := km.Keys(a)
		if !contains(bound, k.token) {
			t.Errorf("%s:%d: %s presses %q, but %s is bound to %v — re-record the tape or fix the annotation",
				tapePath, k.line, k.cmd, k.token, a, bound)
			continue
		}
		checked++
	}
	if checked == 0 {
		t.Error("the tape annotates no action keypresses — the tour presses no kubecom key")
	}
}

// TestScreencastTapeShowsTheHeadlineActions keeps the tour from decaying into a
// launch-and-quit clip: the README sells browsing, filtering, logs and describe,
// so the tape must actually press those (the tour M5-09 specifies). Adding to the
// tour is free; silently dropping one of these is not.
//
// The browse step is app.palette rather than resources.switch since PAL-02: the
// tour still opens Pods by name, but it does so the way the README now describes
// it — `:` then the verb — so the *key* the tape presses is the palette's. The
// resource switch it runs is a pick inside that palette, which no keypress
// annotation can name (the Enter that runs it is nav.drillIn).
func TestScreencastTapeShowsTheHeadlineActions(t *testing.T) {
	want := []Action{ActionPalette, ActionFilter, ActionDrillIn, ActionLogs, ActionDescribe, ActionQuit}

	pressed := map[string]bool{}
	for _, k := range readTape(t) {
		pressed[k.action] = true
	}
	for _, a := range want {
		if !pressed[string(a)] {
			t.Errorf("%s never presses %s — the recorded tour would not show it", tapePath, a)
		}
	}
}

// TestScreencastAssetAndReadmeAgree keeps the README honest about the asset: it
// may reference the screencast exactly when the file the tape writes exists. A
// README pointing at a missing image is worse than no screencast (M5-09), and a
// recorded GIF nobody links to is a 2 MB file doing nothing — so the two are
// checked together, in both directions.
func TestScreencastAssetAndReadmeAgree(t *testing.T) {
	out := tapeOutput(t)
	_, err := os.Stat(filepath.Join("..", "..", "..", out))
	recorded := err == nil

	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	referenced := strings.Contains(string(data), out)

	switch {
	case recorded && !referenced:
		t.Errorf("%s exists but %s never references it — embed it, or delete it", out, readmePath)
	case !recorded && referenced:
		t.Errorf("%s references %s, which does not exist — a README must not point at a missing image", readmePath, out)
	}
}

// tapeOutput returns the path the tape's Output command writes, repo-relative.
func tapeOutput(t *testing.T) string {
	t.Helper()
	for _, line := range tapeLines(t) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Output "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	t.Fatalf("%s declares no Output", tapePath)
	return ""
}

func tapeLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(tapePath)
	if err != nil {
		t.Fatalf("read %s: %v", tapePath, err)
	}
	return strings.Split(string(data), "\n")
}

// readTape parses the tape into its annotated keypresses. The annotation is the
// comment line immediately above a keypress command (vhs's own comment syntax, so
// the tape stays a valid tape); an unannotated keypress is a failure, since an
// unchecked key is exactly what these guards exist to prevent.
func readTape(t *testing.T) []tapeKey {
	t.Helper()

	var keys []tapeKey
	var pending string // the annotation seen since the last command, if any
	var pendingIsInput bool

	for i, raw := range tapeLines(t) {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, actionAnnotation):
			pending = strings.TrimSpace(strings.TrimPrefix(line, actionAnnotation))
			pendingIsInput = false
			continue
		case strings.HasPrefix(line, inputAnnotation):
			pending, pendingIsInput = "", true
			continue
		case strings.HasPrefix(line, "#"):
			continue // ordinary prose comment: leaves any pending annotation alone
		}

		token, isKey := keyToken(line)
		if !isKey {
			pending, pendingIsInput = "", false
			continue
		}
		if pending == "" && !pendingIsInput {
			t.Errorf("%s:%d: %q presses a key with no %s / %s annotation above it",
				tapePath, i+1, line, actionAnnotation, inputAnnotation)
		}
		keys = append(keys, tapeKey{line: i + 1, cmd: line, token: token, action: pending})
		pending, pendingIsInput = "", false
	}
	return keys
}

// keyToken maps one tape line to the keymap token it presses, reporting whether
// the line is a keypress at all. A `Type "…"` line yields the typed string, which
// is a keymap token for a single key ("L", "/") and for a multi-key sequence
// ("gg") alike — the same spelling the registry uses.
func keyToken(line string) (string, bool) {
	if m := typeLine.FindStringSubmatch(line); m != nil {
		return m[1], true
	}
	if m := ctrlLine.FindStringSubmatch(line); m != nil {
		return strings.ToLower(m[1]) + "+" + strings.ToLower(m[2]), true
	}
	// A bare key command, optionally with vhs's repeat count (`Down 3`).
	word, _, _ := strings.Cut(line, " ")
	tok, ok := keyCommands[word]
	return tok, ok
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
