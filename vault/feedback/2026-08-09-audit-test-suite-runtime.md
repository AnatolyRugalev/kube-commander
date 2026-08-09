# Code-quality audit: the test suite burns ~2 minutes on synchronous 5s toast timers

- Submitted: 2026-08-09
- Priority: high
- Area: tests / `make check` runtime

A code-quality audit of the `v1` tree found one problem that costs real
wall-clock on every `make check`: the tui test suite is dominated by tests that
**sleep the full 5-second toast auto-clear timer synchronously**.

## What I measured

- `surfaceError` / `surfaceNotice` return `tea.Tick(errorDisplay, …)` with
  `errorDisplay = 5 * time.Second` (`internal/tui/app.go:540`, `:1655`, `:1669`).
- The test helper `drain(cmd)` executes a returned command synchronously
  (`internal/tui/pin_test.go:645`). For a `tea.Tick` that means **calling `cmd()`
  blocks for the whole 5 seconds** until the tick fires.
- 17 tests in `internal/tui` take *exactly* 5.01s or 10.01s (three are 10s —
  they drain two notices, e.g. `TestPinTwiceUnpinsAndRemovesTheRow`, 10.01s;
  `TestScaleInvalidReplicasDegrades`, 5.01s; `TestPinIsANoOpForAnAuthoredEntry`,
  5.01s). Every one is a test that `drain`s a `surfaceNotice`/`surfaceError`
  tick and pays the full 5s for it.
- The `internal/tui` package as a whole takes ~190s; `searchview` ~53s (its
  debounce is 250ms but some tests drain several, and a few wait real time).
  A cold `make check` (build + test + vet + lint) is on the order of 4–5
  minutes, and the tests are the whole cost.
- `grep` for `t.Parallel` across all of `internal cmd`: **zero** uses in 1,392
  test functions. The suite is fully serial.

## Why it matters

`make check` is the canonical gate every leg and CI run pays (D17). ~90 seconds
of the tui suite's 190s is pure timer-sleep that asserts nothing — the tests
only drain the tick to read the *synchronous* part of the command (the notice
text / the write), then throw the timer message away. It is also self-masking:
a test that must assert on the tick's *arrival* can't distinguish the two, and
the 5s cost grows with every new `surfaceNotice` assertion.

## What I'd want

Make the toast duration injectable so tests can drive it to ~0 instead of
draining a real 5s tick — e.g. a `WithToastTimeout`-style option (or a settable
package-level variable used in the `tea.Tick`, the same pattern
`keymap.SequenceTimeout` already uses at `internal/tui/keymap/sequencer.go:15`).
The 17 tests then drop from 5–10s to milliseconds; `t.Parallel` on the
independent hermetic tests would compound the gain. This is a test-infra change:
no behavior in the running binary moves, so `make check` stays green throughout.

The rest of the audit's findings are filed separately:
`2026-08-09-audit-format-gate.md` and `2026-08-09-audit-app-monolith.md`.
