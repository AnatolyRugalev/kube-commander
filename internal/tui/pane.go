package tui

import ui "github.com/gizak/termui/v3"

type Eventable interface {
	OnEvent(event *ui.Event) bool
}

type Focusable interface {
	OnFocusIn()
	OnFocusOut()
}

type Pane interface {
	Eventable
}
