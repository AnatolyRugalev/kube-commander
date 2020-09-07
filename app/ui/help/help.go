package help

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell/views"
)

type widget struct {
	*views.Text
	*focus.Focusable
	theme commander.ThemeManager
}

func (w widget) MaxSize() (int, int) {
	return w.Text.Size()
}

func (w widget) Size() (int, int) {
	width, _ := w.Text.Size()
	return width, 1
}

var text = `kubecom - browse your Kubernetes cluster in a casual way!

Global:
 D: Describe selected resource              ?: Shows help dialog
 E: Edit selected resource                  Q: Quit
 C: Copy resource name to the clipboard     Ctrl+N or F2: Switch namespace
 Del: Delete resource (with confirmation)   F10, F11: Cycle through themes

Navigation:
 ↑↓→←: List navigation            /: Filter resources
 Enter: Select menu item          Esc, Backspace: Go back

Resource types:
 +(plus): Add resource type to the menu     F6, F7: Move item up/down
 Del: Remove resource type from menu

Pods:
 L: Show logs                     Shift+L: Show previous logs
 F: Forward port
 S: Shell into selected pod
`

func NewHelpWidget(theme commander.ThemeManager) *widget {
	widget := widget{
		Text:      views.NewText(),
		Focusable: focus.NewFocusable(),
		theme:     theme,
	}
	widget.Text.SetText(text)
	return &widget
}

func (w *widget) Draw() {
	w.Text.SetStyle(w.theme.GetStyle("screen"))
	w.Text.Draw()
}

func ShowHelpPopup(workspace commander.Workspace) {
	help := NewHelpWidget(workspace.Theme())
	workspace.ShowPopup("Help", help)
}
