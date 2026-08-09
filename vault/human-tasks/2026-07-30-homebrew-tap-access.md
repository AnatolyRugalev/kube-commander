# Homebrew tap: add `HOMEBREW_TAP_TOKEN` for the new org tap `neuroplastio/homebrew-tap`

- Created: 2026-07-30
- By: M5-06
- Priority: normal
- Blocks: none (advisory — gates only the Homebrew third of the M5 exit criterion
  "Homebrew/AUR/Docker install paths verified". The cask config, the workflow wiring and
  their guards have landed and are inert without the secret, so a tag pushed before this
  is done still releases successfully — it just ships no cask. M5-10's pre-flight should
  carry this item forward, not wait on it.)
- Status: open

## What's needed

One thing now — and the repo half of the original task is done, because the tap moved.

### 1. The tap: done (D253, 2026-08-09)

The release-namespace fold-in settled where the tap lives: an **org-level tap,
`neuroplastio/homebrew-tap`** (`brew tap neuroplastio/tap`), kubecom its first
tool. The agent created that repository on 2026-08-09 (it was empty; goreleaser creates
the `Casks/` directory itself on the first publish). `.goreleaser.yml`, the README and
`docs/install.md` all point at it, and `TestReadmeBrewTapMatchesTheCask` keeps them
together.

The move deliberately abandons the 2020 `AnatolyRugalev/homebrew-kubecom` tap with **no
redirect** — that tap keeps serving the 2020 formula to anyone still on the old address,
which is the acknowledged cost of moving a tap (the human chose the org tap knowing this;
the 2020 formula does **not** need deleting, the old tap is just not where releases go
any more). The new tap is empty, so there is no stale formula to delete there.

### 2. Create the token and add it as a repository secret

`GITHUB_TOKEN` cannot be used: it is scoped to `kubecom` only and cannot write to the tap
repo.

- Mint a fine-grained PAT scoped to **`neuroplastio/homebrew-tap`** with
  **Contents: read and write** (that is the whole scope goreleaser needs).
- Add it to `neuroplastio/kubecom` → Settings → Secrets and variables → Actions,
  named exactly **`HOMEBREW_TAP_TOKEN`**. The name is asserted by
  `TestHomebrewCaskIsInertWithoutItsToken`, so a typo fails `make check` rather than
  releasing quietly.
- Note: the repo moved to the `neuroplastio` org on 2026-08-07, and GitHub **does not
  carry Actions secrets across a repo move** — if a `HOMEBREW_TAP_TOKEN` was ever set on
  the pre-move repo, re-create it here.

Until this secret exists, `.goreleaser.yml`'s
`skip_upload: '{{ if index .Env "HOMEBREW_TAP_TOKEN" }}false{{ else }}true{{ end }}'`
evaluates to `true` and the cask is skipped — the release still succeeds. That is deliberate
(D173 pt 2): a publisher that hard-fails on a missing token turns a release nobody can retry
into a broken one.

## After the first tagged release: verify and document

The exit criterion says *verified*, which means installed from. On a Mac:

```bash
brew tap neuroplastio/tap
brew install --cask kubecom
kubecom version          # must report the tag, a real commit and a build date, not "none"/"unknown"
kubecom                  # must actually launch — see the Gatekeeper note below
```

Then add the install path to [`docs/install.md`](../../docs/install.md), replacing
the "Homebrew" bullet under "Package managers" — since DOC-01 that is where the
install paths live, and the README carries only the short block that links to it:

```markdown
### Homebrew (macOS)

```bash
brew tap neuroplastio/tap
brew install --cask kubecom
```
```

and run `make check` — `TestReadmeBrewTapMatchesTheCask` checks the tap you write there
against the one `.goreleaser.yml` publishes to (remembering that `brew tap owner/x` means the
repository `owner/homebrew-x`, so this repo's `homebrew-tap` taps as `neuroplastio/tap`).

### Two things worth watching on that first install

- **Gatekeeper.** The binaries are unsigned and un-notarized, so macOS quarantines them on
  download and would kill `kubecom` on first run. The cask carries a `postflight` that runs
  `xattr -dr com.apple.quarantine` to strip it (D182 pt 4). If `kubecom` is still killed on
  launch, that hook is not doing its job and it is a real finding — file it as feedback.
- **Linux.** Homebrew on Linux does not install casks at all, even though goreleaser emits
  `on_linux` stanzas into the generated cask from the Linux archives. So the Homebrew path is
  macOS-only by construction; Linux users get the tarball, AUR (M5-07) or `go install`. The
  README already says this. If you would rather Linux Homebrew users be served too, that is a
  decision to record, and the only route is the deprecated `brews:` formula, which
  `goreleaser check` now fails on.

## Why the agent can't do it

- **Minting a token is a credential an agent must not hold (D79).** Creating the secret is
  an account-level act in repository settings, outside what the agent can reach.
- **"Verified" means installed from**, and that needs a macOS machine with Homebrew and a
  published tag — neither of which exists in the sandbox. What the agent *could* verify, it
  did: `goreleaser check` is clean, `goreleaser release --snapshot --clean` renders
  `dist/homebrew/Casks/kubecom.rb` with the postflight hook present, and the token/skip
  template was proved to flip both ways (no token → skip, token → publish).

## How to resolve

Do the token step, then set `Status: done` with a `## Result` recording: that the token was
created, and — after the first tag — whether `brew install --cask kubecom` produced a
launchable binary.

## Update (2026-08-09) — deferred by the maintainer; stays open

Maintainer, verbatim: "do not care, post-release."

The token step is deferred until around the first release, so this task stays **open**
but must not be re-surfaced at every orient (D256 pt 4). Re-ask when the tag
(`2026-07-30-first-release-tag`) is being cut — the cask publish skips itself without
the secret (D173 pt 2), which remains the correct failure mode until then.
