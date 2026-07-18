# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-18 — M1-04 done: on-disk discovery cache (`internal/kube/cache.go`) — kubectl's `discovery/cached/disk.CachedDiscoveryClient` backs discovery+RESTMapper; per-host cache dir under `os.UserCacheDir()/kubecom`, TTL 6h; `Clients.Invalidate()` forces refetch; degrades to in-memory when no cache dir; zero network I/O at construction; D32. Active milestone: M1; next up M1-05 (server-side Table List+Watch → event channel; reconnect/resync). M1-04b (lazy group detail) parked until the M2 menu exists._

## In Progress

- [ ] **M1-05a** Server-side Table **List** (`Accept: as=Table`) → typed `Table{Columns,Rows}`, generic over any resource incl. CRDs (namespaced + cluster-scoped)
      status: in-progress | owner: claude-opus | added: 2026-07-18 | claimed: 2026-07-18
      notes: first slice of M1-05 (List half); watch → event channel with reconnect/resync is M1-05b

## Blocked

_(none)_

## Backlog

### M0 — Groundwork
_(none — M0 complete)_

### M1 — Kube layer
- [ ] **M1-04b** Lazy group detail on first open (fetch a group's full resource detail only when its menu is opened)
      status: todo | owner: — | added: 2026-07-18
      notes: split from M1-04 — M2-coupled; needs the menu open interaction. Do after M2 menu exists.
- [ ] **M1-05b** Server-side Table **Watch** → event channel (add/modify/delete/reset); reconnect/resync on expiry; bounded buffering
      status: todo | owner: — | added: 2026-07-18
      notes: second slice of M1-05 — builds on M1-05a's Table decoding; use a RetryWatcher-style reconnect keyed off resourceVersion
- [ ] **M1-06** Actions: delete/scale/rollout-restart/cordon/drain/cronjob-suspend
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-07** Streaming: logs; describe (kubectl/pkg/describe); get-as-YAML
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-08** Background port-forward (start/stop)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-09** Typed graceful errors (no panics on bad ns/context) (#86)
      status: todo | owner: — | added: 2026-07-18

### M2 — Core TUI (partial seed)
- [ ] **M2-01** Action registry + configurable keymap: Action ids, default (vim-first) keymap, merge(default, config.Keys) + validation; `KeyMsg → Action` resolution; generate bindings/help from registry (D10, D11)
      status: todo | owner: — | added: 2026-07-18
      notes: foundational — land before other input handling; no raw-key matching in views

_Remaining M2–M5 items to be expanded when those milestones open. See milestone files for scope._

## Done

- [x] **M1-04** On-disk discovery cache + invalidation: `internal/kube/cache.go` + rewired `NewClients` — discovery is now backed by kubectl's on-disk `discovery/cached/disk.CachedDiscoveryClient` (replaces the M1-02 in-memory memcache as the base of the deferred RESTMapper, so discovery+mapping share one on-disk cache). Cache dir `os.UserCacheDir()/kubecom/{discovery/<host-slug>,http}` — per host:port (distinct clusters must not cross-serve), disposable, separate from the D20 config dir; TTL 6h. `Clients.Invalidate()` forces a refetch (clears disk cache + Resets the retained deferred mapper); seed mapper untouched. Degrades to in-memory `memcache` when no cache dir resolves; still zero network I/O at construction (fast cold start holds). `Discovery` field widened to `CachedDiscoveryInterface`. New transitive deps: httpcache, diskv, btree. Lazy group detail → M1-04b. D32.
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M1-03** Async full discovery → reconcile signal; per-group fault isolation: `internal/kube/discovery.go` — `discoverResources` runs one `ServerPreferredResources` pass and flattens it into a sorted `[]Resource` (GVK/GVR/scope/verbs/short-names/categories) of listable, non-subresource kinds. Partial `*discovery.ErrGroupDiscoveryFailed` keeps healthy resources and isolates each broken/denied group into `DiscoveryResult.Failed` (#87, #76); total failure → `.Err`, empty results. `StartDiscovery(ctx,d) <-chan DiscoveryResult` runs the pass in a goroutine, delivers once on a cap-1 buffered channel (the "discovery ready" reconcile signal), returns immediately — never blocks first paint. `(*Clients).StartDiscovery` convenience; zero TUI imports. Cache/re-discovery deferred to M1-04. D31.
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M1-02** Seed-set core GVKs with static REST mapping for instant start: `internal/kube/seed.go` — a static `meta.DefaultRESTMapper` seeded with ~28 core GVKs (core/v1, apps/v1, batch/v1, networking/v1, rbac/v1, storage/v1), each with its exact plural/singular resource + scope via `AddSpecific` (irregular plurals kubectl-identical). `NewClients` composes it ahead of the D29 deferred discovery mapper via `meta.FirstHitRESTMapper` — seeded kinds resolve instantly with zero network I/O; unknown kinds fall through to discovery. Instant-start half of D8; M1-03 adds async full-discovery reconcile. D30.
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M1-01** Client bootstrap: `internal/kube/client.go` — `ClientConfig{Kubeconfig,Context}` → `RESTConfig` (clientcmd default rules + context override, non-interactive, wrapped errors) → `NewClients` (clientset + dynamic + discovery + deferred discovery RESTMapper) + `Connect` convenience. No network I/O at construction; RESTMapper deferred/mem-cached so bootstrap never blocks first paint (D8). Hermetic tests (temp kubeconfig + dummy rest.Config, D18); no new deps. D29.
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M1-00** envtest harness: `internal/kube/envtest_test.go` — opt-in behind `KUBECOM_TEST_ENVTEST=1` (`requireEnvtest(t)` skips by default so `make check` stays hermetic), smoke test starts a real control plane and GETs the `default` namespace. Added the first kube deps (client-go + apimachinery v0.31.4, controller-runtime v0.19.4) and `make test-envtest` (fetches binaries via `setup-envtest`). D28.
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-06** goreleaser skeleton (Linux+macOS): `.goreleaser.yml` reshaped to v2 syntax, single build × `goos:[linux,darwin]` × `goarch:[amd64,arm64]`; Windows dropped (D7), publishers (Homebrew/AUR/Docker) deferred to M5, `kubectl` brew dep removed (D2). `goreleaser check` + `--snapshot` verified (D27)
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-09** README references the vault + decision log: rewrite-in-progress banner atop `README.md` linking `vault/`, goals, milestones, board, `decisions.md`, journal, and `CLAUDE.md`. Closes the last open M0 exit criterion; full README rewrite deferred to M5.
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-05** Test harness: teatest (tui) smoke test — bubbletea v2 (`charm.land/bubbletea/v2` v2.0.2) + teatest/v2; placeholder root model; Go floor → 1.24.2 (D26)
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-08** Sweep remaining legacy files M0-07 missed: `Dockerfile`, `get.sh`, `ci/aur/` (incl. `id_rsa.enc`), `ci/terminalizer/`, empty `ci/`; `.goreleaser.yml` de-referenced from `ci/aur/` (D25)
      status: done | owner: claude-opus | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-04** GitHub Actions CI: `make check` (build/test/vet/golangci-lint) on Linux+macOS matrix (D24)
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-02** Toolchain: bump to Go 1.23 (go.mod directive), wire `cmd/kubecom` onto cobra v1.10.2; `ioutil` already gone via M0-07 (D23)
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **PROC-04** Cloud runs set repo-local git identity (maintainer) in routine bootstrap + skill step 0
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **PROC-03** Model split: routine session on Sonnet (orchestration), leg subagents pinned to Opus in `/do-rewrite-run`; cron corrected to UTC
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-07** Delete legacy trees from `v1`: `app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`, Windows sources, Travis/snap CI; prune `go.mod`; drop `.golangci.yml` path excludes. Absorbs M0-03. (D14, D22)
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **PROC-02** Scheduled runs self-prime: checkout `v1` + lint tooling in step 0; legs read the skill by file path (cloud clones start on `master`)
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
- [x] **PROC-01** `/do-rewrite-run` orchestrator skill: fresh subagent per leg, sequential, 4-leg/90-min budgets — for the scheduled routine
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18 (D21)
- [x] **REVIEW-01** Maintainer setup review applied: legacy deletion planned, journal split to per-entry files, claim-push, `make check` + CI-early, fakes-default tests, Bubble Tea v2, XDG config path
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18 (D14–D20)
- [x] **M0-03** Single `kubecom` binary — absorbed into M0-07 (D14)
      status: done (absorbed) | owner: — | added: 2026-07-18 | done: 2026-07-18
- [x] **M0-01** Scaffold new module layout (`cmd/kubecom`, `internal/{kube,tui,config,version}`) + `.golangci.yml` scoped to new code
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18 (D12, D13)
- [x] **BOOT-01** Bootstrap vault, goals, milestones, task board on `v1`
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
