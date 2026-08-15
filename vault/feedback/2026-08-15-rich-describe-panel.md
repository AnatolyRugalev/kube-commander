# A rich describe panel that surfaces issues with color

- Submitted: 2026-08-15
- Priority: high
- Area: describe view (walked during S04/S05; viewer feedback)

The current describe output is weak — a plain dump. For the on-call job I want a
**rich describe panel** that immediately surfaces what is wrong, with color:
health/phase painted (CrashLoopBackOff, ImagePullBackOff, Unschedulable,
unready), key conditions highlighted, and the problem lines standing out from
the noise. Today S02's diagnosis meant reading the describe dump by eye; a rich
describe should make the failing part the thing the eye lands on first.

Shape for the fold-in: reuse the describe data but render it as a styled panel —
phase/status/conditions in theme-aware colors, problem states emphasized — and
pair with the events work (`2026-08-15-events-action.md`) so "why is this red"
is one glance. Lands with the describe-replaces-right-pane item
(`2026-08-15-describe-replaces-right-pane.md`) so the panel has the room to
carry color.
