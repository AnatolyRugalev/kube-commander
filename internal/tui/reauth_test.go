package tui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/modal"
)

// recordedRun is what the suspended remediation was asked to run, captured instead of
// being executed. The whole point of AUTH-05a's package var is that no test spawns
// `aws sso login`.
type recordedRun struct {
	calls  int
	cmd    kube.RemediationCommand
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	print  string // written to the stderr it was handed, as a failing CLI would
	err    error
}

// withReauthRun swaps runReauthCommand for a recorder for the duration of the test.
func withReauthRun(t *testing.T, r *recordedRun) {
	t.Helper()
	prev := runReauthCommand
	runReauthCommand = func(rc kube.RemediationCommand, stdin io.Reader, stdout, stderr io.Writer) error {
		r.calls++
		r.cmd, r.stdin, r.stdout, r.stderr = rc, stdin, stdout, stderr
		if r.print != "" {
			_, _ = io.WriteString(stderr, r.print)
		}
		return r.err
	}
	t.Cleanup(func() { runReauthCommand = prev })
}

// armedPods is a model browsing Pods over a fake watcher with the SSO remediation
// armed — the state AUTH-05b's accepted confirm will leave behind.
func armedPods(t *testing.T) (Model, *fakeWatcher) {
	t.Helper()
	m, fw := browsingPods(t, &fakeAuthDiagnoser{})
	if !m.armReauth(podResource(), ssoReport(t)) {
		t.Fatal("armReauth declined the SSO report the kube layer substantiates a fix for")
	}
	return m, fw
}

// The stash is the resolved command, not the report: the argv the kube layer composed,
// the line a confirm will quote, and the resource whose request failed.
func TestArmReauthStashesTheResolvedCommand(t *testing.T) {
	m, _ := armedPods(t)
	if !m.hasReauth {
		t.Fatal("hasReauth = false after arming")
	}
	want := []string{"aws", "sso", "login", "--profile", "acme-prod"}
	if strings.Join(m.reauthCmd.Argv, " ") != strings.Join(want, " ") {
		t.Errorf("Argv = %q, want %q", m.reauthCmd.Argv, want)
	}
	if m.reauthCmd.Line != "aws sso login --profile acme-prod" {
		t.Errorf("Line = %q, want the command a confirm would quote", m.reauthCmd.Line)
	}
	if m.reauthRes.GVK.Kind != "Pod" {
		t.Errorf("stashed resource = %+v, want the Pod whose request failed", m.reauthRes)
	}
	if len(m.reauthCmd.Env) == 0 {
		t.Error("Env is empty: the login must run in the plugin's environment, not a bare one")
	}
}

// A report that substantiates nothing must *clear* the stash rather than leave the
// previous one armed: the pane now explains a different failure, and a stale approval
// target is the one thing that must never survive to a run.
func TestArmReauthClearsWhenNothingIsSubstantiated(t *testing.T) {
	m, _ := armedPods(t)
	unrecognised := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{
		Stderr:   "An error occurred (AccessDeniedException) when calling the DescribeCluster operation",
		ExitCode: 254,
	})
	if unrecognised.Suggested {
		t.Fatal("precondition: this failure must not be recognised")
	}
	if m.armReauth(podResource(), unrecognised) {
		t.Error("armReauth = true for a report with no substantiated remediation")
	}
	if m.hasReauth || len(m.reauthCmd.Argv) != 0 {
		t.Errorf("stash survived: hasReauth=%v argv=%q", m.hasReauth, m.reauthCmd.Argv)
	}
}

// Nothing armed → nothing runs. This is every state until AUTH-05b's confirm is
// accepted, and it is what keeps an empty argv away from os/exec.
func TestRunReauthWithNothingArmedRunsNothing(t *testing.T) {
	rec := &recordedRun{}
	withReauthRun(t, rec)
	m := sizedWith(t)
	if _, cmd := m.runReauth(); cmd != nil {
		t.Error("runReauth issued a suspend with nothing armed")
	}
	// The same for a stash that was armed with an empty argv (a report that cannot
	// happen through armReauth, asserted so the guard is not merely decorative).
	m.hasReauth = true
	if _, cmd := m.runReauth(); cmd != nil {
		t.Error("runReauth issued a suspend for an empty argv")
	}
	if rec.calls != 0 {
		t.Errorf("the remediation ran %d times, want 0", rec.calls)
	}
}

// The approval covers exactly one run (D195 pt 4: never retried automatically), so
// firing consumes the stash and a second attempt does nothing.
func TestRunReauthConsumesTheApproval(t *testing.T) {
	m, _ := armedPods(t)
	next, cmd := m.runReauth()
	if cmd == nil {
		t.Fatal("an armed remediation should issue the tea.Exec suspend")
	}
	m = next.(Model)
	if m.hasReauth {
		t.Error("the approval survived its run")
	}
	if _, again := m.runReauth(); again != nil {
		t.Error("runReauth fired a second time without a second approval")
	}
}

// The adapter runs the argv and environment the kube layer resolved, verbatim, on the
// suspended terminal's own streams — that last part is why an interactive login works
// at all.
func TestReauthCommandRunsTheResolvedCommandOnTheTerminal(t *testing.T) {
	rec := &recordedRun{}
	withReauthRun(t, rec)
	stdin, stdout, stderr := strings.NewReader("\n"), &bytes.Buffer{}, &bytes.Buffer{}
	rc := kube.RemediationCommand{
		Argv: []string{"aws", "sso", "login", "--profile", "acme-prod"},
		Env:  []string{"AWS_CONFIG_FILE=/work/aws-config"},
		Line: "aws sso login --profile acme-prod",
	}
	c := &reauthCommand{cmd: rc}
	c.SetStdin(stdin)
	c.SetStdout(stdout)
	c.SetStderr(stderr)
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if rec.calls != 1 {
		t.Fatalf("the remediation ran %d times, want 1", rec.calls)
	}
	if strings.Join(rec.cmd.Argv, " ") != strings.Join(rc.Argv, " ") {
		t.Errorf("ran %q, want %q", rec.cmd.Argv, rc.Argv)
	}
	if strings.Join(rec.cmd.Env, " ") != strings.Join(rc.Env, " ") {
		t.Errorf("environment = %q, want the resolved one %q", rec.cmd.Env, rc.Env)
	}
	if rec.stdin != stdin || rec.stdout != stdout {
		t.Error("the command must own the terminal's stdin/stdout: a login prompts and prints a code")
	}
}

// stderr is teed, not replaced: the reader still sees the failure on their terminal,
// and the copy is what the toast can quote once bubbletea has repainted over it.
func TestReauthCommandTeesStderr(t *testing.T) {
	rec := &recordedRun{print: "Error loading SSO Token: Token does not exist\n", err: errors.New("exit status 1")}
	withReauthRun(t, rec)
	terminal := &bytes.Buffer{}
	c := &reauthCommand{cmd: kube.RemediationCommand{Argv: []string{"aws"}}}
	c.SetStderr(terminal)
	if err := c.Run(); err == nil {
		t.Fatal("Run should return the command's failure")
	}
	if !strings.Contains(terminal.String(), "Error loading SSO Token") {
		t.Errorf("the terminal saw %q, want the command's own output", terminal.String())
	}
	if !strings.Contains(c.capturedStderr(), "Error loading SSO Token") {
		t.Errorf("captured %q, want the command's own output", c.capturedStderr())
	}
}

// A command with no stderr writer wired (nothing set the stream) must still capture,
// and must not nil-panic on the way.
func TestReauthCommandCapturesWithoutATerminal(t *testing.T) {
	rec := &recordedRun{print: "boom\n"}
	withReauthRun(t, rec)
	c := &reauthCommand{cmd: kube.RemediationCommand{Argv: []string{"aws"}}}
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if got := strings.TrimSpace(c.capturedStderr()); got != "boom" {
		t.Errorf("captured %q, want boom", got)
	}
}

// The capture is bounded, and a full buffer must never turn a working command into a
// failing one: capWriter reports every write as accepted (a short write is an error to
// io.MultiWriter, which would propagate to Run and be reported as a failed login).
func TestCapWriterBoundsWithoutFailingTheWrite(t *testing.T) {
	w := &capWriter{limit: 8}
	n, err := w.Write([]byte("0123456789abcdef"))
	if err != nil || n != 16 {
		t.Fatalf("Write = %d, %v; want 16, nil", n, err)
	}
	if w.String() != "01234567" {
		t.Errorf("captured %q, want the first 8 bytes", w.String())
	}
	if n, err = w.Write([]byte("more")); err != nil || n != 4 {
		t.Fatalf("Write past the limit = %d, %v; want 4, nil", n, err)
	}
	if w.String() != "01234567" {
		t.Errorf("captured %q after a second write, want the head unchanged", w.String())
	}
}

// The failure toast names the command that failed and the first thing it printed —
// "exit status 1" alone is the nameless auth error this whole line exists to remove.
func TestReauthErrorNamesTheCommandAndTheFirstLine(t *testing.T) {
	got := reauthError("aws sso login --profile acme-prod",
		"Error loading SSO Token: Token does not exist\nusage: aws [options] <command>",
		errors.New("exit status 255")).Error()
	for _, want := range []string{"aws sso login --profile acme-prod", "exit status 255", "Error loading SSO Token"} {
		if !strings.Contains(got, want) {
			t.Errorf("message = %q, want it to contain %q", got, want)
		}
	}
	// The tail is usually a usage dump; it is in the log line, not the toast.
	if strings.Contains(got, "usage: aws") {
		t.Errorf("message = %q, want only the first printed line", got)
	}
	// A command that failed silently still reports what failed and how.
	silent := reauthError("aws sso login", "\n  \n", errors.New("exit status 1")).Error()
	if !strings.Contains(silent, "aws sso login") || !strings.Contains(silent, "exit status 1") {
		t.Errorf("silent failure = %q, want the command and its status", silent)
	}
}

// A failed login degrades to a toast and retries nothing: it would fail the same way
// until the reader does something about it, and the pane's notice already says what is
// wrong.
func TestReauthFailureToastsAndDoesNotRetry(t *testing.T) {
	m, fw := armedPods(t)
	watches := len(fw.res)
	next, cmd := m.handleReauthDone(reauthDoneMsg{
		line:   "aws sso login --profile acme-prod",
		res:    podResource(),
		stderr: "Error loading SSO Token: Token does not exist",
		err:    errors.New("exit status 255"),
	})
	if cmd == nil {
		t.Fatal("a failed remediation should surface a toast")
	}
	m = next.(Model)
	if !strings.Contains(m.status.View(), "re-authenticate") {
		t.Errorf("the status bar should report the failed re-authentication:\n%s", m.status.View())
	}
	if len(fw.res) != watches {
		t.Errorf("watches started = %d, want %d: a failed login must not retry the request", len(fw.res), watches)
	}
}

// Success retries the *request*, not the launch: the failed resource's watch is
// restarted, which is also what re-arms the diagnosis latch so a login that did not
// actually fix the credentials produces a fresh diagnosis rather than silence.
func TestReauthSuccessRetriesTheFailedRequest(t *testing.T) {
	m, fw := armedPods(t)
	watches, gen := len(fw.res), m.watchGen
	next, cmd := m.handleReauthDone(reauthDoneMsg{line: "aws sso login --profile acme-prod", res: podResource()})
	m = next.(Model)
	if len(fw.res) != watches+1 {
		t.Fatalf("watches started = %d, want %d (one retry)", len(fw.res), watches+1)
	}
	if got := fw.res[len(fw.res)-1]; got.GVK.Kind != "Pod" {
		t.Errorf("retried %+v, want the Pod whose request failed", got)
	}
	if m.watchGen <= gen {
		t.Errorf("watchGen = %d, want it bumped past %d so the diagnosis latch re-arms", m.watchGen, gen)
	}
	if m.authDiagGen == m.watchGen {
		t.Error("the diagnosis latch is still closed on the retried selection")
	}
	if cmd == nil {
		t.Fatal("a successful remediation should report the outcome and pump the new watch")
	}
	if !strings.Contains(m.status.View(), "re-authenticated") {
		t.Errorf("the status bar should report the successful re-authentication:\n%s", m.status.View())
	}
}

// Nothing to retry — a watch-inert model, or an outcome that names no resource — still
// reports the outcome instead of panicking (principle 3).
func TestReauthSuccessWithNothingToRetryStillReports(t *testing.T) {
	m := sizedWith(t)
	next, cmd := m.handleReauthDone(reauthDoneMsg{line: "aws sso login"})
	if cmd == nil {
		t.Fatal("the outcome should still be surfaced")
	}
	if !strings.Contains(next.(Model).status.View(), "re-authenticated") {
		t.Error("the status bar should report the successful re-authentication")
	}
}

// The outcome arrives as a message from the suspend's callback, so it has to be routed
// by Update — not merely handled by a method a test can call.
func TestReauthDoneRoutesThroughUpdate(t *testing.T) {
	withReauthRun(t, &recordedRun{})
	m, fw := armedPods(t)
	watches := len(fw.res)
	suspended, cmd := m.runReauth()
	if cmd == nil {
		t.Fatal("an armed remediation should issue the tea.Exec suspend")
	}
	m = suspended.(Model)
	if m.hasReauth {
		t.Error("the stash should be gone once the run has been issued")
	}
	next, _ := m.Update(reauthDoneMsg{line: "aws sso login --profile acme-prod", res: podResource()})
	if len(fw.res) != watches+1 {
		t.Fatalf("watches started = %d, want %d: Update should route the outcome", len(fw.res), watches+1)
	}
	if !strings.Contains(next.(Model).status.View(), "re-authenticated") {
		t.Error("the status bar should report the outcome Update routed")
	}
}

// A context switch drops the stash: it names the departing context's credentials and a
// resource on the cluster being left, and running it after the switch would
// re-authenticate for a cluster nobody is looking at.
func TestResetClusterDropsTheArmedRemediation(t *testing.T) {
	m, _ := armedPods(t)
	m.resetCluster()
	if m.hasReauth || len(m.reauthCmd.Argv) != 0 || m.reauthRes.GVK.Kind != "" {
		t.Errorf("stash survived the switch: hasReauth=%v argv=%q res=%+v",
			m.hasReauth, m.reauthCmd.Argv, m.reauthRes)
	}
}

// ---- AUTH-05b: the offer ----------------------------------------------------

// offeredPods drives the whole AUTH line end to end on fakes: browse Pods, fail the
// watch the way an expired SSO session does, run the diagnosis that failure issues,
// and feed its answer back — which is the only path that opens an offer. Nothing
// short of this arms one, which is the property AUTH-05a shipped and this leg keeps.
func offeredPods(t *testing.T) (Model, *fakeWatcher) {
	t.Helper()
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, fw := browsingPods(t, fd)
	m, cmd := failWatch(t, m, execPluginErr())
	next, _ := m.Update(firstAuthDiagMsg(t, cmd))
	return next.(Model), fw
}

// The headline: a diagnosis that substantiates a fix asks the reader whether to run
// it, naming the exact command (D195 pt 4) — and the pane stops telling them to go to
// another terminal, because the prompt in front of them is the other half of the same
// sentence.
func TestDiagnosisOffersToRunTheRemediation(t *testing.T) {
	m, _ := offeredPods(t)

	if !m.modal.Active() || m.modal.Kind() != reauthModalKind {
		t.Fatalf("no offer opened: active=%v kind=%q", m.modal.Active(), m.modal.Kind())
	}
	if !m.hasReauth {
		t.Error("the offer is on screen with nothing armed for it to approve")
	}
	if view := m.modal.View(); !strings.Contains(view, "aws sso login") {
		t.Errorf("the prompt does not name the command it would run:\n%s", view)
	}
	notice := m.table.Notice()
	if !strings.Contains(notice, "kubecom is asking whether to run this for you:") {
		t.Errorf("the pane does not name the open prompt:\n%s", notice)
	}
	if strings.Contains(notice, "Run this in another terminal") {
		t.Errorf("the pane still sends the reader to another terminal while offering to do it:\n%s", notice)
	}
	if !strings.Contains(notice, "aws sso login --profile acme-prod") {
		t.Errorf("the pane must still carry the full command — the prompt box clips it:\n%s", notice)
	}
}

// Accepting runs the remediation exactly once, and the pane goes back to the
// self-service copy: the prompt it named is gone the moment it is answered.
func TestAcceptingTheOfferRunsTheRemediation(t *testing.T) {
	withReauthRun(t, &recordedRun{})
	m, _ := offeredPods(t)

	next, cmd := m.Update(modal.ConfirmedMsg{Kind: reauthModalKind})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("accepting the offer issued no suspend")
	}
	if m.modal.Active() {
		t.Error("the offer is still on screen after being answered")
	}
	if m.hasReauth {
		t.Error("the approval survived the run it authorised")
	}
	if notice := m.table.Notice(); !strings.Contains(notice, "Run this in another terminal") {
		t.Errorf("the pane still names a prompt that has closed:\n%s", notice)
	}
	if _, again := m.Update(modal.ConfirmedMsg{Kind: reauthModalKind}); again != nil {
		t.Error("a second confirm ran a second remediation without a second offer")
	}
}

// Declining runs nothing at all — and drops the approval, because an unanswered one
// may not linger (D215 pt 4). The pane returns to the copy that assumes the reader
// will do it themselves.
func TestDecliningTheOfferRunsNothing(t *testing.T) {
	rec := &recordedRun{}
	withReauthRun(t, rec)
	m, fw := offeredPods(t)
	watches := len(fw.res)

	next, cmd := m.Update(modal.CancelledMsg{Kind: reauthModalKind})
	m = next.(Model)
	if cmd != nil {
		t.Errorf("declining issued work: %T", cmd())
	}
	if rec.calls != 0 {
		t.Errorf("the remediation ran %d times after a decline, want 0", rec.calls)
	}
	if m.hasReauth {
		t.Error("a declined approval stayed armed")
	}
	if m.modal.Active() {
		t.Error("the declined offer is still on screen")
	}
	if len(fw.res) != watches {
		t.Error("declining retried the request")
	}
	notice := m.table.Notice()
	if !strings.Contains(notice, "Run this in another terminal") || strings.Contains(notice, "kubecom is asking") {
		t.Errorf("the pane did not go back to the self-service copy:\n%s", notice)
	}
}

// A diagnosis lands whenever it lands; every other surface is one the reader opened
// deliberately. So an offer never opens over one — and it must not, because a confirm
// is only ever composited over the plain browse view: an offer opened under the logs
// view would be an invisible modal swallowing every key.
func TestOfferDoesNotInterruptAnotherSurface(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, _ := browsingPods(t, fd)
	m, cmd := failWatch(t, m, execPluginErr())
	diag := firstAuthDiagMsg(t, cmd)

	// The reader opened a delete confirm while the plugin was being re-run.
	m.modal.ShowConfirm(deleteModalKind, "Delete", "Delete Pod default/web-1?")
	next, _ := m.Update(diag)
	m = next.(Model)

	if m.modal.Kind() != deleteModalKind {
		t.Errorf("the offer replaced the question the reader was answering: kind=%q", m.modal.Kind())
	}
	if m.hasReauth {
		t.Error("a remediation was armed with no offer to approve it")
	}
	if notice := m.table.Notice(); !strings.Contains(notice, "Run this in another terminal") {
		t.Errorf("the pane must keep the self-service copy when nothing was offered:\n%s", notice)
	}
}

// A diagnosis that substantiates no command has nothing to offer: the pane says so
// (AUTH-04a) and no prompt appears. Asking "shall I run nothing?" is worse than
// silence.
func TestNoOfferWhenNothingIsSubstantiated(t *testing.T) {
	unrecognised := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{
		Stderr:   "An error occurred (AccessDeniedException) when calling the DescribeCluster operation",
		ExitCode: 254,
	})
	if unrecognised.Suggested {
		t.Fatal("precondition: this failure must not be recognised")
	}
	fd := &fakeAuthDiagnoser{rep: unrecognised}
	m, _ := browsingPods(t, fd)
	m, cmd := failWatch(t, m, execPluginErr())
	next, _ := m.Update(firstAuthDiagMsg(t, cmd))
	m = next.(Model)

	if m.modal.Active() {
		t.Errorf("an offer opened for a report that substantiates nothing: kind=%q", m.modal.Kind())
	}
	if m.hasReauth {
		t.Error("something was armed for a report that substantiates nothing")
	}
	if !strings.Contains(m.table.Notice(), "no command it can suggest") {
		t.Errorf("the pane should say it has nothing to suggest:\n%s", m.table.Notice())
	}
}

// The restore is scoped to the text the offer wrote: by the time the reader answers,
// the pane may be explaining something else entirely, and putting this offer's notice
// back would resurrect an explanation the shell already retired.
func TestAnsweringDoesNotOverwriteANewerNotice(t *testing.T) {
	m, _ := offeredPods(t)
	m.table.SetNotice("Cannot list Pod\nThe API server could not be reached.")

	next, _ := m.Update(modal.CancelledMsg{Kind: reauthModalKind})
	m = next.(Model)
	if !strings.Contains(m.table.Notice(), "could not be reached") {
		t.Errorf("answering the offer overwrote a newer failure's notice:\n%s", m.table.Notice())
	}
	if m.hasReauth {
		t.Error("the declined approval stayed armed")
	}
}

// The question names the cause and the exact invocation, and says where it runs —
// accepting blanks the TUI and hands the terminal to a browser flow, which is not
// what a reader expects a confirm to do.
func TestReauthQuestionNamesTheExactCommand(t *testing.T) {
	got := reauthQuestion(kube.RemediationCommand{
		Line:  "aws sso login --profile acme-prod",
		Cause: "Your AWS SSO session for profile acme-prod has expired.",
	})
	for _, want := range []string{"aws sso login --profile acme-prod", "expired", "this terminal"} {
		if !strings.Contains(got, want) {
			t.Errorf("the question %q does not carry %q", got, want)
		}
	}
	// A remediation with no cause still asks a complete question.
	bare := reauthQuestion(kube.RemediationCommand{Line: "aws sso login"})
	if !strings.Contains(bare, "aws sso login") || !strings.Contains(bare, "Run this now") {
		t.Errorf("the bare question = %q", bare)
	}
}

// A context switch drops the offer's pane text along with the approval: both name the
// departing context, and resetCluster is what hides the prompt itself.
func TestResetClusterDropsTheOffer(t *testing.T) {
	m, _ := offeredPods(t)
	m.resetCluster()
	if m.modal.Active() || m.hasReauth || m.reauthOffer.shown != "" {
		t.Errorf("the offer survived the switch: active=%v armed=%v shown=%q",
			m.modal.Active(), m.hasReauth, m.reauthOffer.shown)
	}
}
