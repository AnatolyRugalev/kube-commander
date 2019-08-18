package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	v1 "k8s.io/api/core/v1"
)

type NamespacesListTable struct {
	*listTable.ResourceListTable
}

func NewNamespacesListTable() *NamespacesListTable {
	namespaces := v1.NamespaceList{}
	lt := listTable.NewClusterResourceListTable(
		v1.SchemeGroupVersion.WithResource("namespaces"),
		&namespaces,
		func(addRow listTable.AddRowFunc) {
			for _, item := range namespaces.Items {
				addRow(
					item.Name,
					item.Status.Phase,
					item.CreationTimestamp,
				)
			}
		},
		[]listTable.Column{
			listTable.NewStringColumn("Name"),
			listTable.NewStringColumn("Status"),
			listTable.NewAgeColumn(),
		},
	)
	return &NamespacesListTable{ResourceListTable: lt}
}
