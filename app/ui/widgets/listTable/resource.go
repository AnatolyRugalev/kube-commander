package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/spf13/cast"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ResourceListTable struct {
	*ReloadableListTable

	container commander.ResourceContainer
	resource  *commander.Resource
	opts      *ResourceListTableOptions

	table *metav1.Table
}

type ResourceTableFormat int

const (
	FormatWide = ResourceTableFormat(iota)
	FormatShort
	FormatNameOnly
)

var DefaultResourceTableOpts = &ResourceListTableOptions{
	ShowHeaders: true,
	Format:      FormatWide,
}

type ResourceListTableOptions struct {
	ShowHeaders bool
	Format      ResourceTableFormat
}

func NewResourceListTable(container commander.ResourceContainer, resource *commander.Resource, opts *ResourceListTableOptions) *ResourceListTable {
	if opts == nil {
		opts = DefaultResourceTableOpts
	}
	resourceLt := &ResourceListTable{
		container: container,
		resource:  resource,
		opts:      opts,
	}
	resourceLt.ReloadableListTable = NewReloadableListTable(container, opts.ShowHeaders, resourceLt.loadResourceRows)
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

func (r *ResourceListTable) loadResourceRows() ([]string, []commander.Row, error) {
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
		switch r.opts.Format {
		case FormatWide:
			add = true
		case FormatShort:
			add = col.Priority == 0
		case FormatNameOnly:
			add = col.Name == "Name"
		}
		if add {
			cols = append(cols, col.Name)
			colIds = append(colIds, colId)
		}
	}

	for _, row := range r.table.Rows {
		var newRow commander.Row
		for _, colId := range colIds {
			newRow = append(newRow, cast.ToString(row.Cells[colId]))
		}
		rows = append(rows, newRow)
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
	err = e.Pipe(b.Describe(metadata.Namespace, r.resource.Resource, metadata.Name), b.Viewer())
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
