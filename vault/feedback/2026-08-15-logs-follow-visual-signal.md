# Follow needs a stronger visual signal than the [following] label

- Submitted: 2026-08-15
- Priority: high
- Area: logs view (walked during S03)

S03 answer (3), verbatim: "this is confusing, yeah." I could not reliably tell
live output from a frozen snapshot. The only cue is the header's text label
`[following]` / `[paused]` (`internal/tui/components/logsview/logsview.go`,
header at ~1150) — a word in a status line that the eye skips.

I want the following state to be unmistakable at a glance: paint the follow
indicator (or the header line) with a **background color** when following, and
plain when paused — the strong, positional cue `tail -f`-style tools give.
