package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if !strings.Contains(out.String(), "kubecom") {
		t.Errorf("version output = %q, want it to contain %q", out.String(), "kubecom")
	}
}

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute --version: %v", err)
	}
	// The custom template prints Info() verbatim, so no "version" filler word.
	if got := out.String(); !strings.Contains(got, "kubecom") || strings.Contains(got, "kubecom version") {
		t.Errorf("--version output = %q, want Info() verbatim (starts with %q, no %q)", got, "kubecom", "kubecom version")
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"bogus"})
	if err := cmd.Execute(); err == nil {
		t.Error("execute bogus: expected error, got nil")
	}
}
