package tui

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type MenuList struct {
	*widgets.List
	items        []menuItemFunc
	selectedItem Pane
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

func NewMenuList() *MenuList {
	ml := &MenuList{
		List: widgets.NewList(),
	}
	ml.Title = "Cluster"
	ml.TitleStyle = theme["title"].inactive
	ml.BorderStyle = theme["grid"].inactive
	ml.TextStyle = theme["listItem"].inactive
	ml.SelectedRowStyle = theme["listItemSelected"].inactive
	ml.WrapText = false
	for _, item := range items {
		ml.Rows = append(ml.Rows, item.name)
		ml.items = append(ml.items, item.itemFunc)
	}
	ml.SelectedRow = 0
	return ml
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
	case "<MouseLeft>":
		m := event.Payload.(ui.Mouse)
		if ml.foundAndClick(0, m.Y-1) {
			return true
		}
		return false
	}
	return false
}

func (ml *MenuList) CursorDown() {
	ml.SelectedRow += 1
	ml.onCursorMove()
}

func (ml *MenuList) CursorUp() {
	ml.SelectedRow -= 1
	ml.onCursorMove()
}

func (ml *MenuList) onCursorMove() {
	ml.selectedItem = ml.items[ml.SelectedRow]()
	screen.ReplaceRightPane(ml.selectedItem)
}

func (ml *MenuList) activateCurrent() {
	if ml.selectedItem != nil {
		screen.Focus(ml.selectedItem)
	}
}

func (ml *MenuList) foundAndClick(xCoord, yCoord int) bool {
	if yCoord > len(ml.Rows)-1 {
		yCoord = len(ml.Rows) - 1
	}
	if yCoord < 0 {
		yCoord = 0
	}
	ml.SelectedRow = yCoord
	ml.onCursorMove()
	ml.activateCurrent()
	return true
}

func (ml *MenuList) OnFocusIn() {
	ml.TitleStyle = theme["title"].active
	ml.BorderStyle = theme["grid"].active
	ml.TextStyle = theme["listItem"].active
	ml.SelectedRowStyle = theme["listItemSelected"].active
}

func (ml *MenuList) OnFocusOut() {
	ml.TitleStyle = theme["title"].inactive
	ml.BorderStyle = theme["grid"].inactive
	ml.TextStyle = theme["listItem"].inactive
	ml.SelectedRowStyle = theme["listItemSelected"].inactive
}
