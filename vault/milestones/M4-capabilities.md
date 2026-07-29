# M4 — New Capabilities

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 4

_Scope expanded into ordered, leg-sized Backlog slices **M4-01 … M4-12** on the
[board](../tasks/board.md) (M4-PLAN, D155). Two of the six scope bullets below were
already closed before M4 opened — cluster search (pulled forward by feedback as the
SEARCH line) and sort by column (landed in M2) — so the slices cover the context
switcher (M4-01…05, the hard part: a switch is a teardown, not a pointer swap),
column-aware coloring (M4-06), owner→children drill-down (M4-07/08), metrics
(M4-09/10) and themes (M4-11/12). Per-leg history: `vault/journal/`._

## Goal

The capabilities the original lacked, now natural on the new architecture.

## Scope

- **Context/cluster switcher** in-UI + current context in the top bar (**#80**, old #79).
- **Sort by column** (**#85**) — **done early in M2** (M2-13a/13b); column-aware coloring
  (pod phase, restarts, readiness) is what remains of this bullet (M4-06).
- **Owner → children drill-down** (Deployment → Pods, Node → Pods, etc.).
- **Metrics** (CPU/mem) via `metrics.k8s.io` when the API is available; hidden otherwise.
- **Theme selection**; ship a couple of solid built-ins (port monokai/solarized).
- **Cluster search** — cross-object query across kinds (Kind · ns · name), one-shot +
  curated-scope by default, drill into a hit (feedback-driven, D131; kube primitive
  `kube.Search` landed as SEARCH-01, TUI slices SEARCH-02…04 on the board).

## Exit criteria

- [ ] Switch context without restarting; watches and menu rebind to the new cluster.
- [x] Any column sortable; sort indicator visible; stable under live updates. (met since M2-13a/13b/D94/D98 — `table.SortBy`/`ClearSort` sort the *displayed* view over the authoritative watch-ordered set, so deltas keep flowing and re-sort in place: `TestSortSurvivesWatchDelta`, `TestSortPreservesSelectionByUID` (selection follows its object by UID, not its row), `TestClearSortRestoresWatchOrder`, `TestSetTableResetsSort`; the header arrow and its column alignment are `TestHeaderShowsSortIndicator`/`TestSortIndicatorKeepsColumnsAligned`; the `sort.column`/`sort.clear` cycle through the real key path is `TestSortCycleAdvancesColumnsAndClears`/`TestClearSortKeyRestoresOrder`. Ticked here rather than reopening M2: #85 was scheduled in M4 but implemented early, in the milestone that owns the table. **Column-aware coloring, the other half of that scope bullet, is not done** — it is M4-06, and it is not what this criterion asks for.)
- [ ] Drill-down navigates from an owner to its pods and back.
- [ ] Metrics columns appear only when metrics-server is present; absence is silent.
- [ ] At least two themes selectable and persisted.
- [x] Cluster search returns matching objects across kinds and drills into the selected hit. (met since SEARCH-02b/D141 — `ctrl+s`, streamed cross-kind hits, `enter` switches the browse table to the hit; scope is now widenable on both axes, `search.allKinds`/D149 and `search.allNamespaces`/D150. Matching gained a server-side label selector (`-l app=web`, SEARCH-04c-1/D151), score ranking (SEARCH-04c-2a/D152) and a subsequence fallback ranked below it (SEARCH-04c-2b/D153), which closes the SEARCH line. Ticked here rather than reopening M4: the capability was pulled forward by feedback while M3 is the active milestone.)

## Depends on
M2 browse shell + M1 discovery (context switch = rebuild client + discovery).
