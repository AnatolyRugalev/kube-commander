# Target Stack

The intended libraries and versions for kubecom. Confirm exact versions at M0
`go.mod` time; pin to recent stable.

## Language / toolchain
- **Go 1.23+**
- cobra (CLI), stdlib **slog** for logging (replaces klog-as-primary).

## TUI (Charmbracelet)
- **bubbletea** — Elm-architecture runtime (Model/Update/View, `tea.Msg`, `tea.Cmd`).
- **bubbles** — components: `table`, `list`, `textinput`, `viewport`, `help`,
  `key`, `spinner`. NOTE: `bubbles/table` is basic — expect a **custom table**
  for sorting + large row counts.
- **lipgloss** — styling/layout; drives themes.
- **teatest** — TUI model testing.
- **vhs** — recording the README screencast (replaces terminalizer).

## Kubernetes
- **k8s.io/client-go** (target **v0.31**), apimachinery, cli-runtime as needed.
- **k8s.io/client-go/dynamic** — generic typed-free access; server-side Table
  printing via `Accept: application/json;as=Table` for kubectl-identical columns.
- **discovery** + **restmapper** — GVK↔GVR, namespaced?, verbs; async + cached
  (cached discovery client with disk cache + invalidation).
- **client-go/tools/remotecommand** — exec/attach (interactive; suspend + raw PTY).
- **client-go/tools/portforward** + SPDY/websocket dialer — background port-forward.
- **k8s.io/kubectl/pkg/describe** — in-process describe output.
- Pod logs via `CoreV1().Pods(ns).GetLogs(...).Stream(ctx)`.
- **metrics.k8s.io** client — optional CPU/mem columns when metrics-server present.

## Key API patterns
- **Watch → messages:** a `kube` goroutine runs List+Watch and pushes events onto
  a Go channel; a `tea.Cmd` reads one and returns it as a `tea.Msg`. UI state is
  only ever mutated inside `Update`.
- **Discovery is non-blocking:** callers get the seed set immediately; full
  discovery arrives later as a message.
- **Fault isolation:** wrap per-group discovery so a failing/denied group returns
  a partial result, never an error that aborts the whole load.

## Config
- Plain typed struct → YAML at `~/.kube/kubecom.yaml` (path TBD). Migration shim
  reads the legacy protobuf-yaml file once.
- Includes a **`keys:`** section (`action id → [keys]`) merged onto the default
  keymap — the single source of key bindings. No key literal lives in view code
  (see [D11](decisions.md#d11--fully-configurable-keybindings-zero-hard-coded-keys)
  and [`keybindings.md`](keybindings.md)).

## Keymap / input
- **Action registry**: named `Action` ids; a default keymap (one data table)
  expressing the vim-first scheme; `merge(default, config.Keys)` at load with
  validation (unknown-action, collision, nav-shadow warning).
- Views resolve `tea.KeyMsg → Action` via the keymap and switch on `Action`;
  `bubbles/key.Binding`s and the help/keybindings doc are generated from it.
