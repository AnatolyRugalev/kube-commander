# kubecom — Plan

kubecom is a fast, zero-deploy, SSH-friendly, real-time terminal UI for observing
and operating Kubernetes clusters — the "kubernetes-dashboard in your terminal",
approachable and keyboard-driven. This file is the current plan: the locked
decisions, the architecture as built, the release/update model, and the work that
remains.

The history of how the 2020 **kube-commander** became **kubecom** — the phased
rewrite (M0–M5), the reasoning and every superseded choice — lives in
[`knowledge/decisions.md`](knowledge/decisions.md), the [`journal/`](journal/) and
git. It is not restated here.

## Locked decisions

| Area | Decision |
|------|----------|
| TUI framework | **Bubble Tea v2** + Bubbles + Lipgloss (Elm architecture, single update loop) |
| K8s access | **In-process client-go**; shell out **only** where real interactivity requires it (exec shell, `$EDITOR`) |
| Resource listing | **Server-side Table** printing via the dynamic client (`Accept: as=Table`) — kubectl-identical columns for any resource incl. CRDs |
| Config | **KDL** (D302): an authored `config.kdl` kubecom never writes, a `config.overlay.kdl` it does, `state/<context>.kdl`; one-shot migration from the old `~/.kubecom.yaml` |
| Keys | **A context-bound keymap** (D303): `bind`/`unbind`/`context "<cel>"` over the focus path, resolved by **myelin**, the shared key-binding library (D304) |
| Platforms | **Linux + macOS**; Windows via WSL2 |
| Release | **engram channels** (`pkg.neuroplast.io/kubecom/{dev,stable}`) + a thin **launcher**; goreleaser retired (D292) |
| Version identity | CalVer — `YY.MM.DD` stable, `YY.MM.DD-dev.<sha7>` dev; `Commit` is the identity |

## Architecture

```
kubecom/
  cmd/kubecom/                 # complete binary: cobra root, keys, update, logging, run
  cmd/kubecom-launcher/        # enlaunch shim a package installs as `kubecom` (D293)
  internal/
    kube/                      # all client-go access; no TUI imports
    tui/                       # Bubble Tea shell: app, browse, views, actions
      components/              # table, menu, picker, viewer, modal, logs, statusbar, …
      keymap/                  # action registry + bindings; no raw keys in view code
      styles/                  # lipgloss palettes: default, monokai, solarized-dark
      help/ elide/ safetext/
    config/                    # KDL config + overlay + state + legacy migration
    channel/                   # engram channel client; pinned server + release key
    update/                    # `kubecom update`: fetch, verify, install
    keylog/                    # --keylog recorder + analyzer
    version/                   # ldflags-stamped Version/Commit/Date/Channel (+ guards)
    vault/ stories/            # drift guards for vault/ and stories/ fixtures
  packaging/{aur,homebrew}/    # PKGBUILD/formula render + publish scripts
```

**Data flow (no shared mutable UI state):** `kube.watch` runs a List+Watch and
pushes events onto a channel → a `tea.Cmd` reads one and returns it as a
`tea.Msg` → the table `Update` applies add/modify/delete → `View` re-renders.
Goroutines never touch UI state; they only send messages. This is the whole reason
for the framework choice — protect it.

## Release & update model

- **`make dist`** publishes bare `kubecom_<os>_<arch>` and
  `kubecom-launcher_<os>_<arch>` to the **dev** channel on every push to `main`,
  and to **stable** on a day tag (`YY.MM.DD`). It assumes the OIDC role
  `github-pkg-publish-kubecom`; there are no secrets (D292).
- **A package installs the launcher plus a seed.** The launcher is `/usr/bin/kubecom`,
  the seed is a complete build so a fresh install runs offline; `kubecom update`
  installs newer builds into `~/.local/kubecom`, verifying the signed manifest
  against the pinned key (D293/D295). Without enlaunch it replaces its own binary.
- **Distributors:** AUR `kubecom-bin`, Homebrew `neuroplastio/tap`, and the no-root
  `install.sh` all ship the launcher + seed (D295/D296/D299).

## Forward roadmap

The rewrite is done and v1 is shipped on the channels. What remains is packaging,
docs and UX polish; the live list is [`tasks/board.md`](tasks/board.md).

- **Docs.** DOC-03 (README → landing page), DOC-04 (`docs/usage.md`), DOC-05
  (`docs/troubleshooting.md`) — the guided tour the README stops carrying (D268) —
  and ENGRAM-03 to reword the install/update docs and the Definition of Done to the
  channel/launcher model.
- **Packaging.** ENGRAM-02: publish the Homebrew formula from CI
  (`HOMEBREW_TAP_TOKEN`); retire the 2020 `kube-commander` from the AUR.
- **UX polish.** STORY-06i-3 (reverse-selector relations), TAPE-01 (re-cut the
  screencast), BOX-04 (browse body's two-column gap).
- **Config & keys.** KDL config and a contextual keymap (D302/D303): KEYS-01,
  KDL-01, KEYS-02, then KEYS-03/04.
- **Feedback.** The open AGE-sort bug in [`feedback/`](feedback/).
- **Housekeeping.** Close the resolved GitHub issues (#8, #28, #68, #76, #80, #83,
  #84, #85, #86, #87, #89, #90) now that a release carries the fixes.
