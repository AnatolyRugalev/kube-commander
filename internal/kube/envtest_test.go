package kube

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// envtest integration tests are opt-in (D18). They spin up a real
// kube-apiserver + etcd via controller-runtime's envtest, so they need the
// control-plane binaries on disk (fetched with `setup-envtest`, exposed via
// KUBEBUILDER_ASSETS) and don't belong in the hermetic default suite that runs
// anywhere. The default `make check` runs with the gate off and skips them;
// fake clients (added in later M1 legs) are the everyday test strategy.
//
// To run locally:
//
//	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
//	export KUBEBUILDER_ASSETS="$(setup-envtest use -p path 1.31.x)"
//	KUBECOM_TEST_ENVTEST=1 go test ./internal/kube/...
//
// or use the `make test-envtest` convenience target.
const envtestGateEnv = "KUBECOM_TEST_ENVTEST"

// requireEnvtest skips the calling test unless the envtest gate is set, so the
// integration harness is opt-in. It is the single guard every envtest-backed
// test in this package calls first.
func requireEnvtest(t *testing.T) {
	t.Helper()
	if os.Getenv(envtestGateEnv) != "1" {
		t.Skipf("envtest disabled; set %s=1 to run (needs setup-envtest binaries)", envtestGateEnv)
	}
}

// controlPlaneOption tweaks the envtest.Environment before it is started. Some
// behaviors only exist on a plane configured to produce them (a watch expiry
// needs etcd compaction to actually run), and every such knob is a
// kube-apiserver flag that must be set *before* env.Start().
type controlPlaneOption func(*envtest.Environment)

// withAPIServerFlag appends a kube-apiserver command-line flag to the plane the
// calling test starts. Flags are appended, not replaced, so envtest's own
// defaults (certs, ports, service account keys) stay intact — an unknown flag
// makes the apiserver refuse to start, which surfaces as a failed env.Start().
func withAPIServerFlag(flag string, values ...string) controlPlaneOption {
	return func(env *envtest.Environment) {
		env.ControlPlane.GetAPIServer().Configure().Append(flag, values...)
	}
}

// startControlPlane stands up a real kube-apiserver + etcd for the calling test
// and returns the environment (needed for envtest.Environment.AddUser, which is
// how a test gets a *restricted* client) together with the admin *rest.Config.
// It skips the test when the gate is off and registers the teardown, so it is
// the single bootstrap every envtest-backed test in this package starts from.
// opts configure the apiserver before it starts (see withAPIServerFlag).
//
// Each test gets its own control plane rather than sharing one via TestMain:
// startup is ~5 s, and these tests mutate cluster-global state (an APIService,
// cluster-scoped RBAC) whose whole point is to break or restrict discovery — a
// shared plane would leak that breakage into every other test. Per-test options
// make sharing wrong for a second reason: the planes are no longer identical.
func startControlPlane(t *testing.T, opts ...controlPlaneOption) (*envtest.Environment, *rest.Config) {
	t.Helper()
	requireEnvtest(t)

	env := &envtest.Environment{}
	for _, opt := range opts {
		opt(env)
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start envtest control plane: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stop envtest control plane: %v", err)
		}
	})
	return env, cfg
}

// TestEnvtestSmoke is the M1-00 harness proof: it stands up a real control
// plane, talks to it with a client-go clientset, and asserts the API server is
// serving. Later M1 legs reuse this bootstrap to integration-test discovery,
// watch reconnect, and the action set against a live apiserver.
func TestEnvtestSmoke(t *testing.T) {
	_, cfg := startControlPlane(t)

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The `default` namespace exists on any fresh control plane; fetching it is
	// a minimal end-to-end check that the apiserver is reachable and serving.
	ns, err := clientset.CoreV1().Namespaces().Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get default namespace: %v", err)
	}
	if ns.Name != "default" {
		t.Fatalf("namespace name = %q, want %q", ns.Name, "default")
	}
}
