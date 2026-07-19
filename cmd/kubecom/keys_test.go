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
	// A missing config file resolves to the vim-first defaults.
	out, errOut, err := runKeys(t, "--config", filepath.Join(t.TempDir(), "none.yaml"))
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

func TestKeysOverrideAndWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// nav.down moved off j; app.help binds nav key j → resolves, with a warning.
	if err := os.WriteFile(path, []byte("keys:\n  nav.down: [down]\n  app.help: [j]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runKeys(t, "--config", path)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	// nav.down now resolves to just "down" (j was moved to app.help), so the
	// default "j, down" must be gone.
	if !strings.Contains(out, "app.help") || strings.Contains(out, "j, down") {
		t.Errorf("output missing overridden bindings:\n%s", out)
	}
	if !strings.Contains(errOut, "navigation key") {
		t.Errorf("stderr = %q, want a nav-shadow warning", errOut)
	}
}

func TestKeysInvalidConfigFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  bogus.action: [x]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runKeys(t, "--config", path); err == nil {
		t.Fatal("keys(unknown action): expected error, got nil")
	}
}
