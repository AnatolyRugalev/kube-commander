# M4 — New Capabilities

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 4

_Scope expanded into ordered, leg-sized Backlog slices **M4-01 … M4-12** on the
[board](../tasks/board.md) (M4-PLAN, D155). Two of the six scope bullets below were
already closed before M4 opened — cluster search (pulled forward by feedback as the
SEARCH line) and sort by column (landed in M2) — so the slices cover the context
switcher (M4-01…05, the hard part: a switch is a teardown, not a pointer swap),
column-aware coloring (M4-06), owner→children drill-down (M4-07/08), metrics
(M4-09/10) and themes (M4-11/12). The switcher, coloring, drill-down and metrics lines
are done, and the themes line is one slice from finished: M4-11 landed the built-in
palettes, M4-12a made `theme:` in `config.yaml` take effect and M4-12b-1 the live
restyle a runtime pick needs, leaving M4-12b-2 (the picker and the write-back). Per-leg
history: `vault/journal/`._

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
- **Metrics** (CPU/mem) via `metrics.k8s.io` when the API is available; hidden otherwise
  — **done**: the primitive (M4-09/D167) and the polled table overlay that consumes it
  (M4-10/D168) both landed, so this bullet is closed.
- **Theme selection**; ship a couple of solid built-ins (port monokai/solarized) — the
  built-ins and the registry over them landed as M4-11/D169 (`monokai`,
  `solarized-dark`), the `theme:` config field that selects one at launch as
  M4-12a/D170, and the live restyle a runtime pick needs — `SetStyles` on every
  component behind one `applyStyles` fan-out — as M4-12b-1/D171; the picker over it
  and the write-back are M4-12b-2, the bullet's remaining half.
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
- [x] Metrics columns appear only when metrics-server is present; absence is silent.
      (met since M4-10/D168, on M4-09/D167's primitive: browsing a measured kind arms a
      10 s poll of `Clients.Metrics`, scoped to the same kind and namespace as the browse
      watch, and the table grows CPU/MEMORY columns joined onto the watched rows by
      namespace/name — millicores and mebibytes, sorted on the raw sample rather than the
      formatted cell. Availability is the discovery lookup `kube.MetricsFor` makes with no
      probe request, so metrics-server absent and metrics-server present-but-down are the
      same silent answer (#87). Evidence: `internal/tui/metrics_test.go` —
      `TestMetricsColumnsAppearForMeasuredKind` (the columns and the joined samples),
      `TestMetricsSilentWithoutMetricsServer` / `TestMetricsSilentForUnmeasuredKind` (no
      request, no columns, nothing said — the "absence is silent" half),
      `TestMetricsPollScopedToWatchNamespace` / `…ToChildScopeNamespace`,
      `TestMetricsRefreshErrorKeepsPreviousSamples` (a 503 does not blank the columns and
      is never toasted), `TestMetricsStaleResultDropped` / `…StoppedOnKindChange` /
      `…StoppedByClusterTeardown`; and `internal/tui/components/table/usage_test.go` for
      the overlay itself — `TestUsageSurvivesWatchDelta` (samples outlive a RESET),
      `TestUsageBlankWithoutSample` (never `0m`), `TestUsageEmptyMapKeepsColumns`,
      `TestUsageSortsNumerically`. Ticked on hermetic evidence: what no fake shows is a
      real metrics-server's numbers, and reading those is a taste question the next
      dogfood pass answers for free.)
- [ ] At least two themes selectable and persisted. (Three built-in palettes exist
      behind one registry since M4-11/D169 — `default`, `monokai`, `solarized-dark`,
      each complete and rendering distinctly — and since M4-12a/D170 one of them is
      *selectable*: `theme:` in `config.yaml` is resolved by the launcher through
      `styles.ByName` and built into every component, with an unknown name degrading
      to the default plus a single startup notice (`TestWithThemeReachesTheComponents`
      in `internal/tui/theme_test.go` is the load-bearing one — a themed shell renders
      differently while its glyphs stay identical; `TestResolveTheme*` in
      `cmd/kubecom/run_test.go` cover the resolution and its degrade). Since M4-12b-1/D171
      a theme can also be swapped on an *already-built* shell — `Model.applyStyles` fans
      a `styles.Styles` out to every component, guarded by
      `TestApplyStylesMatchesLaunchTimeTheme` (a restyled shell must render byte-identically
      to one built with that theme, per surface, so a component left behind is a failure
      rather than a shrug). Still unticked because the criterion claims *persisted*, which
      means chosen from inside kubecom and written back: the picker and `Config.SaveFile`
      are M4-12b-2. A hand-edited config file is configuration, not persistence.)
- [x] Cluster search returns matching objects across kinds and drills into the selected hit. (met since SEARCH-02b/D141 — `ctrl+s`, streamed cross-kind hits, `enter` switches the browse table to the hit; scope is now widenable on both axes, `search.allKinds`/D149 and `search.allNamespaces`/D150. Matching gained a server-side label selector (`-l app=web`, SEARCH-04c-1/D151), score ranking (SEARCH-04c-2a/D152) and a subsequence fallback ranked below it (SEARCH-04c-2b/D153), which closes the SEARCH line. Ticked here rather than reopening M4: the capability was pulled forward by feedback while M3 is the active milestone.)

## Depends on
M2 browse shell + M1 discovery (context switch = rebuild client + discovery).
