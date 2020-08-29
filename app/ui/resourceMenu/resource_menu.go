package resourceMenu

import (
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/pod"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
)

type resourceItem struct {
	title      string
	gk         schema.GroupKind
	resource   *commander.Resource
	widget     commander.Widget
	decoration string
}

func (r resourceItem) Id() string {
	return r.gk.String()
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
	clusterGKs = []schema.GroupKind{
		{Kind: "Namespace", Group: ""},
		{Kind: "Node", Group: ""},
		{Kind: "StorageClass", Group: "storage.k8s.io"},
		{Kind: "PersistentVolume", Group: ""},
	}
	namespacedGKs = []schema.GroupKind{
		{Kind: "Deployment", Group: "apps"},
		{Kind: "StatefulSet", Group: "apps"},
		{Kind: "DaemonSet", Group: "apps"},
		{Kind: "ReplicaSet", Group: "apps"},
		{Kind: "Pod", Group: ""},
		{Kind: "CronJob", Group: "batch"},
		{Kind: "Job", Group: "batch"},
		{Kind: "PersistentVolumeClaim", Group: ""},
		{Kind: "ConfigMap", Group: ""},
		{Kind: "Secret", Group: ""},
		{Kind: "Service", Group: ""},
		{Kind: "Ingress", Group: "networking.k8s.io"},
		{Kind: "ServiceAccount", Group: ""},
	}
	knownTitles = map[string]string{
		"Namespace":             "Namespaces",
		"Node":                  "Nodes",
		"StorageClass":          "Storage",
		"PersistentVolume":      "Volumes",
		"Deployment":            "Deployments",
		"StatefulSet":           "Stateful",
		"DaemonSet":             "Daemons",
		"ReplicaSet":            "Replicas",
		"Pod":                   "Pods",
		"CronJob":               "Cron Jobs",
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

	clusterItems         int
	extraClusterItems    []*resourceItem
	extraNamespacedItems []*resourceItem
	showExtra            bool

	onSelect        SelectFunc
	selectNamespace func()

	resources   commander.ResourceProvider
	rowProvider commander.RowProvider
	workspace   commander.Workspace
}

func NewResourcesMenu(workspace commander.Workspace, onSelect SelectFunc, selectNamespace func(), resourceProvider commander.ResourceProvider) (*ResourceMenu, error) {
	prov := make(commander.RowProvider)
	lt := listTable.NewListTable(prov, listTable.NoHorizontalScroll|listTable.WithFilter, workspace.ScreenUpdater())
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
	var ops []commander.Operation

	ops = append(ops,
		&commander.OpClear{},
		&commander.OpSetColumns{Columns: []string{"Title"}},
		&commander.OpInitStart{},
	)

	cluster, namespaced := r.splitResources(client.CoreResources())
	clusterItems, _ := r.buildResourceItems(cluster, clusterGKs)
	r.clusterItems = len(clusterItems)
	namespacedItems, _ := r.buildResourceItems(namespaced, namespacedGKs)
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
		r.workspace.Status().Error(err)
		return
	}
	ops = []commander.Operation{}
	cluster, namespaced = r.splitResources(serverResources)
	clusterItems, r.extraClusterItems = r.buildResourceItems(cluster, clusterGKs)
	namespacedItems, r.extraNamespacedItems = r.buildResourceItems(namespaced, namespacedGKs)
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
	if event.Key() == tcell.KeyF3 {
		go r.toggleExtra()
		return true
	}
	return false
}

func (r *ResourceMenu) toggleExtra() {
	var ops []commander.Operation
	if r.showExtra {
		for _, item := range r.extraClusterItems {
			ops = append(ops, &commander.OpDeleted{
				RowId: item.Id(),
			})
		}
		for _, item := range r.extraNamespacedItems {
			ops = append(ops, &commander.OpDeleted{
				RowId: item.Id(),
			})
		}
	} else {
		for i, item := range r.extraClusterItems {
			index := r.clusterItems + i
			ops = append(ops, &commander.OpAdded{
				Row:   item,
				Index: &index,
			})
		}
		for _, item := range r.extraNamespacedItems {
			ops = append(ops, &commander.OpAdded{
				Row:      item,
				SortById: false,
			})
		}
	}
	r.rowProvider <- ops
	r.showExtra = !r.showExtra
}
func (r *ResourceMenu) SelectItem(id string) {
	r.ListTable.SelectId(id)
}

func (r *ResourceMenu) buildResourceItems(resources commander.ResourceMap, gks []schema.GroupKind) ([]*resourceItem, []*resourceItem) {
	var items []*resourceItem
	var leftovers []*resourceItem
	visited := make(map[string]struct{})
	for _, gk := range gks {
		res := resources[gk]
		items = append(items, r.buildItem(gk, res))
		if res != nil {
			visited[res.Gvk.String()] = struct{}{}
		}
	}
	for kind, res := range resources {
		if _, ok := visited[res.Gvk.String()]; ok {
			continue
		}
		leftovers = append(leftovers, r.buildItem(kind, res))
	}
	return items, leftovers
}

func (r *ResourceMenu) buildItem(gk schema.GroupKind, res *commander.Resource) *resourceItem {
	title, ok := knownTitles[gk.Kind]
	if !ok {
		title = plural(gk.Kind)
	}
	item := &resourceItem{
		title: title,
		gk:    gk,
	}
	if res != nil {
		constructor, ok := CustomWidgets[res.Gvk.Kind]
		if !ok {
			constructor = StandardWidget
		}
		item.resource = res
		item.widget = constructor(r.workspace, res, listTable.Wide|listTable.WithHeaders|listTable.WithFilter)
	}
	return item
}
