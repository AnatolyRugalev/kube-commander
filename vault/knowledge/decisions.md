# Decision Log

Append-only. Supersede rather than delete; note the date. Newest at the bottom.

---

_Compressed 2026-10-06 (D291): each decision's binding statement and the lead of every rule; the argumentation lives in git history._

### D1 — TUI framework: Bubble Tea
**2026-07-18.** Use **Bubble Tea + Bubbles + Lipgloss** (Elm architecture).

### D2 — In-process client-go, shell out only for interactivity
**2026-07-18.** Reach for client-go first; shell out **only** for exec shell and `$EDITOR`.

### D3 — Config: plain YAML
**2026-07-18.** Drop the protobuf config (`pb/config.proto` + generated code) for a plain typed Go struct marshalled to YAML. **Why:** the protobuf machinery and its abandoned theme engine were over-engineered for a small local file.

### D4 — Name: `kubecom`, single binary
**2026-07-18.** Standardize on `kubecom`; retire the duplicate `kube-commander` binary. **Why:** the original shipped two identical entrypoints and inconsistent naming.

### D5 — Kubernetes/client-go version
**2026-07-18.** Build against a recent client-go (**target v0.31 / K8s 1.31**); support servers **~1.27+** via discovery so it degrades gracefully. "Recent, not too aggressive."

### D6 — Backwards compatibility: clean break + migration
**2026-07-18.** No runtime BC with the old app; instead a one-shot **migration** of the old `~/.kubecom.yaml` on first start. **Why:** clean architecture beats carrying legacy config semantics.

### D7 — Platforms: Linux + macOS only (WSL2 for Windows)
**2026-07-18.** Drop native Windows support; recommend **WSL2**. **Why:** removes Windows PTY/exec complexity (a flagged risk) for a niche of the user base.

### D8 — Async, cached resource discovery
**2026-07-18.** First-paint from a seed set of core GVKs; run full discovery in the background and reconcile the menu via a "discovery ready" message; isolate per-group failures; cache discovery on disk.

### D9 — Branch model
**2026-07-18.** `master` = original code, untouched for now. `v1` = rewrite branch, holds this vault and all rewrite work. A `main` branch becomes the final destination when the rewrite is ready to be default. Push `v1` to origin.

### D10 — Vim-style navigation first-class; arrows/classic as fallback
**2026-07-18.** Navigation is **vim-first**: `hjkl`, `gg`/`G`, `Ctrl+u`/`Ctrl+d`, `/` search + `n`/`N`, `l`/`Enter` to drill in, `h`/`Esc` to go back.

### D11 — Fully configurable keybindings; zero hard-coded keys
**2026-07-18.** **No key literal is ever matched in view/update code.** Every user-triggerable behavior is a named **Action**; an **action registry** maps `Action → []key` and is the *only* place keys exist.

### D12 — golangci-lint scoped to new code only
**2026-07-18.** `.golangci.yml` lints **only** the rewrite (`cmd/kubecom`, `internal/...`); the legacy 2020 trees (`app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`) are excluded.

### D13 — `kubecom` binary is the new skeleton; legacy reachable via `kube-commander`
**2026-07-18.** `cmd/kubecom` now builds the **new** binary (M0 skeleton: `version`/`help`); the legacy 2020 app stays reachable **only** via `cmd/kube-commander` until it is ported (M1–M3), after which M0-03 deletes it.

### D14 — Legacy trees are deleted from `v1` up-front, not kept compiling
**2026-07-18.** Maintainer-approved (setup review).

### D15 — Journal format: one file per entry; no Commit field; milestones kept current
**2026-07-18.** Maintainer-approved. The journal is a directory, `vault/journal/`, with one file per leg named `YYYY-MM-DD.N.md` (`N` = sequence within the day; filename sort == chronological order).

### D16 — Board claims are committed and pushed immediately
**2026-07-18.** Maintainer-approved. Claiming a task (step 3 of the leg loop) is its own commit (`chore(board): claim <leg-id>`) pushed to `v1` **before** implementation starts.

### D17 — `make check` is the canonical gate; CI lands early in M0
**2026-07-18.** Maintainer-approved. A `Makefile` with `check` (= build + test + vet + lint) is the single verify gate used by agents and CI, so "green" means the same thing everywhere.

### D18 — Tests: fake clients by default; envtest opt-in, deferred to M1
**2026-07-18.** Maintainer-approved. Kube-layer tests use client-go **fake clients** (incl.

### D19 — Bubble Tea v2
**2026-07-18.** Maintainer-approved. Prefer **bubbletea v2** (with matching bubbles/lipgloss releases) when dependencies land (M0-02/M2).

### D20 — Config path: `os.UserConfigDir()/kubecom/config.yaml`
**2026-07-18.** Maintainer-approved. The config lives at `os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on Linux, `~/Library/Application Support/kubecom/config.yaml` on macOS) — not inside `~/.kube/`, which other tooling treats as kubeconfig-shaped.

### D21 — Scheduled runs batch legs via fresh subagents (`/do-rewrite-run`)
**2026-07-18.** Maintainer-approved. The scheduled routine invokes **`/do-rewrite-run`**, an orchestrator that sequentially spawns a **fresh subagent per leg**, each executing exactly one `/do-rewrite-leg`.

### D22 — Executing D14: legacy trees gone; `.goreleaser.yml` patched to keep working, not redesigned
**2026-07-18.** M0-07 deleted `app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`, `.travis.yml`, and the snap CI files (`ci/snap-deps.sh`, `ci/snap.login.enc`); pruned `go.mod`/`go.sum` to empty via `go mod tidy` (no external import remains until M0-02/M1 reintroduce cobra/client-go); and dropped…

### D23 — Executing M0-02: Go 1.23 floor, cobra v1.10.2 root command
**2026-07-18.** Toolchain bump landed (M0-02): `go.mod` `go` directive set to **1.23** (the stack floor, not the local 1.24.x — a `go 1.23` module still builds on newer toolchains), and `cmd/kubecom` rewired from the hand-rolled `run(out, args)` dispatch onto **cobra v1.10.2** (D13's planned replacement).

### D24 — CI runs `make check` verbatim; golangci-lint installed, not action-run
**2026-07-18.** M0-04 landed `.github/workflows/ci.yml`: a single `check` job on a `[ubuntu-latest, macos-latest]` matrix (D7) that runs **`make check`** as one step — the same gate agents run locally (D17), so "green" is identical in both places.

### D25 — Executing M0-08: legacy-file sweep; `.goreleaser.yml` de-referenced from deleted `ci/aur/`
**2026-07-18.** M0-08 deleted the legacy files M0-07's tree-based deletion missed: `Dockerfile` (golang:1.15 + baked-in kubectl, contradicts D2), `get.sh`, `ci/aur/` (old binary names `kube-commander`/`kubectl-ui`, `PKGBUILD`/`.SRCINFO` templates, `publish.sh`, and the encrypted deploy key `id_rsa.enc`), and…

### D26 — Executing M0-05: bubbletea v2 adopted; Go floor bumped to 1.24.2; placeholder root model
**2026-07-18.** M0-05 landed the teatest smoke harness, which required the first real TUI dependencies.
**Refs:** supersedes the go-directive part of D23.

### D27 — Executing M0-06: goreleaser skeleton is artifact-only; publishers deferred to M5; Linux+macOS × amd64+arm64
**2026-07-18.** M0-06 reshaped `.goreleaser.yml` into the release **skeleton** D22 and D25 deferred to this leg.

### D28 — Executing M1-00: envtest harness lands the kube dependency graph; pinned client-go v0.31 / controller-runtime v0.19
**2026-07-18.** M1-00 added the opt-in envtest integration harness (D18) and, with it, the first client-go dependencies (the kube layer's foundation).

### D31 — Executing M1-03: async full discovery delivers a one-shot reconcile signal; per-group failures isolated
**2026-07-18.** M1-03 landed `internal/kube/discovery.go`: the background-discovery half of D8.

### D30 — Executing M1-02: static seed RESTMapper composed ahead of discovery
**2026-07-18.** M1-02 landed `internal/kube/seed.go`: a static `meta.DefaultRESTMapper` seeded with ~28 core, high-traffic GVKs (core/v1, apps/v1, batch/v1, networking.k8s.io/v1, rbac.../v1, storage.k8s.io/v1) and composed **ahead of** the deferred discovery mapper from D29 via…

### D29 — Executing M1-01: client bootstrap shape; deferred RESTMapper; no network at construction
**2026-07-18.** M1-01 landed `internal/kube/client.go`: the client-go bootstrap the whole kube layer builds on.

### D32 — Executing M1-04: on-disk cached discovery via kubectl's diskcached client; per-host dir; TTL + explicit Invalidate; memory fallback
**2026-07-18.** M1-04 landed `internal/kube/cache.go` and rewired `NewClients` (client.go): the discovery client is now `k8s.io/client-go/discovery/cached/disk`'s `CachedDiscoveryClient` — the same on-disk cache kubectl uses — completing the "cache discovery on disk" half of D8.
**Refs:** replaces the M1-02/D29.

### D33 — Executing M1-05a: server-side Table List via a per-GroupVersion REST client (dynamic can't negotiate Table per call); M1-05 split into List + Watch
**2026-07-18.** M1-05 ("server-side Table List+Watch → event channel; reconnect/resync") was too big for one ≤300-line green leg, so it was split: **M1-05a = List** (this leg), **M1-05b = Watch** (event channel + reconnect/resync, back to Backlog).

### D34 — Executing M1-05b: reconnecting List→Watch driver streaming Table deltas on a channel; RESET-on-resync
**2026-07-18.** M1-05b landed `internal/kube/watch.go` — `Clients.Watch(ctx, r, ns, opts) (<-chan WatchEvent, error)` starts a background goroutine that streams server-side Table deltas to a bounded channel.

### D35 — Executing M1-06a: generic delete via the dynamic client; UID precondition guards the row-snapshot race; M1-06 split into 06a–06d
**2026-07-18.** M1-06 ("Actions: delete/scale/rollout-restart/cordon/drain/ cronjob-suspend") is six actions — too big for one green ≤300-line leg — so it was split (cf.

### D36 — Executing M1-06b: scale + rollout-restart as generic merge patches through the dynamic client
**2026-07-18.** Second slice of the M1-06 action set (after 06a delete, D35), same generic-dynamic-client posture (D2, D35).

### D37 — Executing M1-06c: cordon/uncordon as a generic `spec.unschedulable` merge patch; M1-06c narrowed, drain split to M1-06e
**2026-07-18.** Third slice of the M1-06 action set (after 06a delete D35, 06b scale/restart D36).

### D38 — Executing M1-06d: cronjob suspend/resume as a generic `spec.suspend` merge patch
**2026-07-19.** Fourth slice of the M1-06 action set (after 06a delete D35, 06b scale/restart D36, 06c cordon/uncordon D37); 06e drain remains.

### D39 — Executing M1-06e-1: drain pod selection (typed clientset + pure classifier); M1-06e split into 06e-1 selection + 06e-2 eviction
**2026-07-19.** Fifth slice of the M1-06 action set (06a delete D35, 06b scale/restart D36, 06c cordon/uncordon D37, 06d suspend/resume D38).

### D40 — M1-06e-2: drain eviction loop (policy/v1 Eviction API, PDB-aware 429-retry, wait-for-deletion); candidates-before-cordon ordering
**2026-07-19.** Sixth and final slice of the M1-06 action set, completing drain (06e-1 selection D39 + this eviction loop).

### D41 — Executing M1-07a: get-object-as-YAML via the dynamic client; managedFields stripped; M1-07 split into 07a/07b/07c
**2026-07-19.** M1-07 ("streaming: logs; describe; get-as-YAML") is three distinct viewers — too big for one ≤300-line green leg — so it was split (cf.

### D42 — Executing M1-07b: describe via kubectl/pkg/describe; RESTMapping built from the discovery Resource
**2026-07-19.** Landed `Clients.Describe(r, ref)` + the pure `describerFor` / `restMappingFor` helpers in `internal/kube/describe.go` — the second in-process viewer (D2: no external pager, no kubectl binary).

### D43 — Executing M1-07c: streaming pod logs via the typed clientset GetLogs subresource; single connection, reconnect split to M1-07d
**2026-07-19.** Landed `Clients.Logs(ctx, ref, opts)` + the pure `podLogOptions` mapper and the `streamLogs` line pump in `internal/kube/logs.go` — the third in-process viewer (D2: no external pager, no kubectl binary).

### D44 — Executing M1-07d: reconnecting/resuming follow logs — force wire timestamps, resume by SinceTime, dedup the re-served second
**2026-07-19.** Hardened `Clients.Logs` so a **Follow** stream survives a transient transport drop, extending `internal/kube/logs.go` (the M1-07c single-connection core, D43) into a reconnecting loop à la watch (D34).

### D45 — Executing M1-08: background port-forward over an SPDY dialer; channel-based handle, no mutex-guarded result
**2026-07-19.** Landed `Clients.PortForward(ctx, ref ObjectRef, ports []string) (*PortForward, error)` + the `PortForward` handle in a new `internal/kube/portforward.go` — the in-process equivalent of `kubectl port-forward` (D2: no kubectl binary), completing the M1-06/M1-07 in-process action/viewer set's remaining…

### D46 — Executing M1-09: typed graceful errors as a Classify(err) → ErrorKind taxonomy over the wrapped chain; RESTConfig tags bad-context
**2026-07-19.** Landed `internal/kube/errors.go` — the "graceful, typed errors, never panic on bad ns/context" exit clause (#86, old #55, principle 3).

### D47 — Executing M2-01a: keymap core — Action registry + canonical chord model; M2-01 split into slices
**2026-07-19.** First slice of the M2 action registry (D10/D11). New package `internal/tui/keymap` is the single place keys exist; views will resolve a keypress to a named `Action` and never match a raw key.
- **Canonical `chord` as the join key.**
- **Shift is never an explicit modifier.**
- **Defaults are one data table, collision-free by construction.**
- **`Merge(overrides)` returns a new validated keymap + warnings**
- **Scope / splits.**

### D48 — M2-01b: multi-key sequences + timeout-driven resolution; timer lives in the model, not the keymap
**2026-07-19.** Second slice of the M2 action registry (D10/D11): the vim `gg` → `nav.top` case and the general multi-key mechanism, built on M2-01a's canonical chord model.
- **Sequences unify with single chords.**
- **Stateful matching is a separate `Sequencer`, keymap stays immutable.**
- **The keymap package never runs a timer.**
- **A prefix that is also a complete binding pends, then fires on timeout.**
- **`Keymap.Action(key)` stays**
- **pgdn/pgup stay half-page only.**

### D49 — M2-01c: config `keys:` wiring — plain-YAML in `internal/config`, `Config.Keymap()` resolves, `kubecom keys` prints
**2026-07-19.** The user config gets its first real field and the keymap its first config surface (M2-01c, on M2-01a's `Merge`).
- **`internal/config` is now a real package**
- **`sigs.k8s.io/yaml`, not a new YAML dep.**
- **A missing config file is not an error.**
- **Config path stays D20:**
- **`Config.Keymap()` is where config meets the keymap.**
- **`kubecom keys`**

### D50 — M2-01d: help generated from the registry — bubbles/key.Binding bridge + toggleable overlay; pin bubbles v2.0.0
**2026-07-19.** Fourth slice of the M2 action registry (D10/D11): the help side of "zero hard-coded keys". Help is built **from the resolved keymap**, never from literals, so it can't drift from actual bindings.
- **Registry → bubbles bridge (`internal/tui/keymap/help.go`).**
- **Overlay component (`internal/tui/help`).**
- **Dep pin: `charm.land/bubbles/v2` v2.0.0**
- **Split.**

### D51 — M2-01e: keybindings doc generated from the registry + drift-guarded golden test
**2026-07-19.** The committed keybindings reference is **generated from the default keymap**, not hand-written, closing the "generated keybindings doc" half of D11 (the help overlay was the in-app half, D50).

### D52 — M2 app-shell decomposed into ordered, leg-sized Backlog slices
**2026-07-19.** With the M2-01 action-registry group complete (D47–D51), the rest of M2 was a single prose paragraph on the board — no pickable item for the next agent.

### D53 — TUI msg boundary: one-item channel→msg pumps; errors bridged, discovery kept whole
**2026-07-19 (M2-02).** `internal/tui/msg.go` is the single boundary between the concurrent `kube` layer and the single-threaded Bubble Tea update loop.

### D54 — M2-03: styles package = Theme (named colors) → Styles (derived lipgloss); lipgloss v2 promoted to direct
**2026-07-19 (M2-03).** `internal/tui/styles` is the single source of visual truth: a **`Theme`** (a struct of named, semantic `color.Color` fields — no styling) and a **`Styles`** (the `lipgloss.Style` values every component renders through), with `New(Theme) Styles` the one place a color becomes a style.

### D55 — M2-04: status bar renders purely from props; spinner ticks gated on discovering
**2026-07-19 (M2-04).** `internal/tui/components/statusbar` is kubecom's bottom bar: `context · namespace · [spinner] discovering…` on the left, the keymap short-help hint right-aligned.

### D56 — M2-05a: resource menu owns its "resource selected" message (emitter owns the type)
**2026-07-19 (M2-05a).** `internal/tui/components/menu` is the browse view's left pane: a static-seeded vertical list of resource kinds, navigated through keymap actions, that emits a selection message when the user drills in.

### D57 — M2-05b: menu reconcile merges into the ordered seed; append extras, never blank
**2026-07-19 (M2-05b).** `(*Model).Reconcile(kube.DiscoveryResult)` folds the async discovery result into the M2-05a static seed.

### D58 — M2-06a: custom table renders a server-printed snapshot, priority-0 columns, clip-not-wrap
**2026-07-20 (M2-06a).** `internal/tui/components/table` is the browse view's right pane: a custom table (bubbles/table is too basic for the live watch/hscroll needs — the M2 risk item) that renders a `kube.Table` snapshot.

### D59 — M2-06b: table applies live watch deltas keyed by object UID, preserving the selection
**2026-07-20 (M2-06b).** `(*table.Model).ApplyEvent(kube.WatchEvent)` folds one live watch delta onto the M2-06a snapshot.

### D60 — M2-06c: table scrolls horizontally on nav.left/nav.right, snapping to column boundaries
**2026-07-20 (M2-06c).** The resource table can now be wider than its pane (the server-printed column set for a resource often overflows a split-pane width).

### D61 — M2-07a: root app model owns the keymap + Sequencer; a generation-tagged timeout tick
**2026-07-20 (M2-07a).** The M0 `internal/tui/tui.go` placeholder is replaced by the real root model in `internal/tui/app.go` — the M2 app shell, built up across M2-07a..d.

### D62 — Two-pane browse layout: focus switching via nav.left/nav.right
**2026-07-20.** M2-07b composes the root model's browse view: the M2-05 resource menu (left pane) and the M2-06 resource table (right pane) side by side under the M2-04 status bar (bottom line).

### D63 — M2-07c: drilling into a resource starts a live kube.Watch, streamed into the table
**2026-07-20.** M2-07c wires the menu's `ResourceSelectedMsg` (drill-in) to a live `kube.Watch`, feeding its deltas into the table through the M2-02 watch pump.

### D64 — M2-07d: async discovery on Init reconciles the menu + drives the status-bar spinner
**2026-07-20.** M2-07d kicks off the async discovery pass on startup and folds its result into the resource menu (M2-05b `Reconcile`), running the M2-04 status-bar spinner while it is in flight — the "discovery ready" reconcile of D8, now wired into the shell.

### D65 — M2-08a: generic modal picker over bubbles/list, driven by keymap actions
**2026-07-20.** M2-08 (the namespace switcher) is too large for one leg, so it is split: **08a** is the picker component in isolation, **08b** adds filtering, **08c** wires it into the app shell (a `ns.switch` action + a `NamespaceLister` seam that re-scopes the watch).

### D66 — M1 completion bar: fake-client coverage; envtest integration tests deferred
**2026-07-20.** Maintainer-approved (progress review).

### D67 — Vault hygiene: decisions are constraints, not changelog; status lines stay terse
**2026-07-20.** Maintainer-approved (progress review).

### D68 — Runnable + dogfooded: land the launch leg, then keep it launchable; README stays current
**2026-07-20.** Maintainer-directed (progress review).

### D69 — Feedback inbox: `vault/feedback/`, drained before every leg, deleted once addressed
**2026-07-20.** Maintainer-directed. A human → agent inbox lives at `vault/feedback/` (format mirrors the journal: one markdown file per item, `YYYY-MM-DD-slug.md`, title + optional Priority/Area + free-form prose; `README.md` is the only non-item file).

### D70 — bubbletea v2 full-screen is a `View` property, not a program option
**2026-07-20.** M2-RUN wired `tea.NewProgram`. The board's sketch called `tea.NewProgram(model, tea.WithAltScreen())`, but **`tea.WithAltScreen()` does not exist in bubbletea v2** (v2.0.2) — it was a v1 program option.

### D71 — TUI logging goes to a file with klog fully off stderr (raise the stderr threshold)
**2026-07-20.** The `stack.md` rule "nothing may write to stdout/stderr while the TUI owns the terminal" needs an explicit klog step: **`klog.LogToStderr(false)` is not sufficient**, because klog copies every ERROR-level line to stderr regardless whenever the line's severity meets the stderr *threshold* (default…

### D72 — M2-08b: picker filtering is picker-owned (its own textinput), with a control/text key split
**2026-07-20.** The modal picker filters itself: it holds its own `bubbles/textinput` over the unfiltered value set and narrows the visible list by **case-insensitive substring** — the list's native filter stays disabled (D65).

### D73 — M2-08c: namespace switch is a `ns.switch` action + a `NamespaceLister` seam; picker re-scopes the current watch
**2026-07-20.** The namespace switcher is wired into the app shell as an action, a seam, and a re-scope, each a constraint future legs (context/container/port pickers, config persistence) build on: - **`ns.switch` action, default `ctrl+n`.** A new `ns` action namespace (its own help column / doc section).

### D74 — Errors surface only inside the fixed layout: a transient, single-line status-bar toast
**2026-07-20.** An `ErrorMsg` (any classified error from an async seam — watch start, namespace list, a watch ERROR bridged by the pump) is surfaced **only** as a transient message in the **status bar**, never printed to stdout/stderr and never rendered into a growing/scrolling pane.

### D75 — README install is local-checkout-only until an M5 release tag; no `@v1` remote form
**2026-07-20.** `go install …/cmd/kubecom@v1` is broken and stays out of the README: `@v1` is a **module version query**, and `v1` matches Go's semver-prefix form (major version 1), so the toolchain resolves it to a `v1.x.x` **tag** (none exist) and never falls back to the branch named `v1`.

### D76 — Right browse pane is a slot: welcome page pre-drill-in, live table after
**2026-07-20.** The right pane of the browse view is a single slot the root model fills conditionally, gated by `m.hasCurrent`: - **Before the first drill-in** (`!hasCurrent`) it renders the `welcome` component (`internal/tui/components/welcome`) — app name/version, `context · namespace` scope, a pick-a-resource hint…

### D77 — Resource menu is grouped into Dashboard-style sections with non-selectable headers
**2026-07-20.** The left resource menu renders as **grouped sections**, not a flat list: `Cluster` → `Workloads` → `Config` → `Network` → `Storage` → `Access Control`, plus a trailing `Custom Resources` section for discovered CRDs/extra groups.

### D78 — Table filter is a view over an authoritative unfiltered row set
**2026-07-20 (M2-09a).** The resource table keeps two row sets: `full` (every row the watch has delivered) and the displayed `table` (full, or full narrowed by the active `filter`).

### D79 — Human-task queue: `vault/human-tasks/` (agent → human), can block the board / a milestone
**2026-07-20.** Maintainer-directed. The inverse of the feedback inbox (D69): a directory where the **agent parks work only a human can do** — dogfood/visual-UX confirmation against a real cluster, credentials/infra it must not fabricate, running envtest locally, irreversible/outward-facing actions (release tag…

### D80 — Table filter wiring: `/` opens a live field, enter commits · esc clears, and n/N step matches with wrap
**2026-07-20 (M2-09b).** The M2-09a filter core is wired into the shell as an input mode, mirroring the namespace picker's control/text routing (D73).

### D81 — M1-04b (lazy group-detail-on-open) retired as obsolete; do not reintroduce a collapsible-group menu for it
**2026-07-20 (M1-04b).** The parked M1-04b item ("fetch a group's full resource detail only when its menu is opened") is **retired won't-do** — it does not fit the realized architecture and its intent is already delivered.
**Refs:** supersedes D77.

### D82 — Menu carries non-resource rows (`Item.Kind`); the namespace picker is a seam row between cluster-scoped and namespaced sections
**2026-07-21 (FB-ns-menu-seam).** The left menu is no longer resource-rows-only.

### D83 — Per-context menu customization lives in its own file per kubeconfig context, under `<configdir>/kubecom/menus/<sanitized-context>.yaml`
**2026-07-21 (FB-menu-config-01, feedback `2026-07-21-02`).** CRDs and other resource types the built-in menu doesn't seed are added via a **dynamic, per-context** menu config — not merged into the single `config.yaml`.

### D84 — A pane's inner text region is `innerW-2`, not `innerW`: lipgloss borders are border-box for width; size content and clip to the real region
`2026-07-21` · feedback `2026-07-21-03` (menu overflow/scroll).
- **A viewport component must size its content lines — and clip long text — to the real inner region `innerW-2`, not `innerW`.**
- **This is a latent hazard in the other bordered components**

### D85 — The persistent bottom key-hint is focus-aware: menu-context vs table-context curated subsets, chosen by which pane holds focus
`2026-07-21` · feedback `2026-07-21-06` (status-bar key hints).
- `keymap.HelpContext` (`HelpMenu`/`HelpTable`) names a **focus context, never a key**; the concrete keys still come from the registry, so a new context is added by…
- **Menu context**
- The root model owns the focus→context mapping in one place (`syncHints`) and calls it wherever focus switches (drill-in, nav.left/right pane switch, esc focus-pop…

### D86 — Mouse is additive and routed through keymap Actions, never raw mouse behaviour in views; enabled per-View via MouseModeCellMotion
`2026-07-21` · feedback `2026-07-21-08` (mouse support). **Partially superseded by D97 (2026-07-22): mouse capture is now off by default and opt-in via `mouse.toggle` — the "additive, routed through Actions" invariant here still holds; only the "enabled per-View unconditionally" clause is replaced.**
- **Mouse is strictly additive**
- **No view matches a raw mouse event for behaviour**
- **Mouse is inert while an overlay is up**
**Refs:** superseded by D97.

### D87 — The persistent key-hint is a dedicated bottom line of its own (the hintbar), not a status-bar segment
`2026-07-21` · board `FB-hintbar-dedicated` (deferred remainder of D85).
- **The status bar no longer lays out a hint.**
- **The hintbar is fed the same registry-generated, focus-aware string**
- **The hint is now always visible**
- The welcome landing page keeps its own in-pane focus-agnostic hint (`welcome.SetShortHelp`), unchanged.

### D88 — The confirm/prompt modal resolves accept/decline through nav.drillIn / nav.back (Enter/Esc), not dedicated y/n actions
M2-10 landed the confirm/prompt overlay (`internal/tui/components/modal`) that replaces the original's racy tcell popup.
- **Accept = `nav.drillIn` (Enter), decline = `nav.back` (Esc).**
- **Two modes on one Model.**
- **Component-only, like the picker was (M2-08a).**

### D89 — `Config.Save`/`SaveFile` are the config write-back primitives; M2-11's menu-customization scope is subsumed by the per-context menu files (D83)
`2026-07-22` (M2-11a). The main config gained write-back to match Load: `Config.Save(io.Writer)` marshals via `sigs.k8s.io/yaml` (round-trips `keys:` today; what Save emits, Load reads back equal), and `Config.SaveFile(path)` persists it — `MkdirAll(dir, 0o700)`, marshal to a temp file in the **same** dir, `Chmod…
- **M2-11 narrows.**
- **Last namespace is per-context.**

### D90 — Per-context runtime state lives in its own `state/<context>.yaml` store, not in config.yaml or the menu file
`2026-07-22` (M2-11b-1). Last-namespace persistence (and future per-context runtime values kubecom records for you) gets a **dedicated per-context state store**: `config.State` in `internal/config/state.go`, persisted to `os.UserConfigDir()/kubecom/state/<sanitized-context>.yaml` (`StateDir`/`StatePath`, reusing…
- **Not `config.yaml`.**
- **Not the per-context menu file.**
- **Config-package only.**

### D91 — Explicit `-n` overrides the stored last-namespace for that run; a namespace picked in the UI is persisted; the flag being *set* is what matters, not its value
`2026-07-22` (M2-11b-2). The initial watch scope is resolved from the `-n`/`--namespace` flag and the per-context state (D90) with this precedence:
- **Explicit `-n` wins for the run.**
- **With no `-n`, the stored last namespace is restored**
- **A namespace picked in the UI is persisted**
- **A malformed/unreadable state file degrades to the zero State with a logged warning**

### D92 — Legacy `~/.kubecom.yaml` migration is detect-and-report, not a field-for-field port: its menu can't be auto-mapped (no version/resource) and its themes have no v1 home
`2026-07-22` (M2-12a). The 2020 `~/.kubecom.yaml` (protobuf-yaml `pb.Config`) held only two user-authored things — a resource `menu` and color `themes`/`currentTheme` — and **neither maps cleanly into kubecom v1**, so `config.Migrate` does **not** attempt a faithful field-for-field port:
- **Menu entries can't be auto-migrated.**
- **Themes are dropped.**
- **Keys never existed in the legacy file**

### D93 — Legacy migration is one-shot, gated on `config.yaml` **absence**; migration notes preempt the single startup-toast slot
`2026-07-22` (M2-12b). The launcher (`cmd/kubecom/run.go` `maybeMigrate`) runs the D92 `config.Migrate` on first start only, and never blocks launch (principle 3):
- **One-shot is gated on the new config's absence, checked with `os.Stat`**
- **Degrade paths write nothing and surface nothing:**
- **The shell has a single startup-toast slot**

### D94 — Table column sort is a view over the authoritative row set (like filter): stable, type-aware only for integer/number, reset on `SetTable`, preserved across watch deltas
`2026-07-22` (M2-13a). The table's column sort is not a mutation of the delivered data — it is a display transformation layered onto the same authoritative `full` row set that the filter narrows, re-derived by `applyFilter` on every change so it survives live updates.
- **Sort follows filter in `applyFilter`.**
- **`sortCol` is a visible-column position (index into `visible`), or -1 for the unsorted watch order.**
- **`SetTable` resets the sort**
- **Type-aware only where cheap: integer/number sort numerically, everything else as case-insensitive text.**

### D95 — Modals composite over the base browse view (a floating popup), never replace it; components return a bare box and the root overlays it
`2026-07-22` (FB-popups-overlay, feedback `2026-07-22-popups-should-overlay`).
- **Modal components return a bare bordered box from `View()`**
- **The root model owns overlaying.**
- **No dimming of the base yet**

### D96 — Status bar sits at the **top**; the target navigation model is an optional/popup menu with a command-palette resource switch (left pane is not a permanent fixture)
`2026-07-22` (FB-status-bar-top, feedback `2026-07-22-status-bar-top-and-optional-left-panel`). Two load-bearing constraints this decision locks, plus a direction the follow-on legs implement:
- **The status bar renders at the top row of the screen**
- **The status bar names the browsed resource type**
- **Direction (not yet built, queued as FB-nav-* board tasks): the left menu is not a permanent fixed pane.**

### D97 — Mouse capture is off by default (native select-to-copy); mouse is opt-in via the `mouse.toggle` keybind. Supersedes D86's unconditional capture
`2026-07-22` (feedback `2026-07-22-text-selection-select-to-copy`).
- **Off by default.**
- **Opt-in via a runtime toggle keybind**
- **The state is visible.**
- **The D86 mouse handlers are unchanged.**

### D98 — Column sort is driven by one cycling key over a stateless derivation of the table's own sort state; there is no separate column-selection gesture
`2026-07-22` (M2-13b). M2-13a delivered `SortBy(visibleCol)`/`ClearSort` as a view over the row set (D94) but no way to reach it. The table has no column cursor, so this leg's wiring had to decide **which column** the sort acts on. Locked choices a later leg must not silently contradict:
- **One key cycles everything.**
- **The cycle is stateless.**
- **Rejected: sort the leftmost-visible column**
- **Header indicator lives in the component.**

### D99 — The left menu pane is toggleable (`menu.toggle`); a hidden menu is zero-width and cannot hold focus. First implemented slice of D96
`2026-07-22` (FB-nav-menu-toggle). D96 recorded that the left pane is not a permanent fixture; this leg makes it hideable and locks the toggle contract the remaining D96 slices (FB-nav-resource-palette, FB-nav-menu-popup) must preserve or supersede:
- **`menu.toggle` (registry action, default `m`, rebindable — D11)**
- **A hidden menu is zero-width everywhere.**
- **Focus follows visibility.**
- **Known gap (by design, this slice):**

### D100 — Resource command palette (`resources.switch`) reuses the generic picker keyed by a distinct Kind; selecting drives `selectResource`. Second slice of D96
`2026-07-22` (FB-nav-resource-palette). D96 named the pane-free, k9s-`:`-style resource switch as the counterpart to the toggleable menu (D99); this leg builds it and locks how it is wired, so FB-nav-menu-popup (which may fold the menu into this palette) and any future picker preserve or supersede the contract:
- **One generic picker component, two instances, disambiguated by `Kind`.**
- **`resources.switch` (registry action, default `:`, rebindable — D11)**
- **The source list is the menu's own item set.**
- **Selecting drives the same `selectResource` path a menu drill-in takes**
- **Routing generalized to "the active picker."**
**Refs:** supersedes D99.

### D101 — FB-nav-menu-popup (menu-as-overlay-popup) folded into the resource palette; retired won't-do-separately. Third slice of D96, resolving the reassess
**2026-07-22 (FB-nav-menu-popup).** D96 triaged the "left pane is not a permanent fixture" direction into three slices and flagged the third — floating the whole sectioned menu as an `overlayCenter` popup — as one to **reassess once the toggle (D99) and palette (D100) landed** (D100's own Consequence: "may promote the…
- **Pane-free resource switching is already the palette.**
- **"Default view = table only" is already the toggle.**
**Refs:** supersedes D99.

### D102 — Board Done entries are one line too (extends D67)
**2026-07-22.** D67 kept the milestone `Status:` and the board `Last updated:` lines terse, but did not name the **Done list**, so legs drifted back to writing a full journal-length paragraph per completed item — the board reswelled from ~17KB to ~50KB (35 of 93 Done entries over 400 chars), and it is read on every…

### D103 — List/watch params encode with metav1.ParameterCodec, not scheme.ParameterCodec
**2026-07-22.** `tableRequest` (shared by List and Watch, `internal/kube/table.go`) must encode `VersionedParams` with **`metav1.ParameterCodec`** (`k8s.io/apimachinery/pkg/apis/meta/v1`), **never** `scheme.ParameterCodec` (`client-go/kubernetes/scheme`).

### D104 — Watch degrades to list-only polling for kinds that can't be watched
**2026-07-22.** `watchLoop` (`internal/kube/watch.go`) must not blank the view or retry-loop when a kind lacks the `watch` verb (e.g.

### D105 — M3 (actions & viewers) decomposed into ordered, leg-sized Backlog slices
**2026-07-22.** With M2's board section down to only blocked/deferred items (M2-14b is M3-gated per D88; M1-INT is deferred envtest), the next milestone M3 was still a single prose paragraph — no pickable leg.

### D106 — All M3 read-only viewers share one `viewer.Model` overlay
**2026-07-22 (M3-01).** The YAML/describe/logs/secret viewers (M3-03…08) each render into the **one** `internal/tui/components/viewer` component, not their own pager.

### D107 — M3 row actions dispatch a typed `rowActionMsg` intent; applicability is a kind-keyed registry
**2026-07-23 (M3-02).** The M3 action surface is split from the individual viewers/actions: this leg lands only **opening the actions menu and routing**, no action behaviour.

### D108 — M3 viewer legs wire through a narrow kube getter seam, fetch async with a generation guard, and capture input while open
**2026-07-23 (M3-03).** The first viewer (YAML) establishes the pattern every later read-only viewer leg (describe M3-04, logs M3-05…, secret M3-08) follows so they do not each invent their own wiring: (1) the kube call is reached through a **narrow single-method seam** on the shell (`YAMLGetter`, wired with…

### D109 — Streaming viewer legs pump a kube channel line-by-line into the shared viewer via a gen-tagged pump, cancel on close/supersede, and append preserving scroll
**2026-07-23 (M3-05).** The logs viewer is the first *streaming* viewer, so it extends D108's one-shot-fetch shape with the channel→msg pump rhythm (M2-02/D53) that M3-06 (follow) and M3-07 (container picker / pod-owning kinds) build on.

### D110 — The logs viewer opens in follow mode (streaming + auto-scroll); `logs.follow` (`f`) toggles it and a manual up-scroll pauses it
**2026-07-23 (M3-06).** Building on D109, the logs viewer opens **following**: the stream is opened with `kube.LogOptions{Follow:true}` (M1-07d — the stream stays open and reconnects transparently across transport drops rather than ending at EOF), and while following each appended line snaps the viewport to the bottom…

### D111 — Opening logs on a multi-container pod resolves the pod's containers first and prompts which to stream; a single-container pod streams directly
**2026-07-23 (M3-07a).** `kubectl logs` requires `-c` to disambiguate a multi-container pod (the API server errors on an empty container name when a pod has more than one), so the logs viewer can no longer stream blindly.

### D112 — Logs on a pod-owning workload kind resolve a backing pod (selector → newest ready pod), then take the pod path
**2026-07-23 (M3-07b, #84).** Logs are offered for pod-owning kinds (Deployment/ReplicaSet/StatefulSet/DaemonSet/Job/ReplicationController), not only pods.

### D113 — The secret viewer opens masked; values are revealed only by the deliberate `secret.reveal` (`r`) gesture; decoding uses the typed clientset
**2026-07-23 (M3-08a, #89).** The Secret viewer is the first read-only viewer that transforms content (decode + mask) rather than showing it verbatim.

### D114 — The secret viewer has an entry cursor (nav.up/down select, not scroll); `secret.copy` (`c`) yanks the selected value to the clipboard via bubbletea's OSC-52, masked or revealed
**2026-07-23 (M3-08b, #89).** Copy completes D113's Secret viewer and ticks the M3 secret exit criterion.

### D115 — Delete wired through the confirm modal: root owns one `modal.Model` captured in `handleAction`, `ConfirmedMsg` routes by Kind, result to the status bar, no raw y/n
**2026-07-23 (M3-09).** First confirm wiring — D88's "the M3 action that needs it wires the modal into the shell" — and the pattern the remaining mutating actions (M3-10 scale/rollout, M3-11 cordon/drain, M3-12 suspend/resume) must follow.

### D116 — Full-program teatest of an async modal resolve syncs on a side-effect signal, never Quit-ordering
**2026-07-23 (M2-14b).** The confirm modal accepts/declines through an **async round-trip** (KeyMsg → `ConfirmedMsg`/`CancelledMsg` cmd → the root hides the modal and, on accept, runs the action off the update loop — D115).

### D117 — Prompt-mode modal input routes through routeModalPromptKey; mutating actions share one target stash keyed by modal Kind
**2026-07-23 (M3-10).** Scale is the first action to use the modal's **prompt mode** (a replica count), so it extends D115's confirm wiring with the text-entry rhythm the picker/filter already use (D73).

### D118 — k9s is framed as a contemporary/peer of kube-commander, not "prior art"
**2026-07-23 (FB-k9s-not-prior-art).** Both kube-commander and k9s emerged around 2019–2020, so k9s is a **contemporary / kindred** Kubernetes TUI, not a predecessor kube-commander came after or built upon.

### D119 — Menu does not eagerly count resource types; empty-type graying is declined for now
**2026-07-23 (FB-gray-out-empty-types).** Triaging the soft/low idea of graying left-menu resource types that have zero objects in the current view.

### D120 — Idempotent mutating actions dispatch directly (no confirm modal); cordon/uncordon set the pattern
**2026-07-23 (M3-11a).** Cordon/uncordon are the first mutating actions wired **without** a confirm modal: cordoning is idempotent (a merge patch of `spec.unschedulable`, M1-06c — re-running it is a no-op, no UID guard, D35), so a yes/no gate would be friction with no safety value.

### D121 — Drain policy default: IgnoreDaemonSets on, Force/DeleteEmptyDirData off (refuse over silent data loss); progress streams like the log pump
**2026-07-23 (M3-11b).** The Drain action (unlike cordon/uncordon, which are idempotent and dispatch directly, D120) evicts pods, so it is **confirm-gated** (D115) — the second shape of the M3 mutating split.

### D122 — Port-forward is a tracked background handle observed via messages, not a stream pump; started behind a ports prompt, Pod-only for now, cancel-all-on-exit
**2026-07-23 (M3-13a).** The Port-forward action starts M1-08's `kube.PortForward` (SPDY to the pod's portforward subresource, no kubectl binary, D2) as a **long-lived background handle** the shell tracks — a different shape from both the one-shot mutating actions (D120) and the drain's step-by-step pump (D121).

### D123 — Port-forwarding a Service resolves it to a backing endpoint pod first (ServiceResolver seam), mirroring the logs PodResolver hop
**2026-07-24.** A **Service cannot be port-forwarded directly** — `kube.PortForward` POSTs to the pod `portforward` subresource (D122/M1-08), which a Service does not have.

### D124 — In-process exec is a blocking `Clients.Exec` over the pod exec subresource (SPDY remotecommand), apimachinery-free, driven by the TUI via `tea.Exec` off the update loop
**Date:** 2026-07-24 · M3-14a.
1. **It is a blocking call, not a stream pump or a background handle.**
2. **Apimachinery-free boundary (D33).**
3. **TTY folds stderr into stdout.**
4. **Injectable executor factory**

### D125 — The exec TUI wire runs `kube.Exec` inside a `tea.Exec` `ExecCommand` that puts the local terminal raw itself; the exec size queue delivers one seeded size then session-end
**Date:** 2026-07-24 · M3-14b-1.
1. **The ExecCommand owns raw mode, not bubbletea.**
2. **`SetStderr` is a no-op on the wire.**
3. **The size queue seeds the initial size once, then blocks until session-end.**
4. **This slice execs the pod's *default* container with `/bin/sh`.**

### D126 — The container-resolution path is purpose-tagged (logs ↔ exec) and routes the resolved container via `streamOrExec`; the shared `ctrPicker` is disambiguated by `ctrPurpose`
**Date:** 2026-07-24 · M3-14b-2.
1. **One picker, purpose-routed.**
2. **Exec goes through the same fast-path/pick split.**
3. **`viewerGen` guards the exec fetch too.**

### D127 — Live exec terminal resize: a SIGWINCH watcher pushes the current size into the exec size queue, which is now a latest-wins one-slot channel
**Date:** 2026-07-24 · M3-14b-3.
1. **The size queue is latest-wins, never lossy-blocking.**
2. **The watcher is stopped before the queue is closed.**
3. **SIGWINCH resize is Linux/macOS only**

### D128 — Exec prefers `kubectl exec` when the binary is on PATH (parity escape hatch); the in-process SPDY path is the fallback that keeps exec working without kubectl
**Date:** 2026-07-24 · M3-14b-4.
1. **Prefer kubectl when present; SPDY is the fallback, not the primary.**
2. **The shelled-out kubectl must target the same cluster kubecom launched with.**
3. **The kubectl lookup is a seam (`lookupKubectl`, a package var).**
4. **This does not close the M3 exec exit criterion.**

### D129 — Edit applies via a client-side Update (PUT), not server-side apply
**2026-07-24.** The Edit action (M3-15) writes the edited object back with `Clients.Update` (`internal/kube/apply.go`): parse the edited YAML → unstructured → dynamic-client **Update** (PUT) of the full object.

### D130 — Port-forward: bind failures are actionable, not raw; `:0`/`:remote` is the free-local-port escape
**2026-07-24.** A local-listener bind failure (client-go's `unable to listen on any of the requested ports: [{6379 6379}]`) is **not surfaced raw**.

### D131 — Cluster search: one-shot concurrent fan-out over List, curated-scope default
**2026-07-24.** Cross-object **cluster search** (feedback `2026-07-24-cluster-search-multi-resource`: type a query → matching objects across kinds, not a within-table filter).

### D132 — Key contexts: the confirm modal resolves `y`/`n` in its own key context; supersedes the "no y/n" of D88/D115
**2026-07-24** (feedback `2026-07-24-confirm-modal-yn-keys`).

### D133 — Default row-action keys: delete is `d`, describe relocates to `D`
**2026-07-24** (feedback `2026-07-24-delete-default-key-d`).

### D134 — Logs get a dedicated full-screen logs view (`logsview`) with a live filter, off the shared read-only viewer
**2026-07-24** (feedback `2026-07-24-logs-dedicated-view-live-grep`).

### D135 — View YAML and Edit unify into one object-YAML action ($EDITOR edit-in-place); the standalone read-only YAML viewer is retired
**2026-07-24** (feedback `2026-07-24-unify-yaml-view-and-edit`).

### D136 — M3-15c resolves D135: the unified View/Edit YAML action keeps `e`, gates on `canGet`, and `y` is retired
**2026-07-24** (M3-15c, completing D135). Two choices D135 left open, now settled:
1. **The surviving key is `e` (`res.edit`); `y` (`res.yaml`) is removed and left unbound in the browse context.**
2. **The action's applicability predicate is `canGet`, not `update`/`patch`.**

### D137 — Port-forward port discovery: declared ports only, TCP only, Service ports resolve to the pod side
**2026-07-24** (FB-pf-port-picker-a). The port-forward flow is moving from free-text remote entry to a **picker of known ports** (feedback `2026-07-24-port-forward-picker-and-local-port` part 1).

### D138 — Port-forward port picker: a pick is a whole spec (local = remote), and the picker never gates the action
**2026-07-24** (FB-pf-port-picker-b). The TUI wire over D137: the Port-forward action lists the target's declared ports through a `PortLister` seam (`kube.PodPorts`/`ServicePorts`) before deciding what to open.

### D139 — Port-forward local port: every declared port goes through the picker, whose two gestures own the local side; `:0` is not a valid spec
**2026-07-24** (FB-pf-local-port). The last slice of the port-forward feedback (`2026-07-24-port-forward-picker-and-local-port` part 2): the local end of a forward is now choosable.
**Refs:** supersedes D138 pt 2.

### D140 — The cluster-search view owns the query; hits stream into it and never move the cursor
**2026-07-24** (SEARCH-02a). The search mini-app's component half (`internal/tui/components/searchview`), split component-first from SEARCH-02 in the D52 rhythm (LOGS-01 → LOGS-02).

### D141 — Cluster search is a full-screen mini-app on `ctrl+s`: debounced launch, generation-guarded hits, drill-in via a pending selection
**2026-07-25** (SEARCH-02b). The app wiring of the cluster search — the half D140's view deliberately left out (`internal/tui/search.go`).

### D142 — `kube.Search` streams typed `SearchEvent`s: one kind-done per requested kind, a terminal done that names the cap, and the close as the only teardown
**2026-07-25** (SEARCH-03a). A bare hit channel could not express *how far along* a search was or *why it stopped* — the close meant "finished", "capped", and "cancelled" alike (D131 pt 4). The channel item is therefore widened once, and these are the guarantees a consumer may build on:
1. **Three event types, one channel.**
2. **Exactly one `SearchKindDone` per resource passed in**
3. **`SearchDone{Capped}` is the only truthful cap signal.**
4. **The channel close remains the single teardown point.**
5. **Cancelled ⇒ silent.**

### D143 — A focus context's key hint may only advertise keys that context actually honours; a full-screen view gets its own `HelpContext`
**2026-07-25** (SEARCH-03b). The bottom hint is registry-generated (D11) but the *subset* is chosen per focus context, and until now there were only the two browse contexts (menu / table).
1. **A hint entry is a promise.**
2. **A view that captures all input owns a `HelpContext`.**
3. **`syncHints` is called wherever input ownership moves**

### D144 — Streaming content gets a dedicated full-screen view; the shared viewer is for one-shot content only
**2026-07-25** (LOGS-02). Logs left the shared read-only viewer (M3-01) for the LOGS-01 component, completing the split D134 asked for.
1. **The shared viewer serves one-shot content only**
2. **State the view renders is the view's, not the shell's.**
3. **`viewerGen` stays the one "an async open was superseded" clock**
4. **A full-screen view with a text field needs two hint contexts**

### D145 — A live filter degrades to its last working pattern, and a match is shown where it was found
**2026-07-25** (LOGS-03). The logs grep gained a second mode: `logs.regex` reads the same field as a case-insensitive regex instead of a case-insensitive substring, and either way the matched spans are highlighted in the shown lines.
1. **A filter that cannot be parsed keeps narrowing by the last one that could**
2. **A mode toggle for a text field must be bound to a chord carrying no text.**
3. **The unfiltered render path stays free of match work.**
4. **Highlighting is a `styles.Match` role, not a per-view colour.**

### D146 — A pager clips long lines by default; wrapping is an opt-in mode that owns the horizontal offset
**2026-07-28** (LOGS-04a). The logs view learned what to do with a line wider than the screen: `logs.wrap` (`w`) switches between soft-wrapped continuation rows and clipping, and while clipping, `nav.left`/`nav.right` scroll the view horizontally.
1. **Clipping is the default in a streaming view.**
2. **Wrap and horizontal scroll are one toggle, not two settings.**
3. **A mode-dependent binding is not hinted.**
4. **Display state the reader can lose sight of is named in the header.**

### D147 — In a streaming view, an explicit jump to the end rejoins the stream
**2026-07-28** (LOGS-04c). `nav.bottom` (`G`) in the logs view now sets following as well as scrolling, making it the single "catch up and keep tailing" gesture.
1. **The end of a live buffer is a state, not a position.**
2. **It is the inverse of the pause rule, and the pair is the whole model.**
3. **Prefer overloading the existing nav key over adding a view-specific one**

### D148 — A stream's optional metadata is fetched always and *displayed* on toggle; the grep matches the message (2026-07-28, LOGS-04b)
`logs.timestamps` (`t`) shows each log line's server timestamp. The stream is opened with `kube.LogOptions.Timestamps` set **unconditionally**, even though the view starts with the stamps hidden, and the toggle only changes whether they are drawn.
1. **Fetch the metadata always; toggle the display.**
2. **Metadata is stored beside the payload, never prefixed into it.**
3. **A filter matches the payload, not the metadata.**
4. **The hint line is full; a self-announcing toggle does not get a slot.**

### D149 — A scope widen is per-visit, announced only when on, and bounded at the source (2026-07-28, SEARCH-04a)
`search.allKinds` (`ctrl+a`) widens a cluster search from the curated kind set to every discovered kind — the opt-in widen D131 pt 2 held back. Four constraints come with it.
1. **An expensive opt-in scope resets on every open, never on a keystroke within one.**
2. **Changing the scope invalidates results exactly as changing the query does.**
3. **Name the widened state, not the default.**
4. **Bound the fan-out where it is issued, not where it is triggered.**

### D150 — Scope is independent axes; a widened scope replaces its default's name, and never mutates the app's own scope (2026-07-28, SEARCH-04b)
`search.allNamespaces` (`ctrl+w`) widens a cluster search to every namespace, completing the scope D131 pt 2 asked for. It inherits D149 whole (per-visit, invalidates results, debounced, hinted) and adds three constraints of its own.
1. **Scope is independent flags, never a cycle.**
2. **A widened scope replaces the name of the default it widens; it never adds a second name for the same thing.**
3. **A per-search scope widen never mutates the app's own scope.**

### D151 — A query line may carry a server-side term; it is parsed as it is typed, and an unusable query is reported, never sent (2026-07-28, SEARCH-04c-1)
Cluster search grew a second matching term: `-l <selector>` in the same query line hands a label selector to `metav1.ListOptions` on every kind's List, alongside (or instead of) the client-side name substring.
1. **A search term goes to the server whenever the server can evaluate it.**
2. **Server-side matching is why field selectors are excluded, not an argument for them.**
3. **A query that cannot be searched is reported where it was typed, and never sent.**
4. **A grammar in a text field discriminates by an explicit token, never by shape.**

### D152 — The stream stays in arrival order and the view does the ranking; a re-ranking list carries the cursor with its row (2026-07-28, SEARCH-04c-2a)
`kube.Search` scores every match (`SearchHit.Score`) but still emits hits in whatever order the kinds return; the *consumer* keeps them sorted. Four constraints.
1. **Ranking never buys itself by buffering.**
2. **A score is an ordering, not a measurement.**
3. **Match bands are ordered by match *kind* first, quality second.**
4. **A list that reorders under the reader moves rows, never the selection.**

### D153 — Fuzzy is a fallback, ranked in its own band and budgeted against the cap (2026-07-28, SEARCH-04c-2b)
`kube.Search`'s name matcher tries a contiguous substring first and only then a subsequence. Three constraints, all of them about keeping a widened matcher from degrading the result it widens.
1. **A widened matcher is a *fallback*, never a replacement.**
2. **Bands may weigh their terms differently, and should.**
3. **An emit-time cap must be budgeted by match quality, not just counted.**

### D154 — A milestone closes on evidence named in the criterion, read against the decisions that narrowed it (2026-07-29, M2-EXIT)
**M2 is feature-complete.** Its five remaining exit criteria were audited against the code and all five hold; each now carries the tests and code paths that prove it, inline in [`../milestones/M2-core-tui.md`](../milestones/M2-core-tui.md). Three constraints follow, and they apply to M3/M4/M5 too.
1. **No criterion is ticked on prose.**
2. **A criterion is read against the decisions that narrowed it, not its original wording.**
3. **A milestone does not stay open for an enhancement its criteria never asked for.**

### D155 — M4 decomposed into leg-sized slices; a context switch is a teardown, not a pointer swap (2026-07-29, M4-PLAN)
M4 was a six-bullet prose scope with no board surface, so there was no pickable item for the next agent.
1. **Switching context is a teardown of the old cluster, not a rebind of a client pointer.**
2. **The cluster-bound seams get one indirection, and every future seam goes through it.**
3. **New data rides the existing paths.**

### D156 — Per-cluster async has one teardown inventory, and cancellation alone never proves a message will not arrive (2026-07-29, M4-03)
`resetCluster` (M4-03) is the first half of a context switch, and building it exposed two constraints that outlive it.
1. **One inventory, two callers.**
2. **Cancel, then guard.**

### D157 — A context switch connects before it tears anything down (2026-07-29, M4-04a)
`switchContext` issues the connect off the update loop and does **nothing else**; the reset-swap-rediscover sequence runs only in `handleClusterConnected`, with the new cluster's seams already in hand.

### D158 — The context picker marks the shell's context, not the kubeconfig's (2026-07-29, M4-04b)
kubecom's context switch is **session-scoped**: it connects a new client and repoints the `Cluster` bundle (D157), and it never writes `current-context` back to the kubeconfig.

### D159 — Every error the user is shown is also written to the log file (2026-07-29, DIAG-01)
The TUI owns the terminal, so a failure has exactly two places it can go: a transient status-bar toast (5s, clipped to the terminal width) and `~/.cache/kubecom/kubecom.log`.
1. **`surfaceError` logs before it toasts.**
2. **What is quiet on screen by design is loud in the log.**
3. **The log is a diagnostic record, not a trace.**
4. **The sink is injected (`tui.WithLogger`), and unset means discard.**

### D160 — The logs view opens on a bounded tail, not the container's whole history (2026-07-29, LOGS-05a)
`kubecom`'s logs view exists to answer "what is this container doing **now**".
1. **Every logs open is bounded.**
2. **The bound is on the *fetch*, not on the buffer.**
3. **`kube.LogOptions` keeps `kubectl`'s semantics: the zero value replays everything.**
4. **Initial-read selectors never survive a reconnect.**

### D161 — A pod's containers are all of them, classified; the picker offers what the purpose can act on (2026-07-29, LOGS-06)
`kube.PodContainers` returned only `spec.containers`, so an init container's logs were unreachable from the TUI — precisely the logs you need when the pod never got as far as its regular containers (feedback `2026-07-29-logs-init-containers`). From here on:
1. **The kube layer returns the whole set, classified.**
2. **The consumer narrows by purpose, and the narrowing is a claim about the container, not about the feature.**
3. **The single-container fast path counts the offered set, not the regular containers.**
4. **A non-regular container is always marked in the UI.**

### D162 — A streamed view's cost is the number of viewport syncs, so pumps that feed one batch (2026-07-29, LOGS-05b)
The logs view got slower the longer it ran: `Append` re-scanned and re-joined the entire buffer for every line, so streaming n lines cost O(n²) (feedback `2026-07-29-logs-tail-and-perf`). Removing our own rescan turned out to be the smaller half. The binding rule for any view fed by a stream:
1. **The rendered body is a cache, extended by an append and rebuilt only by a reader gesture.**
2. **Handing content to the viewport is O(n) and there is no append API.**
3. **Therefore the pump batches.**
4. **A batching pump must carry the stream's end, not drop it.**
5. **The viewport owns any slice handed to `SetContentLines`**

### D163 — Per-context state is re-resolved on a switch, through one launcher seam
**2026-07-29.** A context switch rebinds everything keyed by the *kubeconfig context* — the `menus/<context>.yaml` extras (D83), the last-used namespace and the per-context state file a namespace is persisted to (D90/D91) — not just the cluster client (M4-05, completing D155/D156/D157).
1. **The tui package stays context- and storage-agnostic.**
2. **The load rides the connect's Cmd.**
3. **`LoadContextState` cannot fail.**
4. **Order around the reset is load-bearing.**
5. **`-n` names the launch context's scope, not every context's.**

### D164 — Table cell coloring is keyed off the column *name*, per cell, purely
**2026-07-29.** The browse table paints status-carrying cells with the theme's `Success`/`Warn`/`Error` roles (M4-06). The classifier is a pure function of the column name and the cell text — nothing else — and everything it does not recognise stays ordinary body text.
1. **The column name is the key, not the resource kind.**
2. **The classifier stays per-cell and pure.**
3. **An unrecognised value is uncolored, never guessed at.**
4. **Severity is an ordering, and the most severe part wins.**
5. **Selection wins outright over cell color.**
6. **A styled line is built from complete segments, never nested.**

### D165 — Owner → children is a *scope*, never a fetched list
**2026-07-29.** `kube.Children` (M4-07) answers "what does this object drill into?" with a `ChildScope` — the child `Resource`, a namespace, and a `metav1.ListOptions` — and never with rows.
1. **A scope, not a list.**
2. **The server does the filtering.**
3. **The child kind comes from the caller's available set, matched on `GroupKind`.**
4. **A match-everything selector is refused, not passed through.**
5. **The relation is "related pods", not `ownerReferences`.**
6. **`HasChildren` is a pure predicate.**

### D166 — A drill-down scope is browse state the *watch start* reads, and it is a nav level
**2026-07-29.** M4-08 wires `kube.Children` (D165) to the browse table. The scope is not a second data path and not a mode; it is one more thing `m.current` is qualified by.
1. **The scope is read where the watch is started, never at the call site.**
2. **Both halves of the scope are load-bearing, including the namespace.**
3. **Every direct re-point of the table clears the scope.**
4. **Resolve before switching, degrade in place.**
5. **A drill-down is its own `nav.back` level**

### D167 — Metrics are an optional, join-by-name overlay: availability is a discovery fact and absence is silent (2026-07-29, M4-09)
`internal/kube/metrics.go` reads `metrics.k8s.io/v1beta1` through the ordinary dynamic client. The constraints a future leg must not contradict:
1. **Availability is answered by discovery, not by a probe request.**
2. **The join key is namespace/name, never UID.**
3. **One-shot List, never a watch.**
4. **A sample is whole or absent.**
5. **A failed request is the caller's to log, not to surface.**

### D168 — The metrics overlay is displayed-view state, joined per derivation and never written into watched rows (2026-07-29, M4-10)
The TUI half of the metrics line (`internal/tui/metrics.go`, the poll; `internal/tui/components/table/usage.go`, the columns). What a future leg must not contradict:
1. **Samples live beside the rows, never in them.**
2. **Deriving them there, rather than at render time, is what keeps them ordinary.**
3. **`SetUsage(nil)` means "this cluster does not measure this kind" and a non-nil empty map means "measured, nothing scraped yet".**
4. **The poll is armed from `watchResource`, the single browse-watch start**
5. **A failed refresh keeps the previous samples and is logged, never toasted**

### D169 — A theme name is a persisted identifier, and a built-in theme is complete or it is not built in (2026-07-29, M4-11)
`internal/tui/styles/themes.go` holds the built-in palettes (`MonokaiTheme`, `SolarizedDarkTheme`) and the registry over them (`Themes`, `ThemeNames`, `ByName`). What a future leg must not contradict:
1. **A theme's `Name` is API, not a label.**
2. **Lookup is lenient about formatting, never fuzzy.**
3. **Every built-in sets every `Theme` role.**
4. **`builtins` is the single registry**

### D170 — The palette is resolved by the launcher and fixed at construction (2026-07-29, M4-12a)
`config.yaml`'s `theme:` field now reaches the shell: the launcher resolves the name through `styles.ByName` and passes the palette in as `tui.WithTheme`, and `NewWithKeymap` builds every component from it. What a future leg must not contradict:
1. **`config.Config` carries the theme *name*, a plain string — not a `Theme`.**
2. **Options run before the components are constructed.**
3. **An unknown theme name never fails the launch.**

### D171 — A live restyle is a fan-out every component must join (2026-07-30, M4-12b-1)
D170 fixed the palette at construction, which is enough for a `theme:` config field and not enough for a theme picked from inside kubecom: the components are already built.
1. **A component that caches a `Styles` exposes `SetStyles`, and `applyStyles` calls it.**
2. **`SetStyles` re-derives; it does not merely assign.**
3. **A restyle is colors only, never a reset.**
4. **The two theming paths stay separate.**

### D172 — A theme is user preference, not cluster state: the picker is never inert, the write-back is load-modify-save (2026-07-30, M4-12b-2)
`theme.switch` (`T`) opens a picker over `styles.Themes()`, applies the pick through D171's `applyStyles`, and writes the name back to `config.yaml` through a `tui.ThemePersister` the launcher implements. What a future leg must not contradict:
1. **The gesture works with no seam wired; only *persistence* needs one.**
2. **A config write-back is `LoadFile` → set one field → `SaveFile`.**
3. **No save preserves comments or formatting.**
4. **A theme belongs to the reader's terminal, not to the cluster or the context.**

### D173 — A release leaves the repo and cannot be reverted, so an agent prepares and dry-runs it and a human publishes it (2026-07-30, M5-PLAN)
M5 is decomposed into slices M5-01…M5-11 on the [board](../tasks/board.md). Every milestone before it enjoyed the same safety net — land it, and if it is wrong, revert it. M5 does not have one: its artifacts leave the repository. What a future leg must not contradict:
1. **No agent leg pushes a release tag or performs a distributor's first publish.**
2. **A publisher is inert without its credential, never fatal.**
3. **Install docs describe only paths that actually work, and land with them.**
4. **The credential-free gate for release config is `goreleaser release --snapshot --clean`.**

### D174 — The Definition of Done is audited against evidence, never edited to match what shipped (2026-07-30, M5-01)
M5-01 audited all 13 boxes of the Definition of Done in [`../goals.md`](../goals.md) and ticked 6. The rules it followed are the ones a future audit — M5-10's pre-flight, or whatever closes the remaining boxes — must not silently contradict:
1. **A box is ticked only when its claim is decidable from the code and its tests, or has been confirmed by a human against a real cluster.**
2. **An open bug outranks a green test suite.**
3. **Where the DoD text and a later decision disagree, the divergence is filed, not edited.**
4. **A DoD audit files what it finds and fixes nothing.**

### D175 — A released binary reports complete build metadata, and the release config is gated like code (2026-07-30, M5-02)
1. **Every exported var in `internal/version` must be injected by `.goreleaser.yml`'s ldflags.**
2. **`.goreleaser.yml` tracks the current goreleaser v2 schema, so whatever runs it must be current too.**

### D176 — There is exactly one release entry point, and the tag path is gated by the same gate a branch push is (2026-07-30, M5-03)
M5-03 added `.github/workflows/release.yml`: a `--snapshot --clean` dry run on every push to `v1`/`main` and on PRs, and a real `goreleaser release --clean` on a `v*` tag. What a future leg must not silently contradict:
1. **goreleaser runs in that one workflow, at one pinned version.**
2. **The tag path gates on `make check` by *calling* ci.yml, not by copying it.**
3. **Distribution slices extend the existing pipeline; they never add a second `v*` workflow.**

### D177 — Which log instance you are reading is one bit of one request, toggled inside the view, and the header always names it (2026-07-30, M5-01a)
`kube.LogOptions.Previous` existed since M1-07c but nothing reached it, so the log that explains a `CrashLoopBackOff` — the one belonging to the instance that already died — was unreachable. M5-01a added `logs.previous` (`ctrl+p`). What a future leg must not silently contradict:
1. **`L` stays the only way into logs.**
2. **The flip changes `Previous` and nothing else about the request.**
3. **A restream is not a reset.**
4. **Nothing pre-checks for a previous instance.**
5. **The `[previous]` header marker is not optional, and sits ahead of the follow state.**
**Refs:** superseded by D257.

### D178 — A DoD claim is amended only on the maintainer's own recorded words, and deleted feedback is still that record (2026-07-30, M5-01b)
M5-01b was filed as "a maintainer decision, not a defect": the DoD promised an in-TUI YAML viewer, D135 had retired it, and D174 pt 3 forbade the agent from rewording the promise.
1. **The one exception to D174 pt 3.**
2. **Deleted feedback is evidence, not history that was thrown away.**
3. **The object's YAML has exactly one surface, and it is the editor**

### D179 — Migration carries the theme *name*, never the palette, and a legacy rename is an enumerated alias rather than a guess (2026-07-30, M5-04)
M5-PLAN found `migrationNotes` telling a 2020 user "legacy theme configuration was dropped: kubecom v1 uses a single fixed theme and has no runtime theming" — true under D6 when M2-12a wrote it, false since M4-11/12 shipped three built-in themes, a `theme:` field (D170) and a picker (D172).
1. **The migratable unit is the theme's name, and only its name.**
2. **A legacy rename is an enumerated alias; D169 pt 2 (never fuzzy) is unchanged.**
3. **An empty `currentTheme` is not a selection, even though the 2020 build defaulted it.**
4. **A migration note that describes v1's own capabilities has to be re-read whenever those capabilities change.**

### D180 — A legacy-format fixture is generated from the 2020 writer and pinned to `master:pb/config.proto`, never hand-typed (2026-07-30, M5-05)
Every test of the legacy migration up to this leg fed `Migrate` YAML a test author typed from memory, and one of them was wrong: `rgb: "#000000"`, when `theme.ColorToProto` wrote `fmt.Sprintf("%06x", …)` — a bare hex string, no `#`, with `ProtoToColor` prepending the `#` on read.
1. **The 2020 file's shape is not a matter of opinion — it is `master:pb/config.proto` plus `protojson`.**
2. **`internal/config/testdata/legacy-kubecom.yaml` is generated, not authored, and the proto beside it is a verbatim copy.**
3. **The fixture is bound to the schema in both directions, and that binding is the point.**
4. **A generated fixture is strong evidence about the format and says nothing about a real user's file.**

### D181 — The screencast is a committed script plus a human recording, and its keys are pinned to the keymap (2026-07-30, M5-09)
A demo GIF is documentation that no test can read: it asserts, to the first visitor the project ever gets, that pressing these keys does these things — and it goes on asserting it long after a rebinding makes it false, because a recorder types keys, it never checks them.
1. **The tape is the artifact this repo owns; the GIF is recorded, never fabricated.**
2. **Every keypress in the tape carries a `# kubecom-action:` annotation naming the action it triggers, or `# kubecom-input:` when it is plain text.**
3. **The README references the screencast exactly when the file exists.**
4. **The tour is a claim about what kubecom is for, so it has a floor.**

### D182 — Homebrew ships as a cask, to the 2020 tap, inert without its token (2026-07-30, M5-06)
The first of the three distribution slices. Two of its constraints exist because the obvious config is silently wrong rather than rejected. What a future leg must not silently contradict:
1. **kubecom is distributed as a Homebrew *cask*, not a formula.**
2. **The tap is `AnatolyRugalev/homebrew-kubecom`, the one the 2020 build already published to**
3. **Every publisher needing a human-owned secret must skip when the secret is absent, never fail**
4. **A cask over unsigned darwin binaries carries the quarantine-stripping `postflight`.**
5. **`goreleaser check` runs in CI alongside the snapshot, because they catch different things.**

### D183 — The AUR package is `kubecom-bin`, a new package; the 2020 `kube-commander` is retired by hand (2026-07-30, M5-07)
1. **The published AUR package is `kubecom-bin`, not the 2020 `kube-commander`, and that is forced rather than preferred.**
2. **`git_url` is written out explicitly, with the `-bin` suffix.**
3. **The package declares `conflicts=('kubecom' 'kube-commander')` and no `depends`.**
4. **The 2020 `kube-commander` and `kubectl-ui` shell shims are not carried forward.**

### D184 — The container image is the released binary over distroless-root, published to ghcr.io with no human-owned secret (2026-07-30, M5-08)
1. **The image ships the released artifact; it never rebuilds it.**
2. **`dockers_v2:`, not `dockers:` + `docker_manifests:`.**
3. **The base is distroless `static`, root variant, and `:nonroot` is rejected on purpose.**
4. **Docker is the one publisher with nothing to make inert.**
5. **`latest` is conditioned on `.Prerelease`.**

### D185 — Release notes are rendered, not reasoned about; and nothing is closed on the tracker before a release carries the fix (2026-07-30, M5-10)
1. **Every changelog filter and group in `.goreleaser.yml` matches a *scoped* conventional subject**
2. **`feedback:` and `dogfood …` commits are excluded from the notes.**
3. **A GitHub issue is closed when a release carries its fix, not when the code lands.**
4. **The first release's notes span the 2020 tag `0.7.6`.**

### D186 — envtest runs in the sandbox, so live-apiserver evidence is an agent's job; and aggregated discovery hides which group failed (2026-07-31, M1-INT-a)
Two constraints, both found by actually running envtest rather than reasoning about it.
1. **The envtest deferral premise is dead.**
2. **`DiscoveryResult.Failed` is empty on any aggregated-discovery cluster, and a fix must change the discovery client, not the parsing.**

### D187 — A broken API group is named from the group list, not from a different discovery client (2026-07-31, DISC-01)
Supersedes the *mechanism* clause of **D186** pt 2, which said the fix "must read through a client that implements `GroupsAndMaybeResources`".
1. **A group the server lists with no versions *is* a broken group, and that is what `DiscoveryResult.Failed` reports.**
2. **A failure names a group; a version is optional.**
3. **The discovery pass may read the group list, and only the group list.**
4. **This kind of claim is verified against a live apiserver, not a fake.**
**Refs:** Supersedes the *mechanism* clause of **D186.

### D188 — A merge-patch action is only correct because the UI gated it: the server does not refuse a wrong-kind patch (2026-07-31, M1-INT-c-3)
Proven live against a real 1.31 apiserver: `Suspend` on a **Deployment** returns `nil`.
1. **Applicability is decided before the request, never by the error.**
2. **The server does validate what it knows, which is why this is specifically about unknown fields.**
3. **A test that proves a merge patch was refused must read the object back.**
4. **Anything that makes an action reachable by a new route inherits this.**

### D189 — The edit buffer is the server's object minus managedFields, and nothing else: `resourceVersion` and `uid` are preconditions, not noise (2026-07-31, M1-INT-c-4)
Proven live against a real 1.31 apiserver. `Update` (the Edit write-back, D129) is a PUT, and everything that makes it *safe* is metadata the code never reads — it survives into the buffer only because `GetYAML` strips managedFields and leaves the rest alone:
- **With `metadata.resourceVersion`:**
- **Without it:**
- **With `metadata.uid`:**
- **Without the uid:**
1. **`GetYAML` strips managedFields and nothing else.**
2. **A Conflict from Edit is the answer, not a retry signal.**
3. **Two different situations both surface as `KindConflict`**
4. **A status-only edit is a no-op the server reports as success.**

### D190 — The envtest suite runs in its own workflow; `make check` stays hermetic and is never gated on a control-plane download (2026-07-31, M1-INT-d)
The gated suite (`KUBECOM_TEST_ENVTEST=1`, D18) now runs on every push and PR from [`.github/workflows/envtest.yml`](../../.github/workflows/envtest.yml). Where it runs is the constraint, not that it runs:
1. **Not in ci.yml, and not in `make check`.**
2. **The suite skipping is a green run, so the wiring needs its own guard.**
3. **`make test-envtest` is the single invocation.**

### D191 — What the 2026-08-01 dogfood closures do and do not license (2026-08-01, HT-dogfood-0801)
Three human tasks came back `done` in one session and all three closed *without* a change to the code. A closure with no diff is the easiest kind to over-read later, so what each one settles is written down here rather than left to the deleted file:
1. **There is no client-side CRD bug, and CRD-01 is not one.**
2. **"No problem found" is not "verified".**
3. **The logs throughput measurement confirms D162; it does not retire it.**

### D192 — The editor is resolved once at startup, and PATH detection is the last resort (2026-08-01, EDIT-01)
From feedback `2026-08-01-editor-autodetect`: with no editor variable set, kubecom fell straight through to `vi`, and a modern Arch host has `/usr/bin/vim` but no `vi` symlink — so `e` dead-ended on a machine with two perfectly good editors installed. The resolution is now:
1. **Precedence is `KUBE_EDITOR` → `EDITOR` → `VISUAL` → first of `nvim`, `vim`, `nano`, `vi` on `PATH`.**
2. **A set variable is never second-guessed.**
3. **The candidate order prefers editors whose presence implies a choice.**
4. **Resolution happens once, at startup, in the launcher — not at `e`-press time.**
5. **`resolveEditorArgv` stays pure**

### D193 — A pinned kind is recorded state, not authored config; the two files meet at `AddExtras` (2026-08-01, CRD-PIN-01)
From feedback `2026-08-01-custom-resources-pinning`: a kind you reach for once should stay in the menu for that context.
1. **Pins live in the state file**
2. **Both files feed the same merge.**
3. **Where they collide, the authored entry wins.**
4. **The merge happens on the launch path *and* the context-switch path**
5. **The GVR is the pin's identity.**
6. **A malformed pin is loud, in both files.**

### D194 — One fuzzy matcher; a picker filters as you type; `:` is the palette's key and the letter keys stay (2026-08-01, PAL-01)
From feedback `2026-08-01-command-palette-unification`, which asked for one surface you type into and explicitly asked that the keybinding consequences be recorded, since the target model supersedes one-key-per-picker.
1. **kubecom has exactly one fuzzy matcher.**
2. **A picker filters as you type.**
3. **`WithOptInFilter` is the exception, and stays rare.**
4. **`:` is the palette's key, and the existing shortcuts are kept, not retired.**

### D195 — An exec credential plugin failure is its own error kind; kubecom may *offer* a remediation command but never runs one unasked (2026-08-01, AUTH-01)
From feedback `2026-08-01-eks-sso-reauth`, which asked kubecom to notice an expired AWS SSO session and offer to run `aws sso login --profile x`.
1. **A failed exec credential plugin is `KindExecPlugin`, not `KindUnreachable`.**
2. **The detection is text-matching, narrowly, and that is not a shortcut.**
3. **The plugin's stderr is not in the error and has to be earned.**
4. **kubecom never runs an auth command the user did not just approve.**
5. **A remediation is only offered when it can be substantiated from the kubeconfig.**

### D196 — A departed cluster may be retained only *connector-side*, never by the shell; the teardown stays unconditional, and the retention is measured before it is built (2026-08-02, CTX-WARM-01)
From feedback `2026-08-01-context-switch-keep-state`: "can we keep the state of the previous cluster, so we can switch between contexts instantly?" — the submitter flagged that this pushes against M4-04a/D157 and asked for the resolution to be recorded rather than quietly made.
1. **The shell's teardown does not change.**
2. **Retention, if it happens, lives on the connector side.**
3. **Measure first; the numbers gate the build.**
4. **Bounded, and never a correctness claim.**
5. **This waits on the baseline.**

### D197 — The command palette is a picker over the action registry, and every registered action keeps a key of its own (2026-08-02, PAL-02)
PAL-02 landed the palette shell: `:` opens a list of the app's verbs, you type to narrow it, and the pick runs the verb. Two things about it are constraints rather than implementation, because the three remaining PAL slices all build on this surface.
1. **The palette resolves to an `Action` and dispatches it through the same entry point a key press reaches.**
2. **A verb is offered in the palette exactly when its key exists, and is inert exactly when its key is inert.**
3. **`:` is `app.palette`; `resources.switch` moved to `R` rather than becoming palette-only.**
**Refs:** supersedes D194 pt 4.

### D198 — An argument verb resolves inside the palette, and its stage ends in the verb's own apply function (2026-08-02, PAL-03a)
PAL-03a turned the palette's line into `<verb> <argument>`: a verb that needs a value no longer dispatches — it **commits in place**, and the same picker re-prompts itself `:resource ` and lists that verb's values.
1. **An argument verb does not dispatch its action; it commits into the palette's argument stage.**
2. **The stage's apply must be the same function the verb's standalone picker ends in**
3. **A verb whose values cannot be produced does not enter the stage at all.**
4. **In the verb stage, space is the line's separator and is never query text.**
5. **The line unwinds the way it was typed.**

### D199 — A fetched value list is addressed to the surface that asked for it (2026-08-02, PAL-03b)
PAL-03b gave the palette the two argument verbs whose values are **not in hand** when the stage opens: `:namespace ` lists against the cluster, `:context ` reads the kubeconfig.
1. **The load carries its destination.**
2. **A result whose stage no longer exists is dropped, not painted.**
3. **Inertness is decided before the stage opens, never by the answer.**
4. **A pending stage says so.**

### D200 — A pane that has nothing to show says why, and never guesses (2026-08-02, CRD-01)
CRD-01 gave the browse table an empty state that carries the reason its LIST failed. It is the first user-facing copy keyed on `kube.ErrorKind` — the taxonomy exists precisely so "the TUI renders its own message per kind" — so the rules are set here for every surface that follows (AUTH-04 is next):
1. **The reason lives where the reader is looking, and outlives the toast.**
2. **The success that ends it is the one that clears it.**
3. **A cluster-side cause the taxonomy would misname is recognised by name.**
4. **Every reason says where the fix is, and quotes the server.**

### D201 — Pinning writes state, dedupes against the *extras* list, and joins an existing action namespace (2026-08-02, CRD-PIN-02)
CRD-PIN-01 (D193) decided where a pin is stored. This is the gesture that writes one, and the three choices a later slice must not silently reverse:
1. **The "already there" test is the extras list, never the menu.**
2. **Pinning is per-context state, so its writer is rebound on a context switch.**
3. **A new action joins an existing action namespace unless it needs its own.**

### D202 — The pin key is a toggle, and the two menu-entry lists reach the shell unmerged (2026-08-02, CRD-PIN-03)
D193 decided where a pin is stored, D201 the gesture that writes one. This is the gesture that takes one back out, and what had to change for it to be possible:
1. **The authored list and the pinned list reach the shell separately.**
2. **A row may be removed only if the pin is the reason it exists.**
3. **A gesture that writes user state must be reversible from the same surface.**
4. **Provenance is not a display state.**

### D203 — A picker value carries match-only aliases, and a name two kinds share is qualified (2026-08-02, CRD-PIN-04)
The CRD-PIN line rests on "a kind you reach for once is yours from then on", and the reaching happens in the resource picker, not by scrolling a menu of hundreds. Two ways that surface lost kinds are closed here, and both generalise beyond it:
1. **A picker value may carry aliases: terms the filter matches but the row never shows**
2. **An alias hit is scored on its own merit, unpenalised.**
3. **A label a picker shows must identify exactly one value.**
4. **Aliases and qualification are computed where the item set is built, once**

### D204 — A gesture that reads the cursor gets a palette verb that takes the target as its argument (2026-08-02, CRD-PIN-05)
CRD-PIN-05 asked how a kind is pinned from the surface it is *found* on.
1. **A palette verb may take as its argument what the equivalent key reads from the cursor.**
2. **A verb reached two ways decides once.**
3. **A toggle keeps one verb and one word in the line.**

### D205 — Row-scoped verbs in the palette: one source for the set, the title for the target (2026-08-02, PAL-04)
PAL-04 put the actions that operate on the **selected row** into the command palette, so `:` finally answers "what can I do right now?" and not only "what can this app do".
1. **The set is the actions menu's set, computed by the actions menu's code.**
2. **The palette names the object it would act on, in its title.**
3. **A row verb dispatches the row action's intent, and inherits every guard on it.**
4. **The app-global verbs keep the top of the list; row verbs are appended.**

### D206 — The hint line is derived from whoever owns input, at the tail of every Update (2026-08-02, HINT-01)
D143 made a hint entry a promise and D143 pt 3 said the promise is kept by calling `syncHints` "wherever input ownership moves".
1. **The hint context is derived, not pushed.**
2. **The derivation mirrors Update's key-routing precedence, in the same order.**
3. **An overlay's hint context is chosen by its input state, not by its kind.**

### D207 — A shortcut key opens the palette on its verb's stage; it never opens a second surface (2026-08-02, PAL-05a)
The PAL feedback's complaint was five modals with five sets of habits. PAL-02…04 built the one surface; PAL-05 is where the keys stop being the *other* way of doing the same thing. The rule for each conversion, starting with `T`:
1. **The key ends in `enterPaletteArg`, not in an opener of its own.**
2. **Esc and backspace rewind to the verb list before they close.**
3. **The standalone picker is retired in the same leg, not left dormant.**
4. **The tests move with it rather than dying with it.**

### D208 — A converted key's other doors convert with it; the picker `Kind` switch keeps no default arm (2026-08-04, PAL-05c-1)
Two constraints D207 does not state, found by converting the first key that had a second entry point and the first verb whose values are fetched.
1. **Every door onto a retired surface converts in the same leg.**
2. **The `picker.SelectedMsg`/`CancelledMsg` switches have no default arm.**
3. **A collapsed `dest` is not a dropped guard.**

### D209 — A verb's argument list is a palette stage; no verb gets a modal of its own (2026-08-04, PAL-05c-2)
PAL-05 is finished: `T`, `R`, `ctrl+n` and `C` all open the one palette on their verb's stage, and no standalone value picker survives. That makes the rule general rather than a per-key backlog, so it is stated once as the constraint future legs must not contradict.
1. **A new verb that takes an argument gets a `:verb ` stage, not a picker.**
2. **Inertness and the value list live in that arm, not in the key.**
3. **The remaining pickers are not conversions this rule owes.**

### D210 — A curated set is a verb's argument even when the key is a noun; `a` is `:action ` (2026-08-04, PAL-05d)
D209 pt 3 filed `actPicker` with `ctrPicker`/`portPicker` as row data that keeps its modal.
1. **A verb's argument may be a curated set, not just a name the reader supplies.**
2. **A row-scoped stage lists its set alone, and the co-listing drop does not follow it.**
3. **A row-scoped stage's title names the object, not the verb.**
4. **The palette's own state must be read before `closePalette`, not after.**

### D211 — A credential plugin may be re-run only as a diagnostic on an already-failed request; its stdout is a credential and is never captured (2026-08-04, AUTH-02)
D195 pt 3 recorded that the plugin's stderr — the sentence that says *why* auth failed — is unavailable from the failure, because client-go streams it to the process's own `os.Stderr` (invisible under the alt-screen) and its error text carries only the executable name and an exit code.
1. **It is a diagnostic on an already-failed request, never a pre-flight.**
2. **The plugin's stdout is discarded and never returned.**
3. **Stdin is closed.**
4. **The run is bounded twice, and one of the bounds is not the obvious one.**
5. **A re-run that succeeds is an outcome, not an error.**

### D212 — A remediation is recognised by plugin **and** stderr, is composed only from the stanza, and a recognised-but-unsubstantiated failure ends the search (2026-08-04, AUTH-03)
D195 pt 5 promised the plugin → remediation catalogue would be a small explicit table rather than a provider framework. AUTH-03 writes its only entry (AWS SSO), which is where the table's rules have to be fixed — the next entry (`gcloud`, `az`) will be written by someone reading this, not the code.
1. **Recognition takes both halves, and neither alone is enough.**
2. **Every stderr marker must name SSO.**
3. **Only the stanza substantiates the command — never the process environment.**
4. **The catalogue is first-match-wins, and a recognised entry ends the search.**
5. **Suggesting is not running, and the API name says so.**

### D213 — The diagnosed plugin failure travels as one report, and its notice puts the fix above the evidence (2026-08-04, AUTH-04a)
AUTH-01…03 left three separate values in the kube layer (the stanza, the diagnosis, the remediation) and no surface. AUTH-04a fixes how they cross into the TUI and what the TUI is allowed to do with them.
1. **`kube.ExecPluginReport` is the boundary value, and the shell renders only.**
2. **The notice orders the fix above the plugin's stderr, because the pane drops its tail.**
3. **The diagnosis supersedes client-go's error; it does not join it.**
4. **`ExecPluginReport.Suggested` is for renderers, not for actors.**
5. **The cause sentence branches on the diagnosis, and a plugin that now works is offered nothing.**

### D214 — Diagnosing a credential plugin is one call over a `ClientConfig`, fired once per browse selection, and it may only rewrite the pane that asked (2026-08-04, AUTH-04b)
AUTH-04a made the copy; this is the runtime rule for producing it.
1. **The kube layer answers in one call, and it is the only thing that runs anything.**
2. **The seam is kubeconfig-scoped, not cluster-scoped.**
3. **One re-run per browse selection, however often the failure repeats.**
4. **A diagnosis may only rewrite the pane that asked for it, and only while that pane is still failing.**
5. **A diagnosis that establishes nothing changes nothing on screen.**

### D215 — The one subprocess kubecom composes runs through the existing suspend, on a single-use approval, and its success retries the *request* (2026-08-04, AUTH-05a)
D213 pt 1 said the shell never runs a subprocess.
1. **Composed in the kube layer, run by the shell.**
2. **The existing suspend, and no timeout.**
3. **The remediation runs in the *plugin's* environment**
4. **An approval is single-use.**
5. **Success retries the failed *request*, never the connection or the launch.**

### D216 — kubecom may *ask* to run a remediation only from a landed diagnosis, only over the plain browse view, and an unanswered offer is dropped rather than deferred (2026-08-05, AUTH-05b)
D215 fenced what an approved remediation may do. This fences the **asking**, which is the other half of D195 pt 4 ("offer — never run unasked") and the part a future surface is most likely to widen by accident, because an offer looks like copy rather than like an action.
1. **A landed diagnosis is the only thing that may open one.**
2. **It opens only over the plain browse view, and never queues.**
3. **Answering it, either way, ends it.**
4. **The pane and the prompt say the same thing, and the pane goes back when the prompt goes.**

### D217 — Every input-capturing surface has a hint context, and a context whose keys live outside the browse keymap still reads them from the registry (2026-08-05, HINT-02)
D206 made the hint derived rather than pushed and left the rule for *which* surfaces get a case implicit. HINT-02 closes the set the picker work started, and hits the one context whose keys are not browse keys, so both halves are worth pinning.
1. **A surface that captures input gets a `HelpContext`; it never falls through to the browse set.**
2. **A context-scoped action is hinted through the registry, like every other.**

### D218 — The hint-context set is closed by the router, and a repurposed key is still hinted under its global description (2026-08-05, HINT-03)
HINT-03 covered the last two capturing surfaces D217 pt 1 named, so the enumeration is complete. Two rules keep it that way.
1. **`hintContext` is the router's mirror, and a new capturing surface ships its case in the same leg that adds it.**
2. **A hint entry promises that the key acts, not what it does there.**

### D219 — A view may state its own verbs, but never its own keys: a body that names a gesture reads the key from the resolved keymap (2026-08-05, HINT-04)
D218 pt 2 leaves one escape hatch — where a surface's verb genuinely needs saying, the surface says it in its own body — and the port-forward panel's footer was the only place that used it.
1. **The verbs are local, the keys are generated.**
2. **An action the user unbound drops out of the body, and a body with nothing left disappears.**

### D220 — An overlay renders no taller than the box it computes, and elides its explanation before its ask (2026-08-05, BOX-01)
`overlayCenter` composites a box onto a fixed `width×bodyHeight` canvas (D95), so a box bigger than the body is not shrunk, scrolled or scaled — it is **clipped, bottom-first, in silence**.
1. **A component that renders free-form content into a fixed-size box bounds that content itself.**
2. **What is elided is the explanation, never the ask.**
3. **Dropped content says it was dropped.**
4. **A geometry minimum must fit what the surface always renders.**

### D221 — An overlay with a cursor scrolls rather than truncates, and counts what it hides in its title (2026-08-05, BOX-02)
D220 pt 1 says a box bounds its own height; it left open *how* the surplus goes.
1. **A bounded surface that has a cursor keeps the cursor on screen.**
2. **A scrollable elision announces itself in chrome that always renders, not in a row taken from the content.**
3. **Chrome is first-class, but a footer is not worth the last content row.**

### D222 — The elision marker names where the rest is, and the clamp has one home (2026-08-05, BOX-03)
D220 pt 3 fixed *that* a cut is announced and, with only the modal to go on, fixed the sentence too.
1. **When elided content is reachable elsewhere, the marker says where.**
2. **The clamp lives in `internal/tui/elide`, and a bounded overlay uses it rather than re-deriving it.**
3. **A box with no room for content renders nothing rather than a frame.**

### D223 — A `HelpContext` is only real when both of its sides are closed (2026-08-05, HINT-05)
The HINT line (D206/D217/D218) gave every capturing surface in kubecom a `HelpContext`, and D218 pt 1 recorded what it could not give them: nothing enforced that a *new* one arrives complete.
1. **The enum is bounded and enumerable, and both sides of a context are checked.**
2. **The fallback stays, and is for out-of-range integers only.**
3. **A reachability failure is a routing bug, not a hint bug.**

### D224 — A vault rule that a leg can break silently is checked by `make check` (2026-08-05, BOARD-01)
D102 established that a board **Done entry is one line** and re-collapsed the list by hand (~50KB → ~17KB).
1. **D102 is now a gate, not a convention.**
2. **The guard covers the Done list only.**
3. **This is the general shape, not a one-off.**

### D225 — The `## Done` index is the canonical record of a finished item; a section's `- [x]` is a working view (2026-08-06, BOARD-02a)
BOARD-02's notes left the board in a half-and-half state: the `## Done` index had not been appended to since 2026-08-02, so 26 finished items existed only as the `- [x]` line inside their own Backlog line's section, while 207 older items existed only in the index.
1. **The index is canonical.**
2. **The backfill is a copy, never a summary.**
3. **A rollup entry carries the union of its slices' pointers, not a new claim.**

### D226 — A board paragraph is not a backlog: deferred work names a destination, and a closed line's prose is a claim that goes stale (2026-08-06, BOARD-02b-1)
Four closed lines (SEARCH, CRD-PIN, PAL, AUTH) ended with a sentence of the form "two things it deliberately left, either its own small item if a dogfood wants them".
1. **A closed line's paragraph states current state, and nothing checks it.**
2. **Every deferral names its destination in the same paragraph.**
3. **"No item until asked" is a real answer, and it is not the same as declined.**

### D227 — `:resource ` and `:pin ` share one snapshot, so narrowing the menu re-sources both or takes both down (2026-08-06, BOARD-02b-1)
Carried out of the CRD-PIN paragraph, because it constrains a leg nobody has written yet rather than describing one that landed.

### D228 — A row action declares whether it asks before it acts, in the registry, pinned to its handler (2026-08-06, PAL-06)
1. **The declaration is a column of `rowActions`, not a list beside it.**
2. **The column is pinned to the handlers, not trusted.**
3. **What is marked is permission, not input.**
4. **The marker is part of the label, and the label is the identity.**

### D229 — A closed line's paragraph collapses to its outcome, its pointers and its standing answers (2026-08-06, BOARD-02b-2)
D224 pt 2 left the working area unchecked on the grounds that its prose "carries why a line was split and what a slice deliberately left, which the journal does *not* duplicate", and declined to authorise a compaction on the strength of that decision.
1. **Closed lines only, and the shape is already on this board.**
2. **Three things survive the collapse, and nothing else.**
3. **A paragraph is collapsed only after its claims have been checked.**
4. **This stays unchecked, deliberately.**

### D230 — The logs view bounds what it *fetches* and what a line *costs*, never how much it holds (2026-08-06, BOARD-02b-3)
BOARD-02b-3 collapsed the LOGS line to "closed on features and on cost (D160, D162)" and had to check that sentence first.
1. **No leg may cite D160 or D162 as evidence that the logs view's memory is bounded.**
2. **A streaming view that keeps a rendered cache alongside its buffer bounds both together.**

### D231 — A criterion closed on substitute evidence carries the substitution in the tick (2026-08-06, HT-dogfood-0806)
The M5 criterion "migration verified from a **real** legacy config file" asked for the one artifact nobody has: the maintainer's answer to `2026-07-30-real-legacy-config-migration` was *"I can't test, I don't have old config.
1. **The tick and the substitution travel together, in the box itself.**
2. **No leg may cite this tick as real-file evidence.**
3. **"The human cannot do it" closes a task; it does not close the question.**

### D232 — Text kubecom did not write is sanitized at the seam that renders it, and that is only half of what corrupts the layout (2026-08-06, AUTH-06)
Feedback `2026-08-06-auth-error-breaks-layout` reported an auth failure *distorting the surrounding UI* rather than showing as an error. Two separate mechanisms do that, and this decision exists so a later leg cannot mistake one for both.
1. **Every surface that displays text kubecom did not write sanitizes it, at the point it renders it, through `internal/tui/safetext`.**
2. **It is applied to the render, never to the stored string.**
3. **No leg may cite this as evidence that an auth failure cannot corrupt the layout.**

### D233 — Esc backs out of the surface, backspace unwinds the line; a key-opened palette stage closes on the first Esc (2026-08-06, PAL-07)
Feedback `2026-08-06-action-menu-esc-behavior`: press `a` on a pod, press `Esc`, and the palette is still there.
1. **Esc leaves the surface the reader is on for the one they came from.**
2. **Backspace keeps rewinding, from every stage.**
3. **A typed query still costs its own esc, in every picker.**
**Refs:** supersedes D207 pt 2.

### D234 — A printed cell that is really a clock is re-derived locally; nothing else in the table is (2026-08-06, AGE-01)
Feedback `2026-08-06-age-column-stale`: leave a resource pane open and the AGE column stops telling the truth.
1. **AGE is re-derived on the client, from the object's own `creationTimestamp`, with the server's formatter.**
2. **AGE is the only cell kubecom recomputes.**
3. **A row kubecom cannot date keeps the server's string.**
4. **The clock that drives it is unconditional and self-perpetuating.**

### D235 — In cluster search, enter is the seam between typing and moving; focus is the view's only mode (2026-08-06, SEARCH-05)
Feedback `2026-08-06-cross-search-enter-navigate`: after typing a query and pressing enter, typing should stop being captured by the query input and `hjkl` should move through the results.
1. **`nav.drillIn` commits before it opens.**
2. **`nav.back` unwinds one step at a time**
3. **Focus decides how a key is routed, and the two rules are asymmetric on purpose.**
4. **Focus follows the rows.**
5. **The committed query line is muted (`styles.Subtle`), and that is load-bearing, not decoration.**
**Refs:** amends D140 pt 1.

### D236 — A built-in theme is a palette, an attribution and a name that cannot move; light palettes wait on a background (2026-08-06, THEME-01)
Feedback `2026-08-06-more-themes` asked for ~10 built-ins, Catppuccin among them, and for licences to be **checked per theme rather than assumed**. The survey and the per-scheme licence findings live in `vault/knowledge/themes.md`; what a future leg must not contradict:
1. **A ported palette carries its attribution in the constructor's doc comment**
2. **`default` and `catppuccin-frappe` are one palette under two names, on purpose.**
3. **No light theme until kubecom paints its own background.**
4. **A flavor family is named `<scheme>-<flavor>`.**

### D237 — A palette row is the command's name *and* its description, in two columns (2026-08-06, PAL-08)
Feedback `2026-08-06-palette-two-columns`: the palette showed only each verb's description, so the reader could not see what the command was actually called. Addressed by giving `picker.Item` an optional `Name` drawn in a column before the label. What a later leg must not silently contradict:
1. **The name is display + match; the Label is still the identity.**
2. **A command's name is the id it already has elsewhere**
3. **A picker row is one line — it is truncated, never wrapped.**
4. **The column is measured over the *visible* rows, not the whole item set**
5. **Unnamed lists render exactly as before.**

### D238 — An editing key with nothing to edit is a cancel: backspace on an empty `/` query resolves to nav.back (2026-08-06, FILT-01)
Feedback `2026-08-06-search-backspace-cancel`: press `/`, press backspace with nothing typed, and the query field stays open and empty.
1. **The gesture is the last backspace, not the first.**
2. **It resolves to `nav.back`, it does not get a cancel path of its own.**
3. **The keymap wins.**
4. **It applies to a `/` that opened over content, not to a picker's filter.**

### D239 — A `/` match is painted over the row's own colors, and the cursor bar does not hide it (2026-08-07, FILT-02)
Feedback `2026-08-06-search-highlight-matches` asked for matched text to be highlighted in both `/` search and cluster search.
1. **The highlight's scope is the filter's scope, exactly.**
2. **A match wins over a cell's status color, and the role span is cut around it rather than replaced.**
3. **The selection bar does not win over a match**
4. **Span coordinates stay in runes and unclipped columns.**

### D240 — Landing where you left off is per-context *state*, not a warm cluster: pane memory is an inert bookmark on the ContextState seam (2026-08-07, CTX-MEM-01)
From feedback `2026-08-06-context-switch-pane-memory`: "flip to context B, look at its `pods` pane, flip back to context A, and be looking at what I was looking at before".
1. **The feedback is two asks and they belong on two lines.**
2. **Pane memory rides the existing `ContextState` seam (M4-05/D163), never a shell-side map of departed contexts.**
3. **What is remembered is an address, not data.**
4. **It restores at launch as well as at a switch.**
5. **The bound the feedback offered does not apply to this half, and D196 pt 4's one-entry cap is unchanged for the other.**
6. **The drill-in scope is deferred, not forgotten (CTX-MEM-04).**

### D241 — The README is the narrative and `docs/` is the reference; a doc guard scans the doc set, not one file (2026-08-07, DOC-01)
From feedback `2026-08-07-readme-structural-rewrite`: the README had grown to 587 lines by accretion — every leg that added a capability appended a paragraph under one flat `## Usage` heading, install sat ahead of a single concrete thing kubecom does, and config/theme/menu reference was interleaved with the walkthrough.
1. **The README answers "is this for me", organised by capability.**
2. **Reference material lives in `docs/`.**
3. **A guard over user-facing docs reads the doc set, not `README.md`.**

### D242 — The logs view's cursor addresses log lines; the selection bar is derived, never cached (2026-08-07, LOGS-SEL-01)
From feedback `2026-08-07-logs-selection-and-yank`. The logs view had scrolling and no cursor, so nothing could say "this line" and nothing could copy one. This is the cursor; LOGS-SEL-02 is the visual mode and the yank on top of it. The constraints a later leg must not walk into:
1. **The cursor counts log lines, not screen rows.**
2. **The cursor addresses the shown set, and `shownIdx` is how it reaches the buffer.**
3. **Selection and Match share the cursor's line.**
4. **The bar is painted on the way to the viewport, never into `shownLines`.**
5. **Following owns the cursor.**
6. **Row arithmetic is done here, in wrap mode only.**

### D243 — Pane memory is written where the watch goes live and replayed where discovery lands; the attempt is single and always loses (2026-08-07, CTX-MEM-02)
D240 said *what* is remembered (an address on the `ContextState` seam) and *that* a miss must be silent. This is where the two ends attach, and CTX-MEM-03/04 extend these points rather than re-choosing them.
1. **One write point: `watchResource`, after `Watch` returned.**
2. **One replay point: `handleDiscovery`, after `Reconcile`.**
3. **The restore is attempted once per cluster and loses every tie.**

### D244 — Visual mode owns follow; the yank copies the buffer, and `y` is a second confirm-context twin (2026-08-07, LOGS-SEL-02)
D242 built the cursor and named the constraints a selection would inherit. This is the selection and the copy on top of it. What a later leg must not silently contradict:
1. **A selection and a running stream are mutually exclusive.**
2. **`nav.bottom` changes meaning inside visual mode.**
3. **Both ends of the selection are log lines, carried across a rebuild by buffer index.**
4. **The copy is the buffer, not the screen.**
5. **`y` is browse-context `logs.yank` and confirm-context `confirm.accept` at once.**
6. **The closed-grep logs hint no longer offers `logs.regex`.**

### D245 — The logs buffer is bounded, drops from the top, and says so once it has (2026-08-07, LOGS-07)
Nothing bounded it before: `TailLines` bounds the **replay** that precedes the tail, and LOGS-05b bounds what a line **costs**, not how many are held (D230).
1. **The cap must exceed the opening replay.**
2. **The trim drops from the top, in chunks, and is O(1) per line amortized.**
3. **Everything that addresses a line by index moves with the trim.**
4. **A trimmed buffer says so, once, in a word.**

### D246 — A cluster-search hit carries the runes it matched (2026-08-07, SEARCH-06)
FILT-02/D239 painted the table's `/` matches, and the obvious move was to do the same thing here.
1. **`kube.SearchHit.Match` is the marks, in the *name's* rune space.**
2. **The marks are the occurrence the score was read from.**
3. **Spans are a second pass, not an extra return from `Match`.**
4. **The cursor row keeps its marks**

### D247 — fd 2 belongs to the log for the life of the TUI, and to the terminal only inside a suspend (2026-08-07, AUTH-07)
The alt screen is not a second terminal. Anything written to file descriptor 2 while kubecom is up paints over the panes and survives until bubbletea happens to repaint those exact lines — client-go's exec credential plugin (`cmd.Stderr = a.stderr`, on *every* refresh, not only a failing one), a panic trace, a cgo…
1. **The descriptor, not the variable.**
2. **The redirect covers the TUI's life, not the process's.**
3. **Every `tea.Exec` in `internal/tui` goes through `Model.suspend`.**
4. **A failed handover never fails the action.**
5. **This does not license printing to fd 2.**

### D248 — "Dark" is a measured admission criterion, and a bare scheme name means that scheme's default variant (2026-08-08, THEME-02)
THEME-02 took the registry from six built-ins to eleven (`dracula`, `gruvbox-dark`, `nord`, `rose-pine`, `tokyo-night`, each transcribed from the upstream data file `vault/knowledge/themes.md` names). Two of the things it settled are constraints rather than descriptions:
1. **D236 pt 3's "is it dark?" now has a number, and THEME-03 owes it an argument.**
2. **A bare scheme name means the scheme's own default variant; a suffix is for peers.**
3. **The status bar takes the first background shade above the scheme's base.**

### D249 — The theme's canvas is the terminal's background, painted once by the root View (2026-08-08, THEME-03)
`Theme` gains a fourteenth role, `Background`, and it reaches the screen as `tea.View.BackgroundColor` — the terminal's own default background for as long as kubecom holds the screen. Every built-in sets it (D169 pt 3, enforced by `TestBuiltinThemesAreComplete`), and `View` reads it off `m.styles` on every frame.
1. **No component paints the canvas, and none may start.**
2. **Terminal-level state is the price, and Bubble Tea pays it.**
3. **A nil `Background` resets to the terminal's own — it is not black.**
4. **That fallback is what still gates a light palette, and D248 pt 1 is unchanged.**

### D250 — kubecom asks the terminal what its background is, and warns only on a polarity mismatch (2026-08-08, THEME-04a)
D249 pt 4 left the light-palette slice one question: what does kubecom do where it cannot paint?
1. **The terminal's own answer is the only evidence; environment sniffing is not.**
2. **The probe is deliberately late, single, and generation-tagged.**
3. **Three silences, and they are the decision.**
4. **The line names the observation, never the cause, and nothing is refused.**
5. **Polarity is measured, not declared.**

### D251 — a built-in palette is admitted for coherence, not for darkness (2026-08-08, THEME-04b)
`catppuccin-latte` and `solarized-light` are the registry's first light palettes, and the question they force is what the admission guard was ever protecting.
1. **The criterion is internal polarity plus the unchanged contrast floor.**
2. **A port may move along the upstream ladder to clear the floor; it may not invent a value or lower the floor.**
3. **Latte is the family's fourth flavor, not a light theme beside it.**

### D252 — the cursor bar and the match highlight share a line; the yank gesture set is closed (2026-08-08, LOGS-SEL-03)
The two questions feedback `2026-08-07-logs-selection-and-yank` left open, answered so a later leg stops re-deriving them.
1. **They coexist; neither yields while visual mode is active.**
2. **`y` on the cursor line and `gg v G y` for the buffer are the whole gesture set.**
3. **The `Match` pair is body text, and is held to the body floor.**

### D253 — the Homebrew tap is the org-level `neuroplastio/homebrew-tap`, kubecom its first tool (2026-08-09, release-namespace fold-in)
The maintainer's answer to `2026-08-07-release-namespaces-after-org-move` moved the tap from the 2020 personal one to an org-level tap, and authorized the agent to set it up. This supersedes **D182 pt 2**, whose "the tap is `AnatolyRugalev/homebrew-kubecom`" premise is now deliberately abandoned.
1. **`brew tap neuroplastio/tap` is the install line, from the repository `neuroplastio/homebrew-tap`.**
2. **The 2020 `AnatolyRugalev/homebrew-kubecom` tap is abandoned with no redirect.**
3. **The org move invalidates repository secrets.**
**Refs:** supersedes **D182 pt 2.

### D254 — container builds are dropped for now (2026-08-09, release-namespace fold-in)
The maintainer's decision, verbatim: *"containers: drop container builds for now."* This **supersedes D184 entirely** — no `Dockerfile`, no `dockers_v2:` block, no docker steps in the release workflow (`setup-buildx-action`, `login-action`, `packages: write`), no container section in `docs/install.md`, and…
1. **The release pipeline publishes two things now: the Homebrew cask and the AUR package.**
2. **This is reversible, deliberately.**
**Refs:** supersedes D184.

### D255 — a remembered drill-in comes back as a drill-in; the re-resolve degrades legibly (2026-08-09, CTX-MEM-04)
D240 pt 6 deferred the drill-in scope (CTX-MEM-04) until a leg could make its owner-gone failure legible on screen; this is that leg.
1. **The drill-in address rides the same seam and shape as the remembered kind.**
2. **The restore re-resolves the scope; it never replays it.**
3. **The owner-gone failure is legible, and it lands on the plain list.**

### D256 — maintainer review 2026-08-09: context switching ships as-is; remaining dogfood QA declined (2026-08-09, human-tasks fold-in)
The maintainer reviewed the open human tasks via margin and closed most of them by directive. Constraints a future leg must not silently contradict:
1. **Context switching is accepted as-is.**
2. **CTX-WARM-02/03/04 are cancelled.**
3. **The remaining dogfood QA is declined, not failed.**
4. **The Homebrew/AUR credential tasks are deferred to release time**
**Refs:** supersedes the gating in D196 pt 3.

### D257 — a rejected previous-instance flip keeps the log view; the running instance's stream resumes under the toast (2026-08-09, LOGS-08)
Feedback `2026-08-09-logs-no-previous-keeps-view` (maintainer, verbatim: "Show error and exists log view.
1. **A rejected `Previous` flip never closes the view.**
2. **The fallback re-issues the stashed request with only `Previous` cleared**
3. **Nothing pre-checks for a previous instance, still.**
**Refs:** supersedes D177 pt 4.

### D258 — the palette family passes through the logs view; `o` is the previous-instance default (2026-08-09, LOGS-09)
Two feedback items, one surface: `2026-08-09-logs-view-palette-bindings` ("Ctrl+P is a bit weird … allow command palette inside logs view at the very least") and `2026-08-09-context-switch-key-from-overlays` ("C doesn't work when popup or logs are open"). Constraints a future leg must not silently contradict:
1. **The logs view passes the palette family through**
2. **Row-scoped gestures stay swallowed over the logs view**
3. **Transient capture surfaces keep owning their keys**
4. **The palette over the logs view lists the logs view's own verbs**
5. **`logs.previous` defaults to `o`, keeping `ctrl+p` as the second binding.**

### D259 — the match highlight paints canvas-on-Warn on dark palettes; weight-only on light ones (2026-08-09, LOGS-SEL-04)
Feedback `2026-08-09-log-match-highlight-background` (maintainer, verbatim: "Highlighted text (matches) should have bright (yellow) background.
1. **`Match` is bold + underline on every palette, and on a dark canvas also paints the canvas color on `Warn`.**
2. **The two-background rule is untouched (D252 pt 1 stands).**
3. **The three light palettes keep weight-only, and that is scope, not a leftover.**

### D260 — the theme picker previews live; a preview is repaint only, and cancel restores (2026-08-09, THEME-07)
Feedback `2026-08-09-theme-picker-live-preview` (maintainer, verbatim: "theme switching should happen as I change selection in the pallette, so I can test the look without pressing enter").
1. **A preview is repaint only.**
2. **A cancel is a restore, not a switch.**
3. **The anchor marker does not follow the preview.**
4. **Committing the anchor row is still a no-op**

### D261 — the screencast tape must demo a match and start from a clean state; reruns are reproducible (2026-08-09, M5-09b)
Feedback `2026-08-09-screencast-tape-tuning` (maintainer, verbatim): "search across cluster doesn't find anything (bad example)"; "initial state gets modified when rerunning the tape"; "we need to show off more features and add more captions". Three constraints a future tape edit must not silently break:
1. **A demo query must be one the tour already proved matches.**
2. **Every rerun starts from the welcome screen.**
3. **The tour grows, never shrinks, and every keypress stays annotated.**

### D262 — the toast auto-clear duration is a per-model option, defaulting to 5s (2026-08-09, audit-test-suite-runtime)
Feedback `2026-08-09-audit-test-suite-runtime`: the tui suite spent ~90s of its 190s draining real 5s toast ticks — 17 tests slept exactly 5.01s or 10.01s because `surfaceError`/`surfaceNotice` returned `tea.Tick(errorDisplay)` and the `drain` helper executes commands synchronously.
1. **`tea.Tick` reads `m.toastTimeout`, never the `errorDisplay` const.**
2. **The test default is `WithToastTimeout(time.Nanosecond)` via `sized`/ `sizedWith`, with an explicit option winning**

### D263 — `make check` enforces `gofmt` via the golangci-lint formatter floor (2026-08-09, FORMAT-GATE)
Feedback `2026-08-09-audit-format-gate`: the lint step ran only the standard linters and so could not see formatting drift, which had already slipped in once (`styles.go`'s `Match` field, LOGS-SEL-04).
1. **The format floor is `gofmt` only, and the gate is the golangci-lint run.**

### D264 — the plan's `views/` directory is superseded; full-screen surfaces are `components/*` sub-models, and the root package is the shell (2026-08-09, APP-MONOLITH)
Feedback `2026-08-09-audit-app-monolith` (Priority: medium): `internal/tui/app.go` has grown to 4,635 lines / 149 functions while the package layout REWRITE_PLAN's target architecture and D52 committed to — `internal/tui/views/` holding browse, logs, describe, yaml — was never created, and no decision ever revoked or…
1. **The `views/` directory as literally planned is revoked.**
2. **The browse 2-pane is not a separable view — it is the shell.**
3. **Load-bearing constraint a future leg must not silently break: a new full-screen, self-contained surface is a `components/*` sub-model**
4. **app.go's current size is accepted for now, and reducing it is a standing, pickable item, not an obligation.**

### D265 — a shell-owned listing is handed to a `components/*` panel as read-only entries; the shell keeps the authoritative set and performs the mutations (2026-08-09, MONO-01)
The first MONO-01 extraction (D264 pt 4) moved the M3-13b port-forward panel out of `app.go` into `components/forwards`.
1. **The shell keeps the authoritative set; the component receives a read-only `Entry` per member**
2. **The component owns its interaction state**
3. **A side-effecting gesture is an intent, not a method**
4. **Every component with a `SetStyles` must be in `applyStyles` and in `themeSurfaces`**

### D266 — publishing a release is a leg that changes the install docs; the docs name the exact version that exists (2026-08-12, DOC-02)
D68 requires a leg to update `README.md` when it "changes how a user installs, launches, configures, or uses `kubecom`".
1. **Publishing a release is an install-docs change.**
2. **Name the version that exists, not the state of the tag list.**
3. **A pre-release is documented as one, and asked for by name.**
4. **Publisher inertness is not a pre-release rule.**
5. **A criterion asking for artifacts is closed by the rc.**

### D267 — a Done entry's **ID** is unique; a committed collision is renamed on the cheaper side, never rewritten (2026-08-12, BOARD-03)
D15 makes the leg id the join key: the commit subject carries it, the journal entry is found by it, and the board's Done index is where a reader looks it up.
1. **An `**ID**` in the Done index names exactly one leg**
2. **Completeness cannot stand in for uniqueness.**
3. **A collision that has already been pushed is resolved by renaming, not by rewriting history.**
4. **Rename the side with fewer references, and say what its commits carry.**

### D268 — the tag waits on a walked user path; stories are the instrument, and they are versioned with the code (2026-08-15, UX-PLAN)
The `v1.0.0-rc.1` run proved the *pipeline* — artifacts, ldflags, notes, `go install` — and nothing about the *product*.
1. **A story is a goal, not a script.**
2. **The keystroke log ships, off by default**
3. **The cluster fixture is committed, and clean on every run.**
4. **The docs reorganize, they do not become a site.**

### D269 — the letter remap: one verb, one easy key; switchers are capitals; search is the browser key (2026-08-15, STORY-06a)
The S05 walk read two letter-families as kubecom's most confusing corners — the n-family (namespace vs next/previous match) and the s-family (search vs sort vs sort-clear) — and filed one master redesign (`2026-08-15-keymap-redesign.md`) with the whole intended map.
1. **search.cluster moves to `ctrl+f`**
2. **ns.switch moves to `N`**
3. **delete takes `D`, describe takes `d`**
4. **`logs.select` gains `V`**
5. **The displaced-key rule generalises.**

### D270 — the sort column-picker is a header-focus mode on the table, not a popup; `s` is freed and `sort.clear` rides `x` (2026-08-15, STORY-06b)
The redesign's S interaction (`2026-08-15-sort-column-picker.md`, superseded at 06a and carried here by the 06b board note) lands as a **column-header sort mode** on the table rather than a popup: `S` focuses the header row, `h`/`l`/`left`/`right` move a cursor across the columns, `enter` toggles the sort direction on…
1. **`sort.column` is `S`, not a cycle.**
2. **`sort.clear` is `x`, it works everywhere a table is showing, and inside the mode it resolves the mode.**
3. **The mode is a capturing surface.**

### D271 — `enter` opens the actions menu on the selected row; it lives in a resource-table key context and `a` frees up (2026-08-16, STORY-06c)
The walk's finding (`2026-08-15-enter-actions-menu.md`): pressing `enter` on a resource row should launch the actions menu (`actions.menu`, today the `:action ` palette stage), and drill-in (`res.children`, "Show pods") is already an entry in it — so drilling into an owner's pods is one menu pick away.
1. **`actions.menu` binds `enter` in a third key context, `ctxTable`, resolved while the resource table owns the keys**
2. **`a` frees up.**
3. **The table's own `enter` no longer emits the dead `RowSelectedMsg`.**

### D272 — pickers open in navigation mode: the list is the target, `j`/`k` move it, and `/` opens the filter; the current choice is preselected (2026-08-16, STORY-06d)
The S01 walk's sharpest dead end (`2026-08-15-picker-navigation-mode.md`): pressing `j`/`k` in the namespace switcher typed into its filter instead of navigating — 4 dead `j`s across two picker visits.
1. **A picker's `Show()` opens in navigation mode**
2. **The command palette's verb list is the one type-to-filter surface left.**
3. **A value picker preselects the current choice.**
4. **This does not touch `WithOptInFilter` or a second matcher.**
**Refs:** supersedes D194 pt 2.

### D273 — the trace instrument tells the truth about both dead-end classes: the recorder writes the confirm modal's resolved action, and the analyzer reports text-surface presses in their own section (2026-08-16, STORY-06e)
The S02 walk's two mirror-image instrument failures, filed together (`2026-08-15-confirm-key-false-deadends.md` + `2026-08-15-analyzer-text-surface-blindspot.md`): one hid real dead ends, the other fabricated them.
1. **The recorder writes a confirm modal's *resolved* action.**
2. **A press on a text surface is not a dead end and not invisible: it is its own report section.**
3. **The analyzer's dead-end predicate stays `action == "" && !text && !pending`; the text branch comes first.**

### D274 — a dedicated `events` action lists an object's own core Events, filtered by involvedObject UID and rendered as the server-printed table (2026-08-16, STORY-06f)
S02's describe was the diagnostic lever only because events lived nowhere else (`2026-08-15-events-action.md`); the fold-in gives the object's events their own surface, "why is this red" in one gesture.
1. **The kube primitive filters by `involvedObject.uid`, falling back to the name.**
2. **The list is a server-printed Table, kubectl-identical.**
3. **`E` is the default direct key**

### D275 — the unhealthy quick-access is a per-kind filter on the current table (`H`), split from the cross-kind surface (2026-08-16, STORY-06g-1)
S02's two sharpest findings — "no way to jump straight at the failing rows" and "a failure that is not a pod is invisible" — both came from the fold-in's STORY-06g, which the board had folded into one slice.
1. **`app.unhealthy` (`H`, shift+h) narrows the current resource table to the rows the M4-06 cell classifier reads as unhealthy.**
2. **The unhealthy view composes with the `/` substring filter and is a view over the authoritative full set.**
3. **The status bar shows an `unhealthy` marker while the view is on**

### D276 — the cross-kind unhealthy sweep is a `kube.Scan` primitive over a caller-supplied `RowFilter` (2026-08-16, STORY-06g-2a)
S02's miss — the fifth failure is a PVC and a pod-first walk never visits it — needs a cross-kind sweep, the other half of STORY-06g (D275 pt 3). The primitive lands first, before the surface (D52's bottom-up rhythm, the M4-07→M4-08 / SEARCH-02a→02b shape):
1. **The sweep is a kube-layer primitive over a caller-supplied predicate.**
2. **A scan hit carries the row, cells included**
3. **STORY-06g-2 is two slices**

### D277 — `/` filters whichever pane holds focus: the resource kinds in the menu, or the table rows (2026-08-16, STORY-06m)
Feedback `2026-08-15-resources-pane-filter.md`: pressing `/` while the **resources pane** holds focus launched the filter in the **table** instead of narrowing the kind list in that pane. The old rule — "`/` targets the table wherever focus is" — is replaced by "`/` targets the focused pane":
1. **`/` is a pane-scoped filter.**
2. **The filter matches the D203 alias surface**
3. **The menu hint set advertises `/`**

### D278 — the scan surface is a `components/unhealthyview` sub-model fed by `kube.Scan`; a hit carries the columns so the reason is renderable (2026-08-16, STORY-06g-2b-1)
The surface half of STORY-06g-2 (D276) is a full-screen, cursor-navigable list of ScanHits — the "the broken things find the operator" view for the S02 miss (a failure that is not a pod). Its shape follows the decisions that already govern full-screen surfaces:
1. **`kube.ScanHit` carries the columns the row sat under.**
2. **The row predicate is the table component's exported `UnhealthyRow` / `UnhealthyCells`**
3. **The surface is a `components/unhealthyview` sub-model**

### D279 — the unhealthy sweep is wired as a one-shot scan on `U`, over the menu's kinds, reusing the search pending-selection mechanism (2026-08-16, STORY-06g-2b-2)
The wiring that makes the cross-kind list reachable (the second half of D278 pt 3) lands the seam and the gesture:
1. **The `Scanner` seam mirrors `Searcher` exactly**
2. **The gesture is a browse action `app.unhealthyScan` on `U`**
3. **The sweep runs once, on open, over a generation-guarded pump.**
4. **Drill-in reuses the search pending-selection mechanism.**
5. **The view routes like the logs view with its grep closed**

### D280 — the follow state is a painted badge, `styles.Follow`, gated by the canvas's polarity (2026-08-16, STORY-06j-1)
The S03 feedback `2026-08-15-logs-follow-visual-signal.md` wanted the follow/pause state to be unmistakable at a glance — "this is confusing, yeah" — because a word (`[following]`/`[paused]`) in the header is exactly what the eye skips. The shape of the fix is settled so later legs stop re-deriving it:
1. **The indicator is a `styles.Follow` role, not a per-view colour.**
2. **The paint rule is Match's, exactly (D252 pt 3): weight everywhere, paint only where the paint can carry the floor.**
3. **The header composes the badge mid-line without breaking the neighbours.**

### D281 — a downward scroll *past* the newest log line re-arms following (2026-08-16, STORY-06j-2)
The S03 feedback `2026-08-15-logs-scroll-past-end-resumes-follow.md` (Priority high) reported the trap: scrolling up to read pauses follow, and scrolling back down leaves the reader at the end of a *silently frozen* stream — only `G` re-arms, and a reader already sitting on the newest line has no reason to press it.
1. **The gesture is the press *after* the one that lands on the newest line.**
2. **Every downward navigation says it, and only when the move is a no-op.**
3. **Visual mode is excluded.**
**Refs:** replaces D147; supersedes D147.

### D282 — in the cluster search, `enter` opens a hit and a *movement* commits into the result list (2026-08-20, STORY-06k-1)
The S05 feedback `2026-08-15-search-single-enter.md` (Priority high) reported the cost of SEARCH-05's mode: reaching a result took two enters, because the first one was spent moving focus from the query field to the result list. The split that replaces D235's commit-then-open half:
1. **`nav.drillIn` always opens the highlighted hit**
2. **A movement over the list is what commits into it.**
3. **The hand-off refuses an empty list.**
4. **`nav.back` is unchanged and is now the only action here that reads the focus**
**Refs:** replaces D235.

### D283 — a search hit carries the row it was matched in, and the search view previews it (2026-08-20, STORY-06k-2)
The S05 feedback `2026-08-15-search-result-preview.md` (Priority high) reported the cost of a result list that shows only identity: the wrong hit gets opened and the search is paid for twice. The preview that answers it:
1. **`kube.SearchHit` carries `Columns` and `Cells`**
2. **The preview is a two-line footer, reserved unconditionally.**
3. **A preview is the data the surface already holds, never a fetch.**

### D284 — a pager fills the pane; only popups overlay (2026-08-20, STORY-06h-1)
The S02 feedback `2026-08-15-describe-replaces-right-pane.md` reported the cost of rendering the shared viewer as a centered inset: describe was the walk's primary diagnostic and its output needed scrolling immediately, inside a box deliberately kept 4 cells clear of the screen edge. The split this settles:
1. **The shared viewer (`components/viewer`) is a pane, not a popup.**
2. **D95 is unchanged for the modals.**
3. **The pane boundary is read off the rendered menu, never recomputed.**

### D285 — one health vocabulary: a surface paints through the M4-06 classifier, never its own word list (2026-08-21, STORY-06h-2)
The S04/S05 feedback `2026-08-15-rich-describe-panel.md` asked for a describe panel that surfaces the failure with colour instead of a plain dump. Painting text raises the question every non-table surface will now hit — *what counts as broken here?* — and there must be exactly one answer:
1. **`table.ClassifyValue(column, value)` is the single classifier.**
2. **A non-table surface maps its own labels onto the classifier's columns.**
3. **Painting adds colour and nothing else.**
4. **Painted content is palette state.**

### D286 — a relation is a scope or a name, resolved in one Get, and never an edge that cannot be opened (2026-08-21, STORY-06i-1)
The S04 feedback `2026-08-15-relations-navigation-popup.md` asks to move from a pod to its parent workload and back, and on to whatever a resource is linked to.
1. **A relation is either *named* or *set-shaped*, and the shape decides how it opens.**
2. **A relation's target `Resource` comes from the caller's discovered kind set, never synthesized.**
3. **`Relations` costs exactly one Get.**
4. **`RelationRole` is a display label; `RelationDirection` is the grouping.**

### D287 — navigating a relation reuses the two paths the shell has; `gr` opens the list (2026-08-21, STORY-06i-2)
D286 shaped the graph so its surface could be thin. This is the surface, and these constraints bind whatever extends it (STORY-06i-3's reverse-selector relations first):
1. **The gesture is `gr`, and it is a row action.**
2. **A relation opens through a path the shell already has, never a third one.**
3. **A cross-namespace hop re-scopes display state only.**
4. **Direction is the grouping, and the label is the identity.**
5. **No neighbours is a notice, not a popup.**

### D288 — the browse view's first frame is a table, and the menu lists only the kinds you asked for (2026-08-21, STORY-06l)
Two startup defaults, from the S01/S02 walk (`2026-08-15-land-on-pods-by-default.md`, `2026-08-15-hide-custom-resources.md`). Both are about the frame kubecom paints before the reader does anything, and both bind whatever changes it later:
1. **A launch, and a context switch, land on a table.**
2. **The resources pane lists a custom resource only when someone asked for it.**
3. **Holding back is a display rule, never an inventory one.**
4. **The displayed list is not addressable by index.**

### D289 — the logs view stays oldest-first; newest-first is declined, and reopens only on event grouping (2026-08-21, STORY-06j-3)
The S03 feedback `2026-08-15-logs-newest-first-order.md` (Priority normal) asked this to be *considered*, not done: reverse the buffer so the newest line is at the top with the filter box at the top, the way grafana and datadog render a live stream.
1. **Half the ask is already true, and the other half is what the follow rule bought.**
2. **It would invert every nav key in exactly one view.**
3. **Grafana reverses events; this pager addresses lines.**
4. **A reversal would split the screen from the clipboard.**
5. **The reopen condition is event grouping, not taste.**

### D290 — The screencast is also an asciicast, from the same run, and neuroplast.io reads it from `v1` (2026-10-03, CAST-01)
The maintainer wants the tour played on neuroplast.io by a terminal player of its own (a HOTTY surface drawing a terminal emulator's screen), with the GIF kept for the README.
1. **One tape, two recordings.**
2. **Captions are markers.**
3. **The cast's URL is a public contract.**
4. **D181 pt 1, amended.**

### D291 — Retain the vault as key facts: per-month journals, an open-only board, arguments in git (2026-10-06, vault cleanup)

The vault had grown to ~2.9 MB — 311 per-leg journal files (~2 MB), a `## Done` index of ~350 long entries, and a 600 KB decision log averaging ~28 lines per entry. Every leg reads these on Orient, so the cost was paid repeatedly while the placement of finished work bought nothing.

1. **Journals are per-month key-fact files.** `vault/journal/YYYY-MM.md`, newest last under a `### YYYY-MM-DD` heading; one bullet per leg (id, title, 1–3 sentence what/why, decisions). The verbatim per-leg history is in git before the cleanup commit.
2. **The board holds open work only.** A finished item is dropped, not indexed; its record is the journal bullet, the commit and the decision log. `internal/vault` fails `make check` if a `- [x]` reappears, beside the surviving D226 deferral guard.
3. **The decision log keeps each `Dn`'s binding statement and the lead of every rule; the argumentation lives in git.** Every `Dn` and every supersede pointer survives the compression.
4. **A milestone `Status:` line is current state, not history.**
5. **A leg id remains the join key (D15)** — commit subject, journal bullet, decisions.
**Refs:** supersedes the board-list half of D102, D224, D225 and D267; D226 pt 2 stands.

### D292 — kubecom publishes engram update channels; goreleaser is retired (2026-10-06, REL-01)

Maintainer direction: "AUR and homebrew should deploy a thin wrapper that gets updates from the engram channel"; follow margin, which did this first. The release path is now margin's, not goreleaser's.

1. **Two channels on `pkg.neuroplast.io/kubecom`.** Every push to `v1` that passes `make check` is a **dev** build; a tag named for its day (`YY.MM.DD`) is a **stable** release. Each is published with `engram publish` — one signed manifest per build naming the size and sha256 of bare `kubecom_<os>_<arch>` and `kubecom-launcher_<os>_<arch>` for linux/darwin × amd64/arm64 — and read back through the CDN by `engram verify` as `kubecom update` will. Dev retention 720h; stable forever.
2. **Bare binaries, not archives.** `kubecom update` puts the file in place of itself; there is nothing to unpack.
3. **Name and identity are margin's.** `Version` is CalVer (`YY.MM.DD` stable = the tag; `YY.MM.DD-dev.<sha7>` dev), `Commit` is the identity, `Channel` is stamped only by `make dist` so every local build is on none and updates to dev. `make dist` replaces goreleaser; `make release` tags HEAD with today's date and pushes.
4. **No secrets.** The publish assumes the OIDC role `github-pkg-publish-kubecom` (infra trusts `refs/heads/v1` and `refs/tags/??.??.??`), which may write under `kubecom/` and sign with `alias/release-signing`.
5. **Retired with goreleaser**: `.goreleaser.yml`, the AUR/cask/changelog config guards, and the goreleaser pin/dry-run guards. The remaining property — every `version` var is stamped — is guarded against the Makefile instead.
**Refs:** supersedes the goreleaser halves of **D175**, **D176**, **D185**.

### D293 — a package ships the launcher; the complete binary self-updates (2026-10-06, REL-02)

Extends D292, following margin's D20.

1. **`cmd/kubecom-launcher`** is `enlaunch.Main(channel.Launcher(version.Channel))` and nothing else; a package installs it as `/usr/bin/kubecom`. With `~/.local/kubecom/bin` present it execs it (same pid, arguments and terminal); with none it fetches the channel's newest, verifies it against the pinned key, lays it in `~/.local/kubecom/builds/<commit>/`, links `bin`, and hands over. It compares nothing.
2. **`kubecom update [commit]`** (internal/update) is in the complete binary. Started by a launcher (it sets `ENLAUNCH_HOME`) it `enlaunch.Install`s the build into the home and never rewrites the file it runs from; otherwise it replaces its own executable atomically (temp file beside it, fsync, rename). It never asks for root: an unwritable directory is an error that says to install somewhere the user owns.
3. **The server and key are pinned** in `internal/channel`; only a `-tags updatetest` build can move them, and a test without the tag proves the variables do nothing.
4. **Packaging follows** (ENGRAM-01/02): AUR and Homebrew install the launcher, not the complete binary — the previous cask/AUR shipped archives.

### D294 — an action may be palette-only (2026-10-06, REL-03)

`app.update` is registered in the keymap with **no default key**, reachable by typing `:update`; `keymap.PaletteOnly` names the exception and the help/keybindings guards subtract it. Updating is rare and hands the terminal over, so a bare key would invite an accidental fetch. The palette action suspends through `Model.suspend` and runs `internal/update` **in-process** — a child `kubecom update` would not see `ENLAUNCH_HOME`, which enlaunch unsets when read, and would replace the build binary instead of the launcher's home.

### D295 — The package ships a seed, and the launcher rides its version (2026-10-06, REL-05)

Amends D293. A launcher-only package makes every install depend on the channel:
with `~/.local/kubecom` empty, `pkg.neuroplast.io` down meant nothing to run, and
`pkgver` named a version the package did not contain (raised by the maintainer:
"if pkg.neuroplast.io goes down, all binaries just die").

1. **The package carries two files.** The launcher as `/usr/bin/kubecom`, and a complete kubecom binary as a **seed** at `/usr/lib/kubecom/bin/kubecom` — enlaunch's default `Config.Seed`. The launcher runs the seed while the home has no build, so a fresh install works offline and a channel outage stops only *updating*, not running.
2. **`pkgver` is the seed's version** — the build a user runs out of the box. `kubecom update` installs newer builds into `~/.local/kubecom`, which the launcher prefers from then on; the package version is the floor, self-update the ceiling.
3. **The launcher is rebuilt and version-stamped with every release**, reusing `DIST_LDFLAGS`, so `kubecom --launcher-version` names the packaged shim while `kubecom version` names the build actually running. It rides the seed's version rather than being byte-stable (supersedes D293 pt 1's "its bytes are its version", which was margin's D22 for a launcher-only package).
4. **The seed location is package-manager-specific** — `/usr/lib/kubecom` suits the AUR; Homebrew on macOS is not `/usr/lib` — so ENGRAM-02 sets it per package.
**Refs:** amends D293 (pt 3 supersedes its pt 1).

### D296 — A no-root install script drops the launcher and seeds the home (2026-10-07, REL-06)

`install.sh` installs kubecom into a directory the user owns, with no package
manager: it reads the channel's `head`, verifies the signed manifest against the
pinned release key (`ssh-keygen -Y verify`), fetches `kubecom-launcher_<os>_<arch>`
and `kubecom_<os>_<arch>`, checks each sha256, installs the launcher at
`${KUBECOM_BINDIR:-~/.local/bin}/kubecom`, and seeds the launcher's home
(`~/.local/kubecom/builds/<commit>/kubecom` + a `bin` symlink) so the first run
needs no network — the same launcher+seed combo the packages ship (D295), for a
user without root. An existing home build (a self-updated install) is kept unless
`KUBECOM_FORCE` is set. Channel, server, key and commit are overridable by
environment so a mirror or a test can be pointed at, but the key is pinned in the
script by default.

### D297 — The AUR publish runs in CI, in an Arch container (2026-10-07, REL-07)

Amends the "a human runs publish.sh" note. A stable release now publishes the AUR
package from `.github/workflows/aur.yml`: on a day-named release (and on
`workflow_dispatch` with a tag), it runs `packaging/aur/publish.sh <tag> <dir> --push`
inside `archlinux:base-devel` as a non-root `builder` user, because `makepkg`
refuses root and needs base-devel. It reads the private half of the AUR key from
the `AUR_SSH_PRIVATE_KEY` repository secret and **skips** when it is absent, the
way every other publisher here degrades rather than failing a release. The
build-before-push validation (makepkg against the GitHub release) is unchanged;
only who invokes it moved. The human-run path still works for a re-publish.

### D298 — The rewrite is `main`, and the default branch (2026-10-07, M5-11)

The maintainer directed the move: `v1` is pushed to `main`, and `main` becomes the
repository's default branch. `master` stays as the 2020 reference (D14, do not
delete). The unblock was the first stable release (26.10.06), which the rename
was waiting for.

1. **All triggers say `main`.** `release.yml` runs the dev publish on pushes to `main` and the stable release on a day tag; `ci.yml`/`envtest.yml` gate `main`; the Makefile's `make release` checks `origin/main`.
2. **`publish_branches` is empty again** — the infra publisher role trusts `refs/heads/main` for kubecom like every other project; the `v1` override is gone (applied through `neuroplastio/infra`).
3. **The leg loop, skills, vault branch model and docs say `main`.** An installer or doc URL that named `v1` now names `main`.
4. **Not deleted:** `v1` may be deleted once nothing points at it; the neuroplast.io screencast cast URL (D290 pt 3) is the remaining external reference to move.
**Refs:** closes M5-11; supersedes the branch model of D9 (no behavior change, the branch is renamed).

### D299 — Homebrew ships the launcher plus a seed, on the org tap (2026-10-07, REL-09)

`brew install neuroplastio/tap/kubecom` installs the same launcher+seed combo the
AUR package does (D295): the launcher as `bin/kubecom` and a complete build in
`libexec`, with a small wrapper that **seeds the launcher's home** from that build
on first run. The AUR can use enlaunch's default seed path (`/usr/lib/<project>`);
Homebrew's prefix is not `/usr/lib`, so the wrapper writes `~/.local/kubecom` — the
home the launcher already reads — and leaves an already-updated install alone.
`packaging/homebrew/{kubecom.rb,publish.sh}` render `Formula/kubecom.rb` from a
release's eight binaries (version, commit, and each sha256) and push it to
`neuroplastio/homebrew-tap`; a `homebrew` job in `release.yml` does it on a day tag
via `HOMEBREW_TAP_TOKEN`, skipping without it. The seed's build directory is named
for the release commit (40 hex), so enlaunch's home layout and pruning stay
correct. Published for 26.10.07, the first formula; the tap's default branch is
`master`.

### D300 — The rewrite is over: milestones retired, the plan is current, the vault keeps key facts (2026-10-07, vault cleanup)

The maintainer asked for a fresh start: "drop completed items, rewrite plan doc,
… without losing key work. Drop all human tasks."

1. **Milestones are retired.** M0–M5 described a rewrite that is finished and
   shipped; a milestone file that can never advance is history, not a plan. The
   five files and `milestones/README.md` are deleted, and `CLAUDE.md`, the leg
   skill, the vault README and the board no longer know the word. Their content,
   including the box-by-box Definition-of-Done audit, survives in this log, the
   journal and git (D291).
2. **`PLAN.md` replaces `REWRITE_PLAN.md`.** The old file planned the rewrite
   (protobuf removal, the `v1` branch, five phases); the new one states the
   architecture as built, the channel/launcher release model, and the remaining
   roadmap. History stays in this log, not in the plan.
3. **`goals.md` is compressed** to the vision, the shipped definition of done,
   the two administrative leftovers, the non-goals and the principles. The
   per-box audit annotations are gone.
4. **`human-tasks/` is empty.** The open packaging/release tasks were retired
   with the rewrite's end; git keeps them if they are needed again.
5. **The board is open work only**, grouped by workstream rather than milestone
   (D291 stands).
6. **The branch model says `v0`**, reflecting the 2026-10-07 rename of `master`.
**Refs:** extends D291 to the milestones and the plan; supersedes `REWRITE_PLAN.md`.

### D301 — kubecom.neuroplast.io serves the installer; the repository's install.sh forwards to it (2026-10-10, REL-12)

The maintainer asked for kubecom's website ("kubecom.neuroplast.io website,
xterm-driven, simple layout, hotty vt player with a kubecom recording") and
for the install script to be hosted there. The site is `kubecom/` of the
private web monorepo (neuroplastio/web), a folder of neuroplast.io's one
distribution (infra `sites`, dccb517).

1. **The installer's source moves to the site,** `web/kubecom/web/static/install.sh`,
   as HOTTY's `run.sh` lives in its site: the one-liner is
   `curl -fsSL https://kubecom.neuroplast.io/install.sh | sh`, deployed with
   the site, sent as text so a browser shows it. Its behaviour is D296's,
   unchanged: the channel's head, the manifest checked against the pinned
   release key, the launcher plus a seed.
2. **`install.sh` in this repository forwards.** It fetches the site's
   installer and runs it with the same environment, so the old
   raw.githubusercontent.com one-liner keeps working; it holds no logic, so
   there is one installer.
3. **The pinned key is now in three places:** `internal/channel`,
   `release.yml` (`ENGRAM_SIGNERS`) and the site's installer. A key
   rotation changes all three.
4. **The site plays the screencast** from `docs/screencast.cast` on `main`
   (D290), with the player neuroplast.io uses (web `shared/screencast`), and
   neuroplast.io now reads the cast and the GIF from `main` too: D298 pt 4's
   last external `v1` reference is gone.
**Refs:** amends D296 (where the script lives); closes D298 pt 4.

### D302 — Config is KDL: an authored `config.kdl` kubecom never writes, an overlay it does, and KDL state (2026-10-10, KDL-01)

The maintainer: *"See how plexos implemented configuration files using .kdl —
I need kubecom to move to .kdl."* plexos's model (its D75/D83) is taken
whole, so one person's two tools read one grammar.

1. **Three kinds of file**, all in the D20 directory
   (`os.UserConfigDir()/kubecom`), all starting `version 1` (a newer
   version is refused, naming the file):
   - `config.kdl` — the person's. kubecom reads it and **never writes it**,
     so its comments, order and layout stay theirs.
   - `config.overlay.kdl` — kubecom's. Every write goes here (the theme
     picker today; the palette's key bindings later), merged on top of
     `config.kdl` key by key. A write changes one node and keeps the rest.
   - `state/<context>.kdl` — per-context runtime state (D90 stands: kubecom
     rewrites it freely).
   `--config-dir` replaces `--config`: the unit is the directory, because
   the overlay sits beside `config.kdl`.
2. **The grammar is plexos's:** a key is a dotted path through nodes
   (`theme "monokai"`); a value is a node with one argument; a group has
   children; a list is `-` items carrying properties. Two **documents** are
   handed to their reader whole: `keymap { … }` (D303), which the overlay
   may hold too, and `menu ["<context>"] { resource "<plural>" group=…
   version=… kind=… namespaced=… section=… title=… }`, which replaces
   `menus/<context>.yaml`. A `menu` without a context applies to every
   context; a context's own entries come after those.
3. **No YAML.** `config.yaml`, `menus/*.yaml` and `state/*.yaml` are no
   longer read. When `config.yaml` or `menus/` is there and `config.kdl` is
   not, the start-up notice names them and says what replaced them. There
   is no converter: the maintainer is the only user, and the old `keys:`
   overrides do not map one-for-one onto D303's bindings.
4. **The 2020 migration (D6) stays, retargeted:** a carried-over theme is
   written to the overlay, and it runs when neither `config.kdl` nor the
   overlay exists.
5. **Library:** `github.com/calico32/kdl-go` v0.16.0, parsed strictly as KDL
   v2 — plexos's choice (its D83), for its line/column errors and a
   formatter that keeps comments. Its API is 0.x; `internal/config` is its
   only importer besides the keymap's parser.
**Refs:** supersedes the PLAN's "plain YAML" locked decision and D90's file
format (not its split); amends D6's target.

### D303 — Key bindings are a context-bound keymap: CEL contexts over a focus path, layered precedence (2026-10-10, KEYS-01)

The maintainer: *"support contextual key bindings, similar to plexos."*
plexos's D119 model, with kubecom's own path. It replaces the flat browse
map and its two side tables (`ConfirmAction`, `TableAction`). The `Action`
registry and the D11 rule that views switch on actions, never on keys,
stay.

1. **A `keymap` document**, in three layers: `default.kdl` (embedded,
   hand-written; kubecom's registry is fixed, so there is nothing to
   generate it from), then `config.kdl`'s, then the overlay's (both the
   user layer, in that order). Nodes: `bind "<keys>" "<action>" [value]
   [label=]`, `unbind "<keys>"`, `group "<keys>" "+label"`, `context
   "<cel>" { … }` (nested blocks are joined with `&&`), `leader "<key>"`
   (none by default; `<leader>` with no leader named is a load error),
   `base "kubecom"|"none"`. Keys are written as plexos writes them,
   separated by spaces: `g g`, `ctrl+d`, `H` (= `shift+h`), `pgdn`,
   `space`. The old concatenated `gg` is not read. CEL strings take
   single quotes, so nothing is escaped.
2. **A value runs an argument verb with it**: `bind "g p"
   "resources.switch" "pods"` switches straight to pods, while no value
   opens the palette stage as the key does today. Only the palette's
   argument verbs (`ns.switch`, `resources.switch`, `ctx.switch`,
   `theme.switch`, `menu.pin`, `actions.menu`) take one, and a value on
   any other action is a load error.
3. **The context is the path of what holds the keyboard**, outermost first.
   Each node is a boolean, and its attributes are qualified names under it.
   An attribute of a node that is not on the path is absent, and a misspelt
   one is a load error:
   - `cluster` (`context`, `namespace`; "" = all), on the path while a
     cluster is connected;
   - `menu`, or `table` (`resource`, `group`, `kind`, `namespaced`,
     `drilled`, `filtered`, `unhealthy_only`), whichever pane holds focus;
   - `sort`, the table's column-header mode;
   - at most one view opened over the pane: `logs` (`follow`, `wrap`,
     `timestamps`, `previous`, `selecting`), `viewer` (`content`:
     `describe`|`events`|`secret`), `search` (`all_kinds`,
     `all_namespaces`), `unhealthy`;
   - at most one overlay: `forwards`, `help`, `picker` (`id`:
     `palette`|`containers`|`ports`|`relations`; `stage`, the palette's
     committed verb), `prompt`, `confirm`;
   - `field`, a text field holding the keyboard (the menu or table filter,
     the logs grep, a picker's query, the search query, a prompt).
   **`focus` is the innermost node's name** (kubecom's addition): `context
   "table"` holds anywhere above the table, so a confirm over it still sees
   it, while `context "focus == 'table'"` holds only while the table itself
   has the keys. That is how `enter` opens the actions menu on a row and
   nowhere else. Spelling out every overlay with `!` would break each time
   one is added.
   `context "cluster.context.startsWith('prod-')" { unbind "D" }` is the
   kind of rule this is for.
4. **Precedence (plexos's):** the deepest context wins, then the layer
   (user over default), then the later line. An exact binding that is also
   a prefix of a longer one waits only for longer bindings at least as deep
   as itself, and runs when the sequence times out.
5. **Fields keep their text** (kubecom's own rule; D140 pt 1 becomes the
   keymap's): with `field` on the path, a key that types text goes to the
   field unless a binding at least as deep as `field` takes it. A user's
   global `bind "x" …` can never break typing `x` into a filter, and the
   `key.Text == ""` checks in the routes go.
6. **The defaults first reproduce today's keys exactly.** A golden of every
   surface × key, written against the old map before the swap, is the proof.
   Moving a key now that a context can free its letter is a later UX
   decision, not part of the port.
7. **Hints, the `?` overlay and `docs/keybindings.md` read the live path.**
   The curated hint sets still choose *which* actions to show, and the keys
   come from the bindings that win there. `kubecom keys` lists bindings with
   their context and `file:line`; `kubecom keys default` prints
   `default.kdl`, and `kubecom keys check` validates the user layer.
8. **Ported, not imported.** plexos is private and kubecom is public, so
   kubecom carries its own copy of the engine (parse, CEL context, match).
   A shared public module is the later home if a third tool wants it.
   Matching is on kubecom's canonical chords, not on `tea.Key`: the TUI
   adapts its keys, and an A2UI/HOTTY front end will build its path from its
   own focus tree and feed the same keymap.
**The cost of CEL**, measured by plexos (its D119): +6.8 MB stripped and
~1 ms more at start; matching is microseconds, and a key no binding starts
with is one map lookup.
**Refs:** supersedes the flat-context half of D11's mechanism (not its rule)
and D132's side contexts; restates D140 pt 1 as pt 5; D10's vim-first
defaults stand.
