# Quick access to unhealthy workloads (suggested: shift+H)

- Submitted: 2026-08-15
- Priority: high
- Area: browsing / health triage (walked during S02)

On-call for the `broken` namespace I had to scroll through the whole table to find
what was wrong. The S02 trace: 43 half-page-downs in 2m54s, and the walk spent
most of its time navigating to the failing rows. There is no way to jump straight
at the workloads that are not healthy.

I want a quick-access action that surfaces unhealthy workloads (pods not
Running/Ready, failing to start, unschedulable, etc.) — I suggested `shift+H`
(healthy vs h). The fold-in decides the exact gesture; the point is one keypress
from anywhere to "what's broken, sorted to the top / filtered".
