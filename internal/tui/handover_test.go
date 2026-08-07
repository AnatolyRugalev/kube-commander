package tui

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// fakeTermStderr records the fd-2 handovers in order, alongside whatever the wrapped
// command records, so a test can assert not just that both calls happened but that
// they bracketed the Run.
type fakeTermStderr struct {
	log        *[]string
	releaseErr error
	reclaimErr error
}

func (f fakeTermStderr) Release() error {
	*f.log = append(*f.log, "release")
	return f.releaseErr
}

func (f fakeTermStderr) Reclaim() error {
	*f.log = append(*f.log, "reclaim")
	return f.reclaimErr
}

// execStub is a tea.ExecCommand that notes when it ran (into the shared log) and what
// streams it was handed.
type execStub struct {
	log    *[]string
	err    error
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (e *execStub) SetStdin(r io.Reader)  { e.stdin = r }
func (e *execStub) SetStdout(w io.Writer) { e.stdout = w }
func (e *execStub) SetStderr(w io.Writer) { e.stderr = w }

func (e *execStub) Run() error {
	*e.log = append(*e.log, "run")
	return e.err
}

// TestHandoverBracketsTheRun is the ordering claim: fd 2 belongs to the terminal for
// exactly the length of the wrapped command, so an editor's or a login's complaint is
// seen, and nothing before or after it can paint over the panes.
func TestHandoverBracketsTheRun(t *testing.T) {
	var log []string
	inner := &execStub{log: &log}
	h := handover{inner: inner, term: fakeTermStderr{log: &log}}
	if err := h.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if got := strings.Join(log, ","); got != "release,run,reclaim" {
		t.Fatalf("handover order = %q, want release,run,reclaim", got)
	}
}

// A failed command must still give fd 2 back — the terminal is already the TUI's
// again by then, so leaving the descriptor on it reopens the hole AUTH-07 closes.
func TestHandoverReclaimsAfterAFailedRun(t *testing.T) {
	var log []string
	inner := &execStub{log: &log, err: errors.New("editor exited 1")}
	h := handover{inner: inner, term: fakeTermStderr{log: &log}}
	if err := h.Run(); err == nil {
		t.Fatal("the wrapped command's error must reach the callback")
	}
	if got := strings.Join(log, ","); got != "release,run,reclaim" {
		t.Fatalf("handover order = %q, want the reclaim to survive the failure", got)
	}
}

// A Release that fails does not abort the suspend: the subprocess still runs, its
// stderr merely lands where it was already going. Refusing an approved `aws sso login`
// over a failed dup2 would trade a cosmetic fault for a functional one.
func TestHandoverRunsEvenIfTheDescriptorWillNotMove(t *testing.T) {
	var log []string
	inner := &execStub{log: &log}
	h := handover{inner: inner, term: fakeTermStderr{log: &log, releaseErr: errors.New("dup2: bad fd"), reclaimErr: errors.New("dup2: bad fd")}}
	if err := h.Run(); err != nil {
		t.Fatalf("Run returned %v, want the command to run regardless", err)
	}
	if got := strings.Join(log, ","); got != "release,run,reclaim" {
		t.Fatalf("handover order = %q, want the run to happen anyway", got)
	}
}

// Unwired (every hermetic test, and a launch whose redirect could not be installed)
// the wrapper is a pass-through rather than a nil dereference.
func TestHandoverWithoutAGuardIsAPassThrough(t *testing.T) {
	var log []string
	inner := &execStub{log: &log}
	h := handover{inner: inner}
	if err := h.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if got := strings.Join(log, ","); got != "run" {
		t.Fatalf("handover order = %q, want just the run", got)
	}
	// The streams still reach the wrapped command — the wrapper adds a descriptor
	// swap, it must not swallow what bubbletea hands over.
	var in bytes.Buffer
	var out bytes.Buffer
	h.SetStdin(&in)
	h.SetStdout(&out)
	h.SetStderr(&out)
	if inner.stdin == nil || inner.stdout == nil || inner.stderr == nil {
		t.Fatal("the wrapper must pass all three streams through")
	}
}

// TestProcCommandFillsOnlyUnsetStreams pins the adapter to bubbletea's own contract:
// a command that already chose a stream keeps it, an unset one gets the program's.
// The kubectl exec path relies on both halves.
func TestProcCommandFillsOnlyUnsetStreams(t *testing.T) {
	var chosen bytes.Buffer
	proc := exec.Command("true")
	proc.Stdout = &chosen
	c := procCommand{Cmd: proc}

	var in, out bytes.Buffer
	c.SetStdin(&in)
	c.SetStdout(&out)
	c.SetStderr(&out)

	if proc.Stdout != (&chosen) { //nolint:staticcheck // identity is the assertion
		t.Fatal("a stream the caller chose must not be replaced")
	}
	if proc.Stdin == nil || proc.Stderr == nil {
		t.Fatal("the unset streams should have been filled from the program's")
	}
}

// suspendedCommandType reports the concrete type bubbletea would be handed by a
// tea.Exec command. It reads the unexported field of tea's internal exec message by
// reflection — deliberately: the whole point of AUTH-07's TUI half is that every
// suspend is wrapped, and nothing else in the package can observe that a call site
// used tea.Exec directly. Type inspection is allowed on unexported fields (only
// reading their value is not), so this needs no unsafe.
func suspendedCommandType(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a suspend command, got nil")
	}
	v := reflect.ValueOf(cmd())
	if v.Kind() != reflect.Struct {
		t.Fatalf("expected bubbletea's exec message, got %T", v.Interface())
	}
	f := v.FieldByName("cmd")
	if !f.IsValid() || f.IsNil() {
		t.Fatalf("expected bubbletea's exec message to carry a command, got %s", v.Type())
	}
	return f.Elem().Type().String()
}

// TestEverySuspendGoesThroughTheHandover is the wiring guard. Each of the three flows
// that hand the terminal over — $EDITOR, exec (both the kubectl parity path and the
// in-process one), an approved re-login — must route through Model.suspend, or that
// subprocess's stderr silently lands in the log file where the reader cannot see it.
// Adding a fourth suspend that calls tea.Exec directly is not a compile error, so it
// is caught here.
func TestEverySuspendGoesThroughTheHandover(t *testing.T) {
	const want = "tui.handover"

	t.Run("edit", func(t *testing.T) {
		m := podEditModel(t, WithEditor(&fakeEditor{}), WithYAMLGetter(&fakeYAMLGetter{yaml: "kind: Pod\n"}))
		_, cmd := m.handleEditFetched(editFetchedMsg{
			res: m.current, ref: kube.ObjectRef{Namespace: "default", Name: "web-1"}, content: "kind: Pod\n",
		})
		if got := suspendedCommandType(t, cmd); got != want {
			t.Fatalf("$EDITOR suspends as %s, want %s", got, want)
		}
	})

	t.Run("exec in-process", func(t *testing.T) {
		forceKubectl(t, false)
		m := podExecModel(t, WithExecer(&fakeExecer{}))
		_, cmd := dispatchRowAction(t, m, rowActionExec)
		if got := suspendedCommandType(t, cmd); got != want {
			t.Fatalf("the in-process exec suspends as %s, want %s", got, want)
		}
	})

	t.Run("exec via kubectl", func(t *testing.T) {
		forceKubectl(t, true)
		m := podExecModel(t, WithExecer(&fakeExecer{}))
		_, cmd := dispatchRowAction(t, m, rowActionExec)
		if got := suspendedCommandType(t, cmd); got != want {
			t.Fatalf("the kubectl exec suspends as %s, want %s", got, want)
		}
	})

	t.Run("reauth", func(t *testing.T) {
		m, _ := armedPods(t)
		_, cmd := m.runReauth()
		if got := suspendedCommandType(t, cmd); got != want {
			t.Fatalf("the remediation suspends as %s, want %s", got, want)
		}
	})
}
