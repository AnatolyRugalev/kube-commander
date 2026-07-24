package tui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
)

// This file is the FB-pf-port-picker-b wire: the TUI surface that turns the
// port-forward flow's free-text remote-port entry into a *choice* of the ports the
// selected object actually declares. The kube layer already has the primitive —
// Clients.PodPorts / Clients.ServicePorts (FB-pf-port-picker-a/D137), which read the
// declared containerPorts (and a Service's targetPort mapping) — so the shell's job
// is only to resolve them off the update loop and route the result.
//
// The flow mirrors the logs/exec container resolution (M3-07a/M3-14b-2): resolve
// asynchronously, then a single result is used directly and several open the shared
// modal picker. Declared ports are an *affordance*, never a gate (D137): an empty
// list, a listing error, or no lister wired all fall back to the free-text ports
// prompt the flow has taken since M3-13a (principle 3) — a container listening on a
// port it never declared is perfectly normal, so the picker must never be the only
// way to name a port.

// PortLister is the narrow slice of the kube layer the shell needs to offer the
// declared ports of a port-forward target as choices (FB-pf-port-picker-b): a pod's
// declared containerPorts, or a Service's ports resolved to the pod-side number a
// forward must target. *kube.Clients satisfies it via PodPorts/ServicePorts (D137).
// Both live on one seam because the two are the same question asked of the two kinds
// the Port-forward action applies to, and the flow picks between them by kind.
//
// Without it wired the Port-forward action keeps its M3-13a behaviour — the free-text
// ports prompt — so the pre-wiring app and the hermetic tests that predate the picker
// stay unchanged.
type PortLister interface {
	PodPorts(ctx context.Context, ref kube.ObjectRef) ([]kube.Port, error)
	ServicePorts(ctx context.Context, svcRef, podRef kube.ObjectRef) ([]kube.Port, error)
}

// WithPortLister wires the kube client the shell uses to list a port-forward target's
// declared ports, so the flow offers them as a picker instead of asking the user to
// remember a container port (FB-pf-port-picker-b). Without it the Port-forward action
// opens the free-text ports prompt directly (the M3-13a behaviour).
func WithPortLister(l PortLister) Option {
	return func(m *Model) { m.portLister = l }
}

// portPickerKind is the Kind stamped on the port picker (picker.New(s, "port")).
// Every picker emits the same SelectedMsg/CancelledMsg types (D65), so the root
// branches on this Kind to route a picked port into the forward rather than the
// namespace/resource/action/container paths.
const portPickerKind = "port"

// portsLoadedMsg carries the outcome of the async declared-ports listing issued when
// the Port-forward action is invoked with a port lister wired. gen ties it to the
// pfResolveGen bumped when the listing was requested — the same generation guarding
// the Service→pod resolution — so a result that lands after a newer port-forward
// request superseded it is dropped rather than picking for the wrong object. res/ref
// are the pod the listing was for, threaded back so the single-port fast path, the
// picker, and the free-text fallback all act on it.
type portsLoadedMsg struct {
	gen   int
	res   kube.Resource
	ref   kube.ObjectRef
	ports []kube.Port
	err   error
}

// resolvePortsFor starts the declared-ports resolution for a port-forward over podRef
// (a pod of res — a Pod row directly, or the endpoint pod a Service resolved to).
// svcRef names the Service the forward was invoked on, so its ports are read through
// ServicePorts (which resolves each targetPort to the pod-side number a forward must
// target); it is the zero ObjectRef for a Pod row, which reads PodPorts instead.
//
// With no lister wired it opens the free-text ports prompt directly (the M3-13a
// behaviour). Otherwise the listing runs off the update loop, stamped with a fresh
// pfResolveGen so a superseded request is dropped (handlePortsLoaded).
func (m Model) resolvePortsFor(res kube.Resource, podRef, svcRef kube.ObjectRef) (tea.Model, tea.Cmd) {
	if m.portLister == nil {
		return m.showPortForwardPrompt(res, podRef)
	}
	m.pfResolveGen++
	gen := m.pfResolveGen
	lister := m.portLister
	return m, func() tea.Msg {
		var (
			ports []kube.Port
			err   error
		)
		if svcRef.Name != "" {
			ports, err = lister.ServicePorts(context.Background(), svcRef, podRef)
		} else {
			ports, err = lister.PodPorts(context.Background(), podRef)
		}
		return portsLoadedMsg{gen: gen, res: res, ref: podRef, ports: ports, err: err}
	}
}

// handlePortsLoaded acts on a resolved set of declared ports. A result whose gen no
// longer matches (a newer port-forward request superseded it) is dropped. Then:
//
//   - a listing error or an empty list falls back to the free-text ports prompt — the
//     picker is an affordance over declared ports, and declaring them is optional in
//     the API, so neither case may block the forward (D137/principle 3). The fallback
//     is silent: the user gets the prompt they got before the picker existed, and a
//     toast alongside a freshly-opened prompt would only add noise;
//   - a single declared port is forwarded straight away (local = remote), skipping the
//     prompt entirely — the common Pod case, one keystroke instead of a typed spec;
//   - several open the port picker, stashing the target so the pick knows what it
//     applies to (the picker's SelectedMsg carries only the chosen label, D65).
func (m Model) handlePortsLoaded(msg portsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.pfResolveGen {
		return m, nil // superseded by a newer port-forward request; drop.
	}
	if msg.err != nil || len(msg.ports) == 0 {
		return m.showPortForwardPrompt(msg.res, msg.ref)
	}
	m.mutateRes = msg.res
	m.mutateRef = msg.ref
	if len(msg.ports) == 1 {
		return m.runPortForward(portForwardSpec(msg.ports[0]))
	}
	m.pfPorts = msg.ports
	m.portPicker.SetItems(portPickerItems(msg.ports))
	m.portPicker.Show()
	return m, nil
}

// handlePortSelected acts on the port the user picked: it closes the picker and starts
// the forward over the stashed target. A label that no longer matches a listed port
// (only reachable if the set changed under the open picker) degrades to the free-text
// prompt rather than forwarding a guessed number.
func (m Model) handlePortSelected(msg picker.SelectedMsg) (tea.Model, tea.Cmd) {
	m.portPicker.Hide()
	p, ok := portForLabel(m.pfPorts, msg.Value)
	if !ok {
		return m.showPortForwardPrompt(m.mutateRes, m.mutateRef)
	}
	return m.runPortForward(portForwardSpec(p))
}

// portForwardSpec renders a chosen port as the port spec kube.PortForward takes. A
// bare number is kubectl's shorthand for "same local port as remote", which is what a
// user picking a declared port means. The local side stays implicit here — making it
// editable (and offering ":0" for an OS-assigned free local port) is FB-pf-local-port;
// until then a local clash still surfaces the D130 bind hint naming the retry syntax.
func portForwardSpec(p kube.Port) string {
	return strconv.Itoa(int(p.Port))
}

// portPickerItems renders the listed ports as picker rows, in listing order.
func portPickerItems(ports []kube.Port) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, portLabel(p))
	}
	return out
}

// portLabel describes one declared port as a picker row: the number a forward will
// target, prefixed with the Service port it is reached through when the two differ
// ("80 → 8080" — the number the user knows the service by, then the pod-side number
// the forward actually uses, D137), and suffixed with the declared name and container
// when known ("8080 (http · nginx)") so an unfamiliar port is identifiable.
func portLabel(p kube.Port) string {
	s := strconv.Itoa(int(p.Port))
	if p.ServicePort != 0 && p.ServicePort != p.Port {
		s = strconv.Itoa(int(p.ServicePort)) + " → " + s
	}
	var meta []string
	if p.Name != "" {
		meta = append(meta, p.Name)
	}
	if p.Container != "" {
		meta = append(meta, p.Container)
	}
	if len(meta) > 0 {
		s += " (" + strings.Join(meta, " · ") + ")"
	}
	return s
}

// portForLabel maps a picked row back to the port it was rendered from. The picker
// resolves to a label (D65), so the label is the join key; ports are de-duplicated by
// number at the primitive (D137), so a label identifies at most one entry.
func portForLabel(ports []kube.Port, label string) (kube.Port, bool) {
	for _, p := range ports {
		if portLabel(p) == label {
			return p, true
		}
	}
	return kube.Port{}, false
}
