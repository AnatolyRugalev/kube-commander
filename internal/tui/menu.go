package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
)

type MenuList struct {
	items        []menuItemFunc
	selectedItem Pane
}

type menuItemFunc func() Pane

type menuItem struct {
	name     string
	itemFunc menuItemFunc
}

var menuItems = []*menuItem{
	{"Namespaces", func() Pane {
		return NewNamespacesTable()
	}},
	{"Nodes", func() Pane {
		return NewNodesTable()
	}},
	{"Storage Classes", func() Pane {
		return NewStorageClassesTable()
	}},
	{"PVs", func() Pane {
		return NewPVsTable()
	}},
}

func NewMenuList() *widgets.ListTable {
	var rows []widgets.ListRow
	for _, item := range menuItems {
		rows = append(rows, widgets.ListRow{item.name})
	}
	lt := widgets.NewListTable(rows, &MenuList{}, nil)
	lt.Title = "Cluster"
	lt.IsContext = true
	return lt
}

func (ml *MenuList) OnCursorChange(row widgets.ListRow) bool {
	var menuItem *menuItem
	for _, i := range menuItems {
		if i.name == row[0] {
			menuItem = i
			break
		}
	}
	if menuItem == nil {
		return false
	}
	ml.selectedItem = menuItem.itemFunc()
	screen.ReplaceRightPane(ml.selectedItem)
	return true
}

func (ml *MenuList) OnSelect(row widgets.ListRow) bool {
	if ml.selectedItem != nil {
		screen.Focus(ml.selectedItem)
		return true
	}
	return false
}
