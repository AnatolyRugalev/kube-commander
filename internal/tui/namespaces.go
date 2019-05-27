package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesTable struct {
}

func NewNamespacesTable() *ListTable {
	lt := NewListTable(&NamespacesTable{})
	lt.Title = "Namespaces"
	return lt
}

func (nt *NamespacesTable) getTitleRow() []string {
	return []string{"NAME", "STATUS", "AGE"}
}

func (nt *NamespacesTable) OnSelect(item []string) bool {
	screen.LoadRightPane(NewPodsTable(item[0]))
	return true
}

func (nt *NamespacesTable) loadData() ([][]string, error) {
	client, err := kube.GetClient()
	if err != nil {
		return nil, err
	}
	namespaces, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
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
