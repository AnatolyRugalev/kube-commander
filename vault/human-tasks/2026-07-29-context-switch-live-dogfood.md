# Switch contexts live between two real clusters, in a real terminal

- Created: 2026-07-29
- By: M4-04b
- Amended: 2026-08-07 by CTX-MEM-02 — item 8 added (the pane comes back now). Same two-cluster
  setup as everything above, so do it in the same sitting; it is the first item here that is
  about how the switch *looks* rather than what it tears down.
- Priority: normal
- Blocks: none (advisory — gates only the M4 "switch context without restarting; watches
  and menu rebind" exit criterion, which stays unticked until this is done; M4-05 and every
  other M4 slice may proceed)
- Status: open

## What's needed

Run `kubecom` against a kubeconfig that declares **at least two reachable contexts**,
drill into a resource so a watch is live, then press `C` (`ctx.switch`) and pick the
other context:

1. **The picker.** It lists every context in the kubeconfig, sorted, with the one you
   are on marked `*` and the cluster name shown where it differs from the context name.
   `/` filters, `esc` closes, `enter` picks. Does the row text read well at your
   terminal width — especially with long GKE/EKS-style context names?
2. **The switch.** The table empties, the menu returns to the seed set, the status bar
   names the new context and shows a discovery spinner; a moment later the new
   cluster's kinds (including its CRDs) appear in the menu. Drill in: the rows are the
   **new** cluster's, and they keep updating (the watch really rebound).
3. **Nothing from the old cluster leaks.** Open a log stream / a viewer / the port-forward
   panel first, then switch: overlays close, forwards stop, and nothing from the departed
   cluster keeps streaming. Watch for a stale row set flashing into the new table.
4. **Switch back**, then switch to the context you are already on — the marked row. That
   is deliberately a no-op (D157): it must *not* tear the working cluster down and rebuild
   it.
5. **A context that cannot connect** (edit one in the kubeconfig to point at a bogus
   server, or pick one whose cluster is down): you should get one transient error toast and
   stay exactly where you were — same context, same rows, still updating. This is the whole
   reason the connect runs before the teardown.
6. **Namespace and menu.** _(updated 2026-07-29, after M4-05/D163 — this task was raised
   when a switch cleared the scope.)_ A switch now lands you in the namespace **that
   context** was last left in, and builds the menu from **that context's**
   `menus/<context>.yaml`. So: scope one context into a namespace, switch away, switch
   back — you should return to it, and picking a namespace on the new context must not
   change what the first one reopens on. A context kubecom has never recorded starts on
   all-namespaces, which is the old behaviour and still correct.

7. **How long does it take?** _(added 2026-08-02 by CTX-WARM-01, from feedback
   `2026-08-01-context-switch-keep-state` — "switching back should feel like flipping a
   tab".)_ Since that leg, every completed switch writes its own cost to the diagnostic
   log. After doing pts 1-6, switch away and back a couple of times and paste:

   ```bash
   grep 'context switch complete' ~/.cache/kubecom/kubecom.log
   ```

   Each line reads `context=… connect=… discovery=… total=…`: `connect` is building the
   client for the new context (local, no round-trip by design), `discovery` is the wait
   from the swap landing to the new cluster's menu being complete, and `total` is what the
   reader actually experiences. **This is the number that decides CTX-WARM-02/03** — whether
   retaining a departed cluster is worth its risk, and if so which half is worth retaining
   (D196 pt 3). A first switch to a cluster and a switch *back* to one seen already are
   different measurements: please include both, the second is the one the feedback is about.

8. **Does the pane coming back feel like arriving, or like a jump?** _(added 2026-08-07 by
   CTX-MEM-02, from feedback `2026-08-06-context-switch-pane-memory` — "my pane gets reset",
   pt 3 of the 2026-08-06 update above.)_ kubecom now remembers the **kind you had open**,
   per context, and reopens it — at launch as well as across a switch. The mechanism is
   hermetically covered (`internal/tui/panememory_test.go`); what is not is the *timing*, and
   it is the one thing that could make this worse rather than better:

   The restore fires when the new cluster's discovery pass completes, not when the switch
   lands, because a CRD is not resolvable before then. So for however long discovery takes
   you see the seed menu and the welcome pane, and *then* the table appears under you.
   Please say which of these it is:

   - it reads as the switch finishing (good — this is what the design assumes), or
   - it reads as a flash of the wrong screen followed by a jump (bad — then the fix is to
     hold the welcome pane's transition, or to restore a *seed* kind immediately and upgrade
     to a discovered one when the pass lands; either is a small follow-up, but only one of
     them is worth writing).

   Two more things only a real cluster shows: (a) scope one context to a CRD its cluster
   serves and the other to `pods`, then switch back and forth — each should reopen its own
   kind, and neither should reopen the other's; (b) point kubecom at a cluster **without**
   that CRD's operator. The remembered kind is deliberately skipped in silence there — you
   should land on the menu and welcome pane with **no toast and no error in the table**. If
   anything at all is said on screen about the kind that could not be restored, that is a
   bug against D240 pt 3 and belongs in `../feedback/`.

## Why the agent can't do it

The sandbox has no cluster, let alone two, and no interactive terminal. Every *mechanism*
above is covered hermetically (`internal/tui/context_test.go`, `reset_test.go`): connect
off the loop, connect-before-teardown, the reset inventory, the bundle swap, discovery
restarting against the new cluster's discoverer, the picker's marker and routing. What
hermetic tests cannot show is the thing the exit criterion actually claims — that a real
second cluster's client rebinds a *live* watch and menu without a restart, and that the
transition looks like a switch rather than a glitch. Ticking that criterion on fakes would
be exactly the green D79 forbids.

## How to resolve

Do the check, then EITHER set `Status: done` with a `## Result` (the next agent leg ticks
the M4 context-switch exit criterion, folds the result in and deletes this file), OR delete
it if nothing needs to flow back. Any bug goes in `../feedback/`.

## Result

**Partially verified 2026-08-01 — kept open; the M4 exit criterion should NOT be ticked
from this alone.**

Setup: `k3d-kubecom-test` (the seeded dogfood cluster) plus a second real cluster
`k3d-kubecom-alt` holding objects that exist nowhere else (`alt-only/only-in-alt`), plus
a deliberately unreachable `k3d-kubecom-bogus` pointing at `https://127.0.0.1:1`. All
three are in the kubeconfig and remain there for the next attempt.

- **pts 1-2 — passed.** `C` opens the picker and switching contexts works.

Not exercised, and each is a distinct claim the criterion actually makes:

- **pt 3 (nothing leaks).** A log stream / viewer / port-forward was not opened *before*
  switching, so the teardown claim — overlays close, forwards stop, no stale rows flash
  in from the departed cluster — is unverified. This is the check most likely to find a
  real defect and the reason the fixtures above are worth keeping.
- **pt 4 (same-context switch is a no-op, D157).** Not tried. A regression here would
  silently tear down and rebuild a working cluster.
- **pt 5 (unreachable context).** `k3d-kubecom-bogus` was staged for exactly this and not
  used: one transient toast, stay put, rows still updating. This is what justifies
  connect-before-teardown.
- **pt 6 (per-context namespace + menu memory, D163).** Not tried.

The happy path working is real evidence, but the criterion claims the *transition* is
clean — which is pts 3-6, not pt 2. The remainder is about five minutes.

**Fixtures are preserved but the clusters are stopped** (2026-08-01, to free the machine).
Everything above comes back with:

```bash
k3d cluster start kubecom-test kubecom-alt   # ~30s; contexts and objects intact
```

`k3d-kubecom-bogus` is a plain kubeconfig entry pointing at `https://127.0.0.1:1`, so it
needs nothing — it is unreachable by construction whether or not anything is running.
Also still in the test cluster for the other open dogfoods: `broken/crashloop` (2,305
restarts, for the previous-logs stream question), `shop/firehose` (~1,900 lines/sec) and
`shop/demo-secret` (external-secrets).

## Update (2026-08-06) — still open; pts 3-6 still unexercised

Maintainer: "live context switch is fast, but my pane gets reset." Qualitative,
no `context switch complete` log lines pasted — pt 7 stays unanswered with hard
numbers, though "fast" is consistent with CTX-WARM-01/02's expectation that
`connect=` is cheap.

No report on pts 3 (leak check), 4 (same-context no-op), 5 (unreachable context)
or 6 (per-context namespace/menu memory) this round either — do not tick the M4
exit criterion or unblock CTX-WARM-02/03 from this update alone.

New finding, not something pts 1-7 above asked about: the maintainer wants the
**pane/view** (which resource type is open, scroll position, drill-down) to
survive a switch away and back too — "similar to window switching in OS," and
explicitly fine with bounding it (time-based eviction or a cap on N contexts
held in memory), not unconditional retention. This is a different, larger ask
than CTX-WARM-02/03 (which retain only the *connector's* client/discovery for
speed, capped at one entry, with the shell's teardown explicitly untouched per
D196 pt 1/2) — filed as `../feedback/2026-08-06-context-switch-pane-memory.md`
since it is a product/design question (does it revise D196's "shell teardown
never changes"?) rather than something this dogfood task itself can resolve.

## Update (2026-08-09) — still open; qualitative only

Maintainer, verbatim: "context switch UX is working well."

That is a qualitative confirmation of the happy path, not the exit criteria as
written, so this task stays **open** and the M4 context-switch exit criterion
stays unticked. Still missing, each a distinct claim the criterion makes:

- **pt 3 (nothing leaks)** — no report that a log stream / viewer / port-forward
  opened *before* a switch was torn down cleanly (overlays close, forwards stop,
  no stale rows flash in).
- **pt 4 (same-context switch is a no-op, D157)** — not confirmed.
- **pt 5 (unreachable context)** — not confirmed: one transient toast, stay put,
  rows still updating.
- **pt 6 (per-context namespace + menu memory, D163)** — not confirmed.
- **pt 7 (timing)** — no `context switch complete` log lines pasted, so the
  `connect=`/`discovery=`/`total=` numbers — for a first switch **and** a
  switch-back, the second being the one CTX-WARM-01's feedback is about — are
  still unknown. This is the number that decides CTX-WARM-02/03 (D196 pt 3).
- **pt 8 (pane restore reading)** — no statement on whether the pane coming back
  reads as the switch finishing (good) or as a flash + jump (bad).

"Fast" and "working well" are consistent with the cheap-`connect=` hypothesis,
but they are not the leak checks or the measured numbers. Do **not** unblock
CTX-WARM-02/03/04 or tick the M4 criterion from this alone.
