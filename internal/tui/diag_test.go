package tui

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/neuroplastio/kubecom/internal/kube"
)

// logSink builds a model whose diagnostic logger writes into a buffer, so a test can
// read exactly what a dogfooding user would find in `~/.cache/kubecom/kubecom.log`.
// Debug level so nothing this leg might add later is filtered out of the assertion.
func logSink(t *testing.T, opts ...Option) (Model, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	opts = append(opts, WithLogger(slog.New(h)))
	return sizedWith(t, opts...), &buf
}

// TestSurfacedErrorIsLogged is the leg's headline: the single error funnel writes the
// failure to the log file. The toast is transient and clipped; the log line is the
// only durable record, which is what makes a "it just errors out" bug report
// actionable (D159). The assertion demands the *unwrapped* detail — the wrapped kube
// error, not just the short label the status bar shows — because the truncated part
// is precisely the part a reporter cannot otherwise recover.
func TestSurfacedErrorIsLogged(t *testing.T) {
	m, buf := logSink(t)

	m.surfaceError(NewErrorMsg("watch externalsecrets", errors.New("conversion webhook denied the request")))

	got := buf.String()
	for _, want := range []string{
		"level=ERROR",
		"watch externalsecrets",
		"conversion webhook denied the request",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q\ngot: %s", want, got)
		}
	}
}

// TestSurfacedErrorLogsTheClassifiedKind pins that the kind travels with the line.
// The kind is what the shell already decided about the failure, and a reader of the
// log should not have to re-derive "this was RBAC, not a bug" from the raw text.
func TestSurfacedErrorLogsTheClassifiedKind(t *testing.T) {
	m, buf := logSink(t)
	denied := apierrors.NewForbidden(
		schema.GroupResource{Group: "external-secrets.io", Resource: "externalsecrets"},
		"", errors.New("no permission"),
	)

	m.surfaceError(NewErrorMsg("list externalsecrets", denied))

	if got := buf.String(); !strings.Contains(got, "kind=forbidden") {
		t.Errorf("log did not carry the classified kind\ngot: %s", got)
	}
}

// TestSurfacedErrorStillToasts guards the leg against changing behaviour: logging is
// additive, so the status bar must still show what it showed before.
func TestSurfacedErrorStillToasts(t *testing.T) {
	m, _ := logSink(t)

	cmd := m.surfaceError(NewErrorMsg("describe", errors.New("boom")))

	if cmd == nil {
		t.Fatal("surfaceError returned no auto-clear Cmd")
	}
	if body := m.View().Content; !strings.Contains(body, "describe") {
		t.Errorf("error is no longer surfaced in the status bar:\n%s", body)
	}
}

// TestErrorMsgRoutesThroughTheLoggedFunnel proves the funnel is reached by the normal
// message path, not only by a direct call — an ErrorMsg from any async seam lands in
// the log without its own logging call.
func TestErrorMsgRoutesThroughTheLoggedFunnel(t *testing.T) {
	m, buf := logSink(t)

	if _, cmd := m.Update(NewErrorMsg("logs", errors.New("stream reset by peer"))); cmd == nil {
		t.Fatal("ErrorMsg produced no Cmd")
	}
	if got := buf.String(); !strings.Contains(got, "stream reset by peer") {
		t.Errorf("an ErrorMsg through Update was not logged\ngot: %s", got)
	}
}

// TestDefaultModelLogsNothing pins the discard default. A model built without
// WithLogger must be silent: hermetic tests construct hundreds of models and drive
// error paths deliberately, and a global default sink would turn every one of them
// into log noise (and, worse, into stderr writes under a TUI).
func TestDefaultModelLogsNothing(t *testing.T) {
	m := sized(t)
	if m.logger == nil {
		t.Fatal("logger is nil — every log call site would panic")
	}
	if m.logger.Enabled(t.Context(), slog.LevelError) {
		t.Error("a model built without WithLogger has an enabled logger")
	}
}

// TestWithLoggerIgnoresNil keeps the option from re-introducing the nil the discard
// default exists to prevent: a launcher passing a nil logger must leave the model
// loggable, not arm a panic on the first error.
func TestWithLoggerIgnoresNil(t *testing.T) {
	m := sizedWith(t, WithLogger(nil))
	if m.logger == nil {
		t.Fatal("WithLogger(nil) cleared the discard default")
	}
	m.surfaceError(ErrorMsg{Context: "still fine"}) // must not panic
}

// TestDiscoveryFailuresAreLogged covers the shell's one class of failure that never
// reaches a toast at all. A group that fails discovery degrades silently to
// "unavailable" in the menu (#87/#76), which is right on screen and useless when the
// question is why a CRD's kind is missing — so it goes to the log instead.
func TestDiscoveryFailuresAreLogged(t *testing.T) {
	m, buf := logSink(t)

	m.logDiscovery(kube.DiscoveryResult{
		Resources: []kube.Resource{{
			GVK:        schema.GroupVersionKind{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret"},
			GVR:        schema.GroupVersionResource{Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets"},
			Namespaced: true,
			Verbs:      metav1.Verbs{"list"},
		}},
		Failed: []kube.FailedGroup{{
			Group:   "metrics.k8s.io",
			Version: "v1beta1",
			Err:     errors.New("service unavailable"),
		}},
	})

	got := buf.String()
	for _, want := range []string{"level=WARN", "metrics.k8s.io/v1beta1", "service unavailable"} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q\ngot: %s", want, got)
		}
	}
}

// TestTotalDiscoveryFailureIsLoggedAsAnError separates the two severities: one group
// failing is a warning (the menu is still usable), the whole pass failing is an error
// (the menu stays on its seed set and the user sees nothing at all). It also pins the
// early return — a pass that failed wholesale has nothing meaningful to say about
// individual groups, so a stray Failed entry beside a total Err must not add noise
// under the line that actually explains it.
func TestTotalDiscoveryFailureIsLoggedAsAnError(t *testing.T) {
	m, buf := logSink(t)

	m.logDiscovery(kube.DiscoveryResult{
		Err:    errors.New("connection refused"),
		Failed: []kube.FailedGroup{{Group: "metrics.k8s.io", Version: "v1beta1", Err: errors.New("noise")}},
	})

	got := buf.String()
	if !strings.Contains(got, "level=ERROR") || !strings.Contains(got, "connection refused") {
		t.Errorf("a total discovery failure was not logged as an error\ngot: %s", got)
	}
	if strings.Contains(got, "level=WARN") {
		t.Errorf("a total failure also logged per-group warnings\ngot: %s", got)
	}
}

// TestSuccessfulDiscoveryLogsNothing keeps the log signal-only: the healthy path is
// the common one and must not fill the file with noise a reporter has to scroll past.
func TestSuccessfulDiscoveryLogsNothing(t *testing.T) {
	m, buf := logSink(t)

	m.logDiscovery(kube.DiscoveryResult{Resources: []kube.Resource{{
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}}})

	if got := buf.String(); got != "" {
		t.Errorf("a clean discovery pass wrote to the log: %s", got)
	}
}

// TestDiscoveryReadyLogsThroughUpdate proves the discovery log is wired to the real
// message path (handleDiscovery), not only reachable through the helper.
func TestDiscoveryReadyLogsThroughUpdate(t *testing.T) {
	m, buf := logSink(t)

	m.Update(DiscoveryReadyMsg{Result: kube.DiscoveryResult{
		Failed: []kube.FailedGroup{{Group: "external-secrets.io", Version: "v1", Err: errors.New("the server could not find the requested resource")}},
	}})

	if got := buf.String(); !strings.Contains(got, "external-secrets.io/v1") {
		t.Errorf("DiscoveryReadyMsg through Update did not log the failed group\ngot: %s", got)
	}
}
