package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStateEmptyIsZero(t *testing.T) {
	s, err := LoadState(strings.NewReader(""))
	if err != nil {
		t.Fatalf("LoadState(empty): %v", err)
	}
	if s.LastNamespace != "" {
		t.Errorf("LastNamespace = %q, want empty", s.LastNamespace)
	}
}

func TestLoadStateReads(t *testing.T) {
	s, err := LoadState(strings.NewReader("lastNamespace: kube-system\n"))
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.LastNamespace != "kube-system" {
		t.Errorf("LastNamespace = %q, want kube-system", s.LastNamespace)
	}
}

func TestLoadStateRejectsUnknownField(t *testing.T) {
	_, err := LoadState(strings.NewReader("bogus: true\n"))
	if err == nil {
		t.Fatal("LoadState(unknown field): expected error, got nil")
	}
}

func TestLoadStateFileMissingReturnsZero(t *testing.T) {
	s, err := LoadStateFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("LoadStateFile(missing): %v", err)
	}
	if s == nil || s.LastNamespace != "" {
		t.Errorf("LoadStateFile(missing) = %+v, want zero state", s)
	}
}

func TestStateDir(t *testing.T) {
	d, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(d), "kubecom/state") {
		t.Errorf("StateDir = %q, want it to end with kubecom/state", d)
	}
}

func TestStatePathSanitizesContext(t *testing.T) {
	dir, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"k3d-dev":                      "k3d-dev.yaml",
		"gke_proj_zone_cluster":        "gke_proj_zone_cluster.yaml",
		"arn:aws:eks:eu:1:cluster/foo": "arn_aws_eks_eu_1_cluster_foo.yaml",
		"has spaces":                   "has_spaces.yaml",
		"../escape":                    ".._escape.yaml",
	}
	for ctx, wantName := range cases {
		p, err := StatePath(ctx)
		if err != nil {
			t.Fatalf("StatePath(%q): %v", ctx, err)
		}
		if got := filepath.Base(p); got != wantName {
			t.Errorf("StatePath(%q) base = %q, want %q", ctx, got, wantName)
		}
		// The resolved path must stay inside StateDir — a context name can never
		// escape via path separators.
		if filepath.Dir(p) != dir {
			t.Errorf("StatePath(%q) dir = %q, want %q (escaped StateDir)", ctx, filepath.Dir(p), dir)
		}
	}
}

func TestStatePathEmptyContextErrors(t *testing.T) {
	if _, err := StatePath("   "); err == nil {
		t.Fatal("StatePath(empty): expected error, got nil")
	}
}

func TestStateSaveRoundTrips(t *testing.T) {
	orig := &State{LastNamespace: "default"}
	var buf bytes.Buffer
	if err := orig.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadState(&buf)
	if err != nil {
		t.Fatalf("LoadState(saved): %v", err)
	}
	if got.LastNamespace != orig.LastNamespace {
		t.Errorf("round-trip LastNamespace = %q, want %q", got.LastNamespace, orig.LastNamespace)
	}
}

func TestStateSaveEmptyRoundTripsToZero(t *testing.T) {
	var buf bytes.Buffer
	if err := (&State{}).Save(&buf); err != nil {
		t.Fatalf("Save(empty): %v", err)
	}
	got, err := LoadState(&buf)
	if err != nil {
		t.Fatalf("LoadState(empty saved): %v", err)
	}
	if got.LastNamespace != "" {
		t.Errorf("round-trip empty LastNamespace = %q, want empty", got.LastNamespace)
	}
}

func TestStateSaveFileCreatesParentAndRoundTrips(t *testing.T) {
	// A parent dir that does not yet exist must be created (0o700).
	dir := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(dir, "ctx.yaml")
	orig := &State{LastNamespace: "kube-system"}
	if err := orig.SaveFile(path); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	got, err := LoadStateFile(path)
	if err != nil {
		t.Fatalf("LoadStateFile(saved): %v", err)
	}
	if got.LastNamespace != orig.LastNamespace {
		t.Errorf("round-trip LastNamespace = %q, want %q", got.LastNamespace, orig.LastNamespace)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file perm = %o, want 600", perm)
	}
}

func TestStateSaveFileReplacesAtomicallyWithoutLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.yaml")
	if err := os.WriteFile(path, []byte("lastNamespace: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&State{LastNamespace: "new"}).SaveFile(path); err != nil {
		t.Fatalf("SaveFile(overwrite): %v", err)
	}
	got, err := LoadStateFile(path)
	if err != nil {
		t.Fatalf("LoadStateFile: %v", err)
	}
	if got.LastNamespace != "new" {
		t.Errorf("overwritten LastNamespace = %q, want new", got.LastNamespace)
	}
	// The atomic temp file must be gone — only ctx.yaml should remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "ctx.yaml" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir entries = %v, want only [ctx.yaml]", names)
	}
}
