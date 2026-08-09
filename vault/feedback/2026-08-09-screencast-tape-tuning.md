# Screencast tape: fix the search example, make reruns idempotent, show more

- Submitted: 2026-08-09
- Priority: normal
- Area: docs / screencast (M5-09 tape)

The GIF is recorded and live in the README (commit 484e60c, recorded by the
maintainer with tmux-driven captions added). Remaining gaps, from the
maintainer verbatim:

- **"search across cluster doesn't find anything (bad example)"** — the tour's
  search step demos a query that returns zero results on the dogfood cluster.
  Pick a query that visibly matches.
- **"initial state gets modified when rerunning the tape"** — a rerun does not
  start from the same screen as the first run. Likely suspect: the CTX-MEM
  pane/scope memory (D240/D255) persists `LastResource`/drill-in state across
  launches, so the tape's second run opens on the remembered pane instead of
  the welcome screen. The tape (or `make screencast`) should reset or
  point at a throwaway kubecom state dir so recordings are reproducible.
- **"we need to show off more features and add more captions"** — extend the
  tour; captions infrastructure (tmux-driven) is already in place from the
  maintainer's pass, build on it.

If keys change, change the `# kubecom-action:` annotations with them —
`make check` validates them.
