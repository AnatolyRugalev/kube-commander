package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMenuEmptyIsZero(t *testing.T) {
	m, err := LoadMenu(strings.NewReader(""))
	if err != nil {
		t.Fatalf("LoadMenu(empty): %v", err)
	}
	if len(m.Resources) != 0 {
		t.Errorf("Resources = %v, want empty", m.Resources)
	}
}

func TestLoadMenuReadsEntry(t *testing.T) {
	const src = `resources:
  - group: cert-manager.io
    version: v1
    resource: certificates
    kind: Certificate
    namespaced: true
    section: Custom Resources
    title: Certs
`
	m, err := LoadMenu(strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadMenu: %v", err)
	}
	if len(m.Resources) != 1 {
		t.Fatalf("Resources = %d, want 1", len(m.Resources))
	}
	got := m.Resources[0]
	want := MenuResource{
		Group: "cert-manager.io", Version: "v1", Resource: "certificates",
		Kind: "Certificate", Namespaced: true, Section: "Custom Resources", Title: "Certs",
	}
	if got != want {
		t.Errorf("resource = %+v, want %+v", got, want)
	}
}

func TestLoadMenuCoreGroupOmittedGroup(t *testing.T) {
	// The core group is expressed by omitting group entirely.
	m, err := LoadMenu(strings.NewReader("resources:\n  - version: v1\n    resource: pods\n"))
	if err != nil {
		t.Fatalf("LoadMenu: %v", err)
	}
	if m.Resources[0].Group != "" {
		t.Errorf("Group = %q, want empty (core group)", m.Resources[0].Group)
	}
}

func TestLoadMenuRejectsUnknownField(t *testing.T) {
	_, err := LoadMenu(strings.NewReader("bogus: true\n"))
	if err == nil {
		t.Fatal("LoadMenu(unknown field): expected error, got nil")
	}
}

func TestLoadMenuRejectsMissingVersion(t *testing.T) {
	_, err := LoadMenu(strings.NewReader("resources:\n  - resource: certificates\n"))
	if err == nil || !strings.Contains(err.Error(), "version is required") {
		t.Fatalf("LoadMenu(missing version): err = %v, want version-required error", err)
	}
}

func TestLoadMenuRejectsMissingResource(t *testing.T) {
	_, err := LoadMenu(strings.NewReader("resources:\n  - version: v1\n"))
	if err == nil || !strings.Contains(err.Error(), "resource is required") {
		t.Fatalf("LoadMenu(missing resource): err = %v, want resource-required error", err)
	}
}

func TestLoadMenuFileMissingReturnsZero(t *testing.T) {
	m, err := LoadMenuFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadMenuFile(missing): %v", err)
	}
	if m == nil || len(m.Resources) != 0 {
		t.Errorf("LoadMenuFile(missing) = %+v, want zero MenuConfig", m)
	}
}

func TestLoadMenuFileReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.yaml")
	if err := os.WriteFile(path, []byte("resources:\n  - version: v1\n    resource: pods\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMenuFile(path)
	if err != nil {
		t.Fatalf("LoadMenuFile: %v", err)
	}
	if len(m.Resources) != 1 || m.Resources[0].Resource != "pods" {
		t.Errorf("Resources = %+v, want one pods entry", m.Resources)
	}
}

func TestMenuDir(t *testing.T) {
	d, err := MenuDir()
	if err != nil {
		t.Fatalf("MenuDir: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(d), "kubecom/menus") {
		t.Errorf("MenuDir = %q, want it to end with kubecom/menus", d)
	}
}

func TestMenuPathSanitizesContext(t *testing.T) {
	cases := map[string]string{
		"k3d-kubecom-test":            "k3d-kubecom-test.yaml",
		"gke_proj_zone_cluster":       "gke_proj_zone_cluster.yaml",
		"arn:aws:eks:us-east-1:1/foo": "arn_aws_eks_us-east-1_1_foo.yaml",
		"has spaces/and.slashes":      "has_spaces_and.slashes.yaml",
	}
	for context, wantFile := range cases {
		p, err := MenuPath(context)
		if err != nil {
			t.Fatalf("MenuPath(%q): %v", context, err)
		}
		if got := filepath.Base(p); got != wantFile {
			t.Errorf("MenuPath(%q) base = %q, want %q", context, got, wantFile)
		}
		// The sanitized name must be a single path segment under MenuDir.
		dir, _ := MenuDir()
		if filepath.Dir(p) != dir {
			t.Errorf("MenuPath(%q) dir = %q, want %q (context must not escape MenuDir)", context, filepath.Dir(p), dir)
		}
	}
}

func TestMenuPathEmptyContextErrors(t *testing.T) {
	if _, err := MenuPath("   "); err == nil {
		t.Fatal("MenuPath(empty): expected error, got nil")
	}
}
