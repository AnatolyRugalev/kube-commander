package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// Paths are relative to this package dir (go test sets cwd to the package).
var (
	makefilePath  = filepath.Join("..", "..", "Makefile")
	workflowsDir  = filepath.Join("..", "..", ".github", "workflows")
	ciWorkflowRel = "ci.yml"
)

// envtestTarget is the one command that runs the gated suite. CI runs exactly
// this so a leg and a CI job cannot diverge in how the suite is invoked.
const envtestTarget = "make test-envtest"

// ciWorkflow is the sliver of a workflow file this guard reads.
type ciWorkflow struct {
	// GitHub spells the trigger block `on:`, which YAML 1.1 — what
	// sigs.k8s.io/yaml parses with on its way to JSON — reads as the *boolean*
	// true. So after conversion the key is literally "true"; there is no "on".
	On   map[string]any `json:"true"`
	Jobs map[string]struct {
		Steps []struct {
			Run string `json:"run"`
		} `json:"steps"`
	} `json:"jobs"`
}

// TestEnvtestSuiteRunsInCI is the drift guard for M1-INT-d: it ties the gate
// constant, the Makefile target and the CI workflow together so the envtest
// suite cannot quietly stop running.
//
// It exists because the failure mode a CI job introduces here is a **green** one.
// `go test ./internal/kube/...` with the gate unset passes with every
// TestEnvtest* skipped, so dropping `KUBECOM_TEST_ENVTEST=1` from the recipe, or
// renaming the constant on the Go side, leaves CI reporting success over a suite
// that ran nothing — exactly the rot the job was added to prevent. Every other
// way this job can break is loud: a failed download exits non-zero, and a wrong
// KUBEBUILDER_ASSETS fails env.Start().
//
// It is hermetic (it reads files, not a cluster), so unlike its neighbours in
// this package it is *not* behind requireEnvtest and runs in `make check`.
func TestEnvtestSuiteRunsInCI(t *testing.T) {
	makefile, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %s: %v", makefilePath, err)
	}

	// 1. The target sets the gate. Written against the Go constant, so renaming
	//    it on either side fails here rather than silently skipping the suite.
	recipe := makeRecipe(t, string(makefile), "test-envtest")
	if want := envtestGateEnv + "=1"; !strings.Contains(recipe, want) {
		t.Errorf("%s: the test-envtest recipe does not set %s; the suite would skip and CI would still be green:\n%s",
			makefilePath, want, recipe)
	}

	// 2. The gate stays off `make check` (D17/D18): "green" must keep meaning the
	//    same hermetic thing in CI as in a sandbox.
	for _, prereq := range makePrereqs(t, string(makefile), "check") {
		if prereq == "test-envtest" {
			t.Errorf("%s: `check` depends on test-envtest; the canonical gate must not download a control plane (D17/D18)",
				makefilePath)
		}
	}

	// 3. Some workflow actually runs it — on push, not only by hand — and it is
	//    not ci.yml, which release.yml calls as the tag gate (D173 pt 1/D190).
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		t.Fatalf("read %s: %v", workflowsDir, err)
	}
	var runners []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
			continue
		}
		path := filepath.Join(workflowsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var wf ciWorkflow
		if err := yaml.Unmarshal(data, &wf); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if !runsEnvtest(wf) {
			continue
		}
		if name == ciWorkflowRel {
			t.Errorf("%s runs %q; it is the gate release.yml calls for a `v*` tag, so a control-plane "+
				"download failure there would block a release of a green tree (D190)", path, envtestTarget)
			continue
		}
		if _, ok := wf.On["push"]; !ok {
			t.Errorf("%s runs %q but not on push, so the suite still only runs when someone remembers to",
				path, envtestTarget)
			continue
		}
		runners = append(runners, name)
	}
	if len(runners) == 0 {
		t.Errorf("no workflow in %s runs %q on push — the gated envtest suite (%d live tests) runs nowhere automatically (M1-INT-d)",
			workflowsDir, envtestTarget, countEnvtestTests(t))
	}
}

// runsEnvtest reports whether any step of any job runs the envtest target.
func runsEnvtest(wf ciWorkflow) bool {
	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, envtestTarget) {
				return true
			}
		}
	}
	return false
}

// makeRecipe returns the recipe lines (tab-indented) of a Makefile target.
func makeRecipe(t *testing.T, makefile, target string) string {
	t.Helper()
	lines := strings.Split(makefile, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, target+":") {
			continue
		}
		var recipe []string
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "\t") {
				break
			}
			recipe = append(recipe, next)
		}
		return strings.Join(recipe, "\n")
	}
	t.Fatalf("%s declares no %q target — the guard is not reading the Makefile", makefilePath, target)
	return ""
}

// makePrereqs returns the prerequisites listed after a Makefile target's colon.
func makePrereqs(t *testing.T, makefile, target string) []string {
	t.Helper()
	for _, line := range strings.Split(makefile, "\n") {
		if rest, ok := strings.CutPrefix(line, target+":"); ok {
			return strings.Fields(rest)
		}
	}
	t.Fatalf("%s declares no %q target — the guard is not reading the Makefile", makefilePath, target)
	return nil
}

// countEnvtestTests counts the gated tests in this package, so the "nothing runs
// them" failure says how much coverage is at stake rather than being abstract.
func countEnvtestTests(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob("envtest_*_test.go")
	if err != nil {
		t.Fatalf("glob envtest test files: %v", err)
	}
	matches = append(matches, "envtest_test.go")
	var count int
	for _, path := range matches {
		if path == "envtest_ci_test.go" {
			// This guard is hermetic, not gated — it is not part of what is at stake.
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		count += strings.Count(string(data), "\nfunc TestEnvtest")
	}
	return count
}
