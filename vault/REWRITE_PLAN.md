# kubecom — Rewrite Plan

A complete rewrite of kube-commander (2020) on a modern Go stack, keeping the
original idea: a **fast, zero-deploy, SSH-friendly, real-time terminal UI for
observing and operating Kubernetes clusters** — the "kubernetes-dashboard in
your terminal", approachable and keyboard-driven.

## Locked decisions

| Area | Decision |
|------|----------|
| TUI framework | **Bubble Tea** + Bubbles + Lipgloss (Elm architecture, message loop) |
| K8s access | **In-process client-go** as far as it goes; shell out **only** where real interactivity is required (exec shell, `$EDITOR`) |
| Scope | **Full rewrite**, adopt modern client-go capabilities, close open GH issues |
| Config | **Plain YAML** typed struct (drop protobuf) |
| Name | **`kubecom`**, single binary (retire the `kube-commander` duplicate) |
| Min K8s | Build against a recent client-go (target **client-go v0.31 / K8s 1.31**), support servers **~1.27+** (discovery-driven, so degrades gracefully) |
| Backwards compat | **Clean break**; one-shot migration of the old `~/.kubecom.yaml` on first start |
| Platforms | **Linux + macOS only.** Drop native Windows support (removes the Windows PTY/exec complexity) — **recommend WSL2** for Windows users |

## Why rewrite (issues being addressed)

**Architecture**
- `commander/` mixes live interface contracts with a dead duplicate app impl → single clean module.
- `tcell/views` imperative widget tree with hand-rolled focus manager, manual redraws, and mutexes everywhere (`viewMu`/`rowsMu`/`tableMu`/`popupMu`/`configMu`). Recent history is a string of *"fix data races" / "popup mutex" / "popup ESC"* commits. Bubble Tea's single-threaded update loop removes this entire bug class.
- kubectl shell-out for describe/edit/logs/exec/port-forward suspends the whole TUI; port-forward blocks the UI. In-process client-go removes the hard `kubectl` dependency (**#68**) and enables background port-forward.

**Stack age**: Go 1.14, `ioutil`, `k8s.io/*@v0.18` (2020), abandoned protobuf theme engine, dead Travis/snap/AUR CI, build breakage (**#88**).

**Product**: only Pod has real actions; no in-TUI logs/describe/YAML; no context switcher; no sorting; broken RBAC listing (**#76**); panics on bad namespace (**#86**).

## Target architecture

```
kubecom/
  cmd/kubecom/main.go            # cobra root, flags/env, wires kube + tui
  internal/
    kube/                        # all client-go; no TUI imports
      client.go                  # clientset + dynamic + discovery + RESTMapper
      resources.go               # discovery, GVK<->GVR, namespaced?, verbs, CRDs
      watch.go                   # server-side Table List+Watch -> event channel
      logs.go                    # streaming pod logs
      exec.go                    # remotecommand (interactive => suspend+raw PTY)
      portforward.go             # background "port manager" (goroutine per fwd)
      describe.go                # k8s.io/kubectl/pkg/describe, in-process
      actions.go                 # delete/scale/cordon/drain/rollout-restart
    tui/
      app.go                     # root tea.Model: view routing, global keys
      msg.go                     # tea.Msg types (resource events, errors, ...)
      keys/bindings.go           # bubbles/key maps (+ help integration)
      styles/theme.go            # lipgloss styles; named-color themes
      components/                # table, menu sidebar, viewport, pickers,
                                 #   statusbar, help, confirm/prompt modal,
                                 #   and the full-screen views as sub-models:
                                 #   searchview, logsview, viewer (D264)
      # views/ (browse, logs, describe, yaml) is superseded by D264 — browse is
      #   the root Model (the shell) itself, and full-screen surfaces are
      #   components/* sub-models, not a separate views/ tree.
    config/                      # yaml struct + load/save + migrate.go
    version/
```

**Data flow (no shared mutable state):**
`kube.watch` runs a List+Watch and pushes events onto a Go channel → a
`tea.Cmd` reads one event and returns it as a `tea.Msg` → the table `Update`
applies add/modify/delete to its rows → `View` re-renders. Goroutines never
touch UI state; they only send messages.

**Resource listing strategy (generic + parity):**
Default path = **server-side Table printing** via the dynamic client with the
`Accept: application/json;as=Table` header, driven by discovery + RESTMapper.
This gives kubectl-identical columns for *any* resource including CRDs, and
fixes the RBAC/CRD/discovery failures generically (**#76**, **#87**). Sorting
(**#85**) and coloring are applied client-side on the resulting rows. Custom
per-resource renderers (e.g. Pods) layer on top only where we want extras.

**Async discovery (cold-start responsiveness):**
Discovery + RESTMapper build is the main cold-start cost, especially on large
clusters or ones with many CRD groups / flaky aggregated-API endpoints. The old
app blocked the whole UI on `ServerPreferredResources()`. Instead:
- **Render immediately** from a built-in seed set of core GVKs (the default menu:
  Namespaces, Nodes, Deployments, Pods, …) with a known-good static REST mapping,
  so the user can browse the common resources within milliseconds.
- Kick off **full discovery in a background `tea.Cmd`**; when it completes it emits
  a `DiscoveryReadyMsg` that reconciles the menu (adds CRDs/extra groups, marks
  unavailable ones). A spinner in the status bar shows discovery in progress.
- **Per-group fault isolation**: a failing API group (aggregated/metrics endpoint
  down, RBAC-denied) degrades only that group, never the whole load (**#87**, **#76**).
- **Cache discovery to disk** (`~/.kube/cache`-style, like kubectl's cached
  discovery) with invalidation, so subsequent starts are near-instant.
- Lazy-load a group's full resource detail only when first opened.

## Phased plan

### Phase 0 — Groundwork
- New Go module layout, Go 1.23+, latest cobra; **delete the legacy trees
  up-front** (`app/`, `cli/`, `commander/`, `config/`, `pb/`,
  `cmd/kube-commander/`) — `master` is the permanent reference (D14; one module
  cannot hold k8s.io v0.18 and client-go v0.31 simultaneously).
- Retire duplicate `kube-commander` binary; single `kubecom` (part of the deletion).
- **Linux + macOS build matrix only**; drop Windows/`cmd_windows.go`. README documents WSL2 as the Windows path.
- CI: GitHub Actions running `make check` (build/test/`go vet`/`golangci-lint`), goreleaser for Linux+macOS release artifacts (**#28**); drop Travis.
- Test harness: teatest for the TUI. Kube-layer tests use **fake clients by
  default**; `envtest` is opt-in and lands in Phase 1 (D18).

### Phase 1 — kube layer (in-process)
- clientset + dynamic + discovery + RESTMapper; robust discovery that tolerates partial API group failures (**#87**, **#76**).
- **Async, cached discovery**: seed core GVKs for instant start; background full discovery emitting `DiscoveryReadyMsg`; on-disk discovery cache with invalidation; per-group fault isolation.
- Server-side Table List+Watch → event channel; resource metadata (namespaced, verbs).
- Graceful errors instead of panics for bad namespace/context (**#86**, old #55).
- Unit/integration tests against envtest.

### Phase 2 — Core TUI (parity)
- Root model + two-pane browse view: resource menu sidebar + live table.
- Bubbles components: table, list (pickers), textinput (filter), viewport, help, spinner, statusbar.
- Namespace picker, filter, horizontal/vertical scroll, Home/End.
- Global keys, confirm/prompt modal (replaces the racy popup), themed via Lipgloss.
- Migrate old config on startup; menu customization (add/remove/reorder) persisted to YAML.

### Phase 3 — Actions & viewers (kill most shell-outs)
- In-TUI **logs** viewer (client-go stream → viewport; follow, previous, container picker) — also enable logs for pod-owning resources: Deployment/RS/StatefulSet/DaemonSet/Job (**#84**).
- In-TUI **describe** (kubectl/pkg/describe) and **YAML** view (viewport, no external pager).
- **Delete**, and generic workload actions: scale, rollout restart, cordon/drain, suspend cronjob (**#83**).
- **Port-forward manager**: background goroutines, a panel listing active forwards, start/stop without suspending.
- **Exec shell**: suspend Bubble Tea (`tea.ExecProcess`) → raw PTY via client-go `remotecommand` (fallback to `kubectl exec`). Linux/macOS PTY only — no Windows path to maintain.
- **Edit**: suspend to `$EDITOR`, apply on save.
- **View secret contents** with reveal/decode + copy (**#89**).

### Phase 4 — New capabilities
- **Context/cluster switcher** in-UI + context shown in top bar (**#80**, old #79).
- **Sort by column** (**#85**); column-aware coloring (pod phase, restarts, readiness).
- Owner→children drill-down (Deployment→Pods, Node→Pods).
- Optional metrics (CPU/mem) via metrics.k8s.io when available.
- Theme selection; ship a couple of good built-ins (port monokai/solarized).

### Phase 5 — Release & docs
- Update README (screencast via vhs), keybindings doc, install paths (brew/AUR/binary/docker), migration note.
- goreleaser + package distributors (**#28**); AUR/brew tap refresh.

## Open GH issues → resolution

| Issue | Resolved by |
|------|-------------|
| #68 snap can't find kubectl | In-process client-go (Phase 1/3) |
| #76 RBAC resources broken | Discovery + dynamic Table path (Phase 1) |
| #87 recover from api-resources errors | Fault-tolerant discovery (Phase 1) |
| #86 panic on nonexistent namespace | Error handling, no panics (Phase 1/2) |
| #88 build fails (spdystream) | New deps/toolchain (Phase 0) |
| #84 logs for pod-related resources | Logs viewer w/ owner selection (Phase 3) |
| #83 deployment actions | Generic workload actions (Phase 3) |
| #89 view secret contents | Secret viewer (Phase 3) |
| #80 context switching | Context switcher (Phase 4) |
| #85 sort by column | Client-side sort (Phase 4) |
| #28 CD to distributors | goreleaser + GH Actions (Phase 0/5) |

## Risks / watch-items
- **Bubbles `table`** is basic (no built-in virtualized huge lists / column sort). Likely need a custom table component — budget for it.
- **Interactive exec** — Linux/macOS PTY via `remotecommand`, `kubectl exec` fallback. Windows is out of scope (WSL2).
- **Server-side Table watch** semantics differ from informer caches; validate resync/reconnect on watch expiry.
- **Async discovery reconciliation**: menu must update in place when `DiscoveryReadyMsg` arrives without disrupting the user's current selection/scroll; handle the seed-set vs. discovered-set diff cleanly, and the stale-disk-cache case.
```
