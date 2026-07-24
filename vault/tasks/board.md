# Task Board

Live board for the kubecom rewrite. See [`README.md`](README.md) for workflow and
the item template. Status: `todo` · `in-progress` · `blocked` · `done`.

_Last updated: 2026-07-24 — SEARCH-01: cluster-search kube primitive (`kube.Search`, one-shot concurrent fan-out over List, curated-scope default, per-kind isolation, capped/cancellable, D131) landed and `cluster-search-multi-resource` feedback triaged into SEARCH-01…04 + deleted. Feedback inbox now 4 items (still preempts the board): confirm-modal-yn-keys, delete-default-key-d, logs-dedicated-view-live-grep, unify-yaml-view-and-edit. Per-leg history: `vault/journal/`._

## In Progress

_(none)_

## Blocked

_(none)_

## Backlog

### M0 — Groundwork
_(none — M0 complete)_

### M1 — Kube layer
_M1 is feature-complete (D66); M1-04b was retired as obsolete (D81), so only the
deferred envtest item remains — not a blocker._
- [ ] **M1-INT** envtest integration tests (opt-in `KUBECOM_TEST_ENVTEST=1`): restricted-RBAC group isolation, watch reconnect/resync, action set against a live apiserver
      status: deferred | owner: — | added: 2026-07-20
      notes: D66 — fake-client coverage is the autonomous-loop bar; these need control-plane binaries (fragile in cloud, D18), so a human runs them locally or a dedicated CI job with `setup-envtest` does. Not an M1 blocker.

### M2 — Core TUI
M2-01 (action registry + configurable keymap, D10/D11) is **complete** (01a keymap
core / 01b sequences / 01c config wiring / 01d help overlay / 01e generated doc).
The rest of M2 is the **app shell** — expanded here into ordered, leg-sized slices
(D52). Take them top-down; each notes what it depends on. Package layout follows
[`../REWRITE_PLAN.md`](../REWRITE_PLAN.md): `internal/tui/{app.go,msg.go}`,
`internal/tui/styles`, `internal/tui/components/*`, `internal/tui/views/*`. Every
slice keeps **zero shared mutable UI state** (principle 1); goroutines only send
messages.

> **NEXT PICK — M2-RUN** must land before the remaining component legs: today the
> binary never launches the TUI or touches a cluster (no `tea.NewProgram`, no real
> client construction), so nothing has been exercised end-to-end against a real
> apiserver. Once it lands, every subsequent leg is verified against the running
> binary and must **incrementally improve the real-cluster experience** (D68).

- [x] **M2-07** Root app model / shell (`internal/tui/app.go`, replaces the M0
      `internal/tui/tui.go` placeholder)
      status: done (07a, 07b, 07c, 07d done) | owner: claude-opus | added: 2026-07-19 | done: 2026-07-20
      notes: Split — **M2-07a** ✅ done 2026-07-20 (D61): root `tea.Model` skeleton
      owning the resolved keymap + `Sequencer`, routing `KeyMsg`→`Action` (schedules
      `tea.Tick` on `ResultPending`, D48; gen-tagged tick drops stale timeouts),
      embedding the M2-01d help overlay (toggle `app.help`, `nav.back` closes it),
      window-size, and `app.quit` — no panes yet.
      **M2-07b** ✅ done 2026-07-20 (D62): composed the two-pane **browse** layout —
      the M2-05 menu (left) + M2-06 table (right) over the M2-04 status bar; menu
      starts focused; `nav.right`/`nav.left` switch focus (table's horizontal scroll
      takes precedence until `HOffset()==0`, D60); non-switching nav routes to the
      focused pane; the open help overlay swallows nav.
      **M2-07c** ✅ done 2026-07-20 (D63): drilling into a menu item starts a live
      `kube.Watch` (via a narrow `ResourceWatcher` seam injected with `WithWatcher`;
      nil → watch-inert), streaming deltas into the table through the M2-02 pump
      (`ResourceEventMsg`→`ApplyEvent`; first RESET repopulates); a new selection
      cancels the previous watch, a generation guard dropping its in-flight deltas;
      focus moves to the table. Top-unblocked next: **M2-07d**.
      **M2-07d** ✅ done 2026-07-20 (D64): kicks off async discovery on `Init`
      (via a `Discoverer` seam injected with `WithDiscoverer`, mirroring
      `WithWatcher`; nil → discovery-inert). `Init` defers the start one message
      hop (`startDiscoveryMsg`) since it can't mutate the value model; `Update`
      opens a cancellable pass, starts the M2-04 spinner, and batches its tick with
      the M2-02 `discoveryPump`. `DiscoveryReadyMsg` stops the spinner and calls
      `menu.Reconcile` (M2-05b) — no-op/seed on total failure (principle 3);
      `spinner.TickMsg` forwarded to the status bar; `app.quit` cancels the pass.
      **M2-07 (root shell) is complete.** Top-unblocked next: **M2-08**.

### M3 — Actions & Viewers
M3 makes kubecom *operate*: in-TUI viewers + the curated action set, killing nearly
all kubectl shell-outs. The **kube layer already has every verb** — logs stream
(M1-07c/d), describe (M1-07b), YAML (M1-07a), delete/scale/rollout-restart/cordon/
drain/suspend (M1-06*), background port-forward (M1-08). So M3 is almost entirely
the **TUI surface**: reusable read-only viewers, wiring actions through the M2-10
confirm modal (D88), an actions surface off the reserved nav keys (D10), and the two
sanctioned suspend flows (exec, edit). Built bottom-up (D52 rhythm): the shared
viewer + the actions surface first, then each viewer/action as its own leg. Every
overlay composites over the base browse view (D95); zero shared mutable UI state
(principle 1); no raw-key matching — actions are named keymap entries (D11). Ordering
is a default, not a contract — re-split any slice that proves > ~300 lines.

- [ ] **FB-pf-port-picker** Port-forward: offer a picker of the pod's declared ports (containerPorts / the resolved Service's ports) instead of free-text remote entry — pick a known target port
      status: todo | owner: — | added: 2026-07-24 (triaged from feedback 2026-07-24-port-forward-picker-and-local-port, part 1)
      notes: Needs a pod-ports introspection seam (containers' ports from the pod spec; a Service contributes its ports/targetPorts) + reuse the modal picker (like the logs container picker, M3-07a). Keymap-driven (D11), message-only (principle 1). Bigger than one leg — split when picked (picker over remote ports → then local-port field, FB-pf-local-port).
- [ ] **FB-pf-local-port** Port-forward: editable local port with one-keystroke "use a free port" (`:0` auto-assign) in the picker/prompt flow
      status: todo | owner: — | added: 2026-07-24 (triaged from feedback 2026-07-24-port-forward-picker-and-local-port, part 1/2)
      notes: Default local = remote, editable; a gesture that rewrites the spec to `:0` (OS-assigned free local port, already supported by kube.PortForward + reported via Ports()). Builds on FB-pf-port-picker; supersedes needing the D130 retry-hint for the common clash once shipped (the hint stays as the fallback). Depends on the picker UI.
- [ ] **M3-15b** Edit — TUI wire (the suspend flow): `res.edit`/`e` → `GetYAML` → temp
      file → `tea.ExecProcess` `$EDITOR` → read back → `Editor` seam (`kube.Update`, M3-15a).
      status: todo | owner: — | added: 2026-07-24 (split from M3-15)
      notes: The second sanctioned suspend action. no-change (edited bytes == original) and
      parse-error degrade without mutating; result to the status bar. Needs a real-terminal
      dogfood human-task like exec (D125). Depends on M3-15a.

### Cluster search (SEARCH — feedback-driven, D131)
Cross-object cluster search (feedback `2026-07-24-cluster-search-multi-resource`): type a
query → matching objects **across kinds** (Kind · namespace · name), drill into the hit.
An M4-class capability pulled forward by feedback. One-shot, concurrent, curated-scope by
default — never "watch everything" (D131). Built bottom-up (D52): kube primitive first
(done), then the TUI search mini-app, then streaming/progress, then scope-widening/fuzzy.

- [ ] **SEARCH-02** Search mini-app shell + `search.cluster` action: a registered key (D11)
      opens a full-screen search view (like the logs viewer / palette) — query textinput +
      results list (Kind · ns · name); `enter` drills into the selected hit (switch browse
      to that kind + select the row); `esc` closes. Message-only (principle 1).
      status: todo | owner: — | added: 2026-07-24 (SEARCH triage, D131)
      notes: Wire `kube.Search` (SEARCH-01) over `CommonSearchResources(discovered)` in the
      current namespace; run the fan-out off the update loop, stream hits in via a gen-tagged
      pump (like the log pump D53), cancel the previous query on a new one / on close. Reuse
      the picker/list rendering. This is the MVP first UI slice from the feedback.
- [ ] **SEARCH-03** Streaming results + progress + cap indicator: show hits as each kind
      returns (not only when all finish), a "searching N/M kinds…" progress line, the cap
      state, and cancel-in-flight on query change. Per-kind failures stay silent (D131 pt 3).
      status: todo | owner: — | added: 2026-07-24 (SEARCH triage, D131)
      notes: `kube.Search` already streams + caps + cancels; this is the TUI surfacing of
      progress/cap. Depends on SEARCH-02.
- [ ] **SEARCH-04** Scope widen + richer matching: opt-in **all discovered kinds** and/or
      **all namespaces** toggle (the expensive widen, off by default per D131 pt 2), plus
      fuzzy / label / field matching beyond name substring.
      status: todo | owner: — | added: 2026-07-24 (SEARCH triage, D131)
      notes: Keep the widen explicit + rate-limit-aware on big clusters. Depends on SEARCH-02.

_Remaining M4–M5 items to be expanded when those milestones open. See milestone files for scope._

## Done


- [x] **SEARCH-01** Feedback (normal, `2026-07-24-cluster-search-multi-resource`, first slice): cluster-search **kube primitive** (`internal/kube/search.go`) — `Clients.Search`/`searchRows` fan out one-shot **concurrent** server-side `List`s over a caller-supplied `[]Resource`, match `Row.Object.Name` by case-insensitive substring, stream `SearchHit{Resource,ObjectRef}` on a channel; per-kind failure isolates (principle 3), hit **cap** + ctx cancel bound it, `CommonSearchResources` gives the curated default scope; feedback triaged into SEARCH-02…04 and deleted — done 2026-07-24 (D131)
- [x] **FB-pf-bind-toast** Feedback (high, `2026-07-24-port-forward-picker-and-local-port`, first slice): a port-forward local-listener bind failure now surfaces an actionable status-bar hint (naming the clashing local port(s) + `:0`/`:<remote>` free-local-port retry) instead of client-go's raw "unable to listen on any of the requested ports"; prompt hint surfaces the `:80=free local` syntax; parts 1/2 (port picker, editable/auto local port) triaged to FB-pf-port-picker/FB-pf-local-port — done 2026-07-24 (D130)
- [x] **HT-exec-dogfood** Closed the exec live-cluster dogfood human-task — maintainer confirmed the Exec-shell action works end-to-end against a real cluster in a real terminal (shell drops in, TUI restores cleanly); ticked the M3 exec exit criterion, deleted `vault/human-tasks/2026-07-24-exec-live-cluster-dogfood.md` — done 2026-07-24
- [x] **M3-15a** Edit — kube-layer apply/update primitive (`internal/kube/apply.go`): `Clients.Update` parses the edited `$EDITOR` bytes (YAML→JSON→unstructured, int64-safe) and PUT-updates the object through the generic dynamic client (built-ins + CRDs, no kubectl, D2); the buffer's `metadata.resourceVersion` gives optimistic concurrency (concurrent change → Conflict, not clobber), identity (name/namespace) guarded before any request — rename/empty/invalid/null rejected without mutation; no-change left to the caller (M3-15b) — done 2026-07-24 (D129)
- [x] **M3-14b-4** Exec — kubectl parity fallback: when `kubectl` is on PATH `execInto` suspends into `kubectl exec -i -t <pod> [-c ctr] -- /bin/sh` via `tea.ExecProcess` (pointed at the same cluster via `--kubeconfig`/`--context`/`-n`, new `WithKubeconfig` option wired in `run.go`), else the in-process SPDY path (14b-1) — kubectl owns its own raw PTY/resize/edge-cases when present, SPDY keeps exec working with no kubectl (#68/D2); `lookupKubectl` seam (overridable in tests) + pure `kubectlExecArgs` builder, hermetically tested — done 2026-07-24 (D128)
- [x] **M3-14b-3** Exec — live terminal resize: a `syscall.SIGWINCH` watcher (`watchResize`, real-terminal path) reads `term.GetSize` (injected `sizeOf`) and pushes it into the exec size queue, now a latest-wins one-slot channel (`push` supersedes an unread stale size, drops 0×0), so the remote PTY tracks the local window mid-session; watcher stopped (`signal.Stop` + wait for the goroutine) before `close`, so no push races the closed channel; hermetic test raises SIGWINCH in-process — done 2026-07-24 (D127)
- [x] **M3-14b-2** Exec — multi-container picker reuse: `openExec` routes through the shared M3-07a container-resolution path tagged with a new `ctrPurpose` (logs↔exec); single-container Pod execs directly, multi-container prompts via the reused `ctrPicker`, `streamOrExec` routes to `streamLogsInto`/`execInto`; `newExecCommand` takes the chosen container — done 2026-07-24 (D126)
- [x] **M3-14b-1** Exec — TUI wire (in-process SPDY primary path): `tea.Exec`→`execCommand.Run()` drives blocking `kube.Exec` off the loop, local raw terminal (x/term) + seed-once size queue, Pod default container `/bin/sh`, result to status bar; `Execer` seam; live exec dogfood raised as a human-task — done 2026-07-24 (D125)
- [x] **M3-14a** Exec — kube-layer exec primitive (`internal/kube/exec.go`): blocking `Clients.Exec` over the pod `exec` subresource via `remotecommand.NewSPDYExecutor` (SPDY, no kubectl binary, D2); apimachinery-free `ExecOptions`/`TerminalSize`/`TerminalSizeQueue` surface with a `sizeQueueAdapter` (D33); TTY folds stderr into stdout + wires the size queue; injectable executor factory, hermetic fake tests (D18) — done 2026-07-24 (D124)
- [x] **M3-13c** Port-forward for Services: a Service can't be forwarded directly (kube.PortForward posts to the pod subresource), so the Port-forward action — re-extended to apply to `Service` as well as `Pod` — resolves it to a backing endpoint pod first via a new `ServiceResolver` seam (`WithServiceResolver`; `kube.PodForService`: `spec.selector` → newest ready pod via `newestReadyPod`, selector-less/no-pods → toast), then opens the ports prompt over — and forwards — the resolved pod; resolve-then-prompt runs off the update loop, generation-guarded (`pfResolveGen`), no resolver → toast (a Pod still forwards directly) — done 2026-07-24 (D123)
- [x] **M3-13b** Port-forward panel: `forwards.panel` (`F`) toggles an app-global overlay listing active forwards (label · bound/requested ports · ready state) with a cursor; `nav.drillIn` stops the selected forward (context cancel → the M3-13a Done flow removes it + notices), `forwards.stopAll` (`X`) stops all via `stopForwards` + a sweep notice; inline overlay state + `forwardsPanelView` composited via `overlayCenter` (D95), captured like help/viewer; ticks the M3 port-forward exit criterion — done 2026-07-24
- [x] **M3-13a** Port-forward start + background lifecycle: `PortForwarder`/`ActiveForward` seam (`WithPortForwarder`, `PortForwarderFunc` launcher adapter) → M1-08 `kube.PortForward` on a Pod row behind a ports prompt (D117 stash); lifecycle via two-edge `waitForward`/`waitForwardDone` messages (not a pump), bound ports to the status bar, forwards tracked in the model, all cancelled on quit (`stopForwards`); Pod-only (Service → M3-13c, panel → M3-13b) — done 2026-07-23 (D122)
- [x] **M3-12** CronJob suspend/resume wired: `Suspender` seam (both verbs, `WithSuspender`) → `kube.Suspend`/`Resume` on CronJob rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **M3-11b** Drain wired: `Drainer` seam (`WithDrainer`) → `kube.DrainStream` (channel twin of `Drain`) on Node rows behind the D115 confirm modal; `drainPump` (mirrors the log pump, `drainGen`-tagged) streams cordon→evict→remove progress to the status bar, terminal error toast / clean-close success notice, `stopDrain` cancel-on-quit; default `{IgnoreDaemonSets:true}` (Force/DeleteEmptyDirData off) — done 2026-07-23 (D121)
- [x] **M3-11a** Cordon/uncordon wired: `Cordoner` seam (both verbs, `WithCordoner`) → `kube.Cordon`/`Uncordon` on Node rows, dispatched **directly** (idempotent — no confirm modal, no target stash, D120), result to the status bar (neutral notice / error toast) — done 2026-07-23 (D120)
- [x] **FB-gray-out-empty-types** Feedback (low/soft, `2026-07-23-gray-out-empty-resource-types`): triaged the "gray out empty left-menu resource types" idea — **dismissed** (D119): the eager per-type-count version is forbidden (fights lazy-list D8/principle 4), the cheap opportunistic variant declined for now (marginal revisit-only value vs a namespace-keyed cache + hot-path plumbing + a third menu visual state needing a real-terminal UX check) — done 2026-07-23 (D119)
- [x] **FB-k9s-not-prior-art** Feedback (normal, `2026-07-23-k9s-not-prior-art`): reworded the README "Special thanks" k9s line from "prior art in the Kubernetes-TUI space" to "a contemporary Kubernetes TUI in the same space" (k9s is a ~2019-2020 peer, not a predecessor); split M3-11 → M3-11a/M3-11b — done 2026-07-23 (D118)
- [x] **M3-10** Scale (prompt → `kube.Scale`) + rollout-restart (confirm → `kube.RolloutRestart`) wired through the D115 modal; new `Scaler`/`RolloutRestarter` seams, prompt-mode key routing (`routeModalPromptKey`), shared `mutateRes`/`mutateRef` stash, results to the status bar — done 2026-07-23 (D117)
- [x] **M2-14b** teatest coverage: modal confirm flow — full-program (teatest/v2) delete confirm: `x`→open→`enter` accept (delete runs) / `esc` decline (no delete), async accept synced on a side-effect signal not Quit-ordering — done 2026-07-23 (D116)
- [x] **M3-09** Delete action wired through the confirm modal (`res.delete`/`x` → `modal.ShowConfirm` on the selected row → accept `nav.drillIn` runs `kube.Delete` (row's UID guards the snapshot race), result to the status bar (error toast / neutral notice); decline `nav.back`/quit closes it, no raw y/n; `Deleter` seam + `WithDeleter`) — **unblocks M2-14b** — done 2026-07-23 (D115)
- [x] **M3-08b** Secret viewer — copy the selected value to the clipboard (#89): per-entry cursor (`nav.up`/`nav.down` select, not scroll; `> ` gutter marks it, `EnsureLineVisible` keeps it on screen), `secret.copy` (`c`) yanks the selected decoded value via `tea.SetClipboard` OSC-52 (masked or revealed), neutral status-bar notice `copied "key" (N bytes)` (new `SetNotice` channel) — done 2026-07-23 (D114)
- [x] **M3-08a** Secret viewer — reveal/base64-decode (#89): `SecretGetter` seam (`kube.SecretData`, typed clientset → decoded + key-sorted entries) → shared M3-01 viewer; values masked on open (`key: •••• (N bytes)`), the registered `secret.reveal` (`r`) gesture toggles reveal (viewer-only, re-renders the same fetched data), `WithSecretGetter` gates it; copy split to M3-08b — done 2026-07-23 (D113)
- [x] **M3-07b** Logs — pod-owning kinds (#84): `PodResolver` seam (`kube.PodForOwner`) resolves a Deployment/RS/StatefulSet/DaemonSet/Job/RC to a backing pod (dynamic Get → `spec.selector` → newest Ready pod, fallback newest); `openLogsViewer` resolves off the update loop then feeds the pod into the shared `resolveContainersFor` (M3-07a container path), titled as a Pod; `WithPodResolver` gates it (no resolver → the M3-05…07a not-yet-available toast) — done 2026-07-23 (D112)
- [x] **M3-07a** Logs container picker for multi-container pods: `ContainerLister` seam (`kube.PodContainers`) resolves a pod's containers before streaming — multiple open the reused modal picker (`ctrPicker`) and the pick streams the chosen container, a single container streams directly; no lister → default container (no picker); streaming factored into `streamLogsInto`, container named in the title — done 2026-07-23 (D111)
- [x] **M3-06** Logs viewer — follow + reconnect: opens tailing (`LogOptions{Follow:true}`, M1-07d) with auto-scroll; `logs.follow`/`f` toggles it (logs-viewer-only, per-open `viewer.SetKind`), a manual up-scroll pauses it, title marks `[following]`/`[paused]` — done 2026-07-23 (D110)
- [x] **M3-05** Logs viewer wired (initial, no follow): `res.logs`/`L` → `LogStreamer` seam (`kube.Logs`, M1-07c) streamed line-by-line into the shared M3-01 viewer via a gen-tagged pump (D53); pods first, `stopLogStream` teardown, open-failure closes/mid-stream error keeps lines — done 2026-07-23 (D109)
- [x] **M3-04** Describe viewer wired: `res.describe`/`d` → `Describer` seam (`kube.Describe`, M1-07b) → shared M3-01 viewer overlay; async render + gen-guard (shared viewerGen), error degrades to a toast — done 2026-07-23 (D108)
- [x] **M3-03** YAML viewer wired: `res.yaml`/`y` → `YAMLGetter` seam (`kube.GetYAML`) → shared M3-01 viewer overlay; async fetch + gen-guard, error degrades to a toast — done 2026-07-23 (D108)
- [x] **M3-02** Action surface + M3 keymap: actions menu (Kind `"action"` picker) + direct keys (`a d y L e x`) → typed `rowActionMsg` intent — done 2026-07-23 (D107)
- [x] **M3-01** Read-only viewer/pager component (`internal/tui/components/viewer`): keymap-routed scrollable text overlay, bare box, `ClosedMsg` — done 2026-07-22 (D106)
- [x] **M3-PLAN** Expand the M3 milestone (actions & viewers) into ordered, leg-sized Backlog slices M3-01…M3-15 — done 2026-07-22 (D105)
- [x] **FB-watch-unsupported-list-only** Feedback (normal, `2026-07-22-watch-unsupported-resource-list-only`): kinds without the `watch` verb (e.g. componentstatuses) blanked/retry-looped — watch degrades to list-only polling — done 2026-07-22 (D104)
- [x] **FB-crd-parametercodec** Feedback (high, `2026-07-22-crd-list-watch-parametercodec`): non-built-in CRD groups failed list/watch — encode params with `metav1.ParameterCodec` — done 2026-07-22 (D103)
- [x] **FB-nav-menu-popup** Left menu as an overlay popup — **retired won't-do-separately**, folded into the resource palette (D96 slice 3, resolving the… — done 2026-07-22 (D81, D96, D99, D100, D101)
- [x] **FB-nav-resource-palette** Command-palette resource switch (k9s `:`-style), D96 slice 2 / D100 — done 2026-07-22 (D11, D65, D95, D96, D99, D100)
- [x] **FB-nav-menu-toggle** Make the left menu pane optional (D96 slice 1) — done 2026-07-22 (D11, D96, D99)
- [x] **M2-13b** Column sort — keymap actions + app wiring (#85) — done 2026-07-22 (D11, D94, D98)
- [x] **FB-mouse-optin** Feedback (normal, `2026-07-22-text-selection-select-to-copy`): the app captured the mouse on every frame (`View` set… — done 2026-07-22 (D11, D86, D97)
- [x] **FB-status-bar-top** Feedback (normal, `2026-07-22-status-bar-top-and-optional-left-panel`): first concrete slice + direction triage — done 2026-07-22 (D11, D96)
- [x] **FB-left-pane-width-smaller** Feedback (normal, `2026-07-22-left-pane-width-smaller`): the left menu pane's `total/4` default ate room the table needs on wide… — done 2026-07-22 (D84)
- [x] **FB-popups-overlay** Feedback (high, `2026-07-22-popups-should-overlay`): the help overlay and namespace picker **replaced** the browse body… — done 2026-07-22 (D88, D95)
- [x] **M2-13a** Table column-sort primitive (`internal/tui/components/table`, component-only) — done 2026-07-22 (D94)
- [x] **M2-12b** Legacy config migration — launcher wiring (`cmd/kubecom/run.go` `maybeMigrate`) — done 2026-07-22 (D93)
- [x] **M2-12a** Legacy config migration — parse + report primitive (`internal/config/migrate.go`): `LegacyPath()` (`~/.kubecom.yaml`) +… — done 2026-07-22 (D92)
- [x] **M2-11b-2** Config: last-namespace load-on-start + persist wiring (`internal/tui`, `cmd/kubecom`) — done 2026-07-22 (D91)
- [x] **M2-11b-1** Config: per-context state store (`internal/config/state.go`): `State{LastNamespace}` + `StateDir`/`StatePath(context)` (reuses… — done 2026-07-22 (D90)
- [x] **M2-11a** Config write-back primitives (`internal/config`): `Config.Save(io.Writer)` marshals via `sigs.k8s.io/yaml` (write-back… — done 2026-07-22 (D83, D89)
- [x] **M2-10** Confirm/prompt modal (`internal/tui/components/modal`): a Kind-stamped, centered/bordered overlay replacing the original's racy… — done 2026-07-22 (D11, D56, D65, D88)
- [x] **FB-hintbar-dedicated** Board (deferred remainder of feedback `2026-07-21-06`/D85): promoted the persistent, focus-aware key hint off the status bar onto… — done 2026-07-21 (D85, D87)
- [x] **FB-menu-config-03** Third/final slice of feedback `2026-07-21-02` (D83): **app wiring** for the per-context menu — done 2026-07-21 (D83)
- [x] **FB-menu-config-02** Second slice of feedback `2026-07-21-02` (D83): per-context menu **merge** — done 2026-07-21 (D57, D77, D83)
- [x] **FB-mouse-support** Feedback (normal, `2026-07-21-08`): additive Bubble Tea mouse support (keyboard/vim stays primary) — done 2026-07-21 (D11, D70, D86)
- [x] **FB-help-popup** Feedback (normal, `2026-07-21-07`): the `?` help/keys view was a full-screen replacement of the whole TUI; it is now a… — done 2026-07-21
- [x] **FB-hint-focus** Feedback (normal, `2026-07-21-06`): the persistent bottom key-hint (status bar) is now **focus-aware** — it shows the keys… — done 2026-07-21 (D11, D85)
- [x] **FB-menu-item-states** Feedback (normal, `2026-07-21-05`): the left menu now shows two independent states so it's always clear both which resource is… — done 2026-07-21
- [x] **FB-ns-seam-followup** Feedback (high, `2026-07-21-09`): three namespace-seam dogfood fixes, the third a functional dead-end — done 2026-07-21
- [x] **FB-esc-back-to-menu** Feedback (normal, `2026-07-21-04`): esc is now the one-level-back gesture — done 2026-07-21
- [x] **FB-menu-scroll** Feedback (high, `2026-07-21-03`): left menu is now a real viewport — done 2026-07-21 (D84)
- [x] **FB-menu-config-01** Feedback (high, `2026-07-21-02`): per-context dynamic menu config — **first slice** (triaged the rest into FB-menu-config-02/03) — done 2026-07-21 (D83)
- [x] **FB-ns-menu-seam** Feedback (normal, `2026-07-21-01`): namespace picker surfaced as a row in the left menu, marking the cluster-scoped ↔ namespaced… — done 2026-07-21 (D77, D82)
- [x] **M2-14d** teatest coverage: error-toast path (D74/FB-errors-layout) — `TestProgramErrorToastDegradesGracefully` delivers a live `ErrorMsg`… — done 2026-07-21 (D74)
- [x] **M2-14c** teatest coverage: filter flow (M2-09b) driven end-to-end through the real bubbletea program — `/` → type → enter-commit via live… — done 2026-07-21
- [x] **M1-04b** Lazy group-detail-on-open: **retired won't-do** — doesn't fit the realized flat Dashboard-sectioned menu (no group-open… — done 2026-07-20 (D77, D81)
- [x] **M2-14a** teatest coverage (update loop + menu reconcile): split from M2-14 — the modal-flow half is deferred to M2-14b (blocked by the… — done 2026-07-20 (D57, D73, D74, D78, D80)
- [x] **M2-09a** Table filter core: authoritative unfiltered `full` row set + a displayed filtered `table` view;… — done 2026-07-20 (D78)
- [x] **FB-menu-nesting** Feedback (normal): flat left menu read as disorganized → resource menu now renders Dashboard-style sections (Cluster / Workloads… — done 2026-07-20 (D57, D77)
- [x] **FB-welcome-page** Feedback (normal): bare launch showed an empty right-pane table → new `welcome` component (`internal/tui/components/welcome`)… — done 2026-07-20 (D76)
- [x] **FB-go-install** Feedback (normal): README `go install …/cmd/kubecom@v1` failed (`@v1` is a semver version query — resolves to a nonexistent… — done 2026-07-20 (D75)
- [x] **FB-errors-layout** Feedback (high): errors broke the TUI layout → transient single-line status-bar toast; root `Update` now handles `ErrorMsg` (was… — done 2026-07-20 (D74)
- [x] **M2-08c** Wire namespace picker into the app shell: `ns.switch`/`ctrl+n` action + `NamespaceLister` seam (`WithNamespaceLister`, nil →… — done 2026-07-20 (D73)
- [x] **M2-08** Namespace picker (08a component / 08b filtering / 08c app wiring) — done 2026-07-20 (D65, D72, D73)
- [x] **M2-RUN** Bare `kubecom` launches the browse UI against a real cluster (root `RunE` + kubeconfig/context/`-n` flags → live `*kube.Clients`… — done 2026-07-20 (D68, D70, D71)
- [x] **M2-08b** Picker filtering: picker-owned textinput, case-insensitive substring narrowing, control/text key split, back clears-then-cancels — done 2026-07-20 (D11, D65, D72)
- [x] **M2-08a** Generic modal picker component — done 2026-07-20 (D11, D45, D56, D65)
- [x] **M2-07d** Root app shell: async discovery on Init → menu reconcile + status-bar spinner — done 2026-07-20 (D8, D18, D57, D63, D64)
- [x] **M2-07c** Root app shell: live table wired to `kube.Watch` — done 2026-07-20 (D18, D60, D61, D62, D63)
- [x] **M2-07b** Root app shell: two-pane browse layout with focus switching — done 2026-07-20 (D60, D62)
- [x] **M2-07a** Root app model / shell: keymap-routed skeleton — done 2026-07-20 (D11, D48, D61)
- [x] **M2-06c** Table component: horizontal scroll — done 2026-07-20 (D58, D60)
- [x] **M2-06b** Table component: live watch deltas — done 2026-07-20 (D34, D59)
- [x] **M2-06a** Table component: snapshot render — done 2026-07-20 (D11, D54, D56, D58)
- [x] **M2-05b** Resource-menu sidebar: discovery reconcile — done 2026-07-19 (D57)
- [x] **M2-05a** Static seed resource-menu sidebar — done 2026-07-19 (D11, D54, D56)
- [x] **M2-04** Status bar component — done 2026-07-19 (D11, D54, D55)
- [x] **M2-03** Lipgloss theme + style set — done 2026-07-19 (D6, D50, D54)
- [x] **M2-02** TUI message types + channel→msg pumps — done 2026-07-19 (D53)
- [x] **M2-PLAN** Expand the M2 app-shell into ordered, leg-sized Backlog slices (M2-02 … M2-14) — done 2026-07-19 (D52)
- [x] **M2-01e** Generated keybindings doc from the registry + drift check — done 2026-07-19 (D11, D50, D51)
- [x] **M2-01d** Help generated from the keymap registry — done 2026-07-19 (D11, D26, D50)
- [x] **M2-01c** Config `keys:` wiring — done 2026-07-19 (D20, D49)
- [x] **M2-01b** Multi-key sequences + timeout resolution — done 2026-07-19 (D10, D47, D48)
- [x] **M2-01a** Keymap core — done 2026-07-19 (D10, D11, D47)
- [x] **M1-09** Typed graceful errors — done 2026-07-19 (D46)
- [x] **M1-08** Background port-forward — done 2026-07-19 (D2, D33, D39, D43, D45)
- [x] **M1-07d** Reconnecting/resuming follow logs — done 2026-07-19 (D34, D40, D44)
- [x] **M1-07c** Streaming pod logs — done 2026-07-19 (D2, D39, D43)
- [x] **M1-07b** Describe — done 2026-07-19 (D2, D42)
- [x] **M1-07a** Get object as **YAML** — done 2026-07-19 (D2, D41)
- [x] **M1-06e-2** Drain: **eviction loop** — done 2026-07-19 (D35, D40)
- [x] **M1-06e-1** Drain: pod **selection** — done 2026-07-19 (D33, D39)
- [x] **M1-06d** Actions: **cronjob suspend/resume** — done 2026-07-19 (D37, D38)
- [x] **M1-06c** Actions: **cordon/uncordon** — done 2026-07-18 (D37)
- [x] **M1-06b** Actions: **scale** + **rollout-restart** — done 2026-07-18 (D36)
- [x] **M1-06a** Action: generic **delete** — done 2026-07-18 (D33, D35)
- [x] **M1-05b** Server-side Table **Watch** → event channel: `internal/kube/watch.go` — done 2026-07-18 (D34)
- [x] **M1-05a** Server-side Table **List** → typed `Table{Columns,Rows}`: `internal/kube/table.go` — done 2026-07-18 (D33)
- [x] **M1-04** On-disk discovery cache + invalidation: `internal/kube/cache.go` + rewired `NewClients` — done 2026-07-18 (D20, D32)
- [x] **M1-03** Async full discovery → reconcile signal; per-group fault isolation: `internal/kube/discovery.go` — done 2026-07-18 (D31)
- [x] **M1-02** Seed-set core GVKs with static REST mapping for instant start: `internal/kube/seed.go` — done 2026-07-18 (D8, D29, D30)
- [x] **M1-01** Client bootstrap: `internal/kube/client.go` — done 2026-07-18 (D8, D18, D29)
- [x] **M1-00** envtest harness: `internal/kube/envtest_test.go` — done 2026-07-18 (D28)
- [x] **M0-06** goreleaser skeleton (Linux+macOS): `.goreleaser.yml` reshaped to v2 syntax, single build × `goos:[linux,darwin]` ×… — done 2026-07-18 (D2, D7, D27)
- [x] **M0-09** README references the vault + decision log: rewrite-in-progress banner atop `README.md` linking `vault/`, goals, milestones,… — done 2026-07-18
- [x] **M0-05** Test harness: teatest (tui) smoke test — done 2026-07-18 (D26)
- [x] **M0-08** Sweep remaining legacy files M0-07 missed: `Dockerfile`, `get.sh`, `ci/aur/` (incl — done 2026-07-18 (D25)
- [x] **M0-04** GitHub Actions CI: `make check` (build/test/vet/golangci-lint) on Linux+macOS matrix (D24) — done 2026-07-18 (D24)
- [x] **M0-02** Toolchain: bump to Go 1.23 (go.mod directive), wire `cmd/kubecom` onto cobra v1.10.2; `ioutil` already gone via M0-07 (D23) — done 2026-07-18 (D23)
- [x] **PROC-04** Cloud runs set repo-local git identity (maintainer) in routine bootstrap + skill step 0 — done 2026-07-18
- [x] **PROC-03** Model split: routine session on Sonnet (orchestration), leg subagents pinned to Opus in `/do-rewrite-run`; cron corrected to UTC — done 2026-07-18
- [x] **M0-07** Delete legacy trees from `v1`: `app/`, `cli/`, `commander/`, `config/`, `pb/`, `cmd/kube-commander/`, Windows sources,… — done 2026-07-18 (D14, D22)
- [x] **PROC-02** Scheduled runs self-prime: checkout `v1` + lint tooling in step 0; legs read the skill by file path (cloud clones start on… — done 2026-07-18
- [x] **PROC-01** `/do-rewrite-run` orchestrator skill: fresh subagent per leg, sequential, 4-leg/90-min budgets — done 2026-07-18 (D21)
- [x] **REVIEW-01** Maintainer setup review applied: legacy deletion planned, journal split to per-entry files, claim-push, `make check` + CI-early,… — done 2026-07-18 (D14, D20)
- [x] **M0-03** Single `kubecom` binary — done 2026-07-18 (D14)
- [x] **M0-01** Scaffold new module layout — done 2026-07-18 (D12, D13)
- [x] **BOOT-01** Bootstrap vault, goals, milestones, task board on `v1` — done 2026-07-18
