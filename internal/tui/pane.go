package tui

import (
	"image"

	ui "github.com/gizak/termui/v3"
)

type Eventable interface {
	OnEvent(event *ui.Event) bool
}

type Focusable interface {
	OnFocusIn()
	OnFocusOut()
}

type Pane interface {
	Eventable
	In(image.Rectangle) bool
	Bounds() image.Rectangle
}
