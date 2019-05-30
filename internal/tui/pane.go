package tui

import (
	ui "github.com/gizak/termui/v3"
	"image"
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
	In(s image.Rectangle) bool
	Bounds() image.Rectangle
}

type ListExtension interface {
	getTitleRow() []string
	loadData() ([][]string, error)
}

type ListExtensionEventable interface {
	ListExtension
	OnEvent(event *ui.Event, item []string) bool
}

type ListExtensionSelectable interface {
	ListExtension
	OnSelect(item []string) bool
}

type ListExtensionDeletable interface {
	ListExtension
	OnDelete(item []string) error
	DeleteDialogText(item []string) string
}
