package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
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
// gen tags the discovery pass the result belongs to, exactly as watchGen tags the
// table watch: cancelling a pass does not guarantee its goroutine loses the race to
// the cap-1 buffer (StartDiscovery selects between the send and ctx.Done), so a
// pass cancelled by a cluster reset can still deliver — and reconciling the
// *previous* cluster's resources into the new context's menu is a correctness bug,
// not a leak (M4-03/D155 pt 1). The zero value is generation 0, the pass the model
// starts at launch, so a message constructed without one is the launch pass's.
type DiscoveryReadyMsg struct {
	Result kube.DiscoveryResult
	gen    int
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

// Message renders the error as a short human line for the status bar: the context
// label and the underlying error joined, degrading gracefully if either is empty.
// The status bar flattens any embedded newlines, so this need not.
func (e ErrorMsg) Message() string {
	switch {
	case e.Err == nil:
		return e.Context
	case e.Context == "":
		return e.Err.Error()
	default:
		return e.Context + ": " + e.Err.Error()
	}
}

// A message emitted *by* a component (rather than by the kube boundary above) is
// owned by that component's package, not declared here: the root model imports
// the component packages, so a component cannot import this one without a cycle
// (D56). The resource menu's "resource selected" message therefore lives in
// internal/tui/components/menu (menu.ResourceSelectedMsg); the table's
// "row selected" message likewise lives in the table package
// (table.RowSelectedMsg, M2-06a).
// The types in this file are the kube-boundary messages and the generic ErrorMsg,
// which no component originates.

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

// LogLineMsg is a run of log lines from a pod's log stream (M3-05), carried into the
// update loop verbatim from kube.LogEvent Lines (the trailing newline stripped — the
// view joins lines itself). Lines always holds at least one line. A stream error is
// *not* delivered as a line — the log pump bridges it to a classified ErrorMsg — so a
// consumer of Lines only ever sees data lines.
//
// It is a batch rather than a single line because showing a line costs a pass over the
// whole body (LOGS-05b): one message per line made a fast pod quadratic. End carries the
// stream's terminal message (LogClosedMsg or an ErrorMsg) when the same drain that
// collected Lines also consumed the end of the stream — the batch cannot simply drop it,
// since a channel receive is destructive and there is no way to put it back. It is nil
// in the common case, and the model applies the lines first and then that message, so a
// mid-stream error still lands after the lines it followed.
type LogLineMsg struct {
	Lines []string
	End   tea.Msg
}

// logBatchMax bounds how many lines one pump Cmd delivers. The drain is non-blocking, so
// it stops on its own the moment the stream is not ahead of the UI — this cap is only for
// a producer that is *always* ahead (a pod flooding faster than the terminal can draw),
// where an unbounded drain would keep collecting instead of ever yielding a frame. A few
// thousand lines is far more than any terminal shows and still one render, so the cap
// costs nothing in the case it does not fire.
const logBatchMax = 2048

// LogClosedMsg tells the model a Logs channel has closed and the pump has stopped —
// for a non-following stream (M3-05) this is the normal end of the log (EOF). Like
// WatchClosedMsg it is the pump's terminal message: the model must not re-issue the
// pump after receiving it, or it would busy-loop receiving from a closed channel.
type LogClosedMsg struct{}

// logPump reads from a kube.Logs channel and returns what it read as a message: lines
// become a LogLineMsg, a terminal error event (Err set) becomes a classified ErrorMsg,
// and a closed channel becomes a LogClosedMsg. The model re-issues logPump after each
// LogLineMsg to pull the next batch, and stops re-issuing on LogClosedMsg or the bridged
// ErrorMsg (a LogEvent with Err set is always the stream's last event, so the channel
// closes right after; the model does not re-pump past an error). The whole receive
// happens inside the returned tea.Cmd, off the update goroutine.
//
// It blocks for the first event and then drains whatever is *already* buffered in the
// channel, up to logBatchMax, into one message (LOGS-05b). The blocking receive is what
// keeps Update from ever spinning (M2-02/D53) and the non-blocking drain is what keeps a
// fast stream from costing one render per line: when the producer is not ahead the drain
// takes nothing and this is exactly the old one-line-per-Cmd pump. A terminal event met
// mid-drain rides along in End rather than being dropped.
func logPump(ch <-chan kube.LogEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return LogClosedMsg{}
		}
		if ev.Err != nil {
			return NewErrorMsg("logs", ev.Err)
		}
		batch := LogLineMsg{Lines: []string{ev.Line}}
		for len(batch.Lines) < logBatchMax {
			select {
			case ev, ok := <-ch:
				switch {
				case !ok:
					batch.End = LogClosedMsg{}
					return batch
				case ev.Err != nil:
					batch.End = NewErrorMsg("logs", ev.Err)
					return batch
				default:
					batch.Lines = append(batch.Lines, ev.Line)
				}
			default:
				return batch
			}
		}
		return batch
	}
}

// SearchEventMsg is one event from a cluster-search fan-out (SEARCH-02b), carried
// into the update loop verbatim from kube.Search's channel: a match, a kind that
// finished (the progress signal), or the terminal done-with-cap-reason
// (SEARCH-03a). A search has no error channel — a per-kind List failure is
// swallowed by the kube layer into SearchKindDone{Failed} and simply contributes
// nothing (D131 pt 3) — so a consumer only ever sees events and the terminal
// SearchClosedMsg.
type SearchEventMsg struct {
	Event kube.SearchEvent
}

// SearchClosedMsg tells the model a Search channel has closed and the pump has
// stopped: the fan-out finished, hit its cap, or was cancelled (a new query or the
// view closing). Like WatchClosedMsg it is the pump's terminal message — the model
// must not re-issue the pump after it.
type SearchClosedMsg struct{}

// searchPump reads one event from a kube.Search channel and returns it as a
// SearchEventMsg, or SearchClosedMsg when the channel closes. The model re-issues it
// after each event to pull the next (one receive per Cmd, M2-02/D53) and stops on the
// close, so results stream into the view as each kind returns rather than landing in
// one batch at the end. The whole receive happens inside the returned tea.Cmd, off
// the update goroutine.
func searchPump(ch <-chan kube.SearchEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return SearchClosedMsg{}
		}
		return SearchEventMsg{Event: ev}
	}
}

// ScanEventMsg is one event from a cross-kind scan sweep (STORY-06g-2), carried
// into the update loop verbatim from kube.Scan's channel: a match, a kind that
// finished (the progress signal), or the terminal done-with-cap-reason (D276). A
// scan has no error channel — a per-kind List failure is swallowed by the kube
// layer into ScanKindDone{Failed} and simply contributes nothing (D131 pt 3) — so
// a consumer only ever sees events and the terminal ScanClosedMsg.
type ScanEventMsg struct {
	Event kube.ScanEvent
}

// ScanClosedMsg tells the model a Scan channel has closed and the pump has
// stopped: the sweep finished, hit its cap, or was cancelled (the view closing or
// the app quitting). Like SearchClosedMsg it is the pump's terminal message — the
// model must not re-issue the pump after it.
type ScanClosedMsg struct{}

// scanPump reads one event from a kube.Scan channel and returns it as a
// ScanEventMsg, or ScanClosedMsg when the channel closes. The model re-issues it
// after each event to pull the next (one receive per Cmd, M2-02/D53) and stops on
// the close, so hits stream into the view as each kind returns rather than
// landing in one batch at the end. The whole receive happens inside the returned
// tea.Cmd, off the update goroutine.
func scanPump(ch <-chan kube.ScanEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return ScanClosedMsg{}
		}
		return ScanEventMsg{Event: ev}
	}
}

// DrainProgressMsg is one progress step from a streaming node drain (M3-11b),
// carried into the update loop verbatim from a kube.DrainEvent's Message ("evicted
// ns/web (2/5)"). A drain failure is *not* delivered as a DrainProgressMsg — the
// drain pump bridges it to a DrainDoneMsg with Err set — so a consumer of
// DrainProgressMsg only ever sees a progress line.
type DrainProgressMsg struct {
	Message string
}

// DrainDoneMsg tells the model a drain finished: Err nil is a clean success (the
// DrainStream channel closed with no terminal error), a non-nil Err the wrapped
// failure. Like the log pump's terminal messages it ends the pump chain — the
// model must not re-issue drainPump after it.
type DrainDoneMsg struct {
	Err error
}

// drainPump reads one event from a kube.DrainStream channel and returns it as a
// message: a progress event becomes a DrainProgressMsg, a terminal error event
// (Err set) becomes a DrainDoneMsg carrying it, and a cleanly closed channel
// becomes a DrainDoneMsg with no Err (the success terminator). The model re-issues
// drainPump after each DrainProgressMsg to pull the next step (one receive per Cmd,
// M2-02/D53), and stops on either DrainDoneMsg. The whole receive happens inside
// the returned tea.Cmd, off the update goroutine.
func drainPump(ch <-chan kube.DrainEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return DrainDoneMsg{} // channel closed cleanly → drain succeeded
		}
		if ev.Err != nil {
			return DrainDoneMsg{Err: ev.Err}
		}
		return DrainProgressMsg{Message: ev.Message}
	}
}

// discoveryPump reads the single result from a kube.StartDiscovery channel and
// returns it as a DiscoveryReadyMsg (the discovery channel delivers exactly once
// on a cap-1 buffer, D8). A total failure is carried inside the result — the
// model classifies Result.Err itself — so discovery always yields one reconcile
// signal. A channel closed with no value (should not happen for discovery, but is
// handled rather than panicked on) yields a nil message, which Bubble Tea ignores.
// The result is tagged with the generation of the pass that produced it so the
// model can drop one belonging to a cluster it has since left (M4-03).
func discoveryPump(ch <-chan kube.DiscoveryResult, gen int) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-ch
		if !ok {
			return nil
		}
		return DiscoveryReadyMsg{Result: res, gen: gen}
	}
}
