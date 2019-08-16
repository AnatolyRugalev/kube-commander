package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodesListTable struct {
	*listTable.ReloadableListTable
}

func NewNodesListTable() *NodesListTable {
	lt := listTable.NewReloadableListTable(
		[]listTable.Column{
			listTable.NewStringColumn("Name"),
			listTable.NewStringColumn("Status"),
			listTable.NewAgeColumn(),
		},
		true,
		func() []listTable.Row {
			Nodes, _ := kube.GetClient().CoreV1().Nodes().List(v1.ListOptions{})
			var rows []listTable.Row
			for _, ns := range Nodes.Items {
				rows = append(rows, listTable.Row{
					ns.Name,
					ns.Status.Phase,
					ns.CreationTimestamp,
				})
			}
			return rows
		},
	)
	return &NodesListTable{ReloadableListTable: lt}
}
