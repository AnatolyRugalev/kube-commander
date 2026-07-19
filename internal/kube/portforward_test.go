package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/client-go/tools/portforward"
)

// fakeForwarder is a hermetic stand-in for *portforward.PortForwarder: it never
// touches the network, closes the ready channel when it "starts", and unblocks on
// the stop channel — enough to drive PortForward's whole lifecycle (D18). The real
// SPDY-backed forward is envtest territory.
type fakeForwarder struct {
	ready chan struct{}
	stop  <-chan struct{}

	ports    []portforward.ForwardedPort
	portsErr error

	// fwdErr is returned by ForwardPorts. returnNow makes it return that error
	// straight away (a fatal dial error); otherwise ForwardPorts marks ready and
	// blocks until stop closes, then returns fwdErr (nil = clean stop).
	fwdErr    error
	returnNow bool
}

func (f *fakeForwarder) ForwardPorts() error {
	if f.returnNow {
		return f.fwdErr
	}
	if f.ready != nil {
		close(f.ready)
	}
	<-f.stop
	return f.fwdErr
}

func (f *fakeForwarder) GetPorts() ([]portforward.ForwardedPort, error) {
	return f.ports, f.portsErr
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestPortForwardReadyStopAndPorts(t *testing.T) {
	fake := &fakeForwarder{
		ports: []portforward.ForwardedPort{
			{Local: 8080, Remote: 80},
			{Local: 55000, Remote: 443}, // an OS-assigned local port (requested ":443")
		},
	}
	factory := func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error) {
		fake.ready, fake.stop = readyCh, stopCh
		return fake, nil
	}

	pf, err := newPortForward(context.Background(), factory)
	if err != nil {
		t.Fatalf("newPortForward: %v", err)
	}

	waitClosed(t, pf.Ready(), "ready")

	got, err := pf.Ports()
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	want := []ForwardedPort{{Local: 8080, Remote: 80}, {Local: 55000, Remote: 443}}
	if len(got) != len(want) {
		t.Fatalf("Ports len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Ports[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Not yet stopped: Err must be nil (Done not closed).
	if err := pf.Err(); err != nil {
		t.Fatalf("Err before Stop = %v, want nil", err)
	}

	pf.Stop()
	waitClosed(t, pf.Done(), "done after Stop")
	if err := pf.Err(); err != nil {
		t.Errorf("Err after clean Stop = %v, want nil", err)
	}
}

func TestPortForwardFatalError(t *testing.T) {
	sentinel := errors.New("lost connection to pod")
	factory := func(_ <-chan struct{}, _ chan struct{}) (portForwarder, error) {
		return &fakeForwarder{returnNow: true, fwdErr: sentinel}, nil
	}

	pf, err := newPortForward(context.Background(), factory)
	if err != nil {
		t.Fatalf("newPortForward: %v", err)
	}

	waitClosed(t, pf.Done(), "done after fatal error")
	if got := pf.Err(); !errors.Is(got, sentinel) {
		t.Errorf("Err = %v, want %v", got, sentinel)
	}
}

func TestPortForwardCtxCancelStops(t *testing.T) {
	fake := &fakeForwarder{}
	factory := func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error) {
		fake.ready, fake.stop = readyCh, stopCh
		return fake, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	pf, err := newPortForward(ctx, factory)
	if err != nil {
		t.Fatalf("newPortForward: %v", err)
	}
	waitClosed(t, pf.Ready(), "ready")

	cancel() // ctx cancellation must stop the forward via Stop
	waitClosed(t, pf.Done(), "done after ctx cancel")
	if err := pf.Err(); err != nil {
		t.Errorf("Err after ctx-cancel stop = %v, want nil", err)
	}
}

func TestPortForwardFactoryError(t *testing.T) {
	sentinel := errors.New("bad transport")
	factory := func(_ <-chan struct{}, _ chan struct{}) (portForwarder, error) {
		return nil, sentinel
	}

	pf, err := newPortForward(context.Background(), factory)
	if !errors.Is(err, sentinel) {
		t.Errorf("newPortForward err = %v, want %v", err, sentinel)
	}
	if pf != nil {
		t.Errorf("handle = %v, want nil on factory error", pf)
	}
}

func TestPortForwardStopIdempotent(t *testing.T) {
	fake := &fakeForwarder{}
	factory := func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error) {
		fake.ready, fake.stop = readyCh, stopCh
		return fake, nil
	}
	pf, err := newPortForward(context.Background(), factory)
	if err != nil {
		t.Fatalf("newPortForward: %v", err)
	}
	waitClosed(t, pf.Ready(), "ready")

	pf.Stop()
	pf.Stop() // second call must not panic (double-close guard)
	waitClosed(t, pf.Done(), "done")
}

func TestPortForwardPortsError(t *testing.T) {
	sentinel := errors.New("not ready")
	fake := &fakeForwarder{portsErr: sentinel}
	factory := func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error) {
		fake.ready, fake.stop = readyCh, stopCh
		return fake, nil
	}
	pf, err := newPortForward(context.Background(), factory)
	if err != nil {
		t.Fatalf("newPortForward: %v", err)
	}
	defer pf.Stop()

	if _, err := pf.Ports(); !errors.Is(err, sentinel) {
		t.Errorf("Ports err = %v, want wrapped %v", err, sentinel)
	}
}

func TestPortForwardRejectsBadArgs(t *testing.T) {
	c := &Clients{}
	if _, err := c.PortForward(context.Background(), ObjectRef{Namespace: "default"}, []string{"8080:80"}); err == nil {
		t.Error("PortForward with empty pod name: want error, got nil")
	}
	if _, err := c.PortForward(context.Background(), ObjectRef{Name: "pod"}, nil); err == nil {
		t.Error("PortForward with no ports: want error, got nil")
	}
}
