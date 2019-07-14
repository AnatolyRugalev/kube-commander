package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReplicaSetsTable struct {
	namespace string
}

func (dt *ReplicaSetsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().AppsV1().ReplicaSets(dt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (dt *ReplicaSetsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Replica Set " + dt.namespace + "/" + row[0]
}

func (dt *ReplicaSetsTable) TypeName() string {
	return "replicaset"
}

func (dt *ReplicaSetsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (dt *ReplicaSetsTable) Namespace() string {
	return dt.namespace
}

func (dt *ReplicaSetsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(dt)
}

func NewReplicaSetsTable(namespace string) *widgets.DataTable {
	pt := &ReplicaSetsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Replica Sets"
	return lt
}

func (dt *ReplicaSetsTable) LoadData() ([]widgets.ListRow, error) {
	replicaSets, err := kube.GetClient().AppsV1().ReplicaSets(dt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, rs := range replicaSets.Items {
		rows = append(rows, dt.newRow(rs))
	}
	return rows, nil
}

func (dt *ReplicaSetsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DESIRED", "CURRENT", "READY", "AGE"}
}

func (dt *ReplicaSetsTable) newRow(rs v1.ReplicaSet) []string {
	return widgets.ListRow{
		rs.Name,
		fmt.Sprintf("%d", *rs.Spec.Replicas),
		fmt.Sprintf("%d", rs.Status.Replicas),
		fmt.Sprintf("%d", rs.Status.ReadyReplicas),
		Age(rs.CreationTimestamp.Time),
	}
}
