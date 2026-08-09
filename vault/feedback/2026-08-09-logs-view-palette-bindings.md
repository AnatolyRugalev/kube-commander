# Logs view: allow the command palette, and give previous-logs a better binding

- Submitted: 2026-08-09
- Priority: normal
- Area: logs view / keybindings

From the maintainer, verbatim: "Ctrl+P is a bit weird. I think we need to allow
command palette inside logs view at the very least and have better default
bindings."

Two asks:

1. **The command palette (`:`) should work from inside the logs view.** Today
   the logs view swallows keys the rest of the app treats as global, so actions
   that exist (previous-instance toggle among them) are undiscoverable from
   there. Letting the palette open over the logs view fixes discoverability
   without inventing new bindings.
2. **A better default binding for the previous-instance toggle than `Ctrl+P`.**
   No specific key requested — pick a conventional one, record the choice in
   `decisions.md` if it turns out load-bearing, and keep the keybindings doc
   (README / docs) in sync in the same leg.
