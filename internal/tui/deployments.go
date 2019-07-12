package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentsTable struct {
	namespace string
}

func (dt *DeploymentsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().AppsV1().Deployments(dt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (dt *DeploymentsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "deployment " + dt.namespace + "/" + row[0]
}

func (dt *DeploymentsTable) TypeName() string {
	return "deployment"
}

func (dt *DeploymentsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (dt *DeploymentsTable) Namespace() string {
	return dt.namespace
}

func (dt *DeploymentsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(dt)
}

func NewDeploymentsTable(namespace string) *widgets.DataTable {
	pt := &DeploymentsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Deployments"
	return lt
}

func (dt *DeploymentsTable) LoadData() ([]widgets.ListRow, error) {
	deployments, err := kube.GetClient().AppsV1().Deployments(dt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, deployment := range deployments.Items {
		rows = append(rows, dt.newRow(deployment))
	}
	return rows, nil
}

func (dt *DeploymentsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DESIRED", "CURRENT", "UP-TO-DATE", "AVAILABLE", "AGE"}
}

func (dt *DeploymentsTable) newRow(deployment v1.Deployment) []string {
	return widgets.ListRow{
		deployment.Name,
		fmt.Sprintf("%d", *deployment.Spec.Replicas),
		fmt.Sprintf("%d", deployment.Status.ReadyReplicas),
		fmt.Sprintf("%d", deployment.Status.UpdatedReplicas),
		fmt.Sprintf("%d", deployment.Status.AvailableReplicas),
		Age(deployment.CreationTimestamp.Time),
	}
}
