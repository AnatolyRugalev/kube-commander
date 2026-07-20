# Right panel should show a welcome page at startup

- Submitted: 2026-07-20
- Priority: normal
- Area: browse view / right pane

At startup, before any resource is selected, the right pane should show a
**welcome page** rather than an empty/blank table. Good content for it: the
kubecom name/version, the current context + namespace, a short "pick a resource
on the left to begin" hint, and maybe the top few keybindings (help = `?`). It
should replace the right pane until the user drills into a resource, then the
live table takes over.
