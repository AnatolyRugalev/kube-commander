# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-08-20 — STORY-06k-2 (the search result preview) landed, closing STORY-06k: `kube.SearchHit` now carries the server-printed row it was matched in (`Columns`/`Cells`, free — the search already listed that row; D276 pt 2's "a search hit carries no cells" rationale is superseded, drilling in still re-lists and watches), and the cluster search renders a two-line preview under the results for the highlighted hit — identity (kind · apiVersion · namespace/name) over the object's own cells as `COLUMN: value` pairs, the NAME column and empty cells dropped — so two similarly named objects tell themselves apart before either is opened; the block's height is reserved unconditionally so rows never shift when the first hit lands, a kind that printed no cells keeps it with a blank line, and a preview stays request-free by rule (D283). The S05 feedback `2026-08-15-search-result-preview.md` is deleted. Earlier: STORY-06k-1 (single enter reaches a search hit) landed: STORY-06k was split on pickup into 06k-1 and 06k-2 (the result preview), and `nav.drillIn` in the cluster search now opens the highlighted hit whatever the focus — superseding SEARCH-05/D235's commit-then-open half — while every cursor action routes through a new `enterResults` that hands the keyboard to the result list on the first movement over it and then moves, so the `hjkl`/`g`/`G`/page mode is entered by using it (no key of its own, nothing new to hint), the hand-off refuses an empty list, `nav.back` is unchanged and now the only action here that reads the focus, and the S05 feedback `2026-08-15-search-single-enter.md` is deleted (D282). Earlier: STORY-06j-2 (scroll-past-the-end re-arms follow) landed: every downward navigation in the logs view routes through `scrollDown`, which re-arms following when the press is the no-op at the newest line (`atNewest`: paused, not selecting, cursor already last) — landing on the last line is still browsing, the press *after* it is the "and keep going" that in a pager over a live stream can only mean rejoin it, so it does what `G` does; visual mode is excluded (D242 pt 5), a page-down overshooting from above only lands, D147's downward half is superseded, the test that pinned it is replaced by the landing/past-the-end pair, and the S03 feedback `2026-08-15-logs-scroll-past-end-resumes-follow.md` is deleted (D281). Earlier: STORY-06j-1 (the painted follow indicator) landed: the header renders `[following]` through a new `styles.Follow` badge (canvas-on-Success on a dark canvas, bold alone on a light one, D252 pt 3's rule) so live vs frozen is unmistakable, and the slice was split out of STORY-06j with 06j-2/06j-3 (follow-rearm, newest-first) split back to Backlog. Earlier: STORY-06g-2b-2 (the cross-kind unhealthy wiring) landed: `U` (`app.unhealthyScan`, the capital partner to the per-kind `H`) opens `components/unhealthyview` and runs exactly one `kube.Scan` over the menu's kinds with `table.UnhealthyRow` as the predicate (every offered kind — the point is the broken thing may be a kind the operator is not looking at), in the app's own namespace, capped at `scanHitLimit`=200, over a generation-guarded pump (`scanGen`/`scanCancel`/`scanCh` mirror search, D140 pt 3) on a `Scanner` seam that mirrors `Searcher` (Cluster bundle, WithScanner, nil → scan-inert); each drill-in closes the list and switches browse to the hit's kind with its object stashed as the shared pending selection (applyPendingSelect); the view routes like the logs view with its grep closed (handleAction intercepts, `q` closes), owns a `HelpUnhealthy` hint context (D143 pt 1), `stopScan` joins the `stopClusterAsync` inventory, and the S02 feedback `2026-08-15-pod-first-blinds-non-pod-failures.md` is deleted (D279). Earlier: STORY-06m (pane-scoped `/`) landed: `app.filter` now targets whichever pane holds focus — with the resources pane focused `/` narrows the kinds there (`menu.SetFilter`, matching the D203 alias surface) instead of the table, with the table focused it keeps its M2-09b meaning; the menu gained the table's full/displayed split (authoritative `full` beside a filtered `items` view, the three mutators re-deriving it) so a narrowing never loses a kind, `Items()` still returns the whole kind set for search/pickers/pane-memory, `HelpMenu` advertises `/`, and the feedback file is deleted (D277). Earlier: STORY-06g-2a (the cross-kind sweep primitive) landed: `kube.Scan(ctx, resources, namespace, keep RowFilter, limit)` is the kube-layer fan-out that a cross-kind unhealthy list will ride — it Lists each kind once, keeps the rows a caller-supplied predicate accepts (the M4-06 health classifier, a TUI concern, is a seam, D276), and streams ScanMatch / exactly-one-ScanKindDone-per-kind (a failed List degrades to `Failed`, never aborts) / a terminal ScanDone reporting Capped, under Search's own concurrency bound and cancellation rules (D131 pt 2, SEARCH-03); a hit carries the row cells so the surface can render the reason without re-listing, unlike a SearchHit (D276); `sendEvent` generalized to the shared generic guard; the healthy rows stay out and the PVC/stuck-claim case is proven in tests. STORY-06g-2 is split on pickup: 2a (this primitive) done, **2b (the surface that makes a non-pod failure findable, deleting its feedback file) next up** (D69). Earlier: STORY-06g-1 (the unhealthy filter on the current table) landed: `app.unhealthy` (`H`, shift+h — the feedback's own "healthy vs h", `2026-08-15-unhealthy-workloads-quick-access.md`) narrows the current resource table to the rows the M4-06 classifier reads as unhealthy — a row any visible cell classifies to warn/error (CrashLoopBackOff, ImagePullBackOff, Pending/unschedulable, not-ready, a stuck claim); it composes with the `/` substring filter, resets on a new resource, `esc` clears it, the status bar shows an `unhealthy` marker while on, and the palette lists the verb (D275); the feedback file is deleted, and the cross-kind sweep that makes a non-pod failure findable from anywhere is split to STORY-06g-2. Earlier: STORY-06f (a dedicated `events` action) landed: the selected object's **own core Events** — the `kubectl get events` columns, server-printed, no hard-coded columns (D33) — are now a viewer of their own on `E`, filtered to the object by `involvedObject.uid` (name fallback, empty name rejected), the surface for "why is this red" instead of hunting through describe (D274); new `kube.Clients.Events` primitive + `EventLister` seam on the Cluster bundle, every column padded to its widest cell except the last which flows, empty list says "(no events)", an error degrades to a toast (D74); verified live against the story cluster's crash-looping pod; the feedback file is deleted. Earlier: STORY-06e (the instrument's mirror fixes) landed: the recorder writes the **confirm modal's resolved action** — a handled `y`/`n`/`enter`/`esc` records `confirm.accept`/`confirm.decline` instead of a blank press the analyzer misread as a dead end (the S02 `esc 2 modal-confirm` lie), shared `confirmResolved` with the router while an unhandled confirm key stays a dead end; and `keys analyze` gains a **"presses on text surfaces"** section ranking `Text` presses that resolved to nothing, so the picker's 4 dead `j`s of S01 are a finding rather than silently skipped (D273); both feedback files deleted. Earlier: STORY-06d (picker navigation mode) landed: pickers open in **navigation mode** — list focused, j/k navigate, `/` opens the filter, and the current choice is preselected (namespace/context/theme) — flipping D194 pt 2 for value pickers (D272); the palette verb list keeps type-to-filter, the palette's argument stages use navigation mode with `OpenFilter`/`CloseFilter` in-place transitions, the port picker's opt-in construction is gone, and the feedback file is deleted. Earlier: STORY-06c (enter → actions menu) landed: a **table key context** (ctxTable) binds `actions.menu` to `enter` while the resource table owns the keys (the D132 context-split pattern — `enter` keeps its browse `nav.drillIn` meaning everywhere else), `a` frees up, and the table's old `enter`→RowSelectedMsg dead end is gone; drill-in (Show pods) is an entry in the menu, so drilling into an owner's pods is one menu pick away (D271). Earlier: STORY-06b (the S-mode sort) landed: `s` freed, `S` focuses the table's column-header row, `h`/`l`/`left`/`right` move the cursor across the columns, `enter` toggles direction, `esc` returns to the rows, and `x` (sort.clear) is the mode's clear pick — a capturing surface with its own `HelpSort` hint context (D270). Earlier: STORY-06a (the letter remap) landed: search `ctrl+f`, ns.switch `N`, delete `D`, describe `d`, `#` for previous-match, `space` for full-page-down, `V` selects log lines; the keymap doc, the tape and the README updated, the seven superseded key feedback items deleted, and the help overlay width-clamped via a new `elide.Width` (D269). The redesign's two interaction halves (S column-header sort, enter→actions-menu) are STORY-06b/06c; both done, 06d next up. Earlier: STORY-06 claimed and re-split into per-finding slices 06a…06m; the master `keymap-redesign.md` was triaged at the claim, its S-interaction and enter→menu halves carried by 06b/06c. Earlier still: STORY-03 landed `kubecom keys analyze <trace.jsonl>`: the read side of the instrument, reporting the four findings a UX pass asks — unresolved presses ranked by frequency, action counts, the longest pauses, and the abandoned sequences, judged against the resolved keymap so a completed `gg` is not a finding (D268). The whole STORY line's inputs now exist, and STORY-05 (the maintainer's walk) is the only unblocked item between here and the fold-in. Earlier: STORY-04 wrote the stories: `stories/s01…s05`, S02 marked the main story, each guard-tested to name no key, and `CONTRIBUTING.md` now makes writing a story the way to argue for a UX change, so the maintainer can start walking scenarios (STORY-05) once STORY-03's analyzer lands. Earlier: STORY-02 landed `--keylog`: one JSONL record per keypress carrying the surface and the resolved action, recorded at the single `KeyPressMsg` funnel so the trace cannot disagree with the routing, off unless asked for, and verified against the live cluster (D268). **STORY-04 is pulled ahead of STORY-03** — the maintainer wants to start walking scenarios as soon as they exist, and the first traces are short enough to read by hand. Earlier today: STORY-01 committed the story cluster: `stories/cluster/up.sh` destroys and rebuilds `shop`/`data`/`broken` so every run starts identical, the five failures in `broken` each have a different answer, and `internal/stories` guards the manifests plus the tape queries that must still match them — verified end to end on the maintainer's docker (D268). Earlier today: UX-PLAN expanded the maintainer's pre-tag scope into a UX-validation line that now gates the stable tag: STORY-01…06 (committed k3s fixture, `--keylog` + its analyzer, the stories, the maintainer's walk, the fold-in), TAPE-01 (re-cut the screencast around the main story) and DOC-03…05 (README landing page, `docs/usage.md`, `docs/troubleshooting.md`) — D268. Earlier: BOARD-03 resolved the `DOC-02` id collision (the 2026-08-09 README prose pass is now `DOC-01b`) and guarded id uniqueness, the one index property D225's guards had assumed rather than checked (D267). Earlier today: DOC-02 caught the install docs up with the `v1.0.0-rc.1` tag they had been contradicting since 2026-08-10, and ticked the M5 exit criterion that run closed (D266). Earlier: RC-PRERELEASE set `release.prerelease: auto` after the rc shipped as a full release. Earlier: MONO-03 landed the third app.go cut: the browse filter seam is `components/filter`, owning the `/` field's open/query state while the shell keeps the table and performs the narrowing; the `filterInput`/`filtering` pair is gone and the MONO line is closed (D265). Earlier today: MONO-02 landed the second app.go cut: the secret viewer's entry list is `components/secretviewer`, a sub-model owning reveal/mask + the cursor and rendering the body from the shell's authoritative `SecretData`; the shell still fetches and performs the copy, and `secretRevealed`/`secretSel`/`secretEntryLines` are gone (D265). Earlier today: MONO-01 landed the first app.go cut: the M3-13b port-forward panel is now `components/forwards`, a sub-model fed read-only `Entry`s while the shell keeps the handles and performs the stops (D265). Earlier today: APP-MONOLITH closed feedback `2026-08-09-audit-app-monolith` (D264): the plan's `views/` directory is revoked — full-screen views are `components/*` sub-models (`searchview`, `logsview`, `viewer`), browse is the root shell itself, and `app.go`'s reduction is the standing MONO-01 item; REWRITE_PLAN and the M2 layout note now match the tree. Earlier: FORMAT-GATE (D263); TEST-RUNTIME (D262); also: M5-09b closed the screencast-tape-tuning feedback (D261); THEME-07 (D260); LOGS-SEL-04 (D259); LOGS-09 (D258); HT-fold-0809 closed the five done human tasks (M4 **done** on the D256 pt 1 waiver, CTX-WARM-02/03/04 cancelled on pt 2, the goals DoD at 11 of 13 on pt 3, the screencast criterion half closed); LOGS-08 (D257); CTX-MEM-04 (D255); THEME-06 landed `gruvbox-light`; M5-11 stays blocked on the deferred tag. Per-leg history: `vault/journal/`._
## In Progress

## Blocked

- [ ] **M5-11** Make the rewrite the default branch (`v1` → `main`)
      status: blocked | owner: — | added: 2026-07-30
      notes: Blocked on human task `2026-07-30-first-release-tag` — the maintainer deferred the
      tag on 2026-08-09 ("a bit too early for that"), so the block stands; renaming the branch before
      a release exists would retarget every clone and PR for a tree nobody can install yet, and
      the rename also dissolves the `@v1` collision the tag is what actually fixes. (The release-
      namespaces blocker that also gated M5-11 was resolved 2026-08-09 — D253/D254 folded in.)
      The agent share is preparation: what to rename, `master` kept as the permanent 2020 reference
      (D14, do *not* delete), the workflow `branches:` lists (both already name `main`) and the
      README/vault links that say `v1`. The act itself is a GitHub admin setting — a human's.

## Backlog

### M0 — Groundwork
_(none — M0 complete)_

### M1 — Kube layer
_(none — M1 is **done** (D66; M1-04b retired as obsolete, D81), and the **M1-INT envtest
line is closed** as of M1-INT-d (2026-07-31): the un-deferral (D186), discovery isolation
+ DISC-01 (D187), both watch-reconnect branches (b-1/b-2), the whole action set
(c-1…c-4, D188/D189) and a CI job that runs the gated suite on every push (D190). The
per-slice history is in `vault/journal/`.)_

### M2 — Core TUI
_(none — M2 is **done** (2026-07-29): every exit criterion in
[`../milestones/M2-core-tui.md`](../milestones/M2-core-tui.md) is ticked against named
evidence (M2-EXIT/D154), and M2-15 cleared the one enhancement left behind.)_

M2-01 (action registry + configurable keymap, D10/D11) is **complete** (01a keymap
core / 01b sequences / 01c config wiring / 01d help overlay / 01e generated doc).
The rest of M2 is the **app shell** — expanded here into ordered, leg-sized slices
(D52). Take them top-down; each notes what it depends on. Package layout follows
[`../REWRITE_PLAN.md`](../REWRITE_PLAN.md): `internal/tui/{app.go,msg.go}`,
`internal/tui/styles`, `internal/tui/components/*` (the full-screen views are
sub-models there — `searchview`, `logsview`, `viewer`; the `views/` directory D52
named is revoked by **D264**, since browse is the shell itself). Every
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
_(none — M3 is **done** (2026-08-06): every exit criterion in
[`../milestones/M3-actions-viewers.md`](../milestones/M3-actions-viewers.md) is ticked
against named evidence, the last of them — Edit round-tripping through a live `$EDITOR` —
on the maintainer's own cluster (HT-dogfood-0806). The standalone YAML viewer that was in
M3's scope never shipped and never will: `e` opens the object's YAML in the user's real
editor for reading and writing (D135/D178). Per-slice history: `vault/journal/`.)_

### app.go reduction (MONO — audit-driven, D264)
Feedback `2026-08-09-audit-app-monolith` (Priority: medium): `app.go` is a 4.6k-line
god object and the plan's `views/` split was never built. **Closed as a decision
(D264)** — the `views/` directory is revoked (full-screen views are `components/*`
sub-models; browse is the shell), and reducing `app.go` is a standing, pickable
effort, done when a clean seam appears rather than as a forced multi-line move.
**The first cut landed 2026-08-09 (MONO-01/D265)**: the port-forward panel is
`components/forwards`, and the shape a seam follows is settled — a shell-owned
listing is handed to the component as read-only `Entry`s while the shell keeps the
authoritative set and performs the mutations. **The second cut landed 2026-08-09
(MONO-02)**: the secret viewer's entry list is `components/secretviewer`, owning
reveal/mask and the entry cursor and rendering the body from the shell's
authoritative `SecretData`, the clipboard copy staying a shell gesture — and the
three interaction fields (`secretRevealed`/`secretSel`/`secretEntryLines`) are
gone from `app.go`. **The third cut landed 2026-08-09 (MONO-03)**: the browse
filter seam is `components/filter`, owning the `/` field's open/query state
while the shell keeps the table (the authoritative rows) and performs the
narrowing (`table.SetFilter`) — the `filterInput`/`filtering` pair is gone, and
**this closes the MONO line**: every seam D264 pt 4 named (panel, entry list,
browse filter) is extracted, so the standing effort stops at the three cuts and a
future leg re-opens it only when a new clean seam appears.

- [x] **MONO-01** The port-forward panel is `components/forwards` — a sub-model owning open/cursor and the BOX-02/HINT-04 geometry, fed read-only `Entry`s while the shell keeps the handles and performs the stops — the first cut out of `app.go` (~220 lines) — done 2026-08-09 (D265)

- [x] **MONO-02** The secret viewer's entry list is `components/secretviewer` — a sub-model owning reveal/mask and the entry cursor, rendering the body from the shell's authoritative `SecretData` at render time, the clipboard copy staying a shell gesture — the second cut out of `app.go` — done 2026-08-09 (D265)

### Cluster search (SEARCH — feedback-driven, D131) — closed, reclosed at SEARCH-06
Feedback `2026-07-24-cluster-search-multi-resource`: `ctrl+s`, type a query, get matching
objects **across kinds** (Kind · namespace · name), drill into a hit. **Closed** at
SEARCH-04c-2b: one-shot, concurrent, curated-scope by default and never "watch everything"
(D131/D141); progress and the hit cap on screen (D142/D143); both scopes widenable as
independent flags — `ctrl+a` kinds over a fan-out bounded to eight lists (D149), `ctrl+w`
namespaces without touching the app's own scope (D150); `-l app=web` a server-side label
selector (D151); and exact ranked above fuzzy by a reserved band the view inserts into
(D152/D153). Per-slice history: `vault/journal/`, by slice id.

Two standing answers, so a later leg answers them rather than re-deriving them. A **field**
selector is **declined**: per-kind field support varies (`spec.nodeName` is a Pod thing), a
selector the kind does not support fails its List, and a failed kind is silent by design
(D131 pt 3) — it would quietly drop most of the scope (D226 pt 3). And `searchScatteredShare
= limit/4` and the absent minimum needle length stay **open guesses**: the 2026-08-01 dogfood
found no problem, which is narrower than verified, and D191 pt 2 forbids citing it as
evidence for keeping them *or* for changing them.

Reopened once on **input**, by feedback `2026-08-06-cross-search-enter-navigate`, and
reclosed at **SEARCH-05**: the view now has a focus (**D235**). Enter commits the query into
the result list — `hjkl`/`g`/`G`/page keys navigate there, esc hands the keyboard back with
the query and hits intact — and only from the list does enter open a hit. This **amends
D140 pt 1**: the query field is still open for the view's whole life, it just no longer
holds the keyboard unconditionally, which is what made every rune text and `j` a `j`. Two
constraints a later leg must not walk into: on the results an **unmapped key is dropped, not
typed** (typing would cancel the fan-out and discard the rows the reader is standing on), and
the **muted query line is the signal** that typing stopped reaching it — blur only removes a
cursor, which is an absence nobody notices.

Reopened again on **legibility**, by feedback `2026-08-06-search-highlight-matches`, whose
table half landed as FILT-02 (D239) — and **reclosed at SEARCH-06**, which took the cluster
half the way D239 could not: `kube.SearchHit` now carries the runes it matched, because a
fuzzy or label-selector hit is not a substring a view can re-derive (**D246**). Three
standing constraints come with it: the spans are in the **name's** coordinate space and the
consumer that laid out the row does the shift; they mark the occurrence the *score* was read
from, which is where this deliberately differs from the table's mark-them-all (D239); and
they are a second pass over the emitted hits only, never a cost the cluster-wide scan pays.

### In-panel `/` search (FILT — feedback-driven) — closed, reclosed at FILT-02
Feedback `2026-08-06-search-backspace-cancel`: `/` then backspace with nothing typed left an
empty prompt open. **Closed** at FILT-01 (**D238**): a backspace that finds the line already
empty resolves to `nav.back` and takes the surface's own unwind step — `clearFilter` on the
table, `closeFilter` in the logs grep — so the cancel gesture and esc can never come to mean
different things. Two constraints a later leg must not walk into: the check runs **after**
the keymap (a config that binds backspace still wins), and it belongs to a `/` opened **over
content**, not to a picker's incidental filter or to the cluster-search view whose query *is*
the view (D140 pt 1/D235 pt 2). Per-leg history: `vault/journal/`.

Reopened once on **legibility**, by feedback `2026-08-06-search-highlight-matches` ("both
regular (in-panel `/`) search and cross-resource search should visually highlight the matched
text … check current behavior for both before doing the work"). The survey found the logs
grep already highlighting (D145) and the other two not, so the item split: **FILT-02**
(reclosing this line) paints the table's matches, and **SEARCH-06** (done, D246) was the
cluster-search half. Constraints a later leg must not walk into (**D239**): the highlight's scope is the
filter's scope exactly, a match cuts a status-colored cell rather than replacing it, and the
**cursor row keeps its marks** — the one exception to M4-06's "selection wins outright".

### Logs dedicated view (LOGS — feedback-driven, D134) — closed again at LOGS-09
Feedback `2026-07-24-logs-dedicated-view-live-grep`, then `2026-07-29-logs-tail-and-perf`
and `2026-07-29-logs-init-containers`: a **dedicated full-screen logs mini-app** with a
`/`-filter that narrows the stream live while following. **Closed twice.** On *features* at
LOGS-04c: the shared viewer has no logs mode any more (D144), the grep is substring or regex
with its hits highlighted (D145), a long line wraps or scrolls sideways (D146), `nav.bottom`
rejoins the stream rather than scrolling to it (D147), and `logs.timestamps` is a redraw over
stamps the stream already carries (D148). On *cost* at LOGS-05b: an open tails the last 1000
lines instead of replaying a week (D160), and an appended line extends a rendered cache that
the pump feeds in batches — ~131 ms → ~2 ms for a 1000-line open (D162,
`BenchmarkStreamLines*`). LOGS-06 reached the init and ephemeral containers, classified and
marked `name (init)`, since an init container's logs are the only diagnosis a pod stuck in
`Init:` has (D161). Per-slice history: `vault/journal/`.

The first two closures were on **latency**, and BOARD-02b-3's sweep of the paragraphs above
reopened the line a third time on **memory**: nothing bounded the buffer at all. That was
LOGS-07, and it is now closed — the buffer holds the newest 10 000 lines and drops from the
top (**D245**), so this line has no open item and everything here is the collapsed history of
closed work (D229 pt 1). The 2026-08-01 throughput dogfood (~1,900 lines/sec, no degradation)
**confirms D162 and does not retire it**; what that closure licenses, and the buffer depth it
never measured, is D191 pt 3, and neither D160 nor D162 was ever evidence that the depth was
bounded (D230) — D245 pt 1 is, and it is the constraint the two constants now live under.

- [x] **LOGS-07** Logs buffer bounded at 10 000 lines, trimmed from the top — done 2026-08-07 (D245)

Reopened a fourth time by feedback `2026-08-09-logs-no-previous-keeps-view` and
**reclosed at LOGS-08 (D257)**: a rejected previous-instance flip no longer closes the
log view — the running instance's stream resumes under the toast naming the server's
reason (partially superseding D177 pt 4; the no-pre-check half stands).

- [x] **LOGS-08** A rejected previous-instance flip keeps the log view; the running stream resumes under the toast — done 2026-08-09 (D257)

Reopened a fifth time by feedback `2026-08-09-logs-view-palette-bindings` (paired with
`2026-08-09-context-switch-key-from-overlays`) and **reclosed at LOGS-09 (D258)**: the
palette family passes through the logs view — `:` opens over the stream, listing the
view's own verbs beside the globals, and `C`/`T`/`R`/`ctrl+n` open their stages, so
ctx.switch is reachable from a log — while row verbs stay swallowed (their target is
invisible there); `logs.previous` defaults to `o` with `ctrl+p` kept for mid-grep.

- [x] **LOGS-09** The palette opens over the logs view with the logs verbs listed; `o` is the previous-instance default — done 2026-08-09 (D258)

### Logs selection and yank (LOGS-SEL — feedback-driven) — closed again at LOGS-SEL-04
Feedback `2026-08-07-logs-selection-and-yank`: the logs viewer scrolls but has no cursor, so
there is no way to say "this line" and therefore no way to copy one. The only route today is
`M` (drop mouse capture) and the terminal's own select-to-copy, which costs the mouse, cannot
reach past the screen, and on a wrapped line copies the *visual rows* — a long entry comes
back with breaks that were never in the log. The ask is a line cursor, a vim visual mode on
top of it, and `y` through the clipboard path `secret.copy` already built (M3-08b, OSC-52
included). The submitter asked for the split below and named the cases that decide whether it
feels right: follow must pause while selecting, the cursor and the selection are over **log
lines** not screen rows, selection covers what a `/` query *displays*, a yank matches the
timestamps toggle's current state, and **no styling may reach the clipboard**.

LOGS-SEL-02 landed visual mode and the yank on top of it (**D244**): a selection and a
running stream are mutually exclusive, `G` extends rather than re-arming the tail (so
`gg v G y` copies the buffer), both ends of the range survive a grep change as *log lines*,
and the clipboard gets the buffer — no styling, no wrap breaks, the stamp iff
`logs.timestamps`. `y` is now live in two key contexts (`logs.yank` in browse,
`confirm.accept` in confirm), which is legal because the surfaces are modal.

LOGS-SEL-01 landed the cursor half and wrote the constraints the rest inherits (**D242**):
the cursor counts log lines, it addresses the *shown* set with `shownIdx` as the only route
back to raw text, Selection and Match share the cursor's line, the bar is derived on the way
to the viewport rather than cached into `shownLines`, and following owns the cursor.

LOGS-SEL-03 closed both of the questions the feedback left, and **this line is now closed**
(**D252**). They were written as legibility judgements only a dogfood could make, which is
why two legs skipped them; the first is in fact *measurable* — a contrast between two
backgrounds — and measuring it is what answered it. (1) The bar and the `/` highlight
**coexist**, neither yields: `Match` paints inside the bar and the two backgrounds are
4.05–9.89:1 apart across the eleven dark built-ins, so blanking either on the cursor's line
is forbidden rather than merely unnecessary. (2) **No yank-all binding** — `y` already
copies the cursor's line, `gg v G y` the buffer, and "everything visible" is ambiguous once
`w` wraps or `/` narrows. The third question the feedback raised (the top of the buffer
while lines stream in above) was already answered by LOGS-07's cap. The human-eye check
(item 6 of the crash-loop dogfood) came back 2026-08-09: the bar and the highlight do read
as two things; what the maintainer wants brighter is the match itself — tracked as
feedback `2026-08-09-log-match-highlight-background`.

The same measurement found a **defect it did not cause**: `styles.Match` is `StatusBarBg`
on `Warn`, mapped when every palette was dark, and on the two light ones admitted at
THEME-04b a matched span renders near-white on yellow (2.15:1, 2.62:1). That is **THEME-05**
below, and D252 pt 3 holds it to the body-text floor.

Reopened a second time by feedback `2026-08-09-log-match-highlight-background` — the
maintainer wants the match itself brighter ("bright (yellow) background"), and THEME-05's
weight-only treatment had removed the paint he remembered — and **reclosed at LOGS-SEL-04
(D259)**: `Match` keeps THEME-05's bold + underline everywhere and regains paint on a dark
canvas, now **canvas-on-`Warn`** (4.68–12.91:1 for the matched text across the eleven dark
built-ins, clearing the 4.5 floor even on `solarized-dark`, which the old `StatusBarBg`
mapping shipped at 4.05). The bar/highlight separation is unchanged (4.05–9.89:1, D252
pt 1 stands); the three light palettes stay weight-only, which is scope, not a leftover.

- [x] **LOGS-SEL-04** The match highlight paints the bright (Warn) background on dark canvases; weight-only on light — done 2026-08-09 (D259)

- [x] **LOGS-SEL-03** Both open questions answered: the bar and the highlight coexist, and the yank gesture set closes — done 2026-08-08 (D252)

### Diagnostics (DIAG — feedback-driven) — closed
Feedback `2026-07-29-external-secrets-crd-error` ("need to find the actual error"): opening
the external-secrets `ExternalSecret` CRD errored out, and the report could carry no error
text because kubecom had **nowhere to put one** — every failure funnelled into a 5-second
status-bar toast and was gone. **Closed** at CRD-01. DIAG-01 made the error obtainable:
`surfaceError` logs before it toasts and discovery's deliberately-silent failures (total and
per-group) log too, so `~/.cache/kubecom/kubecom.log` — documented in the README since
M2-RUN — finally holds what the user hit (D159). CRD-01 then put the degradation on screen:
an empty browse pane says why its LIST failed, naming a conversion webhook and a 406 on
Table conversion apart from the kind that would otherwise misdescribe them (D200). Per-slice
history: `vault/journal/`.

Two things the closure does **not** license. There is **no client-side CRD bug**: the report
is not reproducible, the clean install differs only in serving `strategy: None`, and a
conversion webhook that is down fails the LIST *in the apiserver*, killing `kubectl` with it
— so no leg may write a fix to kubecom's CRD handling on the strength of CRD-01's original
title, which would be inventing a bug (D191 pt 1, D79). And the wording CRD-01 landed is
unverified against a real broken-webhook cluster **by maintainer choice**: the dogfood that
would have read it was declined 2026-08-09 ("do not care"), the hermetic coverage standing as
the verification (D256 pt 3). If a real apiserver's wording ever diverges, it comes back as
ordinary feedback — do not re-raise the task.

### Custom resources (CRD-PIN — feedback-driven) — closed
Feedback `2026-08-01-custom-resources-pinning`: on a CRD-heavy cluster, a kind you reach for
**once** is in your menu for that context from then on. The usability half of CRDs, kept
apart from CRD-01's degradation half (D191 pt 1). **Closed** at CRD-PIN-05: a per-context
pin store merged behind the authored menu entries (D193), `*` to pin and to unpin from the
menu row (D201/D202), a picker that finds a kind by any name it answers to (D203), and
`:pin <kind>` in the palette — an argument stage needs no key the pickers cannot spare
(D204). Per-slice history: `vault/journal/`.

Two standing answers, both **no item until asked**: `:resource ` has no in-place pin chord
(you retype the kind under `:pin `), and neither stage shows which kinds are already pinned
— the second argues with D202 pt 4, which keeps pin provenance off the display on purpose,
so it wants a user saying the notice is not enough (D226 pt 2). Where both stages get their
kinds is **D227**: narrowing what the menu lists takes `:resource ` *and* `:pin ` down with
it unless `resourcePickerItems` is re-sourced from the discovery result first.

### Hint-line truth (HINT — agent-found) — closed
The bottom hint line is a promise about which keys act **right now** (D143 pt 1), and it had
been lying wherever a surface captures input — a picker opens its filter field with itself,
so `/`, `n`, `s`, `a`, `?` and `q` type into the query while the hint underneath still showed
the browse set. **Closed** at HINT-05: every capturing surface has a `HelpContext` written
where its router tests it — the pickers (D206), the modals, help overlay and shared viewer
(D217), the browse filter field and the port-forward panel (D218) — no view spells a key into
its own body any more (D219), and the completeness is **enforced** rather than merely reached:
`helpContextCount` bounds the enum, `HelpContexts()` enumerates it, and a declared context
that carries no curated set (keymap) or that no model state produces (tui) turns one of two
tests red (D223). The sets stay hand-curated; only their completeness is mechanical, and the
`ShortHelpContext` fallback survives for the out-of-range integer it was always right for.
Nothing is deferred here. Per-slice history: `vault/journal/`.

### Overlay geometry (BOX — agent-found) — closed
Every overlay is centered over the browse body by `overlayCenter`, which flattens onto a fixed
`width×bodyHeight` canvas — so a box taller than the body is not scrolled or shrunk, it is
**silently clipped, bottom-first**. **Closed** at BOX-03: the confirm/prompt modal (D220), the
port-forward panel (D221) and the keybindings overlay (D222) each bound their own height, and
the clamp lives in `internal/tui/elide` so a fourth overlay inherits it instead of re-deriving
it (D222 pt 2). The two shapes are settled: a bounded surface **without** a cursor truncates
and marks the cut (`… (truncated)`, or a marker naming where the rest is — D220 pt 3/D222
pt 1), one **with** a cursor scrolls and counts what it hides in its title (D221). Per-slice
history: `vault/journal/`.

Nothing is deferred, but one warning stands, because this class of bug is invisible to the
obvious test: a unit test reading `View()`'s own string cannot catch it — the string is
complete, only the composited frame is short. Assert against the height the geometry promised,
or read the box through the canvas that will clip it (D220 pt 1).

### Command palette (PAL — feedback-driven) — closed again at PAL-07
Feedback `2026-08-01-command-palette-unification`: one place you type to make anything
happen, instead of five modal pickers on five keys with five opt-in filters. **Closed** at
PAL-06: every list picker filters as you type, ranked by the cluster-search matcher (D194);
`:` opens a verb list (D197); a verb commits in place and the list becomes its values,
including the asynchronous ones (D198/D199); the selected row's verbs are offered there too
(D205); the five old keys became sugar for a palette stage, one key per slice because
retiring a picker takes its 20–52 references with it (D207–D210); and `:action ` marks the
verbs that ask permission before they act (D228). `ctrPicker`/`portPicker` stay modals —
they list an object's own containers and ports, not a compiled-in set. Per-slice history:
`vault/journal/`.

Two standing answers, both **no item until asked**: ranking the row verbs by frequency
(it wants usage nobody has reported, and a list that reorders under you is its own
complaint), and marking the *prompts* — Scale and Port-forward — as well as the confirms,
which is a second marker rather than a wider one, because `(confirm)` declares permission
and not input (D228 pt 3).

**Reopened and reclosed 2026-08-06** by feedback `2026-08-06-action-menu-esc-behavior`:
`a` then esc left the palette up. D207 pt 2 had esc and backspace both rewind a key-opened
stage to the verb list; PAL-07 splits them (**D233**) — esc backs out of the *surface* (a
key-opened stage closes, a typed one still rewinds, because that is where its reader came
from), backspace still unwinds the *line* from anywhere, so D207 pt 1's "a key is sugar for
a stage" is untouched. The one thing a later leg must not undo: a non-empty query still
costs its own esc first, in this and every picker (D233 pt 3).

**Reopened and reclosed again 2026-08-06** by feedback `2026-08-06-palette-two-columns`: the
list showed each verb's description and never its name. PAL-08 gives `picker.Item` a `Name`
column, seeded with the id the command already has elsewhere — `ns.switch`, `delete` —
matched as well as shown (**D237**). Two standing constraints come with it: the Label is
still the identity a pick resolves by, and a picker row is truncated, never wrapped, since
the row that wraps costs the modal its last item.

### Credential-plugin auth (AUTH — feedback-driven, D195) — closed at AUTH-07
Feedback `2026-08-01-eks-sso-reauth`: an expired AWS SSO session surfaced as a nameless auth
failure, leaving the user to work out that the fix was `aws sso login --profile x` in another
terminal. Provider-specific auth is not a non-goal, but the shape was fixed up front — detect
narrowly, offer, **never run unasked**, and only from a command the kubeconfig's own
`user.exec` stanza substantiates (D195 pt 4/5), which binds any later leg here. **Closed** at
AUTH-05b: the plugin behind a context is named and its failure classified (D195), re-run once
as a diagnostic to capture its stderr (D211), an expired SSO session recognised by profile
(D212), the diagnosis rendered onto the browse pane over seven distinct cases (D213/D214),
and the remediation offered in one confirm per occurrence and run in the suspended terminal
before the failed request is retried (D215/D216). Per-slice history: `vault/journal/`.

The live claim — the suspend into a real `aws sso login`, and the retry after it — is item 7
on `vault/human-tasks/2026-08-02-conversion-webhook-reason-dogfood.md` (advisory, blocks
nothing).

**Reopened 2026-08-06** by feedback `2026-08-06-auth-error-breaks-layout` (an auth failure
*distorts* the UI rather than rendering in it) and **closed again 2026-08-07**: AUTH-06
sanitized what kubecom renders (D232), AUTH-07 took fd 2 away from everything else —
`internal/stderrfd` dup2s it onto the log for the life of the TUI and `Model.suspend` lends
it back for exactly the length of a handover (D247). The standing constraints are D247's
five points; the one worth repeating here is that reassigning the `os.Stderr` *variable* is
never the fix, since client-go captures it when the authenticator is built and a child
inherits the descriptor.

- [x] **AUTH-07** fd 2 points at the log for the life of the TUI, and at the terminal only inside a suspend — done 2026-08-07 (D247)

### Live table freshness (AGE — feedback-driven, D234) — closed at AGE-01
Feedback `2026-08-06-age-column-stale`: an open pane's AGE column drifts stale. **Closed** at
AGE-01: every cell of a server-printed Table is rendered once, when the server answers, so an
object nothing modifies emits no delta and its age is pinned to when it was listed. AGE is now
re-derived on the client from the row's own `creationTimestamp` with the printers' own
`duration.HumanDuration`, on an unconditional one-second tick (`internal/kube/age.go`,
`internal/tui/age.go`). Per-leg detail: `vault/journal/`.

The two constraints a later leg must not walk into (**D234** pt 2/4): **AGE is the only cell
kubecom recomputes** — every other column is the server's reading of an object body kubecom
never fetched, so wanting one live means re-listing or watching, not widening this seam; and
**the tick stays ungated** — a generation tag would need restarting in five places, and the
failure it buys is a clock left off, which is the bug that was reported.

### Built-in themes (THEME — feedback-driven, D236)
Feedback `2026-08-06-more-themes`: ship ~10 built-in palettes, Catppuccin among them, and
**check each one's licence rather than assuming MIT**. THEME-01 did the survey — the
candidates, their licence files and the two that are not plain MIT are in
`vault/knowledge/themes.md` — and landed the Catppuccin dark flavors, taking the registry
from 3 to 6. **THEME-02 closed the count** at eleven (D248), **THEME-03 landed the canvas**
(D249), **THEME-04a made kubecom report a canvas that did not arrive** (D250) and
**THEME-04b landed the two light palettes** (D251), taking the registry to thirteen. The
feedback that opened this line is met in full, and the line **reopened at THEME-05**: the
first light palettes exposed a role still mapped for a dark canvas. **THEME-06 then
landed `gruvbox-light`** — the values-only leg that follow-up named — taking the registry
to fourteen (eleven dark, three light) against the criterion D251 pt 1 now states.

Two constraints any later palette inherits (**D236** pt 2, **D249**): **`default` and
`catppuccin-frappe` are one palette under two names** (kubecom's default has always been
Frappé and D169 pt 1 will not let the name move) and it is the registry's only permitted
duplicate; and **no component paints the canvas** — `Theme.Background` reaches the screen
once, as the root `View`'s terminal background, because lipgloss will not re-open an outer
background after a nested reset and half the screen is nobody's component anyway.

- [x] **THEME-02** The five remaining schemes ported; registry at eleven — done 2026-08-08 (D248)
- [x] **THEME-03** `Theme.Background` painted by the root View — done 2026-08-08 (D249)
THEME-04 was **split** (2026-08-08): D249 pt 4 puts the decision — what kubecom does where
it cannot paint — *before* the values, and the two halves are separately green, so the
mechanism is **04a** and the palettes are **04b**. 04a is not speculative work for a palette
that does not exist yet: the same mismatch is live today in the other direction, since ten
dark palettes on a *light* terminal that filters the escape are dark-on-dark right now.

- [x] **THEME-04a** kubecom asks the terminal for its background, warns on a polarity mismatch — done 2026-08-08 (D250)
- [x] **THEME-04b** The light palettes; the guard is coherence, not darkness — done 2026-08-08 (D251)
- [x] **THEME-06** `gruvbox-light` ported; registry at fourteen — done 2026-08-09 (none)

### Context switch warmth (CTX-WARM — feedback-driven, D196) — cancelled (D256 pt 2)
Raised by feedback `2026-08-01-context-switch-keep-state`: switching away from a context
and back pays the full cost again (reconnect, rediscover, re-watch), and the submitter
wants `C` → other → `C` → back to feel like flipping a tab. D196 fixed the shape before
any code retained anything, and gated the retention items on the dogfood's leak checks and
on CTX-WARM-01's measured numbers.

**The line is cancelled, 2026-08-09 (D256 pt 2).** The maintainer accepted the current
switch speed as-is — "current switching functionality is overall good enough and I don't
want to spend more time testing it. We'll ship it like this" — which both waived the
dogfood pts the gating waited on and removed the reason to optimise: **CTX-WARM-02,
CTX-WARM-03 and CTX-WARM-04 must not be started**, and the dogfood must not be re-raised
in any form. CTX-WARM-01's switch timing in the diagnostic log stays landed and is
unaffected.

This line was the **speed** half of "switching should feel like tabs". The other half —
the pane you were on coming back with you — is **CTX-MEM** below, which D240 pt 1 kept
apart from this one and which closed at CTX-MEM-04 (D255).

- [x] **CTX-WARM-01** Triage the warmth feedback; time the switch into the diagnostic log
      — done 2026-08-02 (D196)

### Context pane memory (CTX-MEM — feedback-driven, D240)
Raised by feedback `2026-08-06-context-switch-pane-memory`: every switch resets the pane,
so flipping to another context and back starts over instead of returning you to the
resource you had open. D240 splits it from CTX-WARM before any code remembers anything:
that line is *speed* (live retention, gated), this one is *place*. What comes back with
you is an **address written to the context's own state file** — the same `ContextState`
seam the namespace, menu extras and pins already ride (M4-05/D163) — so the shell still
holds nothing from a context it is not on and `resetCluster` stays unconditional (D196
pt 1). Nothing here is gated on the dogfood: no client, no watch and no row survives a
switch, only a GVR.

CTX-MEM-02 landed the address itself and the two ends it attaches to (**D243**): the
write is in `watchResource` once the watch is live, so every browse surface records
through one point and a refused LIST records nothing; the replay is in `handleDiscovery`
after `Reconcile`, so a remembered CRD resolves; and the attempt is single and loses every
tie — to a reader who drilled in first, and to a second pass on the same cluster.

**CTX-MEM-04 then closed the drill-in deferral (D240 pt 6, D255):** a remembered
drill-in now records the *owner* beside the child kind, and the restore re-enters the
scope by re-resolving the owner through the ChildResolver — never replaying the
selector. The owner-gone case is the legibility D240 pt 6 waited for: it lands on the
plain child list and says the owner is gone on screen.

- [x] **CTX-MEM-01** Triage the pane-memory feedback into this line — done 2026-08-07 (D240)
- [x] **CTX-MEM-02** Last-browsed kind remembered per context and restored — done 2026-08-07 (D243)
- [x] **CTX-MEM-04** The drill-in scope comes back as a drill-in — re-resolved through the ChildResolver, never replayed, and the owner-gone case lands on the plain list with a notice saying so — done 2026-08-09 (D255)

### User-facing docs (DOC — feedback-driven)
Raised by feedback `2026-08-07-readme-structural-rewrite`: the README grew by accretion —
587 lines, `## Usage` alone spanning ~290 of them under one flat heading, install ahead of
a single concrete thing kubecom does, and reference material (config, theme, menu file,
migration) interleaved with the walkthrough. The ask is structural: organise around the
**capability surface**, make every capability findable by skimming headings, put install
after the payoff, push reference to `docs/`, and end up shorter. The feedback itself asks
for the split — outline first, prose after — because a 587-line rewrite in one commit is
not reviewable.

- [x] **DOC-01** README restructured around the capability surface; install/config out to `docs/` — done 2026-08-07 (D241)
- [x] **DOC-01b** Prose pass over the restructured README — the prose half of DOC-01's feedback; renamed from `DOC-02` by BOARD-03, whose commits still carry the old id — done 2026-08-09 (D267)

The prose half closed this feedback on 2026-08-09, and it was indexed as `DOC-02` — the id
the agent-found install-docs leg then took three days later, which is the collision BOARD-03
resolved. It is `DOC-01b` here because the two halves are one item under the board's own
a/b convention, and because the alternative — renaming the newer leg — would have orphaned
D266, an M5 exit criterion and a human-task update instead of one journal entry (D267).

DOC-02 was agent-found at Orient rather than raised by feedback: the `v1.0.0-rc.1` tag
landed on 2026-08-10 and the install docs went on saying "no version has been tagged yet"
for two days, because cutting a tag changes what the install instructions *mean* without
touching a line of install code — the shape D68's "if a leg changes install/…" trigger
misses. D266 widens the trigger to cover publishing and fixes the docs on a rule that
survives the next tag: name the version that exists, not the state of the tag list.

- [x] **DOC-02** The install docs name the release that exists — README and `docs/install.md` document `v1.0.0-rc.1` and installing it by name instead of saying none is tagged, the archive path leads, `cd kube-commander` is fixed, and the M5 CI-artifacts criterion is ticked on the rc run — done 2026-08-12 (D266)

### Board hygiene (BOARD — agent-found)
The board is read on every Orient, so its size is a cost every leg pays and no leg sees.
D102 named the rule (a Done entry is one line) and collapsed the list by hand; it drifted
back anyway, which is what BOARD-01 found and fixed — this time with a guard, so the Done
half is now closed for good (D224). BOARD-02a closed the second half of the same question:
the index below `## Done` is the **canonical** record and a section's `- [x]` is a working
view of it, because a closing line's section collapses and takes its entries with it (D225).

- [x] **BOARD-01** The Done list is one line per entry again, and `make check` keeps it there — done 2026-08-05 (D224)
BOARD-02 was **split on pickup** into the two unrelated questions its notes had folded
together: **BOARD-02a**, the `## Done` index's half-appended state — a mechanical question
with a checkable answer — and **BOARD-02b**, the planning prose, which is the judgement
call D224 pt 2 refused to authorise. They are done in that order because the index question
gates nothing and the prose question is the one that can go wrong.

- [x] **BOARD-02a** The `## Done` index is canonical, complete again, and guarded — done 2026-08-06 (D225)
BOARD-02b was **split on pickup**, because its own last sentence sequences the work and the
two halves answer to different standards: "do not touch a paragraph that names a constraint no
decision records — **move it first**". Moving is mechanical and checkable (**BOARD-02b-1**);
deciding what narrative is left worth losing is the judgement call D224 pt 2 declined to
authorise in advance (**BOARD-02b-2**).

- [x] **BOARD-02b-1** Harvest the work the prose defers into real Backlog items — done 2026-08-06 (D226, D227)

The harvest came back **7 deferrals, 1 item**: two of AUTH's were already false (one never
true, one closed by HINT-02 four days later), three want a human to ask, one is declined with
reasons, and one — **PAL-06**, filed under the PAL line — was a real unblocked item that three
consecutive legs reported did not exist. That is the finding BOARD-02b-2 now has to weigh:
the prose is not merely long, parts of it have quietly stopped being true (D226 pt 1).
- [x] **BOARD-02b-2** A closed line collapses to its outcome, its pointers and its standing answers — done 2026-08-06 (D229)

The judgement came back **yes, for a closed line only, and not as a delete** (D229): the
board already collapses a finished section to one sentence — M1, M2 and M4 read that way —
and the four lines BOARD-02b-1 verified were the ones safe to do it to, because their
deferrals and constraints had already been harvested out. SEARCH, CRD-PIN, PAL and AUTH now
carry their outcome, their `Dnn` join keys and their standing answers in ~16 lines each
instead of ~55; the board is 71KB, down from 82KB. The other four closed lines (LOGS, DIAG,
HINT, BOX) have never been swept, and D226 pt 1 forbids compacting a paragraph whose claims
nobody has checked — that sweep is **BOARD-02b-3**, and it is the same shape as 02b-1.
- [x] **BOARD-02b-3** Sweep and collapse the four closed lines 02b-1 did not reach — done 2026-08-06 (D230)

The sweep came back **one stale claim and one real defect**: HINT's paragraph still said
nothing enforced the hint-context completeness, three lines above the entry saying HINT-05
enforced it, and checking LOGS' "closed on cost" against the code found that nothing bounds
the logs buffer at all — filed as **LOGS-07** (D230), the second consecutive harvest to turn
up a product item the prose had been hiding. DIAG's fourteen lines of D191 pt 1 and BOX's
paragraphs collapsed as written. The four sections are ~15 lines each instead of ~36. **The
BOARD line is closed**: the Done list is one line per entry and guarded (D224), the index is
canonical and guarded (D225), a deferral names its destination and is guarded (D226), and a
line collapses its own section when it closes (D229, unguarded on purpose).

**Reopened and reclosed 2026-08-12 at BOARD-03** (D267), on the one property the index guards
had assumed rather than checked: that an **ID** names one leg. It had stopped being true —
two legs were indexed as `DOC-02` — and `TestBoardDoneIndexIsComplete` was structurally
unable to see it, because it collects ids into a `map[string]bool` where a duplicate is
indistinguishable from the entry it collides with, and one entry's presence satisfies
completeness for *both* working-area lines. The collision is resolved (the prose pass is
`DOC-01b`) and uniqueness is now its own guard. The standing constraint is D267: the id is
the join key, so when a collision has already been committed the fix renames the side with
fewer references and records the id its commits carry — never a rewrite of pushed history.

- [x] **BOARD-03** A Done entry's **ID** is unique — the `DOC-02` collision resolved to `DOC-01b` and guarded by `TestBoardDoneIDsAreUnique` — done 2026-08-12 (D267)

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

**The drill-down line (M4-07/08) is closed** as of M4-08/D166: the scope primitive and
its consumer both landed, so `P` on an owner row switches the browse table to that
owner's pods under a live server-side selector and `esc` returns to it.

**The switcher line (M4-01…05) is closed** as of M4-05/D163: what a switch rebinds is now
the cluster's client *and* everything keyed by the context (menu extras, remembered
namespace, state file). The M4 exit criterion stays unticked on purpose — it claims a live
rebind against a second real cluster, which is the standing dogfood human-task (D79).

**The metrics line (M4-09/10) is closed** as of M4-10/D168: a browse table on a measured
kind grows CPU/MEMORY columns fed by a 10 s poll and joined onto the watched rows by
namespace/name; a cluster with no metrics API shows nothing and says nothing.

**M4-12 was split on pickup**, as its own notes allowed, into **M4-12a** (the config
field applied at launch — no live restyle needed, the shell is built with the resolved
theme) and **M4-12b** (the picker, the write-back and therefore the live restyle: a
`SetStyles` on every component that caches a `styles.Styles` at construction). The
criterion claims selection *and* persistence, so it stays unticked until 12b.

**M4-12b was split on pickup**, as its own notes and the M4-12a journal both predicted,
into **M4-12b-1** (the live restyle: `SetStyles` across every component, plus the shell
fan-out that applies one) and **M4-12b-2** (the `theme.switch` action, the picker over
`styles.Themes()` and the write-back to `config.yaml`) — the D52 bottom-up rhythm the
whole M4 switcher line followed: the mechanism lands and is tested before any gesture can
reach it, exactly as M4-04a preceded M4-04b. Restyling is the larger and riskier half:
eleven components cache a `styles.Styles` and three of them *derive* from it at
construction, so a plain field assignment is a silent half-restyle.

_(none — M4 is **feature-complete** (2026-07-30): every slice M4-01…M4-12b-2 is landed
and every exit criterion in [`../milestones/M4-capabilities.md`](../milestones/M4-capabilities.md)
is ticked but the context switch, which waits on its two-cluster dogfood human-task (D79).)_

### M5 — Release & docs
M5 ships v1: documented, packaged, installable — expanded here into ordered, leg-sized
slices (M5-PLAN, D52/D173). M5 differs from every milestone before it in one way that
shapes the whole plan: **its output leaves the repo and cannot be recalled.** A pushed
`v1.x.x` tag is cached immutably by the Go module proxy and mirrored by distributors, so
"green, then revert" — the safety net every previous leg relied on — does not exist here.
D173 therefore draws the line: an agent leg prepares and dry-runs everything, and a human
performs each act that publishes (the tag, and each distributor's first push). Ordering is
bottom-up as usual (D52): audit what is actually done, make the artifact *correct*, then
make it *publishable*, then publish. Re-split any slice that proves > ~300 lines.

Verification without credentials is `goreleaser release --snapshot --clean` — no tag, no
secrets, no network publish (D173 pt 4). Neither `goreleaser` nor `vhs` is in the sandbox
image; goreleaser is a Go tool (`go install github.com/goreleaser/goreleaser/v2@latest`)
so a leg can likely obtain it, and if it cannot, the gate for a config-only slice is the
CI dry-run job M5-03 adds rather than a claimed-but-unrun command (D79).

_The **release** half of M5 is agent-done: M5-10's share is finished and M5-11 is in
**Blocked** above, waiting on the tag. The release-namespace fold-in **HT-relns-0809**
closed 2026-08-09 (D253/D254): Homebrew publishes to the new org tap
`neuroplastio/homebrew-tap` (created this leg; kubecom its first tool) and container builds
are dropped, so the distribution surface is the cask + the AUR package, both inert until
their secrets exist. Every remaining release act publishes, and D173 pt 1 makes each one a
human's._

**The tag is no longer the next thing.** On 2026-08-15 the maintainer named four items that
gate `v1.0.0` (**UX-PLAN**, D268), and they add a **UX-validation line** in front of the
release slices: the rc proved the *pipeline*, and what is unproven is the *product* — no
user path through kubecom has ever been walked deliberately end to end and judged. The line
below builds the instrument (a committed cluster fixture + a keystroke log), writes the
paths down as **stories**, has the maintainer walk them, and folds the findings back in;
the tape and the docs are re-cut against the main story once it exists rather than before.
Order is bottom-up as usual (D52): fixture and instrument first, then the stories, then the
run, then everything that quotes them. Re-split any slice that proves > ~300 lines.

- [x] **STORY-03** Analyse a trace: `kubecom keys analyze <trace.jsonl>`
      status: done | owner: claude-opus | added: 2026-08-15 | done: 2026-08-15
      notes: See the M5 backlog entry above. The four findings a UX pass asks are the
      report: unresolved presses ranked by frequency (the dead ends, keyed by key+mode),
      action counts, the longest pauses (the gaps between presses, each tied to the press
      that finally came), and the abandoned sequences (a run of pending presses that never
      resolved — judged against the resolved keymap, so a completed `gg` is not a finding).
      The read side lives in `internal/keylog/analyze.go` beside the writer, and the
      command rides `kubecom keys`.

- [x] **STORY-04** Write the stories: `stories/*.md`, one user path each
      status: done | owner: claude-opus | added: 2026-08-15 | done: 2026-08-15
      notes: See the M5 backlog entry below. The five stories are `stories/s01…s05`
      (S02 the **main story**, one of five `Main story: yes`), each enforcing its shape —
      the situation, the goal, what to report — and its one hard rule, never naming a key,
      through `internal/stories` guards, and the maintainer's ask folded in: `CONTRIBUTING.md`
      now makes **writing a story the way to argue for a UX change**, with the trace the
      evidence and a dead-end press the finding.

- [x] **STORY-05** Human task: walk every story against a fresh fixture, hand back the traces
      status: done | owner: maintainer | added: 2026-08-15 | done: 2026-08-15
      notes: Depends on STORY-01..04. This is the act the whole line exists for and it is a
      human's (D79): judging a UX needs a person with intent, and the maintainer is the only
      one who can run the binary against a real terminal. **Filed 2026-08-15 as
      `vault/human-tasks/2026-08-15-walk-the-stories.md`** (STORY-03 landed the last
      prerequisite, the analyzer), carrying the traces + freeform reactions back.
      **Walked 2026-08-15**: all five stories against the fresh fixture (`~/traces/s01…s05.jsonl`,
      run with the clean-user-dir `run.sh`), verdicts in the human task's `## Result`; the walk
      returned **23 feedback files** in `vault/feedback/` — the fold-in's raw material.
      → milestone: M5 · knowledge: decisions.md D268

### Story findings fold-in (STORY-06 — D268)
The 23 feedback files the S05 walk returned, folded in as per-finding slices. The order is
bottom-up as D268 pt 3 suggests: the **keymap redesign first** (it supersedes the individual
key items and resolves the most dead ends — `2026-08-15-keymap-redesign.md` is the master
spec and was deleted at this triage; the remaining S-interaction and enter→menu designs are
carried by 06b/06c below), then the picker navigation mode (the walk's sharpest dead end),
then the instrument fixes (so the next walk's analyzer sees what this one's missed), then the
feature legs. Re-split any slice that proves > ~300 lines. **06a (the letter remap) landed
2026-08-15 (D269)**; 06b and 06c are the interaction halves the redesign's spec still owes.

- [x] **STORY-06a** The letter remap — search `ctrl+f`, ns.switch `N`, delete `D`, describe `d`, the two displaced homes (`#` for previous-match, `space` for full-page-down), and `V` selects log lines; help + keybindings doc + tape updated — the letters half of `keymap-redesign` (D269)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-15
      notes: The **collisions the redesign names are resolved here**: `app.searchPrev` (was `N`) takes `#` (vim's backward-occurrence gesture, out of the n-family entirely) and `nav.pageDown` (was `ctrl+f`) takes `space` (vim's `<space>`-scrolls-a-screen, pairing the kept `ctrl+b` pageUp). `logs.select` gains `V` (both `v` and `V` select — `2026-08-15-log-line-selection-shift-v.md`). Deletes the master `keymap-redesign.md` plus the items it supersedes — `context-switch-key-inconsistent` (N answers it), `s-key-family-confusion` (search leaves the s-family), `sort-column-picker` and `enter-actions-menu` (their designs live in 06b/06c now) and `namespace-switch-inconsistent` (both paths already open the one `:namespace ` stage, PAL-05c-1; the capital N is the ergonomics). The keys that are *interactions*, not letters, stay put until their slices: `s`/`S` sort until 06b, `a`/`enter` actions-menu until 06c. Also clamped the help overlay's width — bubbles/help's full layout dumps every column when its own ellipsis cannot fit, which the narrower keys tripped (new `elide.Width`).
      → milestone: M5 · knowledge: decisions.md D268

- [x] **STORY-06b** Sort by column-header focus — `s` freed entirely, `S` focuses the table's column-header row, `h`/`l`/`left`/`right` move across columns, `enter` toggles direction, `esc` returns focus to the rows, and `sort.clear` lives inside the mode (a "clear" pick)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-15
      notes: The S-mode half of `keymap-redesign.md` (deleted at 06a) and the full design of
      `2026-08-15-sort-column-picker.md`. Supersedes the popup design that file proposed.
      The       mode landed as a header-focus interaction (D270): `S` focuses the header row,
      `h`/`l`/`left`/`right` move the cursor across the visible columns, `enter` toggles the
      sort direction (SortBy toggles), `esc`/`S`/`q` return to the rows, `x` clears — and
      the clear resolves the mode too, so `x` lands back on the rows with the sort gone —
      the mode is a capturing surface with its own `HelpSort` hint context (declared, named,
      set, reachable), and `s` is freed entirely. `sort.clear` keeps working everywhere a
      table is showing so the palette verb and the outside-the-mode clear are preserved.
      `docs/keybindings.md` regenerated (sort.column `S`, sort.clear `x`); the table's sort
      mode reuses the existing SortBy/ClearSort state (cursor is the only new field),
      reveals the cursor through the existing horizontal-scroll machinery, and clamps it on
      a RESET/delta that shrinks the columns. `TestSortModeFocusesHeaderAndMovesAcrossColumns`,
      `TestSortModeClearProvesTheClearPick`, `TestClearSortKeyRestoresOrder` and the
      `HelpSort` reachability entry pin the interaction.
      → milestone: M5 · knowledge: decisions.md D270

- [x] **STORY-06c** `enter` opens the actions menu on a resource row — drill-in moves inside the menu (an entry in it), so drilling into an owner's pods is one menu pick away; `a` frees up
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-16
      notes: The enter half of `keymap-redesign.md` (deleted at 06a) and the ask of
      `2026-08-15-enter-actions-menu.md`. `enter` stays the universal accept key on modals,
      prompts, pickers and confirms — only the resource-table context changes. Landed as a
      **third key context** (ctxTable, D271): `actions.menu` binds `enter` there, the shell
      consults `TableAction` first while the resource table owns the keys (gated on the same
      ladder the hint uses, `hintContext() == HelpTable` — an open modal/logs/viewer/forwards/
      help/sort-mode surface keeps its own enter, and a pending `g` of `gg` still owns the
      press), and `enter` keeps its browse meaning (`nav.drillIn`) on the menu pane, in
      pickers and on modals — the D132 context-split pattern. The table's old `enter`→
      `RowSelectedMsg` dead end is gone: the actions menu opens instead, and drill-in (Show
      pods) is an entry in it, so drilling into an owner's pods is one menu pick away. `a`
      frees up (pinned unbound in the keymap test); `:` and the `:action ` verb still reach
      the menu; `docs/keybindings.md` regenerated; the `openActionStage` test helper now
      presses `enter`.
      → milestone: M5 · knowledge: decisions.md D271

- [x] **STORY-06d** Picker navigation mode — in a pre-launched pane, `j`/`k` navigate the list, `/` starts filtering, and the current choice is preselected — the walk's sharpest dead end (`2026-08-15-picker-navigation-mode.md`, 4 dead `j`s the analyzer could not see)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-16
      notes: Landed as D272: a picker's `Show()` opens in **navigation mode** — list
      focused, j/k navigate, `/` opens the filter — flipping D194 pt 2 for value
      pickers. The command palette's verb list is the one type-to-filter surface left
      (`ShowFiltered`), the palette's argument stages open in navigation mode with the
      current choice preselected (namespace/context/theme, `picker.SelectValue`),
      `OpenFilter`/`CloseFilter` drive the in-place verb→arg→verb transitions, and
      backspace-rewind still works from a navigation-mode stage. The port picker's old
      opt-in construction and the `WithOptInFilter` option are gone; `picker.New` takes
      no options. Help/hint/README updated; the feedback file deleted (D69).
      Pairs with the `2026-08-15-resources-pane-filter.md` slice below in spirit (both make a pane's own list the target).
      → milestone: M5 · knowledge: decisions.md D272

- [x] **STORY-06e** The instrument sees what it missed — the recorder records a resolved confirm accept/decline (so a handled `esc` is not a dead end, `2026-08-15-confirm-key-false-deadends.md`) and the analyzer reports text-surface presses separately (so the picker `j`s are findings, `2026-08-15-analyzer-text-surface-blindspot.md`)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-16
      notes: The two mirror-image instrument fixes; the walk's two dead-end classes were each invisible to the other half of `keys analyze`. Landed as D273: the recorder writes the confirm modal's resolved action (ConfirmAction → confirm.accept/decline, shared `confirmResolved` with the router) so a handled `y`/`n`/`enter`/`esc` reads as the action it ran, not a dead end — while an unhandled confirm key stays a dead end (the modal is not a text surface); and `keys analyze` gains a "presses on text surfaces" section ranking `Text` presses that resolved to nothing, so the picker `j`s of S01 are a finding rather than silently skipped. The analyzer's dead-end predicate is unchanged; text presses just tally into their own bucket. Both feedback files deleted (D69).
      → milestone: M5 · knowledge: decisions.md D268, D273

- [ ] **STORY-06f** A dedicated `events` action — the selected resource's events (kind, reason, message, age) as its own list, the surface for "why is this red" instead of hunting through describe (`2026-08-15-events-action.md`)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-16
      notes: Landed as D274: a new `kube.Clients.Events` primitive lists the object's own core Events as a server-printed Table (the `kubectl get events` columns, no hard-coded columns) filtered by involvedObject UID (name fallback, empty name rejected), and `res.events` (`E`, gated on the kind's get verb) opens it in the shared viewer — the columns padded to their widest cell except the last, which flows. `EventLister` seam on the Cluster bundle; empty list says "(no events)", error degrades to a toast (D74); README + generated keybindings doc updated. Verified live against the story cluster's crash-looping pod.
      → milestone: M5 · knowledge: decisions.md D268, D274

- [x] **STORY-06g-1** The unhealthy filter on the current table — `H` (shift+h, the feedback's own suggestion) toggles the browse table to show only rows the M4-06 classifier reads as unhealthy (CrashLoopBackOff, ImagePullBackOff, Pending/unschedulable, not-ready, a stuck claim), one keypress from anywhere to "what's broken, filtered" for the kind you're on
      status: done | owner: opencode | added: 2026-08-16 | done: 2026-08-16
      notes: Landed as D275: `app.unhealthy` (`H`, shift+h — the feedback's own "healthy vs h") narrows the current resource table to the rows the M4-06 classifier reads as unhealthy — a row any visible cell classifies to warn/error (CrashLoopBackOff, ImagePullBackOff, Pending/unschedulable, not-ready, a stuck claim). It composes with the `/` substring filter (a row must survive both), resets on a new resource exactly as the substring filter does, and `esc` clears it as "show me everything again". The status bar shows an `unhealthy` marker while the view is on, so a narrowed table never reads as an empty one; the palette lists the verb too. Feedback `2026-08-15-unhealthy-workloads-quick-access.md` deleted (D69). The cross-kind sweep that makes a non-pod failure findable from anywhere is 06g-2.
      → milestone: M5 · knowledge: decisions.md D268, D275

- [x] **STORY-06g-2a** The cross-kind unhealthy sweep primitive — `kube.Scan` fans out a concurrent, capped, fault-isolated list across kinds keeping rows a caller-supplied `RowFilter` accepts (the M4-06 health predicate is a display concern, so the primitive takes it as a seam), streaming ScanMatch/ScanKindDone/ScanDone events like Search — the bottom-up first slice of STORY-06g-2
      status: done | owner: opencode | added: 2026-08-16 | done: 2026-08-16
      notes: First half of STORY-06g-2, split on pickup (D52's primitive-before-surface rhythm, the M4-07→M4-08 / SEARCH-02a→02b shape). The feedback file `2026-08-15-pod-first-blinds-non-pod-failures.md` stays with the surface that completes the address (06g-2b, D69). The predicate is `func(*Table, Row) bool` so kube never learns the M4-06 classifier (color.go); a hit carries the row cells so the surface can render the reason without re-listing.
      → milestone: M5 · knowledge: decisions.md D268, D275, D276

- [x] **STORY-06g-2b-1** The cross-kind unhealthy list component — `components/unhealthyview`, a full-screen streaming list of `kube.ScanHit`s (kind · name · namespace · the offending cell), navigable with a cursor, with a header tracking the scan's progress/cap — the surface in isolation, before the gesture that opens it (`2026-08-15-pod-first-blinds-non-pod-failures.md`)
      status: done | owner: opencode | added: 2026-08-16 | done: 2026-08-16
      notes: First slice of STORY-06g-2b, split on pickup (D52's component-before-wiring rhythm, the SEARCH-02a→02b shape). Landed as D278: the view is a full-screen list of ScanHits with no query field (the sweep runs once on open, not per keystroke), navigation and drill-in/back as keymap actions, `SelectedMsg`/`ClosedMsg` (D56). The hit needs the table's columns to render the reason, so `kube.ScanHit` gained `Columns` and the table package exports `UnhealthyRow`/`UnhealthyCells` — the M4-06 classifier lifted to a whole table, the concrete seam D276 named: the sweep's filter, the reason a hit shows, and the per-kind `H` filter all run the same `classifyCell`, so the cross-kind list and the per-kind filter cannot disagree about what is broken (D275 pt 1). The wiring — the action + key + Scanner seam + the drill-in that switches browse to a hit — is 06g-2b-2, which deletes the feedback file when it lands (D69). The component runs no scan itself and knows nothing about clients, exactly like SEARCH-02a's search view.
      → milestone: M5 · knowledge: decisions.md D268, D275, D276, D278

- [x] **STORY-06g-2b-2** The wiring: the cross-kind unhealthy gesture — a keymap action that opens the unhealthyview and runs `kube.Scan` over the menu's kinds with the M4-06 row predicate, pumping hits in, each drill-in switching browse to that resource (`2026-08-15-pod-first-blinds-non-pod-failures.md`)
      status: done | owner: opencode | added: 2026-08-16 | done: 2026-08-16
      notes: Landed as D279: the `Scanner` seam mirrors `Searcher` (on the Cluster bundle, WithScanner, nil → scan-inert), `app.unhealthyScan` (`U`, the capital Unhealthy partner to the per-kind `H`) opens the view and runs exactly one sweep over `availableResources()` — every kind the menu offers, the whole point being that the broken thing may be a kind the operator is not looking at — in the app's own namespace, capped at `scanHitLimit` = 200. The pump mirrors search's (scanGen/scanCancel/scanCh, D140 pt 3), `stopScan` joins the `stopClusterAsync` inventory, drill-in closes the list and switches browse via selectResource with the hit's object stashed as the shared pending selection (the search fields, consumed by applyPendingSelect), the view routes like the logs view with its grep closed (handleAction intercepts, `q` closes it), and it gets its own `HelpUnhealthy` hint context (D143 pt 1). README + keybindings doc updated; the feedback file deleted (D69).
      → milestone: M5 · knowledge: decisions.md D268, D275, D276, D278, D279

- [ ] **STORY-06h** Rich describe panel — the describe view fully replaces the right pane and paints the diagnosis with color: phase/status/conditions in theme-aware colours, problem states emphasized (`2026-08-15-rich-describe-panel.md` + `2026-08-15-describe-replaces-right-pane.md`)
      status: todo | owner: — | added: 2026-08-15
      notes: Reuse the describe data, re-render it styled; pairs with STORY-06f so "why is this red" is one glance.
      → milestone: M5 · knowledge: decisions.md D268

- [ ] **STORY-06i** The relations popup — on any resource, one gesture lists its parents, children and linked resources (owner, selector-matched services/pods, claims/volumes), each row navigable — the reverse of `res.children`, generalised to every kind (`2026-08-15-relations-navigation-popup.md`)
      status: todo | owner: — | added: 2026-08-15
      notes: The owner-address bookkeeping from CTX-MEM-04 already records half of it; the fold-in decides the gesture and the relation graph's exact shape.
      → milestone: M5 · knowledge: decisions.md D268

- [x] **STORY-06j-1** A painted follow indicator — the header renders `[following]` as a badge (canvas ink on the palette's Success green on a dark canvas; bold alone on a light one, D252 pt 3's rule), paused stays plain — so live vs frozen is unmistakable at a glance (`2026-08-15-logs-follow-visual-signal.md`)
      status: done | owner: opencode | added: 2026-08-16 | done: 2026-08-16
      notes: Landed as D280: the new `styles.Follow` role paints the `[following]` token (bold everywhere; canvas-on-Success on a dark canvas, 4.69–11.03:1 measured; no paint on a light canvas where no shade clears the floor — the Match/D252 pt 3 rule, gated by IsDark), while `[paused]` stays plain Header text. The header clips first, then composes the badge mid-line with the neighbours re-rendered through Header so the badge's reset does not strand them in the terminal default. `TestFollowBadgeIsDistinguishable` holds every built-in to the rule; `TestFollowStateBadgeAndPausedPlain` pins live-paints / paused-plain. README's logs paragraph names the badge. Feedback `2026-08-15-logs-follow-visual-signal.md` deleted (D69). 06j-2/06j-3 split back to Backlog.
      → milestone: M5 · knowledge: decisions.md D268, D280

- [x] **STORY-06j-2** Scrolling past the last log line re-arms follow — the same re-arm `G` gives, so a reader who scrolled back down to the newest line is following again without a keypress (`2026-08-15-logs-scroll-past-end-resumes-follow.md`)
      status: done | owner: claude-opus | added: 2026-08-16 | done: 2026-08-20
      notes: Landed as D281: every downward navigation (`nav.down`, `nav.halfPageDown`, `nav.pageDown`) routes through a new `scrollDown`, which re-arms following iff `atNewest()` — paused, not selecting, non-empty body, cursor already on the last shown line. Landing on the newest line stays browsing (a page-down that overshoots from above only lands there); the *next* press — vim's no-op — is the "and keep going" that in a pager over a live stream can only mean rejoin it, so it does exactly what `nav.bottom` does. Supersedes D147's downward half; `TestDownwardScrollDoesNotResumeFollowing` is replaced by the paused-on-landing / re-armed-past-the-end pair plus a visual-mode guard (D242 pt 5). README's logs paragraph names the gesture; feedback file deleted (D69).
      → milestone: M5 · knowledge: decisions.md D268, D281

- [ ] **STORY-06j-3** The newest-first log order weighed — grafana/datadog style, newest line at the top with the filter box also at the top — a design to consider, not a demand; lands with follow-rearm (06j-2) or not at all (`2026-08-15-logs-newest-first-order.md`)
      status: todo | owner: — | added: 2026-08-16
      notes: Third slice of STORY-06j. Weigh the trade-off (it inverts `G`-to-bottom muscle memory and what "scrolling down" means) against the follow-rearm work; record the fold-in decision, and only then decide whether to implement.
      → milestone: M5 · knowledge: decisions.md D268

- [x] **STORY-06k-1** Single enter reaches a search hit — `enter` opens the highlighted hit from the query line itself, and the hand-off into the result list (where `hjkl` navigate) moves to the arrow that already meant "down the list" (`2026-08-15-search-single-enter.md`)
      status: done | owner: claude-opus | added: 2026-08-20 | done: 2026-08-20
      notes: Landed as D282: `nav.drillIn` opens the highlighted hit whatever the focus (and emits nothing with no hits), superseding D235's commit-then-open half, while every cursor action routes through a new `enterResults` that hands the keyboard to the list on the first movement over it and then moves — so the mode that makes `hjkl`/`g`/`G`/the page chords navigate is entered by using it, needs no key of its own and nothing new in the hint. The hand-off refuses an empty list (focus stays on the query); `nav.back` is unchanged and is now the only action here that reads the focus. The test helper commits with `nav.up` (nothing above the top hit, so only the focus changes); README's cluster-search paragraph names both gestures. Feedback `2026-08-15-search-single-enter.md` deleted (D69); the preview is 06k-2.
      → milestone: M5 · knowledge: decisions.md D268, D282

- [x] **STORY-06k-2** Search result preview — the highlighted hit's identity/snippet shown inside the search view, so the right row is picked before the view closes (`2026-08-15-search-result-preview.md`)
      status: done | owner: claude-opus | added: 2026-08-20 | done: 2026-08-20
      notes: Landed as D283: `kube.SearchHit` now carries the printed row it was matched in (`Columns`/`Cells` — free, the search already listed it; supersedes D276 pt 2's "a search hit carries no cells" rationale, drilling in still re-lists), and the view renders a two-line footer under the results describing the highlighted hit: identity (kind · apiVersion · namespace/name) over the object's own cells as `COLUMN: value` pairs, NAME and empty cells dropped, short rows indexed defensively. `previewHeight` is reserved unconditionally so rows never shift when the first hit lands; a kind that printed no cells keeps the height with a blank line; no hits, no block. A preview stays request-free — a fetch belongs to opening the object. README's cluster-search section names it; feedback `2026-08-15-search-result-preview.md` deleted (D69). STORY-06k is closed.
      → milestone: M5 · knowledge: decisions.md D268, D283

- [ ] **STORY-06l** The default panel — land on the Pods table instead of the welcome page (`2026-08-15-land-on-pods-by-default.md`) and hide custom resources by default, shown only once pinned (`2026-08-15-hide-custom-resources.md`)
      status: todo | owner: — | added: 2026-08-15
      notes: Two startup defaults; the welcome page stays reachable and the pin gesture already exists (`menu.pin`).
      → milestone: M5 · knowledge: decisions.md D268

- [x] **STORY-06m** `/` filters the focused pane — with the resources pane focused it narrows the kinds there; with the table focused it keeps filtering the table (`2026-08-15-resources-pane-filter.md`)
      status: done | owner: opencode | added: 2026-08-15 | done: 2026-08-16
      notes: Landed as D277: `app.filter` (`/`) targets whichever pane holds focus — the menu gets its own filter view (`menu.SetFilter`/`Filter`/`ClearFilter`) over the authoritative `full` kind set, matching the D203 alias surface (title/kind/plural/short-name/group) so `deploy` finds Deployments, the three mutators (Reconcile/addExtras/Unpin) write `full` and re-derive the displayed `items` so a narrowing never loses a kind, and the target is fixed at open (`app.menuFilter`); `Items()` keeps returning the full list so search/pickers/pane-memory never shrink with the pane. `HelpMenu` advertises `/`; the open field's hint is the shared filter context. Feedback `2026-08-15-resources-pane-filter.md` deleted (D69).
      → milestone: M5 · knowledge: decisions.md D268, D277

- [ ] **TAPE-01** Re-cut `docs/screencast.tape` around the main story
      status: todo | owner: — | added: 2026-08-15
      notes: Depends on STORY-04 (the main story) and lands after STORY-06 so the GIF shows
      the refined UX, not the one the stories found fault with. Today's tape is a **feature
      tour** — palette, filter, logs, describe, search, theme, help — assembled before any
      user path existed; the maintainer wants it re-cut so it walks the main story instead.
      The three guards in `internal/tui/keymap/screencast_test.go` (annotated keypresses
      checked against `DefaultKeymap`, headline actions pressed, README/GIF agreement) hold
      across the re-cut and the headline list may need revising with it. The tape also stops
      depending on an ad-hoc cluster once STORY-01 lands. **Re-recording is the maintainer's**
      (D181): it needs ttyd + ffmpeg, a real terminal and the fixture cluster.
      → milestone: M5 · knowledge: decisions.md D268, D181

- [ ] **DOC-03** README becomes a landing page
      status: todo | owner: — | added: 2026-08-15
      notes: Today's README is 212 lines and carries a full feature catalogue ("What kubecom
      can do", six subsections) that duplicates what `docs/` should own. Cut it to what a
      landing page owes a reader: what/why, the screencast, a short install block linking
      `docs/install.md` (DOC-01's shape, keep it), one screen of tour, and links out. The
      install-path drift guards (`TestReadmeBrewTapMatchesTheCask`,
      `TestReadmeAURPackageMatchesTheConfig`, `TestScreencastAssetAndReadmeAgree`) all read
      `README.md`, so the block they check must survive the cut.
      → milestone: M5 · knowledge: decisions.md D268

- [ ] **DOC-04** `docs/usage.md` — the guided tour the README no longer carries
      status: todo | owner: — | added: 2026-08-15
      notes: Depends on STORY-04. The prose walks the **main story**, so the doc, the tape
      and the story tell one story in three media (D268 pt 4). Absorbs the feature catalogue
      DOC-03 cuts, and links `docs/keybindings.md` for the full keymap rather than restating
      keys — a second hand-written key list is a second thing to drift (D51).
      → milestone: M5 · knowledge: decisions.md D268

- [ ] **DOC-05** `docs/troubleshooting.md`
      status: todo | owner: — | added: 2026-08-15
      notes: The failure surfaces exist and are documented nowhere a user looks: the auth
      diagnosis (`internal/tui/authdiag.go`), browse failures, the log file at
      `os.UserCacheDir()/kubecom/kubecom.log`, discovery partial-failure reporting (DISC-01),
      Gatekeeper quarantine on macOS, and the WSL2 note. Best written after STORY-05, which
      is the first time anyone hits these paths without knowing the code.
      → milestone: M5 · knowledge: decisions.md D268

## Done

- [x] **STORY-06k-2** Search result preview — the highlighted hit's identity and its own printed row render under the results, carried on the hit itself, so the right one is picked before the view closes — done 2026-08-20 (D283)
- [x] **STORY-06k-1** Single enter reaches a search hit — `enter` opens the highlighted hit from the query line and a movement over the list is what commits into it, so a result is one press away — done 2026-08-20 (D282)
- [x] **STORY-06j-2** Scroll past the newest log line re-arms follow — every downward nav routes through `scrollDown`, which rejoins the stream when the press is the no-op at the end, so a reader who scrolled back down is live again — done 2026-08-20 (D281)

- [x] **STORY-06j-1** The painted follow indicator — the header renders `[following]` through the new `styles.Follow` badge (canvas ink on Success on a dark canvas, bold alone on a light one), paused stays plain, so live vs frozen is unmistakable — done 2026-08-16 (D280)

- [x] **STORY-06g-2b-2** The cross-kind unhealthy wiring — `U` (`app.unhealthyScan`) opens the view and runs one `kube.Scan` over the menu's kinds with the `table.UnhealthyRow` predicate over a generation-guarded pump, each drill-in switching browse to the hit with its object pending selection, the feedback file deleted — done 2026-08-16 (D279)

- [x] **STORY-06g-2b-1** The cross-kind unhealthy list component — `components/unhealthyview`, a full-screen streaming list of `kube.ScanHit`s (kind · name · namespace · the offending cell in its role's hue) with a sweep header, plus the seams: `ScanHit` carrying the columns and `table.UnhealthyRow`/`UnhealthyCells` — done 2026-08-16 (D278)

- [x] **STORY-06m** `/` filters the focused pane — the menu's kinds or the table's rows, matching the D203 alias surface, the menu gaining its own filter view over the authoritative kind set — done 2026-08-16 (D277)

- [x] **STORY-06g-2a** The cross-kind unhealthy sweep primitive — `kube.Scan` fans out a concurrent, capped, fault-isolated list across kinds keeping rows a caller-supplied `RowFilter` accepts, streaming ScanMatch/ScanKindDone/ScanDone events like Search, with a hit carrying the row so the surface can render the reason without re-listing — done 2026-08-16 (D276)

- [x] **STORY-06g-1** The unhealthy filter on the current table — `H` toggles the browse table to the rows the M4-06 classifier reads as unhealthy (CrashLoopBackOff, ImagePullBackOff, Pending/unschedulable, not-ready, a stuck claim), composing with `/` and cleared by esc, with an `unhealthy` status marker — done 2026-08-16 (D275)

- [x] **STORY-06f** A dedicated `events` action — the selected object's own core Events (the `kubectl get events` columns) as their own viewer list on `E`, filtered by involvedObject UID, the surface for "why is this red" — done 2026-08-16 (D274)
- [x] **STORY-06e** The instrument's mirror fixes — the recorder writes the confirm modal's resolved action (a handled `y`/`n`/`enter`/`esc` is `confirm.accept`/`confirm.decline`, an unhandled key stays a dead end) and `keys analyze` reports text-surface presses in their own section (the picker `j`s of S01 are findings again) — done 2026-08-16 (D273)
- [x] **STORY-06d** Pickers open in navigation mode — list focused, j/k navigate, `/` opens the filter, current choice preselected; the palette verb list keeps type-to-filter, and the port picker's opt-in construction is gone — done 2026-08-16 (D272)
- [x] **STORY-06a** The letter remap — search `ctrl+f`, ns.switch `N`, delete `D`, describe `d`, `#` for previous-match, `space` for page-down, `V` selects log lines; the seven superseded key feedback items deleted and the help overlay width-clamped (`elide.Width`) — done 2026-08-15 (D269)
- [x] **STORY-06b** Sort by column-header focus — `s` freed, `S` focuses the header row, `h`/`l` move, `enter` toggles direction, `esc` returns, `x` clears and resolves the mode; `HelpSort` hint context and the table's cursor land with it — done 2026-08-15 (D270)
- [x] **STORY-06c** `enter` opens the actions menu on the selected row — a table key context (`ctxTable`) binds `actions.menu` to `enter` while the table owns the keys, `a` frees up, and drill-in (Show pods) stays an entry inside the menu — done 2026-08-16 (D271)
- [x] **STORY-05** The maintainer walked all five stories against the fresh fixture and handed back the traces + verdicts — five traces at `~/traces/s01…s05.jsonl` and 23 feedback files in `vault/feedback/` as the fold-in's raw material; the fifth (non-workload) failure was the miss that matters — done 2026-08-15 (D268)
- [x] **STORY-03** `kubecom keys analyze <trace.jsonl>` reads a trace back as the four findings — dead ends ranked, action counts, longest pauses, abandoned sequences — judged against the resolved keymap — done 2026-08-15 (D268)
- [x] **STORY-04** The five stories are written — `stories/s01…s05`, S02 marked the main story, each guard-tested for its shape and for naming no key; `CONTRIBUTING.md` makes a story the way to argue for a UX change — done 2026-08-15 (D268)
- [x] **STORY-02** `--keylog` traces every keypress with the surface and the resolved action — a press resolving to none is the signal, off by default — done 2026-08-15 (D268)
- [x] **STORY-01** The story cluster is committed — `stories/cluster/` rebuilds `shop`/`data`/`broken` clean every run, guarded by `internal/stories` — done 2026-08-15 (D268)
- [x] **UX-PLAN** Expand the maintainer's pre-tag UX/docs scope into STORY-01…06, TAPE-01 and DOC-03…05; the tag now waits on them — done 2026-08-15 (D268)
- [x] **BOARD-03** A Done entry's **ID** is unique — the `DOC-02` collision resolved to `DOC-01b` and guarded by `TestBoardDoneIDsAreUnique` — done 2026-08-12 (D267)

- [x] **DOC-02** The install docs name the release that exists — README and `docs/install.md` document `v1.0.0-rc.1` and installing it by name instead of saying none is tagged, the archive path leads, `cd kube-commander` is fixed, and the M5 CI-artifacts criterion is ticked on the rc run — done 2026-08-12 (D266)

- [x] **RC-PRERELEASE** `.goreleaser.yml` now sets `release.prerelease: auto`, so a pre-release tag (`-rc`/`-beta`) publishes as a GitHub *pre-release* and a stable tag does not; the first `v1.0.0-rc.1` shipped with the default `false` and was re-marked by hand — done 2026-08-10 (journal 2026-08-10.1)

- [x] **MONO-03** The browse filter seam is `components/filter` — a sub-model owning the `/` field's open/query state, the shell keeping the table (the authoritative rows) and performing the narrowing (`table.SetFilter`) — the third cut out of `app.go`, closing the MONO line — done 2026-08-09 (D265)

- [x] **MONO-02** The secret viewer's entry list is `components/secretviewer` — a sub-model owning reveal/mask and the entry cursor, rendering the body from the shell's authoritative `SecretData` at render time, the clipboard copy staying a shell gesture — the second cut out of `app.go` — done 2026-08-09 (D265)

- [x] **MONO-01** The port-forward panel is `components/forwards` — a sub-model owning open/cursor and the BOX-02/HINT-04 geometry, fed read-only `Entry`s while the shell keeps the handles and performs the stops — the first cut out of `app.go` (~220 lines) — done 2026-08-09 (D265)

- [x] **APP-MONOLITH** The plan's `views/` directory is revoked by D264 — full-screen views are `components/*` sub-models (`searchview`, `logsview`, `viewer`), browse is the root shell, and `app.go`'s reduction is the standing MONO-01 item — feedback `2026-08-09-audit-app-monolith` — done 2026-08-09 (D264)

- [x] **FORMAT-GATE** `make check` enforces `gofmt` via the golangci-lint v2 `formatters:` block; the one styles.go drift is fixed — feedback `2026-08-09-audit-format-gate` — done 2026-08-09 (D263)

- [x] **TEST-RUNTIME** Toast auto-clear duration is a `Model` option (`WithToastTimeout`, default 5s); `sized`/`sizedWith` build tests with ~0 so the 17 drain-the-tick tests drop from 5–10s to ms and the tui package from ~190s to ~90s — feedback `2026-08-09-audit-test-suite-runtime` — done 2026-08-09 (D262)

- [x] **M5-09b** Screencast tape tuning — the search demo reuses the filter's `shop` so it cannot come back empty, the tape wipes a throwaway XDG dir so reruns start from the same welcome screen, and the tour adds a theme-preview step and the help overlay with captions — feedback `2026-08-09-screencast-tape-tuning` — done 2026-08-09 (D261)

- [x] **THEME-07** The palette's `:theme ` stage previews live — the screen re-themes to the cursor's row as it moves, `enter` commits and writes back, `esc`/rewind restores the theme you opened with — feedback `2026-08-09-theme-picker-live-preview` — done 2026-08-09 (D260)

- [x] **LOGS-SEL-04** Match highlight paints canvas-on-Warn on dark palettes (bright yellow background, 4.68–12.91:1 measured), weight-only on the three light ones — feedback `2026-08-09-log-match-highlight-background` — done 2026-08-09 (D259)

- [x] **LOGS-09** The palette opens over the logs view listing the logs verbs, `C` reaches ctx.switch from a stream, and `o` is the previous-instance default — feedback `2026-08-09-logs-view-palette-bindings` + `2026-08-09-context-switch-key-from-overlays` — done 2026-08-09 (D258)

- [x] **LOGS-08** A rejected previous-instance flip keeps the log view open — the running instance's stream resumes under a toast naming the server's reason — feedback `2026-08-09-logs-no-previous-keeps-view` — done 2026-08-09 (D257)

- [x] **CTX-MEM-04** The drill-in scope comes back as a drill-in — owner recorded beside the kind, re-resolved through the ChildResolver on restore, and the owner-gone case lands on the plain list with a notice saying so — done 2026-08-09 (D255)

- [x] **DOC-01b** Prose pass over the restructured README — the prose half of DOC-01's feedback; renamed from `DOC-02` by BOARD-03, whose commits still carry the old id — done 2026-08-09 (D267)

- [x] **LOGS-SEL-03** Both open questions answered: the bar and the highlight coexist, and the yank gesture set closes — done 2026-08-08 (D252)

- [x] **THEME-04b** `catppuccin-latte` and `solarized-light` ported, taking the registry to thirteen, with the admission guard retuned from "dark" to "chrome and text on opposite sides of one canvas" — done 2026-08-08 (D251)
- [x] **THEME-05** `styles.Match` relies on weight not paint — done 2026-08-09 (none)
- [x] **THEME-06** `gruvbox-light` ported from the fork's own light-mode palette (faded accents, bg0=light0), taking the registry to fourteen — done 2026-08-09 (none)

- [x] **THEME-04a** kubecom asks the terminal for its background after the first paint and warns only when the palette's polarity disagrees with the answer — done 2026-08-08 (D250)

- [x] **THEME-03** `Theme.Background`, set in all eleven built-ins and painted by the root View as the terminal's own background — done 2026-08-08 (D249)

- [x] **THEME-02** `dracula`, `gruvbox-dark`, `nord`, `rose-pine` and `tokyo-night` ported from their upstream data files, taking the built-in registry to eleven and meeting the feedback's "~10" — done 2026-08-08 (D248)

- [x] **AUTH-07** fd 2 points at the log file for the life of the TUI, and back at the terminal only inside a suspend, so a credential plugin's stderr can no longer paint over the panes — done 2026-08-07 (D247)

- [x] **SEARCH-06** A cluster-search hit carries the runes it matched, so a fuzzy or label-selector result says why it is there — second half of feedback `2026-08-06-search-highlight-matches` — done 2026-08-07 (D246)

- [x] **LOGS-07** Logs buffer bounded at 10 000 lines, trimmed from the top — done 2026-08-07 (D245)

- [x] **LOGS-SEL-02** Visual mode and yank in the logs view — `v` selects log lines, `y` copies them unpainted and unwrapped via OSC-52, and a selection suspends the tail — done 2026-08-07 (D244)

- [x] **CTX-MEM-02** kubecom reopens the kind you left open, per context — recorded once the watch is live, replayed after discovery reconciles, silent when the cluster does not serve it — done 2026-08-07 (D243)

- [x] **LOGS-SEL-01** A line cursor in the logs view — j/k move a highlighted log line (not a screen row), Selection and Match share it, and `shownIdx` is the map a yank reads — done 2026-08-07 (D242)

- [x] **DOC-01** README restructured around the capability surface (587 → 438 lines); install and configuration reference moved to `docs/install.md` + `docs/configuration.md`; the three install guards now scan the doc set — outline half of feedback `2026-08-07-readme-structural-rewrite` — done 2026-08-07 (D241)

- [x] **CTX-MEM-01** Triaged the pane-memory feedback into the CTX-MEM line, split from CTX-WARM — feedback `2026-08-06-context-switch-pane-memory` — done 2026-08-07 (D240)

- [x] **FILT-02** The `/` filter's matched text is highlighted in the rows it kept, cursor row included — first half of feedback `2026-08-06-search-highlight-matches` — done 2026-08-07 (D239)

- [x] **FILT-01** Backspace past the start of an empty `/` query cancels the search — feedback `2026-08-06-search-backspace-cancel` — done 2026-08-06 (D238)

- [x] **PAL-08** Palette rows show the command's name beside its description, in two columns — feedback `2026-08-06-palette-two-columns` — done 2026-08-06 (D237)

- [x] **THEME-01** Licence survey of the popular schemes + the Catppuccin dark flavors land, taking the registry to six — first slice of feedback `2026-08-06-more-themes` — done 2026-08-06 (D236)

- [x] **SEARCH-05** Enter commits the cluster-search query into the result list, so `hjkl` navigates there and a second enter opens the hit — feedback `2026-08-06-cross-search-enter-navigate` — done 2026-08-06 (D235)

- [x] **AGE-01** AGE is re-derived from the object's own timestamp on a tick, instead of staying the string the server printed once — feedback `2026-08-06-age-column-stale` — done 2026-08-06 (D234)

- [x] **PAL-07** Esc closes a key-opened palette stage; backspace keeps rewinding the line — feedback `2026-08-06-action-menu-esc-behavior` — done 2026-08-06 (D233)

- [x] **AUTH-06** Untrusted text is sanitized where it is rendered, so a plugin's own bytes cannot steer the terminal — half of feedback `2026-08-06-auth-error-breaks-layout`; AUTH-07 is the half the reader sees — done 2026-08-06 (D232)

- [x] **HT-dogfood-0806** Closed the two human tasks the maintainer finished — Edit's live `$EDITOR` confirmed, completing **M3**; no legacy config survives, so migration ticks on the generated fixture and says so — done 2026-08-06 (D231)

- [x] **BOARD-02b-3** Sweep and collapse the four closed lines 02b-1 did not reach — done 2026-08-06 (D230)

- [x] **BOARD-02b-2** A closed line collapses to its outcome, its pointers and its standing answers — done 2026-08-06 (D229)

- [x] **PAL-06** `:action ` marks the verbs that will ask before they act — done 2026-08-06 (D228)

- [x] **BOARD-02b-1** Harvest the work the prose defers into real Backlog items — done 2026-08-06 (D226, D227)

- [x] **BOARD-02a** The `## Done` index is canonical, complete again, and guarded — done 2026-08-06 (D225)

- [x] **BOARD-01** The Done list is one line per entry again, and `make check` keeps it there — done 2026-08-05 (D224)

- [x] **HINT-05** Nothing enforced the completeness — done 2026-08-05 (D223)

- [x] **BOX-03** The keybindings overlay is a fixed 15 rows on every screen — done 2026-08-05 (D222)

- [x] **BOX-02** The port-forward panel is as tall as the number of forwards — done 2026-08-05 (D221)

- [x] **BOX-01** The modal renders no taller than the box it computes — done 2026-08-05 (D220)

- [x] **HINT-04** The port-forward panel's footer spells its keys literally — done 2026-08-05 (D219)

- [x] **HINT-03** The last two liars: the browse filter field and the port-forward panel — done 2026-08-05 (D218)

- [x] **HINT-02** The same for the modals, the help overlay and the shared viewer — done 2026-08-05 (D217)

- [x] **AUTH-05b** Offer it: one confirm per occurrence, naming the exact command — done 2026-08-05 (D216)

- [x] **AUTH-05a** Run an approved remediation in the suspended terminal, then retry the request — done 2026-08-04 (D215)

- [x] **AUTH-04b** Wire the diagnosis into the browse surface — done 2026-08-04 (D214)

- [x] **AUTH-04a** The copy for a diagnosed credential-plugin failure — done 2026-08-04 (D213)

- [x] **AUTH-03** Recognise an expired AWS SSO session, and name the profile — done 2026-08-04 (D212)

- [x] **AUTH-02** Capture the plugin's stderr by re-running it as a diagnostic — done 2026-08-04 (D211)

- [x] **PAL-05d** `a` opens the palette's `:action ` stage — done 2026-08-04 (D210)

- [x] **PAL-05c-2** `C` opens the palette's `:context ` stage — done 2026-08-04 (D209)

- [x] **PAL-05c-1** `ctrl+n` opens the palette's `:namespace ` stage — done 2026-08-04 (D208)

- [x] **PAL-05b** `R` opens the command palette on its `:resource ` line instead of a resource modal of its own — done 2026-08-02 (D207)

- [x] **PAL-05a** `T` opens the command palette on its `:theme ` line instead of a theme modal of its own — done 2026-08-02 (D207)

- [x] **HINT-01** The bottom hint line stops promising keys an open picker swallows — done 2026-08-02 (D206)

- [x] **PAL-04** Contextual verbs for the selected row — done 2026-08-02 (D205)

- [x] **CRD-PIN-05** Pin/unpin a kind by naming it — the palette's `:pin ` verb — done 2026-08-02 (D204)

- [x] **CRD-PIN-04** The resource picker finds a kind by any name it answers to — done 2026-08-02 (D203)

- [x] **CRD-PIN-03** `*` on a pinned row unpins it — done 2026-08-02 (D202)

- [x] **CRD-PIN-02** `*` pins the kind under the cursor for this context — done 2026-08-02 (D201)

- [x] **CRD-01** The browse table's empty pane says why its LIST failed — done 2026-08-02 (D200)

- [x] **PAL-03** `:namespace ` / `:resource ` argument completion in one surface, via its two slices — done 2026-08-02 (D198, D199)

- [x] **PAL-03b** The asynchronous argument verbs: `:namespace ` and `:context ` — done 2026-08-02 (D199)

- [x] **PAL-03a** The argument stage: a verb commits in place, the list becomes its values — done 2026-08-02 (D198)

- [x] **PAL-02** `:` opens the command palette over the action registry — done 2026-08-02 (D197)

- [x] **CTX-WARM-01** Triaged the context-warmth feedback and landed the switch timing in the diagnostic log — done 2026-08-02 (D196)

- [x] **AUTH-01** `kube.ExecPluginFor` + `KindExecPlugin` name a failed exec credential plugin; AUTH line triaged — feedback `2026-08-01-eks-sso-reauth` — done 2026-08-01 (D195)

- [x] **PAL-01** Every list picker filters as you type, ranked by the cluster-search matcher — done 2026-08-01 (D194)

- [x] **CRD-PIN-01** A pinned kind is per-context state, merged behind the authored menu entries; CRD-PIN line triaged — feedback `2026-08-01-custom-resources-pinning` — done 2026-08-01 (D193)

- [x] **EDIT-01** The editor is resolved once at startup over `KUBE_EDITOR → EDITOR → VISUAL → first of nvim/vim/nano/vi on PATH` and the choice is logged, so a user learns which editor they get before pressing `e`; a box with none of the four launches anyway and reports `no editor found; set $EDITOR` instead of suspending — feedback `2026-08-01-editor-autodetect` — done 2026-08-01 (D192)

- [x] **HT-dogfood-0801** Closed the three human tasks the 2026-08-01 dogfood session finished — done 2026-08-01 (D191)

- [x] **M1-INT-d** The gated envtest suite runs in CI — its own workflow on every push/PR, never the `make check` gate a release tag rides on; `make test-envtest` fixed to find `setup-envtest` off `PATH`, and a hermetic guard ties gate constant ↔ recipe ↔ workflow because a skipped suite is a green one — closes the M1-INT line — done 2026-07-31 (D190)

- [x] **M1-INT-c-4** `Update`'s optimistic concurrency against a live apiserver — done 2026-07-31 (D189)

- [x] **M1-INT-c-3** The five merge-patch actions against a live apiserver — done 2026-07-31 (D188)

- [x] **M1-INT-c-2** Scale reaches a live `scale` subresource — done 2026-07-31 (M1-INT-c-2)

- [x] **M1-INT-c-1** Delete's `DeleteOptions` reach a live apiserver — done 2026-07-31 (M1-INT-c split into c-1…c-4)

- [x] **M1-INT-b-2** A real 410/Expired forces the re-List — done 2026-07-31 (D34 proven live)

- [x] **M1-INT-b-1** A watch survives a real transport drop — done 2026-07-31 (D34 proven live)

- [x] **DISC-01** A broken API group is now named, not just isolated — `DiscoveryResult.Failed` reports any group the server serves no version of, which is the only trace an aggregated apiserver leaves in the cached group list; proven live by turning M1-INT-a's tripwire into its positive assertion — done 2026-07-31 (D187)

- [x] **M1-INT-a** envtest: a denied/broken API group is isolated, proven against a live apiserver — a real aggregated APIService broken on purpose and a real pods-only RBAC user; ticks M1's last `[~]` criterion, un-defers M1-INT (envtest runs in the sandbox in 12 s, D186 pt 1) and found DISC-01 — done 2026-07-31 (D186)

- [x] **M5-10** Release pre-flight — every M5 slice confirmed, the notes rendered against a throwaway local tag, the README install paths audited — done 2026-07-30 (D185)

- [x] **M5-08** Docker — built and run in the sandbox, then reverted 2026-08-09: container builds dropped (D254) — done 2026-07-30 (D184)

- [x] **M5-07** AUR — an `aurs:` `kubecom-bin` package from the released archives, inert without its key — done 2026-07-30 (D183)

- [x] **M5-06** Homebrew — a `homebrew_casks:` cask, inert without its token; tap moved to the org `neuroplastio/homebrew-tap` (D253) — done 2026-07-30 (D182)

- [x] **M5-09** Screencast — `docs/screencast.tape` + `make screencast`, pinned to the keymap by three guards — done 2026-07-30 (D181)
- [x] **M5-05** Migration verified against a `~/.kubecom.yaml` the 2020 writer itself generated — done 2026-07-30 (D180)
- [x] **M5-04** Migration carries the legacy `currentTheme` onto `theme:` — done 2026-07-30 (D179)

- [x] **M5-01b** Settle the DoD's "in-TUI YAML viewer" against D135 — done 2026-07-30 (D178)
- [x] **M5-01a** Previous-container logs — `logs.previous` (`ctrl+p`) toggles the open logs view between the running instance and the previous terminated one (`kubectl logs -p`), re-issuing the stashed request with one bit flipped; the buffer is replaced but the grep/wrap/timestamps survive it, and the header names the instance with `[previous]` — done 2026-07-30 (D177)
- [x] **M5-03** Release workflow — `release.yml` runs `goreleaser release` on a `v*` tag (gated on `make check` via ci.yml as a reusable workflow) plus a `--snapshot --clean` dry run on every push to `v1`/`main`, with the goreleaser pin and the dry run guarded by `TestReleaseWorkflowPinsGoreleaser` — done 2026-07-30 (D176)
- [x] **M5-02** Wire `Commit` and `Date` into the release ldflags — released binaries now report their commit and build date, guarded by `TestGoreleaserSetsAllVersionVars` so an unwired `internal/version` var fails `make check` — done 2026-07-30 (D175)
- [x] **M5-01** Audit the Definition of Done against named evidence — 6 of 13 boxes ticked, every unticked box names what closes it; found two gaps (no previous-logs surface → M5-01a, the DoD's YAML bullet vs D135 → M5-01b) — done 2026-07-30 (D174)
- [x] **M5-PLAN** Expand M5 (release & docs) into ordered, leg-sized Backlog slices M5-01…M5-11 — M5 set in-progress; found four concrete gaps (unticked DoD, unset `Commit`/`Date` ldflags, no release workflow, a migration note stale since themes landed) — done 2026-07-30 (D173)

- [x] **CTX-MEM-03** Bring the table's own view state back with the pane — done 2026-08-09 (none)
- [x] **M4-12b-2** Theme picker + write-back — `T` picks a theme, `applyStyles` repaints it live and the name is written back to `config.yaml` load-modify-save; closes M4 — done 2026-07-30 (D172)

- [x] **M4-12b-1** Live restyle mechanism — `SetStyles` on all eleven components plus the `applyStyles` fan-out; three re-derive rather than assign, and nothing calls it yet — done 2026-07-30 (D171)

- [x] **M4-12a** Theme applied at launch — `theme:` config field resolved by the launcher and built into every component; unknown name → default + one startup notice — done 2026-07-29 (D170)

- [x] **M4-11** Built-in themes + registry — ported monokai + solarized-dark palettes beside the default, with `Themes()`/`ThemeNames()`/`ByName()` over one `builtins` list; nothing selects them yet — done 2026-07-29 (D169)

- [x] **M4-10** TUI metrics columns when available — CPU/MEMORY overlay on the Pod/Node table, 10 s poll joined onto the watched rows, absent and silent without metrics-server — done 2026-07-29 (D168)

- [x] **M4-09** Kube layer metrics primitive over `metrics.k8s.io` — `MetricsFor`/`HasMetrics` answer availability from the discovery result (no probe request), `Clients.Metrics` lists samples through the dynamic client into a `map[UsageKey]Usage` keyed by namespace/name — done 2026-07-29 (D167)

- [x] **M4-08** TUI owner → children drill-down (`res.children`, `P`) — done 2026-07-29 (D166)

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

- [x] **SEARCH-04c-2b** Fuzzy (subsequence) cluster-search matching — done 2026-07-28 (D153)
- [x] **SEARCH-04c-2a** Ranked cluster-search results — scored by where the query lands, inserted at rank while streaming — done 2026-07-28 (D152)
- [x] **SEARCH-04c-1** Label-selector matching for cluster search — done 2026-07-28 (D151)
- [x] **SEARCH-04b** All-namespaces scope widen for cluster search — `search.allNamespaces` (`ctrl+w`) searches every namespace and re-runs the query, replacing the namespace the header names rather than adding a segment, independent of the kind widen and leaving the app's own namespace scope untouched; `kubecom keys` now sizes its columns from the widest id — done 2026-07-28 (D150)
- [x] **SEARCH-04a** All-kinds scope widen for cluster search — `search.allKinds` (`ctrl+a`) swaps the curated kind set for every discovered kind and re-runs the query; the header names the widened scope only while it is on, the widen resets on every fresh open, and `kube.Search` now lists at most 8 kinds at a time so the widen cannot flood the apiserver (D149) — done 2026-07-28 (D149)
- [x] **LOGS-04b** Timestamps toggle in the logs view — `logs.timestamps` (`t`) draws each line's server stamp; the stream always requests timestamps (free on a followed stream) and the view keeps them in a buffer parallel to the messages, so the toggle is a redraw not a restream and the grep still matches only the message (D148) — done 2026-07-28 (D148)
- [x] **LOGS-04c** Jump-to-latest in the logs view — `nav.bottom` (`G`) now re-arms following as well as scrolling, the inverse of "any upward scroll pauses"; incremental downward movement still does not (D147) — done 2026-07-28 (D147)
- [x] **LOGS-04a** Wrap toggle + horizontal scroll for long lines in the logs view — done 2026-07-28 (D146)
- [x] **LOGS-03** Regex grep mode + match highlighting in the logs view — done 2026-07-25 (D145)
- [x] **LOGS-02** Wire `res.logs` to the dedicated logs view — done 2026-07-25 (D144)
- [x] **SEARCH-03b** Search progress + cap surfaced in the searchview header, plus a `HelpSearch` hint context — done 2026-07-25 (D143)
- [x] **SEARCH-03a** Search progress/completion signal in the kube layer — a typed `SearchEvent` channel — done 2026-07-25 (D142)
- [x] **SEARCH-02b** Cluster search app wiring — `ctrl+s` opens the view over a debounced `kube.Search`, `enter` drills into the hit — done 2026-07-25 (D141)
- [x] **SEARCH-02a** Cluster-search view component (`internal/tui/components/searchview`) — done 2026-07-24 (D140)
- [x] **FB-pf-local-port** Port-forward local port — every declared port opens the picker, with free-local (`0`) and an editable local port (`p`) — done 2026-07-24 (D139)
- [x] **FB-pf-port-picker-b** Port-forward port picker — TUI wire: a `PortLister` seam lists the target's declared ports — done 2026-07-24 (D138)
- [x] **FB-pf-port-picker-a** Port-forward port picker — kube-layer declared-ports primitive (`internal/kube/ports.go`): `Clients.PodPorts` (declared containerPorts, native sidecars included) + `Clients.ServicePorts` (service ports resolved to the pod-side targetPort, named targets looked up on the backing pod); TCP-only, de-duplicated, apimachinery-free `Port` — done 2026-07-24 (D137)
- [x] **M3-15c** Unify YAML view + edit — retired the standalone read-only YAML viewer (`openYAMLViewer`/`yamlLoadedMsg`/`viewerKindYAML`/`rowActionYAML`/`ActionYAML`/`res.yaml`); the View/Edit YAML action (`e`, gated `canGet`) is now the only YAML surface, `y` unbound; `YAMLGetter` seam kept; `docs/keybindings.md` regenerated — done 2026-07-24 (D135, D136)
- [x] **M3-15b** Edit — the TUI suspend flow: `e` → YAML → `$EDITOR` → `kube.Update`, only on change — done 2026-07-24 (D135)
- [x] **LOGS-01** Dedicated full-screen logs-view component with a live filter — feedback `2026-07-24-logs-dedicated-view-live-grep` triaged into LOGS-01…04 — done 2026-07-24 (D134)
- [x] **FB-delete-key-d** Feedback (normal, `2026-07-24-delete-default-key-d`): shipped default delete binding is now `d` (vim `dd` muscle memory, was `x`); describe relocated off `d` to `D` (read-only, not a reserved nav chord); registry-driven/rebindable (D11), `docs/keybindings.md` regenerated, feedback deleted — done 2026-07-24 (D133)
- [x] **FB-confirm-yn-keys** Feedback (normal, `2026-07-24-confirm-modal-yn-keys`): confirm modal accepts `y` (confirm)/`n` (decline) plus enter/esc via registered, rebindable `confirm.accept`/`confirm.decline` resolved in a dedicated keymap **context** (no raw-key match, D11) — done 2026-07-24 (D132)
- [x] **SEARCH-01** Cluster-search kube primitive (`internal/kube/search.go`) — feedback `2026-07-24-cluster-search-multi-resource` triaged into the SEARCH line — done 2026-07-24 (D131)
- [x] **FB-pf-bind-toast** A port-forward bind failure names the clashing local port and the free-local retry — feedback `2026-07-24-port-forward-picker-and-local-port` triaged — done 2026-07-24 (D130)
- [x] **HT-exec-dogfood** Closed the exec live-cluster dogfood human-task — maintainer confirmed the Exec-shell action works end-to-end against a real cluster in a real terminal (shell drops in, TUI restores cleanly); ticked the M3 exec exit criterion, deleted `vault/human-tasks/2026-07-24-exec-live-cluster-dogfood.md` — done 2026-07-24
- [x] **M3-15a** Edit — the kube-layer update primitive (`internal/kube/apply.go`), optimistic-concurrency guarded — done 2026-07-24 (D129)
- [x] **M3-14b-4** Exec — kubectl parity fallback: suspend into `kubectl exec` when it is on PATH, else the SPDY path — done 2026-07-24 (D128)
- [x] **M3-14b-3** Exec — live terminal resize: a SIGWINCH watcher pushes the new size into the exec size queue — done 2026-07-24 (D127)
- [x] **M3-14b-2** Exec — multi-container picker reuse: `openExec` routes through the shared M3-07a container-resolution path tagged with a new `ctrPurpose` (logs↔exec); single-container Pod execs directly, multi-container prompts via the reused `ctrPicker`, `streamOrExec` routes to `streamLogsInto`/`execInto`; `newExecCommand` takes the chosen container — done 2026-07-24 (D126)
- [x] **M3-14b-1** Exec — TUI wire (in-process SPDY primary path): `tea.Exec`→`execCommand.Run()` drives blocking `kube.Exec` off the loop, local raw terminal (x/term) + seed-once size queue, Pod default container `/bin/sh`, result to status bar; `Execer` seam; live exec dogfood raised as a human-task — done 2026-07-24 (D125)
- [x] **M3-14a** Exec — the kube-layer exec primitive (`internal/kube/exec.go`) over SPDY, no kubectl binary — done 2026-07-24 (D124)
- [x] **M3-13c** Port-forward for Services — resolved to a backing endpoint pod first, then forwarded — done 2026-07-24 (D123)
- [x] **M3-13b** Port-forward panel (`F`) — an overlay of the active forwards; stop the selected one or all — done 2026-07-24
- [x] **M3-13a** Port-forward start + background lifecycle on a Pod row — done 2026-07-23 (D122)
- [x] **M3-12** CronJob suspend/resume wired: `Suspender` seam (both verbs, `WithSuspender`) → `kube.Suspend`/`Resume` on CronJob rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **M3-11b** Drain wired — `kube.DrainStream` behind the confirm modal, progress to the status bar — done 2026-07-23 (D121)
- [x] **M3-11a** Cordon/uncordon wired: `Cordoner` seam (both verbs, `WithCordoner`) → `kube.Cordon`/`Uncordon` on Node rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **FB-gray-out-empty-types** Graying out empty left-menu resource types triaged and dismissed — feedback `2026-07-23-gray-out-empty-resource-types` — done 2026-07-23 (D119)
- [x] **FB-k9s-not-prior-art** Feedback (normal, `2026-07-23-k9s-not-prior-art`): reworded the README "Special thanks" k9s line from "prior art in the Kubernetes-TUI space" to "a contemporary Kubernetes TUI in the same space" (k9s is a ~2019-2020 peer, not a predecessor); split M3-11 → M3-11a/M3-11b — done 2026-07-23 (D118)
- [x] **M3-10** Scale (prompt → `kube.Scale`) + rollout-restart (confirm → `kube.RolloutRestart`) wired through the D115 modal; new `Scaler`/`RolloutRestarter` seams, prompt-mode key routing (`routeModalPromptKey`), shared `mutateRes`/`mutateRef` stash, results to the status bar — done 2026-07-23 (D117)
- [x] **M2-14b** teatest coverage: modal confirm flow — full-program (teatest/v2) delete confirm: `x`→open→`enter` accept (delete runs) / `esc` decline (no delete), async accept synced on a side-effect signal not Quit-ordering — done 2026-07-23 (D116)
- [x] **M3-09** Delete action wired through the confirm modal (`res.delete`/`x` → `modal.ShowConfirm` on the selected row → accept `nav.drillIn` runs `kube.Delete` (row's UID guards the snapshot race), result to the status bar (error toast / neutral notice); decline `nav.back`/quit closes it, no raw y/n; `Deleter` seam + `WithDeleter`) — **unblocks M2-14b** — done 2026-07-23 (D115)
- [x] **M3-08b** Secret viewer — copy the selected value to the clipboard (#89) — done 2026-07-23 (D114)
- [x] **M3-08a** Secret viewer — reveal/base64-decode (#89) — done 2026-07-23 (D113)
- [x] **M3-07b** Logs — pod-owning kinds (#84): a `PodResolver` seam resolves a workload to a backing pod — done 2026-07-23 (D112)
- [x] **M3-07a** Logs container picker for multi-container pods: `ContainerLister` seam (`kube.PodContainers`) resolves a pod's containers before streaming — done 2026-07-23 (D111)
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
- [x] **M2-07** Root app model / shell (`internal/tui/app.go`, replaces the M0 `internal/tui/tui.go` placeholder), via its four slices — done 2026-07-20 (D61, D62, D63, D64)
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
