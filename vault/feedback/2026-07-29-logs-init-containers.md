# Logs view doesn't support init containers

- Submitted: 2026-07-29
- Priority: normal
- Area: logs view

The logs view only lets you pick from a pod's regular containers. Init
containers aren't listed/selectable at all, so there's no way to see their
logs (useful for debugging init failures, e.g. an init container stuck or
erroring before the main container starts).
