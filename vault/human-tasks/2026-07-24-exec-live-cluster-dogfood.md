# Dogfood the exec shell against a real cluster in a real terminal

- Created: 2026-07-24
- By: M3-14b-1
- Priority: normal
- Blocks: none (advisory)
- Status: open

## What's needed

Run `kubecom` against a real cluster and confirm the Exec-shell action works end to
end (the sandbox has no cluster and no TTY, so this path is unverified by tests):

1. Browse to a **Pod** (single-container is the simplest first check), open the
   actions menu, and pick **Exec shell**.
2. Confirm the TUI suspends and a working `/bin/sh` **drops in** — typing echoes,
   `ls`, arrow/history editing, and **`Ctrl-C`** all behave (i.e. the local terminal
   is in raw mode and keystrokes reach the remote PTY).
3. `exit` the shell and confirm the **browse UI is restored** cleanly (no leftover
   raw mode, no garbled screen) and the status bar shows a neutral "exec session
   ended" notice.
4. Trigger a **failure** path — a Pod/image with no `/bin/sh`, or a namespace where
   RBAC forbids `pods/exec` — and confirm it degrades to a **status-bar error toast**
   (no panic, TUI intact).
5. Note whether the initial shell size matches the terminal, and what happens on a
   window resize mid-session (live SIGWINCH resize is intentionally deferred to
   M3-14b-3 — a mismatch after resize is expected, not a bug in this slice).

## Why the agent can't do it

The autonomous loop runs in a cluster-less sandbox with no interactive TTY, so it
cannot exercise the SPDY exec dial, local raw-mode passthrough, or the
suspend/restore round-trip — only the routing, option construction, and result
handling are covered by hermetic tests (M3-14b-1). Confirming an actual shell is a
real-terminal, real-cluster check (teatest ≠ a human's eyes).

## How to resolve

Do the steps above. If it all works, set `Status: done` with a one-line `## Result`
(the agent ticks the M3 exec exit criterion and deletes this file next leg), or just
delete this file. Anything that needs fixing (a raw-mode glitch, a restore bug, a bad
default shell) goes in `../feedback/` so the loop acts on it.
