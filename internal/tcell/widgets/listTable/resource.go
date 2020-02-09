package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/gdamore/tcell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"strings"
)

type ResourceListTable struct {
	*ReloadableListTable

	client    client.Client
	resource  *client.Resource
	namespace NamespaceAccessor
	shell     ShellFunc

	table metav1.Table
}

type NamespaceAccessor func() string
type ShellFunc func(string) error

func NewClusterResourceListTable(client client.Client, resource *client.Resource, shell ShellFunc) *ResourceListTable {
	return NewResourceListTable(client, resource, shell, func() string {
		return ""
	})
}

func NewResourceListTable(client client.Client, resource *client.Resource, shell ShellFunc, ns NamespaceAccessor) *ResourceListTable {
	resourceLt := &ResourceListTable{
		client:    client,
		resource:  resource,
		namespace: ns,
		shell:     shell,
	}
	r := NewReloadableListTable(true, resourceLt.loadResourceRows)
	resourceLt.ReloadableListTable = r
	resourceLt.RegisterRowEventHandler(resourceLt)
	return resourceLt
}

func (r *ResourceListTable) loadResourceRows() ([]Column, []Row, error) {
	opts := metav1.ListOptions{}
	gv := r.resource.GroupVersion
	req, err := r.client.NewRequest(&gv)
	if err != nil {
		return nil, nil, err
	}

	req.
		Verb("GET").
		Resource(r.resource.Resource).
		VersionedParams(&opts, scheme.ParameterCodec).
		SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ",")).
		Namespace(r.namespace())
	err = req.Do().Into(&r.table)
	if err != nil {
		return nil, nil, err
	}

	var cols []Column
	var rows []Row

	for _, col := range r.table.ColumnDefinitions {
		cols = append(cols, NewStringColumn(col.Name))
	}

	for _, row := range r.table.Rows {
		var newRow Row
		for _, cell := range row.Cells {
			newRow = append(newRow, cell)
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
	metadata, err := r.rowMetadata(rowIndex)
	if err != nil {
		return err
	}
	return r.shell(r.client.Viewer(r.client.Describe(r.namespace(), r.resource.Resource, metadata.Name)))
}

func (r ResourceListTable) edit(rowIndex int) error {
	metadata, err := r.rowMetadata(rowIndex)
	if err != nil {
		return err
	}
	return r.shell(r.client.Edit(r.namespace(), r.resource.Resource, metadata.Name))
}
