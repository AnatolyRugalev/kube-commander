package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesTable struct {
}

func (nt *NamespacesTable) TypeName() string {
	return "namespace"
}

func (nt *NamespacesTable) Name(item []string) string {
	return item[0]
}

func (nt *NamespacesTable) OnDelete(item []string) bool {
	name := item[0]
	ShowConfirmDialog("Are you sure you want to delete an ENTIRE NAMESPACE "+name+"?", func() error {
		return kube.GetClient().CoreV1().Namespaces().Delete(name, metav1.NewDeleteOptions(0))
	})
	return true
}

func NewNamespacesTable() *widgets.ListTable {
	lt := widgets.NewListTable(screen, &NamespacesTable{}, NewActionList(true))
	lt.Title = "Namespaces"
	return lt
}

func (nt *NamespacesTable) GetHeaderRow() []string {
	return []string{"NAME", "STATUS", "AGE"}
}

func (nt *NamespacesTable) OnSelect(item []string) bool {
	screen.LoadRightPane(NewPodsTable(item[0]))
	return true
}

func (nt *NamespacesTable) LoadData() ([][]string, error) {
	namespaces, err := kube.GetClient().CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, ns := range namespaces.Items {
		rows = append(rows, nt.newRow(ns))
	}
	return rows, nil
}

func (nt *NamespacesTable) newRow(ns v1.Namespace) []string {
	return []string{
		ns.Name,
		string(ns.Status.Phase),
		Age(ns.CreationTimestamp.Time),
	}
}
