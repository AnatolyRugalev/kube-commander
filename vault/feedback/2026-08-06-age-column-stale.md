# Age column isn't recalculated while the panel stays open

- Submitted: 2026-08-06
- Priority: normal
- Area: resource list / age column

Leave a resource panel (e.g. pods) open for a while without navigating away,
and the `AGE` column stops reflecting reality — it looks like it's computed
once (probably at fetch/render time from the resource's creation timestamp)
and never refreshed on a tick, so it drifts stale the longer the panel stays
open.

Want: `AGE` should recompute live (e.g. re-render periodically, or compute
from `time.Now()` on each render pass rather than caching a rendered string),
same as any other elapsed-time display.
