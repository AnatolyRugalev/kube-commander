package tui

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type MenuList struct {
	*widgets.List
	items          []Focusable
	onCursorChange func(focusable Focusable)
	onActivate     func(focusable Focusable)
}

func NewMenuList(items map[string]Focusable) *MenuList {
	ml := &MenuList{
		List: widgets.NewList(),
	}
	ml.Title = "Cluster"
	ml.SelectedRowStyle = ui.NewStyle(ui.ColorYellow)
	ml.WrapText = false
	for row, item := range items {
		ml.Rows = append(ml.Rows, row)
		ml.items = append(ml.items, item)
	}
	ml.SelectedRow = 0
	return ml
}

func (ml *MenuList) OnCursorChange(onCursorChange func(focusable Focusable)) {
	ml.onCursorChange = onCursorChange
}

func (ml *MenuList) OnActivate(onActivate func(focusable Focusable)) {
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
	case "<Right>":
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
		ml.onCursorChange(ml.items[ml.SelectedRow])
	}
}

func (ml *MenuList) activateCurrent() {
	if ml.onActivate != nil {
		ml.onActivate(ml.items[ml.SelectedRow])
	}
}

func (ml *MenuList) OnFocusIn() {
	ml.BorderStyle = ui.NewStyle(ui.ColorYellow)
}

func (ml *MenuList) OnFocusOut() {
	ml.BorderStyle = ui.NewStyle(ui.ColorWhite)
}
