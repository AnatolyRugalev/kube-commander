# Logs view: tail last 1k instead of full history, and rendering is slow

- Submitted: 2026-07-29
- Priority: high
- Area: logs view

Opening logs currently streams from container boot, which is slow and wasteful
on containers that have been running a long time. Want it to default to
tailing the last ~1000 lines instead of replaying the entire history.

Separately, the append/render path is slow: every incoming line seems to cost
real time to render, so even when we do tail from the beginning it takes ages
to catch up to "now". This needs to be fixed regardless of the above — once we
tail last-1k it'll matter less, but the per-line render cost is a real
inefficiency and will still bite on high-volume logs.
