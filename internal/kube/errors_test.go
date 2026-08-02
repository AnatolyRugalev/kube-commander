package kube

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClassify(t *testing.T) {
	gr := schema.GroupResource{Group: "apps", Resource: "deployments"}

	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil", nil, KindUnknown},
		{"plain", errors.New("boom"), KindUnknown},
		{"not-found", apierrors.NewNotFound(gr, "web"), KindNotFound},
		{"already-exists", apierrors.NewAlreadyExists(gr, "web"), KindAlreadyExists},
		{"conflict", apierrors.NewConflict(gr, "web", errors.New("uid precondition")), KindConflict},
		{"unauthorized", apierrors.NewUnauthorized("bad token"), KindUnauthorized},
		{"forbidden", apierrors.NewForbidden(gr, "web", errors.New("rbac")), KindForbidden},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "web", nil), KindInvalid},
		{"bad-request", apierrors.NewBadRequest("malformed selector"), KindInvalid},
		{"server-timeout", apierrors.NewServerTimeout(gr, "list", 1), KindTimeout},
		{"timeout", apierrors.NewTimeoutError("took too long", 1), KindTimeout},
		{"service-unavailable", apierrors.NewServiceUnavailable("no backend"), KindUnreachable},
		{"context-deadline", context.DeadlineExceeded, KindTimeout},
		{"url-error", &url.Error{Op: "Get", URL: "https://x", Err: errors.New("connection refused")}, KindUnreachable},
		{"net-error", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, KindUnreachable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.err); got != tc.want {
				t.Fatalf("Classify(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestClassifyThroughWrap proves classification survives the layer's own
// `fmt.Errorf(... %w)` wrapping — the whole point of walking the chain (#86).
func TestClassifyThroughWrap(t *testing.T) {
	gr := schema.GroupResource{Group: "", Resource: "pods"}
	inner := apierrors.NewNotFound(gr, "web")
	wrapped := fmt.Errorf("kube: getting %s %q: %w", gr.Resource, "web", inner)
	if got := Classify(wrapped); got != KindNotFound {
		t.Fatalf("Classify(wrapped not-found) = %v, want %v", got, KindNotFound)
	}

	// A wrapped transport failure still reads as unreachable.
	dialWrap := fmt.Errorf("kube: listing pods: %w", &url.Error{
		Op: "Get", URL: "https://x", Err: errors.New("connection refused"),
	})
	if got := Classify(dialWrap); got != KindUnreachable {
		t.Fatalf("Classify(wrapped url error) = %v, want %v", got, KindUnreachable)
	}
}

// TestClassifyBadContext drives a real clientcmd error through RESTConfig (the
// construction-time path #86 names) and asserts it classifies as KindBadContext,
// wrapping and all.
func TestClassifyBadContext(t *testing.T) {
	path := writeKubeconfig(t)
	_, err := RESTConfig(ClientConfig{Kubeconfig: path, Context: "nope"})
	if err == nil {
		t.Fatal("RESTConfig with unknown context: want error, got nil")
	}
	if got := Classify(err); got != KindBadContext {
		t.Fatalf("Classify(unknown-context error) = %v, want %v (err: %v)", got, KindBadContext, err)
	}
}

func TestClassifyEmptyConfig(t *testing.T) {
	// An empty kubeconfig yields clientcmd's empty-config error.
	_, err := RESTConfig(ClientConfig{Kubeconfig: writeEmptyKubeconfig(t)})
	if err == nil {
		t.Skip("empty kubeconfig did not error in this environment")
	}
	if got := Classify(err); got != KindBadContext {
		t.Fatalf("Classify(empty-config error) = %v, want %v (err: %v)", got, KindBadContext, err)
	}
}

func TestErrorKindString(t *testing.T) {
	cases := map[ErrorKind]string{
		KindUnknown:       "unknown",
		KindNotFound:      "not-found",
		KindAlreadyExists: "already-exists",
		KindConflict:      "conflict",
		KindForbidden:     "forbidden",
		KindUnauthorized:  "unauthorized",
		KindInvalid:       "invalid",
		KindTimeout:       "timeout",
		KindUnreachable:   "unreachable",
		KindBadContext:    "bad-context",
		ErrorKind(999):    "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}

// writeEmptyKubeconfig writes a syntactically valid but context-less kubeconfig
// so RESTConfig's clientcmd load surfaces an empty/invalid-config error.
func writeEmptyKubeconfig(t *testing.T) string {
	t.Helper()
	const cfg = `apiVersion: v1
kind: Config
clusters: []
contexts: []
users: []
`
	path := filepath.Join(t.TempDir(), "empty-kubeconfig")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write empty kubeconfig: %v", err)
	}
	return path
}

// TestConversionWebhookFailed pins the narrow match: the apiserver's own
// "conversion webhook for … failed" wording is recognised through kubecom's
// wrapping, and no other server error is dressed up as one (CRD-01/D191 pt 1).
func TestConversionWebhookFailed(t *testing.T) {
	gr := schema.GroupResource{Group: "external-secrets.io", Resource: "externalsecrets"}
	webhook := apierrors.NewInternalError(errors.New(
		`conversion webhook for external-secrets.io/v1beta1, Kind=ExternalSecret failed: ` +
			`Post "https://external-secrets-webhook.external-secrets.svc:443/convert?timeout=30s": ` +
			`dial tcp 10.0.0.1:443: connect: connection refused`))

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"the apiserver's conversion failure", webhook, true},
		{"wrapped as the list path wraps it", fmt.Errorf("kube: listing externalsecrets: %w", webhook), true},
		{"a plain internal error", apierrors.NewInternalError(errors.New("boom")), false},
		{"a forbidden list", apierrors.NewForbidden(gr, "", errors.New("denied")), false},
		{"a transport failure", &url.Error{Op: "Get", Err: errors.New("connection refused")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConversionWebhookFailed(tc.err); got != tc.want {
				t.Fatalf("ConversionWebhookFailed = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTableUnsupported proves a 406 — the server refusing the Table content type
// every List and Watch asks for — is recognised through the wrap chain, and that
// neighbouring statuses are not.
func TestTableUnsupported(t *testing.T) {
	notAcceptable := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    406,
		Reason:  metav1.StatusReasonNotAcceptable,
		Message: "only the following media types are accepted: application/json",
	}}

	if !TableUnsupported(fmt.Errorf("kube: listing widgets: %w", notAcceptable)) {
		t.Fatal("a wrapped 406 must be recognised")
	}
	if TableUnsupported(nil) {
		t.Fatal("nil is not a 406")
	}
	if TableUnsupported(apierrors.NewBadRequest("nope")) {
		t.Fatal("a 400 is not a 406")
	}
	// 406 carries no ErrorKind of its own: the copy layer switches on the
	// predicate, and Classify must not be quietly taught to guess at it.
	if got := Classify(notAcceptable); got != KindUnknown {
		t.Fatalf("Classify(406) = %v, want KindUnknown", got)
	}
}
