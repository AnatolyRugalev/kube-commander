package config

import (
	"reflect"
	"strings"
	"testing"
)

// A `menu` with no context applies to every context; a context's own entries come
// after those, each in file order.
func TestMenuDocuments(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, AuthoredName, `version 1
menu "prod" {
    resource "rollouts" group="argoproj.io" version="v1alpha1" kind="Rollout" namespaced=#true
}
menu {
    resource "certificates" group="cert-manager.io" version="v1" kind="Certificate" namespaced=#true section="TLS" title="Certs"
}
menu "dev" {
    resource "widgets" group="example.com" version="v1"
}
`)
	c := mustLoad(t, dir)
	cert := MenuResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates", Kind: "Certificate",
		Namespaced: true, Section: "TLS", Title: "Certs"}
	rollout := MenuResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts", Kind: "Rollout", Namespaced: true}
	if got := c.Menu("prod"); !reflect.DeepEqual(got, []MenuResource{cert, rollout}) {
		t.Errorf("prod menu = %+v", got)
	}
	if got := c.Menu("staging"); !reflect.DeepEqual(got, []MenuResource{cert}) {
		t.Errorf("staging menu = %+v", got)
	}
	if got := c.Menu("dev"); len(got) != 2 || got[1].Group != "example.com" || got[1].Namespaced {
		t.Errorf("dev menu = %+v", got)
	}
}

// The core group is written by leaving group out.
func TestMenuCoreGroup(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, AuthoredName, "version 1\nmenu { resource \"configmaps\" version=\"v1\" namespaced=#true; }\n")
	if got := mustLoad(t, dir).Menu("x"); len(got) != 1 || got[0].Group != "" || got[0].Resource != "configmaps" {
		t.Fatalf("menu = %+v", got)
	}
}

// An entry the kube layer could not address, or a menu written wrong, fails the
// load at its line.
func TestMenuErrors(t *testing.T) {
	for src, want := range map[string]string{
		`menu { resource "certificates" group="cert-manager.io"; }`: ":2:8: resource: version is required",
		`menu { resource version="v1"; }`:                           "takes the plural resource name",
		`menu { resource "" version="v1"; }`:                        "the resource name is required",
		`menu { resource "a" version="v1" colour="red"; }`:          `has no property "colour"`,
		`menu { resource "a" version="v1" namespaced="yes"; }`:      "namespaced is #true or #false",
		`menu { resource "a" version=1; }`:                          "version is a string",
		`menu { kind "a"; }`:                                        "a menu holds `resource` nodes, not \"kind\"",
		`menu "a" "b" { }`:                                          "menu takes a context name, or none",
		`menu ctx="a" { }`:                                          "menu takes a context name, or none",
		`menu { resource "a" version="v1" { x }; }`:                 "takes no block",
	} {
		dir := t.TempDir()
		write(t, dir, AuthoredName, "version 1\n"+src+"\n")
		if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
}

// The overlay is kubecom's, and kubecom writes no menu.
func TestTheOverlayHoldsNoMenu(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, OverlayName, "version 1\nmenu { }\n")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "the overlay holds no menu") {
		t.Fatalf("overlay menu: %v", err)
	}
}
