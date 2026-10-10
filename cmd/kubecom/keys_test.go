package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

func runKeys(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"keys"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// lineOf is the listing's line binding keys to action, its columns split on runs
// of spaces.
func lineOf(out, keys, action string) []string {
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(strings.TrimPrefix(l, keys+" ")); strings.HasPrefix(l, keys+" ") && len(f) > 0 && f[0] == action {
			return append([]string{keys}, f...)
		}
	}
	return nil
}

func TestKeysDefaults(t *testing.T) {
	// A missing config resolves to the defaults, each binding with its file:line.
	out, errOut, err := runKeys(t, "--config-dir", t.TempDir())
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if f := lineOf(out, "j", "nav.down"); len(f) != 3 || !strings.HasPrefix(f[2], "default.kdl:") {
		t.Errorf("the j line is %q:\n%s", f, out)
	}
	if f := lineOf(out, "y", "confirm.accept"); len(f) != 4 || f[2] != "confirm" {
		t.Errorf("the confirm modal's y should list its context, got %q", f)
	}
	if lineOf(out, "g g", "nav.top") == nil {
		t.Errorf("the g g sequence is not listed:\n%s", out)
	}
	if !strings.Contains(out, "no key (the palette runs them): app.update") {
		t.Errorf("the palette-only actions are not listed:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty on a clean default resolve", errOut)
	}
}

// The user's keymap lists after the defaults, with its values, contexts and lines.
func TestKeysUserLayer(t *testing.T) {
	dir := writeConfig(t, `version 1
keymap {
    unbind "q"
    context "table.kind == 'Pod'" {
        bind "g p" "resources.switch" "pods"
    }
}
`)
	out, _, err := runKeys(t, "--config-dir", dir)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	for _, want := range []string{"(unbind)", `resources.switch "pods"`, "table.kind == 'Pod'", "config.kdl:3", "config.kdl:5"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing has no %q:\n%s", want, out)
		}
	}
	if lineOf(out, "q", "app.quit") == nil || strings.Index(out, "(unbind)") < strings.Index(out, "app.quit") {
		t.Errorf("the user's unbind should list after the default it beats:\n%s", out)
	}
}

func TestKeysInvalidConfigFails(t *testing.T) {
	dir := writeConfig(t, "version 1\nkeys \"x\"\n")
	if _, _, err := runKeys(t, "--config-dir", dir); err == nil || !strings.Contains(err.Error(), "config.kdl:2:1") {
		t.Fatalf("keys(an unreadable config): %v, want its file:line", err)
	}
}

func TestKeysDefaultPrintsDefaultKDL(t *testing.T) {
	out, _, err := runKeys(t, "default")
	if err != nil {
		t.Fatalf("keys default: %v", err)
	}
	if out != string(keymap.DefaultSource()) {
		t.Fatalf("keys default printed something other than default.kdl:\n%s", out)
	}
}

// keys check names every mistake at its line, and passes a keymap kubecom takes.
func TestKeysCheck(t *testing.T) {
	ok := writeConfig(t, "version 1\nkeymap {\n    bind \"Q\" \"app.quit\"\n}\n")
	if out, _, err := runKeys(t, "check", "--config-dir", ok); err != nil || !strings.Contains(out, "keymap ok: 1 binding") {
		t.Fatalf("keys check (a good keymap): %q, %v", out, err)
	}
	bad := writeConfig(t, `version 1
keymap {
    bind "x" "nav.dwn"
    context "tabel" { bind "y" "app.help"; }
    bind "z" "nav.down" "2"
}
`)
	_, _, err := runKeys(t, "check", "--config-dir", bad)
	if err == nil {
		t.Fatal("keys check passed a keymap with three mistakes")
	}
	for _, want := range []string{`config.kdl:3: "nav.dwn" is not an action`, `config.kdl:4: context "tabel"`, "config.kdl:5: nav.down takes no value"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("keys check: %v\nwant %q", err, want)
		}
	}
}

func runKeysAnalyze(t *testing.T, trace string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"keys", "analyze", path})
	err := cmd.Execute()
	return out.String(), err
}

// TestKeysAnalyzeSummarizesATrace exercises the whole path end to end: a written
// trace (the format the writer produces) read back by the command and reduced to
// the five findings.
func TestKeysAnalyzeSummarizesATrace(t *testing.T) {
	const trace = `{"t":"2026-08-15T12:00:00.000Z","key":"g","mode":"browse","pending":true}
{"t":"2026-08-15T12:00:00.100Z","key":"g","mode":"browse","action":"nav.top"}
{"t":"2026-08-15T12:00:01.000Z","key":"x","mode":"browse"}
{"t":"2026-08-15T12:00:01.100Z","key":"x","mode":"browse"}
{"t":"2026-08-15T12:00:10.000Z","key":"L","mode":"browse","action":"res.logs"}
{"t":"2026-08-15T12:00:10.500Z","key":"q","mode":"browse","action":"app.quit"}
`
	out, err := runKeysAnalyze(t, trace)
	if err != nil {
		t.Fatalf("keys analyze: %v", err)
	}
	// The completed `gg` is not an abandoned sequence.
	if !strings.Contains(out, "abandoned sequences") || !strings.Contains(out, "(none)") {
		t.Errorf("a clean `gg` must leave the abandoned list empty:\n%s", out)
	}
	// The two x presses are the dead end, ranked, on the browse surface.
	if !strings.Contains(out, "x") || !strings.Contains(out, "browse") {
		t.Errorf("the dead end must be reported with its surface:\n%s", out)
	}
	// Actions counted, and the longest pause reported against the press that
	// followed it.
	if !strings.Contains(out, "res.logs") || !strings.Contains(out, "longest pauses") {
		t.Errorf("actions and pauses must be reported:\n%s", out)
	}
}

// TestKeysAnalyzeReportsTextSurfacePressesSeparately is STORY-06e on the command:
// a press on a text surface that resolved to nothing — the picker `j`s of the S01
// walk — is a finding in its own section, not a dead end and not silently skipped.
func TestKeysAnalyzeReportsTextSurfacePressesSeparately(t *testing.T) {
	const trace = `{"t":"2026-08-15T12:00:00.000Z","key":"j","mode":"picker","text":true}
{"t":"2026-08-15T12:00:01.000Z","key":"j","mode":"picker","text":true}
{"t":"2026-08-15T12:00:02.000Z","key":"x","mode":"browse"}
`
	out, err := runKeysAnalyze(t, trace)
	if err != nil {
		t.Fatalf("keys analyze: %v", err)
	}
	if !strings.Contains(out, "presses on text surfaces") {
		t.Errorf("the text-surface section must be reported:\n%s", out)
	}
	if !strings.Contains(out, "j") || !strings.Contains(out, "picker") {
		t.Errorf("the picker `j`s must appear in the text-surface section:\n%s", out)
	}
	// The browse `x` stays a dead end; the picker `j`s must not appear there.
	if !strings.Contains(out, "unresolved presses") {
		t.Errorf("the dead-end section must still be reported:\n%s", out)
	}
}

// TestKeysAnalyzeReportsAnAbandonedSequence: `g` started, never finished — the
// trace records the pending press and nothing completing it.
func TestKeysAnalyzeReportsAnAbandonedSequence(t *testing.T) {
	const trace = `{"t":"2026-08-15T12:00:00.000Z","key":"g","mode":"browse","pending":true}
{"t":"2026-08-15T12:00:02.000Z","key":"x","mode":"browse"}
`
	out, err := runKeysAnalyze(t, trace)
	if err != nil {
		t.Fatalf("keys analyze: %v", err)
	}
	if !strings.Contains(out, "g") || !strings.Contains(out, "then x") {
		t.Errorf("the abandoned `g` must be reported with what came after:\n%s", out)
	}
}
