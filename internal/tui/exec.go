package tui

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is the M3-14b-1 exec wire: the TUI surface that drops the user into an
// interactive shell inside a Pod's container and restores the browse UI afterwards.
// The kube layer already has the primitive — a *blocking* Clients.Exec over the pod
// exec subresource (M3-14a/D124) — so the TUI's job is only to run it from a
// suspended terminal. That is what bubbletea's tea.Exec is for: it releases the
// terminal, runs an ExecCommand synchronously (so the shell owns the screen, off the
// update loop — D124's contract), then re-captures the terminal and delivers the
// result as a Msg. An interactive shell needs the local terminal in raw mode so
// keystrokes (and ^C) pass straight through to the remote PTY, so execCommand.Run
// puts it raw for the exec's lifetime and restores it before returning — nested
// inside bubbletea's own release/restore.
//
// A multi-container Pod prompts which container to exec into via the shared container
// picker (M3-14b-2, reusing the M3-07a ctrPicker); a single-container Pod execs
// directly. The exec runs /bin/sh, seeds the initial terminal size, and tracks live
// window resizes via a SIGWINCH watcher so the remote PTY follows the local terminal
// (M3-14b-3). Linux/macOS only — the raw-PTY path is a non-goal on native Windows
// (WSL2 instead, D7).
//
// M3-14b-4 adds the parity escape hatch (D2/D7/D128): when the `kubectl` binary is on
// PATH, exec suspends into `kubectl exec -it` (via tea.ExecProcess) instead of the
// in-process SPDY path — kubectl owns its own raw PTY, SIGWINCH resize, and every
// server-side edge case, so it is the battle-tested parity path when available. The
// in-process path is the *fallback* that keeps exec working with no kubectl installed
// (removing the hard kubectl dependency, #68/D2). The shelled-out kubectl is pointed
// at the same cluster via --kubeconfig / --context / -n so it matches what kubecom is
// browsing.

// Execer is the narrow slice of the kube layer the shell needs to run the exec
// action (M3-14b-1): open an interactive session in a pod's container (M3-14a's
// blocking Exec). *kube.Clients satisfies it. As with the other action seams the
// shell depends on this interface, not the concrete client, so the tui package
// never constructs a client and the exec flow is driveable in hermetic tests with a
// fake execer. A model built without one (the default) is exec-inert: the Exec-shell
// action is a no-op, exactly as the entry is absent from a non-Pod kind's menu.
type Execer interface {
	Exec(ctx context.Context, ref kube.ObjectRef, opts kube.ExecOptions) error
}

// WithExecer wires the kube client the shell uses to open an interactive exec
// session in a Pod's container (M3-14b-1). Without it the Exec-shell action is inert.
func WithExecer(e Execer) Option {
	return func(m *Model) { m.execer = e }
}

// defaultExecShell is the argv exec runs when the user picks Exec-shell. /bin/sh is
// the lowest-common-denominator shell present in virtually every image (unlike
// /bin/bash), matching `kubectl exec -it pod -- /bin/sh`. A shell probe / override
// is a later refinement; this slice keeps it fixed.
var defaultExecShell = []string{"/bin/sh"}

// execDoneMsg carries the outcome of an interactive exec once tea.Exec resumes the
// program (M3-14b-1). label is the human target ("Pod default/web-1") for the
// status-bar result. err is nil on a clean shell exit, non-nil on a failure to
// attach (RBAC denial, missing shell) or a non-zero shell exit (remotecommand
// surfaces the exit code as an error) — either way it degrades to a transient toast,
// never a panic (principle 3).
type execDoneMsg struct {
	label string
	err   error
}

// openExec starts the Exec-shell flow over the selected Pod (M3-14b-1/2). With no
// execer wired, or an empty ref (a row with no name, guarded so an empty ref never
// reaches the kube layer), it is a no-op — exactly as the Exec-shell entry is absent
// from a non-Pod kind's actions menu. Otherwise it resolves the Pod's containers
// (reusing the M3-07a container-resolution path, tagged with the exec purpose): a
// single-container Pod execs directly, a multi-container Pod prompts which container
// via the shared picker (execInto is the terminal both routes reach).
func (m Model) openExec(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.execer == nil || msg.Object.Name == "" {
		return m, nil
	}
	return m.resolveContainersFor(msg.Resource, msg.Object, ctrPurposeExec)
}

// execInto suspends the TUI into an interactive shell in ref's container (M3-14b-2).
// container "" execs the pod's default/sole container (the M3-14b-1 behaviour, used
// when no container lister is wired); a resolved single container or a picked one is
// passed by name. It is the exec twin of streamLogsInto — the terminal both the
// single-container fast path and the picker selection route to (streamOrExec). It
// returns the tea.Exec command bubbletea runs from a released terminal; the callback
// reports the session result to the status bar (handleExecDone). Guarded against a
// nil execer / empty ref so an empty target never reaches the kube layer.
func (m Model) execInto(res kube.Resource, ref kube.ObjectRef, container string) (tea.Model, tea.Cmd) {
	if m.execer == nil || ref.Name == "" {
		return m, nil
	}
	label := viewerTitle(res, ref)
	callback := func(err error) tea.Msg {
		return execDoneMsg{label: label, err: err}
	}
	// Parity escape hatch (M3-14b-4/D128): when kubectl is on PATH, suspend into it
	// rather than the in-process SPDY path — kubectl owns its own raw PTY + resize and
	// covers every server-side edge case. tea.ExecProcess wraps the *exec.Cmd,
	// releasing/re-capturing the terminal around it exactly as the in-process wire does.
	if proc, ok := m.kubectlExecProc(ref, container); ok {
		return m, tea.ExecProcess(proc, callback)
	}
	cmd := newExecCommand(m.execer, ref, container)
	return m, tea.Exec(cmd, callback)
}

// lookupKubectl resolves the kubectl binary on PATH for the exec parity fallback
// (M3-14b-4/D128). It is a package var so tests can force the fallback on or off
// without a real kubectl on the runner. Empty path + false → not found, use the
// in-process SPDY path.
var lookupKubectl = func() (path string, ok bool) {
	p, err := exec.LookPath("kubectl")
	if err != nil {
		return "", false
	}
	return p, true
}

// kubectlExecProc builds the `kubectl exec -it` process to suspend into when the
// kubectl binary is on PATH (the parity escape hatch, M3-14b-4/D128), or returns
// (nil,false) to signal the caller to use the in-process SPDY path. The kubectl is
// pointed at the same cluster kubecom launched with via --kubeconfig / --context and
// the row's namespace via -n, so it targets exactly the pod being browsed.
func (m Model) kubectlExecProc(ref kube.ObjectRef, container string) (*exec.Cmd, bool) {
	path, ok := lookupKubectl()
	if !ok {
		return nil, false
	}
	return exec.Command(path, kubectlExecArgs(m.kubeconfig, m.context, ref, container)...), true //nolint:gosec // path is the resolved kubectl; args are namespace/context/pod identifiers, not shell.
}

// kubectlExecArgs builds the argv for `kubectl exec` targeting ref's container
// (M3-14b-4). Connection flags (--kubeconfig, --context, -n) are emitted only when set
// so kubectl falls back to its standard resolution otherwise; -i -t requests the
// interactive TTY, an empty container omits -c (kubectl picks the default container,
// matching the in-process path), and `-- /bin/sh` runs the same shell as the SPDY path.
func kubectlExecArgs(kubeconfig, context string, ref kube.ObjectRef, container string) []string {
	args := make([]string, 0, 12)
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	if context != "" {
		args = append(args, "--context", context)
	}
	if ref.Namespace != "" {
		args = append(args, "-n", ref.Namespace)
	}
	args = append(args, "exec", "-i", "-t", ref.Name)
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--")
	return append(args, defaultExecShell...)
}

// handleExecDone reports a finished exec session: a failure (attach error, missing
// shell, non-zero exit) degrades to a transient error toast (D74), a clean exit to a
// neutral status notice. The browse UI is already restored by the time this lands —
// tea.Exec re-captured the terminal before delivering the callback msg.
func (m Model) handleExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("exec "+msg.label, msg.err))
	}
	return m, m.surfaceNotice("exec session ended · " + msg.label)
}

// execCommand adapts kube.Exec to bubbletea's ExecCommand (tea.Exec), so an
// interactive shell runs in the suspended terminal off the update loop (D124).
// bubbletea releases the terminal, calls the Set* setters with the program's
// streams, runs Run() to completion, then re-captures the terminal. Run puts the
// local terminal into raw mode (bubbletea released it to cooked) so input passes
// through to the remote PTY, then blocks in kube.Exec until the shell exits, then
// restores cooked mode — all before returning, so it nests cleanly inside
// bubbletea's own release/restore.
type execCommand struct {
	execer    Execer
	ref       kube.ObjectRef
	container string   // "" → the pod's default/sole container; else the chosen one (14b-2)
	command   []string // the shell argv

	stdin  io.Reader
	stdout io.Writer
}

// newExecCommand builds the exec adapter for container in ref's pod (M3-14b-2); an
// empty container execs the pod's default/sole container (M3-14b-1).
func newExecCommand(execer Execer, ref kube.ObjectRef, container string) *execCommand {
	return &execCommand{execer: execer, ref: ref, container: container, command: defaultExecShell}
}

// SetStdin/SetStdout capture the terminal streams bubbletea hands the exec. SetStderr
// is a no-op: an exec with a TTY has no separate stderr stream — the server
// multiplexes it into stdout (D124), so kube.Exec attaches only stdin+stdout.
func (c *execCommand) SetStdin(r io.Reader)  { c.stdin = r }
func (c *execCommand) SetStdout(w io.Writer) { c.stdout = w }
func (c *execCommand) SetStderr(io.Writer)   {}

// Run streams the interactive exec to completion. When stdin is a real terminal it
// switches it to raw mode for the session (restored on return), seeds the exec's size
// queue with the terminal's current size, and starts a SIGWINCH watcher that pushes
// the new size on every window resize so the remote PTY tracks the local window
// (M3-14b-3); a non-terminal stdin (a test's buffer, a piped run) skips the
// raw/size/resize handling and just streams. The blocking kube.Exec owns the terminal
// until the shell exits. The watcher is stopped (deferred before the queue's close, so
// LIFO tears it down first) before Run returns, so no push ever races a closed queue.
func (c *execCommand) Run() error {
	q := newExecSizeQueue()
	defer q.close()

	if fd, ok := fileFd(c.stdin); ok && term.IsTerminal(fd) {
		if st, err := term.MakeRaw(fd); err == nil {
			defer func() { _ = term.Restore(fd, st) }()
		}
		sizeOf := func() (uint16, uint16, bool) {
			w, h, err := term.GetSize(fd)
			if err != nil {
				return 0, 0, false
			}
			return uint16(w), uint16(h), true
		}
		if w, h, ok := sizeOf(); ok {
			q.seed(w, h)
		}
		defer watchResize(q, sizeOf)()
	}

	return c.execer.Exec(context.Background(), c.ref, kube.ExecOptions{
		Container: c.container,
		Command:   c.command,
		TTY:       true,
		Stdin:     c.stdin,
		Stdout:    c.stdout,
		SizeQueue: q,
	})
}

// watchResize pumps live terminal-size changes into q until the returned stop func is
// called (M3-14b-3). It listens for SIGWINCH and on each one reads the current size
// via sizeOf and pushes it to the queue (latest-wins), so the remote PTY follows the
// local window mid-session — 14b-1 seeded only the size at exec start. stop
// unregisters the signal and blocks until the pump goroutine has exited, so the caller
// can then close the queue with no push racing a closed channel. sizeOf is injected so
// the pump is hermetically testable (fed a fake reader + an in-process SIGWINCH)
// without a real terminal.
func watchResize(q *execSizeQueue, sizeOf func() (uint16, uint16, bool)) (stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-sig:
				if w, h, ok := sizeOf(); ok {
					q.push(w, h)
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
		<-finished
	}
}

// fileFd returns the file descriptor of r when it is an *os.File (the real terminal
// bubbletea passes for a standard program), so raw-mode / size handling only kicks in
// for an actual terminal. Anything else (a test buffer, a pipe) reports false.
func fileFd(r io.Reader) (int, bool) {
	if f, ok := r.(*os.File); ok {
		return int(f.Fd()), true
	}
	return 0, false
}

// execSizeQueue is the exec's kube.TerminalSizeQueue (M3-14b-1/3): it delivers the
// initial terminal size (seed), then the latest size on every window resize (push),
// then blocks until the session ends (close → Next returns nil, the queue's
// end-of-session signal). A one-slot latest-wins buffer holds the pending size so
// producers never block; a burst of resizes collapses to the newest. Concurrency-safe:
// seed runs on the exec goroutine before the size watcher starts, push on the SIGWINCH
// watcher goroutine, Next on remotecommand's reader goroutine, close on the exec
// goroutine after the watcher has stopped (so push never races the close).
type execSizeQueue struct {
	ch        chan kube.TerminalSize
	closeOnce sync.Once
}

func newExecSizeQueue() *execSizeQueue {
	return &execSizeQueue{ch: make(chan kube.TerminalSize, 1)}
}

// seed offers the initial terminal size, dropped if a size is already queued or the
// dimensions are degenerate (0×0 — remotecommand then falls back to server defaults).
func (q *execSizeQueue) seed(w, h uint16) {
	if w == 0 || h == 0 {
		return
	}
	select {
	case q.ch <- kube.TerminalSize{Width: w, Height: h}:
	default:
	}
}

// push replaces the pending terminal size with the latest w×h on a window resize
// (M3-14b-3), so the newest size always wins and a slow reader never lags behind a
// burst of SIGWINCH events. It never blocks the watcher goroutine: if a stale size is
// still queued it is dropped and the newer one takes its place. A degenerate 0×0 read
// is ignored. Safe against close because the watcher is stopped before close (see
// watchResize) — push is never called on a closed channel.
func (q *execSizeQueue) push(w, h uint16) {
	if w == 0 || h == 0 {
		return
	}
	s := kube.TerminalSize{Width: w, Height: h}
	for {
		select {
		case q.ch <- s:
			return
		default:
			select {
			case <-q.ch: // drop the stale pending size, then retry with the newer one
			default:
			}
		}
	}
}

// Next returns the next terminal size, or nil once the queue is closed (session end).
func (q *execSizeQueue) Next() *kube.TerminalSize {
	s, ok := <-q.ch
	if !ok {
		return nil
	}
	return &s
}

// close ends the session: a blocked Next returns nil, and the reader goroutine exits.
// Idempotent so a double teardown never panics on a closed channel.
func (q *execSizeQueue) close() {
	q.closeOnce.Do(func() { close(q.ch) })
}
