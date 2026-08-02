package kube

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
