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
- **A CRD's conversion webhook can fail a LIST, and it is not a client problem**
  (2026-08-01, HT-dogfood-0801/D191). A CRD with more than one *served* version and
  `spec.conversion.strategy: Webhook` makes every LIST of that kind pass through the
  webhook: if it is down, unreachable or serving a bad cert, the **apiserver** returns the
  error, identically to `kubectl get`. This is the diagnosis of the `ExternalSecret` report
  that CRD-01 was raised for — a fresh chart (only `v1` served, `strategy: None`) lists fine
  from the same code. So when a whole kind errors on open and sibling kinds in the same
  group do too, read the CRD before the client:
  `kubectl get crd <name> -o jsonpath='{.spec.conversion.strategy}{"\n"}{range
  .spec.versions[*]}{.name}{" served="}{.served}{"\n"}{end}'`. Note DISC-01/D187 only covers
  *discovery* failing for a group; a conversion failure is per-LIST and shows up later.
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
- **Exec credential plugins** (AUTH-01, D195, `internal/kube/authexec.go`): the
  `user.exec` stanza client-go runs to mint credentials (`aws eks get-token`,
  `gke-gcloud-auth-plugin`, `az`). `ExecPluginFor(cc)` reads the selected context's
  stanza from the raw kubeconfig (command/args/env/apiVersion/installHint) — no
  execution, no network; a context without one returns `nil, nil`.
  **Three gotchas, all in `plugin/pkg/client/auth/exec/exec.go` (v0.31):**
  (1) the plugin's **stderr goes to the process's `os.Stderr`** (`a.stderr` is set at
  construction and has no seam) — under the alt-screen the user never sees it, and it is
  *not* in the returned error, so recovering "why" needs a diagnostic re-run (AUTH-02);
  (2) the failure is formatted `fmt.Errorf("getting credentials: %v", err)` — **`%v`, not
  `%w`** — so the `*exec.ExitError` does not survive the chain and `errors.As` cannot see
  it; text matching is the only route (`ExecPluginFailed`, matching client-go's
  `wrapCmdRunErrorLocked` shapes `exec: executable X not found` / `… failed with exit code
  N`, but not its nameless `exec: %v` default branch);
  (3) it fails **inside `RoundTrip`**, so net/http wraps it in a `*url.Error` and it
  classified as `KindUnreachable` until `KindExecPlugin` was added ahead of the transport
  net — a cluster reported unreachable that was never contacted.

- **Connecting to a second cluster is cheaper than it looks** (checked in v0.31 for
  CTX-WARM-01/D196, before any "keep the previous cluster warm" work is built):
  (1) `kube.Connect` does **no network I/O** — `RESTConfig` reads the kubeconfig,
  `NewClients` builds the clientset/dynamic/discovery handles locally, and the RESTMapper is
  a static seed ahead of a *deferred* discovery mapper (D8/M1-02) that warms on first use;
  (2) discovery is **disk-cached per host with a 6 h TTL** (`internal/kube/cache.go`,
  kubectl's own), so a second visit to a cluster in the same session reads files, not the
  API server;
  (3) client-go keeps **process-wide caches** that survive building a new `rest.Config` for
  the same cluster — `transport/cache.go`'s `tlsCache` (keyed `tlsCacheKey`, so an identical
  config reuses the `*http.Transport` and its connection pool) and
  `plugin/pkg/client/auth/exec`'s `globalCache` of authenticators (so a credential plugin is
  not re-run per client).
  Net: "reconnect" is mostly local work already, which is why D196 pt 3 gates any retention
  cache on the switch timings (`context switch complete` in the diagnostic log) instead of
  on the assumption that reconnecting is expensive.

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
- **Exec credential plugins** (`user.exec` in the kubeconfig; `internal/kube/authexec.go`,
  AUTH-01/02):
  - client-go runs the plugin inside `RoundTrip`, so a failure arrives wrapped in a
    `*url.Error` and reads as "cluster unreachable" unless classified first (D195 pt 1).
  - Its **stderr goes straight to the process's `os.Stderr`** — under the alt-screen the
    user never sees it — and there is no seam to capture it. The returned error is
    formatted with `%v` (`getting credentials: %v`), so the `*exec.ExitError` does **not**
    survive in the wrap chain: no `errors.As` path, only text. Recovering *why* it failed
    means re-running the plugin (`ExecPlugin.Diagnose`, D211).
  - `Cmd.Env` is the process environment **plus** the stanza's `env:`; the stanza's own
    `args`/`env` are the only place a remediation's detail (`--profile`, `AWS_PROFILE`) may
    be substantiated from (D195 pt 5).
  - **A failed plugin is not negatively cached** (v0.31 `exec.go`: `getCreds` →
    `refreshCredsLocked`, which assigns `a.cachedCreds` only *after* a successful run). So
    the next request through the same client runs the plugin again and picks up whatever a
    re-authentication wrote in between — no client rebuild, no reconnect. That is what makes
    "retry the failed request" a complete recovery after a remediation (AUTH-05a/D215), and
    it is also why a cluster whose session expired emits a failure per watch retry rather
    than one and then silence (the AUTH-04b latch).
- **AWS CLI wordings for an expired/absent SSO session** (what `aws eks get-token` prints
  on stderr; matched by `awsSSOExpiryMarkers`, AUTH-03). All three name SSO, which is what
  keeps the match narrow:
  - `The SSO session associated with this profile has expired or is otherwise invalid. To
    refresh this SSO session run aws sso login with the corresponding profile.` —
    botocore's `UnauthorizedSSOTokenError`; the cached token exists but the server rejected it.
  - `Error when retrieving token from sso: Token has expired and refresh failed` — the
    cached token is past its expiry and the refresh grant did not work either.
  - `Error loading SSO Token: Token for https://acme.awsapps.com/start does not exist` —
    never logged in (or the cache was cleared). Same remediation, different cause.
  - **Not** SSO expiry, and needing different fixes: `ExpiredTokenException` (an STS session
    token, refreshed by the plugin itself), `Unable to locate credentials` (no profile
    configured), `AccessDeniedException … eks:DescribeCluster` (an IAM policy).
  - The CLI's own profile precedence, which `awsProfileOf` mirrors: `--profile` on the
    command line > `AWS_PROFILE` > the legacy `AWS_DEFAULT_PROFILE`.
- **`os/exec` gotcha: killing a process does not unblock `Wait`.** With a non-`*os.File`
  `Stdout`/`Stderr`, `os/exec` copies through a pipe in a goroutine and `Wait` blocks on it.
  A killed process that spawned a child leaves the child holding the write end, so `Wait`
  hangs long past the context deadline — an `sh -c 'sleep 30'` takes the full 30s despite a
  50 ms deadline. **`Cmd.WaitDelay`** is the bound that actually closes the pipe (found in
  AUTH-02; applies to any future subprocess kubecom captures output from).
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
  - **It works in the agent sandbox** (M1-INT-a, D186 pt 1 — the D18/D66 assumption
    that it would not is what deferred M1-INT for ten days). Recipe, ~90 s cold:

    ```
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19
    export PATH=$(go env GOPATH)/bin:$PATH
    make test-envtest        # or: KUBEBUILDER_ASSETS="$(setup-envtest use -p path 1.31.x)" \
                             #     KUBECOM_TEST_ENVTEST=1 go test ./internal/kube/...
    ```

    Pin the tool to `@release-0.19` to match controller-runtime v0.19.4; `@latest`
    pulls a newer module line. `@release-0.19` is a *branch*, so it re-resolves on
    every install — CI pins the pseudo-version it points at instead
    (`v0.0.0-20250308055145-5fe7bb3edc86`, M1-INT-d/D190). Binaries land in
    `~/.local/share/kubebuilder-envtest/k8s/1.31.0-linux-amd64`. A whole control
    plane starts in ~4 s, so a test per plane is affordable — and preferable, since
    these tests break cluster-global state on purpose (`startControlPlane` in
    `envtest_test.go` is the shared bootstrap).
  - **What a live apiserver catches that a fake cannot** (M1-INT-a): a fake
    discovery client returns whatever shape the test wrote, so it can only confirm
    that `discoverResources` handles `*ErrGroupDiscoveryFailed` — never that a real
    server produces one. It does not. Registering an `APIService` whose backing
    Service is missing (`v1beta1.metrics.k8s.io` → `kube-system/metrics-server`, the
    metrics-server outage of #87) makes the group genuinely fail, and on this path
    `ServerPreferredResources` returns **no error**, because aggregated discovery
    reports the group with zero versions and the disk-cached client drops the stale
    marker (D186 pt 2, DISC-01). The APIService is `Available=False
    ServiceNotFound` within a second or two; `ServerResourcesForGroupVersion` on the
    broken GV is the reliable signal that the breakage has landed (poll on it, after
    `Clients.Invalidate()`, before asserting anything).
  - **How a broken group is actually detected** (DISC-01, D187): not from the
    preferred-resources error — there is none on this path — but from `ServerGroups()`,
    where the broken group survives with an **empty `Versions` slice**. Useful map of
    what each call says about `metrics.k8s.io/v1beta1` while its backing service is
    missing, all against the same live plane:

    | call | result |
    | --- | --- |
    | `APIService` status | `Available=False  ServiceNotFound` |
    | `ServerGroups()` | group present, `Versions: []` ← the detectable signal |
    | `ServerResourcesForGroupVersion(gv)` | `stale GroupVersion discovery` |
    | raw `DiscoveryClient.ServerGroupsAndResources()` | `*ErrGroupDiscoveryFailed` |
    | cached `ServerPreferredResources()` (what kubecom calls) | lists, `err=<nil>` |

    Both cached clients drop the failure, for different reasons: the **disk** cache is
    not an `AggregatedDiscoveryInterface` at all, and what it persists is the *split*
    group list, so it could not become one; the **memory** cache is one, but it costs a
    network round trip per pass (against D8). The group list is the cheap, cached,
    already-fetched artifact — hence D187.
  - **Breaking the transport, not the server** (M1-INT-b-1): to prove the watch loop's
    *resume* path against a live apiserver you need a failure the server never
    produces — a dead connection. `startKillableProxy` in `envtest_watch_test.go` is a
    raw TCP proxy on loopback that forwards to `cfg.Host` and can `dropAll()` its live
    connections; point a copy of the config at it (`rest.CopyConfig`, `Host =
    "https://"+proxy.addr()`, and carry `ServerName` over from the real host so the
    apiserver's serving cert still verifies — the proxy is a pipe, not a MITM). Raw TCP
    keeps TLS end-to-end, so nothing about the client's behavior changes except that the
    wire can be cut. Two rules learned: write through a **direct** client (never the
    proxied one) so cluster changes during the outage are unaffected by it, and **count
    dials** — without asserting the connection was re-established, a test like this
    passes just as happily when the drop silently did nothing.
  - **A stock envtest plane does not expire a resourceVersion** (probed for M1-INT-b-2):
    watching pods from `resourceVersion=1` does not 410 — not immediately, and not after
    150 writes push the revision far past it (all 150 events replay). The window that
    matters is etcd's revision history, not the watch cache's size, and nothing compacts
    etcd on a short-lived test plane (`--etcd-compaction-interval` defaults to 5m).
  - **How to make a real apiserver produce a 410/Expired inside a test** (M1-INT-b-2).
    Two flags on `startControlPlane(t, withAPIServerFlag(…))`, and **both** are needed:

    ```
    withAPIServerFlag("etcd-compaction-interval", "100ms")  // history is bounded at all
    withAPIServerFlag("watch-cache", "false")               // …and *that* bound is the one in force
    ```

    Compaction alone is not enough, and the reason is the trap: with the watch cache on,
    the apiserver answers the watch from memory, so etcd's compaction is invisible and the
    watch replays happily from a revision etcd no longer holds. The cache's own window is
    what bounds history then — and it cannot be shrunk into test range, because it starts
    at 100 events and **grows** whenever it fills within 75 s. Writing past it therefore
    enlarges it instead of evicting (which is exactly what the 150-event probe above
    measured). Both paths answer an out-of-window watch with the same 410/`Reason:
    Expired` status, so serving from etcd is a faithful shortcut, not a different code
    path in the client.
    With those flags a held resourceVersion becomes unreplayable **~300 ms** after the
    next write — fast enough to stale a live watch inside one `watchRetryBackoff` gap.
    Compaction only drops history below a revision it has already seen, so keep writing
    while waiting: a quiet cluster never expires anything.
  - **Prove the expiry happened before asserting the reaction** (M1-INT-b-2). Open a
    throwaway watch from the held resourceVersion and read one event
    (`firstWatchEventFrom`): a `Status` with `Reason: Expired`, code 410, message *"The
    resourceVersion for the provided watch is too old."*, which `watchStatusError` maps to
    `*errExpired`. Without that precondition check, an unexpired revision makes the whole
    test vacuous in the most confusing way available — the loop simply *resumes*, which is
    correct behavior for the situation it is actually in, so the failure looks like a
    watch-loop bug rather than an unmet premise.
  - **`DeleteOptions` only exist on a real server** (M1-INT-c-1). The fake dynamic
    client discards them wholesale, so nothing kubecom puts in them — the UID
    precondition, the propagation policy — means anything until an apiserver reads it.
    Two facts from doing so:
    - A **failed UID precondition is a 409 Conflict**, message *"Precondition failed:
      UID in precondition: …, UID in object meta: …"*, which `Classify` already maps to
      `KindConflict`. Staging the race it guards needs no timing: delete the object and
      recreate it under the same name through the typed client, then act from the stale
      ref. Assert the survivor's UID too — a refusal that let the object die anyway
      would still produce the error.
    - **A foreground delete never completes on an envtest plane**, because envtest runs
      no controller-manager and therefore no garbage collector. The object keeps its
      `deletionTimestamp` and its `foregroundDeletion` finalizer for the life of the
      plane. That is what makes the policy *observable* (it is the only DeleteOption
      with visible server-side state), and it is a trap for any later test that deletes
      foreground and then waits for the object to disappear.
  - **A subresource is a different endpoint, and the fake has no notion of one**
    (M1-INT-c-2). The fake dynamic client applies whatever patch it is handed to the
    whole tracked object and ignores the subresource argument entirely, so a hermetic
    test can prove the *wire format* of a subresource patch and nothing about where it
    lands. Two consequences when the same call meets a real apiserver:
    - **Scaling a kind that has no `scale` subresource is a 404**, not a silently
      applied field. `apps/v1` registers `scale` for Deployment / ReplicaSet /
      StatefulSet / ReplicationController but **not for DaemonSet** (one pod per node
      by definition). Under the fake the same call succeeds and grows a `spec.replicas`
      the kind does not have; the server has no route and answers NotFound. That
      disagreement — the fake permitting what a server refuses — is the only reliable
      discriminator for "did this patch go to the subresource?", because for the kinds
      that *do* scale, `{"spec":{"replicas":N}}` against the object body sets the same
      field and passes either way.
    - A merge patch carrying a field the typed schema does not have is **dropped with
      a warning, not rejected**: the apiserver logs `unknown field "spec.replicas"` and
      returns 200. So an assertion that a bad patch "fails" must read the object back
      and check the field is absent, not just check the error.
  - **A merge patch is validated per field, and "unknown" is not an error**
    (M1-INT-c-3). A real apiserver type-checks a field its schema *knows* — a string
    in `spec.unschedulable` is a **422 Invalid** (`Classify` → `KindInvalid`), message
    *"json: cannot unmarshal string into Go struct field NodeSpec.spec.unschedulable
    of type bool"* — but a field it does **not** know is dropped with a warning and a
    **200**, unchanged `resourceVersion` included (the c-2 finding, now confirmed for a
    whole action: `Suspend` on a Deployment returns nil and does nothing). So a
    merge-patch action pointed at the wrong kind is a silent success, and the only
    guard is the kind gating in the UI registry (D107/D188).
  - **"Set the flag to an explicit false" lands differently per schema**
    (M1-INT-c-3). `NodeSpec.Unschedulable` is a `bool` with `omitempty`, so after an
    uncordon the key is **gone** from the stored object — `NestedBool(…, "spec",
    "unschedulable")` returns `found=false`, and any test asserting "present and false"
    fails against a server while passing against the fake. `CronJobSpec.Suspend` is a
    `*bool`, so a resumed CronJob really does carry `suspend: false`. Both mean the same
    thing to the controller; assert the *meaning* (read it typed) rather than the
    key's presence.
  - **A rollout restart is the generation bump, not the annotation** (M1-INT-c-3).
    Patching `spec.template.metadata.annotations` bumps `metadata.generation`, which is
    what a controller observes; a patch that stamped the same annotation on the object's
    own `metadata` would set an annotation and roll nothing. Assert the bump, and assert
    a pre-existing sibling annotation survives — real pod templates carry
    sidecar-injection and config-hash annotations there.
  - **A PUT is conditional only because the buffer carries the metadata** (M1-INT-c-4).
    The fake dynamic client enforces **no optimistic concurrency at all** — its tracker
    replaces the object whatever `resourceVersion` it is handed — so the guarantee the
    Edit flow rests on (D129/D189) is only observable against a server:
    - **A stale `resourceVersion` is a 409 Conflict** (`KindConflict`); **an absent one
      is a legal unconditional overwrite** that returns 200 and destroys the concurrent
      write. So "the update succeeded" proves nothing about concurrency — the negative
      control (strip the field, watch the race be lost silently) is what does.
    - **`metadata.uid` on an update is a precondition too.** An object deleted while the
      buffer was open is a **Conflict**, not the NotFound you would expect: *"Operation
      cannot be fulfilled … StorageError: invalid object, Code: 4 … Precondition failed:
      UID in precondition: …, UID in object meta:"*. Remove the uid from the buffer and
      the same request is a plain NotFound (asserted for ConfigMap; a few kinds allow
      create-on-update, so do not generalize "a PUT never creates" without checking).
    - **`status` is a subresource, so an edited status is silently discarded** while the
      spec in the *same* PUT lands, 200 either way. The fake has no subresources and
      would store both, letting a hermetic test "prove" a status edit worked.
    - **An immutable field is a 422** (`KindInvalid`): editing a Deployment's
      `spec.selector` is refused whole-object, unlike a merge patch of two keys, because
      a PUT is validated as an entire object.
  - **It runs in CI** as of M1-INT-d (D190): `.github/workflows/envtest.yml`, its own
    check, ubuntu-only, ~2 min including the download. Two things to know before
    touching that wiring:
    - **A skipped gated suite is a green suite.** `go test ./internal/kube/...` with
      `KUBECOM_TEST_ENVTEST` unset exits 0 with every `TestEnvtest*` skipped, so the
      whole job can silently stop testing anything. `TestEnvtestSuiteRunsInCI`
      (hermetic, runs in `make check`) is the guard: gate constant ↔ Makefile recipe ↔
      a workflow that runs `make test-envtest` on push, and not ci.yml.
    - **`setup-envtest` lives wherever `go install` put it**, which is on `PATH` only if
      `$(go env GOPATH)/bin` is — a sandbox can have the *assets* on disk and no tool on
      `PATH`. `make test-envtest` now honours a preset `KUBEBUILDER_ASSETS`, falls back to
      `$(go env GOPATH)/bin/setup-envtest`, and fails with instructions instead of feeding
      an empty command substitution into the environment (which surfaced as an
      unrelated-looking `env.Start()` error).
  - **Parsing a workflow file in a guard test: the `on:` key is not `"on"`.**
    `sigs.k8s.io/yaml` converts YAML→JSON with YAML 1.1 semantics, where the bare word
    `on` is the **boolean true** — so a GitHub workflow's trigger block arrives under the
    key `"true"` (`json:"true"` on the struct field). Every other key is literal. Two
    guards read workflows this way (`TestReleaseWorkflowPinsGoreleaser`,
    `TestEnvtestSuiteRunsInCI`); a third that silently found no `"on"` key would pass
    vacuously.
  - **One plane per test function is the rule, not one per assertion** (M1-INT-c-1).
    `startControlPlane`'s per-test isolation exists for tests that wreck cluster-global
    state (an APIService, cluster RBAC) or need the plane configured; an action test
    only creates and destroys objects, so it can run its cases as `t.Run` subtests over
    one plane with distinct object names — ~5 s total instead of ~5 s each. Take the
    `Resource` the actions address from a real `discoverFresh` pass rather than building
    one inline: it is the value the menu hands the action at runtime, so a wrong GVR or
    `Namespaced` flag fails the test instead of being papered over by the stand-in.
  - **A restricted user** comes from `env.AddUser(envtest.User{Name, Groups}, nil)`
    → `user.Config()`, with the grant written as ordinary ClusterRole +
    ClusterRoleBinding through the admin clientset. The RBAC authorizer reads
    through an informer, so poll the *allowed* call until it succeeds before
    asserting the denied one. Discovery is unaffected by RBAC (`system:discovery` is
    bound to `system:authenticated`), which is the point: a denied kind stays on the
    menu and fails per call with `KindForbidden`.
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
