# Consider newest-first log order (grafana/datadog style)

- Submitted: 2026-08-15
- Priority: normal
- Area: logs view (walked during S03)

A design to **consider**, not a demand: reverse the log entry order so the
newest line is at the **top**, filter box also at the top — grafana and datadog
both do this. Reading top-to-bottom suits a file; a live stream is the opposite:
the first thing you see is the fresh line, and your filter input sits right over
the newest entries instead of a wall of old ones.

Trade-off to weigh in the fold-in: it inverts vim's `G`-to-bottom / scroll-up-to-
older muscle memory and changes what "scrolling down" means in the logs view, so
it would land with the follow-rearm work above (`2026-08-15-logs-scroll-past-end-resumes-follow.md`)
rather than on its own.
