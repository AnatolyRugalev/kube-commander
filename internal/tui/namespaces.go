package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespacesTable struct {
}

func (nt *NamespacesTable) GetActions() []*widgets.ListAction {
	return append(GetDefaultActions(nt), &widgets.ListAction{
		Name:          "Switch to",
		HotKey:        "s",
		HotKeyDisplay: "S",
		Func: func(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
			screen.SetNamespace(row[0])
			return true
		},
	})
}

func (nt *NamespacesTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "namespace " + row[0]
}

func (nt *NamespacesTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Namespaces().Delete(row[0], metav1.NewDeleteOptions(0))
}

func (nt *NamespacesTable) TypeName() string {
	return "namespace"
}

func (nt *NamespacesTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewNamespacesTable() *widgets.DataTable {
	lt := widgets.NewDataTable(&NamespacesTable{}, screen)
	lt.Title = "Namespaces"
	return lt
}

func (nt *NamespacesTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "STATUS", "AGE"}
}

func (nt *NamespacesTable) OnSelect(idx int, row widgets.ListRow) bool {
	screen.FocusToNamespace(row[0])
	return true
}

func (nt *NamespacesTable) LoadData() ([]widgets.ListRow, error) {
	namespaces, err := kube.GetClient().CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, ns := range namespaces.Items {
		rows = append(rows, nt.newRow(ns))
	}
	return rows, nil
}

func (nt *NamespacesTable) newRow(ns v1.Namespace) widgets.ListRow {
	return widgets.ListRow{
		ns.Name,
		string(ns.Status.Phase),
		Age(ns.CreationTimestamp.Time),
	}
}
