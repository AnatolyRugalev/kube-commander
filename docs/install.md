# Installing kubecom

kubecom is a single static binary you run locally or over SSH. There is nothing
to deploy in the cluster and no `kubectl` binary to install alongside it.

The [README](../README.md#install) carries the two paths most people want. This
page is the full set: what each path gives you, and what is still waiting on the
**stable** release.

**Where the release stands.** The first release candidate, `v1.0.0-rc.1`, was
published on 2026-08-10 with archives and bare binaries for `linux`/`darwin` ×
`amd64`/`arm64`. Stable `v1.0.0` has not been tagged. That distinction decides
every path below: a pre-release is something you must **ask for by name**, so
`@latest` skips it, Homebrew and the AUR do not carry it, and only the release
archives and an explicitly-versioned `go install` reach it today.

## Requirements

- **Linux or macOS** (Windows via WSL2 — native Windows is a non-goal).
- A working **kubeconfig** (kubecom uses your current context by default).
- **Go 1.24+** to install or build from source (not needed for a release archive).

## From a release archive (recommended today)

Every release carries `.tar.gz` archives for `linux`/`darwin` × `amd64`/`arm64`,
the same four binaries bare, and a `checksums.txt`. Download the one for your
platform from the [Releases page](https://github.com/neuroplastio/kubecom/releases)
— today that is the **pre-release** `v1.0.0-rc.1`, which GitHub marks as such and
does not offer as "Latest" — verify it, and put the binary on your `PATH`:

```bash
sha256sum --check --ignore-missing checksums.txt
tar xzf kubecom_<version>_<os>_<arch>.tar.gz
install -m 0755 kubecom ~/.local/bin/kubecom
```

The archives are the same artifacts the Homebrew cask and the AUR package install,
so this is the no-package-manager path, not a lesser one. Note that macOS binaries
are unsigned: downloaded by hand rather than through Homebrew, they need
`xattr -dr com.apple.quarantine kubecom` before Gatekeeper will run them.

## With `go install`

```bash
go install github.com/neuroplastio/kubecom/cmd/kubecom@v1.0.0-rc.1
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

**Name the version.** `@latest` resolves to the newest *stable* release and there
is none yet, so it will not find kubecom until `v1.0.0` is tagged; until then the
rc has to be asked for by name. One difference from the archive: a binary the Go
toolchain builds reports `kubecom dev (commit none, built unknown)` from
`kubecom version`, because the real values are stamped by the release build's
ldflags and nothing else sets them. The binary is otherwise identical.

> **Why not `go install …@v1`?** In `go install path@v1`, the `@v1` is a *module
> version query*, and `v1` matches Go's semver-prefix form (major version 1), so
> the toolchain looks for the newest stable `v1.x.x` release **tag** and never
> falls back to the branch named `v1`. `v1.0.0-rc.1` is a pre-release, which such
> a query excludes, so `@v1` does not resolve today either — name the full version
> or use a commit SHA, which is *not* semver-parsed:
>
> ```bash
> go install github.com/neuroplastio/kubecom/cmd/kubecom@<commit-sha>
> ```
>
> Both `@v1` and `@latest` start working with the stable tag.

## From a local checkout

To build the branch itself — the tip of `v1`, ahead of the rc:

```bash
git clone -b v1 https://github.com/neuroplastio/kubecom
cd kubecom
go install ./cmd/kubecom      # installs kubecom to $(go env GOPATH)/bin
```

Prefer a plain binary in the current directory? Use
`go build -o kubecom ./cmd/kubecom` instead. As with `go install`, `kubecom
version` reports `dev` for a build made this way.

## Package managers

Homebrew and the AUR package are wired but **carry nothing yet**: the
`v1.0.0-rc.1` run skipped both publishers, which are deliberately inert until
their credentials are configured, so neither address serves kubecom today. Expect
them with the stable `v1.0.0` tag. Two notes if you are coming from the 2020
kube-commander, since both of its addresses survive with different contents:

- **Homebrew.** The tap moved to the org (`brew tap neuroplastio/tap` —
  the repository is `neuroplastio/homebrew-tap`, D253), where kubecom is the
  first tool, and ships as a **cask**, which
  Homebrew supports on macOS only — on Linux, use the release tarball, the AUR
  package or `go install` above. The org tap is still empty, and the 2020
  `AnatolyRugalev/kubecom` tap (a *different* repository, abandoned by the move)
  still serves the 2020 formula — so do not install from either expecting
  kubecom.
- **Arch Linux.** The AUR package will be **`kubecom-bin`** — *not* the 2020
  `kube-commander`, whose name this project can no longer publish to (`goreleaser`
  requires the `-bin` suffix on a prebuilt-binary package, and the AUR requires the
  package name to match the repository). `kubecom-bin` does not exist yet, and
  `kube-commander` still installs the 2020 build, so do
  not treat one as an upgrade of the other. The two conflict deliberately: both own
  `/usr/bin/kubecom`, so `pacman` will refuse to install `kubecom-bin` until
  `kube-commander` is removed.

The container image was **dropped** on 2026-08-09 (D254) — no image is built or
published, so there is no container install path to document.
