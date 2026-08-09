# No-previous-instance logs: keep the log view open, the toast is enough

- Submitted: 2026-08-09
- Priority: normal
- Area: logs view (previous-instance toggle, D177)

From the maintainer, verbatim: "Show error and exists log view. It shouldn't
exit log view." (i.e. it shows the error and *exits* the log view — and it
should not exit.)

Observed on a pod that has never restarted: `L` then `Ctrl+P` shows the
apiserver's `previous terminated container … not found` error **and closes the
log view entirely**, dumping the user back to the table. The current close was
deliberate ("the view closes rather than sit empty"), but the maintainer
overrides: staying in the log view on the current instance's stream, with the
toast naming why there is no previous one, is the wanted behavior. The user was
reading logs; an error about an optional toggle should not take the view away.

Expected: `Ctrl+P` on a pod with no previous instance → status-bar toast with
the server's wording, the running instance's stream keeps showing. Flipping
back is a non-issue because nothing changed.
