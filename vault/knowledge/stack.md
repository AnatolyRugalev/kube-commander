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
- **vhs** — recording the README screencast (replaces terminalizer). The tape lives at
  `docs/screencast.tape`, `make screencast` runs it, and D181 governs both. Practical
  facts an agent needs (verified M5-09): vhs is a Go program, so
  `go install github.com/charmbracelet/vhs@latest` works in the sandbox and
  **`vhs validate <tape>` parses a tape with no cluster, no terminal and no display** —
  use it, it is the only mechanical check of tape *syntax* there is. It cannot
  **record** there: vhs drives **ttyd** for the terminal and **ffmpeg** for encoding, and
  neither is in the image nor obtainable with `go install`. Tape syntax notes: `Require`
  goes at the top (before `Output`), comments are `#` lines (keep annotations on their own
  line rather than trailing a command), and `Set Width/Height` are **pixels** — at
  `FontSize 16` a 1200×700 frame is roughly 125×36 cells.

## Release & packaging (goreleaser)

Pinned to **v2.17.1** in `.github/workflows/release.yml` (one place, guarded). It is a Go
tool, so `go install github.com/goreleaser/goreleaser/v2@v2.17.1` works in the sandbox —
do that rather than reasoning about the config. Note it needs Go ≥ 1.26.5 and will switch
toolchains on its own; that does not affect this module's 1.24.2 floor.

Three commands, and they are **not** substitutes for each other:
- `goreleaser check` — validates the config, and is the only thing that reports
  **deprecated** options. Verified M5-06: a snapshot passes a config `check` rejects.
- `goreleaser release --snapshot --clean` — builds and *renders every publisher's output*
  into `dist/` (`dist/homebrew/Casks/*.rb`, `dist/aur/*.pkgbuild`, `*.srcinfo`) without
  publishing anything. Reading those files is how a publisher gets verified without
  credentials.
- `goreleaser jsonschema -o schema.json` — dumps the full config schema. Faster and more
  reliable than recalling field names; `$defs` holds one entry per config block.

Publisher facts an agent needs (all verified by reading `internal/pipe/*` at v2.17.1):
- **`skip_upload` is templated and checked first**, before any credential is read, in every
  publisher — which is what makes the D182 pt 3 / D183 "inert without its secret" pattern
  work. Use `{{ index .Env "X" }}`, never `.Env.X` (the latter *errors* on an absent key).
- **A field can be accepted, documented and silently dropped.** `homebrew_casks.conflicts.formula`
  parses fine and never reaches the generated `.rb` (M5-06). `check` catches some of these;
  reading the pipe's `template.go` catches the rest. Do not assume a config line does
  something because goreleaser accepted it.
- **`aurs:` forces a `-bin` suffix on `name`** (`Default()`), and `git_url` has **no
  default** — an unset one makes the publish a silent no-op (`pipe.Skip("url is empty")`).
  `aurs` also matches *both* archive types, so `ids:` is required when the config declares a
  bare-binary archive alongside the tar.gz, or each arch gets duplicate `source_` lines.
  AUR keys must be **passphrase-less**: goreleaser hard-errors on an encrypted one.
- **Version transforms differ per packager.** `.Version` drops the tag's leading `v`; the
  AUR PKGBUILD additionally rewrites `-` to `_` (`v1.0.0-rc.1` → `pkgver=1.0.0_rc.1`).
- **`dockers_v2` is the live docker pipe; `dockers:` + `docker_manifests:` are deprecated**
  (`check` says "being phased out", M5-08). One entry replaces both. Its build context is a
  **temp dir**, not the repo — the Dockerfile is copied in and the binaries laid out as
  `<goos>/<goarch>/<binary>`, so `ARG TARGETPLATFORM` + `COPY $TARGETPLATFORM/<bin>` is the
  only correct source path, a repo-root `.dockerignore` is dead, and any repo file the
  Dockerfile needs must be listed in `extra_files`. Under `--snapshot` it builds **one image
  per platform** with `--load` and suffixes each tag with `-<arch>`; the real run builds one
  index and pushes it. `sbom:` defaults to **true**, which with `--push` adds
  `--attest=type=sbom` — that plus a multi-platform index is why the docker-container buildx
  driver is required and the default `docker` driver is not enough (D184 pt 2).
- **Sandbox artifact:** download URLs are derived from the git remote, which here is the
  local proxy — so generated files contain `https://github.com/git/AnatolyRugalev/…` and a
  synthesized `v0.0.0-next`. Both are correct on a real tagged CI run; neither is a bug to
  chase.

**Docker in the sandbox (verified M5-08).** The `docker` *client* is on PATH but **no daemon
is running** — `dockerd &` starts one (we are root) and then everything works: pulls reach
`gcr.io`/`docker.io` through the proxy, buildx builds both arches of a `COPY`-only Dockerfile
with no QEMU, and `--load`ed images run. So the Docker slice is the one distribution path an
agent can verify end to end: build the image, `docker run … version`, and even drive the TUI
— `docker run -d -it` keeps it alive and `script -qec "docker attach <c>" /dev/null` with a
FIFO on stdin captures a real rendered frame. (`docker logs` on a tty container returns
nothing useful; attach is the way.)

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
- **`metrics.k8s.io/v1beta1` on the wire** (M4-09, `metrics.go` — read through the
  dynamic client; kubecom takes no `k8s.io/metrics` dependency):
  - The two kinds sit at **counter-intuitive resource names**: `PodMetrics` is
    served at `pods` (namespaced) and `NodeMetrics` at `nodes` (cluster-scoped),
    within the `metrics.k8s.io` group. Never guess the plural from the kind.
  - **Two different shapes.** A `NodeMetrics` carries one flat top-level
    `usage: {cpu, memory}`; a `PodMetrics` carries `containers: [{name, usage}]`
    and the pod total is the **sum** of them (what `kubectl top pod` reports).
  - Quantities are always JSON **strings** (`"250m"`, `"64Mi"`) — `Quantity`
    marshals as one even for whole numbers — so `unstructured.NestedString` +
    `resource.ParseQuantity` is the right read; `MilliValue()` for CPU,
    `Value()` for memory.
  - `timestamp` (RFC3339) and `window` (a Go-parseable `"30s"`) are the **only
    staleness signal**: metrics-server keeps serving its last scrape, so a
    successful request says nothing about freshness.
  - There is **no watch verb** — samples are point-in-time and must be polled.
- **`labels.Parse` quirks** (`k8s.io/apimachinery/pkg/labels`, used by the search
  query parser, SEARCH-04c-1):
  - It accepts a **bare identifier** — `labels.Parse("nginx")` is the valid
    existence selector *has label `nginx`*. So "does it parse as a selector" can
    never be used to detect that a string *is* one; a search query needs an
    explicit token (`-l`).
  - `labels.Parse("")` is `Everything()`, whose `String()` is `""` — so an empty
    selector round-trips to "no selector" and needs no special case.
  - `app=` is **valid** (equals the empty value); `app=!!`, `!`, and `a b` are not.
  - `Selector.String()` normalises (`tier in (a, b)` → `tier in (a,b)`), so
    storing the parsed form is what makes selectors comparable in tests.
  - Errors name the requirement but not the concept (`unable to parse
    requirement: found '!', expected: identifier`) — wrap before showing a user.

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
  - **Seeding an object whose resource name is not guessable (M4-09):** objects
    passed to `NewSimpleDynamicClient*` are filed under a GVR the fake *infers from
    the kind* (`meta.UnsafeGuessKindToResource`), which is right for
    `Deployment`→`deployments` and wrong wherever the API disagrees — a
    `PodMetrics` is served at `pods` and a `NodeMetrics` at `nodes`, so seeded
    metrics items land in a resource nothing lists and every List comes back empty
    **with no error**. Construct the fake *empty* and add such objects through
    `f.Tracker().Create(gvr, obj, ns)` with the explicit GVR; the custom
    GVR→listKind map is still needed, separately, for List to decode.
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
