# Custom resources are really hard to use — let them be pinned per context on first use

- Submitted: 2026-08-01
- Priority: high
- Area: CRDs, resource menu, per-context config

Custom resources are really hard to use. We need a system where custom resources are
**added into kubecom once they are first used** and can be **deleted from it later**.
Should be stored in **per-context config**.

## The shape I'm after

A CRD-heavy cluster has hundreds of kinds (the dogfood k3d cluster alone has the whole
`hub.traefik.io` + `gateway.networking.k8s.io` + `generators.external-secrets.io` sets).
Putting all of them in the menu makes the menu useless; leaving them all out makes CRDs
unreachable. So: you reach for one *once* — via search or the palette — and from then on
it is **in your menu for that context**, until you remove it. The menu becomes the small
set of kinds you actually work with on that cluster, and it accumulates by use rather
than by configuration.

Removal has to be as easy as the adding: a key on the menu row, not a config-file edit.

## Grounding (what already exists)

- **Per-context state already exists**: `internal/config/state.go` defines `State`,
  written to `StateDir()/<sanitized-context>.yaml`, currently holding `LastNamespace`
  (D163). A pinned-kinds list is the same kind of thing — recorded *for* you as you work,
  not authored by you — so it likely belongs in `State`, not in `config.yaml`.
- But note there are **already per-context menu files** (`menus/<context>.yaml`,
  `internal/config/menu.go`, `MenuConfig`) which *are* user-authored. So there is a real
  design question to settle and record: does a pinned CRD land in the authored menu file
  (visible, hand-editable, survives as config) or in the state file (invisible, automatic,
  "recorded for you")? Those two files exist precisely because that distinction was drawn
  deliberately — pick the side consciously rather than by convenience.
- The context-switch path already rebuilds the menu from `menus/<context>.yaml` per D163,
  so whichever file this lands in must be part of that rebuild, or a switch will show the
  wrong cluster's pins.

## Relationship to CRD-01

Unrelated to the `ExternalSecret` LIST error (that turned out to be cluster-side — see the
closed human task `2026-07-29-external-secrets-crd-error-log`). **This is the usability
half of CRDs, not the error half.** Don't let one absorb the other.

Larger than a leg — triage into slices and land the first.
