package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var (
	deploys = MenuResource{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true}
	certs   = MenuResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates", Kind: "Certificate",
		Namespaced: true, Section: "TLS"}
	nodes = MenuResource{Version: "v1", Resource: "nodes", Kind: "Node"}
)

func statePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state", "prod.kdl")
}

func TestLoadStateFileMissingIsZero(t *testing.T) {
	s, err := LoadStateFile(statePath(t))
	if err != nil || !reflect.DeepEqual(*s, State{}) {
		t.Fatalf("missing state = %+v, %v", s, err)
	}
}

// Every field survives a write and a read, and the file is the documented shape.
func TestStateRoundTrips(t *testing.T) {
	path := statePath(t)
	want := State{
		LastNamespace:     "kube-system",
		PinnedResources:   []MenuResource{certs, nodes},
		LastResource:      &deploys,
		LastSortColumn:    "AGE",
		LastSortAscending: true,
		LastDrillOwner:    &DrillOwner{Resource: deploys, Namespace: "web", Name: "api"},
	}
	if err := want.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", *got, want)
	}
	src, _ := os.ReadFile(path)
	for _, line := range []string{
		"version 1",
		`namespace kube-system`,
		`resource deployments group=apps version=v1 kind=Deployment namespaced=#true`,
		`drill-owner deployments group=apps version=v1 kind=Deployment namespaced=#true namespace=web name=api`,
		`sort AGE ascending=#true`,
		`pin certificates group=cert-manager.io version=v1 kind=Certificate namespaced=#true section=TLS`,
		`pin nodes version=v1 kind=Node`,
	} {
		if !strings.Contains(string(src), line) {
			t.Errorf("the file has no line %q:\n%s", line, src)
		}
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %v, %v", fi.Mode().Perm(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("a temp file was left behind: %v", entries)
	}
}

// The zero state is `version 1` alone and reads back as zero.
func TestStateZeroRoundTrips(t *testing.T) {
	path := statePath(t)
	if err := (&State{}).SaveFile(path); err != nil {
		t.Fatal(err)
	}
	if src, _ := os.ReadFile(path); string(src) != "version 1\n" {
		t.Fatalf("zero state = %q", src)
	}
	if got, err := LoadStateFile(path); err != nil || !reflect.DeepEqual(*got, State{}) {
		t.Fatalf("zero state read back as %+v, %v", got, err)
	}
}

// A state file kubecom could not have written is an error at its line, which the
// launcher degrades to the zero state.
func TestStateErrors(t *testing.T) {
	for src, want := range map[string]string{
		"lastNamespace \"x\"\n": `"lastNamespace" is not a state node`,
		"namespace 3\n":         "namespace takes one string",
		"pin \"certificates\" group=\"cert-manager.io\"\n":          "pin: version is required",
		"resource \"\" version=\"v1\"\n":                            "resource: the resource name is required",
		"drill-owner \"deployments\" version=\"v1\"\n":              "drill-owner: name is required",
		"drill-owner \"d\" version=\"v1\" name=\"a\" owner=\"x\"\n": `has no property "owner"`,
		"sort \"AGE\" descending=#true\n":                           `sort has no property "descending"`,
	} {
		path := statePath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("version 1\n"+src), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadStateFile(path); err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), ":2:") {
			t.Errorf("%q: %v, want %q at line 2", src, err, want)
		}
	}
}

func TestStateDirAndPath(t *testing.T) {
	dir, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := Dir()
	if dir != filepath.Join(base, "state") {
		t.Fatalf("StateDir() = %q", dir)
	}
	for ctx, want := range map[string]string{
		"kind-dev":                   "kind-dev.kdl",
		"gke_proj_zone/cluster":      "gke_proj_zone_cluster.kdl",
		"arn:aws:eks:eu:1:cluster/x": "arn_aws_eks_eu_1_cluster_x.kdl",
		"../../etc/passwd":           ".._.._etc_passwd.kdl",
	} {
		p, err := StatePath(ctx)
		if err != nil || p != filepath.Join(dir, want) {
			t.Errorf("StatePath(%q) = %q, %v; want %s", ctx, p, err, want)
		}
	}
	if _, err := StatePath("  "); err == nil {
		t.Error("an empty context resolved a state path")
	}
}

// Pin is idempotent by GVR; Unpin removes by GVR.
func TestPinAndUnpin(t *testing.T) {
	var s State
	if !s.Pin(certs) || s.Pin(MenuResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates", Title: "other"}) {
		t.Fatal("a second pin of the same GVR was added")
	}
	if !s.Pin(nodes) || len(s.PinnedResources) != 2 {
		t.Fatalf("pins = %+v", s.PinnedResources)
	}
	if s.Unpin("apps", "v1", "deployments") {
		t.Fatal("unpinned a kind that was not pinned")
	}
	if !s.Unpin("cert-manager.io", "v1", "certificates") || !reflect.DeepEqual(s.PinnedResources, []MenuResource{nodes}) {
		t.Fatalf("after unpin: %+v", s.PinnedResources)
	}
}
