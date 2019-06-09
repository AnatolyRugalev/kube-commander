package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatefulSetsTable struct {
	namespace string
}

func (st *StatefulSetsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Pods(st.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (st *StatefulSetsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Stateful Set " + st.namespace + "/" + row[0]
}

func (st *StatefulSetsTable) TypeName() string {
	return "statefulset"
}

func (st *StatefulSetsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (st *StatefulSetsTable) Namespace() string {
	return st.namespace
}

func (st *StatefulSetsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(st)
}

func NewStatefulSetsTable(namespace string) *widgets.DataTable {
	pt := &StatefulSetsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Stateful Sets <" + namespace + ">"
	return lt
}

func (st *StatefulSetsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DESIRED", "CURRENT", "AGE"}
}

func (st *StatefulSetsTable) LoadData() ([]widgets.ListRow, error) {
	sets, err := kube.GetClient().AppsV1().StatefulSets(st.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, set := range sets.Items {
		rows = append(rows, st.newRow(set))
	}
	return rows, nil
}

func (st *StatefulSetsTable) newRow(set v1.StatefulSet) []string {

	return widgets.ListRow{
		set.Name,
		fmt.Sprintf("%d", *set.Spec.Replicas),
		fmt.Sprintf("%d", set.Status.ReadyReplicas),
		Age(set.CreationTimestamp.Time),
	}
}
