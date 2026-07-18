# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-18 — REVIEW-02: M0-07 execution reviewed (clean); M0-08 added for legacy files the tree-based deletion missed._

## In Progress

_(none yet)_

## Blocked

_(none)_

## Backlog

### M0 — Groundwork (ordered top-to-bottom; ids are stable, list order is priority)
- [ ] **M0-02** Toolchain: bump to Go 1.23+, latest cobra (wire `cmd/kubecom` onto cobra); drop `ioutil` (trivial after M0-07)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-04** GitHub Actions CI: `make check` (build/test/vet/golangci-lint) on Linux+macOS (D17)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-05** Test harness: teatest (tui) smoke test — envtest moved to M1 (D18)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-08** Sweep remaining legacy files missed by M0-07's tree-based deletion: `Dockerfile` (golang:1.15 + baked-in kubectl, contradicts D2), `get.sh`, `ci/aur/` (old binary names; incl. `id_rsa.enc` encrypted SSH key), `ci/terminalizer/` (vhs replaces it). Delete; anything still wanted gets rebuilt in M0-06/M5.
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-06** goreleaser skeleton (Linux+macOS)
      status: todo | owner: — | added: 2026-07-18

### M1 — Kube layer
- [ ] **M1-00** envtest harness: opt-in via `KUBECOM_TEST_ENVTEST=1`, one passing smoke test (moved from M0-05, D18)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-01** Client bootstrap: clientset + dynamic + discovery + RESTMapper from kubeconfig/context
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-02** Seed-set core GVKs with static REST mapping for instant start
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-03** Async full discovery → reconcile signal; per-group fault isolation (#87, #76)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-04** On-disk discovery cache + invalidation; lazy group detail
      status: todo | owner: — | added: 2026-07-18
- [ ] **M1-05** Server-side Table List+Watch → event channel; reconnect/resync
      status: todo | owner: — | added: 2026-07-18
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
