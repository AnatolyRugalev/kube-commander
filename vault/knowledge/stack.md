# Target Stack

The intended libraries and versions for kubecom. Confirm exact versions at M0
`go.mod` time; pin to recent stable.

## Language / toolchain
- **Go 1.23+**
- cobra (CLI), stdlib **slog** for logging (replaces klog-as-primary).
- **Logging rule:** while the TUI owns the terminal, *nothing* may write to
  stdout/stderr — a stray print corrupts the alt-screen. slog writes to a log
  file (under the user state/cache dir); route klog/client-go warnings there
  too. No `fmt.Print*` outside `cmd/` pre-TUI paths. Implemented in
  `cmd/kubecom/logging.go`; **`klog.LogToStderr(false)` alone is not enough** —
  klog still copies ERROR lines to stderr unless you also raise `stderrthreshold`
  to FATAL (D71).

## TUI (Charmbracelet)
- **bubbletea v2** (D19) — Elm-architecture runtime (Model/Update/View, `tea.Msg`,
  `tea.Cmd`). Pin v2 with **matching bubbles/lipgloss releases**; write all TUI
  code against the v2 API. Beware v1-era examples/snippets — APIs differ.
  - **Import path rebranded** to **`charm.land/bubbletea/v2`** (was
    `github.com/charmbracelet/bubbletea/v2`); pinned **v2.0.2** (D26) — v2.0.3+
    require Go 1.25, v2.0.2 holds the floor at Go 1.24.2. `teatest` stays at
    `github.com/charmbracelet/x/exp/teatest/v2`.
  - v2 API shape: `Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)`,
    **`View() tea.View`** (not `string`; build with `tea.NewView("...")`). Keys
    arrive as **`tea.KeyPressMsg`** (v1's `KeyMsg` split into press/release).
- **bubbles** — components: `table`, `list`, `textinput`, `viewport`, `help`,
  `key`, `spinner`. NOTE: `bubbles/table` is basic — expect a **custom table**
  for sorting + large row counts.
  - **Pinned `charm.land/bubbles/v2` v2.0.0** (D50, added M2-01d for `help`+`key`).
    v2.0.0's go directive is **1.24.2** and it requires bubbletea **v2.0.0** (MVS
    keeps our pinned **v2.0.2** — no downgrade). v2.1.0 requires bubbletea v2.0.2
    (fine) but its **go directive is 1.25.0**; v2.1.1 requires bubbletea v2.0.7.
    So v2.0.0 is the release that pairs with our v2.0.2/Go-1.24.2 floor without a
    toolchain bump — hold it here in lockstep with the bubbletea pin (D26).
- **lipgloss** — styling/layout; drives themes. Pulled in as
  **`charm.land/lipgloss/v2` v2.0.0** (indirect, via bubbles/help; go directive
  1.24.2).
- **teatest** — TUI model testing.
- **vhs** — recording the README screencast (replaces terminalizer).

## Kubernetes
- **k8s.io/client-go** (target **v0.31**), apimachinery, cli-runtime as needed.
- **k8s.io/client-go/dynamic** — generic typed-free access (CRDs, unstructured).
- **Server-side Table** printing via `Accept: application/json;as=Table;v=v1;g=meta.k8s.io,application/json`
  for kubectl-identical columns. **Gotcha (D33):** the dynamic client can *not*
  set this Accept header per call — its `List` always returns the plain object
  list. `List` (M1-05a, `internal/kube/table.go`) instead builds a per-GroupVersion
  `rest.Interface` (`restClientForGV`) and sets the header on the raw request, then
  `decodeTable` flattens the `metav1.Table` JSON into a TUI-facing
  `Table{Columns,Rows}` (no apimachinery in the TUI); per-row identity
  (`ObjectRef`) comes from the row's embedded `PartialObjectMetadata`
  (`IncludeObject=Metadata`, the Table default). Watch (M1-05b, `internal/kube/watch.go`,
  D34) reuses both: `Clients.Watch` runs a reconnecting List→Watch goroutine that
  streams `WatchEvent{ADDED/MODIFIED/DELETED/RESET/ERROR}` on a bounded channel;
  it opens the same request with `watch=true` via `.Stream()` and decodes the
  `metav1.WatchEvent` stream with `decodeTableRV`.
- **discovery** + **restmapper** — GVK↔GVR, namespaced?, verbs; async + cached.
  On-disk cache landed M1-04 (D32): `discovery/cached/disk`'s `CachedDiscoveryClient`
  (kubectl's own), base of the deferred RESTMapper. Cache dir
  `os.UserCacheDir()/kubecom/{discovery/<host-slug>,http}` (per host:port —
  distinct clusters must not share a dir); TTL 6h; `Clients.Invalidate()` forces a
  refetch (clears the disk cache *and* Resets the deferred mapper). No cache dir
  resolvable → degrade to in-memory `memcache`. New transitive deps:
  `gregjones/httpcache`, `peterbourgon/diskv`, `google/btree`.
- **In-process action set** (M1-06, D2): mutating actions target objects
  **generically through the dynamic client**, addressed by GVR + scope (from the
  discovery `Resource`) and namespace/name (from a row `ObjectRef`) via the shared
  `resourceInterface(r, ns)` helper — one path for built-ins and CRDs, no per-kind
  typed clients. **Delete** (M1-06a, D35) adds a **UID precondition** from the row
  when present, so acting on a table snapshot never hits a recreated same-named
  object. **Scale + RolloutRestart** (M1-06b, D36) are **merge patches** through
  the same `resourceInterface` helper — scale merge-patches the `scale`
  subresource (`.Patch(..., "scale")`, replicas live at `scale.spec.replicas` for
  every scalable kind), rollout-restart merge-patches
  `spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"]` with a
  UTC RFC3339 timestamp (kubectl's exact key, so the two tools interoperate). A
  merge patch (not strategic) keeps it schema-free → works on unstructured/CRDs,
  and only adds restartedAt without clobbering sibling annotations. No UID guard:
  `PatchOptions` carries no preconditions and these actions are idempotent.
  06c cordon+drain / 06d cronjob-suspend build on this.
- **client-go/tools/remotecommand** — exec/attach (interactive; suspend + raw PTY).
- **client-go/tools/portforward** + SPDY/websocket dialer — background port-forward.
- **k8s.io/kubectl/pkg/describe** — in-process describe output.
- Pod logs via `CoreV1().Pods(ns).GetLogs(...).Stream(ctx)`.
- **metrics.k8s.io** client — optional CPU/mem columns when metrics-server present.
- **Typed error taxonomy** (M1-09, D46, `internal/kube/errors.go`): the layer wraps
  every error (`fmt.Errorf(... %w)`); `Classify(err) ErrorKind` walks that chain to a
  small enum — `KindNotFound`/`AlreadyExists`/`Conflict`/`Forbidden`/`Unauthorized`/
  `Invalid`/`Timeout`/`Unreachable`/`BadContext`/`Unknown` — so the M2 TUI degrades one
  feature instead of crashing (#86). apierrors `Is*` predicates already unwrap `%w`;
  transport failures come as `*url.Error`/`net.Error` (no HTTP status). **Gotcha:** a
  bad **override** context is a plain `fmt.Errorf("context %q does not exist")` in
  clientcmd (`client_config.go`) that **no clientcmd predicate matches** — so
  `RESTConfig` tags its errors with the `errBadContext` sentinel (dual-`%w`) and
  Classify keys off `errors.Is`, not clientcmd's wording.

## Key API patterns
- **Server-side Table watch gotchas** (all handled in M1-05b, `watch.go`, D34):
  - Request `includeObject=Metadata` (or `Object`) — without it Table rows carry
    no per-row object identity (name/namespace/uid), which actions need. _(Table
    default is `IncludeObject=Metadata`; List/Watch rely on it.)_
  - **Column definitions are only guaranteed on the first Table response**;
    subsequent watch chunks may omit them. `streamTableWatch` caches columns per
    connection and stamps every emitted event with the current set.
  - Uses **bookmark events** (`allowWatchBookmarks=true`) + `resourceVersion` to
    resume cheaply; `410 Gone`/`Expired` (sentinel `*errExpired`) forces a full
    re-List + `RESET`, resumable drops reconnect from the last RV with no re-List.
- **Watch → messages:** a `kube` goroutine runs List+Watch and pushes events onto
  a Go channel; a `tea.Cmd` reads one and returns it as a `tea.Msg`. UI state is
  only ever mutated inside `Update`.
- **Discovery is non-blocking:** callers get the seed set immediately; full
  discovery arrives later as a message.
- **Fault isolation:** wrap per-group discovery so a failing/denied group returns
  a partial result, never an error that aborts the whole load.

## Testing (D18)
- Default: **client-go fake clients** (`fake.Clientset`, fake dynamic + fake
  discovery) — hermetic, no network, runs in any sandbox/CI.
  - **Gotcha (D35):** the **fake dynamic client discards `DeleteOptions`** — its
    `Delete` builds `testing.NewDeleteAction` (no options variant), so a recorded
    action's `GetDeleteOptions()` is always zero. It cannot verify propagation of
    delete preconditions / propagation policy. Keep option-shaping logic in a
    **pure helper** and unit-test that directly (e.g. `withUIDPrecondition` in
    `actions.go`); use the fake only for the round-trip (object removed, namespace
    routing, wrapped errors). Build it with
    `NewSimpleDynamicClientWithCustomListKinds` + an explicit GVR→listKind map so
    it never guesses list kinds for unstructured seed objects.
  - **Patch does round-trip (D36):** unlike Delete, the fake dynamic client
    *applies* a merge patch to the whole tracked object and **ignores the
    subresource**, so a `scale`-subresource merge patch (`{"spec":{"replicas":N}}`)
    lands as `spec.replicas` on the seed object and is directly assertable via
    `unstructured.NestedInt64`. Merge-patch semantics let a rollout-restart test
    prove restartedAt is added without clobbering sibling annotations. Still keep
    the wire format in a pure helper (`scalePatch`/`restartPatch`) so key + format
    are testable without a client; the subresource itself is asserted off the
    captured `PatchAction.GetSubresource()`.
- **envtest** (real kube-apiserver via `setup-envtest`) is opt-in behind
  `KUBECOM_TEST_ENVTEST=1`. **Harness landed in M1-00** (D28):
  `internal/kube/envtest_test.go` — `requireEnvtest(t)` skips unless the gate is
  set (so `make check` stays hermetic), `make test-envtest` fetches binaries via
  `setup-envtest` (`ENVTEST_K8S_VERSION ?= 1.31.x`) and runs the gated suite. It
  downloads binaries — do not make `go test ./...` depend on it.
  - Deps that arrived with it: `k8s.io/client-go` + `k8s.io/apimachinery` v0.31.4,
    `sigs.k8s.io/controller-runtime` v0.19.4 (the release paired with client-go
    v0.31). These are the kube layer's foundation for M1-01+.
- **teatest** for TUI model tests.

## Config
- Plain typed struct → YAML at **`os.UserConfigDir()/kubecom/config.yaml`**
  (D20) — `~/.config/kubecom/config.yaml` on Linux. Not under `~/.kube/`.
  Migration shim reads the legacy protobuf-yaml file once.
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

## TUI rendering (lipgloss v2)
- **Bordered `Style.Width`/`Height` include the border.** A `styles.Pane`/`PaneFocus`
  frame (rounded border) sized `Width(w)` has a **content area of `w-2`**, not `w`.
  Size a bordered pane to the component's *total* width/height and render the
  inner content to the `(w-2)×(h-2)` region — sizing the frame to the inner width
  wraps every full-width line. Clip inner lines to the content width yourself
  (rune cut) rather than relying on `MaxWidth`, which does not prevent `Width`'s
  wrapping. See D58; guarded by `table.TestViewFitsPaneNoWrap`. The `menu` and
  `statusbar` panes only render short lines today so they don't visibly hit this,
  but the same total-size rule applies when they need full-width rows.
