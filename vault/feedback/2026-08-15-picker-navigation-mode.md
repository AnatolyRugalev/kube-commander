# Popup/picker panes need a navigation mode: j/k move, / filters, current preselected

- Submitted: 2026-08-15
- Priority: high
- Area: namespace/picker popups (walked in S01)

Trying to use `j`/`k` in the namespace switcher starts typing into the search
field instead of navigating. The S01 trace: 4 dead `j` presses across two picker
visits (each followed by backspaces and a fallback to the arrow keys), 8
`down`-arrow presses, and one visit where the walker typed `name` + tab to make
progress. The arrows and typing are the workaround for j/k not navigating.

I want pre-launched panes (the namespace picker and any other popup) to use a
special mode where:

- `j`/`k` navigate the list,
- `/` starts filtering,
- the current choice (current workspace, or the current choice in any other
  popup) is preselected.

Note for the instrument: these picker `j` presses are the most important finding
of the walk and `kubecom keys analyze` does not report them (the picker is a
text surface, so its presses are excluded as dead ends — see
`2026-08-15-analyzer-text-surface-blindspot.md`).
