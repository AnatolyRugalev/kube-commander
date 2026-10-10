# Task Board

Live board for kubecom. See [`README.md`](README.md) for workflow and the item
template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-10-10 — KEYS-02 landed (keys resolve in the context path; `kubecom keys` lists, `default`, `check`); KEYS-03 next, pushed with KDL-01 and KEYS-02._

## In Progress

## Blocked

_(none)_

## Backlog

### Config & keys in KDL (D302/D303)

_plexos's model: an authored `config.kdl` kubecom never writes, an overlay it does, and a `keymap` of `bind`/`unbind`/`group`/`context "<cel>"` over a focus path. KEYS-01 is additive; KDL-01, KEYS-02 and KEYS-03 break `config.yaml` and land together._

- [ ] **KEYS-03** A bind's value runs the argument verb directly (`bind "g p" "resources.switch" "pods"`), with no palette stage
      status: todo | owner: — | added: 2026-10-10
      → knowledge: decisions.md D303 pt 2

- [ ] **KEYS-04** Keys that only mean something in one surface move into that surface's context, and the letters that frees are offered back (`p`, `s`, `a`, …) — a remap proposal for the maintainer, not a silent change
      status: in-progress | owner: kubecom | added: 2026-10-10
      → knowledge: decisions.md D303 pt 6

### UX, docs & polish

- [ ] **STORY-06i-3** Reverse-selector relations — a pod's Services (and the workloads whose selector matches it) folded into `kube.Relations`, so the popup answers "who routes to this pod" as well as "who made it"
      status: todo | owner: — | added: 2026-08-21
      notes: Third slice of STORY-06i, deliberately after the popup: unlike every 06i-1 edge this one cannot be read off the object — it needs a List of the namespace's Services and a client-side selector match, so it is a different cost class and gets its own leg. Bound it (namespace-scoped, one List) and let a failure degrade the relation away rather than fail the whole set.
      → knowledge: decisions.md D268

- [ ] **TAPE-01** Re-cut `docs/screencast.tape` around the main story
      status: todo | owner: — | added: 2026-08-15
      notes: Depends on STORY-04 (the main story) and lands after STORY-06 so the GIF shows
      the refined UX, not the one the stories found fault with. Today's tape is a **feature
      tour** — palette, filter, logs, describe, search, theme, help — assembled before any
      user path existed; the maintainer wants it re-cut so it walks the main story instead.
      The three guards in `internal/tui/keymap/screencast_test.go` (annotated keypresses
      checked against `DefaultKeymap`, headline actions pressed, README/GIF agreement) hold
      across the re-cut and the headline list may need revising with it. The tape also stops
      depending on an ad-hoc cluster once STORY-01 lands. **Re-recording is the maintainer's**
      (D181): it needs ttyd + ffmpeg, a real terminal and the fixture cluster.
      → knowledge: decisions.md D268, D181

- [ ] **DOC-03** README becomes a landing page
      status: todo | owner: — | added: 2026-08-15
      notes: Today's README is 212 lines and carries a full feature catalogue ("What kubecom
      can do", six subsections) that duplicates what `docs/` should own. Cut it to what a
      landing page owes a reader: what/why, the screencast, a short install block linking
      `docs/install.md` (DOC-01's shape, keep it), one screen of tour, and links out. The
      install-path drift guards (`TestReadmeBrewTapMatchesTheCask`,
      `TestReadmeAURPackageMatchesTheConfig`, `TestScreencastAssetAndReadmeAgree`) all read
      `README.md`, so the block they check must survive the cut.
      → knowledge: decisions.md D268

- [ ] **DOC-04** `docs/usage.md` — the guided tour the README no longer carries
      status: todo | owner: — | added: 2026-08-15
      notes: Depends on STORY-04. The prose walks the **main story**, so the doc, the tape
      and the story tell one story in three media (D268 pt 4). Absorbs the feature catalogue
      DOC-03 cuts, and links `docs/keybindings.md` for the full keymap rather than restating
      keys — a second hand-written key list is a second thing to drift (D51).
      → knowledge: decisions.md D268

- [ ] **DOC-05** `docs/troubleshooting.md`
      status: todo | owner: — | added: 2026-08-15
      notes: The failure surfaces exist and are documented nowhere a user looks: the auth
      diagnosis (`internal/tui/authdiag.go`), browse failures, the log file at
      `os.UserCacheDir()/kubecom/kubecom.log`, discovery partial-failure reporting (DISC-01),
      Gatekeeper quarantine on macOS, and the WSL2 note. Best written after STORY-05, which
      is the first time anyone hits these paths without knowing the code.
      → knowledge: decisions.md D268

- [ ] **BOX-04** The body's two-column dead gap — the menu's frame renders two columns narrower than the width `resize` sizes it to (its `View` hands `m.width-2` to a border-box frame), so the browse body is 78 columns wide on an 80-column terminal and the rightmost two are bare canvas
      status: todo | owner: — | added: 2026-08-20
      notes: Agent-found while landing D284 (whose pt 3 is the workaround: composite from `rightPaneX()`, the *rendered* menu width, not `menuPaneWidth`). The fix is to settle one border-box convention across the two panes — the table already renders exactly the width it is sized to — and then `rightPaneX()` may collapse back to `paneWidths()`. Guard it with a body-width assertion (`lipgloss.Width(browseBody()) == m.width`), the one property no current test states.
      → knowledge: decisions.md D284

### Release channels & launcher (ENGRAM — D292–D294)

_Published to engram channels (`pkg.neuroplast.io/kubecom/{dev,stable}`); a package installs `kubecom-launcher` as `kubecom`, and the complete binary self-updates with `kubecom update` (D292/D293). The code and pipeline landed 2026-10-06; what is left is packaging and docs._

- [ ] **ENGRAM-02** The Homebrew path installs the launcher — a cask/formula on `neuroplastio/homebrew-tap` that installs `kubecom-launcher` as `kubecom`, with the macOS quarantine postflight the old cask carried; the formula is published for `26.10.07`, and CI publishing needs `HOMEBREW_TAP_TOKEN`
      status: todo | owner: — | added: 2026-10-06
      notes: The GitHub release of a stable tag carries `kubecom-launcher_darwin_amd64`/`_arm64` and the seed `kubecom_darwin_<arch>`, so a hand-maintained formula can point at them. It must ship the seed too (D295) and set enlaunch's `Seed` to the Homebrew prefix, since `/usr/lib/kubecom` does not suit Homebrew on macOS. A package cannot update the launcher in place (it is root-owned); the copy the user runs lives in `~/.local/kubecom` and updates there. Until the token exists, `packaging/homebrew/publish.sh <tag> --push` updates the tap by hand.
      → knowledge: decisions.md D292, D293, D299

- [ ] **ENGRAM-03** Docs: `docs/install.md` and the README document the launcher and `kubecom update`; the delivery criteria and the goals Definition of Done that name goreleaser artifacts are reworded to the channel/launcher model, and `docs/usage.md` (DOC-04) covers updating
      status: todo | owner: — | added: 2026-10-06
      notes: D68 requires install/usage docs to match the binary in the same change; the launcher + `kubecom update` changed both. `kubecom-bin` is published on the AUR (26.10.06) and verified to launch and install the stable build; the "Homebrew/AUR install paths verified" criterion now means "installed the launcher from", and the AUR half is met (ENGRAM-01 landed 2026-10-06); the Homebrew half is ENGRAM-02.
      → knowledge: decisions.md D292
