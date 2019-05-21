package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	ui "github.com/gizak/termui/v3"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type NodesTable struct {
	*ListTable
}

func NewNodesTable() *NodesTable {
	nt := &NodesTable{NewListTable()}
	nt.Title = "Nodes"
	nt.RowSeparator = false
	nt.SelectedRowStyle = ui.NewStyle(ui.ColorYellow)
	nt.RowStyle = ui.NewStyle(ui.ColorWhite)
	nt.resetRows()
	return nt
}

func (nt *NodesTable) resetRows() {
	nt.Rows = [][]string{
		nt.getTitleRow(),
	}
}

func (nt *NodesTable) getTitleRow() []string {
	return []string{"NAME", "STATUS", "ROLES", "AGE", "VERSION"}
}

func (nt *NodesTable) newRow(n v1.Node) []string {
	var roles []string
	for l := range n.Labels {
		if strings.HasPrefix(l, "node-role.kubernetes.io/") {
			roles = append(roles, l[24:])
		}
	}
	if len(roles) == 0 {
		roles = []string{"<none>"}
	}

	var conditions []string
	for _, c := range n.Status.Conditions {
		if c.Status == v1.ConditionTrue {
			conditions = append(conditions, string(c.Type))
		}
	}

	return []string{
		n.Name,
		strings.Join(conditions, ","),
		strings.Join(roles, ","),
		Age(n.CreationTimestamp.Time),
		n.Status.NodeInfo.KubeletVersion,
	}
}

func (nt *NodesTable) OnFocusIn() {
	nt.ListTable.OnFocusIn()
}

func (nt *NodesTable) Reload() error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	namespaces, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	nt.resetRows()
	for _, ns := range namespaces.Items {
		nt.Rows = append(nt.Rows, nt.newRow(ns))
	}
	return nil
}
