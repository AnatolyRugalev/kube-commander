package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Port is one declared, forwardable port discovered on a pod (or on a Service that
// fronts one) — the *remote* side of a port-forward, offered to the user as a
// choice instead of the free-text remote entry the ports prompt takes today
// (FB-pf-port-picker). Like ForwardedPort it is decoupled from client-go's types so
// the TUI never imports corev1 (the apimachinery-free boundary, D33).
//
// Only TCP ports appear here: kubecom's port-forward tunnels TCP streams over the
// SPDY connection to the pod's portforward subresource, exactly as `kubectl
// port-forward` does, so a UDP or SCTP declaration is not a forwardable target and
// is filtered out rather than offered as a choice that could never work.
type Port struct {
	// Port is the port number on the pod — what a forward's remote side targets and
	// what the "8080:80" spec's right-hand side carries. Always non-zero.
	Port uint16
	// Name is the declared port name ("http", "metrics"), empty when the declaration
	// is unnamed. It is a label for the picker, never part of a forward spec.
	Name string
	// Container names the pod container that declares the port. Empty for a
	// Service-derived port whose targetPort is a bare number matching no declaration.
	Container string
	// ServicePort is the Service port number this entry was reached through — the
	// number a user knows the service by, which may differ from Port (the pod-side
	// targetPort). It is 0 for a port read directly off a pod.
	ServicePort uint16
}

// PodPorts lists the TCP ports a pod declares, in declaration order, so the
// port-forward flow can offer them as choices (FB-pf-port-picker) rather than
// asking the user to remember a container port. It reads the pod's spec — the
// declared containerPorts, which is what `kubectl describe pod` shows — and never
// probes the running container, so it is a single cheap Get.
//
// Both regular containers and *native sidecar* initContainers (an initContainer
// with restartPolicy: Always, which runs for the pod's whole life) are scanned:
// a sidecar proxy or metrics exporter is exactly the sort of port a user wants to
// forward. Plain init containers are skipped — they have exited by the time a
// forward could reach them.
//
// A pod that declares no ports is not an error: it yields an empty list, and the
// caller degrades to the free-text ports prompt (principle 3). Declaring ports is
// optional in the API — a container listening on a port it never declared is
// perfectly normal, so an empty list must never be read as "nothing is listening".
// A missing pod or an RBAC denial is wrapped and returned, never panicked (#86).
func (c *Clients) PodPorts(ctx context.Context, ref ObjectRef) ([]Port, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: pod ports: empty pod name")
	}
	pod, err := c.Clientset.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: getting ports for pod %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	return podDeclaredPorts(pod), nil
}

// ServicePorts lists the TCP ports of a Service, each resolved to the port number
// on the backing pod that a forward actually targets. A Service cannot be forwarded
// directly (M1-08 posts to the pod portforward subresource), so the shell resolves
// it to an endpoint pod first (PodForService, M3-13c) and forwards *that* pod —
// which means the remote port must be the Service's targetPort, not its port. Each
// returned Port therefore carries the pod-side number in Port and the
// service-side number the user recognises in ServicePort, so the picker can label
// a choice "80 → 8080" without the caller re-deriving the mapping.
//
// A targetPort is an int-or-string: a number is used as-is, and a *named* target
// ("http") must be looked up among the backing pod's declared ports. podRef names
// that backing pod (as resolved by PodForService) and is fetched to do the lookup
// and to label each choice with its declaring container; pass the zero ObjectRef
// when no pod has been resolved. An unset targetPort defaults to the Service's own
// port, per the API.
//
// A named targetPort that cannot be resolved — no pod given, the pod is gone, or it
// declares no such port — is dropped rather than guessed: forwarding to a wrong
// number is worse than falling back to the free-text prompt (principle 3). As with
// PodPorts, an empty result is not an error.
func (c *Clients) ServicePorts(ctx context.Context, svcRef, podRef ObjectRef) ([]Port, error) {
	if svcRef.Name == "" {
		return nil, fmt.Errorf("kube: service ports: empty service name")
	}
	svc, err := c.Clientset.CoreV1().Services(svcRef.Namespace).Get(ctx, svcRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: getting ports for service %s/%s: %w", svcRef.Namespace, svcRef.Name, err)
	}

	// The backing pod resolves named targetPorts and labels every choice with its
	// declaring container. Its absence is not fatal — a caller with no pod ref, or a
	// pod that vanished since it was resolved, still gets the numeric ports.
	var pod *corev1.Pod
	if podRef.Name != "" {
		if p, err := c.Clientset.CoreV1().Pods(podRef.Namespace).Get(ctx, podRef.Name, metav1.GetOptions{}); err == nil {
			pod = p
		}
	}

	out := make([]Port, 0, len(svc.Spec.Ports))
	for i := range svc.Spec.Ports {
		p, ok := resolveServicePort(svc.Spec.Ports[i], pod)
		if !ok {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// podDeclaredPorts extracts a pod's forwardable declared ports. Pure, so the
// filtering (TCP only, native sidecars in, plain init containers out) and the
// de-duplication are unit-tested without a client.
//
// Duplicates are collapsed by port number: two containers declaring 8080 (or one
// container declaring it twice) are one forward target, so the picker shows one
// entry — the first declaration, which keeps its name and container for the label.
func podDeclaredPorts(pod *corev1.Pod) []Port {
	var out []Port
	seen := map[uint16]bool{}

	add := func(c *corev1.Container) {
		for _, cp := range c.Ports {
			if !forwardableProtocol(cp.Protocol) {
				continue
			}
			n, ok := portNumber(cp.ContainerPort)
			if !ok || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, Port{Port: n, Name: cp.Name, Container: c.Name})
		}
	}

	// Native sidecars first would reorder the common case; scan regular containers
	// first so the app's own ports head the picker, then the always-on sidecars.
	for i := range pod.Spec.Containers {
		add(&pod.Spec.Containers[i])
	}
	for i := range pod.Spec.InitContainers {
		if nativeSidecar(&pod.Spec.InitContainers[i]) {
			add(&pod.Spec.InitContainers[i])
		}
	}
	return out
}

// resolveServicePort maps one Service port onto the pod-side port a forward must
// target, reporting false when it is not a forwardable target (a non-TCP port, an
// out-of-range number, or a named targetPort with no backing declaration to resolve
// it against). Pure, so every branch of the int-or-string resolution is testable.
func resolveServicePort(sp corev1.ServicePort, pod *corev1.Pod) (Port, bool) {
	if !forwardableProtocol(sp.Protocol) {
		return Port{}, false
	}
	svcPort, ok := portNumber(sp.Port)
	if !ok {
		return Port{}, false
	}
	out := Port{Name: sp.Name, ServicePort: svcPort}

	switch {
	case sp.TargetPort.Type == intstr.String && sp.TargetPort.StrVal != "":
		// A named target ("http") resolves only against the backing pod's declarations.
		p, container, found := namedPodPort(pod, sp.TargetPort.StrVal)
		if !found {
			return Port{}, false
		}
		out.Port, out.Container = p, container
	case sp.TargetPort.IntVal != 0:
		p, ok := portNumber(sp.TargetPort.IntVal)
		if !ok {
			return Port{}, false
		}
		out.Port = p
		// Label the choice with the container declaring it, when one does.
		if c, found := containerForPort(pod, p); found {
			out.Container = c
		}
	default:
		// An unset targetPort defaults to the Service's own port (API default).
		out.Port = svcPort
		if c, found := containerForPort(pod, svcPort); found {
			out.Container = c
		}
	}
	return out, true
}

// namedPodPort finds the container port a Service's named targetPort refers to,
// returning the number and the declaring container. A nil pod (never fetched, or
// the fetch failed) simply resolves nothing.
func namedPodPort(pod *corev1.Pod, name string) (uint16, string, bool) {
	for _, p := range podDeclaredPorts(podOrEmpty(pod)) {
		if p.Name == name {
			return p.Port, p.Container, true
		}
	}
	return 0, "", false
}

// containerForPort names the container declaring the given port number, if any —
// used only to label a numeric Service target, so a miss is not a failure.
func containerForPort(pod *corev1.Pod, port uint16) (string, bool) {
	for _, p := range podDeclaredPorts(podOrEmpty(pod)) {
		if p.Port == port {
			return p.Container, true
		}
	}
	return "", false
}

// podOrEmpty lets the pure helpers treat "no backing pod" as "a pod declaring
// nothing", keeping the nil check in one place.
func podOrEmpty(pod *corev1.Pod) *corev1.Pod {
	if pod == nil {
		return &corev1.Pod{}
	}
	return pod
}

// nativeSidecar reports whether an initContainer is a sidecar that keeps running
// alongside the pod's regular containers (restartPolicy: Always, Kubernetes 1.28+)
// rather than a plain init container that exits before they start.
func nativeSidecar(c *corev1.Container) bool {
	return c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// forwardableProtocol reports whether a declared protocol can be port-forwarded.
// Port-forwarding tunnels TCP only; an empty protocol defaults to TCP per the API.
func forwardableProtocol(p corev1.Protocol) bool {
	return p == "" || p == corev1.ProtocolTCP
}

// portNumber narrows an API port (int32) to the wire's uint16, rejecting the
// out-of-range values the API type can hold but a TCP port cannot.
func portNumber(p int32) (uint16, bool) {
	if p < 1 || p > 65535 {
		return 0, false
	}
	return uint16(p), true
}
