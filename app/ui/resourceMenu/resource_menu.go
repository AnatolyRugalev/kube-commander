package resourceMenu

import (
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/pod"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"strings"
)

type item interface {
	commander.Row
	OnSelect() bool
}

type resourceItem struct {
	title      string
	resource   *commander.Resource
	widget     commander.Widget
	decoration string
}

func (r resourceItem) Id() string {
	return r.title
}

func (r resourceItem) Cells() []string {
	return []string{r.decoration + r.title}
}

func (r resourceItem) Enabled() bool {
	return r.widget != nil
}

func (r resourceItem) OnSelect() bool {
	panic("implement me")
}

type namespaceSelector struct {
	accessor commander.NamespaceAccessor
}

func (n namespaceSelector) Id() string {
	return "__namespace__"
}

func (n namespaceSelector) Cells() []string {
	namespace := n.accessor.CurrentNamespace()
	if namespace == "" {
		namespace = "All Namespaces"
	}
	return []string{"► " + namespace}
}

func (n namespaceSelector) OnSelect() bool {
	panic("implement me")
}

func (n namespaceSelector) Enabled() bool {
	return true
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
	clusterKinds = []string{
		"Namespace",
		"Node",
		"StorageClass",
		"PersistentVolume",
	}
	namespacedKinds = []string{
		"Deployment",
		"StatefulSet",
		"DaemonSet",
		"ReplicaSet",
		"Pod",
		"CronJob",
		"Job",
		"PersistentVolumeClaim",
		"ConfigMap",
		"Secret",
		"Service",
		"Ingress",
		"ServiceAccount",
	}
	knownTitles = map[string]string{
		"Namespace":             "Namespaces",
		"Node":                  "Nodes",
		"StorageClass":          "Storage Classes",
		"PersistentVolume":      "PVs",
		"Deployment":            "Deployments",
		"StatefulSet":           "Stateful",
		"DaemonSet":             "Daemons",
		"ReplicaSet":            "Replicas",
		"Pod":                   "Pods",
		"CronJob":               "Cron",
		"Job":                   "Jobs",
		"PersistentVolumeClaim": "PVCs",
		"ConfigMap":             "Configs",
		"Secret":                "Secrets",
		"Service":               "Services",
		"Ingress":               "Ingresses",
		"ServiceAccount":        "Accounts",
	}
)

type WidgetConstructor func(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat) commander.Widget

type SelectFunc func(itemId string, widget commander.Widget) bool

type ResourceMenu struct {
	*listTable.ListTable

	clusterItems    []item
	namespacedItems []item

	itemIndex map[string]item

	onSelect        SelectFunc
	selectNamespace func()

	resources   commander.ResourceProvider
	rowProvider commander.RowProvider
	workspace   commander.Workspace
}

func NewResourcesMenu(workspace commander.Workspace, onSelect SelectFunc, selectNamespace func(), resourceProvider commander.ResourceProvider) (*ResourceMenu, error) {
	prov := make(commander.RowProvider)
	lt := listTable.NewListTable(prov, listTable.NoHorizontalScroll, workspace.ScreenUpdater())
	r := &ResourceMenu{
		ListTable:       lt,
		onSelect:        onSelect,
		selectNamespace: selectNamespace,
		resources:       resourceProvider,
		rowProvider:     prov,
		workspace:       workspace,
	}
	lt.BindOnKeyPress(r.OnKeyPress)
	return r, nil
}

func (r *ResourceMenu) provideItems() {
	defer close(r.rowProvider)
	var ops []commander.Operation

	ops = append(ops,
		&commander.OpClear{},
		&commander.OpSetColumns{Columns: []string{"Title"}},
		&commander.OpInitStart{},
	)

	cluster, namespaced := r.splitResources(client.CoreResources())
	clusterItems := r.buildResourceItems(cluster, clusterKinds)
	namespacedItems := r.buildResourceItems(namespaced, namespacedKinds)
	for _, item := range clusterItems {
		item.decoration = " "
		ops = append(ops, &commander.OpAdded{Row: item})
	}
	ops = append(ops, &commander.OpAdded{Row: &namespaceSelector{r.workspace}})
	for i, item := range namespacedItems {
		if i == len(namespacedItems)-1 {
			item.decoration = " └"
		} else {
			item.decoration = " ├"
		}
		ops = append(ops, &commander.OpAdded{Row: item})
	}
	r.rowProvider <- ops
	serverResources, err := r.resources.Resources()
	if err != nil {
		r.rowProvider <- []commander.Operation{&commander.OpInitFinished{}}
		r.workspace.HandleError(err)
		return
	}
	ops = []commander.Operation{}
	cluster, namespaced = r.splitResources(serverResources)
	clusterItems = r.buildResourceItems(cluster, clusterKinds)
	namespacedItems = r.buildResourceItems(namespaced, namespacedKinds)
	for _, item := range clusterItems {
		item.decoration = " "
		ops = append(ops, &commander.OpModified{Row: item})
	}
	for i, item := range namespacedItems {
		if i == len(namespacedItems)-1 {
			item.decoration = " └"
		} else {
			item.decoration = " ├"
		}
		ops = append(ops, &commander.OpModified{Row: item})
	}
	ops = append(ops, &commander.OpInitFinished{})
	r.rowProvider <- ops
}

func (r *ResourceMenu) OnShow() {
	go r.provideItems()
	r.ListTable.OnShow()
}

func (r *ResourceMenu) splitResources(m commander.ResourceMap) (commander.ResourceMap, commander.ResourceMap) {
	namespaced := make(commander.ResourceMap)
	cluster := make(commander.ResourceMap)
	for kind, res := range m {
		if res.Namespaced {
			namespaced[kind] = res
		} else {
			cluster[kind] = res
		}
	}
	return cluster, namespaced
}

func plural(s string) string {
	if strings.HasSuffix(s, "s") {
		return s + "es"
	} else {
		return s + "s"
	}
}

func (r *ResourceMenu) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		switch i := row.(type) {
		case *resourceItem:
			r.onSelect(row.Id(), i.widget)
		case *namespaceSelector:
			r.selectNamespace()
		}
		return true
	}
	return false
}

func (r *ResourceMenu) SelectItem(id string) {
	r.ListTable.SelectId(id)
}

func (r *ResourceMenu) buildResourceItems(resources commander.ResourceMap, order []string) []*resourceItem {
	var items []*resourceItem
	for _, kind := range order {
		title, ok := knownTitles[kind]
		if !ok {
			title = plural(kind)
		}

		item := &resourceItem{
			title: title,
		}
		if res, ok := resources[kind]; ok {
			constructor, ok := CustomWidgets[kind]
			if !ok {
				constructor = StandardWidget
			}
			item.resource = res
			item.widget = constructor(r.workspace, res, listTable.Wide|listTable.WithHeaders)
		}
		items = append(items, item)
	}
	return items
}
