package stderrfd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests move the process's real fd 2 around, so none of them may run in
// parallel — with each other or with anything else in this package. Each one first
// installs its own "terminal": a Guard pointing fd 2 at a file, closed on cleanup.
// That is what makes the assertions hermetic — a Release lands in a file the test can
// read rather than on the test runner's own stderr, and nothing is left dangling if a
// case fails midway.

// stubTerminal points fd 2 at a scratch file standing in for the terminal kubecom was
// launched from, and returns its path. The guard is closed on cleanup, restoring the
// real fd 2 the test binary started with.
func stubTerminal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terminal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("opening the stand-in terminal: %v", err)
	}
	g, err := Redirect(f)
	if err != nil {
		t.Fatalf("installing the stand-in terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = g.Close()
		_ = f.Close()
	})
	return path
}

// logFile opens a scratch file standing in for ~/.cache/kubecom/kubecom.log.
func logFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "kubecom.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("opening the log: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// TestRedirectMovesTheDescriptorNotTheVariable is the claim AUTH-07 rests on: after
// Redirect, a write through the os.Stderr *variable* — which is what client-go holds,
// and what the runtime's panic path uses — lands in the log rather than on the
// terminal, without anyone having reassigned that variable.
func TestRedirectMovesTheDescriptorNotTheVariable(t *testing.T) {
	term := stubTerminal(t)
	log := logFile(t)

	g, err := Redirect(log)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}
	defer func() { _ = g.Close() }()

	if _, err := os.Stderr.WriteString("plugin chatter\n"); err != nil {
		t.Fatalf("writing to os.Stderr: %v", err)
	}
	if got := read(t, log.Name()); !strings.Contains(got, "plugin chatter") {
		t.Fatalf("the log should hold what os.Stderr was given, got %q", got)
	}
	if got := read(t, term); got != "" {
		t.Fatalf("nothing should have reached the terminal, got %q", got)
	}
}

// TestRedirectCoversSubprocesses proves the redirect follows a child process, which is
// how the credential plugin actually writes: client-go runs it with cmd.Stderr =
// os.Stderr, so the child inherits the descriptor — and inheriting is a property of
// fd 2, not of the variable.
func TestRedirectCoversSubprocesses(t *testing.T) {
	term := stubTerminal(t)
	log := logFile(t)

	g, err := Redirect(log)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}
	defer func() { _ = g.Close() }()

	cmd := exec.Command("sh", "-c", "echo could not refresh credentials >&2")
	cmd.Stderr = os.Stderr // exactly what plugin/pkg/client/auth/exec does.
	if err := cmd.Run(); err != nil {
		t.Fatalf("running the stand-in plugin: %v", err)
	}
	if got := read(t, log.Name()); !strings.Contains(got, "could not refresh credentials") {
		t.Fatalf("the plugin's stderr should be in the log, got %q", got)
	}
	if got := read(t, term); got != "" {
		t.Fatalf("the plugin must not paint on the terminal, got %q", got)
	}
}

// TestReleaseAndReclaim drives the suspend cycle: fd 2 goes back to the terminal for
// the length of the handover — which is what makes $EDITOR's or `aws sso login`'s
// complaints visible — and comes back to the log afterwards.
func TestReleaseAndReclaim(t *testing.T) {
	term := stubTerminal(t)
	log := logFile(t)

	g, err := Redirect(log)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}
	defer func() { _ = g.Close() }()

	if err := g.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stderr.WriteString("vim: cannot open file\n"); err != nil {
		t.Fatalf("writing during the suspend: %v", err)
	}
	if err := g.Reclaim(); err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	if _, err := os.Stderr.WriteString("post-suspend chatter\n"); err != nil {
		t.Fatalf("writing after the suspend: %v", err)
	}

	gotTerm, gotLog := read(t, term), read(t, log.Name())
	if !strings.Contains(gotTerm, "vim: cannot open file") {
		t.Fatalf("the suspended subprocess should reach the terminal, got %q", gotTerm)
	}
	if strings.Contains(gotTerm, "post-suspend chatter") {
		t.Fatalf("Reclaim did not take fd 2 back: %q", gotTerm)
	}
	if !strings.Contains(gotLog, "post-suspend chatter") {
		t.Fatalf("writes after Reclaim belong in the log, got %q", gotLog)
	}
	if strings.Contains(gotLog, "vim: cannot open file") {
		t.Fatalf("the released write should not have gone to the log: %q", gotLog)
	}
}

// TestCloseRestoresPermanently covers the exit path: once the program is leaving,
// cobra's error and anything else printed after the TUI must reach the user, and the
// guard is inert from then on rather than able to steal fd 2 back.
func TestCloseRestoresPermanently(t *testing.T) {
	term := stubTerminal(t)
	log := logFile(t)

	g, err := Redirect(log)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("a second Close should be a no-op, got %v", err)
	}
	// Inert afterwards: a stale Reclaim from a suspend that outlived the program must
	// not put fd 2 back on a file nobody is reading.
	if err := g.Reclaim(); err != nil {
		t.Fatalf("Reclaim after Close: %v", err)
	}
	if _, err := os.Stderr.WriteString("kubecom exited with error\n"); err != nil {
		t.Fatalf("writing after Close: %v", err)
	}
	if got := read(t, term); !strings.Contains(got, "kubecom exited with error") {
		t.Fatalf("after Close the user should see stderr again, got %q", got)
	}
	if got := read(t, log.Name()); strings.Contains(got, "kubecom exited with error") {
		t.Fatalf("nothing should still be going to the log, got %q", got)
	}
}

// TestNilGuardIsInert pins the degrade path: a redirect that could not be installed
// leaves the launcher with no guard, and every method must still be callable so the
// shell's suspends stay code-free of nil checks.
func TestNilGuardIsInert(t *testing.T) {
	var g *Guard
	if err := g.Release(); err != nil {
		t.Fatalf("Release on a nil guard: %v", err)
	}
	if err := g.Reclaim(); err != nil {
		t.Fatalf("Reclaim on a nil guard: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("Close on a nil guard: %v", err)
	}
}

// TestRedirectRejectsNoFile keeps the constructor from installing a guard over a nil
// file, which would panic on the first Reclaim rather than at the call that was wrong.
func TestRedirectRejectsNoFile(t *testing.T) {
	if _, err := Redirect(nil); err == nil {
		t.Fatal("Redirect(nil) should fail")
	}
}
