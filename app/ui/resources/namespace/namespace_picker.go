package namespace

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
)

type NamespaceFunc func(namespace string)

func PickNamespace(workspace commander.Workspace, resource *commander.Resource, f NamespaceFunc) {
	picker, err := NewNamespacePicker(workspace, resource, func(namespace string) {
		workspace.FocusManager().Blur()
		f(namespace)
	})
	if err != nil {
		workspace.HandleError(err)
		return
	}
	picker.SelectId(workspace.CurrentNamespace())
	workspace.ShowPopup("Select namespace", picker)
}

func NewNamespacePicker(container commander.ResourceContainer, resource *commander.Resource, f NamespaceFunc) (*listTable.ResourceListTable, error) {
	rlt := listTable.NewResourceListTable(container, resource, listTable.NameOnly|listTable.NoActions|listTable.NoWatch)
	rlt.BindOnKeyPress(func(row commander.Row, event *tcell.EventKey) bool {
		if event.Key() == tcell.KeyEnter {
			go func() {
				f(row.Id())
			}()
			return true
		}
		return false
	})
	rlt.SetExtraRows(map[int]commander.Row{
		0: commander.NewSimpleRow("", []string{"All Namespaces"}, true),
	})
	return rlt, nil
}
