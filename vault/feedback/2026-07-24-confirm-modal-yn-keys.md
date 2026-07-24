# Confirmation popup should accept y / n

- Submitted: 2026-07-24
- Priority: normal
- Area: confirm modal (M3 / D115)

The confirm modal should accept **`y` to confirm** and **`n` to decline**, the
universal muscle memory for a yes/no prompt. Today it only takes `enter`
(`nav.drillIn`) / `esc` (`nav.back`) — D115 deliberately avoided raw `y`/`n`.

**Reconcile with D11 (no hard-coded keys):** don't match raw `y`/`n` in view code —
add them as **registered actions** so they stay rebindable. Introduce
`confirm.accept` (default `y` **and** `enter`) and `confirm.decline` (default `n`
**and** `esc`), resolved through the keymap like everything else, and route the
modal on those actions. This supersedes the "no y/n" part of D115 while keeping the
zero-hard-coded-keys invariant. Keep `enter`/`esc` working too (multiple keys per
action is already supported).
