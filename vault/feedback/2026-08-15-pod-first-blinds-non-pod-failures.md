# Pod-first walking misses failures that are not pods

- Submitted: 2026-08-15
- Priority: high
- Area: health triage / finding (walked during S02)

S02 answer, verbatim: "I did not find it. I used 'pods' as the source of data
only and didn't notice the fifth failure." The fifth failure is a **PVC** — it is
not a workload at all — and a pod-first walk never surfaces it. The trace agrees:
pods were the only lens (9 logs, 3 describes, heavy pod-table scrolling); no
volume-claim kind was visited.

This is the sharpest finding of the main story: **a failure that is not a pod is
invisible to the operator's natural mental model.** The fold-in should make the
"unhealthy" surface span kinds, not just pods — e.g. the quick-access unhealthy
action (`2026-08-15-unhealthy-workloads-quick-access.md`) should include broken
claims/volumes and other non-pod resources, so the red things find the operator
instead of the operator having to guess which kind to visit.
