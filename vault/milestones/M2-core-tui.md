# M2 — Core TUI (parity)

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 2

_M2-01 (keymap) + M2-02…M2-07 (shell) + M2-RUN + namespace picker (M2-08) + table filter with n/N search (M2-09) + confirm/prompt modal component (M2-10) + config write-back primitives (M2-11a) + per-context last-namespace persistence (M2-11b, restored on start with `-n` override, D91) have landed. Legacy-config migration has landed end-to-end (M2-12a parse+report primitive, D92; M2-12b launcher wiring, one-shot on `config.yaml` absence, D93). The table column-sort primitive has landed (M2-13a, `SortBy`/`ClearSort` as a stable, type-aware view over the row set, D94). Modal popups (help, namespace picker) now float over the base browse view instead of replacing it (FB-popups-overlay, `overlayCenter`/lipgloss layers, D95). The status bar moved to the top row and names the browsed resource type; the optional/popup-menu + command-palette navigation direction is recorded (D96) and queued as FB-nav-* tasks. Column sort is now wired to the app end-to-end: `sort.column` (`s`) cycles the sorted column/direction and back to unsorted, `sort.clear` (`S`) resets, with a `▲`/`▼` header indicator (M2-13b, D98). The left menu pane is now toggleable — `menu.toggle` (`m`) hides/shows it, a hidden menu going zero-width with the table taking the full width and focus (FB-nav-menu-toggle, D96 slice 1 / D99). A resource command palette (`resources.switch`, `:`) now switches the browsed kind pane-free — reusing the generic picker keyed by a distinct Kind, driving `selectResource`, so it works with the menu hidden (FB-nav-resource-palette, D96 slice 2 / D100). Remaining: the last FB-nav-* slice (popup menu, FB-nav-menu-popup) and the M3-gated teatest modal-confirm coverage (M2-14b). Per-leg history: `vault/journal/` and the [board](../tasks/board.md)._

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
      covered launch/render/graceful-degrade; live-cluster drill-in awaits a human
      dogfood pass.)_
- [ ] Browse, select a resource, see live-updating rows for any discovered kind.
- [ ] Menu customization persists across restarts; async discovery reconciles menu without disturbing selection/scroll.
- [x] Namespace + filter work; scrolling and Home/End behave. (ns picker M2-08c; table filter + n/N search M2-09b/D80; vertical+horizontal scroll and top/bottom in the table.)
- [ ] Vim keys and their fallbacks both navigate every list/table; help overlay shows both.
- [ ] Rebinding an action in config takes effect; invalid keymaps fail load with a clear error; no raw-key matching remains in view code.
- [x] Old config migrates cleanly; malformed/legacy files handled gracefully. (One-shot `~/.kubecom.yaml` migration wired into the launcher — M2-12a primitive + M2-12b launcher wiring; detect-and-report, one-shot on config absence, malformed/unreadable degrades to no migration and never blocks, D92/D93.)
- [~] teatest coverage for update loop, menu reconcile, and modal flows. (Update loop + menu reconcile — M2-14a; filter flow — M2-14c; error-toast/degrade-gracefully — M2-14d; all driven through the real program; modal flow deferred to M2-14b — the modal component landed (M2-10/D88) but needs an M3 action to wire it into the shell before the flow can be driven end-to-end.)
- [ ] No mutex-guarded UI state; concurrency is message-driven only.

## Depends on
M1 event channel + discovery signal. Can start against a stubbed `kube` provider.
