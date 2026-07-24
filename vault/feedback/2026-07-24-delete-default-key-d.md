# Default delete key should be `d` (currently `x`)

- Submitted: 2026-07-24
- Priority: normal
- Area: keymap defaults (D10/D11)

Make the **default** delete binding `d` (vim-muscle-memory; `dd`-style deletion).
Today `d` is `res.describe` and `x` is `res.delete`, so this needs the describe
collision resolved — move describe off `d` (options: fold describe into the actions
menu `a`, or give it another key like `D`), your call, record the decision.

Note this interacts with the "unify view-yaml and edit" feedback and the general
viewer/action-surface rework — coordinate the two so the action keys are laid out
once, coherently, rather than in conflicting one-offs. Everything stays
registry-driven and rebindable (D11); this is just the shipped **default**.
