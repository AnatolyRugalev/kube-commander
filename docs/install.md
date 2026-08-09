# Installing kubecom

kubecom is a single static binary you run locally or over SSH. There is nothing
to deploy in the cluster and no `kubectl` binary to install alongside it.

The [README](../README.md#install) carries the one path that works today —
building from a `v1` checkout. This page is the full set: what each path gives
you, and what is waiting on the first tagged release.

## Requirements

- **Linux or macOS** (Windows via WSL2 — native Windows is a non-goal).
- A working **kubeconfig** (kubecom uses your current context by default).
- **Go 1.24+** to build from source (until tagged release binaries ship in M5).

## From a local checkout (recommended today)

Until a tagged release lands (M5), build from a `v1` checkout — this needs no
version tag and respects the branch:

```bash
git clone -b v1 https://github.com/neuroplastio/kubecom
cd kube-commander
go install ./cmd/kubecom      # installs kubecom to $(go env GOPATH)/bin
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Prefer a plain binary in the
current directory? Use `go build -o kubecom ./cmd/kubecom` instead.

> **Why not `go install …@v1`?** In `go install path@v1`, the `@v1` is a *module
> version query*, and `v1` matches Go's semver-prefix form (major version 1), so
> the toolchain looks for a `v1.x.x` release **tag** — which this repo does not yet
> have — and never falls back to the branch named `v1`. A commit SHA is *not*
> semver-parsed, so a pinned remote install does work if you want one:
>
> ```bash
> go install github.com/neuroplastio/kubecom/cmd/kubecom@<commit-sha>
> ```
>
> A clean `go install …@latest` returns with the first tagged release.

## From a release archive

From the first tagged release onward, every release carries `.tar.gz` archives for
`linux`/`darwin` × `amd64`/`arm64` plus a `checksums.txt`. Download the one for your
platform from the [Releases page](https://github.com/neuroplastio/kubecom/releases),
verify it, and put the binary on your `PATH`:

```bash
sha256sum --check --ignore-missing checksums.txt
tar xzf kubecom_<version>_<os>_<arch>.tar.gz
install -m 0755 kubecom ~/.local/bin/kubecom
```

The archives are the same artifacts the Homebrew cask and the AUR package install,
so this is the no-package-manager path, not a lesser one. Note that macOS binaries
are unsigned: downloaded by hand rather than through Homebrew, they need
`xattr -dr com.apple.quarantine kubecom` before Gatekeeper will run them.

## Package managers

Homebrew and the AUR package are wired and waiting on the first tagged release
(M5). Two notes if you are coming from the 2020 kube-commander, since both of
its addresses survive with different contents:

- **Homebrew.** The tap moved to the org (`brew tap neuroplastio/tap` —
  the repository is `neuroplastio/homebrew-tap`, D253), where kubecom is the
  first tool, and ships as a **cask**, which
  Homebrew supports on macOS only — on Linux, use the release tarball, the AUR
  package or `go install` above. Until the first tagged release the org tap is
  empty, and the 2020 `AnatolyRugalev/kubecom` tap (a *different* repository,
  abandoned by the move) still serves the 2020 formula — so do not install from
  either expecting kubecom.
- **Arch Linux.** The AUR package will be **`kubecom-bin`** — *not* the 2020
  `kube-commander`, whose name this project can no longer publish to (`goreleaser`
  requires the `-bin` suffix on a prebuilt-binary package, and the AUR requires the
  package name to match the repository). `kubecom-bin` does not exist until the
  first tagged release, and `kube-commander` still installs the 2020 build, so do
  not treat one as an upgrade of the other. The two conflict deliberately: both own
  `/usr/bin/kubecom`, so `pacman` will refuse to install `kubecom-bin` until
  `kube-commander` is removed.

The container image was **dropped** on 2026-08-09 (D254) — no image is built or
published, so there is no container install path to document.
