package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is AUTH-04b: how the diagnosis of a failed credential plugin *gets*
// onto the browse pane whose copy AUTH-04a wrote (authFailure, browsefail.go).
//
// The shape follows from what the failure is. client-go runs the kubeconfig's
// `user.exec` plugin for every request and, when it fails, returns text carrying
// nothing but the executable's name and an exit code — the plugin's own stderr goes
// to the process's os.Stderr, which under the alt-screen nobody ever sees (D195
// pt 3). So the only way to say *why* authentication failed is to re-run the plugin
// and read what it prints, which is a subprocess: it cannot happen inside Update.
// It is therefore an async Cmd, tagged with watchGen like every other per-cluster
// async here, and its result is written into the pane only if the pane is still the
// one that failed.
//
// Two properties beyond the generation guard:
//
//   - **One re-run per browse selection.** The watch loop re-Lists on a backoff, so
//     a cluster that stays unauthenticated emits an ErrorMsg every few seconds;
//     without the authDiagGen latch each one would spawn another `aws eks get-token`.
//     The notice a diagnosis writes survives until the pane recovers, so one is
//     enough — and re-opening the resource (any drill-in, re-scope or switch bumps
//     watchGen) re-arms it.
//   - **The seam is not part of Cluster.** It answers a question about the
//     *kubeconfig* — which is per-process — and takes the ClientConfig at call time
//     from the shell's own kubeconfig + context fields, so a context switch needs it
//     repointed no more than a theme does (D155 pt 2's converse).

// AuthDiagnoser is the narrow slice of the kube layer the shell needs to explain a
// credential-plugin failure: re-run the plugin behind a kubeconfig context and
// report what it said, plus the remediation its stanza substantiates
// (kube.DiagnoseExecPlugin). As with every other seam the shell depends on the
// interface, so the flow is driveable in hermetic tests with a fake (D18) and the
// tui package neither reads a kubeconfig nor starts a process itself.
//
// A model built without one (the default) is diagnosis-inert: a plugin failure keeps
// the one generic sentence its kind already has, which is true — just thinner.
//
// It takes the kube.ClientConfig per call rather than being bound to one at
// construction, because the context half of that config changes under it: the shell
// switches contexts (M4-04) and the plugin that failed is the one behind the context
// it is on *now*.
type AuthDiagnoser interface {
	DiagnoseExecPlugin(ctx context.Context, cc kube.ClientConfig) (kube.ExecPluginReport, error)
}

// AuthDiagnoserFunc adapts a plain function to an AuthDiagnoser, so the launcher can
// wire kube.DiagnoseExecPlugin — a package function, since a diagnosis is about the
// kubeconfig and not about a connected client — in one line.
type AuthDiagnoserFunc func(ctx context.Context, cc kube.ClientConfig) (kube.ExecPluginReport, error)

// DiagnoseExecPlugin calls the wrapped function, satisfying AuthDiagnoser.
func (f AuthDiagnoserFunc) DiagnoseExecPlugin(ctx context.Context, cc kube.ClientConfig) (kube.ExecPluginReport, error) {
	return f(ctx, cc)
}

// WithAuthDiagnoser wires the credential-plugin diagnosis the browse pane uses to
// replace a KindExecPlugin failure's generic sentence with the plugin's own account
// of it (AUTH-04b). Without it that failure keeps the generic sentence.
func WithAuthDiagnoser(d AuthDiagnoser) Option {
	return func(m *Model) { m.authDiagnoser = d }
}

// authDiagTimeout is the shell's backstop on a diagnosis. kube.Diagnose already
// bounds the run (15s) and the wait after the kill (2s); this sits above both so the
// kube layer's own bound is what normally fires, and a Cmd that somehow outlived it
// still ends rather than holding a subprocess for the session.
const authDiagTimeout = 25 * time.Second

// authDiagMsg carries one completed diagnosis back onto the update loop. It is
// tagged with the watchGen of the browse selection that failed, and carries both the
// resource and the ErrorMsg that started it: the renderer needs the kind for its
// headline and the error for the degrade path (a report with no plugin renders as
// browseFailure), and re-reading either off the model when the message lands would
// read the *current* pane's, which is exactly what the generation guard exists to
// distinguish.
//
// The whole kube.Resource rather than its kind (AUTH-05b), because a remediation
// armed off this message retries the request that failed — and m.current is not that
// resource on the path where the watch never started: it still names the resource the
// reader came *from*, since watchResource assigns it only after the watch is running.
type authDiagMsg struct {
	gen  int
	res  kube.Resource
	fail ErrorMsg
	rep  kube.ExecPluginReport
	err  error
}

// diagnoseAuth issues the diagnosis for a browse failure, or returns nil when there
// is nothing to diagnose: no seam wired, a failure that is not the credential
// plugin's, or a selection whose plugin has already been re-run.
//
// It mutates the receiver (the latch and the cancel), so callers pass the addressable
// model value they are about to return.
func (m *Model) diagnoseAuth(res kube.Resource, e ErrorMsg) tea.Cmd {
	if m.authDiagnoser == nil || e.Kind != kube.KindExecPlugin {
		return nil
	}
	if m.authDiagGen == m.watchGen {
		return nil // this selection's plugin has already been re-run.
	}
	// A diagnosis still in flight belongs to a selection this one supersedes: its
	// message is already stale by generation, and cancelling ends its subprocess
	// rather than leaving it to the kube layer's timeout.
	if m.authDiagCancel != nil {
		m.authDiagCancel()
	}
	m.authDiagGen = m.watchGen
	ctx, cancel := context.WithTimeout(context.Background(), authDiagTimeout)
	m.authDiagCancel = cancel
	diagnoser, gen := m.authDiagnoser, m.watchGen
	cc := kube.ClientConfig{Kubeconfig: m.kubeconfig, Context: m.context}
	return func() tea.Msg {
		rep, err := diagnoser.DiagnoseExecPlugin(ctx, cc)
		return authDiagMsg{gen: gen, res: res, fail: e, rep: rep, err: err}
	}
}

// stopAuthDiag cancels an in-flight diagnosis and re-arms the latch, so the cluster
// the shell lands on next is diagnosed on its own first failure. Called from
// stopClusterAsync — a diagnosis names a context, and the subprocess it started is
// asking about the kubeconfig entry for a cluster that is being left.
//
// Safe to call with nothing running. It mutates the receiver.
func (m *Model) stopAuthDiag() {
	if m.authDiagCancel != nil {
		m.authDiagCancel()
		m.authDiagCancel = nil
	}
	m.authDiagGen = 0
}

// handleAuthDiagMsg writes a completed diagnosis into the browse pane, replacing the
// kind's generic sentence with the plugin's own account of the failure (authFailure).
//
// Everything it declines to write is a case where the notice on screen is *not* the
// one this diagnosis explains:
//
//   - a superseded generation — the reader has drilled into something else, and the
//     pane now failing (or filling) is not the one that asked;
//   - a diagnosis that could not be attempted, or that names no plugin at all — the
//     generic sentence stays, and the reason is logged rather than toasted (the
//     reader asked to browse a resource, not to run a diagnostic, and they already
//     have the failure's own toast);
//   - a pane that recovered while the plugin ran. The watch loop re-Lists on a
//     backoff and a re-List that succeeds clears the notice, so rows (or an empty
//     cleared pane) mean the failure this explains is over — and a notice written
//     behind them would surface later, out of nowhere, on the next empty result.
func (m Model) handleAuthDiagMsg(msg authDiagMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.watchGen {
		return m, nil
	}
	if m.authDiagCancel != nil {
		m.authDiagCancel()
		m.authDiagCancel = nil
	}
	if msg.err != nil {
		m.logger.Warn("credential plugin diagnosis failed",
			"context", m.context, "kubeconfig", m.kubeconfig, "error", msg.err)
		return m, nil
	}
	if msg.rep.Plugin == nil {
		// Nothing to add: the context authenticates some other way (so the plugin
		// failure client-go reported is not this context's — a stale classification, or
		// a kubeconfig that changed under us), or no stanza could be read.
		m.logger.Warn("credential plugin diagnosis named no plugin", "context", m.context)
		return m, nil
	}
	// The durable record of what the plugin said (D159): the toast is transient, the
	// pane is cleared by the first recovery, and this is what a dogfooding bug report
	// is written from. The token never appears here — kube.Diagnose discards the
	// plugin's stdout and captures only stderr (D211 pt 2).
	m.logger.Info("credential plugin diagnosed",
		"context", msg.rep.Plugin.Context, "command", msg.rep.Diagnosis.CommandLine,
		"exit", msg.rep.Diagnosis.ExitCode, "notFound", msg.rep.Diagnosis.NotFound,
		"timedOut", msg.rep.Diagnosis.TimedOut, "stderr", msg.rep.Diagnosis.Stderr,
		"remediation", msg.rep.Remediation.CommandLine())
	if m.table.TotalRowCount() > 0 || m.table.Notice() == "" {
		return m, nil
	}
	// The offer, if this diagnosis substantiates one and the screen is free to carry
	// it (AUTH-05b). It is decided before the notice is written because the notice
	// says which of the two it is: a prompt the reader is about to answer, or a
	// command they will run themselves.
	offered := m.offerReauth(msg.res, msg.rep)
	notice := authFailure(msg.res.GVK.Kind, msg.fail, msg.rep, offered)
	m.table.SetNotice(notice)
	if offered {
		// What the pane goes back to once the offer is answered, either way. Kept as
		// text rather than as the report it came from: restoring it must not depend on
		// the diagnosis still being reachable, and re-rendering later would re-read a
		// report that by then describes a failure the reader may have left behind.
		m.reauthOffer = reauthOffer{shown: notice, answered: authFailure(msg.res.GVK.Kind, msg.fail, msg.rep, false)}
	}
	return m, nil
}
