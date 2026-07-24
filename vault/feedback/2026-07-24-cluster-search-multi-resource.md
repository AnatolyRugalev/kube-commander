# (Bigger feature) Search the cluster — unified multi-resource results

- Submitted: 2026-07-24
- Priority: normal
- Area: new feature — global search (likely several legs; triage into board tasks)

A **cluster search**: type a query and get matching **objects across resource
kinds** in one unified result list (Kind · namespace · name), not just a filter
within the current resource table. Selecting a result drills into that object's
view. Distinct from the resource palette (`:`), which searches *kinds*; this
searches actual *resources*.

**Cost/scope tension (design this carefully — record a decision):** Kubernetes has
no cross-type search API, so "search the cluster" means **listing many resource
types and matching client-side** — the same expensive enumeration the lazy-load /
fast-cold-start design (D8) deliberately avoids. Don't build a "watch everything"
version. Make it a **one-shot, cancellable query** with:
- **Scoped default**: current namespace + a **curated set of common kinds**
  (Pods, Deployments, StatefulSets, DaemonSets, Services, ConfigMaps, Secrets,
  Ingresses, Jobs, CronJobs, PVCs…), reusing the existing server-side Table `List`
  and the discovery set. Whole-cluster / all-discovered-types is an opt-in widen,
  not the default (it's slow + rate-limited on big clusters).
- **Concurrent + streaming**: fire the per-kind lists concurrently, stream results
  into the list as each returns (progress indicator), **cap** total results, and
  **cancel** in-flight lists when the query changes or the view closes.
- **Matching**: name substring (case-insensitive) to start; fuzzy / labels / fields
  as later slices.
- Per-kind failures degrade (a denied/broken group contributes nothing, never
  blanks the whole search — principle 3, same as discovery).

**Suggested MVP first slice:** a `search.cluster` action (registered key, D11)
opening a full-screen search mini-app (like the logs view / palette); query →
concurrent name-match over the curated kinds in the current namespace → unified
streamed results → `enter` drills into the selected object. Later slices widen
scope, add fuzzy/label/field matching, and whole-cluster mode.

Keep it keymap-driven (D11) and message-only (principle 1); reuse `kube.List` +
discovery. Triage into board tasks and do the first slice; record the search
architecture + default-scope decision.
