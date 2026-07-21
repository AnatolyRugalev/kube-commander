# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-21 — M2-10…M2-13 stay gated by the open dogfood human-task; the loop is doing its explicitly-unblocked carve-out (teatest coverage of existing behavior) via M2-14c. Per-leg history: `vault/journal/`._

## In Progress

_(none)_

## Blocked

_(none)_

## Backlog

### M0 — Groundwork
_(none — M0 complete)_

### M1 — Kube layer
_M1 is feature-complete (D66); M1-04b was retired as obsolete (D81), so only the
deferred envtest item remains — not a blocker._
- [ ] **M1-INT** envtest integration tests (opt-in `KUBECOM_TEST_ENVTEST=1`): restricted-RBAC group isolation, watch reconnect/resync, action set against a live apiserver
      status: deferred | owner: — | added: 2026-07-20
      notes: D66 — fake-client coverage is the autonomous-loop bar; these need control-plane binaries (fragile in cloud, D18), so a human runs them locally or a dedicated CI job with `setup-envtest` does. Not an M1 blocker.

### M2 — Core TUI
M2-01 (action registry + configurable keymap, D10/D11) is **complete** (01a keymap
core / 01b sequences / 01c config wiring / 01d help overlay / 01e generated doc).
The rest of M2 is the **app shell** — expanded here into ordered, leg-sized slices
(D52). Take them top-down; each notes what it depends on. Package layout follows
[`../REWRITE_PLAN.md`](../REWRITE_PLAN.md): `internal/tui/{app.go,msg.go}`,
`internal/tui/styles`, `internal/tui/components/*`, `internal/tui/views/*`. Every
slice keeps **zero shared mutable UI state** (principle 1); goroutines only send
messages.

> **NEXT PICK — M2-RUN** must land before the remaining component legs: today the
> binary never launches the TUI or touches a cluster (no `tea.NewProgram`, no real
> client construction), so nothing has been exercised end-to-end against a real
> apiserver. Once it lands, every subsequent leg is verified against the running
> binary and must **incrementally improve the real-cluster experience** (D68).

- [x] **M2-07** Root app model / shell (`internal/tui/app.go`, replaces the M0
      `internal/tui/tui.go` placeholder)
      status: done (07a, 07b, 07c, 07d done) | owner: claude-opus | added: 2026-07-19 | done: 2026-07-20
      notes: Split — **M2-07a** ✅ done 2026-07-20 (D61): root `tea.Model` skeleton
      owning the resolved keymap + `Sequencer`, routing `KeyMsg`→`Action` (schedules
      `tea.Tick` on `ResultPending`, D48; gen-tagged tick drops stale timeouts),
      embedding the M2-01d help overlay (toggle `app.help`, `nav.back` closes it),
      window-size, and `app.quit` — no panes yet.
      **M2-07b** ✅ done 2026-07-20 (D62): composed the two-pane **browse** layout —
      the M2-05 menu (left) + M2-06 table (right) over the M2-04 status bar; menu
      starts focused; `nav.right`/`nav.left` switch focus (table's horizontal scroll
      takes precedence until `HOffset()==0`, D60); non-switching nav routes to the
      focused pane; the open help overlay swallows nav.
      **M2-07c** ✅ done 2026-07-20 (D63): drilling into a menu item starts a live
      `kube.Watch` (via a narrow `ResourceWatcher` seam injected with `WithWatcher`;
      nil → watch-inert), streaming deltas into the table through the M2-02 pump
      (`ResourceEventMsg`→`ApplyEvent`; first RESET repopulates); a new selection
      cancels the previous watch, a generation guard dropping its in-flight deltas;
      focus moves to the table. Top-unblocked next: **M2-07d**.
      **M2-07d** ✅ done 2026-07-20 (D64): kicks off async discovery on `Init`
      (via a `Discoverer` seam injected with `WithDiscoverer`, mirroring
      `WithWatcher`; nil → discovery-inert). `Init` defers the start one message
      hop (`startDiscoveryMsg`) since it can't mutate the value model; `Update`
      opens a cancellable pass, starts the M2-04 spinner, and batches its tick with
      the M2-02 `discoveryPump`. `DiscoveryReadyMsg` stops the spinner and calls
      `menu.Reconcile` (M2-05b) — no-op/seed on total failure (principle 3);
      `spinner.TickMsg` forwarded to the status bar; `app.quit` cancels the pass.
      **M2-07 (root shell) is complete.** Top-unblocked next: **M2-08**.

- [ ] **M2-10** Confirm/prompt modal (`internal/tui/components/modal`)
      status: todo | owner: — | added: 2026-07-19
      notes: Replaces the old racy tcell popup (REWRITE_PLAN motivation). A
      message-driven overlay: confirm (y/n) + text prompt, resolved through keymap
      actions, returns a result msg. No mutex, no shared popup state (the whole
      point). Depends on: M2-03, M2-07a.

- [ ] **M2-11** Config: menu customization persistence (`internal/config`)
      status: todo | owner: — | added: 2026-07-19
      notes: Extend the M2-01c `Config` struct with the menu/browse fields
      (customized resource list, order, last namespace) + `Save`/`SaveFile`
      (round-trips the `keys:` field). Wire the app to load on start and persist
      changes. `UnmarshalStrict` already rejects typos (D49). Depends on: M2-05,
      M2-07.

- [ ] **M2-12** Legacy config migration (`internal/config/migrate.go`)
      status: todo | owner: — | added: 2026-07-19
      notes: One-shot migration from the old `~/.kubecom.yaml` (protobuf-yaml theme
      config) to the new plain-YAML config on first start; malformed/legacy files
      degrade gracefully (principle 3), never block start. Inspect the `master`
      branch's `config/` for the old shape. Depends on: M2-11.

- [ ] **M2-13** Column sort (#85)
      status: todo | owner: — | added: 2026-07-19
      notes: Client-side sort by column on the table's row set (keymap `sort.*`),
      stable, type-aware where cheap. Closes #85. Depends on: M2-06.

- [ ] **M2-14b** teatest coverage: modal confirm flow
      status: todo | owner: — | added: 2026-07-20
      notes: Split from M2-14 — the modal-flow half. Drive a modal confirm (y/n)
      flow with teatest/v2 (M0-05 harness). Depends on: M2-10 (the modal itself),
      which the open dogfood human-task gates — do this once M2-10 lands.

_Remaining M3–M5 items to be expanded when those milestones open. See milestone files for scope._

## Done

- [x] **M2-14c** teatest coverage: filter flow (M2-09b) driven end-to-end through the real bubbletea program — `/` → type → enter-commit via live keypresses, asserted on `FinalModel` (status-bar filter segment is background-styled, so a byte scan misses it; the final model proves the committed query, closed input, and narrowed row set). Added a `fakeWatcher.preload` seam so a program test gets watch rows without racing the channel. Test-only, no product code — done 2026-07-21
- [x] **M1-04b** Lazy group-detail-on-open: **retired won't-do** — doesn't fit the realized flat Dashboard-sectioned menu (no group-open interaction, D77) and its non-blocking cold-start intent is already delivered by seed (M1-02) + async discovery (M1-03) + per-host on-disk cache (M1-04); reintroducing a collapsible-group menu for it would need a fresh UX decision superseding D77 — done 2026-07-20 (D81)
- [x] **M2-14a** teatest coverage (update loop + menu reconcile): split from M2-14 — the modal-flow half is deferred to M2-14b (blocked by the gated M2-10). Added `TestProgramRoutesKeyToPicker` (drives a live ctrl+n through the real bubbletea program via teatest/v2 — key→action→async list→msg→View round-trip, asserting the seeded namespace renders) and `TestReconcilePreservesSelection` (moves the menu cursor off row 0, then a `DiscoveryReadyMsg` appending a CRD must keep the same resource selected by GVR — the D57 preservation guarantee the existing reconcile test never exercised). Test-only, no product code — done 2026-07-20 `app.filter` (`/`) opens a `bubbles/textinput` over the current table (no-op without one), routed through the picker's control/text split (D73) — text narrows live via `SetFilter`, mapped no-text keys are control actions; enter commits (routing resumes for j/k + n/N), esc clears-and-closes (restores rows, D78) and also clears a committed filter; new-resource selection resets filter state; `app.searchNext`/`Prev` (n/N) step matches with wrap (`table.SelectNextWrap`/`SelectPrevWrap`), no-op without a filter; active filter shown as a status-bar segment (`statusbar.SetFilter`) — done 2026-07-20 (D73, D74, D78, D80)
- [x] **M2-09a** Table filter core: authoritative unfiltered `full` row set + a displayed filtered `table` view; `SetFilter`/`ClearFilter`/`Filter`/`TotalRowCount` narrow the rendered rows to a case-insensitive substring match across the *visible* (priority-0) columns, selection preserved by UID; watch deltas mutate `full` so clearing the filter restores every live row; `SetTable` clears the filter, `ApplyEvent` preserves it (D78). Component-only, not yet shell-wired — done 2026-07-20 (D78)
- [x] **FB-menu-nesting** Feedback (normal): flat left menu read as disorganized → resource menu now renders Dashboard-style sections (Cluster / Workloads / Config / Network / Storage / Access Control + trailing Custom Resources for CRDs) with non-selectable, cursor-skipped headers; `Item.Section` + `menu.rows()` expand items into header+item display rows, scroll offset became display-row based (`cursorRow`), items indent under their header, Reconcile appends discovered extras into Custom Resources (D57 otherwise unchanged) — done 2026-07-20 (D77)
- [x] **FB-welcome-page** Feedback (normal): bare launch showed an empty right-pane table → new `welcome` component (`internal/tui/components/welcome`) shows name/version, context · namespace scope, a pick-a-resource hint, and registry-generated key hints until the first drill-in, then the live table takes the slot (gated by `hasCurrent`); `WithContext`/`WithVersion` + `kube.ContextName` (no-network, blank-on-failure) wire the props; status bar now shows the context too — done 2026-07-20 (D76)
- [x] **FB-go-install** Feedback (normal): README `go install …/cmd/kubecom@v1` failed (`@v1` is a semver version query — resolves to a nonexistent `v1.x.x` tag, never the branch) → Install section now leads with a local `v1` checkout + `go install ./cmd/kubecom`, `@v1` remote form dropped, commit-SHA pin noted as the working remote alternative, clean remote `go install …@latest` deferred to an M5 release tag — done 2026-07-20 (D75)
- [x] **FB-errors-layout** Feedback (high): errors broke the TUI layout → transient single-line status-bar toast; root `Update` now handles `ErrorMsg` (was dropped), watch-start/ns-list/watch-ERROR all route through `surfaceError`, auto-clear is generation-guarded — done 2026-07-20 (D74)
- [x] **M2-08c** Wire namespace picker into the app shell: `ns.switch`/`ctrl+n` action + `NamespaceLister` seam (`WithNamespaceLister`, nil → inert); async list seeds the picker, selection sets `m.namespace` + status bar and re-scopes the live watch, `docs/keybindings.md` regenerated — done 2026-07-20 (D73)
- [x] **M2-08** Namespace picker (08a component / 08b filtering / 08c app wiring) — done 2026-07-20 (D65, D72, D73)
- [x] **M2-RUN** Bare `kubecom` launches the browse UI against a real cluster (root `RunE` + kubeconfig/context/`-n` flags → live `*kube.Clients` via `WithWatcher`/`WithDiscoverer`/`WithNamespace`; graceful on bad kubeconfig; file logging) — done 2026-07-20 (D70, D71) | follow-up: human live-cluster dogfood of drill-in/live rows (D68), not reproducible in the sandbox
- [x] **M2-08b** Picker filtering: picker-owned textinput, case-insensitive substring narrowing, control/text key split, back clears-then-cancels — done 2026-07-20 (D11, D65, D72)
- [x] **M2-08a** Generic modal picker component — done 2026-07-20 (D11, D45, D56, D65)
- [x] **M2-07d** Root app shell: async discovery on Init → menu reconcile + status-bar spinner — done 2026-07-20 (D8, D18, D57, D63, D64)
- [x] **M2-07c** Root app shell: live table wired to `kube.Watch` — done 2026-07-20 (D18, D60, D61, D62, D63)
- [x] **M2-07b** Root app shell: two-pane browse layout with focus switching — done 2026-07-20 (D60, D62)
- [x] **M2-07a** Root app model / shell: keymap-routed skeleton — done 2026-07-20 (D11, D48, D61)
- [x] **M2-06c** Table component: horizontal scroll — done 2026-07-20 (D58, D60)
- [x] **M2-06b** Table component: live watch deltas — done 2026-07-20 (D34, D59)
- [x] **M2-06a** Table component: snapshot render — done 2026-07-20 (D11, D54, D56, D58)
- [x] **M2-05b** Resource-menu sidebar: discovery reconcile — done 2026-07-19 (D57)
- [x] **M2-05a** Static seed resource-menu sidebar — done 2026-07-19 (D11, D54, D56)
- [x] **M2-04** Status bar component — done 2026-07-19 (D11, D54, D55)
- [x] **M2-03** Lipgloss theme + style set — done 2026-07-19 (D6, D50, D54)
- [x] **M2-02** TUI message types + channel→msg pumps — done 2026-07-19 (D53)
- [x] **M2-PLAN** Expand the M2 app-shell into ordered, leg-sized Backlog slices (M2-02 … M2-14) — done 2026-07-19 (D52)
- [x] **M2-01e** Generated keybindings doc from the registry + drift check — done 2026-07-19 (D11, D50, D51)
- [x] **M2-01d** Help generated from the keymap registry — done 2026-07-19 (D11, D26, D50)
- [x] **M2-01c** Config `keys:` wiring — done 2026-07-19 (D20, D49)
- [x] **M2-01b** Multi-key sequences + timeout resolution — done 2026-07-19 (D10, D47, D48)
- [x] **M2-01a** Keymap core — done 2026-07-19 (D10, D11, D47)
- [x] **M1-09** Typed graceful errors — done 2026-07-19 (D46)
- [x] **M1-08** Background port-forward — done 2026-07-19 (D2, D33, D39, D43, D45)
- [x] **M1-07d** Reconnecting/resuming follow logs — done 2026-07-19 (D34, D40, D44)
- [x] **M1-07c** Streaming pod logs — done 2026-07-19 (D2, D39, D43)
- [x] **M1-07b** Describe — done 2026-07-19 (D2, D42)
- [x] **M1-07a** Get object as **YAML** — done 2026-07-19 (D2, D41)
- [x] **M1-06e-2** Drain: **eviction loop** — done 2026-07-19 (D35, D40)
- [x] **M1-06e-1** Drain: pod **selection** — done 2026-07-19 (D33, D39)
- [x] **M1-06d** Actions: **cronjob suspend/resume** — done 2026-07-19 (D37, D38)
- [x] **M1-06c** Actions: **cordon/uncordon** — done 2026-07-18 (D37)
- [x] **M1-06b** Actions: **scale** + **rollout-restart** — done 2026-07-18 (D36)
- [x] **M1-06a** Action: generic **delete** — done 2026-07-18 (D33, D35)
- [x] **M1-05b** Server-side Table **Watch** → event channel: `internal/kube/watch.go` — done 2026-07-18 (D34)
- [x] **M1-05a** Server-side Table **List** → typed `Table{Columns,Rows}`: `internal/kube/table.go` — done 2026-07-18 (D33)
- [x] **M1-04** On-disk discovery cache + invalidation: `internal/kube/cache.go` + rewired `NewClients` — done 2026-07-18 (D20, D32)
- [x] **M1-03** Async full discovery → reconcile signal; per-group fault isolation: `internal/kube/discovery.go` — done 2026-07-18 (D31)
- [x] **M1-02** Seed-set core GVKs with static REST mapping for instant start: `internal/kube/seed.go` — done 2026-07-18 (D8, D29, D30)
- [x] **M1-01** Client bootstrap: `internal/kube/client.go` — done 2026-07-18 (D8, D18, D29)
- [x] **M1-00** envtest harness: `internal/kube/envtest_test.go` — done 2026-07-18 (D28)
- [x] **M0-06** goreleaser skeleton (Linux+macOS): `.goreleaser.yml` reshaped to v2 syntax, single build × `goos:[linux,darwin]` × `goarch:[amd64,arm64]`; Windows dropped (D7), publishers (Homebrew/AUR/Docker) deferred to M5, `kubectl` brew dep removed (D2). `goreleaser check` + `--snapshot` verified (D27) — done 2026-07-18 (D2, D7, D27)
- [x] **M0-09** README references the vault + decision log: rewrite-in-progress banner atop `README.md` linking `vault/`, goals, milestones, board, `decisions.md`, journal, and `CLAUDE.md`. Closes the last open M0 exit criterion; full README rewrite deferred to M5. — done 2026-07-18
- [x] **M0-05** Test harness: teatest (tui) smoke test — done 2026-07-18 (D26)
- [x] **M0-08** Sweep remaining legacy files M0-07 missed: `Dockerfile`, `get.sh`, `ci/aur/` (incl. `id_rsa.enc`), `ci/terminalizer/`, empty `ci/`; `.goreleaser.yml` de-referenced from `ci/aur/` (D25) — done 2026-07-18 (D25)
- [x] **M0-04** GitHub Actions CI: `make check` (build/test/vet/golangci-lint) on Linux+macOS matrix (D24) — done 2026-07-18 (D24)
- [x] **M0-02** Toolchain: bump to Go 1.23 (go.mod directive), wire `cmd/kubecom` onto cobra v1.10.2; `ioutil` already gone via M0-07 (D23) — done 2026-07-18 (D23)
- [x] **PROC-04** Cloud runs set repo-local git identity (maintainer) in routine bootstrap + skill step 0 — done 2026-07-18
- [x] **PROC-03** Model split: routine session on Sonnet (orchestration), leg subagents pinned to Opus in `/do-rewrite-run`; cron corrected to UTC — done 2026-07-18
- [x] **M0-07** Delete legacy trees from `v1`: `app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`, Windows sources, Travis/snap CI; prune `go.mod`; drop `.golangci.yml` path excludes. Absorbs M0-03. (D14, D22) — done 2026-07-18 (D14, D22)
- [x] **PROC-02** Scheduled runs self-prime: checkout `v1` + lint tooling in step 0; legs read the skill by file path (cloud clones start on `master`) — done 2026-07-18
- [x] **PROC-01** `/do-rewrite-run` orchestrator skill: fresh subagent per leg, sequential, 4-leg/90-min budgets — done 2026-07-18 (D21)
- [x] **REVIEW-01** Maintainer setup review applied: legacy deletion planned, journal split to per-entry files, claim-push, `make check` + CI-early, fakes-default tests, Bubble Tea v2, XDG config path — done 2026-07-18 (D14, D20)
- [x] **M0-03** Single `kubecom` binary — done 2026-07-18 (D14)
- [x] **M0-01** Scaffold new module layout — done 2026-07-18 (D12, D13)
- [x] **BOOT-01** Bootstrap vault, goals, milestones, task board on `v1` — done 2026-07-18
