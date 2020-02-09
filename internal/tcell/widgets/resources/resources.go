package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
)

type ResourceItem struct {
	Kind  string
	Title string
}

func BuildResourceMenu(client client.Client, itemMap []ResourceItem, resources client.ResourceMap, shell listTable.ShellFunc, ns listTable.NamespaceAccessor) []menu.Item {
	var items []menu.Item
	for _, item := range itemMap {
		res, ok := resources[item.Kind]
		if !ok {
			continue
		}

		var widget focus.FocusableWidget

		if res.Namespaced {
			widget = listTable.NewResourceListTable(client, res, shell, ns)
		} else {
			widget = listTable.NewClusterResourceListTable(client, res, shell)
		}
		if widget != nil {
			items = append(items, menu.NewItem(item.Title, widget))
		}
	}
	return items
}
