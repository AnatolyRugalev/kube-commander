# M2 — Core TUI (parity)

**Status:** `feature-complete` (2026-07-29) — every exit criterion is met and evidenced inline (M2-EXIT audit, D154); one small nav-parity enhancement (M2-15) stays on the board and gates nothing.
**Phase:** REWRITE_PLAN Phase 2

_M2-01 (keymap) + M2-02…M2-07 (shell) + M2-RUN + namespace picker (M2-08) + table filter with n/N search (M2-09) + confirm/prompt modal component (M2-10) + config write-back primitives (M2-11a) + per-context last-namespace persistence (M2-11b, restored on start with `-n` override, D91) have landed. Legacy-config migration has landed end-to-end (M2-12a parse+report primitive, D92; M2-12b launcher wiring, one-shot on `config.yaml` absence, D93). The table column-sort primitive has landed (M2-13a, `SortBy`/`ClearSort` as a stable, type-aware view over the row set, D94). Modal popups (help, namespace picker) now float over the base browse view instead of replacing it (FB-popups-overlay, `overlayCenter`/lipgloss layers, D95). The status bar moved to the top row and names the browsed resource type; the optional/popup-menu + command-palette navigation direction is recorded (D96) and queued as FB-nav-* tasks. Column sort is now wired to the app end-to-end: `sort.column` (`s`) cycles the sorted column/direction and back to unsorted, `sort.clear` (`S`) resets, with a `▲`/`▼` header indicator (M2-13b, D98). The left menu pane is now toggleable — `menu.toggle` (`m`) hides/shows it, a hidden menu going zero-width with the table taking the full width and focus (FB-nav-menu-toggle, D96 slice 1 / D99). A resource command palette (`resources.switch`, `:`) now switches the browsed kind pane-free — reusing the generic picker keyed by a distinct Kind, driving `selectResource`, so it works with the menu hidden (FB-nav-resource-palette, D96 slice 2 / D100). The D96 FB-nav-* line is complete: its third slice (FB-nav-menu-popup, a floating-menu overlay) was retired won't-do-separately — the toggle + palette already deliver the want, so it folds into the palette rather than adding a redundant surface (D101). The teatest modal-confirm coverage that was gated on an M3 action wiring the modal into the shell has landed (M2-14b, full-program accept/decline of the delete confirm, D116). Per-leg history: `vault/journal/` and the [board](../tasks/board.md)._

## Goal

The Bubble Tea application shell reaching interactive parity with the original:
two-pane browse, live table fed by the `kube` event channel, resource menu,
pickers, filter, and persisted config — all with zero shared mutable UI state.

## Scope

- **Action registry + configurable keymap** (foundational): named `Action` ids, a
  default keymap expressing the **vim-first** scheme (`hjkl`, `gg`/`G`, `/` +
  `n`/`N`, `Ctrl+u/d`, arrows/`PgUp`/`PgDn`/`Home`/`End`/`Enter`/`Esc` fallbacks),
  `merge(default, config.Keys)` with validation. Views resolve `KeyMsg → Action`;
  **no view matches a raw key.** `bubbles/key.Binding`s + help are generated from
  the registry. See [`../knowledge/keybindings.md`](../knowledge/keybindings.md).
- Root `tea.Model` with view routing driven by the resolved keymap.
- **Browse view**: resource-menu sidebar + live table pane.
- Table component: consumes `kube` add/modify/delete msgs; horizontal/vertical
  scroll, Home/End, selection. (Likely a **custom table** — see risks.)
- Menu: reconciles with async `DiscoveryReadyMsg`; add/remove/reorder items,
  persisted to YAML.
- Pickers via `bubbles/list`: namespace, and the scaffolding reused later for
  context/container/port.
- Filter via `bubbles/textinput`.
- Confirm/prompt modal (replaces the old racy popup).
- Lipgloss theme + status bar (context, namespace, spinner during discovery).
- **Config**: plain-YAML typed struct, load/save, and one-shot **migration** from
  the old `~/.kubecom.yaml` (protobuf-yaml) format on first start.

## Exit criteria

- [x] **Bare `kubecom` launches the browse UI against the user's current cluster**
      (M2-RUN, done 2026-07-20) and every leg after it keeps that runnable — a human
      periodically installs and dogfoods it against a real cluster, so each leg
      incrementally improves (never regresses) that experience (D68). _(Sandbox smoke
      covered launch/render/graceful-degrade; live-cluster browse + drill-in was
      confirmed incidentally by the M3-14b exec dogfood — 2026-07-24, a real cluster
      in a real terminal, which requires browsing to a pod and selecting its row.)_
- [x] Browse, select a resource, see live-updating rows for any discovered kind.
      (Menu drill-in → `selectResource` → `kube.Watch` through the `ResourceWatcher`
      seam, deltas streamed into the table via the M2-02 pump — M2-07c/D63,
      `TestSelectResourceStartsWatch`/`TestSelectResourceCancelsPrevious`/
      `TestWatchClosedStopsChain`. *Any discovered* kind: `DiscoveryReadyMsg` →
      `menu.Reconcile` appends kinds the seed omits (M2-07d/D64,
      `TestDiscoveryReadyReconcilesMenu` with a CRD), and `resources.switch` (`:`)
      reaches the same set pane-free (D100, `TestResourcePaletteSelectSwitchesResource`).
      End-to-end through the real program: `TestProgramFilterFlow` drills a resource
      in and renders the rows its watch delivers.)
- [x] Menu customization persists across restarts; async discovery reconciles menu without disturbing selection/scroll.
      (Customization is the per-context `menus/<context>.yaml` file — D83, and D89
      narrowed M2-11 to it: no resource list in `config.yaml`, no in-TUI menu editor.
      It loads at launch in `cmd/kubecom/run.go` (`config.MenuPath` →
      `config.LoadMenuFile`, missing file degrades to the default menu) and is folded
      into the menu by `TestMenuExtrasFoldedIn`, with `TestLoadMenuExtrasReadsFile`/
      `…MalformedFile`/`…MissingFile` on the launcher side — so the file a user edits
      is in effect on every start. Reconcile keeps the cursor on its *resource*, not
      its index — `TestReconcilePreservesSelection` (D57) — and `clampOffset` keeps
      the scroll offset valid as the item slice grows.)
- [x] Namespace + filter work; scrolling and Home/End behave. (ns picker M2-08c; table filter + n/N search M2-09b/D80; vertical+horizontal scroll and top/bottom in the table.)
- [x] Vim keys and their fallbacks both navigate every list/table; help overlay shows both.
      (`defaultBindings` pairs every nav action with a non-vim fallback — `k`/`up`,
      `j`/`down`, `h`/`left`, `l`/`right`, `gg`/`home`, `G`/`end`, `ctrl+d`/`pgdn`,
      `ctrl+u`/`pgup` — and resolution is central, so both families reach every list
      and table identically: `TestDefaultKeymapValid`, `TestDefaultResolution`. Help
      is generated from the resolved keymap and shows *all* of an action's keys
      (`Binding` joins `Keys()` — "j/down"): `TestBindingFromDefaults`,
      `TestBindingsCoversRegistry`, `TestHelpMapFullHelp`, overlay `TestHelpToggle`.
      The menu handles up/down/top/bottom but not the half-page/page actions — both
      key families are equally inert there, so parity holds; closing that gap is
      **M2-15**, an enhancement, not a criterion.)
- [x] Rebinding an action in config takes effect; invalid keymaps fail load with a clear error; no raw-key matching remains in view code.
      (`Config.Keymap()` = `DefaultKeymap().Merge(overrides)`, resolved in `runTUI`
      *before* the alt-screen — a bad keymap returns the error and never launches —
      and identically by `kubecom keys`: `TestKeymapResolvesOverride`,
      `TestKeymapUnknownActionErrors`, `TestKeymapBadTokenErrors`,
      `TestKeysOverrideAndWarning`, `TestKeysInvalidConfigFails`. Every raw key token
      in the tree lives under `internal/tui/keymap`; components take a
      `keymap.Action`, and the only `tea.KeyPressMsg` consumers are the text-entry
      surfaces (`UpdateQuery`/`UpdateFilter`/`UpdatePrompt`), where the key *is* the
      text, not a binding.)
- [x] Old config migrates cleanly; malformed/legacy files handled gracefully. (One-shot `~/.kubecom.yaml` migration wired into the launcher — M2-12a primitive + M2-12b launcher wiring; detect-and-report, one-shot on config absence, malformed/unreadable degrades to no migration and never blocks, D92/D93.)
- [x] teatest coverage for update loop, menu reconcile, and modal flows. (Update loop + menu reconcile — M2-14a; filter flow — M2-14c; error-toast/degrade-gracefully — M2-14d; modal confirm flow — M2-14b (accept runs the delete, decline does not; async accept synced on a side-effect signal, D116); all driven through the real program.)
- [x] No mutex-guarded UI state; concurrency is message-driven only.
      (No `sync.Mutex`/`sync.RWMutex`/`sync.Map` exists anywhere under `internal/tui`.
      The one `sync.` in the package is a `sync.Once` in `exec.go`'s `execSizeQueue`,
      guarding a channel close in the suspend plumbing — outside the `tea.Model`, not
      UI state. Producers reach the model only by sending messages through the M2-02
      pumps, and every model method takes and returns a value.)

## Depends on
M1 event channel + discovery signal. Can start against a stubbed `kube` provider.
