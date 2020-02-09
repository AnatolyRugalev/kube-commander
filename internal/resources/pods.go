package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/events"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
)

type PodsListTable struct {
	*listTable.ResourceListTable
}

func (p PodsListTable) HandleEvent(ev tcell.Event) bool {
	switch e := ev.(type) {
	case *events.NamespaceChanged:
		panic(e.Namespace())
	}
	return false
}

func NewPodsListTable() *PodsListTable {
	pods := v1.PodList{}
	lt := listTable.NewResourceListTable(
		v1.SchemeGroupVersion.WithResource("pods"),
		&pods,
		func(addRow listTable.AddRowFunc) {
			for _, p := range pods.Items {
				addRow(
					p.Name,
				)
			}
		},
		[]listTable.Column{
			listTable.NewStringColumn("Name"),
		},
		func() string {
			return "kube-system"
		},
	)
	return &PodsListTable{ResourceListTable: lt}
}
