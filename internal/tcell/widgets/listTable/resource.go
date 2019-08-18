package listTable

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"time"
)

type ResourceListTable struct {
	*ReloadableListTable
}

type List interface {
	runtime.Object
	meta.List
}

type AddRowFunc func(cols ...interface{})
type rowFunc func(addRow AddRowFunc)
type NamespaceAccessor func() string

func NewClusterResourceListTable(gvr schema.GroupVersionResource, list List, rowFunc rowFunc, columns []Column) *ResourceListTable {
	return NewResourceListTable(gvr, list, rowFunc, columns, nil)
}

func NewResourceListTable(gvr schema.GroupVersionResource, list List, rowFunc rowFunc, columns []Column, ns NamespaceAccessor) *ResourceListTable {
	r := NewReloadableListTable(columns, true, loadResourceRows(gvr, list, rowFunc, ns))
	return &ResourceListTable{
		ReloadableListTable: r,
	}
}

func loadResourceRows(gvr schema.GroupVersionResource, list List, rowFunc rowFunc, ns NamespaceAccessor) func() ([]Row, error) {
	return func() ([]Row, error) {
		timeout := time.Duration(kube.GetTimeout()) * time.Second
		opts := v1.ListOptions{}
		gv := gvr.GroupVersion()
		r, err := kube.RESTClientFor(&gv)
		if err != nil {
			return nil, err
		}
		req := r.Get().
			Resource(gvr.Resource).
			VersionedParams(&opts, scheme.ParameterCodec).
			Timeout(timeout)
		if ns != nil {
			req = req.Namespace(ns())
		}
		err = req.Do().Into(list)
		if err != nil {
			return nil, err
		}
		var rows []Row
		rowFunc(func(cols ...interface{}) {
			rows = append(rows, cols)
		})
		return rows, nil
	}
}
