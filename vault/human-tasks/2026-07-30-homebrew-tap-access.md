# Homebrew tap: confirm `homebrew-kubecom` still exists, delete the 2020 formula, add `HOMEBREW_TAP_TOKEN`

- Created: 2026-07-30
- By: M5-06
- Priority: normal
- Blocks: none (advisory — gates only the Homebrew third of the M5 exit criterion
  "Homebrew/AUR/Docker install paths verified". The cask config, the workflow wiring and
  their guards have landed and are inert without the secret, so M5-07/08 proceed and a tag
  pushed before this is done still releases successfully — it just ships no cask. M5-10's
  pre-flight should carry this item forward, not wait on it.)
- Status: open

## What's needed

Three things, and the middle one is the one that is easy to miss.

### 1. Confirm the tap repo

M5-06 chose the tap the **2020 build already published to**, so the install line from the
old README keeps working instead of stranding returning users:

```
repository: AnatolyRugalev/homebrew-kubecom   →   brew tap AnatolyRugalev/kubecom
```

The agent could not check whether that repo still exists (it is outside this session's
repository scope). Please confirm. If it was deleted, recreate it as an empty public repo
under the same name — goreleaser creates the `Casks/` directory itself. If you would rather
consolidate on a generic `AnatolyRugalev/homebrew-tap`, that is a fine decision to make, but
it is a **breaking change for anyone who tapped the old address**, so make it deliberately
and say so — `.goreleaser.yml` and the README both name the tap and
`TestReadmeBrewTapMatchesTheCask` fails `make check` if they disagree.

### 2. Delete `Formula/kubecom.rb` from the tap

This is the step that silently breaks the install if skipped. The 2020 tap published a
**formula**; M5-06 publishes a **cask** (see D182 pt 1 for why the formula route is closed).
Both can coexist in one tap, and when they do, `brew install AnatolyRugalev/kubecom/kubecom`
resolves to the **formula** — so every user would keep installing the 2020 kube-commander,
indefinitely, with no error anywhere.

Normally a cask would declare `conflicts_with formula:` to catch this. That does not work:
Homebrew removed formula conflicts from the cask DSL, and goreleaser accepts
`conflicts.formula` only to drop it on the floor (deprecated *and* never rendered — verified
by reading `internal/pipe/cask/template.go`). There is no config-side remedy. The old
formula has to go:

```bash
git clone https://github.com/AnatolyRugalev/homebrew-kubecom && cd homebrew-kubecom
git rm Formula/kubecom.rb && git commit -m "Retire the 2020 formula; kubecom v1 ships as a cask" && git push
```

If anything else lives in that tap (the 2020 repo may also hold a `kube-commander` formula),
decide what happens to it and note it in the Result.

### 3. Create the token and add it as a repository secret

`GITHUB_TOKEN` cannot be used: it is scoped to `kube-commander` only and cannot write to the
tap repo.

- Mint a fine-grained PAT scoped to **`AnatolyRugalev/homebrew-kubecom`** with
  **Contents: read and write** (that is the whole scope goreleaser needs).
- Add it to `AnatolyRugalev/kube-commander` → Settings → Secrets and variables → Actions,
  named exactly **`HOMEBREW_TAP_TOKEN`**. The name is asserted by
  `TestHomebrewCaskIsInertWithoutItsToken`, so a typo fails `make check` rather than
  releasing quietly.

Until this secret exists, `.goreleaser.yml`'s
`skip_upload: '{{ if index .Env "HOMEBREW_TAP_TOKEN" }}false{{ else }}true{{ end }}'`
evaluates to `true` and the cask is skipped — the release still succeeds. That is deliberate
(D173 pt 2): a publisher that hard-fails on a missing token turns a release nobody can retry
into a broken one.

## After the first tagged release: verify and document

The exit criterion says *verified*, which means installed from. On a Mac:

```bash
brew tap AnatolyRugalev/kubecom
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
brew tap AnatolyRugalev/kubecom
brew install --cask kubecom
```
```

and run `make check` — `TestReadmeBrewTapMatchesTheCask` checks the tap you write there
against the one `.goreleaser.yml` publishes to (remembering that `brew tap owner/x` means the
repository `owner/homebrew-x`).

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

- **Creating/inspecting the tap repo and minting a token are account-level acts** outside
  this repository, and a token is a credential an agent must not hold (D79).
- **Deleting the stale formula writes to a different repository** the agent has no access to.
- **"Verified" means installed from**, and that needs a macOS machine with Homebrew and a
  published tag — neither of which exists in the sandbox. What the agent *could* verify, it
  did: `goreleaser check` is clean, `goreleaser release --snapshot --clean` renders
  `dist/homebrew/Casks/kubecom.rb` with the postflight hook present, and the token/skip
  template was proved to flip both ways (no token → skip, token → publish).

## How to resolve

Do the three steps, then set `Status: done` with a `## Result` recording: whether the tap
survived, what was in it, whether the stale formula was there, and — after the first tag —
whether `brew install --cask kubecom` produced a launchable binary. If you decide to change
the tap address, say so explicitly; that is a decision the closing leg must record, not
infer.
