# Legacy Architecture (original kube-commander, 2020)

How the original works and where it hurts — so the rewrite doesn't repeat it and
can mine it for behavior parity. Code lives on `master`.

## What it is
A two-pane K8s TUI ("kubecom"): left **resource menu**, right **live-watched
table** of the selected resource. Row actions: describe/edit/delete. Pods add
logs / exec shell / port-forward. Menu is customizable and persisted; a
half-built theme engine exists.

## Code map
- `cmd/kube-commander` and `cmd/kubecom` — **two identical** entrypoints, both call `cli.Run()`.
- `cli/cli.go` — cobra command, flags/env, wires config + client + builder + executor + app.
- `app/` (~4.3k LOC) — the **live** implementation.
  - `app/app.go` — root; owns `views.Application` + tcell screen; `Interrupt()` quits the TUI, runs a command, re-inits the screen.
  - `app/client/` — client-go access via raw `rest.Request` with manual Table `Accept` headers; watch via cli-runtime `resource.Builder`.
  - `app/ui/` — tcell/views widgets: `workspace`, `resourceMenu`, `widgets/listTable` (844 LOC, the core), `status`, `border`, `popup`, `theme`, pod pickers.
  - `app/builder/` — builds **kubectl** command lines (describe/edit/logs/exec/port-forward).
  - `app/executor/` — runs those commands (suspends TUI), `cmd_nix.go` / `cmd_windows.go`.
  - `app/focus/` — hand-rolled focus manager.
- `commander/` (~470 LOC) — **mixed**: live shared interface contracts (`App`, `Container`, `Resource`, data `Operation`s) **and** a dead duplicate app/client/builder impl. Confusing.
- `config/` + `pb/` — protobuf config schema (`config.proto` → 632-line generated `config.pb.go`), protojson↔YAML, fsnotify watch. Theme-in-protobuf is abandoned mid-build.

## How data flows today
Discovery via `ServerPreferredResources()` (blocks the UI). Listing/watching uses
server-side Table printing (kubectl columns). `listTable` maintains rows behind
`sync.RWMutex`es and applies watch ops. Redraws are manual (`Update()`,
`UpdateScreen()`).

## Pain points (the reasons for the rewrite)
1. **tcell/views + manual concurrency.** Mutexes everywhere (`viewMu`/`rowsMu`/
   `tableMu`/`popupMu`/`configMu`). Recent git history is literally "fix data
   races", "add popup mutex", "popup ESC". → Bubble Tea removes the class.
2. **kubectl shell-out** for describe/edit/logs/exec/port-forward; TUI suspends;
   port-forward blocks the UI. Requires a matching kubectl in PATH (**#68**).
3. **Blocking discovery**; one bad API group can break the whole load
   (**#87**, **#76**); panics on bad namespace (**#86**).
4. **Old stack**: Go 1.14, `ioutil`, `k8s.io/*@v0.18` (K8s 1.18), build breakage (**#88**).
5. **Over-engineered config** (protobuf) with an abandoned theme engine.
6. **Zero tests.** Nothing in the repo.
7. **Dead CI**: Travis, snapcraft, AUR scripts. Duplicate binary + name inconsistency.

## Behavior worth preserving (parity reference)
- Default resource menu set and ordering — see `app/ui/resourceMenu/resource_menu.go` `DefaultItems`.
- Menu customization: add (`+`), hide (`Del`), reorder (`F6`/`F7`), persisted.
- Namespace picker (`Ctrl+N`/`F2`), `?` help, filter.
- Pod actions: logs (`l`), previous logs (`L`), port-forward (`f`), shell (`s`),
  with container/port pickers.
- Flags/env: config, kubectl, editor, pager, log-pager, tail, kubeconfig,
  context, namespace, timeout (see `cli/cli.go`).
