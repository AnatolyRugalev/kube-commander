package tui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
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
//   - any declared port — including a lone one — opens the port picker, stashing the
//     target so the pick knows what it applies to (the picker's SelectedMsg carries
//     only the chosen label, D65). Confirming is still one keystroke, and the picker
//     is the only surface carrying the local-port gestures, so a single-port target
//     must not skip it (D139 supersedes D138 pt 2).
func (m Model) handlePortsLoaded(msg portsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.pfResolveGen {
		return m, nil // superseded by a newer port-forward request; drop.
	}
	if msg.err != nil || len(msg.ports) == 0 {
		return m.showPortForwardPrompt(msg.res, msg.ref)
	}
	m.mutateRes = msg.res
	m.mutateRef = msg.ref
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
// user confirming a declared port means (D138): the plain pick stays one keystroke.
// The two local-port gestures below build the other two specs.
func portForwardSpec(p kube.Port) string {
	return strconv.Itoa(int(p.Port))
}

// freeLocalPortSpec renders a chosen port with its local side left to the OS:
// kubectl's leading-colon form, ":<remote>". Note it is *not* ":0" — client-go parses
// the half after the colon as the **remote** port and rejects 0 ("remote port must be
// > 0"), so the free-local spec always names the real remote port and leaves the local
// half empty (D139). The bound local port is reported once the forward is ready
// (PortForward.Ports → the status notice), which is how the user learns what it got.
func freeLocalPortSpec(p kube.Port) string {
	return ":" + strconv.Itoa(int(p.Port))
}

// localPortSpec renders a chosen port with the local side the prompt collected. A
// blank entry means "let the OS pick one" (the prompt says so) — the same spec the
// free-local gesture builds — so clearing the field is a second way to reach it.
// Anything else is used verbatim as the local half: kube.PortForward validates it and
// a malformed entry degrades to an error toast (principle 3) rather than being
// second-guessed here.
func localPortSpec(p kube.Port, local string) string {
	local = strings.TrimSpace(local)
	if local == "" {
		return freeLocalPortSpec(p)
	}
	return local + ":" + strconv.Itoa(int(p.Port))
}

// localPortModalKind stamps the prompt the "set the local port" gesture opens, so the
// shared modal's ConfirmedMsg routes back to runLocalPortForward rather than the
// free-text ports prompt (portForwardModalKind) or the other mutating modals (D117).
const localPortModalKind = "portForwardLocal"

// selectedPort returns the kube.Port the port picker currently highlights. The picker
// resolves to a label (D65), so the highlighted row maps back through the listed set
// exactly as a confirmed pick does; false means the picker is empty (or the set
// changed under it), which leaves the gestures inert rather than guessing a port.
func (m Model) selectedPort() (kube.Port, bool) {
	label, ok := m.portPicker.Selected()
	if !ok {
		return kube.Port{}, false
	}
	return portForLabel(m.pfPorts, label)
}

// forwardOnFreeLocalPort is the one-keystroke escape from a local port clash
// (FB-pf-local-port): it forwards the highlighted port immediately with the local side
// left to the OS. It is the gesture the D130 bind hint used to describe in prose —
// "retry with :<remote>" — turned into a key, so the common "port 6379 is already
// taken locally" case never needs a typed spec (the hint stays for a forward that was
// already started).
func (m Model) forwardOnFreeLocalPort() (tea.Model, tea.Cmd) {
	p, ok := m.selectedPort()
	if !ok {
		return m, nil
	}
	m.portPicker.Hide()
	return m.runPortForward(freeLocalPortSpec(p))
}

// promptLocalPort opens the local-port prompt over the highlighted port: a single-line
// entry seeded with the remote number (the local = remote default, editable), whose
// blank value means an OS-assigned free port. The target stays in the shared
// mutate stash (D117) that handlePortsLoaded filled; pfPort carries the chosen port
// across the prompt so the submitted local half can be joined to the right remote one.
func (m Model) promptLocalPort() (tea.Model, tea.Cmd) {
	p, ok := m.selectedPort()
	if !ok {
		return m, nil
	}
	m.portPicker.Hide()
	m.pfPort = p
	remote := strconv.Itoa(int(p.Port))
	cmd := m.modal.ShowPrompt(localPortModalKind, "Port-forward",
		"Local port for remote "+remote+" (blank = free port):", remote)
	return m, cmd
}

// runLocalPortForward starts the stashed forward with the local port the prompt
// collected, joined to the remote port the gesture was invoked on.
func (m Model) runLocalPortForward(value string) (tea.Model, tea.Cmd) {
	return m.runPortForward(localPortSpec(m.pfPort, value))
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

// portPickerTitle renders the port picker's title with the two local-port gestures
// advertised by their *resolved* keys. The picker is the only surface where they do
// anything and a modal has no hint bar, so the title is where they are discoverable —
// e.g. "Port-forward port · p local · 0 free". The keys come from the keymap, never a
// literal (D11), so a rebind is reflected and a disabled action drops out of the title.
func portPickerTitle(km *keymap.Keymap) string {
	title := "Port-forward port"
	var hints []string
	if k := firstKey(km, keymap.ActionLocalPort); k != "" {
		hints = append(hints, k+" local")
	}
	if k := firstKey(km, keymap.ActionFreeLocalPort); k != "" {
		hints = append(hints, k+" free")
	}
	if len(hints) > 0 {
		title += " · " + strings.Join(hints, " · ")
	}
	return title
}

// firstKey is an action's primary bound key ("" when the user disabled it), the token
// a hint shows.
func firstKey(km *keymap.Keymap, a keymap.Action) string {
	if ks := km.Keys(a); len(ks) > 0 {
		return ks[0]
	}
	return ""
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
