# The analyzer is blind to dead ends on text surfaces

- Submitted: 2026-08-15
- Priority: high
- Area: instrument (STORY-03 `keys analyze`)

Found while reading the S01 trace back: `kubecom keys analyze` reported exactly
one dead end (`V` in browse) — but the raw trace holds **4 `j` presses in the
picker that resolved to no action**. The picker is a text surface, and `Analyze`
only counts a press as a dead end when `!rec.Text && !rec.Pending`
(`internal/keylog/analyze.go`), so the single most important finding of the whole
walk — "j/k don't navigate in popups" — is invisible to the tool built to find it.

I want the analyzer to surface text-surface reaches that look like navigation
attempts, or at least to say it skipped them. Ideas for the fold-in: report
"presses on text surfaces" separately, or record the picker's *interpretation*
(`j` typed vs `j` as a navigation intent) so a dead end is distinguishable from
ordinary typing. Whatever the shape, the tool must not silently miss the finding
its own line was built for (see `2026-08-15-picker-navigation-mode.md`).
