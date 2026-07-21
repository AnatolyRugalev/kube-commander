package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
)

// writeMenuFile points the user config dir at a temp dir and writes body to the
// per-context menu file for context, returning its path. It uses config.MenuPath
// so the test exercises the real path resolution the launcher uses.
func writeMenuFile(t *testing.T, context, body string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := config.MenuPath(context)
	if err != nil {
		t.Fatalf("MenuPath(%q): %v", context, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir menus dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write menu file: %v", err)
	}
}

// TestLoadMenuExtrasBlankContext proves an unresolved context yields no extras and
// no error — the launcher must not block when it cannot name a context (principle 3).
func TestLoadMenuExtrasBlankContext(t *testing.T) {
	extras, err := loadMenuExtras("  ")
	if err != nil {
		t.Fatalf("blank context should not error, got %v", err)
	}
	if extras != nil {
		t.Fatalf("blank context should yield no extras, got %v", extras)
	}
}

// TestLoadMenuExtrasMissingFile proves a context with no menu file degrades to the
// default menu: no extras, no error (the common case).
func TestLoadMenuExtrasMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	extras, err := loadMenuExtras("no-such-context")
	if err != nil {
		t.Fatalf("missing menu file should not error, got %v", err)
	}
	if len(extras) != 0 {
		t.Fatalf("missing menu file should yield no extras, got %v", extras)
	}
}

// TestLoadMenuExtrasReadsFile proves a valid per-context menu file's entries are
// returned to the launcher (which feeds them to the menu via WithMenuExtras).
func TestLoadMenuExtrasReadsFile(t *testing.T) {
	const ctx = "prod"
	writeMenuFile(t, ctx, `resources:
  - group: cert-manager.io
    version: v1
    resource: certificates
    kind: Certificate
    namespaced: true
`)
	extras, err := loadMenuExtras(ctx)
	if err != nil {
		t.Fatalf("valid menu file should load, got %v", err)
	}
	if len(extras) != 1 {
		t.Fatalf("expected 1 extra resource, got %d", len(extras))
	}
	got := extras[0]
	want := config.MenuResource{
		Group: "cert-manager.io", Version: "v1", Resource: "certificates",
		Kind: "Certificate", Namespaced: true,
	}
	if got != want {
		t.Fatalf("extra = %+v, want %+v", got, want)
	}
}

// TestLoadMenuExtrasMalformedFile proves a malformed/unreadable menu file returns
// an error (the launcher turns it into a startup toast) rather than silently
// dropping — but never a fatal launch failure (the caller degrades to the default
// menu). Here an unknown field trips the strict decoder.
func TestLoadMenuExtrasMalformedFile(t *testing.T) {
	writeMenuFile(t, "broken", "resources:\n  - bogusField: nope\n")
	if _, err := loadMenuExtras("broken"); err == nil {
		t.Fatal("a malformed menu file should return an error to surface as a toast")
	}
}
