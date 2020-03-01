package commander

import (
	"github.com/gdamore/tcell/views"
)

type Widget interface {
	views.Widget
	IsFocused() bool
	OnFocus()
	OnBlur()
}

type MaxSizeWidget interface {
	Widget
	MaxSize() (int, int)
}
