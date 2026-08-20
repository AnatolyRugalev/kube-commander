# M5 — Release & Docs

**Status:** `in-progress` (2026-08-20) — STORY-06h-1 (the viewer fills the right pane) landed: the shared pager behind describe/YAML/secret/events now takes the browse view's right pane whole instead of floating as a centered inset, so a describe dump has real room to read while the resources menu stays beside it (D284); 06h-2 (the painted describe panel) is what remains of 06h. Earlier: STORY-06k-2 (the search result preview) landed and closed STORY-06k: the cluster search now previews the highlighted hit under the results — identity plus the object's own printed row, carried on the hit itself so a cursor movement costs no request — so the right result is picked before the view closes (D283). Earlier: STORY-06k-1 (single enter reaches a search hit) landed: `enter` in the cluster search opens the highlighted hit from the query line itself and a movement over the list is what hands it the keyboard, so a result costs one press instead of two (D282); 06k-2 (the result preview) is what remains of 06k. Earlier: STORY-06j-2 (scroll-past-the-end re-arms follow) landed: a downward press at the newest log line rejoins the stream, so a reader who scrolled back down is live again without reaching for `G` (D281); 06j-3 (newest-first, weighed) is what remains of 06j. Earlier: STORY-06j-1 (the painted follow indicator) landed: `[following]` now renders as a `styles.Follow` badge (canvas-on-Success on a dark canvas, bold alone on a light one) so live vs frozen is unmistakable, with 06j-2 (follow-rearm) and 06j-3 (newest-first, weighed) split back to Backlog (D280). Earlier in the fold-in:  the UX-validation line's instrument is complete (STORY-01…04 landed: fixture, `--keylog`, `keys analyze`, the stories) and **the walk is done (STORY-05, 2026-08-15)**: the maintainer walked all five stories against the fresh fixture and handed back five traces plus 23 feedback files. **The fold-in is underway (STORY-06a…06f):** the letter remap landed — search `ctrl+f`, ns.switch `N`, delete `D`, describe `d`, `#`/`space` for the displaced keys, `V` selects logs — with the keymap doc, the tape and the README updated and seven key feedback items closed (D269); then the S-mode sort landed — `s` freed, `S` focuses the column-header row, `h`/`l` move, `enter` toggles direction, `esc` returns, `x` clears and resolves the mode, with the `HelpSort` hint context and a table sort cursor (D270); then `enter` on a resource row opened the actions menu — a table key context (ctxTable) binds `actions.menu` to `enter` while the table owns the keys, `a` frees up, and drill-in (Show pods) stays an entry inside the menu (D271); then **picker navigation mode** — pickers open with the list focused, j/k navigate, `/` opens the filter, and the current choice is preselected, flipping D194 pt 2 for value pickers while the palette verb list keeps type-to-filter (D272); then the **instrument's mirror fixes** — the recorder writes the confirm modal's resolved action (a handled `y`/`n`/`enter`/`esc` is `confirm.accept`/`confirm.decline`, not the S02 `esc 2 modal-confirm` dead end) and `keys analyze` reports text-surface presses in their own section so the picker `j`s of S01 are findings again (D273); then **a dedicated `events` action** — the selected object's own core Events (the `kubectl get events` columns) as their own viewer list, `E`, the surface for "why is this red" (D274); then **the unhealthy quick-access** — `H` narrows the current table to the rows the M4-06 classifier reads as unhealthy, composing with `/` and cleared by esc, with an `unhealthy` status marker (06g-1, D275); then **the cross-kind sweep primitive** — `kube.Scan` fans out a concurrent, capped, fault-isolated list across kinds keeping the rows a caller-supplied `RowFilter` accepts, the engine the 06g-2 surface will ride (06g-2a, D276); then **pane-scoped filtering** — `/` narrows whichever pane holds focus, the kinds in the left menu or the table rows, with the menu gaining its own filter view over the authoritative kind set and the D203 alias surface (06m, D277); then **the cross-kind unhealthy list component** — `components/unhealthyview`, the full-screen streaming list of ScanHits the sweep feeds, navigable with a cursor, each row showing kind · name · namespace · the offending cell in its role's hue, alongside the two seams it needed: `kube.ScanHit` now carries the columns the row sat under (so the reason is renderable without re-listing) and the table component exports `UnhealthyRow`/`UnhealthyCells`, the M4-06 classifier lifted to a whole table — the shared source the sweep's filter and the per-kind `H` filter both use (06g-2b-1, D278); then **the cross-kind unhealthy wiring** — `U` (`app.unhealthyScan`) opens the list and runs `kube.Scan` over the menu's kinds with `table.UnhealthyRow` as the predicate, pumping hits in over a generation-guarded pump (the Scanner seam on Cluster, D279), each drill-in switching browse to the hit's kind with the object pending selection, closing the pod-first feedback (`2026-08-15-pod-first-blinds-non-pod-failures.md`) — a failure that is not a pod is now findable from anywhere (06g-2b-2). What remains: the rest of STORY-06 (06h-2, 06i, 06j-3, 06l), TAPE-01, DOC-03…05, then the `v1.0.0` tag, the tap/AUR access, and the branch rename (M5-11).
**Phase:** REWRITE_PLAN Phase 5

_Scope expanded into ordered, leg-sized Backlog slices **M5-01 … M5-11** on the
[board](../tasks/board.md) (M5-PLAN, D173). M5 is unlike its predecessors in one way that
shapes the plan: **its output leaves the repo and cannot be recalled** — a pushed tag is
cached immutably by the Go module proxy — so "green or revert" does not apply and D173
splits every publishing act off to a human. The slices are: the DoD audit (M5-01), the
artifact's correctness (M5-02 build metadata, M5-03 release workflow + CI dry run),
migration (M5-04/05), distribution (M5-06 Homebrew, M5-07 AUR), the vhs
screencast (M5-09), and the irreversible end (M5-10 tag, M5-11 `v1`→`main`).
**A second line was added on 2026-08-15** (UX-PLAN/D268), in front of the tag rather than
after it: the rc proved the pipeline and nothing about the product, so **STORY-01…06** build
a committed k3s fixture and a `--keylog` recorder, write the user paths down as stories, have
the maintainer walk them and fold the findings back in; **TAPE-01** re-cuts the screencast
around the main story and **DOC-03…05** reorganize the README into a landing page over a
`docs/` that gains `usage.md` and `troubleshooting.md`. Per-leg history: `vault/journal/`._

## Goal

Ship v1: documented, packaged, and installable, with a clean migration story
from the old kube-commander.

## Scope

- Rewrite README for kubecom: what/why, install, usage, WSL2 note for Windows —
  **largely already done**, and continuously, because D68 makes every leg that moves the
  install/launch/config/usage surface update the README in the same leg. What is left is
  not a rewrite but the install section's *release* half: each distribution slice
  (M5-06/07/08) documents its own path as it lands, and M5-10 drops the "why not `@v1`"
  explainer once a real tag exists. There is deliberately **no standalone README slice**
  (D173 pt 3).
- Screencast via **vhs** (replaces the old terminalizer GIF pipeline). **M5-09 ✅ 2026-07-30**
  (D181): `docs/screencast.tape` scripts the browse → filter → logs → describe tour (opened
  through the resource palette, so it does not depend on menu position), `make screencast`
  builds this checkout's binary onto PATH and runs vhs on it, and three guards hold the tape
  to the registry — every keypress is annotated with the action it triggers and checked
  against `DefaultKeymap`, the tour must press the headline actions, and the README may
  reference the GIF exactly when the file exists. The tape is validated by `vhs validate`
  (vhs is `go install`-able); **recording** needed ttyd + ffmpeg, a real cluster and a real
  terminal (D79) — **  recorded by the maintainer 2026-08-09** against the k3d dogfood
  cluster, `docs/screencast.gif` + README embed (484e60c). The residual tuning
  (feedback `2026-08-09-screencast-tape-tuning`) **landed 2026-08-09 (D261)**:
  the search demo now types the same `shop` the filter step already matched, so the
  cluster-wide pass cannot come back empty; the tape wipes a throwaway XDG dir before
  every launch so reruns start from the same welcome screen; and the tour grew a
  theme-preview step (THEME-07) and the help overlay, both captioned. Re-recording is
  the maintainer's, whenever they next want to refresh the GIF.
- Keybindings reference (generated from the `keys/` bindings where possible) — **already
  met**: `docs/keybindings.md` is generated from the keymap registry by `make keys-doc`
  and `make check` fails on drift (M2-01e/D51). Nothing to build; M5-01 ticks it.
- Migration note: old `~/.kubecom.yaml` auto-migration + any behavior changes — the
  mechanism landed in M2-12a/12b, but M5-PLAN found its report **stale**: it still told
  the user themes were dropped, which stopped being true at M4-11/12. **M5-04 ✅ 2026-07-30**
  (D179): a legacy `currentTheme` naming a palette v1 still ships is written to `theme:` in
  the migrated config (`solarized` → `solarized-dark` via an enumerated legacy rename), an
  unported name falls back to the default with the note listing the themes that exist, and the
  palette tree stays deliberately un-migratable. **M5-05 ✅ 2026-07-30** (D180): the whole
  launcher path is verified against a `~/.kubecom.yaml` *generated by the 2020 writer* —
  `protojson.Marshal(pb.Config)` → `yaml.JSONToYAML`, the two calls `master:config/config.go`
  made — with the fixture pinned key-for-field to a verbatim copy of `master:pb/config.proto`.
  A genuinely *real* file is the human task `2026-07-30-real-legacy-config-migration`.
- **Distribution** (**#28**): goreleaser release, Homebrew tap, AUR refresh.
  **M5-06 ✅ 2026-07-30** (D182) wired the first:
  a `homebrew_casks:` cask (the formula route is closed — `brews:` is deprecated and
  `goreleaser check` fails on it, which also makes Homebrew macOS-only here) publishing to
  the tap the 2020 build already used, inert without `HOMEBREW_TAP_TOKEN` and carrying the
  Gatekeeper quarantine hook. It also added `goreleaser check` to the CI dry run, which
  caught `conflicts.formula` being accepted and then silently dropped — so the stale 2020
  formula must be deleted from the tap by hand (human task
  `2026-07-30-homebrew-tap-access`). **The tap moved on 2026-08-09 (D253)**: the
  maintainer chose an org-level tap (`neuroplastio/homebrew-tap` → `brew tap
  neuroplastio/tap`), created by the release-namespace fold-in, replacing the 2020
  personal tap with no redirect, and the human task now narrows to the token alone.
  **M5-07 ✅ 2026-07-30** (D183) wired the second: an
  `aurs:` `-bin` package built from the released linux archives, with no kubectl dependency
  (D2, which the 2020 PKGBUILD declared) and a real `conflicts` with the 2020 package. The
  "refresh an existing package" framing turned out to be impossible — goreleaser forces the
  `-bin` suffix and the AUR ties `pkgbase` to the repo name, so `kube-commander` is
  unreachable from any config and `kubecom-bin` is a *new* package with no upgrade path
  (human task `2026-07-30-aur-package-access`). AUR is otherwise unaffected by the org move
  (D253 pt 3); only repo secrets may need re-creating. **M5-08 (Docker) ✅ 2026-07-30 then
  reverted 2026-08-09 (D254)**: the maintainer decided to *drop container builds for now*,
  so the `dockers_v2:` image at `ghcr.io/anatolyrugalev/kubecom` that M5-08 built and ran
  in the sandbox is removed — no Dockerfile, no image pipeline, no container install path.
  The goreleaser skeleton exists (M0-06/D27) but
  carries **no publishers** and has **no workflow to run it** — `.github/workflows/` holds
  only `ci.yml` (and the release workflow it gains below). The `Commit`/`Date` ldflags are done (M5-02/D175, drift-guarded) and
  M5-03 added `release.yml`, the workflow that runs them (D176). Each publisher is
  then its own slice because each needs a human-owned external resource (a tap repo, an AUR
  key) — Docker would have been the exception (its own workflow token), but D254 dropped it.
- **Restore remote `go install`**: tag a real `v1.x.x` release so
  `go install github.com/neuroplastio/kubecom/cmd/kubecom@latest` works
  again (today `@v1` is semver-parsed as a version query, not the branch — see
  FB-go-install / journal 2026-07-20). Renaming `v1`→`main` also dissolves the
  branch-vs-semver collision. Update the README install section back to the remote
  one-liner once tagged.
- Merge/prepare `v1` toward becoming the default branch when ready.

## Exit criteria

Each criterion names the slice that closes it (M5-PLAN/D173). None can be ticked on
hermetic evidence alone: four of the five assert something about an artifact that has left
the repo, which is the D79 line — so they are ticked when the human tasks their slices
raise come back done, not when the config that would produce them compiles.

- [ ] README + keybindings docs current and accurate.
      (Keybindings half is **already met** — generated + drift-gated by `make check`,
      M2-01e/D51. The README half is continuously maintained under D68 but cannot be called
      accurate until the install paths it will describe exist: M5-06/07/08 add them, and
      **M5-10 ✅ 2026-07-30** did the pass — it found the status banner still announcing that
      the browse UI was yet to land (stale since M2-RUN, 2026-07-20), rewrote it, and added
      the release-archive install path the three package paths had left implicit. The
      remaining README edits are the ones only a tag can make true (drop the `@v1` explainer,
      restore `go install …@latest`), listed as step 3 of the tag human task — and since
      **DOC-01 ✅ 2026-08-07** (D241) those live in `docs/install.md`, the README keeping only
      a short install block that links to it. The **screencast** half is **closed**: M5-09 ✅
      2026-07-30 landed the tape, the `make` target and the guards (D181), and the maintainer
      recorded it against the k3d dogfood cluster on 2026-08-09 — `docs/screencast.gif` plus
      the README embed (484e60c), `TestScreencastAssetAndReadmeAgree` holding the pair honest.
      The residual tuning the maintainer asked for (a search example that matches, idempotent
      reruns, more of the tour) is feedback `2026-08-09-screencast-tape-tuning`, tracked
      separately from this criterion. The box itself stays unticked on the README half only.)
- [x] `goreleaser release` produces Linux+macOS artifacts from a tag via CI.
      **— ticked 2026-08-12 (DOC-02) on the `v1.0.0-rc.1` run**: the tag was pushed
      2026-08-10, `release.yml` ran `make check` on both platforms and then `goreleaser
      release`, and the published release carries four `.tar.gz` archives and four bare
      binaries for `linux`/`darwin` × `amd64`/`arm64` plus `checksums.txt`, with a
      downloaded binary reporting its real version/commit/date (journal `2026-08-10.1`).
      This criterion asks for artifacts from a tag via CI, which a pre-release tag
      satisfies as fully as a stable one; the human tag task stays open for `v1.0.0`,
      which the *other* criteria need. The pre-DOC-02 history:
      (M5-02 ✅ 2026-07-30: the artifact reports its own commit and build date, guarded by
      `TestGoreleaserSetsAllVersionVars` (D175). M5-03 ✅ 2026-07-30: `release.yml` exists —
      `goreleaser release` on a `v*` tag, gated on `make check` via ci.yml, plus a
      `--snapshot --clean` dry run on every push that keeps the config from drifting (D176).
      The pipeline is complete; it has just never been fired by a tag. **M5-10 ✅ 2026-07-30**
      (D185) ran the closest thing to firing it that leaves nothing behind: `goreleaser
      release --clean --skip=publish` against a *real* local `v1.0.0-rc.1` tag, which built
      all four platforms, rendered the cask, the PKGBUILD/`.SRCINFO` and the release notes,
      and succeeded — the tag was then deleted, never pushed. It also found and fixed the one
      thing only a real rendering could show: the changelog filters matched unscoped subjects
      and every commit here is scoped, so the notes were 373 lines opening with ~180 board
      claims (now 151, grouped, guarded).)
- [ ] Homebrew/AUR install paths verified.
      (M5-06/07, one each. "Verified" means installed from, so each needs its human task
      back.
      **M5-06 ✅ 2026-07-30** (D182) closed the agent-side Homebrew third: `goreleaser check`
      is clean, `goreleaser release --snapshot --clean` renders
      `dist/homebrew/Casks/kubecom.rb` with the quarantine postflight, and the
      token→`skip_upload` template was proved to flip both ways. The *installed-from* half
      needs a macOS box and a published tag, and the account-level act the agent cannot
      perform is creating `HOMEBREW_TAP_TOKEN` — the tap itself was created by the agent
      (D253, `neuroplastio/homebrew-tap`), so it waits on human task
      `2026-07-30-homebrew-tap-access`.
      **M5-07 ✅ 2026-07-30** (D183) closed the agent-side AUR third: `aurs:` renders
      `dist/aur/kubecom-bin.pkgbuild`/`.srcinfo` for both arches with no kubectl dependency,
      and the key→`skip_upload` template was proved to flip both ways. The package is
      **renamed** — goreleaser forces the `-bin` suffix, so `kube-commander` is unreachable
      and there is no upgrade path — which makes retiring the 2020 package, alongside the
      AUR account/SSH key, human task `2026-07-30-aur-package-access`.
      **The Docker third is gone**: M5-08 built and ran the image (D184), then the maintainer
      dropped container builds entirely (D254, 2026-08-09) — no image to verify, no criterion
      to hold. M5-10's pre-flight carries the tap/AUR push checks, not a container one.)
- [x] Migration verified from a real legacy config file. **— ticked on the generated fixture,
      not on a real file (D231).**
      (M5-04 ✅ 2026-07-30 fixed the stale theme report and made the selection actually
      migrate (D179). **M5-05 ✅ 2026-07-30** (D180) closed the agent-side half: the launcher's
      whole migration path — legacy file → `Migrate` → `SaveFile` → `LoadFile` →
      `resolveTheme` — runs green over a fixture the *2020 writer produced*, bound in both
      directions to a verbatim copy of `master:pb/config.proto`, and it caught a hand-typed
      fixture that had the `rgb` format wrong. The "real" half was the human task
      `2026-07-30-real-legacy-config-migration`, and it came back **done, negative**
      (2026-08-06): *"I can't test, I don't have old config. Rely on tests."* No legacy
      `~/.kubecom.yaml` survives, so the file this criterion asks for does not exist to be
      run — which is the case the task's own "If no legacy file survives" clause anticipated:
      the closing leg ticks it on the fixture **and records that it did**. HT-dogfood-0806 is
      that leg and **D231** is that record. What stays unverified is unchanged and named
      there: an unparseable legacy config degrading *silently* to no migration (D92/D180 pt 4)
      is the one failure a generated file cannot exhibit, and no evidence for it exists.)
- [ ] Definition of Done in [`../goals.md`](../goals.md) fully checked.
      (Audited by M5-01 (2026-07-30, D174): **6 of 13 ticked**, each against named tests /
      decisions / milestone criteria, and each unticked box now names the one thing that
      closes it. **9 of 13 as of HT-dogfood-0806** (2026-08-06): the two `$EDITOR` boxes
      closed on the maintainer's *"editor is working"* and the migration box on D231's
      recorded fixture-not-real-file tick. **11 of 13 as of the 2026-08-09 review fold-in**
      (D256): the **context switcher** and **CRD/generic listing** boxes closed on the
      maintainer's own waivers — the switcher on "we'll ship it like this" (D256 pt 1), the
      CRD box on the declined live QA with the hermetic coverage named as the standing
      verification (D256 pt 3, the D231 shape). The remaining two are the **release boxes** —
      M5-10's tag and the issues it closes — both human acts (D173 pt 1). Nothing waits on
      agent work any more. Also
      found M5-01a: no surface reached previous-container logs, a 2020 parity gap — **closed
      2026-07-30** by `logs.previous`/`ctrl+p` (D177). **M5-01b ✅ 2026-07-30**: the audit's
      one open *question* — in-TUI YAML viewer vs D135 — is settled, and it needed no
      maintainer round-trip after all, because the maintainer had settled it in the 2026-07-24
      feedback that produced D135 (D178). The DoD bullet now states that YAML rides the
      editor, so every unticked box waits on evidence or on work, none on a decision. Ticked
      when the last box is.)

## Depends on
M0 CI/goreleaser scaffolding; feature milestones M1–M4 complete.
