# Resources that don't support `watch` blank the view (e.g. componentstatuses)

- Submitted: 2026-07-22
- Priority: normal
- Area: kube layer — watch (`internal/kube/watch.go`) + menu/table

Selecting a resource whose kind doesn't support the `watch` verb shows an empty
list and errors:

```
kube: watching componentstatuses: watch is not supported on resources of kind "componentstatuses"
```

`componentstatuses` (and some aggregated/legacy resources) support `list`/`get`
but **not** `watch`. The List likely succeeds, but the follow-up Watch errors and
the view ends up empty (and, per the D63 watch loop, may retry the unsupported
watch forever instead of showing the listed rows).

**Fix:** make watch **conditional on the discovered verbs**. Discovery already
carries each resource's `Verbs` (the `kube.Resource`/APIResource) — when `watch`
is not in the set, don't attempt the watch: do a plain **List** and present those
rows (a one-shot RESET), skipping the streaming watch. Optionally poll-refresh on
an interval for list-only kinds, but even a single static list beats an empty view.
Treat a server "watch not supported" error defensively too (degrade to list-only
rather than retry-looping), in case discovery verbs are incomplete.

Goal: `componentstatuses` shows its rows; no empty view, no infinite retry
(principle 3 — degrade, don't blank).

**Test:** a fake resource whose `Verbs` lack `watch` → the layer lists and does not
attempt/parade a watch error.
