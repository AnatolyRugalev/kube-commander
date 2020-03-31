package resourceMenu

import (
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/pod"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/menu"
	"github.com/AnatolyRugalev/kube-commander/commander"
)

type item struct {
	title string
	kind  string
}

var (
	StandardWidget WidgetConstructor = func(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat) commander.Widget {
		return listTable.NewResourceListTable(workspace, resource, format)
	}
	CustomWidgets = map[string]WidgetConstructor{
		"Pod": func(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat) commander.Widget {
			return pod.NewPodsList(workspace, resource, format)
		},
	}
	itemMap = []*item{
		{title: "Namespaces", kind: "Namespace"},
		{title: "Nodes", kind: "Node"},
		{title: "Storage Classes", kind: "StorageClass"},
		{title: "PVs", kind: "PersistentVolume"},
		{title: "Deployments", kind: "Deployment"},
		{title: "Stateful", kind: "StatefulSet"},
		{title: "Daemons", kind: "DaemonSet"},
		{title: "Replicas", kind: "ReplicaSet"},
		{title: "Pods", kind: "Pod"},
		{title: "Cron", kind: "CronJob"},
		{title: "Jobs", kind: "Job"},
		{title: "PVCs", kind: "PersistentVolumeClaim"},
		{title: "Configs", kind: "ConfigMap"},
		{title: "Secrets", kind: "Secret"},
		{title: "Services", kind: "Service"},

		{title: "Ingresses", kind: "Ingress"},
		{title: "Accounts", kind: "ServiceAccount"},
	}
)

type WidgetConstructor func(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat) commander.Widget

type resourceMenu struct {
	*menu.Menu
}

func NewResourcesMenu(workspace commander.Workspace, onSelect menu.SelectFunc, resourceProvider commander.ResourceProvider) (*resourceMenu, error) {
	initialResMap := client.CoreResources()
	itemProvider := make(menu.ItemProvider)
	go func() {
		defer close(itemProvider)
		itemProvider <- buildItems(workspace, initialResMap)
		serverResources, err := resourceProvider.Resources()
		if err != nil {
			// TODO: handle
			return
		}
		itemProvider <- buildItems(workspace, serverResources)
	}()
	m := menu.NewMenu(itemProvider, workspace.ScreenUpdater())
	m.BindOnSelect(onSelect)
	return &resourceMenu{Menu: m}, nil
}

func buildItems(workspace commander.Workspace, resources commander.ResourceMap) []commander.MenuItem {
	var items []commander.MenuItem
	for _, item := range itemMap {
		res, ok := resources[item.kind]
		if !ok {
			continue
		}

		constructor, ok := CustomWidgets[item.kind]
		if !ok {
			constructor = StandardWidget
		}

		items = append(items, menu.NewItem(item.title, constructor(workspace, res, listTable.Wide|listTable.WithHeaders)))
	}
	return items
}
