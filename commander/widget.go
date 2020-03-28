package commander

import (
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Widget interface {
	views.Widget
	IsFocused() bool
	OnFocus()
	OnBlur()
	OnShow()
	OnHide()
	IsVisible() bool
}

type MaxSizeWidget interface {
	Widget
	MaxSize() (int, int)
}

type Style = tcell.Style

type StylableWidget interface {
	SetStyle(style Style)
	Style() Style
}

type Popup interface {
	MaxSizeWidget
	Reposition(view View)
}
