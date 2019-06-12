package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type ServicesTable struct {
	namespace string
}

func (ct *ServicesTable) Namespace() string {
	return ct.namespace
}

func (ct *ServicesTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(ct)
}

func (ct *ServicesTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Service " + row[0]
}

func (ct *ServicesTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Services(ct.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (ct *ServicesTable) TypeName() string {
	return "Services"
}

func (ct *ServicesTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewServicesTable(namespace string) *widgets.DataTable {
	lt := widgets.NewDataTable(&ServicesTable{
		namespace: namespace,
	}, screen)
	lt.Title = "Services"
	return lt
}

func (ct *ServicesTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT(S)", "AGE"}
}

func (ct *ServicesTable) LoadData() ([]widgets.ListRow, error) {
	services, err := kube.GetClient().CoreV1().Services(ct.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, service := range services.Items {
		rows = append(rows, ct.newRow(service))
	}
	return rows, nil
}

func (ct *ServicesTable) newRow(svc v1.Service) widgets.ListRow {
	var external string
	if svc.Spec.Type == v1.ServiceTypeExternalName {
		external = svc.Spec.ExternalName
	} else {
		external = strings.Join(svc.Spec.ExternalIPs, ",")
	}
	if external == "" {
		external = "<none>"
	}
	var ports []string
	for _, port := range svc.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
	}
	if len(ports) == 0 {
		ports = append(ports, "<none>")
	}
	return widgets.ListRow{
		svc.Name,
		string(svc.Spec.Type),
		svc.Spec.ClusterIP,
		external,
		strings.Join(ports, ","),
		Age(svc.CreationTimestamp.Time),
	}
}
