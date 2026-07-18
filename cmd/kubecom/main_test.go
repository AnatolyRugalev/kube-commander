package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, []string{"version"}); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if !strings.Contains(out.String(), "kubecom") {
		t.Errorf("version output = %q, want it to contain %q", out.String(), "kubecom")
	}
}

func TestRunUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, []string{"bogus"}); err == nil {
		t.Error("run bogus: expected error, got nil")
	}
}
