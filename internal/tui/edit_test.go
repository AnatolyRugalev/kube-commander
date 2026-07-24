package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// fakeEditor is a hermetic Editor (M3-15b): it records the edited bytes it was handed
// (so a test can assert the round-tripped buffer reached the apply) and returns a preset
// error. It never dials — the real Update is M3-15a's fake-dynamic-client territory, so
// the edit flow's routing, temp-file round-trip, no-change detection, and apply are all
// covered here without a cluster.
type fakeEditor struct {
	err       error
	calls     int
	gotRef    kube.ObjectRef
	gotEdited []byte
}

func (f *fakeEditor) Update(_ context.Context, _ kube.Resource, ref kube.ObjectRef, edited []byte) error {
	f.calls++
	f.gotRef = ref
	f.gotEdited = edited
	return f.err
}

// podEditModel drills into a pods table so an edit test has a concrete selected Pod row,
// with the given options wired (an editor + a YAML getter for a live edit).
func podEditModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	fw := &fakeWatcher{preload: []kube.WatchEvent{sortReset()}}
	m := sizedWith(t, append([]Option{WithWatcher(fw)}, opts...)...)
	next, cmd := m.Update(menu.ResourceSelectedMsg{Resource: kindResource("pods", "Pod")})
	m = next.(Model)
	next, _ = m.Update(cmd().(watchMsg)) // drain the RESET so the table has rows
	return next.(Model)
}

// withEditorSim swaps runEditor for a simulator that overwrites the temp file with edited
// and returns simErr, restoring the real launcher when the test ends. It lets the flow run
// with no real $EDITOR process. edited == the file's original content simulates a save with
// no changes; a distinct value simulates an edit; simErr simulates an aborted/failed editor.
func withEditorSim(t *testing.T, edited string, simErr error) {
	t.Helper()
	prev := runEditor
	runEditor = func(_ []string, file string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		if simErr != nil {
			return simErr
		}
		return os.WriteFile(file, []byte(edited), 0o600)
	}
	t.Cleanup(func() { runEditor = prev })
}

// TestResolveEditorArgv covers the editor precedence and flag-splitting: KUBE_EDITOR wins
// over EDITOR, EDITOR is the fallback, the vi default applies when both are empty, and a
// value with flags splits into command + args.
func TestResolveEditorArgv(t *testing.T) {
	cases := []struct {
		name       string
		kubeEditor string
		editor     string
		want       []string
	}{
		{"kube-editor wins", "code -w", "vim", []string{"code", "-w"}},
		{"editor fallback", "", "vim", []string{"vim"}},
		{"vi default", "", "", []string{"vi"}},
		{"flags split", "", "subl -n -w", []string{"subl", "-n", "-w"}},
		{"blank editor ignored", "", "   ", []string{"vi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveEditorArgv(tc.kubeEditor, tc.editor); !slices.Equal(got, tc.want) {
				t.Fatalf("resolveEditorArgv(%q,%q) = %v, want %v", tc.kubeEditor, tc.editor, got, tc.want)
			}
		})
	}
}

// TestOpenEditInertWithoutEditor proves the action is a no-op with no editor wired (the
// pre-wiring default): no command, no modal — exactly as an unwired action is absent.
func TestOpenEditInertWithoutEditor(t *testing.T) {
	m := podEditModel(t, WithYAMLGetter(&fakeYAMLGetter{yaml: "kind: Pod\n"}))
	_, cmd := dispatchRowAction(t, m, rowActionEdit)
	if cmd != nil {
		t.Fatal("with no editor wired the edit action must be inert (no command)")
	}
}

// TestOpenEditInertWithoutGetter proves edit needs a YAMLGetter to seed the buffer: with
// an editor but no getter it is inert (there is nothing to open in $EDITOR).
func TestOpenEditInertWithoutGetter(t *testing.T) {
	m := podEditModel(t, WithEditor(&fakeEditor{}))
	_, cmd := dispatchRowAction(t, m, rowActionEdit)
	if cmd != nil {
		t.Fatal("with no YAML getter wired the edit action must be inert (no command)")
	}
}

// TestOpenEditInertOnEmptyRef proves a row with no name never reaches the kube layer — an
// empty ref is guarded so edit is a no-op rather than fetching/applying an empty object.
func TestOpenEditInertOnEmptyRef(t *testing.T) {
	m := podEditModel(t, WithEditor(&fakeEditor{}), WithYAMLGetter(&fakeYAMLGetter{yaml: "kind: Pod\n"}))
	_, cmd := m.openEdit(rowActionMsg{Action: rowActionEdit, Resource: m.current, Object: kube.ObjectRef{}})
	if cmd != nil {
		t.Fatal("an empty ref should make edit inert (no command)")
	}
}

// TestOpenEditFetchesThenSuspends proves the edit intent fetches the object's YAML then
// suspends into a session: the fetch command yields an editFetchedMsg carrying the fetched
// content and the target, and handling it issues the tea.Exec suspend (a non-nil command)
// without opening any modal (edit is a direct suspend, not a mutating confirm).
func TestOpenEditFetchesThenSuspends(t *testing.T) {
	g := &fakeYAMLGetter{yaml: "kind: Pod\nmetadata:\n  name: web-1\n"}
	m := podEditModel(t, WithEditor(&fakeEditor{}), WithYAMLGetter(g))

	m, cmd := dispatchRowAction(t, m, rowActionEdit)
	if cmd == nil {
		t.Fatal("the edit intent should issue the async GetYAML fetch command")
	}
	fetched, ok := cmd().(editFetchedMsg)
	if !ok {
		t.Fatalf("edit fetch produced %T, want editFetchedMsg", cmd())
	}
	if fetched.content != g.yaml {
		t.Fatalf("fetched content = %q, want %q", fetched.content, g.yaml)
	}
	if g.calls != 1 {
		t.Fatalf("GetYAML called %d times, want 1", g.calls)
	}

	next, cmd := m.Update(fetched)
	m = next.(Model)
	if m.modal.Active() {
		t.Fatal("edit is a suspend action — it must not open a confirm modal")
	}
	if cmd == nil {
		t.Fatal("handling the fetched YAML should issue the tea.Exec suspend command")
	}
}

// TestEditFetchErrorDegrades proves a GetYAML failure degrades to a status-bar toast and
// never suspends: no editor is launched, nothing is applied.
func TestEditFetchErrorDegrades(t *testing.T) {
	e := &fakeEditor{}
	m := podEditModel(t, WithEditor(e), WithYAMLGetter(&fakeYAMLGetter{err: errors.New("not found")}))
	_, cmd := m.handleEditFetched(editFetchedMsg{res: m.current, ref: kube.ObjectRef{Name: "web-1"}, err: errors.New("not found")})
	if cmd == nil {
		t.Fatal("a fetch error should surface a status-bar toast command")
	}
	if e.calls != 0 {
		t.Fatal("a fetch error must never apply anything")
	}
}

// TestEditCommandAppliesOnChange drives the ExecCommand adapter's Run with a simulated
// editor that changes the buffer: it must apply the edited bytes through the Editor once,
// report changed, and return no error.
func TestEditCommandAppliesOnChange(t *testing.T) {
	withEditorSim(t, "kind: Pod\nedited: true\n", nil)
	e := &fakeEditor{}
	c := &editCommand{
		editor:     e,
		ref:        kube.ObjectRef{Namespace: "default", Name: "web-1"},
		content:    "kind: Pod\n",
		editorArgv: []string{"vi"},
	}
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !c.changed {
		t.Fatal("a modified buffer should report changed")
	}
	if e.calls != 1 {
		t.Fatalf("Update called %d times, want 1", e.calls)
	}
	if string(e.gotEdited) != "kind: Pod\nedited: true\n" {
		t.Fatalf("applied bytes = %q, want the edited buffer", e.gotEdited)
	}
	if e.gotRef.Name != "web-1" {
		t.Fatalf("Update addressed %q, want web-1", e.gotRef.Name)
	}
}

// TestEditCommandNoChangeSkipsUpdate proves an unchanged buffer is a clean no-op: the
// Editor is never called and Run reports success with changed=false.
func TestEditCommandNoChangeSkipsUpdate(t *testing.T) {
	withEditorSim(t, "kind: Pod\n", nil) // simulator writes back the same content
	e := &fakeEditor{}
	c := &editCommand{editor: e, ref: kube.ObjectRef{Name: "web-1"}, content: "kind: Pod\n", editorArgv: []string{"vi"}}
	if err := c.Run(); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if c.changed {
		t.Fatal("an unchanged buffer must not report changed")
	}
	if e.calls != 0 {
		t.Fatal("an unchanged buffer must not apply anything")
	}
}

// TestEditCommandEditorErrorNoApply proves an editor failure (a non-zero exit / abort)
// returns the error and never applies.
func TestEditCommandEditorErrorNoApply(t *testing.T) {
	withEditorSim(t, "", errors.New("editor aborted"))
	e := &fakeEditor{}
	c := &editCommand{editor: e, ref: kube.ObjectRef{Name: "web-1"}, content: "kind: Pod\n", editorArgv: []string{"vi"}}
	if err := c.Run(); err == nil {
		t.Fatal("an editor failure should return an error")
	}
	if c.changed || e.calls != 0 {
		t.Fatal("an editor failure must never apply anything")
	}
}

// TestEditCommandApplyErrorSurfaces proves an apply rejection (parse error, identity
// guard, Conflict from kube.Update) rides out as Run's error with changed=true, so the
// caller degrades to a toast without a partial mutation.
func TestEditCommandApplyErrorSurfaces(t *testing.T) {
	withEditorSim(t, "kind: Pod\nedited: true\n", nil)
	e := &fakeEditor{err: errors.New("Conflict")}
	c := &editCommand{editor: e, ref: kube.ObjectRef{Name: "web-1"}, content: "kind: Pod\n", editorArgv: []string{"vi"}}
	if err := c.Run(); err == nil {
		t.Fatal("an apply rejection should return an error")
	}
	if !c.changed {
		t.Fatal("the buffer did change — changed should be true even when the apply fails")
	}
	if e.calls != 1 {
		t.Fatalf("Update called %d times, want 1", e.calls)
	}
}

// TestHandleEditDoneReports covers the three result branches: an error degrades to a toast,
// an unchanged buffer to a neutral notice, a clean apply to a neutral notice — all
// returning a (transient-clear) command and never panicking.
func TestHandleEditDoneReports(t *testing.T) {
	m := podEditModel(t, WithEditor(&fakeEditor{}), WithYAMLGetter(&fakeYAMLGetter{}))
	for _, tc := range []struct {
		name string
		msg  editDoneMsg
	}{
		{"error", editDoneMsg{label: "Pod default/web-1", err: errors.New("boom")}},
		{"no change", editDoneMsg{label: "Pod default/web-1", changed: false}},
		{"applied", editDoneMsg{label: "Pod default/web-1", changed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cmd := m.handleEditDone(tc.msg)
			if cmd == nil {
				t.Fatal("handleEditDone should surface a status message (clear command)")
			}
		})
	}
}
