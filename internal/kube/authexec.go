package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/client-go/tools/clientcmd"
)

// ExecPlugin describes the **exec credential plugin** a kubeconfig context uses
// to mint its credentials — the `user.exec` stanza that runs `aws eks get-token`,
// `gcloud config config-helper`, `az`, `aws-iam-authenticator` and friends.
// client-go runs it for us; kubecom reads it so that when it *fails* the failure
// can be reported in terms of the actual command, rather than as a nameless auth
// error (feedback `2026-08-01-eks-sso-reauth`).
//
// Everything here is pure kubeconfig data: the stanza is read, never executed,
// and no server is contacted. Env holds only the stanza's **own** overrides — the
// plugin also inherits the process environment, which this deliberately does not
// try to reproduce.
type ExecPlugin struct {
	// Context is the context the stanza was read for (the explicit
	// ClientConfig.Context, else the kubeconfig's current-context).
	Context string
	// User is the kubeconfig `users:` entry that holds the stanza.
	User string
	// Command is the executable the plugin runs, exactly as the kubeconfig
	// spells it (a bare name resolved on PATH, or an absolute path).
	Command string
	// Args are the plugin's arguments, in order.
	Args []string
	// Env are the stanza's explicit environment overrides, name→value. Nil when
	// the stanza sets none.
	Env map[string]string
	// APIVersion is the client.authentication.k8s.io version the stanza declares.
	APIVersion string
	// InstallHint is the stanza's operator-authored hint, shown by client-go when
	// the binary is missing. Usually empty.
	InstallHint string
}

// CommandLine renders the plugin invocation for display — the command and its
// arguments, single-quoting any token that contains whitespace so a user can
// recognise (and, if they choose, paste) it. It is display copy, not a shell
// escaping routine: nothing in kubecom feeds it to a shell.
func (p *ExecPlugin) CommandLine() string {
	if p == nil {
		return ""
	}
	parts := make([]string, 0, len(p.Args)+1)
	for _, tok := range append([]string{p.Command}, p.Args...) {
		if strings.ContainsAny(tok, " \t") {
			tok = "'" + tok + "'"
		}
		parts = append(parts, tok)
	}
	return strings.Join(parts, " ")
}

// ExecPluginFor reports the exec credential plugin the context selected by cc
// uses, or nil when that context authenticates by any other means (token, client
// certificate, basic auth) — which is the common case and is not an error.
//
// It performs no network I/O, executes nothing, and never panics: an unreadable
// or malformed kubeconfig returns an error tagged so Classify reports
// KindBadContext (#86, principle 3), and a context (or user) the kubeconfig does
// not declare returns nil with no error, since a missing plugin and an
// unresolvable context are both simply "no plugin to name".
func ExecPluginFor(cc ClientConfig) (*ExecPlugin, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cc.Kubeconfig != "" {
		rules.ExplicitPath = cc.Kubeconfig
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		// Tagged like RESTConfig's and Contexts' failures so a broken kubeconfig
		// classifies identically whichever entry point hit it first.
		return nil, fmt.Errorf("kube: loading kubeconfig for exec plugin: %w: %w", errBadContext, err)
	}
	// The same selection rule RESTConfig/ContextName/Contexts use: explicit
	// override wins, else the file's current-context.
	selected := cc.Context
	if selected == "" {
		selected = raw.CurrentContext
	}
	ctx, ok := raw.Contexts[selected]
	if !ok || ctx == nil {
		return nil, nil
	}
	auth, ok := raw.AuthInfos[ctx.AuthInfo]
	if !ok || auth == nil || auth.Exec == nil {
		return nil, nil
	}
	return execPluginFrom(selected, ctx.AuthInfo, auth.Exec), nil
}

// execPluginFrom converts a kubeconfig exec stanza into an ExecPlugin. Split out
// so tests can build one without a file, and so the field mapping lives in one
// place.
func execPluginFrom(context, user string, ec *clientcmdapi.ExecConfig) *ExecPlugin {
	if ec == nil {
		return nil
	}
	p := &ExecPlugin{
		Context:     context,
		User:        user,
		Command:     ec.Command,
		APIVersion:  ec.APIVersion,
		InstallHint: ec.InstallHint,
	}
	if len(ec.Args) > 0 {
		p.Args = append([]string(nil), ec.Args...)
	}
	for _, e := range ec.Env {
		if p.Env == nil {
			p.Env = make(map[string]string, len(ec.Env))
		}
		p.Env[e.Name] = e.Value
	}
	return p
}

// ExecPluginFailure is what an exec credential plugin failure can be recovered
// from the error client-go returns. It is deliberately thin: client-go formats
// the plugin failure with %v (`getting credentials: %v`), so the underlying
// *exec.ExitError does **not** survive in the wrap chain and the plugin's own
// stderr is not in the error at all — client-go streams that straight to the
// process's os.Stderr, which under the alt-screen the user never sees. Text is
// therefore all there is; anything richer has to re-run the plugin.
type ExecPluginFailure struct {
	// Command is the executable name as client-go reported it (the kubeconfig's
	// `command`, not a resolved path).
	Command string
	// ExitCode is the plugin's exit status. Zero when NotFound is true or the
	// status could not be parsed.
	ExitCode int
	// NotFound is true when the binary itself is missing (not on PATH) rather
	// than present and failing — a different remediation entirely: install it,
	// don't re-authenticate.
	NotFound bool
}

// execPluginErrRe matches the two failure messages client-go's
// Authenticator.wrapCmdRunErrorLocked produces (plugin/pkg/client/auth/exec):
//
//	exec: executable aws not found
//	exec: executable aws failed with exit code 255
//
// Its third, default branch (`exec: %v`) is deliberately **not** matched: it
// carries no executable name and its text is whatever os/exec produced, so
// matching it would be guessing. Those stay KindUnknown/KindUnreachable.
var execPluginErrRe = regexp.MustCompile(`exec: executable (\S+) (not found|failed with exit code (\d+))`)

// ExecPluginFailed reports whether err came from an exec credential plugin that
// client-go could not run, and what it could recover about it. It matches on the
// error's rendered text because the chain is broken by %v formatting (see
// ExecPluginFailure) — narrowly, on client-go's own two message shapes, so an
// unrecognised auth failure stays a plain auth failure rather than being dressed
// up as a plugin problem.
func ExecPluginFailed(err error) (ExecPluginFailure, bool) {
	if err == nil {
		return ExecPluginFailure{}, false
	}
	m := execPluginErrRe.FindStringSubmatch(err.Error())
	if m == nil {
		return ExecPluginFailure{}, false
	}
	f := ExecPluginFailure{Command: m[1]}
	if m[2] == "not found" {
		f.NotFound = true
		return f, true
	}
	// m[3] is \d+ by construction; a value too large for int is not a real exit
	// status, so fall back to reporting the failure without a code.
	if code, convErr := strconv.Atoi(m[3]); convErr == nil {
		f.ExitCode = code
	}
	return f, true
}

// errExecPlugin tags an error as an exec-credential-plugin failure. Nothing in
// the kube layer constructs plugin failures itself — they arrive from client-go
// as opaque text — but the sentinel exists so a future layer that *does* wrap one
// (e.g. a diagnostic re-run of the plugin) classifies without re-matching text.
var errExecPlugin = errors.New("exec credential plugin failed")

// execDiagnosisTimeout bounds a diagnostic re-run. A credential plugin that has
// not answered in this long is not going to: the interactive ones (`aws sso
// login`) are a *remediation* (AUTH-05), not this, and a token mint that blocks
// is itself the diagnosis. Generous enough for a cold `aws eks get-token` on a
// slow link, short enough that a wedged binary cannot hold a keystroke's worth
// of UI work open.
const execDiagnosisTimeout = 15 * time.Second

// execDiagnosisWaitDelay bounds the wait *after* the process is killed. Killing
// the plugin does not necessarily close its stderr: a plugin that spawned a
// child (a shell wrapper, a helper) leaves that child holding the pipe, and
// os/exec's Wait blocks on the copy until it exits — so without this a wedged
// plugin's *grandchild* outlives execDiagnosisTimeout unbounded. With it, the
// pipe is closed and whatever was captured so far is returned.
const execDiagnosisWaitDelay = 2 * time.Second

// execDiagnosisStderrLimit caps the captured stderr. Plugins that fail are
// terse; the ones that are not (a Python traceback, a `--debug` firehose) would
// otherwise be carried around whole for a pane that shows a dozen lines.
const execDiagnosisStderrLimit = 8 << 10 // 8 KiB

// ExecPluginDiagnosis is what a diagnostic re-run of an exec credential plugin
// recovered — above all **Stderr**, the plugin's own account of why it failed,
// which is the one thing the failure error cannot carry (D195 pt 3).
//
// A diagnosis is an observation, not a verdict: a re-run that *succeeds* is a
// real and useful outcome (the credential expired between the failed request and
// the diagnosis, or another terminal re-authenticated in between), so callers
// must check Failed rather than assuming the re-run reproduces the failure.
type ExecPluginDiagnosis struct {
	// CommandLine is the invocation as it was run, rendered for display
	// (ExecPlugin.CommandLine). It is what a remediation message quotes.
	CommandLine string
	// Stderr is the plugin's captured standard error, trailing whitespace
	// trimmed. Empty when the plugin said nothing — a plugin is allowed to fail
	// silently, and an empty diagnosis is reported as empty rather than padded.
	Stderr string
	// ExitCode is the plugin's exit status: 0 when the re-run succeeded, -1 when
	// it was killed by a signal.
	ExitCode int
	// NotFound is true when the binary could not be found or executed at all.
	NotFound bool
	// TimedOut is true when the re-run outlived execDiagnosisTimeout (or the
	// caller's own deadline) and was killed.
	TimedOut bool
	// Truncated is true when Stderr was cut at execDiagnosisStderrLimit.
	Truncated bool
}

// Failed reports whether the re-run reproduced a failure.
func (d ExecPluginDiagnosis) Failed() bool {
	return d.NotFound || d.TimedOut || d.ExitCode != 0
}

// Diagnose re-runs the credential plugin and captures its stderr, which is the
// only way to learn *why* it failed: client-go streams the plugin's stderr to the
// process's own os.Stderr — invisible under the alt-screen — and the error it
// returns carries nothing but the executable name and an exit code (D195 pt 3).
//
// It is a **diagnostic on an already-failed request**, never a pre-flight: call
// it after a request failed with KindExecPlugin, and never on a normal launch. It
// is read-only by construction (D195 pt 3): it re-invokes the kubeconfig's *own*
// stanza — exactly p.Command with exactly p.Args, never a command kubecom
// composed. The remediation an operator would type is a *different* command; it
// is offered, never run, and only behind a confirm (D195 pt 4).
//
// Four properties are load-bearing (D211):
//
//   - **Stdin is closed**, so a plugin that would prompt fails instead of hanging
//     on a terminal the TUI owns.
//   - **Stdout is discarded, never captured.** A credential plugin's stdout is an
//     ExecCredential — a live bearer token. Nothing about why it failed lives
//     there, so it is read and dropped rather than returned into a struct that
//     ends up in a pane, a log line or a bug report.
//   - **The run is bounded twice**: execDiagnosisTimeout kills the process, and
//     execDiagnosisWaitDelay is what actually ends the wait, since a killed
//     plugin's child can still hold the stderr pipe open.
//   - **The capture is bounded** by execDiagnosisStderrLimit, and says so
//     (Truncated) rather than silently returning a prefix.
//
// The returned error is non-nil only when the diagnostic could not be *attempted*
// (no stanza, or the caller's context was cancelled). A plugin that ran and
// failed is a successful diagnosis: the detail is in the ExecPluginDiagnosis.
func (p *ExecPlugin) Diagnose(ctx context.Context) (ExecPluginDiagnosis, error) {
	if p == nil || strings.TrimSpace(p.Command) == "" {
		return ExecPluginDiagnosis{}, fmt.Errorf("kube: no exec credential plugin to diagnose: %w", errBadContext)
	}
	d := ExecPluginDiagnosis{CommandLine: p.CommandLine()}

	ctx, cancel := context.WithTimeout(ctx, execDiagnosisTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.Command, p.Args...) //nolint:gosec // the kubeconfig's own credential command, run verbatim; never composed, never shelled.
	cmd.Env = p.environ(os.Environ())
	cmd.Stdin = nil // non-interactive: os/exec connects the null device.
	cmd.Stdout = io.Discard
	stderr := &cappedBuffer{limit: execDiagnosisStderrLimit}
	cmd.Stderr = stderr
	cmd.WaitDelay = execDiagnosisWaitDelay

	runErr := cmd.Run()
	d.Stderr = strings.TrimRight(stderr.String(), " \t\r\n")
	d.Truncated = stderr.truncated

	switch {
	case runErr == nil:
		return d, nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		d.TimedOut = true
	case ctx.Err() != nil:
		// The caller went away; nothing was diagnosed.
		return ExecPluginDiagnosis{}, fmt.Errorf("kube: diagnosing exec credential plugin %q: %w", p.Command, ctx.Err())
	case errors.Is(runErr, exec.ErrNotFound), errors.Is(runErr, fs.ErrNotExist), errors.Is(runErr, fs.ErrPermission):
		d.NotFound = true
	default:
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			// Not an exit status and not a missing binary: the process could not
			// be started for some other reason. Tagged so Classify reports
			// KindExecPlugin without re-matching text.
			return ExecPluginDiagnosis{}, fmt.Errorf("kube: diagnosing exec credential plugin %q: %w: %w", p.Command, errExecPlugin, runErr)
		}
		d.ExitCode = exitErr.ExitCode()
	}
	return d, nil
}

// environ merges the stanza's explicit overrides onto base (the process
// environment), override winning. os/exec's own de-duplication keeps the last
// occurrence of a name, but relying on that would make the precedence a property
// of the standard library rather than of this function, so the merge is explicit
// and the result deterministic (overrides appended in name order).
//
// Only the stanza's *own* env is applied: client-go passes the plugin the process
// environment plus the stanza's, and reproducing anything more (a provider's
// implicit defaults, say) would make the diagnostic a different invocation from
// the one that failed.
func (p *ExecPlugin) environ(base []string) []string {
	if len(p.Env) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(p.Env))
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if ok {
			if _, overridden := p.Env[name]; overridden {
				continue
			}
		}
		out = append(out, kv)
	}
	names := make([]string, 0, len(p.Env))
	for n := range p.Env {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		out = append(out, n+"="+p.Env[n])
	}
	return out
}

// cappedBuffer accumulates at most limit bytes and reports whether it dropped
// any. Writes always report full consumption: a child that overruns the cap must
// keep running to its own exit, not die of a short write on its stderr pipe.
type cappedBuffer struct {
	limit     int
	buf       bytes.Buffer
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	switch room := c.limit - c.buf.Len(); {
	case room <= 0:
		if n > 0 {
			c.truncated = true
		}
	case n > room:
		c.buf.Write(p[:room])
		c.truncated = true
	default:
		c.buf.Write(p)
	}
	return n, nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
