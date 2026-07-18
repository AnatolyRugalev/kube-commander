# Decision Log

Append-only. Supersede rather than delete; note the date. Newest at the bottom.

---

### D1 — TUI framework: Bubble Tea
**2026-07-18.** Use **Bubble Tea + Bubbles + Lipgloss** (Elm architecture).
**Why:** the original's top defect class — data races, focus bugs, popup/redraw
issues — comes from driving `tcell/views`' imperative widget tree from
goroutines. Bubble Tea's single-threaded update loop removes that class by
construction. tview (what k9s uses) was considered but keeps the imperative
model. **Consequence:** no shared mutable UI state; concurrency via messages.

### D2 — In-process client-go, shell out only for interactivity
**2026-07-18.** Reach for client-go first; shell out **only** for exec shell and
`$EDITOR`. **Why:** removes the hard `kubectl` binary dependency (**#68**),
enables background port-forward, and in-TUI logs/describe/YAML. Modern client-go
covers logs/describe/portforward/remotecommand/actions. Push in-process "until we
hit a wall."

### D3 — Config: plain YAML
**2026-07-18.** Drop the protobuf config (`pb/config.proto` + generated code) for
a plain typed Go struct marshalled to YAML. **Why:** the protobuf machinery and
its abandoned theme engine were over-engineered for a small local file.

### D4 — Name: `kubecom`, single binary
**2026-07-18.** Standardize on `kubecom`; retire the duplicate `kube-commander`
binary. **Why:** the original shipped two identical entrypoints and inconsistent
naming.

### D5 — Kubernetes/client-go version
**2026-07-18.** Build against a recent client-go (**target v0.31 / K8s 1.31**);
support servers **~1.27+** via discovery so it degrades gracefully. "Recent, not
too aggressive."

### D6 — Backwards compatibility: clean break + migration
**2026-07-18.** No runtime BC with the old app; instead a one-shot **migration**
of the old `~/.kubecom.yaml` on first start. **Why:** clean architecture beats
carrying legacy config semantics.

### D7 — Platforms: Linux + macOS only (WSL2 for Windows)
**2026-07-18.** Drop native Windows support; recommend **WSL2**. **Why:** removes
Windows PTY/exec complexity (a flagged risk) for a niche of the user base.

### D8 — Async, cached resource discovery
**2026-07-18.** First-paint from a seed set of core GVKs; run full discovery in
the background and reconcile the menu via a "discovery ready" message; isolate
per-group failures; cache discovery on disk. **Why:** the original blocked the
whole UI on `ServerPreferredResources()`, hurting cold starts and turning one bad
API group into a total failure (**#87**, **#76**, **#86**).

### D9 — Branch model
**2026-07-18.** `master` = original code, untouched for now. `v1` = rewrite
branch, holds this vault and all rewrite work. A `main` branch becomes the final
destination when the rewrite is ready to be default. Push `v1` to origin.

### D10 — Vim-style navigation first-class; arrows/classic as fallback
**2026-07-18.** Navigation is **vim-first**: `hjkl`, `gg`/`G`, `Ctrl+u`/`Ctrl+d`,
`/` search + `n`/`N`, `l`/`Enter` to drill in, `h`/`Esc` to go back. Arrow keys,
`PgUp`/`PgDn`, `Home`/`End`, `Enter`/`Esc` work as an equivalent **fallback** so
non-vim users are never stranded. **Why:** the target user is a terminal-native
hacker; muscle-memory navigation is a core UX value, not an add-on.
**Consequence / rule:** `h j k l n` (and `g`, `G`) are the **default**
navigation keys — single-letter *action* defaults must not collide with them.
This supersedes the legacy pod bindings where they clash (legacy used `l` for
logs, `s` shell, `f` port-forward): default actions off the nav keys (e.g. behind
a leader or an actions menu). All of this is *default*, not fixed — see D11. See
[`keybindings.md`](keybindings.md).

### D11 — Fully configurable keybindings; zero hard-coded keys
**2026-07-18.** **No key literal is ever matched in view/update code.** Every
user-triggerable behavior is a named **Action**; an **action registry** maps
`Action → []key` and is the *only* place keys exist. The registry is built from a
**default keymap** (data in one place, expressing the vim-first scheme of D10)
overlaid by the user's config, then consulted by every view to resolve
`tea.KeyMsg → Action`. **Why:** users must be able to rebind anything, and
scattered `case 'l':`-style handling is exactly what made the old code rigid.
**Consequences:**
- Config carries a `keys:` section: `action -> [keys]` overrides merged onto defaults.
- `bubbles/key.Binding`s are constructed *from* the resolved keymap, not literals.
- Loading **validates**: unknown action names error; within-context collisions
  error with a clear message; the vim/nav defaults are just defaults a user may
  override (with a warning if they shadow navigation).
- The help overlay and the generated keybindings doc are **derived from the
  registry**, so they can never drift from actual bindings.
See [`keybindings.md`](keybindings.md).

### D12 — golangci-lint scoped to new code only
**2026-07-18.** `.golangci.yml` lints **only** the rewrite (`cmd/kubecom`,
`internal/...`); the legacy 2020 trees (`app/`, `cli/`, `commander/`, `config/`,
`pb/`, `cmd/kube-commander/`) are excluded. **Why:** those trees are deleted
tree-by-tree across M1–M3 — linting dead code is pure noise, and the maintainer
asked for lint to apply to new code only. **How:** golangci-lint **v2** with
`run.relative-path-mode: gomod` (normalizes match paths to module-relative so the
`^`-anchored excludes hit only the top-level legacy dirs — `^config/` excludes
legacy `config/` but not `internal/config/`) and `linters.default: standard`
(lenient ruleset per M0; tighten later). **Consequence:** remove an exclude entry
when its tree is deleted; new packages are linted by default.

### D13 — `kubecom` binary is the new skeleton; legacy reachable via `kube-commander`
**2026-07-18.** `cmd/kubecom` now builds the **new** binary (M0 skeleton:
`version`/`help`); the legacy 2020 app stays reachable **only** via
`cmd/kube-commander` until it is ported (M1–M3), after which M0-03 deletes it.
Both were identical `cli.Run()` entrypoints (the duplicate D4 flagged). The M0
skeleton uses a **stdlib** command dispatch for now; **cobra** replaces it in
M0-02 (kept out of this leg to avoid a premature go.mod/dependency bump).
**Why:** makes `kubecom` the single forward binary immediately while keeping the
old code compiling in parallel, per the M0 plan.

### D14 — Legacy trees are deleted from `v1` up-front, not kept compiling
**2026-07-18.** Maintainer-approved (setup review). Delete `app/`, `cli/`,
`commander/`, `config/`, `pb/`, and `cmd/kube-commander/` (plus Windows sources
and dead Travis/snap CI) from `v1` in an early M0 leg, pruning `go.mod`.
**Why:** the original "keep old trees compiling until ported" plan is unworkable
in a single Go module — the legacy code imports `k8s.io/*@v0.18` while the new
kube layer needs client-go v0.31, and one module cannot hold both. The clean
break (D6) means zero code reuse, and `master` preserves the old code forever.
**Consequence:** legacy behavior reference = `master` + `git show master:<path>`
+ [`legacy-architecture.md`](legacy-architecture.md). Supersedes the "compiling
in parallel" parts of D12/D13: `.golangci.yml` drops the path excludes once the
trees are gone (the `standard` ruleset stays), and M0-03 (remove duplicate
binary) is absorbed by the deletion leg.

### D15 — Journal format: one file per entry; no Commit field; milestones kept current
**2026-07-18.** Maintainer-approved. The journal is a directory,
`vault/journal/`, with one file per leg named `YYYY-MM-DD.N.md` (`N` = sequence
within the day; filename sort == chronological order). The `Commit:` field is
dropped — the entry is written before the commit exists; the **leg id in the
commit message** is the join key between journal, board, and git history. Every
leg also keeps the active **milestone file** current: tick exit criteria as they
are met and flip its `Status:` line.

### D16 — Board claims are committed and pushed immediately
**2026-07-18.** Maintainer-approved. Claiming a task (step 3 of the leg loop) is
its own commit (`chore(board): claim <leg-id>`) pushed to `v1` **before**
implementation starts. **Why:** a claim that lands only with the finished leg is
invisible to concurrent agents and locks nothing; pushing it first makes the
board a real mutex.

### D17 — `make check` is the canonical gate; CI lands early in M0
**2026-07-18.** Maintainer-approved. A `Makefile` with `check` (= build + test +
vet + lint) is the single verify gate used by agents and CI, so "green" means
the same thing everywhere. M0 is reordered: legacy deletion → toolchain bump →
**CI (M0-04)**, before any further feature legs — for an autonomous process
pushing straight to `v1`, CI is the only independent green check a reviewer has.

### D18 — Tests: fake clients by default; envtest opt-in, deferred to M1
**2026-07-18.** Maintainer-approved. Kube-layer tests use client-go **fake
clients** (incl. fake discovery) by default so `go test ./...` is hermetic and
runs anywhere. **envtest** integration tests are opt-in behind an env var
(`KUBECOM_TEST_ENVTEST=1`) because envtest downloads control-plane binaries —
fragile in sandboxed agent environments and costly in CI. The envtest harness
moves from M0-05 to M1 (where there is a kube layer to integration-test);
M0-05 keeps only the teatest smoke test.

### D19 — Bubble Tea v2
**2026-07-18.** Maintainer-approved. Prefer **bubbletea v2** (with matching
bubbles/lipgloss releases) when dependencies land (M0-02/M2). Pin v2 and write
all TUI code against its API; fall back to v1 only if v2 proves unusable in
practice, recorded as a superseding decision. **Why:** avoids building parity UI
on an API that is being replaced, and avoids mixing v1 examples with v2 code.

### D20 — Config path: `os.UserConfigDir()/kubecom/config.yaml`
**2026-07-18.** Maintainer-approved. The config lives at
`os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on
Linux, `~/Library/Application Support/kubecom/config.yaml` on macOS) — not
inside `~/.kube/`, which other tooling treats as kubeconfig-shaped. The one-shot
migration (D6) reads the legacy config from its old location once.

### D21 — Scheduled runs batch legs via fresh subagents (`/do-rewrite-run`)
**2026-07-18.** Maintainer-approved. The scheduled routine invokes
**`/do-rewrite-run`**, an orchestrator that sequentially spawns a **fresh
subagent per leg**, each executing exactly one `/do-rewrite-leg`. Budgets: max
4 legs per run; no new leg after 90 minutes (runs occupy the last ~2h of the
5-hour usage window); stop on any failure without retrying. **Why:** one leg
per invocation stays the rule (reviewability), while a routine run can use its
whole window; per-leg subagents keep context from accumulating across legs —
the alternative (`/loop` in one session) grows context unboundedly.
**Consequence:** legs must stay strictly sequential — claims + pushes to `v1`
would collide if parallelized.

### D22 — Executing D14: legacy trees gone; `.goreleaser.yml` patched to keep working, not redesigned
**2026-07-18.** M0-07 deleted `app/`, `cli/`, `commander/`, `config/`, `pb/`,
`cmd/kube-commander/`, `.travis.yml`, and the snap CI files (`ci/snap-deps.sh`,
`ci/snap.login.enc`); pruned `go.mod`/`go.sum` to empty via `go mod tidy` (no
external import remains until M0-02/M1 reintroduce cobra/client-go); and dropped
the now-unneeded `.golangci.yml` path excludes (D12's exclusion list has no
targets left). `.goreleaser.yml` referenced the deleted `cli.version` symbol and
the deleted `cmd/kube-commander` binary/snap craft — fixed the ldflags to target
`internal/version.Version` (the var already designed for this) and removed the
`kube-commander-linux` build id and the `snapcrafts:` block, since both only
existed to package the now-gone legacy binary. **Scope note:** the `kubecom-windows`
build target and the `aur`/`brews` publishers were left as-is — untangling the
release matrix into the Linux+macOS-only shape (D7) is M0-06's job, not this
leg's; this decision only covers unblocking what the deletion itself broke.
**Why not fold into M0-06 now:** keeping M0-07 to "delete + keep buildable" is a
smaller, safer diff than also redesigning the release config in the same leg.
