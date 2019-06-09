package tui

import (
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PVCsTable struct {
	namespace string
}

func (pt *PVCsTable) Namespace() string {
	return pt.namespace
}

func (pt *PVCsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Persistent Volume " + row[0]
}

func (pt *PVCsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().PersistentVolumes().Delete(row[0], metav1.NewDeleteOptions(0))
}

func (pt *PVCsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(pt)
}

func (pt *PVCsTable) TypeName() string {
	return "pv"
}

func (pt *PVCsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewPVCsTable(namespace string) *widgets.DataTable {
	lt := widgets.NewDataTable(&PVCsTable{
		namespace: namespace,
	}, screen)
	lt.Title = "Persistent Volumes"
	return lt
}

func (pt *PVCsTable) LoadData() ([]widgets.ListRow, error) {
	pvcs, err := kube.GetClient().CoreV1().PersistentVolumeClaims(pt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, pvs := range pvcs.Items {
		rows = append(rows, pt.newRow(pvs))
	}
	return rows, nil
}

func (pt *PVCsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "STATUS", "VOLUME", "CAPACITY", "ACCESS MODES", "STORAGECLASS", "AGE"}
}

func (pt *PVCsTable) newRow(pv v1.PersistentVolumeClaim) widgets.ListRow {
	var accessModes []string
	for _, mode := range pv.Spec.AccessModes {
		switch mode {
		case v1.ReadWriteOnce:
			accessModes = append(accessModes, "RWO")
		case v1.ReadOnlyMany:
			accessModes = append(accessModes, "ROM")
		case v1.ReadWriteMany:
			accessModes = append(accessModes, "RWM")
		}
	}
	capacity := pv.Status.Capacity["storage"]
	return widgets.ListRow{
		pv.Name,
		string(pv.Status.Phase),
		pv.Spec.VolumeName,
		capacity.String(),
		strings.Join(accessModes, ","),
		*pv.Spec.StorageClassName,
		Age(pv.CreationTimestamp.Time),
	}
}
