package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is the boundary between the concurrent `kube` layer and the
// single-threaded Bubble Tea update loop. The kube layer hands work back on
// channels (watch deltas, the discovery-ready signal); Bubble Tea only ever
// wants messages. The types here are those messages, and the pumps below are the
// `tea.Cmd` adapters that read *one* item from a kube channel and return it as a
// message — the only sanctioned way state crosses the goroutine boundary, so no
// UI state is ever shared or mutated across goroutines (principle 1, D1). A pump
// reads exactly one item per invocation: the model re-issues it after each
// delivered message to pull the next, which keeps `Update` from ever blocking on
// more than a single receive.

// ResourceEventMsg is one live table delta from a resource Watch, carried into
// the update loop verbatim from kube.WatchEvent. A RESET event's Rows replace the
// whole table; ADDED/MODIFIED/DELETED affect a single row; Columns is set on
// RESET and carried forward. A watch ERROR event is *not* delivered as a
// ResourceEventMsg — the watch pump bridges it to an ErrorMsg instead — so a
// consumer of ResourceEventMsg only ever sees data deltas.
type ResourceEventMsg struct {
	Event kube.WatchEvent
}

// WatchClosedMsg tells the model a Watch channel has closed and the pump has
// stopped. The kube layer closes a watch channel when its context is cancelled
// (e.g. the user selected a different resource or namespace, so the old watch was
// torn down). It is the pump's terminal message: the model must *not* re-issue
// the pump after receiving it, or it would busy-loop receiving from a closed
// channel.
type WatchClosedMsg struct{}

// DiscoveryReadyMsg carries the outcome of one async discovery pass — the
// "discovery ready" reconcile signal (D8). Result.Resources holds the healthy,
// listable resources; Result.Failed holds per-group failures isolated from them
// (the menu marks those groups unavailable); Result.Err is set only on a *total*
// discovery failure, in which case the model should surface it and retry rather
// than reconcile an empty menu. A total failure is delivered here (not as an
// ErrorMsg) so the model keeps the reconcile signal and the isolated per-group
// detail together — it can classify Result.Err with kube.Classify when rendering.
type DiscoveryReadyMsg struct {
	Result kube.DiscoveryResult
}

// ErrorMsg is a generic, classified error surfaced to the UI so it can degrade a
// single feature — show a message on the affected pane — instead of crashing
// (#86, principle 3). Context is a short human label for what failed (e.g.
// "watch pods", "discovery"); Err is the underlying error, still inspectable via
// errors.Is/As; Kind is kube.Classify(Err), the taxonomy the UI switches on.
// Build one with NewErrorMsg so Kind is always in sync with Err.
type ErrorMsg struct {
	Context string
	Err     error
	Kind    kube.ErrorKind
}

// NewErrorMsg builds an ErrorMsg, classifying err once so callers never set Kind
// out of step with Err. A nil err classifies to kube.KindUnknown.
func NewErrorMsg(context string, err error) ErrorMsg {
	return ErrorMsg{Context: context, Err: err, Kind: kube.Classify(err)}
}

// ResourceSelectedMsg is emitted by the resource menu when the user picks a kind
// to browse; the app reacts by (re)starting a watch for it. It is defined here
// with the other cross-component messages so the menu and app agree on one type.
type ResourceSelectedMsg struct {
	Resource kube.Resource
}

// RowSelectedMsg is emitted by the table when the selected row changes; the app
// reacts by re-scoping actions/viewers to Object. Namespace is empty for
// cluster-scoped resources.
type RowSelectedMsg struct {
	Object kube.ObjectRef
}

// (Terminal size is delivered by Bubble Tea's own tea.WindowSizeMsg; kubecom does
// not define its own size message.)

// watchPump reads one event from a kube.Watch channel and returns it as a
// message: a data delta becomes a ResourceEventMsg, a watch ERROR event becomes a
// classified ErrorMsg, and a closed channel becomes a WatchClosedMsg. The model
// re-issues watchPump after each ResourceEventMsg/ErrorMsg to pull the next event
// (one receive per Cmd — Update never blocks on more than one), and stops
// re-issuing on WatchClosedMsg. The whole receive happens inside the returned
// tea.Cmd, off the update goroutine.
func watchPump(ch <-chan kube.WatchEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return WatchClosedMsg{}
		}
		if ev.Type == kube.WatchError {
			return NewErrorMsg("watch", ev.Err)
		}
		return ResourceEventMsg{Event: ev}
	}
}

// discoveryPump reads the single result from a kube.StartDiscovery channel and
// returns it as a DiscoveryReadyMsg (the discovery channel delivers exactly once
// on a cap-1 buffer, D8). A total failure is carried inside the result — the
// model classifies Result.Err itself — so discovery always yields one reconcile
// signal. A channel closed with no value (should not happen for discovery, but is
// handled rather than panicked on) yields a nil message, which Bubble Tea ignores.
func discoveryPump(ch <-chan kube.DiscoveryResult) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-ch
		if !ok {
			return nil
		}
		return DiscoveryReadyMsg{Result: res}
	}
}
