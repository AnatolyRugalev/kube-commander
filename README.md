# kubecom

A fast, vim-friendly, zero-deploy Kubernetes TUI — *"the kubernetes-dashboard in
your terminal."* Browse and operate any cluster over SSH, in real time, with no
in-cluster deployment and **no `kubectl` binary required**.

> ### 🚧 `v1` is a ground-up rewrite in progress
>
> This branch (`v1`) rebuilds the 2020 codebase from scratch on a modern Go stack
> (**Bubble Tea + client-go**), fixing the old data-race/focus/redraw bug class by
> construction and dropping the hard `kubectl` dependency. The original 2020 code
> lives on [`master`](https://github.com/AnatolyRugalev/kube-commander/tree/master).
>
> **Current status:** the in-process Kubernetes layer is complete and the TUI
> components are built and tested; the interactive UI is being wired to launch
> against a live cluster (task `M2-RUN`). Today the binary builds and exposes the
> `version` and `keys` subcommands — the browse UI lands with `M2-RUN`. This README
> tracks what the built binary actually does and is updated as the UI comes online.
>
> The rewrite is driven autonomously and documents itself in **[`vault/`](vault/)**
> (goals, plan, live task board, decision log, per-leg journal); see
> [`CLAUDE.md`](CLAUDE.md) for the operating model.

## Why kubecom

- **Zero deploy.** A single static binary you run locally or over SSH — nothing to
  install in the cluster, no HTTP ingress to expose.
- **Real-time.** Lists are server-side watched and update live; no refresh key.
- **No `kubectl` binary.** Discovery, list/watch, logs, describe, YAML, and the
  action set are all in-process via client-go.
- **Vim-first, fully rebindable.** `hjkl`, `gg`/`G`, `/`, `n`/`N` by default, with
  arrows and classic keys as an equivalent fallback — and every key is
  configurable (no hard-coded keys anywhere). See
  [`docs/keybindings.md`](docs/keybindings.md).
- **Approachable.** Simpler and more discoverable than k9s by design.

## Requirements

- **Linux or macOS** (Windows via WSL2 — native Windows is a non-goal).
- A working **kubeconfig** (kubecom uses your current context by default).
- **Go 1.24+** to build from source (until tagged release binaries ship in M5).

## Install

### From a local checkout (recommended today)

Until a tagged release lands (M5), build from a `v1` checkout — this needs no
version tag and respects the branch:

```bash
git clone -b v1 https://github.com/AnatolyRugalev/kube-commander
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
> go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@<commit-sha>
> ```
>
> A clean `go install …@latest` returns with the first tagged release.

> Release binaries (goreleaser), Homebrew, AUR, and Docker images are planned for
> the M5 release milestone and will be documented here when they land.

## Usage

Run bare `kubecom` to launch the interactive browse UI against your current
kubeconfig context:

```bash
kubecom                        # browse the current context, all namespaces
kubecom --context my-cluster --namespace my-ns
kubecom --kubeconfig ~/.kube/other-config -n kube-system
```

Flags: `--kubeconfig` (path; default `$KUBECONFIG`, else `~/.kube/config`),
`--context` (default the file's current-context), `-n`/`--namespace` (default all
namespaces), `--config` (kubecom config file). A missing or invalid
kubeconfig/context fails with a clear message instead of launching. While the UI
runs it owns the terminal, so all logs (including client-go warnings) go to a file
under your cache dir (`~/.cache/kubecom/kubecom.log` on Linux), never the screen.

Navigation is keyboard-first (vim keys by default; see
[`docs/keybindings.md`](docs/keybindings.md)). The mouse is additive: click a menu
item to open it, click a table row to select it, and scroll the wheel to move
through whichever pane the pointer is over.

The other subcommands report information and exit:

```bash
# Print build information
kubecom version

# Print the resolved keymap (defaults merged with your config)
kubecom keys
```

### Configuration

kubecom reads an optional YAML config from
`os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on
Linux). It is not required — kubecom runs on sensible defaults. The config's
`keys:` section rebinds any action; see [`docs/keybindings.md`](docs/keybindings.md)
for the action list and defaults, and inspect the effective map any time with
`kubecom keys`.

```yaml
# ~/.config/kubecom/config.yaml
keys:
  nav.down: ["j", "down"]
  nav.up:   ["k", "up"]
```

#### Per-context menu

kubecom can add extra resource types (chiefly CRDs the built-in menu doesn't know)
to the browse menu, per kubeconfig context. Each context reads its own file from
`os.UserConfigDir()/kubecom/menus/<context>.yaml` (`~/.config/kubecom/menus/` on
Linux; the context name is sanitized into a safe filename). The file is optional —
a context with no file uses the default menu; a malformed file falls back to the
default menu and shows a brief startup notice rather than failing to launch.

```yaml
# ~/.config/kubecom/menus/my-cluster.yaml
resources:
  - group: cert-manager.io   # omit for the core ("") group
    version: v1
    resource: certificates   # plural, as the API addresses it
    kind: Certificate        # optional; defaults from resource
    namespaced: true         # optional; default false (cluster-scoped)
    section: Custom Resources # optional; default the Custom Resources group
```

An entry whose resource is already in the menu (a seed row or one discovery finds)
is merged, never listed twice.

## Contributing

The rewrite is currently driven autonomously against the plan in
[`vault/`](vault/). If you'd like to contribute, please open an issue describing
your intent first so we can align with the milestone plan.

## Special thanks

- [Bubble Tea / Bubbles / Lipgloss](https://github.com/charmbracelet) — the TUI stack
- [client-go](https://github.com/kubernetes/client-go) — in-process Kubernetes access
- [k9s](https://github.com/derailed/k9s) — prior art in the Kubernetes-TUI space
