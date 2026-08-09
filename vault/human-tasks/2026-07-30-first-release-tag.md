# Push the first release tag — `v1.0.0-rc.1` first, then `v1.0.0`

- Created: 2026-07-30
- By: M5-10 (release pre-flight)
- Priority: high
- Blocks: M5-11
- Status: open

## What's needed

The pre-flight is done: the pipeline, the two publishers, the ldflags, the migration
and the docs are all landed and dry-run clean (see `## Pre-flight result` below). What
is left is the one act an agent must not perform (D173 pt 1) — pushing the tag that
publishes.

**Do it in two steps, and cut the release candidate first.** A pre-release costs
nothing to get wrong: `proxy.golang.org` caches every version permanently, so a bad
`v1.0.0` can never be replaced (only superseded by `v1.0.1`), while `v1.0.0-rc.1` is
excluded from `@latest` by Go, and is something Homebrew/the AUR make you ask for by
name. (The container image is gone — builds were dropped on 2026-08-09, D254 — so there
is no image-side pre-release handling to reason about.)

### 1. The release candidate

```bash
git checkout v1 && git pull
git tag -a v1.0.0-rc.1 -m 'kubecom v1.0.0-rc.1'
git push origin v1.0.0-rc.1
```

Then watch `.github/workflows/release.yml` (it runs `make check` first, then
`goreleaser release --clean`) and check, in this order:

1. **The GitHub release exists** with four `.tar.gz` archives, four bare binaries and
   `checksums.txt`, and is marked *pre-release*.
2. **The binary reports itself**: download one, run `kubecom version` — it must print
   the real version, the full commit and the build date, not `commit none, built
   unknown`.
3. **The release notes read sensibly** — features and fixes grouped, no
   `chore(board): claim …` lines. They span from the 2020 tag `0.7.6`, so they are
   long (~150 lines) and include the whole rewrite. That is expected for the first
   release only; edit the body freely if you want a shorter story, the release body is
   the one part of a release that *is* editable.
4. **`go install github.com/neuroplastio/kubecom/cmd/kubecom@v1.0.0-rc.1`**
   works from a clean `GOPATH` — this is what finally proves the module path is
   installable remotely (FB-go-install).

The Homebrew and AUR steps will **skip themselves** unless their secrets exist
(`HOMEBREW_TAP_TOKEN`, `AUR_SSH_PRIVATE_KEY` — human tasks
`2026-07-30-homebrew-tap-access` and `2026-07-30-aur-package-access`). That is by
design (D173 pt 2): a missing credential must never fail a release run. If you want
the rc to exercise them too, do those two tasks first.

### 2. The release

Once the rc looks right:

```bash
git tag -a v1.0.0 -m 'kubecom v1.0.0'
git push origin v1.0.0
```

This one creates — with the secrets in place — the Homebrew cask (on the org tap
`neuroplastio/homebrew-tap`, D253) and the AUR package. Verify at least one install path
end to end (`brew install --cask neuroplastio/tap/kubecom` or `yay -S kubecom-bin`)
and say which in the `## Result` below — those are two thirds of an M5 exit criterion.

### 3. After the tag (agent work, listed here so it is not lost)

Set `Status: done`, add a `## Result`, and the next leg will:

- drop the "why not `@v1`" explainer and restore `go install …/cmd/kubecom@latest`
  as the primary install line (FB-go-install) — since DOC-01 that explainer lives in
  [`docs/install.md`](../../docs/install.md), and the README's short install block
  and its `v1`-checkout line change with it;
- retire the "no version has been tagged yet" status note in the README;
- tick the M5 exit criteria the tag closes, plus the DoD's "Linux + macOS release
  artifacts via goreleaser + GitHub Actions" box;
- close the resolved GitHub issues (#8, #28, #68, #76, #80, #83, #84, #85, #86, #87,
  #89 — each resolved in code, none yet closed on the tracker) and the rewrite
  announcement issue #90, now that a release actually carries the fixes;
- proceed to M5-11 (`v1` → `main`).

## Why the agent can't do it

A pushed tag leaves the repo and cannot be recalled: the Go module proxy caches the
version immutably and the distributors mirror it. D173 pt 1 draws the line here —
agents prepare and dry-run, a human publishes. Everything in this task that *could* be
done from the sandbox already has been.

## Pre-flight result (M5-10, 2026-07-30)

What was checked, and how:

- **Every earlier M5 slice has landed**: M5-01/01a/01b (DoD audit + its two findings),
  M5-02 (Commit/Date ldflags), M5-03 (`release.yml` + CI dry run), M5-04/05
  (migration), M5-06/07/08 (Homebrew, AUR, Docker), M5-09 (screencast tape). Each has
  guards in `internal/version/*_test.go` that fail `make check` on config drift.
- **`goreleaser check` is clean** and **`goreleaser release --clean` was run against a
  real local `v1.0.0-rc.1` tag** with `--skip=publish` — it built all four platforms,
  rendered the cask, the PKGBUILD/`.SRCINFO` and the changelog, and succeeded. The tag
  was deleted afterwards and never pushed.
- **The changelog filters were broken and are fixed** (D185 pt 1) — they matched
  unscoped subjects only, so the first notes would have been 373 lines opening with
  ~180 `chore(board): claim …` entries. Now 151, grouped, guarded by
  `TestChangelogFiltersDropTheNoise`.
- **The README's install paths match what ships**: the cask (macOS-only, on the org tap),
  the `kubecom-bin` AUR rename, and the release archives are each documented and each
  drift-guarded against `.goreleaser.yml`. The container image was dropped on 2026-08-09
  (D254) and no longer appears in any of them.
- **Not verifiable before the tag**, and therefore the checks in step 1 above: the
  GitHub release upload, the tap/AUR pushes, and `go install @<version>`.

## Update (2026-08-09) — deferred by the maintainer; stays open

Maintainer, verbatim: "a bit too early for that."

The tag is deferred, not declined, so this task stays **open** and `Blocks:
M5-11` remains accurate — the default-branch rename still waits on a release
existing. The `## Pre-flight result` (M5-10) above stands unchanged; re-ask at a
later review or when the maintainer raises it. No agent work is unblocked by
this answer: M5-11 was already the only item this task gates, and everything
else in M5 that remains publishes (D173 pt 1).
