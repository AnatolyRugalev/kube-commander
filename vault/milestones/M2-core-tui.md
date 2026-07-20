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
gated on discovering, D55). **M2-05a** landed the second, the resource-menu
sidebar (`internal/tui/components/menu`: a static seed of core resource kinds,
navigated through keymap actions, emitting its own `menu.ResourceSelectedMsg` on
drill-in — the emitter owns the message type to avoid a component→`tui` import
cycle, D56). **M2-05b** added `(*Model).Reconcile(kube.DiscoveryResult)`, folding
the async discovery result into the seed — twins fill metadata, failed-group
entries go unavailable, CRDs/extra groups append — **without disturbing selection
or scroll**, degrading to the navigable seed on total/partial failure (D57).
**M2-06a** landed the third `components/*` package, the resource table
(`internal/tui/components/table`: renders a `kube.Table` snapshot — kubectl-identical
priority-0 columns — with vertical scroll, a highlighted selection, and rows
clipped-not-wrapped to the pane; drill-in emits its own `table.RowSelectedMsg`;
fixed the bordered-pane sizing gotcha D58). **M2-06b** added
`(*table.Model).ApplyEvent(kube.WatchEvent)`, folding live watch deltas onto that
snapshot **preserving the selection by object UID** — RESET replaces columns+rows
(selection preserved across reconnects), ADDED/MODIFIED upsert / DELETED removes a
row keyed by `ObjectRef.UID`, cursor keeping its index when the selected row is gone
(D59). **M2-06c** added **horizontal scroll** — a table wider than its pane scrolls
on `nav.left`/`nav.right` (`h`/`l`), one `hoffset` windowing header+rows in step and
snapping to column starts (`hclip` replaces `truncate`; no-wrap invariant D58 kept;
no keymap/doc change), completing the M2-06 table trio (D60). **M2-07a** landed the
root app shell's first slice (`internal/tui/app.go`, replacing the M0 `tui.go`
placeholder): the keymap-routed `tea.Model` skeleton — owns the resolved keymap + the
one `Sequencer`, routes every `KeyMsg` through it to an `Action` (no raw-key match,
D11), schedules the sequence-timeout tick with a generation guard that drops stale
ticks (D48), embeds the M2-01d help overlay (`app.help` toggles, `nav.back` closes),
and quits on `app.quit`; no panes yet (D61). **M2-07b** composed the two-pane **browse
layout**: the root model now owns the M2-05 menu (left pane), the M2-06 table (right
pane), and the M2-04 status bar (bottom line), sized on `WindowSizeMsg`; exactly one
pane holds focus (menu first), `nav.right`/`nav.left` switch focus between panes (the
table's horizontal scroll taking precedence until `HOffset()==0`, D60), non-switching
nav routes to the focused pane, and the open help overlay swallows navigation (D62).
Top-unblocked next is **M2-07c** (wire the live table to `kube.Watch` via the M2-02
pump: menu selection starts/stops the watch, `ResourceEventMsg`→`table.ApplyEvent`)._

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
