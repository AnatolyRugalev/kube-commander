package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// ForwardedPort is one local:remote port pairing of a running port-forward,
// decoupled from client-go's portforward.ForwardedPort so the TUI never imports
// client-go's tooling types (the same apimachinery-free boundary the Table layer
// keeps, D33). Local is the address the user connects to on localhost; Remote is
// the container port it reaches. When a forward is requested with local port 0
// (":<remote>"), Local carries the OS-assigned port, readable once Ready fires.
type ForwardedPort struct {
	Local  uint16
	Remote uint16
}

// PortForward is a handle to a background port-forward to a pod — the in-process
// equivalent of `kubectl port-forward`, over an SPDY-upgraded connection to the
// pod's portforward subresource, so no kubectl binary is needed (D2). Forwarding
// runs in a goroutine owned by this handle; the caller observes it through the
// channels and stops it with Stop (or by cancelling the ctx passed to
// PortForward). It never blocks the UI: PortForward returns immediately and the
// dial happens on the goroutine.
//
// Lifecycle: PortForward returns a live handle; Ready closes once the local
// listeners are established (after which Ports reports the bound ports); Done
// closes when forwarding ends — on Stop, ctx cancellation, or a fatal transport
// error, the last of which is then readable via Err. The zero PortForward is not
// usable; obtain one from Clients.PortForward.
type PortForward struct {
	fwd portForwarder

	// stopCh is closed by Stop to tell the forwarder to shut its listeners; readyCh
	// is closed by the forwarder once forwarding is established; doneCh is closed by
	// this handle's goroutine when ForwardPorts returns.
	stopCh   chan struct{}
	readyCh  chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once

	// err holds a fatal forwarding error. It is written once, before doneCh is
	// closed, and read only after doneCh is observed closed — that happens-before
	// makes it race-free without a mutex (principle 1: no mutex-guarded shared
	// state; the goroutine hands the result off through the channel close).
	err error
}

// portForwarder is the minimal behaviour PortForward drives, so its lifecycle can
// be exercised by a fake in hermetic tests (D18). *portforward.PortForwarder
// satisfies it. The real forwarder dials the API server (network), so an
// end-to-end forward is envtest territory; start/stop, ready, port readback, and
// error propagation are all covered without a cluster.
type portForwarder interface {
	// ForwardPorts blocks, serving forwarded connections, until the stop channel is
	// closed (returns nil) or a fatal error occurs (returns it).
	ForwardPorts() error
	// GetPorts returns the bound local:remote pairs; it is meaningful only after
	// forwarding is ready.
	GetPorts() ([]portforward.ForwardedPort, error)
}

// forwarderFactory builds a portForwarder wired to the given stop and ready
// channels. Injecting it lets newPortForward be tested with a fake forwarder while
// the exported PortForward supplies the real SPDY-backed one.
type forwarderFactory func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error)

// PortForward starts forwarding local ports to a pod in the background and returns
// a handle to observe and stop it. ports uses kubectl's syntax: "8080:80" (local
// 8080 → remote 80), "80" (same port both ends), or ":80" (an OS-assigned local
// port → remote 80). Forwarding runs until Stop is called, ctx is cancelled, or a
// fatal transport error ends it; none of this blocks first paint — the dial
// happens on the handle's goroutine (principle 4). An empty pod name or an empty
// port list is rejected, as is a malformed port spec, all before the handle is
// returned; errors are wrapped, never panicked (#86).
func (c *Clients) PortForward(ctx context.Context, ref ObjectRef, ports []string) (*PortForward, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: port-forward: empty pod name")
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("kube: port-forward: no ports given")
	}
	factory := func(stopCh <-chan struct{}, readyCh chan struct{}) (portForwarder, error) {
		dialer, err := c.portForwardDialer(ref)
		if err != nil {
			return nil, err
		}
		// out/errOut carry kubectl's "Forwarding from ..." lines and per-connection
		// forwarding errors; the TUI reads the bound ports via Ports instead and
		// surfaces fatal failures via Err, so both are discarded here.
		return portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	}
	return newPortForward(ctx, factory)
}

// portForwardDialer builds the SPDY dialer for a POST to the pod's portforward
// subresource — exactly what `kubectl port-forward` upgrades. It reuses the
// retained *rest.Config (the transport source the Clients doc reserves for this).
func (c *Clients) portForwardDialer(ref ObjectRef) (httpstream.Dialer, error) {
	transport, upgrader, err := spdy.RoundTripperFor(c.Config)
	if err != nil {
		return nil, fmt.Errorf("kube: building port-forward transport: %w", err)
	}
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(ref.Namespace).
		Name(ref.Name).
		SubResource("portforward")
	return spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL()), nil
}

// newPortForward builds the forwarder from factory, starts it on a background
// goroutine, and bridges ctx cancellation to Stop. It is separated from
// PortForward so tests can drive the lifecycle with a fake factory. A factory
// error (bad transport, malformed port spec) is returned directly — no handle,
// no goroutine.
func newPortForward(ctx context.Context, factory forwarderFactory) (*PortForward, error) {
	pf := &PortForward{
		stopCh:  make(chan struct{}),
		readyCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	fwd, err := factory(pf.stopCh, pf.readyCh)
	if err != nil {
		return nil, err
	}
	pf.fwd = fwd

	go func() {
		defer close(pf.doneCh)
		if err := fwd.ForwardPorts(); err != nil {
			pf.err = err
		}
	}()

	// Cancelling ctx stops the forward, mirroring Logs/Watch; the goroutine exits on
	// its own once forwarding ends so it never outlives the handle.
	go func() {
		select {
		case <-ctx.Done():
			pf.Stop()
		case <-pf.doneCh:
		}
	}()

	return pf, nil
}

// Ready returns a channel closed once forwarding is established and the local
// listeners are accepting connections. After it fires, Ports reports the bound
// ports. If forwarding fails before it is ever ready, Done closes without Ready.
func (pf *PortForward) Ready() <-chan struct{} { return pf.readyCh }

// Done returns a channel closed when forwarding has ended — after Stop, ctx
// cancellation, or a fatal error. Once it is closed, Err reports the cause (nil
// for a clean stop).
func (pf *PortForward) Done() <-chan struct{} { return pf.doneCh }

// Err returns the fatal error that ended forwarding, or nil if it was stopped
// cleanly. It is meaningful only after Done is closed; calling it earlier returns
// nil.
func (pf *PortForward) Err() error {
	select {
	case <-pf.doneCh:
		return pf.err
	default:
		return nil
	}
}

// Ports returns the forward's bound local:remote pairs. It is meaningful only
// after Ready has fired (the OS assigns any local port 0 during startup); called
// earlier it surfaces the forwarder's not-ready error.
func (pf *PortForward) Ports() ([]ForwardedPort, error) {
	fp, err := pf.fwd.GetPorts()
	if err != nil {
		return nil, fmt.Errorf("kube: reading forwarded ports: %w", err)
	}
	out := make([]ForwardedPort, len(fp))
	for i, p := range fp {
		out[i] = ForwardedPort{Local: p.Local, Remote: p.Remote}
	}
	return out, nil
}

// Stop ends forwarding and closes the local listeners. It is idempotent and safe
// to call from any goroutine; after it returns, Done closes once the forwarder has
// unwound.
func (pf *PortForward) Stop() {
	pf.stopOnce.Do(func() { close(pf.stopCh) })
}
