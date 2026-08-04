package kube

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

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

// --- AUTH-02: the diagnostic re-run ---------------------------------------

// shellPlugin builds an ExecPlugin that runs script through /bin/sh, standing in
// for a real credential plugin: the point of Diagnose is that it runs the
// stanza's own command verbatim, so the tests exercise a real process rather
// than a seam. Windows is a declared non-goal (goals.md), so a POSIX shell is a
// fair assumption — but the probe stays, because a container without one should
// skip rather than fail.
func shellPlugin(t *testing.T, script string) *ExecPlugin {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("no POSIX shell on PATH: %v", err)
	}
	return &ExecPlugin{Context: "eks", User: "sso-user", Command: "sh", Args: []string{"-c", script}}
}

func TestDiagnoseCapturesStderrAndExitCode(t *testing.T) {
	p := shellPlugin(t, `echo 'Error loading SSO Token: Token for https://acme.awsapps.com/start does not exist' >&2; exit 255`)
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if !strings.Contains(d.Stderr, "Error loading SSO Token") {
		t.Errorf("Stderr = %q, want the plugin's own message", d.Stderr)
	}
	if d.ExitCode != 255 {
		t.Errorf("ExitCode = %d, want 255", d.ExitCode)
	}
	if !d.Failed() {
		t.Error("Failed() = false for a plugin that exited 255")
	}
	if d.NotFound || d.TimedOut || d.Truncated {
		t.Errorf("NotFound/TimedOut/Truncated = %v/%v/%v, want all false", d.NotFound, d.TimedOut, d.Truncated)
	}
	if d.CommandLine == "" || !strings.HasPrefix(d.CommandLine, "sh ") {
		t.Errorf("CommandLine = %q, want the invocation that was run", d.CommandLine)
	}
}

// The credential is on stdout. Nothing about *why* the plugin failed lives
// there, so the diagnosis must not carry it anywhere — not in a field, not
// accidentally merged into the stderr capture (D211 pt 2).
func TestDiagnoseNeverCarriesStdout(t *testing.T) {
	// The token is assembled by the shell so the marker never appears in the
	// command line itself — otherwise CommandLine would fail this test for the
	// wrong reason.
	p := shellPlugin(t, `tok=SECRET; echo "{\"status\":{\"token\":\"k8s-aws-v1.${tok}TOKEN\"}}"; echo 'expired' >&2; exit 1`)
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if got := fmt.Sprintf("%+v", d); strings.Contains(got, "SECRETTOKEN") {
		t.Errorf("diagnosis carries the plugin's stdout: %s", got)
	}
	if d.Stderr != "expired" {
		t.Errorf("Stderr = %q, want %q", d.Stderr, "expired")
	}
}

// A re-run that succeeds is a real outcome — the session may have been renewed
// between the failed request and the diagnosis — and must not be reported as a
// failure or as an error.
func TestDiagnoseSucceedingRunIsNotAFailure(t *testing.T) {
	p := shellPlugin(t, `exit 0`)
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if d.Failed() {
		t.Errorf("Failed() = true for a plugin that exited 0: %+v", d)
	}
}

// A missing binary is a different remediation (install it, don't re-authenticate),
// so it is a field on the diagnosis rather than an error out of it.
func TestDiagnoseMissingBinaryIsNotFound(t *testing.T) {
	p := &ExecPlugin{Command: "kubecom-no-such-credential-plugin", Args: []string{"get-token"}}
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if !d.NotFound || !d.Failed() {
		t.Errorf("diagnosis = %+v, want NotFound", d)
	}
}

// A plugin that would prompt must fail, not hang: the TUI owns the terminal, so
// stdin is closed and a read sees EOF (D211 pt 1).
func TestDiagnoseClosesStdin(t *testing.T) {
	p := shellPlugin(t, `if read line; then echo "READ:$line" >&2; else echo EOF >&2; fi; exit 1`)
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if d.Stderr != "EOF" {
		t.Errorf("Stderr = %q, want EOF — the plugin's stdin was not closed", d.Stderr)
	}
}

// The stanza's env is part of the invocation that failed, and it wins over the
// process environment — otherwise the diagnostic runs against a different
// profile from the one client-go used.
func TestDiagnoseAppliesStanzaEnvOverProcessEnv(t *testing.T) {
	t.Setenv("AWS_PROFILE", "from-process")
	p := shellPlugin(t, `echo "$AWS_PROFILE" >&2; exit 4`)
	p.Env = map[string]string{"AWS_PROFILE": "acme-prod"}
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if d.Stderr != "acme-prod" {
		t.Errorf("Stderr = %q, want the stanza's AWS_PROFILE", d.Stderr)
	}
}

// A plugin inherits the process environment too — the stanza is an overlay, not
// a replacement.
func TestEnvironOverlaysRatherThanReplaces(t *testing.T) {
	p := &ExecPlugin{Env: map[string]string{"B": "stanza", "A": "stanza"}}
	got := p.environ([]string{"PATH=/bin", "B=process", "MALFORMED"})
	want := []string{"PATH=/bin", "MALFORMED", "A=stanza", "B=stanza"}
	if len(got) != len(want) {
		t.Fatalf("environ = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("environ = %q, want %q", got, want)
		}
	}
	base := []string{"PATH=/bin"}
	if plain := (&ExecPlugin{}).environ(base); len(plain) != 1 || plain[0] != "PATH=/bin" {
		t.Errorf("environ with no overrides = %q, want the base unchanged", plain)
	}
}

// A wedged plugin is killed, and the kill is reported as a timeout rather than
// as a signal exit — the caller's deadline is honoured, not just the 15s cap.
func TestDiagnoseTimesOut(t *testing.T) {
	p := shellPlugin(t, `sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	d, err := p.Diagnose(ctx)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if !d.TimedOut || !d.Failed() {
		t.Errorf("diagnosis = %+v, want TimedOut", d)
	}
	if elapsed := time.Since(start); elapsed > execDiagnosisTimeout {
		t.Errorf("Diagnose took %v — the caller's deadline was ignored", elapsed)
	}
}

// A cancelled caller diagnosed nothing, so it gets an error and an empty
// diagnosis rather than a half-observation that reads as a plugin failure.
func TestDiagnoseCancelledCallerReturnsError(t *testing.T) {
	p := shellPlugin(t, `exit 0`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := p.Diagnose(ctx)
	if err == nil {
		t.Fatalf("Diagnose on a cancelled context = %+v, want an error", d)
	}
	if d != (ExecPluginDiagnosis{}) {
		t.Errorf("diagnosis = %+v, want the zero value", d)
	}
}

func TestDiagnoseTruncatesRunawayStderr(t *testing.T) {
	p := shellPlugin(t, `i=0; while [ $i -lt 400 ]; do echo 0123456789012345678901234567890123456789; i=$((i+1)); done >&2; exit 1`)
	d, err := p.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if !d.Truncated {
		t.Error("Truncated = false for stderr past the cap")
	}
	if len(d.Stderr) > execDiagnosisStderrLimit {
		t.Errorf("Stderr is %d bytes, want at most %d", len(d.Stderr), execDiagnosisStderrLimit)
	}
}

// Nothing to run is a configuration error, not a plugin failure: it classifies
// like every other unusable-kubeconfig case (#86, principle 3).
func TestDiagnoseWithoutAStanzaIsBadContext(t *testing.T) {
	var nilPlugin *ExecPlugin
	for name, p := range map[string]*ExecPlugin{
		"nil":           nilPlugin,
		"empty command": {Context: "eks"},
		"blank command": {Context: "eks", Command: "   "},
	} {
		d, err := p.Diagnose(context.Background())
		if err == nil {
			t.Fatalf("%s: Diagnose = %+v, want an error", name, d)
		}
		if got := Classify(err); got != KindBadContext {
			t.Errorf("%s: Classify = %v, want %v", name, got, KindBadContext)
		}
	}
}

// The cap must never short-write: a child whose stderr pipe reports fewer bytes
// than it wrote can die of EPIPE instead of reaching its own exit status.
func TestCappedBufferReportsFullWrites(t *testing.T) {
	c := &cappedBuffer{limit: 4}
	for _, chunk := range []string{"ab", "cdef", "gh"} {
		n, err := c.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v; want %d, nil", chunk, n, err, len(chunk))
		}
	}
	if got := c.String(); got != "abcd" {
		t.Errorf("String() = %q, want abcd", got)
	}
	if !c.truncated {
		t.Error("truncated = false after overflowing the cap")
	}
	empty := &cappedBuffer{limit: 0}
	if n, err := empty.Write(nil); n != 0 || err != nil || empty.truncated {
		t.Errorf("empty write = %d, %v, truncated=%v; want 0, nil, false", n, err, empty.truncated)
	}
}
