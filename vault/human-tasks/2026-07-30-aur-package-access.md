# AUR: the package is renamed to `kubecom-bin` — create it, retire `kube-commander`, add `AUR_SSH_PRIVATE_KEY`

- Created: 2026-07-30
- By: M5-07
- Priority: normal
- Blocks: none (advisory — gates only the AUR third of the M5 exit criterion
  "Homebrew/AUR/Docker install paths verified". The `aurs:` config, the workflow wiring and
  their four guards have landed and are inert without the key, so M5-08 proceeds and a tag
  pushed before this is done still releases successfully — it just publishes no AUR package.
  M5-10's pre-flight should carry this item forward, not wait on it.)
- Status: open

## Read this part first: the package name changed, and it was not a choice

The 2020 build published to `aur@aur.archlinux.org:kube-commander`
(`master:ci/aur/publish.sh`). **This config cannot publish there**, and no amount of
configuration will make it. Two facts collide:

- goreleaser's AUR pipe appends `-bin` to any `name` that does not already end in it —
  unconditionally, in `Default()` (`internal/pipe/aur/aur.go`). There is no opt-out.
- The AUR requires a package's `pkgbase` to equal the name of the git repository it is
  pushed to. So a `pkgname` of `kubecom-bin` can only live in the repo `kubecom-bin`.

Together those mean the published package is **`kubecom-bin`**, a *new* AUR package. This
is correct on the merits too — `-bin` is the Arch convention for a package that installs a
prebuilt binary, and the 2020 `kube-commander` package violated it — but the practical
consequence is what matters: **there is no upgrade path.** `pacman`/`yay` will not offer
`kubecom-bin` to anyone who has `kube-commander` installed. The AUR has no `replaces:`
mechanism a maintainer can use to redirect one to the other, and goreleaser exposes no such
field either.

What the config *can* do, it does: `conflicts=('kubecom' 'kube-commander')`. `pacman` honours
that, so a user who tries to install both gets a clear conflict instead of one package's
`/usr/bin/kubecom` silently overwriting the other's. (This is the one place the Homebrew
problem from M5-06 has a real remedy — a Homebrew cask cannot declare a conflict with a
formula at all. See D182 pt 2 vs D183 pt 3.)

## What's needed

### 1. Create the `kubecom-bin` AUR package

An AUR package repo is created by the first push, so there is nothing to click. But the name
must be **unclaimed** — check https://aur.archlinux.org/packages/kubecom-bin first. If
somebody else has already published a `kubecom-bin`, stop and file feedback: that changes the
name, and the name is asserted in three places (`.goreleaser.yml`'s `name` and `git_url`, and
the README), which `TestAURGitURLMatchesThePackageName` and
`TestReadmeAURPackageMatchesTheConfig` keep in agreement.

The account pushing must be a registered AUR account with the SSH public key uploaded at
https://aur.archlinux.org/account — AUR pushes authenticate by key, not password.

### 2. Retire the 2020 `kube-commander` package

Skipping this leaves a package on the AUR that installs the **2020 binary** for as long as
anyone finds it — the same failure shape as the stale Homebrew formula, except here it is
publicly searchable and nothing in this repo can even see it. Please do one of:

- **Orphan and mark it deleted.** On https://aur.archlinux.org/packages/kube-commander use
  *Submit Request* → *Merge* into `kubecom-bin` if the AUR lets you (a merge request leaves a
  pointer, which is the friendliest outcome for existing users), or *Deletion* otherwise.
- **Or push one last update to it** whose `pkgdesc` says the package moved to `kubecom-bin`.
  Less clean, but it reaches people who already have it installed.

Either way, note in the Result which route you took — the closing leg should record it.

While you are there: the 2020 package also installed two shell shims, `/usr/bin/kube-commander`
and `/usr/bin/kubectl-ui`, that just re-exec the binary (`master:ci/aur/kube-commander`,
`master:ci/aur/kubectl-ui`). **`kubecom-bin` installs neither** (D183 pt 4) — the binary is
named `kubecom` and there is no kubectl-plugin story in v1. If you actually use
`kubectl ui`, say so and it can come back as a separate slice.

### 3. Create the SSH key and add it as a repository secret

- Generate a key dedicated to this — do not reuse a personal one:

  ```bash
  ssh-keygen -t ed25519 -C "kubecom-aur-release" -f ~/.ssh/kubecom_aur -N ""
  ```

  The empty passphrase (`-N ""`) is **required**, not laziness: goreleaser hard-errors on a
  password-protected key (`git: key is password-protected`, `internal/client/git.go`) rather
  than skipping, which would turn a release nobody can retry into a failed one.

- Upload `~/.ssh/kubecom_aur.pub` to your AUR account's SSH keys.
- Add the **private** key (`~/.ssh/kubecom_aur`, whole file including the BEGIN/END lines) to
  `AnatolyRugalev/kube-commander` → Settings → Secrets and variables → Actions, named exactly
  **`AUR_SSH_PRIVATE_KEY`**. The name is asserted by `TestAURIsInertWithoutItsKey`, so a typo
  fails `make check` rather than releasing quietly.

Until the secret exists, `.goreleaser.yml`'s
`skip_upload: '{{ if index .Env "AUR_SSH_PRIVATE_KEY" }}false{{ else }}true{{ end }}'`
evaluates to `true` and the AUR step is skipped — the release still succeeds (D173 pt 2).

## After the first tagged release: verify and document

The exit criterion says *verified*, which means installed from. On an Arch box:

```bash
yay -S kubecom-bin        # or: git clone https://aur.archlinux.org/kubecom-bin.git && cd kubecom-bin && makepkg -si
kubecom version           # must report the tag, a real commit and a build date, not "none"/"unknown"
kubecom                   # must actually launch
```

Then replace the "Arch Linux users, note the rename" paragraph under
[Install](../../README.md#install) with the real path:

```markdown
### Arch Linux (AUR)

```bash
yay -S kubecom-bin
```

Replaces the 2020 `kube-commander` package, which is no longer maintained.
```

and run `make check` — `TestReadmeAURPackageMatchesTheConfig` checks the package name you
write there against the one `.goreleaser.yml` publishes.

### Two things worth watching on that first publish

- **`pkgver`.** goreleaser strips the leading `v` and replaces `-` with `_`, so tag `v1.0.0`
  becomes `pkgver=1.0.0` and `v1.0.0-rc.1` becomes `1.0.0_rc.1` (verified by probe). The 2020
  package's `pkgver` kept the `v`. Since these are different package names nothing compares
  them, but if a version ever looks wrong on the AUR, that transform is why.
- **The download URLs.** goreleaser derives them from the git remote. In the sandbox that
  produces a bogus `https://github.com/git/AnatolyRugalev/…` because the remote is a local
  proxy; on GitHub Actions the remote is real. If the first published PKGBUILD has a `/git/`
  in its source URLs, something is wrong with the checkout in CI — file it as feedback.

## Why the agent can't do it

- **An AUR account, its SSH key and the package namespace are account-level**, outside this
  repository, and a private key is a credential an agent must not hold (D79).
- **Retiring `kube-commander` writes to a repository/namespace the agent has no access to**,
  and a deletion request is irreversible.
- **"Verified" means installed from**, which needs an Arch machine and a published tag.

What the agent *could* verify, it did: `goreleaser check` is clean; `goreleaser release
--snapshot --clean` renders `dist/aur/kubecom-bin.pkgbuild` and `.srcinfo` with both
architectures, `conflicts=('kubecom' 'kube-commander')`, no `depends`, and
`install -Dm755 "./kubecom" "${pkgdir}/usr/bin/kubecom"`; the `skip_upload` template was
proved to flip both ways by probe (no key → `true`, key → `false`); and the `ids:` restriction
was proved necessary by removing it and counting four source lines instead of two.

## How to resolve

Do the three steps, then set `Status: done` with a `## Result` recording: whether
`kubecom-bin` was free, which route you took to retire `kube-commander`, whether anyone was
using `kubectl-ui`, and — after the first tag — whether `yay -S kubecom-bin` produced a
launchable binary.
