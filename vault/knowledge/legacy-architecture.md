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

## The legacy config file on disk (`~/.kubecom.yaml`) — verified, M5-05

The migration's input, as the 2020 build actually wrote it. `config/config.go`:
`Save` = `protojson.Marshal(*pb.Config)` → `yaml.JSONToYAML` → `WriteFile`; `Load` is the
inverse, and an **empty file is valid** (it short-circuits to the zero `pb.Config`). A
generated example lives in `internal/config/testdata/legacy-kubecom.yaml`; the four shape
rules that follow from that pipeline are D180 pt 1.

Only three top-level keys exist (`pb/config.proto`), and only two things could ever appear
in a real file:

- **`menu`** — written by `app/ui/resourceMenu.saveItems`, which serializes the *whole* menu
  (not a diff) as `{namespaced, group, kind, title}`. There is **no apiVersion and no plural
  resource name**, which is why v1 cannot carry it over: `MenuResource` addresses a resource
  by group/version/resource, and only live discovery can supply the rest.
- **`currentTheme`** — written by `theme/manager.NextTheme`/`PrevTheme`, so its value is one
  of the five 2020 built-ins (`base16`, `monokai`, `paraiso`, `solarized`, `twilight`).
  `ConfigUpdated` substituted `base16` when it was empty, so an **empty value meant base16**
  in the running app while still being empty on disk (D179 pt 3 declines to migrate that).
- **`themes`** — the abandoned theme engine's palette tree. The 2020 app **never wrote this
  key**: nothing calls `UpdateConfig` with it, so a palette tree in a real file was
  hand-authored by the user. It round-trips through `Save` once present, because `Save`
  re-marshals the whole loaded message.

There were **no keybindings** in the legacy config — the 2020 keymap was hardcoded, so
v1's `keys:` has nothing to migrate from.
