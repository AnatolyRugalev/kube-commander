package tui

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
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
	c := newExecCommand(f, ref, "")

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

// execFetch delivers the Exec-shell intent over the selected pod and returns the model
// plus the async container-fetch cmd openExec issues when a container lister is wired
// (M3-14b-2). A nil-lister model suspends directly instead (no fetch) — the M3-14b-1 path.
func execFetch(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	return dispatchRowAction(t, m, rowActionExec)
}

// resolveExecContainers runs the container-fetch cmd, asserting it produced a
// containersLoadedMsg carrying the exec purpose, and delivers it — returning the model
// plus any follow-on cmd (the tea.Exec suspend for a single container; nil once the
// picker is shown).
func resolveExecContainers(t *testing.T, m Model, fetchCmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	msg := fetchCmd()
	clm, ok := msg.(containersLoadedMsg)
	if !ok {
		t.Fatalf("opening exec with a lister produced %T, want containersLoadedMsg", msg)
	}
	if clm.purpose != ctrPurposeExec {
		t.Fatalf("the exec container fetch should carry the exec purpose, got %v", clm.purpose)
	}
	next, follow := m.Update(clm)
	return next.(Model), follow
}

// TestExecSingleContainerExecsDirectly proves a single-container pod skips the picker:
// resolving its one container suspends straight into an exec session (a non-nil
// tea.Exec cmd), with no picker shown.
func TestExecSingleContainerExecsDirectly(t *testing.T) {
	l := &fakeContainerLister{names: []string{"app"}}
	m := podExecModel(t, WithExecer(&fakeExecer{}), WithContainerLister(l))

	m, fetchCmd := execFetch(t, m)
	if fetchCmd == nil {
		t.Fatal("exec with a lister wired should issue the async container fetch")
	}
	if m.ctrPicker.Active() {
		t.Fatal("the picker should not open until the container resolves")
	}
	m, execCmd := resolveExecContainers(t, m, fetchCmd)
	if l.calls != 1 {
		t.Fatalf("PodContainers called %d times, want 1", l.calls)
	}
	if m.ctrPicker.Active() {
		t.Fatal("a single-container pod should not open the container picker")
	}
	if execCmd == nil {
		t.Fatal("a single-container pod should suspend directly into an exec session")
	}
}

// TestExecMultiContainerOpensPicker proves a multi-container pod prompts which container
// to exec into (the picker opens, stamped with the exec purpose) rather than execing
// blindly — and no exec starts before a pick.
func TestExecMultiContainerOpensPicker(t *testing.T) {
	f := &fakeExecer{}
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := podExecModel(t, WithExecer(f), WithContainerLister(l))

	m, fetchCmd := execFetch(t, m)
	m, follow := resolveExecContainers(t, m, fetchCmd)
	if !m.ctrPicker.Active() {
		t.Fatal("a multi-container pod should open the container picker for exec")
	}
	if m.ctrPurpose != ctrPurposeExec {
		t.Fatal("the open picker should carry the exec purpose so the pick routes to exec")
	}
	if follow != nil {
		t.Fatal("opening the picker issues no follow-on command")
	}
	if f.calls != 0 {
		t.Fatal("no exec should start before a container is picked")
	}
	if m.ctrPicker.Len() != 2 {
		t.Fatalf("the picker should list 2 containers, got %d", m.ctrPicker.Len())
	}
}

// TestExecContainerPickSuspendsIntoSession proves picking a container from an
// exec-purpose picker closes it and suspends into an exec session (a non-nil tea.Exec
// cmd) rather than opening the logs viewer.
func TestExecContainerPickSuspendsIntoSession(t *testing.T) {
	l := &fakeContainerLister{names: []string{"app", "sidecar"}}
	m := podExecModel(t, WithExecer(&fakeExecer{}), WithContainerLister(l))

	m, fetchCmd := execFetch(t, m)
	m, _ = resolveExecContainers(t, m, fetchCmd)

	next, execCmd := m.Update(picker.SelectedMsg{Kind: containerPickerKind, Value: "sidecar"})
	m = next.(Model)
	if m.ctrPicker.Active() {
		t.Fatal("picking a container should close the picker")
	}
	if m.viewer.Active() {
		t.Fatal("an exec pick must not open the logs viewer (wrong purpose route)")
	}
	if execCmd == nil {
		t.Fatal("picking a container should suspend into an exec session")
	}
}

// TestExecSkipsInitContainers proves the LOGS-06 set is narrowed by purpose: exec drops
// a pod's init containers (they have normally terminated, so there is no shell to open),
// so a pod with one regular container beside an init one still execs straight in — while
// an ephemeral debug container, whose whole point is being exec'd into, is offered.
func TestExecSkipsInitContainers(t *testing.T) {
	l := &fakeContainerLister{containers: []kube.Container{
		{Name: "app", Kind: kube.ContainerRegular},
		{Name: "wait-for-db", Kind: kube.ContainerInit},
	}}
	m := podExecModel(t, WithExecer(&fakeExecer{}), WithContainerLister(l))

	m, fetchCmd := execFetch(t, m)
	m, execCmd := resolveExecContainers(t, m, fetchCmd)
	if m.ctrPicker.Active() {
		t.Fatal("an init container should not make exec prompt: it is not exec'able")
	}
	if execCmd == nil {
		t.Fatal("the pod's one exec'able container should suspend directly into a session")
	}

	l2 := &fakeContainerLister{containers: []kube.Container{
		{Name: "app", Kind: kube.ContainerRegular},
		{Name: "debugger", Kind: kube.ContainerEphemeral},
	}}
	m2 := podExecModel(t, WithExecer(&fakeExecer{}), WithContainerLister(l2))
	m2, fetchCmd2 := execFetch(t, m2)
	m2, _ = resolveExecContainers(t, m2, fetchCmd2)
	if !m2.ctrPicker.Active() {
		t.Fatal("an ephemeral debug container should be offered to exec into")
	}
	if got, ok := m2.ctrByLabel["debugger (ephemeral)"]; !ok || got != "debugger" {
		t.Fatalf("the ephemeral container's row should map to debugger, got %q (rows: %v)", got, m2.ctrByLabel)
	}
}

// TestExecContainerResolveErrorDegrades proves a PodContainers failure on the exec path
// degrades to a status-bar error toast (labelled "exec"), opening no picker or session.
func TestExecContainerResolveErrorDegrades(t *testing.T) {
	l := &fakeContainerLister{err: errors.New("forbidden")}
	m := podExecModel(t, WithExecer(&fakeExecer{}), WithContainerLister(l))

	m, fetchCmd := execFetch(t, m)
	m, _ = resolveExecContainers(t, m, fetchCmd)
	if m.ctrPicker.Active() {
		t.Fatal("a resolve error should not open the picker")
	}
	if !m.status.HasError() {
		t.Fatal("a container-resolve failure should surface a status-bar error toast")
	}
}

// TestExecCommandRunUsesChosenContainer proves a picked (non-empty) container is passed
// through to kube.Exec — the last hop of the picker-reuse route (M3-14b-2).
func TestExecCommandRunUsesChosenContainer(t *testing.T) {
	f := &fakeExecer{}
	c := newExecCommand(f, kube.ObjectRef{Name: "api-1"}, "sidecar")
	c.SetStdin(&bytes.Buffer{})
	c.SetStdout(&bytes.Buffer{})
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if f.gotOpts.Container != "sidecar" {
		t.Fatalf("exec container = %q, want sidecar (the picked container)", f.gotOpts.Container)
	}
}

// TestExecCommandRunPropagatesError proves a failed attach (RBAC, missing shell) or a
// non-zero shell exit is returned from Run so the callback can toast it.
func TestExecCommandRunPropagatesError(t *testing.T) {
	f := &fakeExecer{err: errors.New("forbidden")}
	c := newExecCommand(f, kube.ObjectRef{Name: "api-1"}, "")
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

// forceKubectl overrides the lookupKubectl seam for a test and restores it after, so
// the exec parity fallback (M3-14b-4) is exercised deterministically without a real
// kubectl on the runner. present=false forces the in-process SPDY path.
func forceKubectl(t *testing.T, present bool) {
	t.Helper()
	old := lookupKubectl
	if present {
		lookupKubectl = func() (string, bool) { return "/usr/bin/kubectl", true }
	} else {
		lookupKubectl = func() (string, bool) { return "", false }
	}
	t.Cleanup(func() { lookupKubectl = old })
}

// TestKubectlExecProcWhenPresent proves that with kubectl on PATH the exec routes to a
// `kubectl exec -it` process (M3-14b-4) targeting the same cluster kubecom launched
// with — the resolved binary, --kubeconfig / --context, the row namespace, container,
// and `-- /bin/sh`.
func TestKubectlExecProcWhenPresent(t *testing.T) {
	forceKubectl(t, true)
	m := sizedWith(t, WithExecer(&fakeExecer{}), WithContext("prod"), WithKubeconfig("/home/u/.kube/config"))

	proc, ok := m.kubectlExecProc(kube.ObjectRef{Namespace: "web", Name: "api-1"}, "sidecar")
	if !ok {
		t.Fatal("with kubectl on PATH the exec should route to the kubectl process")
	}
	if proc.Path != "/usr/bin/kubectl" {
		t.Fatalf("process path = %q, want the resolved kubectl", proc.Path)
	}
	want := []string{
		"/usr/bin/kubectl",
		"--kubeconfig", "/home/u/.kube/config",
		"--context", "prod",
		"-n", "web",
		"exec", "-i", "-t", "api-1",
		"-c", "sidecar",
		"--", "/bin/sh",
	}
	if got := proc.Args; !slices.Equal(got, want) {
		t.Fatalf("kubectl argv =\n  %v\nwant\n  %v", got, want)
	}
}

// TestKubectlExecProcAbsent proves that with no kubectl on PATH kubectlExecProc reports
// no process, so execInto falls back to the in-process SPDY path.
func TestKubectlExecProcAbsent(t *testing.T) {
	forceKubectl(t, false)
	m := sizedWith(t, WithExecer(&fakeExecer{}))
	if _, ok := m.kubectlExecProc(kube.ObjectRef{Namespace: "web", Name: "api-1"}, ""); ok {
		t.Fatal("with no kubectl on PATH kubectlExecProc must report no process (use SPDY)")
	}
}

// TestKubectlExecArgsMinimal proves the connection flags and -c are omitted when unset:
// a cluster-less launch (no --kubeconfig/--context) execing a Pod's default container
// yields just `exec -i -t <pod> -- /bin/sh` (kubectl uses its standard resolution).
func TestKubectlExecArgsMinimal(t *testing.T) {
	got := kubectlExecArgs("", "", kube.ObjectRef{Name: "api-1"}, "")
	want := []string{"exec", "-i", "-t", "api-1", "--", "/bin/sh"}
	if !slices.Equal(got, want) {
		t.Fatalf("minimal argv = %v, want %v", got, want)
	}
}

// TestKubectlExecArgsNamespaceOnly proves a namespaced Pod adds -n but still omits the
// unset connection flags and container.
func TestKubectlExecArgsNamespaceOnly(t *testing.T) {
	got := kubectlExecArgs("", "", kube.ObjectRef{Namespace: "kube-system", Name: "dns-1"}, "")
	want := []string{"-n", "kube-system", "exec", "-i", "-t", "dns-1", "--", "/bin/sh"}
	if !slices.Equal(got, want) {
		t.Fatalf("namespaced argv = %v, want %v", got, want)
	}
}

// TestExecRoutesToKubectlWhenPresent proves the end-to-end intent still suspends (a
// non-nil command) when kubectl is on PATH — the kubectl branch of execInto — without
// ever calling the in-process execer.
func TestExecRoutesToKubectlWhenPresent(t *testing.T) {
	forceKubectl(t, true)
	f := &fakeExecer{}
	m := podExecModel(t, WithExecer(f))

	_, cmd := dispatchRowAction(t, m, rowActionExec)
	if cmd == nil {
		t.Fatal("with kubectl present the exec intent should still issue a suspend command")
	}
	if f.calls != 0 {
		t.Fatal("the kubectl fallback must not call the in-process execer")
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

// TestExecSizeQueuePushLatestWins proves a live resize (push) replaces the pending size
// so the newest window size always wins over a stale one queued but not yet read — the
// coalescing that keeps the remote PTY from lagging behind a burst of SIGWINCH events.
func TestExecSizeQueuePushLatestWins(t *testing.T) {
	q := newExecSizeQueue()
	q.push(100, 30)
	q.push(120, 40) // supersedes the unread 100×30
	got := q.Next()
	if got == nil || got.Width != 120 || got.Height != 40 {
		t.Fatalf("Next = %+v, want the latest {120 40}", got)
	}
	q.close()
}

// TestExecSizeQueuePushDropsDegenerate proves a 0×0 resize read is ignored rather than
// queued, leaving a prior good size intact (a bad GetSize must not clobber the PTY size).
func TestExecSizeQueuePushDropsDegenerate(t *testing.T) {
	q := newExecSizeQueue()
	q.seed(80, 24)
	q.push(0, 0) // a failed size read must not overwrite the good seeded size
	got := q.Next()
	if got == nil || got.Width != 80 || got.Height != 24 {
		t.Fatalf("Next = %+v, want the seeded {80 24} (degenerate push dropped)", got)
	}
	q.close()
}

// TestWatchResizeTracksSIGWINCH proves the resize watcher pushes the current terminal
// size into the queue on a SIGWINCH, so the remote PTY tracks the local window
// mid-session (M3-14b-3). It feeds a fake size reader and raises SIGWINCH in-process
// (no real terminal needed), then reads the delivered size; stop tears the watcher down
// so a later close never races a push.
func TestWatchResizeTracksSIGWINCH(t *testing.T) {
	q := newExecSizeQueue()
	sizeOf := func() (uint16, uint16, bool) { return 132, 43, true }
	stop := watchResize(q, sizeOf)
	defer stop()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("raising SIGWINCH: %v", err)
	}
	got := q.Next() // blocks until the watcher reads the new size and pushes it
	if got == nil || got.Width != 132 || got.Height != 43 {
		t.Fatalf("after SIGWINCH Next = %+v, want the resized {132 43}", got)
	}
}
