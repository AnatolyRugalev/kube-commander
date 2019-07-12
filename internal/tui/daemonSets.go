package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DaemonSetsTable struct {
	namespace string
}

func (dt *DaemonSetsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().AppsV1().DaemonSets(dt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (dt *DaemonSetsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Daemon Set " + dt.namespace + "/" + row[0]
}

func (dt *DaemonSetsTable) TypeName() string {
	return "daemonset"
}

func (dt *DaemonSetsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (dt *DaemonSetsTable) Namespace() string {
	return dt.namespace
}

func (dt *DaemonSetsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(dt)
}

func NewDaemonSetsTable(namespace string) *widgets.DataTable {
	pt := &DaemonSetsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Daemon Sets"
	return lt
}

func (dt *DaemonSetsTable) LoadData() ([]widgets.ListRow, error) {
	sets, err := kube.GetClient().AppsV1().DaemonSets(dt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, set := range sets.Items {
		rows = append(rows, dt.newRow(set))
	}
	return rows, nil
}

func (dt *DaemonSetsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DESIRED", "CURRENT", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}
}

func (dt *DaemonSetsTable) newRow(set v1.DaemonSet) []string {
	return widgets.ListRow{
		set.Name,
		fmt.Sprintf("%d", set.Status.DesiredNumberScheduled),
		fmt.Sprintf("%d", set.Status.CurrentNumberScheduled),
		fmt.Sprintf("%d", set.Status.NumberReady),
		fmt.Sprintf("%d", set.Status.UpdatedNumberScheduled),
		fmt.Sprintf("%d", set.Status.NumberAvailable),
		Age(set.CreationTimestamp.Time),
	}
}
