package tui

import ui "github.com/gizak/termui/v3"

type Pane interface {
	OnEvent(event *ui.Event) bool
	OnFocusIn()
	OnFocusOut()
}
