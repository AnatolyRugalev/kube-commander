# Logs: match highlight should carry the bright (yellow) background

- Submitted: 2026-08-09
- Priority: normal
- Area: logs view / themes (D252 pt 1)

From the maintainer, verbatim: "Looks good, but needs improvement. Highlighted
text (matches) should have bright (yellow) background. Selection row looks
good."

On a line that has both, the selection bar and the match highlight do read as
two things (the LOGS-SEL-03 question is answered: **they coexist fine**), and
the selection row is good as-is. What reads wrong is the match itself: it
should render with the **bright yellow background** (the strong highlight), not
the subtler treatment it has today.

Note the existing constraint this sits next to: D252 pt 1 keeps both
backgrounds visible on the same line deliberately, with measured ratios
(4.05–9.89:1 across the dark palettes). Brightening the match background is a
role-color change, not a redesign — but keep the two-background rule, and keep
`catppuccin-latte`/`solarized-light` out of scope (known-bad until THEME-05).
