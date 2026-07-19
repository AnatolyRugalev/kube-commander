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
