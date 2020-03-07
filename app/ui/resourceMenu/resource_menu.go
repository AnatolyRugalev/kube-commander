package resourceMenu

import (
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
	StandartWidget WidgetConstructor = func(workspace commander.Workspace, resource *commander.Resource, options *listTable.ResourceListTableOptions) commander.Widget {
		return listTable.NewResourceListTable(workspace, resource, options)
	}
	CustomWidgets = map[string]WidgetConstructor{
		"Pod": func(workspace commander.Workspace, resource *commander.Resource, options *listTable.ResourceListTableOptions) commander.Widget {
			return pod.NewPodsList(workspace, resource, options)
		},
	}
	itemMap = []*item{
		{title: "Namespaces", kind: "Namespace"},
		{title: "Nodes", kind: "Node"},
		{title: "Storage Classes", kind: "StorageClass"},
		{title: "PVs", kind: "PersistentVolume"},
		{title: "Deployment", kind: "Deployments"},
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

type WidgetConstructor func(workspace commander.Workspace, resource *commander.Resource, options *listTable.ResourceListTableOptions) commander.Widget

type resourceMenu struct {
	*menu.Menu
}

func NewResourcesMenu(workspace commander.Workspace, onSelect menu.SelectFunc) (*resourceMenu, error) {
	res, err := workspace.ResourceProvider().Resources()
	if err != nil {
		return nil, err
	}
	m := menu.NewMenu(buildItems(workspace, res))
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
			constructor = StandartWidget
		}

		items = append(items, menu.NewItem(item.title, constructor(workspace, res, nil)))
	}
	return items
}
