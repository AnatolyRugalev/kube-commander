package tui

import (
	"fmt"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PVsTable struct {
}

func (pt *PVsTable) DeleteDescription(row widgets.ListRow) string {
	return "Persistent Volume " + row[0]
}

func (pt *PVsTable) Delete(row widgets.ListRow) error {
	return kube.GetClient().CoreV1().PersistentVolumes().Delete(row[0], metav1.NewDeleteOptions(0))
}

func (pt *PVsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(pt)
}

func (pt *PVsTable) TypeName() string {
	return "pv"
}

func (pt *PVsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewPVsTable() *widgets.DataTable {
	lt := widgets.NewDataTable(&PVsTable{}, screen)
	lt.Title = "Persistent Volumes"
	return lt
}

func (pt *PVsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "CAPACITY", "ACCESS MODES", "RECLAIM POLICY", "STATUS", "CLAIM", "STORAGECLASS", "REASON", "AGE"}
}

func (pt *PVsTable) LoadData() ([]widgets.ListRow, error) {
	pvs, err := kube.GetClient().CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, pv := range pvs.Items {
		rows = append(rows, pt.newRow(pv))
	}
	return rows, nil
}

func (pt *PVsTable) newRow(pv v1.PersistentVolume) widgets.ListRow {
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
	var claim string
	if pv.Spec.ClaimRef != nil {
		claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}
	capacity := pv.Spec.Capacity["storage"]
	return widgets.ListRow{
		pv.Name,
		capacity.String(),
		strings.Join(accessModes, ","),
		string(pv.Spec.PersistentVolumeReclaimPolicy),
		string(pv.Status.Phase),
		claim,
		pv.Spec.StorageClassName,
		pv.Status.Reason,
		Age(pv.CreationTimestamp.Time),
	}
}
