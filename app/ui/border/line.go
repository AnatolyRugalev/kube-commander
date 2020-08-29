package border

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type VerticalLine struct {
	*focus.Focusable
	views.WidgetWatchers

	view  views.View
	style tcell.Style
}

func NewVerticalLine(style tcell.Style) *VerticalLine {
	return &VerticalLine{
		Focusable: focus.NewFocusable(),
		style:     style,
	}
}

func (l VerticalLine) Draw() {
	l.view.Fill(vertical, l.style)
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
