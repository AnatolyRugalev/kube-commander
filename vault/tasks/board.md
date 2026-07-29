# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-29 — M4-07 landed, so `kube.Children` hands the TUI a live child *scope* rather than a fetched list (D165); its consumer M4-08 is next. Five human-tasks open: one **blocking** (the CRD error text, blocking CRD-01) and four advisory dogfoods. Per-leg history: `vault/journal/`._

## In Progress

_(none)_

## Blocked

- [ ] **CRD-01** Opening the `ExternalSecret` CRD errors out instead of listing it
      status: blocked | owner: — | added: 2026-07-29
      notes: Blocked on human task `2026-07-29-external-secrets-crd-error-log` (the error text). See the DIAG line in the Backlog.

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
_(none — M2 is **done** (2026-07-29): every exit criterion in
[`../milestones/M2-core-tui.md`](../milestones/M2-core-tui.md) is ticked against named
evidence (M2-EXIT/D154), and M2-15 cleared the one enhancement left behind.)_

M2-01 (action registry + configurable keymap, D10/D11) is **complete** (01a keymap
core / 01b sequences / 01c config wiring / 01d help overlay / 01e generated doc).
The rest of M2 is the **app shell** — expanded here into ordered, leg-sized slices
(D52). Take them top-down; each notes what it depends on. Package layout follows
[`../REWRITE_PLAN.md`](../REWRITE_PLAN.md): `internal/tui/{app.go,msg.go}`,
`internal/tui/styles`, `internal/tui/components/*`, `internal/tui/views/*`. Every
slice keeps **zero shared mutable UI state** (principle 1); goroutines only send
messages.

> **M2-RUN landed 2026-07-20** — the binary launches the TUI against a real cluster,
> so every leg since is verified against the running binary and must **incrementally
> improve the real-cluster experience** (D68).

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

### M3 — Actions & Viewers
M3 makes kubecom *operate*: in-TUI viewers + the curated action set, killing nearly
all kubectl shell-outs. The **kube layer already has every verb** — logs stream
(M1-07c/d), describe (M1-07b), YAML (M1-07a), delete/scale/rollout-restart/cordon/
drain/suspend (M1-06*), background port-forward (M1-08). So M3 is almost entirely
the **TUI surface**: reusable read-only viewers, wiring actions through the M2-10
confirm modal (D88), an actions surface off the reserved nav keys (D10), and the two
sanctioned suspend flows (exec, edit). Built bottom-up (D52 rhythm): the shared
viewer + the actions surface first, then each viewer/action as its own leg. Every
overlay composites over the base browse view (D95); zero shared mutable UI state
(principle 1); no raw-key matching — actions are named keymap entries (D11). Ordering
is a default, not a contract — re-split any slice that proves > ~300 lines.

### Cluster search (SEARCH — feedback-driven, D131)
Cross-object cluster search (feedback `2026-07-24-cluster-search-multi-resource`): type a
query → matching objects **across kinds** (Kind · namespace · name), drill into the hit.
An M4-class capability pulled forward by feedback. One-shot, concurrent, curated-scope by
default — never "watch everything" (D131). Built bottom-up (D52): kube primitive first
(done), then the TUI search mini-app, then streaming/progress, then scope-widening/fuzzy.

SEARCH-02 was split D52-style into the component (**SEARCH-02a**) and the app wiring
(**SEARCH-02b**), mirroring LOGS-01 → LOGS-02. Both are done: cluster search is live on
`ctrl+s` (D141) and the remaining slices refine it.

SEARCH-03 was split D52-style into the kube-layer signal (**SEARCH-03a**) and the view
surfacing (**SEARCH-03b**); both are done, so progress and the cap are on screen and only
the scope widen (SEARCH-04) is left in this line.

SEARCH-04 was three unrelated surfaces behind one line item, so it was split on pickup
into **SEARCH-04a** (widen the *kind* scope), **SEARCH-04b** (widen the *namespace*
scope) and **SEARCH-04c** (richer matching). Both widens are now done, so scope is
complete: `search.allKinds` (`ctrl+a`) swaps the curated set for every discovered kind
over a fan-out `kube.Search` bounds to eight concurrent lists (D149), and
`search.allNamespaces` (`ctrl+w`) searches every namespace without touching the app's own
namespace scope (D150). They are independent flags, not a cycle, so all four scope
combinations are reachable.

SEARCH-04c was split on pickup, as its own notes predicted, into **SEARCH-04c-1** (label
selector) and **SEARCH-04c-2** (fuzzy matching): the selector is a `metav1.ListOptions`
field the server evaluates and needs no matcher at all, while fuzzy needs client-side
scoring *and* a ranking decision that the current arrival-ordered, streamed result list
does not have a place for. A **field** selector was considered with the label one and
deliberately left out: per-kind field support varies (`spec.nodeName` is a Pod thing), a
selector the kind does not support fails its List, and a failed kind is silent by design
(D131 pt 3) — so a field selector would quietly drop most of the scope. Raise it as its
own item if it is ever wanted. **SEARCH-04c-1 is done** — `-l app=web` in the query line is
a server-side label selector (D151).

SEARCH-04c-2 was split on pickup into **SEARCH-04c-2a** (rank the results) and
**SEARCH-04c-2b** (fuzzy matching), in that order, because doing them the other way round
is a regression: a fuzzy matcher over an arrival-ordered list buries the exact match the
reader wanted under scattered ones. Ranking first also settles the ordering question the
old note flagged — see D152: `kube.Search` keeps streaming in arrival order and the *view*
does the ranking, as a stable ordered insert with the cursor pinned to its row.

Both halves are done, so **the SEARCH line is closed**: the matcher falls back to
subsequence matching under the band the score reserves for it, and the emit-time hit cap
budgets scattered hits to a fraction of itself so fuzzy can never starve exact (D153).

### Logs dedicated view (LOGS — feedback-driven, D134)
Logs move off the shared read-only viewer (M3-01) into a **dedicated full-screen logs
mini-app** with real-time grep (feedback `2026-07-24-logs-dedicated-view-live-grep`):
type a `/`-filter that narrows the streamed lines **live while following** (à la
stern/k9s), full-screen for high throughput, clear `[following]`/`[paused]` + active-filter
indicator. Built bottom-up (D52): the component first (LOGS-01), then the app wiring that
retires the shared-viewer logs path (LOGS-02), then regex/highlight (LOGS-03), then the
nice-to-haves (LOGS-04). Keymap-driven (D11), message-only (principle 1).

LOGS-01 (component), LOGS-02 (wiring) and LOGS-03 (regex + highlighting) are done, so the
dedicated logs view is live on `res.logs`, the shared viewer no longer has a logs mode
(D144), and the grep matches by substring or regex with the hits highlighted (D145). Only
the nice-to-haves are left, split D52-style into **LOGS-04a** (wrap toggle + horizontal
scroll), **LOGS-04b** (timestamps) and **LOGS-04c** (jump-to-latest) — three unrelated
surfaces that were one line item. All three are done: long lines wrap on `logs.wrap` or
scroll sideways on `nav.left`/`nav.right` (D146); `nav.bottom` rejoins the stream rather
than just scrolling to it (D147); and `logs.timestamps` shows each line's server stamp as
a pure display toggle over stamps the stream already carries (D148). That closed the line
on **features**: the dedicated logs view is feature-complete for M3.

**LOGS-05 reopens it on cost.** Feedback `2026-07-29-logs-tail-and-perf` (high) names two
independent costs in the same view, so it triages into two slices rather than one leg:
opening a log **replayed the container's whole history** (`openLogs` set no `TailLines`,
so a pod up for a week streamed a week), and **every appended line re-renders the whole
buffer** (`logsview.Append` → `render` → `shown` joins all lines, so the cost of one line
grows with the number held — the exact quadratic the LOGS-02 throughput human-task
predicted). The first bounds what is fetched, the second bounds what a fetched line
costs; the feedback is explicit that the second must be fixed either way, since a
followed stream keeps growing the buffer long after the initial tail.

LOGS-05a is done: every logs open asks for the last 1000 lines and tails from there, and
the reconnect path is pinned not to re-tail on top of what the reader already has (D160).
**LOGS-05b is done too, so the LOGS line is closed again** — on cost this time. Both halves
landed: the rendered body is a cache an append extends rather than a buffer every append
re-joins, and the pump drains the log channel so a burst is one render instead of hundreds
(D162). The measured cost of the 1000-line open LOGS-05a introduced went from ~131 ms to
~2 ms (`BenchmarkStreamLines*`). The residual cost is the viewport's own re-measure of
every line it holds, which has no append API — hence the batching, and hence D162 pt 2 for
whoever builds the next streaming surface. The standing throughput dogfood human-task is
still the eyes-on half of this and is worth re-running now.

**LOGS-06** (feedback `2026-07-29-logs-init-containers`, normal) is done and separate: the
container picker only ever offered `spec.containers`, so an init container's logs — the
only thing there is to read when a pod is stuck in `Init:` — could not be reached at all.
`kube.PodContainers` now returns the init and ephemeral containers too, classified, each
consumer narrows the set by what it can act on (exec still skips init containers), and a
non-regular row is marked `name (init)` (D161).

### Diagnostics (DIAG — feedback-driven)
Raised by feedback `2026-07-29-external-secrets-crd-error` ("Need to find the actual
error"): opening the external-secrets `ExternalSecret` CRD errors out. The report carries
no error text — and it could not, because kubecom **had nowhere to put one**. Every
runtime failure funnels through `surfaceError` into a 5-second, width-clipped status-bar
toast and is then gone; the log file (`~/.cache/kubecom/kubecom.log`, documented in the
README since M2-RUN) held only launcher warnings. So the CRD fix has a prerequisite: make
the error obtainable. DIAG-01 does that, CRD-01 is the fix itself.

DIAG-01 is done: `surfaceError` — the shell's single error funnel — now logs before it
toasts, and discovery's deliberately-silent failures (total and per-group) log too, so the
file the README already documented finally holds the errors the user actually hit (D159).
CRD-01 is in **Blocked** above, waiting on the human task for the error text: the sandbox
has no cluster and no external-secrets CRDs, and the plausible causes (a conversion-webhook
failure the apiserver reports on LIST, a Table-conversion 406, an RBAC 403 on that group, a
decode edge case in a printer column) call for opposite fixes — one of which is "kubecom is
right, degrade more legibly". Guessing between them would be inventing a bug. Check the
sibling kinds (`SecretStore`, `PushSecret`) when it unblocks.

### M4 — New capabilities
M4 adds what the original lacked, now natural on the new architecture — expanded here
into ordered, leg-sized slices (M4-PLAN, D52/D155). Two of M4's six scope bullets are
already closed: **cluster search** (pulled forward by feedback as the SEARCH line, D131…
D153) and **sort by column** (#85, landed in M2 as M2-13a/13b), so what remains is the
context switcher, column coloring, owner→children drill-down, metrics and themes. Built
bottom-up like M2/M3: the kube-layer primitive first, then the TUI surface. Inherited
constraints hold — zero shared mutable UI state (principle 1), message pumps not mutexes
(D53), overlays composite over the browse body (D95), no raw-key matching (D10/D11),
degrade-don't-crash (principle 3). Ordering is a default, not a contract — re-split any
slice that proves > ~300 lines.

**The context switcher (#80) is the milestone's hard part** and takes five slices, because
switching clusters is a teardown, not a pointer swap (D155): the **21** cluster-bound
seams now sit in one swappable `tui.Cluster` (M4-02, and every new seam goes in it), but
every per-cluster async in flight (watch, discovery, log stream, search sweep, drain,
port-forwards) still belongs to the cluster being left and must be cancelled first.

M4-04 was split on pickup, as its own notes and the M4-03 journal both predicted, into
**M4-04a** (the switch itself) and **M4-04b** (the picker that triggers it) — the D52
bottom-up rhythm the whole switcher line has followed: the connect+swap path lands and is
tested before any gesture can reach it, exactly as M4-03's reset did.

**The switcher line (M4-01…05) is closed** as of M4-05/D163: what a switch rebinds is now
the cluster's client *and* everything keyed by the context (menu extras, remembered
namespace, state file). The M4 exit criterion stays unticked on purpose — it claims a live
rebind against a second real cluster, which is the standing dogfood human-task (D79).

- [ ] **M4-08** TUI: owner → children drill-down (`res.children`)
      status: todo | owner: — | added: 2026-07-29
      notes: Depends on M4-07 (**done** — `kube.Children` returns `ChildScope{Resource, Namespace, Options}`, `HasChildren` is the gate, D165). A registered action on an owner row switches the browse table to the child kind under that scope, with `ChildScope.Selector()` named in the status bar so it is obvious the table is filtered; `nav.back` returns to the owner. The browse model must carry **both** scope fields alongside the resource — a Node's children are cluster-wide (`Namespace: ""`), so reusing the app's own namespace would silently show one namespace's pods — and re-apply them on every watch restart (namespace change, reconnect). Ticks the drill-down exit criterion.
- [ ] **M4-09** Kube layer: **metrics** primitive over `metrics.k8s.io` (`internal/kube/metrics.go`)
      status: todo | owner: — | added: 2026-07-29
      notes: Pod + node CPU/memory via the **dynamic client** on `metrics.k8s.io/v1beta1` — no new module dependency (`k8s.io/metrics` is not in `go.mod`) and no kubectl (D2). Availability comes from the discovery result already in hand (the group is absent when metrics-server is not installed); absence is silent, never an error (principle 3) — the aggregated API being *present but down* is the common case and must degrade the same way.
- [ ] **M4-10** TUI: metrics columns when available
      status: todo | owner: — | added: 2026-07-29
      notes: Depends on M4-09. CPU/mem columns appended to the Pod/Node table, refreshed on a **slow ticker** — metrics are point-in-time samples and are not watchable, so this is an overlay joined onto the watched rows by object ref, never a second watch (D155 pt 3). No metrics group → the columns simply never appear and nothing is said. Ticks the metrics exit criterion.
- [ ] **M4-11** Built-in themes + registry (`internal/tui/styles`)
      status: todo | owner: — | added: 2026-07-29
      notes: Pure data. Two more `Theme` constructors (ported monokai + solarized, D6) beside `DefaultTheme`, plus a lookup/list (`Themes()`/`ByName`) for the picker and the config field. Nothing selects them yet.
- [ ] **M4-12** Theme selection + persistence
      status: todo | owner: — | added: 2026-07-29
      notes: Depends on M4-11. A `theme:` field in `config.yaml` applied at launch (unknown name → default + a startup toast, principle 3) and a picker action that writes the choice back through `Config.Save` (M2-11a/D89). **Split on pickup if needed:** a live restyle needs a `SetStyles` on every component that caches a `styles.Styles` at construction — if that half is large, land the config field first and the picker second. Ticks the themes exit criterion.

_Remaining M5 items to be expanded when that milestone opens. See the milestone file for scope._

## Done

- [x] **M4-07** Kube layer owner → children scope primitive — `kube.Children` returns a `ChildScope` (child `Resource` + namespace + `ListOptions`) instead of rows, so the TUI gets a live child table; `spec.selector` for workloads/Service, `spec.nodeName` for Node, `HasChildren` as the pure gate — done 2026-07-29 (D165)

- [x] **M4-06** Column-aware cell coloring in the table — a pure classifier keyed off the server-side column name (`STATUS`/`STATE`/`PHASE`, `READY`, `RESTARTS`) paints status cells with the theme's Success/Warn/Error roles; selection still wins on the cursor row — done 2026-07-29 (D164)

- [x] **M4-05** Per-context state follows the context switch — a `ContextStateLoader` seam re-resolves the new context's menu extras, last-used namespace and state-file persister, loaded alongside the connect and applied around the reset — done 2026-07-29 (D163)

- [x] **LOGS-05b** Logs view no longer pays per line for every line already held — a cached rendered body an append extends, plus a pump that drains the log channel into one batch — done 2026-07-29 (D162)

- [x] **LOGS-06** Init and ephemeral containers are offered by the logs container picker, marked by kind — `kube.PodContainers` returns the whole classified set and each purpose narrows it (exec still skips init) — done 2026-07-29 (D161)

- [x] **LOGS-05a** Logs open on the last 1000 lines instead of replaying from container boot — `defaultLogTail` on every `openLogs`, and a reconnect still resumes rather than re-tailing — done 2026-07-29 (D160)

- [x] **DIAG-01** Every surfaced error (and every discovery failure) is written to the log file — `WithLogger` seam, logged in the single `surfaceError` funnel — done 2026-07-29 (D159)

- [x] **M4-04b** Context switch action + picker — `C` (`ctx.switch`) opens the reused modal picker over a `ContextLister` seam, the pick routes into `switchContext`; the marker follows the shell's context, not the kubeconfig's — done 2026-07-29 (D158)
- [x] **M4-04a** Context switch machinery — `ClusterConnector` seam + `switchContext` connects off the update loop, then reset → swap → rediscover; a failed connect changes nothing — done 2026-07-29 (D157)
- [x] **M4-03** Cluster reset path — `resetCluster` + the single `stopClusterAsync` teardown inventory (quit shares it), discovery generation-guarded — done 2026-07-29 (D156)
- [x] **M4-02** One indirection for the 21 cluster-bound seams — `tui.Cluster` bundle (embedded, built by `NewCluster`/`clusterFor`), no behavior change — done 2026-07-29

- [x] **M4-01** Kubeconfig context-list primitive `kube.Contexts` — name/cluster/namespace/current, sorted, no network I/O — done 2026-07-29

- [x] **M4-PLAN** Expand M4 (new capabilities) into ordered, leg-sized Backlog slices M4-01…M4-12 — sort-by-column ticked as already met by M2-13a/13b, M4 set in-progress — done 2026-07-29 (D155)

- [x] **M2-15** Half-page/page nav in the resource menu — `ctrl+d`/`pgdn`/`ctrl+u`/`pgup` now page the left pane, stepping in display rows (headers counted) and landing on the nearest selectable item; M2's board section is empty and the milestone is done — done 2026-07-29

- [x] **M2-EXIT** Audit and close M2's exit criteria — all five unticked criteria ticked against named tests/code paths, M2 set feature-complete, M2-15 filed as the one remaining enhancement — done 2026-07-29 (D154)

- [x] **SEARCH-04c-2b** Fuzzy (subsequence) cluster-search matching — `kube.Search`'s name matcher falls back to a subsequence match when the contiguous pass finds nothing (`apisrv` finds `api-server`), scored in a band strictly below every substring hit and by tightness rather than position, so the noise lands at the bottom of the ranked list; the emit-time hit cap now budgets scattered hits to a fraction of itself, without cancelling the sweep or reporting `Capped`, so a fuzzy near-miss can never spend a slot an exact match in a slower kind still needs — done 2026-07-28 (D153)
- [x] **SEARCH-04c-2a** Ranked cluster-search results — `kube.Search` scores every match by where the query lands in the name (`SearchHit.Score`, contiguous matches in a band no scattered match can reach) and still streams in arrival order, while `searchview` holds its hits in score order and inserts each streamed hit at its rank with the cursor carried along with its row, so the best answer rises to the top without buffering the sweep; matching itself is unchanged — done 2026-07-28 (D152)
- [x] **SEARCH-04c-1** Label-selector matching for cluster search — a `-l <selector>` term in the query line is parsed into `kube.SearchQuery{Name, LabelSelector}` and evaluated by the apiserver on every kind's List (so it narrows the wire instead of costing client work), while the name substring stays client-side; an unparseable selector is reported under the query line and never sent — done 2026-07-28 (D151)
- [x] **SEARCH-04b** All-namespaces scope widen for cluster search — `search.allNamespaces` (`ctrl+w`) searches every namespace and re-runs the query, replacing the namespace the header names rather than adding a segment, independent of the kind widen and leaving the app's own namespace scope untouched; `kubecom keys` now sizes its columns from the widest id — done 2026-07-28 (D150)
- [x] **SEARCH-04a** All-kinds scope widen for cluster search — `search.allKinds` (`ctrl+a`) swaps the curated kind set for every discovered kind and re-runs the query; the header names the widened scope only while it is on, the widen resets on every fresh open, and `kube.Search` now lists at most 8 kinds at a time so the widen cannot flood the apiserver (D149) — done 2026-07-28 (D149)
- [x] **LOGS-04b** Timestamps toggle in the logs view — `logs.timestamps` (`t`) draws each line's server stamp; the stream always requests timestamps (free on a followed stream) and the view keeps them in a buffer parallel to the messages, so the toggle is a redraw not a restream and the grep still matches only the message (D148) — done 2026-07-28 (D148)
- [x] **LOGS-04c** Jump-to-latest in the logs view — `nav.bottom` (`G`) now re-arms following as well as scrolling, the inverse of "any upward scroll pauses"; incremental downward movement still does not (D147) — done 2026-07-28 (D147)
- [x] **LOGS-04a** Wrap toggle + horizontal scroll for long lines in the logs view — done 2026-07-28 (D146)
- [x] **LOGS-03** Regex grep mode + match highlighting in the logs view — `logs.regex` (`ctrl+r`, a no-text chord so it still fires while the grep field is open) re-reads the query as a case-insensitive regex, matched spans are painted with a new shared `styles.Match` role in either mode, an uncompilable pattern keeps narrowing by the last good one and the header says `invalid regex`, and the unfiltered render path stays match-work-free (plus one pre-existing double scan removed) — done 2026-07-25 (D145)
- [x] **LOGS-02** Wire `res.logs` to the dedicated logs view — retired the shared-viewer logs path (`streamLogsInto`/`syncLogViewerTitle`/`logFollow`/`logTitle`/`viewerKindLogs`/`isScrollUp`) for `internal/tui/logs.go`: logs stream full-screen into the LOGS-01 component with its live grep (`/` narrows while following), follow/pause and the grep live in the view not the shell, the container picker (M3-07a) + pod-owning resolution (M3-07b) still feed it on the same `viewerGen`, plus `HelpLogs`/`HelpLogsFilter` hint contexts; logs-throughput dogfood raised — done 2026-07-25 (D144)
- [x] **SEARCH-03b** Search progress + cap surfacing: the searchview header reports `searching N/M kinds…` (fed from `SearchKindDone`, silent on a failed kind) and `first 200 matches — narrow the query` (from `SearchDone{Capped}`), reset with the results they describe; plus a `keymap.HelpSearch` hint context so the bottom hint stops advertising the six text keys the always-open query field swallows — done 2026-07-25 (D143)
- [x] **SEARCH-03a** Search progress/completion signal in the kube layer: `kube.Search`'s channel item widened from `SearchHit` to a typed `SearchEvent` (`SearchMatch` · one `SearchKindDone` per requested kind, `Failed` only for a genuine List error · terminal `SearchDone{Capped}`), TUI pump adapted (`SearchEventMsg`) with the close still the single teardown point — no surfacing yet — done 2026-07-25 (D142)
- [x] **SEARCH-02b** Cluster search app wiring (`internal/tui/search.go`): `search.cluster` (`ctrl+s`) opens the SEARCH-02a view as the full-screen body, a `Searcher` seam (`WithSearcher`, nil → search-inert) runs `kube.Search` over `CommonSearchResources` of the menu's available kinds in the current namespace after a 250 ms debounce, hits stream in through a `searchGen`-tagged pump (cancel + drop on query change/close/quit), and `enter` switches browse to the hit's kind with the object selected once the watch's rows arrive (`table.SelectObject` pending selection) — done 2026-07-25 (D141)
- [x] **SEARCH-02a** Cluster-search view component (`internal/tui/components/searchview`): full-screen always-open query field over a streaming cross-kind result list (kind-aligned `Kind  ns/name` rows from `kube.SearchHit`), keymap-driven cursor that streamed hits never move, `SelectedMsg`/`ClosedMsg`/`QueryChangedMsg`, clear-then-close `nav.back`, and an empty list that always says which empty it is — done 2026-07-24 (D140)
- [x] **FB-pf-local-port** Port-forward local port (`internal/tui/ports.go`): every declared port now opens the picker (a lone one no longer auto-forwards, which dead-ended a local clash), where `enter` still forwards local = remote, `forwards.freeLocal` (`0`) forwards `:<remote>` on an OS-assigned local port, and `forwards.localPort` (`p`) opens a prompt seeded with the remote number (blank = free port); the bind hint stopped suggesting the invalid `:0` — done 2026-07-24 (D139)
- [x] **FB-pf-port-picker-b** Port-forward port picker — TUI wire (`internal/tui/ports.go`): a `PortLister` seam over `kube.PodPorts`/`ServicePorts` lists the target's declared ports off the update loop (`pfResolveGen`-guarded); one port forwards directly (local = remote), several open the reused modal picker (`80 → 8080 (http · app)` rows), and no lister / an empty list / a listing error all fall back to the free-text ports prompt — done 2026-07-24 (D138)
- [x] **FB-pf-port-picker-a** Port-forward port picker — kube-layer declared-ports primitive (`internal/kube/ports.go`): `Clients.PodPorts` (declared containerPorts, native sidecars included) + `Clients.ServicePorts` (service ports resolved to the pod-side targetPort, named targets looked up on the backing pod); TCP-only, de-duplicated, apimachinery-free `Port` — done 2026-07-24 (D137)
- [x] **M3-15c** Unify YAML view + edit — retired the standalone read-only YAML viewer (`openYAMLViewer`/`yamlLoadedMsg`/`viewerKindYAML`/`rowActionYAML`/`ActionYAML`/`res.yaml`); the View/Edit YAML action (`e`, gated `canGet`) is now the only YAML surface, `y` unbound; `YAMLGetter` seam kept; `docs/keybindings.md` regenerated — done 2026-07-24 (D135, D136)
- [x] **M3-15b** Edit — TUI wire (the suspend flow): `res.edit`/`e` → `GetYAML` (existing `YAMLGetter`) → temp file → `tea.Exec` `$EDITOR` → read back → `Editor` seam (`kube.Update`, M3-15a) only on change; no-change / editor-abort / apply-rejection degrade to a status-bar toast without mutating; `$EDITOR` resolved `KUBE_EDITOR`→`EDITOR`→`vi` (space-split for flags). First slice of the unify-yaml-view-and-edit feedback (D69/D135; feedback triaged into M3-15c + deleted); live `$EDITOR` suspend dogfood raised as a human-task, so the M3 Edit exit criterion stays unticked — done 2026-07-24 (D135)
- [x] **LOGS-01** Feedback (normal, `2026-07-24-logs-dedicated-view-live-grep`, first slice): dedicated full-screen logs-view component (`internal/tui/components/logsview`) — streaming append buffer + live case-insensitive substring filter that narrows the shown lines **while following** (reuses `app.filter`), follow/pause (reuses `logs.follow`) with auto-scroll on append + upward-scroll pauses, full-screen header (`[following]`/`[paused]` + query + matched/total), keymap-driven (D11)/message-only (principle 1)/`ClosedMsg`; feedback triaged into LOGS-01…04 and deleted — done 2026-07-24 (D134)
- [x] **FB-delete-key-d** Feedback (normal, `2026-07-24-delete-default-key-d`): shipped default delete binding is now `d` (vim `dd` muscle memory, was `x`); describe relocated off `d` to `D` (read-only, not a reserved nav chord); registry-driven/rebindable (D11), `docs/keybindings.md` regenerated, feedback deleted — done 2026-07-24 (D133)
- [x] **FB-confirm-yn-keys** Feedback (normal, `2026-07-24-confirm-modal-yn-keys`): confirm modal accepts `y` (confirm)/`n` (decline) plus enter/esc via registered, rebindable `confirm.accept`/`confirm.decline` resolved in a dedicated keymap **context** (no raw-key match, D11) — done 2026-07-24 (D132)
- [x] **SEARCH-01** Feedback (normal, `2026-07-24-cluster-search-multi-resource`, first slice): cluster-search **kube primitive** (`internal/kube/search.go`) — `Clients.Search`/`searchRows` fan out one-shot **concurrent** server-side `List`s over a caller-supplied `[]Resource`, match `Row.Object.Name` by case-insensitive substring, stream `SearchHit{Resource,ObjectRef}` on a channel; per-kind failure isolates (principle 3), hit **cap** + ctx cancel bound it, `CommonSearchResources` gives the curated default scope; feedback triaged into SEARCH-02…04 and deleted — done 2026-07-24 (D131)
- [x] **FB-pf-bind-toast** Feedback (high, `2026-07-24-port-forward-picker-and-local-port`, first slice): a port-forward local-listener bind failure now surfaces an actionable status-bar hint (naming the clashing local port(s) + `:0`/`:<remote>` free-local-port retry) instead of client-go's raw "unable to listen on any of the requested ports"; prompt hint surfaces the `:80=free local` syntax; parts 1/2 (port picker, editable/auto local port) triaged to FB-pf-port-picker/FB-pf-local-port — done 2026-07-24 (D130)
- [x] **HT-exec-dogfood** Closed the exec live-cluster dogfood human-task — maintainer confirmed the Exec-shell action works end-to-end against a real cluster in a real terminal (shell drops in, TUI restores cleanly); ticked the M3 exec exit criterion, deleted `vault/human-tasks/2026-07-24-exec-live-cluster-dogfood.md` — done 2026-07-24
- [x] **M3-15a** Edit — kube-layer apply/update primitive (`internal/kube/apply.go`): `Clients.Update` parses the edited `$EDITOR` bytes (YAML→JSON→unstructured, int64-safe) and PUT-updates the object through the generic dynamic client (built-ins + CRDs, no kubectl, D2); the buffer's `metadata.resourceVersion` gives optimistic concurrency (concurrent change → Conflict, not clobber), identity (name/namespace) guarded before any request — rename/empty/invalid/null rejected without mutation; no-change left to the caller (M3-15b) — done 2026-07-24 (D129)
- [x] **M3-14b-4** Exec — kubectl parity fallback: when `kubectl` is on PATH `execInto` suspends into `kubectl exec -i -t <pod> [-c ctr] -- /bin/sh` via `tea.ExecProcess` (pointed at the same cluster via `--kubeconfig`/`--context`/`-n`, new `WithKubeconfig` option wired in `run.go`), else the in-process SPDY path (14b-1) — kubectl owns its own raw PTY/resize/edge-cases when present, SPDY keeps exec working with no kubectl (#68/D2); `lookupKubectl` seam (overridable in tests) + pure `kubectlExecArgs` builder, hermetically tested — done 2026-07-24 (D128)
- [x] **M3-14b-3** Exec — live terminal resize: a `syscall.SIGWINCH` watcher (`watchResize`, real-terminal path) reads `term.GetSize` (injected `sizeOf`) and pushes it into the exec size queue, now a latest-wins one-slot channel (`push` supersedes an unread stale size, drops 0×0), so the remote PTY tracks the local window mid-session; watcher stopped (`signal.Stop` + wait for the goroutine) before `close`, so no push races the closed channel; hermetic test raises SIGWINCH in-process — done 2026-07-24 (D127)
- [x] **M3-14b-2** Exec — multi-container picker reuse: `openExec` routes through the shared M3-07a container-resolution path tagged with a new `ctrPurpose` (logs↔exec); single-container Pod execs directly, multi-container prompts via the reused `ctrPicker`, `streamOrExec` routes to `streamLogsInto`/`execInto`; `newExecCommand` takes the chosen container — done 2026-07-24 (D126)
- [x] **M3-14b-1** Exec — TUI wire (in-process SPDY primary path): `tea.Exec`→`execCommand.Run()` drives blocking `kube.Exec` off the loop, local raw terminal (x/term) + seed-once size queue, Pod default container `/bin/sh`, result to status bar; `Execer` seam; live exec dogfood raised as a human-task — done 2026-07-24 (D125)
- [x] **M3-14a** Exec — kube-layer exec primitive (`internal/kube/exec.go`): blocking `Clients.Exec` over the pod `exec` subresource via `remotecommand.NewSPDYExecutor` (SPDY, no kubectl binary, D2); apimachinery-free `ExecOptions`/`TerminalSize`/`TerminalSizeQueue` surface with a `sizeQueueAdapter` (D33); TTY folds stderr into stdout + wires the size queue; injectable executor factory, hermetic fake tests (D18) — done 2026-07-24 (D124)
- [x] **M3-13c** Port-forward for Services: a Service can't be forwarded directly (kube.PortForward posts to the pod subresource), so the Port-forward action — re-extended to apply to `Service` as well as `Pod` — resolves it to a backing endpoint pod first via a new `ServiceResolver` seam (`WithServiceResolver`; `kube.PodForService`: `spec.selector` → newest ready pod via `newestReadyPod`, selector-less/no-pods → toast), then opens the ports prompt over — and forwards — the resolved pod; resolve-then-prompt runs off the update loop, generation-guarded (`pfResolveGen`), no resolver → toast (a Pod still forwards directly) — done 2026-07-24 (D123)
- [x] **M3-13b** Port-forward panel: `forwards.panel` (`F`) toggles an app-global overlay listing active forwards (label · bound/requested ports · ready state) with a cursor; `nav.drillIn` stops the selected forward (context cancel → the M3-13a Done flow removes it + notices), `forwards.stopAll` (`X`) stops all via `stopForwards` + a sweep notice; inline overlay state + `forwardsPanelView` composited via `overlayCenter` (D95), captured like help/viewer; ticks the M3 port-forward exit criterion — done 2026-07-24
- [x] **M3-13a** Port-forward start + background lifecycle: `PortForwarder`/`ActiveForward` seam (`WithPortForwarder`, `PortForwarderFunc` launcher adapter) → M1-08 `kube.PortForward` on a Pod row behind a ports prompt (D117 stash); lifecycle via two-edge `waitForward`/`waitForwardDone` messages (not a pump), bound ports to the status bar, forwards tracked in the model, all cancelled on quit (`stopForwards`); Pod-only (Service → M3-13c, panel → M3-13b) — done 2026-07-23 (D122)
- [x] **M3-12** CronJob suspend/resume wired: `Suspender` seam (both verbs, `WithSuspender`) → `kube.Suspend`/`Resume` on CronJob rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **M3-11b** Drain wired: `Drainer` seam (`WithDrainer`) → `kube.DrainStream` (channel twin of `Drain`) on Node rows behind the D115 confirm modal; `drainPump` (mirrors the log pump, `drainGen`-tagged) streams cordon→evict→remove progress to the status bar, terminal error toast / clean-close success notice, `stopDrain` cancel-on-quit; default `{IgnoreDaemonSets:true}` (Force/DeleteEmptyDirData off) — done 2026-07-23 (D121)
- [x] **M3-11a** Cordon/uncordon wired: `Cordoner` seam (both verbs, `WithCordoner`) → `kube.Cordon`/`Uncordon` on Node rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **FB-gray-out-empty-types** Feedback (low/soft, `2026-07-23-gray-out-empty-resource-types`): triaged the "gray out empty left-menu resource types" idea — **dismissed** (D119): the eager per-type-count version is forbidden (fights lazy-list D8/principle 4), the cheap opportunistic variant declined for now (marginal revisit-only value vs a namespace-keyed cache + hot-path plumbing + a third menu visual state needing a real-terminal UX check) — done 2026-07-23 (D119)
- [x] **FB-k9s-not-prior-art** Feedback (normal, `2026-07-23-k9s-not-prior-art`): reworded the README "Special thanks" k9s line from "prior art in the Kubernetes-TUI space" to "a contemporary Kubernetes TUI in the same space" (k9s is a ~2019-2020 peer, not a predecessor); split M3-11 → M3-11a/M3-11b — done 2026-07-23 (D118)
- [x] **M3-10** Scale (prompt → `kube.Scale`) + rollout-restart (confirm → `kube.RolloutRestart`) wired through the D115 modal; new `Scaler`/`RolloutRestarter` seams, prompt-mode key routing (`routeModalPromptKey`), shared `mutateRes`/`mutateRef` stash, results to the status bar — done 2026-07-23 (D117)
- [x] **M2-14b** teatest coverage: modal confirm flow — full-program (teatest/v2) delete confirm: `x`→open→`enter` accept (delete runs) / `esc` decline (no delete), async accept synced on a side-effect signal not Quit-ordering — done 2026-07-23 (D116)
- [x] **M3-09** Delete action wired through the confirm modal (`res.delete`/`x` → `modal.ShowConfirm` on the selected row → accept `nav.drillIn` runs `kube.Delete` (row's UID guards the snapshot race), result to the status bar (error toast / neutral notice); decline `nav.back`/quit closes it, no raw y/n; `Deleter` seam + `WithDeleter`) — **unblocks M2-14b** — done 2026-07-23 (D115)
- [x] **M3-08b** Secret viewer — copy the selected value to the clipboard (#89): per-entry cursor (`nav.up`/`nav.down` select, not scroll; `> ` gutter marks it, `EnsureLineVisible` keeps it on screen), `secret.copy` (`c`) yanks the selected decoded value via `tea.SetClipboard` OSC-52 (masked or revealed), neutral status-bar notice `copied "key" (N bytes)` (new `SetNotice` channel) — done 2026-07-23 (D114)
- [x] **M3-08a** Secret viewer — reveal/base64-decode (#89): `SecretGetter` seam (`kube.SecretData`, typed clientset → decoded + key-sorted entries) → shared M3-01 viewer; values masked on open (`key: •••• (N bytes)`), the registered `secret.reveal` (`r`) gesture toggles reveal (viewer-only, re-renders the same fetched data), `WithSecretGetter` gates it; copy split to M3-08b — done 2026-07-23 (D113)
- [x] **M3-07b** Logs — pod-owning kinds (#84): `PodResolver` seam (`kube.PodForOwner`) resolves a Deployment/RS/StatefulSet/DaemonSet/Job/RC to a backing pod (dynamic Get → `spec.selector` → newest Ready pod, fallback newest); `openLogsViewer` resolves off the update loop then feeds the pod into the shared `resolveContainersFor` (M3-07a container path), titled as a Pod; `WithPodResolver` gates it (no resolver → the M3-05…07a not-yet-available toast) — done 2026-07-23 (D112)
- [x] **M3-07a** Logs container picker for multi-container pods: `ContainerLister` seam (`kube.PodContainers`) resolves a pod's containers before streaming — multiple open the reused modal picker (`ctrPicker`) and the pick streams the chosen container, a single container streams directly; no lister → default container (no picker); streaming factored into `streamLogsInto`, container named in the title — done 2026-07-23 (D111)
- [x] **M3-06** Logs viewer — follow + reconnect: opens tailing (`LogOptions{Follow:true}`, M1-07d) with auto-scroll; `logs.follow`/`f` toggles it (logs-viewer-only, per-open `viewer.SetKind`), a manual up-scroll pauses it, title marks `[following]`/`[paused]` — done 2026-07-23 (D110)
- [x] **M3-05** Logs viewer wired (initial, no follow): `res.logs`/`L` → `LogStreamer` seam (`kube.Logs`, M1-07c) streamed line-by-line into the shared M3-01 viewer via a gen-tagged pump (D53); pods first, `stopLogStream` teardown, open-failure closes/mid-stream error keeps lines — done 2026-07-23 (D109)
- [x] **M3-04** Describe viewer wired: `res.describe`/`d` → `Describer` seam (`kube.Describe`, M1-07b) → shared M3-01 viewer overlay; async render + gen-guard (shared viewerGen), error degrades to a toast — done 2026-07-23 (D108)
- [x] **M3-03** YAML viewer wired: `res.yaml`/`y` → `YAMLGetter` seam (`kube.GetYAML`) → shared M3-01 viewer overlay; async fetch + gen-guard, error degrades to a toast — done 2026-07-23 (D108)
- [x] **M3-02** Action surface + M3 keymap: actions menu (Kind `"action"` picker) + direct keys (`a d y L e x`) → typed `rowActionMsg` intent — done 2026-07-23 (D107)
- [x] **M3-01** Read-only viewer/pager component (`internal/tui/components/viewer`): keymap-routed scrollable text overlay, bare box, `ClosedMsg` — done 2026-07-22 (D106)
- [x] **M3-PLAN** Expand the M3 milestone (actions & viewers) into ordered, leg-sized Backlog slices M3-01…M3-15 — done 2026-07-22 (D105)
- [x] **FB-watch-unsupported-list-only** Feedback (normal, `2026-07-22-watch-unsupported-resource-list-only`): kinds without the `watch` verb (e.g. componentstatuses) blanked/retry-looped — watch degrades to list-only polling — done 2026-07-22 (D104)
- [x] **FB-crd-parametercodec** Feedback (high, `2026-07-22-crd-list-watch-parametercodec`): non-built-in CRD groups failed list/watch — encode params with `metav1.ParameterCodec` — done 2026-07-22 (D103)
- [x] **FB-nav-menu-popup** Left menu as an overlay popup — **retired won't-do-separately**, folded into the resource palette (D96 slice 3, resolving the… — done 2026-07-22 (D81, D96, D99, D100, D101)
- [x] **FB-nav-resource-palette** Command-palette resource switch (k9s `:`-style), D96 slice 2 / D100 — done 2026-07-22 (D11, D65, D95, D96, D99, D100)
- [x] **FB-nav-menu-toggle** Make the left menu pane optional (D96 slice 1) — done 2026-07-22 (D11, D96, D99)
- [x] **M2-13b** Column sort — keymap actions + app wiring (#85) — done 2026-07-22 (D11, D94, D98)
- [x] **FB-mouse-optin** Feedback (normal, `2026-07-22-text-selection-select-to-copy`): the app captured the mouse on every frame (`View` set… — done 2026-07-22 (D11, D86, D97)
- [x] **FB-status-bar-top** Feedback (normal, `2026-07-22-status-bar-top-and-optional-left-panel`): first concrete slice + direction triage — done 2026-07-22 (D11, D96)
- [x] **FB-left-pane-width-smaller** Feedback (normal, `2026-07-22-left-pane-width-smaller`): the left menu pane's `total/4` default ate room the table needs on wide… — done 2026-07-22 (D84)
- [x] **FB-popups-overlay** Feedback (high, `2026-07-22-popups-should-overlay`): the help overlay and namespace picker **replaced** the browse body… — done 2026-07-22 (D88, D95)
- [x] **M2-13a** Table column-sort primitive (`internal/tui/components/table`, component-only) — done 2026-07-22 (D94)
- [x] **M2-12b** Legacy config migration — launcher wiring (`cmd/kubecom/run.go` `maybeMigrate`) — done 2026-07-22 (D93)
- [x] **M2-12a** Legacy config migration — parse + report primitive (`internal/config/migrate.go`): `LegacyPath()` (`~/.kubecom.yaml`) +… — done 2026-07-22 (D92)
- [x] **M2-11b-2** Config: last-namespace load-on-start + persist wiring (`internal/tui`, `cmd/kubecom`) — done 2026-07-22 (D91)
- [x] **M2-11b-1** Config: per-context state store (`internal/config/state.go`): `State{LastNamespace}` + `StateDir`/`StatePath(context)` (reuses… — done 2026-07-22 (D90)
- [x] **M2-11a** Config write-back primitives (`internal/config`): `Config.Save(io.Writer)` marshals via `sigs.k8s.io/yaml` (write-back… — done 2026-07-22 (D83, D89)
- [x] **M2-10** Confirm/prompt modal (`internal/tui/components/modal`): a Kind-stamped, centered/bordered overlay replacing the original's racy… — done 2026-07-22 (D11, D56, D65, D88)
- [x] **FB-hintbar-dedicated** Board (deferred remainder of feedback `2026-07-21-06`/D85): promoted the persistent, focus-aware key hint off the status bar onto… — done 2026-07-21 (D85, D87)
- [x] **FB-menu-config-03** Third/final slice of feedback `2026-07-21-02` (D83): **app wiring** for the per-context menu — done 2026-07-21 (D83)
- [x] **FB-menu-config-02** Second slice of feedback `2026-07-21-02` (D83): per-context menu **merge** — done 2026-07-21 (D57, D77, D83)
- [x] **FB-mouse-support** Feedback (normal, `2026-07-21-08`): additive Bubble Tea mouse support (keyboard/vim stays primary) — done 2026-07-21 (D11, D70, D86)
- [x] **FB-help-popup** Feedback (normal, `2026-07-21-07`): the `?` help/keys view was a full-screen replacement of the whole TUI; it is now a… — done 2026-07-21
- [x] **FB-hint-focus** Feedback (normal, `2026-07-21-06`): the persistent bottom key-hint (status bar) is now **focus-aware** — it shows the keys… — done 2026-07-21 (D11, D85)
- [x] **FB-menu-item-states** Feedback (normal, `2026-07-21-05`): the left menu now shows two independent states so it's always clear both which resource is… — done 2026-07-21
- [x] **FB-ns-seam-followup** Feedback (high, `2026-07-21-09`): three namespace-seam dogfood fixes, the third a functional dead-end — done 2026-07-21
- [x] **FB-esc-back-to-menu** Feedback (normal, `2026-07-21-04`): esc is now the one-level-back gesture — done 2026-07-21
- [x] **FB-menu-scroll** Feedback (high, `2026-07-21-03`): left menu is now a real viewport — done 2026-07-21 (D84)
- [x] **FB-menu-config-01** Feedback (high, `2026-07-21-02`): per-context dynamic menu config — **first slice** (triaged the rest into FB-menu-config-02/03) — done 2026-07-21 (D83)
- [x] **FB-ns-menu-seam** Feedback (normal, `2026-07-21-01`): namespace picker surfaced as a row in the left menu, marking the cluster-scoped ↔ namespaced… — done 2026-07-21 (D77, D82)
- [x] **M2-14d** teatest coverage: error-toast path (D74/FB-errors-layout) — `TestProgramErrorToastDegradesGracefully` delivers a live `ErrorMsg`… — done 2026-07-21 (D74)
- [x] **M2-14c** teatest coverage: filter flow (M2-09b) driven end-to-end through the real bubbletea program — `/` → type → enter-commit via live… — done 2026-07-21
- [x] **M1-04b** Lazy group-detail-on-open: **retired won't-do** — doesn't fit the realized flat Dashboard-sectioned menu (no group-open… — done 2026-07-20 (D77, D81)
- [x] **M2-14a** teatest coverage (update loop + menu reconcile): split from M2-14 — the modal-flow half is deferred to M2-14b (blocked by the… — done 2026-07-20 (D57, D73, D74, D78, D80)
- [x] **M2-09a** Table filter core: authoritative unfiltered `full` row set + a displayed filtered `table` view;… — done 2026-07-20 (D78)
- [x] **FB-menu-nesting** Feedback (normal): flat left menu read as disorganized → resource menu now renders Dashboard-style sections (Cluster / Workloads… — done 2026-07-20 (D57, D77)
- [x] **FB-welcome-page** Feedback (normal): bare launch showed an empty right-pane table → new `welcome` component (`internal/tui/components/welcome`)… — done 2026-07-20 (D76)
- [x] **FB-go-install** Feedback (normal): README `go install …/cmd/kubecom@v1` failed (`@v1` is a semver version query — resolves to a nonexistent… — done 2026-07-20 (D75)
- [x] **FB-errors-layout** Feedback (high): errors broke the TUI layout → transient single-line status-bar toast; root `Update` now handles `ErrorMsg` (was… — done 2026-07-20 (D74)
- [x] **M2-08c** Wire namespace picker into the app shell: `ns.switch`/`ctrl+n` action + `NamespaceLister` seam (`WithNamespaceLister`, nil →… — done 2026-07-20 (D73)
- [x] **M2-08** Namespace picker (08a component / 08b filtering / 08c app wiring) — done 2026-07-20 (D65, D72, D73)
- [x] **M2-RUN** Bare `kubecom` launches the browse UI against a real cluster (root `RunE` + kubeconfig/context/`-n` flags → live `*kube.Clients`… — done 2026-07-20 (D68, D70, D71)
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
- [x] **M0-06** goreleaser skeleton (Linux+macOS): `.goreleaser.yml` reshaped to v2 syntax, single build × `goos:[linux,darwin]` ×… — done 2026-07-18 (D2, D7, D27)
- [x] **M0-09** README references the vault + decision log: rewrite-in-progress banner atop `README.md` linking `vault/`, goals, milestones,… — done 2026-07-18
- [x] **M0-05** Test harness: teatest (tui) smoke test — done 2026-07-18 (D26)
- [x] **M0-08** Sweep remaining legacy files M0-07 missed: `Dockerfile`, `get.sh`, `ci/aur/` (incl — done 2026-07-18 (D25)
- [x] **M0-04** GitHub Actions CI: `make check` (build/test/vet/golangci-lint) on Linux+macOS matrix (D24) — done 2026-07-18 (D24)
- [x] **M0-02** Toolchain: bump to Go 1.23 (go.mod directive), wire `cmd/kubecom` onto cobra v1.10.2; `ioutil` already gone via M0-07 (D23) — done 2026-07-18 (D23)
- [x] **PROC-04** Cloud runs set repo-local git identity (maintainer) in routine bootstrap + skill step 0 — done 2026-07-18
- [x] **PROC-03** Model split: routine session on Sonnet (orchestration), leg subagents pinned to Opus in `/do-rewrite-run`; cron corrected to UTC — done 2026-07-18
- [x] **M0-07** Delete legacy trees from `v1`: `app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`, Windows sources,… — done 2026-07-18 (D14, D22)
- [x] **PROC-02** Scheduled runs self-prime: checkout `v1` + lint tooling in step 0; legs read the skill by file path (cloud clones start on… — done 2026-07-18
- [x] **PROC-01** `/do-rewrite-run` orchestrator skill: fresh subagent per leg, sequential, 4-leg/90-min budgets — done 2026-07-18 (D21)
- [x] **REVIEW-01** Maintainer setup review applied: legacy deletion planned, journal split to per-entry files, claim-push, `make check` + CI-early,… — done 2026-07-18 (D14, D20)
- [x] **M0-03** Single `kubecom` binary — done 2026-07-18 (D14)
- [x] **M0-01** Scaffold new module layout — done 2026-07-18 (D12, D13)
- [x] **BOOT-01** Bootstrap vault, goals, milestones, task board on `v1` — done 2026-07-18
