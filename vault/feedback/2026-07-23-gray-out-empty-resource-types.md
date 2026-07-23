# (Soft idea) Gray out left-pane resource types that have zero resources in view

- Submitted: 2026-07-23
- Priority: low
- Area: resource menu (left pane)

**Idea (soft — dismiss if expensive):** in the left menu, gray out a resource type
when there are **zero** of it in the current view — current namespace for
namespaced kinds, cluster-wide for cluster-scoped ones — so the menu hints at
what's actually present. Distinct from the existing "unavailable" muting (API
group missing/denied, M2-05b); an *empty* type is available, just has no objects.

**Honest cost read (why it's flagged soft):** the full/live version is **expensive
and fights a core principle**. kubecom lists a type only when you drill in (lazy;
fast cold start + async discovery, D8/principle 4). Knowing every menu item's count
up front means **listing every type on every namespace switch** — dozens of API
calls per switch, rate-limit exposure on big clusters — and the counts go stale
immediately (would need re-polling or watching all types). **Do not implement that
eager version.**

**Cheap subset (if we do anything):** gray *opportunistically* — cache the
last-known emptiness per `(type, namespace)` from types the user has **already
opened**, and gray those that came back empty. Zero extra API calls (reuses the
watch we already ran), and the graying self-corrects as the live watch adds/removes
rows. Partial (only reflects visited types) but free, and it degrades honestly.

**Recommendation:** either the cheap opportunistic variant or **dismiss** — record
the decision either way. If doing it: keep "empty" visually distinct from
"unavailable/denied", and don't gray a type whose count is simply unknown (never
visited) — unknown ≠ empty.
