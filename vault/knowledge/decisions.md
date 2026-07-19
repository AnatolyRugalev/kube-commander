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

### D33 — Executing M1-05a: server-side Table List via a per-GroupVersion REST client (dynamic can't negotiate Table per call); M1-05 split into List + Watch
**2026-07-18.** M1-05 ("server-side Table List+Watch → event channel;
reconnect/resync") was too big for one ≤300-line green leg, so it was split:
**M1-05a = List** (this leg), **M1-05b = Watch** (event channel + reconnect/resync,
back to Backlog). M1-05a landed `internal/kube/table.go`. **Choices:**
- **List talks to the REST layer directly, not through the dynamic client.** The
  stack note (D8-adjacent) pointed at "server-side Table via the dynamic client",
  but `dynamic.Interface`'s `List` gives no hook to set the `Accept` header per
  call — it always negotiates the plain object list. To request server-side
  printing you must set `Accept: application/json;as=Table;v=v1;g=meta.k8s.io,application/json`
  on the request yourself. So `List` builds a `rest.Interface` scoped to the
  resource's GroupVersion (`restClientForGV`: `rest.CopyConfig` + `GroupVersion` +
  `/api` for the core group else `/apis` + `scheme.Codecs.WithoutConversion()`),
  then `Get().NamespaceIfScoped(ns, namespaced).Resource(gvr.Resource).VersionedParams(&opts, scheme.ParameterCodec).SetHeader("Accept", tableAcceptHeader).Do(ctx).Raw()`.
  Columns come entirely from the server (incl. a CRD's `additionalPrinterColumns`),
  so columns are kubectl-identical for **every** resource with zero hard-coding.
- **TUI-facing `Table{Columns,Rows}` decoupled from `metav1.Table`.** `decodeTable`
  flattens the server's JSON `metav1.Table` into `Column`/`Row`/`ObjectRef` so the
  TUI never imports apimachinery. Row identity (`ObjectRef{Namespace,Name,UID}`)
  is pulled from each row's embedded `PartialObjectMetadata` — server-side Table
  defaults to `IncludeObject=Metadata`, so it's present without asking. A row with
  missing/malformed object metadata is **kept with a zero ObjectRef, not dropped**
  (principle 3), so one odd row never blanks a list.
- **`List` takes a discovery `Resource`** (from M1-03) — it already carries GVR +
  `Namespaced`, so `List` needs no separate scope lookup. `namespace` is dropped
  for cluster-scoped resources by `NamespaceIfScoped`.
- **No new deps.** `rest`, `rest/fake`, and `kubernetes/scheme` are all already
  vendored via client-go. Hermetic tests (D18) use `rest/fake.RESTClient`
  (canned Table body + recorded request) to assert the `Accept` header, the
  namespaced vs cluster-scoped URL path, and the decode — plus a pure `decodeTable`
  suite (columns/priority, object refs, degrade-on-bad-row, invalid JSON). The
  M1-05b watch leg reuses `decodeTable` (Table watch chunks are `metav1.Table`
  deltas) and the per-GV REST client (watch is the same request with `watch=true`).

### D34 — Executing M1-05b: reconnecting List→Watch driver streaming Table deltas on a channel; RESET-on-resync
**2026-07-18.** M1-05b landed `internal/kube/watch.go` — `Clients.Watch(ctx, r, ns,
opts) (<-chan WatchEvent, error)` starts a background goroutine that streams
server-side Table deltas to a bounded channel. **Choices:**
- **Raw `Stream()`, not `rest.Request.Watch()`.** A Table watch's stream is a
  sequence of `metav1.WatchEvent` JSON objects whose `.Object.Raw` is a
  `metav1.Table` delta. `Request.Watch()` would need a scheme/decoder wired for
  Table objects; instead the loop opens `tableRequest(...).Stream(ctx)` (same
  endpoint + `Accept` header as List, `watch=true`, `allowWatchBookmarks=true`,
  `resourceVersion=rv`) and `streamTableWatch` decodes the `WatchEvent` stream
  with a plain `json.Decoder`, reusing `decodeTableRV` for each delta. This keeps
  the whole path apimachinery-free at the TUI boundary and hermetically testable
  from a byte stream.
- **`RESET` is a kubecom-level event, not a k8s verb.** The event vocabulary is
  `ADDED/MODIFIED/DELETED` (one row each, mirroring the watch verbs) plus **`RESET`**
  (full current row set from a fresh List — consumer replaces its whole set) and
  **`ERROR`** (terminal failure in `Err`; the loop keeps retrying). Every
  (re)connection begins with a List → `RESET`, so a consumer that treats each
  `RESET` as a full replace is correct across reconnects without knowing they
  happened.
- **RetryWatcher-style reconnect keyed off resourceVersion.** `decodeTableRV`
  (added to `table.go`, `decodeTable` now delegates) surfaces the Table's
  `ListMeta.ResourceVersion`; the loop resumes the watch from the last RV seen
  (advanced by deltas *and* bookmark events). A clean stream end or a resumable
  drop reconnects from `rv` with **no** re-List (no spurious `RESET`); a **410
  Gone / `StatusReasonExpired`** — detected in `watchStatusError`, tagged as a
  sentinel `*errExpired` — forces a full re-List because the server can no longer
  replay from `rv`.
- **Columns cached across the connection.** The API server only guarantees column
  definitions on the **first** Table response; later chunks omit them.
  `streamTableWatch` carries the columns in/out and stamps every emitted event
  with the current set, so a consumer can always read columns off any event.
- **Bounded buffering + context-owned lifecycle.** The channel is buffered
  (`watchChanBuffer=64`) so a briefly slow consumer never stalls the watch;
  reconnects are paced by `watchRetryBackoff=2s` (cancellable) so a server that
  instantly closes every watch can't spin the loop. The goroutine owns all sends
  and `close`s the channel on `ctx` cancellation — UI state mutates only in the
  consumer's `Update` (principle 1). No new deps; hermetic tests drive
  `streamTableWatch` from byte streams (deltas, bookmark, 410→`errExpired`,
  generic error) and the full `watchLoop` over a `rest/fake` transport that answers
  List vs `watch=true` differently (asserts `RESET`-then-delta with carried columns).

### D35 — Executing M1-06a: generic delete via the dynamic client; UID precondition guards the row-snapshot race; M1-06 split into 06a–06d
**2026-07-18.** M1-06 ("Actions: delete/scale/rollout-restart/cordon/drain/
cronjob-suspend") is six actions — too big for one green ≤300-line leg — so it was
split (cf. D33's M1-05 split): **06a delete** (this leg), **06b scale +
rollout-restart**, **06c cordon/drain**, **06d cronjob suspend/resume** (back to
Backlog). M1-06a landed `internal/kube/actions.go` — the first slice of the
in-process action set (D2). **Choices:**
- **Delete addresses objects through the *dynamic* client, not typed clients.**
  `Clients.Delete(ctx, r Resource, ref ObjectRef, opts)` mirrors `List`'s shape:
  the GVR + scope come from the discovery `Resource`, the namespace/name from the
  row's `ObjectRef`. One code path deletes **any** resource — built-in or CRD —
  with zero per-kind wiring, exactly as the Table List/Watch path is generic. A
  shared `resourceInterface(r, ns)` helper (namespaces the dynamic client iff
  `r.Namespaced`) is the addressing primitive 06b–06d will reuse.
- **UID precondition guards the snapshot race.** A table row is a point-in-time
  snapshot; between listing and acting the named object can be deleted and a new
  one recreated under the same name. When `ObjectRef.UID` is present, Delete sets
  it as a `metav1.Preconditions{UID}` so the server only removes *that* object
  (else Conflict) — it never deletes the wrong same-named object. A row with no
  UID (degraded metadata, principle 3) deletes by name alone; a caller that set
  its own preconditions keeps them (the guard is layered only when `opts` has
  none). The logic is a pure `withUIDPrecondition(ref, opts)` helper so 06b–06d
  can reuse it and it is unit-testable directly.
- **Testing shape forced by a fake-client limitation.** The client-go **fake
  dynamic client discards `DeleteOptions`** — its `Delete` calls
  `testing.NewDeleteAction` (no options variant), so a recorded action's
  `GetDeleteOptions()` is always zero. Therefore the precondition contract is
  proven against `withUIDPrecondition` as a pure function, and the fake
  (`dynamic/fake.NewSimpleDynamicClientWithCustomListKinds`, custom list kinds so
  it never guesses) is used only for the round-trip: object actually removed,
  namespace routing (namespaced vs cluster-scoped — a namespace on a node ref is
  dropped), empty-name rejected, and NotFound surfaced **wrapped** (`errors.Is` →
  `apierrors.IsNotFound` still true, #86). **Knowledge:** the fake-client quirk +
  the action-set pattern are recorded in [`stack.md`](stack.md) so 06b–06d don't
  rediscover them.
- **New dep:** `go mod tidy` pulled `gopkg.in/evanphx/json-patch.v4` (indirect,
  via the dynamic fake). No direct-dep change.

---

### D36 — Executing M1-06b: scale + rollout-restart as generic merge patches through the dynamic client
**2026-07-18.** Second slice of the M1-06 action set (after 06a delete, D35), same
generic-dynamic-client posture (D2, D35). Landed `Clients.Scale` and
`Clients.RolloutRestart` in `internal/kube/actions.go`. **Choices:**
- **Scale merge-patches the `scale` subresource, not the object body.**
  `Scale(ctx, r, ref, replicas)` sends a `types.MergePatchType` patch
  `{"spec":{"replicas":N}}` to `.Patch(..., "scale")`. Every scalable kind
  (Deployment/ReplicaSet/StatefulSet/ReplicationController and any CRD exposing a
  scale subresource) stores replicas at `scale.spec.replicas` regardless of its
  own schema, so one code path scales built-ins **and** CRDs with no per-kind
  wiring — exactly like Delete/List. Negative replicas rejected locally (clear
  error, no needless round-trip); empty name rejected; NotFound wrapped (#86).
- **Rollout-restart stamps kubectl's exact annotation key.**
  `RolloutRestart(ctx, r, ref)` merge-patches
  `spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"]` with a
  UTC RFC3339 timestamp — byte-identical to what `kubectl rollout restart` does.
  Mutating the pod template is what the controller observes as a change, so it
  rolls all pods. Reusing **kubectl's** key (not a kubecom-specific one) makes the
  two tools interoperable and stops the annotation proliferating across repeated
  restarts.
- **Merge patch, not strategic merge.** Strategic merge needs a per-type schema
  and does not work on unstructured objects / CRDs; a plain RFC 7386 merge patch
  is schema-free and, for an annotation *add*, has identical effect (merges the
  annotations map, leaving siblings intact). So both actions stay generic over the
  dynamic client.
- **No UID precondition (unlike Delete).** `metav1.PatchOptions` carries no
  preconditions, and scale/restart are idempotent — re-issuing converges rather
  than destroying a wrongly-matched object — so the snapshot-race guard Delete
  needs does not apply here.
- **Testing:** wire format proven against pure `scalePatch`/`restartPatch`
  helpers; the fake dynamic client (which, unlike for Delete, *does* round-trip a
  merge patch and ignores the subresource) exercises the apply — `spec.replicas`
  set, restartedAt added without clobbering a seeded sibling annotation, empty
  name / negative replicas / wrapped NotFound. No new deps. (Fake-patch behavior
  recorded in `stack.md`.)

### D37 — Executing M1-06c: cordon/uncordon as a generic `spec.unschedulable` merge patch; M1-06c narrowed, drain split to M1-06e
**2026-07-18.** Third slice of the M1-06 action set (after 06a delete D35, 06b
scale/restart D36). **M1-06c was narrowed from "cordon/uncordon + drain" to
cordon/uncordon; drain moved to a new M1-06e** — drain (list a node's pods, skip
DaemonSet/mirror/completed pods, evict each via the policy/v1 Eviction API, honor
PDBs through 429-retry, and wait for deletion) is its own logical change that
would blow the ≤300-line green-leg budget together with cordon. This mirrors the
D33 (M1-05) and D35 (M1-06) split precedent. Landed `Clients.Cordon` /
`Clients.Uncordon` in `internal/kube/actions.go`. **Choices:**
- **Cordon/uncordon merge-patch `spec.unschedulable` through the dynamic client**,
  exactly as `kubectl cordon`/`uncordon` do — Cordon sets it true, Uncordon sets
  it false. Same generic-dynamic-client posture as Scale/RolloutRestart (D36): no
  typed node client, addressed by the discovery `Resource`'s GVR via the shared
  `resourceInterface(r, ns)` helper. Nodes are cluster-scoped, so a namespace on
  the ref is ignored (r.Namespaced == false), just like node Delete (D35).
- **Uncordon writes the concrete value `false`, not `null`.** RFC 7386 merge patch
  only *removes* a key when its value is null; an explicit `false` keeps the field
  present and reads identically to the scheduler. (kubectl uncordon does the same.)
- **Cordon ≠ drain.** Cordon only stops *new* pods landing; existing pods keep
  running. Evicting them is drain's job (M1-06e), which cordons first then evicts.
  Documented on `Cordon` so a caller doesn't mistake it for drain.
- **No UID precondition (like Scale/RolloutRestart, unlike Delete).** PatchOptions
  carries none and the toggle is idempotent, so the snapshot-race guard doesn't
  apply. Empty name rejected locally; NotFound wrapped (#86). Shared body
  `setUnschedulable(ctx, r, ref, bool)`; the error verb tracks the flag
  (cordoning/uncordoning) via tiny pure `cordonNoun`/`cordonVerb` helpers.
- **Testing:** pure `unschedulablePatch(bool)` asserts the wire format; the fake
  dynamic client (which round-trips a merge patch, per D36) exercises the apply —
  cordon sets unschedulable true (namespace dropped for the cluster-scoped node),
  uncordon flips a seeded-true node to false, empty-name rejected for both,
  NotFound wrapped. No new deps.

### D38 — Executing M1-06d: cronjob suspend/resume as a generic `spec.suspend` merge patch
**2026-07-19.** Fourth slice of the M1-06 action set (after 06a delete D35, 06b
scale/restart D36, 06c cordon/uncordon D37); 06e drain remains. Landed
`Clients.Suspend` / `Clients.Resume` in `internal/kube/actions.go` — a near-exact
mirror of Cordon/Uncordon (D37), which is the whole point: this slice reuses the
established toggle-a-bool-via-merge-patch shape rather than inventing anything.
**Choices:**
- **Suspend/Resume merge-patch `spec.suspend` through the dynamic client**, exactly
  as `kubectl patch cronjob NAME -p '{"spec":{"suspend":true|false}}'` does —
  Suspend sets it true, Resume sets it false. Same generic-dynamic-client posture
  as the other actions: no typed batch client, addressed by the discovery
  `Resource`'s GVR via the shared `resourceInterface(r, ns)` helper. CronJobs are
  **namespaced** (unlike the cluster-scoped node in cordon), so the ref's namespace
  is honored.
- **Resume writes the concrete value `false`, not `null`** — identical rationale to
  Uncordon (D37): an RFC 7386 merge patch removes a key only when its value is null;
  explicit `false` keeps the field present and reads the same to the controller.
- **Suspend gates only *new* Jobs.** Already-running Jobs a suspended CronJob
  spawned keep running — the parallel of "cordon stops only new pods". Documented on
  `Suspend` so a caller doesn't expect it to stop in-flight Jobs.
- **No UID precondition (like Cordon/Scale, unlike Delete).** PatchOptions carries
  none and the toggle is idempotent, so the row-snapshot guard doesn't apply. Empty
  name rejected locally; NotFound wrapped (#86). Shared body
  `setSuspend(ctx, r, ref, bool)`; the error verb tracks the flag
  (suspending/resuming) via tiny pure `suspendNoun`/`suspendVerb` helpers, mirroring
  `cordonNoun`/`cordonVerb`.
- **Testing:** pure `suspendPatch(bool)` asserts the wire format; the fake dynamic
  client round-trips the merge patch — suspend flips a seeded-false CronJob to true
  (namespace honored), resume flips a seeded-true one to false, empty-name rejected
  for both, NotFound wrapped. No new deps.

### D39 — Executing M1-06e-1: drain pod selection (typed clientset + pure classifier); M1-06e split into 06e-1 selection + 06e-2 eviction
**2026-07-19.** Fifth slice of the M1-06 action set (06a delete D35, 06b
scale/restart D36, 06c cordon/uncordon D37, 06d suspend/resume D38). **M1-06e
drain was split into 06e-1 (this: pod selection) + 06e-2 (eviction loop)** — the
full drain (list a node's pods, classify, evict via the policy/v1 Eviction API,
PDB-aware 429-retry, wait for deletion) blows the ≤300-line green-leg budget, as
D37 already flagged; the selection/classification half is a clean, pure,
fully-testable unit that the eviction half consumes. Landed `Clients.DrainCandidates`
+ pure `classifyDrainPods` in a new `internal/kube/drain.go`. **Choices:**
- **Drain uses the typed clientset, not the dynamic client** (the departure from
  06a–06d's generic-dynamic posture). Drain is pod-and-node specific, never generic
  over CRDs: it lists pods by the `spec.nodeName` field selector via
  `Clientset.CoreV1().Pods(NamespaceAll).List` and (06e-2) will evict through the
  typed `EvictV1` subresource. That is exactly what `kubectl drain` does; a dynamic
  path would buy nothing here and lose the typed pod fields the classifier reads.
- **Selection is a pure `classifyDrainPods([]corev1.Pod, DrainOptions)` split from
  the client call**, so the whole policy is unit-testable without a cluster — the
  fake clientset does not honor field selectors (server-side; envtest territory),
  so filtering-by-node is not asserted by the fake, only the classification is.
- **Classification mirrors `kubectl drain`.** Skipped silently (not evicted, not
  blocking): mirror pods (annotation `kubernetes.io/config.mirror`, inlined — we do
  not depend on k8s.io/kubernetes), already-terminated pods (Succeeded/Failed), and
  DaemonSet-managed pods when `IgnoreDaemonSets`. Blocking (collected into one
  refusal error naming each pod, drain refuses as a whole → nil slice, never a
  partial drain): standalone/unmanaged pods without `Force`, DaemonSet pods without
  `IgnoreDaemonSets`, emptyDir-backed pods without `DeleteEmptyDirData`. Check order
  is load-bearing (mirror→terminated→controller→emptyDir) so each pod is reported
  at most once; a DaemonSet pod is a DS skip/block, never an emptyDir block.
- **DaemonSet detection is by controller owner-ref Kind == "DaemonSet"**, not by
  confirming the DaemonSet still exists (kubectl does the extra GET to treat an
  orphaned DS pod as unmanaged). Simplification: one fewer API call, and an orphaned
  controller ref is rare; revisit in 06e-2 if envtest shows it matters.
- **`DrainCandidates` returns `[]ObjectRef`, not `[]corev1.Pod`** — same
  apimachinery-free boundary the table layer keeps (D33), and exactly the input
  06e-2's eviction loop needs (namespace/name/UID per pod). Empty node name
  rejected; list error wrapped (#86). `k8s.io/api` moves indirect→direct in go.mod
  (corev1 now imported); no new module version.

### D40 — M1-06e-2: drain eviction loop (policy/v1 Eviction API, PDB-aware 429-retry, wait-for-deletion); candidates-before-cordon ordering
**2026-07-19.** Sixth and final slice of the M1-06 action set, completing drain
(06e-1 selection D39 + this eviction loop). Landed `Clients.Drain(ctx, nodeRes
Resource, node ObjectRef, opts DrainOptions)` plus unexported `evictPod` /
`waitPodDeleted` in `internal/kube/drain.go`. **Choices:**
- **Candidates computed *before* cordon.** `Drain` calls `DrainCandidates` first
  (which refuses upfront on any blocking pod) and only cordons once the pod set is
  settled. A drain that will be refused therefore never leaves the node cordoned —
  a small divergence from `kubectl drain` (which cordons first) that avoids the
  "cordoned but not drained" state. Sequence: candidates → cordon (reuse M1-06c
  `Cordon`, dynamic client) → evict all → wait for all deleted.
- **Eviction via the typed policy/v1 Eviction API** (`Clientset.PolicyV1().
  Evictions(ns).Evict`), not a dynamic delete — the same subresource `kubectl
  drain` posts to, so PodDisruptionBudgets are honored server-side. Continues D39's
  "drain uses the typed clientset, not the generic dynamic path" posture.
- **PDB-aware retry.** A PDB with no allowed disruptions makes Evict return `429
  TooManyRequests`; `evictPod` waits `evictionRetryInterval` and retries, because
  the budget frees up as other pods reschedule. `NotFound` (pod already gone) is
  treated as success. The overall budget is the caller's **ctx deadline**, not a
  fixed attempt count (mirrors `kubectl drain --timeout`); ctx cancellation ends
  the retry with a wrapped `ctx.Err()`.
- **Two-pass evict-then-wait.** All evictions are requested first, then all pods
  waited on, so grace periods overlap rather than serialize. `waitPodDeleted` polls
  every `drainPollInterval` until the pod is `NotFound` **or the name resolves to a
  different UID** (a pod recreated under the same name ⇒ the original is gone) — the
  same row-snapshot identity guard the UID-precondition delete uses (D35).
- **Timing knobs are package `var`s** (`evictionRetryInterval` 5s,
  `drainPollInterval` 2s), not consts, so tests shrink them to 1ms — the same
  pattern that keeps the retry/wait loops hermetic and instant.
- **Testing:** the fake clientset's `Evict` posts a `create` on `pods` subresource
  `eviction`; tests intercept it with a reactor to inject 429/NotFound/success and,
  in the full `Drain` test, delete the pod from the tracker so the wait observes it
  disappear. Cordon runs against a fake dynamic client (node), eviction/list/wait
  against the fake typed clientset (pod). `TestDrainRefusesBeforeCordon` asserts the
  candidates-before-cordon ordering (blocked drain leaves the node uncordoned). No
  new deps: `k8s.io/api/policy/v1` and `apimachinery/api/errors` were already in the
  graph.
