package border

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type VerticalLine struct {
	*focus.Focusable
	views.WidgetWatchers

	view  views.View
	theme commander.ThemeManager
}

func NewVerticalLine(theme commander.ThemeManager) *VerticalLine {
	return &VerticalLine{
		Focusable: focus.NewFocusable(),
		theme:     theme,
	}
}

func (l VerticalLine) Draw() {
	l.view.Fill(vertical, l.theme.GetStyle("screen"))
}

func (l VerticalLine) Resize() {

}

func (l VerticalLine) HandleEvent(_ tcell.Event) bool {
	return false
}

func (l *VerticalLine) SetView(view views.View) {
	l.view = view
}

func (l VerticalLine) Size() (int, int) {
	if l.view == nil {
		return 1, 1
	}
	return 1, 1
}
