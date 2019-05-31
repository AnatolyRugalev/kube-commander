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
	lt := widgets.NewListTable(screen, &MenuList{})
	lt.Title = "Cluster"
	_ = lt.Reload()
	return lt
}

func (ml *MenuList) LoadData() ([][]string, error) {
	var items [][]string
	for _, item := range menuItems {
		items = append(items, []string{item.name})
	}
	return items, nil
}

func (ml *MenuList) OnCursorChange(item []string) bool {
	var menuItem *menuItem
	for _, i := range menuItems {
		if i.name == item[0] {
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

func (ml *MenuList) OnSelect(item []string) bool {
	if ml.selectedItem != nil {
		screen.Focus(ml.selectedItem)
		return true
	}
	return false
}
