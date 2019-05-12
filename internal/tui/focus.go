package tui

import ui "github.com/gizak/termui/v3"

type Focusable interface {
	OnEvent(event *ui.Event) bool
	OnFocusIn()
	OnFocusOut()
}
