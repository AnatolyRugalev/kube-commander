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
	workspace.ShowPopup("Select namespace", picker)
	currentNs := workspace.CurrentNamespace()
	for i := 0; i < len(picker.Rows()); i++ {
		meta, err := picker.RowMetadata(i)
		if err != nil {
			workspace.HandleError(err)
			return
		}
		if currentNs == meta.Name {
			picker.Select(i)
			break
		}
	}
	workspace.ScreenUpdater().UpdateScreen()
}

func NewNamespacePicker(container commander.ResourceContainer, resource *commander.Resource, f NamespaceFunc) (*listTable.ResourceListTable, error) {
	// TODO: all namespaces option
	rlt := listTable.NewResourceListTable(container, resource, &listTable.ResourceListTableOptions{
		ShowHeaders: false,
		Format:      listTable.FormatNameOnly,
	})
	rlt.BindOnKeyPress(func(rowId int, row commander.Row, event *tcell.EventKey) bool {
		if event.Key() == tcell.KeyEnter {
			go func() {
				metadata, err := rlt.RowMetadata(rowId)
				if err != nil {
					container.HandleError(err)
					return
				}
				f(metadata.Name)
			}()
			return true
		}
		return false
	})
	return rlt, nil
}
