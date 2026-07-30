# Top-Level Goals

## Vision

Rewrite the 2020 kube-commander into **kubecom**: a fast, approachable,
keyboard-driven terminal UI for observing and operating Kubernetes clusters.
Keep the original idea — *"kubernetes-dashboard in your terminal"* — while
replacing the aging foundation with a modern Go TUI stack and closing the
long-standing issues.

## Primary goal

Ship a Kubernetes TUI on **Bubble Tea + client-go** that reaches feature parity
with the original, eliminates its data-race / focus / redraw bug class by
construction, removes the hard `kubectl` binary dependency, and adds the
high-value capabilities the original lacked.

## Definition of Done (v1)

_Audited item by item against named evidence by **M5-01** (2026-07-30, D174): **6 of 13
ticked**, and every unticked box names the one thing that closes it. A box is ticked only
when its claim is decidable from the code and its tests, or has been confirmed by a human
against a real cluster — never to make the list read as finished (D79/D174). Four boxes wait
on an open dogfood or bug; three wait on work that has not happened yet (the migration
note, and the two release acts). **M5-01b** (2026-07-30, D178) amended the wording of the
logs/describe/YAML bullet — the one amendment made to a claim in this list, made on the
maintainer's own recorded feedback and not on the agent's reading of the code; the original
text is preserved in that bullet's annotation._

- [x] Two-pane browse UX (resource menu + live-watched table) at parity with the original.
      (M2 exit criteria 1–2: menu drill-in → `kube.Watch` deltas pumped into the table —
      `TestSelectResourceStartsWatch`, `TestWatchClosedStopsChain`, `TestProgramFilterFlow`
      end-to-end through the real program — and live browse + drill-in was human-confirmed
      against a real cluster on 2026-07-24. Two differences from the 2020 UX are recorded
      decisions, not gaps: menu customization is the per-context `menus/<context>.yaml` file
      instead of in-TUI add/hide/reorder (D83/D89), and the menu pane is optional with a
      `:` resource palette beside it (D96/D100).)
- [ ] Resource listing works generically for **any** resource incl. CRDs (discovery-driven, kubectl-identical columns).
      (Mechanism complete: server-side Table printing for any GVR, CRD
      `additionalPrinterColumns` included (`internal/kube/table.go`, D33); discovery folds
      CRDs into the menu (`TestDiscoveryReadyReconcilesMenu`, with a CRD); CRD group
      list/watch param encoding fixed (D103); kinds without `watch` degrade to list-only
      polling (D104). **Unticked on an open bug, not on missing evidence** — CRD-01: a real
      `ExternalSecret` errors instead of listing. A box reading "any resource incl. CRDs"
      cannot be ticked while a CRD is reported broken, whatever the tests say. Closes with
      CRD-01, itself blocked on the human task for the error text.)
- [ ] In-TUI logs and describe viewers; an object's YAML round-trips through your `$EDITOR`
      (no external pager required).
      (**Wording amended by M5-01b, 2026-07-30, D178.** It read "In-TUI logs, describe, and
      YAML viewers (no external pager required)" until then. M5-01's audit found the YAML
      third had stopped describing kubecom — D135/D136/M3-15c retired the standalone
      read-only YAML viewer and made `e` open the object's YAML in `$EDITOR` — and filed the
      call as a maintainer decision (D174 pt 3). **The maintainer had already made it**, in
      their own words, in the feedback that caused D135: "Having a separate read-only YAML
      viewer (`y`) and a separate edit (`e`) action is redundant … prefer suspending to the
      user's real `$EDITOR` … that IS the 'proper editor' for YAML"
      (`vault/feedback/2026-07-24-unify-yaml-view-and-edit.md`, deleted per D69 when
      addressed, readable at `git show 7897d1f -- <that path>`). So the promise changed
      shape by the maintainer's choice, and the bullet now says so rather than the box
      staying open on a question that was answered before it was asked. D135 pt 1 stands: no
      leg re-adds a read-only YAML viewer.
      Logs and describe are met and then some — a dedicated full-screen logs view with a
      live grep, wrap, sideways scroll, timestamps and the previous-instance toggle
      (LOGS-01…04c, M5-01a, D144–D148, D177) and
      `kubectl describe`-identical output in-process (M3-04, `internal/kube/describe.go`).
      Nothing here requires a pager: the editor is the user's own, invoked once and returned
      from, not a pager kubecom shells out to for reading.
      **Still unticked — but on missing evidence now, not on an open question**: the YAML
      third rides the same live `$EDITOR` suspend that leaves the Exec/Edit box below
      unticked, so it closes with the same human task,
      `2026-07-24-edit-live-cluster-dogfood`.)
- [x] Core actions in-process: delete, scale, rollout restart, cordon/drain, port-forward (background), view secrets.
      (M3 exit criteria 2–4, all ticked, all client-go: delete (D115), scale +
      rollout-restart (D117, kubectl's own `restartedAt` annotation so the two tools are
      interchangeable), cordon/uncordon (D120), drain with streamed eviction progress
      (D121), CronJob suspend/resume (D120), port-forwards that run in the background, are
      listed in a panel and stop cleanly on exit (D122/D123, `forwards.panel`/`stopAll`),
      and secrets with explicit reveal + per-entry clipboard copy (D113/D114, #89).)
- [ ] Exec shell + `$EDITOR` edit (the only sanctioned TUI-suspending actions).
      (Exec is done and human-confirmed against a real cluster in a real terminal
      (2026-07-24, HT-exec-dogfood): in-process SPDY with a `kubectl exec` parity fallback
      when the binary is present (D125–D128). Edit is built and hermetically covered —
      temp-file round-trip, no-change detection, identity/conflict guards that refuse rather
      than clobber (M3-15a/15b, D129/D135) — but its **live** `$EDITOR` suspend has never
      been driven by a human, so the M3 Edit exit criterion is deliberately unticked and so
      is this. Closes with the human task `2026-07-24-edit-live-cluster-dogfood`.)
- [ ] Context/cluster switcher; namespace switcher; filter; sort by column.
      (Three of four met: namespace picker (M2-08c) that remembers per-context scope
      (D163), table filter with `n`/`N` search (D80), and any column sortable, stable under
      live watch deltas, selection following its object by UID (M2-13a/13b, D94/D98, #85).
      The context switcher is complete as a mechanism — connect-then-teardown, watch and
      menu rebind, per-context namespace/menu/state (M4-03…05, D156/D157/D163) — but the
      claim is a *live* rebind against a second real cluster, which no fake can show, so
      the M4 criterion and this box both wait on
      `2026-07-29-context-switch-live-dogfood`.)
- [x] **Vim-first navigation** (`hjkl`, `gg`/`G`, `/`, `n`/`N`) with arrows/classic keys as an equivalent fallback.
      (M2 exit criterion 5: `defaultBindings` pairs every nav action with a non-vim
      fallback — `k`/`up`, `j`/`down`, `h`/`left`, `l`/`right`, `gg`/`home`, `G`/`end`,
      `ctrl+d`/`pgdn`, `ctrl+u`/`pgup` — and resolution is central, so both families reach
      every list and table identically (`TestDefaultKeymapValid`, `TestDefaultResolution`,
      `TestMenuPagingKeysReachTheMenu`). `/` filters and `n`/`N` walk the matches
      (`TestFilterOpensAndNarrows`, `TestSearchWrapsThroughMatches`), and the `?` overlay
      shows *both* keys per action (`TestHelpMapFullHelp`).)
- [x] **Fully configurable keybindings** — every action rebindable via config; zero hard-coded keys in view code.
      (M2 exit criterion 6: `Config.Keymap()` = `DefaultKeymap().Merge(overrides)`, resolved
      before the alt-screen so a bad keymap reports and never launches
      (`TestKeymapResolvesOverride`, `TestKeymapUnknownActionErrors`,
      `TestKeymapBadTokenErrors`, `TestKeysOverrideAndWarning`). Every raw key token in the
      tree lives under `internal/tui/keymap`; components take a `keymap.Action`, and the
      only `tea.KeyPressMsg` consumers are the text-entry surfaces where the key *is* the
      text (D11). All 16 action namespaces are covered by `TestBindingsCoversRegistry`, and
      `docs/keybindings.md` is generated from the registry with `make check` failing on
      drift (M2-01e/D51).)
- [x] **Cold start is responsive** — UI renders immediately; discovery is async and cached.
      (D8, and it is structural rather than tuned: `menu.New` opens on a static seed set
      (`internal/kube/seed.go`, `seedItems()`) so the two panes and the welcome page paint
      on the first `WindowSizeMsg` with no round-trip (`TestViewRendersWhenSized`);
      discovery is started from `Init` as a channel-fed pump and merged in place when it
      arrives (`TestInitStartsDiscovery`, `TestReconcilePreservesSelection`), and a total
      discovery failure leaves the seed menu navigable (`TestDiscoveryTotalFailureKeepsSeed`
      — degrade, don't blank). Cached on disk per cluster host with kubectl's own 6 h TTL
      (`internal/kube/cache.go`, `TestComputeDiscoverCacheDirPerHost`,
      `TestNewCachedDiscoveryImplementsInterface`, `TestClientsInvalidate`).)
- [x] No `kubectl` binary required for anything except the exec fallback.
      (D2, and checkable by grep rather than by claim: the only `os/exec` uses in the whole
      tree are `internal/tui/edit.go` (the `$EDITOR` the next-but-one bullet sanctions) and
      `internal/tui/exec.go`'s *optional* `kubectl exec` parity path, which is used only
      when the binary is on PATH and otherwise falls back to in-process SPDY
      (`TestKubectlExecProcAbsent`, `TestExecRoutesToKubectlWhenPresent`, D128). Every other
      capability the 2020 build shelled out for is client-go now — logs, describe, YAML,
      exec, port-forward, apply, drain, secrets (`internal/kube/`). Closes #68.)
- [ ] Plain-YAML config with one-shot migration from the old `~/.kubecom.yaml`.
      (The config half is met: plain YAML via `sigs.k8s.io/yaml`, config/state/menus split
      across XDG dirs (D20/D83/D91). The migration half is built and one-shot (M2-12a/12b,
      D92/D93) and degrades rather than blocks on a malformed legacy file — but its report
      **states something false**: it still tells the user themes were dropped because v1 has
      no runtime theming, which stopped being true at M4-11/12 (three built-in palettes, a
      `theme:` field and a picker). Unticked until M5-04 fixes the note and M5-05 verifies
      the path against a genuinely legacy file; this is the smallest gap in the list.)
- [ ] Linux + macOS release artifacts via goreleaser + GitHub Actions; tests green.
      (Tests green is continuous — `make check` (build + test + vet + lint) gates every leg
      and CI runs it (D17). The artifacts half has **not happened**: `.goreleaser.yml`
      builds linux/darwin × amd64/arm64 but sets only `Version` in its ldflags (M5-02), and
      `.github/workflows/` holds only `ci.yml`, so nothing has ever run it (M5-03). Ticked
      when a real tag has produced real artifacts — a human act by D173, so M5-10.)
- [ ] Every open GH issue in scope is resolved or explicitly deferred with a reason.
      (10 of the 11 issues in the REWRITE_PLAN table are resolved in code with named
      evidence: #68 (in-process client-go), #76 (discovery + dynamic Table path), #87
      (fault-isolating discovery — one bad group cannot break the load,
      `TestDiscoverResourcesGroupFaultIsolation`), #86 (no panics, degrade), #88 (new
      toolchain), #84 (logs for pod-owning kinds), #83 (workload actions), #89 (secret
      viewer), #80 (context switcher — mechanism), #85 (column sort). **#28** (CD to
      distributors) is the outstanding one: M5-06/07/08. Also unticked because the tracker
      itself was not checked — no `gh` in the sandbox — so whether these issues are
      *closed* is unknown here; closing them is part of M5-10's pre-flight.)

## Non-goals

- **Native Windows support** — dropped. Windows users run kubecom under **WSL2**.
- **Cloning k9s.** k9s is feature-dense but its UX is deliberately *not* our
  model; kubecom stays simpler and more approachable.
- Cluster mutation beyond the curated action set (no arbitrary `apply`, no manifests authoring).
- Multi-cluster dashboards / server mode. Single-context, local, zero-deploy.

## Principles

1. **No shared mutable UI state.** Concurrency flows through Bubble Tea messages,
   not mutexes. This is the whole reason for the framework choice — protect it.
2. **In-process first.** Reach for client-go before shelling out. Shell out only
   where genuine interactivity requires it (exec, editor).
3. **Degrade, don't crash.** A missing API group, RBAC denial, or bad namespace
   degrades one feature; it never panics or blocks the UI.
4. **Fast cold start.** Never block first paint on discovery or network round-trips.
5. **Discoverable process.** Knowledge and state live in the vault, not in an
   agent's head or a chat log.
6. **Vim-first, never vim-only.** `hjkl` and friends are the primary path; arrows
   and classic keys are always an equivalent fallback. See
   [`knowledge/keybindings.md`](knowledge/keybindings.md).
7. **Zero hard-coded keys.** All input flows through a configurable action
   registry; no view matches a raw key. Every binding is rebindable via config.
8. **Keep it runnable; dogfood it.** Once the binary launches (M2-RUN), it stays
   launchable every leg. A human periodically installs `kubecom` and runs it
   against a real cluster; each leg must incrementally improve — never regress —
   that real-cluster experience, and keep the README's install/usage current
   (D68). Fake-tested parts are not "done" until they work in the running binary.

See [`REWRITE_PLAN.md`](REWRITE_PLAN.md) for the full architecture and
rationale, and [`knowledge/decisions.md`](knowledge/decisions.md) for the locked
decisions.
