# M2 — Core TUI (parity)

**Status:** `todo`
**Phase:** REWRITE_PLAN Phase 2

## Goal

The Bubble Tea application shell reaching interactive parity with the original:
two-pane browse, live table fed by the `kube` event channel, resource menu,
pickers, filter, and persisted config — all with zero shared mutable UI state.

## Scope

- Root `tea.Model` with view routing and global keybindings (`bubbles/key` + help).
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
- [ ] Old config migrates cleanly; malformed/legacy files handled gracefully.
- [ ] teatest coverage for update loop, menu reconcile, and modal flows.
- [ ] No mutex-guarded UI state; concurrency is message-driven only.

## Depends on
M1 event channel + discovery signal. Can start against a stubbed `kube` provider.
