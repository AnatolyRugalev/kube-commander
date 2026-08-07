package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/components/logsview"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
)

// This file is the app wiring of the dedicated logs mini-app (LOGS-02): the terminal
// of the res.logs flow, the generation-guarded log pump that feeds the LOGS-01
// component, and the key routing that gives the view its live grep. Logs used to share
// the M3-01 read-only viewer; that path is retired here (D144). The shared viewer keeps
// serving YAML / describe / secret — one-shot content a pager suits — while logs, which
// stream continuously and want every row plus a filter that narrows *while following*,
// get their own full-screen view (D134).
//
// The container picker (M3-07a) and the pod-owning resolution (M3-07b) are unchanged
// and still feed this terminal: they live in app.go because exec shares them, and both
// still tag their async work with viewerGen, which remains the one "an async open was
// superseded" clock for every content surface (shared viewer and logs view alike).

// defaultLogTail is how much history a logs open replays before it starts tailing live
// output (`kubectl logs --tail=1000`). Without it the server streams the container's log
// from boot, so opening the logs of a pod that has been up for a week means waiting for a
// week of output to arrive and be rendered before reaching *now* — and "now" is what the
// gesture is for (feedback `2026-07-29-logs-tail-and-perf`).
//
// 1000 is chosen to be a screenful-of-screenfuls: comfortably more than any terminal can
// show (so scrolling back has somewhere to go, and a grep over what just happened has a
// corpus), and small enough that the open is bounded work regardless of the container's
// age. It is deliberately not user-configurable yet — kubecom's config carries only
// `keys:` today, and a knob nobody has asked to turn is not worth a config section.
const defaultLogTail int64 = 1000

// logRequest is everything one logs open is made of: the object whose logs are shown,
// the container inside it (empty = the pod's default/sole one), and which instance —
// the running one, or the previous terminated one (M5-01a). The model keeps the last
// one so logs.previous can re-issue the very same open with that one bit flipped,
// without re-resolving the owning workload's pod or re-asking which container.
type logRequest struct {
	res       kube.Resource
	ref       kube.ObjectRef
	container string
	previous  bool
}

// openLogs shows the logs view over ref and starts streaming container's logs into it
// at generation gen — the fresh open every row gesture takes. Reset drops any previous
// object's lines and its reader state, so the view always opens clean, tailing and on
// the running instance. container "" streams the pod's default/sole container.
func (m Model) openLogs(res kube.Resource, ref kube.ObjectRef, container string, gen int) (tea.Model, tea.Cmd) {
	m.logsView.Reset()
	return m.startLogStream(logRequest{res: res, ref: ref, container: container}, gen)
}

// toggleLogsPrevious flips the open logs view between the container's running instance
// and its previous terminated one (logs.previous, M5-01a) — `kubectl logs -p`, and the
// gesture that answers why a CrashLoopBackOff pod is crash-looping, since the log that
// explains it belongs to the instance that already died.
//
// It re-issues the *same* request with Previous flipped rather than being a second row
// action, which is what makes it cheap: the pod resolution (M3-07b) and the container
// pick (M3-07a) that got the reader here are already spent, and one entry point means
// `L` stays the only way to reach logs. Two instances are two different logs, so this
// really is a restream — the buffer is replaced — but Restream keeps the reader's grep,
// wrap and timestamps, since the point of flipping is to look for the same thing in the
// other log. The generation is bumped so lines still draining from the stream being
// replaced are dropped rather than interleaved into the new instance's output.
//
// With no previous instance the server rejects the request and the stream's terminal
// error lands on an empty view, which the existing open-failure path (handleLogMsg)
// degrades exactly as it degrades any other: a status-bar toast naming the reason, and
// the view closes (D74). Nothing here pre-checks for one — only the apiserver knows,
// and a wrong guess would either hide a readable log or promise one that is not there.
func (m Model) toggleLogsPrevious() (tea.Model, tea.Cmd) {
	if m.logStreamer == nil || m.logReq.ref.Name == "" {
		return m, nil // logs-viewer-inert, or nothing has streamed yet.
	}
	req := m.logReq
	req.previous = !req.previous
	m.logsView.Restream()
	m.viewerGen++
	return m.startLogStream(req, m.viewerGen)
}

// startLogStream is the shared tail of every logs open: it tears down whatever stream
// was running, titles the view for req, and starts streaming. The view is shown
// immediately (empty, so the gesture feels instant) and the log channel is pumped line
// by line off the update loop (D53), appending each line as it lands — a large or slow
// log never blocks Update. The stream runs on a cancellable context torn down when the
// view closes or a newer open supersedes it (stopLogStream). An open failure degrades to
// a status-bar toast and leaves the view closed (D74); a mid-stream error after some
// lines already showed keeps them on screen. The caller has already emptied the buffer
// (Reset for a new object, Restream for an instance flip), which is the one thing the
// two paths do differently.
func (m Model) startLogStream(req logRequest, gen int) (tea.Model, tea.Cmd) {
	m.stopLogStream() // idempotent; ensures no prior stream survives this open.
	m.logReq = req
	title := viewerTitle(req.res, req.ref)
	if req.container != "" {
		title += " · " + req.container
	}
	m.logsView.SetTitle(title)
	// Which instance is on screen is a property of the request, so the view is told
	// with every open — including the fresh ones, where it re-asserts the false Reset
	// just set.
	m.logsView.SetPrevious(req.previous)

	ctx, cancel := context.WithCancel(context.Background())
	// Follow keeps the stream open and reconnects transparently across transport
	// drops (M1-07d), so the view tails live output; stopLogStream cancels it on
	// close/supersede/quit. The view opens following, so the stream must too.
	//
	// Timestamps is asked for unconditionally, even though the view starts with them
	// hidden (LOGS-04b/D148): a following stream already forces server timestamps on
	// the wire so the kube layer can anchor its reconnect, so this costs nothing and
	// only stops them being stripped before delivery. Having them in the buffer is
	// what lets logs.timestamps be a redraw rather than a re-fetch — the reader never
	// loses a line, their grep or their place to see when something happened.
	//
	// TailLines bounds the history replayed before the live tail begins
	// (defaultLogTail): the view exists to show what a container is doing now, and
	// re-reading its whole life to get there is the wait the feedback named. It governs
	// only this initial read — a follow reconnect anchors on the last line's timestamp
	// instead (M1-07d), so a transient drop resumes where the reader was rather than
	// re-tailing the last 1000 lines on top of them.
	//
	// Previous carries the instance choice (M5-01a). It rides alongside Follow rather
	// than instead of it: a terminated instance's log cannot grow, so the kubelet
	// serves it and closes, and a clean end is exactly how the kube layer stops
	// following (followLogStream) — the same way following a running pod ends when its
	// container dies. So the flip changes one bit and nothing else about the request.
	tail := defaultLogTail
	ch, err := m.logStreamer.Logs(ctx, req.ref, kube.LogOptions{
		Follow:     true,
		Container:  req.container,
		Previous:   req.previous,
		Timestamps: true,
		TailLines:  &tail,
	})
	if err != nil {
		cancel()
		return m, m.surfaceError(NewErrorMsg("logs", err))
	}
	m.logCancel = cancel
	m.logCh = ch
	m.logsView.Show()
	m.syncHints() // the logs view owns input now → logs-context hints
	return m, m.pumpLogs(gen)
}

// pumpLogs issues the tea.Cmd that pulls the next line from the current log channel,
// tagged with the generation that started the stream so a line from a superseded open
// is recognisable as stale. It returns nil when no stream is active.
func (m Model) pumpLogs(gen int) tea.Cmd {
	ch := m.logCh
	if ch == nil {
		return nil
	}
	pump := logPump(ch)
	return func() tea.Msg { return logMsg{gen: gen, msg: pump()} }
}

// handleLogMsg applies one log-pump message to the logs view and re-issues the pump to
// pull the next line — the one-receive-per-Cmd loop that keeps Update from ever blocking
// (M2-02/D53). A message from a superseded stream (wrong gen) or one that arrives after
// the view closed is dropped and its chain stops. A line is appended (the component
// decides whether the filter shows it and whether following pins the viewport); a closed
// channel ends the chain (the normal EOF of a non-following stream); a bridged stream
// error degrades — it surfaces a transient status-bar toast (D74) and closes the view
// only if nothing was shown yet (an open failure), leaving any partial lines on screen
// for a mid-stream drop.
func (m Model) handleLogMsg(l logMsg) (tea.Model, tea.Cmd) {
	if l.gen != m.viewerGen || !m.logsView.Active() {
		return m, nil // superseded stream or closed view; drop and stop this chain.
	}
	switch inner := l.msg.(type) {
	case LogLineMsg:
		// The stream is timestamped (see openLogs), so each line arrives as
		// "<RFC3339Nano> <message>". Split it once here, at the boundary, and hand the
		// two parts to the view separately: the message is what the grep matches and
		// what is always drawn, the stamp only appears while logs.timestamps is on
		// (LOGS-04b/D148). A line the server did not stamp splits to an empty stamp and
		// itself, so it still shows verbatim.
		//
		// The whole batch goes in with one call so the view renders it once (LOGS-05b).
		batch := make([]logsview.Line, len(inner.Lines))
		for i, raw := range inner.Lines {
			stamp, line := kube.SplitLogTimestamp(raw)
			batch[i] = logsview.Line{Stamp: stamp, Message: line}
		}
		m.logsView.AppendBatch(batch)
		// A drain that reached the end of the stream carries it in End: apply it now,
		// after its lines, instead of re-pumping a channel that has nothing left.
		if inner.End != nil {
			return m.handleLogMsg(logMsg{gen: l.gen, msg: inner.End})
		}
		return m, m.pumpLogs(l.gen)
	case LogClosedMsg:
		m.stopLogStream() // stream ended (EOF); release the context, keep the lines shown.
		return m, nil
	case ErrorMsg:
		empty := m.logsView.Empty()
		m.stopLogStream()
		if empty {
			m.closeLogs() // nothing shown yet (an open failure) → close the empty view.
		}
		return m, m.surfaceError(inner)
	}
	return m, nil
}

// routeLogsFilterKey resolves one keypress while the logs view's live grep is open. The
// filter field captures text, so the split is by whether the key carries text — the same
// split the search view makes (D140 pt 1) and the table filter before it: a mapped
// no-text key (esc/arrows/ctrl+…) is a control Action the view consumes, while anything
// text-producing or editing (a rune, or an unmapped no-text key like backspace) is
// filter input. So `f` and `q` and `/` type a character instead of firing their action,
// and only esc (clear the filter) / ctrl+c (close the view) / the arrows still act. No
// view matches a raw key for behaviour (D11).
//
// Keys with the filter *closed* are deliberately not routed here: they go through the
// sequencer like every browse key, so `gg`/`G` work in a log the way they work
// everywhere else (handleLogsAction is the action end of that path).
//
// The one editing key that is not filter input is a backspace on an empty query: it
// resolves to nav.back (D238), which here closes the grep and restores the full stream
// — the same step of the view's own esc ladder, never the step that dismisses the view,
// because this route is only reached while the filter is open.
func (m Model) routeLogsFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		return m.handleLogsAction(action)
	}
	if isEmptyLineBackspace(key, m.logsView.Query()) {
		return m.handleLogsAction(keymap.ActionBack)
	}
	var cmd tea.Cmd
	m.logsView, cmd = m.logsView.UpdateFilter(msg)
	m.syncHints() // the filter may have closed under us (esc → cleared)
	return m, cmd
}

// handleLogsAction routes a resolved action to the open logs view. app.quit closes the
// view rather than exiting kubecom — a full-screen pager owns the quit key while it is
// up, exactly as the help modal, the shared viewer and the search view do. Everything
// else is handed to the component, which scrolls, opens/narrows the live grep, toggles
// follow (pausing it on any upward scroll), and closes itself on nav.back via its own
// ClosedMsg. Actions the view does not honour are swallowed, so nothing underneath moves
// while the logs view is up (the capture pattern).
func (m Model) handleLogsAction(a keymap.Action) (tea.Model, tea.Cmd) {
	if a == keymap.ActionQuit {
		m.closeLogs()
		return m, nil
	}
	// logs.previous is handled here rather than in the component: it is a *request*
	// flag, not a display mode, so honouring it means re-opening the stream — which
	// only the shell can do (M5-01a).
	if a == keymap.ActionLogsPrevious {
		return m.toggleLogsPrevious()
	}
	var cmd tea.Cmd
	m.logsView, cmd = m.logsView.Update(a)
	// The filter may have just opened (app.filter) or closed (nav.back), which moves
	// input ownership between the action set and the text field — so the hint context
	// has to be re-picked here, not only when the view itself opens (D143 pt 3).
	m.syncHints()
	return m, cmd
}

// closeLogs dismisses the logs view and cancels the stream feeding it, bumping the
// generation so lines still draining from the cancelled stream are dropped rather than
// appended to a view that is no longer up. It mutates the receiver, so callers pass the
// addressable model they are about to return.
func (m *Model) closeLogs() {
	m.logsView.Hide()
	m.stopLogStream()
	m.viewerGen++
	m.syncHints() // back to the browse view → menu/table-context hints
}

// stopLogStream cancels the live log stream (if any) and clears its handles, so the
// stream's goroutine is torn down and no stale line is pumped. Safe to call with no
// stream active. Called before starting a new stream, when the view closes, and on
// quit — the log twin of stopSearch. It deliberately does not bump the generation:
// an EOF is not a supersede, and a later close/open owns that bump.
func (m *Model) stopLogStream() {
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.logCh = nil
}
