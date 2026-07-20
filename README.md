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

### From source (recommended today)

```bash
go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@v1
```

This installs the `kubecom` binary to `$(go env GOPATH)/bin` — make sure that's on
your `PATH`.

Or clone and build:

```bash
git clone -b v1 https://github.com/AnatolyRugalev/kube-commander
cd kube-commander
go build -o kubecom ./cmd/kubecom
```

> Release binaries (goreleaser), Homebrew, AUR, and Docker images are planned for
> the M5 release milestone and will be documented here when they land.

## Usage

```bash
# Print build information
kubecom version

# Print the resolved keymap (defaults merged with your config)
kubecom keys
```

Launching the interactive browser — bare `kubecom`, using your current
kubeconfig/context — lands with task **`M2-RUN`**. The intended invocation:

```bash
kubecom                        # browse the current context
kubecom --context my-cluster --namespace my-ns
kubecom --kubeconfig ~/.kube/other-config
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

## Contributing

The rewrite is currently driven autonomously against the plan in
[`vault/`](vault/). If you'd like to contribute, please open an issue describing
your intent first so we can align with the milestone plan.

## Special thanks

- [Bubble Tea / Bubbles / Lipgloss](https://github.com/charmbracelet) — the TUI stack
- [client-go](https://github.com/kubernetes/client-go) — in-process Kubernetes access
- [k9s](https://github.com/derailed/k9s) — prior art in the Kubernetes-TUI space
