package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
)

type menuItemFunc func(namespace string) Pane

const (
	itemTypeCluster   = 0
	itemTypeSelector  = 1
	itemTypeNamespace = 2
)

type menuItem struct {
	name     string
	itemType int
	itemFunc menuItemFunc
}

var menuItems = []*menuItem{
	{"Namespaces", itemTypeCluster, func(namespace string) Pane {
		return NewNamespacesTable()
	}},
	{"Nodes", itemTypeCluster, func(namespace string) Pane {
		return NewNodesTable()
	}},
	{"Storage Classes", itemTypeCluster, func(namespace string) Pane {
		return NewStorageClassesTable()
	}},
	{"PVs", itemTypeCluster, func(namespace string) Pane {
		return NewPVsTable()
	}},
	{"<Namespace>", itemTypeSelector, nil},
	{"Deployments", itemTypeNamespace, func(namespace string) Pane {
		return NewDeploymentsTable(namespace)
	}},
	{"Stateful Sets", itemTypeNamespace, func(namespace string) Pane {
		return NewStatefulSetsTable(namespace)
	}},
	{"Daemon Sets", itemTypeNamespace, func(namespace string) Pane {
		return NewDaemonSetsTable(namespace)
	}},
	{"Pods", itemTypeNamespace, func(namespace string) Pane {
		return NewPodsTable(namespace)
	}},
	{"Cron Jobs", itemTypeNamespace, func(namespace string) Pane {
		return NewCronJobsTable(namespace)
	}},
	{"Jobs", itemTypeNamespace, func(namespace string) Pane {
		return NewJobsTable(namespace)
	}},
	{"PVCs", itemTypeNamespace, func(namespace string) Pane {
		return NewPVCsTable(namespace)
	}},
	{"Config Maps", itemTypeNamespace, func(namespace string) Pane {
		return NewConfigMapsTable(namespace)
	}},
	{"Secrets", itemTypeNamespace, func(namespace string) Pane {
		return NewSecretsTable(namespace)
	}},
	{"Services", itemTypeNamespace, func(namespace string) Pane {
		return NewServicesTable(namespace)
	}},
	{"Ingresses", itemTypeNamespace, func(namespace string) Pane {
		return NewIngressesTable(namespace)
	}},
}

type MenuList struct {
	*widgets.ListTable
	items        []*menuItem
	selectedPane Pane
	namespace    string
}

func NewMenuList() *MenuList {
	ml := &MenuList{}
	lt := widgets.NewListTable([]widgets.ListRow{}, ml, nil)
	lt.Title = "Cluster"
	lt.IsContext = true
	ml.ListTable = lt
	return ml
}

func (ml *MenuList) updateMenu(namespace string) {
	var rows []widgets.ListRow
	ml.items = []*menuItem{}
	for i, item := range menuItems {
		var row widgets.ListRow
		switch item.itemType {
		case itemTypeCluster:
			row = widgets.ListRow{item.name}
		case itemTypeSelector:
			row = widgets.ListRow{
				"[" + string(termui.DOWN_ARROW) + " " + namespace + "](mod:bold)",
			}
		case itemTypeNamespace:
			last := i == len(menuItems)-1
			var text string
			if last {
				text = string(termui.BOTTOM_LEFT) + " " + item.name
			} else {
				text = string(termui.VERTICAL_RIGHT) + " " + item.name
			}
			row = widgets.ListRow{text}
		default:
			panic("Unknown menu item type")
		}
		rows = append(rows, row)
		ml.items = append(ml.items, item)
	}
	ml.namespace = namespace
	ml.SetRows(rows)
	if screen.focus == ml {
		ml.OnCursorChange(ml.SelectedRowIdx(), ml.SelectedRow())
	}
}

func (ml *MenuList) OnCursorChange(idx int, row widgets.ListRow) bool {
	menuItem := ml.items[idx]
	if menuItem == nil {
		return false
	}
	switch menuItem.itemType {
	case itemTypeCluster, itemTypeNamespace:
		ml.selectedPane = menuItem.itemFunc(ml.namespace)
		screen.ReplaceRightPane(ml.selectedPane)
	}
	return true
}

func (ml *MenuList) OnSelect(idx int, row widgets.ListRow) bool {
	menuItem := ml.items[idx]
	if menuItem == nil {
		return false
	}
	switch menuItem.itemType {
	case itemTypeCluster, itemTypeNamespace:
		if ml.selectedPane != nil {
			screen.Focus(ml.selectedPane)
			return true
		}
	case itemTypeSelector:
		screen.ShowNamespaceSelection()
		return true
	}
	return false
}
