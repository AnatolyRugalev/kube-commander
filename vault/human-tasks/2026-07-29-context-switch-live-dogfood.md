# Switch contexts live between two real clusters, in a real terminal

- Created: 2026-07-29
- By: M4-04b
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
