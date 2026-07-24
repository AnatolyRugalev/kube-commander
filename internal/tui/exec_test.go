package tui

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// fakeExecer is a hermetic Execer (M3-14b-1): it records the pod addressed and the
// ExecOptions it was handed (so a test can assert the selected row was targeted with
// a TTY shell) and returns a preset error. It never dials — the real SPDY exec is
// envtest / live-cluster territory (D124), so the routing, option construction, and
// result handling are all covered without a cluster.
type fakeExecer struct {
	err     error
	calls   int
	gotRef  kube.ObjectRef
	gotOpts kube.ExecOptions
}

func (f *fakeExecer) Exec(_ context.Context, ref kube.ObjectRef, opts kube.ExecOptions) error {
	f.calls++
	f.gotRef = ref
	f.gotOpts = opts
	return f.err
}

// podExecModel drills into a pods table (two rows via sortReset) with the given
// options wired, so an exec test has a concrete selected Pod row.
func podExecModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// TestExecOpensSession proves the Exec-shell intent suspends into a session: it
// returns a non-nil command (the tea.Exec bubbletea runs from a released terminal)
// and opens no confirm modal — exec is a direct suspend, not a mutating action.
func TestExecOpensSession(t *testing.T) {
	m := podExecModel(t, WithExecer(&fakeExecer{}))

	m, cmd := dispatchRowAction(t, m, rowActionExec)
	if m.modal.Active() {
		t.Fatal("exec is a suspend action — it must not open a confirm modal")
	}
	if cmd == nil {
		t.Fatal("the exec intent should issue the tea.Exec suspend command")
	}
}

// TestExecInertWithoutExecer proves the action is a no-op with no execer wired (the
// pre-wiring default): no command, no modal.
func TestExecInertWithoutExecer(t *testing.T) {
	m := podExecModel(t)
	m, cmd := dispatchRowAction(t, m, rowActionExec)
	if cmd != nil {
		t.Fatal("with no execer wired the exec action must be inert (no command)")
	}
	if m.modal.Active() {
		t.Fatal("exec-inert must not open any modal")
	}
}

// TestExecInertOnEmptyRef proves a row with no name never reaches the kube layer —
// an empty ref is guarded so exec is a no-op rather than dialing an empty pod.
func TestExecInertOnEmptyRef(t *testing.T) {
	f := &fakeExecer{}
	m := podExecModel(t, WithExecer(f))
	_, cmd := m.openExec(rowActionMsg{Action: rowActionExec, Resource: m.current, Object: kube.ObjectRef{}})
	if cmd != nil {
		t.Fatal("an empty ref should make exec inert (no command)")
	}
}

// TestExecCommandRunStreamsTTYShell drives the ExecCommand adapter's Run against a
// fake executor with a non-terminal stdin (so raw-mode / size handling is skipped):
// it must call kube.Exec once on the addressed pod's default container with a TTY,
// the /bin/sh argv, and the captured stdin/stdout streams attached.
func TestExecCommandRunStreamsTTYShell(t *testing.T) {
	f := &fakeExecer{}
	ref := kube.ObjectRef{Namespace: "web", Name: "api-1", UID: "u1"}
	c := newExecCommand(f, ref)

	var in bytes.Buffer
	var out bytes.Buffer
	c.SetStdin(&in)
	c.SetStdout(&out)
	c.SetStderr(&out) // must be a no-op — a TTY exec has no separate stderr

	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil on a clean exit", err)
	}
	if f.calls != 1 {
		t.Fatalf("Exec called %d times, want exactly 1", f.calls)
	}
	if f.gotRef != ref {
		t.Fatalf("Exec addressed %+v, want the selected pod %+v", f.gotRef, ref)
	}
	if !f.gotOpts.TTY {
		t.Fatal("an interactive shell must request a TTY")
	}
	if f.gotOpts.Container != "" {
		t.Fatalf("14b-1 execs the default container (empty), got %q", f.gotOpts.Container)
	}
	if len(f.gotOpts.Command) != 1 || f.gotOpts.Command[0] != "/bin/sh" {
		t.Fatalf("Command = %v, want [/bin/sh]", f.gotOpts.Command)
	}
	if f.gotOpts.Stdin != &in || f.gotOpts.Stdout != &out {
		t.Fatal("the captured stdin/stdout should be attached to the exec")
	}
	if f.gotOpts.Stderr != nil {
		t.Fatal("SetStderr must be a no-op for a TTY exec (stderr folds into stdout)")
	}
	if f.gotOpts.SizeQueue == nil {
		t.Fatal("the exec should be given a size queue")
	}
}

// TestExecCommandRunPropagatesError proves a failed attach (RBAC, missing shell) or a
// non-zero shell exit is returned from Run so the callback can toast it.
func TestExecCommandRunPropagatesError(t *testing.T) {
	f := &fakeExecer{err: errors.New("forbidden")}
	c := newExecCommand(f, kube.ObjectRef{Name: "api-1"})
	c.SetStdin(&bytes.Buffer{})
	c.SetStdout(&bytes.Buffer{})
	if err := c.Run(); err == nil {
		t.Fatal("Run should propagate the executor's error")
	}
}

// TestExecDoneReportsResult proves a clean exit surfaces a neutral status notice and a
// failure surfaces a transient error toast (D74).
func TestExecDoneReportsResult(t *testing.T) {
	m := sizedWith(t)

	next, _ := m.handleExecDone(execDoneMsg{label: "Pod web/api-1"})
	got := next.(Model)
	if !got.status.HasNotice() || got.status.HasError() {
		t.Fatal("a clean exec exit should surface a neutral notice, no error")
	}

	next, _ = m.handleExecDone(execDoneMsg{label: "Pod web/api-1", err: errors.New("forbidden")})
	got = next.(Model)
	if !got.status.HasError() {
		t.Fatal("a failed exec should surface a status-bar error toast")
	}
}

// TestExecSizeQueueSeedThenEnd proves the size queue delivers the seeded initial size
// once, then returns nil after close (the session-end signal remotecommand waits on).
func TestExecSizeQueueSeedThenEnd(t *testing.T) {
	q := newExecSizeQueue()
	q.seed(80, 24)

	first := q.Next()
	if first == nil || first.Width != 80 || first.Height != 24 {
		t.Fatalf("first Next = %+v, want {80 24}", first)
	}
	q.close()
	if got := q.Next(); got != nil {
		t.Fatalf("after close Next = %+v, want nil (session end)", got)
	}
	q.close() // idempotent — a double teardown must not panic
}

// TestExecSizeQueueDropsDegenerateSeed proves a 0×0 seed is dropped (remotecommand
// then falls back to server defaults) rather than queued.
func TestExecSizeQueueDropsDegenerateSeed(t *testing.T) {
	q := newExecSizeQueue()
	q.seed(0, 0)
	q.close()
	if got := q.Next(); got != nil {
		t.Fatalf("a degenerate seed should be dropped, got %+v", got)
	}
}
