package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	ui "github.com/gizak/termui/v3"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesTable struct {
	*ListTable
}

func NewNamespacesTable() *NamespacesTable {
	nt := &NamespacesTable{NewListTable()}
	nt.Title = "Namespaces"
	nt.RowSeparator = false
	nt.SelectedRowStyle = ui.NewStyle(ui.ColorYellow)
	nt.RowStyle = ui.NewStyle(ui.ColorWhite)
	nt.resetRows()
	return nt
}

func (nt *NamespacesTable) resetRows() {
	nt.Rows = [][]string{
		nt.getTitleRow(),
	}
}

func (nt *NamespacesTable) getTitleRow() []string {
	return []string{"NAME", "STATUS", "AGE"}
}

func (nt *NamespacesTable) newRow(ns v1.Namespace) []string {
	return []string{
		ns.Name,
		string(ns.Status.Phase),
		Age(ns.CreationTimestamp.Time),
	}
}

func (nt *NamespacesTable) OnFocusIn() {
	nt.ListTable.OnFocusIn()
}

func (nt *NamespacesTable) Reload(errChan chan<- error) {
	client, err := kube.GetClient()
	if err != nil {
		errChan <- err
		return
	}
	namespaces, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		errChan <- err
		return
	}
	nt.resetRows()
	for _, ns := range namespaces.Items {
		nt.Rows = append(nt.Rows, nt.newRow(ns))
	}
	close(errChan)
}
