# A dedicated `events` action to list a resource's events

- Submitted: 2026-08-15
- Priority: high
- Area: row actions / describe (walked during S02)

Diagnosing the five `broken` failures I kept wanting the resource's **events**
(the scheduler's decision, the probe history) as its own view. Today events are
only reachable buried inside `Describe` — you have to open the describe text and
find them. In S02 the trace shows describe was the lever (3 opens) because it was
the only place events live.

I want a dedicated `events` action on a selected resource that opens a list of
its events (kind, reason, message, age) — the surface for "why is this red" —
instead of hunting through describe.
