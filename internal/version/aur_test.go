package version

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// aurConfig is the sliver of .goreleaser.yml these guards read.
type aurConfig struct {
	AURs []struct {
		Name       string   `json:"name"`
		IDs        []string `json:"ids"`
		SkipUpload string   `json:"skip_upload"`
		PrivateKey string   `json:"private_key"`
		GitURL     string   `json:"git_url"`
		Depends    []string `json:"depends"`
		Conflicts  []string `json:"conflicts"`
	} `json:"aurs"`
}

// legacyAURPackage is the package name the 2020 build published
// (`master:ci/aur/publish.sh` → `aur@aur.archlinux.org:kube-commander`). It
// installs its own `/usr/bin/kubecom`, so it is what the new package must
// declare a conflict with.
const legacyAURPackage = "kube-commander"

func readAURs(t *testing.T) aurConfig {
	t.Helper()
	data, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserPath, err)
	}
	var cfg aurConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", goreleaserPath, err)
	}
	if len(cfg.AURs) == 0 {
		t.Fatalf("%s declares no aurs — the guard is not reading the config", goreleaserPath)
	}
	return cfg
}

// aurPackageName mirrors goreleaser's own defaulting
// (internal/pipe/aur/aur.go, Default): a name that does not already end in
// `-bin` gets the suffix appended. The configured name is therefore *not* the
// published name, which is precisely the mismatch the guards below exist to
// catch.
func aurPackageName(configured string) string {
	if strings.HasSuffix(configured, "-bin") {
		return configured
	}
	return configured + "-bin"
}

// TestAURIsInertWithoutItsKey is the AUR half of D173 pt 2: a publisher that
// needs a human-owned secret must skip when the secret is absent, never fail.
//
// The AUR pipe has three separate ways to no-op — an empty `private_key`, an
// empty `git_url`, and `skip_upload` — and only the last one is checked before
// goreleaser starts doing work. Conditioning `skip_upload` on the same env var
// the key reads is what makes "the maintainer has not created the secret yet"
// an explicit, first-thing skip rather than an accident of two empty strings.
//
// As with the cask, this cannot be caught by running anything: publishers do
// not run under `--snapshot`, so the CI dry run is blind to them, and the first
// real execution is a tag push that nobody can retry (D173 pt 1).
func TestAURIsInertWithoutItsKey(t *testing.T) {
	cfg := readAURs(t)

	for _, pkg := range cfg.AURs {
		name := aurPackageName(pkg.Name)

		key := envRef.FindStringSubmatch(pkg.PrivateKey)
		if key == nil {
			t.Errorf("aur %q: private_key is %q, want an %s template so an absent secret yields \"\" instead of an error",
				name, pkg.PrivateKey, `{{ index .Env "NAME" }}`)
			continue
		}
		secret := key[1]

		if pkg.SkipUpload == "" {
			t.Errorf("aur %q: no skip_upload — without %s goreleaser would reach the publish step and skip on an empty key, which is a no-op that reads as success (D173 pt 2)",
				name, secret)
			continue
		}
		if !strings.Contains(pkg.SkipUpload, secret) {
			t.Errorf("aur %q: skip_upload %q is not conditioned on %s, the secret its private_key reads",
				name, pkg.SkipUpload, secret)
		}

		assertReleaseStepPassesSecret(t, secret)
	}
}

// aurGitURL matches the SSH remote of an AUR package repository, capturing the
// package name.
var aurGitURL = regexp.MustCompile(`^ssh://aur@aur\.archlinux\.org/([\w.+-]+)\.git$`)

// TestAURGitURLMatchesThePackageName is the guard for the trap this slice was
// written around, and it is two failures in one.
//
//  1. `git_url` has **no default**. goreleaser's git upload client returns
//     `pipe.Skip("url is empty")` when it is unset — so a config that forgets it
//     produces a green release that published nothing to the AUR, forever, with
//     no error anywhere.
//  2. goreleaser appends `-bin` to `name` but never to `git_url`, and the AUR
//     rejects a push whose `pkgbase` does not equal the repository name. So the
//     two are only correct together, and the natural spelling of `git_url`
//     (copying the configured `name`) is the wrong one.
//
// Both failures surface exactly once, on the tag push, after the GitHub release
// has already been created.
func TestAURGitURLMatchesThePackageName(t *testing.T) {
	cfg := readAURs(t)

	for _, pkg := range cfg.AURs {
		name := aurPackageName(pkg.Name)

		match := aurGitURL.FindStringSubmatch(pkg.GitURL)
		if match == nil {
			t.Errorf("aur %q: git_url is %q, want ssh://aur@aur.archlinux.org/<pkgname>.git — goreleaser has no default for it and silently skips the publish when it is empty",
				name, pkg.GitURL)
			continue
		}
		if repo := match[1]; repo != name {
			t.Errorf("aur %q: git_url points at the AUR repository %q, but goreleaser publishes the package as %q (it appends `-bin` to `name`); the AUR requires pkgbase to equal the repository name, so this push would be rejected",
				name, repo, name)
		}
	}
}

// TestAURPackageHonorsItsConstraints pins the three properties of the PKGBUILD
// that a plausible-looking config gets wrong, each for a different reason.
func TestAURPackageHonorsItsConstraints(t *testing.T) {
	cfg := readAURs(t)

	for _, pkg := range cfg.AURs {
		name := aurPackageName(pkg.Name)

		// D2: kubecom shells out to nothing. The 2020 PKGBUILD declared
		// `depends=('kubectl')`, so this is a dependency that would be
		// *restored* by copying the old package rather than introduced fresh.
		for _, dep := range pkg.Depends {
			if strings.Contains(dep, "kubectl") {
				t.Errorf("aur %q: depends on %q, but kubecom talks to the apiserver through client-go and requires no kubectl at runtime (D2)",
					name, dep)
			}
		}

		// The 2020 package installs its own /usr/bin/kubecom. Without this
		// conflict pacman would let both be installed and one binary would
		// overwrite the other, which is the file-collision equivalent of the
		// stale-Homebrew-formula trap (D182 pt 2) — except that here the
		// package manager can actually be told about it.
		if !slices.Contains(pkg.Conflicts, legacyAURPackage) {
			t.Errorf("aur %q: conflicts %v does not include %q; both packages own /usr/bin/kubecom, and unlike a Homebrew cask the AUR can express that",
				name, pkg.Conflicts, legacyAURPackage)
		}

		// The config declares two archives with the same GOOS/GOARCH pairs (a
		// tar.gz and a bare binary). The AUR pipe matches both types, and the
		// PKGBUILD keys its sources by architecture — so an unrestricted `ids`
		// emits two `source_x86_64=` lines and the second wins silently.
		if len(pkg.IDs) == 0 {
			t.Errorf("aur %q: no `ids` — the pipe would match every linux archive, including the bare-binary one, and the duplicate per-arch sources overwrite each other",
				name)
		}
	}
}

// aurPackageRef matches an AUR package name mentioned in the install docs as code.
var aurPackageRef = regexp.MustCompile("`(kubecom-bin|kube-commander-bin|kubecom-git)`")

// TestReadmeAURPackageMatchesTheConfig keeps the documented Arch instructions
// pinned to the name that is actually published.
//
// The docs currently name `kubecom-bin` only to warn a returning 2020 user
// that it is a different package from `kube-commander` and not an upgrade of it,
// so the guard is live from this leg — and it becomes load-bearing when human
// task `2026-07-30-aur-package-access` turns that warning into an install line.
func TestReadmeAURPackageMatchesTheConfig(t *testing.T) {
	cfg := readAURs(t)

	refs := aurPackageRef.FindAllStringSubmatch(installDocText(t), -1)
	if len(refs) == 0 {
		return // dormant: no AUR package documented yet
	}

	want := make([]string, 0, len(cfg.AURs))
	for _, pkg := range cfg.AURs {
		want = append(want, aurPackageName(pkg.Name))
	}

	for _, ref := range refs {
		if !slices.Contains(want, ref[1]) {
			t.Errorf("the install docs name the AUR package %q, but %s publishes %v",
				ref[1], goreleaserPath, want)
		}
	}
}
