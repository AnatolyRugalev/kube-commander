package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// fakeAuthDiagnoser is a hermetic AuthDiagnoser: it returns a preset report and
// records what it was asked, so a test can assert the shell handed it the
// kubeconfig and context it launched with — and never runs a subprocess.
type fakeAuthDiagnoser struct {
	rep   kube.ExecPluginReport
	err   error
	calls int
	ccs   []kube.ClientConfig
	ctxs  []context.Context
}

func (f *fakeAuthDiagnoser) DiagnoseExecPlugin(ctx context.Context, cc kube.ClientConfig) (kube.ExecPluginReport, error) {
	f.calls++
	f.ccs = append(f.ccs, cc)
	f.ctxs = append(f.ctxs, ctx)
	return f.rep, f.err
}

// ssoReport is the diagnosis of an expired AWS SSO session — the case the whole
// AUTH line was raised for — built through the kube layer so the test asserts on
// the answer the real producer gives.
func ssoReport(t *testing.T) kube.ExecPluginReport {
	t.Helper()
	rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{Stderr: ssoStderr, ExitCode: 255})
	if !rep.Suggested {
		t.Fatal("precondition: the kube layer should substantiate a remediation for this stanza")
	}
	return rep
}

// podResource is the kind these tests browse — with a GVK, since the pane's
// headline names it.
func podResource() kube.Resource {
	return kube.Resource{
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		GVK: schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
	}
}

// drillInto selects a resource and immediately **seals** the watch the shell just
// started, by closing the channel the fake handed it: a pump Cmd on a live-but-empty
// fake channel would block whoever runs it, and these tests do run the commands a
// browse failure issues. Each selection creates exactly one channel, sealed here, so
// nothing is closed twice.
func drillInto(t *testing.T, m Model, fw *fakeWatcher, r kube.Resource) Model {
	t.Helper()
	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: r})
	if n := len(fw.chans); n > 0 {
		close(fw.chans[n-1])
	}
	return next.(Model)
}

// browsingPods drills into Pods over a fake watcher, with the diagnoser wired and
// the shell holding the kubeconfig/context a real launch would have resolved.
func browsingPods(t *testing.T, fd *fakeAuthDiagnoser) (Model, *fakeWatcher) {
	t.Helper()
	fw := &fakeWatcher{}
	m := sizedWith(t,
		WithWatcher(fw),
		WithAuthDiagnoser(fd),
		WithKubeconfig("/home/u/.kube/config"),
		WithContext("acme-prod"),
	)
	return drillInto(t, m, fw, podResource()), fw
}

// firstAuthDiagMsg runs the command a browse failure issued and returns the
// diagnosis request among its messages. The leaves are run concurrently and the
// first wanted message wins, because the same batch also carries the five-second
// toast-clear Tick (surfaceError): draining it in order would make every test here
// sit out a timer none of them is about.
func firstAuthDiagMsg(t *testing.T, cmd tea.Cmd) authDiagMsg {
	t.Helper()
	msgs := make(chan tea.Msg, 16)
	var walk func(tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			switch msg := c().(type) {
			case tea.BatchMsg:
				for _, inner := range msg {
					walk(inner)
				}
			default:
				msgs <- msg
			}
		}()
	}
	walk(cmd)
	for deadline := time.After(2 * time.Second); ; {
		select {
		case msg := <-msgs:
			if d, ok := msg.(authDiagMsg); ok {
				return d
			}
		case <-deadline:
			t.Fatal("no authDiagMsg among the messages the failure produced")
		}
	}
}

// failWatch feeds the failure the watch loop's List produced, exactly as the pump
// bridges it, and returns the batched command it issued.
func failWatch(t *testing.T, m Model, err error) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(watchMsg{gen: m.watchGen, msg: NewErrorMsg("watch pods", err)})
	return next.(Model), cmd
}

// TestExecPluginFailureRewritesThePaneWithTheDiagnosis is AUTH-04b's headline: the
// generic "your credential plugin failed" sentence the pane starts with is replaced,
// once the plugin has actually been re-run, by which command failed, what it printed
// and the command that fixes it.
func TestExecPluginFailureRewritesThePaneWithTheDiagnosis(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, _ := browsingPods(t, fd)

	m, cmd := failWatch(t, m, execPluginErr())
	if !strings.Contains(m.table.Notice(), "The credential plugin in your kubeconfig failed") {
		t.Fatalf("precondition: the pane should start on the kind's own sentence\ngot: %s", m.table.Notice())
	}

	diag := firstAuthDiagMsg(t, cmd)
	if fd.calls != 1 {
		t.Fatalf("diagnoser called %d times, want 1", fd.calls)
	}
	// The seam is handed the kubeconfig and context the shell holds — the two halves
	// that name the stanza to re-run.
	if got := fd.ccs[0]; got.Kubeconfig != "/home/u/.kube/config" || got.Context != "acme-prod" {
		t.Errorf("diagnoser asked about %+v, want the shell's own kubeconfig/context", got)
	}

	next, _ := m.Update(diag)
	m = next.(Model)

	notice := m.table.Notice()
	for _, want := range []string{
		"Cannot list Pod",
		"aws sso login --profile acme-prod",
		"The SSO session associated",
		"aws --region eu-west-1 eks get-token",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("the diagnosed pane does not carry %q\ngot:\n%s", want, notice)
		}
	}
	if strings.Contains(notice, "Re-authenticate in a terminal, then open the resource again.") {
		t.Errorf("the generic sentence should have been replaced\ngot:\n%s", notice)
	}
}

// TestWatchStartFailureIsDiagnosedToo covers the other entry point: when Watch
// itself fails no channel is ever returned, so this pane has no retry coming and is
// the one that most needs the plugin's own words.
func TestWatchStartFailureIsDiagnosedToo(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	fw := &fakeWatcher{err: execPluginErr()}
	m := sizedWith(t, WithWatcher(fw), WithAuthDiagnoser(fd), WithContext("acme-prod"))

	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: podResource()})
	m = next.(Model)

	diag := firstAuthDiagMsg(t, cmd)
	if diag.res.GVK.Kind != "Pod" {
		t.Errorf("diagnosis carries kind %q, want Pod", diag.res.GVK.Kind)
	}
	next, _ = m.Update(diag)
	m = next.(Model)
	if !strings.Contains(m.table.Notice(), "aws sso login --profile acme-prod") {
		t.Errorf("a failed watch start is not diagnosed\ngot:\n%s", m.table.Notice())
	}
}

// TestOnlyPluginFailuresAreDiagnosed: the re-run is a subprocess, so it happens only
// for the one classification it explains. An RBAC denial, a conversion webhook or an
// unreachable server already say everything they can.
func TestOnlyPluginFailuresAreDiagnosed(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, _ := browsingPods(t, fd)

	for name, err := range map[string]error{
		"forbidden": apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("denied")),
		"webhook":   conversionWebhookErr(),
		"plain":     errors.New("kube: listing pods: something else"),
	} {
		if cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", err)); cmd != nil {
			t.Errorf("%s: a non-plugin failure must not re-run the credential plugin", name)
		}
	}
	if fd.calls != 0 {
		t.Errorf("diagnoser called %d times for failures that are not the plugin's, want 0", fd.calls)
	}
}

// TestDiagnosisRunsOncePerSelection is the latch: the watch loop re-Lists on a
// backoff, so an unauthenticated cluster produces a failure every few seconds, and
// one credential-plugin subprocess per retry is not a diagnostic — it is a loop.
// Re-opening a resource (any drill-in bumps watchGen) re-arms it.
func TestDiagnosisRunsOncePerSelection(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, fw := browsingPods(t, fd)

	if cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", execPluginErr())); cmd == nil {
		t.Fatal("the first plugin failure of a selection must be diagnosed")
	}
	if cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", execPluginErr())); cmd != nil {
		t.Error("the retry's failure re-ran the plugin a second time")
	}

	// A fresh selection is a fresh question.
	m = drillInto(t, m, fw, gvrResource("configmaps"))
	if cmd := m.diagnoseAuth(gvrResource("configmaps"), NewErrorMsg("watch configmaps", execPluginErr())); cmd == nil {
		t.Error("a new selection must re-arm the diagnosis")
	}
}

// TestStaleDiagnosisDoesNotOverwriteTheNewPane is the generation guard: re-running a
// plugin takes seconds, and the reader may have drilled into something else by the
// time it answers. Its answer describes the pane they left.
func TestStaleDiagnosisDoesNotOverwriteTheNewPane(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, fw := browsingPods(t, fd)

	cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", execPluginErr()))
	diag, ok := cmd().(authDiagMsg)
	if !ok {
		t.Fatalf("diagnosis produced %T, want authDiagMsg", cmd())
	}

	// The reader moves on before the plugin answers.
	m = drillInto(t, m, fw, gvrResource("configmaps"))
	next, _ := m.Update(diag)
	m = next.(Model)

	if m.table.Notice() != "" {
		t.Errorf("a diagnosis for the previous resource was written into the new pane: %q", m.table.Notice())
	}
}

// TestDiagnosisDroppedOnceThePaneRecovered: the watch loop's next re-List may succeed
// while the plugin is still running (a re-authentication in another terminal is
// exactly that), and a notice written behind the rows would resurface later, out of
// nowhere, on the next empty result.
func TestDiagnosisDroppedOnceThePaneRecovered(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, _ := browsingPods(t, fd)

	m, _ = failWatch(t, m, execPluginErr())
	diag := authDiagMsg{gen: m.watchGen, res: podResource(), fail: NewErrorMsg("watch pods", execPluginErr()), rep: fd.rep}

	// Recovery: a re-List that worked, and rows.
	next, _ := m.Update(watchMsg{gen: m.watchGen, msg: ResourceEventMsg{Event: resetEvent("nginx", "u1")}})
	m = next.(Model)
	next, _ = m.Update(diag)
	m = next.(Model)

	if m.table.Notice() != "" {
		t.Errorf("the diagnosis overwrote a recovered pane: %q", m.table.Notice())
	}
	if m.table.RowCount() != 1 {
		t.Errorf("RowCount = %d, want the recovered row", m.table.RowCount())
	}
}

// TestDiagnosisFailureKeepsTheGenericSentence: a diagnostic that could not be
// attempted (an unreadable kubeconfig, a plugin that would not start), or one that
// names no plugin at all, leaves the pane exactly as it was. The kind's sentence is
// thinner, but it is true — and the reader asked to browse a resource, not to run a
// diagnostic, so nothing extra is put in front of them.
func TestDiagnosisFailureKeepsTheGenericSentence(t *testing.T) {
	for name, diagnosis := range map[string]struct {
		rep kube.ExecPluginReport
		err error
	}{
		"attempt failed": {err: errors.New("kube: loading kubeconfig for exec plugin: boom")},
		"no plugin named": {rep: kube.ExecPluginReport{
			Diagnosis: kube.ExecPluginDiagnosis{ExitCode: 255},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			fd := &fakeAuthDiagnoser{rep: diagnosis.rep, err: diagnosis.err}
			m, _ := browsingPods(t, fd)
			m, _ = failWatch(t, m, execPluginErr())
			before := m.table.Notice()
			if before == "" {
				t.Fatal("precondition: the failure should have written the kind's sentence")
			}

			next, cmd := m.Update(authDiagMsg{
				gen:  m.watchGen,
				res:  podResource(),
				fail: NewErrorMsg("watch pods", execPluginErr()),
				rep:  diagnosis.rep,
				err:  diagnosis.err,
			})
			m = next.(Model)
			if cmd != nil {
				t.Errorf("a diagnosis that established nothing should issue no further work, got %T", cmd())
			}
			if m.table.Notice() != before {
				t.Errorf("the pane changed on a diagnosis that established nothing:\nwas: %s\nnow: %s", before, m.table.Notice())
			}
		})
	}
}

// TestDiagnosisInertWithoutTheSeam: with nothing wired, a plugin failure behaves
// exactly as it did before this leg — the kind's sentence, no subprocess, no message.
// That is the state every hermetic test that does not opt in runs in.
func TestDiagnosisInertWithoutTheSeam(t *testing.T) {
	fw := &fakeWatcher{}
	m := drillInto(t, sizedWith(t, WithWatcher(fw)), fw, podResource())

	m, _ = failWatch(t, m, execPluginErr())
	if cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", execPluginErr())); cmd != nil {
		t.Error("an unwired shell must not diagnose")
	}
	if !strings.Contains(m.table.Notice(), "The credential plugin in your kubeconfig failed") {
		t.Errorf("the kind's own sentence should still be shown\ngot: %s", m.table.Notice())
	}
}

// TestDiagnosisStoppedByClusterTeardown: the re-run asks about the context being
// left, so a context switch (and a quit) must end it — and re-arm the latch, so the
// cluster switched *to* is diagnosed on its own first failure.
func TestDiagnosisStoppedByClusterTeardown(t *testing.T) {
	fd := &fakeAuthDiagnoser{rep: ssoReport(t)}
	m, _ := browsingPods(t, fd)

	cmd := m.diagnoseAuth(podResource(), NewErrorMsg("watch pods", execPluginErr()))
	if cmd == nil {
		t.Fatal("precondition: the failure should have started a diagnosis")
	}
	cmd() // run it, so the diagnoser records the context it was handed
	if len(fd.ctxs) != 1 {
		t.Fatalf("diagnoser recorded %d contexts, want 1", len(fd.ctxs))
	}

	m.stopClusterAsync()

	if fd.ctxs[0].Err() == nil {
		t.Error("stopClusterAsync should cancel the credential-plugin re-run")
	}
	if m.authDiagCancel != nil {
		t.Error("stopClusterAsync should leave no diagnosis cancel behind")
	}
	if m.authDiagGen != 0 {
		t.Errorf("authDiagGen = %d after teardown, want the latch re-armed", m.authDiagGen)
	}
}
