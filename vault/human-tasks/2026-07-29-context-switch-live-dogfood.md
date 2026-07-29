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
6. **Namespace.** After a switch the scope is cleared to all-namespaces by design (D156's
   corollary). Landing in the new context's *last-used* namespace is M4-05 and is not yet
   implemented — please confirm the cleared scope is at least not confusing in practice.

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
