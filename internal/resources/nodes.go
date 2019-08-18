package resources

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	v1 "k8s.io/api/core/v1"
	"strings"
)

type NodesListTable struct {
	*listTable.ResourceListTable
}

func NewNodesListTable() *NodesListTable {
	nodes := v1.NodeList{}
	lt := listTable.NewClusterResourceListTable(
		v1.SchemeGroupVersion.WithResource("nodes"),
		&nodes,
		func(addRow listTable.AddRowFunc) {
			for _, n := range nodes.Items {
				var roles []string
				for l := range n.Labels {
					if strings.HasPrefix(l, "node-role.kubernetes.io/") {
						roles = append(roles, l[24:])
					}
				}
				addRow(
					n.Name,
					n.Status.Conditions,
					roles,
					n.CreationTimestamp,
					n.Status.NodeInfo.KubeletVersion,
				)
			}
		},
		[]listTable.Column{
			listTable.NewStringColumn("Name"),
			newNodeStatusColumn(),
			listTable.NewTagsColumn("Roles", "<none>"),
			listTable.NewAgeColumn(),
			listTable.NewStringColumn("Version"),
		},
	)
	return &NodesListTable{ResourceListTable: lt}
}

type nodeStatusColumn struct {
	*listTable.TagsColumn
}

func newNodeStatusColumn() *nodeStatusColumn {
	return &nodeStatusColumn{
		TagsColumn: listTable.NewTagsColumn("Status", "<none>"),
	}
}

func (n *nodeStatusColumn) Render(value interface{}) (string, error) {
	var conditions []string
	for _, c := range value.([]v1.NodeCondition) {
		if c.Status == v1.ConditionTrue {
			conditions = append(conditions, string(c.Type))
		}
	}
	return n.TagsColumn.Render(conditions)
}
