# Decision Log

Append-only. Supersede rather than delete; note the date. Newest at the bottom.

---

### D1 — TUI framework: Bubble Tea
**2026-07-18.** Use **Bubble Tea + Bubbles + Lipgloss** (Elm architecture).
**Why:** the original's top defect class — data races, focus bugs, popup/redraw
issues — comes from driving `tcell/views`' imperative widget tree from
goroutines. Bubble Tea's single-threaded update loop removes that class by
construction. tview (what k9s uses) was considered but keeps the imperative
model. **Consequence:** no shared mutable UI state; concurrency via messages.

### D2 — In-process client-go, shell out only for interactivity
**2026-07-18.** Reach for client-go first; shell out **only** for exec shell and
`$EDITOR`. **Why:** removes the hard `kubectl` binary dependency (**#68**),
enables background port-forward, and in-TUI logs/describe/YAML. Modern client-go
covers logs/describe/portforward/remotecommand/actions. Push in-process "until we
hit a wall."

### D3 — Config: plain YAML
**2026-07-18.** Drop the protobuf config (`pb/config.proto` + generated code) for
a plain typed Go struct marshalled to YAML. **Why:** the protobuf machinery and
its abandoned theme engine were over-engineered for a small local file.

### D4 — Name: `kubecom`, single binary
**2026-07-18.** Standardize on `kubecom`; retire the duplicate `kube-commander`
binary. **Why:** the original shipped two identical entrypoints and inconsistent
naming.

### D5 — Kubernetes/client-go version
**2026-07-18.** Build against a recent client-go (**target v0.31 / K8s 1.31**);
support servers **~1.27+** via discovery so it degrades gracefully. "Recent, not
too aggressive."

### D6 — Backwards compatibility: clean break + migration
**2026-07-18.** No runtime BC with the old app; instead a one-shot **migration**
of the old `~/.kubecom.yaml` on first start. **Why:** clean architecture beats
carrying legacy config semantics.

### D7 — Platforms: Linux + macOS only (WSL2 for Windows)
**2026-07-18.** Drop native Windows support; recommend **WSL2**. **Why:** removes
Windows PTY/exec complexity (a flagged risk) for a niche of the user base.

### D8 — Async, cached resource discovery
**2026-07-18.** First-paint from a seed set of core GVKs; run full discovery in
the background and reconcile the menu via a "discovery ready" message; isolate
per-group failures; cache discovery on disk. **Why:** the original blocked the
whole UI on `ServerPreferredResources()`, hurting cold starts and turning one bad
API group into a total failure (**#87**, **#76**, **#86**).

### D9 — Branch model
**2026-07-18.** `master` = original code, untouched for now. `v1` = rewrite
branch, holds this vault and all rewrite work. A `main` branch becomes the final
destination when the rewrite is ready to be default. Push `v1` to origin.

### D10 — Vim-style navigation first-class; arrows/classic as fallback
**2026-07-18.** Navigation is **vim-first**: `hjkl`, `gg`/`G`, `Ctrl+u`/`Ctrl+d`,
`/` search + `n`/`N`, `l`/`Enter` to drill in, `h`/`Esc` to go back. Arrow keys,
`PgUp`/`PgDn`, `Home`/`End`, `Enter`/`Esc` work as an equivalent **fallback** so
non-vim users are never stranded. **Why:** the target user is a terminal-native
hacker; muscle-memory navigation is a core UX value, not an add-on.
**Consequence / rule:** `h j k l n` (and `g`, `G`) are the **default**
navigation keys — single-letter *action* defaults must not collide with them.
This supersedes the legacy pod bindings where they clash (legacy used `l` for
logs, `s` shell, `f` port-forward): default actions off the nav keys (e.g. behind
a leader or an actions menu). All of this is *default*, not fixed — see D11. See
[`keybindings.md`](keybindings.md).

### D11 — Fully configurable keybindings; zero hard-coded keys
**2026-07-18.** **No key literal is ever matched in view/update code.** Every
user-triggerable behavior is a named **Action**; an **action registry** maps
`Action → []key` and is the *only* place keys exist. The registry is built from a
**default keymap** (data in one place, expressing the vim-first scheme of D10)
overlaid by the user's config, then consulted by every view to resolve
`tea.KeyMsg → Action`. **Why:** users must be able to rebind anything, and
scattered `case 'l':`-style handling is exactly what made the old code rigid.
**Consequences:**
- Config carries a `keys:` section: `action -> [keys]` overrides merged onto defaults.
- `bubbles/key.Binding`s are constructed *from* the resolved keymap, not literals.
- Loading **validates**: unknown action names error; within-context collisions
  error with a clear message; the vim/nav defaults are just defaults a user may
  override (with a warning if they shadow navigation).
- The help overlay and the generated keybindings doc are **derived from the
  registry**, so they can never drift from actual bindings.
See [`keybindings.md`](keybindings.md).

### D12 — golangci-lint scoped to new code only
**2026-07-18.** `.golangci.yml` lints **only** the rewrite (`cmd/kubecom`,
`internal/...`); the legacy 2020 trees (`app/`, `cli/`, `commander/`, `config/`,
`pb/`, `cmd/kube-commander/`) are excluded. **Why:** those trees are deleted
tree-by-tree across M1–M3 — linting dead code is pure noise, and the maintainer
asked for lint to apply to new code only. **How:** golangci-lint **v2** with
`run.relative-path-mode: gomod` (normalizes match paths to module-relative so the
`^`-anchored excludes hit only the top-level legacy dirs — `^config/` excludes
legacy `config/` but not `internal/config/`) and `linters.default: standard`
(lenient ruleset per M0; tighten later). **Consequence:** remove an exclude entry
when its tree is deleted; new packages are linted by default.

### D13 — `kubecom` binary is the new skeleton; legacy reachable via `kube-commander`
**2026-07-18.** `cmd/kubecom` now builds the **new** binary (M0 skeleton:
`version`/`help`); the legacy 2020 app stays reachable **only** via
`cmd/kube-commander` until it is ported (M1–M3), after which M0-03 deletes it.
Both were identical `cli.Run()` entrypoints (the duplicate D4 flagged). The M0
skeleton uses a **stdlib** command dispatch for now; **cobra** replaces it in
M0-02 (kept out of this leg to avoid a premature go.mod/dependency bump).
**Why:** makes `kubecom` the single forward binary immediately while keeping the
old code compiling in parallel, per the M0 plan.

### D14 — Legacy trees are deleted from `v1` up-front, not kept compiling
**2026-07-18.** Maintainer-approved (setup review). Delete `app/`, `cli/`,
`commander/`, `config/`, `pb/`, and `cmd/kube-commander/` (plus Windows sources
and dead Travis/snap CI) from `v1` in an early M0 leg, pruning `go.mod`.
**Why:** the original "keep old trees compiling until ported" plan is unworkable
in a single Go module — the legacy code imports `k8s.io/*@v0.18` while the new
kube layer needs client-go v0.31, and one module cannot hold both. The clean
break (D6) means zero code reuse, and `master` preserves the old code forever.
**Consequence:** legacy behavior reference = `master` + `git show master:<path>`
+ [`legacy-architecture.md`](legacy-architecture.md). Supersedes the "compiling
in parallel" parts of D12/D13: `.golangci.yml` drops the path excludes once the
trees are gone (the `standard` ruleset stays), and M0-03 (remove duplicate
binary) is absorbed by the deletion leg.

### D15 — Journal format: one file per entry; no Commit field; milestones kept current
**2026-07-18.** Maintainer-approved. The journal is a directory,
`vault/journal/`, with one file per leg named `YYYY-MM-DD.N.md` (`N` = sequence
within the day; filename sort == chronological order). The `Commit:` field is
dropped — the entry is written before the commit exists; the **leg id in the
commit message** is the join key between journal, board, and git history. Every
leg also keeps the active **milestone file** current: tick exit criteria as they
are met and flip its `Status:` line.

### D16 — Board claims are committed and pushed immediately
**2026-07-18.** Maintainer-approved. Claiming a task (step 3 of the leg loop) is
its own commit (`chore(board): claim <leg-id>`) pushed to `v1` **before**
implementation starts. **Why:** a claim that lands only with the finished leg is
invisible to concurrent agents and locks nothing; pushing it first makes the
board a real mutex.

### D17 — `make check` is the canonical gate; CI lands early in M0
**2026-07-18.** Maintainer-approved. A `Makefile` with `check` (= build + test +
vet + lint) is the single verify gate used by agents and CI, so "green" means
the same thing everywhere. M0 is reordered: legacy deletion → toolchain bump →
**CI (M0-04)**, before any further feature legs — for an autonomous process
pushing straight to `v1`, CI is the only independent green check a reviewer has.

### D18 — Tests: fake clients by default; envtest opt-in, deferred to M1
**2026-07-18.** Maintainer-approved. Kube-layer tests use client-go **fake
clients** (incl. fake discovery) by default so `go test ./...` is hermetic and
runs anywhere. **envtest** integration tests are opt-in behind an env var
(`KUBECOM_TEST_ENVTEST=1`) because envtest downloads control-plane binaries —
fragile in sandboxed agent environments and costly in CI. The envtest harness
moves from M0-05 to M1 (where there is a kube layer to integration-test);
M0-05 keeps only the teatest smoke test.

### D19 — Bubble Tea v2
**2026-07-18.** Maintainer-approved. Prefer **bubbletea v2** (with matching
bubbles/lipgloss releases) when dependencies land (M0-02/M2). Pin v2 and write
all TUI code against its API; fall back to v1 only if v2 proves unusable in
practice, recorded as a superseding decision. **Why:** avoids building parity UI
on an API that is being replaced, and avoids mixing v1 examples with v2 code.

### D20 — Config path: `os.UserConfigDir()/kubecom/config.yaml`
**2026-07-18.** Maintainer-approved. The config lives at
`os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on
Linux, `~/Library/Application Support/kubecom/config.yaml` on macOS) — not
inside `~/.kube/`, which other tooling treats as kubeconfig-shaped. The one-shot
migration (D6) reads the legacy config from its old location once.

### D21 — Scheduled runs batch legs via fresh subagents (`/do-rewrite-run`)
**2026-07-18.** Maintainer-approved. The scheduled routine invokes
**`/do-rewrite-run`**, an orchestrator that sequentially spawns a **fresh
subagent per leg**, each executing exactly one `/do-rewrite-leg`. Budgets: max
4 legs per run; no new leg after 90 minutes (runs occupy the last ~2h of the
5-hour usage window); stop on any failure without retrying. **Why:** one leg
per invocation stays the rule (reviewability), while a routine run can use its
whole window; per-leg subagents keep context from accumulating across legs —
the alternative (`/loop` in one session) grows context unboundedly.
**Consequence:** legs must stay strictly sequential — claims + pushes to `v1`
would collide if parallelized.

### D22 — Executing D14: legacy trees gone; `.goreleaser.yml` patched to keep working, not redesigned
**2026-07-18.** M0-07 deleted `app/`, `cli/`, `commander/`, `config/`, `pb/`,
`cmd/kube-commander/`, `.travis.yml`, and the snap CI files (`ci/snap-deps.sh`,
`ci/snap.login.enc`); pruned `go.mod`/`go.sum` to empty via `go mod tidy` (no
external import remains until M0-02/M1 reintroduce cobra/client-go); and dropped
the now-unneeded `.golangci.yml` path excludes (D12's exclusion list has no
targets left). `.goreleaser.yml` referenced the deleted `cli.version` symbol and
the deleted `cmd/kube-commander` binary/snap craft — fixed the ldflags to target
`internal/version.Version` (the var already designed for this) and removed the
`kube-commander-linux` build id and the `snapcrafts:` block, since both only
existed to package the now-gone legacy binary. **Scope note:** the `kubecom-windows`
build target and the `aur`/`brews` publishers were left as-is — untangling the
release matrix into the Linux+macOS-only shape (D7) is M0-06's job, not this
leg's; this decision only covers unblocking what the deletion itself broke.
**Why not fold into M0-06 now:** keeping M0-07 to "delete + keep buildable" is a
smaller, safer diff than also redesigning the release config in the same leg.

### D23 — Executing M0-02: Go 1.23 floor, cobra v1.10.2 root command
**2026-07-18.** Toolchain bump landed (M0-02): `go.mod` `go` directive set to
**1.23** (the stack floor, not the local 1.24.x — a `go 1.23` module still builds
on newer toolchains), and `cmd/kubecom` rewired from the hand-rolled `run(out,
args)` dispatch onto **cobra v1.10.2** (D13's planned replacement). Shape:
`newRootCmd()` (a constructor, not a package var, so tests get isolated I/O +
args) with a `version` subcommand; `--version`/`-v` are cobra's built-in version
flag with a custom `SetVersionTemplate("{{.Version}}\n")` so it prints
`version.Info()` verbatim instead of cobra's `"<name> version <version>"` line
(Info() already leads with "kubecom"). `SilenceUsage: true` keeps runtime/arg
errors terse (no usage dump). Behavior parity + extras: `version` prints Info();
no-args prints help (exit 0); unknown command errors on stderr (exit 1); cobra
adds `help` + `completion` subcommands for free. `ioutil` was already gone
(M0-07), so that clause was a no-op. New root subcommands / the default TUI run
hang off this root in M2+. **Why 1.23 not 1.24:** pin the minimum the stack
commits to (D5-adjacent), keeping the module buildable on the widest toolchain
range; bump only if a dependency forces it.

### D24 — CI runs `make check` verbatim; golangci-lint installed, not action-run
**2026-07-18.** M0-04 landed `.github/workflows/ci.yml`: a single `check` job on a
`[ubuntu-latest, macos-latest]` matrix (D7) that runs **`make check`** as one
step — the same gate agents run locally (D17), so "green" is identical in both
places. golangci-lint is installed via the project's official `install.sh`
pinned to `GOLANGCI_LINT_VERSION` (v2.5.0, matching the local tool) and added to
`PATH`, **instead of** `golangci-lint-action`. **Why not the action:** the action
runs the linter *itself* and would split verification into "action lints + make
does the rest", so CI would no longer be `make check` end-to-end — the whole
point of D17 is one gate with one meaning. Go comes from `setup-go` with
`go-version-file: go.mod` so the floor has a single source of truth (D23);
`check-latest: true` takes the newest patch of that minor. Triggers: push +
pull_request on `v1`/`main`; `concurrency` cancels superseded runs;
`permissions: contents: read` (least privilege — CI only reads the tree).
**Consequence:** bumping golangci-lint means updating both the local tool and
`GOLANGCI_LINT_VERSION`; the setup-go build cache is the only caching (no lint
cache from the action) — acceptable while the tree is tiny, revisit if CI slows.

### D25 — Executing M0-08: legacy-file sweep; `.goreleaser.yml` de-referenced from deleted `ci/aur/`
**2026-07-18.** M0-08 deleted the legacy files M0-07's tree-based deletion
missed: `Dockerfile` (golang:1.15 + baked-in kubectl, contradicts D2), `get.sh`,
`ci/aur/` (old binary names `kube-commander`/`kubectl-ui`, `PKGBUILD`/`.SRCINFO`
templates, `publish.sh`, and the encrypted deploy key `id_rsa.enc`), and
`ci/terminalizer/` (asciicast recorder — vhs replaces it in M5). The now-empty
`ci/` directory went too, including its stale `ci/.gitignore` (`/snap.login`, a
leftover from the snap CI already removed in M0-07/D22). Following **D22's
precedent** (keep config consistent, defer the redesign), `.goreleaser.yml` was
minimally patched to drop the two blocks that *exclusively* referenced the
deleted `ci/aur/`: the `aur` archive and the `publishers:` section (its only
entry ran `ci/aur/publish.sh`). Everything else in the release config — the
`kubecom-windows` build target, the `brews` tap, the `kubectl` dependency — is
**left as-is for M0-06** to reshape into the Linux+macOS-only matrix (D7); this
leg only removed references the deletion itself broke, exactly as D22 scoped
M0-07. The AUR badge + install section in `README.md` are docs, not file refs,
and are M0-06/M5's to revise. `make check` unaffected (goreleaser isn't part of
the gate); `.goreleaser.yml` re-validated as well-formed YAML.

### D26 — Executing M0-05: bubbletea v2 adopted; Go floor bumped to 1.24.2; placeholder root model
**2026-07-18.** M0-05 landed the teatest smoke harness, which required the first
real TUI dependencies. **Choices:**
- **bubbletea `charm.land/bubbletea/v2 v2.0.2`** (not the newest v2.0.8). The v2
  module was **rebranded** from `github.com/charmbracelet/bubbletea/v2` to
  **`charm.land/bubbletea/v2`** — the github path no longer resolves at stable v2
  tags. Import path is now `charm.land/bubbletea/v2`. teatest v2 stays at
  `github.com/charmbracelet/x/exp/teatest/v2`.
- **Go floor 1.23 → 1.24.2** (supersedes the go-directive part of D23). bubbletea
  v2 *forces* a bump — D23 itself reserved this ("bump only if a dependency forces
  it"). v2.0.0–v2.0.2 require `go 1.24.2`; **v2.0.3+ jump to `go 1.25.0`**. Pinned
  **v2.0.2** — the newest v2 that keeps the floor at **1.24.2**, the minimal bump
  (widest toolchain range) — rather than chasing v2.0.8/1.25. Bump further only
  when a dependency forces it. No `toolchain` directive is added, so CI's
  `setup-go` (`go-version-file: go.mod`, `check-latest`) installs the latest
  1.24.x (D24 unaffected).

### D27 — Executing M0-06: goreleaser skeleton is artifact-only; publishers deferred to M5; Linux+macOS × amd64+arm64
**2026-07-18.** M0-06 reshaped `.goreleaser.yml` into the release **skeleton** D22
and D25 deferred to this leg. **Choices:**
- **`version: 2`** header + v2 syntax throughout (the config was still v1-shaped):
  `archives[].builds` → `ids`, `archives[].format` → `formats: [...]`,
  `snapshot.name_template` → `version_template`. Validated with `goreleaser check`
  (v2.17.0) — clean, no deprecations.
- **Windows dropped** (D7): the three per-OS build blocks (incl. `kubecom-windows`)
  collapse into a **single `kubecom` build** with `goos: [linux, darwin]` ×
  `goarch: [amd64, arm64]`. `goreleaser release --snapshot` produces exactly four
  artifacts (linux/darwin × amd64/arm64) as `tar.gz` + raw binary + `checksums.txt`.
- **arm64 added** (was amd64-only). Apple Silicon is arm64 and arm64 Linux servers
  are common; a modern Linux+macOS skeleton must cover it. Cheap, cross-compiled,
  `CGO_ENABLED=0`.
- **All publishers removed from the skeleton — deferred uniformly to M5.** The
  `brews` block (with its `kubectl` dependency, which contradicts **D2**) is
  **deprecated** in goreleaser v2 in favour of Homebrew **casks**; rather than
  migrate a publisher M5 must verify anyway (it needs the `homebrew-kubecom` tap to
  exist), the block was dropped. M5 owns *all* distribution (`#28`: Homebrew, AUR,
  Docker) per its milestone scope, so the skeleton stays purely artifact-building
  (builds/archives/checksum/release/changelog). A comment in the file points to M5.
- **README not touched.** Its Travis/AUR/Docker badges and install section are docs,
  not release-config refs; revising them is M5's job (README rewrite is M5 scope).
  Keeping M0-06 to the `.goreleaser.yml` skeleton keeps the leg one logical change.
**Why not fold publishers in now:** a publisher that can't run (no tap/registry) is
not a working skeleton, and `goreleaser check` green is the leg's gate. Supersedes
the `brews`/`kubecom-windows`/amd64-only parts of the M0-07/M0-08 stopgap config.
- **Placeholder root model** (`internal/tui/tui.go`): renders a static splash,
  records `WindowSizeMsg`, and **matches no key literals** — input must flow
  through the M2 action registry (D11), so the smoke test stops the program via
  `tea.Quit`, not a keystroke. It exists solely to give teatest a real program to
  drive; M2 replaces it with the action-registry-driven root model.
- **bubbletea v2 API notes:** `Init() tea.Cmd`, `Update(tea.Msg) (tea.Model,
  tea.Cmd)`, and **`View() tea.View`** (not `string`) — build views with
  `tea.NewView("...")`. Key presses arrive as `tea.KeyPressMsg` (v1's `KeyMsg`
  split into press/release). teatest v2: `teatest.NewTestModel(t, m,
  teatest.WithInitialTermSize(w,h))`, then `WaitFor(t, tm.Output(), cond,
  WithDuration(...))` and `tm.WaitFinished(t, WithFinalTimeout(...))`.

### D28 — Executing M1-00: envtest harness lands the kube dependency graph; pinned client-go v0.31 / controller-runtime v0.19
**2026-07-18.** M1-00 added the opt-in envtest integration harness (D18) and, with
it, the first client-go dependencies (the kube layer's foundation). **Choices:**
- **Versions pinned: `k8s.io/client-go` + `k8s.io/apimachinery` v0.31.4,
  `sigs.k8s.io/controller-runtime` v0.19.4.** client-go v0.31 is the D5 target
  (K8s 1.31); controller-runtime **v0.19.x is the release paired with client-go
  v0.31** (v0.20+ jumps to v0.32), so v0.19.4 keeps envtest and client-go on the
  same minor. All compatible with the Go 1.24.2 floor (D26). These are the first
  `k8s.io/*` deps on `v1`; later M1 legs build the real `internal/kube` on them.
- **Gate = `KUBECOM_TEST_ENVTEST=1` (const `envtestGateEnv`), guard = one
  `requireEnvtest(t)` helper every envtest test calls first.** With the gate unset,
  `go test ./...` (so `make check`, D17) **skips** — the default suite stays
  hermetic and needs no control-plane binaries. Verified both paths: gate off →
  `SKIP`/green; gate on without binaries → clean `t.Fatalf` (no panic), pointing at
  the missing etcd binary. envtest is **never** in `make check`.
- **Binaries via `setup-envtest`, run via `make test-envtest`.** envtest needs a
  real kube-apiserver + etcd on disk (`KUBEBUILDER_ASSETS`); the new Makefile target
  fetches them (`ENVTEST_K8S_VERSION ?= 1.31.x`) and runs the gated suite. Not
  wired into CI here — an M1/M5 leg adds an envtest CI job once the kube layer has
  integration tests worth running there.
- **Smoke test = start control plane → clientset → GET the `default` namespace.**
  Minimal end-to-end proof the apiserver is reachable; later M1 legs reuse this
  bootstrap to integration-test discovery, watch reconnect, and actions.

### D31 — Executing M1-03: async full discovery delivers a one-shot reconcile signal; per-group failures isolated
**2026-07-18.** M1-03 landed `internal/kube/discovery.go`: the background-discovery
half of D8. **Choices:**
- **`ServerPreferredResources`, not `ServerGroupsAndResources`.** The menu wants one
  entry per resource at its server-preferred version (`deployments` once, not per
  served version), which is exactly what `ServerPreferredResources` returns. The
  original blocked the whole UI on this same call; here it runs off the caller's
  path.
- **Per-group fault isolation via `*discovery.ErrGroupDiscoveryFailed` (#87, #76).**
  That call returns the resources it *could* load **together with** an
  `ErrGroupDiscoveryFailed{Groups: map[GV]error}` for the ones it couldn't. We
  `errors.As` it, keep the partial `lists`, and record each failed group in
  `DiscoveryResult.Failed` — so a broken/denied aggregated API (the classic
  metrics-server outage) degrades only itself instead of blanking the menu, the
  original's central bug. Any *other* error (unreachable server, auth) is a total
  failure surfaced in `DiscoveryResult.Err` with empty results, so the caller
  retries rather than reconciling an empty menu. A malformed `GroupVersion` string
  isolates that one list, not the pass.
- **One-shot buffered channel = the "discovery ready" reconcile signal.**
  `StartDiscovery(ctx, d) <-chan DiscoveryResult` runs the pass in a goroutine and
  delivers the snapshot exactly once on a cap-1 channel, returning immediately —
  never blocks the caller (fast cold start; the seed mapper already covers core
  kinds). The channel is buffered so the sender never leaks if the caller stops
  listening, and a cancelled ctx drops the send. **Repeated/periodic re-discovery
  and the on-disk cache are M1-04**, not this leg. The TUI (M2) turns the received
  `DiscoveryResult` into a reconcile `tea.Msg`; the kube layer keeps **zero TUI
  imports**.
- **Menu filter: listable, non-subresource only.** Subresources (`pods/log`) and
  create-only resources (`tokenreviews`, `subjectaccessreviews` — no `list` verb)
  are dropped; you browse what you can list. Each `Resource` carries GVK, GVR,
  scope, verbs, short names, and categories so the table/menu layers never
  re-derive them.
- **Narrow `preferredResourceDiscoverer` interface (one method)** so the core is
  hermetically fakeable with a tiny stub (D18) — no fake-discovery plumbing or
  server needed to exercise happy-path, partial-failure, total-failure, and
  malformed-GV branches. Discovery does **not** feed the RESTMapper here: the D29
  deferred discovery mapper already resolves non-seed kinds lazily, so wiring
  discovered mappings into it would be redundant scope.

### D30 — Executing M1-02: static seed RESTMapper composed ahead of discovery
**2026-07-18.** M1-02 landed `internal/kube/seed.go`: a static `meta.DefaultRESTMapper`
seeded with ~28 core, high-traffic GVKs (core/v1, apps/v1, batch/v1,
networking.k8s.io/v1, rbac.../v1, storage.k8s.io/v1) and composed **ahead of** the
deferred discovery mapper from D29 via
`meta.FirstHitRESTMapper{MultiRESTMapper: {seed, deferred}}`. **Choices:**
- **Exact GVRs, hard-coded — not heuristic pluralization.** Each seed entry carries
  its literal plural/singular resource name and scope, added via
  `DefaultRESTMapper.AddSpecific`. `DefaultRESTMapper.Add`'s built-in pluralizer
  would mangle the irregulars (Endpoints→"endpointses", NetworkPolicy→
  "networkpolicys"); AddSpecific sidesteps that and keeps mappings kubectl-identical.
- **Seed first, discovery fallback.** `FirstHitRESTMapper` returns the first mapper
  that resolves, so for seeded kinds the seed short-circuits and discovery is **never
  consulted → zero network I/O** — the "instant start" half of D8. Unknown kinds
  (CRDs, uncommon groups) fall through to the deferred discovery mapper, which warms
  lazily. A composed-mapper test proves `RESTMapping(Pod)` succeeds against an
  unreachable dummy config (no server round-trip).
- **Curated, not exhaustive.** The seed covers the resources a user browses first;
  the long tail is discovery's job. Every seeded mapping is a long-stable GA
  relationship, so the static copy cannot drift from the server for those kinds.
- **Permanent layer, not a warm-up cache.** The seed mapper stays in the chain for
  the process lifetime (it is cheap and authoritative for its kinds); M1-03 adds the
  **async full discovery → reconcile signal** that surfaces the rest of the menu, it
  does not replace the seed.

### D29 — Executing M1-01: client bootstrap shape; deferred RESTMapper; no network at construction
**2026-07-18.** M1-01 landed `internal/kube/client.go`: the client-go bootstrap
the whole kube layer builds on. **Choices:**
- **API shape:** `ClientConfig{Kubeconfig, Context}` (both optional; zero value =
  standard rules + current-context) → `RESTConfig(cc) (*rest.Config, error)` →
  `NewClients(cfg) (*Clients, error)`, plus a `Connect(cc)` convenience that
  chains them. `Clients` bundles `Config`, `Clientset` (typed), `Dynamic`
  (untyped/CRDs), `Discovery`, and `RESTMapper`. Split RESTConfig from NewClients
  so callers can inject a config (e.g. envtest's `*rest.Config`, tests) without
  going through kubeconfig files.
- **Loading:** `clientcmd.NewDefaultClientConfigLoadingRules()` (honors
  `KUBECONFIG` then `~/.kube/config`) with `ExplicitPath` set when `Kubeconfig` is
  given, and `ConfigOverrides.CurrentContext` for context selection —
  `NewNonInteractiveDeferredLoadingClientConfig(...).ClientConfig()`. The
  "NonInteractive" variant never prompts (auth prompts would hang an autonomous
  TUI). Errors are wrapped (`kube: loading kubeconfig: %w`), never panicked — bad
  path / unknown context degrade gracefully (#86; full typed-error taxonomy is
  M1-09).
- **RESTMapper = `restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))`.**
  Deferred + memory-cached: **zero network I/O at construction**, so building
  `Clients` never blocks first paint (D8/goal "fast cold start"); it populates
  lazily on first mapping and caches. M1-02 (static seed set) and M1-03 (async
  full discovery) layer on top of this; this leg deliberately does not seed or
  pre-warm.
- **`NewForConfig` does no server call**, so construction succeeds against an
  unreachable/dummy apiserver — connection failures surface on first request, not
  at bootstrap. This makes the unit tests hermetic (D18): a two-context temp
  kubeconfig exercises current-context vs override vs unknown-context/missing-file
  errors; a dummy `&rest.Config{Host:...}` exercises client wiring — no fake
  clients or network needed. No new dependencies (all `k8s.io/client-go`
  sub-packages already vendored via M1-00).

### D32 — Executing M1-04: on-disk cached discovery via kubectl's diskcached client; per-host dir; TTL + explicit Invalidate; memory fallback
**2026-07-18.** M1-04 landed `internal/kube/cache.go` and rewired `NewClients`
(client.go): the discovery client is now
`k8s.io/client-go/discovery/cached/disk`'s `CachedDiscoveryClient` — the same
on-disk cache kubectl uses — completing the "cache discovery on disk" half of D8.
**Choices:**
- **Reuse kubectl's `diskcached` client, don't hand-roll a cache.** It already
  does everything M1-04 needs — TTL-gated JSON docs on disk, an internal memcache
  delegate, `Invalidate()`/`Fresh()` (the `CachedDiscoveryInterface`) — and is the
  exact code kubectl runs, so kubecom's cache behavior matches the tool users know.
  It replaces the M1-02/D29 `memory.NewMemCacheClient` wrapper as the base of the
  deferred RESTMapper, so **discovery and mapping now share one on-disk cache**.
- **Cache dir = `os.UserCacheDir()/kubecom`, per-host subdir.** Caches are
  disposable, so they live under the **cache** dir (`~/.cache/kubecom` on Linux,
  `~/Library/Caches/kubecom` on macOS), deliberately *separate* from the config
  dir (D20 — config is user data, cache is not). Discovery docs go under
  `.../discovery/<host-slug>/` and the HTTP response cache under `.../http/`. The
  host is slugged (scheme stripped, non-`[\w/.]` → `_`, mirroring kubectl) because
  **the discovery cache must be unique per host:port** — two clusters share
  group/version names but not resource sets, so a shared dir would cross-serve.
- **TTL 6h + explicit `Clients.Invalidate()`.** 6h matches kubectl's default:
  long enough the steady state never re-hits the server, short enough a new API
  group is picked up within a session. `Invalidate()` bypasses the TTL for a
  user-forced refresh or after a CRD install; it clears **both** the discovery
  cache and the deferred RESTMapper (`Reset()`), which is retained on `Clients`
  (`deferredMapper`) for exactly this. The static seed mapper (M1-02) is left
  untouched — its core mappings are authoritative and can't go stale.
- **Degrade, don't crash (principle 3).** If no cache dir resolves (e.g. `HOME`
  unset), `newCachedDiscovery` falls back to the in-memory `memcache` client —
  discovery still works, it just isn't persisted across runs. Both paths return a
  `CachedDiscoveryInterface`, so the RESTMapper and `Invalidate` are identical
  either way. Construction still does **no network I/O** (docs read/written lazily,
  dirs created on first write), so fast cold start (D8) holds.
- **New transitive deps:** `github.com/gregjones/httpcache`,
  `github.com/peterbourgon/diskv`, `github.com/google/btree` — pulled by the
  diskcached package, added via `go mod tidy`. No direct-dep change.
- **`Clients.Discovery` field type widened** `DiscoveryInterface` →
  `CachedDiscoveryInterface` (a superset) so callers can `Invalidate/Fresh`
  directly; existing `StartDiscovery` (needs only `ServerPreferredResources`) is
  unaffected. Tests stay hermetic (dummy `rest.Config`): they cover host-slugging,
  per-host uniqueness, the interface contract, and `Invalidate` no-panic without a
  server. **"Lazy group detail on first open" split out to M1-04b** — it needs the
  M2 menu open interaction that doesn't exist yet, so it isn't actionable now.
