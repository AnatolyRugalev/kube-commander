package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// installDocs are the user-facing documents that carry install instructions: the
// README's short install section and the `docs/install.md` it links to, which is
// where the per-path detail (archives, container, Homebrew, AUR) moved when the
// README was restructured around the capability surface (DOC-01/D241).
//
// The three README guards below read this *set* rather than the README alone. A
// guard that keys on one file silently goes dormant the day a section is
// relocated — each of them returns early on "no install path documented yet" —
// so relocating the text would have retired three live checks and nothing would
// have failed.
var installDocs = []string{
	filepath.Join("..", "..", "README.md"),
	filepath.Join("..", "..", "docs", "install.md"),
}

// installDocText returns the concatenated contents of installDocs. Concatenated
// because every guard over it is a scan for references, not a per-file claim.
func installDocText(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, p := range installDocs {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

// caskConfig is the sliver of .goreleaser.yml these guards read.
type caskConfig struct {
	Casks []struct {
		Name       string `json:"name"`
		SkipUpload string `json:"skip_upload"`
		Repository struct {
			Owner string `json:"owner"`
			Name  string `json:"name"`
			Token string `json:"token"`
		} `json:"repository"`
	} `json:"homebrew_casks"`
}

// envRef matches goreleaser's safe env lookup, `{{ index .Env "NAME" }}`.
//
// The `index` spelling is load-bearing and not interchangeable with `.Env.NAME`:
// `index` returns "" for an absent key, while `.Env.NAME` errors out — which
// would turn "the maintainer has not created the secret yet" into "the release
// run fails", the exact outcome skip_upload exists to prevent.
var envRef = regexp.MustCompile(`index\s+\.Env\s+"([A-Z0-9_]+)"`)

func readCasks(t *testing.T) caskConfig {
	t.Helper()
	data, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserPath, err)
	}
	var cfg caskConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", goreleaserPath, err)
	}
	if len(cfg.Casks) == 0 {
		t.Fatalf("%s declares no homebrew_casks — the guard is not reading the config", goreleaserPath)
	}
	return cfg
}

// TestHomebrewCaskIsInertWithoutItsToken is the guard for D173 pt 2 / D182 pt 3:
// a publisher that needs a human-owned secret must *skip* when the secret is
// absent, never fail.
//
// The failure it prevents cannot be caught by running anything. Publishers do
// not run under `--snapshot`, so the CI dry run is blind to them; the first
// execution of this code path is the tag push, which the Go module proxy caches
// permanently and nobody can retry (D173 pt 1). Without a token-conditioned
// skip_upload, goreleaser falls back to the workflow's own GITHUB_TOKEN — which
// is scoped to this repository and cannot write to the tap — so the release
// would die *after* creating the GitHub release, leaving a half-published tag.
//
// Three things must agree, and the point of the test is that they are three:
// the token comes from an env var, skip_upload is conditioned on that same env
// var, and the release workflow actually passes it to goreleaser.
func TestHomebrewCaskIsInertWithoutItsToken(t *testing.T) {
	cfg := readCasks(t)

	for _, cask := range cfg.Casks {
		token := envRef.FindStringSubmatch(cask.Repository.Token)
		if token == nil {
			t.Errorf("cask %q: repository.token is %q, want an %s template so an absent secret yields \"\" instead of an error",
				cask.Name, cask.Repository.Token, `{{ index .Env "NAME" }}`)
			continue
		}
		secret := token[1]

		if cask.SkipUpload == "" {
			t.Errorf("cask %q: no skip_upload — a release without %s would try to push to the tap with the wrong credentials and fail (D173 pt 2)",
				cask.Name, secret)
			continue
		}
		if !strings.Contains(cask.SkipUpload, secret) {
			t.Errorf("cask %q: skip_upload %q is not conditioned on %s, the secret its token reads",
				cask.Name, cask.SkipUpload, secret)
		}

		assertReleaseStepPassesSecret(t, secret)
	}
}

// assertReleaseStepPassesSecret checks the real (non-snapshot) goreleaser step
// hands the secret down. A secret that exists in repository settings but never
// reaches the process is indistinguishable from one that was never created:
// the cask silently skips, and every release quietly ships without it.
func assertReleaseStepPassesSecret(t *testing.T, secret string) {
	t.Helper()

	data, err := os.ReadFile(releaseWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", releaseWorkflowPath, err)
	}
	var wf releaseWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parse %s: %v", releaseWorkflowPath, err)
	}

	var found bool
	for name, job := range wf.Jobs {
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
				continue
			}
			// Only the real publishing run needs the secret: `--snapshot` never
			// runs publishers, and `check` only parses the config.
			args, _ := step.With["args"].(string)
			if !strings.Contains(args, "release") || strings.Contains(args, "--snapshot") {
				continue
			}
			found = true
			if _, ok := step.Env[secret]; !ok {
				t.Errorf("job %q: the `goreleaser release` step does not pass %s, so the cask would silently skip on every release",
					name, secret)
			}
		}
	}
	if !found {
		t.Fatalf("%s has no real `goreleaser release` step — the guard is not reading the workflow", releaseWorkflowPath)
	}
}

// brewTap matches a `brew tap <owner>/<name>` line in the install docs.
var brewTap = regexp.MustCompile(`brew\s+tap\s+([\w.-]+)/([\w.-]+)`)

// TestReadmeBrewTapMatchesTheCask keeps the documented Homebrew instructions and
// the tap goreleaser publishes to from drifting apart.
//
// Homebrew's shorthand drops the `homebrew-` prefix — the repository
// `AnatolyRugalev/homebrew-kubecom` is tapped as `AnatolyRugalev/kubecom` — and
// that asymmetry is exactly where a plausible-looking README line goes wrong.
// A wrong tap is a silent failure of the worst kind: the command works, it just
// taps a repository that does not exist or is not ours.
//
// The docs name the tap today only to tell a returning 2020 user that its
// address survives and that what it currently serves is the old formula, so the
// guard is live from this leg. It stays live — and becomes load-bearing rather
// than merely correct — when human task `2026-07-30-homebrew-tap-access` turns
// that mention into a real install path.
func TestReadmeBrewTapMatchesTheCask(t *testing.T) {
	cfg := readCasks(t)

	taps := brewTap.FindAllStringSubmatch(installDocText(t), -1)
	if len(taps) == 0 {
		return // dormant: no Homebrew install path documented yet
	}

	want := make(map[string]bool, len(cfg.Casks))
	for _, cask := range cfg.Casks {
		want[cask.Repository.Owner+"/"+cask.Repository.Name] = true
	}

	for _, tap := range taps {
		// `brew tap owner/x` resolves to the repository `owner/homebrew-x`.
		repo := tap[1] + "/homebrew-" + tap[2]
		if !want[repo] {
			t.Errorf("the install docs say `brew tap %s/%s`, i.e. the repository %s, which no cask in %s publishes to",
				tap[1], tap[2], repo, goreleaserPath)
		}
	}
}
