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

// TestStatePinnedResourcesRoundTrip proves a pinned kind survives Save → Load with
// every field intact — the property CRD-PIN-02's write gesture depends on, since a
// pin that lost its Namespaced flag would list a namespaced CRD cluster-wide.
func TestStatePinnedResourcesRoundTrip(t *testing.T) {
	in := &State{
		LastNamespace: "kube-system",
		PinnedResources: []MenuResource{{
			Group: "hub.traefik.io", Version: "v1alpha1", Resource: "aigateways",
			Kind: "AIGateway", Namespaced: true,
		}},
	}
	var buf bytes.Buffer
	if err := in.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := LoadState(&buf)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(out.PinnedResources) != 1 || out.PinnedResources[0] != in.PinnedResources[0] {
		t.Fatalf("PinnedResources = %+v, want %+v", out.PinnedResources, in.PinnedResources)
	}
	if out.LastNamespace != "kube-system" {
		t.Errorf("LastNamespace = %q, want kube-system", out.LastNamespace)
	}
}

// TestStateNoPinsStaysEmpty proves the new field is omitted entirely when nothing is
// pinned, so an existing state file gains no key until the user pins something.
func TestStateNoPinsStaysEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := (&State{}).Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if strings.Contains(buf.String(), "pinnedResources") {
		t.Errorf("empty state emitted %q, want no pinnedResources key", buf.String())
	}
}

// TestLoadStateRejectsUnaddressablePin proves a pin the kube layer could not address
// (no resource) fails the load exactly as the same entry in the authored menu file
// does — one validation for both files.
func TestLoadStateRejectsUnaddressablePin(t *testing.T) {
	_, err := LoadState(strings.NewReader("pinnedResources:\n  - version: v1\n"))
	if err == nil {
		t.Fatal("a pin with no resource should fail the load")
	}
	if !strings.Contains(err.Error(), "resource is required") {
		t.Errorf("error = %v, want it to name the missing field", err)
	}
}

// TestStatePinIsIdempotentByGVR proves pinning the same kind twice adds one row and
// reports the second as a no-op — the menu dedupes on the GVR, so the store must too.
func TestStatePinIsIdempotentByGVR(t *testing.T) {
	s := &State{}
	r := MenuResource{Group: "g", Version: "v1", Resource: "widgets", Kind: "Widget"}
	if !s.Pin(r) {
		t.Fatal("first Pin should report a change")
	}
	// Same GVR, different display hints: still the same resource, still one row.
	if s.Pin(MenuResource{Group: "g", Version: "v1", Resource: "widgets", Title: "Other"}) {
		t.Error("re-pinning the same GVR should report no change")
	}
	if len(s.PinnedResources) != 1 || s.PinnedResources[0] != r {
		t.Fatalf("PinnedResources = %+v, want the first entry only", s.PinnedResources)
	}
	// A different version is a different resource to the API, so it is a new pin.
	if !s.Pin(MenuResource{Group: "g", Version: "v1beta1", Resource: "widgets"}) {
		t.Error("a different version should pin as its own entry")
	}
	if len(s.PinnedResources) != 2 {
		t.Fatalf("PinnedResources = %+v, want 2 entries", s.PinnedResources)
	}
}

// TestStateUnpin proves the removal half: an unpinned GVR goes away, the others keep
// their order, and unpinning something that was never pinned reports no change (so
// the caller writes no file).
func TestStateUnpin(t *testing.T) {
	s := &State{PinnedResources: []MenuResource{
		{Group: "g", Version: "v1", Resource: "widgets"},
		{Group: "h", Version: "v1", Resource: "gadgets"},
		{Version: "v1", Resource: "configmaps"},
	}}
	if !s.Unpin("h", "v1", "gadgets") {
		t.Fatal("Unpin of a pinned GVR should report a change")
	}
	if len(s.PinnedResources) != 2 ||
		s.PinnedResources[0].Resource != "widgets" || s.PinnedResources[1].Resource != "configmaps" {
		t.Fatalf("PinnedResources = %+v, want widgets then configmaps", s.PinnedResources)
	}
	if s.Unpin("h", "v1", "gadgets") {
		t.Error("Unpin of an absent GVR should report no change")
	}
	if !s.Unpin("", "v1", "configmaps") {
		t.Error("the core group ('') must be unpinnable, not treated as a wildcard")
	}
}
