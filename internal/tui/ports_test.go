package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/modal"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/picker"
)

// fakePortLister is a hermetic PortLister (FB-pf-port-picker-b): it returns a preset
// port set (or an error) and records which method it was asked and with which refs,
// so the picker flow is driven without a cluster (D18).
type fakePortLister struct {
	ports []kube.Port
	err   error

	podCalls int
	svcCalls int
	gotRef   kube.ObjectRef
	gotSvc   kube.ObjectRef
}

func (f *fakePortLister) PodPorts(_ context.Context, ref kube.ObjectRef) ([]kube.Port, error) {
	f.podCalls++
	f.gotRef = ref
	return f.ports, f.err
}

func (f *fakePortLister) ServicePorts(_ context.Context, svcRef, podRef kube.ObjectRef) ([]kube.Port, error) {
	f.svcCalls++
	f.gotSvc = svcRef
	f.gotRef = podRef
	return f.ports, f.err
}

// loadPorts runs the async listing command the Port-forward action issued and delivers
// its result, returning the resulting model. It fails the test if the command is
// missing or produced some other message.
func loadPorts(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("the port-forward action should issue the async ports listing command")
	}
	msg, ok := cmd().(portsLoadedMsg)
	if !ok {
		t.Fatalf("the listing command should produce a portsLoadedMsg, got %T", cmd())
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

// TestPortForwardSingleDeclaredPortForwardsDirectly proves the fast path: a pod
// declaring exactly one port forwards straight away (local = remote), with no prompt
// and no picker in the way.
func TestPortForwardSingleDeclaredPortForwardsDirectly(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{ports: []kube.Port{{Port: 8080, Name: "http", Container: "app"}}}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))
	row, _ := m.table.SelectedRow()

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	if m.modal.Active() {
		t.Fatal("the ports prompt must not open before the declared ports are listed")
	}
	m = loadPorts(t, m, cmd)

	if pl.podCalls != 1 || pl.gotRef.Name != row.Object.Name {
		t.Fatalf("PodPorts should be asked for the selected row %q once, got calls=%d ref=%q", row.Object.Name, pl.podCalls, pl.gotRef.Name)
	}
	if m.modal.Active() || m.portPicker.Active() {
		t.Fatal("a single declared port should forward directly, opening no prompt or picker")
	}
	if pf.calls != 1 || pf.gotRef.Name != row.Object.Name {
		t.Fatalf("the forward should target the selected row once, got calls=%d ref=%q", pf.calls, pf.gotRef.Name)
	}
	if len(pf.gotPorts) != 1 || pf.gotPorts[0] != "8080" {
		t.Fatalf("the forward spec = %v, want [8080] (local = remote)", pf.gotPorts)
	}
	if len(m.forwards) != 1 {
		t.Fatalf("the started forward should be tracked, got %d", len(m.forwards))
	}
}

// TestPortForwardMultiplePortsOpenPicker proves several declared ports open the port
// picker (labelled with name/container), and that the pick forwards that port.
func TestPortForwardMultiplePortsOpenPicker(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{ports: []kube.Port{
		{Port: 8080, Name: "http", Container: "app"},
		{Port: 9090, Name: "metrics", Container: "app"},
	}}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	m = loadPorts(t, m, cmd)

	if !m.portPicker.Active() {
		t.Fatal("several declared ports should open the port picker")
	}
	if m.modal.Active() {
		t.Fatal("the picker replaces the free-text prompt, which must stay closed")
	}
	if pf.calls != 0 {
		t.Fatal("opening the picker must not start a forward yet")
	}
	view := m.View().Content
	for _, want := range []string{"8080", "metrics"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the port picker should list %q: %q", want, view)
		}
	}

	next, _ := m.Update(picker.SelectedMsg{Kind: portPickerKind, Value: portLabel(pl.ports[1])})
	m = next.(Model)
	if m.portPicker.Active() {
		t.Fatal("picking a port should close the picker")
	}
	if pf.calls != 1 || len(pf.gotPorts) != 1 || pf.gotPorts[0] != "9090" {
		t.Fatalf("the picked port should be forwarded, got calls=%d ports=%v", pf.calls, pf.gotPorts)
	}
}

// TestPortPickerCancelStartsNothing proves dismissing the picker (nav.back) closes it
// without forwarding — the flow is abandoned, not silently defaulted.
func TestPortPickerCancelStartsNothing(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{ports: []kube.Port{{Port: 80}, {Port: 443}}}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	m = loadPorts(t, m, cmd)
	next, _ := m.Update(picker.CancelledMsg{Kind: portPickerKind})
	m = next.(Model)

	if m.portPicker.Active() {
		t.Fatal("cancelling should close the port picker")
	}
	if pf.calls != 0 || len(m.forwards) != 0 {
		t.Fatal("cancelling the port picker must start no forward")
	}
}

// TestPortForwardNoDeclaredPortsFallsBackToPrompt proves the D137 contract at the TUI
// edge: declaring ports is optional, so an empty listing must leave the free-text
// ports prompt reachable rather than dead-ending the action.
func TestPortForwardNoDeclaredPortsFallsBackToPrompt(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{} // the pod declares nothing
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	m = loadPorts(t, m, cmd)

	if !m.modal.Active() || !m.modal.Prompting() || m.modal.Kind() != portForwardModalKind {
		t.Fatal("no declared ports should fall back to the free-text ports prompt")
	}
	if m.portPicker.Active() {
		t.Fatal("an empty listing must not open the picker")
	}
	if pf.calls != 0 {
		t.Fatal("the fallback prompt must not start a forward on its own")
	}
	// The prompt still forwards what the user types, exactly as before the picker.
	next, _ := m.Update(modal.ConfirmedMsg{Kind: portForwardModalKind, Value: "8080:80"})
	m = next.(Model)
	if pf.calls != 1 || pf.gotPorts[0] != "8080:80" {
		t.Fatalf("the fallback prompt should still start the typed forward, got calls=%d ports=%v", pf.calls, pf.gotPorts)
	}
}

// TestPortForwardListErrorFallsBackToPrompt proves a listing failure (RBAC denial, the
// object vanished) degrades to the free-text prompt: the picker is an affordance, so
// losing it must not lose the action.
func TestPortForwardListErrorFallsBackToPrompt(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{err: errors.New("pods is forbidden")}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	m = loadPorts(t, m, cmd)

	if !m.modal.Active() || !m.modal.Prompting() {
		t.Fatal("a ports-listing error should fall back to the free-text ports prompt")
	}
	if m.portPicker.Active() {
		t.Fatal("a ports-listing error must not open the picker")
	}
}

// TestPortForwardWithoutListerOpensPrompt proves the pre-picker behaviour is intact:
// with no lister wired the action opens the free-text prompt in the same update, with
// no listing hop in between.
func TestPortForwardWithoutListerOpensPrompt(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	m := openPodTable(t, "Pod", WithPortForwarder(pf)) // no WithPortLister

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	if !m.modal.Active() || !m.modal.Prompting() || m.modal.Kind() != portForwardModalKind {
		t.Fatal("with no port lister wired the action should open the ports prompt directly")
	}
	if cmd != nil {
		if _, listing := cmd().(portsLoadedMsg); listing {
			t.Fatal("with no port lister wired the action must issue no ports listing")
		}
	}
}

// TestPortForwardServiceListsServicePorts proves the Service path: the backing pod is
// resolved first (M3-13c), then the *Service's* ports are listed against both refs and
// the forward targets the pod-side number (the Service port is only a label, D137).
func TestPortForwardServiceListsServicePorts(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	sr := &fakeServiceResolver{pod: kube.ObjectRef{Namespace: "web", Name: "api-xyz"}}
	pl := &fakePortLister{ports: []kube.Port{{Port: 8080, ServicePort: 80, Name: "http", Container: "api"}}}
	m := openPodTable(t, "Service", WithPortForwarder(pf), WithServiceResolver(sr), WithPortLister(pl))
	row, _ := m.table.SelectedRow()

	m, resolveCmd := dispatchRowAction(t, m, rowActionPortForward)
	next, listCmd := m.Update(resolveCmd().(serviceResolvedMsg))
	m = loadPorts(t, next.(Model), listCmd)

	if pl.svcCalls != 1 || pl.podCalls != 0 {
		t.Fatalf("a Service should be listed through ServicePorts, got svc=%d pod=%d", pl.svcCalls, pl.podCalls)
	}
	if pl.gotSvc.Name != row.Object.Name || pl.gotRef.Name != "api-xyz" {
		t.Fatalf("ServicePorts should get the Service %q and the resolved pod api-xyz, got svc=%q pod=%q", row.Object.Name, pl.gotSvc.Name, pl.gotRef.Name)
	}
	if pf.calls != 1 || pf.gotRef.Name != "api-xyz" {
		t.Fatalf("the forward should target the resolved pod once, got calls=%d ref=%q", pf.calls, pf.gotRef.Name)
	}
	if len(pf.gotPorts) != 1 || pf.gotPorts[0] != "8080" {
		t.Fatalf("the forward should use the pod-side targetPort, got %v, want [8080]", pf.gotPorts)
	}
}

// TestPortForwardStalePortListingDropped pins the generation guard: a listing that
// lands after a newer port-forward request superseded it opens neither picker nor
// prompt and starts nothing.
func TestPortForwardStalePortListingDropped(t *testing.T) {
	pf := &fakePortForwarder{handle: newFakeForward()}
	pl := &fakePortLister{ports: []kube.Port{{Port: 80}, {Port: 443}}}
	m := openPodTable(t, "Pod", WithPortForwarder(pf), WithPortLister(pl))

	m, cmd := dispatchRowAction(t, m, rowActionPortForward)
	stale := cmd().(portsLoadedMsg)
	m.pfResolveGen++ // a newer port-forward request supersedes the in-flight listing
	next, _ := m.Update(stale)
	m = next.(Model)

	if m.portPicker.Active() || m.modal.Active() {
		t.Fatal("a superseded ports listing should be dropped, opening no picker or prompt")
	}
	if pf.calls != 0 {
		t.Fatal("a superseded ports listing must start no forward")
	}
}

// TestPortLabelAndSpec pins the picker row rendering and the spec a pick becomes: a
// Service-derived port shows both numbers ("80 → 8080") while the forward uses the
// pod-side one; name/container annotate the row when known.
func TestPortLabelAndSpec(t *testing.T) {
	tests := []struct {
		name      string
		port      kube.Port
		wantLabel string
		wantSpec  string
	}{
		{"bare", kube.Port{Port: 6379}, "6379", "6379"},
		{"named", kube.Port{Port: 8080, Name: "http"}, "8080 (http)", "8080"},
		{"named in container", kube.Port{Port: 8080, Name: "http", Container: "app"}, "8080 (http · app)", "8080"},
		{"container only", kube.Port{Port: 8080, Container: "app"}, "8080 (app)", "8080"},
		{"service mapped", kube.Port{Port: 8080, ServicePort: 80, Name: "http"}, "80 → 8080 (http)", "8080"},
		{"service same number", kube.Port{Port: 80, ServicePort: 80}, "80", "80"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := portLabel(tc.port); got != tc.wantLabel {
				t.Errorf("portLabel = %q, want %q", got, tc.wantLabel)
			}
			if got := portForwardSpec(tc.port); got != tc.wantSpec {
				t.Errorf("portForwardSpec = %q, want %q", got, tc.wantSpec)
			}
		})
	}

	ports := []kube.Port{{Port: 80}, {Port: 8080, Name: "http"}}
	if got, ok := portForLabel(ports, "8080 (http)"); !ok || got.Port != 8080 {
		t.Fatalf("portForLabel should map a rendered row back to its port, got %v ok=%v", got, ok)
	}
	if _, ok := portForLabel(ports, "1234"); ok {
		t.Fatal("portForLabel must not resolve a label no listed port rendered")
	}
}
