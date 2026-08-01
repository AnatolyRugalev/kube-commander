# Auto-detect an installed editor at startup instead of falling through to `vi`

- Submitted: 2026-08-01
- Priority: normal
- Area: edit flow, startup, `$EDITOR`

If the editor is not set we should auto detect it at startup. Full precedence:

```
KUBE_EDITOR  ->  EDITOR  ->  VISUAL  ->  first present on PATH of:
                                            1. nvim
                                            2. vim
                                            3. nano
                                            4. vi
```

## Why

Hit this dogfooding on 2026-08-01: with none of the editor variables set,
`resolveEditorArgv` (`internal/tui/edit.go:72`) falls straight through to
`defaultEditor = "vi"` (`edit.go:59`), and the host had no `vi` — Arch's `vim` package
installs `/usr/bin/vim` but no `vi` symlink, and my editor is neovim. So pressing `e` on a
live object dead-ends on a machine with two perfectly good editors installed.

To be clear about what is **not** wrong: the failure degraded correctly — `exec.Command`
failed, it surfaced as a toast, nothing was applied and nothing was mutated. This is a
first-run ergonomics fix, not a bug fix, and it must not weaken the "an aborted edit never
mutates" property (`edit.go:84-90`).

## On the ordering

`edit.go:56` justifies the `vi` fallback as matching kubectl's own precedence, which is a
real argument — but kubectl is not trying to be a friendly TUI, and "vi exists everywhere"
is no longer true on modern Linux.

The order above prefers editors whose *presence implies a choice*. On Arch, `nano` ships
in the `base` meta-package, so finding nano says little about what the user wants, whereas
finding `nvim` or `vim` says a lot. nano still sits ahead of bare `vi` as the
"you can always exit it" fallback. This is also consistent with kubecom being a
vim-friendly tool (`vault/goals.md`) — handing a vim user nano on a box that has both
would be the more annoying error.

`VISUAL` is added after `EDITOR` because the Unix convention reserves it for full-screen
editors, which is exactly this case. Note this **diverges from kubectl**, which consults
only `KUBE_EDITOR` and `EDITOR` — worth a line in the journal, and worth keeping the
divergence to just this one addition.

## Implementation notes

- **Resolve at startup, not at `e`-press time**, as the title says. The point is that
  kubecom can then log the chosen editor (`~/.cache/kubecom/kubecom.log`, D159) so you
  find out which editor you will get *before* you press `e` on a live object — rather than
  discovering the fallback is broken at the moment you wanted to change something.
- **Keep `resolveEditorArgv` pure.** It is currently pure and unit-tested without touching
  the environment, which is why the flag-splitting tests are hermetic. PATH detection is
  impure, so inject the lookup (an `exec.LookPath`-shaped func) rather than calling
  `exec.LookPath` inside it — that keeps the precedence table testable with a fake PATH.
- **If nothing at all is found**, say so clearly and early rather than failing at
  `e`-press: "no editor found; set $EDITOR" is a better message than a failed exec of a
  binary the user never chose.
- The flag-splitting behaviour for values like `code -w` must survive unchanged
  (`edit.go:74`); detection only applies when all three variables are empty.
