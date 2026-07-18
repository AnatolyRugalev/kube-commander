# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-18 — vault bootstrap; added vim-first navigation (D10)._

## In Progress

_(none yet)_

## Blocked

_(none)_

## Backlog

### M0 — Groundwork
- [ ] **M0-01** Scaffold new module layout (`cmd/kubecom`, `internal/{kube,tui,config,version}`)
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-02** Toolchain: bump to Go 1.23+, latest cobra; drop `ioutil`
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-03** Single `kubecom` binary; remove duplicate `kube-commander` entrypoint
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-04** GitHub Actions CI: build/test/vet/golangci-lint (Linux+macOS); drop Travis
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-05** Test harness: envtest (kube) + teatest (tui) smoke tests
      status: todo | owner: — | added: 2026-07-18
- [ ] **M0-06** goreleaser skeleton (Linux+macOS); remove Windows sources
      status: todo | owner: — | added: 2026-07-18

### M1 — Kube layer
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

_M2–M5 items to be expanded when those milestones open. See milestone files for scope._

## Done

- [x] **BOOT-01** Bootstrap vault, goals, milestones, task board on `v1`
      status: done | owner: claude | added: 2026-07-18 | done: 2026-07-18
