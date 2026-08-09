# Code-quality audit: the lint gate does not enforce formatting, and the tree already drifts

- Submitted: 2026-08-09
- Priority: medium
- Area: tooling / `make check`

A code-quality audit of the `v1` tree found that `make check`'s lint step
cannot catch formatting drift, and the tree has drifted already.

## What I measured

- `.golangci.yml` enables `linters.default: standard` only. The standard set is
  errcheck, govet, ineffassign, staticcheck, unused — **no gofmt, gofumpt or
  goimports**. The config comment itself says "Ruleset starts lenient (the
  standard linters) per M0 and is tightened later" — that tightening has never
  happened, and M0's exit-criterion note "Confirm golangci-lint ruleset (start
  lenient, tighten later)" (`vault/milestones/M0-groundwork.md:44`) is still
  open.
- `gofmt -l internal cmd` reports **one real drift today**:
  `internal/tui/styles/styles.go` — the `Match: matchStyle(t),` field at line
  157 is misaligned (a one-space gofmt violation introduced by the LOGS-SEL-04
  leg, commit `826cdba`, 2026-08-09). `make check` passes anyway because no
  format linter runs.
- `go vet ./...` and the enabled linters are clean; this is purely the format
  gap, not a correctness one.

## Why it matters

`make check` is the canonical green gate (D17), and "green" is only meaningful
if the gate covers the things the project says it cares about. Formatting drift
is the cheapest class of technical debt to prevent mechanically and the most
annoying to clean up later — and it has already slipped through once. The M0
note promised a later tightening that never landed; this is that open thread.

## What I'd want

Add a formatting linter to the gate so `gofmt -l` drift fails `make check`
(and fix the one existing drift in `styles.go` in the same leg). Small, purely
mechanical, no behavior change. A future leg deciding the M0 "tighten later"
question more broadly (a wider linter set) would be welcome but is a separate,
bigger call — this item is only about the format floor.

The rest of the audit's findings are filed separately:
`2026-08-09-audit-test-suite-runtime.md` and `2026-08-09-audit-app-monolith.md`.
