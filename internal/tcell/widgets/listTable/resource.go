package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/gdamore/tcell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ResourceListTable struct {
	*ReloadableListTable

	client    client.Client
	resource  *client.Resource
	namespace NamespaceAccessor
	shell     ShellFunc
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

type NamespaceAccessor func() string
type ShellFunc func(string) error

func NewClusterResourceListTable(client client.Client, resource *client.Resource, shell ShellFunc, opts *ResourceListTableOptions) *ResourceListTable {
	return NewResourceListTable(client, resource, shell, opts, func() string {
		return ""
	})
}

func NewResourceListTable(client client.Client, resource *client.Resource, shell ShellFunc, opts *ResourceListTableOptions, ns NamespaceAccessor) *ResourceListTable {
	if opts == nil {
		opts = DefaultResourceTableOpts
	}
	resourceLt := &ResourceListTable{
		client:    client,
		resource:  resource,
		namespace: ns,
		shell:     shell,
		opts:      opts,
	}
	r := NewReloadableListTable(opts.ShowHeaders, resourceLt.loadResourceRows)
	resourceLt.ReloadableListTable = r
	resourceLt.RegisterRowEventHandler(resourceLt)
	return resourceLt
}

func (r *ResourceListTable) loadResourceRows() ([]Column, []Row, error) {
	var err error
	r.table, err = r.client.LoadResourceToTable(r.resource, r.namespace())
	if err != nil {
		return nil, nil, err
	}

	var cols []Column
	var rows []Row
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
			cols = append(cols, NewStringColumn(col.Name))
			colIds = append(colIds, colId)
		}
	}

	for _, row := range r.table.Rows {
		var newRow Row
		for _, colId := range colIds {
			newRow = append(newRow, row.Cells[colId])
		}
		rows = append(rows, newRow)
	}

	return cols, rows, nil
}

func (r *ResourceListTable) HandleRowEvent(event RowEvent) bool {
	switch e := event.(type) {
	case *RowTcellEvent:
		switch ev := e.ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyCtrlD:
				err := r.describe(event.RowId())
				if err != nil {
					// TODO: error handling
				}
				return true
			case tcell.KeyCtrlE:
				err := r.edit(event.RowId())
				if err != nil {
					// TODO: error handling
				}
				return true
			}
		}
	}
	return false
}

func (r ResourceListTable) rowMetadata(rowIndex int) (*metav1.PartialObjectMetadata, error) {
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

func (r ResourceListTable) describe(rowIndex int) error {
	if r.shell == nil {
		return nil
	}
	metadata, err := r.rowMetadata(rowIndex)
	if err != nil {
		return err
	}
	return r.shell(r.client.Viewer(r.client.Describe(metadata.Namespace, r.resource.Resource, metadata.Name)))
}

func (r ResourceListTable) edit(rowIndex int) error {
	if r.shell == nil {
		return nil
	}
	metadata, err := r.rowMetadata(rowIndex)
	if err != nil {
		return err
	}
	return r.shell(r.client.Edit(metadata.Namespace, r.resource.Resource, metadata.Name))
}
