package tui

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type MenuList struct {
	*widgets.List
	screen         *Screen
	items          []menuItemFunc
	selectedItem   Pane
	onCursorChange func(focusable Pane)
	onActivate     func(focusable Pane)
}

type menuItemFunc func() Pane

type menuItem struct {
	name     string
	itemFunc menuItemFunc
}

var items = []menuItem{
	{"Namespaces", func() Pane {
		return NewNamespacesTable()
	}},
	{"Nodes", func() Pane {
		return NewNodesTable()
	}},
}

func NewMenuList(screen *Screen) *MenuList {
	ml := &MenuList{
		List:   widgets.NewList(),
		screen: screen,
	}
	ml.Title = "Cluster"
	ml.SelectedRowStyle = ui.NewStyle(ui.ColorYellow)
	ml.WrapText = false
	for _, item := range items {
		ml.Rows = append(ml.Rows, item.name)
		ml.items = append(ml.items, item.itemFunc)
	}
	ml.SelectedRow = 0
	return ml
}

func (ml *MenuList) OnCursorChange(onCursorChange func(focusable Pane)) {
	ml.onCursorChange = onCursorChange
}

func (ml *MenuList) OnActivate(onActivate func(focusable Pane)) {
	ml.onActivate = onActivate
}

func (ml *MenuList) OnEvent(event *ui.Event) bool {
	switch event.ID {
	case "<Down>":
		if ml.SelectedRow >= len(ml.Rows)-1 {
			return false
		}
		ml.CursorDown()
		return true
	case "<Up>":
		if ml.SelectedRow <= 0 {
			return false
		}
		ml.CursorUp()
		return true
	case "<Right>", "<Enter>":
		ml.activateCurrent()
		return true
	}
	return false
}

func (ml *MenuList) CursorDown() {
	ml.SelectedRow += 1
	ml.onCursor()
}

func (ml *MenuList) CursorUp() {
	ml.SelectedRow -= 1
	ml.onCursor()
}

func (ml *MenuList) onCursor() {
	if ml.onCursorChange != nil {
		ml.selectedItem = ml.items[ml.SelectedRow]()
		ml.onCursorChange(ml.selectedItem)
	}
}

func (ml *MenuList) activateCurrent() {
	if ml.onActivate != nil && ml.selectedItem != nil {
		ml.onActivate(ml.selectedItem)
	}
}

func (ml *MenuList) OnFocusIn() {
	ml.BorderStyle = ui.NewStyle(ui.ColorYellow)
}

func (ml *MenuList) OnFocusOut() {
	ml.BorderStyle = ui.NewStyle(ui.ColorWhite)
}
