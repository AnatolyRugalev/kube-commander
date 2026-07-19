package kube

import (
	"context"
	"errors"
	"net"
	"net/url"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/clientcmd"
)

// ErrorKind is a small, TUI-facing taxonomy of the ways a kube-layer call can
// fail. The kube layer already returns its errors wrapped (`fmt.Errorf(... %w)`)
// around the underlying apierrors/clientcmd/transport error; Classify walks that
// chain and maps it to one of these kinds so a caller (the M2 TUI) can react —
// show a friendly message and degrade a single feature — instead of panicking on
// a bad namespace/context or blanking the whole UI (#86, principle 3 "degrade,
// don't crash"). This is classification only: no error site needs rewiring, and
// the underlying error stays reachable via errors.Is/As for callers that want the
// detail.
type ErrorKind int

// errBadContext tags a kubeconfig/context construction failure. RESTConfig wraps
// every error it returns with this sentinel so Classify reports KindBadContext
// regardless of clientcmd's varied internal wording (e.g. a missing override
// context is a plain fmt.Errorf clientcmd exposes no predicate for). It is
// unexported: callers classify via Classify, not by matching the sentinel.
var errBadContext = errors.New("kubeconfig or context error")

const (
	// KindUnknown is an error the taxonomy does not recognise (including a nil
	// error). Callers should still surface it — it just carries no special
	// handling hint.
	KindUnknown ErrorKind = iota
	// KindNotFound: the addressed object/resource does not exist (HTTP 404).
	KindNotFound
	// KindAlreadyExists: a create/precondition collided with an existing object
	// (HTTP 409 AlreadyExists).
	KindAlreadyExists
	// KindConflict: a write lost an optimistic-concurrency race — a stale
	// resourceVersion or a failed UID precondition (HTTP 409 Conflict). The row
	// snapshot the action was built from is out of date; re-list and retry.
	KindConflict
	// KindForbidden: authenticated but not authorized — RBAC denied the verb on
	// the resource (HTTP 403). Degrade that one feature; the rest of the UI is
	// fine.
	KindForbidden
	// KindUnauthorized: authentication itself failed — missing, expired, or
	// invalid credentials (HTTP 401).
	KindUnauthorized
	// KindInvalid: the request was rejected as malformed or semantically invalid
	// (HTTP 400/422) — e.g. a bad field selector or object.
	KindInvalid
	// KindTimeout: the server did not answer in time, or the caller's context
	// deadline lapsed. Usually transient; retry is reasonable.
	KindTimeout
	// KindUnreachable: the API server could not be reached at all — connection
	// refused, DNS failure, TLS/transport error, or a 503 ServiceUnavailable.
	// Nothing cluster-side works until connectivity returns.
	KindUnreachable
	// KindBadContext: the kubeconfig or the selected context is missing, empty,
	// or invalid — a construction-time configuration error, never a live-cluster
	// one. The classic "panics on bad namespace/context" case (#86) lands here.
	KindBadContext
)

// String returns a stable, lowercase token for the kind (for logs and tests).
// It is a classification token, not user-facing copy — the TUI renders its own
// message per kind.
func (k ErrorKind) String() string {
	switch k {
	case KindNotFound:
		return "not-found"
	case KindAlreadyExists:
		return "already-exists"
	case KindConflict:
		return "conflict"
	case KindForbidden:
		return "forbidden"
	case KindUnauthorized:
		return "unauthorized"
	case KindInvalid:
		return "invalid"
	case KindTimeout:
		return "timeout"
	case KindUnreachable:
		return "unreachable"
	case KindBadContext:
		return "bad-context"
	default:
		return "unknown"
	}
}

// Classify maps err (typically one the kube layer wrapped) to an ErrorKind by
// walking its wrap chain. It never panics and treats a nil error as KindUnknown.
//
// The apierrors.Is* predicates already unwrap to the embedded *StatusError, so
// they see through the layer's `%w` wrapping; the clientcmd predicates are run
// against every link of the chain (chainMatches) because some of them only match
// the concrete error type, not a wrapped one.
func Classify(err error) ErrorKind {
	if err == nil {
		return KindUnknown
	}

	// Configuration/context errors first: these come from client construction
	// (RESTConfig), are distinctive, and must never read as a live-cluster fault.
	// RESTConfig tags them with errBadContext; the clientcmd predicates are a
	// secondary net for a clientcmd error that reached Classify by another path.
	if errors.Is(err, errBadContext) ||
		chainMatches(err, clientcmd.IsContextNotFound) ||
		chainMatches(err, clientcmd.IsEmptyConfig) ||
		chainMatches(err, clientcmd.IsConfigurationInvalid) {
		return KindBadContext
	}

	// Status-based server errors, most specific first.
	switch {
	case apierrors.IsNotFound(err):
		return KindNotFound
	case apierrors.IsAlreadyExists(err):
		return KindAlreadyExists
	case apierrors.IsConflict(err):
		return KindConflict
	case apierrors.IsUnauthorized(err):
		return KindUnauthorized
	case apierrors.IsForbidden(err):
		return KindForbidden
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return KindInvalid
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return KindTimeout
	case apierrors.IsServiceUnavailable(err):
		return KindUnreachable
	}

	// A lapsed caller deadline reads as a timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}

	// Transport-level failures (dial refused, DNS, TLS) surface as *url.Error or
	// a net.Error rather than an apierror, because the request never got a status.
	var urlErr *url.Error
	var netErr net.Error
	if errors.As(err, &urlErr) || errors.As(err, &netErr) {
		return KindUnreachable
	}

	return KindUnknown
}

// chainMatches reports whether pred holds for err or any error in its unwrap
// chain. It exists for predicates (some clientcmd ones) that match only the
// concrete error type and would otherwise miss a wrapped error.
func chainMatches(err error, pred func(error) bool) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if pred(e) {
			return true
		}
	}
	return false
}
