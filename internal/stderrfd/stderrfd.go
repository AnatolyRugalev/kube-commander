// Package stderrfd points the process's standard error **file descriptor** — fd 2
// itself, not the `os.Stderr` variable — at a file of kubecom's choosing, and hands
// it back for the length of a subprocess the user is meant to see.
//
// It exists because of AUTH-07: while the TUI owns the terminal, anything that
// writes to fd 2 paints over the panes and survives until bubbletea repaints those
// lines. The alt screen is the same terminal, so "the TUI is on the alt screen" is
// not protection. client-go is the concrete offender — the exec credential plugin's
// stderr is wired to `os.Stderr` when the authenticator is built
// (`plugin/pkg/client/auth/exec/exec.go`) and streams on *every* credential refresh,
// not only a failing one — but a panic trace, a cgo library's chatter and anything
// else holding fd 2 corrupt the layout identically.
//
// Reassigning the `os.Stderr` *variable* does not fix any of that: client-go has
// already captured the old value, a subprocess inherits the descriptor rather than
// the variable, and the runtime writes a panic straight to fd 2. Only the descriptor
// will do, which is what dup2 is for.
//
// The redirect is not permanent. Three flows hand the terminal over on purpose —
// `$EDITOR`, `kubectl exec`, an approved `aws sso login` — and there the reader
// *must* see the subprocess's stderr, so a Guard releases fd 2 back to the terminal
// for the length of the suspend and reclaims it afterwards (see tui.TerminalStderr).
//
// Unix only, like the rest of kubecom's terminal handling (D7).
package stderrfd

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

// stderrFD is the descriptor being juggled. Named rather than spelled 2 at four call
// sites, since every dup2 below is only meaningful relative to it.
const stderrFD = 2

// Guard owns fd 2 while the TUI holds the terminal: it points the descriptor at a
// file (the log) and keeps a private duplicate of whatever it was pointing at
// before, so the original can be put back.
//
// Its methods are safe to call on a nil *Guard, which is what lets the launcher pass
// one seam whether or not the redirect could be installed (principle 3: a failed
// redirect degrades to today's behaviour, it never blocks launch).
type Guard struct {
	mu     sync.Mutex
	target *os.File // where fd 2 points while the guard holds it
	saved  int      // dup of the original fd 2 — the real terminal
	closed bool
}

// Redirect points fd 2 at f and returns the Guard that can undo it. The caller keeps
// ownership of f (the log file is closed by whoever opened it); the Guard only
// duplicates descriptors.
//
// The saved duplicate is marked close-on-exec: it is kubecom's private handle on the
// terminal, and a subprocess inheriting a stray extra descriptor for it is a leak
// with no upside. fd 2 itself stays inheritable, which is the entire point — a child
// wired to `os.Stderr` must land wherever fd 2 currently points.
func Redirect(f *os.File) (*Guard, error) {
	if f == nil {
		return nil, fmt.Errorf("stderrfd: no file to redirect to")
	}
	saved, err := syscall.Dup(stderrFD)
	if err != nil {
		return nil, fmt.Errorf("stderrfd: duplicating fd %d: %w", stderrFD, err)
	}
	syscall.CloseOnExec(saved)
	if err := dup2(int(f.Fd()), stderrFD); err != nil {
		_ = syscall.Close(saved)
		return nil, fmt.Errorf("stderrfd: pointing fd %d at %s: %w", stderrFD, f.Name(), err)
	}
	return &Guard{target: f, saved: saved}, nil
}

// Release gives fd 2 back to the terminal kubecom started on. Call it around a
// subprocess the reader is meant to watch — the editor, the exec shell, an
// interactive login — and pair it with Reclaim.
//
// It is idempotent in effect: releasing an already-released guard re-points fd 2 at
// the same saved descriptor.
func (g *Guard) Release() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	if err := dup2(g.saved, stderrFD); err != nil {
		return fmt.Errorf("stderrfd: restoring fd %d to the terminal: %w", stderrFD, err)
	}
	return nil
}

// Reclaim points fd 2 back at the file, undoing a Release. It is the half that must
// run even when the suspend failed: a subprocess that died still returns the terminal
// to the TUI, and leaving fd 2 on the terminal after that reopens exactly the hole
// this package closes.
func (g *Guard) Reclaim() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	if err := dup2(int(g.target.Fd()), stderrFD); err != nil {
		return fmt.Errorf("stderrfd: pointing fd %d back at %s: %w", stderrFD, g.target.Name(), err)
	}
	return nil
}

// Close restores fd 2 to the terminal permanently and drops the saved duplicate.
// The launcher defers it around the program run, so anything printed after the TUI
// exits — cobra's error, a panic on the way out — reaches the user rather than the
// log file. Calling it twice is a no-op; every later Release/Reclaim is inert.
func (g *Guard) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	err := dup2(g.saved, stderrFD)
	if cerr := syscall.Close(g.saved); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("stderrfd: releasing fd %d: %w", stderrFD, err)
	}
	return nil
}
