# Code-quality audit: `internal/tui/app.go` is a 4.6k-line god object, and the plan's `views/` split never happened

- Submitted: 2026-08-09
- Priority: medium
- Area: structure / maintainability

A code-quality audit of the `v1` tree found the TUI shell has grown into a
single-file god object, and the package layout the plan committed to was
silently abandoned.

## What I measured

- `internal/tui/app.go` is **4,635 lines / 149 functions** (the next-largest
  file in the repo is `logsview.go` at 1,248). It holds:
  - 19 kube-seam interface definitions (lines 43–520);
  - the entire `Model` struct (~515 lines, 60+ fields, lines 564–1079);
  - `update()` — a 325-line message router with 57 `case` arms (line 1230);
  - the whole browse-view composition, key routing, filter/sort/search glue,
    mouse handling, resize, and `View()` (line 4587).
- `internal/tui/tui_test.go` is the test twin: **6,028 lines / 188 tests**.
- The plan explicitly promised a `views/` split: REWRITE_PLAN's target layout
  (`vault/REWRITE_PLAN.md:54`) lists `views/ # browse (2-pane), logs, describe,
  yaml`, and D52 re-committed to it ("`internal/tui/{app,msg}.go`, …
  `components/*`, `views/*`"). **There is no `views/` directory** — `ls
  internal/tui` shows `components/` and a flat pile of `*.go`, and the browse
  view, logs view, search view, palette, reauth, etc. are all loose root-package
  files. No decision in `decisions.md` ever re-scoped or revoked the `views/`
  layout; it was just not followed.
- Contrast: the kube layer (`internal/kube`, 27 files, no file over ~810 lines)
  and the components (`components/*`) are well-factored by comparison. The
  problem is concentrated in the root `internal/tui` package and its test file.

## Why it matters

The stated goals put structure front and center: "replace the aging foundation
with a modern Go TUI stack", REWRITE_PLAN's package layout, D52's explicit
`views/*`. A 4.6k-line god object is exactly the "aging foundation" failure
mode this rewrite was meant to avoid, and it makes each new surface (the
pattern of the last weeks' feedback legs) more expensive to add: one more case
arm, one more field on the giant Model, one more 60-line test in a 6k-line
file. The claimed layering (`app.go = view routing, global keys`) has been
eroded — app.go is now also the browse view, the action engine, and the
per-cluster state holder.

## What I'd want

Not necessarily a mechanical split of all 149 functions — some (the seam
interfaces, the Model) are cohesive where they are. But the *drift from the
recorded plan* should be resolved one way or the other: either start pulling
cohesive chunks into real subpackages (e.g. a `views/browse` package owning
the browse composition and its ~30 cases in `update()`, mirroring how
`searchview`/`logsview` already got their own packages), or record a decision
(Dn) that the `views/` layout is revoked and the root-package accumulation is
the accepted shape, so the next leg isn't guessing against the plan. This is
deliberately filed as **medium**, not a blocker: the code is green, race-clean,
well-commented and 92.6%-covered — this is about the next N legs' cost, not the
current binary.

The rest of the audit's findings are filed separately:
`2026-08-09-audit-test-suite-runtime.md` and `2026-08-09-audit-format-gate.md`.
