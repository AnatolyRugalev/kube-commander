package stories

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

var (
	manifestDir = filepath.Join("..", "..", "stories", "cluster", "manifests")
	tapePath    = filepath.Join("..", "..", "docs", "screencast", "screencast.tape")
)

// storyNamespaces are the three the fixture exists to provide. They are not an
// arbitrary set: `shop` is the healthy state a story starts from, `broken` is the
// unhealthy one it investigates, and `data` carries the kinds a Deployment-only
// fixture never reaches (StatefulSet, DaemonSet, Job, CronJob, bound PVCs). A leg
// that drops one silently removes the ground a story stands on.
var storyNamespaces = []string{"shop", "data", "broken"}

// object is the sliver of a manifest these guards read.
type object struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

// TestFixtureManifestsParse is the cheap half of "the fixture is real": every
// document is YAML, and every document is a Kubernetes object with the three
// fields anything downstream needs. `kubectl apply` would say the same thing, but
// it needs a cluster and this runs in `make check`.
func TestFixtureManifestsParse(t *testing.T) {
	for _, o := range fixtureObjects(t) {
		switch {
		case o.APIVersion == "":
			t.Errorf("%s: object %q has no apiVersion", o.file, o.Metadata.Name)
		case o.Kind == "":
			t.Errorf("%s: object %q has no kind", o.file, o.Metadata.Name)
		case o.Metadata.Name == "":
			t.Errorf("%s: a %s has no metadata.name", o.file, o.Kind)
		}
	}
}

// TestFixtureDeclaresItsNamespaces checks both directions of the namespace
// relation, because each direction fails differently and neither is loud.
//
// A missing story namespace produces an empty table the reader mistakes for a
// bug in kubecom. An object pointing at a namespace the fixture does not create
// produces an apply error mid-run, after up.sh has already destroyed the previous
// cluster — the fixture is then neither the old state nor the new one, which is
// exactly what "clean initial setup every time" (D268 pt 3) promises against.
func TestFixtureDeclaresItsNamespaces(t *testing.T) {
	objs := fixtureObjects(t)

	declared := map[string]bool{}
	for _, o := range objs {
		if o.Kind == "Namespace" {
			declared[o.Metadata.Name] = true
		}
	}

	for _, ns := range storyNamespaces {
		if !declared[ns] {
			t.Errorf("the fixture declares no namespace %q — every story assumes it (D268)", ns)
		}
	}

	for _, o := range objs {
		if o.Kind == "Namespace" || o.Metadata.Namespace == "" {
			continue
		}
		if !declared[o.Metadata.Namespace] {
			t.Errorf("%s: %s/%s is in namespace %q, which the fixture never creates",
				o.file, o.Kind, o.Metadata.Name, o.Metadata.Namespace)
		}
	}
}

// TestTapeQueriesMatchTheFixture ties the screencast's typed queries to the
// cluster it is recorded against.
//
// The tape's header calls these strings TUNE points, and until the fixture was
// committed they were tuned against a cluster that existed only in one agent's
// shell history (journal 2026-07-21.3) — so "does `shop` still match anything?"
// was a question nobody could answer without rebuilding that cluster by hand. Now
// it is a test.
//
// The check is deliberately loose about *where* the string matches: the browse
// filter narrows on any visible cell (table.rowMatches), so a namespace name, a
// workload name or a container name are all legitimate ways for a query to hit.
// What it refuses is a query that appears nowhere in the fixture at all, which
// records an empty table and reads as a UX finding rather than a broken tape.
func TestTapeQueriesMatchTheFixture(t *testing.T) {
	fixture := strings.ToLower(fixtureText(t))

	queries := tapeQueries(t)
	if len(queries) == 0 {
		t.Fatal("no filter/grep queries found in the tape — has the annotation wording changed? (D268)")
	}
	for _, q := range queries {
		if !strings.Contains(fixture, strings.ToLower(q.text)) {
			t.Errorf("%s:%d types %q as a %s, but no manifest in %s contains it — "+
				"the recording would show an empty result",
				tapePath, q.line, q.text, q.kind, manifestDir)
		}
	}
}

// tapeQuery is one typed string the tape uses to narrow something.
type tapeQuery struct {
	text string
	kind string // "filter" or "grep", from the annotation
	line int
}

// annotation matches the tape's `# kubecom-input:` comments, which sit on the
// line above the keypress they describe (the convention
// TestScreencastTapeMatchesTheKeymap enforces).
var (
	annotation = regexp.MustCompile(`^\s*#\s*kubecom-input:\s*(.*)$`)
	typeLine   = regexp.MustCompile(`^\s*Type(?:@\S+)?\s+"([^"]*)"\s*$`)
)

// tapeQueries returns the strings the tape types to narrow a table or a log
// stream: a `Type "..."` whose annotation says it is a filter or a grep. Reading
// the annotation rather than a hard-coded line number lets the tour be re-cut
// (TAPE-01) without this guard needing to know its new shape — it only has to
// keep saying which typed strings are queries.
func tapeQueries(t *testing.T) []tapeQuery {
	t.Helper()
	lines := readLines(t, tapePath)

	var out []tapeQuery
	for i, l := range lines {
		m := annotation.FindStringSubmatch(l)
		if m == nil || i+1 >= len(lines) {
			continue
		}
		desc := strings.ToLower(m[1])
		kind := ""
		switch {
		case strings.Contains(desc, "filter"):
			kind = "filter"
		case strings.Contains(desc, "grep"), strings.Contains(desc, "search"):
			kind = "grep"
		default:
			continue
		}
		if tm := typeLine.FindStringSubmatch(lines[i+1]); tm != nil && tm[1] != "" {
			out = append(out, tapeQuery{text: tm[1], kind: kind, line: i + 2})
		}
	}
	return out
}

// fixtureObject is an object plus the file it came from, so a failure names it.
type fixtureObject struct {
	object
	file string
}

// fixtureObjects parses every document in every manifest.
func fixtureObjects(t *testing.T) []fixtureObject {
	t.Helper()
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		t.Fatalf("read %s: %v", manifestDir, err)
	}

	var out []fixtureObject
	var files int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		files++
		path := filepath.Join(manifestDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, doc := range splitDocs(string(data)) {
			if strings.TrimSpace(stripComments(doc)) == "" {
				continue
			}
			var o object
			if err := yaml.Unmarshal([]byte(doc), &o); err != nil {
				t.Errorf("%s: document does not parse as YAML: %v", path, err)
				continue
			}
			out = append(out, fixtureObject{object: o, file: path})
		}
	}
	if files == 0 {
		t.Fatalf("no manifests in %s — the fixture is the ground every story stands on", manifestDir)
	}
	return out
}

// fixtureText is every manifest concatenated, for substring questions.
func fixtureText(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		t.Fatalf("read %s: %v", manifestDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(manifestDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

// splitDocs splits a multi-document YAML file on its `---` separators.
func splitDocs(s string) []string {
	var docs []string
	var cur []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimRight(l, " \t") == "---" {
			docs = append(docs, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, l)
	}
	return append(docs, strings.Join(cur, "\n"))
}

// stripComments drops whole-line comments, so a document that is only the header
// comment above the first `---` is recognised as empty rather than parsed.
func stripComments(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(string(data), "\n")
}
