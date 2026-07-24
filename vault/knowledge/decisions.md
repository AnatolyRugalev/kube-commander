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

### D41 — Executing M1-07a: get-object-as-YAML via the dynamic client; managedFields stripped; M1-07 split into 07a/07b/07c
**2026-07-19.** M1-07 ("streaming: logs; describe; get-as-YAML") is three distinct
viewers — too big for one ≤300-line green leg — so it was split (cf. the D33/D35/
D37/D39 split precedent): **07a get-as-YAML** (this leg), **07b describe**
(kubectl/pkg/describe — pulls the big `k8s.io/kubectl` dep), **07c pod logs**
(reconnecting stream, watch-shaped) back to Backlog. Landed `Clients.GetYAML` +
pure `marshalYAML` in `internal/kube/yaml.go` — the first in-process viewer (D2:
no external pager, no kubectl binary). **Choices:**
- **GetYAML addresses the object through the *dynamic* client, reusing the shared
  `resourceInterface(r, ns)` helper** the action set (M1-06) is built on — a plain
  `Get` by GVR (from the discovery `Resource`) + namespace/name (from the row's
  `ObjectRef`). One code path renders **any** resource — built-in or CRD — with zero
  per-kind wiring, exactly like Delete/List. The ref's namespace is dropped for
  cluster-scoped resources (`r.Namespaced == false`), same as node Delete/Cordon.
- **managedFields are stripped before rendering.** They are server-side-apply
  bookkeeping — large and never useful to read — so kubectl itself has hidden them
  from `get`/`describe` output **by default since v1.21**. Stripping them makes
  `GetYAML` match what a user sees from `kubectl get -o yaml` today. The strip is on
  a `DeepCopy` (`unstructured.RemoveNestedField` mutates), so the caller's object is
  untouched — `marshalYAML` stays pure and directly unit-testable.
- **Rendered with `sigs.k8s.io/yaml`, not `gopkg.in/yaml`.** sigs.k8s.io/yaml
  marshals by round-tripping through `encoding/json`, so it honors the API types'
  `json` tags and orders map keys deterministically — byte-for-byte what kubectl
  emits. It was already in the module graph (client-go transitive); this leg only
  promotes it indirect→direct in go.mod (`go mod tidy`), no new module version.
- **Testing (D18):** pure `marshalYAML` asserts managedFields-stripping + that the
  caller's object is not mutated; the fake dynamic client round-trips `GetYAML` for
  a namespaced object, a cluster-scoped node (stray ref namespace ignored), empty
  name (rejected), and a missing object (NotFound surfaced **wrapped** —
  `apierrors.IsNotFound` still holds through the `%w` chain, #86). No new deps.

### D42 — Executing M1-07b: describe via kubectl/pkg/describe; RESTMapping built from the discovery Resource
**2026-07-19.** Landed `Clients.Describe(r, ref)` + the pure `describerFor` /
`restMappingFor` helpers in `internal/kube/describe.go` — the second in-process
viewer (D2: no external pager, no kubectl binary). **Choices:**
- **Reuse kubectl's own describe generators (`k8s.io/kubectl/pkg/describe`)** rather
  than hand-rolling per-kind output. This is the whole point of the milestone-scope
  wording ("describe (kubectl/pkg/describe)") — the output is byte-identical to
  `kubectl describe`, and every built-in kind's specialized section (a Pod's
  containers/conditions/volumes, a Deployment's rollout status, …) plus the trailing
  "Events:" table comes for free and tracks upstream.
- **Dep pinned to `k8s.io/kubectl v0.31.4`** to match the existing k8s stack (api/
  apimachinery/client-go/cli-runtime all v0.31.4). `go mod tidy` *without* a pin
  resolves kubectl to the latest (v0.36.2), which would drag the whole k8s graph to
  v0.36 — so `go get k8s.io/kubectl@v0.31.4` first, then tidy. Footprint: kubectl
  direct + ~12 transitive indirect (cli-runtime, kustomize api/kyaml, liggitt/
  tabwriter, xlab/treeprint, moby/term, go-starlark, …). Acceptable for the describe
  generators; the alternative (reimplementing describe) is far larger and would drift.
- **Describer selection mirrors kubectl's `describe.NewDescriber`:** prefer the
  specialized built-in describer keyed by `GVK.GroupKind()` (`describe.DescriberFor`),
  fall back to the generic unstructured describer (`describe.GenericDescriberFor`)
  otherwise — so **CRDs and rarer built-ins are covered** by the generic path (name/
  namespace/labels/annotations + recursive body dump + events). Kept as a small local
  `describerFor` (not `NewDescriber`, which wants a `genericclioptions.RESTClientGetter`
  we'd have to synthesize) since we already hold the `*rest.Config`.
- **The generic describer's `meta.RESTMapping` is built directly from the discovery
  `Resource` (`restMappingFor`)**, not resolved through the RESTMapper: the `Resource`
  already carries GVR + GVK + scope (the only fields the generic describer reads), so
  there's no reason to make discovery re-derive them. Scope is `RESTScopeNamespace`/
  `RESTScopeRoot` from `r.Namespaced`; the ref's namespace is honored iff namespaced.
- **`Describe` takes no `context`** (unlike `GetYAML`): kubectl's describe package
  exposes no context-aware entry point — it Gets the object and searches events with
  `context.TODO()` internally — so there is nothing to thread one through. The TUI
  runs it off the render goroutine and abandons the result if the view closes. Empty
  name rejected; errors wrapped (NotFound/RBAC-denial surface for display, #86).
- **Testing (D18):** describer *construction* is local (the describers build their
  clients from the config but make no server call), so `describerFor` is tested
  hermetically with a throwaway `rest.Config` — a built-in kind (Pod) resolves to
  `*describe.PodDescriber`, a fictional CRD kind falls back to the generic describer;
  `restMappingFor` is a pure table test; empty-name is rejected before any describer
  is built. Actual describe **output** dials the API server → **envtest territory**
  (opt-in, like the watch live-server exercise), not a hermetic unit test.

### D43 — Executing M1-07c: streaming pod logs via the typed clientset GetLogs subresource; single connection, reconnect split to M1-07d
**2026-07-19.** Landed `Clients.Logs(ctx, ref, opts)` + the pure `podLogOptions`
mapper and the `streamLogs` line pump in `internal/kube/logs.go` — the third
in-process viewer (D2: no external pager, no kubectl binary). **Choices:**
- **Typed clientset GetLogs subresource, not the dynamic client.** Unlike the
  action set (M1-06) and the YAML/describe viewers, logs have no dynamic-client
  path — `pods/log` is a subresource that streams raw bytes, so `CoreV1().Pods(ns).
  GetLogs(name, *corev1.PodLogOptions).Stream(ctx)` is the only in-process route.
  This mirrors drain's deliberate use of the typed clientset (D39) for pod/node
  specifics; logs are pod-only, so genericity over CRDs is moot.
- **A `LogEvent{Line, Err}` channel, twin of the Watch channel.** One line per
  event (trailing newline stripped; consumer re-adds it), a terminal `Err` event as
  the last item before close. The stream is opened *inside* the goroutine (like
  Watch) so `Logs` returns immediately and never blocks first paint on the network;
  the goroutine owns every send and the close (principle 1 — UI state mutates only
  in the consumer's Update). Bounded buffer `logChanBuffer=256` (logs burst on
  connect as the container flushes a backlog).
- **`LogOptions` mirrors `kubectl logs` flags** (Container/Follow/Previous/
  Timestamps/TailLines/SinceSeconds/SinceTime/LimitBytes), mapped 1:1 onto
  `corev1.PodLogOptions` by the pure `podLogOptions`; `SinceTime *time.Time` →
  `*metav1.Time`. Timestamps is passed through verbatim (no internal
  parsing/stripping this leg — that's only needed for resume, which is M1-07d).
- **Single connection this leg; reconnect-on-drop is M1-07d.** Follow keeps the one
  stream open for live lines until the container ends or ctx is cancelled, but a
  transient transport drop ends the stream rather than resuming. The reconnecting/
  resuming layer à la watch (D34) — which needs internal Timestamps + RFC3339Nano
  parsing to set `SinceTime` on reconnect and dedup already-delivered lines within
  the resumed second — is intricate enough to warrant its own focused, tested leg,
  matching how Watch (M1-05b) and the 06/07 series were sliced. Landing the
  single-connection core now already satisfies the "logs stream in-process" exit
  clause; 07d hardens it.
- **`bufio.Scanner` with a 1 MiB max line** (`logScanMaxLine`), raised well past the
  64 KiB default because a single log line can be a stack trace or one-line JSON
  blob. A line over the cap ends the stream with `bufio.ErrTooLong` surfaced as an
  error event — never a silent truncation or a panic (#86, principle 3). Empty pod
  name rejected; the open/decode errors are wrapped.
- **Testing (D18):** `podLogOptions` is a pure table test; `streamLogs` is driven
  directly from byte `strings.Reader`s (verbatim lines, no-trailing-newline,
  over-long line → ErrTooLong, mid-stream ctx-cancel unblocks a full channel); the
  wiring loop `runLogStream` is exercised with a fake `logStreamOpener` (success,
  open-error → terminal event, already-cancelled ctx → no event). A **live** log
  stream dials the API server → **envtest territory** (opt-in, like the watch
  live-server exercise), not a hermetic unit test.

### D44 — Executing M1-07d: reconnecting/resuming follow logs — force wire timestamps, resume by SinceTime, dedup the re-served second
**2026-07-19.** Hardened `Clients.Logs` so a **Follow** stream survives a transient
transport drop, extending `internal/kube/logs.go` (the M1-07c single-connection
core, D43) into a reconnecting loop à la watch (D34). **Choices:**
- **EOF = stop, any other read error = reconnect.** `bufio.Scanner` reports `nil`
  at `io.EOF`, so a clean stream end (the container's log ended, exactly when
  `kubectl logs -f` exits) returns nil from the pump → the follow loop stops. Any
  other error is treated as a transient drop → back off and reopen. This is the
  one reliable byte-stream signal that distinguishes "container done" from "network
  blip" without inspecting error strings; an abruptly-closed HTTP/2 log stream
  surfaces as a non-EOF error, a graceful container-end as clean EOF.
- **Resume by SinceTime, not resourceVersion.** Logs have no resourceVersion; the
  only resume anchor is a timestamp. So Follow **forces `Timestamps` on the wire**
  regardless of the caller's choice (every raw line becomes `"<RFC3339Nano>
  <content>"`), records the last-seen line's timestamp, and on reconnect sets
  `SinceTime` to it (`SinceSeconds`/`TailLines` cleared — they only govern the
  initial read). Timestamps are **stripped before delivery unless the caller asked
  for them** (`opts.Timestamps`); an unparseable prefix degrades to verbatim
  delivery, never a dropped line.
- **Dedup the re-served second.** `SinceTime` is **second-granular** (metav1.Time
  serializes to RFC3339 seconds), so resuming from the last line's second makes the
  server re-serve every line already shown in that second. The `logResumer` keeps
  the set of raw timestamped lines delivered within the current second; right after
  a reconnect it drops any incoming line already in that set, delivering only the
  genuinely-new lines (including ones missed *during* the drop, same second) — then
  clears the dedup window once the stream advances to a later second. Raw lines
  carry the full nanosecond timestamp, so the match is exact.
- **First open error terminal; reconnect errors silent.** Never connecting surfaces
  one terminal `LogEvent{Err}` (like the non-follow path). A *reconnect* open/read
  failure is transient: back off (`logRetryBackoff`, the log twin of
  `watchRetryBackoff`, a package **var** so tests shrink it, D40) and retry, bounded
  by ctx — it is **not** surfaced, because a `LogEvent` with `Err` set is the
  terminal event by contract (a mid-stream reconnect must not look like the end).
  A permanently-failing reconnect (e.g. pod deleted) therefore retries until ctx is
  cancelled, matching watch; surfacing transient reconnect state to the consumer is
  future work.
- **Non-follow path unchanged.** No timestamp forcing, no resume, single connection,
  verbatim lines — exactly M1-07c. The two pumps share `newLogScanner` (the raised
  1 MiB max line, #86); `runLogStream` now dispatches on `follow`.
- **Testing (D18):** `parseLogTimestamp` and `logResumer.process`
  (strip/keep-timestamp, dedup-the-resumed-second, window-clears-on-next-second)
  are pure table/sequence tests; `followLogStream` is driven through
  `runLogStream` with a **resumable fake opener** + a `scriptReader` that ends a
  connection with a chosen error (drop) or clean EOF — covering reconnect+dedup
  (asserting the 2nd open's `SinceTime`), clean-end-stops, first-open-error
  terminal, retried-reconnect-open (silent), and ctx-cancel-mid-backoff. All
  hermetic; a live follow-across-a-real-drop is envtest territory.

### D45 — Executing M1-08: background port-forward over an SPDY dialer; channel-based handle, no mutex-guarded result
**2026-07-19.** Landed `Clients.PortForward(ctx, ref ObjectRef, ports []string)
(*PortForward, error)` + the `PortForward` handle in a new
`internal/kube/portforward.go` — the in-process equivalent of `kubectl
port-forward` (D2: no kubectl binary), completing the M1-06/M1-07 in-process
action/viewer set's remaining M1-08 exit criterion. **Choices:**
- **SPDY dialer to the pod's `portforward` subresource, built from the retained
  `*rest.Config`.** `portForwardDialer` does `spdy.RoundTripperFor(c.Config)` →
  `(transport, upgrader)`, then `spdy.NewDialer(upgrader, &http.Client{transport},
  "POST", url)` where `url` is `Clientset.CoreV1().RESTClient().Post().
  Resource("pods").Namespace(ns).Name(name).SubResource("portforward").URL()` —
  exactly what kubectl upgrades. The `Clients` doc already reserved `Config` "for
  callers (e.g. port-forward, which needs the transport)", so no new plumbing. This
  is the **typed-clientset REST client**, a deliberate departure from the generic
  dynamic path (like drain D39 / logs D43): port-forward is pod-only, so genericity
  over CRDs is moot, and the subresource has no dynamic route.
- **`portforward.PortForwarder` does the forwarding; kubecom wraps it in a
  channel-based `PortForward` handle.** `New(dialer, ports, stopCh, readyCh,
  io.Discard, io.Discard)` then `go ForwardPorts()`. The handle exposes `Ready()`
  (closed when listeners are up — the forwarder closes readyCh), `Done()` (closed
  when `ForwardPorts` returns), `Err()` (the fatal error, or nil for a clean stop),
  `Ports()` (bound local:remote pairs — needed when a local port was requested as
  `0`/`":<remote>"` and the OS assigned it), and `Stop()` (idempotent, via
  `sync.Once` closing stopCh). out/errOut are discarded — the TUI reads ports via
  `Ports()` and fatal state via `Err()`, not kubectl's "Forwarding from …" text.
- **Result handed off through a channel close, not a mutex (principle 1).** The
  forward goroutine writes `pf.err` **before** closing `doneCh`; `Err()` reads it
  only after observing `doneCh` closed (a `select` with a `default` returns nil
  early). That happens-before makes it race-free with no mutex — the kube layer's
  established discipline (watch/logs use channels + goroutines, zero mutexes). The
  only `sync` primitive is `sync.Once` for idempotent stop (a double-close guard,
  not shared mutable state).
- **ctx cancellation stops the forward, mirroring Logs/Watch.** A small bridge
  goroutine does `select { case <-ctx.Done(): pf.Stop(); case <-pf.doneCh: }`, so
  cancelling the ctx passed to `PortForward` tears the forward down; the bridge
  exits on its own once forwarding ends, never outliving the handle.
- **Own `ForwardedPort{Local, Remote uint16}` type**, converted from
  `portforward.ForwardedPort`, keeping the TUI boundary free of client-go tooling
  types (the same decoupling as `ObjectRef`/`Table`, D33).
- **Testability via an injected `forwarderFactory`.** The real SPDY forward dials
  the API server (network → envtest territory). `newPortForward(ctx, factory)` is
  split from the exported method and driven by a `fakeForwarder` (closes readyCh,
  blocks on stopCh, returns a scripted error) covering: ready→ports→clean-stop,
  fatal-error→`Err`, ctx-cancel-stops, factory-error→nil-handle, idempotent Stop,
  and wrapped `Ports` error. Empty pod name / empty port list rejected by the
  exported method before any dial (#86). `-race` clean.
- **Deps:** `go mod tidy` added three indirect transitives pulled by
  portforward/spdy — `github.com/gorilla/websocket v1.5.0`,
  `github.com/moby/spdystream v0.4.0`,
  `github.com/mxk/go-flowrate` — all satisfied within the pinned k8s.io v0.31.4
  graph; **no direct-dep or version change** (the tidy did not drift any existing
  module, unlike the kubectl-dep trap D42 warned about).

### D46 — Executing M1-09: typed graceful errors as a Classify(err) → ErrorKind taxonomy over the wrapped chain; RESTConfig tags bad-context
**2026-07-19.** Landed `internal/kube/errors.go` — the "graceful, typed errors,
never panic on bad ns/context" exit clause (#86, old #55, principle 3). **Choices:**
- **Classification over rewiring.** The kube layer already returns every error
  wrapped (`fmt.Errorf(... %w)`) around the underlying apierrors/clientcmd/transport
  error. Rather than replace ~40 error sites with constructed typed errors (a large,
  churny diff), M1-09 adds a single `Classify(err error) ErrorKind` that **walks the
  existing wrap chain** and maps it to a small taxonomy. Zero call sites change; the
  underlying error stays reachable via `errors.Is`/`As`. This is exactly what the
  M1-08 journal scoped ("wrapping the apierrors/clientcmd errors the layer already
  returns").
- **Taxonomy (`ErrorKind`):** `KindUnknown` (incl. nil), `KindNotFound`,
  `KindAlreadyExists`, `KindConflict` (stale resourceVersion / failed UID
  precondition — the D35 delete race), `KindForbidden` (RBAC/403), `KindUnauthorized`
  (401), `KindInvalid` (400/422), `KindTimeout`, `KindUnreachable` (transport/DNS/503),
  `KindBadContext` (kubeconfig/context). The stated M1-08 set (not-found / forbidden /
  unreachable / bad-context) plus the neighbours with a direct apierrors predicate and
  clear TUI value (conflict/unauthorized/invalid/timeout/already-exists). `String()`
  returns a stable lowercase token (classification, not user copy — the TUI renders
  its own message per kind).
- **apierrors predicates walk the chain themselves** (`ReasonForError` → `errors.As`
  to the embedded `*StatusError`), so `IsNotFound(err)` etc. see through the layer's
  `%w`; called directly, most-specific first.
- **Transport unreachable = `errors.As` to `*url.Error` / `net.Error`** (dial refused,
  DNS, TLS never get an HTTP status, so they arrive as these, not an apierror), plus
  `apierrors.IsServiceUnavailable` (503). `context.DeadlineExceeded` → `KindTimeout`.
- **Bad-context is layer-tagged, not string-matched.** The common case — an override
  context that doesn't exist (`--context nope`) — is a **plain `fmt.Errorf("context
  %q does not exist")`** inside clientcmd (`client_config.go`) that **no clientcmd
  predicate matches** (`IsContextNotFound` only matches the `*errContextNotFound`
  type / its specific "was not found for specified context" string, and validation
  never runs for a getContext override miss). So relying on clientcmd predicates
  misses the most important case. Instead, **`RESTConfig` — the single construction
  entry point, whose every failure is a kubeconfig/context problem — wraps its error
  with an unexported `errBadContext` sentinel** (`fmt.Errorf("kube: loading
  kubeconfig: %w: %w", errBadContext, err)`, dual-`%w`), and `Classify` checks
  `errors.Is(err, errBadContext)` first. This tags at the site with the most context
  and is robust to clientcmd's varied wording; the clientcmd predicates
  (`IsContextNotFound`/`IsEmptyConfig`/`IsConfigurationInvalid`, run per-chain-link
  via `chainMatches` since some match only the concrete type) stay as a secondary net
  for a clientcmd error that reaches Classify by another path.
- **No new deps** (net, net/url, context, apierrors, clientcmd all already vendored).
  Hermetic tests: a `Classify` table over constructed apierrors/url/net errors + a
  dual-`%w`-wrapped not-found/url error (proves chain-walking), a real
  `RESTConfig(unknown-context)` → `KindBadContext` (the #86 path end-to-end), an
  empty-kubeconfig case, and `ErrorKind.String()`.

### D47 — Executing M2-01a: keymap core — Action registry + canonical chord model; M2-01 split into slices
**2026-07-19.** First slice of the M2 action registry (D10/D11). New package
`internal/tui/keymap` is the single place keys exist; views will resolve a
keypress to a named `Action` and never match a raw key.

- **Canonical `chord` as the join key.** A keypress and a configured token both
  normalise to one canonical string — modifiers (`ctrl`/`alt`, lowercased, fixed
  order) `+` a base that is either a **case-sensitive single printable rune**
  (`G` ≠ `g`) or a **lowercased special name** (`up`, `enter`, `pgdn`, …). Two
  constructors feed it: `parseChord(token)` (config/defaults) and
  `chordFromKey(tea.Key)` (live). Resolution is a single reverse-index lookup.
  `TestChordRoundTrip` locks the equality property both constructors must satisfy.
- **Shift is never an explicit modifier.** A shifted letter is its capital rune
  (`G`, `N`), matching the keybindings.md config syntax; `parseChord` rejects a
  `shift+` token with a hint. `chordFromKey` prefers `ShiftedCode`, then `Text`,
  then `Code` so shift+g reads `G` under both the Kitty protocol and legacy
  terminals; under ctrl the rune is lowercased so `ctrl+D` == `ctrl+d`.
- **Defaults are one data table, collision-free by construction.** `DefaultKeymap`
  panics (programming error, caught by `TestDefaultKeymapValid`) if the static
  table is malformed. To keep defaults collision-free, `pgdn`/`pgup` fall back to
  the **half-page** actions only; full-page keeps `ctrl+f`/`ctrl+b` (the doc lists
  PgDn as a fallback for both, which would collide in one flat context).
- **`Merge(overrides)` returns a new validated keymap + warnings**, never mutates
  the receiver. Each entry **replaces** an action's binding wholesale (empty list
  = disable). Errors: unknown action id, bad token, or a chord bound to two
  actions (collision names both, in a stable order). **Warnings** (not errors)
  flag an override that shadows a default navigation key (D10).
- **Scope / splits.** This slice is registry + chord model + defaults + merge +
  single-chord resolution + registry primitives (`Actions`/`Keys`/`Describe`) for
  later help generation. Deferred: **M2-01b** multi-key sequences (`gg`→`nav.top`;
  `top` falls back to `home` for now), **M2-01c** YAML `keys:` config wiring +
  `kubecom keys`, **M2-01d** `bubbles/key.Binding` + help overlay generation. The
  action-menu set (describe/yaml/delete/…) is an M3 deliverable per keybindings.md
  and is not registered here. No new deps (`tea` already vendored). Hermetic,
  pure-logic tests only — no goroutines, no runtime surface yet.

### D48 — M2-01b: multi-key sequences + timeout-driven resolution; timer lives in the model, not the keymap
**2026-07-19.** Second slice of the M2 action registry (D10/D11): the vim `gg` →
`nav.top` case and the general multi-key mechanism, built on M2-01a's canonical
chord model.

- **Sequences unify with single chords.** A binding is now a `seq` (`[]chord`,
  length ≥ 1); a single key is just a length-1 sequence, so `DefaultKeymap`,
  `Merge`, collision detection, and `Keys()` all operate on one model.
  `parseSequence` accepts a lone chord (`j`, `up`, `ctrl+d`), the vim concatenated
  form (`gg` → `g`,`g`), and a space-separated form (`g g`, `ctrl+w k`) for
  sequences that mix modified/special chords. A `+`-token that fails to parse is a
  real error, never silently re-read rune-by-rune (so `shift+g`/`ctrl+` keep their
  M2-01a error), and `seq.String()` round-trips (`gg` stays `gg`) for help gen.
- **Stateful matching is a separate `Sequencer`, keymap stays immutable.** The
  `Keymap` gains two derived indexes (`prefix`: is this a prefix of a binding;
  `extends`: does a longer binding extend it) alongside the exact `bySeq` map. A
  `Sequencer` holds the only mutable input state — the buffered prefix — and the
  model owns one, driving it from the update loop, so there is no shared mutable
  state (principle 1). `Input(key)` returns `ResultAction` (resolved),
  `ResultPending` (buffered, a longer binding may still complete), or `ResultNone`
  (inert; a non-continuing key abandons the buffer and is retried alone).
- **The keymap package never runs a timer.** `Input` returning `ResultPending`
  tells the model to schedule a `tea.Tick(SequenceTimeout)`; when it fires the
  model calls `Sequencer.Timeout()`, which fires the buffered prefix iff it is
  itself a complete binding, else drops it. This keeps the package pure/hermetic
  (no goroutines, no clock) — the M0-05/logs pattern. `SequenceTimeout` is an
  exported package var (default 500ms, vim's timeoutlen is 1000ms; snappier) so
  the model reads one source and tests can shrink it.
- **A prefix that is also a complete binding pends, then fires on timeout.** So
  binding an action to bare `g` while `gg` stays bound keeps `gg` reachable (vim
  semantics), rather than forbidding the overlap.
- **`Keymap.Action(key)` stays** as the single-key resolver (ignores sequences)
  for views that never buffer (modals/pickers); `nav.top` is reachable through it
  via the `home` fallback even though its vim binding is the `gg` sequence.
- **pgdn/pgup stay half-page only.** The board's M2-01b note floated wiring pgdn/
  pgup as full-page fallbacks too; they can't fall back to both half- and
  full-page in one flat context without colliding (the exact reason D47 put them
  on half-page). Full-page keeps `ctrl+f`/`ctrl+b`; revisit only if full-page gets
  a context where pgdn is free. No new deps. Splits remaining: M2-01c YAML `keys:`
  wiring, M2-01d bubbles/key + help gen.

### D49 — M2-01c: config `keys:` wiring — plain-YAML in `internal/config`, `Config.Keymap()` resolves, `kubecom keys` prints
**2026-07-19.** The user config gets its first real field and the keymap its
first config surface (M2-01c, on M2-01a's `Merge`).

- **`internal/config` is now a real package** (was a doc-only stub). `Config` has
  one wired field today, `Keys map[string][]string` (`json:"keys"`), the
  action-id → key-tokens override table. The zero value is valid (runs on
  defaults). The browse/menu/theme sections and the legacy migration (D6) are
  still later legs; this leg only wires `keys:`.
- **`sigs.k8s.io/yaml`, not a new YAML dep.** It was already a direct dep (from
  M1-07a) and gives JSON-tag decoding + `UnmarshalStrict`. Plain user YAML decodes
  through JSON tags; no `gopkg.in/yaml.v3` promotion, no new module. `omitempty`
  json tags; unknown top-level fields are **rejected** (`UnmarshalStrict`) so a
  typo — or a not-yet-wired section written early — fails loudly instead of being
  a silent no-op. Revisit if a future section needs lenient forward-compat.
- **A missing config file is not an error.** `LoadFile` returns the zero config on
  `os.ErrNotExist` (kubecom runs on defaults with no file); other read/parse
  errors propagate wrapped. `Load(io.Reader)` is the stream form both share.
- **Config path stays D20:** `Path()` = `os.UserConfigDir()/kubecom/config.yaml`.
- **`Config.Keymap()` is where config meets the keymap.** config imports keymap
  (one-way; keymap imports no config), casts each string id to `keymap.Action`,
  and calls `DefaultKeymap().Merge(overrides)` — reusing M2-01a's validation
  wholesale: unknown action / bad token / collision are errors; a nav-key shadow
  is a returned warning. No resolution logic is duplicated in config.
- **`kubecom keys`** loads the config (default path or `--config`), resolves, and
  prints the effective map in registry order (`keymap.Actions()` + `Keys()` +
  `Describe()`), disabled actions shown as `(disabled)`. Merge **warnings go to
  stderr**, the table to stdout; an invalid keymap fails the command (exit 1).
  `printKeys(out, errOut, km, warnings)` is split from the cobra `RunE` so it is
  testable without cobra. No new deps.

### D50 — M2-01d: help generated from the registry — bubbles/key.Binding bridge + toggleable overlay; pin bubbles v2.0.0
**2026-07-19.** Fourth slice of the M2 action registry (D10/D11): the help side
of "zero hard-coded keys". Help is built **from the resolved keymap**, never from
literals, so it can't drift from actual bindings.

- **Registry → bubbles bridge (`internal/tui/keymap/help.go`).** `(*Keymap).Binding(a)`
  turns one action into a `bubbles/v2/key.Binding` — `WithKeys` = the resolved
  canonical tokens (`Keys(a)`, vim key first), `WithHelp` = display text (tokens
  joined by `/`, e.g. `gg/home`, `ctrl+d/pgdn`) + the registry `Describe()`; an
  action with **no bound keys** (disabled or unknown id) yields a **disabled**
  binding (never a panic) so renderers skip it via `Enabled()`. `Bindings()` is the
  whole registry in order. `HelpKeyMap` (from `HelpMap()`) satisfies bubbles'
  `help.KeyMap`: `ShortHelp()` = a curated status-bar subset (`shortHelpActions`,
  enabled-only), `FullHelp()` = every enabled binding grouped into columns by
  action **namespace** (id prefix before the first `.`), first-seen column order,
  registry order within a column, disabled dropped.
- **Overlay component (`internal/tui/help`).** A small `Model` wrapping
  `bubbles/help.Model` (`ShowAll=true`) + the `HelpKeyMap`: `Toggle`/`SetVisible`/
  `Visible`, `SetWidth` (width-based short-help elision), `View()` (full overlay
  when visible, `""` when hidden), `ShortHelpView()` (the always-on status-bar
  hint). It **matches no raw keys** — the root model resolves `app.help` through
  the keymap and calls `Toggle`; this component only chooses what help to show.
  No shared mutable state (principle 1): every field is owned by the embedder.
  It's a tested, unwired component today; the root model (later M2) embeds it.
- **Dep pin: `charm.land/bubbles/v2` v2.0.0** (+ transitive `charm.land/lipgloss/v2`
  v2.0.0). v2.0.0's go directive is 1.24.2 and it requires bubbletea v2.0.0 — MVS
  keeps our pinned v2.0.2, no downgrade, no Go bump. bubbles v2.1.0 needs Go 1.25.0;
  v2.1.1 needs bubbletea v2.0.7. Hold v2.0.0 in lockstep with the bubbletea v2.0.2
  / Go-1.24.2 floor (D26), the D42/D45 "pin to match the stack" pattern.
- **Split.** The item bundled a *standalone generated keybindings doc*; that's a
  separable concern (a committed markdown file + a drift-check test/`make` target)
  and became **M2-01e**. This leg is the in-app bindings + overlay only. With it,
  the M2-01 action-registry group is functionally complete bar the doc file; next
  is the M2 app shell (root model, browse view, table) — or M2-01e first.

### D51 — M2-01e: keybindings doc generated from the registry + drift-guarded golden test
**2026-07-19.** The committed keybindings reference is **generated from the
default keymap**, not hand-written, closing the "generated keybindings doc" half
of D11 (the help overlay was the in-app half, D50). **Why:** a hand-maintained
doc drifts from the registry the moment a binding changes; generating it from the
same `Actions()`/`Keys()`/`Describe()` primitives the overlay uses makes drift
structurally impossible, and a golden test makes it *loud*. **How:**
- `(*Keymap).Markdown()` (`internal/tui/keymap/doc.go`) renders the keymap as
  `docs/keybindings.md`: a fixed banner + one table per action **namespace**
  (`groupOf`, reused from help.go), columns Action / Keys / Description, registry
  order; keys quoted and `/`-joined vim-first (`docKeys`), no-keys → em dash.
  Nothing restates a literal key — it's all `Keys(a)`/`Describe()`.
- **Golden drift test** `TestKeybindingsDoc` (`doc_test.go`) compares the
  committed file to `DefaultKeymap().Markdown()`; a `-update` flag rewrites it.
  `make check` runs it (via `go test ./...`) so a stale doc **fails the gate**;
  `make keys-doc` (= `go test ./internal/tui/keymap -run TestKeybindingsDoc
  -update`) regenerates. `TestMarkdownFromRegistry` asserts every action id, key,
  and description appears (proves derivation, not restatement).
- Committed doc lives at **`docs/keybindings.md`** (repo root, discoverable — new
  `docs/` dir); the test resolves it as `../../../docs/keybindings.md` (go test's
  cwd == package dir). Golden-update over a standalone `cmd/` generator: no new
  binary, the test *is* the generator.
No new deps. **Completes the M2-01 action-registry group** (01a keymap core / 01b
sequences / 01c config wiring / 01d help overlay / 01e generated doc); next is the
M2 app shell (root model, browse view, table).

### D52 — M2 app-shell decomposed into ordered, leg-sized Backlog slices
**2026-07-19.** With the M2-01 action-registry group complete (D47–D51), the rest
of M2 was a single prose paragraph on the board — no pickable item for the next
agent. **This planning leg turns the M2 milestone scope + exit criteria into a
dependency-ordered task list** (M2-02 … M2-14), so `/do-rewrite-leg`'s "take the
top unblocked Backlog item" has real input again. **Why a whole leg:** the skill
sanctions "expanding a thin milestone section into concrete small tasks" as a leg
in itself; doing it once, deliberately, beats each subsequent agent re-deriving
the breakdown ad hoc (and risking overlap). **The slicing:**
- **M2-02** msg types + channel→msg pumps (`internal/tui/msg.go`) — pure, the
  first pickable slice (no UI deps); everything downstream consumes these msgs.
- **M2-03** lipgloss theme/styles → **M2-04** status bar → **M2-05a/b** resource
  menu (static seed, then discovery reconcile) → **M2-06a/b/c** custom table
  (snapshot render / live watch deltas / horizontal scroll) — the components,
  built bottom-up so each lands green and testable before the shell composes them.
- **M2-07a/b/c/d** root app shell (`internal/tui/app.go`, replaces the M0
  `tui.go` placeholder): keymap/sequencer-routed skeleton + help overlay + quit;
  two-pane browse layout + focus; live table ↔ `kube.Watch` wiring; async
  discovery reconcile.
- **M2-08** namespace picker · **M2-09** filter · **M2-10** confirm/prompt modal ·
  **M2-11** config menu persistence · **M2-12** legacy `~/.kubecom.yaml` migration
  · **M2-13** column sort (#85) · **M2-14** teatest coverage.
Package layout follows REWRITE_PLAN (`internal/tui/{app,msg}.go`,
`internal/tui/styles`, `internal/tui/components/*`, `internal/tui/views/*`); every
slice preserves zero shared mutable UI state (principle 1, D1). The custom table
(not `bubbles/table`) and the "reconcile discovery without disturbing selection"
constraint are carried from the M2 milestone's risks. Ordering is a default, not a
contract — a later agent may re-split a slice that proves too big (the skill's
split-and-take rule still applies per leg). Board-only; no code, `make check` green.

### D53 — TUI msg boundary: one-item channel→msg pumps; errors bridged, discovery kept whole
**2026-07-19 (M2-02).** `internal/tui/msg.go` is the single boundary between the
concurrent `kube` layer and the single-threaded Bubble Tea update loop. The kube
layer returns work on channels (watch deltas, the cap-1 discovery-ready signal);
Bubble Tea consumes `tea.Msg`. The pumps are `tea.Cmd` adapters that read
**exactly one** item from a kube channel and return it as a message — the only
sanctioned crossing of the goroutine boundary, so no UI state is shared/mutated
across goroutines (principle 1, D1). One-receive-per-Cmd means the model re-issues
the pump after each delivered msg to pull the next, and `Update` never blocks on
more than a single receive. Concrete choices:
- **`watchPump`** maps a data delta → `ResourceEventMsg` (event carried verbatim),
  a watch `ERROR` event → a classified `ErrorMsg` (`NewErrorMsg("watch", err)` →
  `kube.Classify`), and a **closed** channel → the terminal `WatchClosedMsg`. The
  closed case is an explicit message (not a nil) precisely so the model stops
  re-issuing the pump — re-receiving from a closed channel would busy-loop.
- **`discoveryPump`** always returns **`DiscoveryReadyMsg`** carrying the whole
  `kube.DiscoveryResult`, **including a total failure** (`Result.Err`) and the
  isolated per-group failures (`Result.Failed`). A total failure is *not* swapped
  for a generic `ErrorMsg`: discovery is a reconcile signal and the model wants the
  err + isolated detail together, classifying `Result.Err` itself when rendering. A
  closed-with-no-value channel → nil msg (ignored), never a panic.
- **`ErrorMsg`** is the generic degrade-one-feature carrier (#86, principle 3):
  `{Context, Err, Kind}` with `Kind = kube.Classify(Err)` fixed at construction via
  `NewErrorMsg` so it can never drift from `Err`.
- **Size** uses Bubble Tea's own `tea.WindowSizeMsg`; kubecom defines no size msg.
  Cross-component selection msgs (`ResourceSelectedMsg` from the menu,
  `RowSelectedMsg` from the table) live here so producer and consumer share one
  type. No new deps; pure, hermetically fake-channel tested; `-race` clean.

### D54 — M2-03: styles package = Theme (named colors) → Styles (derived lipgloss); lipgloss v2 promoted to direct
**2026-07-19 (M2-03).** `internal/tui/styles` is the single source of visual
truth: a **`Theme`** (a struct of named, semantic `color.Color` fields — no
styling) and a **`Styles`** (the `lipgloss.Style` values every component renders
through), with `New(Theme) Styles` the one place a color becomes a style.
Rationale and locked choices:
- **Named roles, not literals.** Components ask for a role (`s.Selection`,
  `s.Error`, `s.PaneFocus`) and never call `lipgloss.Color`/`NewStyle`
  themselves, so re-theming (M4 theme picker, D6's ported monokai/solarized) is a
  matter of building a `Styles` from a different `Theme`. `Styles.Theme` is
  retained so a component that needs a raw `color.Color` (e.g. a bubbles widget
  that wants a color, not a Style) can reach one without a second palette.
- **Pure, immutable, copyable.** A `Theme` is plain data; `lipgloss.Style` is an
  immutable value type (every setter returns a copy), so a `Styles` is safe to
  copy into any model and read concurrently — no shared mutable UI state
  (principle 1, D1). `New` touches no global (lipgloss v2 dropped the global
  renderer), so it is deterministic: same Theme in, equal styles out (tested).
- **DefaultTheme** is a dark-friendly truecolor palette with a blue accent (a nod
  to the original kube-commander). Terminals without truecolor downsample at
  write time via the Bubble Tea renderer, so the theme carries no per-terminal
  branching (v2 has no `AdaptiveColor`; downsampling is a write-time concern).
- **`Default()`** = `New(DefaultTheme())`, the set the app uses until a theme is
  chosen. Roles shipped: App/Subtle/Selection/Header/Pane/PaneFocus (rounded
  border, accent when focused)/StatusBar/Error/Warn/Success/Spinner — the surfaces
  M2-04…M2-10 render (status bar, menu, table, modal, spinner).
- **Dep:** `charm.land/lipgloss/v2` v2.0.0 **promoted indirect→direct** (was
  transitive via bubbles, D50). No version change, `go.sum` untouched — MVS
  already had it; this is a go.mod require-block move only.

### D55 — M2-04: status bar renders purely from props; spinner ticks gated on discovering
**2026-07-19 (M2-04).** `internal/tui/components/statusbar` is kubecom's bottom
bar: `context · namespace · [spinner] discovering…` on the left, the keymap
short-help hint right-aligned. Locked choices:
- **Pure render from props.** The bar owns no shared state; the root model sets
  `SetContext`/`SetNamespace`/`SetShortHelp`/`SetWidth` and reads `View()`. The
  short-help is generated upstream from the effective keymap (`help.Model.
  ShortHelpView()`, D11) and handed in **as a string**, so the bar never matches
  a raw key or knows what a binding does — it only lays out the pieces it's given.
- **Spinner ticks gated on `discovering`.** The bar embeds a `bubbles/spinner`
  (styled with the `Styles.Spinner` accent role, D54). `StartDiscovery()` sets
  the flag and returns the seed `spinner.Tick` Cmd; `Update` forwards a
  `spinner.TickMsg` to the spinner **only while discovering**, so `StopDiscovery()`
  breaks the self-scheduling tick chain and the animation stops on the next tick —
  no timer to cancel, no goroutine (principle 1). Spinner state is model-local,
  not shared, so this is not the mutex-guarded UI state D1 forbids.
- **Width-aware layout.** With a known width the help is pushed to the right edge
  (`gap = width − W(left) − W(right)` spaces) and the line is clamped
  (`Style.Width(w).MaxWidth(w)`); when the gap can't fit both, the live left
  segment wins and the hint is dropped. Width 0 (pre-`WindowSizeMsg`) joins the
  pieces inline. Empty pieces are skipped so a missing namespace leaves no
  dangling separator.
- **Deps:** none new — `bubbles/v2/spinner`, `bubbletea/v2`, `lipgloss/v2` all
  already direct. First `internal/tui/components/*` package; the layout the rest
  of M2's components follow (New(styles), prop setters, value-receiver View).

### D56 — M2-05a: resource menu owns its "resource selected" message (emitter owns the type)
**2026-07-19 (M2-05a).** `internal/tui/components/menu` is the browse view's left
pane: a static-seeded vertical list of resource kinds, navigated through keymap
actions, that emits a selection message when the user drills in. Locked choices:
- **A component-emitted message is owned by that component's package, not by
  package `tui`.** The root model (package `tui`, M2-07) imports the component
  packages, so a component importing `tui` to build a `tui`-declared message would
  cycle. `ResourceSelectedMsg{Resource kube.Resource}` therefore lives in the
  `menu` package (the emitter); the root model handles the concrete
  `menu.ResourceSelectedMsg`. The pre-declared placeholder `tui.ResourceSelectedMsg`
  is **removed** from `msg.go`; the same rule will move the table's
  `RowSelectedMsg` into the table package in M2-06. `msg.go` keeps only the
  kube-boundary messages + the generic `ErrorMsg`, which no component originates.
- **Static seed, discovery-independent.** `seedItems()` is a fixed core-resource
  list (namespaces/nodes/events · pods/deployments/statefulsets/daemonsets/
  replicasets/jobs/cronjobs · services/ingresses · configmaps/secrets/
  serviceaccounts · pvcs/pvs/storageclasses), each a real `kube.Resource`
  (GVK/GVR/scope) so drilling in gives the kube layer everything List/Watch needs.
  The seed lets the browse view render and be navigated before discovery completes
  (fast cold start, principle 4); M2-05b reconciles it against `DiscoveryReadyMsg`.
- **Display title = `GVK.Kind`.** Canonical, correctly-cased, and works uniformly
  for built-ins and CRDs (no ad-hoc pluralisation). Adjustable later if the UX
  wants plurals.
- **Actions in, not keys.** `Update(a keymap.Action) (Model, tea.Cmd)` moves the
  highlight (up/down/top/bottom, clamped) and, on `nav.drillIn`, emits
  `ResourceSelectedMsg` for the highlighted item; an **unavailable** item (M2-05b)
  is a no-op on drill-in. The menu never matches a raw key (D11). A vertical scroll
  `offset` keeps the cursor visible so a short pane / long list still works;
  page/half-page actions are left to a later slice. Pure value-receiver `View`
  frames the list with `Pane`/`PaneFocus` and highlights the cursor with
  `Selection` (D54); returns `""` until sized. No shared mutable state (principle 1).
- **Deps:** none new. Imports `kube` (Resource) + `apimachinery/.../schema` (to
  build the seed GVK/GVR) + `keymap` + `styles` + `bubbletea/v2` (Cmd).

### D57 — M2-05b: menu reconcile merges into the ordered seed; append extras, never blank
**2026-07-19 (M2-05b).** `(*Model).Reconcile(kube.DiscoveryResult)` folds the async
discovery result into the M2-05a static seed. Locked choices:
- **Merge, never replace.** Reconcile mutates the seed *in place* and appends —
  it never rebuilds the item slice from discovery. So a **total failure**
  (`Result.Err != nil`) is a no-op (the seed stays fully navigable), and a partial
  failure still leaves the user a working menu (principle 3: degrade, don't blank).
  This is the fix for the original's central bug (a broken aggregated API blanking
  the whole menu).
- **Twin match by exact GVR.** A seed item with a discovered twin (same GVR) takes
  the twin's discovery metadata (verbs/short-names/categories) and is confirmed
  `Available`; the seed's curated **title and order are kept** (discovery order is
  used only for appended extras).
- **Unavailable = failed group AND no twin.** A seed item is marked
  `Available = false` (rendered muted, a no-op on drill-in) only when its API
  **group** appears in `Result.Failed` and it has no twin. Match on group (not
  group/version) so a seed item pinned to a version differing from the failed one
  is still caught. A seed item with neither a twin nor a failed group is left
  untouched (conservative — absence alone is not proof of unavailability).
- **Extras appended after the seed**, in discovery's stable sorted order — CRDs and
  extra groups the curated seed omits. Keeps the familiar core kinds at the top;
  reorder/customization is M2-11's job, not reconcile's.
- **Selection & scroll preserved (the M2 risk item).** The highlighted item's GVR
  is resolved back to its post-merge index and the scroll offset re-clamped, so
  reconcile never moves the cursor or jumps the view. Append-only makes the index
  stable anyway; the GVR re-resolve keeps the guarantee robust against future
  reordering.
- **Deps:** none new. Adds an `apimachinery/.../schema` import to `menu.go` (already
  used in `seed.go`).

### D58 — M2-06a: custom table renders a server-printed snapshot, priority-0 columns, clip-not-wrap
**2026-07-20 (M2-06a).** `internal/tui/components/table` is the browse view's right
pane: a custom table (bubbles/table is too basic for the live watch/hscroll needs —
the M2 risk item) that renders a `kube.Table` snapshot. Locked choices:
- **Priority-0 columns by default.** Only columns with `Priority == 0` are shown,
  matching `kubectl get`'s narrow view (`Priority > 0` are the `-o wide` extras). If
  the server sends no priority-0 column, *all* columns are shown rather than a blank
  table (degrade, don't blank — principle 3). A wide/narrow toggle is a later slice.
- **Clip, don't wrap (this slice).** Each rendered row is hard-clipped to the pane
  width with a rune cut (`truncate`), not wrapped, so one logical row is always one
  display line. Horizontal scroll for wide tables is **M2-06c**; until then a wide
  table is cut at the right edge.
- **Bordered-pane sizing gotcha.** lipgloss counts a `Border` *inside*
  `Style.Width`/`Height`, so a bordered `Pane` must be sized to the component's
  **total** width/height (`m.width`/`m.height`); its content area is then the inner
  `(width-2)×(height-2)` region the header+rows are rendered to. Sizing the frame to
  the inner width instead leaves the content area two columns short and wraps every
  full-width row. Guarded by `TestViewFitsPaneNoWrap`. (The `menu`/`statusbar` panes
  render short lines that never hit this, but should adopt the same sizing.)
- **Owns its emitted message (D56).** `table.RowSelectedMsg` lives in the table
  package (drill-in emits it); the forward-reference in `tui/msg.go` is resolved.
- **SetTable resets selection** to the first row (snapshot replace). M2-06b will add
  live watch-delta application that *preserves* the selection instead.
- **Deps:** none new (`kube`, `keymap`, `styles`, bubbletea, lipgloss all present).

### D59 — M2-06b: table applies live watch deltas keyed by object UID, preserving the selection
**2026-07-20 (M2-06b).** `(*table.Model).ApplyEvent(kube.WatchEvent)` folds one live
watch delta onto the M2-06a snapshot. Locked choices:
- **UID is the row identity.** Rows are matched by `ObjectRef.UID` (`indexOfUID`);
  ADDED/MODIFIED **upsert** (update in place, or append when the UID is unseen),
  DELETED removes the matching row. An **empty UID never matches** — a degraded row
  with no object metadata (principle 3) can't be identified, so it is always appended
  rather than collapsed with another empty-UID row.
- **MODIFIED of an unknown UID is treated as an add** (append), mirroring the server
  sending a modify for an object that entered scope before the watch synced it.
- **RESET preserves the selection too.** RESET replaces columns+rows, but the watch
  layer emits RESET on the first sync *and on every reconnect* (D34-era re-List), so
  preserving the selected UID across it keeps the cursor put through a transient
  reconnect; it falls back to the first row only when the selected object is gone.
  (This differs from `SetTable`, which deliberately resets to the top — that's the
  fresh-List-for-a-newly-selected-resource path.)
- **Selection preservation model.** The selected UID is captured *before* the delta,
  then re-resolved to its new index after; if the row is gone the cursor **keeps its
  index position** (clamped to the new range), so deleting the selected row lands on
  the next row rather than jumping to the top (k9s-like). `computeColumns` reruns
  after every delta (a new/wider cell can widen a column) and `clampOffset` re-scrolls
  so the selection stays visible.
- **ERROR never reaches ApplyEvent** — the M2-02 watch pump bridges a watch ERROR to
  an `ErrorMsg`, so `ApplyEvent` only ever sees data deltas; unknown event types are
  ignored. Pure single-threaded (the root model calls it from `Update`); no shared
  mutable state (principle 1).
- **Deps:** none new.

### D60 — M2-06c: table scrolls horizontally on nav.left/nav.right, snapping to column boundaries
**2026-07-20 (M2-06c).** The resource table can now be wider than its pane (the
server-printed column set for a resource often overflows a split-pane width). It
scrolls horizontally instead of only clipping the right edge (D58). Locked choices:
- **Reused `nav.left`/`nav.right` (`h`/`l`), no new action.** The board specified
  left/right; those actions already exist in the registry, so nothing changed in the
  keymap, config surface, help, or `docs/keybindings.md` (the golden drift-check stays
  green). The table's `Update` handles `ActionLeft`/`ActionRight` **before** the
  empty-rows guard, so a header-only table (columns synced before the first rows) can
  still scroll.
- **One horizontal offset (`hoffset`, in display columns) windows the whole block.**
  Every rendered line — header and each data row — is padded to the same content width
  (each cell padded to its column width), so a single offset windows them identically
  and columns stay aligned across the scroll. `hclip` replaces the old `truncate`:
  it returns runes `[hoffset, hoffset+innerW)`; the enclosing lipgloss style pads a
  short window back out to width (no-wrap invariant D58 preserved — guarded test still
  passes).
- **Scroll snaps to column starts.** `scrollRight` advances `hoffset` to the next
  visible column's start (leftmost hidden column becomes flush-left); `scrollLeft`
  retreats to the previous column start, or 0. This reads better than a fixed
  char-step and needs no magic constant. **Fallback:** when no column start remains
  within range (a final column wider than the pane), `scrollRight` snaps to
  `maxHOffset` so that column's tail is still reachable; `scrollLeft` always reaches 0.
- **Offset is state that must stay valid.** `SetTable` resets it to 0 (a fresh
  resource starts fully-left, mirroring the vertical reset-to-top); `SetSize` and
  `ApplyEvent` re-clamp it via `clampHOffset` (a resize or a column-width change from a
  delta can widen/narrow the content). Exposed `HOffset()` for tests and for M2-07b's
  future pane-focus-vs-scroll arbitration (compare before/after a left/right to detect
  an edge). Pure render, no shared mutable state (principle 1).
- **Deps:** none new.

### D61 — M2-07a: root app model owns the keymap + Sequencer; a generation-tagged timeout tick
**2026-07-20 (M2-07a).** The M0 `internal/tui/tui.go` placeholder is replaced by the
real root model in `internal/tui/app.go` — the M2 app shell, built up across
M2-07a..d. This slice (07a) is the keymap-routed **skeleton** (no panes yet). Locked
choices:
- **The root model owns the resolved `*keymap.Keymap` and the one `*keymap.Sequencer`.**
  Every `tea.KeyPressMsg` is fed to `seq.Input(msg.Key())`; the model never matches a
  raw key (D11). The Sequencer is a **pointer** field so its buffered prefix survives
  the value-model copy Bubble Tea makes each `Update`, but it is only ever touched from
  the single-threaded update loop — no shared mutable state across goroutines
  (principle 1).
- **The keymap runs no timer (D48); the model schedules it.** On `ResultPending` the
  model returns `tea.Tick(keymap.SequenceTimeout, …)` producing a private
  `seqTimeoutMsg`; on receipt it calls `seq.Timeout()` and fires the result. This keeps
  the keymap package pure.
- **Timeout ticks are generation-tagged to drop stale ones.** `seqTimeoutMsg` carries
  the `seqGen` value current when it was scheduled; `seqGen` is bumped on every new
  pending. A tick whose `gen` ≠ the model's current `seqGen` was superseded by a newer
  pending and is ignored — otherwise a leftover timer from an already-resolved prefix
  could fire a *newer* pending's short form early (e.g. `g`⟨pend⟩ `gg`⟨resolve⟩ `g`⟨pend⟩
  → the first tick must not fire the second `g`). A tick that finds an empty buffer is
  already inert via `Timeout()`; the gen guard covers the newer-pending case.
- **Actions serviced today:** `app.quit`→`tea.Quit`, `app.help`→toggle the M2-01d help
  overlay, `nav.back`→close the overlay when open (else inert). All nav/filter/search
  actions are inert no-ops until the browse panes land (M2-07b onward).
- **`New()` uses `DefaultKeymap`; `NewWithKeymap(*Keymap)` accepts a config-merged one**
  so the command layer can hand in the resolved keymap (M2-01c/M2-11) without `tui`
  importing `config` (one-way dependency).
- **View draws nothing until the first `WindowSizeMsg`** (never size a layout to a zero
  canvas); when sized it shows a placeholder body (or the help overlay when open) plus
  a one-line short-help hint generated from the registry.
- **Deps:** none new.

### D62 — Two-pane browse layout: focus switching via nav.left/nav.right
**2026-07-20.** M2-07b composes the root model's browse view: the M2-05 resource
menu (left pane) and the M2-06 resource table (right pane) side by side under the
M2-04 status bar (bottom line).
- **Layout.** The status bar takes one line at the bottom; the menu and table split
  the remaining width. `menuPaneWidth = total/4`, floored at `minMenuWidth` (20) and
  never leaving the table below `minTableWidth` (20) — on a narrow terminal the two
  split evenly. Both panes are sized to their **total** width/height incl. border (as
  their `SetSize` expects); the table pane width is `total − menuW` so the two fill the
  row exactly. `View` is empty until the first `WindowSizeMsg`.
- **Focus.** Exactly one pane holds focus (accented border via `PaneFocus`); the menu
  starts focused (pick a resource before drilling into its table). `nav.right` moves
  focus menu→table; `nav.left` moves focus table→menu **only when the table is at its
  left edge** (`HOffset()==0`, nothing to scroll) — otherwise `nav.left` scrolls the
  table's columns. This is the pane-focus-vs-scroll arbitration the table's `HOffset`
  accessor was added for (D60). Every non-switching nav action is routed to the focused
  pane's `Update`; the blurred pane is untouched.
- **Help overlay swallows navigation.** While the help overlay is open, `handleAction`
  services only quit/help/back and drops nav actions, so the panes underneath don't
  move behind the overlay.
- **Watch/discovery deferred.** The menu's `ResourceSelectedMsg` (drill-in) and the
  table's `RowSelectedMsg` are emitted but not yet handled by the root — starting the
  `kube.Watch` on selection is M2-07c and the discovery reconcile + status-bar spinner
  is M2-07d.
- **Deps:** none new.

### D63 — M2-07c: drilling into a resource starts a live kube.Watch, streamed into the table
**2026-07-20.** M2-07c wires the menu's `ResourceSelectedMsg` (drill-in) to a live
`kube.Watch`, feeding its deltas into the table through the M2-02 watch pump.
- **A narrow `ResourceWatcher` seam, not the concrete client.** The root model
  depends on an interface — `Watch(ctx, kube.Resource, namespace, metav1.ListOptions)
  (<-chan kube.WatchEvent, error)` — that `*kube.Clients` satisfies. The tui package
  never constructs a client, and the model is driveable in hermetic tests with a fake
  watch channel (D18). A model built with no watcher is **watch-inert**: selecting a
  resource is a no-op, which is what the pre-launch app and the M2-07b tests want.
- **Constructors take functional options.** `New(opts ...Option)` /
  `NewWithKeymap(km, opts ...Option)` keep their existing call sites working (no
  watcher) while `WithWatcher(w)` injects the client; future slices add their own
  options (e.g. a discoverer in M2-07d) without churning the signature.
- **Selecting a resource (re)starts the watch.** The previous watch's context is
  cancelled, the table is blanked (`SetTable(kube.Table{})`), and a new
  `context.WithCancel(context.Background())`-scoped watch is opened. `Watch` lists
  internally before streaming, so its **first RESET event repopulates the table** —
  no separate List call. Focus moves to the table (drilling *in* is the gesture to
  start browsing rows; `nav.left` at the table's left edge returns to the menu, D60).
  A `Watch` start error surfaces a classified `ErrorMsg` and leaves no watch state.
- **Watch-pump messages are generation-tagged (`watchMsg{gen, msg}`).** Every pump is
  tagged with the `watchGen` current when issued (bumped on each new selection). A
  message whose gen ≠ the model's current `watchGen` comes from a superseded watch
  whose channel is still draining after cancellation, and is **dropped** — it must not
  mutate the table now showing a different resource, nor re-issue a pump that would
  then read the *current* watch's channel and put a second reader on it. This is the
  same stale-message guard `seqGen` gives the sequence timeout (D61). `ResourceEventMsg`
  → `table.ApplyEvent` + re-issue the pump; a watch `ErrorMsg` re-issues the pump (the
  watch loop retries and re-lists on recovery — visible error surfacing on the pane is
  a later slice); `WatchClosedMsg` clears the channel and ends the chain (no re-issue).
- **Namespace scope is "" (all) for now**; the namespace picker re-scopes it in M2-08.
  `app.quit` cancels the current watch before `tea.Quit` so its goroutine unwinds.
- **Deps:** none new.

### D64 — M2-07d: async discovery on Init reconciles the menu + drives the status-bar spinner
**2026-07-20.** M2-07d kicks off the async discovery pass on startup and folds its
result into the resource menu (M2-05b `Reconcile`), running the M2-04 status-bar
spinner while it is in flight — the "discovery ready" reconcile of D8, now wired
into the shell.
- **A narrow `Discoverer` seam, mirroring `WithWatcher` (D63).** The root model
  depends on an interface — `StartDiscovery(ctx) <-chan kube.DiscoveryResult` — that
  `*kube.Clients` satisfies, injected by a new `WithDiscoverer(d)` option. The tui
  package never constructs a client; the model is driveable in hermetic tests with a
  fake channel (D18). A model built with **no discoverer never runs discovery** — the
  menu stays on its static seed, which is itself a fully navigable browse experience
  (principle 4: fast cold start never blocks on discovery).
- **Init defers the start one message hop (`startDiscoveryMsg`).** `Init()` is a value
  receiver returning only a `tea.Cmd`, so it cannot store the cancel func or flip the
  spinner's `discovering` flag. It therefore emits a private `startDiscoveryMsg`;
  `Update` handles it, where the model is mutated and returned — the same place every
  other state change lands. `startDiscovery` opens a `context.WithCancel` pass, calls
  the discoverer, starts the spinner (`status.StartDiscovery()`), and **batches** the
  spinner tick with the M2-02 `discoveryPump` (cap-1 channel, delivers once, D8).
- **Spinner ticks are forwarded to the status bar.** The root `Update` routes
  `spinner.TickMsg` to `status.Update`; the bar drops ticks once discovery finished,
  so the animation self-terminates (M2-04) — no timer to cancel.
- **`DiscoveryReadyMsg` stops the spinner and reconciles.** `handleDiscovery` calls
  `status.StopDiscovery()`, cancels the one-shot context (its result is in hand), and
  `menu.Reconcile(result)` — which merges without disturbing selection/scroll (D57)
  and is a **no-op on a total failure** (`Result.Err` set): the menu then stays on its
  navigable seed rather than blanking (principle 3). Visible surfacing of a discovery
  failure on a pane is a later slice, as with the watch's ERROR handling (D63).
- **`app.quit` cancels the in-flight discovery pass** alongside the watch, so its
  goroutine unwinds before the program exits (the cap-1 channel already prevents a
  leak, but cancelling drops the result promptly).
- **Deps:** none new (reuses the M2-02 pump, M2-04 spinner, M2-05b `Reconcile`).

### D65 — M2-08a: generic modal picker over bubbles/list, driven by keymap actions
**2026-07-20.** M2-08 (the namespace switcher) is too large for one leg, so it is
split: **08a** is the picker component in isolation, **08b** adds filtering, **08c**
wires it into the app shell (a `ns.switch` action + a `NamespaceLister` seam that
re-scopes the watch). This decision covers 08a.
- **A generic, kind-stamped picker.** `internal/tui/components/picker` chooses one
  string value from a set. It is generic (values are plain strings) and carries a
  `kind` ("namespace" first) stamped into its result messages, so the same component
  is reused for the later context/container/port pickers and the root model can tell
  which picker resolved. Namespace-specific plumbing lives in 08c, not the component.
- **Wraps bubbles/list but is driven by keymap actions, never raw keys (D11).** The
  picker holds a `list.Model` for cursor + pagination management (and, in 08b, its
  native filter), but `Update` takes a `keymap.Action` and calls the list's public
  cursor methods (`CursorUp/Down`, `GoToStart/End`, `Prev/NextPage`) — it never feeds
  raw key messages to the list. The list's own key bindings and chrome (title, help,
  status bar, pagination, filtering, quit keys) are all **disabled** so no hard-coded
  key or help string leaks into the view; the picker frames and titles the list
  itself through the shared `styles` (Header title, PaneFocus border, Selection cursor
  row via a minimal one-line `itemDelegate`). This keeps the "zero hard-coded keys"
  invariant intact even though bubbles/list ships its own keymap.
- **Emitter owns its messages (D56).** Drill-in emits `picker.SelectedMsg{Kind,Value}`,
  back emits `picker.CancelledMsg{Kind}` — both owned by the picker package so it never
  imports the root package (which imports it). The picker does **not** hide itself on
  select/cancel; it leaves that to the root model (08c), keeping the component free of
  app-flow assumptions.
- **Modal geometry.** `SetSize` takes the *full screen* size; the picker computes a
  clamped modal box (a fraction of the screen within [24,60]×[5,20], never exceeding
  the screen) and `View` centers it with `lipgloss.Place`. A hidden or unsized picker
  renders `""`. No shared mutable state (principle 1): the root model owns the one
  Model, feeds it actions, reads its View.
- **Deps:** `bubbles/list` pulls two new **indirect** transitives — `github.com/atotto/
  clipboard v0.1.4` and `github.com/sahilm/fuzzy v0.1.1` (via `textinput`/`list`). Added
  by `go mod tidy`; no existing version moved (minimal-dep discipline, D45).

### D66 — M1 completion bar: fake-client coverage; envtest integration tests deferred
**2026-07-20.** Maintainer-approved (progress review). M1 is declared
**feature-complete** on: all in-process feature work (M1-00…M1-09) done, the
hermetic **fake-client** suite covering discovery, watch reconnect (410 → re-List),
and the full action set, and the verified **zero-TUI-imports** invariant. The two
exit criteria that require a **live apiserver** — the restricted-RBAC group-isolation
integration test, and action-set/watch coverage against real etcd — are **deferred**
to backlog item **M1-INT** (opt-in `KUBECOM_TEST_ENVTEST=1`). **Why:** envtest needs
control-plane binaries that are fragile in the sandboxed cloud env the autonomous loop
runs in (D18); blocking M1 on them would leave it perpetually "in-progress" while all
downstream work proceeds. Fault isolation itself is implemented and unit-covered — only
the live-cluster *proof* is deferred. **Consequence:** M1-INT is run by a human locally
or a dedicated CI job with `setup-envtest`; it is not a blocker for M2–M5.

### D67 — Vault hygiene: decisions are constraints, not changelog; status lines stay terse
**2026-07-20.** Maintainer-approved (progress review). Two anti-drift rules for the
autonomous loop, after the decisions log and status lines had swollen into per-leg
narratives:
- **`decisions.md` is load-bearing only.** A `Dn` records a choice a *future* leg must
  not silently contradict (library, API shape, invariant, tradeoff). How a given leg was
  implemented goes in the **journal**, not here. The leg loop skims this file every leg,
  so it must stay signal.
- **Status lines are one sentence.** A milestone's `Status:` line and the board's
  `Last updated:` line are a single terse sentence each; the journal is the changelog.
  Do not append a running per-leg history to either.
This does not supersede earlier `Dn` entries (they stand); it sets the bar going forward.

### D68 — Runnable + dogfooded: land the launch leg, then keep it launchable; README stays current
**2026-07-20.** Maintainer-directed (progress review). A review found the binary
had never launched the TUI or connected to a cluster — `tea.NewProgram` was called
nowhere and no real `*kube.Clients` was built outside tests — so despite a
feature-complete kube layer and a fake-tested app model, nothing had been exercised
end-to-end against a real apiserver, and **no leg in the plan wired it up**. Fix:
- **M2-RUN** (new, the next pick, before the remaining M2 component slices): bare
  `kubecom` builds a real client from kubeconfig/context/namespace flags, injects it
  via `WithWatcher`/`WithDiscoverer`, and runs `tea.NewProgram(...).Run()`. Minimal
  but launchable, with a manual real-cluster smoke as part of "done".
- **Dogfooding is a testing rule, not a one-off.** A human periodically installs
  `kubecom` and runs it against a real cluster between reviews; every leg from
  M2-RUN on must keep the binary launchable and **incrementally improve — never
  regress — the real-cluster experience**. Fake-tested parts are not "done" until
  they work in the running binary. Added as goals.md principle 8 + the leg loop's
  Verify step.
- **README stays current.** Any leg that changes how a user installs, launches,
  configures, or uses `kubecom` updates `README.md` in the same leg (CLAUDE.md hard
  rule + the leg skill). The v1 README now carries a real install + usage guide that
  legs keep in sync with the built binary.
**Why:** an autonomous, fake-everything, DI-everywhere loop can accrete well-tested
libraries that never integrate; forcing a runnable binary early turns "tested parts"
into "a thing that runs" and surfaces real client-go/terminal behavior while the
surface area is still small.

### D69 — Feedback inbox: `vault/feedback/`, drained before every leg, deleted once addressed
**2026-07-20.** Maintainer-directed. A human → agent inbox lives at
`vault/feedback/` (format mirrors the journal: one markdown file per item,
`YYYY-MM-DD-slug.md`, title + optional Priority/Area + free-form prose; `README.md`
is the only non-item file). **Every leg's Orient step lists it**, and unaddressed
feedback **preempts the board**: if any item is present, the oldest / highest-priority
one *is* that leg's work. Addressing = implement it (small), or triage it into concrete
board task(s) and do the first slice (large), or decide+record it (question/direction;
new `Dn` if load-bearing). The feedback file is **deleted in the same leg's commit**
and linked from the journal entry — the inbox is a live to-do list, not an archive, so
a handled item is never left to be re-read. **Why:** the loop runs with no synchronous
human; a durable, in-repo inbox lets the maintainer steer between reviews (bugs found
dogfooding per D68, priority changes, course corrections) without waiting on a chat.
Wired into CLAUDE.md's leg loop, the `do-rewrite-leg` skill (Orient + Pick), and
`vault/README.md`.

### D70 — bubbletea v2 full-screen is a `View` property, not a program option
**2026-07-20.** M2-RUN wired `tea.NewProgram`. The board's sketch called
`tea.NewProgram(model, tea.WithAltScreen())`, but **`tea.WithAltScreen()` does not
exist in bubbletea v2** (v2.0.2) — it was a v1 program option. In v2 the alternate
screen is a field on the view the model returns: `View.AltScreen`. So the root
model requests full-screen mode itself, in `Model.View()` (`v := tea.NewView(...);
v.AltScreen = true`), and the launcher runs a plain `tea.NewProgram(model).Run()`
with no screen option. Any future program-level terminal mode (mouse, focus
reporting, bracketed paste, window title) is likewise a `tea.View` field, set in
`View()`, not a `NewProgram` option — do not reintroduce the v1 option form.
**Why:** v1-era snippets (the board sketch included) mislead here; recording the v2
shape stops the next leg re-deriving it against a missing symbol.

### D71 — TUI logging goes to a file with klog fully off stderr (raise the stderr threshold)
**2026-07-20.** The `stack.md` rule "nothing may write to stdout/stderr while the
TUI owns the terminal" needs an explicit klog step: **`klog.LogToStderr(false)` is
not sufficient**, because klog copies every ERROR-level line to stderr regardless
whenever the line's severity meets the stderr *threshold* (default ERROR) — which is
exactly the `memcache.go "couldn't get current server API group list"` line
client-go emits on an unreachable cluster, and it corrupts the alt-screen. The
launcher (`cmd/kubecom/logging.go`, `setupLogging`) therefore, before any client is
built: points `slog` at a log file under the user cache dir
(`os.UserCacheDir()/kubecom/kubecom.log`), and configures klog through a **private
`flag.FlagSet`** (no global flag pollution) with `logtostderr=false`,
`alsologtostderr=false`, `stderrthreshold=FATAL`, plus `klog.SetOutput(logFile)`.
Verified: with an unreachable cluster, the memcache error lands in the log file and
stderr stays empty. Any code that adds a new logging sink must keep it off
stdout/stderr on the TUI path the same way. **Why:** a single stray client-go line
tears a hole in the rendered UI (D68 dogfooding regression); the threshold detail is
non-obvious and cost a debugging round to find.

### D72 — M2-08b: picker filtering is picker-owned (its own textinput), with a control/text key split
**2026-07-20.** The modal picker filters itself: it holds its own
`bubbles/textinput` over the unfiltered value set and narrows the visible list by
**case-insensitive substring** — the list's native filter stays disabled (D65). Two
constraints bind future legs, chiefly the M2-08c app wiring:
- **Input split.** `Update(keymap.Action)` handles control gestures (nav,
  `nav.drillIn`, `nav.back`, and `app.filter` which *opens* the field); raw text goes
  through the separate `UpdateFilter(tea.KeyPressMsg)` entry point. The root model,
  while `picker.Filtering()` is true, must resolve control keys to actions **first**
  and route only the leftover printable/edit keys to `UpdateFilter` — otherwise a
  value bound to a nav action (`j`, `k`, `G`, `n`, …) could never be typed into the
  filter. This is the sanctioned exception to "no view matches a raw key" (D11): a
  text field consuming text is not action binding.
- **Back clears, then cancels.** `nav.back` while filtering closes the filter and
  restores the full list (no `CancelledMsg`); a second `nav.back` cancels the picker.
  `Hide()` also closes the filter so it reopens clean.

**Why:** locks the seam M2-08c wires against, and prevents a future leg from
reintroducing list-native filtering (its own `/` key would leak a raw binding).

### D73 — M2-08c: namespace switch is a `ns.switch` action + a `NamespaceLister` seam; picker re-scopes the current watch
**2026-07-20.** The namespace switcher is wired into the app shell as an action, a
seam, and a re-scope, each a constraint future legs (context/container/port pickers,
config persistence) build on:
- **`ns.switch` action, default `ctrl+n`.** A new `ns` action namespace (its own
  help column / doc section). `ctrl+n` is the default so `:` stays free for a future
  command palette; rebindable like any action (D11). Future switchers get their own
  `<group>.switch` action, not more keys hard-coded in views.
- **`NamespaceLister` seam** (`Namespaces(ctx) ([]string, error)`), wired with
  `WithNamespaceLister`, mirroring `WithWatcher`/`WithDiscoverer`: `*kube.Clients`
  satisfies it, nil → **namespace-switch-inert** (the action is a no-op, the picker
  never opens). Listing runs off the update loop (async `namespacesLoadedMsg`), so
  opening the picker never blocks on the network; a list failure closes the picker
  and surfaces a classified error (principle 3), never crashes.
- **Selection re-scopes the live watch.** The shell tracks the `current` resource
  (set whenever a watch starts); a `picker.SelectedMsg` sets `m.namespace` +
  `status.SetNamespace` and re-selects `current` so the M2-07c watch re-lists under
  the new scope. With no resource open yet the scope is just stored for the next
  drill-in.
- **Concrete input routing (realizes D72's split).** While `nsPicker.Filtering()`,
  the root model routes a keypress to `UpdateFilter` when it carries text **or** is an
  unmapped no-text edit key (backspace); a *mapped* no-text key (esc/enter/arrows/
  `ctrl+d`…) resolves to a picker `Action`. Outside filtering, mapped keys drive the
  picker and unmapped keys are dropped. The open picker captures **all** input before
  the sequencer, so the panes underneath never move.

**Why:** locks the switcher shape so the later ctx/container/port pickers reuse the
action+seam+routing pattern instead of re-deriving it, and keeps the "all input
through actions" invariant (D11) intact around the one sanctioned text field.

### D74 — Errors surface only inside the fixed layout: a transient, single-line status-bar toast
**2026-07-20.** An `ErrorMsg` (any classified error from an async seam — watch
start, namespace list, a watch ERROR bridged by the pump) is surfaced **only** as a
transient message in the **status bar**, never printed to stdout/stderr and never
rendered into a growing/scrolling pane. Constraints future legs must not contradict:
- **The status bar owns error display.** `statusbar.SetError` flattens the text to
  one line (`strings.Fields`) and `View` clips it to the bar width *before* styling,
  so a multi-line or over-wide error can never grow the bar past its single line and
  scroll/resize the panes (the D68 dogfood feedback this fixes). A shown error takes
  the whole line (help hint dropped) and auto-clears after `errorDisplay` (5s).
- **Auto-clear is generation-guarded** (`statusErrGen`, mirroring `seqGen`/`watchGen`):
  each surfaced error bumps the gen and arms an `errorClearMsg{gen}`; only a clear
  whose gen still matches clears the bar, so a newer error keeps its full window.
- **The root `Update` must handle `ErrorMsg`.** Before this leg the top-level case
  was missing, so watch-start and ns-list errors fell through to `return m, nil` and
  were dropped silently; `handleWatchMsg` likewise swallowed watch ERRORs. All three
  now route through `surfaceError`. A future feature that produces errors emits an
  `ErrorMsg` (via `NewErrorMsg`) and gets this surfacing for free — it must not invent
  its own out-of-layout error rendering. A richer surface (modal for fatal errors,
  history) can supersede this, but the no-stdout / no-layout-shift rule is binding.

**Why:** honors the "degrade, don't crash — and don't wreck the layout either"
principle and the stack.md no-stdout-while-TUI rule (D71); gives every async seam one
sanctioned, layout-safe error channel instead of each inventing its own.

### D75 — README install is local-checkout-only until an M5 release tag; no `@v1` remote form
**2026-07-20.** `go install …/cmd/kubecom@v1` is broken and stays out of the README:
`@v1` is a **module version query**, and `v1` matches Go's semver-prefix form (major
version 1), so the toolchain resolves it to a `v1.x.x` **tag** (none exist) and never
falls back to the branch named `v1`. Constraints future legs must not contradict:
- **The primary install path is a local `v1` checkout** (`git clone -b v1 … && go
  install ./cmd/kubecom`), which needs no tag and respects the branch. Do **not**
  reintroduce a bare `@v1` (or `@latest`) remote one-liner before a real release tag
  exists. A commit-SHA pin (`@<sha>`) is not semver-parsed and may be offered as an
  optional remote form.
- **Restoring the clean remote `go install …@latest` is an M5 release task** (tag a
  real `v1.x.x`; the `v1`→`main` rename also dissolves the branch-vs-semver
  collision). The README install section flips back to the remote one-liner only once
  that tag ships (keep it current per D68).

**Why:** addresses feedback FB-go-install (a dogfood install failure); keeps the
documented install command actually runnable on the untagged `v1` branch instead of
failing on a nonexistent semver tag.

### D76 — Right browse pane is a slot: welcome page pre-drill-in, live table after
**2026-07-20.** The right pane of the browse view is a single slot the root model
fills conditionally, gated by `m.hasCurrent`:
- **Before the first drill-in** (`!hasCurrent`) it renders the `welcome` component
  (`internal/tui/components/welcome`) — app name/version, `context · namespace`
  scope, a pick-a-resource hint, and the registry-generated key hints — sized to the
  table's geometry and reflecting the same right-pane focus. A future leg adding a
  right-pane surface (details/logs/describe view) must respect this slot: it shows
  when a resource is open, never blanks the pane, and never a raw stdout write.
- **After a resource is open** the live table takes the slot (unchanged).
- **Context/version are cosmetic props**, injected via `WithContext`/`WithVersion`
  and shown on the welcome page (both) and the status bar (context). `kube.ContextName`
  resolves the context name with no network I/O and returns `""` on any kubeconfig
  failure — the label degrades to blank, it never blocks start (principle 3).

**Why:** addresses feedback FB-welcome-page (a bare launch showed an empty table);
keeps the first paint a deliberate, informative landing screen and fixes the
status bar's context label, which was wired but never set.

### D77 — Resource menu is grouped into Dashboard-style sections with non-selectable headers
**2026-07-20.** The left resource menu renders as **grouped sections**, not a flat
list: `Cluster` → `Workloads` → `Config` → `Network` → `Storage` → `Access Control`,
plus a trailing `Custom Resources` section for discovered CRDs/extra groups.
Constraints future legs must not contradict:
- **Grouping is via static section headers, not collapse/expand.** A header is a
  non-selectable render-only row (styled `styles.Header`); the cursor only ever lands
  on `Item` rows and navigation (`up`/`down`/`top`/`bottom`) skips headers. No new
  expand/collapse action, no per-section open/closed state — keeps navigation trivial
  and aligns with the approachable, non-k9s goal. A later leg may add collapse/expand,
  but it must supersede this decision explicitly, not bolt raw keys onto the menu (D11).
- **`Item.Section` is the grouping key and section members must be contiguous** in the
  item slice. `menu.rows()` walks the items once and emits exactly one header per
  section on a section change, so a section that is split across the slice would
  render a duplicate header. The seed authors sections contiguously; `Reconcile`
  preserves the invariant by appending every discovered extra into the single trailing
  `Custom Resources` section (never interleaving). Reconcile's twin-fill/mark-
  unavailable/selection-preserve behaviour (D57) is unchanged.
- **The scroll offset is a display-row offset** (it counts header lines), not an item
  index. Selection stays visible via `cursorRow()`; scrolling up onto a section's
  first item pulls its header into view.

**Why:** addresses feedback `2026-07-20-menu-structure-nesting` (the flat menu read as
disorganized). Mirrors the original kube-commander's cluster-then-namespaced split and
the Kubernetes Dashboard's Workloads/Config/Network/Storage grouping, giving a familiar
structure that survives CRDs being appended.

### D78 — Table filter is a view over an authoritative unfiltered row set
**2026-07-20 (M2-09a).** The resource table keeps two row sets: `full` (every row
the watch has delivered) and the displayed `table` (full, or full narrowed by the
active `filter`). Constraints future legs (M2-09b app wiring, and any later
filter/search work) must not contradict:
- **Watch deltas mutate `full`, never the filtered view.** ApplyEvent upserts/
  deletes/reset onto `full`, then re-derives the displayed set via `applyFilter`.
  A narrowing filter therefore never drops a live row: clearing it (`SetFilter("")`
  / `ClearFilter`) brings every row back. Cursor/offset/render/selection all operate
  on the displayed set (so `RowCount`/`SelectedRow` mean the *visible* rows;
  `TotalRowCount` is the unfiltered denominator).
- **Matching is case-insensitive substring across the *visible* (priority-0)
  columns only** — never the hidden `-o wide` extras, so the filter matches what the
  user can see. Selection is preserved by object UID across a filter change (cursor
  follows the row if it still matches, else clamps into the narrowed range).
- **`SetTable` clears the filter; `ApplyEvent` preserves it.** A fresh List for a
  newly selected resource (SetTable) must not carry a stale filter from the previous
  resource, but a watch reconnect (a fresh RESET via ApplyEvent, D59) must keep the
  user's filter — the filter is part of the selection continuity D59 protects.
- The component only narrows. The root model owns opening the filter from the keymap
  `app.filter` action + a text field and rendering the active-filter indicator
  (M2-09b); it drives narrowing through `SetFilter`.

### D79 — Human-task queue: `vault/human-tasks/` (agent → human), can block the board / a milestone
**2026-07-20.** Maintainer-directed. The inverse of the feedback inbox (D69): a
directory where the **agent parks work only a human can do** — dogfood/visual-UX
confirmation against a real cluster, credentials/infra it must not fabricate,
running envtest locally, irreversible/outward-facing actions (release tag, default-
branch change, publishing), or a genuine human judgment call. One markdown file per
task (`YYYY-MM-DD-slug.md`; `README.md` documents the flow) with `Blocks:`,
`Priority:`, and `Status: open|done` fields. **Every leg's Orient step lists it**, and:
- An **open** task's `Blocks:` gates Pick — it removes the named board items (or, with
  `milestone:MX`, the whole milestone) from what the agent may start. `Blocks: none`
  is advisory.
- **Blocking never means busywork.** If open tasks gate all available work, the agent
  **stops and reports** the blocker(s) rather than inventing low-value legs; the
  scheduled run ends and its push notification names the blocker. Feedback items and
  bug fixes are never blocked unless a task's `Blocks:` names them.
- A `Status: done` task → the agent folds its `## Result` into the board/journal/a
  decision/feedback and **deletes** the file.
- The agent **raises** a task here instead of claiming a green it couldn't earn (ties
  to the dogfooding rule D68): if a leg needs a human, it files a task with a
  conservative `Blocks:` rather than faking verification.
**Why:** the autonomous loop had no way to hand work back to the maintainer or to
gate progress on it — so real-cluster validation debt (D68) silently accumulated
while the loop built more UI on top. A blocking agent→human queue closes that gap.
Wired into CLAUDE.md's leg loop + hard rules, the `do-rewrite-leg` skill (Orient,
Pick, Verify), the `do-rewrite-run` orchestrator (blocked → end run + notify), and
`vault/README.md`.

### D80 — Table filter wiring: `/` opens a live field, enter commits · esc clears, and n/N step matches with wrap
**2026-07-20 (M2-09b).** The M2-09a filter core is wired into the shell as an
input mode, mirroring the namespace picker's control/text routing (D73). Constraints
future filter/search legs must not contradict:
- **`app.filter` (`/`) opens a live filter field over the current table**, seeded
  with any active filter (reopening edits it). It is a **no-op unless a resource
  table is showing** (`hasCurrent`) — the welcome page has nothing to narrow. While
  the field is open the root routes every keypress through `routeFilterKey`,
  bypassing the sequencer exactly as the open picker does; typing narrows the rows
  live via `table.SetFilter` (D78).
- **Control/text split (D73), reused verbatim.** A *mapped no-text* key
  (esc/enter/arrows/ctrl+d…) is a control action; any text rune or unmapped no-text
  edit key (backspace) is field input. So a bound vim letter like `j` *types* while
  the field is open (it does not navigate); the no-text arrows/page keys move the
  selection to preview matches live.
- **enter commits, esc clears.** enter (nav.drillIn) closes the field keeping the
  narrowed view — normal routing resumes so `j/k` and `n/N` work over the matches;
  esc (nav.back) clears the filter and closes the field (restores every row, D78).
  esc on a *committed* filter (field closed) also clears it — esc exits the filtered
  view. A new resource selection (`SetTable`, D78) resets the shell's filter state.
- **n/N (searchNext/Prev) step matches with wrap.** With a *narrowing* filter the
  displayed rows are exactly the matches, so search is "step to next/prev displayed
  row, wrapping at the ends" (vim search wraps; plain `j/k` clamp) — backed by
  `table.SelectNextWrap`/`SelectPrevWrap`. **n/N are no-ops with no active filter**
  (nothing to iterate); there is no separate match index over an unfiltered list.
- **The active filter surfaces in the status bar** (a left segment: the live input
  view while editing, `/query` once committed) — inside the fixed layout, never a
  new pane (consistent with the D74 error toast). The component still only narrows;
  the shell owns opening, committing, clearing, and the indicator (D78).

**Why:** locks the one text-input interaction the browse view has into the same
action-routed, layout-safe shape as the picker and the error toast, and settles the
n/N-vs-narrowing-filter question (fold search into filter+step, don't maintain a
parallel match cursor) so later search work builds on it instead of re-deciding.

### D81 — M1-04b (lazy group-detail-on-open) retired as obsolete; do not reintroduce a collapsible-group menu for it
**2026-07-20 (M1-04b).** The parked M1-04b item ("fetch a group's full resource
detail only when its menu is opened") is **retired won't-do** — it does not fit the
realized architecture and its intent is already delivered. Two grounds:
- **No group-open interaction exists, by design.** The M2 menu is a *flat*
  Dashboard-sectioned list of resource *kinds* (D77, hardened after the FB-menu-nesting
  feedback); drilling in (`nav.drillIn`) starts a *watch* on the selected kind, not a
  group-detail fetch. There is no collapsible group node to "open," and adding one
  purely to defer discovery would **regress** UX — discovered CRDs/extra kinds would
  vanish from the menu until their group is expanded, the opposite of "everything
  discovered shows up."
- **The cold-start intent is already met.** Non-blocking cold start without an eager
  blocking full-discovery fetch is delivered by the seed set (M1-02, core kinds usable
  instantly), async background discovery (M1-03, `StartDiscovery` never blocks a
  caller), and kubectl-style per-host on-disk discovery caching (M1-04,
  `diskcached`, zero network I/O at construction, TTL 6h). Per-group discovery detail
  is already read/written lazily *by the disk cache*; there is nothing left to defer
  without the (unwanted) group-open UI.
**Constraint:** do not reintroduce a collapsible-group menu or per-group lazy
discovery on the strength of this old item alone — it would need a fresh UX decision
that supersedes D77's flat-menu direction. **Consequence:** M1 has no remaining
open feature work; only the tracked envtest item (M1-INT, D66) is deferred.

### D82 — Menu carries non-resource rows (`Item.Kind`); the namespace picker is a seam row between cluster-scoped and namespaced sections
**2026-07-21 (FB-ns-menu-seam).** The left menu is no longer resource-rows-only. An
`Item.Kind` (`ItemResource` default / `ItemNamespace`) tags each row; the seed
inserts one `ItemNamespace` **seam row** between the cluster-scoped `Cluster`
section and the first namespaced section, so the menu itself communicates the
cluster/namespaced boundary (the feedback's ask). Constraints future legs must not
contradict:
- **A non-resource row has no GVR/`Section`.** It is selectable (cursor lands on it)
  and drilling in emits its own message — the namespace seam emits
  `menu.NamespaceRequestedMsg`, which the root opens the namespace picker on (the
  same effect as the `ns.switch`/ctrl+n shortcut, which stays). It never starts a
  watch.
- **Discovery `Reconcile` skips non-resource rows** (`Kind != ItemResource`): they
  have no discovered twin and no API group, so twin-fill / mark-unavailable /
  `seen` all bypass them, and selection-preservation resolves the seam by kind (it
  has no GVR to match). The D77 grouping invariant (one header per contiguous
  `Section`) is unaffected — the seam's empty `Section` emits no header and sits
  un-grouped between the two.
- **The seam shows the live scope** (`menu.SetNamespace`, "" → "all namespaces"),
  which the root keeps current from the initial `-n` flag and every picker
  selection — alongside the status bar and welcome page.

**Why:** addresses feedback `2026-07-21-01`; establishes the general "special
non-resource menu row" shape (a future context switcher, actions row, etc. reuse
`Item.Kind` rather than each bolting on a parallel concept) without disturbing the
D57/D77 reconcile+grouping guarantees.

### D83 — Per-context menu customization lives in its own file per kubeconfig context, under `<configdir>/kubecom/menus/<sanitized-context>.yaml`
**2026-07-21 (FB-menu-config-01, feedback `2026-07-21-02`).** CRDs and other
resource types the built-in menu doesn't seed are added via a **dynamic,
per-context** menu config — not merged into the single `config.yaml`. Constraints
future slices must not contradict:
- **One file per context**, in a `menus/` subdir of the kubecom config dir (D20),
  keyed by the kubeconfig context name. A context with no file falls back to the
  built-in seed + discovery menu (principle 3 — a missing customization degrades to
  the default, never blocks). Owned by `config.MenuConfig` / `config.MenuResource`
  (the CRD entry format: group/version/resource + optional kind/namespaced/section/
  title; version+resource required, "" group = core).
- **The context name is sanitized to a safe single filename segment**
  (`config.menuFileName`: every char outside `[A-Za-z0-9._-]` → `_`, then `.yaml`).
  This is intentionally lossy — two contexts differing only in sanitized characters
  collide onto one file — chosen as the conservative safety tradeoff so an arbitrary
  context name can never escape `menus/` or split the path. An empty context is an
  error (no per-context file resolvable).
- **The config package stays free of the kube/menu packages.** It defines the
  schema, resolves the path, and loads/validates only; mapping `MenuResource` →
  menu rows and merging with the seed/discovery set is a later slice
  (FB-menu-config-02) so config keeps no import cycle and stays trivially testable.

**Why:** addresses feedback `2026-07-21-02`; different clusters expose different
CRDs, so per-context files keep each cluster's menu relevant. **Consequence:** the
merge and app-wiring slices build on this loader; they must resolve the file via
`config.MenuPath(context)` and treat a missing/half-broken file as "use the default
menu" rather than an error that blocks start.

### D84 — A pane's inner text region is `innerW-2`, not `innerW`: lipgloss borders are border-box for width; size content and clip to the real region

`2026-07-21` · feedback `2026-07-21-03` (menu overflow/scroll).

A bordered pane rendered through `styles.Pane`/`PaneFocus` uses lipgloss v2, where
**the border is border-box for width but content-box for height**:
`frame.Width(P)` produces a block whose *total* width is `P` (its inner text region
is `P-2`, the two border columns eat into it), while `frame.Height(Q)` produces
`Q` *content* rows plus 2 border rows (total `Q+2`). Components size the frame with
`frame.Width(innerW)` where `innerW = width-2`, so the actual usable text columns
are **`innerW-2`**. Sizing content lines to `innerW` (the prior menu code) makes
every full-width line 2 columns too wide; lipgloss then clips or wraps it past the
border — the visible cause of the dogfood menu overflow (long kind names spilling
onto a detached second line below the frame).

- **A viewport component must size its content lines — and clip long text — to the
  real inner region `innerW-2`, not `innerW`.** The menu now does: it clips titles
  to that region with an ellipsis (one line, never a wrap) and reserves the
  rightmost region column for a proportional scrollbar (shown only when
  `rows > visible`; thumb span = visible/total, position = offset/range).
- **This is a latent hazard in the other bordered components** (`table`, `picker`,
  `statusbar`, `welcome`) — they use the same `Width(innerW).MaxWidth(innerW)`
  shape, so their full-width lines are silently truncated by 2 columns. Not fixed
  here (out of this leg's scope); a future leg touching their layout should adopt
  the `innerW-2` region and can lift the menu's `clip` helper.

**Why:** a menu that wraps long CRD kinds outside its border reads as broken (the
dogfood report). **Consequence:** treat `innerW-2` as the drawable width inside any
`styles.Pane` frame; don't reintroduce full-width content sized to `innerW`.

### D85 — The persistent bottom key-hint is focus-aware: menu-context vs table-context curated subsets, chosen by which pane holds focus

`2026-07-21` · feedback `2026-07-21-06` (status-bar key hints).

The always-on status-bar key hint (D11 — registry-generated, `help.Model.ShortHelpView`)
was a single focus-agnostic curated set (`shortHelpActions`). It is now **focus-aware**:
the browse status bar shows the keys relevant to whatever pane holds focus, so the hint
updates as focus moves (the dogfood ask).

- `keymap.HelpContext` (`HelpMenu`/`HelpTable`) names a **focus context, never a key**;
  the concrete keys still come from the registry, so a new context is added by listing
  actions in `contextShortHelpActions`, not by hard-coding keys in a view (D11 holds).
  `HelpKeyMap.ShortHelpContext(ctx)` returns that context's enabled bindings; an unknown
  context falls back to the focus-agnostic `ShortHelp()`.
- **Menu context** offers drill-in + namespace (no filter/search — nothing to filter on
  the menu); **table context** offers filter + next-match + back (no drill-in); namespace,
  help and quit appear in both. The focus-agnostic `ShortHelp()` is retained for the
  **welcome landing page** (no single focused pane there).
- The root model owns the focus→context mapping in one place (`syncHints`) and calls it
  wherever focus switches (drill-in, nav.left/right pane switch, esc focus-pop, filter
  open) and on resize (the hint re-elides to the new width). A future leg adding a focus
  target must call `syncHints` at that switch or the hint goes stale.

**Why:** a hint that shows keys irrelevant to the focused pane (or omits the relevant
ones, e.g. `/` filter while on a table) is noise. **Consequence:** keep hint subsets in
`contextShortHelpActions` keyed by focus context; don't reintroduce a single flat status
hint, and don't hard-code per-context key lists in views.

**Deferred (board `FB-hintbar-dedicated`):** promoting the hint into a dedicated,
always-visible bottom line of its own so it is never dropped under width pressure (today
the status bar still drops the right-aligned hint when the left state segment leaves no
room, and hides it entirely behind an error toast).

### D86 — Mouse is additive and routed through keymap Actions, never raw mouse behaviour in views; enabled per-View via MouseModeCellMotion

`2026-07-21` · feedback `2026-07-21-08` (mouse support). **Partially superseded by
D97 (2026-07-22): mouse capture is now off by default and opt-in via `mouse.toggle`
— the "additive, routed through Actions" invariant here still holds; only the
"enabled per-View unconditionally" clause is replaced.**

Bubble Tea mouse reporting is enabled in the root model's `View` (`v.MouseMode =
tea.MouseModeCellMotion`, alongside `v.AltScreen` — in bubbletea v2 both are View
properties, not program options, D70). The root `Update` handles `tea.MouseClickMsg`
and `tea.MouseWheelMsg`; every other mouse type is ignored.

- **Mouse is strictly additive** (goals principle 6 — vim-first, never vim-only): it
  covers only the headline gestures — left-click a menu item to open it (drill-in),
  left-click a table row to select it, wheel to step the selection of the pane under
  the pointer. The keyboard path stays the primary, complete interface.
- **No view matches a raw mouse event for behaviour** (D11 in spirit): a click/wheel is
  turned into the same `keymap.Action` (nav.up/down, drill-in) or the same public
  Select* call the keyboard drives, so selection/scroll/drill-in stay single-sourced.
  The coordinate→row mapping lives in the components as pure functions
  (`menu.RowItemAt`, `table.RowAt`, both content-row → item/row index, honouring the
  scroll offset and rejecting headers/borders/blanks); the root converts an absolute
  mouse (X,Y) to a body-relative content row and picks the pane by X
  (`menuPaneWidth`).
- **Mouse is inert while an overlay is up** (help, namespace picker, live filter): a
  stray click must not reach and mutate the panes beneath a modal.

**Why:** mouse must not fork the input model or bake keys/coordinates into rendering.
**Consequence:** a future leg extending mouse must keep it additive, route it through
Actions/public Select* entries (not new raw-mouse logic in a view), keep `MouseMode`
set in `View`, and gate it behind `overlayActive()`. A component adding a
click target exposes a coordinate→index accessor rather than handling mouse itself.

### D87 — The persistent key-hint is a dedicated bottom line of its own (the hintbar), not a status-bar segment

`2026-07-21` · board `FB-hintbar-dedicated` (deferred remainder of D85).

The focus-aware key hint (D85) used to be a right-aligned segment on the status bar,
sharing one line with the live state (context · namespace · filter · spinner). It now
lives on its own dedicated row **below** the status bar — the `hintbar` component
(`internal/tui/components/hintbar`) — so the hint and the state never compete for width.

- **The status bar no longer lays out a hint.** `statusbar` dropped `SetShortHelp` and
  its right-align/gap logic; it renders only its left segment (or a full-line error
  toast), clamped to width. A future leg must not put the hint back on the status bar.
- **The hintbar is fed the same registry-generated, focus-aware string** the status bar
  used to get: the root's `syncHints` now calls `m.hintbar.SetHint(help.ShortHelpContextView(ctx))`
  (D11/D85 intact — the component still matches no raw key). Call `syncHints` at every
  focus switch and on resize, exactly as before.
- **The hint is now always visible**: because it owns a line, it is never dropped under
  width pressure and never hidden behind an error toast — the two failures D85 called
  out. Layout reserves one extra bottom row: `bodyH = height - statusBarHeight -
  hintBarHeight` (mirrored in `bodyHeight()` for mouse mapping); the vertical stack is
  body · status · hint.
- The welcome landing page keeps its own in-pane focus-agnostic hint (`welcome.SetShortHelp`),
  unchanged.

**Why:** state and keys competing for one line meant the hint got truncated or dropped
just when the user needed it (narrow terminal, active error). **Consequence:** keep the
hint on the hintbar line; a leg adding another persistent bottom element must budget its
own row (adjust `bodyH`/`bodyHeight` together) rather than crowding the status or hint line.

### D88 — The confirm/prompt modal resolves accept/decline through nav.drillIn / nav.back (Enter/Esc), not dedicated y/n actions

M2-10 landed the confirm/prompt overlay (`internal/tui/components/modal`) that
replaces the original's racy tcell popup. It is a sibling of the picker (D65): a
Kind-stamped, centered, bordered box driven **only** through resolved
`keymap.Action`s, emitting its own `ConfirmedMsg`/`CancelledMsg` (never importing
the root, D56), holding no shared mutable state (principle 1).

- **Accept = `nav.drillIn` (Enter), decline = `nav.back` (Esc).** There are no
  `confirm.yes`/`confirm.no` actions and no raw `y`/`n` matching — the
  Enter/Esc-consistent modal convention of `knowledge/keybindings.md`, same as the
  picker. A future leg adding a confirm must route through these actions, **not**
  reintroduce raw-key `y`/`n` handling (D11). The board's "(y/n)" was the semantic
  (a yes/no question), not a keybinding mandate.
- **Two modes on one Model.** `ShowConfirm(kind,title,message)` → `ConfirmedMsg`
  with empty `Value`; `ShowPrompt(kind,title,message,initial)` → focuses an owned
  `textinput`, `ConfirmedMsg.Value` carries the entered text. `Prompting()` gates
  raw text to `UpdatePrompt` (the single raw-key entry point), mirroring the
  picker's `Filtering()`/`UpdateFilter`.
- **Component-only, like the picker was (M2-08a).** Nothing triggers a confirm yet
  (delete/scale/… are M3); the app-shell wiring lands **with the M3 action that
  needs it**, which decides the copy and (for destructive actions) whether Enter
  should default to accept. Do not wire an unused confirm into the shell before then.

**Consequence:** any M3 destructive/parameterised action gets its confirmation by
owning a `modal.Model` on the root, calling `ShowConfirm`/`ShowPrompt`, and handling
`ConfirmedMsg`/`CancelledMsg` — no new keymap actions, no raw y/n.

### D89 — `Config.Save`/`SaveFile` are the config write-back primitives; M2-11's menu-customization scope is subsumed by the per-context menu files (D83)

`2026-07-22` (M2-11a). The main config gained write-back to match Load: `Config.Save(io.Writer)`
marshals via `sigs.k8s.io/yaml` (round-trips `keys:` today; what Save emits, Load reads
back equal), and `Config.SaveFile(path)` persists it — `MkdirAll(dir, 0o700)`, marshal to a
temp file in the **same** dir, `Chmod 0o600`, then `os.Rename` over the target so a crash
mid-write never truncates the live config. It is user data, kept private like a kubeconfig.

- **M2-11 narrows.** M2-11 (added 2026-07-19) predates D83: its "customized resource
  list / order" belongs to the **per-context menu files** (`menus/<context>.yaml`, D83),
  which already load on start (FB-menu-config-03). A future leg must **not** duplicate a
  resource list into `config.yaml`. M2-11's genuine remainder is config write-back (this
  leg) + last-namespace persistence + load-on-start wiring (M2-11b).
- **Last namespace is per-context.** M2-11b decides where it lives (a per-context store,
  not a single global field), since a namespace is meaningless across clusters — record
  that choice when taking M2-11b.

**Consequence:** persistence legs (M2-11b) and the M2-12 legacy migration write through
`SaveFile`; don't hand-roll another YAML writer or a non-atomic overwrite.

### D90 — Per-context runtime state lives in its own `state/<context>.yaml` store, not in config.yaml or the menu file

`2026-07-22` (M2-11b-1). Last-namespace persistence (and future per-context runtime
values kubecom records for you) gets a **dedicated per-context state store**:
`config.State` in `internal/config/state.go`, persisted to
`os.UserConfigDir()/kubecom/state/<sanitized-context>.yaml` (`StateDir`/`StatePath`,
reusing `menuFileName`'s context sanitization so a name can never escape the dir).
`LoadState`/`LoadStateFile` (missing file → zero State, strict-unknown-field) and
`Save`/`SaveFile` (atomic 0o600 via the shared `atomicWriteFile` extracted from
`Config.SaveFile`) mirror the `Config`/`MenuConfig` API.

- **Not `config.yaml`.** The main config is not per-context; a namespace name is
  meaningless across clusters (D89 already flagged last-namespace as per-context).
- **Not the per-context menu file.** That file is **user-authored** (hand-edited CRD
  lists, comments); kubecom writing last-namespace into it on every namespace switch
  would reformat/clobber the user's file. Runtime state kubecom rewrites freely must
  be a separate file from config a human edits.
- **Config-package only.** This leg is the store primitive + tests; the load-on-start
  and persist-on-select wiring (a namespace-persister app seam + launcher glue, with
  explicit `-n` overriding stored state for that run) is M2-11b-2.

**Consequence:** any future per-context value kubecom persists on its own (not a
user setting) belongs in `State`/`state/<context>.yaml` through `SaveFile`; do not
add kubecom-written runtime fields to `config.yaml` or a menu file.

### D91 — Explicit `-n` overrides the stored last-namespace for that run; a namespace picked in the UI is persisted; the flag being *set* is what matters, not its value

`2026-07-22` (M2-11b-2). The initial watch scope is resolved from the `-n`/`--namespace`
flag and the per-context state (D90) with this precedence:

- **Explicit `-n` wins for the run.** `cmd.Flags().Changed("namespace")` (not the
  flag's value) decides — so `-n ""` explicitly forces all namespaces even when a
  concrete namespace is stored. An explicit `-n` is a per-run override: it does
  **not** overwrite the stored state on startup.
- **With no `-n`, the stored last namespace is restored** (empty when none saved).
- **A namespace picked in the UI is persisted** through a `tui.NamespacePersister`
  seam (`WithNamespacePersister`; the launcher wires a `statePersister` bound to the
  active context's `StatePath`). The write runs off the update loop (a `tea.Cmd`);
  a failure degrades to a transient error toast (principle 3), never blocks input,
  and the picked scope still applies for the session. A model built without the seam
  (or with an unresolved context) is persistence-inert.
- **A malformed/unreadable state file degrades to the zero State with a logged
  warning** — no toast, no fatal launch. Unlike a hand-authored menu file (whose
  corruption surfaces a toast, D83), the state file is kubecom-owned, so a corrupt
  one is rare and the next namespace switch overwrites it cleanly.

**Consequence:** future per-run overrides of a persisted setting follow this shape —
gate on the flag being *set* (`Changed`), keep the override transient (don't write it
back on startup), and persist only the user's in-UI change through the state store.

### D92 — Legacy `~/.kubecom.yaml` migration is detect-and-report, not a field-for-field port: its menu can't be auto-mapped (no version/resource) and its themes have no v1 home

`2026-07-22` (M2-12a). The 2020 `~/.kubecom.yaml` (protobuf-yaml `pb.Config`) held
only two user-authored things — a resource `menu` and color `themes`/`currentTheme`
— and **neither maps cleanly into kubecom v1**, so `config.Migrate` does **not**
attempt a faithful field-for-field port:

- **Menu entries can't be auto-migrated.** The old `menu` named a kind by
  `group`+`kind` only; the v1 per-context menu (`MenuResource`) addresses a resource
  by group/**version**/**resource**, which the old format never stored and only live
  discovery can resolve. Writing a `MenuConfig` from the legacy data would fail its
  own `validate()` (version+resource required). So legacy menu entries are
  **reported**, not written — the user re-adds them in a per-context menu file (D83).
- **Themes are dropped.** v1 uses a single fixed lipgloss theme (D6); there is no
  runtime theming to migrate into.
- **Keys never existed in the legacy file** — the one thing the new `Config` models —
  so migration produces the **zero `Config`**. Returning it (not nil) is intentional:
  the wiring writes it once to establish the new-format file so the one-shot runs once.

`Migrate` parses the legacy YAML **leniently** (non-strict — leftover theme
color/style detail is ignored, not rejected) and returns `(*Config, notes, error)`;
only unparseable YAML errors, so a legacy file never blocks start (principle 3). The
notes are surfaced to the user by the launcher wiring (M2-12b).

**Consequence:** the "migration" exit criterion is met by recognising the old file
and telling the user what to redo by hand — not by silently reconstructing a menu
that would be wrong. No future leg should claim the legacy menu/themes port
automatically, or write a `MenuConfig` with blank version/resource.

### D93 — Legacy migration is one-shot, gated on `config.yaml` **absence**; migration notes preempt the single startup-toast slot

`2026-07-22` (M2-12b). The launcher (`cmd/kubecom/run.go` `maybeMigrate`) runs the
D92 `config.Migrate` on first start only, and never blocks launch (principle 3):

- **One-shot is gated on the new config's absence, checked with `os.Stat`** — not
  `config.LoadFile`, which maps a missing file to the zero config and so hides the
  present/absent distinction. A present `config.yaml` (migrated earlier, or
  hand-authored) suppresses migration; kubecom never overwrites it. A `Stat` error
  other than not-exist also suppresses migration (don't risk clobbering).
- **Degrade paths write nothing and surface nothing:** an absent legacy
  `~/.kubecom.yaml`, a malformed/unreadable legacy file, or a failed write of the new
  config all leave no `config.yaml` behind (so a fixed file migrates on a later
  start) and only log. A successful migration writes the new config **once** (even an
  empty legacy file → `{}` config) so the one-shot is satisfied.
- **The shell has a single startup-toast slot** (`WithStartupError`, one
  `*tui.ErrorMsg`). When both a migration report and a per-context menu-config error
  exist, the **migration notes take the slot** (first-start is the more notable, rarer
  event); the menu error is still `slog.Warn`-logged, so it is never lost. A future
  leg adding another startup-time toast source must preserve this: log every source,
  and don't silently drop one because the slot is taken — widen the seam to carry
  multiple messages if genuine coincidence becomes common.

**Consequence:** migration is safe to re-attempt every launch (it self-suppresses
once a config exists) and never a launch blocker. Any leg that changes when the
launcher writes `config.yaml`, or adds a startup toast, must keep migration one-shot
and keep every degraded fault logged.

### D94 — Table column sort is a view over the authoritative row set (like filter): stable, type-aware only for integer/number, reset on `SetTable`, preserved across watch deltas

`2026-07-22` (M2-13a). The table's column sort is not a mutation of the delivered
data — it is a display transformation layered onto the same authoritative `full`
row set that the filter narrows, re-derived by `applyFilter` on every change so it
survives live updates. Load-bearing constraints for M2-13b and any later leg:

- **Sort follows filter in `applyFilter`.** `full → filter → sort → measure`. Only
  the visible (displayed) rows are ordered; `full.Rows` is never reordered (the
  unfiltered path now copies into the display slice instead of aliasing `full.Rows`,
  so a sort can't scramble the authoritative set). `SortBy`/`ClearSort` re-derive
  through `applyFilter`, so a watch delta (`ApplyEvent`) re-sorts in place and a new
  row lands in sorted position, not appended.
- **`sortCol` is a visible-column position (index into `visible`), or -1 for the
  unsorted watch order.** `New` starts at -1; a zero-value `Model{}` would read as
  "sort column 0", so tables must be built with `New`.
- **`SetTable` resets the sort** (a sort chosen for one resource's columns must not
  carry to a different resource, mirroring the filter reset); `ApplyEvent` preserves
  it (a reconnect RESET keeps the same resource). Selection is preserved by object
  UID across every re-sort.
- **Type-aware only where cheap: integer/number sort numerically, everything else
  as case-insensitive text.** Column types are the server's OpenAPI names. Date
  columns are deliberately text-sorted — kubectl prints ages ("5d", "2h") that don't
  parse as numbers or times cheaply; a wrong-but-fast numeric parse is worse than an
  honest lexical order. `sort.SliceStable` keeps equal-key rows in watch order in
  both directions.

**Consequence:** M2-13b wires the `sort.*` keymap action(s) to `SortBy(currentCol)`
/ `ClearSort` and renders the header indicator from `SortColumn`/`SortDescending`;
it must not re-implement ordering or sort `full.Rows`. Adding richer typing (real
date/quantity parsing) is a superseding decision, not a silent change here.

### D95 — Modals composite over the base browse view (a floating popup), never replace it; components return a bare box and the root overlays it

`2026-07-22` (FB-popups-overlay, feedback `2026-07-22-popups-should-overlay`). A
modal (help overlay, namespace picker, and — once wired — the M2-10 confirm modal)
is a **popup floating over the two-pane browse layout**, not a page that takes over
the body. The menu + table stay visible underneath. Load-bearing constraints:

- **Modal components return a bare bordered box from `View()`** — just
  `styles.PaneFocus.Render(body)`, no `lipgloss.Place` onto a blank area. A component
  no longer pads itself to fill the screen; a blank-filled string would occlude the
  base when composited. `View()` still returns `""` when hidden/unsized so the root
  can call it unconditionally.
- **The root model owns overlaying.** `Model.View()` always draws `browseBody()`
  first, then, if a modal is open, composites the box centered on top via
  `overlayCenter(base, box, width, bodyHeight)` (`internal/tui/overlay.go`). It uses
  the lipgloss/v2 layer stack (`NewLayer`/`NewCompositor`/`NewCanvas`): base at the
  origin (z0), box centered (z1), flattened onto a fixed `width×bodyH` canvas so the
  result is always exactly the body area. The box occludes only its own rectangle;
  every base cell outside it stays visible. `overlayCenter` returns the base
  unchanged for an empty box or a non-positive area (degrade, principle 3).
- **No dimming of the base yet** — the bordered box is visually distinct on its own;
  a dimmed backdrop is an optional future refinement, not required by this decision.

**Consequence:** any new modal must follow this shape — render a bare box and let
the root overlay it through `overlayCenter`; do not switch `body = modal.View()` to
replace the browse view, and do not re-add `lipgloss.Place` full-area padding inside
a modal component. Wiring the M2-10 confirm modal into the shell (M2-14b / M3) uses
the same `overlayCenter` path.

### D96 — Status bar sits at the **top**; the target navigation model is an optional/popup menu with a command-palette resource switch (left pane is not a permanent fixture)

`2026-07-22` (FB-status-bar-top, feedback
`2026-07-22-status-bar-top-and-optional-left-panel`). Two load-bearing constraints
this decision locks, plus a direction the follow-on legs implement:

- **The status bar renders at the top row of the screen**, not the bottom. The root
  `View()` stacks status (top) · two-pane body · hint line (bottom). Any layout work
  that touches vertical stacking must keep the status bar on top and must keep mouse
  Y-mapping offset by `statusBarHeight` (the body starts one row down —
  `handleMouseClick` subtracts it before resolving a row; a future top-anchored
  element shifts that offset again).
- **The status bar names the browsed resource type** (`kube.Resource.GVK.Kind`, e.g.
  `Pod`) alongside context · namespace, set from `selectResource`
  (`statusbar.SetResourceType`). The bar must always say what the table is listing;
  a leg that changes what resource the table shows keeps this current.
- **Direction (not yet built, queued as FB-nav-* board tasks): the left menu is not a
  permanent fixed pane.** The target navigation model is a **toggleable / popup**
  menu (a keybind shows/hides it; later it becomes an `overlayCenter` popup per D95)
  plus a **command-palette resource switch** (`<resources hotkey>` → `/` filter →
  Enter switches the table to that kind, k9s-`:`-style) so the app can be browsed
  pane-free with just the top status bar + table. A future leg must not treat the
  always-visible left pane as load-bearing; it is on a path to becoming optional.
  All of it stays on the keymap registry (no hard-coded keys, D11) and the
  zero-shared-mutable-state model (principle 1).

**Consequence:** the top status bar + resource-type display is delivered by this
leg. The optional-menu toggle, the popup menu, and the command-palette switch are
FB-nav-menu-toggle / FB-nav-menu-popup / FB-nav-resource-palette on the board.
### D97 — Mouse capture is off by default (native select-to-copy); mouse is opt-in via the `mouse.toggle` keybind. Supersedes D86's unconditional capture

`2026-07-22` (feedback `2026-07-22-text-selection-select-to-copy`). D86 enabled
mouse reporting unconditionally in `View` (`v.MouseMode = tea.MouseModeCellMotion`
on every frame). Any mouse-reporting mode makes the terminal send events to the app
instead of doing its own click-drag selection, so it broke **select-to-copy** — the
human couldn't select names/values/log lines to copy them. This decision flips the
default and makes mouse capture opt-in:

- **Off by default.** `Model.mouseEnabled` starts false; `View` sets
  `MouseModeCellMotion` **only** when it is true, otherwise leaves `MouseModeNone`.
  So the terminal keeps its native select-to-copy everywhere out of the box. This is
  the aligned default per goals principle 6 (vim-first, never vim-only) and principle
  8 (dogfoodable): the keyboard path is complete, so losing default mouse-wheel scroll
  costs nothing a key doesn't already do.
- **Opt-in via a runtime toggle keybind**, not a config flag: `mouse.toggle`
  (registry action, default `M`, rebindable — D11) flips `mouseEnabled` at runtime.
  Chosen over a config-file-only switch so it needs no restart and no cmd/config
  wiring, and over "keep capture + document Shift+drag" because that isn't a true
  default. Switching back to `MouseModeNone` tears reporting down again (bubbletea v2
  toggles the reporting sequences from the View's `MouseMode` each frame), so native
  selection is restored the moment capture is turned off.
- **The state is visible.** Mouse capture is otherwise an invisible mode, so the
  status bar shows a persistent `mouse` marker (`statusbar.SetMouse`) while it is on —
  not a transient toast, because the user needs to know the current state at any time.
- **The D86 mouse handlers are unchanged.** `handleMouseClick` / `handleMouseWheel`
  and their component coordinate→row accessors stay exactly as they were; they simply
  receive no events until capture is toggled on. The `mouse.toggle` action is handled
  in the app-mode branch of `handleAction` (alongside quit/help), so it works even
  while an overlay is open — it is a mode toggle, not navigation.

**Consequence:** a future leg must keep mouse capture off by default and gated on
`mouseEnabled`; it must not reintroduce unconditional `MouseModeCellMotion` in `View`,
and any new mouse affordance stays inert until the user opts in. In-app copy (e.g.
OSC 52 yank of the selected row/field) was noted as a possible complement and is not
built here.

### D98 — Column sort is driven by one cycling key over a stateless derivation of the table's own sort state; there is no separate column-selection gesture

`2026-07-22` (M2-13b). M2-13a delivered `SortBy(visibleCol)`/`ClearSort` as a view
over the row set (D94) but no way to reach it. The table has no column cursor, so
this leg's wiring had to decide **which column** the sort acts on. Locked choices a
later leg must not silently contradict:

- **One key cycles everything.** `sort.column` (registry action, default `s`,
  rebindable — D11) advances a single cycle: unsorted → column 0 ascending → column 0
  descending → column 1 ascending → … → last column descending → **cleared**
  (`ClearSort`) → column 0 ascending. This makes every visible column and both
  directions reachable **without** adding a column-selection cursor/navigation
  gesture. `sort.clear` (default `S`) drops any sort in one press.
- **The cycle is stateless.** It is derived entirely from the table's own
  `SortColumn()`/`SortDescending()` each press (plus `VisibleColumnCount()` for the
  wrap point); the root model stores **no** sort cursor. This keeps principle 1 (no
  shared mutable UI state) and means a `SetTable` reset (which clears the sort, D94)
  automatically restarts the cycle — no separate reset to keep in sync.
- **Rejected: sort the leftmost-visible column** (via the horizontal scroll offset).
  A table that fits without scrolling has offset 0 always, so only the first column
  would ever be sortable, and the last columns can never become leftmost even when
  scrolled — most useful sorts would be unreachable. The cycle avoids both.
- **Header indicator lives in the component.** The sorted column's header carries a
  direction arrow (`▲` ascending / `▼` descending); its width is reserved in
  `measureWidths` so the arrow never overflows the column and misaligns the data rows
  below it. `sort.column` is also added to the table-context short-help hint so the
  key is discoverable.

**Consequence:** a future leg may add a real column cursor / a k9s-style
sort-by-named-column palette, but that supersedes this decision rather than silently
changing the `s` cycle. Richer type-aware ordering is still D94's concern, not this
one.

### D99 — The left menu pane is toggleable (`menu.toggle`); a hidden menu is zero-width and cannot hold focus. First implemented slice of D96

`2026-07-22` (FB-nav-menu-toggle). D96 recorded that the left pane is not a permanent
fixture; this leg makes it hideable and locks the toggle contract the remaining D96
slices (FB-nav-resource-palette, FB-nav-menu-popup) must preserve or supersede:

- **`menu.toggle` (registry action, default `m`, rebindable — D11)** hides/shows the
  left resource-menu pane at runtime. It is handled in the app-global branch of
  `handleAction` (alongside `mouse.toggle`), so it works regardless of which pane is
  focused. `menuHidden` starts false (the menu shows), is touched only from the update
  loop (no shared mutable state, principle 1), and is not persisted (session-only for
  now).
- **A hidden menu is zero-width everywhere.** `resize()` gives the table the full
  width, `browseBody()` renders the table alone (no `JoinHorizontal` with the menu),
  and `inMenu()` returns false so mouse routing sends every click/wheel to the table.
  Any future layout, focus, or mouse-mapping leg must treat `menuHidden` as a
  zero-width menu, not assume an always-present left pane.
- **Focus follows visibility.** Hiding moves focus to the table (a hidden pane can't
  hold focus); showing returns focus to the menu (the gesture to pick a resource). The
  same key re-shows the menu, so it is never a one-way door even before a pane-free
  resource switch exists.
- **Known gap (by design, this slice):** with the menu hidden there is no pane-free
  way to change the browsed resource yet — the user re-shows the menu to switch.
  FB-nav-resource-palette (the command-palette `:`-style switch) closes that gap;
  FB-nav-menu-popup may then fold the toggled menu into an `overlayCenter` popup (D95),
  which would supersede the fixed-pane half of this decision while keeping the toggle
  action and focus contract.

### D100 — Resource command palette (`resources.switch`) reuses the generic picker keyed by a distinct Kind; selecting drives `selectResource`. Second slice of D96

`2026-07-22` (FB-nav-resource-palette). D96 named the pane-free, k9s-`:`-style
resource switch as the counterpart to the toggleable menu (D99); this leg builds it
and locks how it is wired, so FB-nav-menu-popup (which may fold the menu into this
palette) and any future picker preserve or supersede the contract:

- **One generic picker component, two instances, disambiguated by `Kind`.** The root
  now holds a second `picker.Model` (`resPicker`, `picker.New(s, "resource")`)
  alongside the namespace `nsPicker`. Both emit the same `picker.SelectedMsg`/
  `CancelledMsg` (D65), so the root branches on `msg.Kind == resourcePickerKind`
  (`"resource"`) to route a palette result to `handleResourceSelected` rather than the
  namespace path. An empty `Kind` routes to the namespace picker (the hermetic tests
  deliver bare `SelectedMsg{Value:…}`). A future picker must stamp its own distinct
  Kind and add a branch, not overload an existing one.
- **`resources.switch` (registry action, default `:`, rebindable — D11)** opens the
  palette. Handled in the post-help branch of `handleAction` (beside `ns.switch`), so
  it fires whichever pane is focused and while the menu is hidden — the pane-free
  switch D99 flagged as its missing piece. Watch-inert (no `WithWatcher`) → the
  palette does not open (nothing to switch).
- **The source list is the menu's own item set.** `openResourcePicker` snapshots
  `menu.Items()` filtered to available `ItemResource` rows (the namespace seam and
  unavailable rows are skipped, mirroring what a menu drill-in can act on), so
  discovered CRDs and per-context extras (D83) are included for free. The picker is
  generic over strings, so a rebuilt-on-open `resByLabel map[string]kube.Resource`
  resolves the picked title back to its resource; a title collision keeps the first.
- **Selecting drives the same `selectResource` path a menu drill-in takes** (start the
  watch, `menu.SetActive`, focus the table), so a palette switch and a menu drill-in
  are one behaviour. The palette does not move the menu cursor; the active-row marker
  (▸, dogfood-05) shows which kind is open.
- **Routing generalized to "the active picker."** `activePicker()` returns whichever
  of the two is open (at most one ever is); `Update`'s key dispatch, `routePickerKey`,
  `overlayActive`, and `View`'s overlay compositing (D95) all go through it instead of
  naming `nsPicker`. Both pickers are sized in `resize()`.

**Consequence:** FB-nav-menu-popup may promote the toggled menu into this palette (or
an `overlayCenter` menu popup) — that supersedes D99's fixed-pane half while keeping
this Kind-routing + `selectResource` contract. A real column/kind cursor or richer
palette scoring is out of scope here.

### D101 — FB-nav-menu-popup (menu-as-overlay-popup) folded into the resource palette; retired won't-do-separately. Third slice of D96, resolving the reassess

**2026-07-22 (FB-nav-menu-popup).** D96 triaged the "left pane is not a permanent
fixture" direction into three slices and flagged the third — floating the whole
sectioned menu as an `overlayCenter` popup — as one to **reassess once the toggle
(D99) and palette (D100) landed** (D100's own Consequence: "may promote the toggled
menu into this palette *or* an `overlayCenter` menu popup"). Both have landed; this
is that reassessment, and the outcome is **fold, not build a third surface.** The
D96 want is delivered:

- **Pane-free resource switching is already the palette.** `resources.switch` (`:`,
  D100) summons a modal, `/`-filterable list of every browsable kind over the base
  view and drives `selectResource` — a *better* summoned-overlay navigator than a
  floated menu (it filters; the sectioned menu does not), and it already works with
  the menu hidden.
- **"Default view = table only" is already the toggle.** `menu.toggle` (`m`, D99)
  hands the full width to the table on demand; the menu re-shows with the same key.

A second overlay that floats the entire sectioned menu (sections + namespace seam +
active marker) would **duplicate the palette's job** with no recorded UX benefit and
would itself need a fresh UX decision — the D81 (M1-04b) situation exactly.

**Constraint:** do not build a separate floating-menu overlay on the strength of this
old slice alone; the palette (D100) is kubecom's pane-free navigator, the left menu
its always-available toggleable pane (D99). **Rejected** flipping the *default* to
menu-hidden: a first launch would then show only a welcome/empty table with no
visible navigator, hurting discoverability of a tool the human dogfoods (principle
6/8) — the menu stays shown by default. Reopening this needs a new decision that
supersedes D99/D100, not a revival of FB-nav-menu-popup. **Consequence:** the M2
FB-nav-* line (D96) is complete; no menu-popup work remains.

### D102 — Board Done entries are one line too (extends D67)
**2026-07-22.** D67 kept the milestone `Status:` and the board `Last updated:`
lines terse, but did not name the **Done list**, so legs drifted back to writing a
full journal-length paragraph per completed item — the board reswelled from ~17KB
to ~50KB (35 of 93 Done entries over 400 chars), and it is read on every Orient.
Rule: a **Done entry is one line** — `- [x] **ID** <short title> — done
YYYY-MM-DD (Dnn, …)` — no prose, no continuation lines; the full detail is the
journal entry for that leg. Same for backlog `notes:` — keep them short. Re-collapsed
the Done section on this leg (93 items preserved, ~50KB → ~17KB). Wired into
CLAUDE.md step 7 and the `do-rewrite-leg` skill.

### D103 — List/watch params encode with metav1.ParameterCodec, not scheme.ParameterCodec
**2026-07-22.** `tableRequest` (shared by List and Watch, `internal/kube/table.go`)
must encode `VersionedParams` with **`metav1.ParameterCodec`**
(`k8s.io/apimachinery/pkg/apis/meta/v1`), **never** `scheme.ParameterCodec`
(`client-go/kubernetes/scheme`). The built-in clientset scheme only knows built-in
GroupVersions, so it cannot convert `metav1.ListOptions`/watch params to an
arbitrary CRD GroupVersion (`gateway.networking.k8s.io/v1`, `traefik.io/v1alpha1`,
…) and fails every non-built-in CRD group with "v1.ListOptions is not suitable for
converting to …" — directly defeating the discovery-driven "generic over any
resource incl. CRDs" goal (#76/#87). `metav1.ParameterCodec` converts params for
any GroupVersion (it is what `client-go/dynamic` uses) and works for built-ins too,
so it is a strict improvement. Hermetic tests must exercise a **non-built-in** GVR
(the built-in-only fixtures are why this slipped past `make check`). A future leg
must not switch this back.

### D104 — Watch degrades to list-only polling for kinds that can't be watched
**2026-07-22.** `watchLoop` (`internal/kube/watch.go`) must not blank the view or
retry-loop when a kind lacks the `watch` verb (e.g. `componentstatuses`, some
aggregated/legacy resources). Two guards, both required (principle 3 — degrade,
don't blank):
1. **Verb-driven:** if `r.Verbs` is **known and lacks `watch`** it never opens a
   stream — it re-Lists on `listPollInterval` (10s), emitting a fresh RESET each
   cycle. An **empty** verb set is *unknown* (the seed menu carries no verbs
   pre-discovery), so it is **not** treated as list-only — it still tries to watch
   so cold-start browsing of core kinds stays live.
2. **Server-driven backstop:** if the watch request itself returns **405
   MethodNotAllowed** (`apierrors.IsMethodNotSupported`), the loop flips to that
   same list-only polling mode instead of paced-retrying the doomed watch or
   parading the ERROR — this covers incomplete discovery verbs (guard 1's empty
   case) at runtime. A future leg must keep both guards; don't reintroduce the
   unconditional watch that blanked list-only kinds.

### D105 — M3 (actions & viewers) decomposed into ordered, leg-sized Backlog slices
**2026-07-22.** With M2's board section down to only blocked/deferred items
(M2-14b is M3-gated per D88; M1-INT is deferred envtest), the next milestone M3 was
still a single prose paragraph — no pickable leg. This planning leg turns the M3
scope + exit criteria into a dependency-ordered task list **M3-01 … M3-15** (the
D52/M2-PLAN precedent: "expanding a thin milestone section into concrete tasks is a
leg in itself"). **Key framing — M3 is almost all TUI surface:** the kube layer
already implements every verb (logs stream M1-07c/d, describe M1-07b, YAML M1-07a,
delete/scale/rollout-restart/cordon/drain/suspend M1-06*, background port-forward
M1-08), so M3 wires those into viewers, the confirm modal, an actions surface, and
the two suspend flows — it does **not** re-implement action logic. **The slicing
(built bottom-up):**
- **M3-01** reusable read-only viewer/pager component (the shared substrate) →
  **M3-02** action surface (actions menu reusing the picker, D100) + M3 keymap off
  the reserved nav keys (D10) — these two land first because every viewer/action
  needs a reachable trigger and a place to render.
- Viewers on top of M3-01/02: **M3-03** YAML → **M3-04** describe → **M3-05**/**06**/
  **07** logs (initial / follow+reconnect / container-picker+pod-owning-kinds #84) →
  **M3-08** secret viewer (#89).
- Actions through the M2-10 confirm modal (D88 — accept=`nav.drillIn`, decline=
  `nav.back`, no raw y/n): **M3-09** delete (first confirm wiring; **unblocks M2-14b**)
  → **M3-10** scale + rollout-restart → **M3-11** cordon/drain → **M3-12** cronjob
  suspend/resume (#83).
- **M3-13** port-forward manager panel (M1-08 background forward) · **M3-14** exec
  shell (`tea.ExecProcess` + remotecommand) · **M3-15** `$EDITOR` edit — the two
  suspend flows are the only sanctioned TUI-suspending actions (goals).
**Constraints every M3 slice inherits (a future leg must not contradict):** overlays
composite over the base browse view (D95), never replace it; zero shared mutable UI
state (principle 1) — background streams/forwards only send msgs via the M2-02 pumps
(D53); no raw-key matching — every action is a named keymap entry off the nav keys
(D10/D11). Ordering is a default, not a contract — re-split any slice that proves
> ~300 lines (the skill's split-and-take rule still applies per leg). Board-only;
no code, `make check` green.

### D106 — All M3 read-only viewers share one `viewer.Model` overlay
**2026-07-22 (M3-01).** The YAML/describe/logs/secret viewers (M3-03…08) each render
into the **one** `internal/tui/components/viewer` component, not their own pager. Its
contract (mirrors the picker, D95/D56/D11): constructed with a `Kind` string; fed
text via `SetContent` (which resets scroll to the top) and sized via `SetSize`;
driven **only** through resolved `keymap.Action`s (`Update(a keymap.Action)`) —
nav.up/down + nav.top/bottom (gg/G) + half/full page scroll the wrapped
`bubbles/viewport`, nav.back emits `ClosedMsg{Kind}`; it never receives a raw
`tea.KeyMsg`, so the viewport's own key bindings are inert and no hard-coded key
leaks in (D11). `View()` returns a **bare** bordered box (title bar + viewport) that
the root composites via `overlayCenter` (D95) — it never replaces the base browse
view. Mouse-wheel scroll on the viewport is off (input flows through actions).
`AtBottom()` is exposed for the follow-logs slice (M3-06) to decide auto-scroll. A
future viewer leg **feeds this component**; it must not re-implement scroll/framing
or match raw keys. In-viewer `/` search (`n`/`N`) is a deliberately deferred
follow-up slice, not part of M3-01.

### D107 — M3 row actions dispatch a typed `rowActionMsg` intent; applicability is a kind-keyed registry
**2026-07-23 (M3-02).** The M3 action surface is split from the individual
viewers/actions: this leg lands only **opening the actions menu and routing**, no
action behaviour. The curated action set lives in one registry
(`internal/tui/rowaction.go`, `rowActions`): each entry is a `rowAction` id, a menu
title, an optional bound `keymap.Action` (the direct-key shortcut), and a
kind/verb-keyed applicability predicate. The **actions menu** (`actPicker`, a
`picker.Model` of Kind `"action"`, D65/D100) lists exactly the applicable titles for
the browsed kind over the selected row; the direct keys (`res.describe` `d`,
`res.yaml` `y`, `res.logs` `L`, `res.edit` `e`, `res.delete` `x`, and
`actions.menu` `a`) are the only M3 keys — all off the reserved nav set (D10), the
rest of the set is menu-only. Both entry points funnel through **one typed intent**,
`rowActionMsg{Action, Resource, Object}`, dispatched as a `tea.Cmd`. A future M3 leg
(M3-03…) handles its intent by adding a case to (or replacing) `handleRowAction`,
which for now surfaces a transient "not yet available" toast so routing is
observable (D68). Constraints a later leg must not silently break: keys stay in the
keymap (no raw-key match, D11); a new action is a `rowActions` row (+ its handler),
not a bespoke picker or key path; applicability by kind lives in the registry, not
scattered in the shell.

### D108 — M3 viewer legs wire through a narrow kube getter seam, fetch async with a generation guard, and capture input while open
**2026-07-23 (M3-03).** The first viewer (YAML) establishes the pattern every later
read-only viewer leg (describe M3-04, logs M3-05…, secret M3-08) follows so they do
not each invent their own wiring: (1) the kube call is reached through a **narrow
single-method seam** on the shell (`YAMLGetter`, wired with `WithYAMLGetter`;
`*kube.Clients` satisfies it), mirroring `ResourceWatcher`/`Discoverer`/`NamespaceLister`
— the tui package never constructs a client and stays hermetically testable; a model
built without the seam is **viewer-inert** (the action is a no-op, the viewer never
opens). (2) `handleRowAction` branches on the `rowAction` (D107) to an `openXViewer`
that **shows the shared `viewer.Model` immediately (empty) and issues the fetch off
the update loop** as a `tea.Cmd`, seeding content when a typed `xLoadedMsg` lands —
the gesture feels instant and `Update` never blocks. (3) Every open bumps a
**`viewerGen`** carried on the load message; `handleXLoaded` drops a result whose gen
no longer matches or that arrives after the viewer closed (the watchGen/seqGen
stale-message guard). (4) A fetch **error degrades**: close the viewer + a transient
status-bar toast (D74), never an empty box. (5) While the viewer is active the root
**captures input** — `handleAction` routes to `handleViewerAction`, which scrolls on
nav and closes on nav.back (via the viewer's `ClosedMsg`) and app.quit, swallowing
everything else — and the viewer composites over the base browse view via
`overlayCenter` (D95); `overlayActive()` includes it so mouse events stay inert.
Constraints a later viewer leg must not silently break: reach kube through a seam
(no client in tui), keep the fetch async + gen-guarded, degrade on error, and capture
input the same way rather than adding a bespoke key path.

### D109 — Streaming viewer legs pump a kube channel line-by-line into the shared viewer via a gen-tagged pump, cancel on close/supersede, and append preserving scroll
**2026-07-23 (M3-05).** The logs viewer is the first *streaming* viewer, so it
extends D108's one-shot-fetch shape with the channel→msg pump rhythm (M2-02/D53) that
M3-06 (follow) and M3-07 (container picker / pod-owning kinds) build on. The
constraints a later streaming-viewer leg must not silently break: (1) the stream is
reached through a **channel-returning seam** on the shell (`LogStreamer.Logs`, wired
with `WithLogStreamer`; `*kube.Clients` satisfies it) — as with the D108 seams the tui
package constructs no client and a model without it is viewer-inert. (2) The channel is
pumped **one item per `tea.Cmd`** (`logPump` in `msg.go`, mirroring `watchPump`): a
`LogLineMsg` appends and re-issues the pump, a `LogClosedMsg` (EOF of a non-following
stream) ends the chain, a stream error is bridged to a classified `ErrorMsg`. `Update`
never blocks on more than one receive. (3) Each pumped item rides the **shared
`viewerGen`** wrapped in a `logMsg{gen,msg}` (the watchMsg pattern), so a line from a
superseded viewer — closed, or replaced by a newer viewer of *any* kind — is dropped
and its chain stopped. (4) The stream runs on a **cancellable context torn down by
`stopLogStream`** — called before starting a new stream, when the viewer closes, when a
one-shot (YAML/describe) viewer supersedes it, and on quit — the log twin of the watch's
cancel-on-reselect. (5) Lines append via **`viewer.AppendContent`, which preserves the
scroll position** (no auto-scroll — a reader scrolled partway stays put); follow-mode
auto-scroll (via `viewer.AtBottom`) is M3-06's concern. (6) Error handling refines
D108 for a stream: an **open failure closes the empty box** (nothing shown yet) + a
toast, but a **mid-stream error after lines already showed keeps them on screen**
(`viewer.Empty` gates this) — partial output is not discarded. Scope: **pods first** —
the pod-owning kinds the actions menu lists for logs degrade to a "not yet available"
toast until M3-07 resolves their backing pod; `LogOptions{}` (whole log, default
container, no follow) is the initial cut.

### D110 — The logs viewer opens in follow mode (streaming + auto-scroll); `logs.follow` (`f`) toggles it and a manual up-scroll pauses it
**2026-07-23 (M3-06).** Building on D109, the logs viewer opens **following**: the
stream is opened with `kube.LogOptions{Follow:true}` (M1-07d — the stream stays open
and reconnects transparently across transport drops rather than ending at EOF), and
while following each appended line snaps the viewport to the bottom (`viewer.GotoBottom`)
so the newest output is always shown — the `kubectl logs -f` / k9s default. Constraints a
later leg (M3-07 container picker / pod-owning kinds) must not silently break: (1) follow
is a **shell-owned bool** (`m.logFollow`), never shared mutable state, consulted only
while the logs viewer is up (`viewer.Kind()==viewerKindLogs`); the shared viewer is
restamped per open (`viewer.SetKind`) so YAML/describe/logs are distinguishable for
kind-specific gating. (2) `logs.follow` (default `f`, off the reserved nav keys) toggles
follow **only inside the logs viewer** — inert on the YAML/describe viewers and inert in
the browse view; re-enabling snaps to the bottom. (3) A **manual up-scroll while
following pauses follow** (nav.up/top/halfPageUp/pageUp) so scrollback isn't yanked back
to the tail; the toggle (or a fresh open) resumes it. (4) The viewer title carries a
`[following]`/`[paused]` marker so the mode is always visible (D68). This is the first
viewer with a mode of its own; the same follow bool + kind-gated toggle is the shape
M3-07 extends when it adds a container picker to the logs viewer.

### D111 — Opening logs on a multi-container pod resolves the pod's containers first and prompts which to stream; a single-container pod streams directly
**2026-07-23 (M3-07a).** `kubectl logs` requires `-c` to disambiguate a
multi-container pod (the API server errors on an empty container name when a pod has
more than one), so the logs viewer can no longer stream blindly. Constraints a later
leg (M3-07b pod-owning kinds, exec/edit) must not silently break: (1) A **`ContainerLister`
seam** (`PodContainers(ctx, ref) ([]string, error)`, `*kube.Clients` satisfies it via a
pod Get returning `spec.containers` names in spec order) resolves a pod's containers.
Only regular containers are offered — the set kubectl's default-container logic counts;
init/ephemeral container logs are a deliberate later refinement. (2) The flow is
**resolve-then-stream**: opening logs on a pod issues the fetch off the update loop
(tagged with a fresh `viewerGen` so any newer viewer open staleifies it — the shared
generation guard, D108/D109), then a **single** container streams directly (reusing that
gen) while **multiple** open the reused modal picker (`containerPickerKind`), the pick
streaming the chosen container. The pod the pick applies to is stashed
(`logStreamRes`/`logStreamRef`) because the picker's `SelectedMsg` carries only the
chosen string (D65). (3) With **no lister wired the shell streams the pod's default/sole
container directly** (empty `LogOptions.Container`, the M3-05/06 behaviour) — the picker
is simply not offered, keeping the pre-wiring app and non-picker hermetic tests inert
without the extra seam. (4) The streaming half is factored into `streamLogsInto(res, ref,
container, gen)` (shared by the no-lister path, the single-container path, and the
picker-select path); a non-empty container is named in the viewer title (`Logs ns/pod ·
container`). M3-07b resolves a backing pod *before* this container resolution, so the
pod-owning-kinds slice feeds a resolved pod ref into the same `openLogsViewer` pod path.

### D112 — Logs on a pod-owning workload kind resolve a backing pod (selector → newest ready pod), then take the pod path
**2026-07-23 (M3-07b, #84).** Logs are offered for pod-owning kinds
(Deployment/ReplicaSet/StatefulSet/DaemonSet/Job/ReplicationController), not only pods.
Constraints a later leg must not silently break: (1) A **`PodResolver` seam**
(`PodForOwner(ctx, res, ref) (ObjectRef, error)`, `*kube.Clients` satisfies it) resolves
a workload to one backing pod: it Gets the workload through the **dynamic client by GVR**
(no per-kind typed client — all six kinds, and a CRD with a pod selector, are covered
uniformly), reads `spec.selector` (a `metav1.LabelSelector` for every kind but
ReplicationController, whose selector is a **plain label map** — both shapes handled),
lists the matching pods, and returns the **newest Ready pod** (falling back to the newest
pod overall when none is Ready, so a mid-rollout / crash-looping workload still yields a
log target). A missing selector or no matching pods is a wrapped error, never a panic or
a match-everything list (principle 3). (2) The shell flow is **resolve-then-reuse**:
`openLogsViewer` sends a pod straight down the container path, but a pod-owning kind first
issues `PodForOwner` off the update loop (fresh `viewerGen` guard, D108/D109/D111), and
`handlePodResolved` feeds the resolved pod into `resolveContainersFor` — the shared tail
extracted from the pod path — so **D111's container resolution/picker applies to the
resolved pod** unchanged. The resolved pod's logs are titled as a **Pod** (`podLogResource`
carries only the Pod kind) so the user sees which pod is tailing, not the workload.
(3) **`WithPodResolver` gates it**: without a resolver wired a non-pod kind still degrades
to a "not yet available" toast (the M3-05…07a behaviour), keeping the pre-wiring app and
non-resolver hermetic tests inert. Init/ephemeral-container and a pod *picker* across a
workload's pods remain deliberate later refinements.

### D113 — The secret viewer opens masked; values are revealed only by the deliberate `secret.reveal` (`r`) gesture; decoding uses the typed clientset
**2026-07-23 (M3-08a, #89).** The Secret viewer is the first read-only viewer that
transforms content (decode + mask) rather than showing it verbatim. Constraints a later
leg must not silently break: (1) A **`SecretGetter` seam** (`SecretData(ctx, ref)
(kube.SecretData, error)`, `*kube.Clients` satisfies it) fetches through the **typed
clientset** (`CoreV1().Secrets`), not the dynamic client the other viewers use — the typed
client decodes the wire base64 into raw bytes (`Secret.Data map[string][]byte`) for us, so
no manual base64 decode is threaded and a malformed value can't slip through undecoded. It
returns `SecretData{Type, Entries []SecretEntry{Key,Value}}` with **entries sorted by key**
for a deterministic render; empty name is rejected; NotFound/RBAC errors are wrapped, never
panicked (principle 3). (2) **Values start masked and reveal is deliberate** (never
automatic): `openSecretViewer` sets `secretRevealed=false` on every open, and
`renderSecret` shows each value as a fixed mask + byte length (`key: •••••••• (N bytes)`)
until revealed — the length is shown, not the content, so nothing leaks pre-reveal. (3) The
reveal is a **registered keymap action** `secret.reveal` (`r`, D11 — no raw-key matching),
handled in `handleViewerAction` gated on `viewer.Kind()==viewerKindSecret` (inert on the
other viewers and when no viewer is up, exactly like `logs.follow`/`f` for M3-06); toggling
it re-renders the **same fetched data** (no re-fetch) via `SetContent`. (4) `WithSecretGetter`
gates it: without a getter wired the Reveal-secret action is inert (the viewer never opens).
**Reveal masks/unmasks all entries at once**; per-entry selection and **copy-to-clipboard
(M3-08b)** are the deferred follow-up that ticks the M3 secret exit criterion — this leg is
reveal only. A binary value renders as-is (read-only text); the copy slice can special-case
it.

### D114 — The secret viewer has an entry cursor (nav.up/down select, not scroll); `secret.copy` (`c`) yanks the selected value to the clipboard via bubbletea's OSC-52, masked or revealed
**2026-07-23 (M3-08b, #89).** Copy completes D113's Secret viewer and ticks the M3
secret exit criterion. Constraints a later leg must not silently break: (1) The viewer
carries a **per-entry cursor** (`secretSel`, an index into the key-sorted
`secretData.Entries`) reset to 0 on every open/load; `renderSecret(data, revealed, sel)`
marks the selected entry with a 2-cell cursor gutter (`> ` vs `  `, equal-width so keys
stay column-aligned) and returns each entry's 0-based output line so the selection can be
kept on screen. (2) **While the secret viewer is up, `nav.up`/`nav.down` move the entry
cursor rather than line-scrolling the viewport** — a Secret's body is small, so walking
entries is the useful gesture; the selection is pulled back on screen with the viewer's new
`EnsureLineVisible` (half/full-page keys still scroll for a large revealed value). This is
viewer-kind-gated (`viewer.Kind()==viewerKindSecret`), so the YAML/describe/logs viewers
keep their j/k line-scroll unchanged. (3) Copy is a **registered keymap action**
`secret.copy` (`c`, D11 — no raw-key match), handled in `handleViewerAction` gated on the
secret viewer; it writes the selected entry's **decoded value** to the system clipboard via
**`tea.SetClipboard` (bubbletea v2's built-in OSC-52 command)** — no external clipboard
dependency, works over SSH — and is inert on the other viewers and when the Secret has no
entries. (4) **Copy works masked or revealed**: putting a value on the clipboard is itself
the deliberate gesture, so it need not be revealed on screen first; the confirmation echoes
only the key + byte length (`copied "key" (N bytes)`), never the value. (5) The
confirmation is a **neutral status-bar notice** — a new transient channel on the status bar
(`SetNotice`/`ClearNotice`, Accent-styled, auto-cleared on its own `statusNoticeGen` timer,
the non-error twin of `SetError`; an error outranks a notice in `View`) so a success reads
as success, not as the red error toast. A future leg must keep copy off the raw-key path,
keep the value off-screen/out of the confirmation, and not reintroduce an error-styled
success.

### D115 — Delete wired through the confirm modal: root owns one `modal.Model` captured in `handleAction`, `ConfirmedMsg` routes by Kind, result to the status bar, no raw y/n
**2026-07-23 (M3-09).** First confirm wiring — D88's "the M3 action that needs it
wires the modal into the shell" — and the pattern the remaining mutating actions
(M3-10 scale/rollout, M3-11 cordon/drain, M3-12 suspend/resume) must follow.
Constraints a later leg must not silently break: (1) The root owns exactly **one**
`modal.Model` (`modal.New(s)`, sized in `resize()`); each action `ShowConfirm`s it
with its own **Kind** so `modal.ConfirmedMsg`/`CancelledMsg` route back by Kind
(`deleteModalKind` = `"delete"`). (2) The modal captures input in **`handleAction`**
(via `handleModalAction`), **before** the viewer check, so it is the topmost input
surface: `nav.drillIn` accepts, `nav.back`/`app.quit` decline, everything else is
swallowed — it consumes **actions, never raw keys** (D11), so accept/decline are the
Enter/Esc-consistent gestures, **no raw y/n**. (3) The confirm carries **no payload**
in confirm mode, so the action's target (resource + the row's `ObjectRef`, whose UID
guards the snapshot race — M1-06a/D35) is **stashed on the model** (`deleteRes`/
`deleteRef`) between the modal opening and the accept landing, keyed live off the
modal's Kind. (4) The mutating kube call runs **off the update loop** on accept and
reports its outcome via a done-message to the status bar — a failure (NotFound/RBAC/
UID Conflict) as a transient **error toast** (D74), a success as a neutral **notice**
(D114's `surfaceNotice`); the layout never breaks. (5) The affected row **leaves the
table via the live watch stream** (the delete triggers a DELETED event `ApplyEvent`
folds in), **not** by manual table mutation — a mutating action never edits the table
directly. (6) The modal is added to `overlayActive()` (mouse inert while up) and
composited **first** in `View`'s overlay switch (topmost). Without a `Deleter` wired
(`WithDeleter`) the action is inert — the modal never opens — so the pre-wiring app
and non-action tests stay quiet. This **unblocks M2-14b** (the modal-flow teatest now
has a reachable confirm through the running program).

### D116 — Full-program teatest of an async modal resolve syncs on a side-effect signal, never Quit-ordering
**2026-07-23 (M2-14b).** The confirm modal accepts/declines through an **async
round-trip** (KeyMsg → `ConfirmedMsg`/`CancelledMsg` cmd → the root hides the modal
and, on accept, runs the action off the update loop — D115). A full-program test
(teatest/v2, the M0-05 harness) therefore **must not** assert on state that a trailing
`tea.Quit()` would race the resolving message for: `Send(enter)` then `Send(Quit)`
lets Quit win, so the delete may never run in the final model. The constraint a future
modal-flow teatest (M3-10 scale, M3-11 cordon/drain, M3-12 suspend/resume) must not
break: **synchronise on the action's own side effect** — the mutating seam records the
call on a channel (`signalDeleter.called`) the test blocks on before Quit — not on
message ordering. Two corollaries reused across these tests: (1) a modal's **message
line is byte-scannable** in `teatest.Output()` (foreground-only `styles.App`), so
`WaitFor("Delete Pod pod-b?")` is a valid barrier that the open resolved — unlike the
background-filled status bar / selected table row, whose cells a plain byte scan misses
(assert those on `FinalModel`). (2) The modal's **async close on decline is not
final-model-assertable** (it races Quit); prove the close via the accept path (the modal
is hidden in `handleModalConfirmed` strictly before the delete cmd fires) and the
direct-Update decline test, and let the decline teatest assert only the race-free facts
(no delete ran; a swallowed nav left the selection put).

### D117 — Prompt-mode modal input routes through routeModalPromptKey; mutating actions share one target stash keyed by modal Kind
**2026-07-23 (M3-10).** Scale is the first action to use the modal's **prompt mode**
(a replica count), so it extends D115's confirm wiring with the text-entry rhythm the
picker/filter already use (D73). Constraints a later leg must not silently break:
(1) While `modal.Prompting()` the root routes each keypress through
**`routeModalPromptKey`** (checked in `Update`'s KeyPressMsg case, after `filtering`,
before the sequencer): a mapped no-text key (enter/esc) is a control Action fed to
`handleModalAction` (drillIn submits, back cancels), any text/editing key goes to
`modal.UpdatePrompt` — no view matches a raw digit (D11). A **confirm-mode** modal is
not `Prompting()`, so it still routes through the sequencer → `handleAction` →
`handleModalAction` (the D115 path) unchanged. (2) The two M3-10 mutating actions
share **one** stash pair (`mutateRes`/`mutateRef`) rather than a pair each, because
only one modal is ever up at a time; it is consulted only while that action's modal
Kind is up, so a stale value from a declined one is harmless (as with delete's own
pair). Later mutating actions (M3-11 cordon/drain, M3-12 suspend/resume, M3-13
port-forward prompt) reuse `mutateRes`/`mutateRef` + this routing, not a bespoke stash
or key path. (3) A prompt whose submitted text fails to parse (scale: non-integer /
negative replicas) **degrades to a status-bar error toast and runs nothing** (the
modal is already hidden — the user re-invokes), never a panic or a silent no-op —
input validation is the shell's job, the kube layer's own guard (M1-06b) is the
backstop.

### D118 — k9s is framed as a contemporary/peer of kube-commander, not "prior art"
**2026-07-23 (FB-k9s-not-prior-art).** Both kube-commander and k9s emerged around
2019–2020, so k9s is a **contemporary / kindred** Kubernetes TUI, not a predecessor
kube-commander came after or built upon. The README "Special thanks" line was
reworded from "prior art in the Kubernetes-TUI space" to "a contemporary Kubernetes
TUI in the same space." The constraint a future docs/README/marketing leg must not
silently contradict: **do not describe k9s (or any peer TUI) as "prior art" or imply
a predecessor relationship.** The other existing k9s references stay — they are
accurate and non-chronological: the "simpler and more discoverable than k9s"
comparison, the "not cloning k9s" non-goal (goals.md), the "tview (what k9s uses)"
note, and the k9s-`:`-style palette references.

### D119 — Menu does not eagerly count resource types; empty-type graying is declined for now
**2026-07-23 (FB-gray-out-empty-types).** Triaging the soft/low idea of graying left-menu
resource types that have zero objects in the current view. Two constraints a future leg
must not silently contradict:
(1) **Never implement the eager version** — kubecom must not list/count every menu type on
a namespace switch (or on any menu render) to know each item's population. That is dozens
of API calls per switch, rate-limit exposure on large clusters, and instantly-stale counts
needing re-polling/watch-all — it directly fights the lazy-list principle (D8, principle 4:
list a type only when the user drills in). The menu stays stateless-per-item with respect
to object counts.
(2) The **cheap opportunistic variant is declined for now** (not forbidden): graying only
types the user has already opened-and-found-empty, cached per `(GVR, namespace)`, reusing
the one active watch's row count. Rejected on cost/value: the value is marginal and
revisit-only (the user already saw the type was empty when they opened it; it evaporates /
re-keys on every namespace switch since only one type is watched at a time), while it would
add a namespace-keyed emptiness cache, per-`ApplyEvent` plumbing on the watch hot path, and
a **third** always-on menu visual state that must read as distinct from the existing
"unavailable/denied" muting (D57/M2-05b) — a real-terminal UX judgment (D68/D79) not worth
the standing complexity for a low/soft item. If revisited, the opportunistic variant is the
only acceptable shape (never the eager one), it must reuse the existing watch (zero extra
API calls), key emptiness by `(GVR, namespace)`, treat never-visited as **unknown ≠ empty**,
and keep "empty" visually distinct from "unavailable".

### D120 — Idempotent mutating actions dispatch directly (no confirm modal); cordon/uncordon set the pattern
**2026-07-23 (M3-11a).** Cordon/uncordon are the first mutating actions wired
**without** a confirm modal: cordoning is idempotent (a merge patch of
`spec.unschedulable`, M1-06c — re-running it is a no-op, no UID guard, D35), so a
yes/no gate would be friction with no safety value. The constraint a later leg must
not silently contradict: **an idempotent, reversible mutating action fires straight
from `handleRowAction` and reports to the status bar — it does not open the D115
confirm modal, and it carries no target stash** (`mutateRes`/`mutateRef` are for
modal-gated actions that must hold the target between an open modal and the accept;
a direct dispatch has the row's ref in hand, so it passes `msg.Resource`/`msg.Object`
through and needs no field). This splits the M3 mutating actions into two shapes:
**confirm-gated** (delete D115, rollout-restart D117 — destructive/disruptive) vs
**direct** (cordon/uncordon here — idempotent). M3-12 (CronJob suspend/resume) is
also idempotent (M1-06d, same merge-patch shape) and should follow the **direct**
shape, not a modal. The `Cordoner` seam (`WithCordoner`, nil → inert) bundles both
verbs on one interface (they share `setUnschedulable`); the outcome is a neutral
notice (`cordoned`/`uncordoned <node>`) on success or an error toast (D74) on
failure, and the Node's `Unschedulable` status flips via the live watch stream, not
by touching the table (as with every mutating action, D115).

### D121 — Drain policy default: IgnoreDaemonSets on, Force/DeleteEmptyDirData off (refuse over silent data loss); progress streams like the log pump
**2026-07-23 (M3-11b).** The Drain action (unlike cordon/uncordon, which are
idempotent and dispatch directly, D120) evicts pods, so it is **confirm-gated**
(D115) — the second shape of the M3 mutating split. Two constraints a later leg must
not silently contradict:
1. **`defaultDrainOptions = {IgnoreDaemonSets: true}`** is kubecom's drain policy.
   `IgnoreDaemonSets` is on because every real cluster runs never-evictable DaemonSet
   pods (CNI, kube-proxy, agents) — without it *every* drain is refused, a useless
   default. `Force` and `DeleteEmptyDirData` stay **off**: those are the data-loss
   flags (evicting an unmanaged pod's only copy; discarding emptyDir contents), so the
   strict default **refuses the drain upfront** naming the blocking pod
   (`DrainCandidates`) rather than silently destroying data. A future leg that surfaces
   these as per-drain toggles must default them off — never Force-by-default.
2. **A long-running action streams progress like a log stream, not a one-shot
   done-message.** `kube.DrainStream` (the channel twin of `Drain`, which is now a thin
   consumer of it so the two never diverge) emits a `DrainEvent` per step; the shell
   pumps it via `drainPump` (mirroring `logPump`/D53) tagged with a `drainGen` so a
   superseded/cancelled drain's steps are dropped, reports each step to the status bar,
   and tears the stream down on quit via `stopDrain` (the mutating twin of
   `stopLogStream`, cancel-on-quit). The `Drainer` seam (`WithDrainer`, nil → inert)
   exposes only `DrainStream`. The channel closing cleanly (no terminal `Err` event) is
   the success terminator; a terminal `DrainEvent{Err}` degrades to an error toast (D74).
   The next long-running mutating/streaming action should follow this shape.

### D122 — Port-forward is a tracked background handle observed via messages, not a stream pump; started behind a ports prompt, Pod-only for now, cancel-all-on-exit
**2026-07-23 (M3-13a).** The Port-forward action starts M1-08's `kube.PortForward`
(SPDY to the pod's portforward subresource, no kubectl binary, D2) as a **long-lived
background handle** the shell tracks — a different shape from both the one-shot mutating
actions (D120) and the drain's step-by-step pump (D121). Constraints a later leg must
not silently contradict:
1. **The shell depends on an interface, not the concrete handle.** `ActiveForward`
   (`Ready`/`Done`/`Err`/`Ports`/`Stop`) is the subset of `*kube.PortForward` the shell
   observes, so the flow is hermetically fakeable (D18). Because `kube.Clients.PortForward`
   returns the concrete `*kube.PortForward` (which satisfies `ActiveForward`) rather than
   the interface, the launcher adapts it with `tui.PortForwarderFunc` — the first seam that
   needs an adapter rather than `*kube.Clients` satisfying it directly. `PortForwarder`
   (`WithPortForwarder`, nil → inert).
2. **Lifecycle flows in through messages, never a mutex (principle 1) and never a
   channel *pump*.** Unlike the drain (one receive per Cmd re-issued each step), a
   forward has two lifecycle edges: a Cmd `select`s on `Ready()`/`Done()` and reports
   the first (`forwardReadyMsg`/`forwardDoneMsg`); on ready the shell reads the bound
   `Ports()` and arms a second Cmd blocking on `Done()`. Each forward carries a stable
   `id` so a message finds its entry after the tracked slice shifts.
3. **Started behind a ports prompt; several concurrent forwards are tracked.** The
   prompt reuses the D117 prompt-mode modal + shared `mutateRes`/`mutateRef` stash
   (only one modal is up at a time). Multiple forwards accumulate in `m.forwards`;
   **all are cancelled on quit** (`stopForwards`, cancel-on-exit — the third teardown
   after `stopLogStream`/`stopDrain`). Each forward owns a `context.CancelFunc`;
   cancelling it ends the forward cleanly (the kube handle bridges ctx→Stop), so a
   user/quit stop reads as a neutral notice, a transport failure as an error toast (D74).
4. **Pod-only for M3-13a.** `kube.PortForward` posts to the pod subresource, so the
   Port-forward action applies to `Pod` only for now; the listing panel + stop-individual
   is M3-13b, and Service→endpoint-pod resolution (mirroring `PodForOwner`, D112) is M3-13c.

---

### D123 — Port-forwarding a Service resolves it to a backing endpoint pod first (ServiceResolver seam), mirroring the logs PodResolver hop
**2026-07-24.** A **Service cannot be port-forwarded directly** — `kube.PortForward`
POSTs to the pod `portforward` subresource (D122/M1-08), which a Service does not have.
So the Port-forward action, re-extended to apply to `Service` (M3-13c) as well as `Pod`
(D122), **resolves a Service to a backing endpoint pod before opening the ports prompt**,
mirroring the logs viewer's `PodResolver`/`PodForOwner` hop (D112):
1. **`kube.PodForService(ctx, ref)`** reads the Service's `spec.selector` (a flat label
   map, via the typed clientset — no unstructured parsing), lists matching pods, and
   returns the **newest Ready pod** (fallback newest overall), reusing `newestReadyPod`.
   A **selector-less Service** (headless with manual Endpoints, or ExternalName) has no
   pods to forward to → a wrapped error the caller degrades to a toast (principle 3),
   never a panic — same for a missing Service or no matching pods.
2. **`ServiceResolver` seam** (`WithServiceResolver`, `*kube.Clients` satisfies it) — a
   **separate seam from `PodResolver`**, not a second method on it, since the two
   resolve for different features (logs vs port-forward) and a Pod row forwards directly
   with no hop. Without it wired a Service port-forward **degrades to a toast**, not a
   silent no-op (a Pod stays inert-without-forwarder as before).
3. **Resolve-then-prompt**, off the update loop, **generation-guarded** (`pfResolveGen`,
   like `viewerGen`): the prompt (and the resulting forward's label) shows the **resolved
   pod**, not the Service, so the user sees which endpoint pod is forwarding; a resolution
   that lands after a newer port-forward request is dropped. The ports the user types are
   **pod-side** — no Service-port→targetPort translation (deliberately out of this slice's
   scope; a future refinement if dogfooding wants it).

### D124 — In-process exec is a blocking `Clients.Exec` over the pod exec subresource (SPDY remotecommand), apimachinery-free, driven by the TUI via `tea.Exec` off the update loop

**Date:** 2026-07-24 · M3-14a.

kubecom execs into a container **in process** via `remotecommand.NewSPDYExecutor` on a
POST to the pod's `exec` subresource — the same SPDY upgrade `kubectl exec` uses, so no
kubectl binary is required for the primary path (D2). The primitive is
**`Clients.Exec(ctx, ref, ExecOptions) error`** and it **blocks** for the whole exec:

1. **It is a blocking call, not a stream pump or a background handle.** Unlike logs (a
   channel pump, D53) or port-forward (a tracked background handle, D122), an interactive
   exec owns the terminal for its lifetime, so `Exec` runs synchronously and returns when
   the command exits. The TUI must therefore drive it from a **suspended terminal via
   `tea.Exec`** (an `ExecCommand` whose `Run()` calls `kube.Exec`) — **never on the Bubble
   Tea update loop** (M3-14b). A clean exit returns nil; a non-zero command exit or a
   transport drop returns a wrapped error whose chain preserves the underlying
   `exec.CodeExitError` (so the exit code is recoverable).
2. **Apimachinery-free boundary (D33).** The public surface uses kubecom's own
   `ExecOptions` and `TerminalSize`/`TerminalSizeQueue` types, never client-go tooling
   types; `execStreamOptions` maps to `remotecommand.StreamOptions` and a
   `sizeQueueAdapter` translates resizes on the way to the wire, so the TUI never imports
   `remotecommand` (mirrors PortForward's `ForwardedPort`).
3. **TTY folds stderr into stdout.** With `TTY` set the primitive drops the separate
   Stderr stream and consults `SizeQueue` for PTY resizes — `StreamWithContext` rejects a
   TTY exec that also attaches Stderr, and a shell needs the PTY for line editing / job
   control. A non-TTY exec keeps all three streams and ignores the size queue.
4. **Injectable executor factory** (like PortForward's `forwarderFactory`): `runExec`
   drives a `streamExecutor` seam so argument validation, option mapping, size-queue
   adaptation, and error propagation are covered hermetically (D18); the live SPDY exec is
   envtest / dogfood territory. Empty pod name or empty command is rejected before any
   dial (#86). Raw-PTY exec is Linux/macOS only (D7); the `kubectl exec` fallback lands
   with the wiring (M3-14b).

### D125 — The exec TUI wire runs `kube.Exec` inside a `tea.Exec` `ExecCommand` that puts the local terminal raw itself; the exec size queue delivers one seeded size then session-end

**Date:** 2026-07-24 · M3-14b-1.

The Exec-shell action suspends into a shell via **`tea.Exec`** (not `ExecProcess`): an
`execCommand` (`internal/tui/exec.go`) implements bubbletea's `ExecCommand`, and its
`Run()` calls the blocking `kube.Exec` (D124) — so the shell owns the terminal off the
update loop, exactly as D124 requires. Binding constraints for future exec legs:

1. **The ExecCommand owns raw mode, not bubbletea.** bubbletea releases the terminal to
   **cooked** on suspend; an interactive remote PTY needs the *local* terminal **raw** so
   keystrokes and `^C` pass straight through. `Run()` therefore calls
   `term.MakeRaw`/`term.Restore` (`golang.org/x/term`, now a direct dep) around the exec,
   nested inside bubbletea's own release/restore. It does this **only when stdin is a real
   terminal** (`*os.File` + `term.IsTerminal`) — a non-terminal stdin (test buffer, pipe)
   skips raw/size handling and just streams, which is what keeps `Run` hermetically
   testable without a TTY.
2. **`SetStderr` is a no-op on the wire.** A TTY exec has no separate stderr (D124 §3), so
   the adapter attaches only stdin+stdout; bubbletea's `SetStderr(os.Stderr)` is dropped.
3. **The size queue seeds the initial size once, then blocks until session-end.**
   `execSizeQueue` is a one-slot buffered channel: `seed(w,h)` offers the terminal's size
   at exec start (dropped if 0×0 → server default), `Next()` returns it once then blocks,
   `close()` (deferred in `Run`) makes `Next` return nil — remotecommand's end signal.
   **Live mid-session resize (SIGWINCH) is deliberately not wired here** (M3-14b-3).
4. **This slice execs the pod's *default* container with `/bin/sh`.** Empty `Container`
   (kube.Exec → the default-container annotation / sole container), fixed `["/bin/sh"]`
   argv. Multi-container disambiguation (reuse the M3-07a `ctrPicker`) is **M3-14b-2**;
   the `kubectl exec` binary fallback is **M3-14b-4**. A clean shell exit → neutral status
   notice; any failure (attach error, missing shell, non-zero exit) → transient error
   toast (D74), never a panic. Inert with no `Execer` wired or an empty ref.

### D126 — The container-resolution path is purpose-tagged (logs ↔ exec) and routes the resolved container via `streamOrExec`; the shared `ctrPicker` is disambiguated by `ctrPurpose`

**Date:** 2026-07-24 · M3-14b-2.

The M3-07a resolve-then-pick container path (fetch a pod's containers → single one used
directly, multiple open the shared `ctrPicker`) is now shared by **both** the logs viewer
and the exec session. `resolveContainersFor(res, podRef, purpose)` carries a `ctrPurpose`
(`ctrPurposeLogs`/`ctrPurposeExec`) through the async fetch (`containersLoadedMsg.purpose`)
and the picker stash (`ctrPurpose` + the renamed `ctrStreamRes`/`ctrStreamRef`), and
**`streamOrExec`** is the single terminal that routes a resolved container to
`streamLogsInto` (logs) or `execInto` (exec). Binding constraints:

1. **One picker, purpose-routed.** `ctrPicker` is reused, not duplicated: only one is ever
   up, so a single stash serves both purposes. A future action that also picks a container
   adds a `ctrPurpose` value + a `streamOrExec` arm — it must **not** branch on the picker
   Kind (all pickers share `containerPickerKind`, D65) or add a second container picker. The
   picker title (`pickerTitle()`) disambiguates the prompt for the user.
2. **Exec goes through the same fast-path/pick split.** `openExec` no longer suspends
   directly (that was M3-14b-1's default-container behaviour); it calls
   `resolveContainersFor(..., ctrPurposeExec)`. A single-container pod (or **no
   `ContainerLister` wired** → empty container, the M3-14b-1 fallback) execs directly; a
   multi-container pod prompts. `execInto(res, ref, container)` is the exec terminal —
   `newExecCommand` now takes the chosen container (empty = default/sole).
3. **`viewerGen` guards the exec fetch too.** The exec container fetch bumps/checks
   `viewerGen` for supersession like the logs fetch, even though exec opens no viewer; the
   `gen` is unused past `streamOrExec` on the exec arm.

### D127 — Live exec terminal resize: a SIGWINCH watcher pushes the current size into the exec size queue, which is now a latest-wins one-slot channel

**Date:** 2026-07-24 · M3-14b-3.

The exec size queue (`execSizeQueue`, D125 §3) no longer delivers only the seeded size —
it now tracks the local window for the session's life. `execCommand.Run` (real-terminal
path only) starts **`watchResize(q, sizeOf)`**: a goroutine that listens for
**`syscall.SIGWINCH`** and on each one reads the current terminal size (`term.GetSize`,
injected as `sizeOf` so the pump is hermetically testable with a fake reader + an
in-process `syscall.Kill(self, SIGWINCH)`) and calls the new **`push`**. Binding
constraints for future exec/size legs:

1. **The size queue is latest-wins, never lossy-blocking.** `push` replaces the pending
   size (drops a stale unread one, retries) so a burst of resizes collapses to the newest
   and the SIGWINCH goroutine never blocks on a slow `remotecommand` reader. `seed` (the
   initial size) keeps its keep-existing semantics; `push` (resizes) supersedes. A 0×0 read
   is dropped by both.
2. **The watcher is stopped before the queue is closed.** `Run` defers `watchResize`'s
   `stop` *after* `defer q.close()`, so LIFO tears the watcher down first; `stop`
   unregisters the signal (`signal.Stop`) and **blocks until the pump goroutine has
   exited**, guaranteeing no `push` ever races a closed channel. A future leg adding another
   size producer must preserve that stop-before-close ordering.
3. **SIGWINCH resize is Linux/macOS only**, consistent with the raw-PTY path (D7/D125) —
   `syscall.SIGWINCH` exists on both; native Windows is a non-goal (WSL2).

### D128 — Exec prefers `kubectl exec` when the binary is on PATH (parity escape hatch); the in-process SPDY path is the fallback that keeps exec working without kubectl

**Date:** 2026-07-24 · M3-14b-4.

`execInto` now routes through a **`kubectl exec -it`** subprocess (`tea.ExecProcess`)
when the `kubectl` binary is on PATH, and only falls back to the in-process SPDY wire
(D125) when it is not. Binding constraints:

1. **Prefer kubectl when present; SPDY is the fallback, not the primary.** This does
   **not** reopen the hard kubectl dependency (#68/D2): exec still works with no kubectl
   installed via the in-process path. But when kubectl *is* there it owns its own raw PTY,
   SIGWINCH resize, auth plugins, and every server-side edge case, so it is the
   battle-tested parity path. Exec is one of the two sanctioned shell-outs (D2), so
   shelling out here is within the in-process-first principle, not a violation of it.
2. **The shelled-out kubectl must target the same cluster kubecom launched with.**
   `kubectlExecArgs` emits `--kubeconfig` and `--context` from the model
   (`WithKubeconfig` — new — and `WithContext`) plus `-n <namespace>` from the row, each
   only when set (else kubectl uses its standard resolution). A future flag that changes
   how kubecom resolves its cluster must be forwarded here too, or the fallback exec will
   silently hit the wrong context.
3. **The kubectl lookup is a seam (`lookupKubectl`, a package var).** It is overridable in
   tests so the fallback routing is hermetically testable without a real kubectl on the
   runner; `kubectlExecArgs` is a pure argv builder tested directly. The container arg is
   omitted when empty (kubectl picks the pod default, matching the SPDY path), and the
   shell is the same `defaultExecShell` (`/bin/sh`) both paths use.
4. **This does not close the M3 exec exit criterion.** Both paths still need a
   real-terminal/real-cluster dogfood (the advisory human-task
   `vault/human-tasks/2026-07-24-exec-live-cluster-dogfood.md`, now also covering the
   kubectl route); the criterion stays unticked pending that run.

### D129 — Edit applies via a client-side Update (PUT), not server-side apply
**2026-07-24.** The Edit action (M3-15) writes the edited object back with
`Clients.Update` (`internal/kube/apply.go`): parse the edited YAML → unstructured
→ dynamic-client **Update** (PUT) of the full object. This is `kubectl edit`'s
**default** (client-side apply), not `--server-side`. **Why:** the edited buffer is
the whole object GetYAML produced (only managedFields stripped), so it still carries
`metadata.resourceVersion` — a PUT then gets **optimistic concurrency for free**: a
concurrent server-side change → Conflict, degrade, never a silent clobber. SSA on a
full fetched object would instead take field-ownership of everything it round-tripped,
surprising and heavier for a plain edit. **Constraints a future leg must not silently
contradict:**
1. **Edit is not a rename.** `Update` rejects (before any request) an edited object
   whose name is empty or differs from the ref's, or whose namespace differs (a
   namespaced resource with an empty edited namespace is filled from the ref), and
   rejects empty/null/unparseable content — a botched edit never mutates the wrong
   object or wipes this one.
2. **Parsing goes YAML→JSON→unstructured** (`obj.UnmarshalJSON`), so integer fields
   decode as int64 (the unstructured scheme's contract), matching what the dynamic
   client round-trips — a plain YAML unmarshal would yield float64 numbers.
3. **No-change detection is the caller's job** (M3-15b): the TUI compares the edited
   bytes to the original and simply never calls `Update` on a no-op editor exit, so
   `Update` always intends to write.

### D130 — Port-forward: bind failures are actionable, not raw; `:0`/`:remote` is the free-local-port escape
**2026-07-24.** A local-listener bind failure (client-go's `unable to listen on any
of the requested ports: [{6379 6379}]`) is **not surfaced raw**. The shell detects it
by that sentinel substring (`isPortForwardBindErr`) and replaces it with an actionable
status-bar hint (`portForwardBindHint`) naming the clashing local port(s) and telling
the user to retry with a leading-colon spec — `:6379` or `:0` — to have the OS assign a
free local port (feedback `2026-07-24-port-forward-picker-and-local-port`, part 2).
This leans on an existing kube-layer capability, **not** a new one: `kube.PortForward`
already accepts kubectl's `:<remote>` syntax (leading colon → OS-assigned local port),
and `PortForward.Ports()` reports the bound local port once Ready fires, which the
status notice already shows. **Constraints a future leg must not silently contradict:**
1. Never show client-go's raw listener error to the user; route bind failures through
   the hint. Other transport errors still go through `NewErrorMsg` (classified).
2. `:0` / `:<remote>` staying a valid, documented way to auto-assign a free local port
   is load-bearing for this UX — don't remove leading-colon handling from the spec
   parse or the prompt hint.
3. The remaining parts of that feedback — a **port picker** from the pod's declared
   ports (FB-pf-port-picker) and **editable/auto local port** with a one-keystroke
   "use a free port" (FB-pf-local-port) — are deferred board tasks, not done here.

### D131 — Cluster search: one-shot concurrent fan-out over List, curated-scope default
**2026-07-24.** Cross-object **cluster search** (feedback
`2026-07-24-cluster-search-multi-resource`: type a query → matching objects across
kinds, not a within-table filter). Kubernetes has **no cross-type search API**, so
"search the cluster" means listing kinds and matching client-side — the same
expensive enumeration the fast-cold-start design (D8/principle 4) avoids on the hot
path. It is therefore built as a **one-shot, user-triggered, cancellable** query, never
a "watch everything". The kube-layer primitive is `kube.Search` /`searchRows`
(`internal/kube/search.go`, the SEARCH-01 first slice): it fans out **concurrent**
server-side `List`s (M1-05a) over a **caller-supplied** `[]Resource`, matches
`Row.Object.Name` by **case-insensitive substring**, and **streams** `SearchHit`
(`{Resource, ObjectRef}`) onto a channel. **Constraints a future leg must not silently
contradict:**
1. **One-shot, not a watch.** Each kind is listed exactly once per query; the search
   never re-lists or opens watches. Re-running is a new explicit query.
2. **Curated scope is the default; whole-cluster is an opt-in widen.** The default kind
   set is `CommonSearchResources` (Pods, Deployments, StatefulSets, DaemonSets,
   Services, ConfigMaps, Secrets, PVCs, Jobs, CronJobs, Ingresses) in the **current
   namespace**. Searching every discovered kind / all namespaces is a later opt-in
   slice — the default must never enumerate every type (D8/principle 4).
3. **Per-kind failure isolates** (principle 3): a denied/broken kind's List error is
   swallowed and contributes nothing; it never aborts the search or blanks results.
4. **Bounded + cancellable.** A hit **cap** (`limit`) stops the in-flight lists once
   reached; the channel closes on completion, cap, or ctx-cancel (query change / view
   close). A background goroutine owns all sends (principle 1).
5. Matching starts at **name substring**; fuzzy / label / field matching and
   whole-cluster/all-namespace widening are **later slices** (SEARCH-03+), not part of
   this contract. A `SearchHit` carries the `Resource` so drilling in switches the
   browse view to that kind and selects the object.

### D132 — Key contexts: the confirm modal resolves `y`/`n` in its own key context; supersedes the "no y/n" of D88/D115
**2026-07-24** (feedback `2026-07-24-confirm-modal-yn-keys`). The confirm modal now
accepts `y`/`n` (the universal yes/no muscle memory) **and** `enter`/`esc`, via two
**registered, rebindable** actions — `confirm.accept` (default `y`, `enter`) and
`confirm.decline` (default `n`, `esc`) — resolved through the keymap, **not** raw-key
matching (D11). This is the concrete realization of keybindings.md's "two actions bound
to the same key **in the same context**": the keymap now has **key contexts**
(`contextOf`). `y`/`n`/`enter`/`esc` already mean res.yaml / app.searchNext / nav.drillIn
/ nav.back in the **browse** context, so the confirm actions live in a separate
**confirm** context; `build` partitions bindings into per-context resolution indexes
(`bySeq` for browse + the sequencer, `confirmBySeq` for the modal) so the same chord
maps to a browse action **and** a confirm action with no collision. **Constraints a
future leg must not silently contradict:**
1. **The confirm modal captures input and resolves via `ConfirmAction` first**
   (`routeModalConfirmKey`, gated on `m.modal.Active() && !Prompting()`, before the
   sequencer). Unmatched keys fall back to the browse keymap so `app.quit` still
   dismisses; everything else is swallowed. **Prompt-mode** modals are unchanged —
   they still submit/cancel on `nav.drillIn`/`nav.back` via `routeModalPromptKey` (a
   typed `y`/`n` is text there, never accept/decline).
2. **`modal.Update` accepts on `confirm.accept` OR `nav.drillIn`, declines on
   `confirm.decline` OR `nav.back`** — so both confirm keys and the prompt-mode nav
   keys resolve one component.
3. **This supersedes the "no `confirm.yes`/`confirm.no` actions, no raw y/n" clause of
   D88 and D115.** The rest of those decisions stands: the modal is Kind-stamped,
   message-only (principle 1), one `modal.Model` on the root, results routed by Kind.
   Adding another key context (e.g. a viewer context) follows this pattern — a new
   `contextOf` entry + a context-scoped resolver, never raw-key matching in a view.

### D133 — Default row-action keys: delete is `d`, describe relocates to `D`
**2026-07-24** (feedback `2026-07-24-delete-default-key-d`). The shipped **default**
delete binding is now `d` (`res.delete`), matching vim `dd`-style muscle memory; the
old `x` default is dropped. Describe (`res.describe`), which previously owned `d`,
relocates to **`D`** (capital, read-only, not a reserved nav chord). Everything stays
registry-driven and rebindable (D11) — this only changes `defaultBindings`, the
generated `docs/keybindings.md`, and the design-intent table; no view matches a raw
key. **Constraints a future leg must not silently contradict:**
1. **`d` = delete, `D` = describe** in the default keymap. Neither is in the reserved
   nav set (`navChords`), so no warn/collision; the freed `x` is now unbound by default.
2. **The coming "unify view-YAML + edit" leg (feedback `unify-yaml-view-and-edit`)
   must lay out its key against this surface** — it collapses `res.yaml` (`y`) and
   `res.edit` (`e`) into one editable-object action and frees a key; `d`/`D` are settled
   and must not be reused for it. Describe and logs stay read-only viewers. This keeps
   the two coupled feedback items from producing conflicting one-off key layouts.

### D134 — Logs get a dedicated full-screen logs view (`logsview`) with a live filter, off the shared read-only viewer
**2026-07-24** (feedback `2026-07-24-logs-dedicated-view-live-grep`). Logs stop sharing
the M3-01 read-only viewer and move to a **dedicated full-screen logs mini-app**
(`internal/tui/components/logsview`) built for streaming + a **real-time grep**: a
`/`-filter (reusing `app.filter`) that narrows the streamed buffer **live while
following** (case-insensitive substring now; regex is LOGS-03), plus follow/pause
(reusing `logs.follow`) with auto-scroll-to-bottom on append and a header showing
`[following]`/`[paused]` + the active filter and matched/total. The feedback was
triaged into board tasks **LOGS-01** (this component, done) → **LOGS-02** (app wiring,
retire the shared-viewer logs path) → **LOGS-03** (regex + highlight) → **LOGS-04**
(wrap/timestamps/jump-to-latest). **Constraints a future leg must not silently
contradict:**
1. **The shared viewer (M3-01) keeps serving YAML/describe/secret; logs do not.** Once
   LOGS-02 lands, the logs path streams into `logsview`, and the `viewerKindLogs`
   special-casing / `logFollow` / `logTitle` on the shared-viewer path is removed — do
   not re-route logs back onto the shared viewer.
2. **Logs render full-screen (no centered border box)** — the root composites the view
   as the base while it is up, not via `overlayCenter` (D95). It is the one M3 viewer
   that replaces the base rather than floating over it, because logs want every column
   for long lines / high throughput.
3. **The live filter narrows client-side over the full buffer while following** — a
   line that arrives under an active filter is shown only if it matches; clearing the
   filter (one `nav.back`) restores the full stream, a second `nav.back` closes the
   view. Keymap-driven (D11), message-only (principle 1): the only raw-key entry is the
   filter field (`UpdateFilter`), exactly as the picker.
4. **Container picker (M3-07a) and pod-owning resolution (M3-07b) still feed logs** —
   LOGS-02 keeps that resolve-then-stream plumbing and the gen-tagged log pump (D53);
   this decision changes the *sink*, not how a pod/container is chosen.

### D135 — View YAML and Edit unify into one object-YAML action ($EDITOR edit-in-place); the standalone read-only YAML viewer is retired
**2026-07-24** (feedback `2026-07-24-unify-yaml-view-and-edit`). A separate read-only
**View YAML** (`y`, M3-03) and **Edit** (`e`, M3-15) are redundant: viewing and editing
are the same act on an object's YAML. They **collapse into one action that opens the
object's YAML in the user's `$EDITOR`** (the sanctioned suspend, D2/D125) — change
nothing and you just close it (a clean no-op), change something and it applies on save
through `kube.Update` (M3-15a/D129). Describe and logs **stay read-only** (you can't
apply a describe); this unification is specifically the object's own YAML. **Supersedes
the M3-03-vs-M3-15 split.** Delivered bottom-up:
- **M3-15b** (this leg): the Edit → `$EDITOR` **suspend + apply machinery** wired to the
  `res.edit` action — fetch YAML (reusing the M1-07a `YAMLGetter`) → temp file →
  `tea.Exec` `$EDITOR` → read back → `Editor` seam (`kube.Update`) only on change;
  no-change / editor-abort / apply-rejection all degrade to a status-bar toast without a
  partial mutation (principle 3). `$EDITOR` resolution is `KUBE_EDITOR` → `EDITOR` → `vi`,
  space-split for flags (`code -w`).
- **M3-15c** (follow-up): retire the standalone read-only YAML viewer path
  (`openYAMLViewer` / `yamlLoadedMsg` / `handleYAMLLoaded` / `viewerKindYAML`) and collapse
  the surface to **one key** — edit becomes the object-YAML action, `res.yaml`/`View YAML`
  goes away; coordinate the freed key with the D133 delete/describe layout and regenerate
  `docs/keybindings.md`. The `YAMLGetter` seam **stays** — the edit flow fetches through it.

**Constraints a future leg must not silently contradict:**
1. **Edit is the single object-YAML action; there is no separate read-only YAML viewer**
   once M3-15c lands. Do not reintroduce a `y`-opens-a-read-only-viewer path.
2. **A no-op edit (buffer unchanged) never calls `Update`** — the caller compares the
   read-back bytes to the fetched bytes; only a real change applies (kube.Update owns
   validation + optimistic concurrency, M3-15a).
3. **Describe/logs remain read-only viewers** — unification is the object's YAML only.
4. The live `$EDITOR` suspend needs a **human dogfood** (like exec, D125); the M3 "Edit
   round-trips through `$EDITOR`" exit criterion stays unticked until that lands
   (`vault/human-tasks/2026-07-24-edit-live-cluster-dogfood.md`).

### D136 — M3-15c resolves D135: the unified View/Edit YAML action keeps `e`, gates on `canGet`, and `y` is retired
**2026-07-24** (M3-15c, completing D135). Two choices D135 left open, now settled:

1. **The surviving key is `e` (`res.edit`); `y` (`res.yaml`) is removed and left unbound in
   the browse context.** Rationale: `e`=edit is the accurate, conventional mnemonic for an
   action that can mutate, and it was already the shipped edit key — no new muscle memory,
   minimal churn atop the D133 `d`(delete)/`D`(describe) layout. `y` is *not* repurposed
   (no surprise "peek turns into a mutating editor" on the long-standing view key); it stays
   free for a future rebind or user config. `y` keeps its **confirm-context** meaning
   (`confirm.accept`, D132) — that context split now stands on `n`/`enter`/`esc` alone.
2. **The action's applicability predicate is `canGet`, not `update`/`patch`.** The unified
   action is **viewer-first**: you need `get` to render the YAML, and edit is best-effort —
   a save on a resource you can't write degrades to a toast on the apply's RBAC error
   (principle 3), exactly as `kubectl edit` opens a read-only object and fails only on save.
   Gating on `canEdit` would have **regressed** YAML viewing for read-only (get-only) users
   and kinds — a real, common case (read-only kubeconfig, componentstatuses). A future leg
   must not re-gate this action on write verbs. Menu title: **"View / Edit YAML"**; it stays
   in the mutating group (last, before delete) since a save can mutate.
