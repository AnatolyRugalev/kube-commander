package logo

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Logo struct {
	views.WidgetWatchers
	text  *views.Text
	theme commander.ThemeManager
	view  views.View
}

func (l *Logo) Draw() {
	l.text.SetText("☸  ️kubecom")
	l.text.SetStyle(l.theme.GetStyle("logo-text"))
	iconStyle := l.theme.GetStyle("logo-icon")
	l.text.SetStyleAt(0, iconStyle)
	l.text.SetStyleAt(1, iconStyle)
	l.text.Draw()
}

func (l *Logo) Resize() {
}

func (l *Logo) HandleEvent(_ tcell.Event) bool {
	return false
}

func (l *Logo) SetView(view views.View) {
	l.view = view
	l.text.SetView(view)
}

func (l *Logo) Size() (int, int) {
	return 12, 1
}

func NewLogo(theme commander.ThemeManager) *Logo {
	return &Logo{
		theme: theme,
		text:  views.NewText(),
	}
}
