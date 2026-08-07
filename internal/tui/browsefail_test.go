package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/menu"
)

// conversionWebhookErr is the failure the 2026-08-01 dogfood's cluster produced:
// the apiserver answering a LIST with its own inability to reach the CRD's
// conversion webhook, wrapped exactly as the watch's List path wraps it.
func conversionWebhookErr() error {
	return fmt.Errorf("kube: listing externalsecrets: %w", apierrors.NewInternalError(errors.New(
		`conversion webhook for external-secrets.io/v1beta1, Kind=ExternalSecret failed: `+
			`Post "https://external-secrets-webhook.external-secrets.svc:443/convert?timeout=30s": `+
			`dial tcp 10.0.0.1:443: connect: connection refused`)))
}

// TestBrowseFailureNamesTheConversionWebhook is CRD-01's headline case. A
// conversion webhook that is down classifies as an internal/unreachable-looking
// error, so the *kind's* sentence would send the reader to check their network
// for a failure that is entirely inside the cluster and breaks kubectl too
// (D191 pt 1). The named cause must win over the kind, and must say where the
// fix is.
func TestBrowseFailureNamesTheConversionWebhook(t *testing.T) {
	got := browseFailure("ExternalSecret", NewErrorMsg("watch externalsecrets", conversionWebhookErr()))

	head, rest, _ := strings.Cut(got, "\n")
	if head != "Cannot list ExternalSecret" {
		t.Fatalf("headline = %q, want the kind named", head)
	}
	for _, want := range []string{"conversion webhook", "kubectl fails the same way", "cluster-side"} {
		if !strings.Contains(rest, want) {
			t.Errorf("reason missing %q\ngot: %s", want, rest)
		}
	}
	if strings.Contains(rest, "Check your network") {
		t.Errorf("the unreachable copy must not win over the named cause\ngot: %s", rest)
	}
	if !strings.Contains(rest, "connection refused") {
		t.Errorf("the server's own text must be quoted\ngot: %s", rest)
	}
}

// TestBrowseFailureNamesTheTableRefusal covers the second cluster-side cause:
// a 406 on the Table content type every List asks for. It carries no ErrorKind,
// so without the predicate it would fall to the generic sentence.
func TestBrowseFailureNamesTheTableRefusal(t *testing.T) {
	notAcceptable := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure, Code: 406, Reason: metav1.StatusReasonNotAcceptable,
		Message: "only the following media types are accepted: application/json",
	}}
	got := browseFailure("Widget", NewErrorMsg("watch widgets", notAcceptable))

	if !strings.Contains(got, "does not serve table output") {
		t.Fatalf("406 was not named\ngot: %s", got)
	}
}

// TestBrowseFailurePerKindCopy walks the kinds a browse LIST realistically fails
// with and pins that each says where the fix is — this machine or the cluster —
// since a reader who cannot tell will retry the wrong one.
func TestBrowseFailurePerKindCopy(t *testing.T) {
	gr := schema.GroupResource{Group: "external-secrets.io", Resource: "externalsecrets"}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"forbidden", apierrors.NewForbidden(gr, "", errors.New("denied")), "RBAC denied the request"},
		{"unauthorized", apierrors.NewUnauthorized("bad token"), "rejected your credentials"},
		{"not found", apierrors.NewNotFound(gr, "x"), "CRD or API group may have been removed"},
		{"timeout", apierrors.NewTimeoutError("slow", 1), "did not answer in time"},
		{"unknown", errors.New("something else"), "the text below is what it said"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := browseFailure("ExternalSecret", NewErrorMsg("watch externalsecrets", tc.err))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("missing %q\ngot: %s", tc.want, got)
			}
			if !strings.HasPrefix(got, "Cannot list ExternalSecret\n") {
				t.Fatalf("headline missing\ngot: %s", got)
			}
		})
	}
}

// TestBrowseFailureDegradesWithoutAKind proves a failure that arrives before the
// browsed kind is known still reads as a sentence rather than leaving a gap
// (principle 3), and that a nil error contributes no empty quote line.
func TestBrowseFailureDegradesWithoutAKind(t *testing.T) {
	got := browseFailure("", ErrorMsg{Context: "watch"})
	if !strings.HasPrefix(got, "Cannot list this resource\n") {
		t.Fatalf("unknown kind should degrade to a generic subject, got: %q", got)
	}
	if strings.Contains(got, "Server:") {
		t.Fatalf("a nil error must not be quoted, got: %q", got)
	}
}

// TestBrowseFailureFlattensTheServerText pins that a multi-line server error is
// quoted as one paragraph: the table treats each source line as its own wrapped
// block, so an embedded newline would read as a second, unrelated detail.
func TestBrowseFailureFlattensTheServerText(t *testing.T) {
	got := browseFailure("Pod", NewErrorMsg("watch pods", errors.New("first line\n  second line")))
	if !strings.Contains(got, "Server: first line second line") {
		t.Fatalf("server text was not flattened\ngot: %s", got)
	}
	if n := len(strings.Split(got, "\n")); n != 3 {
		t.Fatalf("notice = %d lines, want 3 (headline, reason, quote)\ngot: %s", n, got)
	}
}

// --- the diagnosed credential-plugin failure (AUTH-04a) ---------------------

// awsStanza is the exec stanza an EKS kubeconfig carries, as ExecPluginFor would
// return it: the command kubecom re-ran, and the profile a remediation is
// composed from.
func awsStanza() *kube.ExecPlugin {
	return &kube.ExecPlugin{
		Context: "acme-prod",
		User:    "acme-prod-user",
		Command: "aws",
		Args:    []string{"--region", "eu-west-1", "eks", "get-token", "--cluster-name", "acme", "--profile", "acme-prod"},
	}
}

// ssoStderr is the botocore wording AUTH-03 recognises, as the AWS CLI prints it:
// several lines, which is why this surface exists at all.
const ssoStderr = "The SSO session associated with this profile has expired or is otherwise invalid.\n" +
	"To refresh this SSO session run aws sso login with the corresponding profile."

// diagnosedReport builds the report the shell will hand the renderer in AUTH-04b:
// the stanza, the diagnosis of re-running it, and whatever remediation the kube
// layer substantiates from the stanza — asked for here exactly as the shell will
// ask, so the test cannot drift from the real answer.
func diagnosedReport(t *testing.T, p *kube.ExecPlugin, d kube.ExecPluginDiagnosis) kube.ExecPluginReport {
	t.Helper()
	if d.CommandLine == "" {
		d.CommandLine = p.CommandLine()
	}
	rep := kube.ExecPluginReport{Plugin: p, Diagnosis: d}
	rep.Remediation, rep.Suggested = p.SuggestedRemediation(d)
	return rep
}

// execPluginErr is the error client-go returns for a plugin that ran and failed —
// the one that classifies KindExecPlugin and used to be all the pane had.
func execPluginErr() error {
	return fmt.Errorf("kube: listing pods: %w",
		errors.New("getting credentials: exec: executable aws failed with exit code 255"))
}

// TestAuthFailurePutsTheFixAboveTheEvidence is the headline claim, and the one the
// pane's geometry forces: the notice is rendered into the rows available and the
// remainder is dropped, so a remediation printed after a multi-line stderr is a
// remediation the reader may never see.
func TestAuthFailurePutsTheFixAboveTheEvidence(t *testing.T) {
	rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{Stderr: ssoStderr, ExitCode: 255})
	if !rep.Suggested {
		t.Fatal("precondition: the kube layer should substantiate a remediation for this stanza")
	}
	got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

	head, _, _ := strings.Cut(got, "\n")
	if head != "Cannot list Pod" {
		t.Fatalf("headline = %q, want the kind named", head)
	}
	fix := strings.Index(got, "aws sso login --profile acme-prod")
	if fix < 0 {
		t.Fatalf("the suggested command is not printed\ngot:\n%s", got)
	}
	evidence := strings.Index(got, "The SSO session associated")
	if evidence < 0 {
		t.Fatalf("the plugin's own words are not shown\ngot:\n%s", got)
	}
	if fix > evidence {
		t.Errorf("the fix must precede the stderr the pane may drop\ngot:\n%s", got)
	}
	if !strings.Contains(got, `profile "acme-prod" is expired`) {
		t.Errorf("the recognised cause is not stated\ngot:\n%s", got)
	}
	if !strings.Contains(got, "another terminal") {
		t.Errorf("the pane must say where to run it (kubecom does not, until AUTH-05)\ngot:\n%s", got)
	}
}

// TestAuthFailureNamesTheCommandAndItsExit pins the two facts the generic
// KindExecPlugin sentence could not carry: which invocation failed, and how.
func TestAuthFailureNamesTheCommandAndItsExit(t *testing.T) {
	rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{Stderr: ssoStderr, ExitCode: 255})
	got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

	if !strings.Contains(got, "Plugin: aws --region eu-west-1 eks get-token --cluster-name acme --profile acme-prod (exit 255)") {
		t.Errorf("the failed invocation is not quoted with its exit status\ngot:\n%s", got)
	}
	// The stderr keeps its line structure: the line break is the CLI's own, and
	// flattening it would run two sentences together.
	if n := strings.Count(got, "\n  "); n != 3 {
		t.Errorf("indented quote lines = %d, want 3 (the command + two stderr lines)\ngot:\n%s", n, got)
	}
	// client-go's own text adds nothing the diagnosis does not say better, and a
	// row spent on it is a row of stderr lost.
	if strings.Contains(got, "exec: executable aws failed") {
		t.Errorf("the client-go error should not be restated\ngot:\n%s", got)
	}
}

// TestAuthFailureWithoutARemediationSuggestsNothing covers the two cases
// SuggestedRemediation says no to, which this surface renders identically
// (D212 pt 3): a plugin kubecom knows nothing about, and an AWS SSO expiry whose
// stanza names no profile. Neither may produce a guessed command.
func TestAuthFailureWithoutARemediationSuggestsNothing(t *testing.T) {
	gcloud := &kube.ExecPlugin{Context: "gke-dev", Command: "gcloud", Args: []string{"config", "config-helper"}}
	noProfile := &kube.ExecPlugin{Context: "acme-prod", Command: "aws", Args: []string{"eks", "get-token"}}
	cases := []struct {
		name   string
		plugin *kube.ExecPlugin
		stderr string
	}{
		{"unrecognised", gcloud, "ERROR: (gcloud.config.config-helper) You do not currently have an active account selected."},
		{"recognised but unsubstantiated", noProfile, ssoStderr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := diagnosedReport(t, tc.plugin, kube.ExecPluginDiagnosis{Stderr: tc.stderr, ExitCode: 1})
			if rep.Suggested {
				t.Fatalf("precondition: no remediation should be substantiated, got %q", rep.Remediation.CommandLine())
			}
			got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

			// The remediation block is absent entirely. Asserted on the block's own
			// lines rather than on the string "sso login", which legitimately
			// appears in the *stderr* of the unsubstantiated case — the AWS CLI
			// tells you to run it, and quoting the CLI is not suggesting a command.
			if strings.Contains(got, "Run this in another terminal") {
				t.Errorf("a command was suggested for a failure kubecom cannot substantiate\ngot:\n%s", got)
			}
			if strings.Contains(got, "\n  aws sso login") || strings.Contains(got, "is expired or missing") {
				t.Errorf("the remediation block leaked into the pane\ngot:\n%s", got)
			}
			if !strings.Contains(got, "no command it can suggest") {
				t.Errorf("the pane must admit it has no fix\ngot:\n%s", got)
			}
			if !strings.Contains(got, tc.stderr[:20]) {
				t.Errorf("the plugin's own words must still be shown\ngot:\n%s", got)
			}
		})
	}
}

// TestAuthFailureSaysTheRerunWorked is AUTH-02/D211 pt 5's obligation: the user
// re-authenticated in another terminal between the failed request and the
// diagnosis, so the plugin now works. Reporting that as a failure — or offering a
// fix for it — would be the most confusing possible output.
func TestAuthFailureSaysTheRerunWorked(t *testing.T) {
	rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{ExitCode: 0})
	got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

	if !strings.Contains(got, "running it again just now worked") {
		t.Fatalf("the successful re-run is not reported\ngot:\n%s", got)
	}
	for _, unwanted := range []string{"sso login", "re-authenticate", "no command it can suggest", "It said:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a working plugin must not be given a fix (%q)\ngot:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "keeps retrying") {
		t.Errorf("the pane recovers on its own here; say so\ngot:\n%s", got)
	}
}

// TestAuthFailureMissingBinaryDoesNotSayReauthenticate is the case AUTH-01's
// NotFound flag was created for: the remediation is "install it", and telling a
// user to re-authenticate would send them to fix a session that is not the problem.
func TestAuthFailureMissingBinaryDoesNotSayReauthenticate(t *testing.T) {
	p := awsStanza()
	p.InstallHint = "aws-cli is required to authenticate to EKS.\nSee https://example.test/install"
	rep := diagnosedReport(t, p, kube.ExecPluginDiagnosis{NotFound: true})
	got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

	if !strings.Contains(got, "Install it") {
		t.Fatalf("a missing binary must be reported as missing\ngot:\n%s", got)
	}
	if strings.Contains(got, "re-authenticate the way") {
		t.Errorf("re-authenticating cannot fix a missing binary\ngot:\n%s", got)
	}
	if !strings.Contains(got, "(not found)") {
		t.Errorf("the Plugin line must carry the outcome\ngot:\n%s", got)
	}
	// The operator wrote InstallHint for exactly this failure — and it is free
	// text, so it must arrive as one detail line, not as several.
	if !strings.Contains(got, "install hint: aws-cli is required to authenticate to EKS. See https://example.test/install") {
		t.Errorf("the stanza's install hint is missing or unflattened\ngot:\n%s", got)
	}
	if strings.Contains(got, "printed nothing") {
		t.Errorf("a binary that never ran did not stay silent — it was absent\ngot:\n%s", got)
	}
}

// TestAuthFailureTimedOut: a wedged plugin (one waiting on a terminal prompt it
// will never get, the case Diagnose closes stdin for) is neither a bad credential
// nor a missing binary, and the pane must not name a cause it cannot see.
func TestAuthFailureTimedOut(t *testing.T) {
	rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{TimedOut: true})
	got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)

	if !strings.Contains(got, "did not answer and was killed") {
		t.Fatalf("the timeout is not reported\ngot:\n%s", got)
	}
	if !strings.Contains(got, "(killed: no answer)") {
		t.Errorf("the Plugin line must carry the outcome\ngot:\n%s", got)
	}
	if strings.Contains(got, "printed nothing") {
		t.Errorf("a killed plugin's silence is not evidence\ngot:\n%s", got)
	}
}

// TestAuthFailureStderrShape covers what happens to the evidence itself: a capped
// capture says so, blank lines are dropped (a pane row says nothing), and a plugin
// that failed *silently* is reported as silent rather than as a missing quote.
func TestAuthFailureStderrShape(t *testing.T) {
	t.Run("truncated", func(t *testing.T) {
		rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{
			Stderr: ssoStderr, ExitCode: 255, Truncated: true})
		got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)
		if !strings.HasSuffix(got, "(truncated)") {
			t.Fatalf("a capped capture must say so, and last\ngot:\n%s", got)
		}
	})
	t.Run("blank lines dropped", func(t *testing.T) {
		rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{
			Stderr: "first\n\n   \nsecond\n", ExitCode: 255})
		got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)
		if !strings.HasSuffix(got, "It said:\n  first\n  second") {
			t.Fatalf("blank quote lines were not dropped\ngot:\n%s", got)
		}
	})
	t.Run("silent failure", func(t *testing.T) {
		rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{ExitCode: 3})
		got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)
		if !strings.Contains(got, "printed nothing on stderr") {
			t.Fatalf("silence is the diagnosis here; say it\ngot:\n%s", got)
		}
		if !strings.Contains(got, "(exit 3)") {
			t.Errorf("the exit status is all there is; it must be shown\ngot:\n%s", got)
		}
	})
	t.Run("killed by a signal", func(t *testing.T) {
		rep := diagnosedReport(t, awsStanza(), kube.ExecPluginDiagnosis{ExitCode: -1})
		got := authFailure("Pod", NewErrorMsg("watch pods", execPluginErr()), rep, false)
		if !strings.Contains(got, "(killed by a signal)") {
			t.Fatalf("-1 is a signal, not exit -1\ngot:\n%s", got)
		}
	})
}

// TestAuthFailureFallsBackWithoutAPlugin: the diagnosis can fail to name a plugin
// at all (an unreadable kubeconfig, or a context that authenticates some other
// way). There is then nothing to add, and the pane must still read as a sentence
// rather than as a half-rendered report (principle 3).
func TestAuthFailureFallsBackWithoutAPlugin(t *testing.T) {
	e := NewErrorMsg("watch pods", execPluginErr())
	got := authFailure("Pod", e, kube.ExecPluginReport{}, false)

	if got != browseFailure("Pod", e) {
		t.Fatalf("a plugin-less report must render exactly the undiagnosed notice\ngot:\n%s", got)
	}
	if !strings.Contains(got, "credential plugin in your kubeconfig failed") {
		t.Fatalf("the kind's own sentence is missing\ngot:\n%s", got)
	}
}

// TestAuthFailureDegradesWithoutAKind mirrors browseFailure's guard: a failure
// that arrives before the browsed kind is known still reads as a sentence.
func TestAuthFailureDegradesWithoutAKind(t *testing.T) {
	rep := diagnosedReport(t, &kube.ExecPlugin{Command: "aws"}, kube.ExecPluginDiagnosis{ExitCode: 255})
	got := authFailure("", ErrorMsg{Context: "watch", Kind: kube.KindExecPlugin}, rep, false)

	if !strings.HasPrefix(got, "Cannot list this resource\n") {
		t.Fatalf("unknown kind should degrade to a generic subject\ngot:\n%s", got)
	}
	// No Context on the stanza either: the sentence must not contain an empty
	// quoted name.
	if strings.Contains(got, `context ""`) {
		t.Fatalf("an unnamed context must not be quoted as empty\ngot:\n%s", got)
	}
	if !strings.Contains(got, "for this context") {
		t.Fatalf("the nameless fallback is missing\ngot:\n%s", got)
	}
}

// --- the shell wiring -------------------------------------------------------

// TestWatchErrorWritesTheReasonIntoTheTable is the end-to-end claim: a LIST that
// fails behind the browse table puts the reason where the reader is looking,
// not only in a toast that is gone in five seconds.
func TestWatchErrorWritesTheReasonIntoTheTable(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	next, _ := m.Update(menu.ResourceSelectedMsg{
		Resource: kube.Resource{
			GVR: schema.GroupVersionResource{Group: "external-secrets.io", Resource: "externalsecrets"},
			GVK: schema.GroupVersionKind{Group: "external-secrets.io", Kind: "ExternalSecret"},
		},
	})
	m = next.(Model)

	// The watch loop's first List failed: the pump bridges it to an ErrorMsg and
	// keeps the chain alive (it retries behind the scenes).
	next, cmd := m.Update(watchMsg{gen: m.watchGen, msg: NewErrorMsg(
		"watch externalsecrets", conversionWebhookErr())})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("a watch error must keep the pump chain alive")
	}

	view := m.table.View()
	for _, want := range []string{"Cannot list ExternalSecret", "conversion webhook"} {
		if !strings.Contains(view, want) {
			t.Errorf("the table pane does not show %q\ngot:\n%s", want, view)
		}
	}
	if !strings.Contains(m.status.View(), "watch externalsecrets") {
		t.Errorf("the transient toast must still be shown alongside the pane's reason\ngot: %s", m.status.View())
	}

	// Recovery: the watch loop re-Lists and emits a fresh RESET. The reason is
	// over, even though it arrives as an empty result.
	next, _ = m.Update(watchMsg{gen: m.watchGen, msg: ResourceEventMsg{Event: kube.WatchEvent{
		Type: kube.WatchReset, Columns: []kube.Column{{Name: "NAME"}},
	}}})
	m = next.(Model)
	if m.table.Notice() != "" {
		t.Fatalf("a successful re-List must clear the reason, still %q", m.table.Notice())
	}
}

// TestWatchStartFailureWritesTheReasonIntoTheTable covers the other half: when
// Watch itself fails no channel is ever returned, so no RESET and no retry are
// coming and the pane would otherwise stay blank forever.
func TestWatchStartFailureWritesTheReasonIntoTheTable(t *testing.T) {
	fw := &fakeWatcher{err: apierrors.NewForbidden(
		schema.GroupResource{Group: "external-secrets.io", Resource: "externalsecrets"},
		"", errors.New("denied"))}
	m := sizedWith(t, WithWatcher(fw))

	next, cmd := m.Update(menu.ResourceSelectedMsg{
		Resource: kube.Resource{
			GVR: schema.GroupVersionResource{Group: "external-secrets.io", Resource: "externalsecrets"},
			GVK: schema.GroupVersionKind{Group: "external-secrets.io", Kind: "ExternalSecret"},
		},
	})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("a failed watch start must still surface its error")
	}
	if _, ok := cmd().(ErrorMsg); !ok {
		t.Fatalf("failed watch start produced %T, want ErrorMsg", cmd())
	}
	if view := m.table.View(); !strings.Contains(view, "Cannot list ExternalSecret") {
		t.Fatalf("the table pane does not carry the reason\ngot:\n%s", view)
	}
	// The wording itself is checked on the notice rather than the render, which
	// word-wraps it to the pane.
	if !strings.Contains(m.table.Notice(), "RBAC denied the request") {
		t.Fatalf("the pane does not say where the fix is\ngot: %s", m.table.Notice())
	}
}

// TestSelectingAnotherResourceDropsTheReason guards the one-way-door mistake: a
// failure for one kind must not linger over the next kind's pane.
func TestSelectingAnotherResourceDropsTheReason(t *testing.T) {
	fw := &fakeWatcher{}
	m := sizedWith(t, WithWatcher(fw))

	next, _ := m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("externalsecrets")})
	m = next.(Model)
	next, _ = m.Update(watchMsg{gen: m.watchGen, msg: NewErrorMsg("watch externalsecrets", conversionWebhookErr())})
	m = next.(Model)
	if m.table.Notice() == "" {
		t.Fatal("precondition: the failure should have set a reason")
	}

	next, _ = m.Update(menu.ResourceSelectedMsg{Resource: gvrResource("pods")})
	m = next.(Model)
	if m.table.Notice() != "" {
		t.Fatalf("the previous kind's reason survived the switch: %q", m.table.Notice())
	}
}
