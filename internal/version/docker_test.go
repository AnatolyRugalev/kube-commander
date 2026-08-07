package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

var dockerfilePath = filepath.Join("..", "..", "Dockerfile")

// dockerConfig is the sliver of .goreleaser.yml these guards read. `builds` is
// here too because the image and the archives must agree on the binary name —
// that agreement is the whole point of building the image from the released
// artifact instead of from source.
type dockerConfig struct {
	Builds []struct {
		ID     string `json:"id"`
		Binary string `json:"binary"`
	} `json:"builds"`
	DockersV2 []struct {
		ID         string            `json:"id"`
		IDs        []string          `json:"ids"`
		Dockerfile string            `json:"dockerfile"`
		Images     []string          `json:"images"`
		Platforms  []string          `json:"platforms"`
		Tags       []string          `json:"tags"`
		Labels     map[string]string `json:"labels"`
	} `json:"dockers_v2"`
}

func readDockers(t *testing.T) dockerConfig {
	t.Helper()
	data, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserPath, err)
	}
	var cfg dockerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", goreleaserPath, err)
	}
	if len(cfg.DockersV2) == 0 {
		t.Fatalf("%s declares no dockers_v2 — the guard is not reading the config", goreleaserPath)
	}
	return cfg
}

func readDockerfile(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read %s: %v", dockerfilePath, err)
	}
	return string(data)
}

// dockerfileInstructions returns the Dockerfile's instructions as
// (verb, arguments) pairs, comments and blank lines dropped and line
// continuations joined. Enough of a parser for the four properties below; not
// enough to be a Dockerfile parser, deliberately.
func dockerfileInstructions(t *testing.T, content string) [][2]string {
	t.Helper()
	var out [][2]string
	var pending string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, `\`) {
			pending += strings.TrimSuffix(line, `\`) + " "
			continue
		}
		line, pending = pending+line, ""
		verb, args, _ := strings.Cut(line, " ")
		out = append(out, [2]string{strings.ToUpper(verb), strings.TrimSpace(args)})
	}
	return out
}

// TestDockerfileShipsTheReleasedBinary is the guard for the property that
// separates this image from the 2020 one: it must ship the *same* artifact the
// archives, the cask and the AUR package carry, not a rebuild of it.
//
// The 2020 Dockerfile had a `golang:1.15-alpine` build stage, so the published
// image contained a binary compiled by a different toolchain, with none of the
// version ldflags (M5-02/D175) and no relationship to checksums.txt. Nothing
// about that is visible from the outside — the image runs, `kubecom version`
// just lies — which is why it is a test and not a review note.
//
// The COPY source and the build's binary name are checked together because
// they fail together: rename `binary:` in .goreleaser.yml and the build context
// no longer holds `kubecom`, so the COPY fails at release time, on the tag push
// that cannot be retried (D173 pt 1).
func TestDockerfileShipsTheReleasedBinary(t *testing.T) {
	cfg := readDockers(t)
	instructions := dockerfileInstructions(t, readDockerfile(t))

	var froms, copies []string
	for _, ins := range instructions {
		switch ins[0] {
		case "FROM":
			froms = append(froms, ins[1])
		case "COPY", "ADD":
			copies = append(copies, ins[1])
		case "RUN":
			t.Errorf("Dockerfile: RUN %q — this image is a released binary plus a base, and a RUN both requires QEMU for the foreign architecture and reintroduces the 2020 image's kubectl/nano/jq payload (D2)", ins[1])
		}
	}

	if len(froms) != 1 {
		t.Fatalf("Dockerfile: %d FROM instructions (%v), want exactly 1 — a second stage means the image is built from source instead of from the release artifact", len(froms), froms)
	}
	if strings.Contains(strings.ToLower(froms[0]), " as ") {
		t.Errorf("Dockerfile: FROM %q names a stage, so something later builds on it; the image must be the released binary over a base, nothing more", froms[0])
	}
	// A static Go binary needs a CA bundle to speak TLS to an apiserver, and
	// scratch has none. The failure is x509 on every connection, against a real
	// cluster only — invisible to every hermetic test in this repo.
	if base := strings.Fields(froms[0])[0]; base == "scratch" {
		t.Errorf("Dockerfile: FROM scratch ships no ca-certificates, so every apiserver connection fails x509 verification; use a base that carries a CA bundle")
	}

	// The build id the image is fed from decides which binary name to expect.
	binaries := map[string]string{}
	for _, b := range cfg.Builds {
		binaries[b.ID] = b.Binary
	}
	for _, d := range cfg.DockersV2 {
		if len(d.IDs) == 0 {
			t.Errorf("dockers_v2 %q: no `ids` — every build's binary would land in the build context, so a future second binary silently ships inside the image", d.ID)
			continue
		}
		for _, id := range d.IDs {
			binary, ok := binaries[id]
			if !ok {
				t.Errorf("dockers_v2 %q: ids references build %q, which %s does not declare", d.ID, id, goreleaserPath)
				continue
			}
			// goreleaser lays the build context out as <goos>/<goarch>/<binary>,
			// which is what $TARGETPLATFORM expands to. Without the matching ARG
			// the variable is empty and the COPY silently resolves to /<binary>.
			want := "$TARGETPLATFORM/" + binary
			if !containsSource(copies, want) {
				t.Errorf("dockers_v2 %q: no COPY from %q in the Dockerfile (found %v); goreleaser lays the release binaries out as <goos>/<goarch>/<binary> in the build context",
					d.ID, want, copies)
			}
		}
	}

	var hasTargetPlatformArg bool
	for _, ins := range instructions {
		if ins[0] == "ARG" && strings.HasPrefix(ins[1], "TARGETPLATFORM") {
			hasTargetPlatformArg = true
		}
	}
	if !hasTargetPlatformArg {
		t.Error("Dockerfile: no `ARG TARGETPLATFORM` — buildx only predeclares it, so without the ARG the COPY source expands to an empty string and copies the wrong path")
	}
}

func containsSource(copies []string, want string) bool {
	for _, c := range copies {
		for _, field := range strings.Fields(c) {
			if field == want {
				return true
			}
		}
	}
	return false
}

// prereleaseGuard matches a tag template that is conditioned on .Prerelease.
var prereleaseGuard = regexp.MustCompile(`\{\{-?\s*if\s+\.Prerelease`)

// TestDockerLatestTagSkipsPrereleases guards the one tag that keeps moving.
//
// M5-10 recommends cutting `v1.0.0-rc.1` before `v1.0.0` precisely because a
// pre-release is cheap to get wrong: Go's module proxy excludes it from
// `@latest` and Homebrew/AUR users have to ask for it by name. A Docker tag has
// no such notion — `docker run ghcr.io/anatolyrugalev/kubecom` resolves `latest`
// literally, so an unguarded `latest` would hand every new user the release
// that was explicitly labelled not-ready.
func TestDockerLatestTagSkipsPrereleases(t *testing.T) {
	cfg := readDockers(t)

	for _, d := range cfg.DockersV2 {
		if len(d.Tags) == 0 {
			t.Errorf("dockers_v2 %q: no tags", d.ID)
			continue
		}
		for _, tag := range d.Tags {
			if !strings.Contains(tag, "latest") {
				continue
			}
			if !prereleaseGuard.MatchString(tag) {
				t.Errorf("dockers_v2 %q: tag %q publishes `latest` unconditionally; a pre-release tag (M5-10 recommends v1.0.0-rc.1 first) would then become what a bare `docker run` pulls",
					d.ID, tag)
			}
		}
	}
}

// dockerWorkflow is the sliver of release.yml the publish guard reads. It adds
// job `permissions` on top of the shared releaseWorkflow struct, which the
// earlier guards had no reason to look at.
type dockerWorkflow struct {
	Jobs map[string]struct {
		Permissions map[string]string `json:"permissions"`
		Steps       []struct {
			Uses string         `json:"uses"`
			With map[string]any `json:"with"`
		} `json:"steps"`
	} `json:"jobs"`
}

// TestReleaseWorkflowCanPublishTheDockerImage guards the workflow half of the
// Docker slice — three pieces of wiring that live outside .goreleaser.yml and
// each of which fails only on a tag push (D173 pt 1).
//
//  1. `packages: write` on the publishing job. GITHUB_TOKEN defaults to
//     read-only for packages, so without it the push is a 403 after the GitHub
//     release has already been created.
//  2. A buildx builder on the docker-container driver. `dockers_v2` asks for a
//     multi-platform index and an SBOM attestation and the default `docker`
//     driver supports neither — it errors rather than degrading.
//  3. A registry login. The image is public but the push is not.
//
// The snapshot job is checked for the builder too: without it the dry run would
// exercise a different buildx driver than the tag does, which is the drift the
// dry run exists to catch (D176).
func TestReleaseWorkflowCanPublishTheDockerImage(t *testing.T) {
	cfg := readDockers(t)

	data, err := os.ReadFile(releaseWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", releaseWorkflowPath, err)
	}
	var wf dockerWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parse %s: %v", releaseWorkflowPath, err)
	}
	var plain releaseWorkflow
	if err := yaml.Unmarshal(data, &plain); err != nil {
		t.Fatalf("parse %s: %v", releaseWorkflowPath, err)
	}

	// Every registry the config publishes to needs a login step in the same job.
	registries := map[string]bool{}
	for _, d := range cfg.DockersV2 {
		for _, img := range d.Images {
			if img != strings.ToLower(img) {
				t.Errorf("dockers_v2 %q: image %q has uppercase characters; registries reject them in a repository path", d.ID, img)
			}
			registries[strings.SplitN(img, "/", 2)[0]] = true
		}
	}

	var checkedPublish, checkedSnapshot bool
	for name, job := range plain.Jobs {
		var publish, snapshot bool
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
				continue
			}
			args, _ := step.With["args"].(string)
			if !strings.Contains(args, "release") {
				continue
			}
			if strings.Contains(args, "--snapshot") {
				snapshot = true
			} else {
				publish = true
			}
		}
		if !publish && !snapshot {
			continue
		}

		var hasBuildx bool
		logins := map[string]bool{}
		for _, step := range wf.Jobs[name].Steps {
			switch {
			case strings.HasPrefix(step.Uses, "docker/setup-buildx-action@"):
				hasBuildx = true
			case strings.HasPrefix(step.Uses, "docker/login-action@"):
				registry, _ := step.With["registry"].(string)
				logins[registry] = true
			}
		}
		if !hasBuildx {
			t.Errorf("job %q: no docker/setup-buildx-action step; the default buildx driver cannot produce the multi-platform index or the SBOM attestation dockers_v2 asks for", name)
		}

		if snapshot {
			checkedSnapshot = true
			continue // a snapshot pushes nothing, so it needs neither login nor permission
		}
		checkedPublish = true

		if perm := wf.Jobs[name].Permissions["packages"]; perm != "write" {
			t.Errorf("job %q: permissions.packages is %q, want \"write\" — GITHUB_TOKEN is read-only for packages by default, so the image push 403s after the GitHub release already exists",
				name, perm)
		}
		for registry := range registries {
			if !logins[registry] {
				t.Errorf("job %q: no docker/login-action for %q, so the push is unauthenticated", name, registry)
			}
		}
	}
	if !checkedPublish || !checkedSnapshot {
		t.Fatalf("%s: found publish=%v snapshot=%v goreleaser jobs — the guard is not reading the workflow",
			releaseWorkflowPath, checkedPublish, checkedSnapshot)
	}
}

// dockerImageRef matches a ghcr.io image reference written as code in the docs.
var dockerImageRef = regexp.MustCompile(`ghcr\.io/[a-z0-9._/-]+`)

// TestReadmeDockerImageMatchesTheConfig keeps the documented `docker run` line
// pinned to the image the release actually publishes. A stale registry path in
// an install instruction is worse than none: it fails with "not found", which
// reads as "this project is broken" rather than "this doc is old".
func TestReadmeDockerImageMatchesTheConfig(t *testing.T) {
	cfg := readDockers(t)

	published := map[string]bool{}
	for _, d := range cfg.DockersV2 {
		for _, img := range d.Images {
			published[img] = true
		}
	}

	refs := dockerImageRef.FindAllString(installDocText(t), -1)
	if len(refs) == 0 {
		return // dormant: no Docker install path documented yet
	}
	for _, ref := range refs {
		// The docs write tags; the config declares them separately.
		if image, _, ok := strings.Cut(ref, ":"); ok {
			ref = image
		}
		if !published[ref] {
			t.Errorf("the install docs name the image %q, which %s does not publish (it publishes %v)",
				ref, goreleaserPath, keysOf(published))
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
