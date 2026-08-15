# `enter` on a resource should open the actions menu

- Submitted: 2026-08-15
- Priority: high
- Area: row actions / drill-in (walked in S01)

Pressing `enter` on a resource row should launch the actions menu (`actions.menu`,
today the `:action ` palette) — that's where someone landing on a red thing wants
to be: describe, logs, edit, delete, children, pin, all in one step.

Ripple for the fold-in: `enter` is currently `nav.drillIn` ("open / drill into
selection") and is also the **universal accept key** on modals, prompts, pickers
and confirms — that stays. Only the resource-table context changes: `enter` opens
the actions menu, and drill-in moves **inside** it (an entry in the menu), so
drilling into an owner's pods is one menu pick away, not a lost gesture.
