package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
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
