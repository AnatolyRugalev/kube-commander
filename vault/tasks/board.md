# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-08-05 — BOARD-01 done: the Done list is one line per entry again and `make check` now says so, so D102 is a gate rather than a convention (D224). Per-leg history: `vault/journal/`._

## In Progress

- [ ] **BOARD-02a** Settle the `## Done` index: canonical, backfilled, and guarded
      status: in-progress | owner: claude-opus-5 | added: 2026-08-06

## Blocked

- [ ] **M5-11** Make the rewrite the default branch (`v1` → `main`)
      status: blocked | owner: — | added: 2026-07-30
      notes: Blocked on human task `2026-07-30-first-release-tag` — renaming the branch before
      a release exists would retarget every clone and PR for a tree nobody can install yet, and
      the rename also dissolves the `@v1` collision the tag is what actually fixes. The agent
      share is preparation: what to rename, `master` kept as the permanent 2020 reference
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

**The match-quality dogfood is closed** (2026-08-01, HT-dogfood-0801) as *no problem found*,
which is narrower than "verified": `strfrnt` scoped to `shop` returned 6 hits, every one a real
`storefront` object and no junk tail, so the abbreviation case the fallback exists for
works. But every hit was a true positive, so there was no noise to rank — and short queries,
the widened scopes and the band-gap invariant were not probed. `searchScatteredShare =
limit/4` and the absence of a minimum needle length therefore remain **guesses**, not
findings (D191 pt 2): they are not to be tuned on the strength of this pass, in either
direction.

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
whoever builds the next streaming surface.

**The throughput dogfood is closed** (2026-08-01, HT-dogfood-0801): ~1,900 lines/sec from a
busybox firehose, no degradation, the view responsive to keys throughout. Read it as the
eyes-on confirmation of LOGS-05b/D162 rather than as "no fix was needed" — the task was
raised on 2026-07-25 against the pre-D162 build and pre-authorised the incremental render
*as* the fix, which then landed on 2026-07-29, four days before the measurement. Depth was
not bounded (no multi-hour tail), and pts 2–5 of the task were not walked separately.

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
README since M2-RUN) held only launcher warnings. So the CRD fix had a prerequisite: make
the error obtainable. DIAG-01 did that; CRD-01 was to be the fix — and turned out to be
something else (below).

DIAG-01 is done: `surfaceError` — the shell's single error funnel — now logs before it
toasts, and discovery's deliberately-silent failures (total and per-group) log too, so the
file the README already documented finally holds the errors the user actually hit (D159).
**CRD-01 is re-scoped, not fixed** (2026-08-01, HT-dogfood-0801/D191). Its human task came
back **not reproducible**: external-secrets installed fresh into the dogfood cluster lists
normally, with nothing in `~/.cache/kubecom/kubecom.log`. The one variable the clean install
removed is the conversion webhook — the fresh chart serves only `v1` with
`spec.conversion.strategy: None`, while the reporting cluster served `v1beta1` *and* `v1`
behind `strategy: Webhook`. A conversion webhook that is down or serving a bad cert makes
the **apiserver** fail the LIST, which kills `kubectl` too. So there is no client bug, and a
leg that "fixes" one would be inventing it (D79). What remained was the degradation path —
the half DISC-01 (D187) landed in the log file, not yet on screen — and **CRD-01 landed it
on 2026-08-02 (D200), which closes the DIAG line**: an empty browse pane now carries why its
LIST failed, with the conversion webhook and a 406 on Table conversion named apart from the
kind that would otherwise misdescribe them. The wording, not the mechanism, was the leg.
One claim is unverified against a real broken-webhook cluster and is parked in
`vault/human-tasks/2026-08-02-conversion-webhook-reason-dogfood.md` (advisory, blocks
nothing).

### Custom resources (CRD-PIN — feedback-driven)
Raised by feedback `2026-08-01-custom-resources-pinning` ("custom resources are really hard
to use"): a CRD-heavy cluster has hundreds of kinds, so listing them all makes the menu
useless and listing none makes CRDs unreachable. The ask is that a kind you reach for
**once** — via search or the picker — is **in your menu for that context** from then on,
removable with a key on the menu row, stored per context. This is the *usability* half of
CRDs and is deliberately not CRD-01's degradation half (D191 pt 1 keeps them apart).

Triaged into four slices, bottom-up (D52): the store, then the gesture that writes it, then
the gesture that removes it, then the discoverability that makes "reach for it once" true.
The fourth was split on pickup into **CRD-PIN-04** (finding the kind — done) and
**CRD-PIN-05** (pinning it from where you found it — done), since the two are different
surfaces and the second wanted a key the picker could not spare; the palette's argument
stage is what supplied one (D204).

- [x] **CRD-PIN-01** Where a pinned kind is stored, and the store that holds it — done
      2026-08-01 (D193)
- [x] **CRD-PIN-02** `*` pins the kind under the cursor for this context — done 2026-08-02 (D201)
- [x] **CRD-PIN-03** `*` on a pinned row unpins it — done 2026-08-02 (D202)
- [x] **CRD-PIN-04** The resource picker finds a kind by any name it answers to — done
      2026-08-02 (D203)
- [x] **CRD-PIN-05** Pin/unpin a kind by naming it — the palette's `:pin ` verb — done
      2026-08-02 (D204)

**The CRD-PIN line is closed** as of CRD-PIN-05/D204: the store, both directions of the
gesture, the picker that makes a kind findable, and a palette verb that pins one by name.
The answer to "which surface pins from the picker" was the palette rather than a `ctrl+…`
chord — `:pin ` takes the kind as its argument, so it needs no key the pickers cannot spare
and it works from anywhere. Two things it deliberately left, either of which is its own
small item if a dogfood wants them: the `R` picker still has **no** in-place pin chord (the
palette verb is the way in), and neither surface **shows** which kinds are already pinned
(D202 pt 4 keeps provenance off the display; the status-bar notice is what confirms the
gesture).

_Note on where these surfaces get their kinds, carried forward:_ the source is still the
menu's item list, and `:pin ` now shares it (D204 pt 1). That is equivalent to "every
discovered kind" **today**, because `Reconcile` appends every kind discovery finds — so the
premise holds and nothing was re-plumbed for a hypothetical. The day a slice narrows what
the menu lists (the CRD-heavy-cluster ask the feedback opens with), both must be re-sourced
from the discovery result instead, at the one snapshot D203 pt 4 names
(`resourcePickerItems`), or the narrowing takes the picker **and the pin verb** down with
it — and `:pin ` is the worse loss, since a narrowed menu is exactly when you need to pin.

### Hint-line truth (HINT — agent-found)
The bottom hint line is a promise about which keys act right now (D143 pt 1), and there is
one place it has been lying since M2-08: **while a modal picker is open**. Every picker
captures all input and, since PAL-01, opens its filter field with itself — so `/`, `n`, `s`,
`a`, `?` and `q` all type into the query — yet the hint underneath still shows the browse
menu/table set. Flagged by PAL-02, PAL-03a, PAL-03b, CRD-PIN-04 and CRD-01 in turn, and each
time named as the same leg-sized fix: a picker `HelpContext`, the D143 pt 1 shape.

- [x] **HINT-01** The hint line tells the truth while a picker is open
      — done 2026-08-02 (D206)
- [x] **HINT-02** The same for the modals, the help overlay and the shared viewer
      — done 2026-08-05 (D217)
- [x] **HINT-03** The last two liars: the browse filter field and the port-forward panel
      — done 2026-08-05 (D218)

**The HINT line is closed** as of HINT-03/D218: every surface in kubecom that captures input
has a `HelpContext`, written where its router tests it, so the two switches read as the same
list in the same order. The filter field takes the same four keys as the other two text fields
(D206 pt 3's rule, applied a third time), and the port-forward panel — the only capturing
surface with no text field — takes a transcript of `handleForwardsPanelAction`. Nothing
enforces the completeness, which is D218 pt 1: `contextShortHelpActions` has no exhaustiveness
check and `ShortHelpContext` falls back silently, so a new capturing surface with no case ships
a plausible wrong hint and every test still passes. HINT-04 cleared the residue HINT-03 left
behind, so no view in kubecom writes a key into its own body any more (D219).

- [x] **HINT-04** The port-forward panel's footer spells its keys literally — done 2026-08-05 (D219)
- [x] **HINT-05** Nothing enforced the completeness — done 2026-08-05 (D223)

**The HINT line is now enforced as well as closed** (HINT-05/D223): `HelpContext` is bounded
by `helpContextCount` and enumerable via `HelpContexts()`, and a declared context must both
carry a curated set (keymap) and be produced by some model state (tui) or one of the two tests
goes red. The sets stay hand-curated — only their completeness is mechanical. The
`ShortHelpContext` fallback survives for out-of-range integers, which is the case it was
always right for.

### Overlay geometry (BOX — agent-found)
Every overlay in kubecom is a bordered box centered over the browse body by `overlayCenter`,
which flattens onto a fixed `width×bodyHeight` canvas — so a box larger than the body is not
scrolled or shrunk, it is **silently clipped**, bottom-first. The picker is safe by
construction (its list is a component sized to `innerSize`), and the viewer and logs view are
pagers. The confirm/prompt modal is the outlier: it renders a free-form message that nothing
bounds. Found while reading `modal.View` for the wrapping question AUTH-05b left open.

- [x] **BOX-01** The modal renders no taller than the box it computes — done 2026-08-05 (D220)

**Two more overlays were unbounded**, measured while closing BOX-01 — filed rather than folded
in, since each is a different view with its own tests. The safe ones are safe by composition:
the pickers size their list to `innerSize`, and the viewer and the logs view are pagers with
their own viewport. Note for all three items that a unit test reading `View()`'s own string
cannot catch this class of bug — the string is complete; only the composited frame is short
(D220 pt 1). BOX-02 settled the second question these share: a bounded surface with a
**cursor** scrolls and counts what it hides, where one without a cursor truncates and marks
(D221 vs D220 pt 3). BOX-03 is the latter shape.

- [x] **BOX-02** The port-forward panel is as tall as the number of forwards — done 2026-08-05 (D221)
- [x] **BOX-03** The keybindings overlay is a fixed 15 rows on every screen — done 2026-08-05 (D222)

**The BOX line is closed** (01/02/03 done 2026-08-05): every overlay now bounds its own height,
the two shapes are settled (marker for a static box, window + counter for one with a cursor),
and the clamp itself lives in `internal/tui/elide` so a fourth overlay inherits it rather than
re-deriving it (D222 pt 2).

### Command palette (PAL — feedback-driven)
Raised by feedback `2026-08-01-command-palette-unification`: today every gesture that needs
a value opens its **own** modal picker on its own key (`ctrl+n` namespace, `:` resource, `C`
context, `T` theme, `a` actions), each with its own opt-in `/` filter, so muscle memory does
not transfer between them. The ask is **one place you type to make anything happen** — `:`
opens a palette, you fuzzy-match a verb, and the verb's argument list narrows in the same
surface, with the row-scoped actions (logs, edit, describe, port-forward, delete) offered
there too so `:` answers "what can I do right now?" without memorising the keymap.

Triaged into five slices, shallow-to-deep: the typing behaviour every surface needs first,
then the palette surface, then arguments, then row context, then the old keys become sugar.
The letter keys keep working throughout — PAL-05 is the only slice that changes what they
*are*, and it is last on purpose (D194 pt 4).

- [x] **PAL-01** Every list picker filters as you type, ranked by the cluster-search matcher
      — done 2026-08-01 (D194)
- [x] **PAL-02** The palette shell: `:` opens a verb list — done 2026-08-02 (D197)
- [x] **PAL-03** `:namespace ` / `:resource ` argument completion in one surface — done
      2026-08-02 via its two slices, **PAL-03a** (D198) and **PAL-03b** (D199)
- [x] **PAL-03a** The argument stage: a verb commits in place, the list becomes its values
      — done 2026-08-02 (D198)
- [x] **PAL-03b** The asynchronous argument verbs: `:namespace ` and `:context `
      — done 2026-08-02 (D199)
- [x] **PAL-04** Contextual verbs for the selected row — done 2026-08-02 (D205)
**PAL-05 was split on pickup into four slices**, one per key, because the conversion is not
the line it looks like: `unused` is in the lint gate (`default: standard`), so the moment a
key stops opening its standalone picker that picker, its Kind, its Selected/Cancelled arms,
its `activePicker`/`applyStyles`/`SetSize`/`View`/`capturing` sites **and its tests** all go
in the same leg or the tree is red. That is 20–52 references per picker, so four keys in one
leg is a ~700-line diff. Bottom-up as usual (D52): the mechanism lands on the cheapest key
first, then the keys get converted one at a time. The letter keys keep working throughout —
what changes is which surface they open, never whether they act.

- [x] **PAL-05a** `T` opens the palette's `:theme ` stage — and the sugar mechanism
      — done 2026-08-02 (D207)
- [x] **PAL-05b** `R` opens the palette's `:resource ` stage — done 2026-08-02 (D207)
**PAL-05c was split on pickup**, as its own notes allowed, into **PAL-05c-1** (`ctrl+n` →
`:namespace `) and **PAL-05c-2** (`C` → `:context `). The two verbs are the same *shape* —
both fetch their values, and retiring each picker collapses that message's `dest` routing
(D199) — but they are not one diff: `nsPicker` is the most referenced picker of the five and
has a second entry point the others do not (the menu's namespace-seam row,
`menu.NamespaceRequestedMsg`), while `ctxPicker` carries M4-04b's marker rule and the
kubeconfig read. One key per slice, as PAL-05a/b established.

- [x] **PAL-05c-1** `ctrl+n` opens the palette's `:namespace ` stage — done 2026-08-04 (D208)
- [x] **PAL-05c-2** `C` opens the palette's `:context ` stage — done 2026-08-04 (D209)
- [x] **PAL-05d** `a` opens the palette's `:action ` stage — done 2026-08-04 (D210)

**The PAL line is closed** as of PAL-05d/D210: one surface, six argument stages, and no
modal left that lists a set the palette also lists. The three open questions were answered
in the leg — `a` opens the row verbs *alone* (narrower is the reason to keep the key),
`ActionActions` survives as the verb that enters that stage, and its inertness is the
stage's (`enterPaletteArg`), not the key's. D209 pt 3 is superseded for `actions.menu`
only: its own "could you name the value first?" test says a compiled-in action registry is
a verb's argument, while `ctrPicker`/`portPicker` list an object's own containers/ports and
stay modals. Two things it deliberately left, either its own small item if a dogfood wants
them: the row verbs are still ranked by the shared matcher rather than by frequency, and
`:action ` does not show which entries carry a confirm before you pick one.

### Credential-plugin auth (AUTH — feedback-driven, D195)
Raised by feedback `2026-08-01-eks-sso-reauth`: an expired AWS SSO session surfaces as a
nameless auth failure, and the user is left to work out that the fix is `aws sso login
--profile x` in another terminal. The ask is that kubecom notice the **exec credential
plugin** failed, and offer to run the remediation. Provider-specific auth is **not** a
non-goal (checked against `goals.md`, D195 preamble), but the shape is fixed up front:
detect narrowly, offer — never run unasked — and only when the command can be
substantiated from the kubeconfig's own `user.exec` stanza (D195 pt 4/5).

Triaged into five slices, provider-neutral first, AWS last but one. AUTH-01…03 are all
kube-layer; nothing reaches the screen until AUTH-04, and AUTH-05 is the only slice that
runs anything.

- [x] **AUTH-01** Name the exec credential plugin behind a context; classify its failure
      — done 2026-08-01 (D195)
- [x] **AUTH-02** Capture the plugin's stderr by re-running it as a diagnostic
      — done 2026-08-04 (D211)
- [x] **AUTH-03** Recognise an expired AWS SSO session, and name the profile
      — done 2026-08-04 (D212)
**AUTH-04 was split on pickup** into **AUTH-04a** (what the surface *says*) and **AUTH-04b**
(how the diagnosis *gets* there), because the two halves are independently a leg's worth: the
copy has seven distinct cases to get right (failed re-run with stderr / with none, a re-run
that succeeded, a missing binary, a timeout, a truncated capture, and a remediation that is
present, unrecognised or unsubstantiated), while the wiring is a new seam, an async Cmd, a
generation guard and a launcher line. Bottom-up as usual (D52): the renderer first, since the
wiring calls it. Nothing reaches the screen until 04b. **Both halves are done** (2026-08-04):
a `KindExecPlugin` browse failure re-runs the plugin once per selection and rewrites the pane
with what it printed, so AUTH-05 inherits a diagnosis it only has to *offer to act on*.

- [x] **AUTH-04a** The copy for a diagnosed credential-plugin failure — done 2026-08-04 (D213)
- [x] **AUTH-04b** Wire the diagnosis into the browse surface — done 2026-08-04 (D214)
**AUTH-05 was split on pickup** into **AUTH-05a** (the *run*: what happens once a remediation
has been approved) and **AUTH-05b** (the *offer*: the confirm that approves it), for the same
reason AUTH-04 was — each half is a leg's worth, and the halves fail differently. The run is a
suspend, an argv + environment, a captured failure and a retry of the request that failed; the
offer is a modal kind, a stash, a routing arm, a decline path and the notice copy that stops
telling the reader to go to another terminal. Bottom-up as usual (D52): the runner first, since
the offer calls it. **Nothing can run until 05b** — until then `runReauth` is reachable only
from tests, which is the point: no code path arms it, so nothing executes unasked (D195 pt 4).

- [x] **AUTH-05a** Run an approved remediation in the suspended terminal, then retry the request
      — done 2026-08-04 (D215)
- [x] **AUTH-05b** Offer it: one confirm per occurrence, naming the exact command — done
      2026-08-05 (D216)

**The AUTH line is closed** as of AUTH-05b/D216: an expired credential plugin is named,
re-run, explained on the pane, and offered a fix the reader can accept in one key — which is
the whole of feedback `2026-08-01-eks-sso-reauth`. The offer is the only modal in kubecom no
keypress opens, so most of the slice is refusals: it opens only from a landed diagnosis, only
over the plain browse view, never queues, and is dropped rather than deferred when anything
else holds the screen. Two things it deliberately left, either its own small item: the confirm
box clips its message at sixty cells, so a long invocation is truncated *in the question* (the
pane behind it carries the command wrapped in full, which is the mitigation — the fix would be
`modal` learning to wrap, a component change with every other confirm as a caller); and the
offer's confirm has no `HelpContext`, which is HINT-02's fifth case. The live claim — the
suspend into a real `aws sso login`, and the retry after it — is item 7 on
`vault/human-tasks/2026-08-02-conversion-webhook-reason-dogfood.md` (advisory, blocks nothing).

### Context switch warmth (CTX-WARM — feedback-driven, D196)
Raised by feedback `2026-08-01-context-switch-keep-state`: switching away from a context
and back pays the full cost again (reconnect, rediscover, re-watch), and the submitter
wants `C` → other → `C` → back to feel like flipping a tab. It **pushes against** M4-04a's
unconditional teardown, so D196 fixes the shape before any code retains anything: the
shell's teardown never changes, retention (if it is built at all) lives inside the
launcher's `contextConnector`, it caps at the previous context, and it is **gated on
measurement** rather than on the assumption that reconnecting is slow.

CTX-WARM-02/03 additionally wait on `2026-07-29-context-switch-live-dogfood` pts 3-6 — the
leak checks — so the teardown baseline is known-good before it is optimised (D196 pt 5).

- [x] **CTX-WARM-01** Triage the warmth feedback; time the switch into the diagnostic log
      — done 2026-08-02 (D196)
- [ ] **CTX-WARM-02** Retain the previous context's client in the connector
      status: todo | owner: — | added: 2026-08-02 | blocked-on: the dogfood's pts 3-6 and
      CTX-WARM-01's numbers
      notes: A one-entry cache inside `cmd/kubecom`'s `contextConnector`, keyed by context
      name: `ConnectCluster` returns the retained `tui.Cluster` for the context just left
      instead of rebuilding it. The shell is untouched — it still resets everything and
      still holds exactly one cluster (D196 pt 1/2). Drop the entry on any connect error
      for that name. **Only worth doing if CTX-WARM-01's `connect=` is material**; note
      that `kube.Connect` does no network I/O, so it may well not be.
- [ ] **CTX-WARM-03** Retain the discovery result per context, if discovery is the cost
      status: todo | owner: — | added: 2026-08-02 | blocked-on: CTX-WARM-02
      notes: The other half of the log line. Discovery is already disk-cached per host with
      a 6 h TTL (`internal/kube/cache.go`), so the remaining cost is the walk + reconcile,
      not the network — measure before building. If it is worth it, retain the last
      `DiscoveryResult` beside the client so a switch-back reconciles the menu immediately
      and re-runs the pass in the background, never showing a stale menu as final.
- [ ] **CTX-WARM-04** Prove the retention cannot leak, and bound it
      status: todo | owner: — | added: 2026-08-02 | blocked-on: CTX-WARM-02
      notes: The guard rail for the two above: a test that a retained cluster holds no
      watch, no stream and no overlay (the shell cancelled them all), that the cache never
      exceeds one entry however many contexts are visited, and that a forced refresh
      invalidates rather than trusts it (D196 pt 4). Then a line on the dogfood task asking
      for the switch-back timing again, to confirm the win is real on a real cluster.

### Board hygiene (BOARD — agent-found)
The board is read on every Orient, so its size is a cost every leg pays and no leg sees.
D102 named the rule (a Done entry is one line) and collapsed the list by hand; it drifted
back anyway, which is what BOARD-01 found and fixed — this time with a guard, so the Done
half is now closed for good (D224).

- [x] **BOARD-01** The Done list is one line per entry again, and `make check` keeps it there — done 2026-08-05 (D224)
BOARD-02 was **split on pickup** into the two unrelated questions its notes had folded
together: **BOARD-02a**, the `## Done` index's half-appended state — a mechanical question
with a checkable answer — and **BOARD-02b**, the planning prose, which is the judgement
call D224 pt 2 refused to authorise. They are done in that order because the index question
gates nothing and the prose question is the one that can go wrong.

- [ ] **BOARD-02a** Settle the `## Done` index: canonical, backfilled, and guarded
      status: todo | owner: — | added: 2026-08-06
      notes: The index has not been appended to since 2026-08-02, so ~two dozen legs
      (AUTH-02…05b, PAL-01…05d, CRD-PIN-02…05, HINT-02…05, BOX-01…03) exist only as the
      `- [x]` line in their own section. Decide which half is canonical and make the other
      follow. BOARD-01 declined to backfill because "eight summaries written by a leg that
      did not do the work is how a wrong one gets in" — check whether that objection still
      applies now that every section entry is already in the D102 one-line shape, i.e.
      whether the backfill can be a **copy** rather than a rewrite. If it can, do it and add
      the guard, since an unguarded rule in this vault has now drifted three times (D224).
- [ ] **BOARD-02b** Decide whether the per-line planning prose is worth compacting too
      status: todo | owner: — | added: 2026-08-05
      notes: With the Done list collapsed, the prose above it is the larger half (~40KB of
      75KB): a narrative paragraph per closed line (SEARCH, LOGS, PAL, AUTH, CRD-PIN, HINT,
      BOX …). Unlike a Done entry it is *not* redundant with the journal — it carries why a
      line was split the way it was, and what a slice deliberately left — so this is a
      judgement call, not a mechanical pass, and it is **not** covered by D224's guard.
      What would make it worth a leg: a closed line whose paragraph no future leg can act on
      (the line is done, nothing references it) is a candidate to shrink to one sentence
      plus a journal pointer; an open line's is not. Do not touch a paragraph that names a
      constraint no decision records — move it to `decisions.md` first, or leave it.

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

_(none unblocked — M5-10's agent share is done and M5-11 is in **Blocked** above, waiting
on the tag. Every remaining M5 act publishes, and D173 pt 1 makes each one a human's.)_

## Done

- [x] **BOARD-01** The Done list is one line per entry again, and `make check` keeps it there — done 2026-08-05 (D224)

- [x] **PAL-05b** `R` opens the command palette on its `:resource ` line instead of a resource modal of its own — done 2026-08-02 (D207)

- [x] **PAL-05a** `T` opens the command palette on its `:theme ` line instead of a theme modal of its own — done 2026-08-02 (D207)

- [x] **HINT-01** The bottom hint line stops promising keys an open picker swallows — done 2026-08-02 (D206)

- [x] **CRD-01** The browse table's empty pane says why its LIST failed — done 2026-08-02 (D200)

- [x] **PAL-02** `:` opens the command palette over the action registry — done 2026-08-02 (D197)

- [x] **CTX-WARM-01** Triaged the context-warmth feedback and landed the switch timing in the diagnostic log — done 2026-08-02 (D196)

- [x] **AUTH-01** `kube.ExecPluginFor` + `KindExecPlugin` name a failed exec credential plugin; AUTH line triaged — feedback `2026-08-01-eks-sso-reauth` — done 2026-08-01 (D195)

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

- [x] **M5-08** Docker — a multi-arch `dockers_v2:` image at `ghcr.io/anatolyrugalev/kubecom`, built and run in the sandbox — done 2026-07-30 (D184)

- [x] **M5-07** AUR — an `aurs:` `kubecom-bin` package from the released archives, inert without its key — done 2026-07-30 (D183)

- [x] **M5-06** Homebrew — a `homebrew_casks:` cask publishing to the 2020 tap, inert without its token — done 2026-07-30 (D182)

- [x] **M5-09** Screencast — `docs/screencast.tape` + `make screencast`, pinned to the keymap by three guards — done 2026-07-30 (D181)
- [x] **M5-05** Migration verified against a `~/.kubecom.yaml` the 2020 writer itself generated — done 2026-07-30 (D180)
- [x] **M5-04** Migration carries the legacy `currentTheme` onto `theme:` — done 2026-07-30 (D179)

- [x] **M5-01b** Settle the DoD's "in-TUI YAML viewer" against D135 — done 2026-07-30 (D178)
- [x] **M5-01a** Previous-container logs — `logs.previous` (`ctrl+p`) toggles the open logs view between the running instance and the previous terminated one (`kubectl logs -p`), re-issuing the stashed request with one bit flipped; the buffer is replaced but the grep/wrap/timestamps survive it, and the header names the instance with `[previous]` — done 2026-07-30 (D177)
- [x] **M5-03** Release workflow — `release.yml` runs `goreleaser release` on a `v*` tag (gated on `make check` via ci.yml as a reusable workflow) plus a `--snapshot --clean` dry run on every push to `v1`/`main`, with the goreleaser pin and the dry run guarded by `TestReleaseWorkflowPinsGoreleaser` — done 2026-07-30 (D176)
- [x] **M5-02** Wire `Commit` and `Date` into the release ldflags — released binaries now report their commit and build date, guarded by `TestGoreleaserSetsAllVersionVars` so an unwired `internal/version` var fails `make check` — done 2026-07-30 (D175)
- [x] **M5-01** Audit the Definition of Done against named evidence — 6 of 13 boxes ticked, every unticked box names what closes it; found two gaps (no previous-logs surface → M5-01a, the DoD's YAML bullet vs D135 → M5-01b) — done 2026-07-30 (D174)
- [x] **M5-PLAN** Expand M5 (release & docs) into ordered, leg-sized Backlog slices M5-01…M5-11 — M5 set in-progress; found four concrete gaps (unticked DoD, unset `Commit`/`Date` ldflags, no release workflow, a migration note stale since themes landed) — done 2026-07-30 (D173)

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
