package err

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
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
	var txt string
	if execErr, ok := err.(*commander.ExecErr); ok {
		txt = text.Wrap(err.Error()+"\n"+string(execErr.Output), 50)
	} else {
		txt = text.Wrap(err.Error(), 50)
	}
	widget.Text.SetText(txt)
	widget.Text.SetStyle(theme.Default)
	return &widget
}
