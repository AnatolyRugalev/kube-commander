package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StorageClassesTable struct {
}

func NewStorageClassesTable() *widgets.ListTable {
	lt := widgets.NewListTable(screen, &StorageClassesTable{})
	lt.Title = "Storage Classes"
	return lt
}

func (st *StorageClassesTable) TypeName() string {
	return "storageclass"
}

func (st *StorageClassesTable) Name(item []string) string {
	return item[0]
}

func (st *StorageClassesTable) GetHeaderRow() []string {
	return []string{"NAME", "DEFAULT", "PROVISIONER", "AGE"}
}

func (st *StorageClassesTable) OnDelete(item []string) bool {
	name := item[0]
	ShowConfirmDialog("Are you sure you want to delete STORAGE CLASS "+name+"?", func() error {
		return kube.GetClient().StorageV1().StorageClasses().Delete(name, metav1.NewDeleteOptions(0))
	})
	return true
}

func (st *StorageClassesTable) LoadData() ([][]string, error) {
	storageClasses, err := kube.GetClient().StorageV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, sc := range storageClasses.Items {
		rows = append(rows, st.newRow(sc))
	}
	return rows, nil
}

func (st *StorageClassesTable) newRow(sc storagev1.StorageClass) []string {
	def := "No"
	if d, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && d == "true" {
		def = "Yes"
	}
	return []string{
		sc.Name,
		def,
		sc.Provisioner,
		Age(sc.CreationTimestamp.Time),
	}
}
