# Don't separate "view YAML" and "edit" — one editable object view

- Submitted: 2026-07-24
- Priority: normal
- Area: viewers / edit (M3-03 YAML, M3-15 edit)

Having a separate read-only **YAML viewer** (`y`) and a separate **edit** (`e`)
action is redundant. Collapse them into **one action that opens the object's YAML
in a proper editor** — view and edit are the same thing; if you don't change
anything you just close it, if you do it applies on save (the M3-15a `Clients.Update`
apply path already exists). This also removes an action key and simplifies the
surface.

Notes:
- "Proper editor": prefer suspending to the user's real `$EDITOR` (the M3-15b flow)
  so they get their own editing environment — that IS the "proper editor" for YAML.
  An in-TUI editable buffer is a fallback if we want no-suspend editing later.
- **Describe and logs stay read-only** (you can't apply a describe) — this
  unification is specifically about the object's YAML. Describe/logs remain
  viewers (though they should still scroll/search well — see the logs feedback).
- Coordinate with the `delete → d` / key-layout feedback: fold the freed-up
  view/edit keys into the coherent action surface, and record the superseding
  decision (this supersedes the M3-03-vs-M3-15 split).
