package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

type ResourceListTable struct {
	*ListTable

	container commander.ResourceContainer
	resource  *commander.Resource

	stopWatchCh chan struct{}
	rowProvider commander.RowProvider
	format      TableFormat

	table *metav1.Table
}

type TableFormat uint8

const (
	WithHeaders TableFormat = 1 << iota
	Wide
	Short
	NameOnly
)

func NewResourceListTable(container commander.ResourceContainer, resource *commander.Resource, format TableFormat, updater commander.ScreenUpdater) *ResourceListTable {
	resourceLt := &ResourceListTable{
		container:   container,
		resource:    resource,
		rowProvider: make(commander.RowProvider),
		format:      format,
	}
	resourceLt.ListTable = NewListTable(resourceLt.rowProvider, format, updater)
	resourceLt.BindOnKeyPress(resourceLt.OnKeyPress)
	return resourceLt
}

func (r *ResourceListTable) OnKeyPress(rowId int, _ commander.Row, event *tcell.EventKey) bool {
	switch event.Rune() {
	case 'D', 'd':
		go r.describe(rowId)
		return true
	case 'E', 'e':
		go r.edit(rowId)
		return true
	}
	return false
}

func (r *ResourceListTable) OnShow() {
	r.stopWatchCh = make(chan struct{})
	go r.provideRows(r.format, r.rowProvider)
	r.ListTable.OnShow()
}

func (r *ResourceListTable) OnHide() {
	r.ListTable.OnHide()
	close(r.stopWatchCh)
}

func (r *ResourceListTable) provideRows(format TableFormat, prov commander.RowProvider) {
	prov <- []commander.Operation{{Type: commander.OpLoading}}

	columns, rows, err := r.loadResourceRows(format)
	var ops []commander.Operation
	if err != nil {
		prov <- []commander.Operation{{Type: commander.OpLoadingFinished}}
		// TODO: handle
		return
	}
	ops = append(ops,
		commander.Operation{Type: commander.OpClear},
		commander.Operation{Type: commander.OpColumns, Row: commander.NewSimpleRow("", columns)},
	)
	for _, row := range rows {
		ops = append(ops, commander.Operation{Type: commander.OpAdded, Row: row})
	}
	ops = append(ops, commander.Operation{Type: commander.OpLoadingFinished})
	prov <- ops
	watcher, err := r.container.Client().WatchAsTable(r.resource, r.container.CurrentNamespace())
	if err != nil {
		//TODO: handle
		return
	}
	go func() {
		defer watcher.Stop()
		for {
			select {
			case <-r.stopWatchCh:
				return
			case event := <-watcher.ResultChan():
				var op commander.OpType
				switch event.Type {
				case watch.Added:
					op = commander.OpAdded
				case watch.Modified:
					op = commander.OpModified
				case watch.Deleted:
					op = commander.OpDeleted
				case watch.Error:
					panic(event.Object)
				}
				table, ok := event.Object.(*metav1.Table)
				if ok {
					var ops []commander.Operation
					for _, row := range table.Rows {
						k8sRow, err := commander.NewKubernetesRow(row)
						if err != nil {
							// TODO: handle
							return
						}
						ops = append(ops, commander.Operation{Type: op, Row: k8sRow})
					}
					prov <- ops
				}
			}
		}
	}()
}

func (r *ResourceListTable) loadResourceRows(format TableFormat) ([]string, []commander.Row, error) {
	var err error
	r.table, err = r.container.Client().ListAsTable(r.resource, r.container.CurrentNamespace())
	if err != nil {
		return nil, nil, err
	}

	var cols []string
	var rows []commander.Row
	var colIds []int

	for colId, col := range r.table.ColumnDefinitions {
		add := false
		switch {
		case format&Wide != 0:
			add = true
		case format&Short != 0:
			add = col.Priority == 0
		case format&NameOnly != 0:
			add = col.Name == "Name"
		}
		if add {
			cols = append(cols, col.Name)
			colIds = append(colIds, colId)
		}
	}

	for _, row := range r.table.Rows {
		k8sRow, err := commander.NewKubernetesRow(row)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, k8sRow)
	}

	return cols, rows, nil
}

func (r ResourceListTable) RowMetadata(rowIndex int) (*metav1.PartialObjectMetadata, error) {
	if len(r.table.Rows) <= rowIndex {
		return nil, fmt.Errorf("invalid row index")
	}
	obj := r.table.Rows[rowIndex].Object
	metadata := &metav1.PartialObjectMetadata{}
	err := runtime.DecodeInto(unstructured.UnstructuredJSONScheme, obj.Raw, metadata)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func (r ResourceListTable) describe(rowIndex int) {
	metadata, err := r.RowMetadata(rowIndex)
	if err != nil {
		r.container.HandleError(err)
		return
	}
	e := r.container.CommandExecutor()
	b := r.container.CommandBuilder()
	err = e.Pipe(b.Describe(metadata.Namespace, r.resource.Resource, metadata.Name), b.Pager())
	if err != nil {
		r.container.HandleError(err)
		return
	}
}

func (r ResourceListTable) edit(rowIndex int) {
	metadata, err := r.RowMetadata(rowIndex)
	if err != nil {
		r.container.HandleError(err)
		return
	}
	e := r.container.CommandExecutor()
	b := r.container.CommandBuilder()
	err = e.Pipe(b.Edit(metadata.Namespace, r.resource.Resource, metadata.Name))
	if err != nil {
		r.container.HandleError(err)
		return
	}
}
