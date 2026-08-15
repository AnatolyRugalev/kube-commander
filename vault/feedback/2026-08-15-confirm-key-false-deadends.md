# The recorder mislabels confirm-modal keys as dead ends

- Submitted: 2026-08-15
- Priority: high
- Area: instrument (STORY-02 recorder / STORY-03 analyzer)

The S02 trace reports `esc 2 modal-confirm` as unresolved presses — but both were
**handled**: each `esc` declined the delete-confirm (`confirm.decline` is bound to
`esc`, `keymap.go:489`) and the walker immediately reached browse actions
afterwards. The instrument lies.

Root cause: the recorder writes every non-browse key with a blank action
(`app.go:1270`, `m.recordKey(msg, mode, "", keymap.ResultNone)`), and the confirm
modal is deliberately not a text surface (`keylog.go:66`), so the analyzer counts
each handled accept/decline as a dead end.

The confirm context *does* resolve keys (`ConfirmAction`, `routeModalConfirmKey`),
so the recorder should record the resolved `confirm.accept` / `confirm.decline`
there — an accept or decline that actually worked is not a reach for a missing key.
This is the mirror image of `2026-08-15-analyzer-text-surface-blindspot.md`: one
hides real dead ends, this one fabricates them.
