package version

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// Paths are relative to this package dir (go test sets cwd to the package).
var (
	goreleaserPath = filepath.Join("..", "..", ".goreleaser.yml")
	goModPath      = filepath.Join("..", "..", "go.mod")
)

// goreleaserConfig is the sliver of .goreleaser.yml this guard reads.
type goreleaserConfig struct {
	Builds []struct {
		ID      string   `json:"id"`
		Ldflags []string `json:"ldflags"`
	} `json:"builds"`
}

// TestGoreleaserSetsAllVersionVars is the drift guard for release build
// metadata: every exported var in this package must be injected by
// .goreleaser.yml's ldflags, under this package's real import path.
//
// Without it the failure is invisible until after a release: the binary builds
// and runs, `kubecom version` just reports the placeholder ("commit none, built
// unknown"), and a published tag cannot be rebuilt in place (D173 pt 1). Adding
// a var here without wiring it there now fails `make check` instead.
func TestGoreleaserSetsAllVersionVars(t *testing.T) {
	pkgPath := modulePath(t) + "/internal/version"

	data, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserPath, err)
	}
	var cfg goreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", goreleaserPath, err)
	}
	if len(cfg.Builds) == 0 {
		t.Fatalf("%s declares no builds", goreleaserPath)
	}

	vars := exportedVars(t)
	if len(vars) == 0 {
		t.Fatal("no exported vars found in version.go — the guard is not reading the package")
	}

	for _, build := range cfg.Builds {
		flags := strings.Join(build.Ldflags, " ")
		for _, name := range vars {
			prefix := "-X " + pkgPath + "." + name + "="
			idx := strings.Index(flags, prefix)
			if idx < 0 {
				t.Errorf("build %q: %s does not set %s (want %q<template>)",
					build.ID, goreleaserPath, name, prefix)
				continue
			}
			value := strings.Fields(flags[idx+len(prefix):])
			if len(value) == 0 || value[0] == "" {
				t.Errorf("build %q: %s sets %s to an empty value", build.ID, goreleaserPath, name)
			}
		}
	}
}

// modulePath reads the module line from go.mod, so a module rename fails here
// rather than silently producing ldflags that match no package.
func modulePath(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read %s: %v", goModPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("%s has no module line", goModPath)
	return ""
}

// exportedVars returns the exported package-level var names declared in
// version.go — the set that must be injectable at release time.
func exportedVars(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "version.go", nil, 0)
	if err != nil {
		t.Fatalf("parse version.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, ident := range value.Names {
				if ident.IsExported() {
					names = append(names, ident.Name)
				}
			}
		}
	}
	return names
}
