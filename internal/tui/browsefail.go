package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/elide"
)

// This file is kubecom's user-facing copy for a browse table that has nothing to
// show because the LIST behind it failed (CRD-01), and — for the one kind that has
// more to say than a sentence — the diagnosed version of it (authFailure,
// AUTH-04a). It is the first per-kind wording in the app — kube.ErrorKind is
// deliberately a classification token and not copy ("the TUI renders its own
// message per kind") — so the rules it follows are worth stating once, here, for
// whoever adds the next surface:
//
//   - Say what failed, then why, then what the server said. The headline names
//     the kind so a reader who walked away knows which pane they are looking at.
//   - Never claim a cause the error does not carry. Two cluster-side causes are
//     recognised by name because their default wording would actively mislead —
//     a conversion webhook that is down reads as "cluster unreachable" (D191 pt 1)
//     — and everything else falls back to its kind's sentence.
//   - Say where the fix is: this machine (re-authenticate, switch context) or the
//     cluster (a webhook, RBAC). A reader who cannot tell which will retry the
//     wrong one.
//   - Never promise a retry the code does not perform. The browse watch loop does
//     re-List on a backoff, so the retry sentence is honest here and must not be
//     copied to a surface that has no such loop.

// browseFailure is the reason text the browse table shows in place of its empty
// body when the LIST behind it failed. The first line is the headline (the table
// renders it in the Error style), the rest is detail. kind is the display kind of
// the resource being browsed ("ExternalSecret"); an empty kind degrades to a
// generic subject rather than a stray gap, since a failure that arrives before
// the kind is known is still worth reading (principle 3).
func browseFailure(kind string, e ErrorMsg) string {
	subject := "this resource"
	if kind != "" {
		subject = kind
	}
	lines := []string{"Cannot list " + subject}
	lines = append(lines, browseFailureCause(e))
	if detail := serverDetail(e); detail != "" {
		lines = append(lines, detail)
	}
	return strings.Join(lines, "\n")
}

// browseFailureCause is the one sentence (or two) explaining the failure: the
// named cluster-side causes first — they are recognised by evidence in the error
// and would otherwise be described wrongly by their kind — then the kind's own
// sentence.
func browseFailureCause(e ErrorMsg) string {
	switch {
	case kube.ConversionWebhookFailed(e.Err):
		return "The API server could not call this resource's conversion webhook, " +
			"so it cannot serve the list at all — kubectl fails the same way. " +
			"This is cluster-side: check the CRD's conversion webhook service and its certificate."
	case kube.TableUnsupported(e.Err):
		return "This API server does not serve table output for the resource, " +
			"which is the format kubecom lists in. Nothing here is fixable from this side."
	}

	switch e.Kind {
	case kube.KindForbidden:
		return "You are not allowed to list it here — RBAC denied the request. " +
			"Try another namespace or context, or ask for a role that grants list on this resource."
	case kube.KindUnauthorized:
		return "The API server rejected your credentials. Re-authenticate, then open the resource again."
	case kube.KindExecPlugin:
		return "The credential plugin in your kubeconfig failed, so the request never reached the cluster. " +
			"Re-authenticate in a terminal, then open the resource again."
	case kube.KindNotFound:
		return "The API server does not serve this resource — its CRD or API group may have been removed."
	case kube.KindUnreachable:
		return "The API server could not be reached. Check your network and the cluster endpoint; kubecom keeps retrying."
	case kube.KindTimeout:
		return "The API server did not answer in time; kubecom keeps retrying."
	case kube.KindInvalid:
		return "The API server rejected the request as invalid."
	case kube.KindBadContext:
		return "The selected kubeconfig context is not usable."
	default:
		return "The API server refused the request; the text below is what it said."
	}
}

// authFailure is the same pane's notice once the credential plugin behind a
// KindExecPlugin failure has been re-run and diagnosed (AUTH-04a). It replaces
// browseFailureCause's one generic sentence with the three things that sentence
// cannot carry: *which* command failed, what it printed, and the command that
// fixes it. A report with no plugin degrades to browseFailure — there is nothing
// to add and the kind's sentence is still true.
//
// offered says whether kubecom is, at this moment, asking the reader whether to
// run the remediation itself (AUTH-05b's confirm). It changes one line and it must
// track the offer rather than the diagnosis: telling a reader to go to another
// terminal while a prompt on the same screen offers to do it for them is the pane
// contradicting the modal, and leaving that sentence up after the offer is answered
// is the pane promising a prompt that is gone. The shell therefore re-renders with
// offered=false the moment the offer is accepted or declined (restoreReauthNotice).
//
// Two rules beyond the ones at the top of this file:
//
//   - **Line order is load-bearing.** The pane renders a notice into the rows it
//     has and silently *drops* the rest (table.noticeBody), so everything a reader
//     must act on precedes the plugin's stderr — which is the only part with no
//     bound on its length. Read top-down it is: what failed, why, the fix, then the
//     evidence.
//   - **The client-go error is not quoted here.** browseFailure ends with the
//     server's own text because that is the best account available; here the
//     diagnosis is a strictly better account of the same failure (`exec:
//     executable aws failed with exit code 255` says less than the command line and
//     the stderr do), and a pane row spent restating it is a row of stderr lost.
//     It is still in the toast and the log line (surfaceError).
func authFailure(kind string, e ErrorMsg, rep kube.ExecPluginReport, offered bool) string {
	if rep.Plugin == nil {
		return browseFailure(kind, e)
	}
	subject := "this resource"
	if kind != "" {
		subject = kind
	}
	lines := append([]string{"Cannot list " + subject}, authFailureCause(rep)...)
	if rep.Suggested {
		lines = append(lines, rep.Remediation.Cause, remediationLead(offered),
			"  "+rep.Remediation.CommandLine())
	}
	lines = append(lines, "Plugin: "+authPluginLine(rep))
	return strings.Join(append(lines, authStderrLines(rep.Diagnosis)...), "\n")
}

// remediationLead introduces the suggested command, and is the one line the pane
// owes to whether an offer is open. With the prompt up the reader's next keystroke
// is the answer to it, so the pane names the prompt and quotes the command it is
// asking about — the command is still printed either way, because the prompt box
// is sixty cells wide and bounded in height (D220), so a long invocation can lose
// its tail to the box's own elision while this pane, which wraps and scrolls with
// the table, always carries it whole. With no prompt up (declined, answered, or
// never offered because
// something else held the screen) the fix is the reader's to run.
func remediationLead(offered bool) string {
	if offered {
		return "kubecom is asking whether to run this for you:"
	}
	return "Run this in another terminal, then reopen the resource:"
}

// authFailureCause is the sentence (or two) explaining what the diagnostic re-run
// established, and where the fix therefore is. The four cases are genuinely
// different remediations, not shades of one: a missing binary wants installing, a
// wedged one wants investigating, a re-run that succeeded wants nothing at all,
// and only the last case is "re-authenticate".
func authFailureCause(rep kube.ExecPluginReport) []string {
	who := "The credential plugin your kubeconfig runs for this context"
	if ctx := rep.Plugin.Context; ctx != "" {
		who = fmt.Sprintf("The credential plugin your kubeconfig runs for context %q", ctx)
	}
	switch d := rep.Diagnosis; {
	case !d.Failed():
		// D211 pt 5's obligation: the plugin worked when re-run, which is what
		// happens when the user re-authenticated in another terminal in between.
		return []string{who + " failed this request, but running it again just now worked — " +
			"the credentials were most likely renewed in between. kubecom keeps retrying, " +
			"so the rows appear as soon as a request gets through."}
	case d.NotFound:
		out := []string{who + " could not be run at all: the binary is missing, or is not executable. " +
			"Install it or fix the command in your kubeconfig — re-authenticating cannot help."}
		// The stanza's InstallHint is the operator's own answer to exactly this,
		// and this is the one failure it was written for. Flattened: it is free
		// text and may carry newlines the pane would read as new detail lines.
		if hint := strings.Join(strings.Fields(rep.Plugin.InstallHint), " "); hint != "" {
			out = append(out, "Your kubeconfig's install hint: "+hint)
		}
		return out
	case d.TimedOut:
		return []string{who + " did not answer and was killed, so the request never reached the cluster. " +
			"The fix is on this machine, not in the cluster: run the command below in a terminal and see what it waits for."}
	default:
		out := []string{who + " failed, so the request never reached the cluster. " +
			"The fix is on this machine, not in the cluster."}
		if !rep.Suggested {
			// True of both ways Suggested can be false, which is why it does not say
			// "not recognised": an AWS SSO expiry whose stanza names no profile *is*
			// recognised, and kubecom still has nothing it may suggest (D212 pt 3).
			out = append(out, "kubecom has no command it can suggest for this failure: "+
				"re-authenticate the way this plugin needs, then reopen the resource.")
		}
		return out
	}
}

// authPluginLine is the invocation that failed, with its outcome in parentheses —
// the diagnosis's own CommandLine (what was actually run) in preference to the
// stanza's rendering of it.
func authPluginLine(rep kube.ExecPluginReport) string {
	cmd := rep.Diagnosis.CommandLine
	if cmd == "" {
		cmd = rep.Plugin.CommandLine()
	}
	switch d := rep.Diagnosis; {
	case d.NotFound:
		return cmd + " (not found)"
	case d.TimedOut:
		return cmd + " (killed: no answer)"
	case d.ExitCode < 0:
		return cmd + " (killed by a signal)"
	case d.ExitCode == 0:
		return cmd + " (exited 0 when re-run)"
	default:
		return cmd + " (exit " + strconv.Itoa(d.ExitCode) + ")"
	}
}

// authStderrLines is the plugin's own account, kept multi-line — the one place in
// this file that does not flatten, because the line structure *is* the message
// (a provider CLI prints a sentence per fact). Indented so it reads as a quote,
// blank lines dropped because every pane row is one line of evidence lost, and a
// truncated capture says so rather than ending mid-sentence.
//
// A plugin that failed with an exit status and printed nothing gets a line saying
// so: the silence is itself the diagnosis, and its absence would read as a
// rendering bug. The other outcomes do not — a binary that was never found, one
// that was killed, and one that now works have all said why on the Plugin line.
func authStderrLines(d kube.ExecPluginDiagnosis) []string {
	body := make([]string, 0, 8)
	for _, line := range strings.Split(d.Stderr, "\n") {
		if line = strings.TrimRight(line, " \t\r"); strings.TrimSpace(line) == "" {
			continue
		}
		body = append(body, "  "+line)
	}
	if len(body) == 0 {
		if d.ExitCode > 0 {
			return []string{"It printed nothing on stderr."}
		}
		return nil
	}
	out := append([]string{"It said:"}, body...)
	if d.Truncated {
		out = append(out, "  "+elide.Marker)
	}
	return out
}

// serverDetail is the underlying error, prefixed so it reads as a quote rather
// than as more kubecom prose, and flattened to one paragraph (the table wraps it
// itself and an embedded newline would be mistaken for the start of a new
// detail line). Empty when there is no error to quote.
func serverDetail(e ErrorMsg) string {
	if e.Err == nil {
		return ""
	}
	text := strings.Join(strings.Fields(e.Err.Error()), " ")
	if text == "" {
		return ""
	}
	return "Server: " + text
}
