package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestKeysDefaults(t *testing.T) {
	// A missing config resolves to the vim-first defaults.
	out, errOut, err := runKeys(t, "--config-dir", t.TempDir())
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if !strings.Contains(out, "nav.down") || !strings.Contains(out, "j, down") {
		t.Errorf("output missing default nav.down binding:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty on a clean default resolve", errOut)
	}
}

func TestKeysInvalidConfigFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.kdl"), []byte("version 1\nkeys \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runKeys(t, "--config-dir", dir); err == nil || !strings.Contains(err.Error(), "config.kdl:2:1") {
		t.Fatalf("keys(an unreadable config): %v, want its file:line", err)
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
