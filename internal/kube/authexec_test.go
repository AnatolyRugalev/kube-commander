package kube

import (
	"fmt"
	"net/url"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Two contexts on the same cluster: one authenticates through an EKS-shaped exec
// plugin, the other with a static token. current-context is the token one, so a
// test has to *select* the exec context to see the stanza.
const execKubeconfig = `apiVersion: v1
kind: Config
current-context: plain
clusters:
- name: c
  cluster:
    server: https://c.example:6443
contexts:
- name: plain
  context:
    cluster: c
    user: token-user
- name: eks
  context:
    cluster: c
    user: sso-user
users:
- name: token-user
  user:
    token: t
- name: sso-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args:
      - --region
      - eu-central-1
      - eks
      - get-token
      - --cluster-name
      - my cluster
      - --profile
      - acme-prod
      env:
      - name: AWS_PROFILE
        value: acme-prod
      installHint: install the aws cli
`

func TestExecPluginForReadsSelectedContextStanza(t *testing.T) {
	path := writeKubeconfigContents(t, execKubeconfig)
	p, err := ExecPluginFor(ClientConfig{Kubeconfig: path, Context: "eks"})
	if err != nil {
		t.Fatalf("ExecPluginFor: %v", err)
	}
	if p == nil {
		t.Fatal("ExecPluginFor returned no plugin for a context that declares one")
	}
	if p.Context != "eks" || p.User != "sso-user" {
		t.Errorf("plugin identifies %q/%q, want eks/sso-user", p.Context, p.User)
	}
	if p.Command != "aws" {
		t.Errorf("Command = %q, want aws", p.Command)
	}
	if got, want := len(p.Args), 8; got != want {
		t.Fatalf("Args has %d entries, want %d: %q", got, want, p.Args)
	}
	if p.Args[7] != "acme-prod" {
		t.Errorf("last arg = %q, want acme-prod", p.Args[7])
	}
	if p.Env["AWS_PROFILE"] != "acme-prod" {
		t.Errorf("Env[AWS_PROFILE] = %q, want acme-prod", p.Env["AWS_PROFILE"])
	}
	if p.APIVersion != "client.authentication.k8s.io/v1beta1" {
		t.Errorf("APIVersion = %q", p.APIVersion)
	}
	if p.InstallHint != "install the aws cli" {
		t.Errorf("InstallHint = %q", p.InstallHint)
	}
}

// The common case: a context that authenticates by token has no plugin, and that
// is not an error — callers must be able to ask unconditionally.
func TestExecPluginForNoStanzaIsNotAnError(t *testing.T) {
	path := writeKubeconfigContents(t, execKubeconfig)
	for _, cc := range []ClientConfig{
		{Kubeconfig: path},                      // current-context: plain
		{Kubeconfig: path, Context: "plain"},    // explicit
		{Kubeconfig: path, Context: "nonesuch"}, // undeclared context
	} {
		p, err := ExecPluginFor(cc)
		if err != nil {
			t.Fatalf("ExecPluginFor(%+v): %v", cc, err)
		}
		if p != nil {
			t.Errorf("ExecPluginFor(%+v) = %+v, want nil", cc, p)
		}
	}
}

func TestExecPluginForBadKubeconfigClassifiesAsBadContext(t *testing.T) {
	path := writeKubeconfigContents(t, "not: [valid")
	p, err := ExecPluginFor(ClientConfig{Kubeconfig: path})
	if err == nil {
		t.Fatalf("ExecPluginFor on a malformed kubeconfig returned %+v, want an error", p)
	}
	if got := Classify(err); got != KindBadContext {
		t.Errorf("Classify = %v, want %v", got, KindBadContext)
	}
}

func TestCommandLineQuotesOnlyWhitespaceTokens(t *testing.T) {
	p := &ExecPlugin{Command: "aws", Args: []string{"eks", "get-token", "--cluster-name", "my cluster"}}
	want := "aws eks get-token --cluster-name 'my cluster'"
	if got := p.CommandLine(); got != want {
		t.Errorf("CommandLine() = %q, want %q", got, want)
	}
	var nilPlugin *ExecPlugin
	if got := nilPlugin.CommandLine(); got != "" {
		t.Errorf("nil CommandLine() = %q, want empty", got)
	}
}

func TestExecPluginFailedRecoversClientGoMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ExecPluginFailure
		ok   bool
	}{
		{
			name: "exit code, as client-go formats it",
			// The exact shape of client-go's wrapCmdRunErrorLocked + the
			// `getting credentials: %v` wrap, then net/http's *url.Error.
			err:  &url.Error{Op: "Get", URL: "https://c.example/api", Err: fmt.Errorf("getting credentials: exec: executable aws failed with exit code 255")},
			want: ExecPluginFailure{Command: "aws", ExitCode: 255},
			ok:   true,
		},
		{
			name: "binary missing",
			err:  fmt.Errorf("getting credentials: exec: executable aws-iam-authenticator not found\n\nsome install hint"),
			want: ExecPluginFailure{Command: "aws-iam-authenticator", NotFound: true},
			ok:   true,
		},
		{
			name: "wrapped by the kube layer",
			err:  fmt.Errorf("kube: listing pods: %w", fmt.Errorf("getting credentials: exec: executable gke-gcloud-auth-plugin failed with exit code 1")),
			want: ExecPluginFailure{Command: "gke-gcloud-auth-plugin", ExitCode: 1},
			ok:   true,
		},
		{
			name: "client-go's default branch carries no name — not matched",
			err:  fmt.Errorf("getting credentials: exec: signal: killed"),
			ok:   false,
		},
		{name: "nil", err: nil, ok: false},
		{name: "unrelated", err: fmt.Errorf("connection refused"), ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExecPluginFailed(tc.err)
			if ok != tc.ok {
				t.Fatalf("ExecPluginFailed ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("ExecPluginFailed = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The point of the kind: a plugin failure arrives wrapped in a *url.Error because
// it happens inside RoundTrip, and must not read as "cluster unreachable" — the
// cluster was never contacted.
func TestClassifyExecPluginBeatsTransport(t *testing.T) {
	err := &url.Error{
		Op:  "Get",
		URL: "https://c.example/api",
		Err: fmt.Errorf("getting credentials: exec: executable aws failed with exit code 255"),
	}
	if got := Classify(err); got != KindExecPlugin {
		t.Errorf("Classify = %v, want %v", got, KindExecPlugin)
	}
	if got := KindExecPlugin.String(); got != "exec-plugin" {
		t.Errorf("String() = %q, want exec-plugin", got)
	}
}

// A real server status always wins: a 401 that merely *mentions* an exec plugin
// is still an authentication failure from the apiserver, not a plugin failure.
func TestClassifyStatusErrorWinsOverExecPluginText(t *testing.T) {
	err := apierrors.NewUnauthorized("exec: executable aws failed with exit code 1")
	if got := Classify(err); got != KindUnauthorized {
		t.Errorf("Classify = %v, want %v", got, KindUnauthorized)
	}
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "exec: executable aws not found")
	if got := Classify(notFound); got != KindNotFound {
		t.Errorf("Classify = %v, want %v", got, KindNotFound)
	}
}
