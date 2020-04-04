package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type ResourceListTable struct {
	*ListTable

	container commander.ResourceContainer
	resource  *commander.Resource

	stopWatchCh chan struct{}
	rowProvider commander.RowProvider
	format      TableFormat
	extraRows   map[int]commander.Row
}

func NewResourceListTable(container commander.ResourceContainer, resource *commander.Resource, format TableFormat) *ResourceListTable {
	resourceLt := &ResourceListTable{
		container:   container,
		resource:    resource,
		rowProvider: make(commander.RowProvider),
		format:      format,
	}
	resourceLt.ListTable = NewListTable(resourceLt.rowProvider, format, container.ScreenUpdater())
	if !format.Has(NoActions) {
		resourceLt.BindOnKeyPress(resourceLt.OnKeyPress)
	}
	return resourceLt
}

func (r *ResourceListTable) SetExtraRows(rows map[int]commander.Row) {
	r.extraRows = rows
}

func (r *ResourceListTable) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	switch event.Rune() {
	case 'd':
		go r.describe(row)
		return true
	case 'e':
		go r.edit(row)
		return true
	case 'c':
		go r.copy(row)
		return true
	}
	return false
}

func (r *ResourceListTable) OnShow() {
	r.stopWatchCh = make(chan struct{})
	go r.provideRows()
	r.ListTable.OnShow()
}

func (r *ResourceListTable) OnHide() {
	r.ListTable.OnHide()
	close(r.stopWatchCh)
}

func (r *ResourceListTable) provideRows() {
	r.rowProvider <- []commander.Operation{&commander.OpInitStart{}}

	columns, rows, err := r.loadResourceRows()
	var ops []commander.Operation
	if err != nil {
		r.rowProvider <- []commander.Operation{&commander.OpInitFinished{}}
		r.container.Status().Error(err)
		return
	}
	ops = append(ops,
		&commander.OpClear{},
		&commander.OpSetColumns{Columns: columns},
	)
	for _, row := range rows {
		ops = append(ops, &commander.OpAdded{Row: row})
	}
	for index, row := range r.extraRows {
		ops = append(ops, &commander.OpAdded{Row: row, Index: &index})
	}
	ops = append(ops, &commander.OpInitFinished{})
	r.rowProvider <- ops
	if r.format.Has(NoWatch) {
		return
	}
	watcher, err := r.container.Client().WatchAsTable(r.resource, r.container.CurrentNamespace())
	if err != nil {
		r.container.Status().Error(err)
		return
	}
	go func() {
		defer watcher.Stop()
		for {
			select {
			case <-r.stopWatchCh:
				return
			case event := <-watcher.ResultChan():
				if event.Type == watch.Error {
					err := apierrs.FromObject(event.Object)
					r.container.Status().Error(fmt.Errorf("error while watching: %w", err))
					return
				}
				var ops []commander.Operation
				rows, err := r.extractRows(event)
				if err != nil {
					r.container.Status().Error(err)
					return
				}
				switch event.Type {
				case watch.Added:
					for _, row := range rows {
						ops = append(ops, &commander.OpAdded{Row: row})
					}
				case watch.Modified:
					for _, row := range rows {
						ops = append(ops, &commander.OpModified{Row: row})
					}
				case watch.Deleted:
					for _, row := range rows {
						ops = append(ops, &commander.OpDeleted{RowId: row.Id()})
					}
				}
				if len(ops) > 0 {
					r.rowProvider <- ops
				}
			}
		}
	}()
}

func (r *ResourceListTable) extractRows(event watch.Event) ([]commander.Row, error) {
	var rows []commander.Row
	table, ok := event.Object.(*metav1.Table)
	if ok {
		for _, row := range table.Rows {
			k8sRow, err := commander.NewKubernetesRow(row)
			if err != nil {
				return nil, err
			}
			rows = append(rows, k8sRow)
		}
	}
	return rows, nil
}

func (r *ResourceListTable) loadResourceRows() ([]string, []commander.Row, error) {
	table, err := r.container.Client().ListAsTable(r.resource, r.container.CurrentNamespace())
	if err != nil {
		return nil, nil, err
	}

	var cols []string
	var rows []commander.Row
	var colIds []int

	for colId, col := range table.ColumnDefinitions {
		add := false
		switch {
		case r.format&Wide != 0:
			add = true
		case r.format&Short != 0:
			add = col.Priority == 0
		case r.format&NameOnly != 0:
			add = col.Name == "Name"
		}
		if add {
			cols = append(cols, col.Name)
			colIds = append(colIds, colId)
		}
	}

	for _, row := range table.Rows {
		k8sRow, err := commander.NewKubernetesRow(row)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, k8sRow)
	}

	return cols, rows, nil
}

func (r ResourceListTable) RowMetadata(row commander.Row) (*metav1.PartialObjectMetadata, error) {
	k8sRow, ok := row.(*commander.KubernetesRow)
	if ok {
		return k8sRow.Metadata(), nil
	}
	return nil, fmt.Errorf("invalid row")
}

func (r ResourceListTable) describe(row commander.Row) {
	metadata, err := r.RowMetadata(row)
	if err != nil {
		r.container.Status().Error(err)
		return
	}
	e := r.container.CommandExecutor()
	b := r.container.CommandBuilder()
	err = e.Pipe(b.Describe(metadata.Namespace, r.resource.Resource, metadata.Name), b.Pager())
	if err != nil {
		r.container.Status().Error(err)
		return
	}
}

func (r ResourceListTable) edit(row commander.Row) {
	metadata, err := r.RowMetadata(row)
	if err != nil {
		r.container.Status().Error(err)
		return
	}
	e := r.container.CommandExecutor()
	b := r.container.CommandBuilder()
	err = e.Pipe(b.Edit(metadata.Namespace, r.resource.Resource, metadata.Name))
	if err != nil {
		r.container.Status().Error(err)
		return
	}
}

func (r ResourceListTable) copy(row commander.Row) {
	metadata, err := r.RowMetadata(row)
	if err != nil {
		r.container.Status().Error(err)
		return
	}
	err = clipboard.WriteAll(metadata.Name)
	if err != nil {
		r.container.Status().Error(err)
		return
	}
	r.container.Status().Info(fmt.Sprintf("Resource name copied! '%s'", metadata.Name))
}
