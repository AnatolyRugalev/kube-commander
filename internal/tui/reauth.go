package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is AUTH-05a: what happens once the reader has approved the remediation
// AUTH-04b's diagnosis printed — the run, and the retry of the request that failed.
// The **offer** that produces the approval is AUTH-05b; until it lands nothing here
// is reachable outside tests, which is deliberate: no code path arms a remediation,
// so nothing runs unasked (D195 pt 4).
//
// Three properties this file owns:
//
//   - **It is the existing suspend, not a new one.** A remediation in the catalogue
//     is interactive by construction — `aws sso login` opens a browser and prints a
//     verification code to a terminal it expects someone to be reading — so it runs
//     through tea.Exec with the released terminal's own streams, exactly as $EDITOR
//     does (D125). It has no timeout for the same reason: the reader is watching it,
//     and a browser round-trip is legitimately slow.
//   - **The argv and the environment come from the kube layer**
//     (kube.ExecPluginReport.RemediationCommand). The shell runs the command; it
//     never composes one, so what runs cannot differ from what the confirm named.
//   - **The approval is single-use.** runReauth consumes the stash, so the outcome —
//     success, failure, or a login the reader abandoned — leaves nothing armed. A
//     second run needs a second offer, which needs a second diagnosis (D195 pt 4:
//     never retried automatically).
//
// On success the *request* is retried, not the launch and not the connection: the
// failed browse watch is restarted for the resource that failed. That is enough
// because client-go does not negatively cache a credential — its exec Authenticator
// stores nothing when the plugin fails, so the next request runs the plugin again
// and picks up the session the login just wrote (see `vault/knowledge/stack.md`).

// reauthStderrLimit caps what a remediation's stderr contributes to the failure
// report. The output has already gone to the terminal the suspend released; this
// copy exists only so the toast and the log line can say *why* a login failed
// rather than "exit status 1", and a couple of lines is all either shows.
const reauthStderrLimit = 4 << 10 // 4 KiB

// runReauthCommand runs the approved remediation attached to the suspended
// terminal's streams and blocks until it exits. It is a package var so the whole
// flow is driveable in tests without spawning a process (mirroring runEditor,
// M3-15b); the default builds the *exec.Cmd.
//
// The argv is passed to os/exec verbatim: no shell, no expansion. The environment
// is the merged one the kube layer resolved (the process environment plus the
// failing stanza's own overrides) and replaces rather than extends it.
var runReauthCommand = func(rc kube.RemediationCommand, stdin io.Reader, stdout, stderr io.Writer) error {
	proc := exec.Command(rc.Argv[0], rc.Argv[1:]...) //nolint:gosec // argv is composed by the kube layer from the kubeconfig's own stanza and approved by the reader; never shelled.
	proc.Env = rc.Env
	proc.Stdin, proc.Stdout, proc.Stderr = stdin, stdout, stderr
	return proc.Run()
}

// reauthDoneMsg carries the outcome of a remediation once tea.Exec resumes the
// program. line is the command as the offer quoted it (so the outcome names what
// the question did), res the resource whose request failed — retried on success —
// and stderr the head of what the command printed, which is what makes a failure
// legible. err is nil on a clean exit.
type reauthDoneMsg struct {
	line   string
	res    kube.Resource
	stderr string
	err    error
}

// armReauth stashes the remediation a landed diagnosis substantiates, together with
// the resource whose request failed, and reports whether there is anything to offer.
// AUTH-05b calls it; the offer it opens is what leads to runReauth.
//
// A report that substantiates nothing clears the stash rather than leaving the
// previous one armed: the pane is now explaining a different failure, and a stale
// approval target is exactly what must never survive to a run.
//
// It mutates the receiver.
func (m *Model) armReauth(res kube.Resource, rep kube.ExecPluginReport) bool {
	rc, ok := rep.RemediationCommand(os.Environ())
	if !ok {
		m.clearReauth()
		return false
	}
	m.reauthCmd, m.reauthRes, m.hasReauth = rc, res, true
	return true
}

// clearReauth drops the armed remediation. Called when a run consumes it, when a
// diagnosis substantiates none, and from resetCluster — the stash names a context's
// credentials and a resource on the cluster being left.
//
// It mutates the receiver.
func (m *Model) clearReauth() {
	m.reauthCmd, m.reauthRes, m.hasReauth = kube.RemediationCommand{}, kube.Resource{}, false
}

// runReauth suspends into the approved remediation (AUTH-05a). It is a no-op with
// nothing armed — which is every path until AUTH-05b's confirm is accepted, and the
// guard that keeps an empty argv from reaching os/exec.
//
// It consumes the stash as it fires, so the reader's approval covers exactly this
// one run.
func (m Model) runReauth() (tea.Model, tea.Cmd) {
	if !m.hasReauth || len(m.reauthCmd.Argv) == 0 {
		return m, nil
	}
	rc, res := m.reauthCmd, m.reauthRes
	m.clearReauth()
	cmd := &reauthCommand{cmd: rc}
	callback := func(err error) tea.Msg {
		return reauthDoneMsg{line: rc.Line, res: res, stderr: cmd.capturedStderr(), err: err}
	}
	return m, tea.Exec(cmd, callback)
}

// handleReauthDone reports a finished remediation and, on success, retries the
// request that failed.
//
// A failure degrades to a transient toast naming the command and the first thing it
// printed (D74) — never a panic, never a retry of its own: a login that failed will
// fail again until the reader does something about it, and the pane's notice already
// says what is wrong. The full captured output is logged, since the toast is gone in
// five seconds and this is what a bug report is written from (D159).
//
// On success the browse watch for the failed resource is restarted, which re-issues
// exactly the request that could not authenticate. Restarting is also what re-arms
// the diagnosis latch (a fresh watchGen), so a login that did not actually fix the
// credentials produces a new diagnosis rather than silence.
func (m Model) handleReauthDone(msg reauthDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logger.Warn("remediation failed", "command", msg.line, "context", m.context,
			"error", msg.err, "stderr", msg.stderr)
		return m, m.surfaceError(NewErrorMsg("re-authenticate", reauthError(msg.line, msg.stderr, msg.err)))
	}
	m.logger.Info("remediation succeeded", "command", msg.line, "context", m.context)
	notice := m.surfaceNotice(reauthNotice(msg.res))
	// Nothing to retry: no watcher wired (a watch-inert model), or a remediation that
	// was armed without one — the reader still gets the outcome, and the watch loop's
	// own backoff re-lists if one is running.
	if m.watcher == nil || msg.res.GVR.Resource == "" {
		return m, notice
	}
	model, cmd := m.watchResource(msg.res)
	return model, tea.Batch(notice, cmd)
}

// reauthNotice is the neutral outcome line: the login worked, and here is what
// kubecom does next. It names the kind rather than the command — the command is the
// part that is over.
func reauthNotice(res kube.Resource) string {
	if res.GVK.Kind == "" {
		return "re-authenticated"
	}
	return "re-authenticated · retrying " + res.GVK.Kind
}

// reauthError is the failure the toast shows: the command that failed, its exit
// status, and the first line it printed. The first line, not the last, because a
// CLI's opening line is its complaint ("Error loading SSO Token: …") while the tail
// is usually a usage dump — and the whole capture is in the log line for the cases
// where it is not.
func reauthError(line, stderr string, err error) error {
	if first := firstNonBlankLine(stderr); first != "" {
		return fmt.Errorf("%s: %w: %s", line, err, first)
	}
	return fmt.Errorf("%s: %w", line, err)
}

// firstNonBlankLine is the first line of text with anything on it, trimmed. Empty
// when there is none — a command is allowed to fail silently.
func firstNonBlankLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// reauthCommand adapts the remediation to bubbletea's ExecCommand (tea.Exec), so it
// runs in the suspended terminal off the update loop — the same adapter shape exec
// and edit use (D124/D125). bubbletea releases the terminal, calls the Set* setters
// with the program's streams, runs Run() to completion, then re-captures.
//
// Unlike edit's, this Run does nothing but run the command: there is no buffer to
// diff and nothing to apply. What it adds is a *copy* of stderr, because the
// terminal the command printed to is repainted the instant bubbletea comes back —
// so without this the reason a login failed would be visible for a frame.
type reauthCommand struct {
	cmd kube.RemediationCommand

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	captured capWriter // the head of stderr, for the outcome report
}

// SetStdin/SetStdout/SetStderr capture the terminal streams bubbletea hands the
// command. All three are wanted: an interactive login prints to stdout, complains on
// stderr, and may wait on a keypress (`aws sso login` prompts before opening a
// browser), which is the whole reason this is a suspend and not a background Cmd.
func (c *reauthCommand) SetStdin(r io.Reader)  { c.stdin = r }
func (c *reauthCommand) SetStdout(w io.Writer) { c.stdout = w }
func (c *reauthCommand) SetStderr(w io.Writer) { c.stderr = w }

// Run runs the approved remediation to completion, teeing its stderr into the
// capture so the outcome can be reported after the terminal is repainted. The
// command still owns the real streams: the tee adds a reader, it does not replace
// one.
func (c *reauthCommand) Run() error {
	c.captured.limit = reauthStderrLimit
	stderr := io.Writer(&c.captured)
	if c.stderr != nil {
		stderr = io.MultiWriter(c.stderr, &c.captured)
	}
	return runReauthCommand(c.cmd, c.stdin, c.stdout, stderr)
}

// capturedStderr is the head of what the command printed on stderr. Read after Run
// (from the tea.Exec callback, which bubbletea calls once Run has returned).
func (c *reauthCommand) capturedStderr() string { return c.captured.String() }

// capWriter keeps the first limit bytes written through it and silently discards the
// rest, never erroring: it sits in a tee on the terminal's own stderr, and a failed
// capture must not turn into a failed command. The head is kept rather than the tail
// because a CLI leads with its complaint (see reauthError).
type capWriter struct {
	limit int
	buf   strings.Builder
}

// Write records what fits and reports the whole write as accepted.
func (w *capWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		w.buf.Write(p[:room])
	}
	return len(p), nil
}

// String is what was captured.
func (w *capWriter) String() string { return w.buf.String() }
