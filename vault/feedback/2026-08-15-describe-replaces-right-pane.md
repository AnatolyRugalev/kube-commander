# The describe view should fully replace the right pane

- Submitted: 2026-08-15
- Priority: normal
- Area: describe viewer layout (walked during S02)

The describe view opens as a popup/overlay that does not take the full right
pane — it wastes the space the pane would give it. In S02 describe was a primary
diagnostic (3 opens) and its output needed scrolling (half-page-downs right
after opening); a cramped viewer makes that worse.

I want the describe view to **fully replace the right pane** — the viewer gets
the whole right-pane width/height, not a modal inset — so a describe's output has
real room to read. The same probably applies to the other shared-viewer surfaces
(secret/YAML) if they render the same way.
