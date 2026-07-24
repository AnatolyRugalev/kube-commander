package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// ctrPort builds one declared container port, defaulting the protocol to TCP the
// way the API server does when it is omitted.
func ctrPort(name string, port int32, proto corev1.Protocol) corev1.ContainerPort {
	return corev1.ContainerPort{Name: name, ContainerPort: port, Protocol: proto}
}

// podWithPorts builds a pod whose containers declare the given ports, so
// PodPorts/podDeclaredPorts can be exercised without a cluster.
func podWithPorts(ns, name string, containers ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       corev1.PodSpec{Containers: containers},
	}
}

// container builds a named container declaring ports.
func container(name string, ports ...corev1.ContainerPort) corev1.Container {
	return corev1.Container{Name: name, Ports: ports}
}

// sidecar builds a native sidecar initContainer (restartPolicy: Always) declaring
// ports — it runs for the pod's whole life, so its ports are forwardable.
func sidecar(name string, ports ...corev1.ContainerPort) corev1.Container {
	always := corev1.ContainerRestartPolicyAlways
	c := container(name, ports...)
	c.RestartPolicy = &always
	return c
}

// svcWithPorts builds a Service declaring the given ports.
func svcWithPorts(ns, name string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       corev1.ServiceSpec{Ports: ports},
	}
}

// assertPorts compares a Port slice against the expected entries, in order.
func assertPorts(t *testing.T, got, want []Port) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ports: got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("port %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPodPortsListsDeclaredTCPPorts(t *testing.T) {
	pod := podWithPorts("web", "api",
		container("app", ctrPort("http", 8080, ""), ctrPort("metrics", 9090, corev1.ProtocolTCP)),
		container("side", ctrPort("grpc", 9000, corev1.ProtocolTCP)),
	)
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(pod)}

	got, err := c.PodPorts(context.Background(), ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodPorts: %v", err)
	}
	assertPorts(t, got, []Port{
		{Port: 8080, Name: "http", Container: "app"},
		{Port: 9090, Name: "metrics", Container: "app"},
		{Port: 9000, Name: "grpc", Container: "side"},
	})
}

func TestPodPortsFiltersNonTCPAndOutOfRange(t *testing.T) {
	pod := podWithPorts("web", "api", container("app",
		ctrPort("dns", 53, corev1.ProtocolUDP),
		ctrPort("sctp", 38412, corev1.ProtocolSCTP),
		ctrPort("bad", 0, corev1.ProtocolTCP),
		ctrPort("huge", 70000, corev1.ProtocolTCP),
		ctrPort("http", 80, corev1.ProtocolTCP),
	))

	// UDP/SCTP can't be tunnelled by port-forward and must not be offered; 0 and
	// 70000 are outside the TCP range.
	assertPorts(t, podDeclaredPorts(pod), []Port{{Port: 80, Name: "http", Container: "app"}})
}

func TestPodPortsDeduplicatesByNumber(t *testing.T) {
	pod := podWithPorts("web", "api",
		container("app", ctrPort("http", 8080, "")),
		container("copy", ctrPort("also-http", 8080, "")),
	)

	// One forward target → one picker entry, keeping the first declaration's label.
	assertPorts(t, podDeclaredPorts(pod), []Port{{Port: 8080, Name: "http", Container: "app"}})
}

func TestPodPortsIncludesNativeSidecarsNotInitContainers(t *testing.T) {
	pod := podWithPorts("web", "api", container("app", ctrPort("http", 8080, "")))
	pod.Spec.InitContainers = []corev1.Container{
		container("migrate", ctrPort("debug", 7000, "")), // plain init container: exited
		sidecar("proxy", ctrPort("proxy", 15001, "")),    // native sidecar: still running
	}

	assertPorts(t, podDeclaredPorts(pod), []Port{
		{Port: 8080, Name: "http", Container: "app"},
		{Port: 15001, Name: "proxy", Container: "proxy"},
	})
}

func TestPodPortsEmptyWhenNoneDeclared(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(podWithPorts("web", "api", container("app")))}

	got, err := c.PodPorts(context.Background(), ObjectRef{Namespace: "web", Name: "api"})
	if err != nil {
		t.Fatalf("PodPorts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("declaring no ports is not an error and yields no choices, got %+v", got)
	}
}

func TestPodPortsRejectsEmptyNameAndMissingPod(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}

	if _, err := c.PodPorts(context.Background(), ObjectRef{Namespace: "web"}); err == nil {
		t.Error("expected an error for an empty pod name")
	}
	if _, err := c.PodPorts(context.Background(), ObjectRef{Namespace: "web", Name: "gone"}); err == nil {
		t.Error("expected a wrapped error for a missing pod")
	}
}

func TestServicePortsResolvesNumericTarget(t *testing.T) {
	svc := svcWithPorts("web", "api", corev1.ServicePort{
		Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080),
	})
	pod := podWithPorts("web", "api-1", container("app", ctrPort("http", 8080, "")))
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc, pod)}

	got, err := c.ServicePorts(context.Background(),
		ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{Namespace: "web", Name: "api-1"})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	// The forward targets the pod-side 8080; 80 is kept as the number the user knows.
	assertPorts(t, got, []Port{{Port: 8080, Name: "http", Container: "app", ServicePort: 80}})
}

func TestServicePortsResolvesNamedTargetAgainstPod(t *testing.T) {
	svc := svcWithPorts("web", "api", corev1.ServicePort{
		Name: "http", Port: 80, TargetPort: intstr.FromString("http"),
	})
	pod := podWithPorts("web", "api-1", container("app", ctrPort("http", 8080, "")))
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc, pod)}

	got, err := c.ServicePorts(context.Background(),
		ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{Namespace: "web", Name: "api-1"})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	assertPorts(t, got, []Port{{Port: 8080, Name: "http", Container: "app", ServicePort: 80}})
}

func TestServicePortsDropsUnresolvableNamedTarget(t *testing.T) {
	svc := svcWithPorts("web", "api",
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromString("nope")},
		corev1.ServicePort{Name: "admin", Port: 8081, TargetPort: intstr.FromInt32(8081)},
	)
	pod := podWithPorts("web", "api-1", container("app", ctrPort("http", 8080, "")))
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc, pod)}

	got, err := c.ServicePorts(context.Background(),
		ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{Namespace: "web", Name: "api-1"})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	// A name the pod doesn't declare is dropped, never guessed; the numeric one stays.
	assertPorts(t, got, []Port{{Port: 8081, Name: "admin", ServicePort: 8081}})
}

func TestServicePortsUnsetTargetDefaultsToServicePort(t *testing.T) {
	svc := svcWithPorts("web", "api", corev1.ServicePort{Name: "http", Port: 80})
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc)}

	got, err := c.ServicePorts(context.Background(),
		ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	assertPorts(t, got, []Port{{Port: 80, Name: "http", ServicePort: 80}})
}

func TestServicePortsWithoutPodResolvesNumericOnly(t *testing.T) {
	svc := svcWithPorts("web", "api",
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
		corev1.ServicePort{Name: "admin", Port: 8081, TargetPort: intstr.FromInt32(9091)},
	)
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc)}

	// No backing pod given (nothing resolved it yet): named targets can't be looked
	// up and are dropped, numeric ones still forward.
	got, err := c.ServicePorts(context.Background(), ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	assertPorts(t, got, []Port{{Port: 9091, Name: "admin", ServicePort: 8081}})
}

func TestServicePortsFiltersNonTCP(t *testing.T) {
	svc := svcWithPorts("web", "api",
		corev1.ServicePort{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt32(53)},
		corev1.ServicePort{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(8080)},
	)
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc)}

	got, err := c.ServicePorts(context.Background(), ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	assertPorts(t, got, []Port{{Port: 8080, Name: "http", ServicePort: 80}})
}

func TestServicePortsRejectsEmptyNameAndMissingService(t *testing.T) {
	c := &Clients{Clientset: k8sfake.NewSimpleClientset()}

	if _, err := c.ServicePorts(context.Background(), ObjectRef{Namespace: "web"}, ObjectRef{}); err == nil {
		t.Error("expected an error for an empty service name")
	}
	if _, err := c.ServicePorts(context.Background(), ObjectRef{Namespace: "web", Name: "gone"}, ObjectRef{}); err == nil {
		t.Error("expected a wrapped error for a missing service")
	}
}

func TestServicePortsToleratesUnfetchablePod(t *testing.T) {
	svc := svcWithPorts("web", "api",
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
		corev1.ServicePort{Name: "admin", Port: 8081, TargetPort: intstr.FromInt32(9091)},
	)
	c := &Clients{Clientset: k8sfake.NewSimpleClientset(svc)} // pod absent from the tracker

	// A pod that vanished between resolution and this call degrades to the numeric
	// ports rather than failing the whole listing (principle 3).
	got, err := c.ServicePorts(context.Background(),
		ObjectRef{Namespace: "web", Name: "api"}, ObjectRef{Namespace: "web", Name: "api-1"})
	if err != nil {
		t.Fatalf("ServicePorts: %v", err)
	}
	assertPorts(t, got, []Port{{Port: 9091, Name: "admin", ServicePort: 8081}})
}
