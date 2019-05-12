package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesTable struct {
	*widgets.Table
}

func (pt *NamespacesTable) OnEvent(event *termui.Event) bool {
	return false
}

func (pt *NamespacesTable) OnFocusIn() {
}

func (pt *NamespacesTable) OnFocusOut() {
}

func NewNamespacesTable() *NamespacesTable {
	nt := &NamespacesTable{widgets.NewTable()}
	nt.Title = "Namespaces"
	nt.RowSeparator = false
	nt.resetRows()
	return nt
}

func (pt *NamespacesTable) resetRows() {
	pt.Rows = [][]string{
		pt.getTitleRow(),
	}
}

func (pt *NamespacesTable) getTitleRow() []string {
	return []string{"NAME", "STATUS", "AGE"}
}

func (pt *NamespacesTable) newRow(ns v1.Namespace) []string {
	return []string{
		ns.Name,
		string(ns.Status.Phase),
		Age(ns.CreationTimestamp.Time),
	}
}

func (pt *NamespacesTable) Reload() error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	namespaces, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	pt.resetRows()
	for _, ns := range namespaces.Items {
		pt.Rows = append(pt.Rows, pt.newRow(ns))
	}
	return nil
}
