package tui

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
)

// This file is the M3-15b edit wire: the TUI surface that opens the selected
// object's YAML in the user's $EDITOR and applies the edit on save. It is the
// second (and last) sanctioned TUI-suspending action after exec (D2/D125) — every
// other operation stays in-process. It is also the **first slice of the
// unify-yaml-view-and-edit feedback** (D69/D135): view and edit are the same act on
// an object's YAML, so edit is the object-YAML action, and the standalone read-only
// YAML viewer (M3-03) is retired in favour of it in the follow-up M3-15c.
//
// The flow mirrors exec's suspend rhythm but fetches first so a fetch failure never
// blanks the terminal: openEdit fetches the object's YAML off the update loop (reusing
// the existing YAMLGetter, M1-07a) → editFetchedMsg; handleEditFetched then suspends
// via tea.Exec into $EDITOR over a temp file (editCommand, off the loop), reads the
// buffer back, and — only if it changed — applies it through the Editor seam
// (kube.Update, M3-15a/D129). No-change (edited bytes == original) and any editor /
// apply error degrade to a status-bar toast without mutating (principle 3); kube.Update
// validates the buffer and carries the resourceVersion for optimistic concurrency, so a
// botched or stale edit is refused, not clobbered.
//
// The real interactive $EDITOR suspend (a full-screen editor owning the terminal,
// TUI restore afterwards) is a real-terminal/real-cluster check the sandbox cannot
// perform, so — like exec (D125) — a dogfood human-task gates the M3 "Edit
// round-trips through $EDITOR" exit criterion; the routing, temp-file round-trip,
// no-change detection, and apply are all covered hermetically.

// Editor is the narrow slice of the kube layer the shell needs to apply an edited
// object (M3-15b): PUT the edited YAML bytes back (M3-15a's Clients.Update). *kube.Clients
// satisfies it. As with the other action seams the shell depends on this interface, not
// the concrete client, so the tui package never constructs a client and the edit flow is
// driveable in hermetic tests with a fake editor. A model built without one (the default)
// is edit-inert: the Edit action is a no-op, exactly as an unwired action's entry is.
type Editor interface {
	Update(ctx context.Context, r kube.Resource, ref kube.ObjectRef, edited []byte) error
}

// WithEditor wires the kube client the shell uses to apply an edited object (M3-15b).
// Without it — or without a YAMLGetter to fetch the buffer — the Edit action is inert.
func WithEditor(e Editor) Option {
	return func(m *Model) { m.editor = e }
}

// defaultEditor is the editor launched when neither KUBE_EDITOR nor EDITOR is set. vi is
// the lowest-common-denominator editor present on virtually every Unix system, matching
// kubectl's own fallback.
var defaultEditor = "vi"

// lookupEditor resolves the editor argv to launch, preferring KUBE_EDITOR over EDITOR
// (kubectl's precedence) and falling back to vi. It is a package var so tests can drive
// the flow without depending on the runner's environment. The value is split on spaces so
// an editor with flags (e.g. `code -w`, `subl -w`) works.
var lookupEditor = func() []string {
	return resolveEditorArgv(os.Getenv("KUBE_EDITOR"), os.Getenv("EDITOR"))
}

// resolveEditorArgv picks the editor argv from KUBE_EDITOR then EDITOR then the vi
// fallback, splitting the chosen value into command + flags. Pure so the precedence and
// flag-splitting are unit-testable without touching the environment.
func resolveEditorArgv(kubeEditor, editor string) []string {
	for _, e := range []string{kubeEditor, editor} {
		if fields := strings.Fields(e); len(fields) > 0 {
			return fields
		}
	}
	return []string{defaultEditor}
}

// runEditor launches the resolved editor over file with the suspended terminal's
// streams and blocks until it exits. It is a package var so tests can simulate an edit
// (or an editor failure) without spawning a real process; the default builds the
// *exec.Cmd and runs it. A non-zero editor exit surfaces as an error — the caller then
// degrades to a toast without applying (an aborted edit never mutates).
var runEditor = func(argv []string, file string, stdin io.Reader, stdout, stderr io.Writer) error {
	proc := exec.Command(argv[0], append(append([]string{}, argv[1:]...), file)...) //nolint:gosec // argv is the resolved $EDITOR; file is our own temp path, not shell.
	proc.Stdin, proc.Stdout, proc.Stderr = stdin, stdout, stderr
	return proc.Run()
}

// editFetchedMsg carries the fetched YAML (or a fetch error) for the object the user
// asked to edit (M3-15b). The res/ref are the object the buffer came from, threaded
// through so the apply targets exactly what was fetched even if the selection moved
// while the fetch was in flight.
type editFetchedMsg struct {
	res     kube.Resource
	ref     kube.ObjectRef
	content string
	err     error
}

// editDoneMsg carries the outcome of an edit once tea.Exec resumes the program
// (M3-15b). label is the human target ("Pod default/web-1") for the status-bar result.
// changed reports whether the buffer differed from what was fetched (an unchanged buffer
// is a neutral no-op, never an apply). err is nil on a clean apply (or a clean no-op);
// non-nil on an editor failure or an apply rejection (parse error, identity guard,
// Conflict) — either way it degrades to a transient toast, never a panic (principle 3).
type editDoneMsg struct {
	label   string
	changed bool
	err     error
}

// openEdit starts the Edit flow over the selected object (M3-15b). With no editor or
// YAMLGetter wired, or an empty ref (a row with no name, guarded so an empty ref never
// reaches the kube layer), it is a no-op. Otherwise it fetches the object's YAML off the
// update loop (reusing the YAMLGetter) and hands it to handleEditFetched to suspend into
// $EDITOR — fetching first so a fetch failure degrades to a toast without ever blanking
// the terminal for an editor that would open empty.
func (m Model) openEdit(msg rowActionMsg) (tea.Model, tea.Cmd) {
	if m.editor == nil || m.yamlGetter == nil || msg.Object.Name == "" {
		return m, nil
	}
	getter := m.yamlGetter
	r, ref := msg.Resource, msg.Object
	return m, func() tea.Msg {
		content, err := getter.GetYAML(context.Background(), r, ref)
		return editFetchedMsg{res: r, ref: ref, content: content, err: err}
	}
}

// handleEditFetched suspends into $EDITOR over the fetched YAML (M3-15b). A fetch error
// degrades to a status-bar toast without suspending. Otherwise it returns the tea.Exec
// command bubbletea runs from a released terminal; the callback reports the session
// result (handleEditDone). changed is read from the editCommand the callback closes over —
// Run records whether the buffer was modified and lets any apply error ride out as the
// callback's err.
func (m Model) handleEditFetched(msg editFetchedMsg) (tea.Model, tea.Cmd) {
	label := viewerTitle(msg.res, msg.ref)
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("edit "+label, msg.err))
	}
	cmd := &editCommand{
		editor:     m.editor,
		res:        msg.res,
		ref:        msg.ref,
		content:    msg.content,
		editorArgv: lookupEditor(),
	}
	callback := func(err error) tea.Msg {
		return editDoneMsg{label: label, changed: cmd.changed, err: err}
	}
	return m, tea.Exec(cmd, callback)
}

// handleEditDone reports a finished edit: an editor/apply failure degrades to a transient
// error toast (D74); an unchanged buffer to a neutral "no changes" notice (nothing was
// applied); a clean apply to a neutral "applied" notice. The browse UI is already restored
// by the time this lands — tea.Exec re-captured the terminal before delivering the msg.
func (m Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.surfaceError(NewErrorMsg("edit "+msg.label, msg.err))
	}
	if !msg.changed {
		return m, m.surfaceNotice("edit: no changes · " + msg.label)
	}
	return m, m.surfaceNotice("applied · " + msg.label)
}

// editCommand adapts the edit flow to bubbletea's ExecCommand (tea.Exec), so $EDITOR runs
// in the suspended terminal off the update loop (mirroring execCommand, D124). bubbletea
// releases the terminal, calls the Set* setters with the program's streams, runs Run() to
// completion, then re-captures the terminal. Run writes the fetched YAML to a temp file,
// runs the editor against it (the editor owns the cooked terminal bubbletea released),
// reads the buffer back, and — only if it changed — applies it via the Editor seam. The
// outcome (changed, and any apply error) rides back to the callback: changed via this
// field, the error as Run's return.
type editCommand struct {
	editor     Editor
	res        kube.Resource
	ref        kube.ObjectRef
	content    string   // the fetched YAML seeded into the editor buffer
	editorArgv []string // the resolved $EDITOR command + flags

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	changed bool // set in Run once the buffer is seen to differ from content
}

// SetStdin/SetStdout/SetStderr capture the terminal streams bubbletea hands the editor.
// Unlike exec (a TTY folds stderr into stdout), a plain editor process wants all three, so
// none is a no-op.
func (c *editCommand) SetStdin(r io.Reader)  { c.stdin = r }
func (c *editCommand) SetStdout(w io.Writer) { c.stdout = w }
func (c *editCommand) SetStderr(w io.Writer) { c.stderr = w }

// Run edits the object to completion in the suspended terminal. It writes the fetched
// YAML to a temp file, launches $EDITOR against it, reads the buffer back, and applies it
// through the Editor seam only when it changed. A temp-file / editor failure returns
// early without applying; an unchanged buffer is a clean no-op (changed stays false); a
// changed buffer is applied and any apply error (parse, identity guard, Conflict — all
// enforced by kube.Update, M3-15a) is returned so the caller degrades to a toast without
// a partial mutation.
func (c *editCommand) Run() error {
	f, err := os.CreateTemp("", "kubecom-*.yaml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.WriteString(c.content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := runEditor(c.editorArgv, tmp, c.stdin, c.stdout, c.stderr); err != nil {
		return err
	}

	edited, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if bytes.Equal(edited, []byte(c.content)) {
		return nil // unchanged → a clean no-op, nothing to apply.
	}
	c.changed = true
	return c.editor.Update(context.Background(), c.res, c.ref, edited)
}
