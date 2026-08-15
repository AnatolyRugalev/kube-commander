// Package stories holds the drift guards for the story fixture — the committed
// k3s cluster under `stories/` that the UX stories, and the screencast tape, are
// run against (D268).
//
// It carries no production code and is never imported, for the same reason
// internal/vault does not: `make check` is the only gate this project has (D17),
// so a rule about a repository file that a human must keep true is checked by a
// test rather than by remembering. The rule here has one shape — **the fixture
// and the things that name it must agree**. A story or a tape that filters for
// `shop` against a fixture with no `shop` in it does not fail loudly; it records
// an empty table and looks like a UX finding, which is the most expensive kind of
// wrong answer this line can produce.
//
// What it cannot check is that the cluster comes up: that needs docker and a real
// k3d run, which the sandbox does not have (D79). These guards cover the static
// half — that the manifests parse, that every object lands in a namespace the
// fixture declares, and that the strings the tape types still match something.
//
// Files are read relative to this package (`../../stories/...`), the same way
// internal/vault reads `../../vault/...`.
package stories
