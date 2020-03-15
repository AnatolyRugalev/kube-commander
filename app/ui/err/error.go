package err

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/gdamore/tcell/views"
	"github.com/kr/text"
)

type widget struct {
	*views.Text
	*focus.Focusable
}

func (w widget) MaxSize() (int, int) {
	return w.Text.Size()
}

func NewErrorWidget(err error) *widget {
	widget := widget{
		Text:      views.NewText(),
		Focusable: focus.NewFocusable(),
	}
	widget.Text.SetText(text.Wrap(err.Error(), 50))
	widget.Text.SetStyle(theme.Default)
	return &widget
}
