# AGE column sorts lexicographically, not by actual age

- Submitted: 2026-08-25
- Priority: normal
- Area: table sorting / AGE column

Sorting a resource list by AGE gives nonsense order. It's comparing the rendered
text, so `10d` lands before `2d`, and `5m`, `5h`, `5d` interleave by digit rather
than by duration. What I want is the obvious thing: sorting by AGE orders rows by
how old the objects actually are.

Note for whoever picks this up: `sortRows` in
`internal/tui/components/table/table.go` only special-cases numeric column types
(and the metrics overlay's raw-sample key). AGE arrives from the server-side
printers as a `date`/`string` column holding kubectl's human duration ("2d",
"5h30m", "90s"), so it falls through to the case-insensitive text compare. The
fix likely wants a duration-aware sort key for that column — and it should stay
correct across the whole kubectl range of shapes (`<invalid>`, `0s`, `4y32d`),
with the row order stable when two objects share a rendered age.
