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

func (pt *PVsTable) TypeName() string {
	return "pv"
}

func (pt *PVsTable) Name(item []string) string {
	return item[0]
}

func (pt *PVsTable) OnDelete(item []string) bool {
	name := item[0]
	ShowConfirmDialog("Are you sure you want to delete a PERSISTENT VOLUME "+name+" WITH ITS DATA?", func() error {
		return kube.GetClient().CoreV1().PersistentVolumes().Delete(name, metav1.NewDeleteOptions(0))
	})
	return true
}

func NewPVsTable() *widgets.ListTable {
	pt := &PVsTable{}
	al := NewActionList(true)
	al.AddAction("___________", "", false, nil)
	al.AddAction("Delete", "<Delete>", false, pt.OnDelete)
	lt := widgets.NewListTable(screen, pt, al)
	lt.Title = "Persistent Volumes"
	return lt
}

func (pt *PVsTable) GetHeaderRow() []string {
	return []string{"NAME", "CAPACITY", "ACCESS MODES", "RECLAIM POLICY", "STATUS", "CLAIM", "STORAGECLASS", "REASON", "AGE"}
}

func (pt *PVsTable) LoadData() ([][]string, error) {
	pvs, err := kube.GetClient().CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, pv := range pvs.Items {
		rows = append(rows, pt.newRow(pv))
	}
	return rows, nil
}

func (pt *PVsTable) newRow(pv v1.PersistentVolume) []string {
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
	return []string{
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
