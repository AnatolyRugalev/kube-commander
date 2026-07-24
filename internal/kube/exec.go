package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// TerminalSize is one resize event for an interactive exec — the terminal's
// width and height in cells. It is decoupled from remotecommand.TerminalSize so
// the TUI never imports client-go's tooling types (the same apimachinery-free
// boundary the Table and PortForward layers keep, D33): the TUI builds these
// from its own window-size messages and this package adapts them to client-go on
// the way to the wire.
type TerminalSize struct {
	Width  uint16
	Height uint16
}

// TerminalSizeQueue feeds terminal resize events to an interactive exec so the
// remote PTY tracks the local window. Next blocks until the next resize and
// returns it, or returns nil once the session ends (the queue is closed) — the
// same contract as remotecommand.TerminalSizeQueue but expressed in kubecom's
// own types. It is only consulted for a TTY exec; a non-TTY exec ignores it.
type TerminalSizeQueue interface {
	Next() *TerminalSize
}

// ExecOptions selects which container to exec into and how to wire the streams,
// mirroring the flags of `kubectl exec`. Command is the argv to run (e.g.
// {"/bin/sh"} for a shell); an empty Command is rejected. Stdin/Stdout/Stderr
// are the streams to attach — a nil stream is simply not attached (the API's
// PodExecOptions.Stdin/Stdout/Stderr flags follow which are non-nil). TTY
// requests a pseudo-terminal, which the exec shell needs for line editing and
// job control; with a TTY the server multiplexes stderr into stdout, so Stderr
// is not attached separately and SizeQueue (if set) drives PTY resizes.
type ExecOptions struct {
	// Container names the container to exec into. Empty selects the pod's default
	// container (its sole container, or the default-container annotation), matching
	// `kubectl exec` with no -c.
	Container string
	// Command is the argv executed in the container. It must be non-empty.
	Command []string
	// TTY requests an interactive pseudo-terminal (`kubectl exec -t`). A shell needs
	// it; a one-shot command usually does not. With a TTY, Stderr is folded into
	// Stdout on the wire and SizeQueue governs resizes.
	TTY bool
	// Stdin, when non-nil, is attached as the container's standard input
	// (`kubectl exec -i`) — required for an interactive shell.
	Stdin io.Reader
	// Stdout, when non-nil, receives the container's standard output.
	Stdout io.Writer
	// Stderr, when non-nil, receives the container's standard error. It is ignored
	// when TTY is set (a TTY has no separate stderr stream).
	Stderr io.Writer
	// SizeQueue, when non-nil and TTY is set, supplies terminal resize events so the
	// remote PTY follows the local window. Ignored without a TTY.
	SizeQueue TerminalSizeQueue
}

// streamExecutor is the minimal behaviour Exec drives, so the streaming call can
// be exercised by a fake in hermetic tests (D18). remotecommand.Executor (built
// by NewSPDYExecutor) satisfies it. The real executor dials the API server
// (network) and negotiates an SPDY-upgraded exec stream, so an end-to-end exec is
// envtest / live-cluster territory; argument validation, option mapping, the
// size-queue adaptation, and error propagation are all covered without a cluster.
type streamExecutor interface {
	StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error
}

// execFactory builds the executor for a single exec. Injecting it lets runExec be
// tested with a fake executor while the exported Exec supplies the real
// SPDY-backed one, mirroring PortForward's forwarderFactory.
type execFactory func() (streamExecutor, error)

// Exec runs a command in a pod's container and streams its I/O to the caller's
// streams until the command exits or ctx is cancelled — the in-process equivalent
// of `kubectl exec`, over an SPDY-upgraded connection to the pod's exec
// subresource, so no kubectl binary is needed (D2). It BLOCKS for the lifetime of
// the exec, so the TUI drives it from a suspended terminal (via tea.Exec), never
// on the Bubble Tea update loop. A clean exit returns nil; a non-zero command
// exit or a transport failure returns a wrapped error (the underlying
// exec.CodeExitError is preserved in the chain so a caller can read the exit
// code). An empty pod name or empty command is rejected before any dial; errors
// are wrapped, never panicked (#86). Linux/macOS only — the raw-PTY path is a
// non-goal on native Windows (WSL2 instead, D7).
func (c *Clients) Exec(ctx context.Context, ref ObjectRef, opts ExecOptions) error {
	if ref.Name == "" {
		return fmt.Errorf("kube: exec: empty pod name")
	}
	if len(opts.Command) == 0 {
		return fmt.Errorf("kube: exec: no command given")
	}
	factory := func() (streamExecutor, error) {
		req := c.execRequest(ref, opts)
		exec, err := remotecommand.NewSPDYExecutor(c.Config, http.MethodPost, req.URL())
		if err != nil {
			return nil, fmt.Errorf("kube: building exec executor: %w", err)
		}
		return exec, nil
	}
	return runExec(ctx, factory, opts)
}

// execRequest builds the POST to the pod's exec subresource — exactly what
// `kubectl exec` upgrades. PodExecOptions is a built-in core/v1 type, so its
// params are encoded with scheme.ParameterCodec (unlike the CRD-safe
// metav1.ParameterCodec the Table layer needs for arbitrary groups, D103). The
// Std* flags follow which streams the caller attached; with a TTY, stderr is not
// a separate stream, so its flag is forced off to match the wire.
func (c *Clients) execRequest(ref ObjectRef, opts ExecOptions) *rest.Request {
	return c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(ref.Namespace).
		Name(ref.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil && !opts.TTY,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)
}

// runExec builds the executor from factory and streams it to completion. It is
// separated from Exec so tests can drive it with a fake executor; a factory error
// (bad transport) is returned directly, and a stream error (non-zero exit, dropped
// connection) is wrapped. A clean exit returns nil.
func runExec(ctx context.Context, factory execFactory, opts ExecOptions) error {
	exec, err := factory()
	if err != nil {
		return err
	}
	if err := exec.StreamWithContext(ctx, execStreamOptions(opts)); err != nil {
		return fmt.Errorf("kube: exec stream: %w", err)
	}
	return nil
}

// execStreamOptions maps kubecom's ExecOptions onto client-go's
// remotecommand.StreamOptions. Pure, so the mapping is unit-tested without a
// cluster. With a TTY, Stderr is dropped (the server multiplexes it into Stdout —
// StreamWithContext rejects a TTY exec that also sets Stderr) and the caller's
// TerminalSizeQueue, if any, is wrapped in the adapter that translates kubecom's
// TerminalSize to client-go's.
func execStreamOptions(opts ExecOptions) remotecommand.StreamOptions {
	so := remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    opts.TTY,
	}
	if opts.TTY {
		so.Stderr = nil
		if opts.SizeQueue != nil {
			so.TerminalSizeQueue = &sizeQueueAdapter{q: opts.SizeQueue}
		}
	}
	return so
}

// sizeQueueAdapter bridges kubecom's TerminalSizeQueue to
// remotecommand.TerminalSizeQueue, translating each TerminalSize to client-go's
// type and passing the nil-terminated close signal through unchanged. This keeps
// the exec's public surface free of client-go tooling types (D33).
type sizeQueueAdapter struct{ q TerminalSizeQueue }

// Next returns the next resize as client-go's TerminalSize, or nil when the
// wrapped queue signals the session has ended.
func (a *sizeQueueAdapter) Next() *remotecommand.TerminalSize {
	s := a.q.Next()
	if s == nil {
		return nil
	}
	return &remotecommand.TerminalSize{Width: s.Width, Height: s.Height}
}
