package tui

import (
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is kubecom's user-facing copy for a browse table that has nothing to
// show because the LIST behind it failed (CRD-01). It is the first per-kind
// wording in the app — kube.ErrorKind is deliberately a classification token and
// not copy ("the TUI renders its own message per kind") — so the rules it follows
// are worth stating once, here, for whoever adds the next surface (AUTH-04 is the
// next one queued):
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
