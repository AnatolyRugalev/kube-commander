package kube

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"k8s.io/client-go/tools/remotecommand"
)

// fakeExecutor is a hermetic stand-in for remotecommand.Executor: it never
// touches the network, records the StreamOptions it was handed, and returns a
// canned error — enough to drive runExec's success and failure paths and to
// assert the option mapping without a cluster (D18). The real SPDY-backed exec is
// envtest / live-cluster territory.
type fakeExecutor struct {
	gotOptions remotecommand.StreamOptions
	streamErr  error
}

func (f *fakeExecutor) StreamWithContext(_ context.Context, options remotecommand.StreamOptions) error {
	f.gotOptions = options
	return f.streamErr
}

func TestExecRejectsBadArgs(t *testing.T) {
	c := &Clients{}
	if err := c.Exec(context.Background(), ObjectRef{Namespace: "default"}, ExecOptions{Command: []string{"/bin/sh"}}); err == nil {
		t.Error("Exec with empty pod name: want error, got nil")
	}
	if err := c.Exec(context.Background(), ObjectRef{Name: "pod"}, ExecOptions{}); err == nil {
		t.Error("Exec with no command: want error, got nil")
	}
}

func TestRunExecSuccess(t *testing.T) {
	fake := &fakeExecutor{}
	factory := func() (streamExecutor, error) { return fake, nil }
	if err := runExec(context.Background(), factory, ExecOptions{Command: []string{"/bin/sh"}}); err != nil {
		t.Fatalf("runExec: %v", err)
	}
}

func TestRunExecStreamErrorWrapped(t *testing.T) {
	sentinel := errors.New("command terminated with exit code 1")
	factory := func() (streamExecutor, error) { return &fakeExecutor{streamErr: sentinel}, nil }
	err := runExec(context.Background(), factory, ExecOptions{Command: []string{"false"}})
	if !errors.Is(err, sentinel) {
		t.Errorf("runExec err = %v, want wrapped %v", err, sentinel)
	}
}

func TestRunExecFactoryError(t *testing.T) {
	sentinel := errors.New("bad transport")
	factory := func() (streamExecutor, error) { return nil, sentinel }
	if err := runExec(context.Background(), factory, ExecOptions{Command: []string{"/bin/sh"}}); !errors.Is(err, sentinel) {
		t.Errorf("runExec err = %v, want %v", err, sentinel)
	}
}

func TestExecStreamOptionsNonTTYKeepsStderr(t *testing.T) {
	var in bytes.Buffer
	var out, errOut bytes.Buffer
	so := execStreamOptions(ExecOptions{
		Command: []string{"cat"},
		Stdin:   &in,
		Stdout:  &out,
		Stderr:  &errOut,
	})
	if so.Tty {
		t.Error("Tty = true, want false")
	}
	if so.Stdin == nil || so.Stdout == nil || so.Stderr == nil {
		t.Errorf("streams: stdin=%v stdout=%v stderr=%v, want all attached", so.Stdin, so.Stdout, so.Stderr)
	}
	if so.TerminalSizeQueue != nil {
		t.Error("TerminalSizeQueue set without a TTY, want nil")
	}
}

func TestExecStreamOptionsTTYDropsStderrAndWiresSizeQueue(t *testing.T) {
	var errOut bytes.Buffer
	q := &sliceSizeQueue{sizes: []*TerminalSize{{Width: 80, Height: 24}, nil}}
	so := execStreamOptions(ExecOptions{
		Command:   []string{"/bin/sh"},
		TTY:       true,
		Stderr:    &errOut,
		SizeQueue: q,
	})
	if !so.Tty {
		t.Error("Tty = false, want true")
	}
	if so.Stderr != nil {
		t.Error("Stderr set with a TTY, want nil (stderr is multiplexed into stdout)")
	}
	if so.TerminalSizeQueue == nil {
		t.Fatal("TerminalSizeQueue = nil with a TTY + queue, want the adapter")
	}
	// The adapter must translate kubecom's TerminalSize to client-go's, then pass
	// the nil close signal through.
	if got := so.TerminalSizeQueue.Next(); got == nil || got.Width != 80 || got.Height != 24 {
		t.Errorf("Next() = %+v, want {80 24}", got)
	}
	if got := so.TerminalSizeQueue.Next(); got != nil {
		t.Errorf("Next() after end = %+v, want nil", got)
	}
}

func TestExecStreamOptionsTTYWithoutQueueLeavesItNil(t *testing.T) {
	so := execStreamOptions(ExecOptions{Command: []string{"/bin/sh"}, TTY: true})
	if so.TerminalSizeQueue != nil {
		t.Error("TerminalSizeQueue set without a queue, want nil")
	}
}

// sliceSizeQueue is a test TerminalSizeQueue that yields a fixed sequence of
// resizes and then nil (session end).
type sliceSizeQueue struct{ sizes []*TerminalSize }

func (q *sliceSizeQueue) Next() *TerminalSize {
	if len(q.sizes) == 0 {
		return nil
	}
	s := q.sizes[0]
	q.sizes = q.sizes[1:]
	return s
}
