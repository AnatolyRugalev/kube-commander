package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/gdamore/tcell/views"
)

type ScreenLayout struct {
	*views.BoxLayout
	focus *focus.Manager
}

func NewScreenLayout(root focus.FocusableWidget, fill float64) *ScreenLayout {
	box := views.NewBoxLayout(views.Horizontal)
	box.AddWidget(root, fill)
	return &ScreenLayout{
		BoxLayout: box,
		focus:     focus.NewFocusManager(root),
	}
}
