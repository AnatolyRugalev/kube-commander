package tui

import (
	"io"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// This file is AUTH-07's TUI half: every suspend goes through one wrapper, so the
// terminal handover and the fd-2 handover can never come apart.
//
// The launcher points fd 2 at the log file for the life of the TUI (internal/stderrfd),
// because client-go streams an exec credential plugin's stderr onto the terminal on
// every refresh and the alt screen is no protection. But three flows hand the terminal
// over on purpose — $EDITOR (D125), `kubectl exec` / the SPDY shell (D124), an approved
// `aws sso login` (D215) — and there the reader must see what the subprocess says,
// stderr included: an editor's error, a login's prompt. bubbletea hands those commands
// the `os.Stderr` *variable*, which is fd 2 whatever fd 2 currently points at, so the
// fix is to move the descriptor back for the length of the Run and no longer.
//
// Every tea.Exec in this package therefore goes through Model.suspend rather than
// calling tea.Exec directly. A new suspend that forgets is not a compile error, so the
// three call sites are pinned by a test that asserts on the wrapper's type.

// TerminalStderr is the seam the shell drives to move fd 2 between the log file and
// the terminal (satisfied by *stderrfd.Guard). The shell never opens the log itself —
// it is handed one that is already installed, which keeps this package free of
// syscalls and lets a test observe the two calls without touching the process.
//
// Unwired (the default, and every hermetic test) the shell suspends exactly as it did
// before AUTH-07: the handover is a no-op and fd 2 is whatever the caller left it.
type TerminalStderr interface {
	// Release points fd 2 back at the terminal, for a subprocess the reader watches.
	Release() error
	// Reclaim points fd 2 back at the log file once that subprocess is done.
	Reclaim() error
}

// WithTerminalStderr wires the fd-2 guard the launcher installed (AUTH-07), so the
// suspends that hand the terminal over on purpose also hand back the descriptor.
// Without it the shell is handover-inert — correct anywhere fd 2 was never redirected,
// which is every test and any embedding that does its own logging.
func WithTerminalStderr(s TerminalStderr) Option {
	return func(m *Model) { m.termStderr = s }
}

// suspend is the one route from this package into tea.Exec. It wraps c so fd 2 belongs
// to the terminal for the length of c.Run — see handover.
func (m Model) suspend(c tea.ExecCommand, fn tea.ExecCallback) tea.Cmd {
	return tea.Exec(handover{inner: c, term: m.termStderr}, fn)
}

// suspendProcess is suspend for a command kubecom shells out to (the kubectl exec
// parity path, D128). It is a separate entry point only because bubbletea's own
// *exec.Cmd adapter is unexported, so tea.ExecProcess cannot be composed with the
// wrapper — procCommand is that adapter, reimplemented to the same contract.
func (m Model) suspendProcess(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	return m.suspend(procCommand{Cmd: c}, fn)
}

// handover is a tea.ExecCommand that gives fd 2 back to the terminal around the
// wrapped command's Run. bubbletea has already released the terminal by the time Run
// is called and re-captures it after Run returns, so the descriptor's window is a
// subset of the terminal's — the TUI is never on screen while fd 2 points away from
// the log.
//
// A failing Release is not fatal and does not abort the suspend: the subprocess still
// runs, its stderr merely lands in the log file, which is where it was going a moment
// ago anyway. Aborting an approved `aws sso login` because a dup2 failed would trade a
// cosmetic fault for a functional one.
type handover struct {
	inner tea.ExecCommand
	term  TerminalStderr
}

// SetStdin/SetStdout/SetStderr pass the program's streams straight through to the
// wrapped command — the wrapper adds a descriptor swap, it does not touch the streams.
// Note that the stderr bubbletea passes is the `os.Stderr` variable, which resolves to
// fd 2 at write time, so the swap below is what decides where those writes land.
func (h handover) SetStdin(r io.Reader)  { h.inner.SetStdin(r) }
func (h handover) SetStdout(w io.Writer) { h.inner.SetStdout(w) }
func (h handover) SetStderr(w io.Writer) { h.inner.SetStderr(w) }

// Run releases fd 2 to the terminal, runs the wrapped command, and reclaims it —
// reclaiming in a defer, because a command that failed (or panicked) has still given
// the terminal back to the TUI, and fd 2 left on the terminal after that is exactly
// the hole AUTH-07 closes.
func (h handover) Run() error {
	if h.term != nil {
		_ = h.term.Release()
		defer func() { _ = h.term.Reclaim() }()
	}
	return h.inner.Run()
}

// procCommand adapts an *exec.Cmd to tea.ExecCommand, mirroring bubbletea's own
// unexported osExecCommand: each setter fills a stream only if the caller left it nil,
// so a command that already chose its own stdin/stdout/stderr keeps it. It exists so
// the kubectl exec path can be wrapped in a handover like the other two suspends.
type procCommand struct{ *exec.Cmd }

func (c procCommand) SetStdin(r io.Reader) {
	if c.Stdin == nil {
		c.Stdin = r
	}
}

func (c procCommand) SetStdout(w io.Writer) {
	if c.Stdout == nil {
		c.Stdout = w
	}
}

func (c procCommand) SetStderr(w io.Writer) {
	if c.Stderr == nil {
		c.Stderr = w
	}
}
