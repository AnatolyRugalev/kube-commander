package version

import (
	"os"
	"regexp"
	"testing"

	"sigs.k8s.io/yaml"
)

// changelogConfig is the sliver of .goreleaser.yml these guards read.
type changelogConfig struct {
	Changelog struct {
		Filters struct {
			Exclude []string `json:"exclude"`
		} `json:"filters"`
		Groups []struct {
			Title  string `json:"title"`
			Regexp string `json:"regexp"`
		} `json:"groups"`
	} `json:"changelog"`
}

// Real subjects from this repository's history, and what a release reader should
// see of each. The point of using real ones is that the bug this guards against
// was invisible to invented examples: `^chore:` looks correct until you notice
// that every commit here is *scoped* (`chore(board): …`) and so matches nothing
// (M5-10 pre-flight, D185 pt 1).
var changelogSubjects = []struct {
	subject string
	keep    bool
}{
	{"feat(tui): M4-12b-2 theme picker + config write-back", true},
	{"fix(kube): FB-crd-parametercodec — encode list/watch params with metav1.ParameterCodec", true},
	{"perf(tui): LOGS-05b logs view stops paying per line for every line held", true},
	{"chore(board): claim M5-08", false},
	{"chore(M0-07): delete legacy trees, prune go.mod, drop lint excludes", false},
	{"docs(screencast): M5-09 vhs tape + make screencast, pinned to the keymap", false},
	{"test(config): M5-05 verify migration against a generated legacy config fixture", false},
	{"ci(lint): pin golangci-lint and run it in CI", false},
	{"build(release): M0-06 goreleaser skeleton — Linux+macOS × amd64+arm64", false},
	{"refactor(tui): M4-02 bundle the cluster-bound seams", false},
	{`wip(M0-02): partial cobra migration for cmd/kubecom`, false},
	{`Revert "wip(M0-02): partial cobra migration for cmd/kubecom"`, false},
	{"feedback: README go install @v1 fails (Go parses v1 as a version tag)", false},
	{"dogfood M3: exec passed (human-task done) + 5 feedback items", false},
}

// TestChangelogFiltersDropTheNoise is the drift guard for release notes.
//
// It exists because the failure it catches is silent and one-shot: the notes are
// written by the tag push, which nobody can retry (D173 pt 1), and a filter that
// matches nothing reads exactly like a filter that works. The first rendering of
// this repo's notes was 373 lines opening with ~180 `chore(board): claim …`
// entries, because the patterns were unscoped and every commit here is scoped.
func TestChangelogFiltersDropTheNoise(t *testing.T) {
	cfg := readChangelogConfig(t)

	excludes := cfg.Changelog.Filters.Exclude
	if len(excludes) == 0 {
		t.Fatalf("%s declares no changelog excludes", goreleaserPath)
	}
	patterns := make([]*regexp.Regexp, 0, len(excludes))
	for _, pattern := range excludes {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("%s: changelog exclude %q does not compile: %v", goreleaserPath, pattern, err)
		}
		patterns = append(patterns, re)
	}

	for _, tc := range changelogSubjects {
		excluded := false
		for _, re := range patterns {
			if re.MatchString(tc.subject) {
				excluded = true
				break
			}
		}
		switch {
		case tc.keep && excluded:
			t.Errorf("changelog would drop a user-visible change: %q", tc.subject)
		case !tc.keep && !excluded:
			t.Errorf("changelog would carry bookkeeping into the release notes: %q "+
				"(note every commit in this repo is scoped, so `^type:` matches nothing)", tc.subject)
		}
	}
}

// TestChangelogGroupsTheKeptCommits guards the other half: what survives the
// filters is grouped, so a reader scanning 140-odd rewrite commits sees features
// and fixes apart rather than one alphabetical wall. A group whose regexp misses
// the scope would silently empty itself into the catch-all.
func TestChangelogGroupsTheKeptCommits(t *testing.T) {
	cfg := readChangelogConfig(t)
	if len(cfg.Changelog.Groups) == 0 {
		t.Fatalf("%s declares no changelog groups", goreleaserPath)
	}

	var catchAll bool
	grouped := map[string]string{}
	for _, group := range cfg.Changelog.Groups {
		if group.Regexp == "" {
			catchAll = true
			continue
		}
		re, err := regexp.Compile(group.Regexp)
		if err != nil {
			t.Fatalf("%s: changelog group %q regexp %q does not compile: %v",
				goreleaserPath, group.Title, group.Regexp, err)
		}
		for _, tc := range changelogSubjects {
			if tc.keep && re.MatchString(tc.subject) {
				grouped[tc.subject] = group.Title
			}
		}
	}
	if !catchAll {
		t.Errorf("%s has no catch-all changelog group (one with no regexp); "+
			"goreleaser drops a kept commit that matches no group", goreleaserPath)
	}

	for _, want := range []string{
		"feat(tui): M4-12b-2 theme picker + config write-back",
		"fix(kube): FB-crd-parametercodec — encode list/watch params with metav1.ParameterCodec",
	} {
		if grouped[want] == "" {
			t.Errorf("no changelog group matches %q — the scope makes `^feat:`/`^fix:` miss", want)
		}
	}
}

func readChangelogConfig(t *testing.T) changelogConfig {
	t.Helper()
	data, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserPath, err)
	}
	var cfg changelogConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", goreleaserPath, err)
	}
	return cfg
}
