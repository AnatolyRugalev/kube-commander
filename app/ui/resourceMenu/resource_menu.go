package resourceMenu

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/pod"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/AnatolyRugalev/kube-commander/pb"
	"github.com/gdamore/tcell"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
	"sync"
)

type resourceItem struct {
	title      string
	gk         schema.GroupKind
	namespaced bool
	resource   *commander.Resource
	widget     commander.Widget
	isLast     bool
}

func (r *resourceItem) Id() string {
	return r.gk.String()
}

func (r *resourceItem) Cells() []string {
	var decoration string
	if r.namespaced {
		if r.isLast {
			decoration = " └ "
		} else {
			decoration = " ├ "
		}
	} else {
		decoration = " "
	}
	return []string{decoration + r.title}
}

func (r *resourceItem) Enabled() bool {
	return r.widget != nil
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
	DefaultItems = []*pb.Resource{
		{Kind: "Namespace", Group: "", Title: "Namespaces"},
		{Kind: "Node", Group: "", Title: "Nodes"},
		{Kind: "StorageClass", Group: "storage.k8s.io", Title: "Storage"},
		{Kind: "PersistentVolume", Group: "", Title: "Volumes"},

		{Namespaced: true, Kind: "Deployment", Group: "apps", Title: "Deployments"},
		{Namespaced: true, Kind: "StatefulSet", Group: "apps", Title: "Stateful"},
		{Namespaced: true, Kind: "DaemonSet", Group: "apps", Title: "Daemons"},
		{Namespaced: true, Kind: "ReplicaSet", Group: "apps", Title: "Replicas"},
		{Namespaced: true, Kind: "Pod", Group: "", Title: "Pods"},
		{Namespaced: true, Kind: "CronJob", Group: "batch", Title: "Cron Jobs"},
		{Namespaced: true, Kind: "Job", Group: "batch", Title: "Jobs"},
		{Namespaced: true, Kind: "PersistentVolumeClaim", Group: "", Title: "PVCs"},
		{Namespaced: true, Kind: "ConfigMap", Group: "", Title: "Configs"},
		{Namespaced: true, Kind: "Secret", Group: "", Title: "Secrets"},
		{Namespaced: true, Kind: "Service", Group: "", Title: "Services"},
		{Namespaced: true, Kind: "Ingress", Group: "networking.k8s.io", Title: "Ingresses"},
		{Namespaced: true, Kind: "ServiceAccount", Group: "", Title: "ServiceAccounts"},
	}
)

type WidgetConstructor func(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat) commander.Widget

type SelectFunc func(itemId string, widget commander.Widget) bool

type ResourceMenu struct {
	*listTable.ListTable
	sync.Mutex

	onSelect        SelectFunc
	selectNamespace func()

	resources   commander.ResourceProvider
	rowProvider commander.RowProvider
	workspace   commander.Workspace

	items        []*resourceItem
	clusterItems int
}

func (r *ResourceMenu) ConfigUpdated(config *pb.Config) {
	items := config.Menu
	if len(items) == 0 {
		items = DefaultItems
	}
	ops, err := r.UpdateMenu(items)
	if err != nil {
		r.workspace.Status().Error(err)
		return
	}
	r.rowProvider <- ops
}

func newItemFromPb(res *pb.Resource) *resourceItem {
	return &resourceItem{
		gk:         schema.GroupKind{Kind: res.Kind, Group: res.Group},
		title:      res.Title,
		namespaced: res.Namespaced,
	}
}

func NewResourcesMenu(workspace commander.Workspace, onSelect SelectFunc, selectNamespace func(), resourceProvider commander.ResourceProvider) (*ResourceMenu, error) {
	prov := make(commander.RowProvider)
	lt := listTable.NewListTable(prov, listTable.NoHorizontalScroll|listTable.WithFilter, workspace.ScreenHandler())
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

func (r *ResourceMenu) UpdateMenu(items []*pb.Resource) ([]commander.Operation, error) {
	var ops []commander.Operation

	ops = append(ops,
		&commander.OpClear{},
		&commander.OpSetColumns{Columns: []string{"Title"}},
		&commander.OpInitStart{},
	)
	serverResources, err := r.resources.Resources()
	if err != nil {
		return nil, err
	}
	r.Lock()
	defer r.Unlock()
	r.items = []*resourceItem{}
	r.clusterItems = 0
	var clusterItems, namespacedItems []*resourceItem
	for _, pbItem := range items {
		item := newItemFromPb(pbItem)
		if pbItem.Namespaced {
			namespacedItems = append(namespacedItems, item)
		} else {
			clusterItems = append(clusterItems, item)
			r.clusterItems++
		}
		if item.title == "" {
			item.title = plural(item.gk.Kind)
		}
		resource, ok := serverResources[item.gk]
		if ok {
			item.resource = resource
			item.widget = r.createWidget(resource)
		}
		r.items = append(r.items, item)
	}
	if len(namespacedItems) > 0 {
		namespacedItems[len(namespacedItems)-1].isLast = true
	}
	for _, item := range clusterItems {
		ops = append(ops, &commander.OpAdded{Row: item})
	}
	ops = append(ops, &commander.OpAdded{Row: &namespaceSelector{r.workspace}})
	for _, item := range namespacedItems {
		ops = append(ops, &commander.OpAdded{Row: item})
	}
	ops = append(ops, &commander.OpInitFinished{})

	return ops, nil
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
	r.Lock()
	defer r.Unlock()
	if event.Key() == tcell.KeyEnter || event.Key() == tcell.KeyRight {
		switch i := row.(type) {
		case *resourceItem:
			r.onSelect(row.Id(), i.widget)
		case *namespaceSelector:
			r.selectNamespace()
		}
		return true
	} else if event.Key() == tcell.KeyDelete {
		go func() {
			if r.workspace.Status().Confirm("Do you want to hide this resource? (y/N)") {
				for i, item := range r.items {
					if item.Id() == row.Id() {
						r.items = append(r.items[:i], r.items[i+1:]...)
						break
					}
				}
				r.saveItems()
				r.workspace.Status().Info("Deleted.")
			} else {
				r.workspace.Status().Info("Cancelled.")
			}
		}()
		return true
	} else if event.Rune() == '+' {
		pickResource(r.workspace, func(res *commander.Resource) {
			add := &resourceItem{
				title:      plural(res.Gk.Kind),
				gk:         res.Gk,
				namespaced: res.Namespaced,
				resource:   res,
				widget:     r.createWidget(res),
			}
			r.Lock()
			defer r.Unlock()
			for _, item := range r.items {
				if item.Id() == add.Id() {
					return
				}
			}
			if add.namespaced {
				r.items[len(r.items)-1].isLast = false
				add.isLast = true
				r.items = append(r.items, add)
			} else {
				newItems := append(r.items[:r.clusterItems], add)
				newItems = append(newItems, r.items[r.clusterItems+1:]...)
				r.items = newItems
			}
			r.saveItems()
		})
	} else if event.Key() == tcell.KeyF6 {
		for i, item := range r.items {
			if item.Id() == row.Id() {
				edge := 0
				if item.namespaced {
					edge = r.clusterItems
				}
				if i > edge {
					r.items[i], r.items[i-1] = r.items[i-1], r.items[i]
					r.items[i].isLast, r.items[i-1].isLast = r.items[i-1].isLast, r.items[i].isLast
					r.saveItems()
				}
				break
			}
		}
	} else if event.Key() == tcell.KeyF7 {
		for i, item := range r.items {
			if item.Id() == row.Id() {
				edge := len(r.items) - 1
				if !item.namespaced {
					edge = r.clusterItems - 1
				}
				if i < edge {
					r.items[i], r.items[i+1] = r.items[i+1], r.items[i]
					r.items[i].isLast, r.items[i+1].isLast = r.items[i+1].isLast, r.items[i].isLast
					r.saveItems()
				}
				break
			}
		}
	}
	return false
}

func (r *ResourceMenu) SelectItem(id string) {
	r.ListTable.SelectId(id)
}

func (r *ResourceMenu) createWidget(res *commander.Resource) commander.Widget {
	constructor, ok := CustomWidgets[res.Gvk.Kind]
	if !ok {
		constructor = StandardWidget
	}
	return constructor(r.workspace, res, listTable.Wide|listTable.WithHeaders|listTable.WithFilter)
}

func (r *ResourceMenu) provideItems() {
	ops, err := r.UpdateMenu(DefaultItems)
	if err != nil {
		r.rowProvider <- []commander.Operation{&commander.OpInitFinished{}}
		r.workspace.Status().Error(err)
		return
	}
	r.rowProvider <- ops
}

func (r *ResourceMenu) saveItems() {
	var pbItems []*pb.Resource
	for _, item := range r.items {
		pbItems = append(pbItems, &pb.Resource{
			Namespaced: item.namespaced,
			Group:      item.gk.Group,
			Kind:       item.gk.Kind,
			Title:      item.title,
		})
	}
	err := r.workspace.UpdateConfig(func(config *pb.Config) {
		config.Menu = pbItems
	})
	if err != nil {
		r.workspace.Status().Error(err)
		return
	}
}
