# M4 — New Capabilities

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 4

_Scope expanded into ordered, leg-sized Backlog slices **M4-01 … M4-12** on the
[board](../tasks/board.md) (M4-PLAN, D155). Two of the six scope bullets below were
already closed before M4 opened — cluster search (pulled forward by feedback as the
SEARCH line) and sort by column (landed in M2) — so the slices cover the context
switcher (M4-01…05, the hard part: a switch is a teardown, not a pointer swap),
column-aware coloring (M4-06), owner→children drill-down (M4-07/08), metrics
(M4-09/10) and themes (M4-11/12). The switcher, coloring and drill-down lines are done;
the metrics and theme lines remain. Per-leg history: `vault/journal/`._

## Goal

The capabilities the original lacked, now natural on the new architecture.

## Scope

- **Context/cluster switcher** in-UI + current context in the top bar (**#80**, old #79).
- **Sort by column** (**#85**) — **done**: sorting landed early in M2 (M2-13a/13b) and
  column-aware coloring (pod phase, restarts, readiness) landed as M4-06/D164, so this
  bullet is closed.
- **Owner → children drill-down** (Deployment → Pods, Node → Pods, etc.) — **done**:
  the scope primitive (M4-07/D165) and the `res.children` gesture that consumes it
  (M4-08/D166) both landed, so this bullet is closed.
- **Metrics** (CPU/mem) via `metrics.k8s.io` when the API is available; hidden otherwise.
- **Theme selection**; ship a couple of solid built-ins (port monokai/solarized).
- **Cluster search** — cross-object query across kinds (Kind · ns · name), one-shot +
  curated-scope by default, drill into a hit (feedback-driven, D131; kube primitive
  `kube.Search` landed as SEARCH-01, TUI slices SEARCH-02…04 on the board).

## Exit criteria

- [ ] Switch context without restarting; watches and menu rebind to the new cluster.
      (Mechanism complete and reachable since M4-04b/D158 — `C` (`ctx.switch`) opens the
      picker, the pick connects then resets → swaps → rediscovers (M4-03/04a, D156/D157) —
      and complete on *state* too since M4-05/D163: the switch lands on the new context's
      remembered namespace, its menu file and its state file, not the previous one's.
      Left **unticked** on purpose: the claim is about a live rebind against a second real
      cluster, which no fake can show, so it waits on the dogfood human-task
      `2026-07-29-context-switch-live-dogfood.md` rather than being ticked against
      hermetic tests (D79) — the M3 Edit precedent.)
- [x] Any column sortable; sort indicator visible; stable under live updates. (met since M2-13a/13b/D94/D98 — `table.SortBy`/`ClearSort` sort the *displayed* view over the authoritative watch-ordered set, so deltas keep flowing and re-sort in place: `TestSortSurvivesWatchDelta`, `TestSortPreservesSelectionByUID` (selection follows its object by UID, not its row), `TestClearSortRestoresWatchOrder`, `TestSetTableResetsSort`; the header arrow and its column alignment are `TestHeaderShowsSortIndicator`/`TestSortIndicatorKeepsColumnsAligned`; the `sort.column`/`sort.clear` cycle through the real key path is `TestSortCycleAdvancesColumnsAndClears`/`TestClearSortKeyRestoresOrder`. Ticked here rather than reopening M2: #85 was scheduled in M4 but implemented early, in the milestone that owns the table. Column-aware coloring, the other half of that scope bullet, landed separately as M4-06/D164 — a pure classifier keyed off the server-side column name, `TestClassifyCell` + the render tests in `internal/tui/components/table/color_test.go` — which closes the bullet, though it was never what this criterion asked for.)
- [x] Drill-down navigates from an owner to its pods and back. (met since M4-08/D166 —
      `P` (`res.children`, also "Show pods" in the actions menu, gated on
      `kube.HasChildren`) resolves the selected owner into the `ChildScope` M4-07/D165
      returns and re-points the browse table at the child kind under that scope's
      namespace *and* `ListOptions`, so the child table is a live filtered watch that
      sorts, colors and takes every row action like any other; `esc` returns to the owner
      with the row re-selected. Evidence in `internal/tui/children_test.go`:
      `TestChildrenDrillDownScopesTheWatch` (the new watch is the child kind, in the
      scope's namespace, with the scope's selector), `TestChildScopeLabelUsesField
      SelectorForNode` (the cluster-wide Node path), `TestChildrenBackReturnsToOwner` /
      `TestChildrenBackIsOneLevel` (the "and back" half, and its place in the esc chain),
      `TestChildScopeReappliedOnWatchRestart` (the scope survives a restart rather than
      widening), `TestChildrenRefusalLeavesTableUntouched` (a refusal degrades in place)
      and `TestChildrenScopeNamedInStatusBar` (the table says it is scoped). Ticked on
      hermetic evidence, unlike the context-switch criterion above: what a fake cannot
      show here is only a real apiserver honouring a label/field selector, which is
      client-go's contract rather than kubecom's, and the kube half is fake-client
      covered (D66).)
- [ ] Metrics columns appear only when metrics-server is present; absence is silent.
- [ ] At least two themes selectable and persisted.
- [x] Cluster search returns matching objects across kinds and drills into the selected hit. (met since SEARCH-02b/D141 — `ctrl+s`, streamed cross-kind hits, `enter` switches the browse table to the hit; scope is now widenable on both axes, `search.allKinds`/D149 and `search.allNamespaces`/D150. Matching gained a server-side label selector (`-l app=web`, SEARCH-04c-1/D151), score ranking (SEARCH-04c-2a/D152) and a subsequence fallback ranked below it (SEARCH-04c-2b/D153), which closes the SEARCH line. Ticked here rather than reopening M4: the capability was pulled forward by feedback while M3 is the active milestone.)

## Depends on
M2 browse shell + M1 discovery (context switch = rebuild client + discovery).
