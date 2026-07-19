# M2 — Core TUI (parity)

**Status:** `in-progress`
**Phase:** REWRITE_PLAN Phase 2

_Started 2026-07-19 with M2-01a (keymap core: `internal/tui/keymap`); M2-01b
adds multi-key sequences (`gg`→`nav.top`) + a stateful, model-timed `Sequencer`;
M2-01c wires the plain-YAML `keys:` config (`internal/config`) onto `Merge` and
adds `kubecom keys` to print the resolved map; M2-01d generates the help
(`bubbles/key.Binding`s + a toggleable overlay in `internal/tui/help`) from the
resolved keymap so it can't drift (D50); M2-01e generates the committed
`docs/keybindings.md` from the registry with a golden drift-check test +
`make keys-doc` (D51). The action registry + configurable keymap group
(M2-01a–e) is now **complete**. The app shell (root model, browse view, table, pickers, config
load/save + legacy migration) is the rest of M2 — it will route input through the
keymap/sequencer and embed the M2-01d help overlay. The app shell is now
decomposed into ordered, leg-sized slices **M2-02 … M2-14** on the
[board](../tasks/board.md) (D52). **M2-02** landed the `kube`-channels → Bubble
Tea `tea.Msg` boundary (`internal/tui/msg.go`: msg types + one-item `watchPump`/
`discoveryPump`, D53). **M2-03** landed the styles foundation (`internal/tui/styles`:
a named-color `Theme` → derived `Styles`, `DefaultTheme`/`Default`; lipgloss v2
promoted to a direct dep, D54). **M2-04** landed the first `components/*` package,
the status bar (`internal/tui/components/statusbar`: context · namespace ·
discovery spinner · short-help hint, rendered purely from props; spinner ticks
gated on discovering, D55); top-unblocked next is **M2-05a** (static seed
resource-menu sidebar)._

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

- [ ] Browse, select a resource, see live-updating rows for any discovered kind.
- [ ] Menu customization persists across restarts; async discovery reconciles menu without disturbing selection/scroll.
- [ ] Namespace + filter work; scrolling and Home/End behave.
- [ ] Vim keys and their fallbacks both navigate every list/table; help overlay shows both.
- [ ] Rebinding an action in config takes effect; invalid keymaps fail load with a clear error; no raw-key matching remains in view code.
- [ ] Old config migrates cleanly; malformed/legacy files handled gracefully.
- [ ] teatest coverage for update loop, menu reconcile, and modal flows.
- [ ] No mutex-guarded UI state; concurrency is message-driven only.

## Depends on
M1 event channel + discovery signal. Can start against a stubbed `kube` provider.
