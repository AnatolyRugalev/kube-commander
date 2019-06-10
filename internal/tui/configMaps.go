package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigMapsTable struct {
	namespace string
}

func (ct *ConfigMapsTable) Namespace() string {
	return ct.namespace
}

func (ct *ConfigMapsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(ct)
}

func (ct *ConfigMapsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Config Map " + row[0]
}

func (ct *ConfigMapsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().ConfigMaps(ct.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (ct *ConfigMapsTable) TypeName() string {
	return "configmaps"
}

func (ct *ConfigMapsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewConfigMapsTable(namespace string) *widgets.DataTable {
	lt := widgets.NewDataTable(&ConfigMapsTable{
		namespace: namespace,
	}, screen)
	lt.Title = "Config Maps"
	return lt
}

func (ct *ConfigMapsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DATA", "AGE"}
}

func (ct *ConfigMapsTable) LoadData() ([]widgets.ListRow, error) {
	configmaps, err := kube.GetClient().CoreV1().ConfigMaps(ct.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, cm := range configmaps.Items {
		rows = append(rows, ct.newRow(cm))
	}
	return rows, nil
}

func (ct *ConfigMapsTable) newRow(ns v1.ConfigMap) widgets.ListRow {
	return widgets.ListRow{
		ns.Name,
		fmt.Sprintf("%d", len(ns.Data)),
		Age(ns.CreationTimestamp.Time),
	}
}
