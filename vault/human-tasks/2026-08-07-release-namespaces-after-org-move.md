# Decide where releases publish now the repo lives under `neuroplastio`

- Created: 2026-08-07
- By: the org move (import paths renamed the same day)
- Priority: normal
- Blocks: M5-11 (first release tag) — do not tag until this is settled
- Status: open

## What's needed

The repository moved to `github.com/neuroplastio/kubecom`, and the Go module path,
imports, README, install docs and ldflags moved with it. Three publishing targets
did **not**, because where they should live is a judgement call rather than a
rename:

1. **Container images** — `.goreleaser.yml:266` and `.github/workflows/release.yml:111`
   publish to `ghcr.io/anatolyrugalev/kubecom`. GHCR namespaces follow the owner,
   so the natural new home is `ghcr.io/neuroplastio/kubecom`.
2. **Homebrew tap** — `.goreleaser.yml:140` writes the cask to
   `AnatolyRugalev/homebrew-kubecom` (`owner: AnatolyRugalev`). The tap is a
   *separate repository* and was not part of the move.
3. **AUR** — `kubecom-bin` is namespaced by AUR account, not by GitHub owner, so
   it is probably unaffected. Worth confirming while the others are decided.

## Why an agent should not decide this

Each one changes where users get the software from, and two of them are
outward-facing in a way that is awkward to reverse:

- Moving the tap changes the `brew tap` incantation in every existing install
  instruction, and a tap that moves without a redirect leaves people on a dead
  formula.
- Publishing to a new GHCR namespace does not migrate the old one. If anything
  ever pulled `ghcr.io/anatolyrugalev/kubecom`, that path keeps resolving to
  whatever is there.

Nothing has shipped yet — the first release tag is still open as its own task —
so this is the cheapest possible moment to settle it. After a release it is a
migration.

## Suggested resolution

Move both to the org for consistency with the repository:

- `ghcr.io/neuroplastio/kubecom`
- a `neuroplastio/homebrew-kubecom` tap, so `brew tap neuroplastio/kubecom`

Then update `.goreleaser.yml` (`owner:`, the `images:` list and the two comments
that spell the namespace out) and `.github/workflows/release.yml`, and re-run the
dry run.

If you would rather keep the tap personal, that is fine too — but say so, because
the file currently reads as though the namespace choice was deliberate, and the
next agent to touch it will otherwise have no way to tell that from an oversight.

## Not in scope

The old path references left in `vault/journal/` and
`internal/config/testdata/legacy-config.proto` are deliberate. The journal is a
historical record, and the proto is a fixture of what the 2020 build actually
wrote — rewriting either would falsify it.
