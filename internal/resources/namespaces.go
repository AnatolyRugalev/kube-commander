package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesListTable struct {
	*listTable.ReloadableListTable
}

func NewNamespacesListTable() *NamespacesListTable {
	lt := listTable.NewReloadableListTable(
		[]listTable.Column{
			listTable.NewStringColumn("Name"),
			listTable.NewStringColumn("Status"),
			listTable.NewAgeColumn(),
		},
		true,
		func() []listTable.Row {
			namespaces, _ := kube.GetClient().CoreV1().Namespaces().List(v1.ListOptions{})
			var rows []listTable.Row
			for _, ns := range namespaces.Items {
				rows = append(rows, listTable.Row{
					ns.Name,
					ns.Status.Phase,
					ns.CreationTimestamp,
				})
			}
			return rows
		},
	)
	return &NamespacesListTable{ReloadableListTable: lt}
}
