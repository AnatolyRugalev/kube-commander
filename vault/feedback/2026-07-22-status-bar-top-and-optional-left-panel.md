# Status bar to the top (show resource type); make the left panel optional

- Submitted: 2026-07-22
- Priority: normal
- Area: layout / navigation (direction-setting — likely several legs)

This is a small layout change plus a direction for navigation. Please **triage it
into board tasks** (do the first slice; queue the rest) and record a decision for
the navigation model.

**Now (small):**
- **Move the status bar to the top** of the screen (currently bottom).
- Have it **display the selected resource type** (e.g. the current kind being
  browsed) alongside context · namespace.

**Direction (bigger — split into tasks):**
- Make the **left menu pane optional**: a keybind to show/hide it on demand, so you
  can browse pane-free with just the table + top status bar.
- Later, the menu can become a **popup** (overlay, per the popup-overlay feedback)
  rather than a fixed pane — full pane-free navigation.
- Target **resource-switch flow** (command-palette style, à la k9s `:`):

  `<resources hotkey>` → `/` (search) → type `sec` (filter) → `enter`
  (switch to Secrets) → land on the Secrets list.

  i.e. a hotkey opens the resource list (as a popup/overlay), `/` filters it by
  substring, and Enter switches the table to that resource — no persistent left
  pane required.

Keep it consistent with the existing keymap registry (no hard-coded keys, D11) and
the zero-shared-mutable-state model. The top status bar + resource-type display is
the first concrete slice; the optional/pop-up menu and the switch flow are the
follow-on tasks.
