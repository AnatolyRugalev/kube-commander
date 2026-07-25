package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
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

// openLogs shows the logs view over ref and starts streaming container's logs into it
// at generation gen. It replaces streamLogsInto's shared-viewer open: the view is shown
// immediately (empty, so the gesture feels instant) and the log channel is pumped line
// by line off the update loop (D53), appending each line as it lands — a large or slow
// log never blocks Update. Reset drops any previous object's lines and re-arms following
// (like `kubectl logs -f`), so the view always opens clean and tailing; the follow state
// and the filter now live in the component, not on the model. The stream runs on a
// cancellable context torn down when the view closes or a newer open supersedes it
// (stopLogStream). An open failure degrades to a status-bar toast and leaves the view
// closed (D74); a mid-stream error after some lines already showed keeps them on screen.
// container "" streams the pod's default/sole container.
func (m Model) openLogs(res kube.Resource, ref kube.ObjectRef, container string, gen int) (tea.Model, tea.Cmd) {
	m.stopLogStream() // idempotent; ensures no prior stream survives this open.
	m.logsView.Reset()
	title := viewerTitle(res, ref)
	if container != "" {
		title += " · " + container
	}
	m.logsView.SetTitle(title)

	ctx, cancel := context.WithCancel(context.Background())
	// Follow keeps the stream open and reconnects transparently across transport
	// drops (M1-07d), so the view tails live output; stopLogStream cancels it on
	// close/supersede/quit. The view opens following, so the stream must too.
	ch, err := m.logStreamer.Logs(ctx, ref, kube.LogOptions{Follow: true, Container: container})
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
		m.logsView.Append(inner.Line)
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
func (m Model) routeLogsFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.Key()
	if action, mapped := m.keymap.Action(key); mapped && key.Text == "" {
		return m.handleLogsAction(action)
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
