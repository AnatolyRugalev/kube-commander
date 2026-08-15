# Log line selection should be shift+V

- Submitted: 2026-08-15
- Priority: high
- Area: logs view (walked in S01)

In the logs view I pressed `shift+V` to select lines — it resolved to nothing
(dead end at 13:53:52 in the S01 trace), and I had to fall back to `v` (select)
then `y` (yank) 1.5 seconds later. The reach was for visual-mode line selection.

I want `shift+V` to select log lines (vim's linewise-visual muscle memory). Decide
with the fold-in whether `V` replaces the current `v` (`logs.select`) or both
select; the point is that `V` must select.
