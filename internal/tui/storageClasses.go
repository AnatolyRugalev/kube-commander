package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StorageClassesTable struct {
}

func (st *StorageClassesTable) DeleteDescription(row widgets.ListRow) string {
	return "Storage Class " + row[0]
}

func (st *StorageClassesTable) Delete(row widgets.ListRow) error {
	return kube.GetClient().StorageV1().StorageClasses().Delete(row[0], metav1.NewDeleteOptions(0))
}

func (st *StorageClassesTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(st)
}

func NewStorageClassesTable() *widgets.DataTable {
	lt := widgets.NewDataTable(&StorageClassesTable{}, screen)
	lt.Title = "Storage Classes"
	return lt
}

func (st *StorageClassesTable) TypeName() string {
	return "storageclass"
}

func (st *StorageClassesTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (st *StorageClassesTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "DEFAULT", "PROVISIONER", "AGE"}
}

func (st *StorageClassesTable) LoadData() ([]widgets.ListRow, error) {
	storageClasses, err := kube.GetClient().StorageV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, sc := range storageClasses.Items {
		rows = append(rows, st.newRow(sc))
	}
	return rows, nil
}

func (st *StorageClassesTable) newRow(sc storagev1.StorageClass) widgets.ListRow {
	def := "No"
	if d, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && d == "true" {
		def = "Yes"
	}
	return widgets.ListRow{
		sc.Name,
		def,
		sc.Provisioner,
		Age(sc.CreationTimestamp.Time),
	}
}
