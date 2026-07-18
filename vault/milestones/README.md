# Milestones

Large units of the rewrite. Each maps to a phase in
[`../REWRITE_PLAN.md`](../REWRITE_PLAN.md) and carries its own scope, exit
criteria, and checklist. Work them roughly in order; M1 and M2 can overlap once
the `kube` layer has a stable event channel.

Status legend: `todo` · `in-progress` · `blocked` · `done`

| ID | Milestone | Status | Summary |
|----|-----------|--------|---------|
| [M0](M0-groundwork.md) | Groundwork | `todo` | Module layout, toolchain, CI, test harness, single `kubecom` binary |
| [M1](M1-kube-layer.md) | Kube layer | `todo` | In-process client-go: discovery (async+cached), Table watch, actions |
| [M2](M2-core-tui.md) | Core TUI (parity) | `todo` | Bubble Tea shell, two-pane browse, table/menu/pickers, config+migration |
| [M3](M3-actions-viewers.md) | Actions & viewers | `todo` | In-TUI logs/describe/YAML, workload actions, port-forward mgr, exec/edit |
| [M4](M4-capabilities.md) | New capabilities | `todo` | Context switcher, sort, drill-down, metrics, themes |
| [M5](M5-release.md) | Release & docs | `todo` | README/screencast, keybindings, distribution, migration notes |

## Exit criteria for the project

All of M0–M5 `done` and the [Definition of Done](../goals.md#definition-of-done-v1)
checklist complete, with the open-issue table in `../REWRITE_PLAN.md` fully
resolved or explicitly deferred.
