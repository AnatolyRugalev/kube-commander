package tui

import ui "github.com/gizak/termui/v3"

type Pane interface {
	OnEvent(event *ui.Event) bool
	OnFocusIn()
	OnFocusOut()
}

type ListExtension interface {
	getTitleRow() []string
	loadData() ([][]string, error)
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
